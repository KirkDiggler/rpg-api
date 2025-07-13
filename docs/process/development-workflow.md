# Development Workflow Process

## Session-Based Development Process

### 1. Session Start
- Check current branch and git status
- Review CLAUDE.md for project context and patterns
- Review milestone/issues to understand current task
- Use TodoWrite to plan and track progress

### 2. Development Phase
- **Always work in feature branches**: `feat/descriptive-name`
- **Follow outside-in approach**: API → Service Interface → Implementation
- **Use Input/Output types everywhere** for future-proofing
- **Test as you go**: Write tests for each method implemented
- **Update documentation**: Keep CLAUDE.md current with patterns learned

### 3. Session End Process
- Complete the current logical unit of work
- Run full test suite to ensure everything passes
- Create meaningful commit with conventional commit format
- Push branch and create detailed PR
- **Leave comprehensive notes** for next session
- Merge PR only after review (or auto-merge if allowed)

## PR Standards

### Title Format
`feat: Brief description (Issue #X)`

### PR Body Must Include
- **Summary**: What was accomplished
- **What's Implemented**: Detailed list of features
- **Architecture Notes**: Patterns followed
- **Test Coverage**: What's tested
- **Next Steps**: Clear handoff to next session

### Commit Message Format
```
type: brief description

Detailed explanation of what was implemented
and why. Include architecture decisions and
key patterns used.

Closes #issue-number

🤖 Generated with [Claude Code](https://claude.ai/code)
Co-Authored-By: Claude <noreply@anthropic.com>
```

## Key Learnings

### Outside-In Development Works
1. Start with API/handlers (just return unimplemented)
2. Define service interfaces with Input/Output types
3. Generate mocks and write handler tests
4. Implement orchestrators/business logic
5. Build repositories/storage last

### Always Use Input/Output Types
- Every function at every layer should use structured types
- Enables easy extension without breaking interfaces
- Makes testing and mocking much cleaner
- Future-proofs against requirement changes

### Mock Organization Pattern
- Place mocks in `mock/` subdirectory next to interface
- Use `<parent>mock` package naming (e.g., `charactermock`)
- Generate with `//go:generate` directive above interface
- Consistent with rpg-toolkit project patterns