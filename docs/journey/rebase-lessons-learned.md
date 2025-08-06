# Rebase Lessons Learned: The Draft ID Regression

## What Happened

During PR #180 (racial traits), we rebased onto main and lost critical fixes:
1. Separate ID generators for drafts vs characters
2. ListCharacters implementation
3. These fixes silently disappeared - no merge conflicts warned us

## Why It Happened

Rebasing rewrites history by replaying commits on top of the target branch. When the branch being rebased is older:
- It doesn't know about recent fixes in main
- Git picks the "rebased" version (the older code) when there's no direct conflict
- Critical fixes can be silently lost

## Better Approach: Merge Instead of Rebase

### When to Use Merge
**Default to merge for feature branches:**
```bash
git checkout feature-branch
git merge main
# Resolve conflicts if any
git push
```

Benefits:
- Preserves complete history
- Conflicts are explicit and must be resolved
- Can't accidentally lose code
- Easier to trace what happened

### When Rebase Might Be OK
Only for very simple, short-lived branches:
- Single commit fixes
- No other PRs merged since branch creation
- You're 100% sure of the changes

### Pre-Rebase Checklist (If You Must)

Before ANY rebase:

1. **Document current state**
   ```bash
   git diff main..HEAD > /tmp/my-changes.patch
   ```

2. **Check what's been merged recently**
   ```bash
   git log --oneline main..origin/main
   gh pr list --state merged --limit 5
   ```

3. **Identify critical areas**
   - ID generation
   - Service implementations  
   - Handler implementations
   - Repository methods

4. **After rebase, verify nothing was lost**
   ```bash
   # Check critical files explicitly
   git show HEAD:cmd/server/server.go | grep -A2 "IDGenerator"
   git show HEAD:internal/handlers/dnd5e/v1alpha1/handler.go | grep -A10 "ListCharacters"
   ```

## Red Flags That Should Stop a Rebase

- Branch is more than 2 days old
- Multiple PRs merged since branch creation
- Touching core infrastructure (server.go, handlers, orchestrators)
- Any uncertainty about what's in main

## The Simple Rule

**When in doubt, merge instead of rebase.**

The "messy" merge commit history is worth avoiding silent code loss.

## This Specific Case

What we should have done for PR #180:
```bash
git checkout feat/racial-traits
git merge main
# See explicit conflicts
# Carefully resolve keeping both features
git push
```

Instead of:
```bash
git rebase main  # Silently lost the fixes
```

## Going Forward

1. Default to merge for feature branches
2. Only rebase if you created the branch TODAY
3. Always verify critical code after any rebase
4. Document in PR description if you rebased and what you verified

Remember: A "clean" history isn't worth losing working code.
