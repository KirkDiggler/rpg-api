# Refactor Plan for Equipment Choice Logic

## What We've Done
1. ✅ Fixed the immediate issue - equipment choices work correctly
2. ✅ Created converter functions to simplify handler code
3. ✅ All tests pass
4. ✅ Addressed Copilot's inline comments

## Current State
- External client does heavy lifting (resolves categories, creates nested choices)
- Orchestrator just passes data through
- Handler uses converter functions for cleaner code

## What Should Be Done (Future PR)

### Phase 1: Move Business Logic to Orchestrator
1. External client should only translate D&D API to domain objects
2. Orchestrator should:
   - Resolve equipment categories
   - Create nested choice structures
   - Fetch item names/descriptions
   - Apply business rules

### Phase 2: Simplify External Client
1. Remove category resolution logic
2. Just map API response to domain objects
3. Keep it as a simple API adapter

### Phase 3: Enhance Orchestrator
1. Add methods for choice enhancement
2. Add caching for equipment categories
3. Add proper error handling
4. Make it testable with mocks

## Benefits of Refactor
- Clear separation of concerns
- Business logic in one place
- Easier to test
- External client becomes simpler
- Handler stays simple

## Decision
For this PR, we'll keep the current implementation with the converter functions.
The full refactor should be a separate PR to avoid scope creep.