# Claude AI Development Guidelines - rpg-api

**Note**: General development guidelines are in `/home/kirk/personal/CLAUDE.md`. This file contains rpg-api specific instructions.

## Where things live

Active docs — read these to orient before touching code:

- `docs/status.md` — current health: active work, paused items, known rough edges, per-subsystem confidence
- `docs/quality.md` — A-D scorecard with rationale per component
- `docs/architecture/overview.md` — layer rules (handler → orchestrator → repo), request flow, cross-repo boundaries
- `docs/architecture/data-model.md` — entities, relationships, storage schemas, known gaps
- `docs/architecture/components/` — one doc per major component (auth, character-handler, character-orchestrator, dungeon-component, encounter-handler, encounter-orchestrator, entities, event-processor, integration-test-harness, lobby-service, repositories)
- `docs/how-to/` — task guides: `add-handler-method`, `run-integration-tests`, `run-locally`, `update-proto-dependency`, `local-toolkit-override` (iterate on rpg-toolkit changes in the local Docker loop without publish → tag → `go get`)
- `docs/archive/` — pre-PR #470 historical docs (old ADRs, journey narratives, plans, design notes, session handoffs); read for context, not current state

## Dependency Management

### Proto Updates
To get the latest compiled protos, pull from the `generated` branch:
```bash
GOPROXY=direct go get github.com/KirkDiggler/rpg-api-protos/gen/go@generated
```

### Toolkit Updates
```bash
GOPROXY=direct go get github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e@latest
```

## Core Philosophy

**rpg-api stores data. rpg-toolkit handles rules.**

This separation is fundamental. When in doubt:
- If it's a game mechanic or calculation → rpg-toolkit
- If it's data storage or API orchestration → rpg-api

### Code Smell: Game Logic in the API

**If you find yourself adding game logic in rpg-api, STOP.** This is a smell that the toolkit is missing something.

**Examples of game logic that does NOT belong here:**
- Checking weapon properties (is it light? is it a shield?)
- Determining if conditions are met for abilities
- Calculating bonuses or modifiers
- Knowing what slots weapons go in

**What rpg-api SHOULD do:**
- Load data from repositories
- Pass entities to toolkit functions
- Persist results back to repositories
- Convert between proto and domain types

**When you hit this smell:**
1. Create an issue in rpg-toolkit for the missing helper
2. The toolkit should provide a function that takes the entity directly
3. The API just passes the entity through

**Example - Wrong (game logic in API):**
```go
// BAD: API is checking game rules
mainHandID := slots.Get(character.SlotMainHand)
offHandID := slots.Get(character.SlotOffHand)
if mainHandID != "" && offHandID != "" && offHandID != armor.Shield {
    // extract weapon IDs, check conditions...
}
```

**Example - Right (toolkit handles logic):**
```go
// GOOD: API just passes the character, toolkit knows the rules
result, err := actions.CheckAndGrantOffHandStrikeForCharacter(ctx, char, attackHand, bus)
```

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

The current shape is described in **"Where things live"** at the top of this file. Maintain it in the same PR that invalidates a line — `status.md` and `quality.md` are living docs and must reflect the actual code, not stale snapshots. Cite `file:line` when describing code; verify before asserting; surface gaps honestly.

Older guidance (per-repo `journey/` and `adr/` as primary doc types) has been retired in favor of the platform-mcp shape. Historical journey/ADR content lives under `docs/archive/`.

## Development Workflow

### Feature Release Workflow

**Every feature follows this workflow to ensure quality and catch CI issues early:**

1. **Always start from latest main**
   ```bash
   gcm                          # git checkout main
   gl                           # git pull
   ```

2. **Create feature branch**
   ```bash
   git checkout -b feat/spell-selection
   ```

3. **Develop the feature**
   - Write tests first (TDD) or alongside code
   - Follow existing patterns in the codebase
   - Keep commits focused and atomic

4. **Run tests locally**
   ```bash
   go test ./...                # Run all tests
   go test ./internal/orchestrators/character -v  # Run specific package tests
   ```

5. **Run CI checks locally BEFORE pushing**
   ```bash
   make ci-check               # Detect CI failures before push
   make ci-fix                 # Auto-fix what can be fixed
   ```

6. **When CI fails, learn from it**
   - Add the failure pattern to `scripts/ci-checks.sh`
   - Document why it failed in comments
   - Next time, the local check will catch it
   - Eventually we'll catch all common CI failures locally

7. **Create PR**
   ```bash
   git push origin feat/spell-selection
   gh pr create
   ```

8. **Address review feedback**
   - Check inline comments: `gh api repos/KirkDiggler/rpg-api/pulls/<number>/comments`
   - Fix issues and push updates
   - Thank reviewers for catching issues

9. **Merge when approved**
   - Let the PR author or reviewer merge
   - Delete branch after merge

**Key principle**: Every CI failure is a learning opportunity. Add detection for it locally so it never happens again.

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

## CI/CD Patterns & Common Failures

### Pre-Flight CI Checks
**ALWAYS run `make ci-check` before pushing** to detect CI failures locally:

```bash
make ci-check  # Comprehensive CI failure detection
make ci-fix    # Automatically fix common issues
```

### Staying Current with Standards

We follow **industry-standard Go practices** rather than maintaining custom rules:
- Use `golangci-lint` with recommended linters enabled
- Follow Go's official style guide and effective Go principles
- Let tooling enforce standards, not manual rules

The key is: **If CI fails locally with `make ci-check`, fix it before pushing.**

Common patterns that cause CI failures:
- Generated code out of sync → run `make generate`
- Formatting issues → run `make fmt`
- Linting issues → run `make lint` and fix what it reports
- Test failures → ensure tests pass with race detection enabled

Don't memorize specific rules - let the tools tell you what needs fixing.

### CI Check Script
We maintain `scripts/ci-checks.sh` that catches these issues before push.
Update it when we discover new CI failure patterns.

## Remember

- Explicit > Implicit (always use Input/Output types)
- Simple > Complex (entities are just data)
- rpg-api orchestrates, rpg-toolkit calculates
- Test with real dependencies when safe
- Document the why alongside the what; status.md and quality.md are the living record
- Tests should be thorough and "set and forget"
- **ALWAYS question data structures** - No guarantees we did it correctly
- **Trust your instincts** - If something feels wrong, it probably is
- **Verify assumptions** - Check actual API responses and data flows
- **Don't blindly follow existing patterns** - They might be wrong
- **Run CI checks locally** - Don't let CI be the first to find issues
