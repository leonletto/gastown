# gt-aesoh: Auto-provision settings.local.json for autonomous agents

**Issue ID:** gt-aesoh  
**Priority:** P2  
**Type:** task  
**Status:** open

## Problem Statement

When autonomous agents (polecats, witness, refinery, crew) start Claude Code for the first time, they encounter **two blocking prompts**:

1. **Folder Trust Prompt** - "Do you trust the files in this folder?"
2. **Permission Prompts** - Individual prompts for each bash command or file edit

This breaks autonomous execution because:
- The SessionStart hook (`gt prime && gt mail check --inject`) never fires
- Agents sit idle with work assigned but never start
- Manual intervention required for each agent (10+ polecats per rig)

### Current Manual Workaround
```bash
# For each polecat:
tmux attach -t gt-cleanroom-<polecat>
# Accept trust prompt (or clear "1" from buffer)
# Type: gt prime
# Detach and repeat for next polecat
```

### Real-World Impact (2026-01-03)

Deployed 10 polecats to cleanroom rig for reconciliation work:
- All 10 polecats stuck at trust prompt
- Only 1 polecat (furiosa) completed work after manual intervention
- Required manual `gt prime` injection into 9 remaining polecats
- Lost ~30 minutes of autonomous execution time

## Root Cause

Claude Code requires explicit permissions in `.claude/settings.local.json` before allowing autonomous execution. Currently, we only provision `.claude/settings.json` (with hooks) but not `settings.local.json` (with permissions).

## Proposed Solution

Auto-provision `.claude/settings.local.json` during agent creation with appropriate permissions.

### Comprehensive Permission Set (Learned from 10 Working Polecats)

After running 10 polecats through a full work cycle (2026-01-03), we collected all permissions they needed. Here's the **merged comprehensive set**:

```json
{
  "permissions": {
    "allow": [
      "Bash(bd:*)",
      "Bash(cat:*)",
      "Bash(gt:*)",
      "Bash(git:*)",
      "Bash(ls:*)",
      "Bash(grep:*)",
      "Bash(find:*)",
      "Bash(git add:*)",
      "Bash(git commit:*)",
      "Bash(git push:*)",
      "Bash(gt done:*)",
      "Bash(gt hook:*)",
      "Bash(gt mail:*)",
      "Edit(*)"
    ]
  }
}
```

**Note:** Even with `Bash(git:*)` and `Bash(gt:*)`, Claude Code still prompts for specific subcommands like `git add`, `git commit`, `gt done`, etc. These must be explicitly listed.

### Alternative: Wildcard Permissions (Simpler but Less Restrictive)

For fully autonomous agents in controlled environments:

```json
{
  "permissions": {
    "allow": [
      "Bash(*)",
      "Edit(*)",
      "ReadFile(*)",
      "WriteFile(*)"
    ]
  }
}
```

**Trade-off:** Simpler configuration, no future prompts, but grants full system access.

### Implementation Points

1. **Polecat Creation** (`internal/polecat/manager.go`)
   - Provision `settings.local.json` in `AddWithOptions()` after creating `.claude/settings.json`
   - Location: `internal/polecat/manager.go` around line where `.claude/settings.json` is created
   
2. **Witness/Refinery/Crew** (respective setup code)
   - Add similar provisioning during initialization
   - Check existing setup commands in `internal/cmd/`

3. **Template System** (`internal/templates/` or `internal/claude/`)
   - Create reusable template for `settings.local.json`
   - Consider role-specific permission sets if needed
   - Existing: `internal/claude/settings.go` handles `.claude/settings.json`

4. **Existing Agents**
   - Provide migration command: `gt prime --provision-permissions` or similar
   - Or document manual fix in troubleshooting guide

### Files to Modify

Based on codebase analysis:
- `internal/polecat/manager.go` - Add provisioning in `AddWithOptions()`
- `internal/claude/settings.go` - Add `settings.local.json` template/provisioning
- `internal/cmd/hooks.go` or similar - For witness/refinery/crew setup
- `internal/templates/` - If using template-based provisioning

### Testing

- Create new polecat and verify it starts autonomously without prompts
- Verify SessionStart hook fires immediately
- Test with witness, refinery, crew roles
- Verify existing polecats are not affected

### Impact

- **High Value**: Eliminates manual intervention for 10+ agents per rig
- **Low Risk**: Only affects new agent creation, doesn't modify existing behavior
- **Scope**: ~50-100 lines of code across 3-4 files
- **Benefit**: Enables true autonomous execution from first boot

## Related Code

### Current `.claude/settings.json` Provisioning
Found in `internal/polecat/manager.go`:
```go
// Provision commands (which includes .claude/settings.json)
if err := templates.ProvisionCommands(polecatPath); err != nil {
    return fmt.Errorf("failed to provision commands: %w", err)
}
```

### Existing Settings Management
`internal/claude/settings.go` - Handles Claude Code settings

## Empirical Data: Permission Evolution During Work Cycle

Collected from 10 polecats completing reconciliation tasks (2026-01-03 16:25-17:22):

| Polecat | Permissions Added | Timestamp | Status |
|---------|------------------|-----------|--------|
| furiosa, nux | Base set (8 perms) | 16:25 | Completed work with base set |
| slit | +git add, +git commit | 17:19 | Added during commit phase |
| rictus | +git add, +git commit, +gt hook, +gt mail | 17:19 | Most comprehensive |
| dementus, capable, dag, cheedo | +git add, +git commit, +git push | 17:19 | Added during push phase |
| toast, valkyrie | +git add, +git commit, +git push, +gt done | 17:18-17:19 | Full workflow cycle |

**Key Finding:** Polecats that completed full workflow (commit → push → gt done) accumulated 11-14 permissions. The comprehensive set above (14 permissions) represents the union of all observed needs.

## Next Steps

1. Review existing provisioning code in `internal/polecat/manager.go`
2. Check `internal/claude/settings.go` for settings template patterns
3. Implement `settings.local.json` provisioning with comprehensive permission set
4. Test with new polecat creation
5. Document migration path for existing agents
6. Consider: Should we use comprehensive set or wildcard permissions?

---

**Offer:** I'm happy to implement this if you'd like! I already have the gastown codebase context and know exactly where the changes need to go.

