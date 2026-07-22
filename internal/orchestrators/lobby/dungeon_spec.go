package lobby

import (
	"errors"
	"fmt"

	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/tools/environments"
)

// DungeonKey selects a named dungeon specification passed to the
// toolkit's generic tkenc.Encounter.InitDungeon generator (rpg-api#688).
// rpg-api never computes dungeon geometry itself — a key maps to a
// literal, hand-authored tkenc.DungeonParams; only the toolkit turns that
// spec into an actual Space. Retires the fixed two-chamber constants
// StartEncounter used to hardcode directly (chamberWidth/chamberHeight/
// chamberPattern/chamberDoorID) — two chambers becomes just another spec
// this registry could hold, not a separate code path.
type DungeonKey string

// DungeonKeyCrypt is the first authored dungeon spec (rpg-toolkit#814's
// Approved Slice 3 corrections): one continuous 3-region linear chain —
// entrance -> corridor -> boss chamber — themed "crypt".
const DungeonKeyCrypt DungeonKey = "crypt"

// defaultDungeonKey is used when StartEncounterInput.DungeonKey is the
// zero value — "a named default for now" per rpg-api#688's Scope section
// ("Where the key comes from: lobby settings or a named default for now
// — the point is that rpg-api passes a key and receives geometry back").
// Lobby-settings-driven key selection is future work; no proto field
// carries a key today, so every real caller gets this default.
const defaultDungeonKey = DungeonKeyCrypt

// Crypt spec region identifiers — named constants (this repo's CLAUDE.md:
// "avoid magic strings") rather than inline literals. These are the IDs
// InitDungeon tags SpaceData.Regions with for THIS spec specifically —
// distinct from tkenc.RegionArchetype, the toolkit's fixed generic-role
// vocabulary each region also carries.
const (
	cryptRegionIDEntrance = "entrance"
	cryptRegionIDCorridor = "corridor"
	cryptRegionIDBoss     = "boss"
)

// Crypt spec connector door identifiers.
const (
	cryptDoorEntranceToCorridor core.EntityID = "crypt-door-entrance-corridor"
	cryptDoorCorridorToBoss     core.EntityID = "crypt-door-corridor-boss"
)

// Crypt spec dimensions — explicit, hand-authored, passed to InitDungeon
// verbatim (rpg-toolkit#814's Approved Slice 3 corrections: "the template
// shape and the scale invariant are supplied BY THE KEY"). Height is
// shared by every region in the chain (tkenc.DungeonParams.Height's own
// doc). cryptEntranceWidth matches the retired two-chamber generator's
// per-chamber size — proven safe: large enough relative to
// memberSightRange for out-of-sight goblin placement to find room, and
// for party members spread along spawnPositionSpacing to stay inside the
// region. cryptCorridorWidth is deliberately narrow — "a corridor is a
// long thin region for now" (rpg-toolkit#814's own Scope note) — and gets
// no goblins (see seedGoblins), so it never needs to satisfy an
// out-of-sight placement search itself. cryptBossWidth gives a primary
// playable axis (min(width, height)) of 14 — comfortably past the
// toolkit's own >6-hex-step scale invariant, enforced by
// validateDungeonParams at generation time, not eyeballed here.
const (
	cryptHeight        = 20
	cryptEntranceWidth = 20
	cryptCorridorWidth = 6
	cryptBossWidth     = 14
)

// dungeonSpec is a literal, hand-authored toolkit dungeon configuration
// keyed by DungeonKey. rpg-api's only job for a spec is to hold this
// config and hand it to tkenc.Encounter.InitDungeon verbatim — no region
// derivation, no sizing heuristics, no archetype- or theme-conditional
// branching (rpg-api#688's boundary note; rpg-toolkit#814's Approved
// Slice 3 corrections: "rpg-api still does no geometry math, no room
// sizing heuristics, and no theme-conditional branching").
type dungeonSpec struct {
	theme      string
	height     int
	regions    []tkenc.DungeonRegionParams
	connectors []tkenc.DungeonConnectorParams
}

// dungeonSpecs is the registry of named dungeon specs StartEncounter can
// select by DungeonKey. Exactly one entry today (crypt); adding a second
// named template is additive here — never a change to StartEncounter's
// call site or to seedGoblins' endpoint-seeding rule.
var dungeonSpecs = map[DungeonKey]dungeonSpec{
	DungeonKeyCrypt: {
		theme:  "crypt",
		height: cryptHeight,
		regions: []tkenc.DungeonRegionParams{
			{ID: cryptRegionIDEntrance, Archetype: tkenc.ArchetypeEntrance, Width: cryptEntranceWidth, Pattern: environments.PatternRandom},
			{ID: cryptRegionIDCorridor, Archetype: tkenc.ArchetypeCorridor, Width: cryptCorridorWidth, Pattern: environments.PatternRandom},
			{ID: cryptRegionIDBoss, Archetype: tkenc.ArchetypeBoss, Width: cryptBossWidth, Pattern: environments.PatternRandom},
		},
		connectors: []tkenc.DungeonConnectorParams{
			{DoorID: cryptDoorEntranceToCorridor},
			{DoorID: cryptDoorCorridorToBoss},
		},
	},
}

// ErrUnknownDungeonKey means a DungeonKey has no registered dungeonSpec.
// StartEncounter never receives a caller-supplied key via the current
// proto surface (no proto changes in this issue; lobby-settings-driven
// key selection is future work per the issue's Scope section), so this is
// reachable only via direct orchestrator calls (tests) until that wiring
// lands — unclassified sentinel errors fall through lobbyStatusError's
// default codes.Internal case, an honest mapping for a case no real
// client can trigger yet.
var ErrUnknownDungeonKey = errors.New("lobby orchestrator: unknown dungeon key")

// resolveDungeonSpec maps a DungeonKey to its literal dungeonSpec,
// falling back to defaultDungeonKey for the zero value ("a named default
// for now" — rpg-api#688's Scope section). Returns ErrUnknownDungeonKey
// for any other unregistered key — rpg-api surfaces the lookup failure;
// it never invents geometry for a key it doesn't recognize. Returns the
// EFFECTIVE key alongside the spec so callers building error messages
// after a successful resolution don't need to re-run the zero-value
// fallback themselves.
func resolveDungeonSpec(key DungeonKey) (DungeonKey, dungeonSpec, error) {
	if key == "" {
		key = defaultDungeonKey
	}
	spec, ok := dungeonSpecs[key]
	if !ok {
		return key, dungeonSpec{}, fmt.Errorf("%w: %q", ErrUnknownDungeonKey, key)
	}
	return key, spec, nil
}
