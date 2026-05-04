package toolkit

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
	"github.com/KirkDiggler/rpg-api/internal/components/spawner"
)

type PlacementValidationTestSuite struct {
	suite.Suite
}

func TestPlacementValidationTestSuite(t *testing.T) {
	suite.Run(t, new(PlacementValidationTestSuite))
}

// TestValidateMonsterPlacement traces the full monster placement flow
func (s *PlacementValidationTestSuite) TestValidateMonsterPlacement() {
	generator := CreateGenerator(&ToolkitConfig{})

	output, err := generator.Generate(context.Background(), &dungeon.GenerateInput{
		Theme:     dungeon.ThemeCrypt,
		Size:      dungeon.RoomSizeSmall,
		Length:    3,
		Layout:    dungeon.LayoutLinear,
		PartySize: 4,
		TargetCR:  2,
		Seed:      12345, // Same seed as before for reproducibility
	})

	s.Require().NoError(err)
	s.Require().NotNil(output)

	fmt.Println("\n========== PLACEMENT VALIDATION ==========")

	for i, room := range output.Dungeon.Rooms {
		fmt.Printf("\n=== Room %d (ID: %s, Type: %v) ===\n", i, room.ID, getRoomType(room, output.Dungeon))
		fmt.Printf("Shape: %dx%d\n", room.Shape.Width, room.Shape.Height)

		// Build wall position set
		wallPositions := buildWallPositionSet(room.Walls)
		fmt.Printf("Total wall positions: %d\n", len(wallPositions))

		// Print internal walls (not perimeter)
		fmt.Printf("\nInternal walls (pillars, etc.):\n")
		for _, wall := range room.Walls {
			if !isPerimeterWall(wall.ID) && wall.BlocksMovement {
				fmt.Printf("  %s: (%d,%d,%d) -> (%d,%d,%d)\n",
					wall.ID, wall.Start.X, wall.Start.Y, wall.Start.Z,
					wall.End.X, wall.End.Y, wall.End.Z)
			}
		}

		// Print spawn zones
		fmt.Printf("\nSpawn zones:\n")
		for _, zone := range room.SpawnZones {
			fmt.Printf("  Zone %s (type=%d, cap=%d): %d positions\n",
				zone.ID, zone.Type, zone.Capacity, len(zone.Bounds))
			for _, pos := range zone.Bounds {
				key := fmt.Sprintf("%d,%d,%d", pos.X, pos.Y, pos.Z)
				onWall := wallPositions[key]
				status := "OK"
				if onWall {
					status = "ON WALL!"
				}
				fmt.Printf("    (%d,%d,%d) - %s\n", pos.X, pos.Y, pos.Z, status)
			}
		}

		// Simulate monster spawning
		if room.Encounter != nil && len(room.Encounter.Monsters) > 0 {
			fmt.Printf("\nSimulating monster placement:\n")
			fmt.Printf("  Monsters to place: %d\n", len(room.Encounter.Monsters))

			// Build monster IDs
			monsterIDs := make([]string, 0, len(room.Encounter.Monsters))
			for _, placement := range room.Encounter.Monsters {
				monsterIDs = append(monsterIDs, fmt.Sprintf("monster-%s", placement.ID))
			}

			// Use spawner to place
			sp := spawner.NewSpawner()
			spawnOutput, spawnErr := spawner.SpawnInRoom(sp, &spawner.DungeonSpawnInput{
				Room:              room,
				OccupiedPositions: []dungeon.LocalPosition{}, // No pre-occupied positions
				EntitiesToSpawn:   spawner.CreateMonsterSpawnEntities(monsterIDs),
			})

			if spawnErr != nil {
				fmt.Printf("  ERROR: Spawner failed: %v\n", spawnErr)
			} else {
				fmt.Printf("  Placements: %d, Errors: %d\n",
					len(spawnOutput.Placements), len(spawnOutput.Errors))

				for _, placement := range spawnOutput.Placements {
					key := fmt.Sprintf("%d,%d,%d", placement.Position.X, placement.Position.Y, placement.Position.Z)
					onWall := wallPositions[key]
					status := "OK"
					if onWall {
						status = "ON WALL - BUG!"
					}
					fmt.Printf("    %s -> (%d,%d,%d) - %s\n",
						placement.EntityID, placement.Position.X, placement.Position.Y, placement.Position.Z, status)
				}

				for _, err := range spawnOutput.Errors {
					fmt.Printf("    %s -> ERROR: %s\n", err.EntityID, err.Reason)
				}

				// Check if any placement is on a wall
				for _, placement := range spawnOutput.Placements {
					key := fmt.Sprintf("%d,%d,%d", placement.Position.X, placement.Position.Y, placement.Position.Z)
					s.False(wallPositions[key], "Monster %s placed on wall at %s", placement.EntityID, key)
				}
			}
		}
	}

	fmt.Println("\n========== END VALIDATION ==========")
}

func getRoomType(room *dungeon.Room, d *dungeon.Dungeon) string {
	if room.ID == d.StartRoom {
		return "Entrance"
	}
	if room.ID == d.BossRoom {
		return "Boss"
	}
	return "Normal"
}

func buildWallPositionSet(walls []dungeon.WallSegment) map[string]bool {
	positions := make(map[string]bool)
	for _, wall := range walls {
		if !wall.BlocksMovement {
			continue
		}
		wallPos := getWallPositions(wall)
		for _, pos := range wallPos {
			key := fmt.Sprintf("%d,%d,%d", pos.X, pos.Y, pos.Z)
			positions[key] = true
		}
	}
	return positions
}

func isPerimeterWall(id string) bool {
	return len(id) >= 9 && id[:9] == "perimeter"
}
