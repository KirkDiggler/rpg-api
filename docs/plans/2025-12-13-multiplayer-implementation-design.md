# Multiplayer Implementation Design for rpg-api

**Date:** 2025-12-13
**Status:** Draft
**Related:** rpg-api-protos PR #89 (proto definitions)

## Overview

Implement real-time multiplayer support in rpg-api for the encounter system. This enables multiple players to participate in the same combat encounter, seeing each other's actions in real-time.

## Architecture Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Event publishing location | Orchestrator | Handler stays thin (proto conversion only), orchestrator owns business logic including event publishing |
| Event types location | `internal/entities/` | Events are just data, shared across orchestrator/handler/publisher layers |
| Internal communication | Redis pub/sub | Decouples action handlers from stream handlers, scales horizontally, proven pattern |
| Event type format | Internal types (not proto) | Keeps orchestrator decoupled from proto, handler converts at the edge |

## High-Level Flow

```
Player A calls MoveCharacter (unary RPC)
    │
    ▼
Handler converts proto → internal types
    │
    ▼
Orchestrator executes move, updates state
    │
    ▼
Orchestrator publishes MovementCompletedEvent to Redis
(channel: encounter:{id}:events)
    │
    ▼
All StreamEncounterEvents handlers subscribed to that channel
receive the event, convert to proto, push to gRPC streams
    │
    ▼
All players (including Player A) see the movement
```

## Package Structure

```
internal/
├── entities/
│   └── encounter_events.go      # NEW: Event types (just data)
├── publishers/
│   └── encounter/
│       ├── publisher.go         # NEW: Publisher interface
│       └── redis.go             # NEW: Redis implementation
├── repositories/
│   └── encounters/
│       └── repository.go        # MODIFIED: Add multiplayer fields
├── orchestrators/
│   └── encounter/
│       ├── service.go           # MODIFIED: Add lobby methods
│       └── orchestrator.go      # MODIFIED: Inject publisher, emit events
└── handlers/
    └── dnd5e/v1alpha1/encounter/
        ├── handler.go           # MODIFIED: Add lobby RPCs
        └── stream.go            # NEW: StreamEncounterEvents handler
```

## Component Details

### 1. Event Types (`internal/entities/encounter_events.go`)

```go
package entities

import "time"

type EventType string

const (
    EventTypePlayerJoined        EventType = "player_joined"
    EventTypePlayerLeft          EventType = "player_left"
    EventTypePlayerReady         EventType = "player_ready"
    EventTypeCombatStarted       EventType = "combat_started"
    EventTypeMovementCompleted   EventType = "movement_completed"
    EventTypeAttackResolved      EventType = "attack_resolved"
    EventTypeFeatureActivated    EventType = "feature_activated"
    EventTypeTurnEnded           EventType = "turn_ended"
    EventTypeMonsterTurnCompleted EventType = "monster_turn_completed"
    EventTypeCombatEnded         EventType = "combat_ended"
    EventTypePlayerDisconnected  EventType = "player_disconnected"
    EventTypePlayerReconnected   EventType = "player_reconnected"
    EventTypeCombatPaused        EventType = "combat_paused"
    EventTypeCombatResumed       EventType = "combat_resumed"
)

// EncounterEvent wraps all event types
type EncounterEvent struct {
    ID        string
    Timestamp time.Time
    Type      EventType

    // Only one of these is set based on Type
    PlayerJoined         *PlayerJoinedEvent
    PlayerLeft           *PlayerLeftEvent
    PlayerReady          *PlayerReadyEvent
    CombatStarted        *CombatStartedEvent
    MovementCompleted    *MovementCompletedEvent
    AttackResolved       *AttackResolvedEvent
    FeatureActivated     *FeatureActivatedEvent
    TurnEnded            *TurnEndedEvent
    MonsterTurnCompleted *MonsterTurnCompletedEvent
    CombatEnded          *CombatEndedEvent
    PlayerDisconnected   *PlayerDisconnectedEvent
    PlayerReconnected    *PlayerReconnectedEvent
    CombatPaused         *CombatPausedEvent
    CombatResumed        *CombatResumedEvent
}

// Individual event types - reuse existing orchestrator types where possible

type PlayerJoinedEvent struct {
    PlayerID    string
    CharacterID string
    IsHost      bool
}

type PlayerLeftEvent struct {
    PlayerID    string
    CharacterID string
}

type PlayerReadyEvent struct {
    PlayerID string
    IsReady  bool
}

type CombatStartedEvent struct {
    CombatState interface{} // *encounter.CombatState
    Room        interface{} // Room data
    Players     []PlayerInfo
}

type PlayerInfo struct {
    PlayerID    string
    CharacterID string
    IsHost      bool
    IsReady     bool
    IsConnected bool
}

type MovementCompletedEvent struct {
    EntityID          string
    Path              []Position
    FinalPosition     Position
    MovementRemaining int32
    StopReason        string
    UpdatedRoom       interface{}
}

type Position struct {
    X float64
    Y float64
}

type AttackResolvedEvent struct {
    AttackerID      string
    TargetID        string
    Result          interface{} // *encounter.AttackResult
    UpdatedAttacker interface{} // Character data if changed
    UpdatedTarget   interface{} // Character/monster with new HP
    UpdatedRoom     interface{}
}

type FeatureActivatedEvent struct {
    CharacterID      string
    FeatureID        string
    Message          string
    UpdatedCharacter interface{}
}

type TurnEndedEvent struct {
    TurnChange  interface{} // *encounter.TurnChangeEvent
    CombatState interface{} // *encounter.CombatState
}

type MonsterTurnCompletedEvent struct {
    MonsterTurn       interface{} // *encounter.MonsterTurnResult
    UpdatedCharacters []interface{}
    UpdatedRoom       interface{}
}

type CombatEndedEvent struct {
    Reason string // "victory" or "defeat"
}

type PlayerDisconnectedEvent struct {
    PlayerID    string
    CharacterID string
}

type PlayerReconnectedEvent struct {
    PlayerID string
    Player   PlayerInfo
}

type CombatPausedEvent struct {
    Reason                string
    DisconnectedPlayerID  string
}

type CombatResumedEvent struct {
    CombatState interface{}
}
```

### 2. Publisher Interface (`internal/publishers/encounter/publisher.go`)

```go
package encounter

import (
    "context"
    "github.com/KirkDiggler/rpg-api/internal/entities"
)

type Publisher interface {
    // Publish sends an event to all subscribers of an encounter
    Publish(ctx context.Context, encounterID string, event *entities.EncounterEvent) error

    // Subscribe returns a channel that receives events for an encounter
    Subscribe(ctx context.Context, encounterID string) (<-chan *entities.EncounterEvent, error)

    // Unsubscribe stops receiving events and cleans up resources
    Unsubscribe(ctx context.Context, encounterID string, ch <-chan *entities.EncounterEvent) error
}
```

### 3. Redis Implementation (`internal/publishers/encounter/redis.go`)

```go
package encounter

import (
    "context"
    "encoding/json"
    "fmt"

    "github.com/redis/go-redis/v9"
    "github.com/KirkDiggler/rpg-api/internal/entities"
)

type RedisPublisher struct {
    client *redis.Client
}

type RedisConfig struct {
    Client *redis.Client
}

func NewRedisPublisher(cfg *RedisConfig) *RedisPublisher {
    return &RedisPublisher{client: cfg.Client}
}

func (p *RedisPublisher) channelName(encounterID string) string {
    return fmt.Sprintf("encounter:%s:events", encounterID)
}

func (p *RedisPublisher) Publish(ctx context.Context, encounterID string, event *entities.EncounterEvent) error {
    data, err := json.Marshal(event)
    if err != nil {
        return fmt.Errorf("failed to marshal event: %w", err)
    }

    return p.client.Publish(ctx, p.channelName(encounterID), data).Err()
}

func (p *RedisPublisher) Subscribe(ctx context.Context, encounterID string) (<-chan *entities.EncounterEvent, error) {
    pubsub := p.client.Subscribe(ctx, p.channelName(encounterID))

    events := make(chan *entities.EncounterEvent)

    go func() {
        defer close(events)
        for {
            select {
            case <-ctx.Done():
                pubsub.Close()
                return
            case msg, ok := <-pubsub.Channel():
                if !ok {
                    return
                }
                var event entities.EncounterEvent
                if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
                    continue // Log and skip bad messages
                }
                select {
                case events <- &event:
                case <-ctx.Done():
                    return
                }
            }
        }
    }()

    return events, nil
}

func (p *RedisPublisher) Unsubscribe(ctx context.Context, encounterID string, ch <-chan *entities.EncounterEvent) error {
    // Channel cleanup happens via context cancellation
    // This method exists for explicit cleanup if needed
    return nil
}
```

### 4. Encounter State Machine

**New fields in `EncounterData`:**

```go
type EncounterData struct {
    // Existing fields...
    ID                string
    RoomData          *spatial.RoomData
    InitiativeData    *initiative.TrackerData
    Monsters          []*monster.Data
    MovementRemaining int32

    // NEW: Multiplayer fields
    State     EncounterState
    JoinCode  string
    HostID    string
    Players   map[string]*Player
    CreatedAt time.Time
}

type EncounterState string

const (
    StateWaiting   EncounterState = "waiting"
    StateActive    EncounterState = "active"
    StatePaused    EncounterState = "paused"
    StateCompleted EncounterState = "completed"
)

type Player struct {
    PlayerID    string
    CharacterID string
    IsReady     bool
    IsConnected bool
    JoinedAt    time.Time
}
```

**State transitions:**

```
WAITING ──StartCombat──► ACTIVE ──Victory/TPK──► COMPLETED
    │                      │
    │                      │ Disconnect
    │                      ▼
    │                   PAUSED
    │                      │
    │                      │ Reconnect
    │                      ▼
    │                    ACTIVE
    │
    └──LeaveEncounter──► (deleted if empty)
```

**Validation rules:**
- `JoinEncounter`: Only in WAITING state
- `StartCombat`: Only in WAITING, all players ready, called by host
- Combat actions: Only in ACTIVE state
- Auto-transition to PAUSED on disconnect, back to ACTIVE on reconnect

### 5. New Orchestrator Methods

```go
type Service interface {
    // Existing methods...
    ResolveAttack(ctx context.Context, input *ResolveAttackInput) (*ResolveAttackOutput, error)
    CreateDungeon(ctx context.Context, input *CreateDungeonInput) (*CreateDungeonOutput, error)
    MoveCharacter(ctx context.Context, input *MoveCharacterInput) (*MoveCharacterOutput, error)
    EndTurn(ctx context.Context, input *EndTurnInput) (*EndTurnOutput, error)
    ActivateFeature(ctx context.Context, input *ActivateFeatureInput) (*ActivateFeatureOutput, error)

    // NEW: Lobby management
    CreateEncounter(ctx context.Context, input *CreateEncounterInput) (*CreateEncounterOutput, error)
    JoinEncounter(ctx context.Context, input *JoinEncounterInput) (*JoinEncounterOutput, error)
    SetReady(ctx context.Context, input *SetReadyInput) (*SetReadyOutput, error)
    StartCombat(ctx context.Context, input *StartCombatInput) (*StartCombatOutput, error)
    LeaveEncounter(ctx context.Context, input *LeaveEncounterInput) (*LeaveEncounterOutput, error)

    // NEW: Connection events
    PlayerDisconnected(ctx context.Context, input *PlayerDisconnectedInput) (*PlayerDisconnectedOutput, error)
    PlayerReconnected(ctx context.Context, input *PlayerReconnectedInput) (*PlayerReconnectedOutput, error)
}
```

**Input/Output types:**

```go
type CreateEncounterInput struct {
    PlayerID     string
    CharacterIDs []string
}

type CreateEncounterOutput struct {
    EncounterID string
    JoinCode    string
    Room        interface{}
}

type JoinEncounterInput struct {
    JoinCode     string
    PlayerID     string
    CharacterIDs []string
}

type JoinEncounterOutput struct {
    EncounterID string
    Room        interface{}
    Players     []*PlayerInfo
    State       EncounterState
}

type SetReadyInput struct {
    EncounterID string
    PlayerID    string
    IsReady     bool
}

type SetReadyOutput struct {
    Success bool
}

type StartCombatInput struct {
    EncounterID string
    PlayerID    string // Must be host
}

type StartCombatOutput struct {
    CombatState  *CombatState
    MonsterTurns []*MonsterTurnResult
}

type LeaveEncounterInput struct {
    EncounterID string
    PlayerID    string
}

type LeaveEncounterOutput struct {
    Success bool
}

type PlayerDisconnectedInput struct {
    EncounterID string
    PlayerID    string
}

type PlayerDisconnectedOutput struct {
    CombatPaused bool
}

type PlayerReconnectedInput struct {
    EncounterID string
    PlayerID    string
}

type PlayerReconnectedOutput struct {
    CombatResumed bool
    CombatState   *CombatState
}
```

### 6. Stream Handler

```go
func (h *Handler) StreamEncounterEvents(
    req *pb.StreamEncounterEventsRequest,
    stream pb.EncounterService_StreamEncounterEventsServer,
) error {
    ctx := stream.Context()

    // 1. Subscribe to encounter events via Redis pub/sub
    events, err := h.publisher.Subscribe(ctx, req.EncounterId)
    if err != nil {
        return status.Errorf(codes.Internal, "failed to subscribe: %v", err)
    }
    defer h.publisher.Unsubscribe(ctx, req.EncounterId, events)

    // 2. Forward events until context cancelled (client disconnect)
    for {
        select {
        case <-ctx.Done():
            // Client disconnected - notify orchestrator
            h.orchestrator.PlayerDisconnected(ctx, &encounter.PlayerDisconnectedInput{
                EncounterID: req.EncounterId,
                PlayerID:    req.PlayerId,
            })
            return nil

        case event, ok := <-events:
            if !ok {
                return nil // Channel closed
            }

            // Convert internal event to proto and send
            protoEvent := h.convertEventToProto(event)
            if err := stream.Send(protoEvent); err != nil {
                return err
            }
        }
    }
}
```

### 7. Event Publishing in Existing Methods

Each existing orchestrator method publishes events after completing:

| Method | Event(s) Published |
|--------|-------------------|
| `CreateEncounter` | None (caller starts stream after) |
| `JoinEncounter` | `PlayerJoinedEvent` |
| `SetReady` | `PlayerReadyEvent` |
| `StartCombat` | `CombatStartedEvent` |
| `LeaveEncounter` | `PlayerLeftEvent` |
| `MoveCharacter` | `MovementCompletedEvent` |
| `ResolveAttack` | `AttackResolvedEvent` |
| `EndTurn` | `TurnEndedEvent` + `MonsterTurnCompletedEvent`(s) + possibly `CombatEndedEvent` |
| `ActivateFeature` | `FeatureActivatedEvent` |
| `PlayerDisconnected` | `PlayerDisconnectedEvent` + `CombatPausedEvent` (if in combat) |
| `PlayerReconnected` | `PlayerReconnectedEvent` + `CombatResumedEvent` (if was paused) |

**Example pattern:**

```go
func (o *Orchestrator) MoveCharacter(ctx context.Context, input *MoveCharacterInput) (*MoveCharacterOutput, error) {
    // ... existing validation and movement logic ...

    // Publish event for all subscribers
    o.publisher.Publish(ctx, input.EncounterID, &entities.EncounterEvent{
        ID:        uuid.NewString(),
        Timestamp: time.Now(),
        Type:      entities.EventTypeMovementCompleted,
        MovementCompleted: &entities.MovementCompletedEvent{
            EntityID:          input.EntityID,
            Path:              path,
            FinalPosition:     *output.FinalPosition,
            MovementRemaining: output.MovementRemaining,
            StopReason:        output.StopReason,
            UpdatedRoom:       output.UpdatedRoom,
        },
    })

    return output, nil
}
```

## Join Code Generation

Simple 6-character alphanumeric code:

```go
func generateJoinCode() string {
    const chars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // No I/O/0/1 to avoid confusion
    code := make([]byte, 6)
    for i := range code {
        code[i] = chars[rand.Intn(len(chars))]
    }
    return string(code)
}
```

Store in encounter data and create a lookup index (encounterID by joinCode) in the repository.

## Testing Strategy

1. **Unit tests for orchestrator methods** - Mock publisher, verify events published
2. **Integration tests with miniredis** - Test pub/sub flow end-to-end
3. **Handler tests** - Mock orchestrator, verify proto conversion

## Implementation Order

1. Event types in `internal/entities/`
2. Publisher interface and Redis implementation
3. Repository changes (multiplayer fields, join code index)
4. New orchestrator methods (lobby management)
5. Update existing orchestrator methods to publish events
6. Stream handler
7. Unary handlers for lobby RPCs
8. Handler converters (internal events ↔ proto)

## Future Enhancements

- **Leader kick**: Host can remove disconnected players to continue
- **Reconnection timeout**: Auto-remove players after X minutes disconnected
- **Spectator mode**: Watch without participating
- **Chat/emotes**: In-game communication via events
