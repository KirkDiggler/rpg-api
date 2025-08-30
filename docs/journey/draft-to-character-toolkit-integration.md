# Draft-to-Character: Toolkit Integration Journey

## The Problem We're Solving

Our orchestrator has grown to 1600+ lines with proto types leaking into business logic. We're duplicating logic that rpg-toolkit already provides. Time to fix this architectural debt.

## What Toolkit Provides (And We Should Use)

### Persistent Types (For Storage)
```go
// These go directly into/from the database
character.DraftData   // Draft storage representation
character.Data        // Character storage representation  
character.ChoiceData  // Choice with source tracking
```

### Domain Objects (For Business Logic)
```go
// These have methods and behavior
character.Draft       // Has ToCharacter(), ValidateChoices()
character.Character   // Has Attack(), ApplyCondition(), etc.
```

### Validation Types
```go
choices.ValidationResult  // Rich validation with severity levels
choices.ValidationIssue   // Detailed problems with codes
```

### Key Methods We're Not Using
- `Draft.ToCharacter(raceData, classData, backgroundData)` - Converts completed draft
- `Draft.ValidateChoices()` - Returns comprehensive ValidationResult
- `LoadDraftFromData(DraftData)` - Creates domain object from storage
- `LoadCharacterFromData(Data)` - Creates domain object from storage

## Current Anti-Patterns

### 1. Proto Leakage
```go
// BAD: Proto type in orchestrator
type GetDraftOutput struct {
    Draft      *character.DraftData
    Validation *pb.ValidationResult  // ← Proto!
}

// GOOD: Use toolkit type
type GetDraftOutput struct {
    Draft      *character.DraftData
    Validation *choices.ValidationResult  // ← Domain type
}
```

### 2. Manual Character Assembly
```go
// BAD: 300 lines of manual assembly in FinalizeDraft
characterData := &toolkitchar.Data{
    ID:       o.idGen.Generate(),
    PlayerID: draft.PlayerID,
    // ... 298 more lines of manual mapping
}

// GOOD: Let toolkit do it
draft := character.LoadDraftFromData(draftData)
char, err := draft.ToCharacter(raceData, classData, backgroundData)
```

### 3. Duplicate Validation
```go
// BAD: Custom validation logic
func (o *Orchestrator) validateClassRequirements(...) ([]ValidationWarning, error)

// GOOD: Use toolkit validation
result, err := draft.ValidateChoices()
```

## The Clean Architecture

```
[gRPC Handler]
    ↓ converts pb.Request → Input
[Orchestrator] 
    ↓ uses toolkit types only
[Repository]
    ↓ stores toolkit.Data types
[Database]
```

### Handler Layer (Conversion Only)
```go
func (h *Handler) FinalizeDraft(ctx, req *pb.Request) (*pb.Response, error) {
    // Convert proto → domain
    input := convertProtoToInput(req)
    
    // Call orchestrator (pure domain)
    output, err := h.orchestrator.FinalizeDraft(ctx, input)
    
    // Convert domain → proto
    return convertOutputToProto(output), nil
}
```

### Orchestrator Layer (Pure Domain)
```go
func (o *Orchestrator) FinalizeDraft(ctx, input *FinalizeDraftInput) (*FinalizeDraftOutput, error) {
    // Get draft data from repo
    draftData := o.draftRepo.Get(ctx, input.DraftID)
    
    // Create domain object
    draft := character.LoadDraftFromData(draftData)
    
    // Get external data
    raceData := o.getRaceData(draft.RaceChoice.RaceID)
    classData := o.getClassData(draft.ClassChoice.ClassID)
    backgroundData := o.getBackgroundData(draft.BackgroundChoice)
    
    // Let toolkit do the work!
    char, err := draft.ToCharacter(raceData, classData, backgroundData)
    
    // Store the result
    o.charRepo.Create(ctx, char.ToData())
    
    return &FinalizeDraftOutput{Character: char.ToData()}, nil
}
```

## Migration Strategy

### Step 1: Replace Proto Types (Quick Win)
- Replace `pb.ValidationResult` with `choices.ValidationResult`
- Move proto converters to handler layer
- Remove proto imports from orchestrator

### Step 2: Use Toolkit Loading (Medium)
- Replace manual struct creation with `LoadDraftFromData()`
- Replace manual struct creation with `LoadCharacterFromData()`
- Let toolkit validate the data on load

### Step 3: Simplify Finalization (Big Win)
- Replace 300-line FinalizeDraft with `draft.ToCharacter()`
- Remove duplicate validation logic
- Remove manual character assembly

### Step 4: Clean Repository Layer
- Store `character.DraftData` directly
- Store `character.Data` directly
- No conversion needed!

## Benefits

1. **Delete 500+ lines** of duplicate logic
2. **Single source of truth** - toolkit owns the rules
3. **Better validation** - toolkit knows all the rules
4. **Clean architecture** - protos only at the edge
5. **Easier updates** - change toolkit, everything updates

## The End State

```go
// orchestrator.go shrinks from 1600 to ~400 lines
// Just coordination, no complex logic

// handler.go has all proto conversion
// Clean separation of concerns

// repository just stores/retrieves toolkit types
// No custom types needed
```

## Tracking Issue

See [Issue #230](https://github.com/KirkDiggler/rpg-api/issues/230) for implementation tracking.