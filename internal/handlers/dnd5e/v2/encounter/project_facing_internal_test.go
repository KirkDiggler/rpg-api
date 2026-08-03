package encounter

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/encounter/perception"
)

// TestObservationToProto_PreservesOptionalPlacementFacing forwards the toolkit
// pointer unchanged. In particular, no API-side direction inference may turn
// an absent override into E=0, and explicit E must not be dropped as zero.
func TestObservationToProto_PreservesOptionalPlacementFacing(t *testing.T) {
	facingEast := uint32(0)
	facingNortheast := uint32(1)
	facingNorthwest := uint32(2)
	facingWest := uint32(3)
	facingSouthwest := uint32(4)
	facingSoutheast := uint32(5)

	testCases := []struct {
		name   string
		facing *uint32
	}{
		{name: "absent", facing: nil},
		{name: "explicit east is present zero", facing: &facingEast},
		{name: "northeast", facing: &facingNortheast},
		{name: "northwest", facing: &facingNorthwest},
		{name: "west", facing: &facingWest},
		{name: "southwest", facing: &facingSouthwest},
		{name: "southeast", facing: &facingSoutheast},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			record := observationToProto(perception.HexObservation{
				Contents: []perception.Placement{{
					EntityID: core.EntityID("authored-prop"),
					Facing:   tc.facing,
				}},
			})

			placement := record.GetContents()[0]
			require.Same(t, tc.facing, placement.Facing,
				"projection must forward optional facing without inference, mapping, or allocation")
		})
	}
}
