# Session Notes: Always Run Pre-commit!

Date: 2025-01-13

## The Lesson

We keep hitting CI failures for missing EOF newlines, even though we have `make fix-eof` in our pre-commit workflow.

## The Solution is Already There!

```bash
make pre-commit
```

This command already includes:
1. `fmt` - Format code with gofmt and goimports
2. `tidy` - Clean dependencies with go mod tidy  
3. `fix-eof` - Add missing EOF newlines ← THIS FIXES OUR PROBLEM!
4. `buf-lint` - Lint proto files
5. `lint` - Run golangci-lint
6. `test` - Run unit tests

## Set Up Git Hooks (One Time)

```bash
make install-hooks
```

This will run `make pre-commit` automatically before each git commit!

## Manual Pre-commit

If you prefer to run manually:
```bash
make pre-commit
git add -A
git commit -m "your message"
```

## Why This Matters

- **No more CI failures** for formatting issues
- **No more manual EOF fixes** 
- **Consistent code style** across the team
- **Catch issues before push** not after

## The EOF Fix

The `make fix-eof` command automatically adds newlines to:
- `*.go` files
- `*.proto` files  
- `*.md` files
- `*.yml` and `*.yaml` files
- `*.json` files
- `Makefile`
- `.gitignore`

This matches exactly what the CI checks for!

## Remember

**Before every commit**: `make pre-commit`

Or better yet, install the git hook once and forget about it!