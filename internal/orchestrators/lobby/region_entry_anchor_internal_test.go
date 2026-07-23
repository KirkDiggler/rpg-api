package lobby

// White-box unit tests for regionEntryAnchor (start_encounter.go) —
// package lobby (not lobby_test): regionEntryAnchor is unexported, and the
// bug the second test pins (Copilot review, PR #677, against the retired
// chamberEntryAnchor this generalizes) could only be caught by asserting
// the RETURNED HEX'S OWN REGION TAG directly — the black-box StartEncounter
// tests in start_encounter_test.go never observe this helper's return
// value in isolation.
//
// rpg-api#688: generalized from chamberEntryAnchor (hardcoded to a
// tkenc.RegionChamber1 special case for the first chamber) to a plain
// door-neighbor resolver usable for ANY named region — the entrance
// region's own anchor (SpaceData.Entrance) is now resolved directly by
// seedGoblins itself, not by this helper, so there is no special-cased
// region-zero branch left to test here. Proven below with arbitrary
// region IDs unrelated to the retired two-chamber constants, so the fix
// can never regress back to a hardcoded special case.

import (
	"testing"

	"github.com/stretchr/testify/require"

	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/encounter/perception"
)

// TestRegionEntryAnchor_ReturnsNamedRegionsDoorNeighbor is the Copilot-
// review regression pin (PR #677), generalized: regionEntryAnchor must
// match the door's neighbors against the CALLER-SUPPLIED regionID, not any
// hardcoded region — proven here with arbitrary region IDs, so this can
// never regress back to a hardcoded special case the way the retired
// chamberEntryAnchor once did.
func TestRegionEntryAnchor_ReturnsNamedRegionsDoorNeighbor(t *testing.T) {
	door := &tkenc.DoorData{ID: "door-1", Position: core.Hex{Q: 0, R: 0, S: 0}}
	neighbors := perception.HexNeighbors(door.Position)
	regionANeighbor := neighbors[0]
	regionBNeighbor := neighbors[1]
	space := &tkenc.SpaceData{
		Regions: []tkenc.RegionData{
			{ID: "region-a", Hexes: core.NewHexSet(regionANeighbor)},
			{ID: "region-b", Hexes: core.NewHexSet(regionBNeighbor)},
		},
	}

	got, err := regionEntryAnchor(space, door, "region-b", nil)
	require.NoError(t, err)
	require.Equal(t, regionBNeighbor, got,
		"region-b's anchor must be the hex tagged INTO region-b, not region-a's neighbor")
	require.NotEqual(t, regionANeighbor, got)

	region, ok := space.RegionAt(got)
	require.True(t, ok)
	require.Equal(t, "region-b", region)
}

// TestRegionEntryAnchor_NoMatchingNeighbor_Errors covers the defensive
// error path: a region with no door-adjacent tagged neighbor (a data shape
// InitDungeon never produces) must fail loudly rather than silently
// returning the zero Hex — seedGoblins treats a zero Hex indistinguishably
// from a real coordinate, so a silent zero-value return here would corrupt
// out-of-sight seeding instead of surfacing the gap.
func TestRegionEntryAnchor_NoMatchingNeighbor_Errors(t *testing.T) {
	door := &tkenc.DoorData{ID: "door-1", Position: core.Hex{Q: 0, R: 0, S: 0}}
	space := &tkenc.SpaceData{} // no Regions at all

	_, err := regionEntryAnchor(space, door, "region-b", nil)
	require.Error(t, err)
}
