---
name: dungeon component
description: Procedural dungeon generation — room shapes, wall perimeters, monster placement, hex-grid layouts, theme tables
updated: 2026-05-02
confidence: high — verified by reading types.go, generator.go, and toolkit/ subdirectory
---

# dungeon component

`internal/components/dungeon/` is a self-contained procedural dungeon generator. It creates multi-room dungeons with themed encounters, hex-grid room shapes, perimeter wall generation, door spawning, and CR-scaled monster placements. It is well-structured and well-tested.

**This component is in the wrong repository.** Procedural dungeon generation — including room shapes, wall placement, monster CR budgets, hex-grid coordinate calculation, and theme tables — is game logic. The Boundary Rule (rpg-api CLAUDE.md, rpg-project CLAUDE.md) is explicit: game mechanics belong in rpg-toolkit. Moving this component is a known planned task that has not been executed.

## Structure

```
components/dungeon/
    types.go             — all type definitions (Room, Shape, Position, WallSegment, etc.)
    generator.go         — Generator interface + LayoutInput/Output types
    monster_type.go      — monster type registry
    monster_factory.go   — creates monster.Data from placements
    theme.go             — Theme type
    theme_tables.go      — monster pools and CR tables per theme
    direction_adapter.go — dungeon.Direction ↔ spatial direction conversion

    toolkit/             — wraps rpg-toolkit spatial types into dungeon types
        coords.go        — hex coordinate calculations
        factory.go       — creates spatial.Room from dungeon.Room
        feature.go       — feature layout conversion
        layout.go        — room layout generation (toolkit environments integration)
        patterns.go      — wall pattern generation (sparse, cover clusters, chokepoints, etc.)
        perimeter.go     — room perimeter wall calculation
        shape.go         — room shape generation (hex shapes: rectangle, L, T, etc.)
        shape_selector.go — selects shape based on RoomType and Size
        validation.go    — validates entity placement against room bounds

    encounter/           — encounter content for rooms
        budget.go        — XP budget calculation for encounter difficulty
        generator.go     — generates monster placements for a room
        selector.go      — selects monsters from theme tables by CR

    adapter/             — adapts dungeon types for encounter orchestrator
        encounter.go     — converts dungeon rooms to encounter entity data
        stubs.go         — stub implementations for testing

    presentation/        — display helpers
        hints.go         — connection direction hints ("north door", "stairs down")
        shuffler.go      — randomizes presentation order
```

## Public API

**Generator** (`generator.go`):
```go
type Generator struct {
    // wraps toolkit layout and shape generators
}

func (g *Generator) Generate(ctx context.Context, input *GenerateInput) (*GenerateOutput, error)

type GenerateInput struct {
    Theme    Theme
    Size     RoomSize
    Length   int         // number of rooms
    Layout   LayoutType  // linear, branching, hub
    PartySize int
    TargetCR  int
    Seed     int64
}

type GenerateOutput struct {
    Dungeon *Dungeon
    Seed    int64
}
```

**Room** (from `types.go`):
```go
type Room struct {
    ID       string
    Shape    *Shape          // bounds, connection points, grid type, dimensions
    Features *FeatureLayout  // obstacles, terrain patches
    SpawnZones []*Zone       // player spawn, monster spawn, boss areas
    Walls    []WallSegment   // all walls (perimeter + internal patterns)
    Encounter *Encounter     // monster placements with CR
    Origin   Position        // absolute position in dungeon-space (integer cube coords)
}
```

**Dungeon** (from `types.go`):
```go
type Dungeon struct {
    ID          string
    Theme       Theme
    Rooms       []*Room
    Connections []*RoomConnection
    StartRoom   string
    BossRoom    string
}
```

Note: this is the component's `Dungeon` type, not `entities.Dungeon`. The entity layer wraps this in `entities.Dungeon` with exploration state and a connection graph from toolkit's `environments.ConnectionEdge`.

## Position types

The dungeon component owns the canonical coordinate types (`coords.go`):

- `LocalPosition` — integer cube coordinate inside a single room
- `AbsolutePosition` — integer cube coordinate in dungeon-absolute space
- `Module` (`module.go`) — holds the per-room origin map and bridges local to absolute via `LocalToAbsolute(roomID, LocalPosition) AbsolutePosition`

Both position types enforce the cube invariant `X+Y+Z == 0` in their constructors (`NewLocalPosition`, `NewAbsolutePosition`). The local-vs-absolute distinction is enforced at the type level — the compiler rejects code that mixes them. The previous `dungeon.Position` and `entities.Position` (float64) types were removed in #471.

Proto positions (`apiv1alpha1.Position`) are `int32`; the converter pipeline does a 3-field cast at the proto boundary.

## Integrations

**Depends on:**
- `rpg-toolkit/tools/spatial` — room grid, entity placement, movement validation
- `rpg-toolkit/tools/environments` — connection graph (rooms + connections as graph)
- `rpg-toolkit/rulebooks/dnd5e/monster` — `monster.Data` type for placement
- `rpg-toolkit/rulebooks/dnd5e/combat` — XP budget thresholds per encounter difficulty

**Used by:**
- `orchestrators/encounter` — calls `Generator.Generate`, uses rooms and connections
- `entities/dungeon.go` — `Dungeon.Rooms` is `map[string]*dungeon.Room`
- `entities/dungeon.go` — `Dungeon.RoomOrigins` is `map[string]dungeon.AbsolutePosition`
- `entities/encounter_events.go` — `CombatStartedEvent.Walls []dungeon.WallSegment`
- `handlers/dnd5e/v1alpha1/encounter/converters.go` — converts dungeon types to proto

## Known issues

### Wrong repository (primary issue)

Procedural dungeon generation is game logic. The theme tables (`theme_tables.go`) hardcode monster pools for "crypt", "cave", and "bandit-lair" themes. Encounter budgeting (`encounter/budget.go`) implements the D&D 5e XP threshold system. Room shape selection (`toolkit/shape_selector.go`) implements D&D 5e dungeon design rules. All of this belongs in rpg-toolkit.

The planned migration path: extract to `rpg-toolkit/tools/dungeon` or `rpg-toolkit/rulebooks/dnd5e/dungeon`. The component's internal `toolkit/` subdirectory already wraps rpg-toolkit types, which makes extraction more tractable.

### Coordinate refactor — landed

#471 unified the coordinate model: `LocalPosition` / `AbsolutePosition` / `Module` replaced the old `dungeon.Position` (int) and `entities.Position` (float64). All transform sites now go through `Module.LocalToAbsolute`. The remaining work is the component's broader extraction to rpg-toolkit (see "Wrong repository" above), which is independent of coordinate types.

### No top-level documentation in README

The component has no package-level README or doc.go explaining its role and planned migration status. Someone new to the codebase will not know this is a prototype pending extraction.
