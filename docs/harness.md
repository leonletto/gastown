# Gas Town Harness

A **harness** is a private repository that contains your Gas Town installation with managed rigs gitignored. This document explains what harnesses are, why you'd want one, and how to configure beads redirects.

## What is a Harness?

A harness is the top-level directory where Gas Town is installed. It serves as:

1. **Your workspace root**: Where the Mayor runs and coordinates all work
2. **Configuration home**: Contains town-level settings, Mayor state, and role prompts
3. **Private envelope**: Lets you work on public projects while keeping your GT config private

### Harness vs Rig

| Concept | What it is | Git tracked? |
|---------|------------|--------------|
| **Harness** | Gas Town installation directory | Yes (your private repo) |
| **Rig** | Project container within harness | No (gitignored) |
| **Rig clone** | Git checkout of a project | No (gitignored) |

The harness is YOUR repo. Rigs are containers for OTHER repos (the projects you work on).

### Harness Structure

```
~/gt/                              # Harness root (your private repo)
├── .git/                          # Your harness repo
├── .gitignore                     # Ignores rig contents
├── .beads/                        # Town-level beads (Mayor mail)
│   ├── redirect                   # Points to active rig beads
│   └── ...
├── CLAUDE.md                      # Mayor role context
├── mayor/                         # Mayor config and state
│   ├── town.json
│   ├── rigs.json
│   └── state.json
├── gastown/                       # A rig (gitignored)
│   ├── config.json
│   ├── .beads/                    # Rig beads
│   ├── mayor/rig/                 # Mayor's clone
│   ├── polecats/                  # Worker clones
│   └── ...
└── wyvern/                        # Another rig (gitignored)
```

## Why Use a Harness?

### 1. Work on Public Projects Privately

You might be contributing to open source projects but want to keep your AI workflow private. The harness lets you:

- Track your Gas Town configuration in a private repo
- Work on any project (public or private) as a rig
- Keep project-specific beads and agent state separate from the upstream

### 2. Unified Workspace

One harness can manage multiple rigs:

```bash
~/gt/                    # Your harness
├── gastown/            # The Gas Town Go port
├── wyvern/             # A game engine
├── beads/              # The beads CLI tool
└── client-project/     # Private client work
```

Each rig is independent but shares the same Mayor and town-level coordination.

### 3. Portable Configuration

Your harness repo captures:

- Mayor role prompts (CLAUDE.md)
- Town configuration
- Rig registry (which projects you're managing)
- Town-level beads (handoff messages, coordination)

Clone your harness to a new machine and `gt rig add` to restore your workspace.

## Setting Up a Harness

### 1. Install Gas Town

```bash
# Create harness at ~/gt
gt install ~/gt

# Or initialize current directory
cd ~/gt
gt install .
```

### 2. Initialize Git

```bash
# Create local git repo
gt git-init

# Or create with GitHub remote
gt git-init --github=username/my-gt-harness --private
```

### 3. Add Rigs

```bash
cd ~/gt
gt rig add gastown https://github.com/steveyegge/gastown
gt rig add wyvern https://github.com/steveyegge/wyvern
```

Rigs are automatically gitignored. Your harness tracks THAT you have a gastown rig (in `mayor/rigs.json`), but not its contents.

## Beads Redirects

### The Problem

When the Mayor runs from the harness root (`~/gt/`), where should beads commands go?

- The harness has its own `.beads/` for town-level mail
- But most work happens in rig-level beads (issues, tasks, epics)
- Running `bd list` from `~/gt/` should show rig issues, not just Mayor mail

### The Solution: Redirect Files

The `.beads/redirect` file tells beads to look elsewhere for issues:

```
# ~/gt/.beads/redirect
# Redirect beads queries to the gastown rig
gastown/mayor/.beads
```

With this redirect:
- `bd list` from `~/gt/` shows gastown issues
- `bd mail inbox` from `~/gt/` shows Mayor mail (uses town beads)
- Mayor can manage rig work without `cd`ing into the rig

### Configuring Redirects

The redirect file contains a single relative path to the target beads directory:

```bash
# Edit the redirect
echo "gastown/mayor/.beads" > ~/gt/.beads/redirect

# Or for a different rig
echo "wyvern/mayor/.beads" > ~/gt/.beads/redirect
```

### Directory Structure with Redirect

```
~/gt/.beads/                       # Town beads
├── redirect                       # → gastown/mayor/.beads
├── config.yaml                    # Town beads config
├── beads.db                       # Town beads (Mayor mail)
└── issues.jsonl                   # Town issues

~/gt/gastown/mayor/.beads/         # Rig beads (redirect target)
├── config.yaml                    # Rig beads config
├── beads.db                       # Rig beads
└── issues.jsonl                   # Rig issues (gt-* prefix)
```

### Multi-Rig Workflow

If you're working on multiple rigs, change the redirect to focus on different projects:

```bash
# Working on gastown today
echo "gastown/mayor/.beads" > ~/gt/.beads/redirect

# Switching to wyvern
echo "wyvern/mayor/.beads" > ~/gt/.beads/redirect
```

Or stay in rig directories where beads naturally finds the local `.beads/`:

```bash
cd ~/gt/gastown/polecats/MyPolecat
bd list    # Uses rig beads automatically
```

## The GGT Directory Structure

The Go Gas Town (GGT) uses a specific convention for rig beads:

```
gastown/                           # Rig container
├── mayor/                         # Mayor's per-rig presence
│   ├── .beads/                    # Canonical rig beads
│   └── rig/                       # Mayor's clone (if separate)
└── .beads/ → mayor/.beads         # Symlink (optional)
```

The redirect path for GGT rigs is: `<rig>/mayor/.beads`

This differs from older structures where beads might be at `mayor/rigs/<rig>/.beads`. When updating from older setups, change your redirect accordingly:

```bash
# Old structure
echo "mayor/rigs/gastown/.beads" > ~/.beads/redirect

# GGT structure
echo "gastown/mayor/.beads" > ~/.beads/redirect
```

## Harness Git Workflow

### What Gets Tracked

The harness `.gitignore` (created by `gt git-init`) tracks:

**Tracked:**
- `CLAUDE.md` - Mayor role context
- `mayor/*.json` - Town and rig registry
- `.beads/config.yaml` - Beads configuration
- `.beads/issues.jsonl` - Town-level beads (Mayor mail)

**Ignored:**
- `*/` - All rig directories
- `.beads/*.db` - SQLite databases (regenerated from JSONL)
- `**/rig/` - All git clones
- `**/polecats/` - All worker directories

### Committing Changes

```bash
cd ~/gt
git add .
git commit -m "Update Mayor context"
git push
```

Rig work is committed IN the rig clones, not the harness.

## Troubleshooting

### "Not in a beads repo"

Check that `.beads/` exists and has either issues.jsonl or a redirect:

```bash
ls -la .beads/
cat .beads/redirect
```

### Redirect points to wrong location

Verify the redirect path is relative to the harness root and the target exists:

```bash
# Check current redirect
cat ~/gt/.beads/redirect

# Verify target exists
ls -la ~/gt/$(cat ~/gt/.beads/redirect)
```

### Wrong issues showing

If you see unexpected issues, check which beads directory is active:

```bash
# Where is beads looking?
bd doctor

# Override with explicit path
BEADS_DIR=~/gt/gastown/mayor/.beads bd list
```

## Summary

- **Harness**: Your private Gas Town installation repo
- **Rigs**: Project containers, gitignored from harness
- **Redirect**: Tells town-level beads where to find rig issues
- **GGT path**: `<rig>/mayor/.beads` for Go Gas Town structure

The harness gives you a unified workspace for managing multiple projects while keeping your AI coordination infrastructure private.
