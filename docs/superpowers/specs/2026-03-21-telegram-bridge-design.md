# Telegram Bridge for Gastown

> Spec: 2026-03-21 | Status: Reviewed

## Problem

Gastown's human overseer can only interact with the Mayor through `gt mayor attach` (tmux session) or ACP (IDE integration). There's no way to send commands or receive notifications from a mobile device or when away from the terminal.

## Solution

A Telegram bridge that lets the overseer chat with the Mayor over Telegram and receive configurable notifications about workspace problems. The bridge reuses Gastown's existing `overseer` identity and mail system — no new identity or messaging concepts needed.

## Architecture

```
Telegram Bot API (long-poll)
       ↕
  Bridge.Run() — persistent retry loop
  ├── bot.Poll()        — long-poll goroutine (keeps connection alive)
  ├── inbound loop      — Telegram msg → Sender.SendMail() → Mayor
  ├── outbound replies  — poll overseer inbox → Telegram
  └── outbound notifs   — tail .feed.jsonl → filter categories → Telegram
       ↕
  Sender interface
  ├── DirectSender — mail.Router.Send() + nudge.Enqueue() (daemon mode)
  └── CLISender    — gt mail send + gt nudge CLI (standalone mode)
```

### Inbound: Telegram → Mayor

1. `bot.Poll()` long-polls Telegram with 30s timeout
2. Access gate: reject bots → check `allow_from` (fail-closed) → rate limit (30/min)
3. `inbound.relay()` sends mail: `from: "overseer"`, `to: "mayor/"`, `subject: "Telegram"`, `body: <text>`, `delivery: interrupt`
4. Nudge enqueued to `hq-mayor` session (session name from `session.MayorSessionName()`)
5. Mayor receives on next turn via `UserPromptSubmit` hook (`gt mail check --inject`)
6. Mail `ThreadID` stored in msgmap keyed by Telegram msgID for reply threading

### Outbound: Mayor Replies → Telegram

1. Bridge polls for unread mail addressed to `overseer` every 2-3s
   - Daemon mode: `mail.Mailbox.List()`
   - Standalone mode: `gt mail inbox --identity overseer --json`
2. New messages forwarded to Telegram
3. If message is a reply-to a Telegram-originated thread, msgmap looks up the mail `ThreadID` → original Telegram msgID to set `reply_to_message_id` for Telegram threading
4. Forward to Telegram first, then mark mail as read/closed. If Telegram send fails, the mail stays unread and will be retried next poll cycle

### Outbound: Event Notifications → Telegram

1. Outbound goroutine seeks to end of `.feed.jsonl`, polls every 100ms
2. Filters `FeedEvent.Type` against configured `notify` categories
3. Formats human-readable message
4. Sends via `bot.SendMessage(chatID, text)`
5. Stores Telegram msgID in msgmap (user can reply to discuss the problem)

### Notification Categories

| Category | Event types | Description |
|----------|------------|-------------|
| `stuck_agents` | `mass_death`, `session_death` | Agent sessions dying unexpectedly |
| `escalations` | `escalation_sent` | Problems agents couldn't auto-resolve |
| `merge_failures` | `merge_failed` | Refinery merge queue failures |

Default: `["escalations"]` — minimal, user opts in to more.

Note: `convoy_complete` is omitted from v1 — there is no convoy-level completion event in the feed system today. Individual polecat `done` events exist but would be too noisy. A `convoy_closed` event type can be added later if desired.

## Deployment Modes

Both modes use identical bridge code. The only difference is the `Sender` implementation.

### Daemon Goroutine (recommended)

Enabled via `mayor/daemon.json`:

```json
{
  "patrols": {
    "telegram_bridge": {
      "enabled": true
    }
  }
}
```

Bridge starts after the feed curator in `daemon.Run()`, uses `DirectSender` (Go API calls to `mail.Router.Send()` and `nudge.Enqueue()`).

### Standalone Process

```bash
gt telegram run
```

Long-running process using `CLISender` (shells out to `gt mail send` and `gt nudge`). Same signal handler pattern as `nudge-poller`. Useful for testing or if the user prefers not to modify the daemon.

## Sender Interface

```go
type Sender interface {
    SendMail(ctx context.Context, to, subject, body string) error
    Nudge(ctx context.Context, session, message string) error
}
```

Both implementations are stateful — constructed with the dependencies they need:

- `DirectSender` — constructed with `townRoot string` and a `*mail.Router`. Calls `router.Send()` for mail and `nudge.Enqueue(townRoot, session, ...)` for nudges. The `From` field is set to `"overseer"` on the `mail.Message` struct directly.
- `CLISender` — constructed with `townRoot string`. Execs `gt mail send` and `gt nudge` from the town root directory. The sender identity is auto-detected as `overseer` from the workspace context (the bridge runs in the town root, not inside a rig or agent directory). The `--actor overseer` environment variable (`BD_ACTOR=overseer`) is set on the subprocess to ensure correct attribution.

## Configuration

### `mayor/telegram.json`

```json
{
  "token": "123456789:AAH...",
  "chat_id": 7747509251,
  "allow_from": [7747509251],
  "target": "mayor/",
  "enabled": true,
  "notify": ["escalations"],
  "rate_limit": 30
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `token` | string | required | BotFather bot token |
| `chat_id` | int64 | required | Telegram chat for outbound messages |
| `allow_from` | []int64 | `[]` | Allowed Telegram user IDs (fail-closed: empty blocks all) |
| `target` | string | `"mayor/"` | Mail recipient for inbound messages |
| `enabled` | bool | `true` | Explicit enable/disable |
| `notify` | []string | `["escalations"]` | Notification categories to forward |
| `rate_limit` | int | `30` | Max inbound messages per user per minute |

### CLI Commands

```bash
gt telegram configure \
    --token "123456789:AAH..." \
    --chat-id 7747509251 \
    --allow-from 7747509251 \
    --notify stuck_agents,escalations \
    --yes                              # skip confirmation when replacing token

gt telegram status                     # connected, last msg, counts
gt telegram status --json              # machine-readable

gt telegram run                        # standalone bridge process
```

## Bootstrap / First Run

1. User runs `gt telegram configure --token <token> --chat-id <id> --allow-from <id>`
2. Command validates token format (`^\d+:[A-Za-z0-9_-]+$`). Rejects malformed tokens before writing.
3. Writes `mayor/telegram.json` with 0600 permissions. If file exists, prompts for confirmation (skip with `--yes`).
4. On first run with no existing config, all fields are required. Subsequent runs allow partial updates (only specified flags are changed).
5. Command prints next steps: "Enable in daemon: add `telegram_bridge.enabled: true` to `mayor/daemon.json` and restart daemon. Or run standalone: `gt telegram run`."

**Token security at load time:** `config.Load()` validates 0600 permissions on `mayor/telegram.json`. If the file is group- or world-readable, the bridge refuses to start with an error message instructing the user to `chmod 600`.

## Identity

The bridge uses the existing `overseer` identity (`mayor/overseer.json`). No new identity concepts needed.

- Inbound mail: `from: "overseer"`, `to: "mayor/"` — the Mayor already understands overseer is its boss
- Outbound replies: Mayor replies `to: "overseer"` via `gt mail reply` — reply-to address set automatically
- Notifications: Bridge sends as system, not as overseer (no `from` impersonation for event notifications)

## Package Structure

```
internal/bridge/telegram/
  config.go      — Config struct, validation, IsEnabled()
  bot.go         — Telegram Bot API wrapper (long-poll, send, access gate, rate limit)
  bridge.go      — Bridge orchestrator (retry loop, lifecycle, Restart())
  inbound.go     — Telegram msg → Sender.SendMail() + Sender.Nudge()
  outbound.go    — .feed.jsonl tail → category filter → bot.SendMessage()
  reply.go       — overseer inbox poll → bot.SendMessage()
  msgmap.go      — bidirectional Telegram msgID ↔ mail ThreadID, 10k FIFO eviction
  sender.go      — Sender interface, DirectSender, CLISender
```

Estimated ~700-800 lines across these files.

### Files Changed Outside Bridge Package

| File | Change | Lines |
|------|--------|-------|
| `internal/daemon/types.go` | Add `TelegramBridgeConfig` struct + field on `PatrolsConfig` + `IsPatrolEnabled` case for `"telegram_bridge"` (opt-in, defaults disabled) | ~10 |
| `internal/daemon/daemon.go` | Start/stop bridge goroutine in `Run()` after feed curator | ~15 |
| `internal/cmd/telegram.go` | `configure`, `status`, `run` commands | ~150 |

## Security

| Concern | Mitigation |
|---------|-----------|
| Token storage | `mayor/telegram.json` with 0600 permissions. Never logged. Masked in status output |
| Inbound access | `allow_from` fail-closed — empty list blocks everyone. Numeric Telegram user IDs only |
| Bot messages | Always rejected (`from.is_bot` check before allow-list) |
| Rate limiting | Per-user sliding window, default 30 msgs/min. Excess dropped silently |
| Outbound restriction | Only sends to configured `chat_id` — never to chat IDs from message content |
| Overseer impersonation | Access gate (allow_from + chat_id) ensures only the real human triggers overseer mail |

## Error Handling

- **Bot API connection failures**: 5s backoff retry loop. No crash, no panic propagation
- **Mail send failures**: Log error, send error message back to Telegram, continue
- **`.feed.jsonl` missing/rotated**: The feed curator rotates via atomic rename, which leaves stale file descriptors. The outbound goroutine checks inode (via `os.Stat`) every poll cycle — if the inode changes or the file shrinks, re-open the file and seek to end. If the file is missing, wait and retry
- **Overseer inbox poll failures**: Log warning, skip cycle, retry next interval
- **Daemon shutdown**: Context cancellation, clean bot disconnect, 5s drain grace period

## Scope Boundaries (v1)

- Text messages only — no files, images, documents
- Single `chat_id` only — no group chat routing
- No message editing — Telegram edits ignored
- No inline keyboards or bot commands — plain text only
- No web dashboard integration — CLI only
- No hot-reload via RPC — restart daemon or `gt telegram run` to pick up config changes

## Testing

### Unit Tests (`internal/bridge/telegram/`)

- `config_test.go` — validation, IsEnabled(), allow_from fail-closed
- `msgmap_test.go` — store/lookup, FIFO eviction, concurrent access
- `inbound_test.go` — access gate ordering, rate limiter, mail formatting
- `outbound_test.go` — category filtering, feed line parsing, Telegram formatting
- `bot_test.go` — mock API responses, poll channel behavior

All unit tests use mock `Sender` — no real mail or Telegram API.

### Integration Test

One `//go:build integration` test:
1. Start Dolt server process (existing `testutil.EnsureDoltContainerForTestMain()` pattern)
2. Bridge with `CLISender` pointed at test Dolt
3. Simulate inbound message → verify mail arrives addressed to `mayor/`
4. Simulate mail reply to `overseer` → verify bridge picks it up

Bot is stubbed with channel-based mock. No real Telegram API in CI.

## Dependencies

- `github.com/go-telegram-bot-api/telegram-bot-api/v5` — Telegram Bot API client (same as Thrum)
- No other new dependencies
