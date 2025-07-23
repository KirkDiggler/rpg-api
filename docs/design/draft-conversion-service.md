# Character Draft Conversion Service Design

## Overview

This document outlines the design for a service layer that will centralize the conversion logic between `CharacterDraftData` (storage model) and `CharacterDraft` (domain model).

## Problem Statement

Currently, we have 57 occurrences of conversion calls spread across 8 files, leading to:
- Code duplication
- Difficult testing (need to mock conversions in many places)
- Maintenance overhead when conversion logic changes

## Design Goals

1. **Single Source of Truth**: All conversion logic in one place
2. **Testability**: Easy to mock for unit tests
3. **Extensibility**: Easy to add validation or transformation logic
4. **Performance**: Minimal overhead compared to current approach
5. **Type Safety**: Maintain Go's type safety benefits

## Proposed Architecture

### Option 1: Interface-Based Service (Recommended)

```go
package conversion

// DraftConverter handles conversions between storage and domain models
type DraftConverter interface {
    // ToCharacterDraft converts storage model to domain model
    ToCharacterDraft(data *dnd5e.CharacterDraftData) *dnd5e.CharacterDraft
    
    // FromCharacterDraft converts domain model to storage model
    FromCharacterDraft(draft *dnd5e.CharacterDraft) *dnd5e.CharacterDraftData
    
    // HydrateDraft populates info objects using external data
    HydrateDraft(ctx context.Context, draft *dnd5e.CharacterDraft) (*dnd5e.CharacterDraft, error)
}

// draftConverter is the concrete implementation
type draftConverter struct {
    externalClient external.Client
}

// NewDraftConverter creates a new converter instance
func NewDraftConverter(client external.Client) DraftConverter {
    return &draftConverter{
        externalClient: client,
    }
}
```

### Benefits of This Approach

1. **Dependency Injection**: Easy to mock in tests
2. **Separation of Concerns**: Conversion logic separate from business logic
3. **Extensible**: Can add validation, logging, metrics
4. **Testable**: Mock the interface, not the implementation

### Usage Example

```go
// In orchestrator
type Orchestrator struct {
    draftRepo      draftrepo.Repository
    converter      conversion.DraftConverter
    // ... other deps
}

func (o *Orchestrator) GetDraft(ctx context.Context, input *GetDraftInput) (*GetDraftOutput, error) {
    // Get from repository
    getOutput, err := o.draftRepo.Get(ctx, draftrepo.GetInput{ID: input.DraftID})
    if err != nil {
        return nil, err
    }
    
    // Convert to domain model
    draft := o.converter.ToCharacterDraft(getOutput.Draft)
    
    // Hydrate with external data
    hydratedDraft, err := o.converter.HydrateDraft(ctx, draft)
    if err != nil {
        return nil, err
    }
    
    return &GetDraftOutput{Draft: hydratedDraft}, nil
}
```

## Implementation Plan

### Phase 1: Create Converter Service
1. Create `internal/services/conversion` package
2. Define `DraftConverter` interface
3. Implement `draftConverter` with existing logic
4. Add comprehensive tests

### Phase 2: Migrate Orchestrator
1. Add `DraftConverter` to orchestrator dependencies
2. Replace direct conversion calls with service calls
3. Move `hydrateDraft` logic to converter
4. Update orchestrator tests to mock converter

### Phase 3: Migrate Tests
1. Update test utilities to use converter
2. Create test converter implementation for deterministic tests
3. Remove conversion logic from test builders

### Phase 4: Cleanup
1. Remove conversion methods from `draft_data.go`
2. Update documentation
3. Add performance benchmarks

## Alternative Approaches Considered

### Option 2: Static Functions
Keep current approach but centralize in a package:
```go
package conversion

func ToCharacterDraft(data *dnd5e.CharacterDraftData) *dnd5e.CharacterDraft
func FromCharacterDraft(draft *dnd5e.CharacterDraft) *dnd5e.CharacterDraftData
```

**Pros**: Simple, no interface needed
**Cons**: Harder to mock, can't inject dependencies

### Option 3: Repository Responsibility
Make conversions part of repository:
```go
type Repository interface {
    Get(ctx context.Context, input GetInput) (*dnd5e.CharacterDraft, error)
}
```

**Pros**: Hides storage details completely
**Cons**: Mixes concerns, repository becomes complex

## Decision

Recommend **Option 1 (Interface-Based Service)** because:
- Follows our established patterns
- Provides best testability
- Allows for future enhancements
- Clear separation of concerns

## Migration Strategy

1. **Create service alongside existing code** - No breaking changes
2. **Migrate incrementally** - One file at a time
3. **Maintain backwards compatibility** - Keep old methods during migration
4. **Single PR per phase** - Reviewable chunks

## Success Metrics

- Reduction from 57 to ~10 conversion call sites
- Improved test coverage for conversion logic
- Easier to add new fields to draft models
- Performance within 5% of current implementation