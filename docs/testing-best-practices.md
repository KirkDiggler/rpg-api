# Testing Best Practices

## Mock Testing Guidelines

### Prefer Explicit Parameter Matching Over DoAndReturn

**Issue**: Using `DoAndReturn` with `gomock.Any()` can hide type issues and field validation problems.

**Example of the Problem**:
```go
// ❌ BAD: This test passes even if we add new required fields
s.mockDraftRepo.EXPECT().
    List(s.ctx, gomock.Any()).
    DoAndReturn(func(ctx context.Context, opts any) (any, error) {
        // This creates an anonymous struct that satisfies the interface
        // but doesn't validate the actual types or fields passed
        return &struct {
            Drafts        []*dnd5e.CharacterDraft
            NextPageToken string
            TotalSize     int32
        }{
            Drafts: []*dnd5e.CharacterDraft{s.testDraft},
        }, nil
    })
```

**Why This Is Bad**:
1. If we add a new field to `ListOptions`, this test still passes
2. Type mismatches are hidden (e.g., using wrong import aliases)
3. The test doesn't verify the parameters were constructed correctly
4. Runtime panics instead of compile-time errors

**Example of the Fix**:
```go
// ✅ GOOD: Explicit parameter matching with correct types
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

**Benefits**:
1. Compile-time type checking
2. Explicit field validation
3. Tests fail if new required fields are added
4. Clear documentation of expected behavior

### When DoAndReturn Is Appropriate

Use `DoAndReturn` only when you need to:
- Inspect complex nested structures
- Perform side effects (like capturing values)
- Implement conditional logic based on input

**Example of Appropriate Use**:
```go
// ✅ OK: Need to validate complex nested structure
s.mockDraftRepo.EXPECT().
    Create(s.ctx, gomock.Cond(func(x interface{}) bool {
        draft, ok := x.(*dnd5e.CharacterDraft)
        return ok && 
            draft.PlayerID == s.testPlayerID &&
            draft.Progress.HasName == (draft.Name != "") &&
            draft.Progress.CompletionPercentage > 0
    })).
    Return(nil)
```

## Test Organization

### Use Suite-Based Testing

Following the patterns from rpg-toolkit and this codebase:

```go
type ServiceTestSuite struct {
    suite.Suite
    ctrl         *gomock.Controller
    mockDep      *MockDependency
    service      *Service
    
    // Common test data
    testID       string
    testEntity   *Entity
}

func (s *ServiceTestSuite) SetupTest() {
    // Runs before EACH test method
    s.ctrl = gomock.NewController(s.T())
    s.mockDep = NewMockDependency(s.ctrl)
    
    // Initialize test data
    s.testID = "test-123"
}

func (s *ServiceTestSuite) SetupSubTest() {
    // Runs before EACH s.Run()
    // Reset test data to clean state
}
```

### Test Data Management

**Principle**: Keep test data close to tests, but reuse common fixtures

```go
// Common fixtures in suite
s.testDraft = &dnd5e.CharacterDraft{
    ID:       s.testDraftID,
    PlayerID: s.testPlayerID,
    // ... base fields
}

// Test-specific variations
testCases := []struct {
    name      string
    input     *Input
    draft     *dnd5e.CharacterDraft  // Override if needed
    setupMock func()
    wantErr   bool
    validate  func(*Output)
}{
    {
        name:  "specific scenario",
        draft: s.testDraft,  // Use common fixture
    },
    {
        name: "variation needed",
        draft: &dnd5e.CharacterDraft{
            // Custom draft for this test
        },
    },
}
```

## Coverage Guidelines

### Target Coverage by Layer

Based on our experience with the orchestrator:

- **Handlers**: 40-50% (mostly translation logic)
- **Orchestrators/Services**: 70-80% (business logic)
- **Repositories**: 60-70% (CRUD operations)
- **Entities**: Test through usage (no dedicated tests)

### What Not to Test

- Generated code (mocks, proto)
- Simple getters/setters
- Framework boilerplate

### Integration vs Unit Tests

**Unit Tests**: Mock all external dependencies
```go
// Mock repository, engine, external clients
s.mockRepo.EXPECT().Get(ctx, id).Return(entity, nil)
```

**Integration Tests**: Use real dependencies where safe
```go
// Use real Redis via miniredis
// Mock only external APIs
```

## Common Testing Patterns

### Error Testing

Always test both success and failure paths:

```go
testCases := []struct {
    name      string
    setupMock func()
    wantErr   bool
    errMsg    string  // Expected error substring
}{
    {
        name: "repository error",
        setupMock: func() {
            s.mockRepo.EXPECT().
                Get(s.ctx, s.testID).
                Return(nil, errors.New("connection failed"))
        },
        wantErr: true,
        errMsg:  "failed to get",  // Check error wrapping
    },
}
```

### Validation Testing

Test validation at boundaries:

```go
// Input validation
{
    name:    "nil input",
    input:   nil,
    wantErr: true,
    errMsg:  "input is required",
},
{
    name: "empty required field",
    input: &Input{
        RequiredField: "",
    },
    wantErr: true,
    errMsg:  "required field is required",
},
```

## Debugging Test Failures

### Type Mismatch Issues

**Symptom**: Runtime panic about invalid memory address
**Cause**: Mock returning wrong type
**Fix**: Check imports and use correct types from correct packages

### Missing Mock Expectations

**Symptom**: "missing call(s) to *Mock.Method"
**Cause**: Code path calling unmocked method
**Fix**: Add missing expectation or check test logic

### Unexpected Calls

**Symptom**: "Unexpected call to *Mock.Method"
**Cause**: Business logic calling method not expected in test
**Fix**: Either add expectation or fix business logic

## References

- [Testify Suite Documentation](https://pkg.go.dev/github.com/stretchr/testify/suite)
- [GoMock Documentation](https://pkg.go.dev/go.uber.org/mock/gomock)
- [Go Testing Best Practices](https://go.dev/doc/tutorial/fuzz)