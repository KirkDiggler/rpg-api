# ADR-009: Room Generation Phase 4 - Spatial Query System

**Status**: Proposed  
**Date**: 2025-07-22  
**Decision-makers**: Kirk Diggler, Claude AI  
**Technical story**: [Issue #110](https://github.com/KirkDiggler/rpg-api/issues/110) - Phase 4: Spatial Query System

## Summary

We need to implement advanced spatial features including multi-room queries, complex pathfinding, area of effect calculations, and performance optimization. This phase extends the essential spatial queries from Phase 2 with advanced tactical capabilities.

## Problem Statement

### Current State
- Phase 3 provides basic room CRUD operations with persistence
- Phase 2 provides essential spatial queries (line of sight, movement validation, placement, range)
- Single-room spatial queries work but no multi-room capabilities
- No advanced pathfinding, area effects, or spatial optimization

### Target State  
- Multi-room spatial queries work across connected environments
- Advanced pathfinding with intelligent route calculation
- Area of effect queries for spell/ability targeting
- Advanced spatial optimization with multi-level caching
- Complex tactical scenarios supported (shooting through doorways, etc.)

## Decision

**We will implement advanced spatial features including multi-room queries, pathfinding algorithms, area effects, and performance optimization to complete the tactical spatial system.**

### Core Design Principles

1. **Advanced Spatial Features**: Build on Phase 2's essential queries with complex capabilities
2. **Multi-Room Focus**: Enable spatial operations across connected room environments
3. **Performance Optimization**: Advanced caching and spatial indexing for high-frequency queries
4. **Complex Tactical Support**: Area effects, advanced pathfinding, and multi-entity operations

## Architecture

### Engine Interface Extension

```go
// internal/engine/interface.go - Extended with spatial methods

type Engine interface {
    // Existing methods from Phase 2...
    GenerateRoom(ctx context.Context, input *GenerateRoomInput) (*GenerateRoomOutput, error)
    QueryLineOfSight(ctx context.Context, input *QueryLineOfSightInput) (*QueryLineOfSightOutput, error)
    ValidateMovement(ctx context.Context, input *ValidateMovementInput) (*ValidateMovementOutput, error)
    ValidateEntityPlacement(ctx context.Context, input *ValidateEntityPlacementInput) (*ValidateEntityPlacementOutput, error)
    QueryEntitiesInRange(ctx context.Context, input *QueryEntitiesInRangeInput) (*QueryEntitiesInRangeOutput, error)
    
    // NEW: Advanced spatial query methods  
    CalculateMovementPath(ctx context.Context, input *CalculateMovementPathInput) (*CalculateMovementPathOutput, error)
    QueryAreaOfEffect(ctx context.Context, input *QueryAreaOfEffectInput) (*QueryAreaOfEffectOutput, error)
    QueryMultiRoomLineOfSight(ctx context.Context, input *QueryMultiRoomLineOfSightInput) (*QueryMultiRoomLineOfSightOutput, error)
    CalculateMultiRoomPath(ctx context.Context, input *CalculateMultiRoomPathInput) (*CalculateMultiRoomPathOutput, error)
    QuerySpatialIndex(ctx context.Context, input *QuerySpatialIndexInput) (*QuerySpatialIndexOutput, error)
}
```

### Advanced Spatial Input/Output Types

```go
// internal/engine/types.go - Advanced spatial query types (extends Phase 2 types)

// Enhanced Movement Path types with terrain effects
type MovementPath struct {
    Positions      []Position      `json:"positions"`
    TotalCost      float64         `json:"total_cost"`
    Description    string          `json:"description"`   // Human-readable path description
    TerrainEffects []TerrainEffect `json:"terrain_effects"`  // NEW: Detailed terrain
    PathType       string          `json:"path_type"`      // NEW: "optimal", "safe", "direct"
}

type TerrainEffect struct {
    Position    Position `json:"position"`
    Type        string   `json:"type"`        // "difficult", "hazardous", "impassable"
    Multiplier  float64  `json:"multiplier"`  // Movement cost multiplier
    Description string   `json:"description"`
    Source      string   `json:"source"`      // Entity/effect causing terrain
}

// Advanced Movement Path Calculation (extends Phase 2 basic movement validation)
type CalculateMovementPathInput struct {
    RoomID          string   `json:"room_id"`
    EntityID        string   `json:"entity_id"`
    FromX           float64  `json:"from_x"`
    FromY           float64  `json:"from_y"`
    ToX             float64  `json:"to_x"`
    ToY             float64  `json:"to_y"`
    EntitySize      float64  `json:"entity_size"`
    MovementType    string   `json:"movement_type"`      // "walk", "fly", "teleport", "burrow"
    MaxCost         float64  `json:"max_cost,omitempty"`
    PathPreference  string   `json:"path_preference"`    // "optimal", "safe", "direct", "cover"
    AvoidEntityTypes []string `json:"avoid_entity_types"` // Prefer paths avoiding these
    AllowDifficult  bool     `json:"allow_difficult"`    // Allow difficult terrain
    MaxAlternatives int32    `json:"max_alternatives"`   // Number of alternative paths
}

type CalculateMovementPathOutput struct {
    Path             MovementPath   `json:"path"`
    IsComplete       bool           `json:"is_complete"`      // Whether path reaches destination
    ReachableArea    []Position     `json:"reachable_area"`   // All positions within range
    Alternatives     []MovementPath `json:"alternatives"`     // Alternative paths
    PathfindingStats PathfindingStats `json:"pathfinding_stats"` // Performance metrics
}

type PathfindingStats struct {
    NodesExplored    int32   `json:"nodes_explored"`
    ComputationTime  float64 `json:"computation_time_ms"`
    Algorithm        string  `json:"algorithm"`     // "A*", "Dijkstra", "Jump Point"
    CacheHitRatio    float64 `json:"cache_hit_ratio"`
}

// Advanced Area of Effect Queries
type QueryAreaOfEffectInput struct {
    RoomID        string   `json:"room_id"`
    CenterX       float64  `json:"center_x"`
    CenterY       float64  `json:"center_y"`
    EffectType    string   `json:"effect_type"`     // "circle", "cone", "line", "rectangle", "custom"
    Radius        float64  `json:"radius,omitempty"`
    Width         float64  `json:"width,omitempty"`
    Length        float64  `json:"length,omitempty"`
    Direction     float64  `json:"direction,omitempty"` // For cones/lines (radians)
    EntityTypes   []string `json:"entity_types,omitempty"`
    RequireLOS    bool     `json:"require_los"`
    CustomShape   []Position `json:"custom_shape,omitempty"`  // For complex spell shapes
    ExcludeCenter bool     `json:"exclude_center"`           // Exclude origin position
    CoverRules    string   `json:"cover_rules"`              // "strict", "partial", "ignore"
}

type QueryAreaOfEffectOutput struct {
    AffectedEntities  []AdvancedEntityResult `json:"affected_entities"`
    AreaPositions     []Position     `json:"area_positions"`     // All positions in AoE
    TotalArea         float64        `json:"total_area"`         // Area coverage
    EffectCenter      Position       `json:"effect_center"`
    CoverAnalysis     []CoverResult  `json:"cover_analysis"`     // Detailed cover info
    SecondaryEffects  []SecondaryEffect `json:"secondary_effects"` // Chain reactions, etc.
}

type AdvancedEntityResult struct {
    Entity       EntityData  `json:"entity"`
    Distance     float64     `json:"distance"`
    LineOfSight  bool        `json:"line_of_sight"`
    Direction    float64     `json:"direction"`
    RelativePos  string      `json:"relative_pos"`
    CoverType    string      `json:"cover_type"`    // "none", "partial", "full"
    CoverPercent float64     `json:"cover_percent"`
    EffectStrength float64   `json:"effect_strength"` // 0.0-1.0 based on distance/cover
}

type CoverResult struct {
    Position      Position `json:"position"`
    CoverType     string   `json:"cover_type"`
    CoverSource   string   `json:"cover_source"`   // Entity providing cover
    CoverPercent  float64  `json:"cover_percent"`
}

type SecondaryEffect struct {
    Type        string   `json:"type"`        // "ricochet", "explosion", "chain"
    Origin      Position `json:"origin"`
    Targets     []string `json:"targets"`     // Affected entity IDs
    Intensity   float64  `json:"intensity"`   // Secondary effect strength
    Description string   `json:"description"`
}

// Multi-Room Spatial Queries
type QueryMultiRoomLineOfSightInput struct {
    FromRoomID    string   `json:"from_room_id"`
    ToRoomID      string   `json:"to_room_id,omitempty"` // Omit for auto-detection
    FromX         float64  `json:"from_x"`
    FromY         float64  `json:"from_y"`
    ToX           float64  `json:"to_x"`
    ToY           float64  `json:"to_y"`
    EntitySize    float64  `json:"entity_size,omitempty"`
    MaxRange      float64  `json:"max_range,omitempty"`    // Stop if beyond range
    CrossRooms    bool     `json:"cross_rooms"`           // Allow crossing room boundaries
}

type QueryMultiRoomLineOfSightOutput struct {
    HasLineOfSight    bool              `json:"has_line_of_sight"`
    BlockingEntityID  *string           `json:"blocking_entity_id,omitempty"`
    BlockingPosition  *Position         `json:"blocking_position,omitempty"`
    RoomsCrossed      []string          `json:"rooms_crossed"`      // Room IDs in path
    ConnectionPoints  []ConnectionPoint `json:"connection_points"` // Doorways/passages used
    TotalDistance     float64           `json:"total_distance"`
    PathPositions     []Position        `json:"path_positions"`
}

type ConnectionPoint struct {
    RoomID       string   `json:"room_id"`
    ConnectedTo  string   `json:"connected_to"`
    Position     Position `json:"position"`
    Type         string   `json:"type"`         // "door", "passage", "window"
    IsBlocked    bool     `json:"is_blocked"`
    BlockingEntity *string `json:"blocking_entity,omitempty"`
}

// Multi-Room Pathfinding
type CalculateMultiRoomPathInput struct {
    FromRoomID      string   `json:"from_room_id"`
    ToRoomID        string   `json:"to_room_id,omitempty"`
    FromX           float64  `json:"from_x"`
    FromY           float64  `json:"from_y"`
    ToX             float64  `json:"to_x"`
    ToY             float64  `json:"to_y"`
    EntityID        string   `json:"entity_id"`
    EntitySize      float64  `json:"entity_size"`
    MovementType    string   `json:"movement_type"`
    MaxTotalCost    float64  `json:"max_total_cost,omitempty"`
    AllowDoorOpening bool    `json:"allow_door_opening"`
    PreferKnownPaths bool    `json:"prefer_known_paths"`
}

type CalculateMultiRoomPathOutput struct {
    Path              MultiRoomPath  `json:"path"`
    IsComplete        bool           `json:"is_complete"`
    RoomTransitions   []RoomTransition `json:"room_transitions"`
    TotalDistance     float64        `json:"total_distance"`
    EstimatedTime     float64        `json:"estimated_time_seconds"`
    RequiredActions   []PathAction   `json:"required_actions"`  // "open_door", "climb", etc.
}

type MultiRoomPath struct {
    Segments     []PathSegment `json:"segments"`     // One per room
    TotalCost    float64       `json:"total_cost"`
    Description  string        `json:"description"`
}

type PathSegment struct {
    RoomID      string     `json:"room_id"`
    Positions   []Position `json:"positions"`
    EnterPoint  *Position  `json:"enter_point,omitempty"`
    ExitPoint   *Position  `json:"exit_point,omitempty"`
    SegmentCost float64    `json:"segment_cost"`
}

type RoomTransition struct {
    FromRoomID   string   `json:"from_room_id"`
    ToRoomID     string   `json:"to_room_id"`
    ConnectionID string   `json:"connection_id"`
    Position     Position `json:"position"`
    Type         string   `json:"type"`
    RequiredAction string `json:"required_action,omitempty"`
}

type PathAction struct {
    Type        string   `json:"type"`        // "open_door", "climb", "jump"
    Position    Position `json:"position"`
    Target      string   `json:"target,omitempty"`      // Entity to interact with
    Difficulty  string   `json:"difficulty"`           // "trivial", "easy", "hard"
    Time        float64  `json:"time_seconds"`
    Description string   `json:"description"`
}

// Spatial Index Queries (Performance optimization)
type QuerySpatialIndexInput struct {
    RoomID      string   `json:"room_id"`
    IndexType   string   `json:"index_type"`   // "quadtree", "r_tree", "grid"
    BoundingBox BoundingBox `json:"bounding_box"`
    EntityTypes []string `json:"entity_types,omitempty"`
    Tags        []string `json:"tags,omitempty"`
    Precision   string   `json:"precision"`    // "fast", "accurate", "exhaustive"
}

type QuerySpatialIndexOutput struct {
    Entities       []EntityResult   `json:"entities"`
    IndexStats     IndexStats       `json:"index_stats"`
    QueryRegion    BoundingBox      `json:"query_region"`
    TotalCandidates int32           `json:"total_candidates"` // Before filtering
    FilteredResults int32           `json:"filtered_results"` // After filtering
}

type BoundingBox struct {
    MinX float64 `json:"min_x"`
    MinY float64 `json:"min_y"`
    MaxX float64 `json:"max_x"`
    MaxY float64 `json:"max_y"`
}

type IndexStats struct {
    IndexType       string  `json:"index_type"`
    BuildTime       float64 `json:"build_time_ms"`
    QueryTime       float64 `json:"query_time_ms"`
    CacheHitRatio   float64 `json:"cache_hit_ratio"`
    IndexMemorySize int64   `json:"index_memory_bytes"`
    LastUpdated     string  `json:"last_updated"`
}
```

### Service Interface Extension

```go
// internal/services/room/service.go - Extended with spatial methods

type Service interface {
    // Existing operations from Phases 2-3...
    GenerateRoom(ctx context.Context, input *GenerateRoomInput) (*GenerateRoomOutput, error)
    GetRoom(ctx context.Context, input *GetRoomInput) (*GetRoomOutput, error)
    QueryLineOfSight(ctx context.Context, input *QueryLineOfSightInput) (*QueryLineOfSightOutput, error)
    ValidateMovement(ctx context.Context, input *ValidateMovementInput) (*ValidateMovementOutput, error)  
    ValidateEntityPlacement(ctx context.Context, input *ValidateEntityPlacementInput) (*ValidateEntityPlacementOutput, error)
    QueryEntitiesInRange(ctx context.Context, input *QueryEntitiesInRangeInput) (*QueryEntitiesInRangeOutput, error)
    
    // NEW: Advanced spatial query operations
    CalculateMovementPath(ctx context.Context, input *CalculateMovementPathInput) (*CalculateMovementPathOutput, error)
    QueryAreaOfEffect(ctx context.Context, input *QueryAreaOfEffectInput) (*QueryAreaOfEffectOutput, error)
    QueryMultiRoomLineOfSight(ctx context.Context, input *QueryMultiRoomLineOfSightInput) (*QueryMultiRoomLineOfSightOutput, error)
    CalculateMultiRoomPath(ctx context.Context, input *CalculateMultiRoomPathInput) (*CalculateMultiRoomPathOutput, error)
    QuerySpatialIndex(ctx context.Context, input *QuerySpatialIndexInput) (*QuerySpatialIndexOutput, error)
}
```

## Implementation Strategy

### Phase 4a: Advanced Spatial Engine Extension

**Deliverable**: Advanced spatial methods added to engine adapter

**Tasks**:
1. Extend engine interface with advanced spatial query methods
2. Implement advanced pathfinding algorithms in adapter
3. Add area of effect calculations with complex shapes
4. Implement multi-room spatial query coordination
5. Add spatial indexing for performance optimization

**Key Features**:
- Advanced pathfinding with terrain analysis and path preferences
- Complex area of effect shapes with cover analysis
- Multi-room line of sight and pathfinding across connections
- Spatial indexing (quadtree, R-tree) for high-performance queries

### Phase 4b: Multi-Room Spatial Coordination

**Deliverable**: Multi-room spatial queries and pathfinding

**Tasks**:
1. Implement multi-room line of sight calculations
2. Add cross-room pathfinding with door/passage handling
3. Create room connection management and validation
4. Implement spatial orchestrator for multi-room coordination
5. Add room transition analysis and action requirements

**Advanced Features**:
- Spatial queries that span multiple connected rooms
- Intelligent pathfinding across room boundaries with door opening
- Complex tactical scenarios (shooting through doorways, multi-room spells)
- Room connection caching and optimization

### Phase 4c: Performance Optimization and Caching

**Deliverable**: Multi-level spatial caching and indexing system

**Tasks**:
1. Implement advanced spatial caching strategies
2. Add spatial indexing with quadtree/R-tree structures
3. Create cache invalidation strategies for dynamic entities
4. Implement query result caching with intelligent expiration
5. Add performance monitoring and spatial query analytics

**Optimization Features**:
- Multi-level caching (spatial structures, query results, index data)
- Intelligent cache invalidation based on entity changes
- Performance metrics and query optimization suggestions
- Memory usage optimization for large room environments

### Phase 4d: Handler Implementation and Analytics

**Deliverable**: gRPC handlers for advanced spatial queries with analytics

**Tasks**:
1. Implement advanced spatial query gRPC handlers
2. Add comprehensive validation for complex spatial parameters
3. Implement streaming responses for large query results
4. Add spatial query analytics and performance monitoring
5. Implement proper error handling for multi-room spatial operations
6. Add structured logging for advanced spatial query debugging

## Technical Decisions

### Decision 1: Spatial Toolkit Integration Approach

**Problem**: How should the spatial toolkit be integrated with existing room structures?

**Decision**: Create spatial room representations on-demand and cache them.

**Rationale**:
- Avoids duplicating room data in different formats
- Enables performance optimization through caching
- Allows spatial toolkit to work with its preferred data structures
- Supports dynamic room modifications without sync issues

**Synergy with Seed-Based Persistence** (from ADR-008):
- **Perfect match**: Spatial caching + seed-based persistence = optimal performance
- **Reconstruction on cache miss**: Load seed + config → regenerate room → cache spatial representation
- **Memory vs Storage trade-off**: High-performance spatial cache, lightweight Redis storage
- **Invalidation strategy**: Cache invalidation only needed for dynamic entity changes, not wall modifications

**Implementation**:
```go
// Spatial room cache in adapter
type spatialRoomCache struct {
    rooms map[string]spatial.Room
    mutex sync.RWMutex
}

func (a *Adapter) getSpatialRoom(roomID string) (spatial.Room, error) {
    a.spatialCache.mutex.RLock()
    if room, exists := a.spatialCache.rooms[roomID]; exists {
        a.spatialCache.mutex.RUnlock()
        return room, nil
    }
    a.spatialCache.mutex.RUnlock()
    
    // Load from repository and convert
    roomData, entities, err := a.loadRoomData(roomID)
    if err != nil {
        return nil, err
    }
    
    spatialRoom := a.convertToSpatialRoom(roomData, entities)
    
    a.spatialCache.mutex.Lock()
    a.spatialCache.rooms[roomID] = spatialRoom
    a.spatialCache.mutex.Unlock()
    
    return spatialRoom, nil
}
```

### Decision 2: Query Performance Optimization

**Problem**: How should frequent spatial queries be optimized for real-time gameplay?

**Decision**: Multi-level caching with intelligent cache invalidation.

**Rationale**:
- Tactical queries are frequently repeated (line of sight checks)
- Room structure changes are infrequent compared to query frequency
- Cache hit rates should be very high for tactical gameplay
- Memory usage is acceptable for improved query performance

**Caching Strategy**:
- **Level 1**: Spatial room structures (as above)
- **Level 2**: Query result caching for repeated identical queries
- **Level 3**: Spatial index caching for range queries
- **Invalidation**: Cache invalidation on room/entity updates

### Decision 3: Multi-Room Spatial Queries

**Problem**: How should spatial queries work across connected rooms?

**Decision**: Use spatial orchestrator pattern for multi-room coordination.

**Rationale**:
- Enables complex queries that span room boundaries
- Supports entity movement between connected rooms
- Allows for advanced tactical scenarios (shooting through doorways)
- Maintains separation of concerns between rooms and spatial logic

**Implementation Pattern**:
```go
func (a *Adapter) QueryLineOfSight(ctx context.Context, input *QueryLineOfSightInput) (*QueryLineOfSightOutput, error) {
    // Check if query spans multiple rooms
    if a.isMultiRoomQuery(input) {
        return a.handleMultiRoomLineOfSight(ctx, input)
    }
    
    // Single room query (optimized path)
    spatialRoom, err := a.getSpatialRoom(input.RoomID)
    if err != nil {
        return nil, err
    }
    
    return a.calculateLineOfSight(spatialRoom, input)
}
```

### Decision 4: Grid System Consistency

**Problem**: How should different grid systems be handled consistently?

**Decision**: Use position normalization with grid-specific calculations.

**Rationale**:
- Input/Output positions are always in world coordinates (float64)
- Grid-specific calculations happen in spatial toolkit
- Consistent API regardless of underlying grid type
- Enables rooms with different grid types to interoperate

### Decision 5: Error Handling for Spatial Operations

**Problem**: How should spatial calculation errors be handled and reported?

**Decision**: Use structured error types with spatial context.

**Rationale**:
- Spatial errors often need position and entity context
- Enables better debugging of tactical scenarios
- Supports user-friendly error messages in game clients
- Allows for graceful degradation in edge cases

```go
type SpatialError struct {
    Type        string    `json:"type"`
    Message     string    `json:"message"`
    Position    *Position `json:"position,omitempty"`
    EntityID    string    `json:"entity_id,omitempty"`
    RoomID      string    `json:"room_id,omitempty"`
    GridType    string    `json:"grid_type,omitempty"`
}
```

## Validation Requirements

### Functional Requirements
1. **Advanced Pathfinding**: Intelligent route calculation with terrain analysis
2. **Multi-Room Queries**: Spatial operations across connected room environments
3. **Complex Area Effects**: Advanced area calculations with cover analysis and secondary effects
4. **Spatial Indexing**: High-performance spatial data structures for large environments
5. **Cross-Room Navigation**: Pathfinding that handles doors, passages, and room transitions

### Non-Functional Requirements  
1. **High Performance**: Advanced queries complete in <100ms, simple queries maintain <50ms
2. **Memory Efficiency**: Spatial indexing optimized for memory usage vs query speed
3. **Scalability**: Performance degrades gracefully with multi-room complexity
4. **Cache Efficiency**: High cache hit rates for repeated tactical scenarios
5. **Analytics**: Rich performance metrics and query optimization data

### Integration Requirements
1. **Proto Compatibility**: All spatial types convert properly to/from proto
2. **Repository Integration**: Spatial queries work with persisted room data
3. **Multi-Room Support**: Queries work across connected environments
4. **Event Integration**: Spatial operations generate appropriate events

## Success Criteria

### Phase 4 Complete When:
1. ✅ Advanced pathfinding algorithms implemented with terrain analysis
2. ✅ Multi-room spatial queries work across connected environments
3. ✅ Complex area of effect calculations with secondary effects
4. ✅ Spatial indexing provides high-performance query capabilities
5. ✅ All advanced queries meet performance targets (<100ms complex, <50ms simple)
6. ✅ Service interface extended with advanced spatial operations
7. ✅ Handlers implemented with streaming and analytics capabilities
8. ✅ Multi-level caching system optimized for tactical gameplay

### Validation Tests:
- Advanced pathfinding finds optimal routes through complex terrain
- Multi-room line of sight works through doorways and passages
- Area of effect spells calculate cover and secondary effects correctly
- Spatial indexing provides significant performance improvements
- Performance tests meet targets (<100ms complex, <50ms simple queries)
- Multi-room pathfinding handles door opening and room transitions
- Cache invalidation works correctly when entities move or rooms change
- Cross-room tactical scenarios (shooting through doorways) work accurately

## Dependencies

### Upstream Dependencies
- **Phase 2 Complete**: Essential spatial queries (line of sight, movement validation, placement, range)
- **Phase 3 Complete**: Repository and orchestrator patterns, seed-based room persistence
- **rpg-toolkit**: spatial module with multi-room orchestration and advanced pathfinding capabilities
- **Redis**: Available for L2 spatial caching and environment relationship storage

### Downstream Impact  
- **Phase 5+**: Advanced environment features will build on multi-room foundation
- **Client Integration**: Advanced spatial queries enable complex tactical gameplay scenarios
- **Performance Baseline**: Establishes optimized patterns for large-scale spatial operations
- **Analytics Foundation**: Query pattern analysis enables intelligent game balancing

### Integration Requirements
1. **Phase 3 Integration**: Spatial repository must integrate seamlessly with room repository
2. **Toolkit Compatibility**: All spatial operations must use toolkit's spatial data structures
3. **Cache Coherence**: Multi-level cache must maintain consistency with seed-based persistence
4. **Performance Monitoring**: Spatial query analytics must integrate with existing monitoring systems

## Related Decisions

- **[ADR-006]**: Engine interface patterns from Phase 2
- **[ADR-008]**: Repository and orchestrator patterns from Phase 3
- **[ADR-002 (protos)]**: Proto spatial message definitions
- **[ADR-005]**: Original spatial query requirements and performance targets

## References

- [Issue #110](https://github.com/KirkDiggler/rpg-api/issues/110) - Phase 4: Spatial Query System
- [Epic #107](https://github.com/KirkDiggler/rpg-api/issues/107) - Room Generation Integration
- [ADR-008](./008-room-generation-phase-3.md) - Basic Room Operations (Phase 3)
- [ADR-006](./006-room-generation-phase-2.md) - Core Engine Interface (Phase 2)
- [rpg-toolkit spatial module](https://github.com/KirkDiggler/rpg-toolkit/tree/main/tools/spatial)
- [Journey 002](../journey/002-room-generation-phase-planning.md) - Phase planning and implementation strategy
