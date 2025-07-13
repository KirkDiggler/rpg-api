# ADR-002: Service Boundaries and Client Strategy

## Status
Proposed

## Context

We need to decide how to organize our gRPC services. Each service requires a separate client implementation in consumers (like our Discord bot), so the number of services directly impacts client complexity.

We've identified these potential service boundaries:
- Character management (creation, drafts, updates)
- Session management (joining, leaving, state)
- Encounter management (combat, initiative, damage)
- Rule queries (races, classes, spells)
- Item management (inventory, equipment)
- Story/campaign progression

## Decision Drivers

1. **Client complexity** - Each service = another client to implement
2. **Authentication boundaries** - Different auth requirements per domain
3. **Deployment flexibility** - Ability to scale/deploy services independently
4. **Development velocity** - Simpler is faster
5. **Team boundaries** - Can different teams own different services?

## Considered Options

### Option 1: Fine-grained Services (6+ services)
```protobuf
service CharacterService { }
service SessionService { }
service EncounterService { }
service SpellService { }
service ItemService { }
service RuleService { }
```

**Pros:**
- Maximum flexibility
- Clear single responsibility
- Independent scaling

**Cons:**
- 6+ clients to implement
- Complex service discovery
- Lots of cross-service calls

### Option 2: Domain-based Services (3 services)
```protobuf
service PlayerService {
  // Player actions: characters, inventory, actions
  CreateCharacterDraft()
  UpdateCharacter()
  UseItem()
  RollDice()
}

service GameService {
  // Game management: sessions, encounters, story
  CreateSession()
  JoinSession()
  StartEncounter()
  AdvanceStory()
}

service RuleService {
  // Read-only rulebook queries
  GetRace()
  GetClass()
  GetSpell()
}
```

**Pros:**
- Natural auth boundaries (players vs game master vs public)
- Reasonable client count (3)
- Aligned with user personas
- Clear domain separation

**Cons:**
- Some services might grow large
- Less granular scaling

### Option 3: Monolithic Service (1 service)
```protobuf
service DnD5eService {
  // Everything D&D 5e
}
```

**Pros:**
- Single client
- Simplest to implement
- No cross-service calls

**Cons:**
- No auth flexibility
- Massive API surface
- Hard to maintain

## Decision

**We choose fine-grained services per domain within each ruleset, wrapped by language-specific SDKs**

Multiple focused services under each ruleset (e.g., `v1alpha1.dnd5e`):
- `CharacterService` - Character creation and management
- `ProgressionService` - Leveling and advancement
- `InventoryService` - Items and equipment
- `SessionService` - Game session management
- `EncounterService` - Combat and encounters
- `RuleService` - Rulebook queries

**Key innovation: Language-specific SDKs** that wrap these services:

```go
// Go SDK
client := dnd5e.NewClient(config)
draft, err := client.Character.CreateDraft(ctx, input)
```

```typescript
// TypeScript SDK
const client = new DnD5eClient(config);
const draft = await client.character.createDraft(input);
```

## Consequences

### Positive
- **Independent service evolution** - Update progression without touching character
- **Clear ownership** - Teams can own individual services
- **AWS-proven pattern** - Multiple services work at scale
- **SDK simplicity** - Users get one clean interface per language
- **Flexible deployment** - Services can scale independently
- **Natural versioning** - Each service can evolve at its own pace

### Negative
- **More services to maintain** - 6+ services vs 1-3
- **Initial complexity** - More protos, more clients
- **SDK maintenance** - Need to maintain multiple language SDKs

### Mitigation
- **SDK layer** - Hides complexity from consumers
- **Consistent patterns** - All services follow same conventions
- **Code generation** - Automate client and SDK generation
- **Shared core** - Common auth, errors, middleware

## Implementation Notes

1. Service organization:
   ```
   /api/proto/v1alpha1/dnd5e/
     - character.proto
     - progression.proto
     - inventory.proto
     - session.proto
     - encounter.proto
     - rules.proto
   ```

2. SDK structure:
   ```
   /sdk/
     /go/
       - client.go      # Main SDK entry
       - character/     # Per-service packages
       - progression/
     /typescript/
       - src/client.ts  # Main SDK entry
       - src/character/ # Per-service modules
   ```

3. SDK features:
   - Lazy client initialization
   - Shared configuration
   - Automatic retries
   - Error standardization
   - Observability hooks
   - **Type-safe service version management**

4. Service version configuration:
   ```go
   // Type-safe service versions
   client := dnd5e.NewClient(dnd5e.Config{
       Endpoint: "api.rpg.example.com",
       ServiceVersions: map[rpgapi.ServiceName]versions.Version{
           rpgapi.CharacterService:   versions.V1Alpha1,  // stable
           rpgapi.ProgressionService: versions.V1Alpha1,  // stable  
           rpgapi.EncounterService:   versions.V1Beta1,   // beta testing!
           rpgapi.InventoryService:   versions.V1Alpha1,  // stable
       },
   })
   
   // No magic strings - full type safety
   type ServiceName string
   const (
       CharacterService   ServiceName = "character"
       ProgressionService ServiceName = "progression"
       EncounterService   ServiceName = "encounter"
   )
   
   type Version string
   const (
       V1Alpha1 Version = "v1alpha1"
       V1Beta1  Version = "v1beta1"
       V1       Version = "v1"
   )
   ```

5. Release process:
   - Proto changes trigger SDK regeneration
   - SDKs published to package registries
   - Version compatibility matrix maintained

This decision follows the successful patterns of AWS, Stripe, and other large-scale API providers.
