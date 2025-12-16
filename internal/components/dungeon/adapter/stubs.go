package adapter

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
)

// StubLayoutGenerator returns a simple linear layout
type StubLayoutGenerator struct{}

// NewStubLayoutGenerator creates a new stub layout generator
func NewStubLayoutGenerator() dungeon.LayoutGenerator {
	return &StubLayoutGenerator{}
}

// Generate creates a simple linear layout with entrance, regular rooms, and boss
func (s *StubLayoutGenerator) Generate(ctx context.Context, input *dungeon.LayoutInput) (*dungeon.LayoutOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}

	slots := make([]dungeon.RoomSlot, input.Length)
	connections := make([]*dungeon.LayoutConnection, 0, input.Length-1)

	for i := 0; i < input.Length; i++ {
		var roomType string
		switch i {
		case 0:
			roomType = "entrance"
		case input.Length - 1:
			roomType = "boss"
		default:
			roomType = "regular"
		}

		slots[i] = dungeon.RoomSlot{
			ID:       uuid.New().String(),
			RoomType: roomType,
		}

		// Connect to previous room
		if i > 0 {
			connections = append(connections, &dungeon.LayoutConnection{
				FromRoom:     i - 1,
				ToRoom:       i,
				Type:         dungeon.ConnectionTypeDoor,
				IsMainPath:   true,
				PhysicalHint: "north door",
			})
		}
	}

	return &dungeon.LayoutOutput{
		RoomSlots:   slots,
		Connections: connections,
		StartRoom:   0,
		BossRoom:    input.Length - 1,
	}, nil
}

// StubShapeGenerator returns basic rectangular shapes
type StubShapeGenerator struct{}

// NewStubShapeGenerator creates a new stub shape generator
func NewStubShapeGenerator() dungeon.ShapeGenerator {
	return &StubShapeGenerator{}
}

// Generate creates a simple rectangular shape based on size
func (s *StubShapeGenerator) Generate(ctx context.Context, input *dungeon.ShapeInput) (*dungeon.ShapeOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}

	// Determine dimensions based on size
	var width, height int
	switch input.Size {
	case dungeon.RoomSizeSmall:
		width, height = 10, 10
	case dungeon.RoomSizeMedium:
		width, height = 20, 15
	case dungeon.RoomSizeLarge:
		width, height = 30, 25
	default:
		width, height = 20, 15
	}

	// Create rectangular bounds (4 corners)
	bounds := []dungeon.Position{
		{X: 0, Y: 0, Z: 0},
		{X: width, Y: 0, Z: 0},
		{X: width, Y: height, Z: 0},
		{X: 0, Y: height, Z: 0},
	}

	return &dungeon.ShapeOutput{
		Shape: &dungeon.Shape{
			Bounds:   bounds,
			GridType: dungeon.GridTypeSquare,
			Width:    width,
			Height:   height,
			Area:     width * height,
		},
	}, nil
}

// StubFeatureGenerator returns minimal features with spawn zones
type StubFeatureGenerator struct{}

// NewStubFeatureGenerator creates a new stub feature generator
func NewStubFeatureGenerator() dungeon.FeatureGenerator {
	return &StubFeatureGenerator{}
}

// Generate creates minimal features with appropriate spawn zones
func (s *StubFeatureGenerator) Generate(ctx context.Context, input *dungeon.FeatureInput) (*dungeon.FeatureOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}

	if input.Shape == nil {
		return nil, fmt.Errorf("shape is required")
	}

	zones := make([]dungeon.Zone, 0, 2)

	// Add spawn zones based on room type
	switch input.RoomType {
	case "entrance":
		// Player spawn zone in entrance
		zones = append(zones, dungeon.Zone{
			ID:   uuid.New().String(),
			Type: dungeon.ZoneTypePlayerSpawn,
			Bounds: []dungeon.Position{
				{X: 1, Y: 1, Z: 0},
				{X: 3, Y: 3, Z: 0},
			},
			Capacity: 4,
		})
	case "boss":
		// Boss zone in boss room
		zones = append(zones, dungeon.Zone{
			ID:   uuid.New().String(),
			Type: dungeon.ZoneTypeBoss,
			Bounds: []dungeon.Position{
				{X: input.Shape.Width - 5, Y: input.Shape.Height - 5, Z: 0},
				{X: input.Shape.Width - 1, Y: input.Shape.Height - 1, Z: 0},
			},
			Capacity: 6,
		})
	default:
		// Monster spawn zone in regular rooms
		zones = append(zones, dungeon.Zone{
			ID:   uuid.New().String(),
			Type: dungeon.ZoneTypeMonsterSpawn,
			Bounds: []dungeon.Position{
				{X: input.Shape.Width/2 - 2, Y: input.Shape.Height/2 - 2, Z: 0},
				{X: input.Shape.Width/2 + 2, Y: input.Shape.Height/2 + 2, Z: 0},
			},
			Capacity: 5,
		})
	}

	return &dungeon.FeatureOutput{
		Features: &dungeon.FeatureLayout{
			Obstacles:  []dungeon.Obstacle{},
			Terrain:    []dungeon.TerrainPatch{},
			SpawnZones: zones,
		},
	}, nil
}
