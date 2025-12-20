# Encounter Aggregate Design

**Date:** 2025-12-19
**Status:** Proposed
**Target Milestone:** Next (post-Project 10)

## Problem Statement

Currently, event publishing is scattered throughout the orchestrator (~20 manual `publishEvent` calls). This is fragile:
- Easy to forget when adding new functionality
- No compile-time guarantee that state changes emit events
- Event publishing is mixed with business logic
- Cannot guarantee complete replay fidelity

We want full event sourcing where events ARE the source of truth, enabling complete dungeon replay.

## Design Goals

1. **Structural event guarantees** - Impossible to change state without emitting events
2. **Replay fidelity** - Events contain all data needed to reconstruct exact state
3. **Event consistency** - Events are atomic with state changes
4. **Clean separation** - Aggregate owns state logic, orchestrator coordinates I/O

## Architecture

### Aggregate Pattern

The `Encounter` aggregate encapsulates all state and only modifies it through applied events:

```
┌─────────────────────────────────────────────────────────────┐
│                      Orchestrator                            │
│  - Loads aggregate from event store                         │
│  - Calls aggregate commands                                  │
│  - Persists uncommitted events                              │
│  - Publishes to real-time subscribers                       │
└─────────────────────┬───────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────────┐
│                   Encounter Aggregate                        │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  Private State (only changes via Apply)              │   │
│  │  - Rooms, CurrentRoom                                │   │
│  │  - Players, Characters                               │   │
│  │  - Monsters                                          │   │
│  │  - Combat (turn order, action economy)              │   │
│  │  - Phase, Outcome                                    │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                              │
│  Commands:                    Events:                        │
│  - Attack(cmd) → result       - AttackResolvedEvent         │
│  - Move(cmd) → result         - MovementCompletedEvent      │
│  - EndTurn(cmd) → result      - TurnEndedEvent              │
│  - ...                        - ...                          │
│                                                              │
│  Apply(event) ─────────────────► State mutation             │
└─────────────────────────────────────────────────────────────┘
```

### Aggregate Boundary

A single `Encounter` aggregate encompasses an entire dungeon run:
- All rooms in the dungeon
- All players and their characters
- All monsters (can move between rooms)
- Combat state (initiative, turns, action economy)
- Dungeon progress (rooms explored, victory/failure)

This matches our current model where encounter:dungeon is 1:1. Using "Encounter" as the name keeps us flexible for non-dungeon encounters (arena fights, random wilderness encounters, etc.).

## Core Components

### Encounter Aggregate

```go
// internal/aggregates/encounter/encounter.go

package encounter

type Encounter struct {
    id          string
    state       *State           // private, only changes via Apply
    uncommitted []Event          // events from current operation
}

type State struct {
    Rooms       map[string]*Room
    CurrentRoom string
    Players     map[string]*Player
    Characters  map[string]*CharacterState
    Monsters    map[string]*MonsterState
    Combat      *CombatState
    Phase       EncounterPhase
    Outcome     *Outcome
}

func NewEncounter(id string) *Encounter
func NewEncounterFromEvents(id string, events []Event) *Encounter

func (e *Encounter) Apply(event Event)
func (e *Encounter) UncommittedEvents() []Event
func (e *Encounter) ClearUncommitted()
```

### Apply Method

The single place where state changes happen:

```go
func (e *Encounter) Apply(event Event) {
    switch ev := event.(type) {
    case *PlayerJoinedEvent:
        e.state.Players[ev.PlayerID] = &Player{...}
        e.state.Characters[ev.CharacterID] = &CharacterState{...}

    case *AttackResolvedEvent:
        target := e.state.findEntity(ev.TargetID)
        target.SetHP(ev.TargetHP)
        if ev.TargetDead {
            target.SetDead(true)
        }

    case *TurnEndedEvent:
        e.state.Combat.Round = ev.Round
        e.state.Combat.ActiveIndex = ev.ActiveIndex

    // ... other event types
    }
}
```

### Command Methods

Commands validate, compute, apply events internally, and return results:

```go
func (e *Encounter) Attack(cmd AttackCommand) (*AttackResult, error) {
    // 1. Validate against current state
    if e.state.Phase != PhaseCombat {
        return nil, ErrNotInCombat
    }
    if !e.state.Combat.IsActiveEntity(cmd.AttackerID) {
        return nil, ErrNotYourTurn
    }

    // 2. Compute using rpg-toolkit
    attackResult := combat.ResolveAttack(attacker.Data, target.Data, cmd.WeaponID)
    newHP := target.HP - attackResult.DamageDealt

    // 3. Create and apply event
    event := &AttackResolvedEvent{
        AttackerID: cmd.AttackerID,
        TargetID:   cmd.TargetID,
        Damage:     attackResult.DamageDealt,
        TargetHP:   newHP,
        TargetDead: newHP <= 0,
    }
    e.Apply(event)
    e.uncommitted = append(e.uncommitted, event)

    // 4. Return result
    return &AttackResult{
        Hit:      attackResult.Hit,
        Damage:   attackResult.DamageDealt,
        TargetHP: newHP,
    }, nil
}
```

### Orchestrator Coordination

The orchestrator becomes a thin coordination layer:

```go
func (o *Orchestrator) Attack(ctx context.Context, input *AttackInput) (*AttackOutput, error) {
    // 1. Load aggregate from event store
    enc, err := o.loadEncounter(ctx, input.EncounterID)
    if err != nil {
        return nil, err
    }

    // 2. Execute command
    result, err := enc.Attack(aggregate.AttackCommand{
        AttackerID: input.CharacterID,
        TargetID:   input.TargetID,
    })
    if err != nil {
        return nil, err
    }

    // 3. Persist and publish
    if err := o.persistAndPublish(ctx, enc); err != nil {
        return nil, err
    }

    return &AttackOutput{...}, nil
}

func (o *Orchestrator) persistAndPublish(ctx context.Context, enc *aggregate.Encounter) error {
    events := enc.UncommittedEvents()
    if len(events) == 0 {
        return nil
    }

    if err := o.eventStore.AppendEvents(ctx, enc.ID(), events); err != nil {
        return err
    }

    enc.ClearUncommitted()
    o.publishToSubscribers(ctx, enc.ID(), events)
    return nil
}

func (o *Orchestrator) loadEncounter(ctx context.Context, id string) (*aggregate.Encounter, error) {
    events, err := o.eventStore.GetEvents(ctx, id)
    if err != nil {
        return nil, err
    }
    return aggregate.NewEncounterFromEvents(id, events), nil
}
```

### Event Types

```go
// internal/aggregates/encounter/events.go

type Event interface {
    EventType() string
    EncounterID() string
    Timestamp() time.Time
}

type BaseEvent struct {
    ID         string    `json:"id"`
    Type       string    `json:"type"`
    EncID      string    `json:"encounter_id"`
    OccurredAt time.Time `json:"occurred_at"`
}

// Concrete events
type PlayerJoinedEvent struct {
    BaseEvent
    PlayerID      string          `json:"player_id"`
    CharacterID   string          `json:"character_id"`
    CharacterData *character.Data `json:"character_data"`
}

type AttackResolvedEvent struct {
    BaseEvent
    AttackerID string `json:"attacker_id"`
    TargetID   string `json:"target_id"`
    Hit        bool   `json:"hit"`
    Damage     int    `json:"damage"`
    TargetHP   int    `json:"target_hp"`
    TargetDead bool   `json:"target_dead"`
}

// ... 16 event types total (see entities/encounter_events.go for current list)
```

## Error Handling

### Persistence Failures

If event persistence fails, the aggregate's in-memory state is ahead of what's stored. Strategy:

- Reload from event store on each request (simple, correct)
- Failed persist = command didn't happen
- Client retries, gets fresh state from store

Caching optimization can come later if performance requires it.

## Testing

Aggregates are pure and trivially testable:

```go
func TestAttack_Success(t *testing.T) {
    // Given: Set up state via events
    enc := NewEncounter("enc-123")
    enc.Apply(&PlayerJoinedEvent{...})
    enc.Apply(&CombatStartedEvent{...})

    // When: Execute command
    result, err := enc.Attack(AttackCommand{...})

    // Then: Assert on events and state
    require.NoError(t, err)
    require.Len(t, enc.UncommittedEvents(), 1)

    event := enc.UncommittedEvents()[0].(*AttackResolvedEvent)
    assert.Equal(t, expectedDamage, event.Damage)
}
```

No mocks needed - apply events to set up state, call command, assert on results.

## Migration Plan

Single PR, clean break (codebase is early enough):

### Package Structure

```
internal/
├── aggregates/
│   └── encounter/
│       ├── encounter.go       # Core aggregate
│       ├── state.go           # State struct and helpers
│       ├── events.go          # Event types
│       ├── commands.go        # Command structs
│       ├── apply.go           # Apply method
│       ├── errors.go          # Domain errors
│       └── encounter_test.go  # Tests
├── orchestrators/
│   └── encounter/
│       └── orchestrator.go    # Thin coordinator
```

### Commands to Migrate

Order by complexity:

1. **Simple state toggles**
   - SetPlayerReady

2. **Player lifecycle**
   - JoinEncounter
   - LeaveEncounter
   - PlayerDisconnected
   - PlayerReconnected

3. **Combat lifecycle**
   - StartCombat
   - EndTurn

4. **Combat actions**
   - Move
   - Attack
   - ActivateFeature

5. **Dungeon events**
   - CheckVictoryCondition
   - CheckDefeatCondition

### Verification

- Existing handler tests should pass unchanged
- Add aggregate unit tests for each command
- Event publishing tests verify correct events emitted

## Future Considerations

### Snapshotting

For long encounters (hundreds of events), loading could be slow. Future optimization:
- Periodically snapshot aggregate state
- Load from snapshot + replay events since snapshot
- Not needed now, easy to add later

### Aggregate Caching

Currently reload from store each request. Future optimization:
- Cache aggregates in memory
- Invalidate on persistence failure
- Not needed now, easy to add later

### Event Versioning

When event schemas change:
- Use event upcasters to transform old events to new format
- Or version events explicitly (v1, v2)
- Consider when we approach v1beta1 protos

## Issue Breakdown

Suggested issues for implementation:

1. **Create aggregate package structure** - Set up `internal/aggregates/encounter/` with core types
2. **Implement Apply method** - Event type switch with all 16 event types
3. **Migrate player lifecycle commands** - Join, Leave, Ready, Disconnect, Reconnect
4. **Migrate combat lifecycle commands** - StartCombat, EndTurn
5. **Migrate combat action commands** - Move, Attack, ActivateFeature
6. **Update orchestrator to use aggregate** - Thin coordination layer
7. **Add aggregate tests** - Full coverage of commands and edge cases
8. **Clean up old code** - Remove duplicated logic from orchestrator
