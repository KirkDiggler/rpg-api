// Package spawner provides spatial-aware entity placement for dungeon rooms.
// Designed with zero dependencies on rpg-api for future extraction to rpg-toolkit.
package spawner

// SpawnInput contains all data needed to place entities in a room.
type SpawnInput struct {
	// Width is the room width in grid cells
	Width int
	// Height is the room height in grid cells
	Height int
	// Walls are wall segments that block movement
	Walls []WallSegment
	// Occupied are positions already occupied by entities
	Occupied []CubePosition
	// Zones are spawn zones where entities can be placed
	Zones []SpawnZone
	// Entities are the entities to spawn
	Entities []EntityToSpawn
}

// SpawnOutput contains the results of entity placement.
type SpawnOutput struct {
	// Placements are successfully placed entities
	Placements []EntityPlacement
	// Errors are entities that could not be placed
	Errors []PlacementError
}

// CubePosition represents a position in cube coordinates (x + y + z = 0).
type CubePosition struct {
	X int
	Y int
	Z int
}

// WallSegment represents a wall that blocks movement.
type WallSegment struct {
	// Start is the wall start position
	Start CubePosition
	// End is the wall end position
	End CubePosition
	// BlocksMovement indicates if entities can pass through
	BlocksMovement bool
}

// SpawnZone represents an area where entities can spawn.
type SpawnZone struct {
	// ID uniquely identifies this zone
	ID string
	// Type is the zone purpose
	Type ZoneType
	// Bounds are the positions within this zone
	Bounds []CubePosition
	// Capacity is the maximum entities that can spawn here
	Capacity int
}

// ZoneType defines the purpose of a spawn zone.
type ZoneType string

const (
	// ZoneTypePlayerSpawn is where players enter the room
	ZoneTypePlayerSpawn ZoneType = "player_spawn"
	// ZoneTypeMonsterSpawn is where monsters spawn
	ZoneTypeMonsterSpawn ZoneType = "monster_spawn"
	// ZoneTypeBoss is for boss monsters
	ZoneTypeBoss ZoneType = "boss"
)

// EntityType defines what kind of entity is being spawned.
type EntityType string

const (
	// EntityTypePlayer is a player character
	EntityTypePlayer EntityType = "player"
	// EntityTypeMonster is a monster/NPC
	EntityTypeMonster EntityType = "monster"
	// EntityTypeBoss is a boss monster
	EntityTypeBoss EntityType = "boss"
)

// EntityToSpawn describes an entity that needs to be placed.
type EntityToSpawn struct {
	// ID uniquely identifies this entity
	ID string
	// Type is the kind of entity
	Type EntityType
	// PreferredZone is the zone type this entity prefers
	PreferredZone ZoneType
	// Size is the entity's footprint (1 = single cell)
	Size int
}

// EntityPlacement is a successfully placed entity.
type EntityPlacement struct {
	// EntityID is the placed entity's ID
	EntityID string
	// Position is where the entity was placed
	Position CubePosition
	// ZoneID is which zone the entity was placed in
	ZoneID string
}

// PlacementError describes why an entity could not be placed.
type PlacementError struct {
	// EntityID is the entity that failed to place
	EntityID string
	// Reason describes why placement failed
	Reason string
}
