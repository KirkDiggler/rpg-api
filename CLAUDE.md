# Claude AI Development Guidelines

## Core Philosophy

**rpg-api stores data. rpg-toolkit handles rules.**

This separation is fundamental. When in doubt:
- If it's a game mechanic or calculation → rpg-toolkit
- If it's data storage or API orchestration → rpg-api

## Project Structure

Our battle-tested structure from production gRPC services:

```
/cmd/server/              # Cobra commands
/internal/
  ├── entities/           # Simple data models (just structs)
  ├── handlers/
  │   ├── sessionv1alpha1/  # Proto version naming
  │   └── dicev1alpha1/
  ├── orchestrators/      # Business logic by flow
  │   ├── character_creation/
  │   └── session_management/
  ├── repositories/       # Storage (plural naming)
  │   ├── sessions/
  │   │   ├── repository.go  # Interface + types
  │   │   └── redis.go       # Implementation
  │   └── characters/
  └── engine/             # rpg-toolkit integration
```

## Code Patterns

### Always Use Input/Output Types

**This is our #1 principle.** Every function at every layer:

```go
// ❌ BAD
func CreateSession(name string, dmID string, maxPlayers int) (*Session, error)

// ✅ GOOD  
func CreateSession(ctx context.Context, input *CreateSessionInput) (*CreateSessionOutput, error)
```

This applies everywhere:
- Handlers: Request/Response
- Orchestrators: Input/Output
- Repositories: Input/Output
- Even helpers: Input/Output

### Repository Pattern

```go
type Repository interface {
    Get(ctx context.Context, id string) (*entities.Session, error)
    Save(ctx context.Context, session *entities.Session) error
    List(ctx context.Context, input *ListInput) (*ListOutput, error)
}

type ListInput struct {
    Limit  int
    Offset int
    Filter *FilterOptions
}

type ListOutput struct {
    Sessions  []*entities.Session
    NextToken string
    Total     int
}
```

Benefits:
- No interface changes when adding fields
- No mock regeneration
- Future-proof for pagination

### Entity Design

Keep entities simple - they're just data:

```go
// entities/character.go
type Character struct {
    ID         string
    Name       string
    Level      int
    RaceID     string
    ClassID    string
    BaseStats  Stats  // Just the numbers
}

// NO business logic on entities
// This goes in rpg-toolkit:
// - CalculateProficiencyBonus(level)
// - CalculateAbilityModifier(score)
```

### Testing Approach

- **Uber's gomock** (not mockery)
- **Always use test suites**
- **Real Redis when safe** (miniredis)

```go
type OrchestratorTestSuite struct {
    suite.Suite
    mockRepo     *mocks.MockRepository
    mockEngine   *mocks.MockEngine
    orchestrator *Orchestrator
}

func (s *OrchestratorTestSuite) SetupTest() {
    ctrl := gomock.NewController(s.T())
    s.mockRepo = mocks.NewMockRepository(ctrl)
    s.mockEngine = mocks.NewMockEngine(ctrl)
    s.orchestrator = NewOrchestrator(s.mockRepo, s.mockEngine)
}
```

### Development Workflow

**Always work in branches:**
```bash
git checkout -b feat/character-creation
git checkout -b fix/session-timeout
git checkout -b docs/api-examples
```

**Always run pre-commit:**
```bash
make pre-commit
```

### Error Handling

```go
var (
    ErrSessionNotFound = errors.New("session not found")
    ErrPlayerNotInSession = errors.New("player not in session")
)

// Wrap with context
return fmt.Errorf("failed to get session %s: %w", id, ErrSessionNotFound)
```

### API Versioning

External versioning through handlers:
- `/handlers/sessionv1alpha1/`
- `/handlers/sessionv1beta1/`
- `/handlers/sessionv1/`

Internal stays stable while external evolves.

## Storage Philosophy

- **No database preferences** - users choose
- **Repository pattern** enables flexibility
- **Start with Redis** - simple, fast
- **Add adapters as needed**

## Remember

- Explicit > Implicit (always use Input/Output types)
- Simple > Complex (entities are just data)
- rpg-api orchestrates, rpg-toolkit calculates
- Test with real dependencies when safe
- Document the journey, not just destination
