---
name: character v2 handler
description: gRPC handler for the v1alpha2 CharacterService — out-of-encounter equip/unequip
updated: 2026-07-21
confidence: high — verified by reading handler.go, handler_test.go, and the shared orchestrator/projection code it calls
---

# character v2 handler (rpg-api#680)

The v1alpha2 `dnd5e.api.v1alpha2.character.CharacterService` is the out-of-encounter
character-sheet surface: `EquipItem`/`UnequipItem` for sheet editing between
encounters. It is deliberately narrow — two RPCs, no draft lifecycle, no data-loading
endpoints. The full character creation/management surface stays on the v1alpha1
`CharacterService` (`docs/architecture/components/character-handler.md`); this handler
exists only because equip/unequip needed a v1alpha2 wire shape (`Ref` + `slot_key`,
keys-not-enums) and a response carrying the new `CharacterData` equipment fields
(`docs/architecture/components/encounter.md`'s CharacterData projection section).

The in-encounter equivalent — equipping mid-combat, with an action-economy cost — does
not exist yet. When it lands, it rides the encounter stream, not this service
(rpg-project#94).

## Files

| File | Purpose |
|---|---|
| `internal/handlers/dnd5e/v2/character/handler.go` | `Handler` struct + constructor + `EquipItem`/`UnequipItem` |
| `internal/handlers/dnd5e/v2/character/handler_test.go` | Unit suite (gomock) — validation, delegation, error→gRPC-status mapping, response shape |

No `converters.go` — proto↔domain translation here is small enough to live inline in
`handler.go` (item Ref → toolkit item ID, slot key string → `character.InventorySlot`),
unlike the v1alpha1 handler's 3,132-line converter file.

## gRPC methods handled

- `EquipItem(EquipItemRequest{character_id, item: Ref, slot_key}) → EquipItemResponse{character: CharacterData}`
- `UnequipItem(UnequipItemRequest{character_id, slot_key}) → UnequipItemResponse{character: CharacterData}`

`item` is a `Ref` (module/type/id), not a bare item id string — the handler resolves
`ref.Id` as the toolkit item ID. Module/type on the Ref are NOT validated here;
validating "what makes a Ref a valid item ref" would require rules knowledge this
handler deliberately doesn't have.

## Layering

This handler is a thin wrapper — proto conversion only, no business logic:

1. Validate the envelope (`character_id`, `item`/`item.id`, `slot_key` non-empty) →
   `apierr.InvalidArgument`.
2. Delegate to `character.Service.EquipItem`/`UnequipItem`
   (`internal/orchestrators/character` — the SAME orchestrator the v1alpha1 handler
   uses; see `character-orchestrator.md`'s Equipment section for what that method
   actually does). Orchestrator errors are wrapped with `apierr.ToGRPCError`.
3. Re-fetch via `characterService.GetCharacter`, load the runtime character
   (`character.LoadFromData`), and compose the response via
   `encounterhandler.BuildEquipmentCharacterData` — the SAME composition function the
   v1alpha2 encounter snapshot path uses (`internal/handlers/dnd5e/v2/encounter/
   character_data.go`). One composition, two callers: the character sheet and the
   encounter HUD never independently drift.

`armor_class_detail.total` is the ONLY AC total on this response — there is no
surrounding `Entity.armor_class` to keep in sync with, unlike the encounter snapshot
(see `CharacterData.armor_class_detail`'s doc comment in the proto for the full
rationale on why the field is duplicated there but not here).

## Wiring

Registered in both `cmd/server/server.go` and `internal/integration/harness/harness.go`,
sharing the SAME `character.Service` instance the v1alpha1 `CharacterService` uses — no
separate orchestrator construction.

## Test coverage

`handler_test.go` — gomock suite: validation errors (missing `character_id`/`item`/
`slot_key`), success path (asserts `armor_class_detail` is real and non-zero, `equipped`
contains the newly-equipped slot, `inventory` includes the equipped item per the wire
contract), orchestrator error → gRPC status mapping (`apierr.NotFound` → `codes.NotFound`,
a generic error → `codes.Internal`).

The end-to-end equip→snapshot proof (real AC against a hand-computed expected total,
two-handed occupancy visible across both surfaces, non-equipment-field preservation) is
an integration suite in the encounter package —
`internal/handlers/dnd5e/v2/encounter/integration_equipment_test.go` — since it needs
both this service and the v1alpha2 encounter service wired against the same character
store.
