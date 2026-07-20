package lobby

// White-box unit tests for chamberEntryAnchor (start_encounter.go).
// package lobby (not lobby_test): chamberEntryAnchor is unexported, and the
// bug this file's second test pins (Copilot review, PR #677) could only be
// caught by asserting the RETURNED HEX'S OWN REGION TAG directly — the
// black-box StartEncounter tests in start_encounter_test.go never observe
// chamberEntryAnchor's return value in isolation, so they could not have
// caught a wrong-side anchor whose only symptom is which door-adjacent hex
// gets used (see the second test's doc for why the black-box integration
// suite also stayed green under the bug).

import (
	"testing"

	"github.com/stretchr/testify/require"

	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/encounter/perception"
)

func TestChamberEntryAnchor_Chamber1_ReturnsEntrance(t *testing.T) {
	entrance := core.Hex{Q: 0, R: -5, S: 5}
	space := &tkenc.SpaceData{Entrance: entrance}
	door := &tkenc.DoorData{ID: "door-1", Position: core.Hex{Q: 5, R: -5, S: 0}}

	got, err := chamberEntryAnchor(space, door, tkenc.RegionChamber1)
	require.NoError(t, err)
	require.Equal(t, entrance, got)
}

// TestChamberEntryAnchor_Chamber2_ReturnsChamber2SideNeighbor is the
// Copilot-review regression pin (PR #677): chamberEntryAnchor used to
// match the door's neighbors against a HARDCODED tkenc.RegionChamber1
// regardless of which regionID was actually asked for, so calling it with
// RegionChamber2 silently returned a hex tagged into chamber 1 — violating
// the function's own doc ("the entry hex for the NAMED region").
//
// This fixture gives the door two DISTINCT tagged neighbors, one per
// chamber, so a wrong-side return is directly observable: the old code
// would return chamber1Neighbor here, failing this assertion outright.
// (It would NOT have failed under StartEncounter's black-box integration
// coverage: the door is always closed at seed time, so a closed door's
// full LoS block at its own cell — rpg-toolkit#790 — makes chamber 2
// invisible from EITHER side of it, masking the wrong anchor's practical
// effect. Only a direct, white-box assertion on the returned hex's own
// region tag catches this class of bug.)
func TestChamberEntryAnchor_Chamber2_ReturnsChamber2SideNeighbor(t *testing.T) {
	door := &tkenc.DoorData{ID: "door-1", Position: core.Hex{Q: 0, R: 0, S: 0}}
	neighbors := perception.HexNeighbors(door.Position)
	chamber1Neighbor := neighbors[0]
	chamber2Neighbor := neighbors[1]
	space := &tkenc.SpaceData{
		Regions: []tkenc.RegionData{
			{ID: tkenc.RegionChamber1, Hexes: core.NewHexSet(chamber1Neighbor)},
			{ID: tkenc.RegionChamber2, Hexes: core.NewHexSet(chamber2Neighbor)},
		},
	}

	got, err := chamberEntryAnchor(space, door, tkenc.RegionChamber2)
	require.NoError(t, err)
	require.Equal(t, chamber2Neighbor, got,
		"chamber 2's anchor must be the hex tagged INTO chamber 2, not chamber 1's neighbor")
	require.NotEqual(t, chamber1Neighbor, got)

	region, ok := space.RegionAt(got)
	require.True(t, ok)
	require.Equal(t, tkenc.RegionChamber2, region)
}

// TestChamberEntryAnchor_NoMatchingNeighbor_Errors covers the defensive
// error path: a region with no door-adjacent tagged neighbor (a data shape
// the current two-chamber generator never produces) must fail loudly
// rather than silently returning the zero Hex — seedGoblins treats a zero
// Hex indistinguishably from a real coordinate, so a silent zero-value
// return here would corrupt out-of-sight seeding instead of surfacing the
// gap.
func TestChamberEntryAnchor_NoMatchingNeighbor_Errors(t *testing.T) {
	door := &tkenc.DoorData{ID: "door-1", Position: core.Hex{Q: 0, R: 0, S: 0}}
	space := &tkenc.SpaceData{} // no Regions at all

	_, err := chamberEntryAnchor(space, door, tkenc.RegionChamber2)
	require.Error(t, err)
}
