package spawner

import (
	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
)

// DungeonSpawnInput is a convenience wrapper for spawning entities in a dungeon room.
type DungeonSpawnInput struct {
	// Room is the dungeon room to spawn entities in
	Room *dungeon.Room
	// OccupiedPositions are positions already occupied by other entities, in
	// room-local coordinates (the spawner only operates within a single room).
	OccupiedPositions []dungeon.LocalPosition
	// EntitiesToSpawn are the entities that need placement
	EntitiesToSpawn []EntityToSpawn
}

// SpawnInRoom converts dungeon types and spawns entities using the provided Spawner.
// This is a standalone function to avoid adding dungeon dependencies to the Spawner interface.
func SpawnInRoom(s Spawner, input *DungeonSpawnInput) (*SpawnOutput, error) {
	if input == nil || input.Room == nil {
		return &SpawnOutput{}, nil
	}

	room := input.Room

	// Get room dimensions
	width, height := 10, 10 // defaults
	if room.Shape != nil {
		width = room.Shape.Width
		height = room.Shape.Height
	}

	// Convert spawn zones
	zones := convertZones(room.SpawnZones)

	// Convert walls
	walls := convertWalls(room.Walls)

	// Convert occupied positions
	occupied := convertPositions(input.OccupiedPositions)

	return s.Spawn(&SpawnInput{
		Width:    width,
		Height:   height,
		Walls:    walls,
		Occupied: occupied,
		Zones:    zones,
		Entities: input.EntitiesToSpawn,
	})
}

// convertZones converts dungeon zones to spawner zones.
func convertZones(dungeonZones []*dungeon.Zone) []SpawnZone {
	zones := make([]SpawnZone, 0, len(dungeonZones))
	for _, z := range dungeonZones {
		if z == nil {
			continue
		}
		zones = append(zones, SpawnZone{
			ID:       z.ID,
			Type:     convertZoneType(z.Type),
			Bounds:   convertPositionsFromZone(z.Bounds),
			Capacity: z.Capacity,
		})
	}
	return zones
}

// convertZoneType converts dungeon zone type to spawner zone type.
func convertZoneType(t dungeon.ZoneType) ZoneType {
	switch t {
	case dungeon.ZoneTypePlayerSpawn:
		return ZoneTypePlayerSpawn
	case dungeon.ZoneTypeMonsterSpawn:
		return ZoneTypeMonsterSpawn
	case dungeon.ZoneTypeBoss:
		return ZoneTypeBoss
	default:
		return ZoneTypeMonsterSpawn // Default to monster spawn
	}
}

// convertWalls converts dungeon walls to spawner walls.
func convertWalls(dungeonWalls []dungeon.WallSegment) []WallSegment {
	walls := make([]WallSegment, 0, len(dungeonWalls))
	for _, w := range dungeonWalls {
		walls = append(walls, WallSegment{
			Start: CubePosition{
				X: w.Start.X,
				Y: w.Start.Y,
				Z: w.Start.Z,
			},
			End: CubePosition{
				X: w.End.X,
				Y: w.End.Y,
				Z: w.End.Z,
			},
			BlocksMovement: w.BlocksMovement,
		})
	}
	return walls
}

// convertPositions converts dungeon room-local positions to spawner cube positions.
func convertPositions(positions []dungeon.LocalPosition) []CubePosition {
	result := make([]CubePosition, 0, len(positions))
	for _, p := range positions {
		result = append(result, CubePosition{
			X: p.X,
			Y: p.Y,
			Z: p.Z,
		})
	}
	return result
}

// convertPositionsFromZone converts zone bounds (room-local) to cube positions.
func convertPositionsFromZone(bounds []dungeon.LocalPosition) []CubePosition {
	return convertPositions(bounds)
}

// CreateMonsterSpawnEntities creates EntityToSpawn entries for monsters.
func CreateMonsterSpawnEntities(monsterIDs []string) []EntityToSpawn {
	entities := make([]EntityToSpawn, 0, len(monsterIDs))
	for _, id := range monsterIDs {
		entities = append(entities, EntityToSpawn{
			ID:            id,
			Type:          EntityTypeMonster,
			PreferredZone: ZoneTypeMonsterSpawn,
			Size:          1,
		})
	}
	return entities
}

// CreatePlayerSpawnEntities creates EntityToSpawn entries for players.
func CreatePlayerSpawnEntities(characterIDs []string) []EntityToSpawn {
	entities := make([]EntityToSpawn, 0, len(characterIDs))
	for _, id := range characterIDs {
		entities = append(entities, EntityToSpawn{
			ID:            id,
			Type:          EntityTypePlayer,
			PreferredZone: ZoneTypePlayerSpawn,
			Size:          1,
		})
	}
	return entities
}
