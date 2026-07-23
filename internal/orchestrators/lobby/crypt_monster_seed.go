package lobby

// crypt_monster_seed.go implements rpg-api#689's approved first-swing
// simplification (2026-07-23 correction): exactly one deterministic-anchor
// skeleton minion in the entrance region, zero monsters in the corridor,
// and exactly one deterministic-anchor non-wight skeleton-captain boss
// (rpg-toolkit#816) in the boss region — replacing the retired seedGoblins'
// out-of-sight PositionOracle search entirely for this path.
//
// Anchor ownership: regionMonsterAnchor below is a PURE function over the
// same []tkenc.DungeonRegionParams/[]tkenc.DungeonConnectorParams/
// *tkenc.SpaceData/tkenc.DoorData shapes InitDungeon itself consumes and
// produces — not tied to this package's own dungeonSpec type. It reuses
// regionEntryAnchor (start_encounter.go, already tested, already
// production-proven for the terminal/boss region since rpg-api#676/#688)
// for BOTH the entrance and boss anchors, by resolving whichever connector
// door borders a given archetype's region: rpg-api never computes a
// center, offset, or direction itself — the toolkit's own required-path/
// primary-combat-axis invariants (rpg-toolkit#814/#819) are what make the
// door-neighbor cell safe, not anything reasoned about here. The exact
// same function also runs directly against tkenc.CryptDungeonParams's own
// output (crypt_dungeon_params_fixture_internal_test.go) — proving this
// generalizes to the toolkit's canonical constructor shape independent of
// whether dungeonSpec's crypt entry ever switches to calling it (that
// switch, plus Obstacles projection, is rpg-api#694/PR#697's job, not
// this one's).
//
// No silent fallback (this issue's original scope, still true): an
// unregistered dungeon key, a region whose archetype has no seed spec
// entry other than the corridor's deliberate absence, or a region archetype
// with zero or more than one match is an error — never a default monster,
// never a skipped region silently treated as "fine".

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	rpgcore "github.com/KirkDiggler/rpg-toolkit/core"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster/monsters"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
	"github.com/KirkDiggler/rpg-toolkit/tools/spawn"
)

// Crypt minion (entrance) combat-snapshot constants — the plain Skeleton's
// primary (shortsword melee) action, mirroring the retired goblin
// constants' pattern: MonsterData.AttackBonus/DamageDice/DamageType are a
// flat snapshot npc.go's scripted NPC path reads directly (see
// monsters.NewSkeleton's action for the source of these numbers), not a
// derived or invented value.
const (
	cryptMinionSpeed       = 6 // 30ft / 5ft per hex
	cryptMinionAttackBonus = 4
	cryptMinionDamageDice  = "1d6+2"
	cryptMinionDamageType  = "piercing"
)

// Crypt boss combat-snapshot constants — rpg-toolkit#816's non-wight
// skeleton-captain's primary (longsword) action.
const (
	cryptBossSpeed       = 6 // 30ft / 5ft per hex
	cryptBossAttackBonus = 5
	cryptBossDamageDice  = "1d8+3"
	cryptBossDamageType  = "slashing"
)

// entityGroupTypeMonster is defined in start_encounter.go and reused here.

// monsterSeedSpec pairs a region archetype with the single fixed monster
// this first crypt slice seeds into it, by toolkit factory/ref — rpg-api
// never picks stat blocks, it only selects WHICH toolkit factory to call
// and WHERE (a deterministic anchor), per the dungeon key + region
// archetype.
type monsterSeedSpec struct {
	archetype   tkenc.RegionArchetype
	newMonster  func(id string) *monster.Monster
	speed       int
	attackBonus int
	damageDice  string
	damageType  string
}

// cryptMonsterSeedSpecs is the archetype table for DungeonKeyCrypt — the
// first crypt learning slice's exact composition (rpg-api#689's 2026-07-23
// correction): one entrance minion, one boss, and — by deliberate omission,
// not a zero-count entry — no spec for ArchetypeCorridor at all.
//
// ORDER IS LOAD-BEARING (rpg-api#694/#689 merge finding, 2026-07-23): the
// boss spec MUST come before the entrance spec here. seedRegionMonsters
// below calls enc.AddMonster once per spec, in this slice's order.
// rpg-toolkit's AddMonster has a documented "reinforcement" behavior
// (encounter/encounter.go: "A monster added while combat is already
// running joins the initiative order — appended to the end") that fires
// unconditionally, with NO visibility check, for any monster added AFTER
// the encounter has already flipped to TURN_BASED. Under #694's compact
// crypt dimensions the entrance monster's deterministic anchor (near its
// own outgoing door, by design — see regionMonsterAnchor's doc) is
// frequently within a fresh party's SightRange, so entrance's own
// AddMonster call alone can trigger TURN_BASED before the boss is even
// added. If entrance were added FIRST here, the boss's later AddMonster
// call would hit that reinforcement path and join initiative completely
// unconditionally — provably wrong (verified directly via
// perception.CanSeeAt on the final persisted state: the boss is NOT
// visible) but toolkit-correct BY DESIGN for genuine mid-combat
// reinforcements, which this initial-seeding call is not. Adding the boss
// FIRST (while the encounter is still guaranteed FREE_ROAM — nothing has
// been added yet to see) means the boss is already present in
// e.data.Monsters by the time entrance's own AddMonster call triggers the
// FIRST real combat-entry roll; that roll's rollInitiative->
// engagedMonsters() scan is a genuine, current perception.CanSeeAt check
// against every already-added monster, correctly excluding the
// still-unseen boss. No toolkit change, no new search/retry logic — pure
// call-order sequencing, entirely within rpg-api's own orchestration.
func cryptMonsterSeedSpecs() []monsterSeedSpec {
	return []monsterSeedSpec{
		{
			archetype: tkenc.ArchetypeBoss, newMonster: monsters.NewSkeletonCaptain,
			speed: cryptBossSpeed, attackBonus: cryptBossAttackBonus,
			damageDice: cryptBossDamageDice, damageType: cryptBossDamageType,
		},
		{
			archetype: tkenc.ArchetypeEntrance, newMonster: monsters.NewSkeleton,
			speed: cryptMinionSpeed, attackBonus: cryptMinionAttackBonus,
			damageDice: cryptMinionDamageDice, damageType: cryptMinionDamageType,
		},
	}
}

// monsterSeedSpecsByKey is the dungeon-key-scoped registry of archetype
// tables. Exactly one entry today (crypt); adding a second named dungeon
// key's own table is additive here — never a change to seedRegionMonsters'
// call site. No entry for an unknown key is deliberate: resolved by
// buildMonsterSeedGroups returning an error, not a default table.
var monsterSeedSpecsByKey = map[DungeonKey]func() []monsterSeedSpec{
	DungeonKeyCrypt: cryptMonsterSeedSpecs,
}

// regionAnchorDoor picks whichever connector touches the region at index i
// in a linear N-region chain: the incoming door (connectors[i-1]) when one
// exists, else the outgoing door (connectors[i]) — i.e. the entrance
// region (i==0, no incoming door) resolves to ITS OWN outgoing connector,
// and the boss/terminal region (no outgoing door) resolves to its incoming
// connector. Every region in a valid (>=2 region) chain has at least one
// side, so this only errors on a malformed chain validateDungeonParams
// would already have rejected.
func regionAnchorDoor(i int, connectors []tkenc.DungeonConnectorParams) (core.EntityID, error) {
	if i > 0 {
		return connectors[i-1].DoorID, nil
	}
	if i < len(connectors) {
		return connectors[i].DoorID, nil
	}
	return "", fmt.Errorf("lobby orchestrator: region %d has no adjacent connector door", i)
}

// regionMonsterAnchor resolves the single deterministic anchor hex for the
// ONE region matching archetype in regions — via regionEntryAnchor against
// whichever connector door borders that region (regionAnchorDoor). Errors
// (no silent fallback) when zero or more than one region matches archetype,
// when the resolved door isn't present in doors, or when regionEntryAnchor
// itself fails (including its room.CanPlaceEntity hardening — see that
// function's doc for the seed=66-against-real-CryptDungeonParams evidence
// motivating it). room may be nil (no walkability hardening, pure region-
// tag matching only) for callers/tests with no real room.
//
// Pure over toolkit types (regions/connectors/doors/space/room), not tied
// to this package's dungeonSpec — see crypt_dungeon_params_fixture_internal_
// test.go for this same function exercised directly against
// tkenc.CryptDungeonParams's own output.
func regionMonsterAnchor(
	space *tkenc.SpaceData,
	regions []tkenc.DungeonRegionParams,
	connectors []tkenc.DungeonConnectorParams,
	doors map[core.EntityID]*tkenc.DoorData,
	room spatial.Room,
	archetype tkenc.RegionArchetype,
) (core.Hex, string, error) {
	idx := -1
	for i, r := range regions {
		if r.Archetype != archetype {
			continue
		}
		if idx != -1 {
			return core.Hex{}, "", fmt.Errorf(
				"lobby orchestrator: archetype %q matches more than one region (%q and %q)",
				archetype, regions[idx].ID, r.ID)
		}
		idx = i
	}
	if idx == -1 {
		return core.Hex{}, "", fmt.Errorf("lobby orchestrator: no region with archetype %q", archetype)
	}

	doorID, err := regionAnchorDoor(idx, connectors)
	if err != nil {
		return core.Hex{}, "", err
	}
	door, ok := doors[doorID]
	if !ok {
		return core.Hex{}, "", fmt.Errorf("lobby orchestrator: connector door %q not found (InitDungeon must add it)", doorID)
	}

	anchor, err := regionEntryAnchor(space, door, regions[idx].ID, room)
	if err != nil {
		return core.Hex{}, "", err
	}
	return anchor, regions[idx].ID, nil
}

// monsterSeedPlan carries one built monster alongside the spec that
// produced it, correlated back to its spawned position after
// spawn.BasicSpawnEngine.PopulateRoom runs.
type monsterSeedPlan struct {
	id      core.EntityID
	monster *monster.Monster
	spec    monsterSeedSpec
}

// buildMonsterSeedGroups resolves every cryptMonsterSeedSpecs entry (by
// dungeonKey) to a deterministic anchor (regionMonsterAnchor) and a
// spawn.EntityGroup carrying that ONE anchor as FixedPositions with
// Quantity.Fixed matching len(FixedPositions) exactly — so the spawn
// engine's search path (PositionOracle, scattered candidate sampling) is
// never reached for these groups; FixedPositions is still checked against
// the real room (spawn's own Room.CanPlaceEntity contract) before use, so
// an invalid anchor is a SpawnFailure, never a silent bad placement.
//
// params is the resolved tkenc.DungeonParams for this encounter (rpg-api#694:
// resolveDungeonSpec's own toolkit-constructed output — CryptDungeonParams'
// Regions/Connectors for the crypt key, not an API-owned literal) — this
// function only ever reads params.Regions/params.Connectors, so it is
// generic over whichever key produced them, exactly like
// crypt_dungeon_params_fixture_internal_test.go proved directly against
// tkenc.CryptDungeonParams's own output before this switch landed.
//
// Returns an error (no silent fallback) for an unregistered dungeonKey or
// any archetype whose anchor can't be resolved.
func buildMonsterSeedGroups(
	space *tkenc.SpaceData,
	doors map[core.EntityID]*tkenc.DoorData,
	room spatial.Room,
	params tkenc.DungeonParams,
	dungeonKey DungeonKey,
) (*spawn.BasicSelectablesRegistry, []monsterSeedPlan, []spawn.EntityGroup, error) {
	specsFn, ok := monsterSeedSpecsByKey[dungeonKey]
	if !ok {
		return nil, nil, nil, fmt.Errorf("lobby orchestrator: no monster seed table for dungeon key %q", dungeonKey)
	}
	specs := specsFn()

	one := 1
	plans := make([]monsterSeedPlan, 0, len(specs))
	groups := make([]spawn.EntityGroup, 0, len(specs))
	registry := spawn.NewBasicSelectablesRegistry()

	for _, ms := range specs {
		anchor, regionID, err := regionMonsterAnchor(space, params.Regions, params.Connectors, doors, room, ms.archetype)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("resolve anchor for archetype %q: %w", ms.archetype, err)
		}

		id := core.EntityID(fmt.Sprintf("%s-%s", regionID, ms.archetype))
		m := ms.newMonster(string(id))
		tableID := string(id)
		if regErr := registry.RegisterTable(tableID, []rpgcore.Entity{m}); regErr != nil {
			return nil, nil, nil, fmt.Errorf("register monster selection table %q: %w", tableID, regErr)
		}

		groups = append(groups, spawn.EntityGroup{
			ID:             tableID,
			Type:           entityGroupTypeMonster,
			SelectionTable: tableID,
			Quantity:       spawn.QuantitySpec{Fixed: &one},
			FixedPositions: []spatial.Position{anchor.ToPosition()},
		})
		plans = append(plans, monsterSeedPlan{id: id, monster: m, spec: ms})
	}

	return registry, plans, groups, nil
}

// seedRegionMonsters seeds the crypt's first-swing monster composition
// (rpg-api#689): one deterministic-anchor minion in the entrance region,
// zero in the corridor, one deterministic-anchor boss in the boss region —
// via spawn.FixedPositions only, never a PositionOracle search. Retires
// seedGoblins for this call site entirely; regionEntryAnchor (still used by
// regionMonsterAnchor above) is the only piece of the old goblin path this
// keeps. params is StartEncounter's already-resolved tkenc.DungeonParams
// (rpg-api#694: resolveDungeonSpec's toolkit-constructed output) — the
// SAME value InitDungeon was just called with, never re-derived here.
func (o *Orchestrator) seedRegionMonsters(
	ctx context.Context,
	enc *tkenc.Encounter,
	encID core.EncounterID,
	params tkenc.DungeonParams,
	dungeonKey DungeonKey,
) error {
	room := enc.Room()
	if room == nil {
		return errors.New("lobby orchestrator: encounter has no room to place monsters in (InitDungeon not called)")
	}
	space := enc.ToData().Space
	if space == nil {
		return errors.New("lobby orchestrator: encounter has no space snapshot to seed monsters into")
	}
	doors := enc.ToData().Doors

	registry, plans, groups, err := buildMonsterSeedGroups(space, doors, room, params, dungeonKey)
	if err != nil {
		return err
	}

	engine := spawn.NewBasicSpawnEngine(spawn.BasicSpawnEngineConfig{
		ID:               "startencounter-crypt-monsters",
		SelectablesReg:   registry,
		RoomOrchestrator: enc.RoomOrchestrator(),
	})

	result, err := engine.PopulateRoom(ctx, string(encID), spawn.SpawnConfig{
		EntityGroups: groups,
		Pattern:      spawn.PatternScattered,
	})
	if err != nil {
		return fmt.Errorf("spawn engine populate room: %w", err)
	}
	if len(result.Failures) > 0 {
		return fmt.Errorf("lobby orchestrator: spawn engine failed to place %d of %d monster(s): %+v",
			len(result.Failures), len(groups), result.Failures)
	}

	positions := make(map[core.EntityID]core.Hex, len(result.SpawnedEntities))
	for _, spawned := range result.SpawnedEntities {
		positions[core.EntityID(spawned.Entity.GetID())] = core.HexFromPosition(spawned.Position)
	}

	for _, p := range plans {
		pos, posOK := positions[p.id]
		if !posOK {
			return fmt.Errorf("lobby orchestrator: spawn engine reported no position for monster %q", p.id)
		}
		data := p.monster.ToData()
		dataJSON, marshalErr := json.Marshal(data)
		if marshalErr != nil {
			return fmt.Errorf("marshal monster %q data: %w", p.id, marshalErr)
		}
		if addErr := enc.AddMonster(tkenc.MonsterInput{
			ID:          p.id,
			Position:    pos,
			HP:          data.HitPoints,
			MaxHP:       data.MaxHitPoints,
			AC:          data.ArmorClass,
			Speed:       p.spec.speed,
			MonsterRef:  data.Ref.String(),
			AttackBonus: p.spec.attackBonus,
			DamageDice:  p.spec.damageDice,
			DamageType:  p.spec.damageType,
			DataJSON:    dataJSON,
		}); addErr != nil {
			return fmt.Errorf("add monster %q to encounter: %w", p.id, addErr)
		}
	}
	return nil
}
