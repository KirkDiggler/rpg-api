package encounter

import (
	"testing"

	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
	"github.com/stretchr/testify/assert"
)

func TestHexDistanceCalculation(t *testing.T) {
	// Create a hex grid like in our game
	hexGrid := spatial.NewHexGrid(spatial.HexGridConfig{
		Width:     10,
		Height:    10,
		PointyTop: true, // D&D 5e standard
	})

	testCases := []struct {
		name     string
		from     spatial.Position
		to       spatial.Position
		expected int // Expected distance in hexes
	}{
		{
			name:     "Adjacent hex",
			from:     spatial.Position{X: 5, Y: 5},
			to:       spatial.Position{X: 6, Y: 5},
			expected: 1,
		},
		{
			name:     "Two hexes away",
			from:     spatial.Position{X: 5, Y: 5},
			to:       spatial.Position{X: 7, Y: 5},
			expected: 2,
		},
		{
			name:     "Diagonal movement",
			from:     spatial.Position{X: 5, Y: 5},
			to:       spatial.Position{X: 6, Y: 4},
			expected: 1, // Diagonals in hex are still 1 hex
		},
		{
			name:     "Six hexes straight",
			from:     spatial.Position{X: 2, Y: 3},
			to:       spatial.Position{X: 8, Y: 3},
			expected: 6,
		},
		{
			name:     "Eight hexes (beyond movement limit)",
			from:     spatial.Position{X: 1, Y: 3},
			to:       spatial.Position{X: 9, Y: 3},
			expected: 8,
		},
		{
			name:     "Complex diagonal path",
			from:     spatial.Position{X: 2, Y: 2},
			to:       spatial.Position{X: 7, Y: 7},
			expected: 7, // Hex distance calculation
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			distance := hexGrid.Distance(tc.from, tc.to)
			// Distance returns float64 but for hex grids it's always an integer
			assert.Equal(t, tc.expected, int(distance), "Distance should be %d hexes", tc.expected)
			
			// Also verify movement cost in feet
			movementFeet := int(distance) * 5
			t.Logf("Move from %v to %v: %d hexes = %d feet", tc.from, tc.to, int(distance), movementFeet)
		})
	}
}