package toolkit

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
)

type SpawnDiagnosticTestSuite struct {
	suite.Suite
}

func TestSpawnDiagnosticTestSuite(t *testing.T) {
	suite.Run(t, new(SpawnDiagnosticTestSuite))
}

// TestDiagnoseSpawnPositions generates a dungeon and prints spawn positions vs wall positions
func (s *SpawnDiagnosticTestSuite) TestDiagnoseSpawnPositions() {
	generator := CreateGenerator(&ToolkitConfig{})

	output, err := generator.Generate(context.Background(), &dungeon.GenerateInput{
		Theme:     dungeon.ThemeCrypt,
		Size:      dungeon.RoomSizeSmall,
		Length:    3,
		Layout:    dungeon.LayoutLinear,
		PartySize: 4,
		TargetCR:  2,
		Seed:      12345,
	})

	s.Require().NoError(err)
	s.Require().NotNil(output)

	for i, room := range output.Dungeon.Rooms {
		fmt.Printf("\n=== Room %d (ID: %s) ===\n", i, room.ID)
		fmt.Printf("Shape: %dx%d\n", room.Shape.Width, room.Shape.Height)

		// Print wall positions
		fmt.Printf("\nWall positions:\n")
		wallSet := make(map[string]bool)
		for _, wall := range room.Walls {
			fmt.Printf("  Wall %s: (%d,%d,%d) -> (%d,%d,%d) blocks=%v\n",
				wall.ID, wall.Start.X, wall.Start.Y, wall.Start.Z,
				wall.End.X, wall.End.Y, wall.End.Z, wall.BlocksMovement)

			// Enumerate positions along wall
			if wall.BlocksMovement {
				dx := absInt(wall.End.X - wall.Start.X)
				dz := absInt(wall.End.Z - wall.Start.Z)
				steps := maxInt(dx, dz)
				if steps == 0 {
					steps = 1
				}
				for j := 0; j <= steps; j++ {
					t := float64(j) / float64(steps)
					x := wall.Start.X + int(float64(wall.End.X-wall.Start.X)*t)
					z := wall.Start.Z + int(float64(wall.End.Z-wall.Start.Z)*t)
					y := -x - z
					key := fmt.Sprintf("%d,%d,%d", x, y, z)
					wallSet[key] = true
				}
			}
		}

		// Print spawn zones and check for collisions
		fmt.Printf("\nSpawn zones:\n")
		for _, zone := range room.SpawnZones {
			fmt.Printf("  Zone %s (type=%d, cap=%d):\n", zone.ID, zone.Type, zone.Capacity)
			for _, pos := range zone.Bounds {
				key := fmt.Sprintf("%d,%d,%d", pos.X, pos.Y, pos.Z)
				onWall := wallSet[key]
				status := "OK"
				if onWall {
					status = "ON WALL!"
				}
				fmt.Printf("    Position (%d,%d,%d) - %s\n", pos.X, pos.Y, pos.Z, status)
			}
		}

		// Print perimeter bounds (Z values that are wall rows)
		fmt.Printf("\nPerimeter Z values: 0 and %d\n", room.Shape.Height-1)

		// Check if spawn positions are on perimeter rows
		fmt.Printf("\nChecking if spawn positions are on perimeter rows:\n")
		for _, zone := range room.SpawnZones {
			for _, pos := range zone.Bounds {
				if pos.Z == 0 || pos.Z == room.Shape.Height-1 {
					fmt.Printf("  WARNING: Position (%d,%d,%d) is on perimeter row Z=%d\n",
						pos.X, pos.Y, pos.Z, pos.Z)
				}
			}
		}
	}
}

// absInt and maxInt are defined in feature.go
