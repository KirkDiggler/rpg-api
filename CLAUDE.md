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
  ├── handlers/           # gRPC handlers (API layer)
  │   └── dnd5e/
  │       └── v1alpha1/   # Proto version naming
  ├── services/           # Service interfaces (business logic contracts)
  │   └── character/
  │       ├── service.go  # Interface with Input/Output types
  │       └── mock/       # Generated mocks for testing
  ├── orchestrators/      # Service implementations (business logic)
  │   ├── character_creation/
  │   └── session_management/
  ├── repositories/       # Storage interfaces and implementations
  │   ├── sessions/
  │   │   ├── repository.go  # Interface + types
  │   │   └── redis.go       # Implementation
  │   └── characters/
  └── engine/             # rpg-toolkit integration
```

## Development Approach: Outside-In

**Always work from the API inward:**

1. **Start with gRPC handlers** - Just return `codes.Unimplemented`
   - Validates proto definitions work
   - Ensures server can start and register services
   - No business logic or dependencies yet

2. **Define service interfaces** - With Input/Output types
   - Create the contract for business logic
   - Generate mocks for testing handlers
   - Still no implementation

3. **Write handler tests** - Using mocked services
   - Test request validation
   - Test response mapping
   - Test error handling

4. **Implement orchestrators** - The actual business logic
   - Wire up repositories, engine, external services
   - Test with mocked dependencies

5. **Implement repositories** - Last, when you know what you need

This approach ensures:
- API is usable before implementation starts
- Interfaces are driven by actual needs
- Easy to refactor without breaking contracts
- Clear separation of concerns

## Code Patterns

### Avoid Magic Strings

**Extract all string literals to constants.** This prevents typos and makes refactoring easier:

```go
// ❌ BAD: Magic strings scattered throughout code
if source == "class" {
    // ...
}

// ✅ GOOD: Named constants
const (
    skillSourceClass      = "class"
    skillSourceBackground = "background"
)

if source == skillSourceClass {
    // ...
}
```

This applies to:
- Entity types and sources
- Error codes and messages
- Configuration keys
- Status values
- Any repeated string literal

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

### Mock Organization

Following rpg-toolkit's pattern for consistency:

- **Location**: Mocks go in a `mock/` subdirectory next to the interface
- **Package naming**: Use `<parent>mock` (e.g., `charactermock`, `sessionmock`)
- **File naming**: Use `mock_<interface>.go` (e.g., `mock_service.go`)
- **Generation**: Place `//go:generate` above the interface definition

```go
// In service.go:
//go:generate mockgen -destination=mock/mock_service.go -package=charactermock github.com/KirkDiggler/rpg-api/internal/services/character Service

type Service interface {
    // ...
}

// Usage in tests:
mockService := charactermock.NewMockService(ctrl)
```

Benefits:
- Mocks are close to their interfaces
- Clear package names (`charactermock.NewMock...`)
- Easy to find and maintain
- Consistent with rpg-toolkit patterns

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

**NEVER return (nil, nil) - Always return a valid object or an error**

```go
// ❌ BAD - Never do this
if input == nil {
    return nil, nil
}

// ✅ GOOD - Return error for invalid input
if input == nil {
    return nil, errors.New("input is required")
}

// ✅ GOOD - Return empty/default object if that's the valid behavior
if items == nil {
    return &ListOutput{Items: []*Item{}, Total: 0}, nil
}
```

Define errors at package level:
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

## Documentation Philosophy

### Three Types of Documentation

1. **Journey Docs** (`docs/journey/`): Tell the story
   - Capture exploration, decisions, and trade-offs
   - Include the "why" - what problems we faced, what we tried
   - Show the thinking process, not just the outcome
   - Example: "Why we chose Go 1.24" with performance considerations

2. **ADRs** (`docs/adr/`): Record architectural decisions
   - Formal decision records with context and consequences
   - What we chose and why we chose it
   - Alternatives considered and trade-offs made
   - Example: "ADR-001: Repository pattern with Input/Output types"

3. **READMEs**: Summarize what's implemented
   - Concise overview of what exists and how to use it
   - Avoid lengthy explanations (link to journey docs instead)
   - Focus on practical usage and current state
   - Example: "We use the latest Go version" (link to journey for why)

### Documentation Guidelines

- **Journey docs are stories**: Include context, exploration, failed attempts
- **ADRs are decisions**: Formal structure, clear outcomes
- **READMEs are summaries**: What exists now, how to use it
- **Link between them**: READMEs should link to relevant journey docs/ADRs
- **Avoid assumptions**: Document enough context so readers understand without guessing

This approach ensures:
- Future developers (human or AI) understand the full context
- Decisions can be revisited with full historical knowledge
- READMEs stay readable while preserving important details

## Development Workflow

### Pre-commit Workflow
**ALWAYS** run before committing:
```bash
make pre-commit
```
This runs:
1. `fmt` - Format code with gofmt and goimports  
2. `tidy` - Clean dependencies with go mod tidy
3. `fix-eof` - Add missing EOF newlines
4. `lint` - Run golangci-lint with comprehensive checks
5. `test` - Run unit tests with coverage

### 🚨 CRITICAL RULE: NEVER USE --no-verify 🚨
**NEVER, EVER, EVER use `git commit --no-verify`**
- CI will fail anyway - same checks run there
- Fix issues locally - it's faster than fixing in PR
- If linter seems wrong, fix the config, don't skip the check
- See IMPORTANT_NEVER_SKIP_PRECOMMIT.md for details

### Linting Setup
Based on rpg-toolkit's proven configuration:
- **golangci-lint**: Comprehensive linting with 20+ linters
- **Git hooks**: Automatic pre-commit checks via `.githooks/pre-commit`
- **Auto-formatting**: gofmt with simplify + goimports with local prefixes

Install git hooks once:
```bash
make install-hooks
```

Install development tools:
```bash
make install-tools
```

## Testing & Coverage Philosophy

### Entity Testing Decision
- **Entities are data structs**: Test them through usage in handler/service tests
- **Use explicit EXPECT matching**: Return complete entities in mocks for better coverage
- **Check response thoroughly**: Verify multiple fields to ensure conversions work
- **Postpone dedicated entity tests**: Until entities have behavior (validation, methods)

### Coverage Focus
- **Internal code only**: Exclude `/gen/`, `/mock/`, `/cmd/` from coverage metrics
- **Handler coverage target**: 40-50% is good (mostly translation logic)
- **Service coverage target**: 80%+ (business logic lives here)
- **0% new code coverage is OK**: During outside-in development when adding contracts

## Remember

- Explicit > Implicit (always use Input/Output types)
- Simple > Complex (entities are just data)
- rpg-api orchestrates, rpg-toolkit calculates
- Test with real dependencies when safe
- Document the journey, not just destination
- Tell stories in journey docs, make decisions in ADRs, summarize in READMEs
- Tests should be thorough and "set and forget"
