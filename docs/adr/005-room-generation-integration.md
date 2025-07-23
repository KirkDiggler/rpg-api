# ADR-005: Environment Generation Integration with rpg-toolkit

## Status  
Proposed - **ARCHITECTURAL CORRECTION NEEDED**

## Context

We need to expand rpg-api beyond character creation to support tactical gameplay. The immediate goal is enabling game clients to generate and display tactical environments as a foundation for:

1. **Tactical Combat**: Positioning, movement, line of sight across multiple rooms
2. **Dungeon Exploration**: Multi-room environments with connections and navigation
3. **Interactive Environments**: Wall destruction, environmental hazards
4. **Complex Layouts**: Connected spaces, procedurally generated dungeons

rpg-toolkit provides all the necessary infrastructure:
- **tools/environments**: **PRIMARY INTERFACE** - Complete environment generation, multi-room orchestration, high-level client API
- **tools/spatial**: **LOW-LEVEL MECHANICS** - Individual room mechanics, spatial queries, positioning systems  
- **tools/selectables**: Weighted random selection for environment features
- **tools/spawn**: Entity placement and spawning systems

### **CRITICAL ARCHITECTURAL INSIGHT**
The environments module is the **client-friendly middleware over the spatial module**. It provides:
- `EnvironmentGenerator` for creating entire environments (multiple rooms + connections)
- `Environment` interface that wraps `spatial.RoomOrchestrator` 
- Multi-room queries, pathfinding, and orchestration
- `RoomBuilder` for individual room construction

**Our API should primarily interface with environments, not spatial directly.**

### Success Criteria
> A game client can call a gRPC endpoint and receive environment data containing multiple connected rooms that it can immediately use for tactical gameplay with positioning, line of sight, and room-to-room movement.

### Design Constraints

From ADR-001 and existing patterns:
- **Interface Agnostic**: Works for Discord, web, mobile clients
- **Generic First**: Not D&D-specific, usable by any RPG system  
- **Modular Addition**: Extend existing patterns, don't modify core architecture
- **Repository Pattern**: Support multiple storage backends
- **Input/Output Types**: All functions use structured types, never primitives

## Decision

### 1. Engine Interface Extension

Extend `internal/engine/interface.go` with environment generation methods (using environments as primary interface):

```go  
// === PRIMARY INTERFACE: ENVIRONMENT GENERATION ===
// Environment generation - engine uses environments.EnvironmentGenerator
GenerateEnvironment(ctx context.Context, input *GenerateEnvironmentInput) (*GenerateEnvironmentOutput, error)

// Environment queries - multi-room operations via environments.Environment
QueryEnvironmentEntities(ctx context.Context, input *QueryEnvironmentEntitiesInput) (*QueryEnvironmentEntitiesOutput, error)
FindEnvironmentPath(ctx context.Context, input *FindEnvironmentPathInput) (*FindEnvironmentPathOutput, error)
GetEnvironmentRooms(ctx context.Context, input *GetEnvironmentRoomsInput) (*GetEnvironmentRoomsOutput, error)

// === SECONDARY INTERFACE: INDIVIDUAL ROOM OPERATIONS ===
// Single room generation - for simple use cases, uses environments.RoomBuilder
GenerateRoom(ctx context.Context, input *GenerateRoomInput) (*GenerateRoomOutput, error)

// === SPATIAL QUERY SYSTEM - VIA ENVIRONMENTS AND SPATIAL ===
// Multi-room spatial queries - via environments.Environment
QueryEntitiesInRange(ctx context.Context, input *QueryEntitiesInRangeInput) (*QueryEntitiesInRangeOutput, error)
QueryLineOfSight(ctx context.Context, input *QueryLineOfSightInput) (*QueryLineOfSightOutput, error)
ValidateMovement(ctx context.Context, input *ValidateMovementInput) (*ValidateMovementOutput, error)
ValidateEntityPlacement(ctx context.Context, input *ValidateEntityPlacementInput) (*ValidateEntityPlacementOutput, error)

// === CAPACITY AND FALLBACK ANALYSIS ===
// Environment-level capacity analysis
AnalyzeEnvironmentCapacity(ctx context.Context, input *AnalyzeEnvironmentCapacityInput) (*AnalyzeEnvironmentCapacityOutput, error)
GetGenerationFallbacks(ctx context.Context, input *GetGenerationFallbacksInput) (*GetGenerationFallbacksOutput, error)

// === ENTITY SPAWNING INTEGRATION ===
// Entity spawning system - uses tools/spawn SpawnEngine for intelligent entity placement
PopulateRoom(ctx context.Context, input *PopulateRoomInput) (*PopulateRoomOutput, error)
PopulateEnvironment(ctx context.Context, input *PopulateEnvironmentInput) (*PopulateEnvironmentOutput, error)
PopulateSplitRooms(ctx context.Context, input *PopulateSplitRoomsInput) (*PopulateSplitRoomsOutput, error)

// Spawn configuration and validation
ValidateSpawnConfiguration(ctx context.Context, input *ValidateSpawnConfigurationInput) (*ValidateSpawnConfigurationOutput, error)
GetSpawnRecommendations(ctx context.Context, input *GetSpawnRecommendationsInput) (*GetSpawnRecommendationsOutput, error)

// Entity selection table management (basic spawn registry)
RegisterEntityTable(ctx context.Context, input *RegisterEntityTableInput) (*RegisterEntityTableOutput, error)
GetEntityTables(ctx context.Context, input *GetEntityTablesInput) (*GetEntityTablesOutput, error)

// === SELECTABLES INTEGRATION ===
// Direct access to toolkit's powerful weighted selection system
CreateSelectionTable(ctx context.Context, input *CreateSelectionTableInput) (*CreateSelectionTableOutput, error)
UpdateSelectionTable(ctx context.Context, input *UpdateSelectionTableInput) (*UpdateSelectionTableOutput, error)
DeleteSelectionTable(ctx context.Context, input *DeleteSelectionTableInput) (*DeleteSelectionTableOutput, error)

// Selection operations with rich context support
SelectFromTable(ctx context.Context, input *SelectFromTableInput) (*SelectFromTableOutput, error)
SelectManyFromTable(ctx context.Context, input *SelectManyFromTableInput) (*SelectManyFromTableOutput, error)
SelectUniqueFromTable(ctx context.Context, input *SelectUniqueFromTableInput) (*SelectUniqueFromTableOutput, error)
SelectVariableFromTable(ctx context.Context, input *SelectVariableFromTableInput) (*SelectVariableFromTableOutput, error)

// Selection table management and analytics
ListSelectionTables(ctx context.Context, input *ListSelectionTablesInput) (*ListSelectionTablesOutput, error)
GetSelectionTableInfo(ctx context.Context, input *GetSelectionTableInfoInput) (*GetSelectionTableInfoOutput, error)
GetSelectionAnalytics(ctx context.Context, input *GetSelectionAnalyticsInput) (*GetSelectionAnalyticsOutput, error)
```

**Detailed Input/Output Types**:
```go
// === PRIMARY ENVIRONMENT GENERATION TYPES ===

// GenerateEnvironmentInput - Parameters for full environment generation
type GenerateEnvironmentInput struct {
    Config      EnvironmentConfig
    Seed        int64   // Required - if 0, engine will generate and return actual seed used
    SessionID   string  // Optional grouping context
    TTL         *int32  // Optional TTL in seconds, default 3600
}

type EnvironmentConfig struct {
    RoomCount       int32                    // Number of rooms to generate
    LayoutType      EnvironmentLayoutType    // Layout pattern for connections
    Theme           string                   // Overall environment theme
    GenerationType  GenerationType           // Graph, Prefab, or Hybrid
    Constraints     []GenerationConstraint   // Size, complexity limits
    RoomConfigs     []RoomConfig             // Optional specific room configurations
}

type EnvironmentLayoutType int32
const (
    LayoutTypeOrganic    EnvironmentLayoutType = 0  // Natural/irregular connections
    LayoutTypeLinear     EnvironmentLayoutType = 1  // Sequential room chain
    LayoutTypeBranching  EnvironmentLayoutType = 2  // Hub and spoke
    LayoutTypeGrid       EnvironmentLayoutType = 3  // Grid-based layout
    LayoutTypeTower      EnvironmentLayoutType = 4  // Vertical stacking
)

type GenerationType int32
const (
    GenerationTypeGraph  GenerationType = 0  // Graph-based generation
    GenerationTypePrefab GenerationType = 1  // Prefab-based generation  
    GenerationTypeHybrid GenerationType = 2  // Hybrid approach
)

type GenerationConstraint struct {
    Type        string  // "max_rooms", "max_size", "complexity_limit"
    Value       int32
    Description string
}

type GenerateEnvironmentOutput struct {
    Environment *EnvironmentData
    Rooms       []RoomData
    Connections []ConnectionData
    Metadata    EnvironmentGenerationMetadata
    ExpiresAt   time.Time
}

type EnvironmentData struct {
    ID            string
    Name          string  
    Theme         string
    LayoutType    EnvironmentLayoutType
    RoomCount     int32
    Properties    map[string]string  // Flexible key-value properties
    CreatedAt     time.Time
    SessionID     string
}

type ConnectionData struct {
    ID           string
    Type         ConnectionType    // Door, Stairs, Passage, etc.
    FromRoomID   string
    ToRoomID     string
    FromPosition PositionData
    ToPosition   PositionData  
    Properties   map[string]string
    Bidirectional bool
}

type ConnectionType int32
const (
    ConnectionTypeDoor    ConnectionType = 0
    ConnectionTypeStairs  ConnectionType = 1  
    ConnectionTypePassage ConnectionType = 2
    ConnectionTypePortal  ConnectionType = 3
    ConnectionTypeBridge  ConnectionType = 4
    ConnectionTypeTunnel  ConnectionType = 5
)

type EnvironmentGenerationMetadata struct {
    Seed              int64                    // Master seed used for environment generation
    RoomSeeds         map[string]int64         // Per-room seeds for debugging/reproduction
    GenerationTimeMS  int32
    RoomCount         int32
    ConnectionCount   int32
    ToolkitVersion    string
    LayoutComplexity  float64                  // 0.0 to 1.0, complexity score
    GenerationType    GenerationType
}

// === SECONDARY ROOM GENERATION TYPES (for single room use cases) ===

// GenerateRoomInput - Parameters for individual room generation  
type GenerateRoomInput struct {
    Config    RoomConfig
    Seed      int64   // Required - if 0, engine will generate and return actual seed used
    SessionID string  // Optional grouping context
    TTL       *int32  // Optional TTL in seconds, default 3600
}

// Validate ensures all required fields are present and valid
func (input *GenerateRoomInput) Validate() error {
    if input.Config.Width <= 0 || input.Config.Width > 100 {
        return errors.InvalidArgument("width must be between 1 and 100")
    }
    if input.Config.Height <= 0 || input.Config.Height > 100 {
        return errors.InvalidArgument("height must be between 1 and 100")
    }
    if input.Config.GridType == "" {
        return errors.InvalidArgument("grid_type is required")
    }
    if !isValidGridType(input.Config.GridType) {
        return errors.InvalidArgumentf("invalid grid_type: %s", input.Config.GridType)
    }
    if input.Config.WallConfig.Density < 0.0 || input.Config.WallConfig.Density > 1.0 {
        return errors.InvalidArgument("wall density must be between 0.0 and 1.0")
    }
    if input.Config.WallConfig.DestructibleRatio < 0.0 || input.Config.WallConfig.DestructibleRatio > 1.0 {
        return errors.InvalidArgument("destructible ratio must be between 0.0 and 1.0")
    }
    return nil
}

type GenerateRoomOutput struct {
    Room      *RoomData
    Entities  []EntityData
    Metadata  GenerationMetadata
    ExpiresAt time.Time
}

type RoomConfig struct {
    Width      int32
    Height     int32
    GridType   GridType      // Enum: Square, Hex, Gridless
    Theme      string        // Free-form theme identifier
    WallConfig WallConfig
    Name       string        // Optional display name
}

type WallConfig struct {
    Pattern            WallPattern // Enum: Empty, Random
    Density            float64     // 0.0 to 1.0
    DestructibleRatio  float64     // 0.0 to 1.0  
    Material           string      // "stone", "wood", "metal", etc.
    Height             float64     // Wall height in grid units
}

// GridType enumeration matching proto
type GridType int32
const (
    GridTypeUnspecified GridType = 0
    GridTypeSquare     GridType = 1
    GridTypeHex        GridType = 2
    GridTypeGridless   GridType = 3
)

// WallPattern enumeration matching proto
type WallPattern int32
const (
    WallPatternUnspecified WallPattern = 0
    WallPatternEmpty      WallPattern = 1
    WallPatternRandom     WallPattern = 2
)

// RoomData represents a generated room
type RoomData struct {
    ID         string
    Name       string
    Width      int32
    Height     int32
    GridType   GridType
    Theme      string
    Seed       int64                      // Seed used for this specific room generation
    Properties map[string]string          // Flexible key-value properties
    CreatedAt  time.Time
    SessionID  string
}

// EntityData represents an entity within a room
type EntityData struct {
    ID         string
    Type       EntityType
    Position   PositionData
    Properties map[string]string
    State      EntityState
}

type EntityType int32
const (
    EntityTypeUnspecified EntityType = 0
    EntityTypeWall       EntityType = 1
    EntityTypeDoor       EntityType = 2
    EntityTypeFeature    EntityType = 3
    EntityTypeSpawnPoint EntityType = 4
)

type EntityState struct {
    BlocksMovement    bool
    BlocksLineOfSight bool
    Destroyed         bool
    CurrentHP         *int32  // Optional for destructible entities
    MaxHP             *int32  // Optional for destructible entities
}

type PositionData struct {
    X float64
    Y float64
}

type GenerationMetadata struct {
    Seed               int64    // Exact seed used for this room generation
    GenerationTimeMS   int32
    EntityCount        int32
    ToolkitVersion     string
}

// GetRoomPropertiesInput - For querying room capabilities/metadata
type GetRoomPropertiesInput struct {
    GridType    GridType
    Width       int32
    Height      int32
    WallPattern WallPattern
}

type GetRoomPropertiesOutput struct {
    MaxEntities      int32                    // Theoretical max entities for this size
    SupportedThemes  []string                // Themes this configuration supports
    GridProperties   map[string]interface{}  // Grid-specific properties
    Constraints      []string                // Any limitations or warnings
}

// Validation helper functions
func isValidGridType(gridType string) bool {
    validTypes := map[string]bool{
        "square":   true,
        "hex":      true, 
        "gridless": true,
    }
    return validTypes[gridType]
}

func isValidWallPattern(pattern string) bool {
    validPatterns := map[string]bool{
        "empty":  true,
        "random": true,
    }
    return validPatterns[pattern]
}

// === SPATIAL QUERY SYSTEM INPUT/OUTPUT TYPES ===

// QueryEntitiesInRangeInput - Find entities within a radius
type QueryEntitiesInRangeInput struct {
    RoomID       string
    CenterX      float64
    CenterY      float64
    Radius       float64
    EntityFilter *EntityFilter  // Optional filtering
}

type QueryEntitiesInRangeOutput struct {
    Entities []EntityData
    Count    int32
}

type EntityFilter struct {
    EntityTypes    []EntityType  // Filter by entity types
    ExcludeIDs     []string     // Exclude specific entity IDs
    IncludeStates  []string     // Only entities with these states
    ExcludeStates  []string     // Exclude entities with these states
    Properties     map[string]string  // Match specific properties
}

// QueryLineOfSightInput - Check line of sight between two positions
type QueryLineOfSightInput struct {
    RoomID      string
    FromX       float64
    FromY       float64
    ToX         float64
    ToY         float64
    IgnoreIDs   []string  // Entity IDs to ignore during LOS check
}

type QueryLineOfSightOutput struct {
    HasLineOfSight    bool
    BlockingEntityID  *string    // ID of entity blocking LOS (if any)
    PathPositions     []PositionData  // Positions along the LOS path
    Distance          float64
}

// ValidateMovementInput - Check if movement is valid
type ValidateMovementInput struct {
    RoomID        string
    EntityID      string    // Entity attempting to move
    FromX         float64
    FromY         float64
    ToX           float64
    ToY           float64
    CheckPath     bool      // Whether to validate entire path or just destination
}

type ValidateMovementOutput struct {
    IsValid           bool
    BlockedBy         *string   // ID of blocking entity (if any)
    MaxValidPosition  *PositionData  // Furthest valid position along path
    MovementCost      float64   // Grid-based movement cost
    Warnings          []string  // Non-blocking warnings
}

// ValidateEntityPlacementInput - Check if entity can be placed at position
type ValidateEntityPlacementInput struct {
    RoomID       string
    EntityID     string    // Entity to place
    X            float64
    Y            float64
    EntitySize   int32     // Size of entity (1 = 1 grid square)
    ForceCheck   bool      // Check even if entity already placed
}

type ValidateEntityPlacementOutput struct {
    CanPlace          bool
    ConflictingIDs    []string  // Entity IDs that would conflict
    AlternativePos    []PositionData  // Suggested nearby positions
    Reasons           []string  // Reasons why placement failed
}

// === CAPACITY AND FALLBACK ANALYSIS TYPES ===

// AnalyzeRoomCapacityInput - Analyze room's capacity for entities/features
type AnalyzeRoomCapacityInput struct {
    RoomConfig    RoomConfig
    EntityTypes   []EntityType  // Types of entities to analyze capacity for
    DesiredCount  int32        // Desired number of entities
}

type AnalyzeRoomCapacityOutput struct {
    MaxCapacity       int32                    // Theoretical maximum entities
    RecommendedCount  int32                    // Recommended entity count
    CapacityByType    map[string]int32         // Capacity breakdown by entity type
    DensityAnalysis   RoomDensityAnalysis
    Warnings          []string                 // Capacity warnings
    Recommendations   []CapacityRecommendation
}

type RoomDensityAnalysis struct {
    CurrentDensity    float64  // 0.0 to 1.0, current space utilization
    OptimalDensity    float64  // 0.0 to 1.0, recommended density
    CrowdingRisk      string   // "low", "medium", "high"
    PlayabilityScore  float64  // 0.0 to 1.0, gameplay quality estimate
}

type CapacityRecommendation struct {
    Type        string  // "increase_size", "reduce_walls", "split_room"
    Description string
    Impact      string  // Expected impact of recommendation
}

// GetGenerationFallbacksInput - Get fallback options when generation fails
type GetGenerationFallbacksInput struct {
    FailedConfig    RoomConfig
    FailureReason   string      // Why original generation failed
    Constraints     []string    // Hard constraints that cannot be changed
}

type GetGenerationFallbacksOutput struct {
    FallbackConfigs   []FallbackConfig
    EmergencyConfig   *RoomConfig    // Last resort configuration
    CanRecover        bool
    RecoveryStrategy  string         // Explanation of recovery approach
}

type FallbackConfig struct {
    Config          RoomConfig
    Modifications   []string    // What was changed from original
    QualityScore    float64     // 0.0 to 1.0, expected quality
    ReasonForFallback string    // Why this fallback is suggested
    Priority        int32       // 1 = highest priority
}

// === ENTITY SPAWNING INPUT/OUTPUT TYPES ===

// PopulateRoomInput - Spawn entities in a single room
type PopulateRoomInput struct {
    RoomID      string      // Target room for spawning
    SpawnConfig SpawnConfig // Complete spawn configuration
    SessionID   string      // Optional session grouping
}

type PopulateRoomOutput struct {
    Success              bool                 // Overall operation success
    SpawnedEntities      []SpawnedEntityData  // Successfully placed entities
    Failures             []SpawnFailureData   // Failed placements with reasons
    RoomModifications    []RoomModification   // Room scaling/adaptation changes
    SplitRecommendations []RoomSplitData      // Room splitting suggestions
    Metadata             SpawnMetadata        // Operation metadata
}

// PopulateEnvironmentInput - Spawn entities across connected rooms
type PopulateEnvironmentInput struct {
    EnvironmentID string      // Target environment 
    SpawnConfig   SpawnConfig // Spawn configuration (distributed across rooms)
    SessionID     string      // Optional session grouping
}

type PopulateEnvironmentOutput struct {
    Success              bool                           // Overall operation success
    SpawnedEntities      []SpawnedEntityData            // All spawned entities across rooms
    Failures             []SpawnFailureData             // Failed placements with reasons
    RoomModifications    []RoomModification             // Room scaling/adaptation changes
    SplitRecommendations []RoomSplitData                // Room splitting suggestions
    RoomDistribution     map[string][]SpawnedEntityData // Entities by room ID
    Metadata             SpawnMetadata                  // Operation metadata
}

// PopulateSplitRoomsInput - Spawn entities across specific connected rooms
type PopulateSplitRoomsInput struct {
    RoomIDs     []string    // Connected room IDs
    SpawnConfig SpawnConfig // Spawn configuration (distributed across rooms)
    SessionID   string      // Optional session grouping
}

type PopulateSplitRoomsOutput struct {
    Success              bool                           // Overall operation success
    SpawnedEntities      []SpawnedEntityData            // All spawned entities across rooms
    Failures             []SpawnFailureData             // Failed placements with reasons
    RoomModifications    []RoomModification             // Room scaling/adaptation changes
    SplitRecommendations []RoomSplitData                // Additional room splitting suggestions
    RoomDistribution     map[string][]SpawnedEntityData // Entities by room ID
    Metadata             SpawnMetadata                  // Operation metadata
}

// ValidateSpawnConfigurationInput - Validate spawn setup before execution
type ValidateSpawnConfigurationInput struct {
    SpawnConfig SpawnConfig // Configuration to validate
    RoomID      *string     // Optional specific room context
    RoomIDs     []string    // Optional multi-room context
}

type ValidateSpawnConfigurationOutput struct {
    IsValid        bool                        // Whether configuration is valid
    ValidationErrors []ValidationError         // Specific validation failures
    Warnings       []ValidationWarning        // Non-critical issues
    Recommendations []SpawnRecommendation     // Optimization suggestions
    EstimatedResults SpawnEstimate            // Predicted spawn outcomes
}

// GetSpawnRecommendationsInput - Get AI recommendations for spawn configuration
type GetSpawnRecommendationsInput struct {
    RoomID          *string            // Single room context
    RoomIDs         []string           // Multi-room context  
    DesiredOutcome  SpawnObjective     // What the spawn should achieve
    Constraints     []SpawnConstraint  // Hard limits and requirements
    GameContext     GameContextData    // Additional game state information
}

type GetSpawnRecommendationsOutput struct {
    Recommendations []SpawnConfigRecommendation // Suggested spawn configurations
    Alternatives    []SpawnConfigRecommendation // Alternative approaches
    Warnings        []string                    // Potential issues
    Metadata        RecommendationMetadata      // Analysis details
}

// RegisterEntityTableInput - Register entity selection table
type RegisterEntityTableInput struct {
    TableID   string              // Unique identifier for table
    Entities  []EntityDefinition  // Available entities for selection
    Weights   map[string]float64  // Optional selection weights by entity ID
    SessionID *string             // Optional session scoping
}

type RegisterEntityTableOutput struct {
    TableID     string // Confirmed table ID
    EntityCount int32  // Number of entities registered
    Success     bool   // Registration success status
}

// GetEntityTablesInput - Retrieve available entity tables
type GetEntityTablesInput struct {
    SessionID *string  // Optional session filtering
    TableIDs  []string // Optional specific table IDs to retrieve
}

type GetEntityTablesOutput struct {
    Tables   []EntityTableInfo // Available entity tables
    Metadata TableMetadata     // Additional table information
}

// === SPAWN CONFIGURATION TYPES ===

// SpawnConfig - Complete spawn operation configuration
type SpawnConfig struct {
    EntityGroups        []EntityGroup      // What entities to spawn
    Pattern             SpawnPattern       // How to arrange entities
    TeamConfiguration   *TeamConfig        // Team-based spawn rules
    SpatialRules        SpatialConstraints // Positioning constraints
    Placement           PlacementRules     // General placement rules
    Strategy            SpawnStrategy      // Spawn algorithm approach
    AdaptiveScaling     *ScalingConfig     // Room scaling behavior
    PlayerSpawnZones    []SpawnZone        // Player choice areas
    PlayerChoices       []PlayerSpawnChoice // Player-selected positions
}

// EntityGroup - Defines a group of entities to spawn
type EntityGroup struct {
    ID             string       // Unique group identifier
    Type           string       // Entity category (enemy, ally, treasure, etc.)
    SelectionTable string       // Entity selection table ID
    Quantity       QuantitySpec // How many entities to spawn
}

// QuantitySpec - Flexible quantity specification
type QuantitySpec struct {
    Fixed    *int32  // Exact count
    DiceRoll *string // Future: "2d6+1" dice notation
    Min      *int32  // Future: range minimum
    Max      *int32  // Future: range maximum
}

// SpawnPattern - Available spawn arrangement patterns
type SpawnPattern int32
const (
    PatternScattered    SpawnPattern = 0 // Random distribution
    PatternFormation    SpawnPattern = 1 // Geometric arrangements
    PatternTeamBased    SpawnPattern = 2 // Team separation
    PatternPlayerChoice SpawnPattern = 3 // Player-selected positions
    PatternClustered    SpawnPattern = 4 // Grouped placement
)

// SpatialConstraints - Fine-grained positioning control
type SpatialConstraints struct {
    MinDistance   map[string]float64 // "type1:type2" -> required distance
    LineOfSight   LineOfSightRules   // Visibility requirements
    WallProximity float64            // Distance from walls
    AreaOfEffect  map[string]float64 // Exclusion zone radiuses
    PathingRules  PathingConstraints // Movement accessibility rules
}

// LineOfSightRules - Visibility requirements and restrictions
type LineOfSightRules struct {
    RequiredSight []EntityPair // Entities that MUST see each other
    BlockedSight  []EntityPair // Entities that must NOT see each other
}

// EntityPair - Reference to two entity types
type EntityPair struct {
    From string // Source entity type
    To   string // Target entity type
}

// === SPAWN RESULT TYPES ===

// SpawnedEntityData - Successfully placed entity information
type SpawnedEntityData struct {
    EntityID   string     // Unique entity identifier
    EntityType string     // Entity category
    Position   Position   // Final placement position
    RoomID     string     // Room where entity was placed
    GroupID    string     // Entity group that spawned this entity
    Properties EntityProperties // Entity-specific properties
}

// SpawnFailureData - Failed spawn attempt details
type SpawnFailureData struct {
    EntityType string // Failed entity type
    GroupID    string // Entity group that failed
    Reason     string // Specific failure reason
    AttemptedPositions []Position // Positions that were tried
}

// RoomModification - Changes made to rooms during spawning
type RoomModification struct {
    Type     string      // "scaled", "rotated", "split", etc.
    RoomID   string      // Affected room identifier
    OldValue interface{} // Previous value (dimensions, etc.)
    NewValue interface{} // New value after modification
    Reason   string      // Justification for modification
}

// RoomSplitData - Room splitting recommendation
type RoomSplitData struct {
    OriginalRoomID string          // Room that should be split
    SuggestedSplits []RoomSplitPlan // Proposed split configurations
    Reason         string          // Why splitting is recommended
    Priority       int32           // Urgency of split (1 = highest)
}

// SpawnMetadata - Operation metadata and statistics
type SpawnMetadata struct {
    TotalAttempts     int32   // Total placement attempts made
    SuccessRate       float64 // Percentage of successful placements
    AverageAttempts   float64 // Average attempts per successful placement
    ProcessingTimeMS  int32   // Time taken for spawn operation
    ConstraintViolations int32 // Number of constraint violations encountered
    RoomsModified     int32   // Number of rooms that were modified
}

// === SELECTABLES INPUT/OUTPUT TYPES ===

// CreateSelectionTableInput - Create a new weighted selection table
type CreateSelectionTableInput struct {
    TableID      string              // Unique table identifier
    Name         string              // Human-readable table name
    Description  string              // Table description/purpose
    ItemType     string              // Type of items in table (loot, quests, events, etc.)
    Items        []SelectableItem    // Initial items with weights
    Configuration TableConfiguration // Table behavior settings
    SessionID    *string             // Optional session scoping
}

type CreateSelectionTableOutput struct {
    TableID      string // Confirmed table identifier
    ItemCount    int32  // Number of items in table
    TotalWeight  int32  // Sum of all item weights
    Success      bool   // Creation success status
}

// UpdateSelectionTableInput - Modify existing selection table
type UpdateSelectionTableInput struct {
    TableID       string              // Table to update
    AddItems      []SelectableItem    // Items to add
    RemoveItems   []string            // Item IDs to remove
    UpdateItems   []SelectableItem    // Items to update (by ID)
    Configuration *TableConfiguration // Optional configuration updates
}

type UpdateSelectionTableOutput struct {
    TableID      string // Updated table identifier  
    ItemCount    int32  // New item count
    TotalWeight  int32  // New total weight
    Success      bool   // Update success status
}

// SelectFromTableInput - Single selection from table
type SelectFromTableInput struct {
    TableID         string           // Table to select from
    SelectionContext SelectionContext // Game state context for weighted selection
    Options         SelectionOptions  // Selection behavior options
}

type SelectFromTableOutput struct {
    SelectedItem     interface{}           // The selected item
    SelectionWeight  float64               // Weight of selected item at time of selection
    RollResult       int32                 // Random roll that determined selection
    Alternatives     map[string]float64    // What could have been selected (item -> weight)
    Metadata         SelectionMetadata     // Rich selection analytics
}

// SelectManyFromTableInput - Multiple selections from table
type SelectManyFromTableInput struct {
    TableID         string           // Table to select from
    Count           int32            // Number of selections to make
    SelectionContext SelectionContext // Game state context
    Options         SelectionOptions  // Selection behavior options
}

type SelectManyFromTableOutput struct {
    SelectedItems    []interface{}         // All selected items
    SelectionWeights []float64             // Weight of each selected item
    RollResults      []int32               // Random rolls for each selection
    Alternatives     map[string]float64    // Alternative selections available
    Metadata         SelectionMetadata     // Operation analytics
}

// SelectUniqueFromTableInput - Multiple unique selections from table  
type SelectUniqueFromTableInput struct {
    TableID         string           // Table to select from
    Count           int32            // Number of unique selections
    SelectionContext SelectionContext // Game state context
    Options         SelectionOptions  // Selection behavior options
}

type SelectUniqueFromTableOutput struct {
    SelectedItems    []interface{}         // Unique selected items
    SelectionWeights []float64             // Weight of each selected item
    RollResults      []int32               // Random rolls for selections
    Alternatives     map[string]float64    // Items that could have been selected
    RemainingItems   []string              // Items still available after selection
    Metadata         SelectionMetadata     // Operation analytics
}

// SelectVariableFromTableInput - Variable quantity selection using dice
type SelectVariableFromTableInput struct {
    TableID         string           // Table to select from  
    DiceExpression  string           // Dice expression (e.g., "1d4+1", "2d6")
    SelectionContext SelectionContext // Game state context
    Options         SelectionOptions  // Selection behavior options
}

type SelectVariableFromTableOutput struct {
    SelectedItems    []interface{}         // Selected items (quantity determined by dice)
    DiceResult       int32                 // Dice roll result that determined quantity
    SelectionWeights []float64             // Weight of each selected item
    RollResults      []int32               // Random rolls for each selection
    Alternatives     map[string]float64    // Alternative selections available
    Metadata         SelectionMetadata     // Operation analytics
}

// ListSelectionTablesInput - Retrieve available selection tables
type ListSelectionTablesInput struct {
    SessionID    *string  // Optional session filtering
    ItemType     *string  // Optional item type filtering
    NamePattern  *string  // Optional name pattern matching
    Limit        *int32   // Optional result limit
    Offset       *int32   // Optional pagination offset
}

type ListSelectionTablesOutput struct {
    Tables      []SelectionTableInfo // Available selection tables
    TotalCount  int32                // Total tables matching filter
    HasMore     bool                 // Whether more results available
    Metadata    TableListMetadata    // Additional listing information
}

// GetSelectionTableInfoInput - Get detailed table information
type GetSelectionTableInfoInput struct {
    TableID     string // Table to get information about
    IncludeItems bool  // Whether to include full item list
}

type GetSelectionTableInfoOutput struct {
    TableID      string              // Table identifier
    Name         string              // Table name
    Description  string              // Table description
    ItemType     string              // Type of items in table
    ItemCount    int32               // Number of items
    TotalWeight  int32               // Sum of item weights
    Items        []SelectableItem    // Full item list (if requested)
    Configuration TableConfiguration // Table configuration
    CreatedAt    time.Time           // Table creation time
    LastUpdated  time.Time           // Last modification time
    UsageStats   TableUsageStats     // Selection statistics
}

// GetSelectionAnalyticsInput - Get selection analytics and statistics
type GetSelectionAnalyticsInput struct {
    TableID     string     // Table to analyze
    TimeRange   *TimeRange // Optional time range for analytics
    Granularity string     // "hourly", "daily", "weekly"
}

type GetSelectionAnalyticsOutput struct {
    TableID        string                 // Analyzed table
    SelectionCount int64                  // Total selections made
    ItemStats      []ItemSelectionStats   // Per-item selection statistics
    TimeSeriesData []SelectionDataPoint   // Time-based selection data
    TopSelections  []TopSelectionItem     // Most frequently selected items
    ContextAnalysis ContextUsageAnalysis   // How context affects selections
    Metadata       AnalyticsMetadata      // Analytics metadata
}

// === SELECTABLES CORE TYPES ===

// SelectableItem - Item that can be selected from table
type SelectableItem struct {
    ID          string                 // Unique item identifier
    Content     interface{}            // The actual item (JSON-serializable)
    Weight      float64                // Base selection weight
    Conditions  []WeightCondition      // Context-based weight modifications
    Metadata    map[string]interface{} // Additional item metadata
    Enabled     bool                   // Whether item is available for selection
}

// SelectionContext - Game state context for weighted selection
type SelectionContext struct {
    PlayerLevel      int32                  // Character level
    Location         string                 // Current game location
    PlayerClass      string                 // Character class
    CompletedQuests  []string               // List of completed quest IDs
    PlayerReputation map[string]int32       // Reputation by faction
    GameState        map[string]interface{} // General game state
    SessionData      map[string]string      // Session-specific data
    Timestamp        time.Time              // Context creation time
}

// WeightCondition - Conditional weight modification based on context
type WeightCondition struct {
    ContextKey   string      // Context key to check (e.g., "player_level")
    Operator     string      // Comparison operator ("eq", "gt", "lt", "gte", "lte", "in")
    Value        interface{} // Value to compare against
    Modifier     WeightModifier // How to modify weight if condition matches
}

// WeightModifier - How to modify item weight
type WeightModifier struct {
    Type   string  // "multiply", "add", "override", "disable"
    Value  float64 // Modifier value
    Reason string  // Human-readable explanation
}

// SelectionOptions - Control selection behavior
type SelectionOptions struct {
    EnableEvents     bool   // Whether to publish selection events
    EnableAnalytics  bool   // Whether to record selection for analytics
    ContextLogging   bool   // Whether to log context state
    ReturnAlternatives bool // Whether to include alternative selections in result
    MaxRetries       int32  // Maximum retry attempts on selection failure
}

// TableConfiguration - Table behavior settings
type TableConfiguration struct {
    EnableCaching    bool   // Cache weight calculations
    EnableEvents     bool   // Publish selection events
    CacheTimeout     int32  // Cache timeout in seconds
    MaxRetries       int32  // Max selection retry attempts
    AnalyticsEnabled bool   // Record selections for analytics
}

// SelectionMetadata - Rich selection operation metadata
type SelectionMetadata struct {
    OperationType     string    // "single", "multiple", "unique", "variable"
    SelectionTimeMS   int32     // Time taken for selection
    ContextHash       string    // Hash of selection context for debugging
    WeightCalculations int32    // Number of weight calculations performed
    CacheHits         int32     // Number of cache hits during selection
    RollAttempts      int32     // Number of random rolls made
    EventsPublished   int32     // Number of events published
}

// SelectionTableInfo - Basic information about a selection table
type SelectionTableInfo struct {
    TableID      string    // Table identifier
    Name         string    // Table name  
    Description  string    // Table description
    ItemType     string    // Type of items
    ItemCount    int32     // Number of items
    TotalWeight  int32     // Sum of weights
    LastUsed     time.Time // Last selection time
    UsageCount   int64     // Total selections made
    CreatedAt    time.Time // Creation timestamp
}

// === ANALYTICS TYPES ===

// TableUsageStats - Usage statistics for a selection table
type TableUsageStats struct {
    TotalSelections   int64     // Total selections from this table
    LastSelection     time.Time // Most recent selection
    AverageWeight     float64   // Average weight of selected items
    MostCommonContext string    // Most frequently used context pattern
    SelectionRate     float64   // Selections per hour (recent activity)
}

// ItemSelectionStats - Statistics for individual items
type ItemSelectionStats struct {
    ItemID        string  // Item identifier
    SelectionCount int64   // Times this item was selected
    SelectionRate  float64 // Selection rate relative to weight
    LastSelected   time.Time // Most recent selection
    AverageWeight  float64 // Average effective weight at selection time
}

// SelectionDataPoint - Time-series selection data
type SelectionDataPoint struct {
    Timestamp      time.Time // Data point timestamp
    SelectionCount int32     // Selections in this time period
    UniqueItems    int32     // Unique items selected
    AverageWeight  float64   // Average weight of selections
}

// TopSelectionItem - Most frequently selected items
type TopSelectionItem struct {
    ItemID        string  // Item identifier
    SelectionCount int64   // Total selections
    Percentage     float64 // Percentage of total selections
    TrendDirection string  // "up", "down", "stable"
}

// ContextUsageAnalysis - How context affects selections
type ContextUsageAnalysis struct {
    MostInfluentialKeys []string               // Context keys that most affect selections
    ContextPatterns     []ContextPattern       // Common context combinations  
    WeightModifications []WeightModificationStat // Most common weight modifications
}

// ContextPattern - Common context value combinations
type ContextPattern struct {
    Pattern       map[string]interface{} // Context key-value pattern
    Frequency     int64                  // How often this pattern occurs
    SelectionBias map[string]float64     // How this pattern biases selections
}

// WeightModificationStat - Statistics about weight modifications
type WeightModificationStat struct {
    ConditionType  string  // Type of condition causing modification
    AverageEffect  float64 // Average weight change
    Frequency      int64   // How often this modification occurs
    ItemsAffected  int32   // Number of different items affected
}
```

### 2. rpgtoolkit Engine Adapter Implementation

Extend `internal/engine/rpgtoolkit/adapter.go` to integrate toolkit modules:

```go
// Add to existing AdapterConfig
type AdapterConfig struct {
    // Existing fields...
    EventBus       events.EventBus
    DiceRoller     dice.Roller
    ExternalClient external.Client
    
    // New room generation dependencies
    RoomOrchestrator    spatial.RoomOrchestrator       // For multi-room scenarios (future)
    ShapeLoader         environments.ShapeLoader       // For loading room shapes
    EnvironmentHandler  environments.QueryHandler      // For capacity analysis
    SpawnEngine         spawn.SpawnEngine              // For entity spawning
    SelectablesRegistry spawn.SelectablesRegistry      // For basic entity selection tables
    
    // Direct selectables integration for rich content generation
    SelectablesTables   selectables.TableRegistry      // For weighted selection tables
    SelectablesFactory  selectables.TableFactory       // For creating new selection tables
}

// Validate checks that all required dependencies are provided
func (c *AdapterConfig) Validate() error {
    // Existing validations...
    
    if c.RoomOrchestrator == nil {
        return errors.InvalidArgument("room orchestrator is required")
    }
    if c.ShapeLoader == nil {
        return errors.InvalidArgument("shape loader is required")
    }
    if c.EnvironmentHandler == nil {
        return errors.InvalidArgument("environment handler is required")
    }
    if c.SpawnEngine == nil {
        return errors.InvalidArgument("spawn engine is required")
    }
    if c.SelectablesRegistry == nil {
        return errors.InvalidArgument("selectables registry is required")
    }
    if c.SelectablesTables == nil {
        return errors.InvalidArgument("selectables tables registry is required")
    }
    if c.SelectablesFactory == nil {
        return errors.InvalidArgument("selectables factory is required")
    }
    return nil
}

// GenerateRoom implements room generation using rpg-toolkit modules
func (a *Adapter) GenerateRoom(ctx context.Context, input *GenerateRoomInput) (*GenerateRoomOutput, error) {
    startTime := time.Now()
    
    // 1. Validate input
    if err := input.Validate(); err != nil {
        return nil, err
    }
    
    // 2. Generate or use provided seed
    seed := generateSeed(input.Seed)
    
    // 3. Create spatial grid based on grid type
    grid, err := a.createSpatialGrid(input.Config)
    if err != nil {
        return nil, errors.Wrap(err, "failed to create spatial grid")
    }
    
    // 4. Create basic room
    roomID := generateRoomID()
    room := spatial.NewBasicRoom(spatial.BasicRoomConfig{
        ID:       roomID,
        Type:     input.Config.Theme,
        Grid:     grid,
        EventBus: a.eventBus,
    })
    
    // 5. Generate room content using environments module
    entities, err := a.generateRoomContent(ctx, room, input.Config, seed)
    if err != nil {
        return nil, errors.Wrap(err, "failed to generate room content")
    }
    
    // 6. Convert spatial entities to API entities
    apiEntities, err := a.convertEntitiesToAPI(entities)
    if err != nil {
        return nil, errors.Wrap(err, "failed to convert entities")
    }
    
    // 7. Build room data
    roomData := &RoomData{
        ID:        roomID,
        Name:      input.Config.Name,
        Width:     input.Config.Width,
        Height:    input.Config.Height,
        GridType:  input.Config.GridType,
        Theme:     input.Config.Theme,
        CreatedAt: time.Now(),
        SessionID: input.SessionID,
        Properties: map[string]string{
            "wall_pattern":         wallPatternToString(input.Config.WallConfig.Pattern),
            "wall_density":        fmt.Sprintf("%.2f", input.Config.WallConfig.Density),
            "destructible_ratio":  fmt.Sprintf("%.2f", input.Config.WallConfig.DestructibleRatio),
            "wall_material":       input.Config.WallConfig.Material,
        },
    }
    
    // 8. Calculate TTL expiration
    ttl := int32(3600) // Default 1 hour
    if input.TTL != nil {
        ttl = *input.TTL
    }
    expiresAt := time.Now().Add(time.Duration(ttl) * time.Second)
    
    // 9. Generate metadata
    metadata := GenerationMetadata{
        Seed:             seed,
        GenerationTimeMS: int32(time.Since(startTime).Milliseconds()),
        EntityCount:      int32(len(apiEntities)),
        ToolkitVersion:   getToolkitVersion(),
    }
    
    return &GenerateRoomOutput{
        Room:      roomData,
        Entities:  apiEntities,
        Metadata:  metadata,
        ExpiresAt: expiresAt,
    }, nil
}

// createSpatialGrid creates appropriate grid type based on configuration
func (a *Adapter) createSpatialGrid(config RoomConfig) (spatial.Grid, error) {
    switch config.GridType {
    case GridTypeSquare:
        return spatial.NewSquareGrid(spatial.SquareGridConfig{
            Width:  int(config.Width),
            Height: int(config.Height),
        }), nil
        
    case GridTypeHex:
        return spatial.NewHexGrid(spatial.HexGridConfig{
            Width:     int(config.Width),
            Height:    int(config.Height),
            PointyTop: true, // Default to pointy-top hexes
        }), nil
        
    case GridTypeGridless:
        return spatial.NewGridlessRoom(spatial.GridlessConfig{
            Width:  float64(config.Width),
            Height: float64(config.Height),
        }), nil
        
    default:
        return nil, errors.InvalidArgumentf("unsupported grid type: %d", config.GridType)
    }
}

// generateRoomContent creates walls and features using environments module
func (a *Adapter) generateRoomContent(ctx context.Context, room spatial.Room, config RoomConfig, seed int64) ([]core.Entity, error) {
    var entities []core.Entity
    
    // Skip wall generation for empty pattern
    if config.WallConfig.Pattern == WallPatternEmpty {
        return entities, nil
    }
    
    // Generate walls using environments module
    if config.WallConfig.Pattern == WallPatternRandom {
        wallEntities, err := a.generateRandomWalls(room, config.WallConfig, seed)
        if err != nil {
            return nil, errors.Wrap(err, "failed to generate walls")
        }
        entities = append(entities, wallEntities...)
    }
    
    return entities, nil
}

// generateRandomWalls creates random wall placement using environments module
func (a *Adapter) generateRandomWalls(room spatial.Room, wallConfig WallConfig, seed int64) ([]core.Entity, error) {
    // Use environments.NewRoomBuilder for wall generation
    builder := environments.NewRoomBuilder(environments.RoomBuilderConfig{
        ID:       room.GetID() + "_builder",
        Type:     "wall_builder",
        EventBus: a.eventBus,
    })
    
    // Configure wall generation
    builtRoom := builder.
        WithSize(int(room.GetGrid().GetDimensions().Width), int(room.GetGrid().GetDimensions().Height)).
        WithWallPattern("random").
        WithWallDensity(wallConfig.Density).
        WithDestructibleRatio(wallConfig.DestructibleRatio).
        WithMaterial(wallConfig.Material).
        WithRandomSeed(seed).
        Build()
    
    // Extract wall entities from built room
    wallEntities := environments.GetWallEntitiesInRoom(builtRoom)
    
    // Place walls in our spatial room
    var entities []core.Entity
    for _, wallEntity := range wallEntities {
        position := spatial.Position{
            X: wallEntity.GetPosition().X,
            Y: wallEntity.GetPosition().Y,
        }
        
        if err := room.PlaceEntity(wallEntity, position); err != nil {
            return nil, errors.Wrapf(err, "failed to place wall entity %s", wallEntity.GetID())
        }
        
        entities = append(entities, wallEntity)
    }
    
    return entities, nil
}

// convertEntitiesToAPI converts spatial entities to API entity format
func (a *Adapter) convertEntitiesToAPI(entities []core.Entity) ([]EntityData, error) {
    var apiEntities []EntityData
    
    for _, entity := range entities {
        // Determine entity type
        entityType := a.determineEntityType(entity)
        
        // Get position (if placeable)
        var position PositionData
        if placeable, ok := entity.(spatial.Placeable); ok {
            // Get position from spatial room - this requires room context
            // For now, use entity properties if available
            if posData, exists := entity.(*environments.WallEntity); exists {
                position = PositionData{
                    X: posData.GetPosition().X,
                    Y: posData.GetPosition().Y,
                }
            }
        }
        
        // Build entity state
        state := a.buildEntityState(entity)
        
        // Extract properties
        properties := a.extractEntityProperties(entity)
        
        apiEntity := EntityData{
            ID:         entity.GetID(),
            Type:       entityType,
            Position:   position,
            Properties: properties,
            State:      state,
        }
        
        apiEntities = append(apiEntities, apiEntity)
    }
    
    return apiEntities, nil
}

// determineEntityType maps toolkit entities to API entity types
func (a *Adapter) determineEntityType(entity core.Entity) EntityType {
    switch entity.GetType() {
    case "wall":
        return EntityTypeWall
    case "door":
        return EntityTypeDoor
    case "feature":
        return EntityTypeFeature
    case "spawn_point":
        return EntityTypeSpawnPoint
    default:
        return EntityTypeUnspecified
    }
}

// buildEntityState extracts spatial properties from toolkit entities
func (a *Adapter) buildEntityState(entity core.Entity) EntityState {
    state := EntityState{}
    
    if placeable, ok := entity.(spatial.Placeable); ok {
        state.BlocksMovement = placeable.BlocksMovement()
        state.BlocksLineOfSight = placeable.BlocksLineOfSight()
    }
    
    // Check for destructible entities (wall entities from environments)
    if wallEntity, ok := entity.(*environments.WallEntity); ok {
        current, max, destroyed := wallEntity.GetHealth()
        state.Destroyed = destroyed
        if max > 0 {
            state.CurrentHP = &current
            state.MaxHP = &max
        }
    }
    
    return state
}

// extractEntityProperties gets entity-specific properties
func (a *Adapter) extractEntityProperties(entity core.Entity) map[string]string {
    properties := make(map[string]string)
    
    // Add common properties
    properties["entity_type"] = entity.GetType()
    
    // Add entity-specific properties
    if wallEntity, ok := entity.(*environments.WallEntity); ok {
        properties["material"] = wallEntity.GetMaterial()
        properties["height"] = fmt.Sprintf("%.1f", wallEntity.GetHeight())
        properties["destructible"] = fmt.Sprintf("%t", wallEntity.IsDestructible())
    }
    
    return properties
}

// Helper functions
func generateSeed(providedSeed int64) int64 {
    if providedSeed != 0 {
        return providedSeed
    }
    return time.Now().UnixNano()
}

func generateRoomID() string {
    return "room_" + generateUUID()
}

func generateUUID() string {
    // Implementation would use proper UUID generation
    return fmt.Sprintf("%d", time.Now().UnixNano())
}

func getToolkitVersion() string {
    // Would get actual toolkit version
    return "0.1.0"
}

func wallPatternToString(pattern WallPattern) string {
    switch pattern {
    case WallPatternEmpty:
        return "empty"
    case WallPatternRandom:
        return "random"
    default:
        return "unspecified"
    }
}

// GetRoomProperties provides room capability information
func (a *Adapter) GetRoomProperties(ctx context.Context, input *GetRoomPropertiesInput) (*GetRoomPropertiesOutput, error) {
    // Calculate theoretical max entities based on grid size
    maxEntities := int32(input.Width * input.Height / 4) // Rough estimate
    
    // Define supported themes
    supportedThemes := []string{"dungeon", "forest", "tavern", "outdoor", "cave", "ruins"}
    
    // Build grid-specific properties
    gridProperties := make(map[string]interface{})
    switch input.GridType {
    case GridTypeSquare:
        gridProperties["neighbors"] = 8
        gridProperties["distance_method"] = "chebyshev"
    case GridTypeHex:
        gridProperties["neighbors"] = 6
        gridProperties["distance_method"] = "cube"
    case GridTypeGridless:
        gridProperties["neighbors"] = 8
        gridProperties["distance_method"] = "euclidean"
    }
    
    // Check for constraints
    var constraints []string
    if input.Width*input.Height > 2500 {
        constraints = append(constraints, "Large rooms may have slower generation times")
    }
    if input.WallPattern == WallPatternRandom && input.Width < 5 {
        constraints = append(constraints, "Random walls may not generate effectively in very small rooms")
    }
    
    return &GetRoomPropertiesOutput{
        MaxEntities:     maxEntities,
        SupportedThemes: supportedThemes,
        GridProperties:  gridProperties,
        Constraints:     constraints,
    }, nil
}

// === SPAWN ENGINE INTEGRATION METHODS ===

// PopulateRoom implements entity spawning in a single room
func (a *Adapter) PopulateRoom(ctx context.Context, input *PopulateRoomInput) (*PopulateRoomOutput, error) {
    startTime := time.Now()
    
    // 1. Validate spawn configuration
    if err := a.validateSpawnConfig(input.SpawnConfig); err != nil {
        return nil, errors.Wrap(err, "invalid spawn configuration")
    }
    
    // 2. Convert API spawn config to toolkit spawn config
    toolkitConfig, err := a.convertToToolkitSpawnConfig(input.SpawnConfig)
    if err != nil {
        return nil, errors.Wrap(err, "failed to convert spawn configuration")
    }
    
    // 3. Execute spawning via toolkit spawn engine
    spawnResult, err := a.spawnEngine.PopulateRoom(ctx, input.RoomID, toolkitConfig)
    if err != nil {
        return nil, errors.Wrap(err, "spawn operation failed")
    }
    
    // 4. Convert toolkit spawn result to API format
    apiResult, err := a.convertSpawnResultToAPI(spawnResult, input.RoomID)
    if err != nil {
        return nil, errors.Wrap(err, "failed to convert spawn result")
    }
    
    // 5. Add operation metadata
    apiResult.Metadata.ProcessingTimeMS = int32(time.Since(startTime).Milliseconds())
    
    return apiResult, nil
}

// PopulateEnvironment implements entity spawning across connected rooms
func (a *Adapter) PopulateEnvironment(ctx context.Context, input *PopulateEnvironmentInput) (*PopulateEnvironmentOutput, error) {
    startTime := time.Now()
    
    // 1. Get environment rooms
    environment, err := a.getEnvironmentByID(input.EnvironmentID)
    if err != nil {
        return nil, errors.Wrap(err, "failed to get environment")
    }
    
    roomIDs := a.extractRoomIDsFromEnvironment(environment)
    
    // 2. Validate spawn configuration for multi-room
    if err := a.validateMultiRoomSpawnConfig(input.SpawnConfig, roomIDs); err != nil {
        return nil, errors.Wrap(err, "invalid multi-room spawn configuration")
    }
    
    // 3. Convert to toolkit config
    toolkitConfig, err := a.convertToToolkitSpawnConfig(input.SpawnConfig)
    if err != nil {
        return nil, errors.Wrap(err, "failed to convert spawn configuration")
    }
    
    // 4. Execute multi-room spawning
    spawnResult, err := a.spawnEngine.PopulateSplitRooms(ctx, roomIDs, toolkitConfig)
    if err != nil {
        return nil, errors.Wrap(err, "multi-room spawn operation failed")
    }
    
    // 5. Convert result with room distribution
    apiResult, err := a.convertMultiRoomSpawnResultToAPI(spawnResult, roomIDs)
    if err != nil {
        return nil, errors.Wrap(err, "failed to convert multi-room spawn result")
    }
    
    // 6. Add metadata
    apiResult.Metadata.ProcessingTimeMS = int32(time.Since(startTime).Milliseconds())
    
    return apiResult, nil
}

// PopulateSplitRooms implements entity spawning across specific connected rooms
func (a *Adapter) PopulateSplitRooms(ctx context.Context, input *PopulateSplitRoomsInput) (*PopulateSplitRoomsOutput, error) {
    startTime := time.Now()
    
    // 1. Validate room connectivity
    if err := a.validateRoomConnectivity(input.RoomIDs); err != nil {
        return nil, errors.Wrap(err, "invalid room connectivity")
    }
    
    // 2. Validate spawn configuration
    if err := a.validateMultiRoomSpawnConfig(input.SpawnConfig, input.RoomIDs); err != nil {
        return nil, errors.Wrap(err, "invalid spawn configuration for split rooms")
    }
    
    // 3. Convert configuration
    toolkitConfig, err := a.convertToToolkitSpawnConfig(input.SpawnConfig)
    if err != nil {
        return nil, errors.Wrap(err, "failed to convert spawn configuration")
    }
    
    // 4. Execute split-room spawning
    spawnResult, err := a.spawnEngine.PopulateSplitRooms(ctx, input.RoomIDs, toolkitConfig)
    if err != nil {
        return nil, errors.Wrap(err, "split-room spawn operation failed")
    }
    
    // 5. Convert result
    apiResult, err := a.convertMultiRoomSpawnResultToAPI(spawnResult, input.RoomIDs)
    if err != nil {
        return nil, errors.Wrap(err, "failed to convert split-room spawn result")
    }
    
    // 6. Add metadata
    apiResult.Metadata.ProcessingTimeMS = int32(time.Since(startTime).Milliseconds())
    
    return apiResult, nil
}

// ValidateSpawnConfiguration implements spawn configuration validation
func (a *Adapter) ValidateSpawnConfiguration(ctx context.Context, input *ValidateSpawnConfigurationInput) (*ValidateSpawnConfigurationOutput, error) {
    // 1. Basic configuration validation
    validationErrors, warnings := a.performSpawnConfigValidation(input.SpawnConfig)
    
    // 2. Context-specific validation (room or multi-room)
    var contextErrors []ValidationError
    var recommendations []SpawnRecommendation
    var estimate SpawnEstimate
    
    if input.RoomID != nil {
        // Single room validation
        roomErrors, roomRecs, roomEstimate, err := a.validateSingleRoomSpawn(*input.RoomID, input.SpawnConfig)
        if err != nil {
            return nil, errors.Wrap(err, "failed to validate single room spawn")
        }
        contextErrors = roomErrors
        recommendations = roomRecs
        estimate = roomEstimate
        
    } else if len(input.RoomIDs) > 0 {
        // Multi-room validation
        multiErrors, multiRecs, multiEstimate, err := a.validateMultiRoomSpawn(input.RoomIDs, input.SpawnConfig)
        if err != nil {
            return nil, errors.Wrap(err, "failed to validate multi-room spawn")
        }
        contextErrors = multiErrors
        recommendations = multiRecs
        estimate = multiEstimate
    }
    
    // 3. Combine results
    allErrors := append(validationErrors, contextErrors...)
    
    return &ValidateSpawnConfigurationOutput{
        IsValid:          len(allErrors) == 0,
        ValidationErrors: allErrors,
        Warnings:         warnings,
        Recommendations:  recommendations,
        EstimatedResults: estimate,
    }, nil
}

// GetSpawnRecommendations implements AI-driven spawn configuration recommendations
func (a *Adapter) GetSpawnRecommendations(ctx context.Context, input *GetSpawnRecommendationsInput) (*GetSpawnRecommendationsOutput, error) {
    // 1. Analyze room context
    var roomAnalysis RoomAnalysisData
    var err error
    
    if input.RoomID != nil {
        roomAnalysis, err = a.analyzeSingleRoom(*input.RoomID, input.Constraints)
    } else {
        roomAnalysis, err = a.analyzeMultipleRooms(input.RoomIDs, input.Constraints)
    }
    
    if err != nil {
        return nil, errors.Wrap(err, "failed to analyze room context")
    }
    
    // 2. Generate recommendations based on desired outcome
    recommendations, err := a.generateSpawnRecommendations(roomAnalysis, input.DesiredOutcome, input.GameContext)
    if err != nil {
        return nil, errors.Wrap(err, "failed to generate spawn recommendations")
    }
    
    // 3. Generate alternative approaches
    alternatives, err := a.generateAlternativeSpawnConfigs(roomAnalysis, input.DesiredOutcome)
    if err != nil {
        return nil, errors.Wrap(err, "failed to generate alternative spawn configurations")
    }
    
    // 4. Identify potential warnings
    warnings := a.identifySpawnWarnings(roomAnalysis, recommendations)
    
    return &GetSpawnRecommendationsOutput{
        Recommendations: recommendations,
        Alternatives:    alternatives,
        Warnings:        warnings,
        Metadata: RecommendationMetadata{
            AnalysisType: "ai_driven",
            Confidence:   calculateRecommendationConfidence(roomAnalysis, recommendations),
            Factors:      extractAnalysisFactors(roomAnalysis),
        },
    }, nil
}

// RegisterEntityTable implements entity selection table registration
func (a *Adapter) RegisterEntityTable(ctx context.Context, input *RegisterEntityTableInput) (*RegisterEntityTableOutput, error) {
    // 1. Validate entity definitions
    if err := a.validateEntityDefinitions(input.Entities); err != nil {
        return nil, errors.Wrap(err, "invalid entity definitions")
    }
    
    // 2. Convert API entities to toolkit entities
    toolkitEntities, err := a.convertAPIEntitiesToToolkit(input.Entities)
    if err != nil {
        return nil, errors.Wrap(err, "failed to convert entities")
    }
    
    // 3. Register with selectables registry
    err = a.selectablesRegistry.RegisterTable(input.TableID, toolkitEntities)
    if err != nil {
        return nil, errors.Wrap(err, "failed to register entity table")
    }
    
    // 4. Apply weights if provided
    if len(input.Weights) > 0 {
        if err := a.applyEntityWeights(input.TableID, input.Weights); err != nil {
            return nil, errors.Wrap(err, "failed to apply entity weights")
        }
    }
    
    return &RegisterEntityTableOutput{
        TableID:     input.TableID,
        EntityCount: int32(len(input.Entities)),
        Success:     true,
    }, nil
}

// GetEntityTables implements entity table retrieval
func (a *Adapter) GetEntityTables(ctx context.Context, input *GetEntityTablesInput) (*GetEntityTablesOutput, error) {
    // 1. Get table IDs from registry
    var tableIDs []string
    if len(input.TableIDs) > 0 {
        tableIDs = input.TableIDs
    } else {
        tableIDs = a.selectablesRegistry.ListTables()
    }
    
    // 2. Filter by session if specified
    if input.SessionID != nil {
        tableIDs = a.filterTablesBySession(tableIDs, *input.SessionID)
    }
    
    // 3. Build table information
    var tables []EntityTableInfo
    for _, tableID := range tableIDs {
        entities, err := a.selectablesRegistry.GetEntities(tableID, 0) // Get all entities
        if err != nil {
            continue // Skip unavailable tables
        }
        
        tableInfo := EntityTableInfo{
            TableID:     tableID,
            EntityCount: int32(len(entities)),
            EntityTypes: a.extractEntityTypes(entities),
            LastUpdated: a.getTableLastUpdated(tableID),
        }
        tables = append(tables, tableInfo)
    }
    
    return &GetEntityTablesOutput{
        Tables: tables,
        Metadata: TableMetadata{
            TotalTables:     int32(len(tables)),
            SessionFiltered: input.SessionID != nil,
        },
    }, nil
}

// === SELECTABLES ENGINE INTEGRATION METHODS ===

// CreateSelectionTable implements weighted selection table creation
func (a *Adapter) CreateSelectionTable(ctx context.Context, input *CreateSelectionTableInput) (*CreateSelectionTableOutput, error) {
    // 1. Validate input
    if err := a.validateSelectionTableInput(input); err != nil {
        return nil, errors.Wrap(err, "invalid selection table input")
    }
    
    // 2. Convert API items to toolkit selectables items
    toolkitItems, err := a.convertToToolkitSelectableItems(input.Items)
    if err != nil {
        return nil, errors.Wrap(err, "failed to convert selectable items")
    }
    
    // 3. Create table configuration
    toolkitConfig, err := a.convertToToolkitTableConfig(input.Configuration)
    if err != nil {
        return nil, errors.Wrap(err, "failed to convert table configuration")
    }
    
    // 4. Create selection table via toolkit factory
    table, err := a.selectablesFactory.CreateTable(input.TableID, toolkitItems, toolkitConfig)
    if err != nil {
        return nil, errors.Wrap(err, "failed to create selection table")
    }
    
    // 5. Register table with registry
    if err := a.selectablesTables.RegisterTable(input.TableID, table); err != nil {
        return nil, errors.Wrap(err, "failed to register selection table")
    }
    
    // 6. Calculate total weight
    totalWeight := a.calculateTotalWeight(toolkitItems)
    
    return &CreateSelectionTableOutput{
        TableID:     input.TableID,
        ItemCount:   int32(len(input.Items)),
        TotalWeight: int32(totalWeight),
        Success:     true,
    }, nil
}

// UpdateSelectionTable implements selection table modification
func (a *Adapter) UpdateSelectionTable(ctx context.Context, input *UpdateSelectionTableInput) (*UpdateSelectionTableOutput, error) {
    // 1. Get existing table
    table, err := a.selectablesTables.GetTable(input.TableID)
    if err != nil {
        return nil, errors.Wrap(err, "table not found")
    }
    
    // 2. Apply modifications
    if len(input.AddItems) > 0 {
        toolkitItems, err := a.convertToToolkitSelectableItems(input.AddItems)
        if err != nil {
            return nil, errors.Wrap(err, "failed to convert items to add")
        }
        if err := table.AddItems(toolkitItems); err != nil {
            return nil, errors.Wrap(err, "failed to add items")
        }
    }
    
    if len(input.RemoveItems) > 0 {
        if err := table.RemoveItems(input.RemoveItems); err != nil {
            return nil, errors.Wrap(err, "failed to remove items")
        }
    }
    
    if len(input.UpdateItems) > 0 {
        toolkitItems, err := a.convertToToolkitSelectableItems(input.UpdateItems)
        if err != nil {
            return nil, errors.Wrap(err, "failed to convert items to update")
        }
        if err := table.UpdateItems(toolkitItems); err != nil {
            return nil, errors.Wrap(err, "failed to update items")
        }
    }
    
    // 3. Update configuration if provided
    if input.Configuration != nil {
        toolkitConfig, err := a.convertToToolkitTableConfig(*input.Configuration)
        if err != nil {
            return nil, errors.Wrap(err, "failed to convert configuration")
        }
        if err := table.UpdateConfiguration(toolkitConfig); err != nil {
            return nil, errors.Wrap(err, "failed to update configuration")
        }
    }
    
    // 4. Get updated table stats
    itemCount := table.GetItemCount()
    totalWeight := table.GetTotalWeight()
    
    return &UpdateSelectionTableOutput{
        TableID:     input.TableID,
        ItemCount:   int32(itemCount),
        TotalWeight: int32(totalWeight),
        Success:     true,
    }, nil
}

// SelectFromTable implements single weighted selection
func (a *Adapter) SelectFromTable(ctx context.Context, input *SelectFromTableInput) (*SelectFromTableOutput, error) {
    // 1. Get selection table
    table, err := a.selectablesTables.GetTable(input.TableID)
    if err != nil {
        return nil, errors.Wrap(err, "table not found")
    }
    
    // 2. Convert API context to toolkit context
    toolkitContext, err := a.convertToToolkitSelectionContext(input.SelectionContext)
    if err != nil {
        return nil, errors.Wrap(err, "failed to convert selection context")
    }
    
    // 3. Perform selection
    result, err := table.Select(toolkitContext)
    if err != nil {
        return nil, errors.Wrap(err, "selection failed")
    }
    
    // 4. Convert result to API format
    apiResult, err := a.convertSelectionResultToAPI(result, input.Options)
    if err != nil {
        return nil, errors.Wrap(err, "failed to convert selection result")
    }
    
    return apiResult, nil
}

// SelectManyFromTable implements multiple weighted selections
func (a *Adapter) SelectManyFromTable(ctx context.Context, input *SelectManyFromTableInput) (*SelectManyFromTableOutput, error) {
    // 1. Get selection table
    table, err := a.selectablesTables.GetTable(input.TableID)
    if err != nil {
        return nil, errors.Wrap(err, "table not found")
    }
    
    // 2. Convert API context to toolkit context
    toolkitContext, err := a.convertToToolkitSelectionContext(input.SelectionContext)
    if err != nil {
        return nil, errors.Wrap(err, "failed to convert selection context")
    }
    
    // 3. Perform multiple selections
    results, err := table.SelectMany(toolkitContext, int(input.Count))
    if err != nil {
        return nil, errors.Wrap(err, "multiple selection failed")
    }
    
    // 4. Convert results to API format
    apiResult, err := a.convertMultipleSelectionResultsToAPI(results, input.Options)
    if err != nil {
        return nil, errors.Wrap(err, "failed to convert selection results")
    }
    
    return apiResult, nil
}

// SelectUniqueFromTable implements unique weighted selections
func (a *Adapter) SelectUniqueFromTable(ctx context.Context, input *SelectUniqueFromTableInput) (*SelectUniqueFromTableOutput, error) {
    // 1. Get selection table
    table, err := a.selectablesTables.GetTable(input.TableID)
    if err != nil {
        return nil, errors.Wrap(err, "table not found")
    }
    
    // 2. Convert API context to toolkit context
    toolkitContext, err := a.convertToToolkitSelectionContext(input.SelectionContext)
    if err != nil {
        return nil, errors.Wrap(err, "failed to convert selection context")
    }
    
    // 3. Perform unique selections
    results, err := table.SelectUnique(toolkitContext, int(input.Count))
    if err != nil {
        return nil, errors.Wrap(err, "unique selection failed")
    }
    
    // 4. Convert results to API format
    apiResult, err := a.convertUniqueSelectionResultsToAPI(results, input.Options)
    if err != nil {
        return nil, errors.Wrap(err, "failed to convert unique selection results")
    }
    
    return apiResult, nil
}

// SelectVariableFromTable implements dice-based variable quantity selection
func (a *Adapter) SelectVariableFromTable(ctx context.Context, input *SelectVariableFromTableInput) (*SelectVariableFromTableOutput, error) {
    // 1. Get selection table
    table, err := a.selectablesTables.GetTable(input.TableID)
    if err != nil {
        return nil, errors.Wrap(err, "table not found")
    }
    
    // 2. Convert API context to toolkit context
    toolkitContext, err := a.convertToToolkitSelectionContext(input.SelectionContext)
    if err != nil {
        return nil, errors.Wrap(err, "failed to convert selection context")
    }
    
    // 3. Roll dice to determine quantity
    diceResult, err := a.diceRoller.Roll(input.DiceExpression)
    if err != nil {
        return nil, errors.Wrap(err, "dice roll failed")
    }
    
    quantity := diceResult.Total
    if quantity <= 0 {
        quantity = 1 // Ensure at least one selection
    }
    
    // 4. Perform variable selections
    results, err := table.SelectMany(toolkitContext, quantity)
    if err != nil {
        return nil, errors.Wrap(err, "variable selection failed")
    }
    
    // 5. Convert results to API format
    apiResult, err := a.convertVariableSelectionResultsToAPI(results, diceResult.Total, input.Options)
    if err != nil {
        return nil, errors.Wrap(err, "failed to convert variable selection results")
    }
    
    return apiResult, nil
}

// ListSelectionTables implements table listing with filtering
func (a *Adapter) ListSelectionTables(ctx context.Context, input *ListSelectionTablesInput) (*ListSelectionTablesOutput, error) {
    // 1. Get all tables from registry
    allTables := a.selectablesTables.ListTables()
    
    // 2. Apply filters
    filteredTables := a.applySelectionTableFilters(allTables, input)
    
    // 3. Apply pagination
    limit := int32(50) // Default limit
    if input.Limit != nil {
        limit = *input.Limit
    }
    
    offset := int32(0)
    if input.Offset != nil {
        offset = *input.Offset
    }
    
    totalCount := int32(len(filteredTables))
    
    // Calculate pagination bounds
    start := offset
    end := offset + limit
    if end > totalCount {
        end = totalCount
    }
    
    var paginatedTables []SelectionTableInfo
    if start < totalCount {
        for i := start; i < end; i++ {
            tableInfo := a.convertTableToInfo(filteredTables[i])
            paginatedTables = append(paginatedTables, tableInfo)
        }
    }
    
    return &ListSelectionTablesOutput{
        Tables:     paginatedTables,
        TotalCount: totalCount,
        HasMore:    end < totalCount,
        Metadata: TableListMetadata{
            FilterApplied: input.SessionID != nil || input.ItemType != nil || input.NamePattern != nil,
            ResultLimit:   limit,
            ResultOffset:  offset,
        },
    }, nil
}

// GetSelectionTableInfo implements detailed table information retrieval
func (a *Adapter) GetSelectionTableInfo(ctx context.Context, input *GetSelectionTableInfoInput) (*GetSelectionTableInfoOutput, error) {
    // 1. Get table from registry
    table, err := a.selectablesTables.GetTable(input.TableID)
    if err != nil {
        return nil, errors.Wrap(err, "table not found")
    }
    
    // 2. Get table metadata
    info := table.GetInfo()
    usageStats := table.GetUsageStats()
    
    // 3. Get items if requested
    var items []SelectableItem
    if input.IncludeItems {
        toolkitItems := table.GetAllItems()
        items, err = a.convertToolkitItemsToAPI(toolkitItems)
        if err != nil {
            return nil, errors.Wrap(err, "failed to convert items")
        }
    }
    
    return &GetSelectionTableInfoOutput{
        TableID:      input.TableID,
        Name:         info.Name,
        Description:  info.Description,
        ItemType:     info.ItemType,
        ItemCount:    int32(info.ItemCount),
        TotalWeight:  int32(info.TotalWeight),
        Items:        items,
        Configuration: a.convertToolkitConfigToAPI(info.Configuration),
        CreatedAt:    info.CreatedAt,
        LastUpdated:  info.LastUpdated,
        UsageStats:   a.convertUsageStatsToAPI(usageStats),
    }, nil
}

// GetSelectionAnalytics implements selection analytics retrieval
func (a *Adapter) GetSelectionAnalytics(ctx context.Context, input *GetSelectionAnalyticsInput) (*GetSelectionAnalyticsOutput, error) {
    // 1. Get table from registry
    table, err := a.selectablesTables.GetTable(input.TableID)
    if err != nil {
        return nil, errors.Wrap(err, "table not found")
    }
    
    // 2. Get analytics data
    analytics, err := table.GetAnalytics(a.convertTimeRangeToToolkit(input.TimeRange), input.Granularity)
    if err != nil {
        return nil, errors.Wrap(err, "failed to get analytics")
    }
    
    // 3. Convert to API format
    apiAnalytics, err := a.convertAnalyticsToAPI(analytics)
    if err != nil {
        return nil, errors.Wrap(err, "failed to convert analytics")
    }
    
    return apiAnalytics, nil
}
```

### 3. New Service Layer: Room Orchestrator

Create `internal/orchestrators/room/` following established rpg-api patterns and project structure:

## Project Integration Architecture

The room orchestrator integrates into the existing rpg-api structure following the established **Outside-In** development approach:

```
/internal/
├── entities/              # Simple data models (just structs)
│   ├── room.go           # Room entity definition
│   ├── environment.go    # Environment entity definition  
│   └── selection_table.go # Selection table entity definition
├── handlers/              # gRPC handlers (API layer)
│   └── api/
│       └── v1alpha1/
│           ├── room_handler.go      # Room generation handler
│           └── room_handler_test.go # Handler tests with mocked service
├── services/              # Service interfaces (business logic contracts)
│   └── room/
│       ├── service.go     # Interface with Input/Output types
│       ├── service_test.go # Interface validation tests
│       └── mock/          # Generated mocks for testing
│           └── mock_service.go
├── orchestrators/         # Service implementations (business logic)
│   └── room/             # NEW: Room orchestrator following established patterns
│       ├── orchestrator.go         # Main business logic implementation
│       ├── orchestrator_test.go    # Orchestrator tests
│       ├── room_operations.go      # Room generation and management
│       ├── spawn_operations.go     # Entity spawning logic
│       ├── selectables_operations.go # Selection table operations
│       ├── spatial_operations.go   # Spatial query operations
│       ├── validation.go          # Input validation helpers
│       ├── conversion.go          # Entity conversion utilities
│       └── config.go              # Orchestrator configuration
├── repositories/          # Storage interfaces and implementations
│   └── room/
│       ├── repository.go  # Interface + types
│       ├── redis.go      # Redis implementation
│       └── mock/         # Generated mocks
│           └── mock_repository.go
└── engine/               # rpg-toolkit integration (already extended)
    ├── interface.go      # Extended with room/spawn/selectables methods
    └── rpgtoolkit/
        └── adapter.go    # Extended implementation
```

## File Structure Details

### `internal/services/room/service.go` - Service Interface
```go
package room

import (
    "context"
)

//go:generate mockgen -destination=mock/mock_service.go -package=roommock github.com/KirkDiggler/rpg-api/internal/services/room Service

// Service defines the business logic contract for room generation and management
// This interface follows rpg-api's principle of using Input/Output types for all operations
type Service interface {
    // === ROOM GENERATION ===
    GenerateRoom(ctx context.Context, input *GenerateRoomInput) (*GenerateRoomOutput, error)
    GenerateEnvironment(ctx context.Context, input *GenerateEnvironmentInput) (*GenerateEnvironmentOutput, error)
    
    // === ROOM MANAGEMENT ===
    GetRoom(ctx context.Context, input *GetRoomInput) (*GetRoomOutput, error)
    GetEnvironment(ctx context.Context, input *GetEnvironmentInput) (*GetEnvironmentOutput, error)
    ListRooms(ctx context.Context, input *ListRoomsInput) (*ListRoomsOutput, error)
    DeleteRoom(ctx context.Context, input *DeleteRoomInput) (*DeleteRoomOutput, error)
    
    // === ENTITY SPAWNING ===
    PopulateRoom(ctx context.Context, input *PopulateRoomInput) (*PopulateRoomOutput, error)
    PopulateEnvironment(ctx context.Context, input *PopulateEnvironmentInput) (*PopulateEnvironmentOutput, error)
    PopulateSplitRooms(ctx context.Context, input *PopulateSplitRoomsInput) (*PopulateSplitRoomsOutput, error)
    
    // === SPAWN CONFIGURATION ===
    ValidateSpawnConfiguration(ctx context.Context, input *ValidateSpawnConfigurationInput) (*ValidateSpawnConfigurationOutput, error)
    GetSpawnRecommendations(ctx context.Context, input *GetSpawnRecommendationsInput) (*GetSpawnRecommendationsOutput, error)
    RegisterEntityTable(ctx context.Context, input *RegisterEntityTableInput) (*RegisterEntityTableOutput, error)
    GetEntityTables(ctx context.Context, input *GetEntityTablesInput) (*GetEntityTablesOutput, error)
    
    // === SPATIAL QUERIES ===
    QueryEntitiesInRange(ctx context.Context, input *QueryEntitiesInRangeInput) (*QueryEntitiesInRangeOutput, error)
    QueryLineOfSight(ctx context.Context, input *QueryLineOfSightInput) (*QueryLineOfSightOutput, error)
    ValidateMovement(ctx context.Context, input *ValidateMovementInput) (*ValidateMovementOutput, error)
    ValidateEntityPlacement(ctx context.Context, input *ValidateEntityPlacementInput) (*ValidateEntityPlacementOutput, error)
    
    // === SELECTABLES INTEGRATION ===
    CreateSelectionTable(ctx context.Context, input *CreateSelectionTableInput) (*CreateSelectionTableOutput, error)
    UpdateSelectionTable(ctx context.Context, input *UpdateSelectionTableInput) (*UpdateSelectionTableOutput, error)
    DeleteSelectionTable(ctx context.Context, input *DeleteSelectionTableInput) (*DeleteSelectionTableOutput, error)
    
    // === SELECTION OPERATIONS ===
    SelectFromTable(ctx context.Context, input *SelectFromTableInput) (*SelectFromTableOutput, error)
    SelectManyFromTable(ctx context.Context, input *SelectManyFromTableInput) (*SelectManyFromTableOutput, error)
    SelectUniqueFromTable(ctx context.Context, input *SelectUniqueFromTableInput) (*SelectUniqueFromTableOutput, error)
    SelectVariableFromTable(ctx context.Context, input *SelectVariableFromTableInput) (*SelectVariableFromTableOutput, error)
    
    // === TABLE MANAGEMENT ===
    ListSelectionTables(ctx context.Context, input *ListSelectionTablesInput) (*ListSelectionTablesOutput, error)
    GetSelectionTableInfo(ctx context.Context, input *GetSelectionTableInfoInput) (*GetSelectionTableInfoOutput, error)
    GetSelectionAnalytics(ctx context.Context, input *GetSelectionAnalyticsInput) (*GetSelectionAnalyticsOutput, error)
}

// All Input/Output types are defined here following rpg-api patterns
// (Types already defined in earlier sections of this ADR)
```

### `internal/orchestrators/room/orchestrator.go` - Main Implementation
```go
package room

import (
    "context"
    "time"

    "github.com/KirkDiggler/rpg-api/internal/engine"
    "github.com/KirkDiggler/rpg-api/internal/repositories/room"
    "github.com/KirkDiggler/rpg-api/internal/services/room"
    "github.com/KirkDiggler/rpg-api/pkg/clock"
    "github.com/KirkDiggler/rpg-api/pkg/errors"
    "github.com/KirkDiggler/rpg-api/pkg/idgen"
)

// Orchestrator implements the room.Service interface
// Following rpg-api patterns for business logic orchestration
type Orchestrator struct {
    engine     engine.Engine
    repository room.Repository
    idGen      idgen.Generator
    clock      clock.Clock
    
    // Configuration
    config OrchestratorConfig
}

// OrchestratorConfig defines orchestrator behavior
type OrchestratorConfig struct {
    DefaultRoomTTL     time.Duration
    MaxEntitiesPerRoom int32
    EnableAnalytics    bool
    CacheTimeout       time.Duration
}

// NewOrchestrator creates a new room orchestrator
func NewOrchestrator(config OrchestratorConfig, engine engine.Engine, repository room.Repository, idGen idgen.Generator, clock clock.Clock) *Orchestrator {
    return &Orchestrator{
        engine:     engine,
        repository: repository,
        idGen:      idGen,
        clock:      clock,
        config:     config,
    }
}

// Verify interface compliance at compile time
var _ room.Service = (*Orchestrator)(nil)
```

### `internal/orchestrators/room/room_operations.go` - Room Operations
```go
package room

import (
    "context"
    "time"

    "github.com/KirkDiggler/rpg-api/internal/entities"
    "github.com/KirkDiggler/rpg-api/internal/services/room"
    "github.com/KirkDiggler/rpg-api/pkg/errors"
)

// GenerateRoom orchestrates room generation workflow
func (o *Orchestrator) GenerateRoom(ctx context.Context, input *room.GenerateRoomInput) (*room.GenerateRoomOutput, error) {
    // 1. Validate input
    if err := o.validateGenerateRoomInput(input); err != nil {
        return nil, errors.Wrap(err, "invalid generate room input")
    }
    
    // 2. Generate room via engine
    engineOutput, err := o.engine.GenerateRoom(ctx, &engine.GenerateRoomInput{
        Config:    input.Config,
        Seed:      input.Seed,
        SessionID: input.SessionID,
        TTL:       input.TTL,
    })
    if err != nil {
        return nil, errors.Wrap(err, "engine room generation failed")
    }
    
    // 3. Convert engine entities to repository entities
    repositoryEntities, err := o.convertEngineEntitiesToRepository(engineOutput.Entities)
    if err != nil {
        return nil, errors.Wrap(err, "failed to convert entities")
    }
    
    // 4. Store room data
    repositoryRoom := o.convertEngineRoomToRepository(engineOutput.Room)
    saveInput := &room.SaveRoomInput{
        Room:     repositoryRoom,
        Entities: repositoryEntities,
    }
    
    saveOutput, err := o.repository.Save(ctx, saveInput)
    if err != nil {
        return nil, errors.Wrap(err, "failed to save room")
    }
    
    // 5. Build response
    return &room.GenerateRoomOutput{
        Room:      engineOutput.Room,
        Entities:  engineOutput.Entities,
        Metadata:  engineOutput.Metadata,
        ExpiresAt: engineOutput.ExpiresAt,
        RoomID:    saveOutput.RoomID,
    }, nil
}

// GenerateEnvironment orchestrates multi-room environment generation
func (o *Orchestrator) GenerateEnvironment(ctx context.Context, input *room.GenerateEnvironmentInput) (*room.GenerateEnvironmentOutput, error) {
    // Implementation following similar pattern...
    // (Detailed implementation would follow similar structure)
    return nil, errors.Unimplemented("environment generation not yet implemented")
}

// Additional room management methods...
```

### `internal/orchestrators/room/spawn_operations.go` - Spawning Operations
```go
package room

import (
    "context"

    "github.com/KirkDiggler/rpg-api/internal/services/room"
    "github.com/KirkDiggler/rpg-api/pkg/errors"
)

// PopulateRoom orchestrates entity spawning in a room
func (o *Orchestrator) PopulateRoom(ctx context.Context, input *room.PopulateRoomInput) (*room.PopulateRoomOutput, error) {
    // 1. Validate spawn configuration
    if err := o.validatePopulateRoomInput(input); err != nil {
        return nil, errors.Wrap(err, "invalid populate room input")
    }
    
    // 2. Execute spawning via engine
    engineOutput, err := o.engine.PopulateRoom(ctx, &engine.PopulateRoomInput{
        RoomID:      input.RoomID,
        SpawnConfig: input.SpawnConfig,
        SessionID:   input.SessionID,
    })
    if err != nil {
        return nil, errors.Wrap(err, "engine spawning failed")
    }
    
    // 3. Store spawn data if needed
    if o.config.EnableAnalytics {
        spawnInput := &room.SaveSpawnDataInput{
            RoomID:    input.RoomID,
            SpawnData: engineOutput,
            Timestamp: o.clock.Now(),
        }
        
        if _, err := o.repository.SaveSpawnData(ctx, spawnInput); err != nil {
            // Log error but don't fail the operation
            // Analytics storage is not critical to the main operation
        }
    }
    
    // 4. Build response
    return &room.PopulateRoomOutput{
        Success:              engineOutput.Success,
        SpawnedEntities:      engineOutput.SpawnedEntities,
        Failures:             engineOutput.Failures,
        RoomModifications:    engineOutput.RoomModifications,
        SplitRecommendations: engineOutput.SplitRecommendations,
        Metadata:             engineOutput.Metadata,
    }, nil
}

// Additional spawning methods...
```

### `internal/orchestrators/room/selectables_operations.go` - Selection Operations
```go
package room

import (
    "context"

    "github.com/KirkDiggler/rpg-api/internal/services/room"
    "github.com/KirkDiggler/rpg-api/pkg/errors"
)

// CreateSelectionTable orchestrates selection table creation
func (o *Orchestrator) CreateSelectionTable(ctx context.Context, input *room.CreateSelectionTableInput) (*room.CreateSelectionTableOutput, error) {
    // 1. Validate input
    if err := o.validateCreateSelectionTableInput(input); err != nil {
        return nil, errors.Wrap(err, "invalid create selection table input")
    }
    
    // 2. Create table via engine
    engineOutput, err := o.engine.CreateSelectionTable(ctx, &engine.CreateSelectionTableInput{
        TableID:       input.TableID,
        Name:          input.Name,
        Description:   input.Description,
        ItemType:      input.ItemType,
        Items:         input.Items,
        Configuration: input.Configuration,
        SessionID:     input.SessionID,
    })
    if err != nil {
        return nil, errors.Wrap(err, "engine table creation failed")
    }
    
    // 3. Store table metadata
    tableInput := &room.SaveEntityTableInput{
        TableID:   input.TableID,
        TableData: engineOutput,
        Timestamp: o.clock.Now(),
    }
    
    if _, err := o.repository.SaveEntityTable(ctx, tableInput); err != nil {
        return nil, errors.Wrap(err, "failed to save table metadata")
    }
    
    // 4. Build response
    return &room.CreateSelectionTableOutput{
        TableID:     engineOutput.TableID,
        ItemCount:   engineOutput.ItemCount,
        TotalWeight: engineOutput.TotalWeight,
        Success:     engineOutput.Success,
    }, nil
}

// Additional selection operations...
```

### `internal/orchestrators/room/validation.go` - Input Validation
```go
package room

import (
    "github.com/KirkDiggler/rpg-api/internal/services/room"
    "github.com/KirkDiggler/rpg-api/pkg/errors"
)

// validateGenerateRoomInput validates room generation input
func (o *Orchestrator) validateGenerateRoomInput(input *room.GenerateRoomInput) error {
    if input == nil {
        return errors.InvalidArgument("input is required")
    }
    
    if input.Config.Width <= 0 || input.Config.Width > 100 {
        return errors.InvalidArgument("width must be between 1 and 100")
    }
    
    if input.Config.Height <= 0 || input.Config.Height > 100 {
        return errors.InvalidArgument("height must be between 1 and 100")
    }
    
    // Additional validation logic...
    return nil
}

// Additional validation methods for other operations...
```

### `internal/orchestrators/room/conversion.go` - Entity Conversion
```go
package room

import (
    "github.com/KirkDiggler/rpg-api/internal/engine"
    "github.com/KirkDiggler/rpg-api/internal/entities"
)

// convertEngineRoomToRepository converts engine room data to repository format
func (o *Orchestrator) convertEngineRoomToRepository(engineRoom *engine.RoomData) *entities.Room {
    return &entities.Room{
        ID:         engineRoom.ID,
        Name:       engineRoom.Name,
        Width:      engineRoom.Width,
        Height:     engineRoom.Height,
        GridType:   engineRoom.GridType,
        Theme:      engineRoom.Theme,
        SessionID:  engineRoom.SessionID,
        CreatedAt:  engineRoom.CreatedAt,
        Properties: engineRoom.Properties,
    }
}

// convertEngineEntitiesToRepository converts engine entities to repository format
func (o *Orchestrator) convertEngineEntitiesToRepository(engineEntities []engine.EntityData) ([]entities.RoomEntity, error) {
    var repositoryEntities []entities.RoomEntity
    
    for _, engineEntity := range engineEntities {
        repositoryEntity := entities.RoomEntity{
            ID:         engineEntity.ID,
            Type:       engineEntity.Type,
            Position:   engineEntity.Position,
            State:      engineEntity.State,
            Properties: engineEntity.Properties,
        }
        
        repositoryEntities = append(repositoryEntities, repositoryEntity)
    }
    
    return repositoryEntities, nil
}

// Additional conversion methods...
```

## Development Workflow Integration

Following rpg-api's **Outside-In** approach:

### Phase 1: Service Interface (COMPLETE)
- ✅ Define `internal/services/room/service.go` with all Input/Output types
- ✅ Generate mocks with `go generate`
- ✅ Interface validation tests

### Phase 2: Handler Implementation  
- Create `internal/handlers/api/v1alpha1/room_handler.go`
- Return `codes.Unimplemented` initially
- Test handler with mocked service
- Validate proto request/response conversion

### Phase 3: Orchestrator Implementation
- Implement `internal/orchestrators/room/orchestrator.go`
- Wire up engine, repository, and utility dependencies
- Comprehensive orchestrator tests with mocked dependencies
- Business logic validation and error handling

### Phase 4: Integration Testing
- End-to-end testing with real dependencies
- Performance testing and optimization
- Error handling and edge case validation

This architecture maintains rpg-api's established patterns while providing comprehensive room generation, entity spawning, and selection table capabilities.

### 4. Repository Pattern for Room Storage

**ARCHITECTURAL INSIGHT - Seed-Based Persistence Strategy**:
We should explore using seed-based persistence for significant storage and performance improvements. Instead of storing complete room structures (~5-50KB), store only:
- **Generation parameters**: Config + seed (~500B)  
- **Dynamic entities**: Characters, monsters, loot with positions/state (~100-500B each)
- **Deterministic reconstruction**: Same seed + config = identical walls every time

This provides **90%+ storage reduction** while maintaining perfect reconstruction capabilities. Walls and static structure can be regenerated on-demand and cached for performance.

Create `internal/repositories/room/` following established rpg-api patterns:

```go
// repository.go - Interface definition
type Repository interface {
    // Room storage
    Save(ctx context.Context, input *SaveRoomInput) (*SaveRoomOutput, error)
    Get(ctx context.Context, input *GetRoomInput) (*GetRoomOutput, error)
    List(ctx context.Context, input *ListRoomsInput) (*ListRoomsOutput, error)
    Delete(ctx context.Context, input *DeleteRoomInput) (*DeleteRoomOutput, error)
    
    // Environment storage
    SaveEnvironment(ctx context.Context, input *SaveEnvironmentInput) (*SaveEnvironmentOutput, error)
    GetEnvironment(ctx context.Context, input *GetEnvironmentInput) (*GetEnvironmentOutput, error)
    ListEnvironments(ctx context.Context, input *ListEnvironmentsInput) (*ListEnvironmentsOutput, error)
    DeleteEnvironment(ctx context.Context, input *DeleteEnvironmentInput) (*DeleteEnvironmentOutput, error)
    
    // Entity spawn data storage
    SaveSpawnData(ctx context.Context, input *SaveSpawnDataInput) (*SaveSpawnDataOutput, error)
    GetSpawnData(ctx context.Context, input *GetSpawnDataInput) (*GetSpawnDataOutput, error)
    DeleteSpawnData(ctx context.Context, input *DeleteSpawnDataInput) (*DeleteSpawnDataOutput, error)
    
    // Entity table storage
    SaveEntityTable(ctx context.Context, input *SaveEntityTableInput) (*SaveEntityTableOutput, error)
    GetEntityTable(ctx context.Context, input *GetEntityTableInput) (*GetEntityTableOutput, error)
    ListEntityTables(ctx context.Context, input *ListEntityTablesInput) (*ListEntityTablesOutput, error)
    DeleteEntityTable(ctx context.Context, input *DeleteEntityTableInput) (*DeleteEntityTableOutput, error)
}

// redis.go - Implementation following character/dice patterns
type RedisRepository struct {
    client redis.Client
    idGen  idgen.Generator
}

// Input/Output types for repository operations
type SaveRoomInput struct {
    Room     *entities.Room
    Entities []entities.RoomEntity
}

type SaveRoomOutput struct {
    RoomID string
}

type GetRoomInput struct {
    RoomID string
}

type GetRoomOutput struct {
    Room     *entities.Room
    Entities []entities.RoomEntity
    Found    bool
}

// Redis Implementation Details
type RedisRepository struct {
    client redis.Client
    idGen  idgen.Generator
    clock  clock.Clock
}

// NewRedisRepository creates a new Redis-based room repository
func NewRedisRepository(config RedisRepositoryConfig) (*RedisRepository, error) {
    if err := config.Validate(); err != nil {
        return nil, err
    }
    
    return &RedisRepository{
        client: config.Client,
        idGen:  config.IDGenerator,
        clock:  config.Clock,
    }, nil
}

// Save stores room data with TTL support
func (r *RedisRepository) Save(ctx context.Context, input *SaveRoomInput) (*SaveRoomOutput, error) {
    // Validate input
    if input.Room == nil {
        return nil, errors.InvalidArgument("room is required")
    }
    
    roomID := input.Room.ID
    if roomID == "" {
        roomID = r.idGen.Generate()
        input.Room.ID = roomID
    }
    
    // Calculate TTL
    ttl := time.Hour // Default 1 hour
    if input.TTL != nil {
        ttl = *input.TTL
    }
    
    // Serialize room data
    roomData, err := r.serializeRoom(input.Room, input.Entities)
    if err != nil {
        return nil, errors.Wrap(err, "failed to serialize room data")
    }
    
    // Redis key patterns
    roomKey := r.buildRoomKey(roomID)
    sessionKey := r.buildSessionIndex(input.Room.SessionID, roomID)
    themeKey := r.buildThemeIndex(input.Room.Theme, roomID)
    
    // Redis pipeline for atomic operations
    pipe := r.client.Pipeline()
    
    // Store main room data
    pipe.Set(ctx, roomKey, roomData, ttl)
    
    // Store indexes for querying
    if input.Room.SessionID != "" {
        pipe.Set(ctx, sessionKey, roomID, ttl)
    }
    pipe.Set(ctx, themeKey, roomID, ttl)
    
    // Execute pipeline
    _, err = pipe.Exec(ctx)
    if err != nil {
        return nil, errors.Wrap(err, "failed to save room data to Redis")
    }
    
    return &SaveRoomOutput{
        RoomID: roomID,
    }, nil
}

// Get retrieves room data by ID
func (r *RedisRepository) Get(ctx context.Context, input *GetRoomInput) (*GetRoomOutput, error) {
    if input.RoomID == "" {
        return nil, errors.InvalidArgument("room_id is required")
    }
    
    roomKey := r.buildRoomKey(input.RoomID)
    
    // Get room data from Redis
    data, err := r.client.Get(ctx, roomKey).Result()
    if err == redis.Nil {
        // Room not found
        return &GetRoomOutput{Found: false}, nil
    }
    if err != nil {
        return nil, errors.Wrap(err, "failed to get room from Redis")
    }
    
    // Deserialize room data
    room, entities, err := r.deserializeRoom(data)
    if err != nil {
        return nil, errors.Wrap(err, "failed to deserialize room data")
    }
    
    return &GetRoomOutput{
        Room:     room,
        Entities: entities,
        Found:    true,
    }, nil
}

// List retrieves rooms with filtering
func (r *RedisRepository) List(ctx context.Context, input *ListRoomsInput) (*ListRoomsOutput, error) {
    var keys []string
    var err error
    
    // Build query based on filters
    if input.SessionID != "" {
        keys, err = r.getRoomsBySession(ctx, input.SessionID)
    } else if input.Theme != "" {
        keys, err = r.getRoomsByTheme(ctx, input.Theme)
    } else {
        keys, err = r.getAllRoomKeys(ctx)
    }
    
    if err != nil {
        return nil, errors.Wrap(err, "failed to get room keys")
    }
    
    // Apply pagination
    total := len(keys)
    start := input.Offset
    end := start + input.Limit
    if end > total {
        end = total
    }
    if start >= total {
        return &ListRoomsOutput{
            Rooms:      []*entities.Room{},
            TotalCount: int32(total),
            HasMore:    false,
        }, nil
    }
    
    pageKeys := keys[start:end]
    
    // Get room data for page
    rooms, err := r.getRoomsByKeys(ctx, pageKeys)
    if err != nil {
        return nil, errors.Wrap(err, "failed to get room data")
    }
    
    return &ListRoomsOutput{
        Rooms:      rooms,
        TotalCount: int32(total),
        HasMore:    end < total,
    }, nil
}

// Delete removes room and all indexes
func (r *RedisRepository) Delete(ctx context.Context, input *DeleteRoomInput) (*DeleteRoomOutput, error) {
    if input.RoomID == "" {
        return nil, errors.InvalidArgument("room_id is required")
    }
    
    // Get room first to clean up indexes
    getRoomOutput, err := r.Get(ctx, &GetRoomInput{RoomID: input.RoomID})
    if err != nil {
        return nil, errors.Wrap(err, "failed to get room for deletion")
    }
    
    if !getRoomOutput.Found {
        return &DeleteRoomOutput{Deleted: false}, nil
    }
    
    room := getRoomOutput.Room
    entityCount := len(getRoomOutput.Entities)
    
    // Build all keys to delete
    roomKey := r.buildRoomKey(input.RoomID)
    var keysToDelete []string
    keysToDelete = append(keysToDelete, roomKey)
    
    // Add index keys
    if room.SessionID != "" {
        keysToDelete = append(keysToDelete, r.buildSessionIndex(room.SessionID, input.RoomID))
    }
    keysToDelete = append(keysToDelete, r.buildThemeIndex(room.Theme, input.RoomID))
    
    // Delete all keys
    deleted, err := r.client.Del(ctx, keysToDelete...).Result()
    if err != nil {
        return nil, errors.Wrap(err, "failed to delete room from Redis")
    }
    
    return &DeleteRoomOutput{
        Deleted:         deleted > 0,
        EntitiesDeleted: int32(entityCount),
    }, nil
}

// Redis Key Patterns
const (
    roomKeyPrefix    = "room:"
    sessionIdxPrefix = "session_rooms:"
    themeIdxPrefix   = "theme_rooms:"
)

func (r *RedisRepository) buildRoomKey(roomID string) string {
    return roomKeyPrefix + roomID
}

func (r *RedisRepository) buildSessionIndex(sessionID, roomID string) string {
    return sessionIdxPrefix + sessionID + ":" + roomID
}

func (r *RedisRepository) buildThemeIndex(theme, roomID string) string {
    return themeIdxPrefix + theme + ":" + roomID
}

// Serialization Format
type StoredRoomData struct {
    Room      *entities.Room        `json:"room"`
    Entities  []entities.RoomEntity `json:"entities"`
    CreatedAt time.Time             `json:"created_at"`
    UpdatedAt time.Time             `json:"updated_at"`
    Version   int                   `json:"version"`  // For future migrations
}

func (r *RedisRepository) serializeRoom(room *entities.Room, entities []entities.RoomEntity) (string, error) {
    data := StoredRoomData{
        Room:      room,
        Entities:  entities,
        CreatedAt: r.clock.Now(),
        UpdatedAt: r.clock.Now(),
        Version:   1,
    }
    
    jsonData, err := json.Marshal(data)
    if err != nil {
        return "", errors.Wrap(err, "failed to marshal room data")
    }
    
    return string(jsonData), nil
}

func (r *RedisRepository) deserializeRoom(data string) (*entities.Room, []entities.RoomEntity, error) {
    var stored StoredRoomData
    if err := json.Unmarshal([]byte(data), &stored); err != nil {
        return nil, nil, errors.Wrap(err, "failed to unmarshal room data")
    }
    
    return stored.Room, stored.Entities, nil
}

// Helper methods for querying
func (r *RedisRepository) getRoomsBySession(ctx context.Context, sessionID string) ([]string, error) {
    pattern := sessionIdxPrefix + sessionID + ":*"
    return r.scanKeys(ctx, pattern)
}

func (r *RedisRepository) getRoomsByTheme(ctx context.Context, theme string) ([]string, error) {
    pattern := themeIdxPrefix + theme + ":*"
    return r.scanKeys(ctx, pattern)
}

func (r *RedisRepository) getAllRoomKeys(ctx context.Context) ([]string, error) {
    pattern := roomKeyPrefix + "*"
    return r.scanKeys(ctx, pattern)
}

func (r *RedisRepository) scanKeys(ctx context.Context, pattern string) ([]string, error) {
    var keys []string
    iter := r.client.Scan(ctx, 0, pattern, 0).Iterator()
    
    for iter.Next(ctx) {
        keys = append(keys, iter.Val())
    }
    
    if err := iter.Err(); err != nil {
        return nil, errors.Wrap(err, "failed to scan Redis keys")
    }
    
    return keys, nil
}

func (r *RedisRepository) getRoomsByKeys(ctx context.Context, keys []string) ([]*entities.Room, error) {
    if len(keys) == 0 {
        return []*entities.Room{}, nil
    }
    
    // Get all room data in pipeline
    pipe := r.client.Pipeline()
    cmds := make([]*redis.StringCmd, len(keys))
    
    for i, key := range keys {
        cmds[i] = pipe.Get(ctx, key)
    }
    
    _, err := pipe.Exec(ctx)
    if err != nil {
        return nil, errors.Wrap(err, "failed to get room data from pipeline")
    }
    
    // Deserialize results
    var rooms []*entities.Room
    for _, cmd := range cmds {
        data, err := cmd.Result()
        if err == redis.Nil {
            continue // Skip expired rooms
        }
        if err != nil {
            return nil, errors.Wrap(err, "failed to get room data from command")
        }
        
        room, _, err := r.deserializeRoom(data)
        if err != nil {
            return nil, errors.Wrap(err, "failed to deserialize room")
        }
        
        rooms = append(rooms, room)
    }
    
    return rooms, nil
}
```

### 5. Generic API Endpoint

Extend `internal/handlers/api/v1alpha1/` with room handler:

```go
// room_handler.go
type RoomHandler struct {
    apiv1alpha1.UnimplementedRoomServiceServer
    roomService room.Service
}

func (h *RoomHandler) GenerateRoom(ctx context.Context, req *apiv1alpha1.GenerateRoomRequest) (*apiv1alpha1.GenerateRoomResponse, error) {
    // Request validation
    // Service call with Input/Output conversion  
    // Response mapping
}
```

### 6. Proto Definitions

Add to rpg-api-protos `api/v1alpha1/room.proto`:

```protobuf
syntax = "proto3";

package api.v1alpha1;

// Room generation and management service for all RPG systems
// Provides tactical environment creation with spatial positioning and entity spawning
service RoomService {
  // === ROOM GENERATION ===
  // Generate a new room with procedural content
  rpc GenerateRoom(GenerateRoomRequest) returns (GenerateRoomResponse);
  
  // Generate a multi-room environment with connections
  rpc GenerateEnvironment(GenerateEnvironmentRequest) returns (GenerateEnvironmentResponse);
  
  // === ROOM MANAGEMENT ===
  // Retrieve an existing room by ID
  rpc GetRoom(GetRoomRequest) returns (GetRoomResponse);
  
  // Retrieve an existing environment by ID
  rpc GetEnvironment(GetEnvironmentRequest) returns (GetEnvironmentResponse);
  
  // List rooms with filtering options
  rpc ListRooms(ListRoomsRequest) returns (ListRoomsResponse);
  
  // Delete a room or environment
  rpc DeleteRoom(DeleteRoomRequest) returns (DeleteRoomResponse);
  
  // === ENTITY SPAWNING ===
  // Populate a room with entities using intelligent placement
  rpc PopulateRoom(PopulateRoomRequest) returns (PopulateRoomResponse);
  
  // Populate an environment (multiple connected rooms) with entities
  rpc PopulateEnvironment(PopulateEnvironmentRequest) returns (PopulateEnvironmentResponse);
  
  // Populate specific connected rooms with distributed entities
  rpc PopulateSplitRooms(PopulateSplitRoomsRequest) returns (PopulateSplitRoomsResponse);
  
  // === SPAWN CONFIGURATION ===
  // Validate a spawn configuration before execution
  rpc ValidateSpawnConfiguration(ValidateSpawnConfigurationRequest) returns (ValidateSpawnConfigurationResponse);
  
  // Get AI-driven spawn recommendations
  rpc GetSpawnRecommendations(GetSpawnRecommendationsRequest) returns (GetSpawnRecommendationsResponse);
  
  // Register entity selection tables
  rpc RegisterEntityTable(RegisterEntityTableRequest) returns (RegisterEntityTableResponse);
  
  // Retrieve available entity tables
  rpc GetEntityTables(GetEntityTablesRequest) returns (GetEntityTablesResponse);
  
  // === SPATIAL QUERIES ===
  // Query entities within a radius
  rpc QueryEntitiesInRange(QueryEntitiesInRangeRequest) returns (QueryEntitiesInRangeResponse);
  
  // Check line of sight between positions
  rpc QueryLineOfSight(QueryLineOfSightRequest) returns (QueryLineOfSightResponse);
  
  // Validate movement between positions
  rpc ValidateMovement(ValidateMovementRequest) returns (ValidateMovementResponse);
  
  // Validate entity placement at position
  rpc ValidateEntityPlacement(ValidateEntityPlacementRequest) returns (ValidateEntityPlacementResponse);
  
  // === CAPACITY ANALYSIS ===
  // Analyze room capacity for entities and gameplay
  rpc AnalyzeRoomCapacity(AnalyzeRoomCapacityRequest) returns (AnalyzeRoomCapacityResponse);
  
  // Get fallback configurations when generation fails
  rpc GetGenerationFallbacks(GetGenerationFallbacksRequest) returns (GetGenerationFallbacksResponse);
  
  // === WEIGHTED SELECTION SYSTEM ===
  // Create a new weighted selection table
  rpc CreateSelectionTable(CreateSelectionTableRequest) returns (CreateSelectionTableResponse);
  
  // Update existing selection table 
  rpc UpdateSelectionTable(UpdateSelectionTableRequest) returns (UpdateSelectionTableResponse);
  
  // Delete selection table
  rpc DeleteSelectionTable(DeleteSelectionTableRequest) returns (DeleteSelectionTableResponse);
  
  // === CONTEXT-AWARE SELECTION OPERATIONS ===
  // Single weighted selection from table
  rpc SelectFromTable(SelectFromTableRequest) returns (SelectFromTableResponse);
  
  // Multiple weighted selections with replacement
  rpc SelectManyFromTable(SelectManyFromTableRequest) returns (SelectManyFromTableResponse);
  
  // Multiple unique selections without replacement
  rpc SelectUniqueFromTable(SelectUniqueFromTableRequest) returns (SelectUniqueFromTableResponse);
  
  // Variable quantity selection using dice expression
  rpc SelectVariableFromTable(SelectVariableFromTableRequest) returns (SelectVariableFromTableResponse);
  
  // === SELECTION TABLE MANAGEMENT ===
  // List available selection tables with filtering
  rpc ListSelectionTables(ListSelectionTablesRequest) returns (ListSelectionTablesResponse);
  
  // Get detailed information about a selection table
  rpc GetSelectionTableInfo(GetSelectionTableInfoRequest) returns (GetSelectionTableInfoResponse);
  
  // Get selection analytics and usage statistics
  rpc GetSelectionAnalytics(GetSelectionAnalyticsRequest) returns (GetSelectionAnalyticsResponse);
}

// Request to generate a new environment (multi-room)
message GenerateEnvironmentRequest {
  // Optional environment ID - if empty, will be generated
  string environment_id = 1;
  
  // Environment generation configuration
  EnvironmentConfig config = 2;
  
  // Seed for reproducible generation (if 0, server will generate)
  int64 seed = 3;
  
  // Optional session context for grouping environments
  string session_id = 4;
  
  // TTL in seconds for environment data (default 3600 = 1 hour)
  int32 ttl_seconds = 5;
}

// Request to generate a single room
message GenerateRoomRequest {
  // Optional room ID - if empty, will be generated
  string room_id = 1;
  
  // Room generation configuration
  RoomConfig config = 2;
  
  // Seed for reproducible generation (if 0, server will generate)
  int64 seed = 3;
  
  // Optional session context for grouping rooms
  string session_id = 4;
  
  // TTL in seconds for room data (default 3600 = 1 hour)
  int32 ttl_seconds = 5;
}

// Response with generated environment data
message GenerateEnvironmentResponse {
  // The generated environment
  EnvironmentData environment = 1;
  
  // All rooms in the environment
  repeated RoomData rooms = 2;
  
  // All connections between rooms
  repeated ConnectionData connections = 3;
  
  // All entities across all rooms
  repeated EntityData entities = 4;
  
  // When this environment expires (Unix timestamp)
  int64 expires_at = 5;
  
  // Environment generation metadata
  EnvironmentGenerationMetadata metadata = 6;
}

// Response with generated room data  
message GenerateRoomResponse {
  // The generated room
  RoomData room = 1;
  
  // All entities in the room (walls, features, etc.)
  repeated EntityData entities = 2;
  
  // When this room expires (Unix timestamp)
  int64 expires_at = 3;
  
  // Generation metadata
  GenerationMetadata metadata = 4;
}

// Room configuration parameters
message RoomConfig {
  // Room dimensions
  int32 width = 1;         // Must be > 0, max 100
  int32 height = 2;        // Must be > 0, max 100
  
  // Grid system type
  GridType grid_type = 3;
  
  // Visual/thematic setting
  string theme = 4;        // "dungeon", "forest", "tavern", "outdoor", etc.
  
  // Wall generation parameters
  WallConfig wall_config = 5;
  
  // Optional room name for display
  string name = 6;
}

// Wall generation configuration
message WallConfig {
  // Wall placement pattern
  WallPattern pattern = 1;
  
  // Wall density (0.0 to 1.0)
  float density = 2;
  
  // Ratio of destructible walls (0.0 to 1.0)
  float destructible_ratio = 3;
  
  // Wall material type
  string material = 4;     // "stone", "wood", "metal", etc.
  
  // Wall height in grid units
  float height = 5;
}

// Grid system enumeration
enum GridType {
  GRID_TYPE_UNSPECIFIED = 0;
  GRID_TYPE_SQUARE = 1;      // D&D 5e style square grid
  GRID_TYPE_HEX = 2;         // Hexagonal grid
  GRID_TYPE_GRIDLESS = 3;    // Theater-of-mind, no grid
}

// Wall pattern enumeration
enum WallPattern {
  WALL_PATTERN_UNSPECIFIED = 0;
  WALL_PATTERN_EMPTY = 1;       // No internal walls
  WALL_PATTERN_RANDOM = 2;      // Procedural random placement
}

// Environment configuration parameters
message EnvironmentConfig {
  // Number of rooms to generate
  int32 room_count = 1;
  
  // Layout pattern for room connections
  EnvironmentLayoutType layout_type = 2;
  
  // Overall environment theme
  string theme = 3;
  
  // Generation approach
  GenerationType generation_type = 4;
  
  // Size and complexity constraints
  repeated GenerationConstraint constraints = 5;
  
  // Optional specific room configurations
  repeated RoomConfig room_configs = 6;
}

// Environment layout enumeration
enum EnvironmentLayoutType {
  ENVIRONMENT_LAYOUT_TYPE_UNSPECIFIED = 0;
  ENVIRONMENT_LAYOUT_TYPE_ORGANIC = 1;      // Natural/irregular connections
  ENVIRONMENT_LAYOUT_TYPE_LINEAR = 2;       // Sequential room chain
  ENVIRONMENT_LAYOUT_TYPE_BRANCHING = 3;    // Hub and spoke
  ENVIRONMENT_LAYOUT_TYPE_GRID = 4;         // Grid-based layout
  ENVIRONMENT_LAYOUT_TYPE_TOWER = 5;        // Vertical stacking
}

// Generation type enumeration
enum GenerationType {
  GENERATION_TYPE_UNSPECIFIED = 0;
  GENERATION_TYPE_GRAPH = 1;       // Graph-based generation
  GENERATION_TYPE_PREFAB = 2;      // Prefab-based generation
  GENERATION_TYPE_HYBRID = 3;      // Hybrid approach
}

// Generation constraint
message GenerationConstraint {
  // Constraint type
  string type = 1;  // "max_rooms", "max_size", "complexity_limit"
  
  // Constraint value
  int32 value = 2;
  
  // Human-readable description
  string description = 3;
}

// Generated environment data
message EnvironmentData {
  // Unique environment identifier
  string id = 1;
  
  // Display name
  string name = 2;
  
  // Overall theme
  string theme = 3;
  
  // Layout type used
  EnvironmentLayoutType layout_type = 4;
  
  // Number of rooms in environment
  int32 room_count = 5;
  
  // Custom environment properties
  map<string, string> properties = 6;
  
  // When environment was created (Unix timestamp)
  int64 created_at = 7;
  
  // Session this environment belongs to
  string session_id = 8;
}

// Connection between rooms
message ConnectionData {
  // Unique connection identifier
  string id = 1;
  
  // Connection type
  ConnectionType type = 2;
  
  // Source room ID
  string from_room_id = 3;
  
  // Target room ID
  string to_room_id = 4;
  
  // Position in source room
  Position from_position = 5;
  
  // Position in target room
  Position to_position = 6;
  
  // Connection properties
  map<string, string> properties = 7;
  
  // Whether connection works both ways
  bool bidirectional = 8;
}

// Connection type enumeration  
enum ConnectionType {
  CONNECTION_TYPE_UNSPECIFIED = 0;
  CONNECTION_TYPE_DOOR = 1;        // Standard doorway
  CONNECTION_TYPE_STAIRS = 2;      // Vertical connections
  CONNECTION_TYPE_PASSAGE = 3;     // Open corridors
  CONNECTION_TYPE_PORTAL = 4;      // Magical transport
  CONNECTION_TYPE_BRIDGE = 5;      // Spanning gaps
  CONNECTION_TYPE_TUNNEL = 6;      // Underground passages
}

// Generated room data
message RoomData {
  // Unique room identifier
  string id = 1;
  
  // Display name
  string name = 2;
  
  // Room dimensions
  int32 width = 3;
  int32 height = 4;
  
  // Grid system used
  GridType grid_type = 5;
  
  // Thematic setting
  string theme = 6;
  
  // Seed used for this room generation
  int64 seed = 7;
  
  // Custom room properties (flexible key-value pairs)
  map<string, string> properties = 8;
  
  // When room was created (Unix timestamp)
  int64 created_at = 9;
  
  // Session this room belongs to
  string session_id = 10;
}

// Entity within a room
message EntityData {
  // Unique entity identifier within room
  string id = 1;
  
  // Entity type classification
  EntityType entity_type = 2;
  
  // Position within room
  Position position = 3;
  
  // Entity-specific properties
  map<string, string> properties = 4;
  
  // Visual/gameplay state
  EntityState state = 5;
}

// Entity type enumeration
enum EntityType {
  ENTITY_TYPE_UNSPECIFIED = 0;
  ENTITY_TYPE_WALL = 1;
  ENTITY_TYPE_DOOR = 2;
  ENTITY_TYPE_FEATURE = 3;      // Generic room feature
  ENTITY_TYPE_SPAWN_POINT = 4;  // Future: creature spawn locations
}

// Entity state information
message EntityState {
  // Whether entity blocks movement
  bool blocks_movement = 1;
  
  // Whether entity blocks line of sight
  bool blocks_line_of_sight = 2;
  
  // Whether entity is destroyed/disabled
  bool destroyed = 3;
  
  // Current hit points (if destructible)
  optional int32 current_hp = 4;
  
  // Maximum hit points
  optional int32 max_hp = 5;
}

// 2D position within room
message Position {
  // X coordinate
  float x = 1;
  
  // Y coordinate  
  float y = 2;
}

// Environment generation metadata for debugging/analytics
message EnvironmentGenerationMetadata {
  // Master seed used for environment generation
  int64 seed = 1;
  
  // Per-room seeds for debugging/reproduction
  map<string, int64> room_seeds = 2;
  
  // Generation duration in milliseconds
  int32 generation_time_ms = 3;
  
  // Number of rooms created
  int32 room_count = 4;
  
  // Number of connections created
  int32 connection_count = 5;
  
  // Toolkit version used
  string toolkit_version = 6;
  
  // Layout complexity score (0.0 to 1.0)
  float layout_complexity = 7;
  
  // Generation type used
  GenerationType generation_type = 8;
}

// Room generation metadata for debugging/analytics
message GenerationMetadata {
  // Seed used for generation
  int64 seed = 1;
  
  // Generation duration in milliseconds
  int32 generation_time_ms = 2;
  
  // Number of entities created
  int32 entity_count = 3;
  
  // Toolkit version used
  string toolkit_version = 4;
}

// Request to get existing room
message GetRoomRequest {
  // Room identifier
  string room_id = 1;
}

// Response with room data
message GetRoomResponse {
  // Room data (empty if not found)
  optional RoomData room = 1;
  
  // Room entities (empty if room not found)
  repeated EntityData entities = 2;
  
  // Whether room was found
  bool found = 3;
}

// Request to list rooms
message ListRoomsRequest {
  // Optional session filter
  optional string session_id = 1;
  
  // Optional theme filter
  optional string theme = 2;
  
  // Pagination limit (max 100)
  int32 limit = 3;
  
  // Pagination offset
  int32 offset = 4;
}

// Response with room list
message ListRoomsResponse {
  // Room list
  repeated RoomData rooms = 1;
  
  // Total count (for pagination)
  int32 total_count = 2;
  
  // Whether more results exist
  bool has_more = 3;
}

// Request to delete room
message DeleteRoomRequest {
  // Room identifier
  string room_id = 1;
}

// Response from room deletion
message DeleteRoomResponse {
  // Success confirmation
  bool deleted = 1;
  
  // Number of entities deleted with room
  int32 entities_deleted = 2;
}

// === SPATIAL QUERY SERVICE MESSAGES ===

// Request to find entities within range
message QueryEntitiesInRangeRequest {
  // Room to query
  string room_id = 1;
  
  // Center position for range query
  Position center = 2;
  
  // Search radius
  float radius = 3;
  
  // Optional entity filter
  optional EntityFilter filter = 4;
}

// Response with entities in range
message QueryEntitiesInRangeResponse {
  // Entities found within range
  repeated EntityData entities = 1;
  
  // Total count of entities found
  int32 count = 2;
  
  // Query metadata
  SpatialQueryMetadata metadata = 3;
}

// Entity filtering options
message EntityFilter {
  // Filter by entity types
  repeated EntityType entity_types = 1;
  
  // Exclude specific entity IDs
  repeated string exclude_ids = 2;
  
  // Only entities with these states
  repeated string include_states = 3;
  
  // Exclude entities with these states  
  repeated string exclude_states = 4;
  
  // Match specific properties (key-value pairs)
  map<string, string> properties = 5;
}

// Request for line of sight query
message QueryLineOfSightRequest {
  // Room to query
  string room_id = 1;
  
  // Starting position
  Position from = 2;
  
  // Target position
  Position to = 3;
  
  // Entity IDs to ignore during LOS check
  repeated string ignore_ids = 4;
}

// Response with line of sight information
message QueryLineOfSightResponse {
  // Whether line of sight exists
  bool has_line_of_sight = 1;
  
  // ID of entity blocking LOS (if any)
  optional string blocking_entity_id = 2;
  
  // Positions along the LOS path
  repeated Position path_positions = 3;
  
  // Distance between positions
  float distance = 4;
  
  // Query metadata
  SpatialQueryMetadata metadata = 5;
}

// Request to validate movement
message ValidateMovementRequest {
  // Room to validate movement in
  string room_id = 1;
  
  // Entity attempting to move
  string entity_id = 2;
  
  // Starting position
  Position from = 3;
  
  // Target position
  Position to = 4;
  
  // Whether to check entire path or just destination
  bool check_path = 5;
}

// Response with movement validation
message ValidateMovementResponse {
  // Whether movement is valid
  bool is_valid = 1;
  
  // ID of blocking entity (if any)
  optional string blocked_by = 2;
  
  // Furthest valid position along path
  optional Position max_valid_position = 3;
  
  // Grid-based movement cost
  float movement_cost = 4;
  
  // Non-blocking warnings
  repeated string warnings = 5;
  
  // Query metadata
  SpatialQueryMetadata metadata = 6;
}

// Request to validate entity placement
message ValidateEntityPlacementRequest {
  // Room to place entity in
  string room_id = 1;
  
  // Entity to place
  string entity_id = 2;
  
  // Target position
  Position position = 3;
  
  // Size of entity (in grid squares)
  int32 entity_size = 4;
  
  // Check even if entity already placed
  bool force_check = 5;
}

// Response with placement validation
message ValidateEntityPlacementResponse {
  // Whether entity can be placed
  bool can_place = 1;
  
  // Entity IDs that would conflict
  repeated string conflicting_ids = 2;
  
  // Suggested nearby positions
  repeated Position alternative_positions = 3;
  
  // Reasons why placement failed
  repeated string reasons = 4;
  
  // Query metadata
  SpatialQueryMetadata metadata = 5;
}

// Metadata for spatial queries
message SpatialQueryMetadata {
  // Query execution time in milliseconds
  int32 execution_time_ms = 1;
  
  // Grid type used for calculations
  GridType grid_type = 2;
  
  // Number of entities considered
  int32 entities_processed = 3;
}

// === ROOM ANALYSIS SERVICE MESSAGES ===

// Request for room capacity analysis
message AnalyzeRoomCapacityRequest {
  // Room configuration to analyze
  RoomConfig room_config = 1;
  
  // Types of entities to analyze capacity for
  repeated EntityType entity_types = 2;
  
  // Desired number of entities
  int32 desired_count = 3;
}

// Response with capacity analysis
message AnalyzeRoomCapacityResponse {
  // Maximum theoretical capacity
  int32 max_capacity = 1;
  
  // Recommended entity count
  int32 recommended_count = 2;
  
  // Capacity breakdown by entity type
  map<string, int32> capacity_by_type = 3;
  
  // Density analysis
  RoomDensityAnalysis density_analysis = 4;
  
  // Capacity warnings
  repeated string warnings = 5;
  
  // Optimization recommendations
  repeated CapacityRecommendation recommendations = 6;
}

// Room density analysis
message RoomDensityAnalysis {
  // Current space utilization (0.0 to 1.0)
  float current_density = 1;
  
  // Recommended density (0.0 to 1.0)
  float optimal_density = 2;
  
  // Crowding risk level
  string crowding_risk = 3; // "low", "medium", "high"
  
  // Gameplay quality estimate (0.0 to 1.0)
  float playability_score = 4;
}

// Capacity optimization recommendation
message CapacityRecommendation {
  // Type of recommendation
  string type = 1; // "increase_size", "reduce_walls", "split_room"
  
  // Description of recommendation
  string description = 2;
  
  // Expected impact
  string impact = 3;
}

// Request for generation fallback options
message GetGenerationFallbacksRequest {
  // Configuration that failed to generate
  RoomConfig failed_config = 1;
  
  // Reason why generation failed
  string failure_reason = 2;
  
  // Hard constraints that cannot be changed
  repeated string constraints = 3;
}

// Response with fallback options
message GetGenerationFallbacksResponse {
  // Fallback configuration options
  repeated FallbackConfig fallback_configs = 1;
  
  // Last resort emergency configuration
  optional RoomConfig emergency_config = 2;
  
  // Whether recovery is possible
  bool can_recover = 3;
  
  // Explanation of recovery approach
  string recovery_strategy = 4;
}

// Fallback configuration option
message FallbackConfig {
  // Modified room configuration
  RoomConfig config = 1;
  
  // What was changed from original
  repeated string modifications = 2;
  
  // Expected quality (0.0 to 1.0)
  float quality_score = 3;
  
  // Reason for this fallback
  string reason_for_fallback = 4;
  
  // Priority (1 = highest)
  int32 priority = 5;
}

// === ENTITY SPAWNING PROTO MESSAGES ===

// Request to populate a room with entities
message PopulateRoomRequest {
  // Target room ID for spawning
  string room_id = 1;
  
  // Complete spawn configuration
  SpawnConfig spawn_config = 2;
  
  // Optional session context
  string session_id = 3;
}

// Response with spawn results
message PopulateRoomResponse {
  // Overall operation success
  bool success = 1;
  
  // Successfully spawned entities
  repeated SpawnedEntityData spawned_entities = 2;
  
  // Failed spawn attempts
  repeated SpawnFailureData failures = 3;
  
  // Room modifications made during spawning
  repeated RoomModification room_modifications = 4;
  
  // Room splitting recommendations
  repeated RoomSplitData split_recommendations = 5;
  
  // Operation metadata
  SpawnMetadata metadata = 6;
}

// Request to populate an environment with entities
message PopulateEnvironmentRequest {
  // Target environment ID
  string environment_id = 1;
  
  // Spawn configuration for multi-room distribution
  SpawnConfig spawn_config = 2;
  
  // Optional session context
  string session_id = 3;
}

// Response with environment spawn results
message PopulateEnvironmentResponse {
  // Overall operation success
  bool success = 1;
  
  // All spawned entities across rooms
  repeated SpawnedEntityData spawned_entities = 2;
  
  // Failed spawn attempts
  repeated SpawnFailureData failures = 3;
  
  // Room modifications made during spawning
  repeated RoomModification room_modifications = 4;
  
  // Room splitting recommendations
  repeated RoomSplitData split_recommendations = 5;
  
  // Entity distribution by room ID
  map<string, SpawnedEntityList> room_distribution = 6;
  
  // Operation metadata
  SpawnMetadata metadata = 7;
}

// Request to populate split rooms with entities
message PopulateSplitRoomsRequest {
  // Connected room IDs
  repeated string room_ids = 1;
  
  // Spawn configuration for distributed spawning
  SpawnConfig spawn_config = 2;
  
  // Optional session context
  string session_id = 3;
}

// Response with split room spawn results
message PopulateSplitRoomsResponse {
  // Overall operation success
  bool success = 1;
  
  // All spawned entities across rooms
  repeated SpawnedEntityData spawned_entities = 2;
  
  // Failed spawn attempts
  repeated SpawnFailureData failures = 3;
  
  // Room modifications made during spawning
  repeated RoomModification room_modifications = 4;
  
  // Additional room splitting recommendations
  repeated RoomSplitData split_recommendations = 5;
  
  // Entity distribution by room ID
  map<string, SpawnedEntityList> room_distribution = 6;
  
  // Operation metadata
  SpawnMetadata metadata = 7;
}

// Complete spawn configuration
message SpawnConfig {
  // Entity groups to spawn
  repeated EntityGroup entity_groups = 1;
  
  // Spawn pattern type
  SpawnPattern pattern = 2;
  
  // Team-based configuration (optional)
  TeamConfig team_configuration = 3;
  
  // Spatial positioning constraints
  SpatialConstraints spatial_rules = 4;
  
  // General placement rules
  PlacementRules placement = 5;
  
  // Spawn strategy approach
  SpawnStrategy strategy = 6;
  
  // Adaptive scaling configuration (optional)
  ScalingConfig adaptive_scaling = 7;
  
  // Player spawn zones (optional)
  repeated SpawnZone player_spawn_zones = 8;
  
  // Player spawn choices (optional)
  repeated PlayerSpawnChoice player_choices = 9;
}

// Entity group definition
message EntityGroup {
  // Unique group identifier
  string id = 1;
  
  // Entity category (enemy, ally, treasure, etc.)
  string type = 2;
  
  // Entity selection table ID
  string selection_table = 3;
  
  // Quantity specification
  QuantitySpec quantity = 4;
}

// Flexible quantity specification
message QuantitySpec {
  // Exact count (most common)
  optional int32 fixed = 1;
  
  // Future: dice roll notation
  optional string dice_roll = 2;
  
  // Future: range specification
  optional int32 min = 3;
  optional int32 max = 4;
}

// Available spawn patterns
enum SpawnPattern {
  SPAWN_PATTERN_UNSPECIFIED = 0;
  SPAWN_PATTERN_SCATTERED = 1;     // Random distribution
  SPAWN_PATTERN_FORMATION = 2;     // Geometric arrangements
  SPAWN_PATTERN_TEAM_BASED = 3;    // Team separation
  SPAWN_PATTERN_PLAYER_CHOICE = 4; // Player-selected positions
  SPAWN_PATTERN_CLUSTERED = 5;     // Grouped placement
}

// Spatial positioning constraints
message SpatialConstraints {
  // Type-to-type distance requirements (e.g., "guard:treasure" -> 3.0)
  map<string, float> min_distance = 1;
  
  // Line of sight requirements
  LineOfSightRules line_of_sight = 2;
  
  // Distance from walls
  float wall_proximity = 3;
  
  // Area of effect exclusion zones
  map<string, float> area_of_effect = 4;
}

// Line of sight rules
message LineOfSightRules {
  // Entity pairs that MUST see each other
  repeated EntityPair required_sight = 1;
  
  // Entity pairs that must NOT see each other
  repeated EntityPair blocked_sight = 2;
}

// Entity type pair reference
message EntityPair {
  // Source entity type
  string from = 1;
  
  // Target entity type
  string to = 2;
}

// Successfully placed entity
message SpawnedEntityData {
  // Unique entity identifier
  string entity_id = 1;
  
  // Entity category
  string entity_type = 2;
  
  // Final placement position
  Position position = 3;
  
  // Room where entity was placed
  string room_id = 4;
  
  // Entity group that spawned this
  string group_id = 5;
  
  // Entity-specific properties
  map<string, string> properties = 6;
}

// Failed spawn attempt
message SpawnFailureData {
  // Failed entity type
  string entity_type = 1;
  
  // Entity group that failed
  string group_id = 2;
  
  // Specific failure reason
  string reason = 3;
  
  // Positions that were attempted
  repeated Position attempted_positions = 4;
}

// Room modification during spawning
message RoomModification {
  // Modification type ("scaled", "rotated", "split", etc.)
  string type = 1;
  
  // Affected room identifier
  string room_id = 2;
  
  // Previous value (JSON-encoded)
  string old_value = 3;
  
  // New value (JSON-encoded) 
  string new_value = 4;
  
  // Justification for modification
  string reason = 5;
}

// Room splitting recommendation
message RoomSplitData {
  // Room that should be split
  string original_room_id = 1;
  
  // Proposed split configurations
  repeated RoomSplitPlan suggested_splits = 2;
  
  // Why splitting is recommended
  string reason = 3;
  
  // Urgency (1 = highest priority)
  int32 priority = 4;
}

// Spawn operation metadata
message SpawnMetadata {
  // Total placement attempts made
  int32 total_attempts = 1;
  
  // Percentage of successful placements (0.0 to 1.0)
  float success_rate = 2;
  
  // Average attempts per successful placement
  float average_attempts = 3;
  
  // Time taken for spawn operation
  int32 processing_time_ms = 4;
  
  // Number of constraint violations encountered
  int32 constraint_violations = 5;
  
  // Number of rooms that were modified
  int32 rooms_modified = 6;
}

// Helper message for room distribution
message SpawnedEntityList {
  repeated SpawnedEntityData entities = 1;
}

// === SPAWN CONFIGURATION AND VALIDATION MESSAGES ===

// Request to validate spawn configuration
message ValidateSpawnConfigurationRequest {
  // Configuration to validate
  SpawnConfig spawn_config = 1;
  
  // Optional single room context
  optional string room_id = 2;
  
  // Optional multi-room context
  repeated string room_ids = 3;
}

// Response with validation results
message ValidateSpawnConfigurationResponse {
  // Whether configuration is valid
  bool is_valid = 1;
  
  // Specific validation failures
  repeated ValidationError validation_errors = 2;
  
  // Non-critical issues
  repeated ValidationWarning warnings = 3;
  
  // Optimization suggestions
  repeated SpawnRecommendation recommendations = 4;
  
  // Predicted spawn outcomes
  SpawnEstimate estimated_results = 5;
}

// Request for spawn recommendations
message GetSpawnRecommendationsRequest {
  // Optional single room context
  optional string room_id = 1;
  
  // Optional multi-room context
  repeated string room_ids = 2;
  
  // What the spawn should achieve
  SpawnObjective desired_outcome = 3;
  
  // Hard limits and requirements
  repeated SpawnConstraint constraints = 4;
  
  // Additional game state information
  GameContextData game_context = 5;
}

// Response with AI-driven recommendations
message GetSpawnRecommendationsResponse {
  // Suggested spawn configurations
  repeated SpawnConfigRecommendation recommendations = 1;
  
  // Alternative approaches
  repeated SpawnConfigRecommendation alternatives = 2;
  
  // Potential issues
  repeated string warnings = 3;
  
  // Analysis details
  RecommendationMetadata metadata = 4;
}

// === ENTITY TABLE MANAGEMENT MESSAGES ===

// Request to register entity table
message RegisterEntityTableRequest {
  // Unique identifier for table
  string table_id = 1;
  
  // Available entities for selection
  repeated EntityDefinition entities = 2;
  
  // Optional selection weights by entity ID
  map<string, float> weights = 3;
  
  // Optional session scoping
  optional string session_id = 4;
}

// Response confirming table registration
message RegisterEntityTableResponse {
  // Confirmed table ID
  string table_id = 1;
  
  // Number of entities registered
  int32 entity_count = 2;
  
  // Registration success status
  bool success = 3;
}

// Request to retrieve entity tables
message GetEntityTablesRequest {
  // Optional session filtering
  optional string session_id = 1;
  
  // Optional specific table IDs to retrieve
  repeated string table_ids = 2;
}

// Response with available tables
message GetEntityTablesResponse {
  // Available entity tables
  repeated EntityTableInfo tables = 1;
  
  // Additional table information
  TableMetadata metadata = 2;
}

// Entity table information
message EntityTableInfo {
  // Table identifier
  string table_id = 1;
  
  // Number of entities in table
  int32 entity_count = 2;
  
  // Available entity types
  repeated string entity_types = 3;
  
  // Last update timestamp
  int64 last_updated = 4;
}

// === SELECTABLES PROTO MESSAGES ===

// Request to create a weighted selection table
message CreateSelectionTableRequest {
  // Unique table identifier
  string table_id = 1;
  
  // Human-readable table name
  string name = 2;
  
  // Table description/purpose
  string description = 3;
  
  // Type of items (loot, quests, events, etc.)
  string item_type = 4;
  
  // Initial items with weights
  repeated SelectableItem items = 5;
  
  // Table behavior configuration
  TableConfiguration configuration = 6;
  
  // Optional session scoping
  optional string session_id = 7;
}

// Response confirming table creation
message CreateSelectionTableResponse {
  // Confirmed table identifier
  string table_id = 1;
  
  // Number of items in table
  int32 item_count = 2;
  
  // Sum of all item weights
  int32 total_weight = 3;
  
  // Creation success status
  bool success = 4;
}

// Request to update selection table
message UpdateSelectionTableRequest {
  // Table to update
  string table_id = 1;
  
  // Items to add to table
  repeated SelectableItem add_items = 2;
  
  // Item IDs to remove
  repeated string remove_items = 3;
  
  // Items to update (by ID)
  repeated SelectableItem update_items = 4;
  
  // Optional configuration updates
  optional TableConfiguration configuration = 5;
}

// Response with updated table information
message UpdateSelectionTableResponse {
  // Updated table identifier
  string table_id = 1;
  
  // New item count
  int32 item_count = 2;
  
  // New total weight
  int32 total_weight = 3;
  
  // Update success status
  bool success = 4;
}

// Request to delete selection table
message DeleteSelectionTableRequest {
  // Table ID to delete
  string table_id = 1;
}

// Response confirming table deletion
message DeleteSelectionTableResponse {
  // Deleted table ID
  string table_id = 1;
  
  // Deletion success status
  bool success = 2;
}

// Request for single weighted selection
message SelectFromTableRequest {
  // Table to select from
  string table_id = 1;
  
  // Game state context for weighted selection
  SelectionContext selection_context = 2;
  
  // Selection behavior options
  SelectionOptions options = 3;
}

// Response with single selection result
message SelectFromTableResponse {
  // The selected item (JSON-encoded)
  string selected_item = 1;
  
  // Weight of selected item at selection time
  float selection_weight = 2;
  
  // Random roll result that determined selection
  int32 roll_result = 3;
  
  // Alternative items that could have been selected
  map<string, float> alternatives = 4;
  
  // Selection operation metadata
  SelectionMetadata metadata = 5;
}

// Request for multiple weighted selections
message SelectManyFromTableRequest {
  // Table to select from
  string table_id = 1;
  
  // Number of selections to make
  int32 count = 2;
  
  // Game state context
  SelectionContext selection_context = 3;
  
  // Selection behavior options
  SelectionOptions options = 4;
}

// Response with multiple selection results
message SelectManyFromTableResponse {
  // All selected items (JSON-encoded)
  repeated string selected_items = 1;
  
  // Weight of each selected item
  repeated float selection_weights = 2;
  
  // Random rolls for each selection
  repeated int32 roll_results = 3;
  
  // Alternative selections available
  map<string, float> alternatives = 4;
  
  // Operation metadata
  SelectionMetadata metadata = 5;
}

// Request for unique weighted selections
message SelectUniqueFromTableRequest {
  // Table to select from
  string table_id = 1;
  
  // Number of unique selections
  int32 count = 2;
  
  // Game state context
  SelectionContext selection_context = 3;
  
  // Selection options
  SelectionOptions options = 4;
}

// Response with unique selection results
message SelectUniqueFromTableResponse {
  // Unique selected items (JSON-encoded)
  repeated string selected_items = 1;
  
  // Weight of each selected item
  repeated float selection_weights = 2;
  
  // Random rolls for selections
  repeated int32 roll_results = 3;
  
  // Items that could have been selected
  map<string, float> alternatives = 4;
  
  // Items still available after selection
  repeated string remaining_items = 5;
  
  // Operation metadata
  SelectionMetadata metadata = 6;
}

// Request for variable quantity selection
message SelectVariableFromTableRequest {
  // Table to select from
  string table_id = 1;
  
  // Dice expression for quantity (e.g., "1d4+1", "2d6")
  string dice_expression = 2;
  
  // Game state context
  SelectionContext selection_context = 3;
  
  // Selection options
  SelectionOptions options = 4;
}

// Response with variable selection results
message SelectVariableFromTableResponse {
  // Selected items (quantity determined by dice)
  repeated string selected_items = 1;
  
  // Dice roll result that determined quantity
  int32 dice_result = 2;
  
  // Weight of each selected item
  repeated float selection_weights = 3;
  
  // Random rolls for each selection
  repeated int32 roll_results = 4;
  
  // Alternative selections available
  map<string, float> alternatives = 5;
  
  // Operation metadata
  SelectionMetadata metadata = 6;
}

// Request to list selection tables
message ListSelectionTablesRequest {
  // Optional session filtering
  optional string session_id = 1;
  
  // Optional item type filtering
  optional string item_type = 2;
  
  // Optional name pattern matching
  optional string name_pattern = 3;
  
  // Optional result limit
  optional int32 limit = 4;
  
  // Optional pagination offset
  optional int32 offset = 5;
}

// Response with table listing
message ListSelectionTablesResponse {
  // Available selection tables
  repeated SelectionTableInfo tables = 1;
  
  // Total tables matching filter
  int32 total_count = 2;
  
  // Whether more results available
  bool has_more = 3;
  
  // Additional listing metadata
  TableListMetadata metadata = 4;
}

// Request for detailed table information
message GetSelectionTableInfoRequest {
  // Table ID to get information about
  string table_id = 1;
  
  // Whether to include full item list
  bool include_items = 2;
}

// Response with detailed table information
message GetSelectionTableInfoResponse {
  // Table identifier
  string table_id = 1;
  
  // Table name
  string name = 2;
  
  // Table description
  string description = 3;
  
  // Type of items in table
  string item_type = 4;
  
  // Number of items
  int32 item_count = 5;
  
  // Sum of item weights
  int32 total_weight = 6;
  
  // Full item list (if requested)
  repeated SelectableItem items = 7;
  
  // Table configuration
  TableConfiguration configuration = 8;
  
  // Creation timestamp
  int64 created_at = 9;
  
  // Last modification timestamp
  int64 last_updated = 10;
  
  // Usage statistics
  TableUsageStats usage_stats = 11;
}

// Request for selection analytics
message GetSelectionAnalyticsRequest {
  // Table to analyze
  string table_id = 1;
  
  // Optional time range for analytics
  optional TimeRange time_range = 2;
  
  // Analysis granularity ("hourly", "daily", "weekly")
  string granularity = 3;
}

// Response with selection analytics
message GetSelectionAnalyticsResponse {
  // Analyzed table
  string table_id = 1;
  
  // Total selections made
  int64 selection_count = 2;
  
  // Per-item selection statistics
  repeated ItemSelectionStats item_stats = 3;
  
  // Time-based selection data
  repeated SelectionDataPoint time_series_data = 4;
  
  // Most frequently selected items
  repeated TopSelectionItem top_selections = 5;
  
  // Context usage analysis
  ContextUsageAnalysis context_analysis = 6;
  
  // Analytics metadata
  AnalyticsMetadata metadata = 7;
}

// === SELECTABLES CORE TYPES ===

// Item that can be selected from table
message SelectableItem {
  // Unique item identifier
  string id = 1;
  
  // The actual item (JSON-encoded for flexibility)
  string content = 2;
  
  // Base selection weight
  float weight = 3;
  
  // Context-based weight modifications
  repeated WeightCondition conditions = 4;
  
  // Additional item metadata (JSON-encoded)
  string metadata = 5;
  
  // Whether item is available for selection
  bool enabled = 6;
}

// Game state context for weighted selection
message SelectionContext {
  // Character level
  int32 player_level = 1;
  
  // Current game location
  string location = 2;
  
  // Character class
  string player_class = 3;
  
  // Completed quest IDs
  repeated string completed_quests = 4;
  
  // Reputation by faction (faction -> reputation)
  map<string, int32> player_reputation = 5;
  
  // General game state (JSON-encoded)
  string game_state = 6;
  
  // Session-specific data
  map<string, string> session_data = 7;
  
  // Context creation timestamp
  int64 timestamp = 8;
}

// Conditional weight modification
message WeightCondition {
  // Context key to check
  string context_key = 1;
  
  // Comparison operator ("eq", "gt", "lt", "gte", "lte", "in")
  string operator = 2;
  
  // Value to compare against (JSON-encoded)
  string value = 3;
  
  // How to modify weight if condition matches
  WeightModifier modifier = 4;
}

// Weight modification specification
message WeightModifier {
  // Modification type ("multiply", "add", "override", "disable")
  string type = 1;
  
  // Modifier value
  float value = 2;
  
  // Human-readable explanation
  string reason = 3;
}

// Selection behavior options
message SelectionOptions {
  // Whether to publish selection events
  bool enable_events = 1;
  
  // Whether to record selection for analytics
  bool enable_analytics = 2;
  
  // Whether to log context state
  bool context_logging = 3;
  
  // Whether to include alternatives in result
  bool return_alternatives = 4;
  
  // Maximum retry attempts on selection failure
  int32 max_retries = 5;
}

// Table behavior configuration
message TableConfiguration {
  // Cache weight calculations
  bool enable_caching = 1;
  
  // Publish selection events
  bool enable_events = 2;
  
  // Cache timeout in seconds
  int32 cache_timeout = 3;
  
  // Max selection retry attempts
  int32 max_retries = 4;
  
  // Record selections for analytics
  bool analytics_enabled = 5;
}

// Selection operation metadata
message SelectionMetadata {
  // Operation type
  string operation_type = 1;
  
  // Time taken for selection
  int32 selection_time_ms = 2;
  
  // Context hash for debugging
  string context_hash = 3;
  
  // Weight calculations performed
  int32 weight_calculations = 4;
  
  // Cache hits during selection
  int32 cache_hits = 5;
  
  // Random rolls made
  int32 roll_attempts = 6;
  
  // Events published
  int32 events_published = 7;
}

// Basic selection table information
message SelectionTableInfo {
  // Table identifier
  string table_id = 1;
  
  // Table name
  string name = 2;
  
  // Table description
  string description = 3;
  
  // Type of items
  string item_type = 4;
  
  // Number of items
  int32 item_count = 5;
  
  // Sum of weights
  int32 total_weight = 6;
  
  // Last selection timestamp
  int64 last_used = 7;
  
  // Total selections made
  int64 usage_count = 8;
  
  // Creation timestamp
  int64 created_at = 9;
}

// === ANALYTICS TYPES ===

// Table usage statistics
message TableUsageStats {
  // Total selections from table
  int64 total_selections = 1;
  
  // Most recent selection timestamp
  int64 last_selection = 2;
  
  // Average weight of selected items
  float average_weight = 3;
  
  // Most common context pattern
  string most_common_context = 4;
  
  // Selections per hour (recent activity)
  float selection_rate = 5;
}

// Per-item selection statistics
message ItemSelectionStats {
  // Item identifier
  string item_id = 1;
  
  // Times item was selected
  int64 selection_count = 2;
  
  // Selection rate relative to weight
  float selection_rate = 3;
  
  // Most recent selection timestamp
  int64 last_selected = 4;
  
  // Average effective weight at selection time
  float average_weight = 5;
}

// Time-series selection data point
message SelectionDataPoint {
  // Data point timestamp
  int64 timestamp = 1;
  
  // Selections in time period
  int32 selection_count = 2;
  
  // Unique items selected
  int32 unique_items = 3;
  
  // Average selection weight
  float average_weight = 4;
}

// Most frequently selected item
message TopSelectionItem {
  // Item identifier
  string item_id = 1;
  
  // Total selections
  int64 selection_count = 2;
  
  // Percentage of total selections
  float percentage = 3;
  
  // Trend direction ("up", "down", "stable")
  string trend_direction = 4;
}

// Context usage analysis
message ContextUsageAnalysis {
  // Context keys that most affect selections
  repeated string most_influential_keys = 1;
  
  // Common context combinations
  repeated ContextPattern context_patterns = 2;
  
  // Most common weight modifications
  repeated WeightModificationStat weight_modifications = 3;
}

// Common context value combination
message ContextPattern {
  // Context key-value pattern (JSON-encoded)
  string pattern = 1;
  
  // How often this pattern occurs
  int64 frequency = 2;
  
  // How pattern biases selections (item -> bias)
  map<string, float> selection_bias = 3;
}

// Weight modification statistics
message WeightModificationStat {
  // Type of condition causing modification
  string condition_type = 1;
  
  // Average weight change
  float average_effect = 2;
  
  // How often modification occurs
  int64 frequency = 3;
  
  // Number of different items affected
  int32 items_affected = 4;
}

// Table listing metadata
message TableListMetadata {
  // Whether filters were applied
  bool filter_applied = 1;
  
  // Result limit used
  int32 result_limit = 2;
  
  // Result offset used
  int32 result_offset = 3;
}

// Time range for analytics
message TimeRange {
  // Start timestamp
  int64 start_time = 1;
  
  // End timestamp
  int64 end_time = 2;
}

// Analytics metadata
message AnalyticsMetadata {
  // Analysis type performed
  string analysis_type = 1;
  
  // Analysis confidence score
  float confidence = 2;
  
  // Analysis factors considered
  repeated string factors = 3;
}
```

### 7. Entity Data Structure

Define generic entity format for room contents:

```go
type EntityData struct {
    ID       string
    Type     string            // "wall", "door", "feature", etc.
    Position PositionData
    Properties map[string]interface{}  // Flexible per-entity data
}

type PositionData struct {
    X float64
    Y float64
}

type RoomData struct {
    ID          string
    Name        string
    Width       int32
    Height      int32
    GridType    string
    Theme       string
    Properties  map[string]interface{}
}
```

## Implementation Order

Following rpg-api's "outside-in" approach with proper multi-repository workflow:

### Phase 1: Proto Contract Definition (rpg-api-protos)
1. **Design proto contract**: Create `api/v1alpha1/room.proto` with service definition
2. **Generate and test bindings**: Generate Go/TypeScript bindings, verify compilation
3. **Publish proto changes**: Commit, tag, and publish updated proto bindings
4. **Update rpg-api imports**: Update rpg-api go.mod to use new proto version

**Dependencies**: None - this is the contract foundation
**Success**: rpg-api can import and reference new proto types

### Phase 2: Handler Shell (rpg-api) 
1. **Import updated protos**: Update go.mod with new proto bindings
2. **Create handler shell**: `internal/handlers/api/v1alpha1/room_handler.go` with `codes.Unimplemented`
3. **Register service**: Add RoomService to server registration
4. **Verify endpoints**: Test that gRPC endpoints are accessible

**Dependencies**: Phase 1 complete
**Success**: Client can call endpoints and receive unimplemented errors

### Phase 3: Service Contracts (rpg-api)
1. **Define service interface**: `internal/orchestrators/room/service.go` with Input/Output types
2. **Generate mocks**: Create mocks for service interface
3. **Handler tests**: Write comprehensive handler tests using mocked service
4. **Validate mapping**: Ensure proto ↔ internal type conversion works

**Dependencies**: Phase 2 complete
**Success**: Handler layer fully tested with mocked dependencies

### Phase 4: Engine Integration (rpg-api + rpg-toolkit)
1. **Add toolkit dependencies**: 
   ```bash
   go get github.com/KirkDiggler/rpg-toolkit/tools/spatial@latest
   go get github.com/KirkDiggler/rpg-toolkit/tools/environments@latest
   go get github.com/KirkDiggler/rpg-toolkit/tools/selectables@latest
   ```
2. **Extend engine interface**: Add all spatial methods to `internal/engine/interface.go`
   - Room generation methods
   - Spatial query methods (entities in range, line of sight, movement validation)
   - Capacity analysis and fallback methods
3. **Implement adapter**: Extend `internal/engine/rpgtoolkit/adapter.go` with full toolkit integration
   - Room generation using spatial + environments 
   - Spatial queries using toolkit's query system
   - Capacity analysis using environments scaling capabilities
   - Fallback handling using environments emergency patterns
4. **Engine tests**: Integration tests with real toolkit components for all capabilities

**Dependencies**: Phase 3 complete, specific toolkit module versions
**Success**: Engine exposes all toolkit spatial and scaling capabilities

### Phase 5: Repository Implementation (rpg-api)
1. **Create repository package**: `internal/repositories/room/` with interface + Redis implementation
2. **Repository tests**: Test Redis storage/retrieval with miniredis
3. **Integration patterns**: Follow existing character/dice repository patterns

**Dependencies**: Phase 4 complete
**Success**: Room data persists correctly in Redis

### Phase 6: Orchestrator Implementation (rpg-api)
1. **Implement orchestrator**: Wire engine + repository + business logic
2. **Orchestrator tests**: Full business logic testing with mocked dependencies
3. **Handler integration**: Connect handlers to real orchestrator
4. **End-to-end tests**: Complete flow from gRPC request to persistence

**Dependencies**: Phases 4 & 5 complete
**Success**: Complete room generation API works

### Phase 7: Client Validation
1. **Generate client bindings**: Ensure all target languages work (Go, TypeScript)
2. **Example implementations**: Sample client code for different platforms
3. **Performance validation**: Meet success metrics (<200ms generation)
4. **Documentation**: Usage examples and integration guides

**Dependencies**: Phase 6 complete
**Success**: Clients can successfully consume the API

## Multi-Repository Workflow Considerations

### Proto-First Development
- **Critical**: rpg-api-protos changes must be committed, tagged, and published before rpg-api can use them
- **Versioning**: Use semantic versioning for proto releases (v1.2.0, v1.2.1, etc.)
- **Binding Generation**: Both Go and TypeScript bindings must compile successfully
- **Breaking Changes**: Follow proto evolution best practices for field additions/removals

### Dependency Management
- **Toolkit Modules**: Add only specific modules needed (spatial, environments, selectables)
- **Version Pinning**: Use specific versions, not @latest in production
- **Module Isolation**: Follow rpg-toolkit's development guidelines - no go.work files
- **Update Strategy**: Test toolkit updates in isolation before integrating

### Testing Strategy Across Repos
- **Proto Testing**: Validate generated bindings compile and work
- **Engine Testing**: Test toolkit integration with real modules, not mocks
- **Handler Testing**: Use mocked services for rapid iteration
- **E2E Testing**: Full flow testing across all layers

### Development Environment
```bash
# Required directory structure
/projects/
├── rpg-toolkit/      # For understanding toolkit capabilities
├── rpg-api/          # Primary development repo
├── rpg-api-protos/   # For proto definition changes
```

### Deployment Dependencies
- **Proto bindings** must be published and available
- **Toolkit modules** must be at stable versions  
- **rpg-api** can then be built and deployed independently

## Consequences

### Benefits
- **Modular Extension**: No changes to existing character/dice systems
- **Toolkit Powered**: Leverages rpg-toolkit's proven spatial architecture  
- **Generic Foundation**: Supports any RPG system, not just D&D
- **Storage Flexible**: Redis today, database tomorrow
- **Client Agnostic**: Same API for Discord, web, mobile

#### **Spatial Query Capabilities Enable Rich Gameplay**:
- **Line of Sight**: Combat mechanics, spell targeting, stealth gameplay
- **Movement Validation**: Turn-based tactical movement, pathfinding assistance
- **Range Queries**: Area of effect spells, perception checks, threat detection
- **Placement Validation**: Token placement, spawn point validation, furniture arrangement

#### **Fallback and Scaling Ensure Reliability**:
- **Capacity Analysis**: Prevents overcrowded or empty rooms, optimizes gameplay density
- **Generation Fallbacks**: Graceful degradation when complex rooms fail to generate
- **Emergency Patterns**: Always produces playable room, even under constraints
- **Performance Scaling**: Toolkit handles room complexity limits automatically

#### **Production-Ready Architecture**:
- **Query Performance**: Toolkit's spatial indexing optimized for gameplay queries
- **Memory Efficiency**: Smart entity management, TTL-based cleanup
- **Error Recovery**: Multiple fallback levels prevent generation failures
- **Analytics**: Generation metadata for performance monitoring and optimization

### Trade-offs
- **Complexity**: Adds new orchestrator, repository, handler layers
- **Dependencies**: Increases rpg-toolkit dependency surface area
- **Storage**: Room data + entities requires more Redis memory
- **Learning Curve**: Developers need spatial/environments knowledge

### Migration Path
- Zero impact on existing character/dice functionality
- New endpoints are additive, not breaking
- Can be developed and deployed incrementally
- Easy rollback if issues arise

## Success Metrics

1. **Functional**: Client can generate empty room and display it
2. **Performance**: <200ms room generation for 20x20 rooms  
3. **Reliability**: Room data persists correctly across requests
4. **Flexibility**: Easy to add new room types and features
5. **Testing**: >80% coverage on orchestrator layer

## Future Extensions

This foundation enables:
- **Entity Spawning**: Add monsters, NPCs, treasure to generated rooms
- **Environmental Interactions**: Wall destruction, trap triggering  
- **Multi-Room Dungeons**: Connected rooms with pathfinding
- **Real-time Updates**: Room state changes broadcast to clients
- **Collaborative Building**: Multiple users editing same room

## References
- ADR-001: Foundation and Standards
- ADR-003: Proto Management Strategy
- rpg-toolkit spatial module documentation
- rpg-toolkit environments module documentation