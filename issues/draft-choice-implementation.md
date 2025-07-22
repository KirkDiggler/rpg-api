# Implementation Issues for Choice-Based Character Drafts

Based on ADR-003, here are the issues that need to be created:

## High Priority Issues

### Issue 1: Fix UpdateRace validation behavior
**Title**: UpdateRace returns 200 OK even when validation fails
**Labels**: bug, high-priority
**Description**: 
Currently, when race validation fails (e.g., invalid race ID), the orchestrator converts validation errors to warnings and proceeds with the update anyway, returning HTTP 200. This violates REST principles.

**Expected behavior**:
- If validation fails → return 400 Bad Request, don't update
- If validation passes with warnings → update and return 200 with warnings

**Files to change**:
- `/internal/orchestrators/character/orchestrator.go` - UpdateRace, UpdateClass, etc.

---

### Issue 2: Update CharacterDraft proto to support choices
**Title**: Add choice-based structure to CharacterDraft proto
**Labels**: enhancement, proto-change
**Description**:
Update the CharacterDraft message to store player choices instead of computed state.

**Changes needed**:
```protobuf
message CharacterDraft {
  // ... existing fields ...
  
  // Replace equipment, skills, etc. with:
  repeated ChoiceSelection choices = 20;
}

message ChoiceSelection {
  string choice_id = 1;        // From Choice.id in RaceInfo/ClassInfo
  ChoiceType choice_type = 2;  // EQUIPMENT, SKILL, etc.
  string source = 3;           // "race", "class", "background"
  repeated string selected_keys = 4;  // What was selected
}
```

---

## Medium Priority Issues

### Issue 3: Create draft migration strategy
**Title**: Migrate existing drafts to choice-based format
**Labels**: enhancement, migration
**Description**:
Design and implement migration for existing drafts from computed-state to choice-based format.

**Considerations**:
- May not be able to reverse-engineer all choices from current state
- Consider marking old drafts as "legacy" and requiring recreation
- Implement migration in repository layer

---

### Issue 4: Update orchestrator to handle choices
**Title**: Refactor character orchestrator to work with choices
**Labels**: enhancement, refactor
**Description**:
Update all Update* methods in the character orchestrator to:
1. Accept choices instead of computed values
2. Validate choices against available options from engine
3. Store choices in draft, not computed state

**Methods to update**:
- UpdateEquipment → UpdateEquipmentChoices
- UpdateSkills → UpdateSkillChoices
- UpdateLanguages → UpdateLanguageChoices

---

### Issue 5: Implement choice validation service
**Title**: Create service to validate choices against available options
**Labels**: enhancement, new-feature
**Description**:
Create a validation service that:
1. Gets available choices from RaceInfo/ClassInfo via engine
2. Validates player selections are within allowed options
3. Ensures required choices are made
4. Handles nested choices and category expansions

---

### Issue 6: Update finalization to compile choices
**Title**: Implement choice compilation during character finalization
**Labels**: enhancement, critical-path
**Description**:
Update FinalizeDraft to:
1. Validate all required choices are made
2. Compile choices into final character state
3. Add automatic grants from race/class/background
4. Resolve equipment quantities and nested choices

---

## Low Priority Issues

### Issue 7: Add choice tracking to progress
**Title**: Track which choices have been made in draft progress
**Labels**: enhancement, ux
**Description**:
Enhance progress tracking to show:
- Which choices are required vs optional
- Which choices have been made
- What choices remain

---

### Issue 8: Create choice history/audit trail
**Title**: Track choice changes for better UX
**Labels**: enhancement, nice-to-have
**Description**:
Store history of choice changes to enable:
- Undo/redo functionality
- Understanding why certain options were removed
- Better error messages when choices become invalid

---

## Implementation Order

1. **First**: Fix validation bug (Issue 1) - Critical bug
2. **Second**: Update proto (Issue 2) - Blocks everything else
3. **Third**: Create validation service (Issue 5) - Needed by orchestrator
4. **Fourth**: Update orchestrator (Issue 4) - Core functionality
5. **Fifth**: Update finalization (Issue 6) - Complete the flow
6. **Then**: Migration (Issue 3) - Handle existing data
7. **Finally**: UX improvements (Issues 7, 8) - Polish

## Notes
- Each issue should reference ADR-007
- Consider feature flag for gradual rollout
- Ensure backward compatibility during transition period