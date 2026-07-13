---
name: entities
description: Domain data structures — plain Go structs, but with a known proto contamination problem
updated: 2026-07-13
confidence: high — verified by reading all remaining entity files
---

# entities

`internal/entities/` holds the domain data structures for rpg-api. Entities are the types that flow between layers: from handlers (after proto conversion) through orchestrators to repositories. In the ideal architecture, entities are plain Go structs with no external dependencies.

**Updated 2026-07-13 (rpg-api#642):** every proto-contaminated file this doc
used to describe — `encounter_events.go`, `entity_state.go`,
`encounter_state_builder.go` — plus the v1-only `dungeon.go`, `room.go`, and
`merged_grid.go` are deleted. They were the last consumers of `entities.Dungeon`,
`entities.CombatState`, `entities.EncounterEvent`, and the proto-construction
functions (`ToEntityStateProto`, `BuildEncounterStateData`, `CombatStateToProto`)
this doc previously flagged as the entities package's core boundary violation.
That violation no longer exists — the surviving files below are proto-free.

## Files

| File | Purpose | Proto contamination? |
|---|---|---|
| `character.go` | Thin wrapper — mostly uses toolkit types | No |
| `character_draft.go` | In-progress character creation state | No |
| `appearance.go` | Cosmetic character appearance | No |

`entities.Character` is the one type the surviving v1alpha2 encounter handler
imports from this package (for `hydrate_players.go` and its tests) — confirmed
via grep before the #642 deletion that no v2 file used any of the deleted
types.

## Clean entities

- `character.go`, `character_draft.go`, `appearance.go` — clean, unaffected by #642.

## Recommended path

None outstanding for this package — the proto-contamination items this doc
used to track were resolved by deletion rather than refactor. If the entities
package grows new proto-typed fields in the future, treat that as a fresh
regression, not a continuation of the pre-#642 debt.
