package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"
)

// InboxMessage represents a mail message from the overseer's inbox.
type InboxMessage struct {
	ID       string
	From     string
	Subject  string
	Body     string
	ThreadID string
}

// InboxReader abstracts reading the overseer's mail inbox.
type InboxReader interface {
	UnreadMessages(ctx context.Context) ([]InboxMessage, error)
	MarkRead(ctx context.Context, id string) error
}

// CLIInboxReader reads the overseer inbox by shelling out to gt mail.
type CLIInboxReader struct {
	townRoot string
}

// NewCLIInboxReader creates a CLIInboxReader rooted at townRoot.
func NewCLIInboxReader(townRoot string) *CLIInboxReader {
	return &CLIInboxReader{townRoot: townRoot}
}

// gtMailMessage is the JSON structure returned by `gt mail inbox --json`.
type gtMailMessage struct {
	ID       string `json:"id"`
	From     string `json:"from"`
	Subject  string `json:"subject"`
	Body     string `json:"body"`
	ThreadID string `json:"thread_id"`
}

// UnreadMessages runs `gt mail inbox --identity overseer --json --unread`
// and returns parsed messages.
func (r *CLIInboxReader) UnreadMessages(ctx context.Context) ([]InboxMessage, error) {
	cmd := exec.CommandContext(ctx, "gt", "mail", "inbox", "--identity", "overseer", "--json", "--unread")
	cmd.Dir = r.townRoot
	cmd.Env = append(os.Environ(), "BD_ACTOR=overseer")

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gt mail inbox: %w", err)
	}

	var raw []gtMailMessage
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("gt mail inbox: parse JSON: %w", err)
	}

	msgs := make([]InboxMessage, 0, len(raw))
	for _, m := range raw {
		msgs = append(msgs, InboxMessage{
			ID:       m.ID,
			From:     m.From,
			Subject:  m.Subject,
			Body:     m.Body,
			ThreadID: m.ThreadID,
		})
	}
	return msgs, nil
}

// MarkRead runs `bd close <id>` to mark a message as read.
func (r *CLIInboxReader) MarkRead(ctx context.Context, id string) error {
	cmd := exec.CommandContext(ctx, "bd", "close", id)
	cmd.Dir = r.townRoot
	cmd.Env = append(os.Environ(), "BD_ACTOR=overseer")

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("bd close %s: %w: %s", id, err, out)
	}
	return nil
}

// ReplyForwarder polls the overseer's mail inbox and forwards Mayor replies
// to Telegram.
type ReplyForwarder struct {
	bot    BotSender
	inbox  InboxReader
	msgMap *MessageMap
}

// NewReplyForwarder creates a ReplyForwarder.
func NewReplyForwarder(bot BotSender, inbox InboxReader, msgMap *MessageMap) *ReplyForwarder {
	return &ReplyForwarder{
		bot:    bot,
		inbox:  inbox,
		msgMap: msgMap,
	}
}

// Run polls the inbox every 3 seconds, forwarding new messages to Telegram.
// It blocks until ctx is cancelled.
func (r *ReplyForwarder) Run(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.PollOnce(ctx)
		}
	}
}

// PollOnce performs a single poll cycle: reads unread messages and forwards
// each to Telegram. Forward-first ordering ensures failed sends are retried
// on the next cycle (the message stays unread until Telegram delivery succeeds).
func (r *ReplyForwarder) PollOnce(ctx context.Context) {
	msgs, err := r.inbox.UnreadMessages(ctx)
	if err != nil {
		log.Printf("reply forwarder: UnreadMessages: %v", err)
		return
	}

	for _, msg := range msgs {
		text := fmt.Sprintf("@%s: %s", msg.From, msg.Body)

		// Look up reply threading via the message map.
		var replyTo *int
		if msg.ThreadID != "" {
			if _, msgID, ok := r.msgMap.TelegramID(msg.ThreadID); ok {
				id := msgID
				replyTo = &id
			}
		}

		// Forward to Telegram FIRST. If this fails, leave the message unread
		// so it will be retried on the next cycle.
		sentID, err := r.bot.SendMessage(text, replyTo)
		if err != nil {
			log.Printf("reply forwarder: SendMessage: %v (will retry)", err)
			continue
		}

		// Store the outbound Telegram message ID for future threading.
		// We don't have a chatID here, so we use 0 as a placeholder.
		// The mapping is keyed by threadID so lookup still works.
		if msg.ThreadID != "" {
			r.msgMap.Store(0, sentID, msg.ThreadID)
		}

		// Mark read only after successful Telegram delivery.
		if err := r.inbox.MarkRead(ctx, msg.ID); err != nil {
			log.Printf("reply forwarder: MarkRead %s: %v", msg.ID, err)
		}
	}
}
