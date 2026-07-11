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
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	core "github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
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
// If the encounter is already TURN_BASED (e.g. a second goblin is being
// injected mid-fight), SetMode is skipped rather than erroring — the toolkit
// rejects a redundant SetMode call ("mode is already turn_based").
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

	goblinID := core.EntityID(fmt.Sprintf("goblin-%d", len(data.Monsters)+1))
	goblin := monster.NewGoblin(string(goblinID))
	goblinData := goblin.ToData()
	goblinDataJSON, err := json.Marshal(goblinData)
	if err != nil {
		return nil, fmt.Errorf("marshal goblin data: %w", err)
	}

	// StartEncounter spaces lobby members along Q=0..N-1, R=0
	// (spawnPositionSpacing, internal/orchestrators/lobby/start_encounter.go)
	// — one hex past the last member puts the goblin adjacent to the party
	// instead of stacked on a player.
	goblinQ := len(data.Players) + 1
	goblinPos := core.Hex{Q: goblinQ, R: 0, S: -goblinQ}
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

	alreadyTurnBased := data.Mode == core.ModeTurnBased
	if !alreadyTurnBased {
		if setErr := enc.SetMode(core.ModeTurnBased); setErr != nil {
			return nil, fmt.Errorf("set mode turn_based on encounter %q: %w", input.EncounterID, setErr)
		}
	}

	updated := enc.ToData()
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
