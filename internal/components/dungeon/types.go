// Package dungeon provides procedural dungeon generation with themed rooms and encounters.
// This component creates multi-room dungeons with CR-scaled monster placements for D&D 5e.
package dungeon

// GenerateInput defines the input parameters for dungeon generation
type GenerateInput struct {
	// Theme determines the visual style and monster pool
	Theme Theme
	// Size controls individual room dimensions
	Size RoomSize
	// Length specifies the number of rooms
	Length int
	// Layout controls room connection pattern
	Layout LayoutType
	// PartySize is the number of players for encounter scaling
	PartySize int
	// TargetCR is the party level for encounter difficulty
	TargetCR int
	// Seed is optional for reproducible generation (0 = random)
	Seed int64
}

// GenerateOutput contains the generated dungeon and metadata
type GenerateOutput struct {
	// Dungeon is the complete generated dungeon
	Dungeon *Dungeon
	// Seed is the actual seed used (for replay)
	Seed int64
}

// Dungeon represents a complete multi-room dungeon
type Dungeon struct {
	// ID is a unique identifier for this dungeon
	ID string
	// Theme defines the visual and gameplay style
	Theme Theme
	// Rooms contains all rooms in the dungeon
	Rooms []*Room
	// Connections defines links between rooms
	Connections []*RoomConnection
	// StartRoom is the ID of the entrance room
	StartRoom string
	// BossRoom is the ID of the final boss room
	BossRoom string
}

// Room represents a single room within a dungeon
type Room struct {
	// ID is a unique identifier for this room
	ID string
	// Shape defines the geometric layout
	Shape *Shape
	// Features contains obstacles, terrain, and zones
	Features *FeatureLayout
	// SpawnZones defines player and monster spawn areas
	SpawnZones []*Zone
	// Walls contains all wall segments (perimeter + internal)
	Walls []WallSegment
	// Encounter contains the monster composition
	Encounter *Encounter
	// Origin is the absolute position of this room in dungeon-space.
	// Use AbsolutePosition: room origins are dungeon-relative, not room-local.
	Origin AbsolutePosition
}

// Shape defines the geometric layout of a room
type Shape struct {
	// Bounds defines the polygon vertices in room-local coordinates.
	Bounds []LocalPosition
	// ConnectionPoints defines valid door/passage locations
	ConnectionPoints []ConnectionPoint
	// GridType specifies hex or square grid
	GridType GridType
	// Width is the number of cells across
	Width int
	// Height is the number of cells down
	Height int
	// Area is the total walkable cells
	Area int
}

// ConnectionPoint defines where a room can connect to others
type ConnectionPoint struct {
	// Name identifies this connection (e.g., "north", "south")
	Name string
	// Position is the grid location of the connection in room-local coordinates.
	Position LocalPosition
	// Direction indicates which wall this is on
	Direction string
	// Type specifies the connection kind (e.g., "door", "passage")
	Type string
}

// Position represents a coordinate in the grid.
//
// New code should use LocalPosition / AbsolutePosition from coords.go and the
// Module bridge in module.go. Position is being phased out across the
// dungeon, orchestrator, and handler layers as part of the coordinate-types
// refactor (rpg-api issue #471). It remains here only while the migration is
// in progress; do not introduce new call sites.
type Position struct {
	X int
	Y int
	Z int
}

// FeatureLayout contains all placed features in a room
type FeatureLayout struct {
	// Obstacles are blocking features like pillars
	Obstacles []Obstacle
	// Terrain contains special terrain patches
	Terrain []TerrainPatch
}

// Obstacle represents a blocking feature in the room
type Obstacle struct {
	// ID is a unique identifier
	ID string
	// Type specifies the kind of obstacle
	Type ObstacleType
	// Position is the grid location in room-local coordinates.
	Position LocalPosition
	// BlocksMovement indicates if entities can pass through
	BlocksMovement bool
	// BlocksLineOfSight indicates if vision is blocked
	BlocksLineOfSight bool
}

// TerrainPatch represents a special terrain area
type TerrainPatch struct {
	// ID is a unique identifier
	ID string
	// Type specifies the terrain kind
	Type TerrainType
	// Bounds defines the area covered in room-local coordinates.
	Bounds []LocalPosition
	// MovementCost is the multiplier for movement
	MovementCost float64
}

// Zone defines a designated area for spawning or special purposes
type Zone struct {
	// ID is a unique identifier
	ID string
	// Type specifies the zone purpose
	Type ZoneType
	// Bounds defines the area in room-local coordinates.
	Bounds []LocalPosition
	// Capacity is the maximum entities that can spawn
	Capacity int
}

// Encounter defines the monster composition in a room
type Encounter struct {
	// Monsters contains all monster placements
	Monsters []MonsterPlacement
	// TotalCR is the sum of all monster CRs
	TotalCR float64
}

// MonsterPlacement represents a single monster in the encounter
type MonsterPlacement struct {
	// ID is a unique identifier for this placement
	ID string
	// MonsterID references the rulebook monster
	MonsterID string
	// Role defines tactical purpose
	Role MonsterRole
	// Position is the spawn location in room-local coordinates.
	Position LocalPosition
	// CR is the challenge rating
	CR float64
}

// RoomConnection defines a link between two rooms
type RoomConnection struct {
	// FromRoom is the source room ID
	FromRoom string
	// ToRoom is the destination room ID
	ToRoom string
	// Type specifies the connection kind
	Type ConnectionType
	// IsMainPath indicates if this is on the critical path
	IsMainPath bool
	// Direction is the cardinal direction from FromRoom (which wall the door is on)
	Direction Direction
	// PhysicalHint describes the connection for players (e.g., "heavy stone door")
	PhysicalHint string
}

// RoomSize defines room dimension categories
type RoomSize int

const (
	// RoomSizeSmall creates 10-15 x 10-15 rooms
	RoomSizeSmall RoomSize = iota
	// RoomSizeMedium creates 15-25 x 15-20 rooms
	RoomSizeMedium
	// RoomSizeLarge creates 25-40 x 20-30 rooms
	RoomSizeLarge
)

// LayoutType defines room connection patterns
type LayoutType int

const (
	// LayoutLinear creates a straight path
	LayoutLinear LayoutType = iota
	// LayoutBranching creates a main path with side rooms
	LayoutBranching
	// LayoutHub creates rooms connected to a central area
	LayoutHub
)

// ShapeStyle defines the geometric style of rooms
type ShapeStyle int

const (
	// ShapeStyleStructured creates rectangular, L, and T shapes
	ShapeStyleStructured ShapeStyle = iota
	// ShapeStyleOrganic creates irregular cave-like shapes
	ShapeStyleOrganic
	// ShapeStyleMixed combines both styles
	ShapeStyleMixed
)

// ZoneType defines the purpose of a spawn zone
type ZoneType int

const (
	// ZoneTypePlayerSpawn is where players enter
	ZoneTypePlayerSpawn ZoneType = iota
	// ZoneTypeMonsterSpawn is where monsters spawn
	ZoneTypeMonsterSpawn
	// ZoneTypeBoss is for boss encounters
	ZoneTypeBoss
	// ZoneTypeEntrance is a room entrance
	ZoneTypeEntrance
	// ZoneTypeExit is a room exit
	ZoneTypeExit
)

// MonsterRole defines tactical purpose in encounters
type MonsterRole int

const (
	// RoleMelee is for close combat monsters
	RoleMelee MonsterRole = iota
	// RoleRanged is for distance attackers
	RoleRanged
	// RoleSupport is for healers and buffers
	RoleSupport
	// RoleBoss is for boss monsters
	RoleBoss
)

// ConnectionType defines how rooms are linked
type ConnectionType int

const (
	// ConnectionTypeDoor is a standard door
	ConnectionTypeDoor ConnectionType = iota
	// ConnectionTypeStairs connects different levels
	ConnectionTypeStairs
	// ConnectionTypePassage is an open passage
	ConnectionTypePassage
)

// Direction represents a cardinal direction for connection placement
type Direction string

const (
	// DirectionNorth places connection on the north (top) wall
	DirectionNorth Direction = "north"
	// DirectionSouth places connection on the south (bottom) wall
	DirectionSouth Direction = "south"
	// DirectionEast places connection on the east (right) wall
	DirectionEast Direction = "east"
	// DirectionWest places connection on the west (left) wall
	DirectionWest Direction = "west"
	// DirectionUp for vertical connections (stairs up)
	DirectionUp Direction = "up"
	// DirectionDown for vertical connections (stairs down)
	DirectionDown Direction = "down"
)

// Opposite returns the opposite direction
func (d Direction) Opposite() Direction {
	switch d {
	case DirectionNorth:
		return DirectionSouth
	case DirectionSouth:
		return DirectionNorth
	case DirectionEast:
		return DirectionWest
	case DirectionWest:
		return DirectionEast
	case DirectionUp:
		return DirectionDown
	case DirectionDown:
		return DirectionUp
	default:
		return ""
	}
}

// GridType defines the grid coordinate system
type GridType int

const (
	// GridTypeHex uses hexagonal tiles
	GridTypeHex GridType = iota
	// GridTypeSquare uses square tiles
	GridTypeSquare
)

// ObstacleType defines kinds of obstacles
type ObstacleType int

const (
	// ObstacleTypePillar is a column
	ObstacleTypePillar ObstacleType = iota
	// ObstacleTypeSarcophagus is a stone coffin
	ObstacleTypeSarcophagus
	// ObstacleTypeAltar is a ritual table
	ObstacleTypeAltar
	// ObstacleTypeBoulder is a large rock
	ObstacleTypeBoulder
	// ObstacleTypeStalagmite is a floor spike
	ObstacleTypeStalagmite
	// ObstacleTypePool is a water feature
	ObstacleTypePool
	// ObstacleTypeCrate is a wooden box
	ObstacleTypeCrate
	// ObstacleTypeBarrel is a container
	ObstacleTypeBarrel
)

// TerrainType defines kinds of special terrain
type TerrainType int

const (
	// TerrainTypeDifficult slows movement
	TerrainTypeDifficult TerrainType = iota
	// TerrainTypeHazardous damages entities
	TerrainTypeHazardous
	// TerrainTypeWater is shallow water
	TerrainTypeWater
	// TerrainTypeLava is molten rock
	TerrainTypeLava
	// TerrainTypeIce is slippery surface
	TerrainTypeIce
)

// PatternType defines tactical wall pattern algorithms
type PatternType string

const (
	// PatternEmpty creates no internal walls
	PatternEmpty PatternType = "empty"
	// PatternSparse creates 1-2 single obstacles for easy navigation
	PatternSparse PatternType = "sparse"
	// PatternCoverClusters creates 2-4 groups of connected walls for tactical movement
	PatternCoverClusters PatternType = "cover_clusters"
	// PatternChokepoints creates wall segments that divide space into sections
	PatternChokepoints PatternType = "chokepoints"
	// PatternCentralFeature creates an obstacle cluster in room center
	PatternCentralFeature PatternType = "central_feature"
	// PatternPerimeterCover creates alcoves and pillars along room edges
	PatternPerimeterCover PatternType = "perimeter_cover"
	// PatternPillarGrid creates evenly spaced pillars for sightline breaks
	PatternPillarGrid PatternType = "pillar_grid"
)

// RoomType defines the purpose of a room for pattern selection
type RoomType string

const (
	// RoomTypeEntrance is the dungeon entry point
	RoomTypeEntrance RoomType = "entrance"
	// RoomTypeChamber is a standard tactical room
	RoomTypeChamber RoomType = "chamber"
	// RoomTypeCorridor is a narrow connecting passage
	RoomTypeCorridor RoomType = "corridor"
	// RoomTypeBoss is the final boss encounter room
	RoomTypeBoss RoomType = "boss"
	// RoomTypeTreasure contains loot and is defensible
	RoomTypeTreasure RoomType = "treasure"
	// RoomTypeTrap is maze-like with limited sightlines
	RoomTypeTrap RoomType = "trap"
)

// DensityRange represents a min/max range for obstacle density
type DensityRange struct {
	Min float64
	Max float64
}

// Common density ranges
var (
	DensityLow    = DensityRange{Min: 0.1, Max: 0.3}
	DensityMedium = DensityRange{Min: 0.3, Max: 0.6}
	DensityHigh   = DensityRange{Min: 0.6, Max: 0.9}
)

// WallType categorizes wall behavior
type WallType int

const (
	// WallTypeIndestructible represents permanent structural walls
	WallTypeIndestructible WallType = iota
	// WallTypeDestructible represents walls that can be destroyed
	WallTypeDestructible
)

// WallSegment represents a wall within a room
type WallSegment struct {
	// ID uniquely identifies this wall
	ID string
	// Start is the wall start position in room-local coordinates.
	Start LocalPosition
	// End is the wall end position in room-local coordinates.
	End LocalPosition
	// Type is the wall behavior type
	Type WallType
	// BlocksMovement indicates if entities can pass through
	BlocksMovement bool
	// BlocksLineOfSight indicates if vision is blocked
	BlocksLineOfSight bool
}

// Theme is defined in theme.go and bundles all generation rules for a cohesive dungeon style
