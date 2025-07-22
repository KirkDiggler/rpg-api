# Proto and API Issues for Choice-Based Drafts

## Issue 1: Update CharacterDraft Proto Definition

**Title**: Update CharacterDraft proto to support choice-based structure (ADR-007)
**Labels**: proto-change, breaking-change, enhancement
**Milestone**: v1alpha2

### Description
Based on ADR-007, update the CharacterDraft message to store player choices instead of computed state.

### Current Structure (to be replaced)
```protobuf
message CharacterDraft {
  // ... identity fields ...
  repeated string starting_skill_ids = 11;
  repeated string additional_languages = 12;
  map<string, ChoiceSelection> choice_selections = 13;
  // ... other computed fields ...
}
```

### New Structure
```protobuf
message CharacterDraft {
  // Identity fields remain the same
  string id = 1;
  string player_id = 2;
  string session_id = 3;
  string name = 4;
  Race race_id = 5;
  Subrace subrace_id = 6;
  Class class_id = 7;
  Background background_id = 8;
  
  // REMOVE computed fields (starting_skill_ids, equipment, etc.)
  // ADD choice tracking
  repeated ChoiceSelection choices = 20;
  
  // Keep metadata fields
  CharacterDraftProgress progress = 30;
  int64 expires_at = 31;
  int64 created_at = 32;
  int64 updated_at = 33;
}

// New message for tracking choices
message ChoiceSelection {
  string choice_id = 1;         // ID from Choice in RaceInfo/ClassInfo
  ChoiceType choice_type = 2;   // EQUIPMENT, SKILL, etc.
  string source = 3;            // "race", "class", "background"
  repeated string selected_keys = 4;  // What was selected
  
  // For ability score choices
  repeated AbilityScoreChoice ability_score_choices = 5;
}

message AbilityScoreChoice {
  Ability ability = 1;
  int32 bonus = 2;
}
```

### Breaking Changes
- Clients can no longer read computed state from drafts
- Must call new endpoints to get computed preview

---

## Issue 2: Add Draft Preview Endpoint

**Title**: Add GetDraftPreview RPC to compute draft state
**Labels**: enhancement, api-change
**Depends on**: Issue 1

### Description
Since drafts will only store choices, we need an endpoint to preview what the character would look like if finalized.

### Proto Addition
```protobuf
service CharacterDraftService {
  // ... existing RPCs ...
  
  // Get a preview of what the character would look like if finalized
  rpc GetDraftPreview(GetDraftPreviewRequest) returns (GetDraftPreviewResponse);
}

message GetDraftPreviewRequest {
  string draft_id = 1;
}

message GetDraftPreviewResponse {
  CharacterDraft draft = 1;  // The draft with choices
  Character preview = 2;     // Computed character state
  repeated ValidationWarning warnings = 3;
  repeated ValidationError errors = 4;
}
```

### Implementation Notes
- Computes full character state from choices
- Does NOT persist anything
- Returns validation errors/warnings
- Useful for UI to show "live" preview

---

## Issue 3: Update Draft Modification RPCs

**Title**: Update draft RPCs to accept choices instead of computed values
**Labels**: breaking-change, api-change
**Depends on**: Issue 1

### Description
All Update* RPCs need to work with choices, not final values.

### Changes Needed

#### UpdateSkills
```protobuf
// OLD
message UpdateSkillsRequest {
  string draft_id = 1;
  repeated Skill skills = 2;  // Final skill list
}

// NEW
message UpdateSkillsRequest {
  string draft_id = 1;
  repeated SkillChoice skill_choices = 2;
}

message SkillChoice {
  string choice_id = 1;    // Which choice this is for
  repeated Skill skills = 2;  // Selected skills
}
```

#### UpdateEquipment
```protobuf
// OLD - doesn't exist yet, but would have been:
message UpdateEquipmentRequest {
  string draft_id = 1;
  repeated Equipment equipment = 2;
}

// NEW
message UpdateEquipmentChoicesRequest {
  string draft_id = 1;
  repeated EquipmentChoice equipment_choices = 2;
}

message EquipmentChoice {
  string choice_id = 1;       // Which choice this is for
  repeated string selected_keys = 2;  // Equipment keys selected
  // For nested choices
  repeated EquipmentChoice nested_choices = 3;
}
```

### Similar updates needed for:
- UpdateLanguages → UpdateLanguageChoices
- UpdateProficiencies → UpdateProficiencyChoices
- Any future choice-based updates

---

## Issue 4: Add Validation-Only Mode to Updates

**Title**: Fix draft update RPCs to fail on validation errors
**Labels**: bug, high-priority
**References**: ADR-007

### Description
Currently, update RPCs return 200 OK with warnings even when validation fails. This violates REST/gRPC principles.

### Required Behavior
1. If validation fails → return error (don't update)
2. If validation passes with warnings → update and return success with warnings

### Code Change Pattern
```go
// In orchestrator
validateOutput, err := o.engine.ValidateRaceChoice(ctx, validateInput)
if err != nil {
    return nil, errors.Wrap(err, "failed to validate race choice")
}

// NEW: Check if valid before proceeding
if !validateOutput.IsValid {
    return nil, errors.ValidationFailed("race choice validation failed", validateOutput.Errors)
}

// Only convert to warnings if there are non-fatal issues
var warnings []ValidationWarning
if len(validateOutput.Warnings) > 0 {
    warnings = convertValidationWarningsToProto(validateOutput.Warnings)
}
```

---

## Migration Notes

### Backward Compatibility Strategy
1. Add feature flag: `enable_choice_based_drafts`
2. Support both formats during transition:
   - Read: Check format version, handle both
   - Write: Use flag to determine format
3. Migration endpoint: `POST /api/v1alpha1/drafts/{id}/migrate`
4. After all clients updated, remove old format support

### Client Update Guide
Clients will need to:
1. Stop reading computed fields from drafts
2. Use GetDraftPreview for display
3. Send choices to Update* endpoints
4. Handle new validation error responses

### Timeline
- Phase 1: Add new proto messages (non-breaking)
- Phase 2: Add new endpoints alongside old ones
- Phase 3: Migrate clients to new endpoints
- Phase 4: Deprecate old endpoints
- Phase 5: Remove old endpoints (breaking change)
