# ADR-001: Foundation and Standards

## Status
Accepted

## Context
We are building `rpg-api`, an API gateway and game orchestration service that enables tabletop RPG experiences across multiple interfaces (Discord, web, mobile). This service acts as the bridge between game engines (starting with rpg-toolkit) and user interfaces, providing real-time multiplayer gameplay.

After experiencing architectural debt in dnd-bot-discord (issue #316), we recognize the critical importance of establishing clear boundaries, patterns, and standards from the beginning. This ADR establishes the foundational decisions that will guide all future development.

## Decision

### 1. Mission Statement
> **rpg-api provides a real-time API gateway for tabletop RPG sessions, enabling consistent game experiences across any interface while leveraging rpg-toolkit as the core game engine.**

Key principles:
- **Interface Agnostic**: Discord, web, mobile, CLI - all equal citizens
- **Real-time First**: Built for live, multiplayer gameplay
- **Engine Powered**: rpg-toolkit handles game mechanics; we handle orchestration
- **Ruleset Flexible**: D&D 5e today, other systems tomorrow
- **Data-Powered API**: We are the API version of rpg-toolkit

### 2. Architectural Standards

#### Separation of Concerns
```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   UI Clients    │────▶│    rpg-api      │────▶│   rpg-toolkit   │
│ (Discord, Web)  │ API │  (Orchestrator) │uses │  (Game Engine)  │
└─────────────────┘     └─────────────────┘     └─────────────────┘
```

- **rpg-api**: Session management, real-time streaming, persistence, API gateway
- **rpg-toolkit**: Game mechanics, rules engine, dice, conditions, effects, calculations
- **UI Clients**: Presentation and user interaction only

#### Critical Insight: Data Models vs Rulebook Logic
**rpg-api keeps simple data models. rpg-toolkit owns ALL rulebook logic:**

```go
// rpg-api: Simple data model
type Character struct {
    ID         string
    Name       string
    Level      int
    RaceID     string
    ClassID    string
    BaseStats  Stats  // Just the raw numbers
}

// rpg-toolkit: Rulebook calculations
profBonus := toolkit.CalculateProficiencyBonus(character.Level)
modifier := toolkit.CalculateAbilityModifier(character.BaseStats.Strength)
attackRoll := toolkit.RollAttack(character, weapon, target)
```

This separation ensures:
- rpg-api remains a pure data and orchestration layer
- All game rules live in one place (rpg-toolkit)
- Rule changes don't affect data storage
- Different rulesets can use the same data

#### API-First Design
- All functionality exposed through gRPC APIs
- Protocol buffers define the contract
- gRPC streaming for real-time updates
- No client has special access - Discord uses same APIs as web

#### Layered Architecture
We are a **data-powered API**, the API version of rpg-toolkit. Our architecture enforces strict layering:

```
┌─────────────────────────────────────────────────────┐
│                   gRPC Layer                        │
│  - Proto definitions (external contract)            │
│  - Proto ↔ Domain conversion                        │
│  - gRPC error mapping                               │
├─────────────────────────────────────────────────────┤
│                Orchestrator Layer                   │
│  - Business flow orchestration                      │
│  - Coordinate repositories and rpg-toolkit          │
│  - Transaction boundaries                           │
├─────────────────────────────────────────────────────┤
│                Repository Layer                     │
│  - Storage interfaces                               │
│  - Simple data persistence                          │
│  - Query logic                                      │
├─────────────────────────────────────────────────────┤
│              Storage Adapters                       │
│  - Redis, DynamoDB, PostgreSQL, etc.               │
│  - User's choice of implementation                  │
└─────────────────────────────────────────────────────┘
```

**Layer Principles:**
1. **Protos are External**: Proto definitions can evolve independently of internal models
2. **Entities are Internal**: Simple domain models for internal use, not protos
3. **No Context Mixing**: Each layer only knows about the layer directly below it
4. **Interface Boundaries**: Every layer interaction happens through interfaces
5. **Mockable by Design**: Each layer generates its own mocks for testing

**Core Principle: Be Explicit with Inputs/Outputs**
Every layer and function should use explicit Input/Output types:

```go
// ❌ BAD: Multiple parameters, unclear return
func CreateSession(name string, dmID string, maxPlayers int) (*Session, string, error)

// ✅ GOOD: Explicit Input/Output types
func CreateSession(ctx context.Context, input *CreateSessionInput) (*CreateSessionOutput, error)
```

This applies to ALL layers:
- **Handlers**: Request/Response types
- **Orchestrators**: Input/Output types
- **Repositories**: Input/Output types
- **Helper functions**: Even private functions benefit from explicit types

### 3. Development Standards

#### Documentation Requirements
- **ADRs**: Record architectural decisions with status, context, decision, consequences
- **Journey Docs**: Capture exploration, questions, "dragons" encountered
- **Package READMEs**: Purpose, usage, examples, boundaries
- **API Documentation**: Generated from protobuf definitions

#### Code Standards
- **100% test coverage** for core packages (orchestrators, engine integration)
- **80% test coverage** for API handlers
- **Pre-commit hooks**: fmt, mod tidy, lint, test
- **No magic strings**: All constants defined and typed
- **Error handling**: Wrapped errors with context
- **Logging**: Structured logging with correlation IDs
- **Always work in branches**: feat/, fix/, docs/ prefixes

#### Repository Pattern
Always use Input/Output structs for repository methods to ensure interface stability:

```go
// repository.go
type Repository interface {
    Get(ctx context.Context, id string) (*entities.Session, error)
    Save(ctx context.Context, session *entities.Session) error
    List(ctx context.Context, input *ListInput) (*ListOutput, error)
    Delete(ctx context.Context, id string) error
}

type ListInput struct {
    Limit     int
    Offset    int
    Filter    *FilterOptions
}

type ListOutput struct {
    Sessions  []*entities.Session
    NextToken string  // for pagination
    Total     int     // total count if needed
}
```

**Benefits:**
- No interface changes when adding fields
- No mock regeneration needed
- Supports nil for defaults
- Future-proof for pagination, filtering, metadata

#### Testing Standards
- **Use Uber's gomock** for mock generation (not mockery)
- **Always use test suites** for DRY test setup
- **Real Redis when possible** using miniredis or testcontainers
- **Structured test data** setup in suite initialization

```go
type SessionOrchestratorTestSuite struct {
    suite.Suite
    mockRepo     *mocks.MockRepository
    orchestrator *SessionOrchestrator
    testSession  *entities.Session
}

func (s *SessionOrchestratorTestSuite) SetupTest() {
    ctrl := gomock.NewController(s.T())
    s.mockRepo = mocks.NewMockRepository(ctrl)
    s.orchestrator = &SessionOrchestrator{
        repo: s.mockRepo,
    }
    s.testSession = &entities.Session{
        ID:   "test-123",
        Name: "Test Session",
    }
}
```

### 4. Technology Choices

#### Core Stack
- **Language**: Go (consistency with rpg-toolkit)
- **API**: gRPC with protobuf
- **Real-time**: gRPC streaming + Redis pub/sub
- **Storage**: Repository pattern with adapters (Redis for initial implementation)
- **Web Framework**: None - gRPC-gateway handles HTTP

#### Dependencies
- **rpg-toolkit**: Core game engine (our own)
- **grpc-go**: Google's gRPC implementation
- **testify**: Test framework and suites
- **gomock**: Uber's mock generation framework
- **cobra**: Command line interface
- **Storage adapters**: Pluggable based on deployment needs

### 5. Project Organization

```
rpg-api/
├── api/proto/v1alpha1/ # API contracts (external, versioned)
│   ├── session.proto   # Session management
│   ├── dice.proto      # Dice rolling service
│   └── common.proto    # Shared types
├── cmd/
│   └── server/         # Cobra commands to start the service
├── internal/           # Private application code
│   ├── entities/       # Simple domain models (just data)
│   ├── handlers/       # gRPC handlers by proto version
│   │   ├── sessionv1alpha1/
│   │   ├── dicev1alpha1/
│   │   └── commonv1alpha1/
│   ├── orchestrators/  # Business logic by domain flow
│   │   ├── character_creation/
│   │   ├── session_management/
│   │   └── dice_rolling/
│   ├── repositories/   # Storage layer (plural)
│   │   ├── sessions/
│   │   │   ├── repository.go  # Interface + types
│   │   │   └── redis.go       # Implementation
│   │   └── characters/
│   └── engine/         # rpg-toolkit integration
├── pkg/                # Public packages (if any)
├── web/                # React companion app
└── docs/               # ADRs, journey docs
```

**Package Responsibilities:**
- `api/proto`: External API contract, can evolve independently (v1alpha1 → v1beta1 → v1)
- `internal/entities`: Simple domain models - just data, no business logic
- `internal/handlers`: gRPC handlers organized by proto version
- `internal/orchestrators`: Business logic organized by domain flow
- `internal/repositories`: Storage layer with interface + implementations
- `internal/engine`: Integrates rpg-toolkit for ALL game mechanics

### 6. Operational Standards

#### Observability
- Structured logging with correlation IDs
- Metrics for all gRPC endpoints
- Distributed tracing for request flow
- Health checks for all dependencies

#### Deployment
- Single binary deployment
- Configuration through environment variables
- Graceful shutdown handling
- Rolling updates without dropping connections

## Consequences

### Positive
- Clear boundaries prevent scope creep and architectural drift
- API-first enables multiple UIs without duplication
- rpg-toolkit integration provides proven game mechanics
- Documentation standards ensure knowledge preservation
- Test standards ensure reliability and refactoring confidence
- Layered architecture enables independent evolution of API, business logic, and storage
- Repository pattern allows users to choose their preferred storage backend
- Interface-driven design makes the system highly testable and modular
- Proto/domain separation protects internal models from external API changes
- Data/rulebook separation keeps game logic in one place

### Negative
- More upfront design work than a monolithic approach
- API versioning complexity as system evolves
- Integration testing requires more setup
- Documentation requirements add development overhead
- Conversion code between layers adds boilerplate
- Multiple models (proto, domain, storage) for same concept
- Learning curve for developers new to layered architecture

### Risks and Mitigations
- **Risk**: Over-engineering for current needs
  - **Mitigation**: Start with minimal viable API, expand based on real usage
- **Risk**: rpg-toolkit changes breaking our integration
  - **Mitigation**: Pin versions, comprehensive integration tests
- **Risk**: Real-time complexity for web clients
  - **Mitigation**: Provide both streaming and polling options

## References
- rpg-toolkit architecture: Event-driven game engine design
- dnd-bot-discord issue #316: Lessons learned from architectural debt
- gRPC best practices: https://grpc.io/docs/guides/
- Go project layout: https://github.com/golang-standards/project-layout