# Fix Lint Issues

Run golangci-lint and fix all issues systematically.

## Before Starting - Clarify Intent

**Ask the user:**
1. "Should I fix just the issues from your changes, or all lint issues in the codebase?"
2. "For style-related warnings (like importShadow, hugeParam), do you prefer I fix the code or adjust the linter config to allow them?"
3. "Any lint rules you consider non-negotiable vs ones you're flexible on?"

## Process

1. **Run the linter** to see the full picture:
   ```bash
   golangci-lint run 2>&1 | head -100
   ```

2. **Categorize and report** what you find:
   - How many issues total?
   - What categories? (shadows, errcheck, style, etc.)
   - Which are clear bugs vs style preferences?

3. **Propose a plan** and get approval before making changes:
   - "I found X shadow declarations - these are usually real issues, ok to fix?"
   - "There are Y importShadow warnings in converters - this is a style check, should I disable it or rename variables?"
   - "The dupl checker flagged bidirectional enum converters - this is a false positive, ok to exclude converters.go?"

4. **For config changes**, always explain the tradeoff:
   - What the check catches
   - Why it's triggering here (false positive? intentional pattern?)
   - What we lose by disabling it

## Common Decisions to Clarify

| Issue Type | Question to Ask |
|------------|-----------------|
| Shadow declarations | "Fix by renaming, or is the shadow intentional?" |
| importShadow | "Rename variable or disable check? (noisy in converters)" |
| hugeParam | "Pass by pointer or disable? (adds complexity for read-only data)" |
| dupl in converters | "Refactor or exclude? (bidirectional mappers are inherently similar)" |
| errcheck on Close() | "Handle error or suppress? (usually safe to ignore)" |
| gocyclo high complexity | "Refactor or raise threshold? (enum switches are naturally complex)" |

## Known Patterns in This Codebase

These have been discussed and decided:

- **Converters**: Excluded from `dupl` - bidirectional enum mappers are intentionally similar
- **Test files**: Relaxed rules for dupl, errcheck, goconst, gosec, gocyclo, unparam, lll
- **Proto boundaries**: G115 (int->int32) excluded in handlers - D&D values are small
- **Style checks disabled**: importShadow, hugeParam, rangeValCopy, typeAssertChain, ifElseChain, paramTypeCombine, unnamedResult

## After Fixing

1. Run `golangci-lint run` to verify clean
2. Run `make pre-commit` to ensure tests still pass
3. Summarize what was changed (code fixes vs config changes)
