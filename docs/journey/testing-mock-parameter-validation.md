# Journey: Discovering Mock Parameter Validation Issues

Date: 2025-01-13

## The Problem

While implementing the character orchestrator tests, we discovered that using `DoAndReturn` with `gomock.Any()` was hiding type validation issues. Our tests were passing even though they had fundamental type mismatches.

## The Discovery Process

### Initial Test Implementation

We started with this test for `ListDrafts`:

```go
s.mockDraftRepo.EXPECT().
    List(s.ctx, gomock.Any()).
    DoAndReturn(func(ctx context.Context, opts any) (any, error) {
        return &struct {
            Drafts        []*dnd5e.CharacterDraft
            NextPageToken string
            TotalSize     int32
        }{
            Drafts:        []*dnd5e.CharacterDraft{s.testDraft},
            NextPageToken: "next-page",
            TotalSize:     1,
        }, nil
    })
```

### The Runtime Panic

When running tests, we got:
```
panic: runtime error: invalid memory address or nil pointer dereference
```

The issue: We were returning an anonymous struct that satisfied the interface at compile time, but at runtime the orchestrator was expecting specific types (`draftrepo.ListResult`).

### The First Fix Attempt

We tried to fix by using the mock package types:
```go
Return(&draftrepomock.ListResult{...}, nil)
```

But this failed to compile - there was no `ListResult` in the mock package!

### The Realization

When switching from `gomock.Any()` to explicit parameter matching, we discovered we were using the wrong types entirely:

```go
// ❌ Wrong
List(s.ctx, draftrepomock.ListOptions{...})

// ✅ Correct  
List(s.ctx, draftrepo.ListOptions{...})
```

## The Root Cause

The `DoAndReturn` pattern with `gomock.Any()` was:
1. Accepting any input without type validation
2. Returning any output that satisfied the interface
3. Hiding import/package issues
4. Deferring type errors to runtime

## The Solution

Always use explicit parameter matching when possible:

```go
s.mockDraftRepo.EXPECT().
    List(s.ctx, draftrepo.ListOptions{
        PageSize:  10,
        PageToken: "",
        PlayerID:  s.testPlayerID,
        SessionID: s.testSessionID,
    }).
    Return(&draftrepo.ListResult{
        Drafts:        []*dnd5e.CharacterDraft{s.testDraft},
        NextPageToken: "next-page",
        TotalSize:     1,
    }, nil)
```

## Lessons Learned

1. **Explicit is better than implicit** - Even in tests, being explicit about types catches issues early

2. **Compile-time errors > Runtime errors** - The explicit approach gives compile-time feedback

3. **Test brittleness can be good** - If adding a field breaks tests, that's often what we want

4. **Mock organization matters** - Understanding that mocks only contain the interface, not the types, is crucial

## Impact on Testing Strategy

This discovery led us to establish testing best practices:
- Default to explicit parameter matching
- Use `DoAndReturn` only when necessary for complex validation
- Document when and why we deviate from explicit matching
- Consider test brittleness as a feature, not a bug

## Code Review Checklist

When reviewing tests with mocks:
- [ ] Are parameters explicitly matched?
- [ ] Are return types from the correct package?
- [ ] Would adding a new field to the input break this test?
- [ ] Is `DoAndReturn` justified by complexity?

## References

- [Testing Best Practices](../testing-best-practices.md)
- [GoMock Documentation](https://pkg.go.dev/go.uber.org/mock/gomock)
- Original PR where this was discovered: (Issue #4 - Character Orchestrator)
