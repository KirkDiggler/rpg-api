# Implementation Plan: Feature Activation (Rage) System

**Goal**: Enable players to activate character features (specifically Rage) through the API, see active conditions, and have those conditions affect combat.

**Date**: 2025-11-24
**Context**: Working toward "attack test dummy while raging is active"

---

## Current State Analysis

### ✅ What Already Works

1. **Feature Persistence**: Features ARE stored on characters as `[]json.RawMessage`
2. **Condition Persistence**: Conditions ARE stored as `[]json.RawMessage`
3. **Rage Mechanics**: Rage works in combat via EventBus (adds +2 damage)
4. **Class Resources**: Stored but not exposed (rage uses tracked)
5. **Repository Layer**: Correctly persists all data

### ❌ What's Missing

1. **API Exposure**: Features/Conditions not converted to proto
2. **Activation RPC**: No way to activate features from UI
3. **Status Visibility**: UI can't see if rage is active
4. **Resource Visibility**: Can't see rage uses remaining

---

## Toolkit API Discovery

### Feature Activation Pattern

rpg-toolkit provides a clean activation API:

```go
// From github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/features.Rage

type Rage struct {
    // Has unexported fields
}

// Activate implements core.Action[FeatureInput]
func (r *Rage) Activate(ctx context.Context, owner core.Entity, input FeatureInput) error
func (r *Rage) CanActivate(ctx context.Context, owner core.Entity, input FeatureInput) error
func (r *Rage) GetID() string
func (r *Rage) RestoreOnLongRest()
func (r *Rage) ToJSON() (json.RawMessage, error)

type FeatureInput struct {
    Bus events.EventBus `json:"-"`
}
```

### How It Works

1. **Load character** with EventBus: `char, _ := character.LoadFromData(ctx, data, bus)`
2. **Get feature**: `feature := char.GetFeature("rage")`
3. **Check if can activate**: `err := feature.CanActivate(ctx, char, input)`
4. **Activate**: `err := feature.Activate(ctx, char, input)`
5. **Feature creates condition**: Rage adds "Raging" condition to character
6. **Condition subscribes to events**: Modifies damage in combat
7. **Persist**: `data := char.ToData()` includes new condition in `Conditions []json.RawMessage`

### Condition Behavior

```go
// From github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/events

type ConditionBehavior interface {
    Apply(ctx context.Context, bus events.EventBus) error    // Subscribe to events
    Remove(ctx context.Context, bus events.EventBus) error   // Unsubscribe
    ToJSON() (json.RawMessage, error)                        // For persistence
}
```

Conditions are retrieved via: `char.GetConditions() []ConditionBehavior`

---

## Implementation Plan

### Phase 1: Expose Existing Data (Read-Only)

**Goal**: UI can see features, conditions, and class resources

#### 1.1 Update Proto Definitions (External Dependency)

**Repository**: `github.com/KirkDiggler/rpg-api-protos`

Add to `dnd5e/api/v1alpha1/character.proto`:

```protobuf
message Feature {
  string id = 1;           // e.g., "rage"
  string name = 2;         // e.g., "Rage"
  string description = 3;
  bool can_activate = 4;   // Based on CanActivate() check
  int32 uses_remaining = 5;  // From class resources
  int32 max_uses = 6;
}

message Condition {
  string id = 1;          // e.g., "raging"
  string name = 2;        // e.g., "Raging"
  string description = 3;
  int32 duration_rounds = 4;  // -1 for indefinite
  google.protobuf.Struct state = 5;  // JSON state data
}

message ClassResource {
  string resource_type = 1;  // e.g., "rage_uses"
  int32 current = 2;
  int32 maximum = 3;
  string recovery_type = 4;  // "short_rest", "long_rest"
}

// Update existing Character message
message Character {
  // ... existing fields ...
  repeated Feature features = 20;
  repeated Condition active_conditions = 21;
  map<string, ClassResource> class_resources = 22;
}
```

#### 1.2 Add Converters

**File**: `internal/handlers/dnd5e/v1alpha1/character/converters.go`

After line 1150, add:

```go
// convertFeaturesToProto converts character features to proto
func convertFeaturesToProto(char *toolkitchar.Character, data *toolkitchar.Data) []*dnd5ev1alpha1.Feature {
    features := char.GetFeatures()
    protoFeatures := make([]*dnd5ev1alpha1.Feature, 0, len(features))

    for _, feature := range features {
        // Check if can activate
        canActivate := false
        if activatable, ok := feature.(interface {
            CanActivate(context.Context, core.Entity, features.FeatureInput) error
        }); ok {
            err := activatable.CanActivate(context.Background(), char, features.FeatureInput{})
            canActivate = (err == nil)
        }

        protoFeature := &dnd5ev1alpha1.Feature{
            Id:           feature.GetID(),
            Name:         getFeatureName(feature.GetID()),
            Description:  getFeatureDescription(feature.GetID()),
            CanActivate:  canActivate,
            // TODO: Map to class resources for uses_remaining/max_uses
        }
        protoFeatures = append(protoFeatures, protoFeature)
    }

    return protoFeatures
}

// convertConditionsToProto converts active conditions to proto
func convertConditionsToProto(char *toolkitchar.Character) []*dnd5ev1alpha1.Condition {
    conditions := char.GetConditions()
    protoConditions := make([]*dnd5ev1alpha1.Condition, 0, len(conditions))

    for _, condition := range conditions {
        jsonData, _ := condition.ToJSON()

        // Parse JSON to extract state
        var state map[string]interface{}
        json.Unmarshal(jsonData, &state)

        stateProto, _ := structpb.NewStruct(state)

        protoCondition := &dnd5ev1alpha1.Condition{
            Id:             getConditionID(state),
            Name:           getConditionName(state),
            Description:    getConditionDescription(state),
            DurationRounds: getConditionDuration(state),
            State:          stateProto,
        }
        protoConditions = append(protoConditions, protoCondition)
    }

    return protoConditions
}

// convertClassResourcesToProto converts class resources to proto
func convertClassResourcesToProto(resources map[shared.ClassResourceType]character.ResourceData) map[string]*dnd5ev1alpha1.ClassResource {
    protoResources := make(map[string]*dnd5ev1alpha1.ClassResource)

    for resourceType, resource := range resources {
        protoResources[string(resourceType)] = &dnd5ev1alpha1.ClassResource{
            ResourceType: string(resourceType),
            Current:      resource.Current,
            Maximum:      resource.Maximum,
            RecoveryType: resource.RecoveryType,
        }
    }

    return protoResources
}
```

#### 1.3 Update Character Converter

**File**: `internal/handlers/dnd5e/v1alpha1/character/converters.go`

Modify `convertCharacterDataToProto` (around line 1138):

```go
// Remove the TODO comment and implement:
char.ClassResources = convertClassResourcesToProto(data.ClassResources)

// Add new fields - REQUIRES CHARACTER OBJECT, NOT JUST DATA
// This means we need to modify the handler to load Character, not just Data
```

**CRITICAL ISSUE**: Converters currently receive `*character.Data` but we need `*character.Character` to call `GetFeatures()` and `GetConditions()`.

**Solution**: Handlers must load Character object:

```go
// In handler.go GetCharacter():
// Current: data from repo
// Change to:
bus := events.NewEventBus()
char, err := character.LoadFromData(ctx, data, bus)
defer char.Cleanup(ctx)

// Pass both to converter:
convertCharacterDataToProto(char, data)
```

#### 1.4 Update Handler

**File**: `internal/handlers/dnd5e/v1alpha1/character/handler.go`

Modify `GetCharacter` (around line 505):

```go
func (h *Handler) GetCharacter(ctx context.Context, req *dnd5ev1alpha1.GetCharacterRequest) (*dnd5ev1alpha1.GetCharacterResponse, error) {
    // ... validation ...

    output, err := h.service.GetCharacter(ctx, &character.GetCharacterInput{
        CharacterID: req.CharacterId,
    })
    // ... error handling ...

    // NEW: Load character object for domain logic
    bus := events.NewEventBus()
    char, err := toolkitchar.LoadFromData(ctx, output.CharacterData, bus)
    if err != nil {
        return nil, status.Errorf(codes.Internal, "failed to load character: %v", err)
    }
    defer char.Cleanup(ctx)

    return &dnd5ev1alpha1.GetCharacterResponse{
        Character: convertCharacterToProto(char, output.CharacterData),
    }, nil
}
```

Update converter signature:

```go
func convertCharacterToProto(char *toolkitchar.Character, data *toolkitchar.Data) *dnd5ev1alpha1.Character {
    // ... existing conversions ...

    proto.Features = convertFeaturesToProto(char, data)
    proto.ActiveConditions = convertConditionsToProto(char)
    proto.ClassResources = convertClassResourcesToProto(data.ClassResources)

    return proto
}
```

---

### Phase 2: Feature Activation (Write Operations)

**Goal**: UI can activate rage and see it become active

#### 2.1 Add Proto RPC

**File**: `dnd5e/api/v1alpha1/character.proto`

```protobuf
service CharacterService {
  // ... existing RPCs ...

  rpc ActivateFeature(ActivateFeatureRequest) returns (ActivateFeatureResponse);
}

message ActivateFeatureRequest {
  string character_id = 1;
  string feature_id = 2;  // e.g., "rage"
}

message ActivateFeatureResponse {
  Character character = 1;  // Updated character with new condition
}
```

#### 2.2 Add Service Interface

**File**: `internal/orchestrators/character/service.go`

```go
type ActivateFeatureInput struct {
    CharacterID string
    FeatureID   string
}

type ActivateFeatureOutput struct {
    CharacterData *toolkitchar.Data
    Success       bool
    Message       string  // e.g., "Rage activated!" or error message
}

type Service interface {
    // ... existing methods ...
    ActivateFeature(ctx context.Context, input *ActivateFeatureInput) (*ActivateFeatureOutput, error)
}
```

#### 2.3 Implement Orchestrator

**File**: `internal/orchestrators/character/orchestrator.go`

```go
func (o *Orchestrator) ActivateFeature(ctx context.Context, input *ActivateFeatureInput) (*ActivateFeatureOutput, error) {
    // 1. Validate input
    if input == nil {
        return nil, errors.InvalidArgument("input is required")
    }
    if input.CharacterID == "" {
        return nil, errors.InvalidArgument("character_id is required")
    }
    if input.FeatureID == "" {
        return nil, errors.InvalidArgument("feature_id is required")
    }

    // 2. Load character data
    getOutput, err := o.characterRepo.Get(ctx, characterrepo.GetInput{
        ID: input.CharacterID,
    })
    if err != nil {
        return nil, fmt.Errorf("failed to get character: %w", err)
    }

    // 3. Create EventBus (CRITICAL for conditions to work)
    bus := events.NewEventBus()

    // 4. Load character as domain object
    char, err := toolkitchar.LoadFromData(ctx, getOutput.CharacterData, bus)
    if err != nil {
        return nil, fmt.Errorf("failed to load character: %w", err)
    }
    defer char.Cleanup(ctx)

    // 5. Get the feature
    feature := char.GetFeature(input.FeatureID)
    if feature == nil {
        return &ActivateFeatureOutput{
            CharacterData: getOutput.CharacterData,
            Success:       false,
            Message:       fmt.Sprintf("feature '%s' not found on character", input.FeatureID),
        }, nil
    }

    // 6. Check if feature is activatable
    activatable, ok := feature.(interface {
        CanActivate(context.Context, core.Entity, features.FeatureInput) error
        Activate(context.Context, core.Entity, features.FeatureInput) error
    })
    if !ok {
        return &ActivateFeatureOutput{
            CharacterData: getOutput.CharacterData,
            Success:       false,
            Message:       fmt.Sprintf("feature '%s' cannot be activated", input.FeatureID),
        }, nil
    }

    // 7. Check prerequisites
    featureInput := features.FeatureInput{Bus: bus}
    if err := activatable.CanActivate(ctx, char, featureInput); err != nil {
        return &ActivateFeatureOutput{
            CharacterData: getOutput.CharacterData,
            Success:       false,
            Message:       fmt.Sprintf("cannot activate feature: %v", err),
        }, nil
    }

    // 8. Activate feature
    if err := activatable.Activate(ctx, char, featureInput); err != nil {
        return nil, fmt.Errorf("failed to activate feature: %w", err)
    }

    // 9. Convert back to data (includes new condition in Conditions slice)
    updatedData := char.ToData()

    // 10. Persist updated character
    _, err = o.characterRepo.Update(ctx, characterrepo.UpdateInput{
        ID:   input.CharacterID,
        Data: updatedData,
    })
    if err != nil {
        return nil, fmt.Errorf("failed to save character: %w", err)
    }

    return &ActivateFeatureOutput{
        CharacterData: updatedData,
        Success:       true,
        Message:       fmt.Sprintf("%s activated successfully", feature.GetID()),
    }, nil
}
```

#### 2.4 Implement Handler

**File**: `internal/handlers/dnd5e/v1alpha1/character/handler.go`

```go
func (h *Handler) ActivateFeature(ctx context.Context, req *dnd5ev1alpha1.ActivateFeatureRequest) (*dnd5ev1alpha1.ActivateFeatureResponse, error) {
    // Validate request
    if req.CharacterId == "" {
        return nil, status.Error(codes.InvalidArgument, "character_id is required")
    }
    if req.FeatureId == "" {
        return nil, status.Error(codes.InvalidArgument, "feature_id is required")
    }

    // Call service
    output, err := h.service.ActivateFeature(ctx, &character.ActivateFeatureInput{
        CharacterID: req.CharacterId,
        FeatureID:   req.FeatureId,
    })
    if err != nil {
        return nil, status.Errorf(codes.Internal, "failed to activate feature: %v", err)
    }

    // Load character for conversion
    bus := events.NewEventBus()
    char, err := toolkitchar.LoadFromData(ctx, output.CharacterData, bus)
    if err != nil {
        return nil, status.Errorf(codes.Internal, "failed to load character: %v", err)
    }
    defer char.Cleanup(ctx)

    return &dnd5ev1alpha1.ActivateFeatureResponse{
        Character: convertCharacterToProto(char, output.CharacterData),
    }, nil
}
```

#### 2.5 Add Tests

**File**: `internal/orchestrators/character/orchestrator_test.go`

```go
func (s *OrchestratorTestSuite) TestActivateFeature_Rage_Success() {
    // Setup: Barbarian character with rage feature
    barbarianData := &toolkitchar.Data{
        ID:    "test-barbarian",
        Name:  "Grog",
        Level: 3,
        // ... features include rage ...
    }

    s.mockCharacterRepo.EXPECT().
        Get(gomock.Any(), characterrepo.GetInput{ID: "test-barbarian"}).
        Return(&characterrepo.GetOutput{CharacterData: barbarianData}, nil)

    s.mockCharacterRepo.EXPECT().
        Update(gomock.Any(), gomock.Any()).
        DoAndReturn(func(ctx context.Context, input characterrepo.UpdateInput) (*characterrepo.UpdateOutput, error) {
            // Verify condition was added
            s.Assert().NotEmpty(input.Data.Conditions, "expected condition to be added")
            return &characterrepo.UpdateOutput{Success: true}, nil
        })

    // Execute
    output, err := s.orchestrator.ActivateFeature(context.Background(), &character.ActivateFeatureInput{
        CharacterID: "test-barbarian",
        FeatureID:   "rage",
    })

    // Assert
    s.Require().NoError(err)
    s.Assert().True(output.Success)
    s.Assert().Contains(output.Message, "activated successfully")
}
```

---

### Phase 3: Integration with Combat

**Goal**: Verify rage affects attack damage

#### 3.1 Update Attack Resolution

**File**: `internal/orchestrators/encounter/orchestrator.go`

The attack resolution ALREADY works correctly IF we load the character with EventBus:

```go
// In ResolveAttack (around line 76):
bus := events.NewEventBus()  // ✅ Already present
char, err := character.LoadFromData(ctx, attackerData, bus)  // ✅ Already present

// Rage condition automatically subscribes to DamageChain event
// When combat.ResolveAttack runs, rage adds +2 damage
result, err := combat.ResolveAttack(ctx, attackInput, bus)  // ✅ Already works
```

**No changes needed** - the pattern is already correct!

#### 3.2 Test End-to-End

Flow:
1. Create barbarian character
2. Start dungeon → Combat state initialized
3. Call `ActivateFeature(character_id: "barb-1", feature_id: "rage")`
4. Call `GetCharacter(character_id: "barb-1")` → See "Raging" in active_conditions
5. Call `Attack(attacker_id: "barb-1", target_id: "dummy-1")` → Damage includes +2 bonus
6. Check damage in response → Verify bonus applied

---

## Implementation Order

### Sprint 1: Read-Only Exposure (1-2 days)
- [ ] Update proto definitions (external repo)
- [ ] Add converters for features/conditions/resources
- [ ] Update GetCharacter handler to load Character object
- [ ] Test: Can see features, conditions, resources in API response

### Sprint 2: Feature Activation (1-2 days)
- [ ] Add ActivateFeature proto RPC
- [ ] Implement service interface
- [ ] Implement orchestrator with toolkit integration
- [ ] Implement handler
- [ ] Add comprehensive tests
- [ ] Test: Can activate rage via API

### Sprint 3: Validation & Polish (1 day)
- [ ] Test: Rage shows in active conditions after activation
- [ ] Test: Rage affects attack damage in combat
- [ ] Test: Rage uses decrement on activation
- [ ] Add error handling for edge cases
- [ ] Document API usage patterns

---

## Key Architectural Patterns

### EventBus Pattern (CRITICAL)

```go
// Always use this pattern when working with Character domain object:
bus := events.NewEventBus()
char, err := character.LoadFromData(ctx, data, bus)
defer char.Cleanup(ctx)

// Features and conditions subscribe to bus during LoadFromData
// They participate in combat/mechanics via events
```

### Data vs Domain Object

```go
// character.Data - For storage/transport
// - Features []json.RawMessage
// - Conditions []json.RawMessage
// - Simple struct, JSON serializable

// character.Character - For gameplay
// - GetFeatures() []features.Feature
// - GetConditions() []ConditionBehavior
// - Domain methods, event subscriptions
```

### Conversion Pattern

```go
// Storage → Domain:  LoadFromData(data, bus)
// Domain → Storage:  char.ToData()
// Domain → Proto:    convertCharacterToProto(char, data)
```

---

## Dependencies

### External (Blocking)
- rpg-api-protos repo must be updated first
- Proto generation must run
- go.mod must update proto dependency

### Internal (Non-Blocking)
- Can implement orchestrator/handler logic in parallel with proto updates
- Use TODO comments for proto types until available

---

## Testing Strategy

### Unit Tests
- Orchestrator tests with mocked repositories
- Handler tests with mocked service
- Converter tests with sample data

### Integration Tests
- End-to-end flow: activate → get character → verify condition
- Combat flow: activate rage → attack → verify damage bonus

### Manual Testing
- Use grpcurl to call ActivateFeature
- Verify response includes updated character state
- Verify subsequent GetCharacter shows condition

---

## Success Criteria

✅ UI can see all character features
✅ UI can see which features are activatable
✅ UI can see active conditions (including "Raging")
✅ UI can activate rage via API call
✅ Rage affects attack damage in combat
✅ Rage uses are tracked and decremented
✅ All tests pass

---

## Next Steps

1. **Decision**: Do proto updates first or implement logic with TODOs?
2. **Create issues**: Break plan into GitHub issues
3. **Start implementation**: Begin with Phase 1 (read-only exposure)
4. **Iterate**: Get each phase working before moving to next

---

*This plan provides a clear path from current state (features stored but not exposed) to goal (activate rage and see effects in combat).*
