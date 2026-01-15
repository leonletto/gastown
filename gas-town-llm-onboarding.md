# Gas Town LLM Onboarding Guide

**For:** New LLM agents working with Gas Town
**Based on:** Cleanroom reconciliation project (2026-01-03)
**Experience:** Managing 10+ polecats, full workflow from sling → merge

---

## ⚠️ CRITICAL: Working Directory

**ALWAYS work from the Gas Town installation directory, NOT the source code repository!**

```bash
# ✅ CORRECT - Gas Town installation (where rigs live)
cd /Users/leon/gt

# ❌ WRONG - Source code repository (for development only)
cd /Users/Shared/Falcon/gastown
```

**Before running ANY `gt` commands:**
1. Check your current directory: `pwd`
2. If not in `/Users/leon/gt`, change to it: `cd /Users/leon/gt`
3. Verify you see rigs: `ls -la` should show `cleanroom/`, `mayor/`, `deacon/`, etc.

**Why this matters:**
- All `gt` commands expect to run from the town root
- Polecats, witness, refinery are all relative to the town root
- Running from the wrong directory will fail silently or create confusion

---

## 🏙️ Essential Context

### Town Structure
```
/Users/leon/gt/                    # Town root (HQ)
├── .beads/                        # Town-level issue tracking
├── mayor/                         # Global coordinator
├── deacon/                        # Health monitor daemon
└── <rig>/                         # Project containers
    ├── .beads/ → mayor/rig/.beads # Symlink to canonical beads
    ├── .repo.git/                 # Bare repo (shared by worktrees)
    ├── mayor/rig/                 # Mayor's clone (canonical)
    ├── refinery/rig/              # Merge queue processor
    ├── witness/                   # Polecat lifecycle manager
    ├── crew/<name>/               # Persistent human workspaces
    └── polecats/<name>/           # Transient worker worktrees
```

### Active Rigs
- **cleanroom** - `/Users/leon/gt/cleanroom/` (primary rig from our experience)

### Key Roles

| Role | Lifecycle | Purpose | Location |
|------|-----------|---------|----------|
| **Mayor** | Persistent | Global coordinator | `~/gt/mayor/` |
| **Deacon** | Persistent | Health monitor daemon | `~/gt/deacon/` |
| **Witness** | Persistent (per-rig) | Polecat lifecycle manager | `<rig>/witness/` |
| **Refinery** | Persistent (per-rig) | Merge queue processor | `<rig>/refinery/` |
| **Polecat** | Transient | Worker agent | `<rig>/polecats/<name>/` |
| **Crew** | Persistent | Human workspace | `<rig>/crew/<name>/` |

---

## 📋 Core Command Reference

### Gas Town Commands (`gt`)

#### Polecat Management
```bash
# List polecats
gt polecat list <rig>                    # List all polecats in rig
gt polecat list --all                    # List across all rigs

# Status checking
gt polecat status <rig>/<polecat>        # Detailed status
gt polecat check-recovery <rig>/<polecat> # Check if safe to nuke
gt polecat git-state <rig>/<polecat>     # Check git cleanliness

# Cleanup (IMPORTANT: Use proper workflow!)
gt polecat nuke <rig>/<polecat>          # Safe cleanup (blocks on unpushed work)
gt polecat nuke --force <rig>/<polecat>  # Force cleanup (LOSES WORK - use carefully)
gt polecat nuke <rig> --all              # Cleanup all polecats in rig

# Sync beads
gt polecat sync <rig>/<polecat>          # Sync beads for one polecat
gt polecat sync <rig> --all              # Sync all polecats
```

#### Session Management

**⚠️ CRITICAL: Mail Does NOT Start Sessions!**

Sending mail to a polecat (e.g., `gt mail send cleanroom/polecat/nux -s "start"`) does **NOT** create or start a tmux session. Mail is for communication between **already running** agents.

**To actually start a polecat session:**

```bash
# ✅ CORRECT - Start a polecat session (creates tmux session + launches Claude)
gt session start <rig>/<polecat>         # Start polecat session

# ✅ CORRECT - Sling work (spawns polecat + starts session automatically)
gt sling <issue-id> <rig>                # Creates polecat, starts session, assigns work

# ❌ WRONG - This only creates a mail message, doesn't start anything
gt mail send <rig>/polecat/<name> -s "start" -m "start"
```

**Session control commands:**
```bash
gt session start <rig>/<polecat>         # Start polecat session
gt session stop <rig>/<polecat>          # Stop session gracefully
gt session stop <rig>/<polecat> --force  # Force stop

# Handoff (restart with fresh context)
gt handoff                               # Restart current session
gt handoff --shutdown                    # Terminate (for polecats)
gt handoff <bead-id>                     # Hook work then restart
```

#### Status & Monitoring
```bash
gt status                                # Overall town status
gt convoy list                           # Active work dashboard
gt convoy status <convoy-id>             # Detailed convoy progress
gt agents                                # Navigate between sessions
gt peek <agent>                          # Check agent health
```

#### Communication
```bash
gt mail inbox                            # Check messages
gt mail read <msg-id>                    # Read specific message
gt mail send <addr> -s "Subject" -m "Body"
gt mail send mayor/ -s "RECOVERY_NEEDED" -m "Details..."  # Escalate
```

#### Work Assignment
```bash
gt sling <bead-id> <rig>                 # Assign work to polecat
bd ready                                 # Show available work
bd list --status=in_progress             # Active work
```

### Tmux Commands

#### Session Discovery
```bash
tmux list-sessions                       # List all sessions
tmux list-sessions | grep gt-            # Filter Gas Town sessions
tmux list-sessions | grep gt-<rig>-      # Filter by rig
```

#### Attaching to Sessions
```bash
tmux attach-session -t <session-name>    # Attach to session
tmux attach -t gt-cleanroom-furiosa      # Example: attach to polecat
# Detach: Ctrl+b then d
```

#### Session Inspection
```bash
# Capture last 40 lines from a session
tmux capture-pane -t gt-<rig>-<polecat> -p | tail -40

# Check if session exists
tmux has-session -t gt-<rig>-<polecat> 2>/dev/null && echo "exists" || echo "not found"
```

### Beads Commands (`bd`)

#### Listing Issues
```bash
bd list                                  # All open issues
bd list --status=open                    # Open issues
bd list --status=in_progress             # In-progress issues
bd list --status=closed                  # Closed issues
bd list --assignee=<rig>/polecats/<name> # Issues for specific polecat
bd list --priority=1                     # P1 issues only
bd ready                                 # Issues with no blockers (ready to work)
```

#### Issue Details
```bash
bd show <issue-id>                       # Show issue details
bd show <id1> <id2> <id3> --json         # Batch show (efficient)
```

#### Issue Management
```bash
bd create --title="..." --type=task      # Create new issue
bd update <id> --status=in_progress      # Update status
bd close <id> --reason="..."             # Close issue
bd sync                                  # Sync beads to remote
```

#### Dependencies
```bash
bd dep add <child> <parent>              # child depends on parent
bd dep list <issue-id>                   # Show dependencies
```

---

## 🔧 Troubleshooting Guide

### Common Failure Patterns

#### 0. No Tmux Sessions Exist (Polecats Not Actually Started)

**Symptoms:**
- `tmux list-sessions` shows: `error connecting to /private/tmp/tmux-501/default (No such file or directory)`
- `gt polecat list <rig>` shows polecats as "idle"
- Mayor or other agent says they "started" polecats but nothing happened

**Diagnosis:**
```bash
cd /Users/leon/gt                        # CRITICAL: Be in town root!
tmux list-sessions                       # Check if ANY sessions exist
gt polecat list cleanroom                # Check polecat state
```

**Root Cause:**
Mail messages don't start sessions! Sending `gt mail send cleanroom/polecat/nux -s "start"` only creates a mail bead - it doesn't launch Claude or create a tmux session.

**Solution:**
```bash
cd /Users/leon/gt                        # CRITICAL: Be in town root!

# Start individual polecats:
gt session start cleanroom/nux
gt session start cleanroom/keeper
gt session start cleanroom/capable

# Verify sessions are running:
tmux list-sessions                       # Should show gt-cleanroom-nux, etc.

# Or use sling to spawn + start + assign work in one command:
gt sling <issue-id> cleanroom            # Spawns polecat, starts session, assigns work
```

**Prevention:**
- Always use `gt session start` or `gt sling` to start polecats
- Never rely on mail messages to start sessions
- Verify with `tmux list-sessions` after starting

#### 1. Stuck Polecat at Permission Prompt

**Symptoms:**
- Polecat session exists but no activity
- `gt prime` never executed
- Work assigned but not started

**Diagnosis:**
```bash
tmux attach -t gt-<rig>-<polecat>
# Look for: "Do you trust the files in this folder?" or permission prompts
```

**Solution:**
```bash
# Current workaround (manual):
tmux attach -t gt-<rig>-<polecat>
# Accept trust prompt
# Type: gt prime
# Detach: Ctrl+b then d

# Permanent fix: See gt-aesoh issue for auto-provisioning settings.local.json
```

**Root Cause:** Claude Code requires explicit permissions. Current approach uses `--dangerously-skip-permissions` flag but auto-accept isn't working (commit 94857fc).

#### 2. Polecat Won't Nuke (Uncommitted Changes)

**Symptoms:**
```
Error: Cannot nuke the following polecats:
  cleanroom/cheedo:
    - has 10 uncommitted file(s)
```

**Diagnosis:**
```bash
cd /Users/leon/gt/<rig>/polecats/<polecat>
git status                               # Check what's uncommitted
git log --oneline -3                     # Check commits
git log --oneline origin/<branch> -3     # Check if pushed
```

**Decision Tree:**
1. **If work is pushed** (commit exists on origin):
   ```bash
   gt polecat nuke --force <rig>/<polecat>  # Safe - work is saved
   ```

2. **If uncommitted changes are just infrastructure** (`.beads/`, `.claude/`, `.runtime/`):
   ```bash
   cd /Users/leon/gt/<rig>/polecats/<polecat>
   git reset --hard HEAD
   git clean -fd
   cd /Users/leon/gt/<rig>
   gt polecat nuke <rig>/<polecat>
   ```

3. **If uncommitted changes are real work**:
   ```bash
   # From polecat session:
   git add .
   git commit -m "WIP: description"
   git push
   # Then nuke
   gt polecat nuke <rig>/<polecat>
   ```

**Real Example from Cleanroom:**
```bash
# cheedo had uncommitted .beads/ and .claude/ changes
# Work commit e267342 was already pushed
# Solution: gt polecat nuke --force cleanroom/cheedo
```

#### 3. Session Exists But Polecat Idle

**Symptoms:**
- Tmux session running
- Polecat says "no work" but work is assigned

**Diagnosis:**
```bash
gt polecat status <rig>/<polecat>        # Check state
bd list --assignee=<rig>/polecats/<name> # Check assigned work
tmux attach -t gt-<rig>-<polecat>        # Inspect session
```

**Solutions:**
```bash
# Option 1: Nudge the polecat
gt nudge <rig>/<polecat> "Check your hook: gt hook"

# Option 2: Restart session
gt session stop <rig>/<polecat>
gt session start <rig>/<polecat>

# Option 3: From inside polecat session
gt prime                                 # Re-run context check
gt hook                                  # Check hooked work
```

#### 4. Beads Sync Conflicts

**Symptoms:**
- `bd sync` fails with conflicts
- Beads out of sync between polecats

**Solution:**
```bash
# From polecat worktree:
cd /Users/leon/gt/<rig>/polecats/<polecat>
bd sync                                  # Try sync
# If conflicts:
git -C .beads status                     # Check beads repo status
git -C .beads pull --rebase              # Manual sync
bd sync                                  # Retry
```

#### 5. Git Conflicts During Merge

**Symptoms:**
- Refinery reports merge conflict
- Polecat assigned conflict resolution task

**Solution:**
```bash
# From polecat session working on conflict resolution:
git status                               # See conflicted files
git diff                                 # See conflict markers

# For each conflicted file:
# 1. Edit file to resolve conflicts (remove <<<, ===, >>> markers)
# 2. Stage resolved file:
git add <resolved-file>

# Continue rebase:
git rebase --continue

# If stuck:
bd show <original-issue-id>              # Get context
# Or escalate:
gt mail send <rig>/witness -s "HELP: Complex conflict" -m "Details..."
```

#### 6. Multiple Polecats, Can't Track Status

**Symptoms:**
- 10+ polecats running
- Don't know which are working vs stuck

**Solution:**
```bash
# Quick status check:
tmux list-sessions | grep gt-<rig>-      # See all sessions

# Detailed status:
gt polecat list <rig>                    # Shows state for all polecats

# Check specific polecat:
gt polecat status <rig>/<polecat>

# Capture last output from each:
for p in furiosa nux slit rictus; do
  echo "=== $p ==="
  tmux capture-pane -t gt-cleanroom-$p -p | tail -10
done
```

**Real Example from Cleanroom (2026-01-03):**
- Deployed 10 polecats for reconciliation
- All stuck at permission prompt initially
- Used `tmux attach` + manual `gt prime` for each
- Lesson: Check first polecat before deploying 10!

---

## ✅ Best Practices

### 1. Always Check Status Before Action

**Before nuking polecats:**
```bash
gt polecat check-recovery <rig>/<polecat>  # Returns SAFE_TO_NUKE or NEEDS_RECOVERY
gt polecat git-state <rig>/<polecat>       # Check git status
```

**Before deploying multiple polecats:**
```bash
# Test with ONE polecat first:
gt sling <issue-id> <rig>
# Wait 30 seconds
gt polecat status <rig>/<polecat-name>
# If working, deploy more
```

### 2. Use Proper Cleanup Workflows

**✅ CORRECT:**
```bash
# Let safety checks work:
gt polecat nuke <rig>/<polecat>

# If blocked, investigate:
gt polecat check-recovery <rig>/<polecat>

# Only use --force if work is confirmed pushed:
git log --oneline origin/<branch>  # Verify
gt polecat nuke --force <rig>/<polecat>
```

**❌ WRONG:**
```bash
# Don't manually kill sessions:
tmux kill-session -t gt-<rig>-<polecat>  # Bypasses cleanup

# Don't manually delete worktrees:
rm -rf /Users/leon/gt/<rig>/polecats/<polecat>  # Leaves orphaned state

# Don't force-nuke without checking:
gt polecat nuke --force <rig>/<polecat>  # May lose work!
```

### 3. When to Use `--force` Flags Safely

**Safe scenarios for `--force`:**

1. **Work is pushed to origin:**
   ```bash
   git log --oneline origin/<branch>  # Verify commit exists
   gt polecat nuke --force <rig>/<polecat>
   ```

2. **Uncommitted changes are infrastructure only:**
   ```bash
   git status  # Shows only .beads/, .claude/, .runtime/
   gt polecat nuke --force <rig>/<polecat>
   ```

3. **Polecat is truly stuck and unrecoverable:**
   ```bash
   # After escalating to Mayor and getting approval
   gt polecat nuke --force <rig>/<polecat>
   ```

**Never use `--force` if:**
- Uncommitted changes include actual code
- Commits exist locally but not on origin
- You haven't checked `git status` first

### 4. Navigation Patterns to Avoid Trial-and-Error

**Finding your way around:**
```bash
# Where am I?
pwd                                      # Current directory
echo $GT_RIG                             # Current rig (if in session)
echo $GT_POLECAT                         # Current polecat (if in session)

# Where are the rigs?
ls -la /Users/leon/gt/                   # Town root

# Where are the polecats?
ls -la /Users/leon/gt/<rig>/polecats/    # Polecat worktrees

# What sessions exist?
tmux list-sessions | grep gt-            # All Gas Town sessions
```

**Working with multiple rigs:**
```bash
# Always specify rig in commands:
gt polecat list cleanroom                # Not just "gt polecat list"
gt polecat status cleanroom/furiosa      # Full address

# Use tab completion:
gt polecat status clean<TAB>             # Autocomplete rig name
```

### 5. Communication Patterns

**When to escalate:**
```bash
# Polecat stuck for >5 minutes:
gt mail send <rig>/witness -s "Polecat stuck" -m "Details..."

# Merge conflict too complex:
gt mail send <rig>/witness -s "HELP: Complex conflict" -m "..."

# Work needs recovery:
gt mail send mayor/ -s "RECOVERY_NEEDED <rig>/<polecat>" -m "..."
```

**When NOT to send mail:**
- Routine status updates (use beads instead)
- Health check responses (Deacon tracks via session status)
- Every patrol cycle (creates noise)

### 6. Efficient Batch Operations

**Working with multiple polecats:**
```bash
# Nuke multiple at once:
gt polecat nuke <rig>/<p1> <rig>/<p2> <rig>/<p3>

# Or all at once:
gt polecat nuke <rig> --all

# Sync all polecats:
gt polecat sync <rig> --all

# Check status efficiently:
gt polecat list <rig>  # Shows all at once
```

**Batch beads operations:**
```bash
# Show multiple issues:
bd show issue-1 issue-2 issue-3 --json   # Single command, not 3

# List with filters:
bd list --status=in_progress --priority=1  # Combine filters
```

---

## 📚 Key Learnings from Cleanroom Project

### What We Did
- Deployed 10 polecats for reconciliation work
- Each polecat handled 1-2 naming conflicts
- Full workflow: sling → work → commit → push → merge
- All 10 polecats completed successfully

### What Went Wrong
1. **Permission prompts blocked all 10 polecats** - Required manual intervention
2. **Didn't test first polecat before deploying 10** - Wasted time
3. **Some polecats had uncommitted infrastructure files** - Confused cleanup

### What Went Right
1. **Used `gt polecat check-recovery` before nuking** - Prevented data loss
2. **Verified commits were pushed before force-nuking** - Safe cleanup
3. **Used batch operations** - Efficient management of 10 polecats
4. **Proper escalation** - Witness → Mayor for complex issues

### Time Savings
- **Manual intervention:** ~30 minutes for 10 polecats
- **With auto-permissions:** Would be ~0 minutes (fully autonomous)
- **Lesson:** Fix infrastructure issues before scaling

---

## 🎯 Quick Reference Card

```bash
# ⚠️ FIRST: Always be in the town root!
cd /Users/leon/gt                        # CRITICAL: Run this first!

# Status Check
gt status                                # Town overview
gt polecat list <rig>                    # All polecats
gt polecat status <rig>/<polecat>        # Specific polecat
bd ready                                 # Available work
tmux list-sessions                       # Check running sessions

# Starting Polecats (IMPORTANT!)
gt session start <rig>/<polecat>         # Start a polecat session
gt sling <issue-id> <rig>                # Spawn + start + assign work
# NOTE: Mail does NOT start sessions!

# Work Assignment
gt sling <issue-id> <rig>                # Assign to polecat
bd show <issue-id>                       # Issue details

# Cleanup
gt polecat check-recovery <rig>/<polecat>  # Safety check
gt polecat nuke <rig>/<polecat>          # Safe cleanup
gt polecat nuke --force <rig>/<polecat>  # Force (if work pushed)

# Session Management
tmux list-sessions | grep gt-<rig>-      # List sessions
tmux attach -t gt-<rig>-<polecat>        # Attach
gt handoff                               # Restart session

# Communication
gt mail inbox                            # Check mail
gt mail send <addr> -s "..." -m "..."    # Send mail
gt escalate -s HIGH "Critical issue"     # Escalate

# Debugging
tmux capture-pane -t gt-<rig>-<polecat> -p | tail -40  # Last output
gt doctor                                # Health check
gt doctor --fix                          # Auto-repair
```

---

## 📖 Further Reading

- **Gas Town README:** `/Users/Shared/Falcon/gastown/README.md`
- **Reference Guide:** `/Users/Shared/Falcon/gastown/docs/reference.md`
- **Understanding Gas Town:** `/Users/Shared/Falcon/gastown/docs/understanding-gas-town.md`
- **Permission Issues:** `/Users/Shared/Falcon/gastown/gt-aesoh-autonomous-agent-permissions.md`
- **Beads Documentation:** `https://github.com/steveyegge/beads`

---

**Last Updated:** 2026-01-04
**Based on:** Cleanroom reconciliation project experience
**Maintainer:** Gas Town team


