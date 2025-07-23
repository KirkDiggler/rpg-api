# ADR-008: Room Generation Phase 3 - Basic Room Operations

**Status**: Proposed  
**Date**: 2025-07-22  
**Decision-makers**: Kirk Diggler, Claude AI  
**Technical story**: [Issue #109](https://github.com/KirkDiggler/rpg-api/issues/109) - Phase 3: Basic Room Operations

## Summary

We need to complete the basic room generation pipeline with repository persistence and orchestrator business logic. This phase creates a fully functional single-room generation API with complete CRUD operations.

## Problem Statement

### Current State
- Phase 2 (Core Engine Interface) provides room generation through engine layer
- Handler shells exist but are not connected to real implementations
- No persistence layer for room data
- No business logic orchestration between engine and storage

### Target State  
- Room data persists correctly in Redis following rpg-api patterns
- Complete room CRUD operations work end-to-end
- Handlers properly integrated with orchestrator business logic
- Full test coverage for all layers

## Decision

**We will implement the complete room operations stack with Redis repository, orchestrator business logic, and handler integration following established rpg-api patterns.**

### Core Design Principles

1. **Repository Pattern**: Follow existing Redis repository patterns from character/dice services
2. **Orchestrator Business Logic**: Coordinate between engine, repository, and external services
3. **Entity Ownership**: All rooms owned by entities with proper access control
4. **Input/Output Consistency**: Maintain structured types throughout all layers

## Architecture

### Repository Layer Design

```go
// internal/repositories/room/repository.go - Room repository interface

//go:generate mockgen -destination=mock/mock_repository.go -package=roommock github.com/KirkDiggler/rpg-api/internal/repositories/room Repository

type Repository interface {
    // Create creates a new room with entities
    // Returns errors.InvalidArgument for validation failures
    // Returns errors.AlreadyExists if room with same ID exists
    // Returns errors.Internal for storage failures
    Create(ctx context.Context, input *CreateInput) (*CreateOutput, error)
    
    // Get retrieves a room by ID with entity ownership validation
    // Returns errors.InvalidArgument for empty/invalid IDs
    // Returns errors.NotFound if room doesn't exist
    // Returns errors.PermissionDenied if entity doesn't own the room
    // Returns errors.Internal for storage failures
    Get(ctx context.Context, input *GetInput) (*GetOutput, error)
    
    // Update updates room metadata (not entities - use separate entity operations)
    // Returns errors.InvalidArgument for validation failures
    // Returns errors.NotFound if room doesn't exist
    // Returns errors.PermissionDenied if entity doesn't own the room
    // Returns errors.Internal for storage failures
    Update(ctx context.Context, input *UpdateInput) (*UpdateOutput, error)
    
    // Delete deletes a room and all its entities
    // Returns errors.InvalidArgument for empty/invalid IDs
    // Returns errors.NotFound if room doesn't exist
    // Returns errors.PermissionDenied if entity doesn't own the room
    // Returns errors.Internal for storage failures
    Delete(ctx context.Context, input *DeleteInput) (*DeleteOutput, error)
    
    // ListByOwner retrieves all rooms owned by an entity
    // Returns errors.InvalidArgument for empty/invalid entity IDs
    // Returns errors.Internal for storage failures
    ListByOwner(ctx context.Context, input *ListByOwnerInput) (*ListByOwnerOutput, error)
    
    // GetEntities retrieves all entities in a room
    // Returns errors.InvalidArgument for empty/invalid room IDs
    // Returns errors.NotFound if room doesn't exist
    // Returns errors.PermissionDenied if entity doesn't own the room
    // Returns errors.Internal for storage failures
    GetEntities(ctx context.Context, input *GetEntitiesInput) (*GetEntitiesOutput, error)
}

// Repository Input/Output types following established rpg-api patterns

// CreateInput defines input for creating a room
type CreateInput struct {
    Room     *entities.Room      `json:"room"`
    Entities []*entities.Entity  `json:"entities"`
}

// CreateOutput defines output for creating a room
type CreateOutput struct {
    Room     *entities.Room      `json:"room"`
    Entities []*entities.Entity  `json:"entities"`
}

// GetInput defines input for retrieving a room
type GetInput struct {
    RoomID   string `json:"room_id"`
    OwnerID  string `json:"owner_id"` // Entity ID for ownership validation
}

// GetOutput defines output for retrieving a room
type GetOutput struct {
    Room     *entities.Room      `json:"room"`
    Entities []*entities.Entity  `json:"entities"`
}

// UpdateInput defines input for updating room metadata
type UpdateInput struct {
    Room    *entities.Room `json:"room"`
    OwnerID string         `json:"owner_id"` // Entity ID for ownership validation
}

// UpdateOutput defines output for updating a room
type UpdateOutput struct {
    Room *entities.Room `json:"room"`
}

// DeleteInput defines input for deleting a room
type DeleteInput struct {
    RoomID  string `json:"room_id"`
    OwnerID string `json:"owner_id"` // Entity ID for ownership validation
}

// DeleteOutput defines output for deleting a room
type DeleteOutput struct {
    // Empty for now, can be extended later with cleanup stats
}

// ListByOwnerInput defines input for listing rooms by owner
type ListByOwnerInput struct {
    OwnerID     string     `json:"owner_id"`
    PageSize    int32      `json:"page_size"`       // Default 50, max 200
    PageToken   string     `json:"page_token"`      // For pagination
    Themes      []string   `json:"themes,omitempty"` // Filter by theme
    GridTypes   []string   `json:"grid_types,omitempty"` // Filter by grid type
    CreatedAfter *time.Time `json:"created_after,omitempty"` // Filter by creation time
}

// ListByOwnerOutput defines output for listing rooms
type ListByOwnerOutput struct {
    Rooms         []*entities.Room `json:"rooms"`
    NextPageToken string           `json:"next_page_token"`
    TotalSize     int32            `json:"total_size"` // Total available (expensive to compute)
}

// GetEntitiesInput defines input for getting entities in a room
type GetEntitiesInput struct {
    RoomID   string   `json:"room_id"`
    OwnerID  string   `json:"owner_id"`        // Entity ID for ownership validation
    Types    []string `json:"types,omitempty"` // Filter by entity type ("wall", "door", "monster")
    Tags     []string `json:"tags,omitempty"`  // Filter by entity tags
}

// GetEntitiesOutput defines output for getting entities
type GetEntitiesOutput struct {
    Entities []*entities.Entity `json:"entities"`
}
```

### Entity Model Design

```go
// internal/entities/room.go - Room entity for persistence

// Room entity representing a generated tactical room
type Room struct {
    ID          string                 `json:"id" redis:"id"`
    OwnerID     string                 `json:"owner_id" redis:"owner_id"`         // Entity that owns this room
    SessionID   string                 `json:"session_id" redis:"session_id"`     // Optional session context
    Name        string                 `json:"name,omitempty" redis:"name"`
    Description string                 `json:"description,omitempty" redis:"description"`
    
    // Room generation data
    Config      RoomConfig             `json:"config" redis:"config"`
    Dimensions  Dimensions             `json:"dimensions" redis:"dimensions"`
    GridInfo    GridInformation        `json:"grid_info" redis:"grid_info"`
    
    // Flexible metadata
    Properties  map[string]interface{} `json:"properties" redis:"properties"`
    Tags        []string               `json:"tags" redis:"tags"` // For categorization
    
    // Timestamps
    CreatedAt   time.Time              `json:"created_at" redis:"created_at"`
    UpdatedAt   time.Time              `json:"updated_at" redis:"updated_at"`
    ExpiresAt   time.Time              `json:"expires_at" redis:"expires_at"`
    
    // Generation metadata (for debugging/analytics)
    GenerationTime time.Duration       `json:"generation_time_ms" redis:"generation_time_ms"`
    EntityCount    int32               `json:"entity_count" redis:"entity_count"`
    Version        string              `json:"version" redis:"version"` // API version used to generate
}

// Entity represents any positioned object within a room
type Entity struct {
    ID       string   `json:"id" redis:"id"`
    RoomID   string   `json:"room_id" redis:"room_id"`
    Type     string   `json:"type" redis:"type"`         // "wall", "door", "monster", "character", "loot", etc.
    Position Position `json:"position" redis:"position"`
    Size     float64  `json:"size" redis:"size"`         // Size for collision detection
    
    // Entity metadata and state
    Properties map[string]interface{} `json:"properties" redis:"properties"` // Type-specific data
    Tags       []string               `json:"tags" redis:"tags"`             // "destructible", "blocking", "cover"
    State      EntityState            `json:"state" redis:"state"`
    
    // Generation context
    Source     string    `json:"source" redis:"source"`         // "generated", "user_placed", "imported"
    CreatedAt  time.Time `json:"created_at" redis:"created_at"`
    UpdatedAt  time.Time `json:"updated_at" redis:"updated_at"`
}

type RoomConfig struct {
    Width       int32   `json:"width" redis:"width"`
    Height      int32   `json:"height" redis:"height"`
    Theme       string  `json:"theme" redis:"theme"`
    WallDensity float64 `json:"wall_density" redis:"wall_density"`
    Pattern     string  `json:"pattern" redis:"pattern"`
    GridType    string  `json:"grid_type" redis:"grid_type"`
    GridSize    float64 `json:"grid_size" redis:"grid_size"`
    Seed        int64   `json:"seed" redis:"seed"`
}

type Dimensions struct {
    Width  float64 `json:"width" redis:"width"`
    Height float64 `json:"height" redis:"height"`
}

type GridInformation struct {
    Type     string  `json:"type" redis:"type"`
    Size     float64 `json:"size" redis:"size"`
    CellSize float64 `json:"cell_size" redis:"cell_size"`
}
```

### Orchestrator Business Logic

```go
// internal/orchestrators/room/orchestrator.go - Room orchestrator

type Orchestrator struct {
    engine     engine.Engine
    repository room.Repository
    clock      clock.Clock
    idgen      idgen.Generator
}

//go:generate mockgen -destination=mock/mock_service.go -package=roommock github.com/KirkDiggler/rpg-api/internal/orchestrators/room Service

type Service interface {
    GenerateRoom(ctx context.Context, input *GenerateRoomInput) (*GenerateRoomOutput, error)
    GetRoom(ctx context.Context, input *GetRoomInput) (*GetRoomOutput, error)
    ListRooms(ctx context.Context, input *ListRoomsInput) (*ListRoomsOutput, error)
    DeleteRoom(ctx context.Context, input *DeleteRoomInput) (*DeleteRoomOutput, error)
    GetRoomEntities(ctx context.Context, input *GetRoomEntitiesInput) (*GetRoomEntitiesOutput, error)
}

// Orchestrator Input/Output types following rpg-api patterns

// GenerateRoomInput defines input for room generation orchestration
type GenerateRoomInput struct {
    OwnerID     string     `json:"owner_id"`      // Required: Entity that will own this room
    SessionID   string     `json:"session_id"`    // Required: Session context for the room
    Config      RoomConfig `json:"config"`        // Room generation configuration
    Seed        int64      `json:"seed"`          // Required: Seed for reproducible generation
    Name        string     `json:"name,omitempty"`
    Description string     `json:"description,omitempty"`
    Tags        []string   `json:"tags,omitempty"` // Optional categorization tags
    TTLSeconds  *int32     `json:"ttl_seconds,omitempty"` // Optional TTL override (default 14400 = 4 hours)
}

// GenerateRoomOutput defines output for room generation
type GenerateRoomOutput struct {
    Room            *entities.Room      `json:"room"`
    Entities        []*entities.Entity  `json:"entities"`
    GenerationStats GenerationStats    `json:"generation_stats"`
}

// GenerationStats provides analytics about the generation process
type GenerationStats struct {
    GenerationTimeMs int32  `json:"generation_time_ms"`
    EntityCount      int32  `json:"entity_count"`
    WallCount        int32  `json:"wall_count"`
    FeatureCount     int32  `json:"feature_count"`
    SeedUsed         int64  `json:"seed_used"`
    ConfigUsed       string `json:"config_used"` // JSON string for debugging
}

// GetRoomInput defines input for retrieving a room
type GetRoomInput struct {
    RoomID  string `json:"room_id"`
    OwnerID string `json:"owner_id"` // For ownership validation
}

// GetRoomOutput defines output for retrieving a room
type GetRoomOutput struct {
    Room     *entities.Room      `json:"room"`
    Entities []*entities.Entity  `json:"entities"`
    Stats    RoomStats           `json:"stats"`
}

// RoomStats provides current room analytics
type RoomStats struct {
    EntityCount   int32                `json:"entity_count"`
    EntityCounts  map[string]int32     `json:"entity_counts"` // Count by type
    LastAccessed  time.Time            `json:"last_accessed"`
    AccessCount   int32                `json:"access_count"`
    SpatialQueries int32               `json:"spatial_queries"` // Count of spatial queries run
}

// ListRoomsInput defines input for listing rooms
type ListRoomsInput struct {
    OwnerID     string     `json:"owner_id"`
    SessionID   string     `json:"session_id,omitempty"` // Optional filter by session
    PageSize    int32      `json:"page_size"`            // Default 50, max 200
    PageToken   string     `json:"page_token"`           // Pagination token
    Themes      []string   `json:"themes,omitempty"`     // Filter by theme
    GridTypes   []string   `json:"grid_types,omitempty"` // Filter by grid type
    Tags        []string   `json:"tags,omitempty"`       // Filter by tags
    CreatedAfter *time.Time `json:"created_after,omitempty"`
}

// ListRoomsOutput defines output for listing rooms
type ListRoomsOutput struct {
    Rooms         []*entities.Room `json:"rooms"`
    NextPageToken string           `json:"next_page_token"`
    TotalSize     int32            `json:"total_size,omitempty"` // Expensive to compute, only if requested
}

// DeleteRoomInput defines input for deleting a room
type DeleteRoomInput struct {
    RoomID  string `json:"room_id"`
    OwnerID string `json:"owner_id"` // For ownership validation
    Force   bool   `json:"force"`    // Force delete even if room has active sessions
}

// DeleteRoomOutput defines output for deleting a room
type DeleteRoomOutput struct {
    Success      bool  `json:"success"`
    EntitiesDeleted int32 `json:"entities_deleted"`
}

// GetRoomEntitiesInput defines input for getting room entities
type GetRoomEntitiesInput struct {
    RoomID  string   `json:"room_id"`
    OwnerID string   `json:"owner_id"` // For ownership validation
    Types   []string `json:"types,omitempty"` // Filter by entity types
    Tags    []string `json:"tags,omitempty"`  // Filter by entity tags
}

// GetRoomEntitiesOutput defines output for getting room entities
type GetRoomEntitiesOutput struct {
    Entities []*entities.Entity `json:"entities"`
    Stats    EntityStats        `json:"stats"`
}

// EntityStats provides analytics about entities in the room
type EntityStats struct {
    TotalCount int32                `json:"total_count"`
    TypeCounts map[string]int32     `json:"type_counts"` // Count by type
    TagCounts  map[string]int32     `json:"tag_counts"`  // Count by tag
}

```

### Seed-Based Storage Architecture

**CORE ARCHITECTURAL DECISION**: Use seed-based persistence as the primary storage strategy for massive scalability gains.

**Previous Approach** (Full Room Persistence - NOT USED):
- Store complete room data as JSON (~5-50KB per room)
- Store all wall entities separately (~1-2KB per entity)  
- High storage cost, complex relationship management

**Chosen Seed-Based Approach** (Lightweight Persistence):
```go
// Store only generation parameters and dynamic state
type RoomPersistenceData struct {
    ID          string                 `json:"id" redis:"id"`
    EntityID    string                 `json:"entity_id" redis:"entity_id"`
    Config      RoomConfig             `json:"config" redis:"config"`          // ~500B
    Seed        int64                  `json:"seed" redis:"seed"`               // 8B
    CreatedAt   time.Time              `json:"created_at" redis:"created_at"`
    ExpiresAt   time.Time              `json:"expires_at" redis:"expires_at"`
    
    // Only store dynamic entities (non-walls)
    DynamicEntities []DynamicEntity    `json:"dynamic_entities" redis:"dynamic_entities"` // ~100-500B each
}

type DynamicEntity struct {
    ID         string                 `json:"id"`
    Type       string                 `json:"type"`        // "monster", "character", "loot", etc.
    Position   Position               `json:"position"`
    Properties map[string]interface{} `json:"properties"`
    State      EntityState            `json:"state"`       // HP, status, etc.
}
```

**Benefits**:
- **90%+ storage reduction**: ~500B room metadata vs ~50KB full room data
- **Deterministic reconstruction**: Same seed + config = identical walls every time
- **Faster queries**: Only load/deserialize essential data
- **Simpler relationships**: No complex entity relationship indexing needed
- **Cache-friendly**: Room structure reconstructed and cached on first access

**Reconstruction Pattern**:
```go
func (r *Repository) GetRoom(ctx context.Context, input *GetInput) (*GetOutput, error) {
    // Load lightweight persistence data
    persistenceData, err := r.loadRoomPersistenceData(ctx, input.RoomID)
    if err != nil {
        return nil, err
    }
    
    // Reconstruct room structure from seed + config (deterministic)
    roomStructure, err := r.engine.GenerateRoom(ctx, &engine.GenerateRoomInput{
        Config: persistenceData.Config,
        Seed:   persistenceData.Seed,
    })
    if err != nil {
        return nil, fmt.Errorf("failed to reconstruct room: %w", err)
    }
    
    // Merge with dynamic entities (characters, monsters, loot)
    allEntities := append(roomStructure.Entities, persistenceData.DynamicEntities...)
    
    return &GetOutput{
        Room:     buildRoomFromStructure(roomStructure, persistenceData),
        Entities: allEntities,
    }, nil
}
```

**Implementation Benefits**:
- **90%+ Storage Reduction**: ~500B vs ~50KB per room
- **Massive Scalability**: Hundreds/thousands of rooms feasible
- **Toolkit Optimization**: Environments package designed for fast regeneration
- **Simple Architecture**: No complex Redis relationship management

### Redis Storage Strategy

```go
// internal/repositories/room/redis.go - Redis implementation

type RedisRepository struct {
    client redis.UniversalClient
    clock  clock.Clock
}

// Redis key patterns following rpg-api conventions
const (
    roomKeyPattern     = "room:%s"           // room:{room_id}
    entityKeyPattern   = "entity:%s"         // entity:{entity_id}
    roomEntitiesKey    = "room:%s:entities"  // room:{room_id}:entities (set of entity_ids)
    entityRoomsKey     = "entity:%s:rooms"   // entity:{entity_id}:rooms (set of room_ids)
    entityRoomIndex    = "rooms:by_entity:%s" // rooms:by_entity:{entity_id}
)

func (r *RedisRepository) Save(ctx context.Context, input *SaveInput) (*SaveOutput, error) {
    pipe := r.client.Pipeline()
    
    // Save room data
    roomKey := fmt.Sprintf(roomKeyPattern, input.Room.ID)
    roomData, err := json.Marshal(input.Room)
    if err != nil {
        return nil, fmt.Errorf("failed to marshal room: %w", err)
    }
    
    pipe.Set(ctx, roomKey, roomData, time.Until(input.Room.ExpiresAt))
    
    // Save entities
    entityKeys := make([]string, len(input.Entities))
    for i, entity := range input.Entities {
        entityKey := fmt.Sprintf(entityKeyPattern, entity.ID)
        entityData, err := json.Marshal(entity)
        if err != nil {
            return nil, fmt.Errorf("failed to marshal entity %s: %w", entity.ID, err)
        }
        
        pipe.Set(ctx, entityKey, entityData, time.Until(input.Room.ExpiresAt))
        entityKeys[i] = entity.ID
    }
    
    // Index relationships
    roomEntitiesKey := fmt.Sprintf(roomEntitiesKey, input.Room.ID)
    entityRoomsKey := fmt.Sprintf(entityRoomsKey, input.Room.EntityID)
    entityIndexKey := fmt.Sprintf(entityRoomIndex, input.Room.EntityID)
    
    pipe.SAdd(ctx, roomEntitiesKey, entityKeys)
    pipe.SAdd(ctx, entityRoomsKey, input.Room.ID)
    pipe.ZAdd(ctx, entityIndexKey, &redis.Z{
        Score:  float64(input.Room.CreatedAt.Unix()),
        Member: input.Room.ID,
    })
    
    // Set expiration on indexes
    pipe.Expire(ctx, roomEntitiesKey, time.Until(input.Room.ExpiresAt))
    pipe.Expire(ctx, entityRoomsKey, time.Until(input.Room.ExpiresAt))
    pipe.Expire(ctx, entityIndexKey, time.Until(input.Room.ExpiresAt))
    
    _, err = pipe.Exec(ctx)
    if err != nil {
        return nil, fmt.Errorf("failed to save room: %w", err)
    }
    
    return &SaveOutput{Room: input.Room}, nil
}
```

## Implementation Strategy

### Phase 3a: Repository Implementation

**Deliverable**: Complete Redis-based room repository

**Tasks**:
1. Create `internal/repositories/room/` package structure
2. Define repository interface with comprehensive Input/Output types
3. Implement Redis storage following established patterns from character/dice repositories
4. Create repository tests using miniredis
5. Test pagination, filtering, and relationship indexing

**Key Design Decisions**:
- Use Redis sets for relationship indexing (room ↔ entities, entity ↔ rooms)
- Use sorted sets for entity-based room indexing (by creation time)
- Follow TTL patterns from existing repositories
- Use pipeline operations for atomic multi-key operations

### Phase 3b: Orchestrator Implementation

**Deliverable**: Business logic orchestration between engine and repository

**Tasks**:
1. Create `internal/orchestrators/room/` package structure  
2. Define Service interface with Input/Output types
3. Implement orchestrator business logic:
   - `GenerateRoom`: Engine → Repository coordination
   - `GetRoom`: Repository retrieval with entity loading
   - `ListRooms`: Pagination and filtering business logic
   - `DeleteRoom`: Cleanup and relationship management
4. Wire dependencies (engine, repository, clock, idgen)
5. Create comprehensive orchestrator tests with mocked dependencies

**Key Design Decisions**:
- Orchestrator handles ID generation using existing idgen patterns
- TTL calculation using clock interface for testability
- Error handling and validation following rpg-api conventions
- Business logic separation from persistence concerns

### Phase 3c: Handler Integration

**Deliverable**: Working gRPC endpoints connected to real implementations

**Tasks**:
1. Update handler constructors to accept real orchestrator dependencies
2. Connect proto request/response mapping to orchestrator calls  
3. Implement comprehensive request validation and error handling
4. Add structured logging and metrics following existing patterns
5. Create handler tests using real orchestrator (not mocked)

**Key Design Decisions**:
- Handlers perform proto ↔ internal type conversion
- Validation happens at handler layer before orchestrator calls
- Error mapping from internal errors to gRPC status codes
- Logging includes request IDs and entity ownership context

### Phase 3d: End-to-End Integration

**Deliverable**: Complete working API with persistence

**Tasks**:
1. Integration testing: gRPC request → Redis storage → response
2. Error scenario testing (invalid inputs, missing rooms, ownership violations)
3. Performance testing for room generation and retrieval
4. Integration with existing server infrastructure and middleware

## Technical Decisions

### Decision 1: Seed-Based Storage Strategy

**Problem**: How should room data be stored efficiently to support hundreds/thousands of rooms?

**Decision**: Use seed-based storage with minimal Redis footprint, storing only generation parameters and dynamic entities.

**Rationale**:
- **Massive Storage Savings**: ~500B vs ~50KB per room (90%+ reduction)
- **Scalability**: Hundreds/thousands of rooms become feasible
- **Toolkit Leverage**: Let environments package handle room structure regeneration
- **Simple Patterns**: Minimal Redis complexity, no relationship management

**Lightweight Redis Key Strategy**:
```go
const (
    // Primary data only - no complex relationships
    roomKey           = "room:%s"              // room:{room_id} → RoomPersistenceData JSON (~500B)
    
    // Simple pagination index only
    ownerRoomIndex    = "rooms:by_owner:%s"    // rooms:by_owner:{owner_id} → ZSet(room_id, created_timestamp)
)

// Compact storage structure
type RoomPersistenceData struct {
    ID          string          `json:"id"`
    OwnerID     string          `json:"owner_id"`
    SessionID   string          `json:"session_id"`
    Name        string          `json:"name,omitempty"`
    Description string          `json:"description,omitempty"`
    
    // Generation parameters (environments toolkit regenerates from these)
    Config      RoomConfig      `json:"config"`       // ~300B
    Seed        int64           `json:"seed"`         // 8B
    
    // Timestamps
    CreatedAt   time.Time       `json:"created_at"`
    UpdatedAt   time.Time       `json:"updated_at"`
    ExpiresAt   time.Time       `json:"expires_at"`
    
    // ONLY store entities placed/moved after generation
    DynamicEntities []DynamicEntity `json:"dynamic_entities"` // ~100-500B each
}

// Only store non-generated entities (characters, monsters, moved items)
type DynamicEntity struct {
    ID         string                 `json:"id"`
    Type       string                 `json:"type"`       // "character", "monster", "loot"
    Position   Position               `json:"position"`
    Properties map[string]interface{} `json:"properties"`
    State      EntityState            `json:"state"`
    Source     string                 `json:"source"`     // "user_placed", "moved", "spawned"
}
```

**Simple Storage Operations**:
```go
func (r *RedisRepository) Create(ctx context.Context, input *CreateInput) (*CreateOutput, error) {
    // Convert to lightweight persistence format
    persistenceData := &RoomPersistenceData{
        ID:          input.Room.ID,
        OwnerID:     input.Room.OwnerID,
        SessionID:   input.Room.SessionID,
        Name:        input.Room.Name,
        Description: input.Room.Description,
        Config:      input.Room.Config,
        Seed:        input.Room.Config.Seed,
        CreatedAt:   input.Room.CreatedAt,
        UpdatedAt:   input.Room.UpdatedAt,
        ExpiresAt:   input.Room.ExpiresAt,
        
        // Only store dynamic entities (not walls/doors generated by seed)
        DynamicEntities: filterDynamicEntities(input.Entities),
    }
    
    // Simple Redis operations
    pipe := r.client.Pipeline()
    
    // 1. Store lightweight room data
    roomData, _ := json.Marshal(persistenceData)
    roomKey := fmt.Sprintf(roomKey, persistenceData.ID)
    pipe.Set(ctx, roomKey, roomData, time.Until(persistenceData.ExpiresAt))
    
    // 2. Update pagination index only
    score := float64(persistenceData.CreatedAt.Unix())
    indexKey := fmt.Sprintf(ownerRoomIndex, persistenceData.OwnerID)
    pipe.ZAdd(ctx, indexKey, &redis.Z{Score: score, Member: persistenceData.ID})
    pipe.Expire(ctx, indexKey, time.Until(persistenceData.ExpiresAt))
    
    // Execute atomically
    _, err := pipe.Exec(ctx)
    if err != nil {
        return nil, errors.Internal("failed to store room: %v", err)
    }
    
    return &CreateOutput{Room: input.Room, Entities: input.Entities}, nil
}

func (r *RedisRepository) Get(ctx context.Context, input *GetInput) (*GetOutput, error) {
    // 1. Load lightweight persistence data
    roomKey := fmt.Sprintf(roomKey, input.RoomID)
    data, err := r.client.Get(ctx, roomKey).Result()
    if err == redis.Nil {
        return nil, errors.NotFound("room %s not found", input.RoomID)
    }
    if err != nil {
        return nil, errors.Internal("failed to load room: %v", err)
    }
    
    var persistenceData RoomPersistenceData
    if err := json.Unmarshal([]byte(data), &persistenceData); err != nil {
        return nil, errors.Internal("failed to unmarshal room data: %v", err)
    }
    
    // 2. Ownership validation
    if persistenceData.OwnerID != input.OwnerID {
        return nil, errors.PermissionDenied("room %s not owned by entity %s", input.RoomID, input.OwnerID)
    }
    
    // 3. Regenerate room structure using environments toolkit
    roomStructure, err := r.engine.GenerateRoom(ctx, &engine.GenerateRoomInput{
        Config: persistenceData.Config,
        Seed:   persistenceData.Seed,
    })
    if err != nil {
        return nil, errors.Internal("failed to regenerate room structure: %v", err)
    }
    
    // 4. Merge generated entities with stored dynamic entities
    allEntities := append(roomStructure.Entities, persistenceData.DynamicEntities...)
    
    // 5. Build full room entity from persistence data and generated structure
    room := &entities.Room{
        ID:          persistenceData.ID,
        OwnerID:     persistenceData.OwnerID,
        SessionID:   persistenceData.SessionID,
        Name:        persistenceData.Name,
        Description: persistenceData.Description,
        Config:      persistenceData.Config,
        Dimensions:  roomStructure.Dimensions,
        GridInfo:    roomStructure.GridInfo,
        CreatedAt:   persistenceData.CreatedAt,
        UpdatedAt:   persistenceData.UpdatedAt,
        ExpiresAt:   persistenceData.ExpiresAt,
    }
    
    return &GetOutput{Room: room, Entities: allEntities}, nil
}

// Only store entities that were placed or modified after generation
func filterDynamicEntities(entities []*entities.Entity) []DynamicEntity {
    var dynamic []DynamicEntity
    for _, entity := range entities {
        // Skip generated entities (walls, doors from seed)
        if entity.Source == "generated" {
            continue
        }
        
        dynamic = append(dynamic, DynamicEntity{
            ID:         entity.ID,
            Type:       entity.Type,
            Position:   entity.Position,
            Properties: entity.Properties,
            State:      entity.State,
            Source:     entity.Source,
        })
    }
    return dynamic
}
```

### Decision 2: Entity Relationship Management

**Problem**: How should room-entity relationships be managed in Redis?

**Decision**: Use Redis sets for bidirectional relationship tracking.

**Rationale**:
- Enables efficient lookups in both directions (room → entities, entity → rooms)
- Supports atomic operations for relationship consistency
- Allows efficient cleanup when rooms or entities are deleted
- Follows established patterns from existing repositories

### Decision 3: Orchestrator Error Handling and ID Generation

**Problem**: How should business logic errors be handled and IDs generated consistently?

**Decision**: Use typed error handling with rpg-api error patterns and centralized ID generation.

**Rationale**:
- **Error Consistency**: Use established `errors.InvalidArgument`, `errors.NotFound` patterns
- **gRPC Compatibility**: Error types map directly to gRPC status codes
- **Debugging Support**: Structured error context with room/entity IDs
- **ID Generation**: Use existing idgen patterns for consistency

**Error Handling Pattern**:
```go
import (
    "github.com/KirkDiggler/rpg-api/internal/errors"
    "github.com/KirkDiggler/rpg-api/internal/idgen"
)

func (o *Orchestrator) GenerateRoom(ctx context.Context, input *GenerateRoomInput) (*GenerateRoomOutput, error) {
    // 1. Input validation with specific error types
    if err := o.validateGenerateRoomInput(input); err != nil {
        return nil, errors.InvalidArgument("invalid generate room input: %v", err)
    }
    
    // 2. Generate IDs using established patterns
    roomID, err := o.idgen.Generate()
    if err != nil {
        return nil, errors.Internal("failed to generate room ID: %v", err)
    }
    
    // 3. Engine generation with error context
    engineInput := o.convertToEngineInput(input, roomID)
    engineOutput, err := o.engine.GenerateRoom(ctx, engineInput)
    if err != nil {
        return nil, errors.Wrap(err, "room generation failed for owner %s", input.OwnerID)
    }
    
    // 4. Repository persistence with ownership context
    room := o.buildRoomEntity(engineOutput, input, roomID)
    entities := o.buildEntitySlice(engineOutput.Entities, roomID)
    
    createInput := &room.CreateInput{Room: room, Entities: entities}
    createOutput, err := o.repository.Create(ctx, createInput)
    if err != nil {
        return nil, errors.Wrap(err, "failed to persist room %s for owner %s", roomID, input.OwnerID)
    }
    
    return &GenerateRoomOutput{
        Room:     createOutput.Room,
        Entities: createOutput.Entities,
        GenerationStats: GenerationStats{
            GenerationTimeMs: int32(engineOutput.Metadata.GenerationTime.Milliseconds()),
            EntityCount:      int32(len(entities)),
            SeedUsed:         input.Seed,
        },
    }, nil
}

func (o *Orchestrator) validateGenerateRoomInput(input *GenerateRoomInput) error {
    if input.OwnerID == "" {
        return fmt.Errorf("owner_id is required")
    }
    if input.SessionID == "" {
        return fmt.Errorf("session_id is required")
    }
    if input.Config.Width <= 0 || input.Config.Height <= 0 {
        return fmt.Errorf("room dimensions must be positive (got %dx%d)", input.Config.Width, input.Config.Height)
    }
    if input.Seed == 0 {
        return fmt.Errorf("seed is required for reproducible generation")
    }
    return nil
}
```

**ID Generation Strategy**:
```go
type Orchestrator struct {
    engine     engine.Engine
    repository room.Repository
    clock      clock.Clock
    idgen      idgen.Generator  // Consistent with other orchestrators
}

func (o *Orchestrator) generateRoomID() (string, error) {
    return o.idgen.Generate() // Uses same pattern as character/dice services
}

func (o *Orchestrator) generateEntityID() (string, error) {
    return o.idgen.Generate() // Same generator, different entities
}
```

```go
// Example error handling pattern
func (o *Orchestrator) GenerateRoom(ctx context.Context, input *GenerateRoomInput) (*GenerateRoomOutput, error) {
    // Input validation
    if err := o.validateGenerateRoomInput(input); err != nil {
        return nil, errors.InvalidArgument("invalid generate room input: %v", err)
    }
    
    // Engine call
    engineInput := o.convertToEngineInput(input)
    engineOutput, err := o.engine.GenerateRoom(ctx, engineInput)
    if err != nil {
        return nil, errors.Wrap(err, "engine room generation failed")
    }
    
    // Repository persistence
    room := o.convertToRoomEntity(engineOutput, input)
    entities := o.convertToEntitySlice(engineOutput.Entities)
    
    saveInput := &room.SaveInput{Room: room, Entities: entities}
    _, err = o.repository.Save(ctx, saveInput)
    if err != nil {
        return nil, errors.Wrap(err, "failed to persist room")
    }
    
    return &GenerateRoomOutput{Room: room, Entities: entities}, nil
}
```

### Decision 4: TTL and Expiration Management

**Problem**: How should room expiration and TTL be managed?

**Decision**: Use default 4-hour TTL with client override support and automatic cleanup.

**Rationale**:
- **4-hour default**: Covers typical game session length (dice service pattern)
- **Client override**: Supports different use cases (24h for campaign rooms, 1h for quick battles)
- **Automatic cleanup**: Prevents storage bloat and relationship orphans
- **Grace period**: 30-minute grace period before hard deletion for recovery

**TTL Strategy**:
```go
const (
    DefaultRoomTTL = 4 * time.Hour      // Standard game session
    MinRoomTTL     = 30 * time.Minute    // Minimum allowed TTL
    MaxRoomTTL     = 7 * 24 * time.Hour  // Maximum 7 days for campaign rooms
    CleanupGrace   = 30 * time.Minute    // Grace period before hard delete
)

func (o *Orchestrator) calculateTTL(input *GenerateRoomInput) time.Duration {
    if input.TTLSeconds != nil {
        requested := time.Duration(*input.TTLSeconds) * time.Second
        if requested < MinRoomTTL {
            return MinRoomTTL
        }
        if requested > MaxRoomTTL {
            return MaxRoomTTL
        }
        return requested
    }
    return DefaultRoomTTL
}
```

### Decision 5: Pagination Strategy

**Problem**: How should room listing be paginated efficiently?

**Decision**: Use Redis sorted sets with timestamp scoring and cursor-based pagination.

**Rationale**:
- **Sorted sets**: Enable efficient range queries without SCAN operations
- **Timestamp scoring**: Natural ordering by creation time with filtering support
- **Cursor pagination**: More reliable than offset-based for concurrent modifications
- **Consistent patterns**: Follows character service pagination approach

**Implementation Pattern**:
```go
// Redis key pattern for owner-based room indexing
const ownerRoomIndexKey = "rooms:by_owner:%s" // sorted set with creation timestamp scores

func (r *RedisRepository) ListByOwner(ctx context.Context, input *ListByOwnerInput) (*ListByOwnerOutput, error) {
    indexKey := fmt.Sprintf(ownerRoomIndexKey, input.OwnerID)
    
    // Parse cursor for range start
    start := "-inf"  // Default: from beginning
    if input.PageToken != "" {
        if timestamp, err := parseCursor(input.PageToken); err == nil {
            start = fmt.Sprintf("(%d", timestamp) // Exclusive start
        }
    }
    
    // Query sorted set with limit+1 for next page detection
    limit := input.PageSize
    if limit == 0 {
        limit = 50 // Default page size
    }
    if limit > 200 {
        limit = 200 // Maximum page size
    }
    
    roomIDs, err := r.client.ZRangeByScore(ctx, indexKey, &redis.ZRangeBy{
        Min:    start,
        Max:    "+inf",
        Offset: 0,
        Count:  int64(limit + 1), // +1 to detect next page
    }).Result()
    
    // Process results and generate next page token
    hasNext := len(roomIDs) > int(limit)
    if hasNext {
        roomIDs = roomIDs[:limit] // Trim to requested size
    }
    
    // Load room data in parallel
    rooms, err := r.loadRoomsInParallel(ctx, roomIDs)
    if err != nil {
        return nil, err
    }
    
    // Generate next page token
    var nextToken string
    if hasNext && len(rooms) > 0 {
        lastRoom := rooms[len(rooms)-1]
        nextToken = generateCursor(lastRoom.CreatedAt)
    }
    
    return &ListByOwnerOutput{
        Rooms:         rooms,
        NextPageToken: nextToken,
        // TotalSize omitted - expensive to compute, client can request separately if needed
    }, nil
}
```

## Validation Requirements

### Functional Requirements
1. **Complete CRUD**: Create, read, update, delete rooms with proper persistence
2. **Entity Relationships**: Proper room-entity relationship management
3. **Ownership Control**: All operations respect entity ownership patterns
4. **Error Handling**: Comprehensive validation and error reporting
5. **Pagination**: Efficient listing with proper pagination support

### Non-Functional Requirements
1. **Performance**: Room operations complete in <50ms (excluding generation)
2. **Consistency**: Repository operations are atomic where required
3. **Testability**: Full test coverage with integration testing
4. **Reliability**: Proper error recovery and graceful degradation
5. **Observability**: Structured logging and metrics integration

### Integration Requirements
1. **Proto Compatibility**: Handlers properly convert proto messages
2. **Engine Integration**: Orchestrator properly coordinates with Phase 2 engine
3. **Repository Pattern**: Follows established Redis repository conventions
4. **Service Interface**: Clean interface for future spatial query extension

## Success Criteria

### Phase 3 Complete When:
1. ✅ Room repository implemented with Redis persistence
2. ✅ Orchestrator business logic coordinates engine and repository
3. ✅ Handlers connected to real orchestrator implementations
4. ✅ Complete CRUD operations work end-to-end
5. ✅ Full test coverage for repository, orchestrator, and handler layers
6. ✅ Integration tests demonstrate gRPC → Redis → response flow

### Validation Tests:
- **Seed Reproducibility**: Generate room with seed 12345, verify identical results on repeated calls
- **Theme Variations**: Generate rooms with different themes (dungeon, forest, urban)
- **Grid System Support**: Generate rooms with different grid types (square, hex, gridless)
- **Auto-scaling Scenarios**: Request small room (5x5) with many entities (50+), verify toolkit auto-scales room size or suggests optimization
- **Capacity Analysis**: Test environments toolkit capacity calculations for various entity counts
- **Storage Efficiency**: Verify seed-based storage uses ~500B vs ~50KB for full persistence
- **Regeneration Accuracy**: Load persisted room, verify regenerated structure matches original
- **Dynamic Entity Handling**: Place entities after generation, verify they persist while walls regenerate
- **CRUD Operations**: Generate room, verify persistence, retrieve and compare
- **Pagination and Filtering**: List rooms with pagination and filtering
- **Cleanup**: Delete room and verify cleanup of entities and relationships
- **Ownership Validation**: Test ownership validation (access control)
- **Error Scenarios**: Test error scenarios (invalid inputs, missing rooms, Redis failures)
- **Performance Targets**: Room generation <100ms, reconstruction from seed <50ms

## Dependencies

### Upstream Dependencies
- **Phase 2 Complete**: Engine interface and toolkit integration working
- **Redis**: Available and configured for repository testing

### Downstream Impact  
- **Phase 4**: Spatial query extension will use repository and orchestrator patterns
- **Client Integration**: Handlers ready for client integration testing
- **Performance Baseline**: Establishes performance patterns for later phases

## Related Decisions

- **[ADR-001]**: Established patterns for service interfaces and Input/Output types
- **[ADR-006]**: Engine interface and types from Phase 2
- **[ADR-005]**: Original repository and orchestrator requirements
- **Character Repository]**: Existing patterns for Redis repository implementation
- **Dice Service]**: Existing patterns for orchestrator business logic

## References

- [Issue #109](https://github.com/KirkDiggler/rpg-api/issues/109) - Phase 3: Basic Room Operations
- [Epic #107](https://github.com/KirkDiggler/rpg-api/issues/107) - Room Generation Integration
- [ADR-006](./006-room-generation-phase-2.md) - Core Engine Interface (Phase 2)
- [ADR-005](./005-room-generation-integration.md) - Original comprehensive requirements
- [internal/repositories/character](../../internal/repositories/character/) - Reference repository implementation
- [internal/orchestrators/dice](../../internal/orchestrators/dice/) - Reference orchestrator implementation