# 🚨 CRITICAL: NEVER USE --no-verify 🚨

## ABSOLUTE RULE: NO EXCEPTIONS

**NEVER, EVER, EVER use `git commit --no-verify`**

### Why This Rule Exists
1. **CI will fail anyway** - The same checks run in CI, so skipping locally just wastes time
2. **Broken builds** - Skipping pre-commit means pushing broken code
3. **Wasted PR cycles** - PRs will fail CI and need to be fixed and re-pushed
4. **Team disruption** - Others can't work with broken code

### What Happened (Never Repeat This)
- Used `--no-verify` to skip linter errors
- Created PR #22 that will fail CI
- Now need to fix and force-push
- Wasted time and created unnecessary churn

### The Right Way
When pre-commit fails:
1. **Fix the issues** - Always fix what the linter finds
2. **Understand why** - If linter seems wrong, investigate why
3. **Update config** - If linter is genuinely wrong, update .golangci.yml
4. **Ask for help** - If stuck, ask rather than skip

### Common Fixes
- **Testify suite errors**: The linter sometimes can't resolve embedded methods. Add explicit type assertions or update linter config
- **Import errors**: Run `go mod tidy`
- **Format errors**: Run `make fmt`

### Remember
- Pre-commit checks exist to help, not hinder
- CI runs the same checks - you can't escape them
- Fixing locally is faster than fixing in PR
- Quality > Speed

## DO NOT DELETE THIS FILE
This is a permanent reminder. Every time you think about using --no-verify, read this first.

---
Created: 2025-01-13
Context: PR #22 - Error handling package
Mistake: Used --no-verify to skip linter errors on testify suite methods
