# Gas Town Quick Startup Guide

**Purpose:** Get the Gas Town cleanroom rig up and running quickly.

**Prerequisites:**
- Gas Town installed at `/Users/leon/gt`
- Cleanroom rig configured at `/Users/leon/gt/cleanroom`
- Beads synced and ready

---

## 🚀 Quick Start (TL;DR)

```bash
# 1. Navigate to Gas Town installation
cd /Users/leon/gt

# 2. Start the Mayor (global coordinator)
gt mayor start
gt mayor attach  # Attach to Mayor's session

# 3. From Mayor session, start the cleanroom rig
gt rig start cleanroom

# 4. Verify everything is running
tmux list-sessions
gt status
```

---

## 🏛️ Understanding Gas Town Components

### The Mayor (Global Coordinator)

**What it is:** The Mayor is the central AI coordinator for all of Gas Town. It's a persistent Claude Code session that manages rigs, assigns work, and coordinates between components.

**Key responsibilities:**
- Starting and stopping rigs
- Assigning work to polecats via `gt sling`
- Monitoring overall town health
- Coordinating between rigs

**Starting the Mayor:**
```bash
cd /Users/leon/gt

# Start Mayor session
gt mayor start

# Attach to Mayor
gt mayor attach

# Check Mayor status
gt mayor status
```

**Common Mayor commands (from within Mayor session):**
```bash
# Check overall status
gt status

# List available work
bd ready

# Assign work to a rig (spawns polecat, starts session, assigns work)
gt sling <issue-id> cleanroom

# Check convoy dashboard
gt convoy list

# Start/stop rigs
gt rig start cleanroom
gt rig stop cleanroom

# Check polecats
gt polecat list cleanroom
gt polecat status cleanroom/nux

# Send mail to agents
gt mail send cleanroom/witness -s "Status check" -m "How are things?"
gt mail inbox  # Check Mayor's inbox
```

**Stopping the Mayor:**
```bash
# From outside Mayor session
gt mayor stop

# Or from within Mayor session
exit  # or Ctrl+D
```

### The Witness (Polecat Lifecycle Manager)

**What it is:** A persistent AI agent that manages polecat lifecycles within a rig.

**Key responsibilities:**
- Monitoring polecat health and progress
- Handling polecat completion notifications (POLECAT_DONE messages)
- Creating cleanup wisps when polecats finish
- Auto-nuking polecats when safe
- Escalating issues to Mayor when needed

**How it works:**
1. Polecats send `POLECAT_DONE` messages when they finish work
2. Witness receives the message and checks if work was pushed
3. If safe, Witness auto-nukes the polecat (removes worktree)
4. If not safe, Witness creates a cleanup wisp for manual intervention

**Starting the Witness:**
```bash
# Usually started automatically by `gt rig start`
gt witness start cleanroom

# Attach to witness session
gt witness attach cleanroom

# Check status
gt witness status cleanroom
```

### The Refinery (Merge Queue Processor)

**What it is:** A service that processes merge requests and integrates them into the main branch.

**Key responsibilities:**
- Processing merge requests from polecats
- Running tests and validation
- Merging approved changes to main
- Handling merge conflicts

**How it works:**
1. Polecats push branches and create merge requests
2. Refinery picks up pending merge requests
3. Refinery validates, tests, and merges
4. On success, work is integrated into main
5. On conflict, Refinery may assign conflict resolution back to a polecat

**Monitoring the Refinery:**
```bash
# Check refinery status (usually runs as background service)
gt convoy list  # Shows merge queue status

# Check for pending merges
bd list --type=merge-request --status=open
```

### Polecats (Transient Worker Agents)

**What they are:** Temporary AI agents that do actual work on issues. Each polecat gets its own git worktree and tmux session.

**Key responsibilities:**
- Working on assigned issues
- Making code changes
- Committing and pushing work
- Creating merge requests
- Notifying Witness when done

**Lifecycle:**
1. **Spawn:** Created via `gt sling <issue-id> <rig>` or `gt polecat spawn`
2. **Work:** Polecat works on the assigned issue autonomously
3. **Commit & Push:** Polecat commits changes and pushes to remote
4. **Done:** Polecat sends `POLECAT_DONE` to Witness
5. **Cleanup:** Witness auto-nukes the polecat (removes worktree)

**Working with Polecats:**
```bash
# Spawn and assign work (recommended)
gt sling <issue-id> cleanroom

# List polecats
gt polecat list cleanroom

# Check specific polecat
gt polecat status cleanroom/nux

# Attach to polecat session
gt session at cleanroom/nux

# Stop polecat
gt session stop cleanroom/nux

# Nuke polecat (remove worktree)
gt polecat nuke cleanroom/nux
```

**Polecat states:**
- `○ idle` - Polecat exists but no session running
- `● idle` - Polecat session running but no work assigned
- `● working` - Polecat actively working on an issue

---

## 📋 Step-by-Step Startup

### 1. Navigate to Gas Town Installation

**CRITICAL:** Always work from the Gas Town installation directory, NOT the source code repo!

```bash
cd /Users/leon/gt
pwd  # Should show: /Users/leon/gt
```

### 2. Check Current Status

Before starting, see what's already running:

```bash
# Check tmux sessions
tmux list-sessions 2>&1

# Check Mayor status
gt mayor status

# Check rig status
gt status

# Check witness status
gt witness status cleanroom

# Check polecats
gt polecat list cleanroom
```

### 3. Start the Mayor

The Mayor is the central coordinator and should be started first:

```bash
# Start Mayor session
gt mayor start

# Attach to Mayor (optional - you can work from your own terminal too)
gt mayor attach
```

**From within the Mayor session:**
- You can run all `gt` commands
- The Mayor has full visibility into all rigs
- Use `Ctrl+b then d` to detach without stopping the Mayor

**Working without attaching to Mayor:**
- You can run `gt` commands from your own terminal
- The Mayor session runs in the background
- Useful for scripting or running multiple commands

### 4. Start the Rig

**Option A: Start everything (recommended)**
```bash
gt rig start cleanroom
```

This starts:
- Witness (polecat lifecycle manager)
- Refinery (merge queue processor)
- Optionally: polecats if configured

**Option B: Start components individually**
```bash
# Start witness
gt witness start cleanroom

# Start refinery (if needed)
# Note: Refinery typically runs as a service, check docs

# Start individual polecats (only if you need them immediately)
gt session start cleanroom/nux
gt session start cleanroom/keeper
```

### 5. Verify Startup

```bash
# Check all tmux sessions
tmux list-sessions
# Should show: gt-mayor, gt-cleanroom-witness, and any polecat sessions

# Check Mayor is running
gt mayor status
# Should show: State: running

# Check witness is running
gt witness status cleanroom
# Should show: State: ● running

# Check polecats
gt polecat list cleanroom
# Shows which polecats exist and their state (● = session running, ○ = idle)

# Check overall status
gt status
```

### 6. Assign Work to Polecats

**Option A: Use sling from Mayor (recommended)**
```bash
# From Mayor session or your terminal:
cd /Users/leon/gt

# Check available work
bd ready

# Sling work to a polecat (creates polecat if needed, starts session, assigns work)
gt sling <issue-id> cleanroom
```

**Option B: Start existing polecats and assign work**
```bash
# Start a polecat session
gt session start cleanroom/nux

# Assign work via beads
bd update <issue-id> --assignee=cleanroom/polecats/nux
```

---

## 🔄 How It All Works Together

### Typical Workflow

```
1. Mayor checks for available work (bd ready)
   ↓
2. Mayor slings work to rig (gt sling <issue-id> cleanroom)
   ↓
3. Polecat spawns, session starts, work assigned
   ↓
4. Polecat works autonomously on the issue
   ↓
5. Polecat commits, pushes, creates merge request
   ↓
6. Polecat sends POLECAT_DONE to Witness
   ↓
7. Refinery processes merge request
   ↓
8. Refinery merges to main (or handles conflicts)
   ↓
9. Witness auto-nukes polecat (cleanup)
   ↓
10. Mayor assigns next work item
```

### Component Communication

```
Mayor (Coordinator)
  ├─> Witness (Lifecycle Manager)
  │     ├─> Polecat 1 (Worker)
  │     ├─> Polecat 2 (Worker)
  │     └─> Polecat N (Worker)
  │
  └─> Refinery (Merge Processor)
        └─> Main Branch (Integration)
```

**Communication via:**
- **Mail:** Agents send messages via `gt mail send`
- **Beads:** Shared issue database (synced via git)
- **Git:** Code changes, branches, merge requests

---

## 🔍 Monitoring

### Check Overall Status
```bash
gt status                    # Town-wide status
gt convoy list               # Active work dashboard
```

### Check Specific Components
```bash
# Witness
gt witness status cleanroom

# Polecats
gt polecat list cleanroom
gt polecat status cleanroom/nux

# Tmux sessions
tmux list-sessions | grep cleanroom
```

### Attach to Sessions
```bash
# Attach to witness
gt witness attach cleanroom

# Attach to polecat
gt session at cleanroom/nux
# Or: tmux attach -t gt-cleanroom-nux

# Detach from tmux: Ctrl+b then d
```

---

## 🛑 Shutdown

### Quick Shutdown (Recommended Order)
```bash
cd /Users/leon/gt

# 1. Stop the rig (stops witness, polecats)
gt rig stop cleanroom

# 2. Stop the Mayor (optional - can leave running)
gt mayor stop

# 3. Verify all stopped
tmux list-sessions 2>&1
# Should show: no server running (or only gt-mayor if you kept it running)
```

### Manual Shutdown (Step-by-Step)
```bash
cd /Users/leon/gt

# 1. Stop polecats
gt session stop cleanroom/nux
gt session stop cleanroom/keeper
# ... repeat for each running polecat

# 2. Stop witness
gt witness stop cleanroom

# 3. Stop Mayor (optional)
gt mayor stop

# 4. Verify all stopped
tmux list-sessions 2>&1
# Should show: no server running
```

**Note:** You can leave the Mayor running if you plan to work with Gas Town again soon. It's lightweight and useful for quick status checks.

---

## 🧹 Cleanup After Shutdown

If you want to remove agent beads from the beads database:

```bash
cd /Users/leon/gt/cleanroom/mayor/rig

# Close all agent beads
bd close cleanroom-cleanroom-polecat-* cleanroom-cleanroom-witness cleanroom-cleanroom-refinery --reason="Shutdown" --no-daemon

# Sync to remote
bd sync --no-daemon
```

**Note:** Only do this if you're shutting down Gas Town completely. Agent beads are infrastructure metadata.

---

## ⚠️ Common Issues

### No tmux sessions starting
**Problem:** `tmux list-sessions` shows "no server running"  
**Cause:** Sessions weren't actually started (mail doesn't start sessions!)  
**Solution:** Use `gt session start` or `gt sling` to start sessions

### Permission prompts blocking polecats
**Problem:** Polecat stuck at "Do you trust the files in this folder?"  
**Solution:** 
1. Attach to session: `tmux attach -t gt-cleanroom-nux`
2. Accept the prompt
3. Type: `gt prime`
4. Detach: Ctrl+b then d

**Long-term fix:** Auto-provision settings (see gt-aesoh issue)

### Polecats have no work
**Problem:** Polecat started but says "no work on hook"  
**Cause:** `gt session start` only starts the session, doesn't assign work  
**Solution:** Use `gt sling <issue-id> cleanroom` or assign work via beads

---

## � Quick Reference: Mayor Commands

### Essential Mayor Commands
```bash
# Status and monitoring
gt status                        # Overall town status
gt convoy list                   # Active work dashboard
bd ready                         # Available work

# Work assignment
gt sling <issue-id> cleanroom    # Assign work (spawns polecat, starts session)

# Rig management
gt rig start cleanroom           # Start rig (witness + refinery)
gt rig stop cleanroom            # Stop rig

# Polecat management
gt polecat list cleanroom        # List all polecats
gt polecat status cleanroom/nux  # Check specific polecat
gt session start cleanroom/nux   # Start polecat session
gt session stop cleanroom/nux    # Stop polecat session
gt polecat nuke cleanroom/nux    # Remove polecat worktree

# Communication
gt mail inbox                    # Check Mayor's inbox
gt mail send cleanroom/witness -s "Subject" -m "Message"

# Beads (issue tracking)
bd list --status=open            # Open issues
bd show <issue-id>               # Issue details
bd update <issue-id> --status=in_progress
```

### Mayor Session Management
```bash
# Start/stop Mayor
gt mayor start                   # Start Mayor session
gt mayor stop                    # Stop Mayor session
gt mayor attach                  # Attach to Mayor session
gt mayor status                  # Check Mayor status

# From within Mayor session
Ctrl+b then d                    # Detach (keeps Mayor running)
exit                             # Stop Mayor session
```

---

## �📚 Related Documentation

- **Full onboarding:** `/Users/Shared/Falcon/gastown/output/gas-town-llm-onboarding.md`
- **Gas Town README:** `/Users/Shared/Falcon/gastown/README.md`
- **Understanding Gas Town:** `/Users/Shared/Falcon/gastown/docs/understanding-gas-town.md`
- **Reference Guide:** `/Users/Shared/Falcon/gastown/docs/reference.md`

---

**Last Updated:** 2026-01-03
**Maintainer:** Gas Town team

