I'll investigate how beads and Gas Town handle worktrees to give you accurate guidance for your other project.

Now let me look at how beads handles the redirect mechanism for worktrees:

Perfect! Now I have a comprehensive understanding. Let me create a clear overview document for your team:

## 📋 Worktree Compatibility Guide for Beads & Gas Town

Based on my analysis of the Gas Town and beads codebase, here's what your team needs to know about making worktrees compatible:

---

### 🎯 **Key Insight: Worktrees Should Be INSIDE the Project Structure**

**Current approach (incompatible):**
```
myproject/              # Main repo
../worktree-name/       # Worktree OUTSIDE the project
```

**Beads/Gas Town approach (compatible):**
```
myproject/              # Project root (can be bare repo)
├── .beads/            # Shared beads database
├── mayor/rig/         # Main clone (canonical)
├── polecats/          # Worker worktrees directory
│   ├── worker1/       # Worktree 1
│   └── worker2/       # Worktree 2
└── crew/              # Human worktrees directory
    ├── alice/         # Worktree 3
    └── bob/           # Worktree 4
```

---

### 🏗️ **Architecture: Shared Beads Database via Redirect**

Beads uses a **redirect mechanism** to allow multiple worktrees to share a single beads database:

#### **1. Canonical Location**
One location holds the actual `.beads/` database (usually `mayor/rig/.beads/` or project root `.beads/`)

#### **2. Redirect Files**
Each worktree has a `.beads/redirect` file containing a relative path to the canonical location:

```bash
# In polecats/worker1/.beads/redirect
../../mayor/rig/.beads

# In crew/alice/.beads/redirect  
../../mayor/rig/.beads
```

#### **3. Resolution**
When beads commands run, they:
1. Check for `.beads/redirect` file
2. If found, resolve the path relative to the worktree directory
3. Use the resolved path as the actual beads database

---

### 📁 **Recommended Directory Structure**

```
<project-root>/
├── .repo.git/              # Bare repo (shared by all worktrees)
├── .beads/                 # Shared beads database (canonical)
│   ├── beads.db           # SQLite database
│   ├── issues.jsonl       # JSONL export
│   └── routes.jsonl       # Routing config
├── main/                   # Main worktree (on main branch)
│   └── .beads/
│       └── redirect       # Contains "../.beads"
└── worktrees/              # All other worktrees
    ├── feature-1/
    │   └── .beads/
    │       └── redirect   # Contains "../../.beads"
    └── feature-2/
        └── .beads/
            └── redirect   # Contains "../../.beads"
```

---

### 🔧 **Implementation Steps**

#### **Step 1: Create Bare Repo (Optional but Recommended)**
```bash
# Convert existing repo to bare repo architecture
git clone --bare <url> <project>/.repo.git
```

#### **Step 2: Create Worktrees Inside Project**
```bash
cd <project>

# Create main worktree
git worktree add main main

# Create feature worktrees
git worktree add worktrees/feature-1 -b feature-1
git worktree add worktrees/feature-2 -b feature-2
```

#### **Step 3: Initialize Shared Beads**
```bash
# Initialize beads in canonical location
cd <project>
mkdir -p .beads
cd .beads
bd init --prefix=myproject
```

#### **Step 4: Create Redirect Files**
```bash
# For main worktree
mkdir -p main/.beads
echo "../.beads" > main/.beads/redirect

# For feature worktrees
mkdir -p worktrees/feature-1/.beads
echo "../../.beads" > worktrees/feature-1/.beads/redirect

mkdir -p worktrees/feature-2/.beads
echo "../../.beads" > worktrees/feature-2/.beads/redirect
```

#### **Step 5: Add to .gitignore**
```bash
# In project root .gitignore
.beads/*.db
.beads/*.db-*
.beads/daemon.*
.beads/redirect
.beads/sync-state.json
.beads/last-touched
```

---

### ⚙️ **Helper Functions**

Here's code your team can use to automate this:

```bash
#!/bin/bash
# setup-worktree-beads.sh

PROJECT_ROOT="$1"
WORKTREE_PATH="$2"
BRANCH_NAME="$3"

# Create worktree inside project
cd "$PROJECT_ROOT"
git worktree add "$WORKTREE_PATH" -b "$BRANCH_NAME"

# Calculate relative path from worktree to canonical .beads
# Example: worktrees/feature-1 -> ../../.beads
DEPTH=$(echo "$WORKTREE_PATH" | tr -cd '/' | wc -c)
REDIRECT_PATH=$(printf '../%.0s' $(seq 1 $((DEPTH + 1)))).beads

# Create redirect file
mkdir -p "$WORKTREE_PATH/.beads"
echo "$REDIRECT_PATH" > "$WORKTREE_PATH/.beads/redirect"

echo "✓ Worktree created at $WORKTREE_PATH"
echo "✓ Beads redirect: $REDIRECT_PATH"
```

---

### 🎯 **Key Benefits**

1. **Single Source of Truth**: All worktrees share one beads database
2. **No Sync Needed**: Changes are immediately visible to all worktrees
3. **Git-Friendly**: Redirect files are gitignored, database is in one place
4. **Clean Structure**: Everything contained within project directory
5. **Scalable**: Works with 1 or 100 worktrees

---

### ⚠️ **Important Gotchas**

1. **Redirect Path Must Be Relative**: Always use `../../.beads`, never absolute paths
2. **Redirect File is Gitignored**: Each worktree creates its own redirect locally
3. **Canonical .beads Must Exist First**: Initialize beads before creating worktrees
4. **Circular Redirects**: Never put a redirect file in the canonical `.beads/` directory
5. **Database Files Are Gitignored**: Only `issues.jsonl` and `config.yaml` are tracked

---

### 📚 **Reference Implementation**

See Gas Town's implementation:
- **Redirect creation**: `internal/polecat/manager.go:setupSharedBeads()`
- **Redirect resolution**: `internal/beads/beads.go:ResolveBeadsDir()`
- **Worktree creation**: `internal/polecat/manager.go:Add()`

---

### 🚀 **Migration Path**

If you currently have worktrees outside the project:

```bash
# 1. Create new structure
mkdir -p myproject/worktrees

# 2. Move existing worktrees
git worktree move ../old-worktree myproject/worktrees/old-worktree

# 3. Update redirect files
echo "../../.beads" > myproject/worktrees/old-worktree/.beads/redirect

# 4. Verify
cd myproject/worktrees/old-worktree
bd list  # Should work!
```

---

### ✅ **Summary for Your Team**

**Tell them:**
1. ✅ **DO**: Create worktrees INSIDE the project directory structure
2. ✅ **DO**: Use a shared `.beads/` directory at project root
3. ✅ **DO**: Create `.beads/redirect` files in each worktree pointing to shared location
4. ❌ **DON'T**: Create worktrees outside the project (e.g., `../worktree-name`)
5. ❌ **DON'T**: Have separate beads databases per worktree
6. ❌ **DON'T**: Commit the redirect files to git

**The pattern is**: One canonical `.beads/` database, multiple worktrees with redirect files pointing to it.
