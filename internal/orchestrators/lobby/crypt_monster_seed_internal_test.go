package lobby

// White-box unit tests for the archetype-driven crypt monster seeding
// introduced by rpg-api#689's approved first-swing simplification
// (2026-07-23 correction): exactly one deterministic-anchor skeleton in
// the entrance region, zero in the corridor, exactly one deterministic-
// anchor non-wight skeleton-captain (rpg-toolkit#816) in the boss region
// — no PositionOracle, no search/retry, no goblins.
//
// package lobby (not lobby_test): regionMonsterAnchor/buildMonsterSeedGroups/
// seedRegionMonsters are unexported, and several assertions here (the
// PositionOracle==nil / FixedPositions structural proof) can only be made
// by inspecting the built spawn.EntityGroup directly — the black-box
// StartEncounter tests in start_encounter_test.go never observe this.

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/encounter/perception"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
)

// regionArchetypeAt mirrors start_encounter_test.go's package-lobby_test
// helper of the same name — duplicated here (not imported; test packages
// can't import each other) because this file is white-box (package lobby)
// and needs the same region-archetype lookup for its own assertions.
func regionArchetypeAt(space *tkenc.SpaceData, hex core.Hex) (tkenc.RegionArchetype, bool) {
	for _, r := range space.Regions {
		if r.Hexes.Has(hex) {
			return r.Archetype, true
		}
	}
	return "", false
}

// newTestCryptEncounter builds a fresh, in-memory *tkenc.Encounter with the
// crypt dungeonSpec's InitDungeon already run for the given seed — the same
// call StartEncounter itself makes, exercised directly so these tests don't
// need a full Orchestrator/lobby/character-repo stack.
func newTestCryptEncounter(t *testing.T, seed int64) *tkenc.Encounter {
	t.Helper()
	ctx := context.Background()
	broker := tkenc.NewBroker(tkenc.NewInMemoryTransport())
	enc := tkenc.New(ctx, core.EncounterID("test-enc"), broker)
	_, spec, err := resolveDungeonSpec(DungeonKeyCrypt)
	require.NoError(t, err)
	require.NoError(t, enc.InitDungeon(tkenc.DungeonParams{
		Regions:    spec.regions,
		Connectors: spec.connectors,
		Height:     spec.height,
		RandomSeed: seed,
		Theme:      spec.theme,
	}))
	return enc
}

// addPartyLine mirrors StartEncounter's own party-placement loop (line
// spawnPositionSpacing per member, starting at SpaceData.Entrance) so
// monster-seeding tests see the exact same room occupancy StartEncounter
// itself produces before seeding monsters.
func addPartyLine(t *testing.T, enc *tkenc.Encounter, partySize int) {
	t.Helper()
	base := enc.ToData().Space.Entrance
	for i := 0; i < partySize; i++ {
		q := i * spawnPositionSpacing
		require.NoError(t, enc.AddPlayer(tkenc.PlayerInput{
			PlayerID:   core.PlayerID(fmt.Sprintf("p%d", i)),
			EntityID:   core.EntityID(fmt.Sprintf("char-p%d", i)),
			Position:   core.Hex{Q: base.Q + q, R: base.R, S: base.S - q},
			SightRange: memberSightRange,
			HP:         1, MaxHP: 1, AC: 1,
		}))
	}
}

// TestRegionMonsterAnchor_EntranceAndBoss_ResolveDistinctInRegionAnchors
// proves regionMonsterAnchor resolves each archetype to the door-neighbor
// cell tagged into ITS OWN region (regionEntryAnchor, already tested),
// using each region's OWN adjacent connector — never a hand-rolled center/
// offset computation.
func TestRegionMonsterAnchor_EntranceAndBoss_ResolveDistinctInRegionAnchors(t *testing.T) {
	enc := newTestCryptEncounter(t, 42)
	space := enc.ToData().Space
	_, spec, err := resolveDungeonSpec(DungeonKeyCrypt)
	require.NoError(t, err)

	entranceAnchor, entranceRegionID, err := regionMonsterAnchor(space, spec.regions, spec.connectors, enc.ToData().Doors, enc.Room(), tkenc.ArchetypeEntrance)
	require.NoError(t, err)
	bossAnchor, bossRegionID, err := regionMonsterAnchor(space, spec.regions, spec.connectors, enc.ToData().Doors, enc.Room(), tkenc.ArchetypeBoss)
	require.NoError(t, err)

	require.Equal(t, cryptRegionIDEntrance, entranceRegionID)
	require.Equal(t, cryptRegionIDBoss, bossRegionID)
	require.NotEqual(t, entranceAnchor, bossAnchor)

	region, ok := space.RegionAt(entranceAnchor)
	require.True(t, ok)
	require.Equal(t, cryptRegionIDEntrance, region)

	region, ok = space.RegionAt(bossAnchor)
	require.True(t, ok)
	require.Equal(t, cryptRegionIDBoss, region)
}

// TestRegionMonsterAnchor_CorridorArchetype_NotInTable proves the corridor
// region intentionally has no monster spec — regionMonsterAnchor itself is
// archetype-driven and would resolve a corridor anchor if asked, but the
// crypt monster-seed table (buildMonsterSeedGroups) must never ask for one.
func TestBuildMonsterSeedGroups_CorridorGetsNoGroup(t *testing.T) {
	enc := newTestCryptEncounter(t, 7)
	_, spec, err := resolveDungeonSpec(DungeonKeyCrypt)
	require.NoError(t, err)

	_, plans, groups, err := buildMonsterSeedGroups(enc.ToData().Space, enc.ToData().Doors, enc.Room(), spec, DungeonKeyCrypt)
	require.NoError(t, err)
	require.Len(t, groups, 2, "exactly entrance + boss groups, none for the corridor")
	require.Len(t, plans, 2)
}

// TestBuildMonsterSeedGroups_NoSearch_FixedPositionsOnlyNoOracle is the
// structural proof that this path never exercises PositionOracle/search:
// every built group carries exactly len(FixedPositions)==Quantity.Fixed and
// a nil PositionOracle.
func TestBuildMonsterSeedGroups_NoSearch_FixedPositionsOnlyNoOracle(t *testing.T) {
	enc := newTestCryptEncounter(t, 7)
	_, spec, err := resolveDungeonSpec(DungeonKeyCrypt)
	require.NoError(t, err)

	_, _, groups, err := buildMonsterSeedGroups(enc.ToData().Space, enc.ToData().Doors, enc.Room(), spec, DungeonKeyCrypt)
	require.NoError(t, err)
	for _, g := range groups {
		require.Nil(t, g.PositionOracle, "group %q must not carry a search predicate", g.ID)
		require.NotNil(t, g.Quantity.Fixed)
		require.Len(t, g.FixedPositions, *g.Quantity.Fixed,
			"group %q must supply exactly one fixed position per requested entity — no fallthrough to search", g.ID)
	}
}

// TestBuildMonsterSeedGroups_UnknownDungeonKey_Errors proves no silent
// fallback: an unregistered dungeon key must fail loudly, never silently
// seed zero monsters or substitute a default table.
func TestBuildMonsterSeedGroups_UnknownDungeonKey_Errors(t *testing.T) {
	enc := newTestCryptEncounter(t, 7)
	_, spec, err := resolveDungeonSpec(DungeonKeyCrypt)
	require.NoError(t, err)

	_, _, _, err = buildMonsterSeedGroups(enc.ToData().Space, enc.ToData().Doors, enc.Room(), spec, DungeonKey("no-such-key"))
	require.Error(t, err)
}

// TestSeedRegionMonsters_Composition_OneEntranceZeroCorridorOneBoss is the
// composition done-bar proof: exactly 1 skeleton in the entrance region, 0
// in the corridor, exactly 1 skeleton-captain in the boss region — by ref,
// no goblins anywhere.
func TestSeedRegionMonsters_Composition_OneEntranceZeroCorridorOneBoss(t *testing.T) {
	enc := newTestCryptEncounter(t, 99)
	addPartyLine(t, enc, 4) // worst case: full party occupies the spawn line first.
	_, spec, err := resolveDungeonSpec(DungeonKeyCrypt)
	require.NoError(t, err)

	o := &Orchestrator{}
	require.NoError(t, o.seedRegionMonsters(context.Background(), enc, core.EncounterID("test-enc"), spec, DungeonKeyCrypt))

	data := enc.ToData()
	require.Len(t, data.Monsters, 2, "exactly one entrance minion + one boss, nothing else")

	counts := map[tkenc.RegionArchetype]int{}
	for id, m := range data.Monsters {
		archetype, ok := regionArchetypeAt(data.Space, m.Position)
		require.True(t, ok, "monster %q must be placed inside a tagged region", id)
		counts[archetype]++
		switch archetype {
		case tkenc.ArchetypeEntrance:
			require.Equal(t, refs.Monsters.Skeleton().String(), m.MonsterRef, "entrance monster must be the plain skeleton, not a goblin")
		case tkenc.ArchetypeBoss:
			require.Equal(t, refs.Monsters.SkeletonCaptain().String(), m.MonsterRef, "boss must be rpg-toolkit#816's non-wight skeleton captain")
		default:
			t.Fatalf("unexpected monster in archetype %q", archetype)
		}
		require.Positive(t, m.HP)
		require.NotEmpty(t, m.DataJSON, "monster %q must carry hydration DataJSON", id)
	}
	require.Equal(t, 1, counts[tkenc.ArchetypeEntrance])
	require.Equal(t, 1, counts[tkenc.ArchetypeBoss])
	require.Zero(t, counts[tkenc.ArchetypeCorridor], "the corridor must get zero monsters")
}

// TestSeedRegionMonsters_Deterministic_SameSeedSamePositions is the
// byte-identical determinism done-bar proof: the same seed always produces
// the same monster positions, run twice from scratch.
func TestSeedRegionMonsters_Deterministic_SameSeedSamePositions(t *testing.T) {
	positionsFor := func(seed int64) map[tkenc.RegionArchetype]core.Hex {
		enc := newTestCryptEncounter(t, seed)
		_, spec, err := resolveDungeonSpec(DungeonKeyCrypt)
		require.NoError(t, err)
		o := &Orchestrator{}
		require.NoError(t, o.seedRegionMonsters(context.Background(), enc, core.EncounterID("test-enc"), spec, DungeonKeyCrypt))
		out := map[tkenc.RegionArchetype]core.Hex{}
		for _, m := range enc.ToData().Monsters {
			archetype, ok := regionArchetypeAt(enc.ToData().Space, m.Position)
			require.True(t, ok)
			out[archetype] = m.Position
		}
		return out
	}

	first := positionsFor(2026)
	second := positionsFor(2026)
	require.Equal(t, first, second, "the same seed must produce byte-identical monster positions")
}

// TestSeedRegionMonsters_PartySizeInvariant_1Through4 proves party size
// never alters monster positions: only the seed/layout does.
func TestSeedRegionMonsters_PartySizeInvariant_1Through4(t *testing.T) {
	var reference map[tkenc.RegionArchetype]core.Hex
	for partySize := 1; partySize <= 4; partySize++ {
		enc := newTestCryptEncounter(t, 555)
		addPartyLine(t, enc, partySize)
		_, spec, err := resolveDungeonSpec(DungeonKeyCrypt)
		require.NoError(t, err)
		o := &Orchestrator{}
		require.NoError(t, o.seedRegionMonsters(context.Background(), enc, core.EncounterID("test-enc"), spec, DungeonKeyCrypt),
			"party size %d must place monsters with zero errors", partySize)

		got := map[tkenc.RegionArchetype]core.Hex{}
		for _, m := range enc.ToData().Monsters {
			archetype, ok := regionArchetypeAt(enc.ToData().Space, m.Position)
			require.True(t, ok)
			got[archetype] = m.Position
		}
		if reference == nil {
			reference = got
		} else {
			require.Equal(t, reference, got, "party size %d must not change monster positions", partySize)
		}
	}
}

// TestSeedRegionMonsters_SeedSweep_1To1000_PartySizes1To4_ZeroErrors is the
// revised done-bar's exact matrix: seeds 1..1000 x party sizes 1..4, zero
// placement errors. This is the empirical proof that regionEntryAnchor's
// door-neighbor resolution always lands on a toolkit-guaranteed-safe
// (required-path) cell for every seed the crypt template can generate —
// not something asserted by static analysis, but swept directly against
// the real toolkit generator.
func TestSeedRegionMonsters_SeedSweep_1To1000_PartySizes1To4_ZeroErrors(t *testing.T) {
	_, spec, err := resolveDungeonSpec(DungeonKeyCrypt)
	require.NoError(t, err)

	for seed := int64(1); seed <= 1000; seed++ {
		for partySize := 1; partySize <= 4; partySize++ {
			enc := newTestCryptEncounter(t, seed)
			addPartyLine(t, enc, partySize)
			o := &Orchestrator{}
			err := o.seedRegionMonsters(context.Background(), enc, core.EncounterID("test-enc"), spec, DungeonKeyCrypt)
			require.NoError(t, err, "seed=%d partySize=%d must place both monsters with zero errors", seed, partySize)
			require.Len(t, enc.ToData().Monsters, 2, "seed=%d partySize=%d must seed exactly 2 monsters", seed, partySize)
		}
	}
}

// TestSeedRegionMonsters_BossConcealedByClosedDoor_NotByPlacementSearch
// proves the boss's concealment comes from door/wall geometry (a real
// perception.CanSeeAt check against the closed connector door), not from
// any out-of-sight placement predicate — there is none left in this path.
func TestSeedRegionMonsters_BossConcealedByClosedDoor_NotByPlacementSearch(t *testing.T) {
	enc := newTestCryptEncounter(t, 314)
	_, spec, err := resolveDungeonSpec(DungeonKeyCrypt)
	require.NoError(t, err)
	o := &Orchestrator{}
	require.NoError(t, o.seedRegionMonsters(context.Background(), enc, core.EncounterID("test-enc"), spec, DungeonKeyCrypt))

	data := enc.ToData()
	entranceAnchor, _, err := regionMonsterAnchor(data.Space, spec.regions, spec.connectors, data.Doors, enc.Room(), tkenc.ArchetypeEntrance)
	require.NoError(t, err)

	var bossPos core.Hex
	for _, m := range data.Monsters {
		if archetype, _ := regionArchetypeAt(data.Space, m.Position); archetype == tkenc.ArchetypeBoss {
			bossPos = m.Position
		}
	}
	require.NotZero(t, bossPos)

	room := enc.Room()
	require.NotNil(t, room)
	view := &perception.View{Position: entranceAnchor, SightRange: memberSightRange}
	require.False(t, perception.CanSeeAt(view, bossPos, room),
		"boss must be concealed from the entrance side while the boss door is closed — by wall/door geometry, not a search predicate")
}
