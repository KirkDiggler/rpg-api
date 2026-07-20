// Package devcombat implements the dev-tooling half of rpg-api#634 ("a real
// fight on GameView"): injecting a goblin into an EXISTING encounter and
// flipping it to TURN_BASED, without rebuilding the encounter or touching its
// existing players. cmd/devseed's --inject-combat flag and the integration
// test suite both call Inject so they exercise the identical code path —
// package main (cmd/devseed) cannot be imported by other packages, so this
// logic lives here rather than in cmd/devseed/main.go.
//
// This fills a gap production StartEncounter does not (and should not) fill
// itself: there is no room/encounter-design system yet to decide when and
// what monsters spawn into a lobby-started encounter (the lobby contract
// deliberately dropped an initial_mode field — rpg-project#81 decision log).
// The real combat-entry trigger is future room/encounter-design work; this
// package exists purely so local/MCP playtesting can exercise a fight against
// a lobby-started encounter today.
package devcombat

import (
	"context"
	"encoding/json"
	"fmt"

	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	rpgcore "github.com/KirkDiggler/rpg-toolkit/core"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	core "github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/encounter/perception"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

// monsterRefGoblin identifies the injected monster's type, matching
// cmd/devseed's existing fixture convention.
const monsterRefGoblin = "dnd5e:monsters:goblin"

// goblinDamageDice is the injected goblin's stat-snapshot damage-dice
// fallback (used by StandInCombatResolver if the goblin's own attacks ever
// run without a held character), matching every other devseed fixture's
// goblin.
const goblinDamageDice = "1d6+2"

// goblinAttackBonus matches goblin's NewScimitarAction, same as every other
// devseed fixture.
const goblinAttackBonus = 4

// goblinSpeed is 30ft / 5ft-per-hex.
const goblinSpeed = 6

// goblinDamageType is the injected goblin's stat-snapshot damage type.
const goblinDamageType = "slashing"

// InjectInput carries the parameters for Inject.
type InjectInput struct {
	// EncounterID identifies the EXISTING encounter to load. Inject returns
	// encountersv2.ErrNotFound if no encounter is persisted under this ID.
	EncounterID string

	// ForceNPCFirst deterministically reorders the freshly-rolled Initiative so
	// the just-injected goblin leads it (ActiveIdx=0), instead of leaving the
	// toolkit's real initiative roll to decide who goes first. This exists to
	// reproduce rpg-api#636 ("NPC first in initiative stalls the encounter") on
	// demand — the bug was originally found by a lucky/unlucky roll, not
	// something every --inject-combat run hits, so playtest repro and the
	// regression test both need a deterministic knob rather than looping
	// --inject-combat until the dice cooperate. Dev-tool-only: production
	// combat-entry paths must never set this — a real encounter's first actor
	// should stay whatever the toolkit's own roll decides.
	ForceNPCFirst bool
}

// InjectOutput reports what Inject did, for callers that want to print or
// assert on it.
type InjectOutput struct {
	GoblinID            core.EntityID
	Position            core.Hex
	Mode                core.EncounterMode
	AlreadySetTurnBased bool
	PlayerCount         int
	MonsterCount        int
}

// Inject loads the EXISTING encounter at input.EncounterID, adds a goblin,
// and flips the encounter to TURN_BASED — WITHOUT rebuilding the encounter or
// touching its existing players. This is the honest complement to rpg-api
// #634 Part 1 (lobby StartEncounter seeds real players with an honest
// HP/AC/no-fake-attack-stats snapshot but never spawns monsters): a lobby-
// started encounter has no monster and stays FreeRoam until something adds
// one, and this is that something for dev/playtest purposes.
//
// rpg-toolkit#796 ("combat pockets") scoped rollInitiative to
// engagedMonsters() — monsters at least one player currently has LoS to —
// rather than every monster anywhere in the space. Inject now places the
// goblin at a position it VERIFIES is in-sight (findVisiblePosition, below)
// instead of guessing "one hex past the party's leading edge" and hoping.
// This is what makes AddMonster's own inline combat-entry check
// (checkCombatEntry, rpg-toolkit#759) reliably fire and roll the goblin into
// Initiative by the SAME rule real play uses — Inject no longer needs (and
// no longer has) a forced-SetMode escape hatch for "goblin landed outside
// sight": that state is structurally unreachable now, not just avoided.
//
// If the encounter is already TURN_BASED (e.g. a second goblin is being
// injected mid-fight), the toolkit rejects a redundant SetMode call ("mode is
// already turn_based") — and AddMonster alone never touches Initiative (only
// SetMode's rollInitiative does) once the mode isn't changing, so a monster
// added while already turn-based would otherwise sit in the encounter
// forever without ever getting a turn. Inject instead round-trips FREE_ROAM
// -> TURN_BASED to force a fresh roll that includes the new monster,
// accepting the reset to Round=1/ActiveIdx=0 as the honest cost of adding a
// combatant mid-fight in a dev tool (not a live production interrupt). This
// case is unrelated to the sight-guarantee above (it's about a mode
// transition not firing, not about placement) and is unchanged by it.
//
// Known acceptable gap: a monster added here does NOT go through
// monstertraits.LoadMonsterConditions / the OA reaction-readiness wiring that
// rpg-toolkit encounter's hydrateMonster applies during the LoadFromData
// cascade (npc.go) — AddMonster only seeds the flat MonsterData snapshot +
// DataJSON blob on this call's short-lived encounter, not a live held
// *monster.Monster with conditions subscribed. The injected goblin still
// attacks/defends via its AttackBonus/DamageDice/DamageType snapshot (set
// below) and gets fully rehydrated — including OA wiring — on the NEXT
// combat-capable RPC's own LoadFromData (the v2 encounter orchestrator's
// characterData cascade attaches fresh DataJSON for every seat on every
// combat-capable request). Only the opportunity-attack reaction for THIS
// specific goblin is missing until then — cosmetic for a first fight.
func Inject(ctx context.Context, repo encountersv2.Repository, input InjectInput) (*InjectOutput, error) {
	data, err := repo.Get(ctx, input.EncounterID)
	if err != nil {
		return nil, fmt.Errorf("load encounter %q: %w", input.EncounterID, err)
	}

	// devcombat never publishes events; the broker only exists because
	// LoadFromData/AddMonster/SetMode require one. The persisted Data is what
	// matters — the next real load builds its own broker.
	transport := tkenc.NewInMemoryTransport()
	defer func() { _ = transport.Close() }()
	broker := tkenc.NewBroker(transport)
	defer func() { _ = broker.Close() }()

	enc, err := tkenc.LoadFromData(ctx, data, broker)
	if err != nil {
		return nil, fmt.Errorf("load encounter %q from data: %w", input.EncounterID, err)
	}

	// Captured BEFORE AddMonster: LoadFromData aliases the *Data pointer
	// (e.data = data, encounter.go), so AddMonster's inline combat-entry
	// check (rpg-toolkit#759) can flip data.Mode to TURN_BASED in place —
	// e.g. the goblin lands within an existing player's sight range. Reading
	// data.Mode AFTER AddMonster would then report "already turn-based" even
	// when this call is the one that started the fight.
	alreadyTurnBased := data.Mode == core.ModeTurnBased

	goblinID := core.EntityID(fmt.Sprintf("goblin-%d", len(data.Monsters)+1))
	goblin := monster.NewGoblin(string(goblinID))
	goblinData := goblin.ToData()
	goblinDataJSON, err := json.Marshal(goblinData)
	if err != nil {
		return nil, fmt.Errorf("marshal goblin data: %w", err)
	}

	room := enc.Room()
	if room == nil {
		return nil, fmt.Errorf("inject: encounter %q has no room (InitRoom not called) — cannot verify LoS", input.EncounterID)
	}
	goblinPos, found := findVisiblePosition(data, room, goblin)
	if !found {
		return nil, fmt.Errorf(
			"inject: no unoccupied, in-sight position found in encounter %q's room — every player is boxed off from the rest of it",
			input.EncounterID)
	}
	if addErr := enc.AddMonster(tkenc.MonsterInput{
		ID:          goblinID,
		Position:    goblinPos,
		HP:          goblinData.HitPoints,
		MaxHP:       goblinData.MaxHitPoints,
		AC:          goblinData.ArmorClass,
		Speed:       goblinSpeed,
		MonsterRef:  monsterRefGoblin,
		AttackBonus: goblinAttackBonus,
		DamageDice:  goblinDamageDice,
		DamageType:  goblinDamageType,
		DataJSON:    goblinDataJSON,
	}); addErr != nil {
		return nil, fmt.Errorf("add goblin to encounter %q: %w", input.EncounterID, addErr)
	}

	if alreadyTurnBased {
		// Mid-fight injection: round-trip to force rollInitiative to include
		// the just-added monster — see the doc comment above.
		if setErr := enc.SetMode(core.ModeFreeRoam); setErr != nil {
			return nil, fmt.Errorf("reset encounter %q to free_roam before re-rolling initiative: %w", input.EncounterID, setErr)
		}
		if setErr := enc.SetMode(core.ModeTurnBased); setErr != nil {
			return nil, fmt.Errorf("set mode turn_based on encounter %q: %w", input.EncounterID, setErr)
		}
	}
	// else: AddMonster's inline combat-entry check (rpg-toolkit#759) already
	// flipped FREE_ROAM -> TURN_BASED and rolled initiative INCLUDING this
	// goblin (checkCombatEntry runs after the monster is added to
	// e.data.Monsters) — guaranteed, since findVisiblePosition only ever
	// returns a position at least one player has LoS to. Nothing left to do.

	updated := enc.ToData()
	if input.ForceNPCFirst {
		forceEntityFirst(updated, goblinID)
	}
	if saveErr := repo.Save(ctx, updated); saveErr != nil {
		return nil, fmt.Errorf("save encounter %q: %w", input.EncounterID, saveErr)
	}

	return &InjectOutput{
		GoblinID:            goblinID,
		Position:            goblinPos,
		Mode:                updated.Mode,
		AlreadySetTurnBased: alreadyTurnBased,
		PlayerCount:         len(updated.Players),
		MonsterCount:        len(updated.Monsters),
	}, nil
}

// forceEntityFirst reorders data.Initiative so id leads it (index 0) and sets
// ActiveIdx to 0, preserving the relative order of every other entity. Used
// only by ForceNPCFirst — see its doc comment for why a deterministic reorder
// beats reseeding the toolkit's own initiative roll.
func forceEntityFirst(data *tkenc.Data, id core.EntityID) {
	reordered := make([]core.EntityID, 0, len(data.Initiative))
	reordered = append(reordered, id)
	for _, existing := range data.Initiative {
		if existing != id {
			reordered = append(reordered, existing)
		}
	}
	data.Initiative = reordered
	data.ActiveIdx = 0
}

// findVisiblePosition walks every cell in the room, in stable row-major
// order, and returns the first unoccupied cell at least one player currently
// has LoS to — the same wall-aware perception.CanSeeAt predicate
// checkCombatEntry itself uses to decide whether a newly added monster
// starts a fight, not a hand-rolled distance check. Mirrors seedGoblins'
// validGoblinPosition (start_encounter.go), inverted: that predicate rejects
// any in-sight cell (goblins must NOT start visible, so combat doesn't begin
// at spawn); this one REQUIRES it (Inject exists specifically to force a
// visible fight on demand for dev/playtest purposes).
//
// Occupancy is checked directly against data.Players/data.Monsters rather
// than room.GetEntitiesInRange: that query only reflects entities placed
// into the room's OWN spatial occupancy grid (e.g. via the spawn engine's
// PlaceEntity, as seedGoblins relies on for its already-seeded siblings) —
// AddPlayer does not register a player there, it only sets
// PlayerData.View.Position, so relying on GetEntitiesInRange here silently
// let a candidate land exactly on a player's hex (caught by
// TestInject_AddsGoblinAndFlipsToTurnBased's stacking assertion).
//
// entity is passed to room.CanPlaceEntity to reject candidates the room
// itself would refuse — walls and other blocked terrain (Copilot review,
// PR #675): CanSeeAt only proves a cell is perceivable, not that it's
// walkable/placeable ground; a wall hex is legitimately "visible" (you can
// see a wall) but is not somewhere a monster can stand. The direct
// data.Players/data.Monsters occupancy check above stays alongside this —
// CanPlaceEntity's own occupancy half only sees entities the room's spatial
// grid actually tracks (spawn-placed monsters), not players, exactly the
// gap the comment above already documents.
//
// The room is small (StartEncounter's fixed 20x20) and rarely occupied by
// more than a handful of entities, so a full sweep is cheap and simple —
// deliberately not a spiral-out-from-a-player search, which would need extra
// bookkeeping to stay deterministic across multiple player anchors for no
// real benefit at this scale.
//
// Returns ok=false if the room has no unoccupied, placeable, in-sight cell
// at all (every player boxed off from the rest of it) — Inject surfaces
// that as an error rather than silently falling back to a blind,
// possibly-invisible placement, which is exactly the bug this replaces.
func findVisiblePosition(data *tkenc.Data, room spatial.Room, entity rpgcore.Entity) (core.Hex, bool) {
	if data.Space == nil {
		return core.Hex{}, false
	}
	occupied := make(map[core.Hex]struct{}, len(data.Players)+len(data.Monsters))
	for _, pd := range data.Players {
		if pd.View != nil {
			occupied[pd.View.Position] = struct{}{}
		}
	}
	for _, md := range data.Monsters {
		occupied[md.Position] = struct{}{}
	}

	for y := 0; y < data.Space.Height; y++ {
		for x := 0; x < data.Space.Width; x++ {
			pos := spatial.Position{X: float64(x), Y: float64(y)}
			hex := core.HexFromPosition(pos)
			if _, taken := occupied[hex]; taken {
				continue
			}
			if !room.CanPlaceEntity(entity, pos) {
				continue
			}
			for _, pd := range data.Players {
				if pd.View != nil && perception.CanSeeAt(pd.View, hex, room) {
					return hex, true
				}
			}
		}
	}
	return core.Hex{}, false
}
