package engine

import (
	"time"
	
	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
)

// ValidateCharacterDraftInput contains the draft to validate
type ValidateCharacterDraftInput struct {
	Draft *dnd5e.CharacterDraft
}

// ValidateCharacterDraftOutput contains validation results
type ValidateCharacterDraftOutput struct {
	IsComplete   bool
	IsValid      bool
	Errors       []ValidationError
	Warnings     []ValidationWarning
	MissingSteps []string
}

// CalculateCharacterStatsInput contains character data for stat calculation
type CalculateCharacterStatsInput struct {
	Draft *dnd5e.CharacterDraft
}

// CalculateCharacterStatsOutput contains calculated character stats
type CalculateCharacterStatsOutput struct {
	MaxHP            int32
	ArmorClass       int32
	Initiative       int32
	Speed            int32
	ProficiencyBonus int32
	SavingThrows     map[string]int32
	Skills           map[string]int32
}

// ValidateRaceChoiceInput contains race validation data
type ValidateRaceChoiceInput struct {
	RaceID    string
	SubraceID string
}

// ValidateRaceChoiceOutput contains race validation results
type ValidateRaceChoiceOutput struct {
	IsValid     bool
	Errors      []ValidationError
	RaceTraits  []string
	AbilityMods map[string]int32
}

// ValidateClassChoiceInput contains class validation data
type ValidateClassChoiceInput struct {
	ClassID       string
	AbilityScores *dnd5e.AbilityScores
}

// ValidateClassChoiceOutput contains class validation results
type ValidateClassChoiceOutput struct {
	IsValid           bool
	Errors            []ValidationError
	Warnings          []ValidationWarning
	HitDice           string
	PrimaryAbility    string
	SavingThrows      []string
	SkillChoicesCount int32
	AvailableSkills   []string
}

// AbilityScoreMethod represents the method used to generate ability scores
type AbilityScoreMethod string

// Ability score generation methods
const (
	AbilityScoreMethodStandardArray AbilityScoreMethod = "standard_array"
	AbilityScoreMethodPointBuy      AbilityScoreMethod = "point_buy"
	AbilityScoreMethodManual        AbilityScoreMethod = "manual"
)

// ValidateAbilityScoresInput contains ability scores to validate
type ValidateAbilityScoresInput struct {
	AbilityScores *dnd5e.AbilityScores
	Method        AbilityScoreMethod
}

// ValidateAbilityScoresOutput contains ability score validation results
type ValidateAbilityScoresOutput struct {
	IsValid  bool
	Errors   []ValidationError
	Warnings []ValidationWarning
}

// ValidateSkillChoicesInput contains skill choices to validate
type ValidateSkillChoicesInput struct {
	ClassID          string
	BackgroundID     string
	SelectedSkillIDs []string
}

// ValidateSkillChoicesOutput contains skill validation results
type ValidateSkillChoicesOutput struct {
	IsValid  bool
	Errors   []ValidationError
	Warnings []ValidationWarning
}

// GetAvailableSkillsInput contains data to determine available skills
type GetAvailableSkillsInput struct {
	ClassID      string
	BackgroundID string
}

// GetAvailableSkillsOutput contains available skill choices
type GetAvailableSkillsOutput struct {
	ClassSkills      []SkillChoice
	BackgroundSkills []SkillChoice
}

// ValidateBackgroundChoiceInput contains background validation data
type ValidateBackgroundChoiceInput struct {
	BackgroundID string
}

// ValidateBackgroundChoiceOutput contains background validation results
type ValidateBackgroundChoiceOutput struct {
	IsValid            bool
	Errors             []ValidationError
	SkillProficiencies []string
	Languages          int32
	Equipment          []string
}

// ValidationError represents a validation error
type ValidationError struct {
	Field   string
	Message string
	Code    string
}

// ValidationWarning represents a validation warning
type ValidationWarning struct {
	Field   string
	Message string
	Code    string
}

// SkillChoice represents an available skill choice
type SkillChoice struct {
	SkillID     string
	SkillName   string
	Description string
	Ability     string
}

// =============================================================================
// Room Generation Types
// =============================================================================

// GenerateRoomInput contains room generation parameters
type GenerateRoomInput struct {
	Config    RoomConfig `json:"config"`
	Seed      int64      `json:"seed"`                 // Required for reproducibility
	SessionID string     `json:"session_id,omitempty"` // Optional game session context
	TTL       *int32     `json:"ttl,omitempty"`        // Optional TTL override
}

// GenerateRoomOutput contains generated room data
type GenerateRoomOutput struct {
	Room      *RoomData          `json:"room"`
	Entities  []EntityData       `json:"entities"`
	Metadata  GenerationMetadata `json:"metadata"`
	SessionID string             `json:"session_id"`
	ExpiresAt time.Time          `json:"expires_at"`
}

// RoomConfig defines room generation parameters
type RoomConfig struct {
	Width       int32   `json:"width"`         // Room width in grid units
	Height      int32   `json:"height"`        // Room height in grid units
	Theme       string  `json:"theme"`         // "dungeon", "forest", "urban", etc.
	WallDensity float64 `json:"wall_density"`  // 0.0-1.0 wall coverage
	Pattern     string  `json:"pattern"`       // "empty", "random", "clustered"
	GridType    string  `json:"grid_type"`     // "square", "hex_pointy", "hex_flat", "gridless"
	GridSize    float64 `json:"grid_size"`     // 5.0 for D&D 5ft squares
}

// GetRoomDetailsInput contains room lookup parameters
type GetRoomDetailsInput struct {
	RoomID string `json:"room_id"`
}

// GetRoomDetailsOutput contains room details
type GetRoomDetailsOutput struct {
	Room     *RoomData    `json:"room"`
	Entities []EntityData `json:"entities"`
	Metadata RoomMetadata `json:"metadata"`
}

// RoomData represents a generated room
type RoomData struct {
	ID          string                 `json:"id"`
	EntityID    string                 `json:"entity_id"`    // Owner (following entity ownership pattern)
	Config      RoomConfig             `json:"config"`
	Dimensions  Dimensions             `json:"dimensions"`
	GridInfo    GridInformation        `json:"grid_info"`
	Properties  map[string]interface{} `json:"properties"`
	CreatedAt   time.Time              `json:"created_at"`
	ExpiresAt   time.Time              `json:"expires_at"`
}

// EntityData represents an entity within a room
type EntityData struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`        // "wall", "door", "monster", "character"
	Position   Position               `json:"position"`
	Properties map[string]interface{} `json:"properties"`  // Size, material, health, etc.
	Tags       []string               `json:"tags"`        // "destructible", "blocking", "cover"
	State      EntityState            `json:"state"`
}

// Position represents a 3D coordinate
type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z,omitempty"` // Optional for 3D support
}

// EntityState represents entity gameplay state
type EntityState struct {
	BlocksMovement    bool  `json:"blocks_movement"`
	BlocksLineOfSight bool  `json:"blocks_line_of_sight"`
	Destroyed         bool  `json:"destroyed"`
	CurrentHP         int32 `json:"current_hp,omitempty"`
	MaxHP             int32 `json:"max_hp,omitempty"`
}

// Dimensions represents room dimensions
type Dimensions struct {
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
	Depth  float64 `json:"depth,omitempty"`
}

// GridInformation contains grid system details
type GridInformation struct {
	Type string  `json:"type"` // "square", "hex_pointy", "hex_flat", "gridless"
	Size float64 `json:"size"` // Size of each grid cell
}

// GenerationMetadata contains room generation metadata
type GenerationMetadata struct {
	GenerationTime time.Duration `json:"generation_time"`
	SeedUsed       int64         `json:"seed_used"`
	Attempts       int32         `json:"attempts"`
	Version        string        `json:"version"`
}

// RoomMetadata contains extended room metadata
type RoomMetadata struct {
	GenerationMetadata GenerationMetadata `json:"generation_metadata"`
	AccessCount        int32              `json:"access_count"`
	LastAccessed       time.Time          `json:"last_accessed"`
}

// =============================================================================
// Spatial Query Types
// =============================================================================

// QueryLineOfSightInput contains line of sight query parameters
type QueryLineOfSightInput struct {
	RoomID      string   `json:"room_id"`
	FromX       float64  `json:"from_x"`
	FromY       float64  `json:"from_y"`
	ToX         float64  `json:"to_x"`
	ToY         float64  `json:"to_y"`
	EntitySize  float64  `json:"entity_size,omitempty"`  // Size for collision detection
	IgnoreTypes []string `json:"ignore_types,omitempty"` // Entity types to ignore
}

// QueryLineOfSightOutput contains line of sight results
type QueryLineOfSightOutput struct {
	HasLineOfSight   bool       `json:"has_line_of_sight"`
	BlockingEntityID *string    `json:"blocking_entity_id,omitempty"`
	BlockingPosition *Position  `json:"blocking_position,omitempty"`
	Distance         float64    `json:"distance"`        // Actual distance
	PathPositions    []Position `json:"path_positions"`  // LOS ray positions
}

// ValidateMovementInput contains movement validation parameters
type ValidateMovementInput struct {
	RoomID      string  `json:"room_id"`
	EntityID    string  `json:"entity_id"`          // Entity attempting movement
	FromX       float64 `json:"from_x"`
	FromY       float64 `json:"from_y"`
	ToX         float64 `json:"to_x"`
	ToY         float64 `json:"to_y"`
	EntitySize  float64 `json:"entity_size,omitempty"`
	MaxDistance float64 `json:"max_distance,omitempty"`
}

// ValidateMovementOutput contains movement validation results
type ValidateMovementOutput struct {
	IsValid          bool      `json:"is_valid"`
	BlockedBy        *string   `json:"blocked_by,omitempty"`     // Entity ID blocking path
	BlockingPosition *Position `json:"blocking_position,omitempty"`
	MovementCost     float64   `json:"movement_cost"`            // Cost in movement points
	ActualDistance   float64   `json:"actual_distance"`          // Calculated distance
}

// ValidateEntityPlacementInput contains entity placement parameters
type ValidateEntityPlacementInput struct {
	RoomID     string                 `json:"room_id"`
	EntityID   string                 `json:"entity_id,omitempty"`    // For updates
	EntityType string                 `json:"entity_type"`
	Position   Position               `json:"position"`
	Size       float64                `json:"size"`
	Properties map[string]interface{} `json:"properties,omitempty"`
	Tags       []string               `json:"tags,omitempty"`
}

// ValidateEntityPlacementOutput contains placement validation results
type ValidateEntityPlacementOutput struct {
	CanPlace           bool             `json:"can_place"`
	ConflictingIDs     []string         `json:"conflicting_ids"`     // Conflicting entity IDs
	SuggestedPositions []Position       `json:"suggested_positions"` // Alternative positions
	PlacementIssues    []PlacementIssue `json:"placement_issues"`
}

// PlacementIssue represents an entity placement issue
type PlacementIssue struct {
	Type        string   `json:"type"`        // "collision", "out_of_bounds", "invalid_terrain"
	Description string   `json:"description"`
	Position    Position `json:"position"`
	Severity    string   `json:"severity"`    // "error", "warning", "info"
}

// QueryEntitiesInRangeInput contains range query parameters
type QueryEntitiesInRangeInput struct {
	RoomID          string   `json:"room_id"`
	CenterX         float64  `json:"center_x"`
	CenterY         float64  `json:"center_y"`
	Range           float64  `json:"range"`
	EntityTypes     []string `json:"entity_types,omitempty"`     // Filter by type
	Tags            []string `json:"tags,omitempty"`             // Filter by tags
	ExcludeEntityID string   `json:"exclude_entity_id,omitempty"` // Exclude specific entity
}

// QueryEntitiesInRangeOutput contains range query results
type QueryEntitiesInRangeOutput struct {
	Entities    []EntityResult `json:"entities"`
	TotalFound  int32          `json:"total_found"`
	QueryCenter Position       `json:"query_center"`
	QueryRange  float64        `json:"query_range"`
}

// EntityResult represents an entity found in a range query
type EntityResult struct {
	Entity      EntityData `json:"entity"`
	Distance    float64    `json:"distance"`     // Distance from query center
	Direction   float64    `json:"direction"`    // Angle from center (radians)
	RelativePos string     `json:"relative_pos"` // "north", "southeast", etc.
}
