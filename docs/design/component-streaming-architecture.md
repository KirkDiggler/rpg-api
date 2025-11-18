# Component-Based Streaming Architecture

## Vision

Multi-stream architecture where each component (Character, Room, CombatLog, Notifications) owns its data and provides both sync (Get) and async (Stream) access.

## Core Principle

**All Get RPCs return event_id** - This enables seamless migration from polling to streaming:

```
Phase 2 (Now): Client polls GetCharacter every 500ms
Phase 3 (Later): Client calls GetCharacter once, then StreamCharacter(from_event_id=X)
```

## Architecture Overview

```
┌──────────────────────────────────────────────────────────┐
│ Orchestrators (Business Logic)                           │
│ - CharacterOrchestrator                                  │
│ - EncounterOrchestrator                                  │
│ - Use components, don't know about streaming             │
└──────────────────────────────────────────────────────────┘
                            ↓
┌──────────────────────────────────────────────────────────┐
│ Components (Data + Streaming)                            │
│                                                           │
│ ┌────────────────┐  ┌────────────────┐                  │
│ │ Character      │  │ Room           │                  │
│ │ Component      │  │ Component      │                  │
│ │                │  │                │                  │
│ │ - Repository   │  │ - Repository   │                  │
│ │ - PubSub       │  │ - PubSub       │                  │
│ │ - GetSnapshot  │  │ - GetSnapshot  │                  │
│ │ - Stream       │  │ - Stream       │                  │
│ └────────────────┘  └────────────────┘                  │
│                                                           │
│ ┌────────────────┐  ┌────────────────┐                  │
│ │ CombatLog      │  │ Notification   │                  │
│ │ Component      │  │ Component      │                  │
│ │                │  │                │                  │
│ │ - Aggregates   │  │ - Private msgs │                  │
│ │ - Formatting   │  │ - Per-player   │                  │
│ │ - Stream       │  │ - Stream       │                  │
│ └────────────────┘  └────────────────┘                  │
└──────────────────────────────────────────────────────────┘
                            ↓
┌──────────────────────────────────────────────────────────┐
│ Infrastructure                                            │
│ - Redis (Repository + PubSub)                            │
│ - Event ID generation (Redis INCR per stream)            │
└──────────────────────────────────────────────────────────┘
```

## Components

### 1. Character Component

**Owns**: Character state, conditions, equipment, HP
**Visibility**:
- Owner sees full details (exact HP, all conditions)
- Others see public state (position, "bloodied", visible equipment)

```protobuf
service CharacterService {
  // Sync access (Phase 2)
  rpc GetCharacter(GetCharacterRequest) returns (CharacterSnapshot);

  // Streaming (Phase 3)
  rpc StreamCharacter(StreamCharacterRequest) returns (stream CharacterEvent);
}

message GetCharacterRequest {
  string character_id = 1;
  string player_id = 2;  // For permission check
}

message CharacterSnapshot {
  int64 event_id = 1;  // ← CRITICAL: Enables streaming later
  string character_id = 2;

  // Full state
  int32 current_hp = 3;
  int32 max_hp = 4;
  repeated Condition conditions = 5;
  repeated EquippedItem equipment = 6;
  Position position = 7;
}

message StreamCharacterRequest {
  string character_id = 1;
  int64 from_event_id = 2;  // Stream from this point forward
}

message CharacterEvent {
  int64 event_id = 1;
  string character_id = 2;

  oneof event_type {
    HPChanged hp_changed = 10;
    ConditionApplied condition_applied = 11;
    ConditionRemoved condition_removed = 12;
    ItemEquipped item_equipped = 13;
    ItemUnequipped item_unequipped = 14;
  }
}
```

**Component Interface:**
```go
type CharacterComponent interface {
    // Repository operations (wrap existing repo)
    Get(ctx, characterID) (*CharacterSnapshot, error)
    UpdateHP(ctx, characterID, newHP) error

    // Streaming operations (Phase 3)
    Stream(ctx, characterID, fromEventID) (<-chan CharacterEvent, error)

    // Internal: publish events
    publishEvent(event CharacterEvent) error
}
```

### 2. Room Component

**Owns**: Room state, entity positions, revealed areas
**Visibility**: Everyone in encounter sees same state

```protobuf
service RoomService {
  // Sync access (Phase 2)
  rpc GetRoom(GetRoomRequest) returns (RoomSnapshot);

  // Streaming (Phase 3)
  rpc StreamRoom(StreamRoomRequest) returns (stream RoomEvent);
}

message RoomSnapshot {
  int64 event_id = 1;  // ← CRITICAL
  string room_id = 2;
  Room room = 3;  // Full room state
}

message RoomEvent {
  int64 event_id = 1;
  string room_id = 2;

  oneof event_type {
    EntityMoved entity_moved = 10;
    EntityAdded entity_added = 11;
    EntityRemoved entity_removed = 12;
    TrapRevealed trap_revealed = 13;
    DoorOpened door_opened = 14;
  }
}
```

### 3. Combat Log Component

**Owns**: Formatted narrative of what happened
**Visibility**: Public to all players in encounter
**Purpose**: The storytelling layer

```protobuf
service CombatLogService {
  // Get recent history (Phase 2)
  rpc GetCombatLog(GetCombatLogRequest) returns (CombatLogSnapshot);

  // Streaming (Phase 3)
  rpc StreamCombatLog(StreamCombatLogRequest) returns (stream LogEntry);
}

message CombatLogSnapshot {
  int64 event_id = 1;  // ← CRITICAL
  string encounter_id = 2;
  repeated LogEntry entries = 3;  // Recent history (last 50?)
}

message LogEntry {
  int64 event_id = 1;
  string timestamp = 2;
  string formatted_text = 3;  // "Grog attacks Goblin for 12 damage"

  // Rich data for UI rendering
  LogEntryType type = 4;  // ATTACK, MOVEMENT, TRAP, etc.
  map<string, string> metadata = 5;  // actor_id, target_id, damage, etc.
}

// This is what toolkit integration tests produce!
// ~/personal/rpg-toolkit/rulebooks/dnd5e/combat/integration.md
```

**Example Log Entries:**
```
[18:45:23] Grog moves from (1,1) to (3,1)
[18:45:25] Grog triggers poison dart trap!
[18:45:25] Grog makes DEX save: 1d20(12) + 2 = 14 (DC 13) - Success!
[18:45:25] Grog takes 2 poison damage (half from save)
[18:45:30] Grog attacks Goblin: 1d20(15) + 5 = 20 vs AC 13 - Hit!
[18:45:30] Damage: 1d12(8) + 3 STR + 2 Rage = 13 slashing damage
```

### 4. Notification Component

**Owns**: Private player notifications
**Visibility**: Per-player only
**Purpose**: Secret information, turn notifications

```protobuf
service NotificationService {
  rpc StreamNotifications(StreamNotificationRequest) returns (stream Notification);
}

message StreamNotificationRequest {
  string player_id = 1;
  string encounter_id = 2;
  int64 from_event_id = 3;
}

message Notification {
  int64 event_id = 1;
  NotificationType type = 2;
  string message = 3;
  map<string, string> details = 4;
}

enum NotificationType {
  YOUR_TURN = 0;
  PERCEPTION_RESULT = 1;  // "You notice a secret door"
  SAVING_THROW = 2;
  DAMAGE_TAKEN = 3;
  CONDITION_APPLIED = 4;
}
```

**Example Notifications:**
```
// Public combat log shows:
"Grog makes a perception check"

// Private notification to Grog's player:
{
  type: PERCEPTION_RESULT,
  message: "You notice a hidden pressure plate at (5,3)",
  details: {
    roll: "18",
    dc: "15",
    location: "(5,3)"
  }
}
```

## Event ID Strategy

**Per-Stream Event IDs** (not global):
- `character:<id>:events` → Counter starts at 1
- `room:<id>:events` → Counter starts at 1
- `combat-log:<encounter-id>:events` → Counter starts at 1
- `notifications:<player-id>:events` → Counter starts at 1

```go
// Redis keys
characterEventID := redis.Incr("character:char-123:event-counter")
roomEventID := redis.Incr("room:room-456:event-counter")
```

**Why separate counters?**
- Simpler: Each stream has its own sequence
- Independent: Character events don't affect room event IDs
- Scalable: No global lock on event ID generation

## Phase 2 Implementation (Current)

**All Get RPCs include event_id in response:**

```protobuf
// Character
message CharacterSnapshot {
  int64 event_id = 1;  // ← Add this NOW
  // ... rest of fields
}

// Room
message RoomSnapshot {
  int64 event_id = 1;  // ← Add this NOW
  Room room = 2;
}

// Combat Log
message CombatLogSnapshot {
  int64 event_id = 1;  // ← Add this NOW
  repeated LogEntry recent_entries = 2;
}
```

**Backend Changes:**
```go
// When saving character state
func (c *CharacterComponent) UpdateHP(ctx, characterID, newHP) error {
    // Increment event counter
    eventID := c.redis.Incr("character:" + characterID + ":event-counter")

    // Save to repository
    c.repo.UpdateHP(...)

    // Publish event (Phase 3)
    // c.pubsub.Publish("character:"+characterID, HPChangedEvent{
    //     EventID: eventID,
    //     NewHP: newHP,
    // })

    return nil
}
```

**Client behavior (Phase 2):**
```javascript
// Poll every 500ms
setInterval(() => {
  const snapshot = await getCharacter(characterID)
  // snapshot.event_id is ignored for now, but it's there
  updateUI(snapshot)
}, 500)
```

## Phase 3 Implementation (Streaming)

**Client migrates to streaming:**
```javascript
// Initial state
const snapshot = await getCharacter(characterID)
updateUI(snapshot)

// Stream updates from that point forward
const stream = streamCharacter(characterID, snapshot.event_id)
for await (const event of stream) {
  applyEvent(event)  // Incrementally update state
}
```

**Backend enables streaming:**
```go
func (c *CharacterComponent) Stream(ctx, characterID, fromEventID) (<-chan Event) {
    ch := make(chan CharacterEvent)

    // Subscribe to Redis pub/sub
    sub := c.pubsub.Subscribe("character:" + characterID)

    go func() {
        for msg := range sub {
            event := parseEvent(msg)
            // Only send events after fromEventID
            if event.EventID > fromEventID {
                ch <- event
            }
        }
    }()

    return ch
}
```

## Client Subscription Pattern

**Each client subscribes to multiple streams:**

```javascript
const encounter = {
  characterID: "char-123",
  roomID: "room-456",
  encounterID: "enc-789",
  playerID: "player-abc"
}

// Subscribe to all relevant streams
const streams = await Promise.all([
  streamCharacter(encounter.characterID, lastCharacterEventID),
  streamRoom(encounter.roomID, lastRoomEventID),
  streamCombatLog(encounter.encounterID, lastLogEventID),
  streamNotifications(encounter.playerID, lastNotificationEventID)
])

// Handle events from all streams
for await (const event of mergeStreams(streams)) {
  switch (event.stream) {
    case 'character':
      updateCharacter(event)
      break
    case 'room':
      updateRoom(event)
      break
    case 'combatLog':
      appendToLog(event)
      break
    case 'notification':
      showNotification(event)
      break
  }
}
```

## Component Implementation Example

```go
// internal/components/character/component.go

type Component struct {
    repo   characterrepo.Repository
    redis  *redis.Client
    pubsub redis.PubSub
}

func (c *Component) Get(ctx context.Context, characterID string) (*Snapshot, error) {
    // Get from repository
    char, err := c.repo.Get(ctx, characterID)
    if err != nil {
        return nil, err
    }

    // Get current event counter
    eventID, err := c.redis.Get(ctx, "character:"+characterID+":event-counter").Int64()
    if err != nil {
        eventID = 0  // First time
    }

    return &Snapshot{
        EventID:   eventID,
        Character: char,
    }, nil
}

func (c *Component) UpdateHP(ctx context.Context, characterID string, newHP int) error {
    // Increment event counter
    eventID, err := c.redis.Incr(ctx, "character:"+characterID+":event-counter").Result()
    if err != nil {
        return err
    }

    // Save to repository
    err = c.repo.UpdateHP(ctx, characterID, newHP)
    if err != nil {
        return err
    }

    // Publish event (Phase 3)
    event := CharacterEvent{
        EventID: eventID,
        Type:    "hp_changed",
        NewHP:   newHP,
    }
    c.pubsub.Publish("character:"+characterID, event)

    // Also publish to combat log
    logEntry := formatHPChange(characterID, newHP)
    c.pubsub.Publish("combat-log:"+encounterID, logEntry)

    return nil
}

// Phase 3: Streaming
func (c *Component) Stream(ctx context.Context, characterID string, fromEventID int64) (<-chan Event, error) {
    // Implementation here
}
```

## Migration Strategy

### Phase 2 (Now - Single Player)
- ✅ Add event_id to all snapshot responses
- ✅ Increment counters on each change
- ✅ Client polls Get RPCs
- ❌ No streaming yet

### Phase 3 (Multiplayer)
- ✅ Implement Stream RPCs
- ✅ Enable Redis pub/sub
- ✅ Client migrates to streaming
- ✅ Keep Get RPCs for backwards compat

### Phase 4 (Polish)
- ✅ Buffering for reconnection
- ✅ Compression for large events
- ✅ Rate limiting
- ✅ Spectator mode

## Open Questions

1. **Event retention**: How long to keep event counters? Reset when encounter ends?
2. **Multiple characters**: If player controls 2 characters, subscribe to both?
3. **Spectator mode**: Can non-players subscribe to combat log only?
4. **Reconnection gap**: If client misses events 42-45, request snapshot or replay?
5. **Combat log formatting**: Server-side or client-side? (Probably server for consistency)

## Related Documents

- [Multiplayer Streaming Design](./multiplayer-encounter-streaming-design.md) - Original event sourcing discussion
- [Toolkit Integration Tests](~/personal/rpg-toolkit/rulebooks/dnd5e/combat/integration.md) - Combat log format examples

## Implementation Checklist

### Phase 2 Changes (Required Now)
- [ ] Add `event_id` to all proto snapshot messages
- [ ] Create component interfaces (Character, Room, CombatLog, Notification)
- [ ] Wrap repositories in components
- [ ] Increment event counters on all state changes
- [ ] Update orchestrators to use components instead of repos directly

### Phase 3 Changes (Later)
- [ ] Implement Stream RPCs in protos
- [ ] Add Redis pub/sub to components
- [ ] Implement streaming in handlers
- [ ] Client migration guide
- [ ] Testing strategy for multiplayer

---

**Status**: Design - Ready for Implementation
**Priority**: High - Blocks multiplayer UX
**Owner**: TBD
