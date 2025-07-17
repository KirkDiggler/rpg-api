# API v1alpha1 Handlers

This package implements gRPC handlers for the generic API service interface, providing dice rolling functionality for RPG applications.

## DiceHandler

The `DiceHandler` implements the `DiceServiceServer` interface from the rpg-api-protos, providing three main operations:

### RPCs Implemented

#### RollDice
```protobuf
rpc RollDice(RollDiceRequest) returns (RollDiceResponse)
```
- **Purpose**: Roll dice using standard notation and store results in a session
- **Input**: Entity ID, context, dice notation (e.g., "2d6", "4d6"), optional description
- **Output**: All rolls in the session with expiration timestamp
- **Validation**: Requires entity_id, context, and notation

#### GetRollSession  
```protobuf
rpc GetRollSession(GetRollSessionRequest) returns (GetRollSessionResponse)
```
- **Purpose**: Retrieve existing dice roll session for an entity
- **Input**: Entity ID and context
- **Output**: All rolls in session with creation and expiration timestamps
- **Use Case**: Continue interrupted dice rolling workflows

#### ClearRollSession
```protobuf
rpc ClearRollSession(ClearRollSessionRequest) returns (ClearRollSessionResponse)
```
- **Purpose**: Remove a dice roll session and all its rolls
- **Input**: Entity ID and context  
- **Output**: Confirmation message and count of cleared rolls
- **Use Case**: Clean up after character creation or combat

### Key Features

- **Session-based rolling**: All rolls are grouped by entity and context
- **TTL support**: Sessions automatically expire (default 15 minutes)
- **Individual roll tracking**: Each roll gets a unique ID for flexible assignment
- **Comprehensive validation**: Input validation with descriptive error messages
- **Error handling**: Converts internal errors to appropriate gRPC status codes

### Usage Example

```go
// Create handler
handler := NewDiceHandler(&DiceHandlerConfig{
    DiceService: diceOrchestrator,
})

// Roll ability scores for character creation
response, err := handler.RollDice(ctx, &RollDiceRequest{
    EntityId: "char_draft_123",
    Context:  "ability_scores", 
    Notation: "4d6",
    ModifierDescription: "Ability Score Generation",
})
```

### Session Management

Sessions are identified by `(entity_id, context)` tuples:
- **entity_id**: The entity performing rolls (e.g., "char_draft_123", "player_456")
- **context**: The purpose of the rolls (e.g., "ability_scores", "combat_round_1")

This allows multiple concurrent rolling sessions per entity for different purposes.

### Dependencies

- **DiceService**: Business logic orchestrator (see `/internal/orchestrators/dice/`)
- **rpg-api-protos**: Protocol buffer definitions and generated code
- **Internal errors**: Error handling and gRPC status code conversion

### Implementation Notes

- Follows the outside-in development pattern with clear separation of concerns
- Handlers focus on request/response translation and validation
- All business logic is delegated to the dice service orchestrator
- Uses Input/Output types consistently with other handlers
