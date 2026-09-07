---
name: entities
description: Proto-free API domain envelopes and customization data
updated: 2026-09-04
confidence: high — #897 complete toolkit-owned Appearance shape, JSON presence, finalization, clone isolation, and Docker-backed integration verified
---

# entities

`internal/entities/` holds the proto-free data structures that flow between
handlers, orchestrators, and repositories. Entities are data only; game rules
remain in rpg-toolkit.

## Files

| File | Purpose | Proto contamination? |
|---|---|---|
| `character.go` | Storage wrapper around toolkit `character.Data` | No |
| `character_draft.go` | Storage wrapper around toolkit draft data | No |

## Toolkit-owned character data

`entities.Character` and `entities.CharacterDraft` are storage wrappers only;
their sole field is respectively `*character.Data` or `*character.DraftData`.
Appearance is nested in those toolkit data types and Redis serializes that shape
directly. The session SDK therefore saves complete `Data`, including Appearance,
without an API-side preservation envelope.

Character handlers use the shared `internal/converters/customization` mapping for
proto↔toolkit Appearance. The Session handler separately maps the Session SDK's flat
public roster customization values to its wire types, preserving nil/empty nested
messages, selection oneofs, optional scalar presence, and present zero values.
Validation and provider interpretation remain in rpg-toolkit.

## Boundary status

The package has no proto imports. The pre-#642 proto-contaminated encounter
entities were deleted; new proto-typed fields would be a fresh boundary
regression.
