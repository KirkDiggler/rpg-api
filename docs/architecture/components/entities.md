---
name: entities
description: Proto-free API domain envelopes and customization data
updated: 2026-09-01
confidence: high — #869 typed hair values, JSON presence, finalization, and clone isolation verified by focused and integration tests
---

# entities

`internal/entities/` holds the proto-free data structures that flow between
handlers, orchestrators, and repositories. Entities are data only; game rules
remain in rpg-toolkit.

## Files

| File | Purpose | Proto contamination? |
|---|---|---|
| `character.go` | Toolkit `character.Data` plus API-owned Appearance | No |
| `character_draft.go` | Toolkit draft data plus API-owned Appearance | No |
| `appearance.go` | Provider-neutral typed hair customization | No |

## Character envelope

`entities.Character` and `entities.CharacterDraft` keep `Appearance` beside,
not inside, toolkit data. Redis serializes the complete API envelope, while
session SDK writes replace only `Character.Data`. This preserves cosmetic state
without teaching rpg-toolkit about presentation.

## Hair semantics (#869)

`Appearance.Hair` is optional. Within hair:

- a nil `Scalp` or `FacialHair` pointer means provider default;
- `StyleSelectionKindNone` means explicit no style;
- `StyleSelectionKindStyle` carries an exact provider-owned `StyleRef`;
- `ColorSRGB *uint32` and `Roughness *float32` retain optional presence, so a
  present zero is distinct from omission.

The shared converter at `internal/converters/customization` is the only
proto/entity mapping used by character handlers and public session roster
projection. API-owned clone helpers deep-copy hair, both selections, and both
optional scalar pointers so clone mutation cannot change its source.

## Boundary status

The package has no proto imports. The pre-#642 proto-contaminated encounter
entities were deleted; new proto-typed fields would be a fresh boundary
regression.
