# Multiplayer Encounter Streaming Design

## Context

Designing real-time multiplayer for turn-based co-op dungeon crawler.

## Use Cases

### Primary Scenarios
1. **4 players start dungeon together** - All connected, synced in real-time
2. **Player disconnects mid-combat** - Rejoins, needs to catch up
3. **New player joins ongoing encounter** - Loads current state, sees future updates
4. **Player refreshes browser** - Reconnects seamlessly

### State That Needs Syncing
- **Room state**: Entity positions, HP, conditions, equipped items
- **Combat state**: Initiative order, current turn, actions available
- **Events**: Movement, attacks, traps, items equipped, conditions applied/removed
- **Character changes**: HP, equipped weapons, conditions (bleeding, prone, etc.)
- **Monster changes**: HP, position, conditions, defeated

## Technical Constraints

- Using **Redis pub/sub** (ephemeral, no message history)
- gRPC streaming for client connections
- Server is source of truth
- Turn-based (not real-time action)

## Design Options

### Option A: Event Sourcing (Full History)
**Pattern**: Store all events, replay from any point

```
Client Flow:
1. Connect: StreamEncounter(from_event_id=0)
2. Server replays events 1..N, then streams live
3. Client applies events to build state

Reconnect:
1. Client remembers: "Last saw event #42"
2. Connect: StreamEncounter(from_event_id=43)
3. Server replays 43..current, then streams live
```

**Pros**:
- Perfect sync - replay gets you to exact state
- Time travel debugging
- Combat log is free (event history)

**Cons**:
- Need event storage (Redis Stream or DB)
- More complex server logic
- Have to replay potentially large history

### Option B: Snapshot + Live Updates (Pub/Sub)
**Pattern**: Get current state, then watch for changes

```
Client Flow:
1. GetEncounterSnapshot(encounter_id)
   → Returns: {room, combat_state, event_id: 42}
2. StreamEncounter(encounter_id, after_event_id=42)
   → Server subscribes to Redis pub/sub, streams new events

Reconnect:
1. GetEncounterSnapshot(encounter_id) 
   → Returns latest state at event_id: 57
2. StreamEncounter(encounter_id, after_event_id=57)
   → Catches up from new baseline
```

**Pros**:
- Simpler infrastructure (Redis pub/sub is lightweight)
- Fast sync (snapshot vs replaying 1000s of events)
- Scales better (no replay burden)

**Cons**:
- Snapshot might be slightly stale between GetSnapshot and StreamEncounter
- Need to version snapshots (event_id watermark)
- Combat log requires separate storage if we want history

### Option C: Hybrid (Snapshot + Buffered Events)
**Pattern**: Snapshot + server buffers recent events

```
Client Flow:
1. GetEncounterSnapshot(encounter_id)
   → Returns: {room, combat_state, event_id: 42}
2. StreamEncounter(encounter_id, after_event_id=42)
   → Server sends buffered events 43-45 (from last 30 seconds)
   → Then switches to live Redis pub/sub

Reconnect:
1. Same as Option B, but gets recent buffer to avoid gaps
```

**Pros**:
- Fills gap between snapshot and stream connect
- No event sourcing infrastructure
- Recent history for combat log

**Cons**:
- Buffer size decisions (how much to keep?)
- Slightly more complex than pure pub/sub

## Event Schema

### Event Envelope
```protobuf
message EncounterEvent {
  int64 event_id = 1;        // Monotonic counter
  string timestamp = 2;
  string encounter_id = 3;
  
  oneof event_type {
    EntityMoved entity_moved = 10;
    AttackExecuted attack_executed = 11;
    TrapTriggered trap_triggered = 12;
    TurnEnded turn_ended = 13;
    ConditionApplied condition_applied = 14;
    ConditionRemoved condition_removed = 15;
    EntityDefeated entity_defeated = 16;
    ItemEquipped item_equipped = 17;
    ItemUnequipped item_unequipped = 18;
    HPChanged hp_changed = 19;
    // ... more
  }
}
```

### Event Details
```protobuf
message EntityMoved {
  string entity_id = 1;
  repeated Position path = 2;
  Position final_position = 3;
  string stop_reason = 4;
}

message AttackExecuted {
  string attacker_id = 1;
  string target_id = 2;
  int32 attack_roll = 3;
  int32 damage = 4;
  bool critical = 5;
  bool hit = 6;
}

message ConditionApplied {
  string entity_id = 1;
  string condition = 2;  // "bleeding", "prone", "raging"
  int32 duration = 3;
  string source = 4;     // "trap", "attack", "spell"
}

message ItemEquipped {
  string entity_id = 1;
  string item_id = 2;
  string slot = 3;  // "main_hand", "armor", etc.
}

message HPChanged {
  string entity_id = 1;
  int32 previous_hp = 2;
  int32 new_hp = 3;
  string reason = 4;  // "trap damage", "healing potion"
}
```

### Snapshot
```protobuf
message EncounterSnapshot {
  int64 as_of_event_id = 1;  // Snapshot is valid at this event
  string encounter_id = 2;
  
  Room room = 3;             // All entities with current state
  CombatState combat_state = 4;  // Turn order, current turn
  
  // Entity details not in Room
  repeated EntityDetail entities = 5;
}

message EntityDetail {
  string entity_id = 1;
  int32 current_hp = 2;
  int32 max_hp = 3;
  repeated string conditions = 4;  // ["bleeding", "prone"]
  repeated EquippedItem equipped = 5;
  Position position = 6;
}
```

## Implementation Phases

### Phase 2 (Current - Single Player)
- Unary RPCs only
- Room in every response
- No streaming

### Phase 3 (Multiplayer Foundation)
- Add GetEncounterSnapshot RPC
- Add StreamEncounter RPC
- Implement Option B or C (decide based on testing)
- Migrate client to use snapshot + stream
- Keep unary RPCs for backwards compat

### Phase 4 (Polish)
- Combat log storage/retrieval
- Reconnection UX improvements
- Spectator mode (read-only stream)

## Open Questions

1. **Which option?** B (Snapshot + Pub/Sub) or C (+ Buffer)?
2. **Event ID generation?** Redis INCR? Timestamp? UUID?
3. **How long to buffer events?** 30 seconds? 100 events?
4. **Combat log storage?** Separate concern or use events?
5. **Optimistic updates?** Client predicts, server confirms?
6. **Conflict resolution?** Two players act simultaneously?

## Related

- Redis Pub/Sub: https://redis.io/docs/interact/pubsub/
- gRPC Streaming: https://grpc.io/docs/languages/go/basics/#server-side-streaming-rpc
- Event Sourcing: https://martinfowler.com/eaaDev/EventSourcing.html

