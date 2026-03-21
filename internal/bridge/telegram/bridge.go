package telegram

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"sync"
	"time"
)

// Bridge is the main lifecycle manager that wires all Telegram bridge
// components together. Use NewBridge to construct one, Run to start it,
// and Stop to shut it down.
type Bridge struct {
	cfg         Config
	sender      Sender
	townRoot    string
	inboxReader InboxReader // Optional: injected by daemon, nil = use CLIInboxReader

	mu     sync.Mutex
	cancel context.CancelFunc
}

// NewBridge creates a Bridge. If sender is nil, a CLISender backed by
// townRoot is used as the default.
func NewBridge(cfg Config, sender Sender, townRoot string) *Bridge {
	if sender == nil {
		sender = NewCLISender(townRoot)
	}
	return &Bridge{
		cfg:      cfg,
		sender:   sender,
		townRoot: townRoot,
	}
}

// Run validates the config, then enters a retry loop calling runOnce.
// If runOnce returns an error and ctx is not cancelled, it logs the error,
// waits 5 seconds, and retries. Run blocks until ctx is cancelled.
func (b *Bridge) Run(ctx context.Context) error {
	if err := b.cfg.Validate(); err != nil {
		return fmt.Errorf("bridge: config invalid: %w", err)
	}

	for {
		err := b.runOnce(ctx)
		if ctx.Err() != nil {
			// Context was cancelled — clean shutdown.
			return ctx.Err()
		}
		if err != nil {
			log.Printf("telegram bridge: run error (retrying in 5s): %v", err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
			}
		}
	}
}

// Stop cancels the current run cycle. It is safe to call from any goroutine.
func (b *Bridge) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cancel != nil {
		b.cancel()
	}
}

// runOnce performs a single connection-to-shutdown cycle:
//  1. Recovers from panics.
//  2. Connects to Telegram.
//  3. Starts the outbound notifier and reply forwarder in background goroutines.
//  4. Reads inbound messages from the bot and relays them until ctx is cancelled.
func (b *Bridge) runOnce(ctx context.Context) (retErr error) {
	// Recover from panics so the outer retry loop can restart cleanly.
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("telegram bridge: panic: %v", r)
		}
	}()

	runCtx, cancel := context.WithCancel(ctx)
	b.mu.Lock()
	b.cancel = cancel
	b.mu.Unlock()
	defer cancel()

	// Connect to Telegram.
	bot, err := NewBot(b.cfg)
	if err != nil {
		return fmt.Errorf("telegram bridge: connect: %w", err)
	}

	msgMap := NewMessageMap(10000)
	inbound := NewInboundRelay(b.sender, msgMap, b.cfg.Target)

	feedPath := filepath.Join(b.townRoot, ".feed.jsonl")
	outbound := NewOutboundNotifier(feedPath, b.cfg.Notify, bot, msgMap)

	var inboxReader InboxReader
	if b.inboxReader != nil {
		inboxReader = b.inboxReader
	} else {
		inboxReader = NewCLIInboxReader(b.townRoot)
	}

	replyFwd := NewReplyForwarder(bot, inboxReader, msgMap)

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		bot.Poll(runCtx)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		outbound.Run(runCtx)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		replyFwd.Run(runCtx)
	}()

	// Main loop: relay inbound messages until context is cancelled.
	for {
		select {
		case <-runCtx.Done():
			wg.Wait()
			return nil
		case msg, ok := <-bot.Messages():
			if !ok {
				wg.Wait()
				return nil
			}
			if err := inbound.Relay(runCtx, msg); err != nil {
				log.Printf("telegram bridge: relay error: %v", err)
			}
		}
	}
}
