package lobby

// crypt_dungeon_params_fixture_internal_test.go proves rpg-api#689's
// anchor-resolution logic (regionMonsterAnchor/buildMonsterSeedGroups)
// works directly against rpg-toolkit's OWN canonical CryptDungeonParams
// constructor (rpg-toolkit#826, published encounter v0.38.0) — NOT through
// this package's own dungeonSpec (dungeon_spec.go's hand-authored crypt
// entry, which mirrors the same 3-region entrance/corridor/boss shape at
// different dimensions and omits Obstacles entirely, since Obstacles
// projection is rpg-api#694/PR#697's job, not this one's).
//
// This is the "focused compact-spec production helper path without
// consuming PR #697" the task requires: regionMonsterAnchor/
// buildMonsterSeedGroups are plain functions over
// []tkenc.DungeonRegionParams/[]tkenc.DungeonConnectorParams/*tkenc.
// SpaceData — the SAME production code path start_encounter.go's
// seedRegionMonsters calls, exercised here with CryptDungeonParams'
// literal output instead of dungeonSpecs[DungeonKeyCrypt]. No test-only
// production branch exists anywhere in crypt_monster_seed.go for this.
//
// Independent of PR #697: this file imports nothing from that branch and
// never wires CryptDungeonParams into dungeonSpec itself — it only proves
// the anchor/seeding logic is COMPATIBLE with CryptDungeonParams' shape
// today, so whenever #694/#697 does make that switch, this logic keeps
// working unchanged.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
)

const (
	fixtureEntranceDoorID core.EntityID = "fixture-door-entrance-corridor"
	fixtureBossDoorID     core.EntityID = "fixture-door-corridor-boss"
)

// dungeonSpecFromCryptDungeonParams adapts tkenc.CryptDungeonParams'
// literal DungeonParams output into this package's dungeonSpec shape — a
// pure field copy, no derivation, no geometry.
func dungeonSpecFromCryptDungeonParams(p tkenc.DungeonParams) dungeonSpec {
	return dungeonSpec{
		theme:      p.Theme,
		height:     p.Height,
		regions:    p.Regions,
		connectors: p.Connectors,
	}
}

func newEncounterFromCryptDungeonParams(t *testing.T, seed int64) (*tkenc.Encounter, dungeonSpec) {
	t.Helper()
	params := tkenc.CryptDungeonParams(seed, fixtureEntranceDoorID, fixtureBossDoorID)
	broker := tkenc.NewBroker(tkenc.NewInMemoryTransport())
	enc := tkenc.New(context.Background(), core.EncounterID("fixture-enc"), broker)
	require.NoError(t, enc.InitDungeon(params))
	return enc, dungeonSpecFromCryptDungeonParams(params)
}

// TestRegionMonsterAnchor_AgainstRealCryptDungeonParams_ResolvesBothAnchors
// proves the production anchor helper works against the toolkit's OWN
// canonical constructor, at ITS dimensions (cryptHeight=8/
// cryptEntranceWidth=10/cryptCorridorWidth=5/cryptBossWidth=10 — all
// unexported toolkit internals this test never references directly),
// independent of rpg-api's own dungeonSpec.
func TestRegionMonsterAnchor_AgainstRealCryptDungeonParams_ResolvesBothAnchors(t *testing.T) {
	enc, spec := newEncounterFromCryptDungeonParams(t, 1)
	space := enc.ToData().Space
	require.Equal(t, "crypt", spec.theme, "CryptDungeonParams' own opaque theme")
	require.Len(t, spec.regions, 3)

	entranceAnchor, entranceRegionID, err := regionMonsterAnchor(space, spec.regions, spec.connectors, enc.ToData().Doors, enc.Room(), tkenc.ArchetypeEntrance)
	require.NoError(t, err)
	bossAnchor, bossRegionID, err := regionMonsterAnchor(space, spec.regions, spec.connectors, enc.ToData().Doors, enc.Room(), tkenc.ArchetypeBoss)
	require.NoError(t, err)
	require.NotEqual(t, entranceAnchor, bossAnchor)

	region, ok := space.RegionAt(entranceAnchor)
	require.True(t, ok)
	require.Equal(t, entranceRegionID, region)
	region, ok = space.RegionAt(bossAnchor)
	require.True(t, ok)
	require.Equal(t, bossRegionID, region)
}

// TestSeedRegionMonsters_AgainstRealCryptDungeonParams_Composition proves
// the full production seeding path (buildMonsterSeedGroups +
// seedRegionMonsters) against CryptDungeonParams' real output: exactly one
// entrance skeleton, zero corridor monsters, exactly one boss skeleton-
// captain, refs verified against rpg-toolkit's own refs package.
func TestSeedRegionMonsters_AgainstRealCryptDungeonParams_Composition(t *testing.T) {
	enc, spec := newEncounterFromCryptDungeonParams(t, 7)
	o := &Orchestrator{}
	require.NoError(t, o.seedRegionMonsters(context.Background(), enc, core.EncounterID("fixture-enc"), spec, DungeonKeyCrypt))

	data := enc.ToData()
	require.Len(t, data.Monsters, 2)
	counts := map[tkenc.RegionArchetype]int{}
	for _, m := range data.Monsters {
		archetype, ok := regionArchetypeAt(data.Space, m.Position)
		require.True(t, ok)
		counts[archetype]++
		switch archetype {
		case tkenc.ArchetypeEntrance:
			require.Equal(t, refs.Monsters.Skeleton().String(), m.MonsterRef)
		case tkenc.ArchetypeBoss:
			require.Equal(t, refs.Monsters.SkeletonCaptain().String(), m.MonsterRef)
		}
	}
	require.Equal(t, 1, counts[tkenc.ArchetypeEntrance])
	require.Equal(t, 1, counts[tkenc.ArchetypeBoss])
	require.Zero(t, counts[tkenc.ArchetypeCorridor])
}

// TestSeedRegionMonsters_AgainstRealCryptDungeonParams_SeedSweep_1To1000
// is the same zero-placement-errors matrix proof as the dungeonSpec
// version, run against CryptDungeonParams' own dimensions directly —
// proving the anchor logic's safety isn't an artifact of rpg-api's
// specific (larger) crypt dimensions.
func TestSeedRegionMonsters_AgainstRealCryptDungeonParams_SeedSweep_1To1000(t *testing.T) {
	for seed := int64(1); seed <= 1000; seed++ {
		enc, spec := newEncounterFromCryptDungeonParams(t, seed)
		o := &Orchestrator{}
		err := o.seedRegionMonsters(context.Background(), enc, core.EncounterID("fixture-enc"), spec, DungeonKeyCrypt)
		require.NoError(t, err, "seed=%d must place both monsters with zero errors against real CryptDungeonParams output", seed)
		require.Len(t, enc.ToData().Monsters, 2, "seed=%d", seed)
	}
}
