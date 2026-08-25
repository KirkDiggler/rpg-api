---
name: character v2 handler
description: gRPC handler for owner-private v1alpha2 CharacterService reads and out-of-encounter equipment writes
updated: 2026-08-25
confidence: high — #844 strict projection, ownership, no-reload response, four-build mapping, and no-write gates pass
---

# character v2 handler (rpg-api#680, #844)

The v1alpha2 `dnd5e.api.v1alpha2.character.CharacterService` is the authenticated
owner-private character-sheet surface. It serves `GetCharacterData` and edits equipment
between encounters with `EquipItem`/`UnequipItem`. Draft lifecycle and catalog endpoints
remain on the v1alpha1 CharacterService.

## Files

| File | Purpose |
|---|---|
| `internal/handlers/dnd5e/v2/character/handler.go` | auth/ownership gate, request validation, orchestrator delegation |
| `internal/handlers/dnd5e/v2/character/character_data.go` | detached View → CharacterData field mapping |
| `internal/handlers/dnd5e/v2/character/handler_test.go` | ownership, status mapping, errors, and no-reload response gates |

## Methods

- `GetCharacterData` returns the current complete owner-private `CharacterData`.
- `EquipItem` equips one inventory item by Ref ID and opaque slot key.
- `UnequipItem` clears one opaque slot key.

The item Ref's module/type are not interpreted in the API. The toolkit catalog and equip
verb own item and slot rules.

## Strict owner-private flow

All methods authenticate and call `verifyCallerOwnsCharacter` first. Missing and foreign
characters return the same canonical NOT_FOUND code and text; a malformed foreign sheet
still returns NOT_FOUND because private projection is never attempted before ownership.

`GetCharacterData` then calls the orchestrator package's `ProjectView`: strict toolkit
`character.Load` + `character.Attach`, followed by detached `EquipmentView` and
`StatusView`. Malformed condition/feature/item/resource state or an unknown status
descriptor becomes INTERNAL, never a forgiving partial response.

Equip/Unequip receive an already-composed detached post-view from the orchestrator. The
handler performs no repository re-fetch or fallible projection after a successful write.
All three RPCs pass their View through the same non-fallible `BuildCharacterData` mapper.

## CharacterData mapping

`BuildCharacterData` maps equipment and flattened owner-private status field-for-field:
level, current/max HP (temporary remains zero), base speed, structured feature/condition
Refs, optional feature `resource_key`, optional condition `source_member`, and projected
non-magical resources. Spell slots, legacy class resources, and magic status are absent
by construction because the toolkit StatusView does not expose them.

## Wiring and coverage

The handler is registered in the production server and integration harness with the same
character orchestrator used by v1alpha1. Focused tests cover validation, auth ordering,
byte-identical missing/foreign NOT_FOUND, strict owner failure as INTERNAL, one-read
Equip/Unequip responses, level-3 Fighter equipment/status, optional presence, and
representative Fighter/Barbarian/Monk/Rogue mapping.
