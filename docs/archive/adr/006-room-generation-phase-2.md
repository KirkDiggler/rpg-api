# ADR-006: Room Generation Phase 2 - Core Engine Interface

**Status**: Proposed  
**Date**: 2025-07-22  
**Decision-makers**: Kirk Diggler, Claude AI  
**Technical story**: [Issue #108](https://github.com/KirkDiggler/rpg-api/issues/108) - Phase 2: Core Engine Interface

## Summary

We need to implement basic room generation functionality using the rpg-toolkit environments module and essential spatial queries for tactical gameplay. This phase establishes the core engine interface extension and integrates both environments and spatial toolkit modules.

## Problem Statement

### Current State
- Phase 1 (Proto definitions) is complete with ADR-002 in rpg-api-protos
- Handler shells exist but return `codes.Unimplemented`
- No engine interface exists for room generation
- No toolkit integration for room generation functionality

### Target State  
- Engine interface extended with room generation methods
- environments toolkit successfully integrated in adapter layer
- Essential spatial queries working (line of sight, movement validation, placement validation)
- Single room generation working through complete stack
- Service interfaces defined following rpg-api Input/Output patterns

## Decision

**We will extend the engine interface with room generation methods and essential spatial queries, integrating both environments and spatial toolkit modules for complete room generation and basic tactical capabilities.**

### Core Design Principles

1. **Engine Extension Pattern**: Follow existing engine patterns for consistent interface design
2. **Dual Toolkit Integration**: Use environments module for room generation, spatial module for tactical queries  
3. **Input/Output Consistency**: All methods use structured Input/Output types per rpg-api standards
4. **Essential Spatial First**: Include core spatial queries games need during room generation
5. **Event Integration**: Integrate toolkit events with existing rpg-api event system

## Architecture

### Engine Interface Extension

```go
// internal/engine/interface.go - Extended engine interface
type Engine interface {
    // Existing character validation methods...
    ValidateCharacterDraft(ctx context.Context, input *ValidateCharacterDraftInput) (*ValidateCharacterDraftOutput, error)
    
    // NEW: Room generation methods
    GenerateRoom(ctx context.Context, input *GenerateRoomInput) (*GenerateRoomOutput, error)
    GetRoomDetails(ctx context.Context, input *GetRoomDetailsInput) (*GetRoomDetailsOutput, error)
    
    // NEW: Essential spatial queries
    QueryLineOfSight(ctx context.Context, input *QueryLineOfSightInput) (*QueryLineOfSightOutput, error)
    ValidateMovement(ctx context.Context, input *ValidateMovementInput) (*ValidateMovementOutput, error)
    ValidateEntityPlacement(ctx context.Context, input *ValidateEntityPlacementInput) (*ValidateEntityPlacementOutput, error)
    QueryEntitiesInRange(ctx context.Context, input *QueryEntitiesInRangeInput) (*QueryEntitiesInRangeOutput, error)
}
```

### Input/Output Type Design

Following rpg-api patterns with comprehensive structured types:

```go
// internal/engine/types.go - Room generation types

type GenerateRoomInput struct {
    Config    RoomConfig `json:"config"`
    Seed      int64      `json:"seed"`      // Required for reproducibility
    SessionID string     `json:"session_id,omitempty"` // Optional game session context
    TTL       *int32     `json:"ttl,omitempty"`        // Optional TTL override
}

type GenerateRoomOutput struct {
    Room      *RoomData              `json:"room"`
    Entities  []EntityData           `json:"entities"`
    Metadata  GenerationMetadata     `json:"metadata"`
    SessionID string                 `json:"session_id"`
    ExpiresAt time.Time              `json:"expires_at"`
}

type RoomConfig struct {
    Width       int32   `json:"width"`         // Room width in grid units
    Height      int32   `json:"height"`        // Room height in grid units
    Theme       string  `json:"theme"`         // "dungeon", "forest", "urban", etc.
    WallDensity float64 `json:"wall_density"`  // 0.0-1.0 wall coverage
    Pattern     string  `json:"pattern"`       // "empty", "random", "clustered"
    GridType    string  `json:"grid_type"`     // "square", "hex_pointy", "hex_flat", "gridless"
    GridSize    float64 `json:"grid_size"`     // 5.0 for D&D 5ft squares
}

type GetRoomDetailsInput struct {
    RoomID string `json:"room_id"`
}

type GetRoomDetailsOutput struct {
    Room     *RoomData `json:"room"`
    Entities []EntityData `json:"entities"`
    Metadata RoomMetadata `json:"metadata"`
}
```

### Entity Data Structure

```go
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

type EntityData struct {
    ID         string                 `json:"id"`
    Type       string                 `json:"type"`        // "wall", "door", "monster", "character"
    Position   Position               `json:"position"`
    Properties map[string]interface{} `json:"properties"`  // Size, material, health, etc.
    Tags       []string               `json:"tags"`        // "destructible", "blocking", "cover"
    State      EntityState            `json:"state"`
}

type Position struct {
    X float64 `json:"x"`
    Y float64 `json:"y"`
    Z float64 `json:"z,omitempty"` // Optional for 3D support
}

type EntityState struct {
    BlocksMovement     bool  `json:"blocks_movement"`
    BlocksLineOfSight  bool  `json:"blocks_line_of_sight"`
    Destroyed          bool  `json:"destroyed"`
    CurrentHP          int32 `json:"current_hp,omitempty"`
    MaxHP              int32 `json:"max_hp,omitempty"`
}

// Essential Spatial Query Types

type QueryLineOfSightInput struct {
    RoomID      string   `json:"room_id"`
    FromX       float64  `json:"from_x"`
    FromY       float64  `json:"from_y"`
    ToX         float64  `json:"to_x"`
    ToY         float64  `json:"to_y"`
    EntitySize  float64  `json:"entity_size,omitempty"`  // Size for collision detection
    IgnoreTypes []string `json:"ignore_types,omitempty"` // Entity types to ignore
}

type QueryLineOfSightOutput struct {
    HasLineOfSight    bool      `json:"has_line_of_sight"`
    BlockingEntityID  *string   `json:"blocking_entity_id,omitempty"`
    BlockingPosition  *Position `json:"blocking_position,omitempty"`
    Distance          float64   `json:"distance"`           // Actual distance
    PathPositions     []Position `json:"path_positions"`   // LOS ray positions
}

type ValidateMovementInput struct {
    RoomID        string  `json:"room_id"`
    EntityID      string  `json:"entity_id"`          // Entity attempting movement
    FromX         float64 `json:"from_x"`
    FromY         float64 `json:"from_y"`
    ToX           float64 `json:"to_x"`
    ToY           float64 `json:"to_y"`
    EntitySize    float64 `json:"entity_size,omitempty"`
    MaxDistance   float64 `json:"max_distance,omitempty"`
}

type ValidateMovementOutput struct {
    IsValid           bool      `json:"is_valid"`
    BlockedBy         *string   `json:"blocked_by,omitempty"`     // Entity ID blocking path
    BlockingPosition  *Position `json:"blocking_position,omitempty"`
    MovementCost      float64   `json:"movement_cost"`            // Cost in movement points
    ActualDistance    float64   `json:"actual_distance"`          // Calculated distance
}

type ValidateEntityPlacementInput struct {
    RoomID       string                 `json:"room_id"`
    EntityID     string                 `json:"entity_id,omitempty"`    // For updates
    EntityType   string                 `json:"entity_type"`
    Position     Position               `json:"position"`
    Size         float64                `json:"size"`
    Properties   map[string]interface{} `json:"properties,omitempty"`
    Tags         []string               `json:"tags,omitempty"`
}

type ValidateEntityPlacementOutput struct {
    CanPlace          bool      `json:"can_place"`
    ConflictingIDs    []string  `json:"conflicting_ids"`    // Conflicting entity IDs
    SuggestedPositions []Position `json:"suggested_positions"` // Alternative positions
    PlacementIssues   []PlacementIssue `json:"placement_issues"`
}

type PlacementIssue struct {
    Type        string   `json:"type"`        // "collision", "out_of_bounds", "invalid_terrain"
    Description string   `json:"description"`
    Position    Position `json:"position"`
    Severity    string   `json:"severity"`    // "error", "warning", "info"
}

type QueryEntitiesInRangeInput struct {
    RoomID          string   `json:"room_id"`
    CenterX         float64  `json:"center_x"`
    CenterY         float64  `json:"center_y"`
    Range           float64  `json:"range"`
    EntityTypes     []string `json:"entity_types,omitempty"`     // Filter by type
    Tags            []string `json:"tags,omitempty"`             // Filter by tags
    ExcludeEntityID string   `json:"exclude_entity_id,omitempty"` // Exclude specific entity
}

type QueryEntitiesInRangeOutput struct {
    Entities    []EntityResult `json:"entities"`
    TotalFound  int32          `json:"total_found"`
    QueryCenter Position       `json:"query_center"`
    QueryRange  float64        `json:"query_range"`
}

type EntityResult struct {
    Entity       EntityData `json:"entity"`
    Distance     float64    `json:"distance"`        // Distance from query center
    Direction    float64    `json:"direction"`       // Angle from center (radians)
    RelativePos  string     `json:"relative_pos"`    // "north", "southeast", etc.
}
```

### Service Interface Design

```go
// internal/services/room/service.go - Room service interface

//go:generate mockgen -destination=mock/mock_service.go -package=roommock github.com/KirkDiggler/rpg-api/internal/services/room Service

type Service interface {
    // Basic room generation
    GenerateRoom(ctx context.Context, input *GenerateRoomInput) (*GenerateRoomOutput, error)
    GetRoom(ctx context.Context, input *GetRoomInput) (*GetRoomOutput, error)
    
    // Essential spatial queries
    QueryLineOfSight(ctx context.Context, input *QueryLineOfSightInput) (*QueryLineOfSightOutput, error)
    ValidateMovement(ctx context.Context, input *ValidateMovementInput) (*ValidateMovementOutput, error)  
    ValidateEntityPlacement(ctx context.Context, input *ValidateEntityPlacementInput) (*ValidateEntityPlacementOutput, error)
    QueryEntitiesInRange(ctx context.Context, input *QueryEntitiesInRangeInput) (*QueryEntitiesInRangeOutput, error)
}

// Service Input/Output types (may differ slightly from engine types)
type GenerateRoomInput struct {
    EntityID  string     `json:"entity_id"`  // Required: room owner
    Config    RoomConfig `json:"config"`     // Room generation configuration  
    Seed      int64      `json:"seed"`       // Required for reproducibility
    SessionID string     `json:"session_id,omitempty"`
    TTL       *int32     `json:"ttl,omitempty"`
}

type GenerateRoomOutput struct {
    Room *entities.Room `json:"room"`  // Uses internal entity types
}

type GetRoomInput struct {
    RoomID   string `json:"room_id"`
    EntityID string `json:"entity_id"` // For ownership validation
}

type GetRoomOutput struct {
    Room     *entities.Room   `json:"room"`
    Entities []entities.Entity `json:"entities"`
}
```

## Implementation Strategy

### Phase 2a: Engine Interface Extension

**Deliverable**: Extended engine interface with room generation methods

**Tasks**:
1. Extend `internal/engine/interface.go` with new methods
2. Define comprehensive Input/Output types in `internal/engine/types.go`
3. Add method documentation following existing patterns
4. Update engine mock generation

### Phase 2b: Toolkit Integration

**Deliverable**: environments and spatial toolkit integrated in adapter layer

**Tasks**:
1. Add toolkit dependencies: 
   - `go get github.com/KirkDiggler/rpg-toolkit/tools/environments@latest`
   - `go get github.com/KirkDiggler/rpg-toolkit/tools/spatial@latest`
2. Extend `internal/engine/rpgtoolkit/adapter.go` with dual toolkit integration
3. Implement `GenerateRoom` method using `environments.RoomBuilder`
4. Implement essential spatial queries using `spatial` module
5. Handle toolkit events and integrate with rpg-api event system
6. Implement proper error handling and validation for both modules

### Phase 2c: Service Interface Definition  

**Deliverable**: Complete service interface with room generation and spatial Input/Output types

**Tasks**:
1. Create `internal/services/room/service.go` with interface definition
2. Define service-layer Input/Output types for room generation and spatial queries
3. Generate mocks with `//go:generate mockgen`
4. Follow existing service patterns from character and dice services
5. Ensure spatial query types are optimized for frequent tactical use

### Phase 2d: Integration Testing

**Deliverable**: Validated room generation and essential spatial queries through engine layer

**Tasks**:
1. Create integration tests for engine interface (room generation and spatial)
2. Test room generation with various configurations
3. Test essential spatial queries (line of sight, movement validation, placement)
4. Validate Input/Output type conversions for both modules
5. Test error scenarios and edge cases for spatial calculations
6. Verify performance meets tactical gameplay requirements (<50ms for basic queries)

## Technical Decisions

### Decision 1: Dual Toolkit Integration Strategy

**Problem**: Should we integrate only environments module or include spatial capabilities in Phase 2?

**Decision**: Integrate both environments and spatial modules for complete tactical room generation.

**Rationale**:
- Games need spatial queries immediately during room generation, not in a later phase
- environments module provides room building, spatial module provides tactical queries
- Essential spatial queries (line of sight, movement validation) are needed for room validation
- Advanced spatial features (pathfinding, multi-room) can wait until Phase 4
- This provides complete single-room tactical capability in Phase 2

### Decision 2: Engine Interface Extension Pattern  

**Problem**: How should room generation integrate with existing engine interface?

**Decision**: Extend the existing engine interface with room generation methods.

**Rationale**:
- Maintains consistency with existing character validation patterns
- Allows mock generation and testing using established patterns
- Single engine interface provides coherent API for all game mechanics
- Follows single responsibility principle with focused method signatures

### Decision 3: Comprehensive Input/Output Types

**Problem**: What level of detail should Input/Output types contain?

**Decision**: Use comprehensive structured types with all necessary fields.

**Rationale**:
- Follows established rpg-api pattern from character and dice services
- Enables future extension without interface changes
- Supports rich metadata for debugging and analytics
- Allows proper validation and error handling

### Decision 4: Seed Management Strategy

**Problem**: How should we handle random seeds for reproducible generation?

**Decision**: Require seeds in input, return actual seed used in output.

**Rationale**:
- Enables reproducible room generation for consistent gameplay
- If input seed is 0, engine generates and returns actual seed used
- Supports debugging by allowing replay of exact generation
- Aligns with toolkit's seed-based generation philosophy

### Decision 5: Event System Integration

**Problem**: How should toolkit events integrate with rpg-api event system?

**Decision**: Translate toolkit events to rpg-api event patterns in adapter layer.

**Rationale**:
- Maintains separation between toolkit and rpg-api event systems
- Allows rpg-api-specific event handling and routing
- Enables proper event logging and monitoring
- Supports future event system evolution

## Alternatives Considered

### Alternative 1: Direct Spatial Module Integration

**Pros**:
- More direct control over positioning and grid management
- Potentially simpler integration with fewer dependencies

**Cons**:  
- Would require reimplementing room building logic
- Misses wall pattern generation and room orchestration features
- Duplicates functionality already in environments module
- Increases maintenance burden

### Alternative 2: Monolithic Room Service Interface

**Pros**:
- Single service interface for all room functionality
- Simpler client integration

**Cons**:
- Violates single responsibility principle
- Makes testing more complex
- Harder to evolve different aspects independently
- Conflicts with rpg-api's focused service pattern

### Alternative 3: Optional Seed Parameters

**Pros**:
- More flexible API for clients who don't care about reproducibility
- Simpler request structures

**Cons**:
- Non-deterministic behavior by default
- Makes debugging and testing harder
- Conflicts with toolkit's deterministic generation philosophy
- Reduces reproducibility for gameplay consistency

## Implementation Requirements

### Functional Requirements
1. **Basic Room Generation**: Create tactical rooms with configurable size, theme, and wall patterns
2. **Essential Spatial Queries**: Line of sight, movement validation, placement validation, range queries
3. **Grid System Support**: Support all grid types (square, hex, gridless) with proper positioning
4. **Entity Generation**: Generate appropriate entities (walls, doors) based on configuration
5. **Seed Reproducibility**: Same seed + config produces identical rooms
6. **Tactical Validation**: Validate entity placement and movement during room setup
7. **Ownership Integration**: Rooms owned by entities following rpg-api patterns

### Non-Functional Requirements  
1. **Performance**: Room generation completes in <100ms, spatial queries in <50ms
2. **Error Handling**: Comprehensive validation and error reporting for both modules
3. **Testability**: Full test coverage with integration tests for room generation and spatial queries
4. **Documentation**: Complete method and type documentation for dual toolkit integration
5. **Consistency**: Follows established rpg-api patterns and conventions
6. **Spatial Accuracy**: Calculations match tabletop game expectations

### Integration Requirements
1. **Proto Compatibility**: Engine types convert properly to/from proto messages
2. **Repository Ready**: Types suitable for persistence in upcoming Phase 3
3. **Service Layer**: Clean service interface for orchestrator integration
4. **Event Integration**: Proper event handling and routing

## Success Criteria

### Phase 2 Complete When:
1. ✅ Engine interface extended with room generation and essential spatial methods
2. ✅ Both environments and spatial toolkits successfully integrated in adapter
3. ✅ Single room generation works through engine layer  
4. ✅ Essential spatial queries work (line of sight, movement validation, placement, range)
5. ✅ Service interface defined with proper Input/Output types for both modules
6. ✅ Integration tests passing for room generation and spatial query scenarios
7. ✅ All types properly documented and following rpg-api patterns
8. ✅ Performance targets met for both room generation and spatial queries

### Validation Tests:
- Generate room with seed 12345, verify identical results on repeated calls
- Generate rooms with different themes (dungeon, forest, urban)
- Generate rooms with different grid types (square, hex, gridless)
- Test line of sight blocked by walls, clear sight lines work
- Test movement validation prevents walking through walls
- Test entity placement validation prevents overlapping entities
- Test range queries find correct entities within distance
- Handle invalid configurations gracefully with proper error messages
- Verify entity ownership patterns work correctly
- Performance tests: room generation <100ms, spatial queries <50ms

## Dependencies

### Upstream Dependencies
- **Phase 1 Complete**: Proto definitions and handler shells exist
- **rpg-toolkit**: environments module at stable version

### Downstream Impact
- **Phase 3**: Repository layer will use types defined here
- **Phase 4**: Advanced spatial queries will extend engine interface further
- **Handlers**: Will call service interfaces defined here
- **Games**: Can implement complete single-room tactical scenarios immediately

## Related Decisions

- **[ADR-001]**: Established patterns for Input/Output types and service interfaces
- **[ADR-002 (protos)]**: Proto message definitions that engine types must support
- **[ADR-005]**: Original comprehensive room generation requirements
- **[Issue #108]**: Phase 2 implementation details and task breakdown

## References

- [Issue #108](https://github.com/KirkDiggler/rpg-api/issues/108) - Phase 2: Core Engine Interface
- [Epic #107](https://github.com/KirkDiggler/rpg-api/issues/107) - Room Generation Integration
- [ADR-005](./005-room-generation-integration.md) - Original room generation requirements
- [rpg-toolkit environments module](https://github.com/KirkDiggler/rpg-toolkit/tree/main/tools/environments)
- [Journey 002](../journey/002-room-generation-phase-planning.md) - Phase planning and breakdown
