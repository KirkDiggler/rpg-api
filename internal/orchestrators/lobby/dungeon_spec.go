package lobby

import (
	"errors"
	"fmt"

	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
)

// DungeonKey selects a named dungeon specification passed to the
// toolkit's generic tkenc.Encounter.InitDungeon generator (rpg-api#688).
// rpg-api never computes dungeon geometry itself — a key maps to a
// tkenc.DungeonParams builder, and only the toolkit turns that spec into
// an actual Space. rpg-api#694 goes one step further for the crypt key:
// the builder doesn't even hold its OWN region widths/heights/connectors/
// obstacle specs — it calls the toolkit's tkenc.CryptDungeonParams
// constructor, which is the single source of the crypt's shape, theme,
// archetypes, and physical set-piece specs. rpg-api's only remaining job
// is key/config orchestration: which key maps to which toolkit
// constructor, and which caller-assigned door IDs that constructor's
// connectors get.
type DungeonKey string

// DungeonKeyCrypt is the first authored dungeon spec (rpg-toolkit#814's
// Approved Slice 3 corrections; rpg-toolkit#826 for its physical set
// pieces): one continuous 3-region linear chain — entrance -> corridor ->
// boss chamber — themed "crypt".
const DungeonKeyCrypt DungeonKey = "crypt"

// defaultDungeonKey is used when StartEncounterInput.DungeonKey is the
// zero value — "a named default for now" per rpg-api#688's Scope section
// ("Where the key comes from: lobby settings or a named default for now
// — the point is that rpg-api passes a key and receives geometry back").
// Lobby-settings-driven key selection is future work; no proto field
// carries a key today, so every real caller gets this default.
const defaultDungeonKey = DungeonKeyCrypt

// Crypt spec connector door identifiers — the two caller-assigned entity
// IDs threaded into tkenc.CryptDungeonParams' entranceDoorID/bossDoorID
// parameters below. This is the one piece of "shape" rpg-api still owns
// for the crypt key: naming its own connector doors, not sizing or
// placing anything inside the regions they join.
const (
	cryptDoorEntranceToCorridor core.EntityID = "crypt-door-entrance-corridor"
	cryptDoorCorridorToBoss     core.EntityID = "crypt-door-corridor-boss"
)

// dungeonSpecBuilder produces a toolkit tkenc.DungeonParams for a given
// seed. rpg-api#694: this replaces the retired literal dungeonSpec struct
// (theme/height/regions/connectors hand-authored per key) — a builder
// closure lets each key call whatever toolkit constructor owns its
// content (CryptDungeonParams for the crypt key today) instead of rpg-api
// duplicating that constructor's output as its own literal.
type dungeonSpecBuilder func(seed int64) tkenc.DungeonParams

// dungeonSpecs is the registry of named dungeon builders StartEncounter
// can select by DungeonKey. Exactly one entry today (crypt), forwarding
// verbatim to tkenc.CryptDungeonParams — rpg-api supplies only the seed
// (threaded through from StartEncounterInput.RandomSeed) and its own two
// connector door IDs; every region width/height/archetype/theme/obstacle
// spec is the toolkit's, not rpg-api's. Adding a second named template is
// additive here — never a change to StartEncounter's call site or to
// seedGoblins' endpoint-seeding rule.
var dungeonSpecs = map[DungeonKey]dungeonSpecBuilder{
	DungeonKeyCrypt: func(seed int64) tkenc.DungeonParams {
		return tkenc.CryptDungeonParams(seed, cryptDoorEntranceToCorridor, cryptDoorCorridorToBoss)
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

// resolveDungeonSpec maps a DungeonKey to its toolkit-constructed
// tkenc.DungeonParams for the given seed, falling back to
// defaultDungeonKey for the zero value ("a named default for now" —
// rpg-api#688's Scope section). Returns ErrUnknownDungeonKey for any
// other unregistered key — rpg-api surfaces the lookup failure; it never
// invents geometry for a key it doesn't recognize. Returns the EFFECTIVE
// key alongside the params so callers building error messages after a
// successful resolution don't need to re-run the zero-value fallback
// themselves.
func resolveDungeonSpec(key DungeonKey, seed int64) (DungeonKey, tkenc.DungeonParams, error) {
	if key == "" {
		key = defaultDungeonKey
	}
	build, ok := dungeonSpecs[key]
	if !ok {
		return key, tkenc.DungeonParams{}, fmt.Errorf("%w: %q", ErrUnknownDungeonKey, key)
	}
	return key, build(seed), nil
}
