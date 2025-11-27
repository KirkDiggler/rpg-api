# Fix Lint Issues

Run golangci-lint and fix all issues systematically. This command handles the common patterns we encounter.

## Instructions

1. **Run the linter first** to get the full issue list:
   ```bash
   golangci-lint run 2>&1
   ```

2. **Fix issues by category** (in this order to avoid cascading fixes):

   ### Config Issues (if linter fails to run)
   - Check `.golangci.yml` is valid v2 format
   - `settings` goes under `linters`, not at root
   - `exclusions` replaces `issues.exclude-rules`

   ### Shadow Declarations (govet)
   - `if err := ...` shadowing outer `err` → use `err = ...` or rename inner variable
   - Defer functions: `if err := conn.Close()` → `if closeErr := conn.Close()`

   ### Errcheck
   - `defer conn.Close()` → `defer func() { _ = conn.Close() }()`
   - Or handle the error if it matters

   ### Unconvert
   - Remove unnecessary type conversions like `int(x)` when x is already int

   ### Staticcheck S1001
   - Replace manual copy loops with `copy(dst, src)`

   ### Ineffassign
   - Remove assignments to variables that are never read afterward

   ### Noctx
   - `net.Listen()` → `net.ListenConfig{}.Listen(ctx, ...)`

3. **For noisy style checks**, add to `.golangci.yml` disabled-checks:
   - `importShadow` - too noisy for converters with matching package/variable names
   - `hugeParam` - not worth pointer overhead for read-only data
   - `rangeValCopy` - often false positive when value copy is needed
   - `typeAssertChain` - type switches aren't always cleaner for 2 cases
   - `ifElseChain` - switches aren't always cleaner for error checks
   - `paramTypeCombine` - explicit params improve readability
   - `unnamedResult` - named results not always clearer

4. **For duplicate code (dupl)**:
   - Bidirectional enum converters are false positives
   - Add exclusion: `- path: "converters\\.go"` with `linters: [dupl]`
   - Or increase threshold if needed (250-300 for large enum switches)

5. **Verify clean** before finishing:
   ```bash
   golangci-lint run 2>&1
   make pre-commit
   ```

## Common Exclusion Patterns for .golangci.yml

```yaml
linters:
  exclusions:
    rules:
      # Test files get relaxed rules
      - path: _test\.go
        linters: [dupl, errcheck, goconst, gosec, gocyclo, unparam, lll]

      # Converters have legitimate duplication
      - path: "converters\\.go"
        linters: [dupl]

      # G115 int->int32 safe at proto boundaries
      - linters: [gosec]
        text: "G115:"
        path: "handlers/"
```
