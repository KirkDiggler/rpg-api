---
name: Dungeon YAML v0.4 Wave A API boundary
description: Proto mapping, toolkit provider demand, and authored-source versus running-snapshot lifecycle
updated: 2026-08-09
confidence: checkpoint — API ports/mocks are green; rpg-toolkit#897 provider is not released
---

# Dungeon YAML v0.4 Wave A API boundary

RATIFIED authority is rpg-project PR #203. The consumer checkpoint is
rpg-dnd5e-web#735 commit `2ab0293`; the released generated Go contract is
`github.com/KirkDiggler/rpg-api-protos/gen/go`
`v0.0.0-20260809002602-f4d6396df528` (rpg-api-protos PR #218).

## Thin flow

```
PutDungeon request
  -> API envelope/key checks
  -> Compiler.CompileDungeon(complete source, draft|strict, PartyCap, opaque previous)
  -> exact FieldErrors OR complete Compiled + FloorPlan
  -> validate-only: return projection, mutate nothing
  -> strict write: durable source replacement, then authored registry replacement

StartEncounter
  -> read one strict compiled authored registry entry
  -> toolkit initializes encounter and returns its resolved world
  -> persist encounter.ToData() as the running snapshot

Encounter reload
  -> encounter.LoadFromData(persisted snapshot)
  -> never read the authored registry or current YAML
```

`internal/orchestrators/authoring.Compiler` is protobuf-free. `CompileDungeonInput`
expresses the lifecycle explicitly: `CompileModeDraft` permits structurally valid
empty/tiny/disconnected projections; `CompileModeStrict` is required for durable
write/registration and is also the authored state StartEncounter consumes.
`CompileDungeonOutput.FieldErrors` carries provider-authored `Field`, `Message`, and
`Code` verbatim. API never parses a message to invent a field path.

The API-owned `FloorPlan` demand is:

- resolved `FloorSourceBounds` or `FloorSourceRegions` on every success;
- complete provider-ordered `FloorCells` and `Regions`;
- flat `Edges` passed in provider order with both endpoints, kind, and optional door ID unchanged;
- `Entrance *FloorPlanCell`, where nil remains absent and present `[0,0]` is real data;
- opaque compiled authored state, used by StartEncounter but never interpreted by API.

The handler alone maps these domain values to the released proto enum/presence shape.
No protobuf type crosses the compiler seam.

## Provider dependency

The currently released toolkit (`encounter v0.50.1`) exposes separate
`LoadWithConfig`/`LoadWithPrevious` and `BuildFloorPlan` calls. It has no Wave A
`floor_source`, draft-validity mode, optional entrance result, region-union mask or
envelope result, typed validation paths, or strict-start helper. The temporary
`toolkitCompiler` adapter preserves the existing bounds-only behavior and rejects an
unknown regions field through toolkit decode; it never strips it or calculates a
bounds fallback.

rpg-toolkit#897 must publish a native provider satisfying the seam. Final API work then
replaces the adapter, pins that immutable module, and runs the real ring/tiny/islands
and void/mechanics acceptance. No union, adjacency, connectivity, PartyCap, envelope,
path, LoS, reveal, or fog logic belongs in this repository.

## Persistence ordering

API PR #781 (rpg-api#772) is still open and must land before this branch is updated
from `dev`. Wave A deliberately does not duplicate its per-key transaction lock or
durable same-directory replacement. After #781 lands, merge `origin/dev` into the
single #780 branch and keep its write-before-registry transaction unchanged.

The running-snapshot regression in `start_encounter_test.go` starts an encounter,
strictly replaces the shared authored registry, reloads the old encounter only from
persisted `encounter.Data`, and proves a later new encounter sees the update while the
old snapshot remains byte-equivalent.
