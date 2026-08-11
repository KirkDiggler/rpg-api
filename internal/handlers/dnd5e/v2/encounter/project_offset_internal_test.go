package encounter

import (
	"testing"

	"github.com/stretchr/testify/require"

	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
	tkencevents "github.com/KirkDiggler/rpg-toolkit/encounter/events"
	"github.com/KirkDiggler/rpg-toolkit/encounter/perception"
)

// TestPlacementOffsetProjectionDiscriminatesAxesPresenceAndFacing is mutation
// evidence for the thin wire boundary: dropping/swapping/negating any component,
// treating zero as absent, or rotating by facing makes a distinct assertion fail.
func TestPlacementOffsetProjectionDiscriminatesAxesPresenceAndFacing(t *testing.T) {
	east := uint32(0)
	zero := core.PlacementOffset{0, 0, 0}
	signed := core.PlacementOffset{0.125, -2.5, 3.75}
	obs := perception.HexObservation{Contents: []perception.Placement{
		{EntityID: "absent"},
		{EntityID: "zero", Offset: &zero},
		{EntityID: "signed", Facing: &east, Offset: &signed},
	}}

	got := observationToProto(obs).GetContents()
	require.Len(t, got, 3)
	require.Nil(t, got[0].GetOffset())
	require.NotNil(t, got[1].GetOffset(), "explicit [0,0,0] must remain present")
	require.Zero(t, got[1].GetOffset().GetX())
	require.Zero(t, got[1].GetOffset().GetY())
	require.Zero(t, got[1].GetOffset().GetZ())
	require.NotNil(t, got[2].Facing, "explicit E=0 must remain present independently")
	require.Equal(t, 0.125, got[2].GetOffset().GetX())
	require.Equal(t, -2.5, got[2].GetOffset().GetY())
	require.Equal(t, 3.75, got[2].GetOffset().GetZ())
}

func TestKnownHexToObservation_PreservesPlacementOffsetWithoutReinterpretation(t *testing.T) {
	offset := core.PlacementOffset{-0.25, 1.5, 2.75}
	obs := knownHexToObservation(tkencevents.KnownHex{Contents: []tkencevents.KnownHexPlacement{{
		EntityID: "monster-1", Offset: &offset,
	}}})

	require.Len(t, obs.Contents, 1)
	require.Equal(t, offset, *obs.Contents[0].Offset)
	require.Same(t, &offset, obs.Contents[0].Offset, "same toolkit type needs no conversion or allocation")
}

func TestPlacementForEntity_OffsetFollowsMovedMonsterAndPropMetadata(t *testing.T) {
	monsterOffset := core.PlacementOffset{1, -2, 3}
	propOffset := core.PlacementOffset{-4, 5, -6}
	facing := uint32(0)
	data := &tkenc.Data{
		Monsters: map[core.EntityID]*tkenc.MonsterData{
			"monster-1": {ID: "monster-1", Position: core.Hex{Q: 1, R: -1}, Offset: &monsterOffset},
		},
		Space: &tkenc.SpaceData{Obstacles: []tkenc.ObstacleData{{
			ID: "prop-1", Facing: &facing, Offset: &propOffset,
		}}},
	}

	before := placementForEntity(data, "monster-1")
	data.Monsters["monster-1"].Position = core.Hex{Q: 8, R: -3, S: -5}
	after := placementForEntity(data, "monster-1")
	require.Equal(t, monsterOffset, *before.Offset)
	require.Equal(t, monsterOffset, *after.Offset, "movement changes canonical origin only")

	prop := placementForEntity(data, "prop-1")
	require.NotNil(t, prop.Facing)
	require.Zero(t, *prop.Facing)
	require.Equal(t, propOffset, *prop.Offset)

	vacated := observationToProto(perception.HexObservation{Contents: nil})
	require.Empty(t, vacated.GetContents(), "a total empty record retains no stale offset")
}
