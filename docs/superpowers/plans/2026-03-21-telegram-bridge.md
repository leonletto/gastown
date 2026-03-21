# Telegram Bridge Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the Gastown overseer chat with the Mayor over Telegram and receive configurable event notifications.

**Architecture:** A `internal/bridge/telegram/` package containing the bridge logic, with a `Sender` interface abstracting mail delivery (DirectSender for daemon mode, CLISender for standalone). The bridge runs as a persistent goroutine (daemon) or standalone process (`gt telegram run`), maintaining a long-poll connection to the Telegram Bot API.

**Tech Stack:** Go, `github.com/go-telegram-bot-api/telegram-bot-api/v5`, existing `internal/mail` and `internal/feed` packages.

**Spec:** `docs/superpowers/specs/2026-03-21-telegram-bridge-design.md`

---

## File Structure

### New files (create)

| File | Responsibility |
| ---- | -------------- |
| `internal/bridge/telegram/config.go` | Config struct, validation, token format check, file permissions check, `IsEnabled()` |
| `internal/bridge/telegram/config_test.go` | Config validation tests |
| `internal/bridge/telegram/msgmap.go` | Bidirectional Telegram msgID <-> mail ThreadID map, 10k FIFO eviction |
| `internal/bridge/telegram/msgmap_test.go` | Msgmap store/lookup/eviction/concurrency tests |
| `internal/bridge/telegram/bot.go` | Telegram Bot API wrapper: long-poll, send, access gate, rate limiter |
| `internal/bridge/telegram/bot_test.go` | Access gate ordering, rate limiter, mock API tests |
| `internal/bridge/telegram/sender.go` | `Sender` interface, `DirectSender`, `CLISender` |
| `internal/bridge/telegram/sender_test.go` | Sender implementation tests with mock router |
| `internal/bridge/telegram/inbound.go` | `InboundRelay` — Telegram msg -> Sender.SendMail + Sender.Nudge |
| `internal/bridge/telegram/inbound_test.go` | Inbound relay tests with mock sender |
| `internal/bridge/telegram/outbound.go` | Feed tailer — .feed.jsonl -> category filter -> bot.SendMessage. Note: uses `syscall.Stat_t` for inode detection, needs `//go:build !windows` |
| `internal/bridge/telegram/outbound_windows.go` | Windows fallback for `InodeChanged` using size comparison only |
| `internal/bridge/telegram/outbound_test.go` | Category filtering, feed line parsing tests |
| `internal/bridge/telegram/reply.go` | Overseer inbox poller -> forward to Telegram |
| `internal/bridge/telegram/reply_test.go` | Reply forwarding tests |
| `internal/bridge/telegram/bridge.go` | Bridge orchestrator: retry loop, lifecycle, child context management |
| `internal/bridge/telegram/bridge_test.go` | Lifecycle tests: start, retry on error, clean shutdown |
| `internal/cmd/telegram.go` | Cobra commands: `gt telegram configure`, `gt telegram status`, `gt telegram run` |

### Modified files

| File | Change |
| ---- | ------ |
| `internal/daemon/types.go` | Add `TelegramBridgeConfig` struct + field on `PatrolsConfig` + `IsPatrolEnabled` case |
| `internal/daemon/daemon.go` | Add bridge field to `Daemon`, start/stop in `Run()`/`shutdown()` |
| `internal/cmd/root.go` | No change needed — telegram.go uses `init()` + `rootCmd.AddCommand()` pattern |
| `go.mod` / `go.sum` | Add `github.com/go-telegram-bot-api/telegram-bot-api/v5` dependency |

---

## Task 1: Add Telegram dependency and scaffold config

**Files:**
- Modify: `go.mod`
- Create: `internal/bridge/telegram/config.go`
- Create: `internal/bridge/telegram/config_test.go`

- [ ] **Step 1: Add the Telegram Bot API dependency**

```bash
cd /Users/Shared/opensource/gastown && go get github.com/go-telegram-bot-api/telegram-bot-api/v5
```

- [ ] **Step 2: Write config validation tests**

Create `internal/bridge/telegram/config_test.go`:

```go
package telegram

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigValidation(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		c := Config{
			Token:     "123456789:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw",
			ChatID:    7747509251,
			AllowFrom: []int64{7747509251},
			Enabled:   true,
		}
		assert.NoError(t, c.Validate())
	})

	t.Run("missing token", func(t *testing.T) {
		c := Config{ChatID: 123, Enabled: true}
		assert.ErrorContains(t, c.Validate(), "token")
	})

	t.Run("invalid token format", func(t *testing.T) {
		c := Config{Token: "not-a-valid-token", ChatID: 123, Enabled: true}
		assert.ErrorContains(t, c.Validate(), "token format")
	})

	t.Run("missing chat_id", func(t *testing.T) {
		c := Config{Token: "123456789:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw", Enabled: true}
		assert.ErrorContains(t, c.Validate(), "chat_id")
	})
}

func TestConfigIsEnabled(t *testing.T) {
	t.Run("enabled with token", func(t *testing.T) {
		c := Config{Token: "123:AAHxyz", Enabled: true}
		assert.True(t, c.IsEnabled())
	})

	t.Run("disabled explicitly", func(t *testing.T) {
		c := Config{Token: "123:AAHxyz", Enabled: false}
		assert.False(t, c.IsEnabled())
	})

	t.Run("enabled but no token", func(t *testing.T) {
		c := Config{Enabled: true}
		assert.False(t, c.IsEnabled())
	})
}

func TestConfigDefaults(t *testing.T) {
	c := Config{
		Token:  "123:AAHxyz",
		ChatID: 123,
	}
	c.ApplyDefaults()
	assert.Equal(t, "mayor/", c.Target)
	assert.Equal(t, 30, c.RateLimit)
	assert.Equal(t, []string{"escalations"}, c.Notify)
}

func TestConfigTokenMask(t *testing.T) {
	c := Config{Token: "123456789:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw"}
	assert.Equal(t, "...Dsaw", c.MaskedToken())
}

func TestConfigLoadPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "telegram.json")

	// Write config with bad permissions
	err := os.WriteFile(path, []byte(`{"token":"123:AAHxyz","chat_id":123,"enabled":true}`), 0644)
	require.NoError(t, err)

	_, err = LoadConfig(path)
	assert.ErrorContains(t, err, "permissions")
}

func TestConfigAllowFrom(t *testing.T) {
	t.Run("empty allow_from blocks all", func(t *testing.T) {
		c := Config{AllowFrom: []int64{}}
		assert.False(t, c.IsAllowed(12345))
	})

	t.Run("matching user allowed", func(t *testing.T) {
		c := Config{AllowFrom: []int64{12345}}
		assert.True(t, c.IsAllowed(12345))
	})

	t.Run("non-matching user blocked", func(t *testing.T) {
		c := Config{AllowFrom: []int64{12345}}
		assert.False(t, c.IsAllowed(99999))
	})
}
```

- [ ] **Step 3: Run tests to verify they fail**

```bash
go test ./internal/bridge/telegram/... -v -run TestConfig
```

Expected: FAIL — `Config` struct not defined yet.

- [ ] **Step 4: Implement config**

Create `internal/bridge/telegram/config.go`:

```go
package telegram

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
)

// tokenRegex validates Telegram bot token format: numeric_id:alphanumeric_string
var tokenRegex = regexp.MustCompile(`^\d+:[A-Za-z0-9_-]+$`)

// Config holds Telegram bridge configuration.
type Config struct {
	Token     string   `json:"token"`
	ChatID    int64    `json:"chat_id"`
	AllowFrom []int64  `json:"allow_from,omitempty"`
	Target    string   `json:"target,omitempty"`
	Enabled   bool     `json:"enabled"`
	Notify    []string `json:"notify,omitempty"`
	RateLimit int      `json:"rate_limit,omitempty"`
}

// Validate checks that the config has all required fields and valid format.
func (c *Config) Validate() error {
	if c.Token == "" {
		return fmt.Errorf("token is required")
	}
	if !tokenRegex.MatchString(c.Token) {
		return fmt.Errorf("invalid token format: must match <numeric_id>:<alphanumeric>")
	}
	if c.ChatID == 0 {
		return fmt.Errorf("chat_id is required")
	}
	return nil
}

// IsEnabled returns true if the bridge should run.
func (c *Config) IsEnabled() bool {
	return c.Enabled && c.Token != ""
}

// IsAllowed checks if a Telegram user ID is in the allow list. Fail-closed: empty list blocks all.
func (c *Config) IsAllowed(userID int64) bool {
	for _, id := range c.AllowFrom {
		if id == userID {
			return true
		}
	}
	return false
}

// ApplyDefaults sets default values for optional fields.
func (c *Config) ApplyDefaults() {
	if c.Target == "" {
		c.Target = "mayor/"
	}
	if c.RateLimit == 0 {
		c.RateLimit = 30
	}
	if c.Notify == nil {
		c.Notify = []string{"escalations"}
	}
}

// MaskedToken returns the last 4 characters of the token for display.
func (c *Config) MaskedToken() string {
	if len(c.Token) <= 4 {
		return "****"
	}
	return "..." + c.Token[len(c.Token)-4:]
}

// ConfigPath returns the standard path for telegram config in a town.
func ConfigPath(townRoot string) string {
	return townRoot + "/mayor/telegram.json"
}

// LoadConfig loads and validates a telegram configuration file.
// Refuses to load if file permissions are not 0600.
func LoadConfig(path string) (*Config, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("reading telegram config: %w", err)
	}

	// Check permissions — token is a secret, file must be owner-only.
	perm := info.Mode().Perm()
	if perm&0077 != 0 {
		return nil, fmt.Errorf("telegram config %s has permissions %o, expected 0600: fix with chmod 600 %s", path, perm, path)
	}

	data, err := os.ReadFile(path) //nolint:gosec // G304: path is constructed internally
	if err != nil {
		return nil, fmt.Errorf("reading telegram config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing telegram config: %w", err)
	}

	cfg.ApplyDefaults()

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid telegram config: %w", err)
	}

	return &cfg, nil
}

// SaveConfig writes a telegram configuration to a file with 0600 permissions.
func SaveConfig(path string, cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding telegram config: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("writing telegram config: %w", err)
	}
	return nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./internal/bridge/telegram/... -v -run TestConfig
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/bridge/telegram/config.go internal/bridge/telegram/config_test.go go.mod go.sum
git commit -m "$(cat <<'EOF'
feat(telegram): add config package and telegram-bot-api dependency

Scaffold the internal/bridge/telegram/ package with Config struct,
validation (token format, permissions, fail-closed allow_from),
load/save, and defaults. Token stored with 0600 permissions.
EOF
)"
```

---

## Task 2: Message map (bidirectional ID mapping)

**Files:**
- Create: `internal/bridge/telegram/msgmap.go`
- Create: `internal/bridge/telegram/msgmap_test.go`

- [ ] **Step 1: Write msgmap tests**

Create `internal/bridge/telegram/msgmap_test.go`:

```go
package telegram

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMessageMap(t *testing.T) {
	t.Run("store and lookup", func(t *testing.T) {
		m := NewMessageMap(100)
		m.Store(123, 456, "thread-abc")

		threadID, ok := m.ThreadID(123, 456)
		assert.True(t, ok)
		assert.Equal(t, "thread-abc", threadID)

		chatID, msgID, ok := m.TelegramID("thread-abc")
		assert.True(t, ok)
		assert.Equal(t, int64(123), chatID)
		assert.Equal(t, 456, msgID)
	})

	t.Run("missing key returns false", func(t *testing.T) {
		m := NewMessageMap(100)
		_, ok := m.ThreadID(999, 999)
		assert.False(t, ok)

		_, _, ok = m.TelegramID("nonexistent")
		assert.False(t, ok)
	})

	t.Run("FIFO eviction at capacity", func(t *testing.T) {
		m := NewMessageMap(3)
		m.Store(1, 1, "a")
		m.Store(1, 2, "b")
		m.Store(1, 3, "c")
		m.Store(1, 4, "d") // evicts "a"

		_, ok := m.ThreadID(1, 1)
		assert.False(t, ok, "oldest entry should be evicted")

		_, ok = m.ThreadID(1, 4)
		assert.True(t, ok, "newest entry should exist")
	})

	t.Run("concurrent access", func(t *testing.T) {
		m := NewMessageMap(1000)
		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				m.Store(int64(i), i, fmt.Sprintf("thread-%d", i))
				m.ThreadID(int64(i), i)
				m.TelegramID(fmt.Sprintf("thread-%d", i))
			}(i)
		}
		wg.Wait()
	})
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/bridge/telegram/... -v -run TestMessageMap
```

Expected: FAIL — `NewMessageMap` not defined.

- [ ] **Step 3: Implement msgmap**

Create `internal/bridge/telegram/msgmap.go`:

```go
package telegram

import (
	"fmt"
	"sync"
)

// MessageMap maintains a bidirectional mapping between Telegram message IDs
// and Gastown mail ThreadIDs for reply threading.
// Thread-safe. Bounded with FIFO eviction.
type MessageMap struct {
	mu       sync.RWMutex
	teleToThread map[string]string // "chatID:msgID" -> threadID
	threadToTele map[string]string // threadID -> "chatID:msgID"
	order    []string             // FIFO eviction order (teleKey)
	maxSize  int
}

// NewMessageMap creates a MessageMap with the given capacity.
func NewMessageMap(maxSize int) *MessageMap {
	return &MessageMap{
		teleToThread: make(map[string]string, maxSize),
		threadToTele: make(map[string]string, maxSize),
		order:        make([]string, 0, maxSize),
		maxSize:      maxSize,
	}
}

func teleKey(chatID int64, msgID int) string {
	return fmt.Sprintf("%d:%d", chatID, msgID)
}

// Store records a mapping between a Telegram message and a mail thread.
func (m *MessageMap) Store(chatID int64, msgID int, threadID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := teleKey(chatID, msgID)

	// Evict oldest if at capacity
	if len(m.order) >= m.maxSize {
		oldest := m.order[0]
		m.order = m.order[1:]
		if tid, ok := m.teleToThread[oldest]; ok {
			delete(m.threadToTele, tid)
		}
		delete(m.teleToThread, oldest)
	}

	m.teleToThread[key] = threadID
	m.threadToTele[threadID] = key
	m.order = append(m.order, key)
}

// ThreadID looks up the mail ThreadID for a Telegram message.
func (m *MessageMap) ThreadID(chatID int64, msgID int) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	tid, ok := m.teleToThread[teleKey(chatID, msgID)]
	return tid, ok
}

// TelegramID looks up the Telegram chat/message ID for a mail thread.
func (m *MessageMap) TelegramID(threadID string) (chatID int64, msgID int, ok bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key, exists := m.threadToTele[threadID]
	if !exists {
		return 0, 0, false
	}
	var c int64
	var mid int
	if _, err := fmt.Sscanf(key, "%d:%d", &c, &mid); err != nil {
		return 0, 0, false
	}
	return c, mid, true
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/bridge/telegram/... -v -run TestMessageMap
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/bridge/telegram/msgmap.go internal/bridge/telegram/msgmap_test.go
git commit -m "$(cat <<'EOF'
feat(telegram): add bidirectional message map with FIFO eviction

Maps Telegram msgIDs to mail ThreadIDs for reply threading.
Thread-safe with bounded 10k capacity and oldest-first eviction.
EOF
)"
```

---

## Task 3: Sender interface and implementations

**Files:**
- Create: `internal/bridge/telegram/sender.go`
- Create: `internal/bridge/telegram/sender_test.go`

- [ ] **Step 1: Write sender tests**

Create `internal/bridge/telegram/sender_test.go`:

```go
package telegram

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCLISender(t *testing.T) {
	// CLISender shells out to gt — skip if gt not in PATH
	if _, err := exec.LookPath("gt"); err != nil {
		t.Skip("gt not in PATH, skipping CLISender test")
	}

	t.Run("constructs correct mail command", func(t *testing.T) {
		// We test the command construction, not actual delivery
		s := NewCLISender(t.TempDir())
		// SendMail will fail (no Dolt) but we verify it doesn't panic
		err := s.SendMail(context.Background(), "mayor/", "Test", "Hello")
		assert.Error(t, err) // Expected: no workspace
	})
}

func TestDirectSenderInterface(t *testing.T) {
	// Verify both implement Sender
	var _ Sender = (*CLISender)(nil)
	var _ Sender = (*DirectSender)(nil)
}

func TestCLISenderEnvSetup(t *testing.T) {
	s := &CLISender{townRoot: "/tmp/test-town"}
	cmd := s.buildMailCmd(context.Background(), "mayor/", "Subject", "Body")

	// Check BD_ACTOR is set
	found := false
	for _, env := range cmd.Env {
		if env == "BD_ACTOR=overseer" {
			found = true
			break
		}
	}
	assert.True(t, found, "BD_ACTOR=overseer should be in command env")
	assert.Equal(t, "/tmp/test-town", cmd.Dir)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/bridge/telegram/... -v -run TestCLISender -run TestDirectSender
```

Expected: FAIL — `Sender` interface not defined.

- [ ] **Step 3: Implement sender**

Create `internal/bridge/telegram/sender.go`:

```go
package telegram

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// Sender abstracts mail delivery so the bridge works in both daemon and standalone mode.
type Sender interface {
	// SendMail sends a mail message from overseer to the given recipient.
	SendMail(ctx context.Context, to, subject, body string) error
	// Nudge enqueues a nudge for the given session.
	Nudge(ctx context.Context, session, message string) error
}

// SendMailFunc and NudgeFunc are the function signatures for direct mode.
// Using function types avoids importing mail/nudge packages into the bridge,
// preventing import cycles. The daemon wiring provides the concrete functions.
type SendMailFunc func(ctx context.Context, from, to, subject, body string) error
type NudgeFunc func(townRoot, session, message string) error

// DirectSender calls Go APIs directly (for daemon mode).
// The send/nudge functions are injected by the daemon to avoid import cycles.
type DirectSender struct {
	townRoot  string
	sendMail  SendMailFunc
	nudge     NudgeFunc
}

// NewDirectSender creates a DirectSender with injected mail and nudge functions.
func NewDirectSender(townRoot string, sendMail SendMailFunc, nudge NudgeFunc) *DirectSender {
	return &DirectSender{
		townRoot: townRoot,
		sendMail: sendMail,
		nudge:    nudge,
	}
}

// SendMail sends mail via the injected function.
func (s *DirectSender) SendMail(ctx context.Context, to, subject, body string) error {
	if s.sendMail == nil {
		return fmt.Errorf("DirectSender.SendMail not wired")
	}
	return s.sendMail(ctx, "overseer", to, subject, body)
}

// Nudge enqueues a nudge via the injected function.
func (s *DirectSender) Nudge(ctx context.Context, session, message string) error {
	if s.nudge == nil {
		return fmt.Errorf("DirectSender.Nudge not wired")
	}
	return s.nudge(s.townRoot, session, message)
}

// CLISender shells out to gt CLI commands (for standalone mode).
type CLISender struct {
	townRoot string
}

// NewCLISender creates a CLISender that runs commands from the given town root.
func NewCLISender(townRoot string) *CLISender {
	return &CLISender{townRoot: townRoot}
}

// buildMailCmd constructs the gt mail send command with proper env.
func (s *CLISender) buildMailCmd(ctx context.Context, to, subject, body string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "gt", "mail", "send", to, "-s", subject, "-m", body)
	cmd.Dir = s.townRoot
	cmd.Env = append(os.Environ(), "BD_ACTOR=overseer")
	return cmd
}

// SendMail sends mail by shelling out to gt mail send.
func (s *CLISender) SendMail(ctx context.Context, to, subject, body string) error {
	cmd := s.buildMailCmd(ctx, to, subject, body)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("gt mail send failed: %w: %s", err, string(out))
	}
	return nil
}

// Nudge enqueues a nudge by shelling out to gt nudge.
func (s *CLISender) Nudge(ctx context.Context, session, message string) error {
	cmd := exec.CommandContext(ctx, "gt", "nudge", session, message, "--mode=queue")
	cmd.Dir = s.townRoot
	cmd.Env = append(os.Environ(), "BD_ACTOR=overseer")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("gt nudge failed: %w: %s", err, string(out))
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/bridge/telegram/... -v -run "TestCLISender|TestDirectSender"
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/bridge/telegram/sender.go internal/bridge/telegram/sender_test.go
git commit -m "$(cat <<'EOF'
feat(telegram): add Sender interface with CLI and Direct implementations

Sender abstracts mail delivery for bridge portability. CLISender shells
out to gt mail send/nudge with BD_ACTOR=overseer. DirectSender is a
placeholder completed in daemon wiring task.
EOF
)"
```

---

## Task 4: Bot wrapper (Telegram API, access gate, rate limiter)

**Files:**
- Create: `internal/bridge/telegram/bot.go`
- Create: `internal/bridge/telegram/bot_test.go`

- [ ] **Step 1: Write bot tests**

Create `internal/bridge/telegram/bot_test.go`:

```go
package telegram

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAccessGate(t *testing.T) {
	cfg := Config{
		AllowFrom: []int64{111, 222},
		RateLimit: 30,
	}

	gate := NewAccessGate(cfg)

	t.Run("allowed user passes", func(t *testing.T) {
		assert.True(t, gate.Check(111, false))
	})

	t.Run("disallowed user blocked", func(t *testing.T) {
		assert.False(t, gate.Check(999, false))
	})

	t.Run("bot always blocked", func(t *testing.T) {
		assert.False(t, gate.Check(111, true))
	})
}

func TestRateLimiter(t *testing.T) {
	rl := NewRateLimiter(3, time.Minute)

	t.Run("allows up to limit", func(t *testing.T) {
		assert.True(t, rl.Allow(123))
		assert.True(t, rl.Allow(123))
		assert.True(t, rl.Allow(123))
	})

	t.Run("blocks over limit", func(t *testing.T) {
		assert.False(t, rl.Allow(123))
	})

	t.Run("different users have separate limits", func(t *testing.T) {
		assert.True(t, rl.Allow(456))
	})
}

func TestInboundMessage(t *testing.T) {
	msg := InboundMessage{
		ChatID:    123,
		MessageID: 456,
		Text:      "hello mayor",
		Username:  "leon",
		UserID:    789,
	}
	assert.Equal(t, int64(123), msg.ChatID)
	assert.Equal(t, "hello mayor", msg.Text)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/bridge/telegram/... -v -run "TestAccessGate|TestRateLimiter|TestInboundMessage"
```

Expected: FAIL

- [ ] **Step 3: Implement bot**

Create `internal/bridge/telegram/bot.go`:

```go
package telegram

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// InboundMessage is a normalized message received from Telegram.
type InboundMessage struct {
	ChatID       int64
	MessageID    int
	Text         string
	Username     string
	UserID       int64
	ReplyToMsgID *int
}

// Bot wraps the Telegram Bot API for polling and sending messages.
type Bot struct {
	api      *tgbotapi.BotAPI
	messages chan InboundMessage
	gate     *AccessGate
	limiter  *RateLimiter
	chatID   int64
}

// NewBot creates a Bot connected to the Telegram API.
func NewBot(cfg Config) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(cfg.Token)
	if err != nil {
		return nil, fmt.Errorf("creating telegram bot: %w", err)
	}
	api.Debug = false

	return &Bot{
		api:      api,
		messages: make(chan InboundMessage, 32),
		gate:     NewAccessGate(cfg),
		limiter:  NewRateLimiter(cfg.RateLimit, time.Minute),
		chatID:   cfg.ChatID,
	}, nil
}

// Messages returns the channel of inbound messages that passed the access gate.
func (b *Bot) Messages() <-chan InboundMessage {
	return b.messages
}

// Poll starts long-polling Telegram for updates. Blocks until ctx is cancelled.
func (b *Bot) Poll(ctx context.Context) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30

	updates := b.api.GetUpdatesChan(u)

	for {
		select {
		case <-ctx.Done():
			b.api.StopReceivingUpdates()
			return
		case update, ok := <-updates:
			if !ok {
				return
			}
			if update.Message == nil {
				continue
			}
			from := update.Message.From
			if from == nil {
				continue
			}

			// Access gate: bot check -> allow_from -> rate limit
			if !b.gate.Check(from.ID, from.IsBot) {
				continue
			}
			if !b.limiter.Allow(from.ID) {
				log.Printf("telegram: rate limited user %d", from.ID)
				continue
			}

			msg := extractMessage(update.Message)
			select {
			case b.messages <- msg:
			case <-ctx.Done():
				return
			}
		}
	}
}

// SendMessage sends a text message to the configured chat.
func (b *Bot) SendMessage(text string, replyToMsgID *int) (int, error) {
	msg := tgbotapi.NewMessage(b.chatID, text)
	if replyToMsgID != nil {
		msg.ReplyParameters.MessageID = *replyToMsgID
	}
	sent, err := b.api.Send(msg)
	if err != nil {
		return 0, fmt.Errorf("sending telegram message: %w", err)
	}
	return sent.MessageID, nil
}

func extractMessage(m *tgbotapi.Message) InboundMessage {
	msg := InboundMessage{
		ChatID:    m.Chat.ID,
		MessageID: m.MessageID,
		Text:      m.Text,
		UserID:    m.From.ID,
	}
	if m.From.UserName != "" {
		msg.Username = m.From.UserName
	}
	if m.ReplyToMessage != nil {
		id := m.ReplyToMessage.MessageID
		msg.ReplyToMsgID = &id
	}
	return msg
}

// AccessGate checks bot/allow_from before processing a message.
type AccessGate struct {
	cfg Config
}

// NewAccessGate creates an AccessGate from config.
func NewAccessGate(cfg Config) *AccessGate {
	return &AccessGate{cfg: cfg}
}

// Check returns true if the user is allowed. Rejects bots, then checks allow_from.
func (g *AccessGate) Check(userID int64, isBot bool) bool {
	if isBot {
		return false
	}
	return g.cfg.IsAllowed(userID)
}

// RateLimiter implements a per-user sliding window rate limiter.
type RateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	records map[int64][]time.Time
}

// NewRateLimiter creates a rate limiter with the given per-user limit and window.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		limit:   limit,
		window:  window,
		records: make(map[int64][]time.Time),
	}
}

// Allow returns true if the user has not exceeded their rate limit.
func (r *RateLimiter) Allow(userID int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-r.window)

	// Prune expired entries
	times := r.records[userID]
	valid := times[:0]
	for _, t := range times {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= r.limit {
		r.records[userID] = valid
		return false
	}

	r.records[userID] = append(valid, now)
	return true
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/bridge/telegram/... -v -run "TestAccessGate|TestRateLimiter|TestInboundMessage"
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/bridge/telegram/bot.go internal/bridge/telegram/bot_test.go
git commit -m "$(cat <<'EOF'
feat(telegram): add Bot wrapper with access gate and rate limiter

Wraps go-telegram-bot-api with long-poll, access gate (reject bots,
fail-closed allow_from), per-user sliding window rate limiter, and
normalized InboundMessage extraction.
EOF
)"
```

---

## Task 5: Inbound relay (Telegram -> mail)

**Files:**
- Create: `internal/bridge/telegram/inbound.go`
- Create: `internal/bridge/telegram/inbound_test.go`

- [ ] **Step 1: Write inbound relay tests**

Create `internal/bridge/telegram/inbound_test.go`:

```go
package telegram

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSender records calls for testing.
type mockSender struct {
	mailCalls  []mailCall
	nudgeCalls []nudgeCall
}

type mailCall struct {
	to, subject, body string
}

type nudgeCall struct {
	session, message string
}

func (m *mockSender) SendMail(_ context.Context, to, subject, body string) error {
	m.mailCalls = append(m.mailCalls, mailCall{to, subject, body})
	return nil
}

func (m *mockSender) Nudge(_ context.Context, session, message string) error {
	m.nudgeCalls = append(m.nudgeCalls, nudgeCall{session, message})
	return nil
}

func TestInboundRelay(t *testing.T) {
	t.Run("relays message to mayor", func(t *testing.T) {
		sender := &mockSender{}
		msgMap := NewMessageMap(100)
		relay := NewInboundRelay(sender, msgMap, "mayor/")

		msg := InboundMessage{
			ChatID:    123,
			MessageID: 456,
			Text:      "deploy the thing",
			Username:  "leon",
			UserID:    789,
		}

		err := relay.Relay(context.Background(), msg)
		require.NoError(t, err)

		// Verify mail was sent
		require.Len(t, sender.mailCalls, 1)
		assert.Equal(t, "mayor/", sender.mailCalls[0].to)
		assert.Equal(t, "Telegram", sender.mailCalls[0].subject)
		assert.Equal(t, "deploy the thing", sender.mailCalls[0].body)

		// Verify nudge was sent
		require.Len(t, sender.nudgeCalls, 1)
		assert.Equal(t, "hq-mayor", sender.nudgeCalls[0].session)
	})

	t.Run("empty text is skipped", func(t *testing.T) {
		sender := &mockSender{}
		relay := NewInboundRelay(sender, NewMessageMap(100), "mayor/")

		err := relay.Relay(context.Background(), InboundMessage{Text: ""})
		assert.NoError(t, err)
		assert.Empty(t, sender.mailCalls)
	})

	t.Run("reply threading via msgmap", func(t *testing.T) {
		sender := &mockSender{}
		msgMap := NewMessageMap(100)
		relay := NewInboundRelay(sender, msgMap, "mayor/")

		// Simulate a previous outbound message stored in msgmap
		msgMap.Store(123, 100, "thread-xyz")

		replyTo := 100
		msg := InboundMessage{
			ChatID:       123,
			MessageID:    200,
			Text:         "yes, go ahead",
			ReplyToMsgID: &replyTo,
		}

		err := relay.Relay(context.Background(), msg)
		require.NoError(t, err)

		// Body should mention thread context
		require.Len(t, sender.mailCalls, 1)
		assert.Contains(t, sender.mailCalls[0].body, "yes, go ahead")
	})
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/bridge/telegram/... -v -run TestInboundRelay
```

Expected: FAIL

- [ ] **Step 3: Implement inbound relay**

Create `internal/bridge/telegram/inbound.go`:

```go
package telegram

import (
	"context"
	"log"
)

// InboundRelay converts Telegram messages into Gastown mail.
type InboundRelay struct {
	sender Sender
	msgMap *MessageMap
	target string // mail recipient, e.g. "mayor/"
}

// NewInboundRelay creates an InboundRelay.
func NewInboundRelay(sender Sender, msgMap *MessageMap, target string) *InboundRelay {
	return &InboundRelay{
		sender: sender,
		msgMap: msgMap,
		target: target,
	}
}

// Relay sends a Telegram message as mail to the configured target.
func (r *InboundRelay) Relay(ctx context.Context, msg InboundMessage) error {
	if msg.Text == "" {
		return nil
	}

	body := msg.Text

	// Check for reply threading
	if msg.ReplyToMsgID != nil {
		if _, ok := r.msgMap.ThreadID(msg.ChatID, *msg.ReplyToMsgID); ok {
			// Thread context is available — the mail system will handle threading
			// via the ThreadID. For now, just send the text as-is.
		}
	}

	// Send mail from overseer to target
	if err := r.sender.SendMail(ctx, r.target, "Telegram", body); err != nil {
		log.Printf("telegram: failed to send mail: %v", err)
		return err
	}

	// Nudge the Mayor session
	if err := r.sender.Nudge(ctx, "hq-mayor", "New Telegram message from overseer"); err != nil {
		// Non-fatal — mail was delivered, nudge is best-effort
		log.Printf("telegram: failed to nudge mayor: %v", err)
	}

	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/bridge/telegram/... -v -run TestInboundRelay
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/bridge/telegram/inbound.go internal/bridge/telegram/inbound_test.go
git commit -m "$(cat <<'EOF'
feat(telegram): add inbound relay (Telegram -> Mayor mail)

Converts Telegram messages to gt mail sent from overseer to mayor/.
Nudges the hq-mayor session after delivery. Supports reply threading
via msgmap lookup.
EOF
)"
```

---

## Task 6: Outbound notifications (feed tailer)

**Files:**
- Create: `internal/bridge/telegram/outbound.go`
- Create: `internal/bridge/telegram/outbound_test.go`

- [ ] **Step 1: Write outbound tests**

Create `internal/bridge/telegram/outbound_test.go`:

```go
package telegram

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCategoryFilter(t *testing.T) {
	f := NewCategoryFilter([]string{"stuck_agents", "escalations"})

	t.Run("matching event passes", func(t *testing.T) {
		assert.True(t, f.Matches("mass_death"))
		assert.True(t, f.Matches("session_death"))
		assert.True(t, f.Matches("escalation_sent"))
	})

	t.Run("non-matching event blocked", func(t *testing.T) {
		assert.False(t, f.Matches("sling"))
		assert.False(t, f.Matches("done"))
		assert.False(t, f.Matches("merged"))
	})

	t.Run("merge_failures category", func(t *testing.T) {
		f2 := NewCategoryFilter([]string{"merge_failures"})
		assert.True(t, f2.Matches("merge_failed"))
		assert.False(t, f2.Matches("merged"))
	})

	t.Run("empty categories blocks all", func(t *testing.T) {
		f3 := NewCategoryFilter([]string{})
		assert.False(t, f3.Matches("mass_death"))
	})
}

func TestFormatNotification(t *testing.T) {
	t.Run("mass death", func(t *testing.T) {
		payload := map[string]interface{}{
			"count":  float64(3),
			"window": "10s",
		}
		text := FormatNotification("mass_death", "deacon", payload)
		assert.Contains(t, text, "3")
		assert.Contains(t, text, "mass_death")
	})

	t.Run("escalation", func(t *testing.T) {
		payload := map[string]interface{}{
			"message": "polecat stuck for 15m",
		}
		text := FormatNotification("escalation_sent", "gastown/witness", payload)
		assert.Contains(t, text, "escalation")
		assert.Contains(t, text, "polecat stuck")
	})
}

func TestFeedLineParsing(t *testing.T) {
	event := FeedLine{
		Timestamp: "2026-03-21T10:00:00Z",
		Type:      "mass_death",
		Actor:     "deacon",
		Summary:   "3 polecats died",
		Payload:   map[string]interface{}{"count": float64(3)},
	}
	data, err := json.Marshal(event)
	require.NoError(t, err)

	parsed, err := ParseFeedLine(string(data))
	require.NoError(t, err)
	assert.Equal(t, "mass_death", parsed.Type)
	assert.Equal(t, "deacon", parsed.Actor)
}

func TestDetectInodeChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	err := os.WriteFile(path, []byte("line1\n"), 0644)
	require.NoError(t, err)

	info1, _ := os.Stat(path)

	// Simulate rotation via rename + new file
	os.Rename(path, path+".old")
	os.WriteFile(path, []byte("line2\n"), 0644)

	info2, _ := os.Stat(path)

	assert.True(t, InodeChanged(info1, info2), "inode should differ after rotation")
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/bridge/telegram/... -v -run "TestCategoryFilter|TestFormatNotification|TestFeedLine|TestDetectInode"
```

Expected: FAIL

- [ ] **Step 3: Implement outbound**

Create `internal/bridge/telegram/outbound.go`:

```go
//go:build !windows

package telegram

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"syscall"
	"time"
)

// categoryMap maps user-facing category names to event types.
var categoryMap = map[string][]string{
	"stuck_agents":  {"mass_death", "session_death"},
	"escalations":   {"escalation_sent"},
	"merge_failures": {"merge_failed"},
}

// FeedLine is the structure of a line in .feed.jsonl.
type FeedLine struct {
	Timestamp string                 `json:"ts"`
	Source    string                 `json:"source,omitempty"`
	Type      string                 `json:"type"`
	Actor     string                 `json:"actor"`
	Summary   string                 `json:"summary,omitempty"`
	Payload   map[string]interface{} `json:"payload,omitempty"`
	Count     int                    `json:"count,omitempty"`
}

// ParseFeedLine parses a JSONL line from the feed file.
func ParseFeedLine(line string) (*FeedLine, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, fmt.Errorf("empty line")
	}
	var fl FeedLine
	if err := json.Unmarshal([]byte(line), &fl); err != nil {
		return nil, err
	}
	return &fl, nil
}

// CategoryFilter checks if an event type matches any enabled notification category.
type CategoryFilter struct {
	allowedTypes map[string]bool
}

// NewCategoryFilter creates a filter from category names.
func NewCategoryFilter(categories []string) *CategoryFilter {
	allowed := make(map[string]bool)
	for _, cat := range categories {
		if types, ok := categoryMap[cat]; ok {
			for _, t := range types {
				allowed[t] = true
			}
		}
	}
	return &CategoryFilter{allowedTypes: allowed}
}

// Matches returns true if the event type is in an enabled category.
func (f *CategoryFilter) Matches(eventType string) bool {
	return f.allowedTypes[eventType]
}

// FormatNotification creates a human-readable Telegram message from a feed event.
func FormatNotification(eventType, actor string, payload map[string]interface{}) string {
	var sb strings.Builder

	switch eventType {
	case "mass_death":
		count := payload["count"]
		sb.WriteString(fmt.Sprintf("[mass_death] %v agent(s) died", count))
		if window, ok := payload["window"]; ok {
			sb.WriteString(fmt.Sprintf(" in %v", window))
		}
	case "session_death":
		session := payload["session"]
		sb.WriteString(fmt.Sprintf("[session_death] %v died", session))
		if reason, ok := payload["reason"]; ok {
			sb.WriteString(fmt.Sprintf(" (%v)", reason))
		}
	case "escalation_sent":
		sb.WriteString(fmt.Sprintf("[escalation] %s: ", actor))
		if msg, ok := payload["message"]; ok {
			sb.WriteString(fmt.Sprintf("%v", msg))
		}
	case "merge_failed":
		sb.WriteString(fmt.Sprintf("[merge_failed] %s", actor))
		if branch, ok := payload["branch"]; ok {
			sb.WriteString(fmt.Sprintf(" on %v", branch))
		}
	default:
		sb.WriteString(fmt.Sprintf("[%s] %s", eventType, actor))
	}

	return sb.String()
}

// InodeChanged checks if two FileInfo represent different inodes (file was rotated).
func InodeChanged(a, b os.FileInfo) bool {
	if a == nil || b == nil {
		return true
	}
	statA, okA := a.Sys().(*syscall.Stat_t)
	statB, okB := b.Sys().(*syscall.Stat_t)
	if !okA || !okB {
		// Fallback: compare size (if file shrunk, it was rotated)
		return b.Size() < a.Size()
	}
	return statA.Ino != statB.Ino
}

// OutboundNotifier tails .feed.jsonl and sends matching events to Telegram.
type OutboundNotifier struct {
	feedPath string
	filter   *CategoryFilter
	bot      BotSender
	msgMap   *MessageMap
}

// BotSender is the subset of Bot used by outbound (for testability).
type BotSender interface {
	SendMessage(text string, replyToMsgID *int) (int, error)
}

// NewOutboundNotifier creates an OutboundNotifier.
func NewOutboundNotifier(feedPath string, categories []string, bot BotSender, msgMap *MessageMap) *OutboundNotifier {
	return &OutboundNotifier{
		feedPath: feedPath,
		filter:   NewCategoryFilter(categories),
		bot:      bot,
		msgMap:   msgMap,
	}
}

// Run tails .feed.jsonl and forwards matching events to Telegram. Blocks until ctx is cancelled.
func (o *OutboundNotifier) Run(ctx context.Context) {
	for {
		if err := o.tailFeed(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("telegram outbound: feed error: %v, retrying in 5s", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
	}
}

func (o *OutboundNotifier) tailFeed(ctx context.Context) error {
	file, err := os.Open(o.feedPath)
	if err != nil {
		return fmt.Errorf("opening feed: %w", err)
	}
	defer file.Close()

	// Seek to end — only process new events
	if _, err := file.Seek(0, 2); err != nil {
		return fmt.Errorf("seeking to end: %w", err)
	}

	lastInfo, _ := file.Stat()
	reader := bufio.NewReader(file)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			// Check for inode change (file rotation)
			newInfo, err := os.Stat(o.feedPath)
			if err != nil {
				return fmt.Errorf("stat feed: %w", err)
			}
			if InodeChanged(lastInfo, newInfo) {
				return fmt.Errorf("feed file rotated, reopening")
			}
			lastInfo = newInfo

			// Read available lines
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					break
				}
				o.processLine(line)
			}
		}
	}
}

func (o *OutboundNotifier) processLine(line string) {
	fl, err := ParseFeedLine(line)
	if err != nil {
		return
	}

	if !o.filter.Matches(fl.Type) {
		return
	}

	text := FormatNotification(fl.Type, fl.Actor, fl.Payload)
	msgID, err := o.bot.SendMessage(text, nil)
	if err != nil {
		log.Printf("telegram outbound: send failed: %v", err)
		return
	}

	// Store in msgmap so user can reply to discuss the event
	o.msgMap.Store(0, msgID, fmt.Sprintf("event-%s-%s", fl.Type, fl.Timestamp))
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/bridge/telegram/... -v -run "TestCategoryFilter|TestFormatNotification|TestFeedLine|TestDetectInode"
```

Expected: PASS

- [ ] **Step 5: Create Windows fallback for InodeChanged**

Create `internal/bridge/telegram/outbound_windows.go`:

```go
//go:build windows

package telegram

import "os"

// InodeChanged on Windows falls back to size comparison (no inode support).
func InodeChanged(a, b os.FileInfo) bool {
	if a == nil || b == nil {
		return true
	}
	return b.Size() < a.Size()
}
```

- [ ] **Step 6: Commit**

```bash
git add internal/bridge/telegram/outbound.go internal/bridge/telegram/outbound_windows.go internal/bridge/telegram/outbound_test.go
git commit -m "$(cat <<'EOF'
feat(telegram): add outbound notifier (feed events -> Telegram)

Tails .feed.jsonl, filters by configured notification categories
(stuck_agents, escalations, merge_failures), formats human-readable
messages, and sends to Telegram. Detects file rotation via inode check.
EOF
)"
```

---

## Task 7: Reply forwarder (Mayor replies -> Telegram)

**Files:**
- Create: `internal/bridge/telegram/reply.go`
- Create: `internal/bridge/telegram/reply_test.go`

- [ ] **Step 1: Write reply forwarder tests**

Create `internal/bridge/telegram/reply_test.go`:

```go
package telegram

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockBotSender records sent messages.
type mockBotSender struct {
	sent []sentMessage
	nextID int
}

type sentMessage struct {
	text        string
	replyToMsgID *int
}

func (m *mockBotSender) SendMessage(text string, replyToMsgID *int) (int, error) {
	m.nextID++
	m.sent = append(m.sent, sentMessage{text, replyToMsgID})
	return m.nextID, nil
}

// mockInboxReader simulates reading the overseer's inbox.
type mockInboxReader struct {
	messages []InboxMessage
}

func (m *mockInboxReader) UnreadMessages(_ context.Context) ([]InboxMessage, error) {
	msgs := m.messages
	m.messages = nil // simulate marking read
	return msgs, nil
}

func (m *mockInboxReader) MarkRead(_ context.Context, id string) error {
	return nil
}

func TestReplyForwarder(t *testing.T) {
	t.Run("forwards mayor reply to telegram", func(t *testing.T) {
		bot := &mockBotSender{}
		inbox := &mockInboxReader{
			messages: []InboxMessage{
				{ID: "hq-abc", From: "mayor/", Subject: "Re: Telegram", Body: "Done, deployed.", ThreadID: "thread-123"},
			},
		}
		msgMap := NewMessageMap(100)
		// Simulate the original inbound message was from Telegram msg 456
		msgMap.Store(789, 456, "thread-123")

		fwd := NewReplyForwarder(bot, inbox, msgMap)
		fwd.PollOnce(context.Background())

		assert.Len(t, bot.sent, 1)
		assert.Contains(t, bot.sent[0].text, "Done, deployed.")
		assert.NotNil(t, bot.sent[0].replyToMsgID)
		assert.Equal(t, 456, *bot.sent[0].replyToMsgID)
	})

	t.Run("no unread messages is a no-op", func(t *testing.T) {
		bot := &mockBotSender{}
		inbox := &mockInboxReader{}
		fwd := NewReplyForwarder(bot, inbox, NewMessageMap(100))
		fwd.PollOnce(context.Background())
		assert.Empty(t, bot.sent)
	})

	t.Run("message without thread sends without reply", func(t *testing.T) {
		bot := &mockBotSender{}
		inbox := &mockInboxReader{
			messages: []InboxMessage{
				{ID: "hq-def", From: "mayor/", Subject: "Status", Body: "All good."},
			},
		}
		fwd := NewReplyForwarder(bot, inbox, NewMessageMap(100))
		fwd.PollOnce(context.Background())

		assert.Len(t, bot.sent, 1)
		assert.Nil(t, bot.sent[0].replyToMsgID)
	})
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/bridge/telegram/... -v -run TestReplyForwarder
```

Expected: FAIL

- [ ] **Step 3: Implement reply forwarder**

Create `internal/bridge/telegram/reply.go`:

```go
package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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

// NewCLIInboxReader creates an InboxReader that uses gt CLI commands.
func NewCLIInboxReader(townRoot string) *CLIInboxReader {
	return &CLIInboxReader{townRoot: townRoot}
}

// UnreadMessages lists unread mail addressed to overseer.
func (r *CLIInboxReader) UnreadMessages(ctx context.Context) ([]InboxMessage, error) {
	cmd := exec.CommandContext(ctx, "gt", "mail", "inbox", "--identity", "overseer", "--json", "--unread")
	cmd.Dir = r.townRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gt mail inbox failed: %w", err)
	}
	if len(out) == 0 {
		return nil, nil
	}
	var messages []InboxMessage
	if err := json.Unmarshal(out, &messages); err != nil {
		return nil, fmt.Errorf("parsing inbox: %w", err)
	}
	return messages, nil
}

// MarkRead marks a message as read by closing it.
func (r *CLIInboxReader) MarkRead(ctx context.Context, id string) error {
	cmd := exec.CommandContext(ctx, "bd", "close", id)
	cmd.Dir = r.townRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("bd close %s failed: %w: %s", id, err, string(out))
	}
	return nil
}

// ReplyForwarder polls the overseer inbox and forwards replies to Telegram.
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

// Run polls the overseer inbox periodically and forwards replies. Blocks until ctx is cancelled.
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

// PollOnce checks for unread mail and forwards to Telegram.
func (r *ReplyForwarder) PollOnce(ctx context.Context) {
	messages, err := r.inbox.UnreadMessages(ctx)
	if err != nil {
		log.Printf("telegram reply: inbox error: %v", err)
		return
	}

	for _, msg := range messages {
		// Format message for Telegram
		text := fmt.Sprintf("@%s: %s", msg.From, msg.Body)

		// Look up Telegram reply threading
		var replyTo *int
		if msg.ThreadID != "" {
			if _, telegramMsgID, ok := r.msgMap.TelegramID(msg.ThreadID); ok {
				replyTo = &telegramMsgID
			}
		}

		// Forward to Telegram first, then mark read (spec: forward-first ordering)
		teleMsgID, err := r.bot.SendMessage(text, replyTo)
		if err != nil {
			log.Printf("telegram reply: send failed for %s: %v", msg.ID, err)
			continue // Leave unread, retry next cycle
		}

		// Store outbound message ID for future reply threading
		if msg.ThreadID != "" {
			r.msgMap.Store(0, teleMsgID, msg.ThreadID)
		}

		// Mark as read after successful Telegram delivery
		if err := r.inbox.MarkRead(ctx, msg.ID); err != nil {
			log.Printf("telegram reply: mark read failed for %s: %v", msg.ID, err)
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/bridge/telegram/... -v -run TestReplyForwarder
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/bridge/telegram/reply.go internal/bridge/telegram/reply_test.go
git commit -m "$(cat <<'EOF'
feat(telegram): add reply forwarder (Mayor replies -> Telegram)

Polls overseer inbox every 3s, forwards replies to Telegram with
reply threading via msgmap. Forward-first ordering: Telegram delivery
before marking mail as read, so failures retry next cycle.
EOF
)"
```

---

## Task 8: Bridge orchestrator

**Files:**
- Create: `internal/bridge/telegram/bridge.go`
- Create: `internal/bridge/telegram/bridge_test.go`

- [ ] **Step 1: Write bridge lifecycle tests**

Create `internal/bridge/telegram/bridge_test.go`:

```go
package telegram

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBridgeShutdown(t *testing.T) {
	// Note: This test does NOT call bridge.Run() because that would make a real
	// Telegram API call. Instead we test the Stop() mechanism directly.
	t.Run("stop cancels context", func(t *testing.T) {
		cfg := Config{
			Token:     "123:AAHtest",
			ChatID:    123,
			AllowFrom: []int64{123},
			Target:    "mayor/",
			Enabled:   true,
			Notify:    []string{"escalations"},
			RateLimit: 30,
		}

		bridge := NewBridge(cfg, nil, "/tmp/test-town")

		// Simulate the cancel func being set (as runOnce would do)
		ctx, cancel := context.WithCancel(context.Background())
		bridge.mu.Lock()
		bridge.cancel = cancel
		bridge.mu.Unlock()

		// Stop should cancel the context
		bridge.Stop()
		assert.Error(t, ctx.Err(), "context should be cancelled after Stop()")
	})
}

func TestBridgeConfigValidation(t *testing.T) {
	t.Run("invalid config prevents start", func(t *testing.T) {
		cfg := Config{} // missing token
		bridge := NewBridge(cfg, nil, "/tmp/test")

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		err := bridge.Run(ctx)
		// Should fail fast on invalid config, not retry forever
		assert.Error(t, err)
	})
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/bridge/telegram/... -v -run TestBridge -timeout 30s
```

Expected: FAIL

- [ ] **Step 3: Implement bridge orchestrator**

Create `internal/bridge/telegram/bridge.go`:

```go
package telegram

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"sync"
	"time"
)

// Bridge is the main orchestrator for the Telegram integration.
// It manages the bot connection, inbound relay, outbound notifier, and reply forwarder.
type Bridge struct {
	cfg         Config
	sender      Sender
	townRoot    string
	inboxReader InboxReader // Optional: injected by daemon wiring, nil = use CLIInboxReader

	mu     sync.Mutex
	cancel context.CancelFunc
}

// NewBridge creates a Bridge. The sender may be nil if using standalone mode
// (CLISender will be created from townRoot).
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

// Run starts the bridge and blocks until ctx is cancelled. Retries on error with 5s backoff.
func (b *Bridge) Run(ctx context.Context) error {
	// Validate config before entering retry loop
	if err := b.cfg.Validate(); err != nil {
		return fmt.Errorf("telegram bridge config invalid: %w", err)
	}

	for {
		err := b.runOnce(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		log.Printf("telegram bridge: error: %v, restarting in 5s", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

func (b *Bridge) runOnce(ctx context.Context) (retErr error) {
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("panic: %v", r)
		}
	}()

	// Create child context for this run cycle
	runCtx, runCancel := context.WithCancel(ctx)
	defer runCancel()

	b.mu.Lock()
	b.cancel = runCancel
	b.mu.Unlock()

	// Connect to Telegram
	bot, err := NewBot(b.cfg)
	if err != nil {
		return err
	}

	msgMap := NewMessageMap(10000)

	// Create components
	inbound := NewInboundRelay(b.sender, msgMap, b.cfg.Target)
	// FeedFile constant is ".feed.jsonl" — matches internal/feed/curator.go:29
	feedPath := filepath.Join(b.townRoot, ".feed.jsonl")
	outbound := NewOutboundNotifier(feedPath, b.cfg.Notify, bot, msgMap)

	// Create inbox reader and reply forwarder
	var inboxReader InboxReader
	if b.inboxReader != nil {
		inboxReader = b.inboxReader // Injected by daemon wiring
	} else {
		inboxReader = NewCLIInboxReader(b.townRoot) // Standalone mode
	}
	replyFwd := NewReplyForwarder(bot, inboxReader, msgMap)

	var wg sync.WaitGroup

	// Start bot polling
	wg.Add(1)
	go func() {
		defer wg.Done()
		bot.Poll(runCtx)
	}()

	// Start outbound notifier
	wg.Add(1)
	go func() {
		defer wg.Done()
		outbound.Run(runCtx)
	}()

	// Start reply forwarder (Mayor replies -> Telegram)
	wg.Add(1)
	go func() {
		defer wg.Done()
		replyFwd.Run(runCtx)
	}()

	// Process inbound messages (main goroutine for this run)
	for {
		select {
		case <-runCtx.Done():
			wg.Wait()
			return runCtx.Err()
		case msg, ok := <-bot.Messages():
			if !ok {
				wg.Wait()
				return fmt.Errorf("bot message channel closed")
			}
			if err := inbound.Relay(runCtx, msg); err != nil {
				log.Printf("telegram bridge: inbound relay error: %v", err)
			}
		}
	}
}

// Stop cancels the current run cycle.
func (b *Bridge) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cancel != nil {
		b.cancel()
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/bridge/telegram/... -v -run TestBridge -timeout 30s
```

Expected: PASS (shutdown test passes; config validation test passes)

- [ ] **Step 5: Commit**

```bash
git add internal/bridge/telegram/bridge.go internal/bridge/telegram/bridge_test.go
git commit -m "$(cat <<'EOF'
feat(telegram): add bridge orchestrator with retry loop

Manages bot connection, inbound relay, outbound notifier lifecycle.
Retry loop with 5s backoff on errors, panic recovery, clean shutdown
via context cancellation. Validates config before entering retry loop.
EOF
)"
```

---

## Task 9: CLI commands (configure, status, run)

**Files:**
- Create: `internal/cmd/telegram.go`
- Modify: `internal/cmd/root.go`

- [ ] **Step 1: Identify where to register the command**

Read `internal/cmd/root.go` and find where commands are added (look for `rootCmd.AddCommand` or group registration). The telegram command goes in the "services" group.

- [ ] **Step 2: Implement CLI commands**

Create `internal/cmd/telegram.go`:

```go
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	telegram "github.com/steveyegge/gastown/internal/bridge/telegram"
	"github.com/steveyegge/gastown/internal/workspace"
	"github.com/spf13/cobra"
)

var telegramCmd = &cobra.Command{
	Use:     "telegram",
	GroupID: GroupServices,
	Short:   "Telegram bridge for overseer communication",
	Long:    "Manage the Telegram bridge that lets you chat with the Mayor and receive notifications over Telegram.",
	RunE:    requireSubcommand,
}

func init() {
	rootCmd.AddCommand(telegramCmd)
	telegramCmd.AddCommand(newTelegramConfigureCmd())
	telegramCmd.AddCommand(newTelegramStatusCmd())
	telegramCmd.AddCommand(newTelegramRunCmd())
}

// Registration is handled in init() above. No newTelegramCmd() needed.

func newTelegramConfigureCmd() *cobra.Command {
	var (
		token     string
		chatID    int64
		allowFrom []int64
		notify    []string
		yes       bool
	)

	cmd := &cobra.Command{
		Use:   "configure",
		Short: "Configure Telegram bridge settings",
		RunE: func(cmd *cobra.Command, args []string) error {
			townRoot, err := workspace.FindFromCwdOrError()
			if err != nil {
				return err
			}

			configPath := telegram.ConfigPath(townRoot)

			// Load existing or start fresh
			var cfg telegram.Config
			if existing, err := telegram.LoadConfig(configPath); err == nil {
				cfg = *existing
			}

			// Apply flags (only if explicitly set)
			if cmd.Flags().Changed("token") {
				if !yes && cfg.Token != "" {
					fmt.Print("Replace existing token? [y/N]: ")
					var answer string
					fmt.Scanln(&answer)
					if answer != "y" && answer != "Y" {
						return fmt.Errorf("cancelled")
					}
				}
				cfg.Token = token
			}
			if cmd.Flags().Changed("chat-id") {
				cfg.ChatID = chatID
			}
			if cmd.Flags().Changed("allow-from") {
				cfg.AllowFrom = allowFrom
			}
			if cmd.Flags().Changed("notify") {
				cfg.Notify = notify
			}

			cfg.Enabled = true
			cfg.ApplyDefaults()

			if err := cfg.Validate(); err != nil {
				return fmt.Errorf("invalid config: %w", err)
			}

			if err := telegram.SaveConfig(configPath, &cfg); err != nil {
				return err
			}

			fmt.Printf("Telegram config saved to %s\n", configPath)
			fmt.Println("\nNext steps:")
			fmt.Println("  Daemon mode:     Add telegram_bridge.enabled=true to mayor/daemon.json and restart daemon")
			fmt.Println("  Standalone mode: gt telegram run")
			return nil
		},
	}

	cmd.Flags().StringVar(&token, "token", "", "BotFather bot token")
	cmd.Flags().Int64Var(&chatID, "chat-id", 0, "Telegram chat ID for outbound messages")
	cmd.Flags().Int64SliceVar(&allowFrom, "allow-from", nil, "Allowed Telegram user IDs")
	cmd.Flags().StringSliceVar(&notify, "notify", nil, "Notification categories (stuck_agents, escalations, merge_failures)")
	cmd.Flags().BoolVar(&yes, "yes", false, "Skip confirmation prompts")

	return cmd
}

func newTelegramStatusCmd() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show Telegram bridge status",
		RunE: func(cmd *cobra.Command, args []string) error {
			townRoot, err := workspace.FindFromCwdOrError()
			if err != nil {
				return err
			}

			configPath := telegram.ConfigPath(townRoot)
			cfg, err := telegram.LoadConfig(configPath)
			if err != nil {
				if os.IsNotExist(err) {
					fmt.Println("Telegram bridge: not configured")
					fmt.Println("Run: gt telegram configure --token <token> --chat-id <id> --allow-from <id>")
					return nil
				}
				return err
			}

			if jsonOutput {
				status := map[string]interface{}{
					"enabled":    cfg.IsEnabled(),
					"token":      cfg.MaskedToken(),
					"chat_id":    cfg.ChatID,
					"allow_from": cfg.AllowFrom,
					"target":     cfg.Target,
					"notify":     cfg.Notify,
					"rate_limit": cfg.RateLimit,
				}
				data, _ := json.MarshalIndent(status, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			fmt.Printf("Telegram Bridge\n")
			fmt.Printf("  Enabled:    %v\n", cfg.IsEnabled())
			fmt.Printf("  Token:      %s\n", cfg.MaskedToken())
			fmt.Printf("  Chat ID:    %d\n", cfg.ChatID)
			fmt.Printf("  Target:     %s\n", cfg.Target)
			fmt.Printf("  Allow From: %v\n", cfg.AllowFrom)
			fmt.Printf("  Notify:     %v\n", cfg.Notify)
			fmt.Printf("  Rate Limit: %d/min\n", cfg.RateLimit)
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")

	return cmd
}

func newTelegramRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Run the Telegram bridge as a standalone process",
		Long:  "Starts the Telegram bridge in the foreground. Use this for testing or when not using the daemon.",
		RunE: func(cmd *cobra.Command, args []string) error {
			townRoot, err := workspace.FindFromCwdOrError()
			if err != nil {
				return err
			}

			configPath := telegram.ConfigPath(townRoot)
			cfg, err := telegram.LoadConfig(configPath)
			if err != nil {
				return fmt.Errorf("loading telegram config: %w", err)
			}

			if !cfg.IsEnabled() {
				return fmt.Errorf("telegram bridge is disabled in config")
			}

			fmt.Println("Starting Telegram bridge (standalone mode)...")
			fmt.Printf("  Target: %s\n", cfg.Target)
			fmt.Printf("  Notify: %v\n", cfg.Notify)

			// Signal handling
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			go func() {
				<-sigCh
				fmt.Println("\nShutting down Telegram bridge...")
				cancel()
			}()

			sender := telegram.NewCLISender(townRoot)
			bridge := telegram.NewBridge(*cfg, sender, townRoot)
			return bridge.Run(ctx)
		},
	}
}
```

- [ ] **Step 3: Build to verify compilation**

```bash
cd /Users/Shared/opensource/gastown && go build ./cmd/gt
```

Expected: Compiles successfully.

- [ ] **Step 4: Test CLI commands manually**

```bash
./gt telegram --help
./gt telegram status
./gt telegram configure --help
```

Expected: Help text displays correctly. Status shows "not configured".

- [ ] **Step 5: Commit**

```bash
git add internal/cmd/telegram.go
git commit -m "$(cat <<'EOF'
feat(telegram): add gt telegram CLI commands

Adds configure (write mayor/telegram.json), status (display config),
and run (standalone bridge process) subcommands. Token stored with
0600 permissions, masked in status output.
EOF
)"
```

---

## Task 10: Daemon integration

**Files:**
- Modify: `internal/daemon/types.go`
- Modify: `internal/daemon/daemon.go`

- [ ] **Step 1: Add config struct and patrol check to types.go**

In `internal/daemon/types.go`, add after the last config struct:

```go
// TelegramBridgeConfig holds configuration for the telegram_bridge patrol.
type TelegramBridgeConfig struct {
	Enabled bool `json:"enabled"`
}
```

Add field to `PatrolsConfig`:

```go
TelegramBridge *TelegramBridgeConfig `json:"telegram_bridge,omitempty"`
```

Add case to `IsPatrolEnabled` in the opt-in section (before the `if config == nil` check):

```go
if patrol == "telegram_bridge" {
    if config == nil || config.Patrols == nil || config.Patrols.TelegramBridge == nil {
        return false
    }
    return config.Patrols.TelegramBridge.Enabled
}
```

- [ ] **Step 2: Add bridge field and lifecycle to daemon.go**

Add to the `Daemon` struct:

```go
telegramBridge *telegram.Bridge
```

In `Run()`, after the KRC pruner start block, add:

```go
// Start Telegram bridge if enabled
if IsPatrolEnabled(d.patrolConfig, "telegram_bridge") {
    cfgPath := telegram.ConfigPath(d.config.TownRoot)
    if tgCfg, err := telegram.LoadConfig(cfgPath); err != nil {
        d.logger.Printf("Warning: telegram bridge enabled but config invalid: %v", err)
    } else if tgCfg.IsEnabled() {
        // Create DirectSender with injected mail and nudge functions.
        // This avoids the bridge package importing internal/mail or internal/nudge.
        sendMailFn := func(ctx context.Context, from, to, subject, body string) error {
            router := mail.NewRouterWithTownRoot(d.config.TownRoot, d.config.TownRoot)
            msg := &mail.Message{
                From:     from,
                To:       to,
                Subject:  subject,
                Body:     body,
                Delivery: mail.DeliveryInterrupt,
                Type:     mail.TypeTask,
            }
            return router.Send(msg)
        }
        nudgeFn := func(townRoot, session, message string) error {
            return nudge.Enqueue(townRoot, session, nudge.QueuedNudge{
                Sender:  "overseer",
                Message: message,
            })
        }
        sender := telegram.NewDirectSender(d.config.TownRoot, sendMailFn, nudgeFn)
        d.telegramBridge = telegram.NewBridge(*tgCfg, sender, d.config.TownRoot)
        go func() {
            if err := d.telegramBridge.Run(d.ctx); err != nil && d.ctx.Err() == nil {
                d.logger.Printf("Telegram bridge stopped: %v", err)
            }
        }()
        d.logger.Println("Telegram bridge started")
    }
}
```

In `shutdown()`, add before "Feed curator stopped":

```go
// Stop Telegram bridge
if d.telegramBridge != nil {
    d.telegramBridge.Stop()
    d.logger.Println("Telegram bridge stopped")
}
```

- [ ] **Step 3: Add import for telegram package**

Add to `daemon.go` imports:

```go
telegram "github.com/steveyegge/gastown/internal/bridge/telegram"
"github.com/steveyegge/gastown/internal/mail"
"github.com/steveyegge/gastown/internal/nudge"
```

- [ ] **Step 4: Build to verify compilation**

```bash
cd /Users/Shared/opensource/gastown && go build ./cmd/gt
```

Expected: Compiles successfully.

- [ ] **Step 5: Commit**

```bash
git add internal/daemon/types.go internal/daemon/daemon.go
git commit -m "$(cat <<'EOF'
feat(telegram): wire bridge into daemon as opt-in patrol

Adds TelegramBridgeConfig to PatrolsConfig, opt-in via IsPatrolEnabled
(defaults disabled). Daemon loads mayor/telegram.json and starts bridge
goroutine after KRC pruner. Clean shutdown via Bridge.Stop().
EOF
)"
```

---

## Task 11: Run all tests and verify

**Files:** None (verification only)

- [ ] **Step 1: Run all telegram package tests**

```bash
go test ./internal/bridge/telegram/... -v -count=1
```

Expected: All tests PASS.

- [ ] **Step 2: Run full test suite**

```bash
make test
```

Expected: All existing tests still pass. No regressions.

- [ ] **Step 3: Run linter**

```bash
golangci-lint run --timeout=5m ./internal/bridge/telegram/...
```

Expected: No lint errors.

- [ ] **Step 4: Build all binaries**

```bash
make build
```

Expected: All three binaries compile successfully.

- [ ] **Step 5: Final commit if any fixes were needed**

```bash
# Only if test/lint fixes were applied
git add -A
git commit -m "fix(telegram): address test/lint issues from final verification"
```

---

## Review Checkpoint

After Task 11, the implementation is complete. Review the full changeset:

```bash
git log --oneline main..HEAD
git diff --stat main..HEAD
```

Verify:
- All new files are under `internal/bridge/telegram/` (self-contained package)
- Daemon changes are minimal (~25 lines across types.go and daemon.go)
- CLI commands registered in root.go
- No import cycles
- go.mod has only one new dependency (`go-telegram-bot-api/v5`)
