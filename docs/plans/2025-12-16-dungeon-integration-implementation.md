# Dungeon Integration Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Wire the existing dungeon generator into the encounter flow for solo and multiplayer dungeon runs with multi-room exploration.

**Architecture:** Separate Dungeon entity tracks map/exploration state, Encounter tracks combat. Rooms merge into unified grid as doors open. Progressive reveal via streaming events.

**Tech Stack:** Go, gRPC/protobuf, rpg-toolkit dungeon component, in-memory repositories

**Design Doc:** `docs/plans/2025-01-16-dungeon-integration-design.md`

---

## Issue 1: Proto Changes - Dungeon Enums and Messages

**Repository:** rpg-api-protos

**Goal:** Add proto definitions for dungeon configuration, door info, and new request/response types.

**Context:** The dungeon system needs three enums for player configuration (theme, difficulty, length), a DoorInfo message for visible doors, and updated request/response messages.

### Files

- Modify: `dnd5e/api/v1alpha1/enums.proto` - Add dungeon enums
- Modify: `dnd5e/api/v1alpha1/encounter.proto` - Add DoorInfo, update requests/responses

### Step 1: Add dungeon enums to enums.proto

Add at end of file:

```protobuf
// DungeonTheme defines the visual style and monster pool for dungeons
enum DungeonTheme {
  DUNGEON_THEME_UNSPECIFIED = 0;
  DUNGEON_THEME_CRYPT = 1;
  DUNGEON_THEME_CAVE = 2;
  DUNGEON_THEME_RUINS = 3;
}

// DungeonDifficulty controls encounter CR scaling
enum DungeonDifficulty {
  DUNGEON_DIFFICULTY_UNSPECIFIED = 0;
  DUNGEON_DIFFICULTY_EASY = 1;
  DUNGEON_DIFFICULTY_MEDIUM = 2;
  DUNGEON_DIFFICULTY_HARD = 3;
}

// DungeonLength controls number of rooms
enum DungeonLength {
  DUNGEON_LENGTH_UNSPECIFIED = 0;
  DUNGEON_LENGTH_SHORT = 1;  // 3-4 rooms
  DUNGEON_LENGTH_MEDIUM = 2; // 5-7 rooms
  DUNGEON_LENGTH_LONG = 3;   // 8-10 rooms
}

// DungeonState tracks dungeon completion status
enum DungeonState {
  DUNGEON_STATE_UNSPECIFIED = 0;
  DUNGEON_STATE_ACTIVE = 1;
  DUNGEON_STATE_VICTORIOUS = 2;
  DUNGEON_STATE_FAILED = 3;
  DUNGEON_STATE_ABANDONED = 4;
}
```

### Step 2: Add DoorInfo message to encounter.proto

Add after Room message (~line 54):

```protobuf
// DoorInfo represents a visible door/connection in a room
message DoorInfo {
  // Unique identifier for this connection
  string connection_id = 1;
  // Position in merged grid coordinates
  .api.v1alpha1.Position position = 2;
  // Description of the door (e.g., "iron door", "crumbling archway")
  string physical_hint = 3;
  // Whether the door is currently open
  bool is_open = 4;
  // ID of the room this door leads to (empty if room not yet revealed)
  string leads_to_room_id = 5;
}
```

### Step 3: Update DungeonStartRequest

Replace existing DungeonStartRequest:

```protobuf
// DungeonStartRequest initiates a dungeon encounter
message DungeonStartRequest {
  // IDs of the characters entering the dungeon
  repeated string character_ids = 1;
  // Visual style and monster pool
  DungeonTheme theme = 2;
  // Encounter difficulty scaling
  DungeonDifficulty difficulty = 3;
  // Number of rooms
  DungeonLength length = 4;
}
```

### Step 4: Update DungeonStartResponse

Replace existing DungeonStartResponse:

```protobuf
// DungeonStartResponse contains the generated dungeon with combat started
message DungeonStartResponse {
  // Unique identifier for this encounter
  string encounter_id = 1;
  // Unique identifier for this dungeon
  string dungeon_id = 2;
  // The starting room with all entities placed
  Room room = 3;
  // Doors visible in the starting room
  repeated DoorInfo doors = 4;
  // Combat state with initiative rolled
  CombatState combat_state = 5;
  // If monsters acted before first player
  repeated MonsterTurnResult monster_turns = 6;
}
```

### Step 5: Add OpenDoor request/response

Add after DungeonStartResponse:

```protobuf
// OpenDoorRequest opens a door to reveal the next room
message OpenDoorRequest {
  // The encounter ID
  string encounter_id = 1;
  // The connection ID of the door to open
  string connection_id = 2;
}

// OpenDoorResponse contains the revealed room and updated combat state
message OpenDoorResponse {
  // The newly revealed room
  Room revealed_room = 1;
  // Offset position for placing room in merged grid
  .api.v1alpha1.Position room_offset = 2;
  // Doors visible in the newly revealed room
  repeated DoorInfo new_doors = 3;
  // Monsters in the revealed room
  repeated MonsterInfo monsters = 4;
  // Updated combat state with new monsters in initiative
  CombatState combat_state = 5;
}
```

### Step 6: Add OpenDoor to EncounterService

Find the service definition and add:

```protobuf
  // OpenDoor opens a door to reveal the connected room
  rpc OpenDoor(OpenDoorRequest) returns (OpenDoorResponse);
```

### Step 7: Add dungeon events

Add after existing event messages:

```protobuf
// RoomRevealedEvent is sent when a door is opened
message RoomRevealedEvent {
  // The revealed room
  Room room = 1;
  // Offset for merged grid placement
  .api.v1alpha1.Position offset = 2;
  // Doors in the revealed room
  repeated DoorInfo doors = 3;
  // Monsters that were revealed
  repeated MonsterInfo monsters = 4;
}

// DungeonVictoryEvent is sent when the boss is defeated
message DungeonVictoryEvent {
  // Number of rooms the party explored
  int32 rooms_explored = 1;
  // Total monsters defeated
  int32 monsters_defeated = 2;
}

// DungeonFailureEvent is sent on TPK
message DungeonFailureEvent {
  // Reason for failure
  string reason = 1;
}
```

### Step 8: Format and verify

```bash
cd /home/kirk/personal/rpg-api-protos
buf format -w
buf lint
buf build
```

### Step 9: Commit

```bash
git checkout -b feat/dungeon-proto-types
git add dnd5e/api/v1alpha1/enums.proto dnd5e/api/v1alpha1/encounter.proto
git commit -m "feat: add dungeon configuration protos

- Add DungeonTheme, DungeonDifficulty, DungeonLength enums
- Add DoorInfo message for visible doors
- Update DungeonStartRequest with config options
- Update DungeonStartResponse with dungeon_id and doors
- Add OpenDoor RPC and messages
- Add dungeon events (RoomRevealed, Victory, Failure)"
git push -u origin feat/dungeon-proto-types
gh pr create --title "feat: Add dungeon configuration protos" --body "Adds proto definitions for multi-room dungeon support.

## Changes
- DungeonTheme, DungeonDifficulty, DungeonLength enums
- DoorInfo message
- Updated DungeonStart request/response
- OpenDoor RPC
- Dungeon events

Part of dungeon integration work."
```

### Acceptance Criteria
- [ ] Three dungeon enums defined (Theme, Difficulty, Length)
- [ ] DoorInfo message with connection_id, position, physical_hint, is_open
- [ ] DungeonStartRequest has theme, difficulty, length fields
- [ ] DungeonStartResponse has dungeon_id and doors fields
- [ ] OpenDoor RPC added to EncounterService
- [ ] Event messages for room reveal, victory, failure
- [ ] `buf lint` passes
- [ ] `buf build` succeeds

---

## Issue 2: Dungeon Entity and Repository

**Repository:** rpg-api

**Goal:** Create the Dungeon entity and in-memory repository following existing patterns.

**Context:** The Dungeon entity tracks the generated map, exploration state (which rooms revealed), and door states (open/closed). It's separate from Encounter which tracks combat.

**Depends on:** Issue 1 (for DungeonState enum import, but can stub locally)

### Files

- Create: `internal/entities/dungeon.go`
- Create: `internal/repositories/dungeons/repository.go`
- Create: `internal/repositories/dungeons/inmemory.go`
- Create: `internal/repositories/dungeons/inmemory_test.go`

### Step 1: Create dungeon entity

Create `internal/entities/dungeon.go`:

```go
package entities

import (
	"time"

	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
)

// DungeonState represents the completion status of a dungeon
type DungeonState string

const (
	DungeonStateActive     DungeonState = "active"
	DungeonStateVictorious DungeonState = "victorious"
	DungeonStateFailed     DungeonState = "failed"
	DungeonStateAbandoned  DungeonState = "abandoned"
)

// DungeonTheme represents the visual style of a dungeon
type DungeonTheme string

const (
	DungeonThemeCrypt DungeonTheme = "crypt"
	DungeonThemeCave  DungeonTheme = "cave"
	DungeonThemeRuins DungeonTheme = "ruins"
)

// DungeonDifficulty represents encounter scaling
type DungeonDifficulty string

const (
	DungeonDifficultyEasy   DungeonDifficulty = "easy"
	DungeonDifficultyMedium DungeonDifficulty = "medium"
	DungeonDifficultyHard   DungeonDifficulty = "hard"
)

// DungeonLength represents room count category
type DungeonLength string

const (
	DungeonLengthShort  DungeonLength = "short"
	DungeonLengthMedium DungeonLength = "medium"
	DungeonLengthLong   DungeonLength = "long"
)

// Dungeon represents a multi-room dungeon instance
type Dungeon struct {
	ID         string
	EncounterID string // Links to the combat encounter

	// Configuration
	Theme      DungeonTheme
	Difficulty DungeonDifficulty
	Length     DungeonLength
	Seed       int64

	// Generated content (from dungeon.Generator)
	GeneratedDungeon *dungeon.Dungeon

	// Exploration state
	RevealedRooms map[string]bool // Room ID -> revealed
	OpenDoors     map[string]bool // Connection ID -> open
	CurrentRoomID string          // Where the party currently is

	// Completion state
	State           DungeonState
	RoomsExplored   int
	MonstersDefeated int

	// Timestamps
	CreatedAt   time.Time
	CompletedAt *time.Time
}

// IsRoomRevealed checks if a room has been revealed
func (d *Dungeon) IsRoomRevealed(roomID string) bool {
	return d.RevealedRooms[roomID]
}

// IsDoorOpen checks if a connection/door is open
func (d *Dungeon) IsDoorOpen(connectionID string) bool {
	return d.OpenDoors[connectionID]
}

// RevealRoom marks a room as revealed
func (d *Dungeon) RevealRoom(roomID string) {
	if d.RevealedRooms == nil {
		d.RevealedRooms = make(map[string]bool)
	}
	d.RevealedRooms[roomID] = true
	d.RoomsExplored++
}

// OpenDoor marks a connection as open
func (d *Dungeon) OpenDoor(connectionID string) {
	if d.OpenDoors == nil {
		d.OpenDoors = make(map[string]bool)
	}
	d.OpenDoors[connectionID] = true
}

// GetRoom retrieves a room from the generated dungeon
func (d *Dungeon) GetRoom(roomID string) *dungeon.Room {
	if d.GeneratedDungeon == nil {
		return nil
	}
	for _, room := range d.GeneratedDungeon.Rooms {
		if room.ID == roomID {
			return room
		}
	}
	return nil
}

// GetConnection retrieves a connection from the generated dungeon
func (d *Dungeon) GetConnection(connectionID string) *dungeon.RoomConnection {
	if d.GeneratedDungeon == nil {
		return nil
	}
	for _, conn := range d.GeneratedDungeon.Connections {
		if conn.FromRoom+"-"+conn.ToRoom == connectionID || conn.ToRoom+"-"+conn.FromRoom == connectionID {
			return conn
		}
	}
	return nil
}

// GetDoorsForRoom returns all connections for a given room
func (d *Dungeon) GetDoorsForRoom(roomID string) []*dungeon.RoomConnection {
	if d.GeneratedDungeon == nil {
		return nil
	}
	var doors []*dungeon.RoomConnection
	for _, conn := range d.GeneratedDungeon.Connections {
		if conn.FromRoom == roomID || conn.ToRoom == roomID {
			doors = append(doors, conn)
		}
	}
	return doors
}
```

### Step 2: Create repository interface

Create `internal/repositories/dungeons/repository.go`:

```go
package dungeons

//go:generate mockgen -destination=mock/mock_repository.go -package=dungeonmock github.com/KirkDiggler/rpg-api/internal/repositories/dungeons Repository

import (
	"context"

	"github.com/KirkDiggler/rpg-api/internal/entities"
)

// Repository defines the storage interface for dungeons
type Repository interface {
	// Save stores a dungeon
	Save(ctx context.Context, input *SaveInput) (*SaveOutput, error)

	// Get retrieves a dungeon by ID
	Get(ctx context.Context, input *GetInput) (*GetOutput, error)

	// GetByEncounterID retrieves a dungeon by its encounter ID
	GetByEncounterID(ctx context.Context, input *GetByEncounterIDInput) (*GetOutput, error)

	// Update modifies an existing dungeon
	Update(ctx context.Context, input *UpdateInput) (*UpdateOutput, error)

	// Delete removes a dungeon
	Delete(ctx context.Context, input *DeleteInput) (*DeleteOutput, error)
}

// SaveInput defines the request for saving a dungeon
type SaveInput struct {
	Dungeon *entities.Dungeon
}

// SaveOutput defines the response for saving a dungeon
type SaveOutput struct {
	Success bool
}

// GetInput defines the request for retrieving a dungeon
type GetInput struct {
	DungeonID string
}

// GetOutput defines the response for retrieving a dungeon
type GetOutput struct {
	Dungeon *entities.Dungeon
}

// GetByEncounterIDInput defines the request for retrieving by encounter
type GetByEncounterIDInput struct {
	EncounterID string
}

// UpdateInput defines the request for updating a dungeon
type UpdateInput struct {
	DungeonID     string
	RevealedRooms map[string]bool       // Updated revealed rooms (optional)
	OpenDoors     map[string]bool       // Updated open doors (optional)
	CurrentRoomID *string               // Updated current room (optional)
	State         *entities.DungeonState // Updated state (optional)
	MonstersDefeated *int               // Increment monsters defeated (optional)
}

// UpdateOutput defines the response for updating a dungeon
type UpdateOutput struct {
	Success bool
}

// DeleteInput defines the request for deleting a dungeon
type DeleteInput struct {
	DungeonID string
}

// DeleteOutput defines the response for deleting a dungeon
type DeleteOutput struct {
	Success bool
}
```

### Step 3: Create in-memory implementation

Create `internal/repositories/dungeons/inmemory.go`:

```go
package dungeons

import (
	"context"
	"errors"
	"sync"

	"github.com/KirkDiggler/rpg-api/internal/entities"
)

var (
	ErrDungeonNotFound = errors.New("dungeon not found")
)

// InMemory implements Repository with in-memory storage
type InMemory struct {
	mu                sync.RWMutex
	dungeons          map[string]*entities.Dungeon
	encounterIndex    map[string]string // encounterID -> dungeonID
}

// NewInMemory creates a new in-memory dungeon repository
func NewInMemory() *InMemory {
	return &InMemory{
		dungeons:       make(map[string]*entities.Dungeon),
		encounterIndex: make(map[string]string),
	}
}

// Save stores a dungeon
func (r *InMemory) Save(ctx context.Context, input *SaveInput) (*SaveOutput, error) {
	if input == nil || input.Dungeon == nil {
		return nil, errors.New("dungeon is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.dungeons[input.Dungeon.ID] = input.Dungeon
	if input.Dungeon.EncounterID != "" {
		r.encounterIndex[input.Dungeon.EncounterID] = input.Dungeon.ID
	}

	return &SaveOutput{Success: true}, nil
}

// Get retrieves a dungeon by ID
func (r *InMemory) Get(ctx context.Context, input *GetInput) (*GetOutput, error) {
	if input == nil || input.DungeonID == "" {
		return nil, errors.New("dungeon ID is required")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	dungeon, exists := r.dungeons[input.DungeonID]
	if !exists {
		return nil, ErrDungeonNotFound
	}

	return &GetOutput{Dungeon: dungeon}, nil
}

// GetByEncounterID retrieves a dungeon by encounter ID
func (r *InMemory) GetByEncounterID(ctx context.Context, input *GetByEncounterIDInput) (*GetOutput, error) {
	if input == nil || input.EncounterID == "" {
		return nil, errors.New("encounter ID is required")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	dungeonID, exists := r.encounterIndex[input.EncounterID]
	if !exists {
		return nil, ErrDungeonNotFound
	}

	dungeon, exists := r.dungeons[dungeonID]
	if !exists {
		return nil, ErrDungeonNotFound
	}

	return &GetOutput{Dungeon: dungeon}, nil
}

// Update modifies an existing dungeon
func (r *InMemory) Update(ctx context.Context, input *UpdateInput) (*UpdateOutput, error) {
	if input == nil || input.DungeonID == "" {
		return nil, errors.New("dungeon ID is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	dungeon, exists := r.dungeons[input.DungeonID]
	if !exists {
		return nil, ErrDungeonNotFound
	}

	// Apply optional updates
	if input.RevealedRooms != nil {
		for roomID, revealed := range input.RevealedRooms {
			if revealed {
				dungeon.RevealRoom(roomID)
			}
		}
	}
	if input.OpenDoors != nil {
		for connID, open := range input.OpenDoors {
			if open {
				dungeon.OpenDoor(connID)
			}
		}
	}
	if input.CurrentRoomID != nil {
		dungeon.CurrentRoomID = *input.CurrentRoomID
	}
	if input.State != nil {
		dungeon.State = *input.State
	}
	if input.MonstersDefeated != nil {
		dungeon.MonstersDefeated += *input.MonstersDefeated
	}

	return &UpdateOutput{Success: true}, nil
}

// Delete removes a dungeon
func (r *InMemory) Delete(ctx context.Context, input *DeleteInput) (*DeleteOutput, error) {
	if input == nil || input.DungeonID == "" {
		return nil, errors.New("dungeon ID is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	dungeon, exists := r.dungeons[input.DungeonID]
	if !exists {
		return nil, ErrDungeonNotFound
	}

	// Clean up index
	if dungeon.EncounterID != "" {
		delete(r.encounterIndex, dungeon.EncounterID)
	}
	delete(r.dungeons, input.DungeonID)

	return &DeleteOutput{Success: true}, nil
}
```

### Step 4: Write repository tests

Create `internal/repositories/dungeons/inmemory_test.go`:

```go
package dungeons

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-api/internal/entities"
)

type InMemoryTestSuite struct {
	suite.Suite
	repo *InMemory
	ctx  context.Context
}

func TestInMemorySuite(t *testing.T) {
	suite.Run(t, new(InMemoryTestSuite))
}

func (s *InMemoryTestSuite) SetupTest() {
	s.repo = NewInMemory()
	s.ctx = context.Background()
}

func (s *InMemoryTestSuite) TestSaveAndGet() {
	dungeon := &entities.Dungeon{
		ID:          "dng-123",
		EncounterID: "enc-456",
		Theme:       entities.DungeonThemeCrypt,
		State:       entities.DungeonStateActive,
		CreatedAt:   time.Now(),
	}

	// Save
	saveOut, err := s.repo.Save(s.ctx, &SaveInput{Dungeon: dungeon})
	s.Require().NoError(err)
	s.True(saveOut.Success)

	// Get by ID
	getOut, err := s.repo.Get(s.ctx, &GetInput{DungeonID: "dng-123"})
	s.Require().NoError(err)
	s.Equal("dng-123", getOut.Dungeon.ID)
	s.Equal(entities.DungeonThemeCrypt, getOut.Dungeon.Theme)

	// Get by encounter ID
	getByEncOut, err := s.repo.GetByEncounterID(s.ctx, &GetByEncounterIDInput{EncounterID: "enc-456"})
	s.Require().NoError(err)
	s.Equal("dng-123", getByEncOut.Dungeon.ID)
}

func (s *InMemoryTestSuite) TestGetNotFound() {
	_, err := s.repo.Get(s.ctx, &GetInput{DungeonID: "nonexistent"})
	s.ErrorIs(err, ErrDungeonNotFound)
}

func (s *InMemoryTestSuite) TestUpdate() {
	dungeon := &entities.Dungeon{
		ID:            "dng-123",
		State:         entities.DungeonStateActive,
		RevealedRooms: make(map[string]bool),
		OpenDoors:     make(map[string]bool),
		CreatedAt:     time.Now(),
	}
	_, err := s.repo.Save(s.ctx, &SaveInput{Dungeon: dungeon})
	s.Require().NoError(err)

	// Update with revealed room and open door
	newState := entities.DungeonStateVictorious
	newRoomID := "room-2"
	monstersKilled := 3
	_, err = s.repo.Update(s.ctx, &UpdateInput{
		DungeonID:        "dng-123",
		RevealedRooms:    map[string]bool{"room-2": true},
		OpenDoors:        map[string]bool{"conn-1": true},
		CurrentRoomID:    &newRoomID,
		State:            &newState,
		MonstersDefeated: &monstersKilled,
	})
	s.Require().NoError(err)

	// Verify updates
	getOut, err := s.repo.Get(s.ctx, &GetInput{DungeonID: "dng-123"})
	s.Require().NoError(err)
	s.True(getOut.Dungeon.IsRoomRevealed("room-2"))
	s.True(getOut.Dungeon.IsDoorOpen("conn-1"))
	s.Equal("room-2", getOut.Dungeon.CurrentRoomID)
	s.Equal(entities.DungeonStateVictorious, getOut.Dungeon.State)
	s.Equal(3, getOut.Dungeon.MonstersDefeated)
}

func (s *InMemoryTestSuite) TestDelete() {
	dungeon := &entities.Dungeon{
		ID:          "dng-123",
		EncounterID: "enc-456",
		CreatedAt:   time.Now(),
	}
	_, err := s.repo.Save(s.ctx, &SaveInput{Dungeon: dungeon})
	s.Require().NoError(err)

	// Delete
	_, err = s.repo.Delete(s.ctx, &DeleteInput{DungeonID: "dng-123"})
	s.Require().NoError(err)

	// Verify deleted
	_, err = s.repo.Get(s.ctx, &GetInput{DungeonID: "dng-123"})
	s.ErrorIs(err, ErrDungeonNotFound)

	// Verify index cleaned up
	_, err = s.repo.GetByEncounterID(s.ctx, &GetByEncounterIDInput{EncounterID: "enc-456"})
	s.ErrorIs(err, ErrDungeonNotFound)
}
```

### Step 5: Generate mocks

```bash
cd /home/kirk/personal/rpg-api
mkdir -p internal/repositories/dungeons/mock
go generate ./internal/repositories/dungeons/...
```

### Step 6: Run tests

```bash
go test ./internal/repositories/dungeons/... -v
```

### Step 7: Commit

```bash
git add internal/entities/dungeon.go internal/repositories/dungeons/
git commit -m "feat: add Dungeon entity and repository

- Dungeon entity with exploration state tracking
- RevealedRooms, OpenDoors maps for discovery state
- In-memory repository with encounter ID index
- Full test coverage for repository"
```

### Acceptance Criteria
- [ ] Dungeon entity with all fields from design
- [ ] Helper methods: IsRoomRevealed, IsDoorOpen, RevealRoom, OpenDoor
- [ ] Repository interface with Save, Get, GetByEncounterID, Update, Delete
- [ ] In-memory implementation with proper locking
- [ ] All tests pass
- [ ] Mocks generated

---

## Issue 3: MergedGrid Implementation

**Repository:** rpg-api

**Goal:** Implement the MergedGrid that stitches rooms together as doors open.

**Context:** When a door opens, the connected room needs to be placed adjacent to the existing combat space. The MergedGrid tracks the unified coordinate system and handles position transforms.

**Depends on:** Issue 2 (uses dungeon types)

### Files

- Create: `internal/entities/merged_grid.go`
- Create: `internal/entities/merged_grid_test.go`

### Step 1: Create MergedGrid types

Create `internal/entities/merged_grid.go`:

```go
package entities

import (
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

// Position represents a coordinate in the merged grid
type Position struct {
	X int
	Y int
}

// Bounds represents a rectangular area
type Bounds struct {
	MinX, MinY int
	MaxX, MaxY int
}

// Contains checks if a position is within the bounds
func (b Bounds) Contains(p Position) bool {
	return p.X >= b.MinX && p.X <= b.MaxX && p.Y >= b.MinY && p.Y <= b.MaxY
}

// MergedGrid represents the unified combat space as rooms connect
type MergedGrid struct {
	Width  int
	Height int

	// Entities in merged coordinate space
	Entities map[string]spatial.EntityPlacement

	// Blocked positions (obstacles, closed doors)
	Blocked map[Position]bool

	// Room placement tracking
	RoomBounds  map[string]Bounds   // Room ID -> bounds in merged coords
	RoomOffsets map[string]Position // Room ID -> offset applied to room

	// Door positions in merged coords
	DoorPositions map[string]Position // Connection ID -> position
}

// NewMergedGrid creates a new merged grid
func NewMergedGrid() *MergedGrid {
	return &MergedGrid{
		Entities:      make(map[string]spatial.EntityPlacement),
		Blocked:       make(map[Position]bool),
		RoomBounds:    make(map[string]Bounds),
		RoomOffsets:   make(map[string]Position),
		DoorPositions: make(map[string]Position),
	}
}

// AddRoom places a room in the merged grid at the given offset
func (g *MergedGrid) AddRoom(roomID string, width, height int, offset Position) {
	bounds := Bounds{
		MinX: offset.X,
		MinY: offset.Y,
		MaxX: offset.X + width - 1,
		MaxY: offset.Y + height - 1,
	}

	g.RoomBounds[roomID] = bounds
	g.RoomOffsets[roomID] = offset

	// Expand grid bounds
	if bounds.MaxX >= g.Width {
		g.Width = bounds.MaxX + 1
	}
	if bounds.MaxY >= g.Height {
		g.Height = bounds.MaxY + 1
	}
}

// LocalToMerged converts a position from room-local coords to merged coords
func (g *MergedGrid) LocalToMerged(roomID string, local Position) Position {
	offset, exists := g.RoomOffsets[roomID]
	if !exists {
		return local // Room not in grid, return as-is
	}
	return Position{
		X: local.X + offset.X,
		Y: local.Y + offset.Y,
	}
}

// MergedToLocal converts a merged position to room-local coords
// Returns the room ID and local position
func (g *MergedGrid) MergedToLocal(pos Position) (roomID string, local Position) {
	for id, bounds := range g.RoomBounds {
		if bounds.Contains(pos) {
			offset := g.RoomOffsets[id]
			return id, Position{
				X: pos.X - offset.X,
				Y: pos.Y - offset.Y,
			}
		}
	}
	return "", pos
}

// AddEntity places an entity in the merged grid
func (g *MergedGrid) AddEntity(placement spatial.EntityPlacement, roomID string) {
	// Convert to merged coords
	merged := g.LocalToMerged(roomID, Position{
		X: placement.Position.X,
		Y: placement.Position.Y,
	})

	// Update placement with merged coords
	placement.Position = spatial.Position{X: merged.X, Y: merged.Y}
	g.Entities[placement.EntityID] = placement

	// Mark as blocked if it blocks movement
	if placement.BlocksMovement {
		g.Blocked[merged] = true
	}
}

// RemoveEntity removes an entity from the grid
func (g *MergedGrid) RemoveEntity(entityID string) {
	placement, exists := g.Entities[entityID]
	if !exists {
		return
	}

	pos := Position{X: placement.Position.X, Y: placement.Position.Y}
	delete(g.Blocked, pos)
	delete(g.Entities, entityID)
}

// SetDoorPosition records a door's position in merged coords
func (g *MergedGrid) SetDoorPosition(connectionID string, pos Position) {
	g.DoorPositions[connectionID] = pos
}

// OpenDoor removes a door from the blocked set
func (g *MergedGrid) OpenDoor(connectionID string) {
	pos, exists := g.DoorPositions[connectionID]
	if exists {
		delete(g.Blocked, pos)
	}
}

// CloseDoor adds a door to the blocked set
func (g *MergedGrid) CloseDoor(connectionID string) {
	pos, exists := g.DoorPositions[connectionID]
	if exists {
		g.Blocked[pos] = true
	}
}

// IsBlocked checks if a position is blocked
func (g *MergedGrid) IsBlocked(pos Position) bool {
	return g.Blocked[pos]
}

// GetEntityPosition returns an entity's position in merged coords
func (g *MergedGrid) GetEntityPosition(entityID string) (Position, bool) {
	placement, exists := g.Entities[entityID]
	if !exists {
		return Position{}, false
	}
	return Position{X: placement.Position.X, Y: placement.Position.Y}, true
}

// MoveEntity updates an entity's position
func (g *MergedGrid) MoveEntity(entityID string, newPos Position) bool {
	placement, exists := g.Entities[entityID]
	if !exists {
		return false
	}

	// Remove old blocked position if applicable
	oldPos := Position{X: placement.Position.X, Y: placement.Position.Y}
	if placement.BlocksMovement {
		delete(g.Blocked, oldPos)
	}

	// Update position
	placement.Position = spatial.Position{X: newPos.X, Y: newPos.Y}
	g.Entities[entityID] = placement

	// Add new blocked position if applicable
	if placement.BlocksMovement {
		g.Blocked[newPos] = true
	}

	return true
}

// CalculateRoomOffset determines where to place a new room based on door alignment
// doorInExisting: position of door in the existing room (merged coords)
// doorInNew: position of door in the new room (local coords)
// Returns the offset to apply to the new room
func CalculateRoomOffset(doorInExisting Position, doorInNew Position, direction string) Position {
	// The new room's door should be adjacent to the existing door
	// Direction determines which side the new room is on
	switch direction {
	case "east":
		return Position{
			X: doorInExisting.X + 1 - doorInNew.X,
			Y: doorInExisting.Y - doorInNew.Y,
		}
	case "west":
		return Position{
			X: doorInExisting.X - 1 - doorInNew.X,
			Y: doorInExisting.Y - doorInNew.Y,
		}
	case "north":
		return Position{
			X: doorInExisting.X - doorInNew.X,
			Y: doorInExisting.Y - 1 - doorInNew.Y,
		}
	case "south":
		return Position{
			X: doorInExisting.X - doorInNew.X,
			Y: doorInExisting.Y + 1 - doorInNew.Y,
		}
	default:
		// Default to east
		return Position{
			X: doorInExisting.X + 1 - doorInNew.X,
			Y: doorInExisting.Y - doorInNew.Y,
		}
	}
}
```

### Step 2: Write tests

Create `internal/entities/merged_grid_test.go`:

```go
package entities

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

type MergedGridTestSuite struct {
	suite.Suite
	grid *MergedGrid
}

func TestMergedGridSuite(t *testing.T) {
	suite.Run(t, new(MergedGridTestSuite))
}

func (s *MergedGridTestSuite) SetupTest() {
	s.grid = NewMergedGrid()
}

func (s *MergedGridTestSuite) TestAddRoom() {
	s.grid.AddRoom("room-1", 20, 20, Position{X: 0, Y: 0})

	s.Equal(20, s.grid.Width)
	s.Equal(20, s.grid.Height)

	bounds := s.grid.RoomBounds["room-1"]
	s.Equal(0, bounds.MinX)
	s.Equal(0, bounds.MinY)
	s.Equal(19, bounds.MaxX)
	s.Equal(19, bounds.MaxY)
}

func (s *MergedGridTestSuite) TestAddSecondRoom() {
	s.grid.AddRoom("room-1", 20, 20, Position{X: 0, Y: 0})
	s.grid.AddRoom("room-2", 20, 20, Position{X: 20, Y: 0})

	s.Equal(40, s.grid.Width)
	s.Equal(20, s.grid.Height)

	bounds := s.grid.RoomBounds["room-2"]
	s.Equal(20, bounds.MinX)
	s.Equal(39, bounds.MaxX)
}

func (s *MergedGridTestSuite) TestLocalToMerged() {
	s.grid.AddRoom("room-1", 20, 20, Position{X: 0, Y: 0})
	s.grid.AddRoom("room-2", 20, 20, Position{X: 20, Y: 0})

	// Position in room-2 local coords
	local := Position{X: 5, Y: 10}
	merged := s.grid.LocalToMerged("room-2", local)

	s.Equal(25, merged.X) // 20 + 5
	s.Equal(10, merged.Y)
}

func (s *MergedGridTestSuite) TestMergedToLocal() {
	s.grid.AddRoom("room-1", 20, 20, Position{X: 0, Y: 0})
	s.grid.AddRoom("room-2", 20, 20, Position{X: 20, Y: 0})

	roomID, local := s.grid.MergedToLocal(Position{X: 25, Y: 10})

	s.Equal("room-2", roomID)
	s.Equal(5, local.X)
	s.Equal(10, local.Y)
}

func (s *MergedGridTestSuite) TestAddEntity() {
	s.grid.AddRoom("room-1", 20, 20, Position{X: 0, Y: 0})

	placement := spatial.EntityPlacement{
		EntityID:       "player-1",
		EntityType:     "character",
		Position:       spatial.Position{X: 5, Y: 5},
		BlocksMovement: true,
	}
	s.grid.AddEntity(placement, "room-1")

	pos, exists := s.grid.GetEntityPosition("player-1")
	s.True(exists)
	s.Equal(5, pos.X)
	s.Equal(5, pos.Y)
	s.True(s.grid.IsBlocked(pos))
}

func (s *MergedGridTestSuite) TestMoveEntity() {
	s.grid.AddRoom("room-1", 20, 20, Position{X: 0, Y: 0})

	placement := spatial.EntityPlacement{
		EntityID:       "player-1",
		EntityType:     "character",
		Position:       spatial.Position{X: 5, Y: 5},
		BlocksMovement: true,
	}
	s.grid.AddEntity(placement, "room-1")

	// Move entity
	s.grid.MoveEntity("player-1", Position{X: 10, Y: 10})

	// Old position should be unblocked
	s.False(s.grid.IsBlocked(Position{X: 5, Y: 5}))

	// New position should be blocked
	s.True(s.grid.IsBlocked(Position{X: 10, Y: 10}))

	// Entity should be at new position
	pos, _ := s.grid.GetEntityPosition("player-1")
	s.Equal(10, pos.X)
	s.Equal(10, pos.Y)
}

func (s *MergedGridTestSuite) TestDoorOperations() {
	s.grid.AddRoom("room-1", 20, 20, Position{X: 0, Y: 0})

	doorPos := Position{X: 19, Y: 10}
	s.grid.SetDoorPosition("door-1", doorPos)
	s.grid.CloseDoor("door-1")

	s.True(s.grid.IsBlocked(doorPos))

	s.grid.OpenDoor("door-1")
	s.False(s.grid.IsBlocked(doorPos))
}

func (s *MergedGridTestSuite) TestCalculateRoomOffset() {
	// Door in existing room at east edge
	existingDoor := Position{X: 19, Y: 10}
	// Door in new room at west edge
	newDoor := Position{X: 0, Y: 10}

	offset := CalculateRoomOffset(existingDoor, newDoor, "east")

	// New room should start at X=20 so its door at local (0,10) becomes merged (20,10)
	// which is adjacent to existing door at (19,10)
	s.Equal(20, offset.X)
	s.Equal(0, offset.Y)
}
```

### Step 3: Run tests

```bash
go test ./internal/entities/... -v -run MergedGrid
```

### Step 4: Commit

```bash
git add internal/entities/merged_grid.go internal/entities/merged_grid_test.go
git commit -m "feat: add MergedGrid for multi-room combat space

- Unified coordinate system as rooms connect
- Position transform: local <-> merged coords
- Entity tracking with blocked positions
- Door position management (open/close)
- Room offset calculation for grid stitching"
```

### Acceptance Criteria
- [ ] MergedGrid struct with all fields
- [ ] AddRoom correctly expands bounds
- [ ] LocalToMerged/MergedToLocal transforms work
- [ ] Entity add/remove/move updates blocked positions
- [ ] Door open/close toggles blocked state
- [ ] CalculateRoomOffset aligns rooms correctly
- [ ] All tests pass

---

## Issue 4: Wire Dungeon Generator to Orchestrator

**Repository:** rpg-api

**Goal:** Connect the existing dungeon.Generator to the encounter orchestrator and update CreateDungeon to use it.

**Context:** The orchestrator needs the generator as a dependency. CreateDungeon should use it instead of the hardcoded room.

**Depends on:** Issue 1 (protos), Issue 2 (dungeon repo), Issue 3 (merged grid)

### Files

- Modify: `internal/orchestrators/encounter/orchestrator.go`
- Modify: `internal/orchestrators/encounter/service.go`
- Create: `internal/orchestrators/encounter/dungeon_mapper.go`
- Modify: `cmd/server/server.go`

### Step 1: Add dungeon dependencies to Config

In `internal/orchestrators/encounter/orchestrator.go`, update Config:

```go
type Config struct {
	CharacterRepo characterrepo.Repository
	EncounterRepo encounterrepo.Repository
	DungeonRepo   dungeonrepo.Repository    // ADD
	DungeonGen    *dungeon.Generator        // ADD
	Publisher     encounterpub.Publisher
	EventIDGen    idgen.Generator
}
```

And update the Orchestrator struct:

```go
type Orchestrator struct {
	charRepo    characterrepo.Repository
	encounterRepo encounterrepo.Repository
	dungeonRepo   dungeonrepo.Repository    // ADD
	dungeonGen    *dungeon.Generator        // ADD
	publisher     encounterpub.Publisher
	eventIDGen    idgen.Generator
}
```

And NewOrchestrator:

```go
func NewOrchestrator(cfg *Config) *Orchestrator {
	return &Orchestrator{
		charRepo:      cfg.CharacterRepo,
		encounterRepo: cfg.EncounterRepo,
		dungeonRepo:   cfg.DungeonRepo,    // ADD
		dungeonGen:    cfg.DungeonGen,     // ADD
		publisher:     cfg.Publisher,
		eventIDGen:    cfg.EventIDGen,
	}
}
```

### Step 2: Create dungeon mapper

Create `internal/orchestrators/encounter/dungeon_mapper.go`:

```go
package encounter

import (
	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
	"github.com/KirkDiggler/rpg-api/internal/entities"
)

// MapTheme converts proto theme to internal theme
func MapTheme(theme entities.DungeonTheme) dungeon.Theme {
	// Map to actual dungeon.Theme from the theme package
	switch theme {
	case entities.DungeonThemeCrypt:
		return dungeon.GetTheme("crypt")
	case entities.DungeonThemeCave:
		return dungeon.GetTheme("cave")
	case entities.DungeonThemeRuins:
		return dungeon.GetTheme("ruins")
	default:
		return dungeon.GetTheme("crypt") // Default
	}
}

// MapDifficulty converts difficulty to target CR multiplier
func MapDifficulty(diff entities.DungeonDifficulty, partySize int) int {
	// Base CR roughly equals average party level
	// Easy: -1, Medium: 0, Hard: +2
	baseCR := 1 // Assume level 1 parties for now
	switch diff {
	case entities.DungeonDifficultyEasy:
		return max(1, baseCR-1)
	case entities.DungeonDifficultyMedium:
		return baseCR
	case entities.DungeonDifficultyHard:
		return baseCR + 2
	default:
		return baseCR
	}
}

// MapLength converts length to room count
func MapLength(length entities.DungeonLength) int {
	switch length {
	case entities.DungeonLengthShort:
		return 4
	case entities.DungeonLengthMedium:
		return 6
	case entities.DungeonLengthLong:
		return 9
	default:
		return 6
	}
}

// MapToGeneratorInput creates dungeon generator input from config
func MapToGeneratorInput(
	theme entities.DungeonTheme,
	difficulty entities.DungeonDifficulty,
	length entities.DungeonLength,
	partySize int,
) *dungeon.GenerateInput {
	return &dungeon.GenerateInput{
		Theme:     MapTheme(theme),
		Size:      dungeon.RoomSizeMedium,
		Length:    MapLength(length),
		Layout:    dungeon.LayoutBranching,
		PartySize: partySize,
		TargetCR:  MapDifficulty(difficulty, partySize),
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
```

### Step 3: Update CreateDungeonInput/Output

In `internal/orchestrators/encounter/service.go`, update:

```go
type CreateDungeonInput struct {
	CharacterIDs []string
	Theme        entities.DungeonTheme
	Difficulty   entities.DungeonDifficulty
	Length       entities.DungeonLength
}

type CreateDungeonOutput struct {
	EncounterID   string
	DungeonID     string
	Room          *spatial.RoomData
	Doors         []DoorInfo
	CombatState   *entities.CombatState
	MonsterTurns  []MonsterTurnResult
}

type DoorInfo struct {
	ConnectionID string
	Position     entities.Position
	PhysicalHint string
	IsOpen       bool
}
```

### Step 4: Update CreateDungeon implementation

This is a larger change. Replace the hardcoded room creation in `CreateDungeon` with:

```go
func (o *Orchestrator) CreateDungeon(ctx context.Context, input *CreateDungeonInput) (*CreateDungeonOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}

	// Generate IDs
	encounterID := fmt.Sprintf("enc-%d", time.Now().UnixNano())
	dungeonID := fmt.Sprintf("dng-%d", time.Now().UnixNano())

	// Generate dungeon using the generator
	genInput := MapToGeneratorInput(
		input.Theme,
		input.Difficulty,
		input.Length,
		len(input.CharacterIDs),
	)
	genOutput, err := o.dungeonGen.Generate(ctx, genInput)
	if err != nil {
		return nil, fmt.Errorf("failed to generate dungeon: %w", err)
	}

	// Create dungeon entity
	dungeonEntity := &entities.Dungeon{
		ID:               dungeonID,
		EncounterID:      encounterID,
		Theme:            input.Theme,
		Difficulty:       input.Difficulty,
		Length:           input.Length,
		Seed:             genOutput.Seed,
		GeneratedDungeon: genOutput.Dungeon,
		RevealedRooms:    map[string]bool{genOutput.Dungeon.StartRoom: true},
		OpenDoors:        make(map[string]bool),
		CurrentRoomID:    genOutput.Dungeon.StartRoom,
		State:            entities.DungeonStateActive,
		RoomsExplored:    1,
		CreatedAt:        time.Now(),
	}

	// Save dungeon
	_, err = o.dungeonRepo.Save(ctx, &dungeonrepo.SaveInput{Dungeon: dungeonEntity})
	if err != nil {
		return nil, fmt.Errorf("failed to save dungeon: %w", err)
	}

	// Get start room
	startRoom := dungeonEntity.GetRoom(genOutput.Dungeon.StartRoom)
	if startRoom == nil {
		return nil, fmt.Errorf("start room not found in generated dungeon")
	}

	// Convert to spatial.RoomData
	roomData := convertToRoomData(encounterID, startRoom)

	// Place characters at spawn points
	spawnPositions := getSpawnPositions(startRoom)
	for i, characterID := range input.CharacterIDs {
		if i >= len(spawnPositions) {
			break
		}
		roomData.Entities[characterID] = spatial.EntityPlacement{
			EntityID:       characterID,
			EntityType:     "character",
			Position:       spawnPositions[i],
			Size:           1,
			BlocksMovement: true,
		}
	}

	// Get doors for start room
	doors := getDoorInfoForRoom(dungeonEntity, genOutput.Dungeon.StartRoom, startRoom)

	// Roll initiative and handle combat start (existing logic)
	// ... rest of existing combat initialization ...

	return &CreateDungeonOutput{
		EncounterID:  encounterID,
		DungeonID:    dungeonID,
		Room:         roomData,
		Doors:        doors,
		CombatState:  combatState,
		MonsterTurns: monsterTurns,
	}, nil
}
```

### Step 5: Add helper functions

Add to the orchestrator file:

```go
func convertToRoomData(encounterID string, room *dungeon.Room) *spatial.RoomData {
	roomData := &spatial.RoomData{
		ID:       encounterID + "-room-" + room.ID,
		Type:     "dungeon",
		Width:    room.Shape.Width,
		Height:   room.Shape.Height,
		GridType: spatial.GridTypeHex,
		Entities: make(map[string]spatial.EntityPlacement),
	}

	// Add obstacles
	for _, obstacle := range room.Features.Obstacles {
		roomData.Entities[obstacle.ID] = spatial.EntityPlacement{
			EntityID:          obstacle.ID,
			EntityType:        "obstacle",
			Position:          spatial.Position{X: obstacle.Position.X, Y: obstacle.Position.Y},
			Size:              1,
			BlocksMovement:    obstacle.BlocksMovement,
			BlocksLineOfSight: obstacle.BlocksLineOfSight,
		}
	}

	return roomData
}

func getSpawnPositions(room *dungeon.Room) []spatial.Position {
	var positions []spatial.Position
	for _, zone := range room.SpawnZones {
		if zone.Type == dungeon.ZoneTypePlayerSpawn {
			for _, pos := range zone.Bounds {
				positions = append(positions, spatial.Position{X: pos.X, Y: pos.Y})
				if len(positions) >= 4 {
					return positions
				}
			}
		}
	}
	// Fallback to default positions if no spawn zones
	if len(positions) == 0 {
		return []spatial.Position{
			{X: 5, Y: 8},
			{X: 5, Y: 10},
			{X: 5, Y: 12},
			{X: 4, Y: 10},
		}
	}
	return positions
}

func getDoorInfoForRoom(d *entities.Dungeon, roomID string, room *dungeon.Room) []DoorInfo {
	var doors []DoorInfo
	for _, conn := range d.GetDoorsForRoom(roomID) {
		connID := conn.FromRoom + "-" + conn.ToRoom
		// Find door position (edge of room based on connection)
		pos := findDoorPosition(room, conn)
		doors = append(doors, DoorInfo{
			ConnectionID: connID,
			Position:     entities.Position{X: pos.X, Y: pos.Y},
			PhysicalHint: conn.PhysicalHint,
			IsOpen:       d.IsDoorOpen(connID),
		})
	}
	return doors
}

func findDoorPosition(room *dungeon.Room, conn *dungeon.RoomConnection) spatial.Position {
	// Simple heuristic: place door at edge based on room order
	// This will be refined when we have proper door placement in dungeon generation
	return spatial.Position{X: room.Shape.Width - 1, Y: room.Shape.Height / 2}
}
```

### Step 6: Wire up in server.go

In `cmd/server/server.go`, add the dungeon repository and generator to the orchestrator config.

### Step 7: Run tests and verify

```bash
go build ./...
go test ./internal/orchestrators/encounter/... -v
```

### Step 8: Commit

```bash
git add internal/orchestrators/encounter/ cmd/server/server.go
git commit -m "feat: wire dungeon generator to encounter orchestrator

- Add DungeonRepo and DungeonGen dependencies
- Create dungeon mapper for config -> generator params
- Update CreateDungeon to use generator
- Convert generated rooms to RoomData
- Extract door info for client"
```

### Acceptance Criteria
- [ ] Orchestrator has DungeonRepo and DungeonGen dependencies
- [ ] Dungeon mapper converts enums to generator params
- [ ] CreateDungeon generates actual dungeon instead of hardcoded room
- [ ] Dungeon entity created and saved
- [ ] Start room converted to RoomData correctly
- [ ] Doors extracted for response
- [ ] Builds without errors

---

## Issue 5: OpenDoor Action Implementation

**Repository:** rpg-api

**Goal:** Implement the OpenDoor action that reveals rooms and activates monsters.

**Context:** When a player opens a door, the connected room is revealed, its monsters roll initiative and join the current combat.

**Depends on:** Issue 4 (dungeon wiring)

### Files

- Modify: `internal/orchestrators/encounter/service.go` - Add interface method
- Modify: `internal/orchestrators/encounter/orchestrator.go` - Add implementation
- Modify: `internal/handlers/dnd5e/v1alpha1/encounter/handler.go` - Add handler

### Step 1: Add OpenDoor to service interface

In `service.go`:

```go
type OpenDoorInput struct {
	EncounterID  string
	ConnectionID string
	ActorID      string // Who is opening the door
}

type OpenDoorOutput struct {
	RevealedRoom *spatial.RoomData
	RoomOffset   entities.Position
	NewDoors     []DoorInfo
	Monsters     []MonsterInfo
	CombatState  *entities.CombatState
}

type MonsterInfo struct {
	ID         string
	MonsterID  string
	Position   entities.Position
	HP         int
	MaxHP      int
	Initiative int
}
```

Add to Service interface:

```go
OpenDoor(ctx context.Context, input *OpenDoorInput) (*OpenDoorOutput, error)
```

### Step 2: Implement OpenDoor

In `orchestrator.go`:

```go
func (o *Orchestrator) OpenDoor(ctx context.Context, input *OpenDoorInput) (*OpenDoorOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}

	// Load encounter
	encOutput, err := o.encounterRepo.Get(ctx, &encounterrepo.GetInput{EncounterID: input.EncounterID})
	if err != nil {
		return nil, fmt.Errorf("failed to get encounter: %w", err)
	}

	// Load dungeon
	dngOutput, err := o.dungeonRepo.GetByEncounterID(ctx, &dungeonrepo.GetByEncounterIDInput{
		EncounterID: input.EncounterID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get dungeon: %w", err)
	}
	dng := dngOutput.Dungeon

	// Validate door exists and is closed
	if dng.IsDoorOpen(input.ConnectionID) {
		return nil, fmt.Errorf("door is already open")
	}

	conn := dng.GetConnection(input.ConnectionID)
	if conn == nil {
		return nil, fmt.Errorf("connection not found: %s", input.ConnectionID)
	}

	// Determine which room is being revealed
	var revealedRoomID string
	if dng.IsRoomRevealed(conn.FromRoom) && !dng.IsRoomRevealed(conn.ToRoom) {
		revealedRoomID = conn.ToRoom
	} else if dng.IsRoomRevealed(conn.ToRoom) && !dng.IsRoomRevealed(conn.FromRoom) {
		revealedRoomID = conn.FromRoom
	} else {
		return nil, fmt.Errorf("invalid door state: both rooms already revealed or neither revealed")
	}

	// Mark door open and room revealed
	dng.OpenDoor(input.ConnectionID)
	dng.RevealRoom(revealedRoomID)

	// Get the revealed room
	revealedRoom := dng.GetRoom(revealedRoomID)
	if revealedRoom == nil {
		return nil, fmt.Errorf("revealed room not found")
	}

	// Calculate room offset for grid merge
	// TODO: Implement proper offset calculation based on door positions
	roomOffset := entities.Position{X: 20, Y: 0} // Placeholder

	// Convert room to RoomData with offset applied
	roomData := convertToRoomData(input.EncounterID, revealedRoom)

	// Create monsters and roll initiative
	var monsters []MonsterInfo
	tracker := initiative.LoadTracker(encOutput.Data.InitiativeData)

	for _, placement := range revealedRoom.Encounter.Monsters {
		// Create monster instance
		monsterData := o.createMonster(placement)

		// Roll initiative
		dexMod := monsterData.AbilityModifier("dex")
		roll := initiative.RollForOrder(map[core.Entity]int{
			initiative.NewParticipant(monsterData.ID, "monster"): dexMod,
		})

		// Insert into tracker
		for _, entry := range roll {
			tracker.Insert(entry.Entity, entry.Total)
		}

		monsters = append(monsters, MonsterInfo{
			ID:         monsterData.ID,
			MonsterID:  placement.MonsterID,
			Position:   entities.Position{X: placement.Position.X, Y: placement.Position.Y},
			HP:         monsterData.CurrentHP,
			MaxHP:      monsterData.MaxHP,
			Initiative: roll[0].Total,
		})

		// Add monster to encounter's monster list
		encOutput.Data.Monsters = append(encOutput.Data.Monsters, monsterData)

		// Add to room entities
		roomData.Entities[monsterData.ID] = spatial.EntityPlacement{
			EntityID:       monsterData.ID,
			EntityType:     "monster",
			Position:       spatial.Position{X: placement.Position.X, Y: placement.Position.Y},
			Size:           1,
			BlocksMovement: true,
		}
	}

	// Get doors for newly revealed room
	newDoors := getDoorInfoForRoom(dng, revealedRoomID, revealedRoom)

	// Save updated state
	_, err = o.dungeonRepo.Update(ctx, &dungeonrepo.UpdateInput{
		DungeonID:     dng.ID,
		RevealedRooms: map[string]bool{revealedRoomID: true},
		OpenDoors:     map[string]bool{input.ConnectionID: true},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update dungeon: %w", err)
	}

	_, err = o.encounterRepo.Update(ctx, &encounterrepo.UpdateInput{
		EncounterID:    input.EncounterID,
		InitiativeData: tracker.Export(),
		Monsters:       encOutput.Data.Monsters,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update encounter: %w", err)
	}

	// Build combat state
	combatState := buildCombatState(input.EncounterID, tracker, encOutput.Data)

	// Publish event
	o.publishRoomRevealed(ctx, input.EncounterID, roomData, roomOffset, newDoors, monsters)

	return &OpenDoorOutput{
		RevealedRoom: roomData,
		RoomOffset:   roomOffset,
		NewDoors:     newDoors,
		Monsters:     monsters,
		CombatState:  combatState,
	}, nil
}
```

### Step 3: Add handler method

In `handler.go`:

```go
func (h *Handler) OpenDoor(ctx context.Context, req *v1alpha1.OpenDoorRequest) (*v1alpha1.OpenDoorResponse, error) {
	if req.EncounterId == "" {
		return nil, status.Error(codes.InvalidArgument, "encounter_id is required")
	}
	if req.ConnectionId == "" {
		return nil, status.Error(codes.InvalidArgument, "connection_id is required")
	}

	output, err := h.service.OpenDoor(ctx, &encounter.OpenDoorInput{
		EncounterID:  req.EncounterId,
		ConnectionID: req.ConnectionId,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to open door: %v", err)
	}

	return &v1alpha1.OpenDoorResponse{
		RevealedRoom: convertRoomToProto(output.RevealedRoom),
		RoomOffset:   &roomcommon.Position{X: int32(output.RoomOffset.X), Y: int32(output.RoomOffset.Y)},
		NewDoors:     convertDoorsToProto(output.NewDoors),
		Monsters:     convertMonstersToProto(output.Monsters),
		CombatState:  convertCombatStateToProto(output.CombatState),
	}, nil
}
```

### Step 4: Test implementation

```bash
go build ./...
go test ./internal/orchestrators/encounter/... -v
```

### Step 5: Commit

```bash
git add internal/orchestrators/encounter/ internal/handlers/
git commit -m "feat: implement OpenDoor action

- Add OpenDoor to service interface
- Implement door opening with room reveal
- Roll initiative for revealed monsters
- Insert monsters into existing initiative order
- Publish RoomRevealedEvent
- Add handler for gRPC endpoint"
```

### Acceptance Criteria
- [ ] OpenDoor validates door exists and is closed
- [ ] Correct room is revealed based on which side is already explored
- [ ] Dungeon state updated (door open, room revealed)
- [ ] Monsters created and added to initiative
- [ ] Initiative continues from current position
- [ ] RoomRevealedEvent published
- [ ] Handler returns proper response

---

## Issue 6: Victory and Failure Handling

**Repository:** rpg-api

**Goal:** Detect boss kill for victory and TPK for failure, update dungeon state.

**Context:** When the boss monster dies, the dungeon is won. When all players are down, it's a TPK.

**Depends on:** Issue 4 (dungeon wiring)

### Files

- Modify: `internal/orchestrators/encounter/orchestrator.go` - Add victory/failure checks
- Modify: `internal/entities/dungeon.go` - Add completion helpers

### Step 1: Add boss detection helper

In `entities/dungeon.go`:

```go
// IsBossRoom checks if the given room is the boss room
func (d *Dungeon) IsBossRoom(roomID string) bool {
	if d.GeneratedDungeon == nil {
		return false
	}
	return d.GeneratedDungeon.BossRoom == roomID
}

// MarkVictory marks the dungeon as completed with victory
func (d *Dungeon) MarkVictory() {
	d.State = DungeonStateVictorious
	now := time.Now()
	d.CompletedAt = &now
}

// MarkFailed marks the dungeon as failed
func (d *Dungeon) MarkFailed() {
	d.State = DungeonStateFailed
	now := time.Now()
	d.CompletedAt = &now
}
```

### Step 2: Add victory check after monster death

In `orchestrator.go`, add to `ResolveAttack` after a monster is killed:

```go
func (o *Orchestrator) checkVictoryCondition(ctx context.Context, encounterID string, killedMonsterID string) error {
	// Load dungeon
	dngOutput, err := o.dungeonRepo.GetByEncounterID(ctx, &dungeonrepo.GetByEncounterIDInput{
		EncounterID: encounterID,
	})
	if err != nil {
		return nil // Dungeon not found, skip check
	}
	dng := dngOutput.Dungeon

	// Check if killed monster was the boss
	// Boss is identified by being in boss room with role "boss"
	// For now, simplified check: any monster in boss room
	for _, room := range dng.GeneratedDungeon.Rooms {
		if room.ID == dng.GeneratedDungeon.BossRoom {
			for _, m := range room.Encounter.Monsters {
				if m.ID == killedMonsterID && m.Role == dungeon.RoleBoss {
					// Boss killed - victory!
					dng.MarkVictory()

					_, err = o.dungeonRepo.Update(ctx, &dungeonrepo.UpdateInput{
						DungeonID: dng.ID,
						State:     &dng.State,
					})
					if err != nil {
						return err
					}

					// Publish victory event
					o.publishDungeonVictory(ctx, encounterID, dng.RoomsExplored, dng.MonstersDefeated)
					return nil
				}
			}
		}
	}
	return nil
}
```

### Step 3: Add TPK check after character goes down

```go
func (o *Orchestrator) checkTPKCondition(ctx context.Context, encounterID string, encounterData *encounterrepo.EncounterData) error {
	// Check if all characters are down (HP <= 0)
	allDown := true
	for _, entity := range encounterData.RoomData.Entities {
		if entity.EntityType == "character" {
			// Load character to check HP
			charOutput, err := o.charRepo.Get(ctx, characterrepo.GetInput{ID: entity.EntityID})
			if err != nil {
				continue
			}
			if charOutput.CharacterData.CurrentHP > 0 {
				allDown = false
				break
			}
		}
	}

	if allDown {
		// TPK - dungeon failed
		dngOutput, err := o.dungeonRepo.GetByEncounterID(ctx, &dungeonrepo.GetByEncounterIDInput{
			EncounterID: encounterID,
		})
		if err != nil {
			return err
		}
		dng := dngOutput.Dungeon

		dng.MarkFailed()
		failedState := entities.DungeonStateFailed
		_, err = o.dungeonRepo.Update(ctx, &dungeonrepo.UpdateInput{
			DungeonID: dng.ID,
			State:     &failedState,
		})
		if err != nil {
			return err
		}

		// Update encounter to completed
		completedState := encounterrepo.StateCompleted
		_, err = o.encounterRepo.Update(ctx, &encounterrepo.UpdateInput{
			EncounterID: encounterID,
			State:       &completedState,
		})
		if err != nil {
			return err
		}

		// Publish failure event
		o.publishDungeonFailure(ctx, encounterID, "tpk")
	}

	return nil
}
```

### Step 4: Wire checks into existing flows

Call `checkVictoryCondition` after monster HP reaches 0 in `ResolveAttack`.
Call `checkTPKCondition` after character HP reaches 0.

### Step 5: Test

```bash
go test ./internal/orchestrators/encounter/... -v
```

### Step 6: Commit

```bash
git add internal/orchestrators/encounter/ internal/entities/dungeon.go
git commit -m "feat: add victory and failure detection

- Check for boss kill after monster death
- Check for TPK after character goes down
- Update dungeon state to victorious/failed
- Publish victory/failure events
- Dungeon can continue after victory (optional clear)"
```

### Acceptance Criteria
- [ ] Boss kill detected correctly
- [ ] DungeonVictoryEvent published on boss kill
- [ ] Dungeon state set to VICTORIOUS
- [ ] TPK detected when all characters at 0 HP
- [ ] DungeonFailureEvent published on TPK
- [ ] Dungeon state set to FAILED
- [ ] Encounter state set to COMPLETED on failure

---

## Issue 7: Handler Updates and Proto Integration

**Repository:** rpg-api

**Goal:** Update handlers to use new proto types after protos are merged.

**Context:** Once Issue 1 protos are merged, update rpg-api to use the new generated types.

**Depends on:** Issue 1 (protos merged), Issue 4-6 (implementation)

### Files

- Modify: `internal/handlers/dnd5e/v1alpha1/encounter/handler.go`
- Modify: `internal/handlers/dnd5e/v1alpha1/encounter/converters.go`

### Step 1: Update proto import

```bash
cd /home/kirk/personal/rpg-api
GOPROXY=direct go get github.com/KirkDiggler/rpg-api-protos/gen/go@generated
```

### Step 2: Update DungeonStart handler

```go
func (h *Handler) DungeonStart(ctx context.Context, req *v1alpha1.DungeonStartRequest) (*v1alpha1.DungeonStartResponse, error) {
	// Map proto enums to internal types
	theme := mapProtoTheme(req.Theme)
	difficulty := mapProtoDifficulty(req.Difficulty)
	length := mapProtoLength(req.Length)

	output, err := h.service.CreateDungeon(ctx, &encounter.CreateDungeonInput{
		CharacterIDs: req.CharacterIds,
		Theme:        theme,
		Difficulty:   difficulty,
		Length:       length,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create dungeon: %v", err)
	}

	return &v1alpha1.DungeonStartResponse{
		EncounterId:  output.EncounterID,
		DungeonId:    output.DungeonID,
		Room:         convertRoomToProto(output.Room),
		Doors:        convertDoorsToProto(output.Doors),
		CombatState:  convertCombatStateToProto(output.CombatState),
		MonsterTurns: convertMonsterTurnsToProto(output.MonsterTurns),
	}, nil
}
```

### Step 3: Add converters

In `converters.go`:

```go
func mapProtoTheme(t v1alpha1.DungeonTheme) entities.DungeonTheme {
	switch t {
	case v1alpha1.DungeonTheme_DUNGEON_THEME_CRYPT:
		return entities.DungeonThemeCrypt
	case v1alpha1.DungeonTheme_DUNGEON_THEME_CAVE:
		return entities.DungeonThemeCave
	case v1alpha1.DungeonTheme_DUNGEON_THEME_RUINS:
		return entities.DungeonThemeRuins
	default:
		return entities.DungeonThemeCrypt
	}
}

func mapProtoDifficulty(d v1alpha1.DungeonDifficulty) entities.DungeonDifficulty {
	switch d {
	case v1alpha1.DungeonDifficulty_DUNGEON_DIFFICULTY_EASY:
		return entities.DungeonDifficultyEasy
	case v1alpha1.DungeonDifficulty_DUNGEON_DIFFICULTY_MEDIUM:
		return entities.DungeonDifficultyMedium
	case v1alpha1.DungeonDifficulty_DUNGEON_DIFFICULTY_HARD:
		return entities.DungeonDifficultyHard
	default:
		return entities.DungeonDifficultyMedium
	}
}

func mapProtoLength(l v1alpha1.DungeonLength) entities.DungeonLength {
	switch l {
	case v1alpha1.DungeonLength_DUNGEON_LENGTH_SHORT:
		return entities.DungeonLengthShort
	case v1alpha1.DungeonLength_DUNGEON_LENGTH_MEDIUM:
		return entities.DungeonLengthMedium
	case v1alpha1.DungeonLength_DUNGEON_LENGTH_LONG:
		return entities.DungeonLengthLong
	default:
		return entities.DungeonLengthMedium
	}
}

func convertDoorsToProto(doors []encounter.DoorInfo) []*v1alpha1.DoorInfo {
	result := make([]*v1alpha1.DoorInfo, len(doors))
	for i, d := range doors {
		result[i] = &v1alpha1.DoorInfo{
			ConnectionId: d.ConnectionID,
			Position:     &roomcommon.Position{X: int32(d.Position.X), Y: int32(d.Position.Y)},
			PhysicalHint: d.PhysicalHint,
			IsOpen:       d.IsOpen,
		}
	}
	return result
}
```

### Step 4: Run pre-commit

```bash
make pre-commit
```

### Step 5: Commit

```bash
git add internal/handlers/ go.mod go.sum
git commit -m "feat: integrate dungeon proto types in handlers

- Update proto dependency to latest generated
- Map proto enums to internal types
- Add DoorInfo conversion
- Update DungeonStart handler for new request/response"
```

### Acceptance Criteria
- [ ] Proto dependency updated
- [ ] Enum mappers work correctly
- [ ] DungeonStart handler uses new fields
- [ ] OpenDoor handler implemented
- [ ] All converters in place
- [ ] Pre-commit passes

---

## Dependency Graph

```
Issue 1 (Protos) ─────────────────────────────────────────┐
                                                          │
Issue 2 (Entity/Repo) ──────────────┐                     │
                                    │                     │
Issue 3 (MergedGrid) ───────────────┼─────┐               │
                                    │     │               │
                                    ▼     ▼               │
Issue 4 (Wire Generator) ◄──────────┴─────┘               │
        │                                                 │
        ├───────────────────────────┐                     │
        │                           │                     │
        ▼                           ▼                     │
Issue 5 (OpenDoor)           Issue 6 (Victory/Failure)    │
        │                           │                     │
        └───────────┬───────────────┘                     │
                    │                                     │
                    ▼                                     │
           Issue 7 (Handler Integration) ◄────────────────┘
```

**Parallel work possible:**
- Issues 2 and 3 can run in parallel
- Issues 5 and 6 can run in parallel after Issue 4
- Issue 1 can run independently in rpg-api-protos

---

## Quick Reference: File Locations

| Component | Location |
|-----------|----------|
| Dungeon entity | `internal/entities/dungeon.go` |
| MergedGrid | `internal/entities/merged_grid.go` |
| Dungeon repo | `internal/repositories/dungeons/` |
| Encounter orchestrator | `internal/orchestrators/encounter/` |
| Encounter handler | `internal/handlers/dnd5e/v1alpha1/encounter/` |
| Dungeon generator | `internal/components/dungeon/` |
| Proto definitions | rpg-api-protos: `dnd5e/api/v1alpha1/` |
