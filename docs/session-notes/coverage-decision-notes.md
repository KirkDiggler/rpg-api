# Coverage Decision Notes - Handler Implementation Session

**Date**: 2025-07-13  
**Context**: PR #14 - Character Handler Implementation

## Coverage Analysis

During the handler implementation, we observed:
- **Overall coverage**: 8.9% (includes generated code, mocks, cmd)
- **Internal coverage**: 44% (focusing on actual source code)
- **New code coverage**: 0% (because entities/interfaces have no direct tests)

## Decision Made

We decided to:
1. **Accept the current coverage** as appropriate for outside-in development
2. **Postpone entity testing** until the service implementation phase
3. **Consider renaming coverage** to "Internal Service Test Coverage" to be more accurate

## Why 0% New Code Coverage is OK Here

The PR added:
- `internal/entities/character.go` - Data structs (no logic to test)
- `internal/entities/constants.go` - Just constants  
- `internal/services/character/service.go` - Interface definition (can't test)
- `internal/handlers/dnd5e/v1alpha1/handler.go` - **44% covered**

The handler tests DO exercise the entities through mocked service returns, but since entities have no test file, they show as 0% covered.

## Future Considerations for Next Session

1. **Entity Testing Strategy**:
   - Add tests only when entities have behavior (validation, methods)
   - Currently they're just data structs
   - Consider if we want minimal "can create" tests

2. **Coverage Calculation**:
   - Update CI to focus on `/internal` only
   - Exclude interfaces from coverage
   - Consider separate metrics for different layers

3. **Test Improvement**:
   - We ARE checking entity values in handler tests (good)
   - Could be more thorough in verifying all fields
   - Consider table-driven tests for enum conversions

## Action Items for Service Implementation Session

- [ ] Revisit entity testing when implementing service layer
- [ ] Decide if entities will have validation methods
- [ ] Update coverage reporting to be more meaningful
- [ ] Consider integration tests that exercise full stack

## Key Principle

The 0% coverage is **honest feedback** - we added code that isn't directly tested. This is intentional in outside-in development, where we define contracts (entities, interfaces) before implementations.
