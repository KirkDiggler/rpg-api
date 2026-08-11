---
name: Dungeon YAML v0.4 Wave A API boundary
description: Proto mapping, toolkit provider integration, and authored-source versus running-snapshot lifecycle
updated: 2026-08-09
confidence: high — released encounter v0.53.0 provider plus current generated-proto transport verified by real-provider draft/strict, save/start/restart, and snapshot-isolation acceptance
---

# Dungeon YAML v0.4 Wave A API boundary

RATIFIED authority is rpg-project PR #203. The consumer contract is
rpg-dnd5e-web#735; this branch consumes the immutable generated Go contract
`github.com/KirkDiggler/rpg-api-protos/gen/go`
`v0.0.0-20260809161404-a6648cecf193`. The native provider is immutable
`github.com/KirkDiggler/rpg-toolkit/encounter v0.53.0`, which includes the
Wave A compiler released from rpg-toolkit PR #900 / issue #897 plus the
subsequent placement-offset carrier used by current `dev`.

## Thin flow

```
PutDungeon request
  -> API envelope/key checks
  -> one dungeonspec.CompileDungeon(complete source, Draft|Strict, PartyCap, seed)
  -> exact FieldErrors OR complete CompiledDungeon + FloorPlan
  -> validate-only: return projection, mutate nothing
  -> strict write: durable source replacement, then authored registry replacement

Startup
  -> LoadContentRegistry strictly compiles each authoritative YAML source
  -> invalid content remains discoverable as a disabled registry entry

StartEncounter
  -> read one strict compiled authored registry entry
  -> toolkit initializes encounter and returns its resolved world
  -> persist encounter.ToData() as the running snapshot

Encounter reload
  -> encounter.LoadFromData(persisted snapshot)
  -> never read the authored registry or current YAML
```

`internal/orchestrators/authoring.Compiler` is protobuf-free. `CompileDungeonInput`
expresses lifecycle explicitly: `CompileModeDraft` permits structurally valid
empty, tiny, or disconnected projections; `CompileModeStrict` is required for a
durable write/registry replacement. Every candidate compiles standalone from its
complete source. Prior compiled occupancy is never forwarded, so explicit deletion
or shrink is legal whenever the replacement candidate itself validates.

The production adapter calls native `dungeonspec.CompileDungeon` exactly once. It
maps only provider results:

- opaque `CompiledDungeon` for registry/start use;
- resolved `FloorSourceBounds` or `FloorSourceRegions`;
- provider-ordered floor cells, semantic regions, and flat physical edge pairs;
- optional entrance, preserving nil separately from real `[0,0]`; and
- ordered typed field errors with `field`, `message`, and `code` unchanged.

The handler alone maps these API-domain values to protobuf. Display name remains
API-local source metadata. API never unions cells, derives adjacency/envelopes,
checks connectivity or PartyCap, sorts provider results, parses provider messages,
or applies a bounds fallback.

## Persistence ordering

PR #781 / issue #772 is merged into `dev` and was merged once into the Wave A
branch without rebase. `PutDungeon` retains its reference-counted per-key lock
across complete-candidate compilation, durable same-directory temp/write/sync/
close/rename/directory-sync, and registry replacement. Compilation failure or any
pre-rename storage failure leaves source bytes and registry state unchanged.

Real-provider API acceptance covers the eight-cell ring and its interior/off-canvas
envelope, tiny and disconnected Draft-versus-Strict behavior, exact indexed source
paths, disconnected-island removal, bounds and room-chain regressions, strict
startup compilation, region source save/start/process restart, encounter-data
round-trip, and authored-edit isolation from an already-running snapshot.
