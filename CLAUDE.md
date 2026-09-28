# rpg-api

rpg-api is the game server for the D&D 5e dungeon crawler: it orchestrates by
key and stores data. It knows feature keys, not behavior. **rpg-api stores
data. rpg-toolkit handles rules.**

House truth — the lens, working agreements, wave law — lives in
`rpg-project/CLAUDE.md`. This file carries what is specific to rpg-api.
`AGENTS.md` is a symlink to this file so every agent runtime boots from the
same instructions.

## Where things live

- `docs/status.md` — current health: active work, paused items, known rough edges, per-subsystem confidence
- `docs/quality.md` — A-D scorecard with rationale per component
- `docs/architecture/overview.md` — layer rules (handler → orchestrator → repo), request flow, cross-repo boundaries
- `docs/architecture/data-model.md` — entities, relationships, storage schemas, known gaps
- `docs/architecture/components/` — one doc per major component (auth, authoring-service, character-handler, character-orchestrator, dungeon-component, encounter-handler, encounter-orchestrator, entities, event-processor, integration-test-harness, lobby-service, repositories)
- `docs/how-to/` — task guides: `add-handler-method`, `run-integration-tests`, `run-locally`, `update-proto-dependency`, `local-toolkit-override` (iterate on rpg-toolkit changes in the local Docker loop without publish → tag → `go get`)
- `docs/archive/` — pre-PR #470 historical docs (old ADRs, journey narratives, plans, design notes, session handoffs); read for context, not current state

## The law: no game logic in the API

If it is a game mechanic or calculation → rpg-toolkit. If it is data storage
or API orchestration → rpg-api.

**Adding game logic here is a smell that the toolkit is missing something.**
The toolkit provides the function that takes the entity directly; the API
passes the entity through:

```go
// BAD: the API checking game rules
mainHandID := slots.Get(character.SlotMainHand)
offHandID := slots.Get(character.SlotOffHand)
if mainHandID != "" && offHandID != "" && offHandID != armor.Shield {
    // extract weapon IDs, check conditions...
}

// GOOD: the API passes the character; the toolkit knows the rules
result, err := actions.CheckAndGrantOffHandStrikeForCharacter(ctx, char, attackHand, bus)
```

When you hit the smell: open the issue in rpg-toolkit for the missing helper,
then pass the entity through here. Never reconstruct the rule in the API.

## Project structure

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

## Development approach: outside-in

1. **Start with gRPC handlers** — return `codes.Unimplemented`. Validates the
   proto, the server starts, services register; no logic, no dependencies.
2. **Define service interfaces** with Input/Output types; generate mocks for
   the handler tests.
3. **Write handler tests** against the mocked services — request validation,
   response mapping, error handling.
4. **Implement orchestrators** — the business logic, wired to repositories and
   engine, tested with mocked dependencies.
5. **Implement repositories** last, when you know what you need.

Why: the API is usable before implementation exists, interfaces are driven by
real need, and contracts stay refactorable.

## Laws

**Explicit > implicit — Input/Output types on every function, at every layer.**
Handlers: Request/Response. Orchestrators: Input/Output. Repositories:
Input/Output. Even helpers. Why: adding a field never changes an interface,
never regenerates a mock, and is future-proof for pagination.

```go
// BAD
func CreateSession(name string, dmID string, maxPlayers int) (*Session, error)
// GOOD
func CreateSession(ctx context.Context, input *CreateSessionInput) (*CreateSessionOutput, error)
```

**No magic strings.** Every repeated string literal becomes a named constant —
entity types and sources, error codes and messages, configuration keys,
status values.

**Entities are data structs.** No business logic on them; calculations belong
in the toolkit (proficiency bonuses, ability modifiers, condition checks).
Test entities through usage in handler/service tests, not standalone.

**Never return `(nil, nil)`** — a valid object or an error:

```go
if input == nil {
    return nil, errors.New("input is required")   // invalid input → error
}
if items == nil {
    return &ListOutput{Items: []*Item{}, Total: 0}, nil   // valid emptiness → default
}
```

Define errors at package level (`ErrSessionNotFound`,
`ErrPlayerNotInSession`) and wrap with context:
`fmt.Errorf("failed to get session %s: %w", id, ErrSessionNotFound)`.

**API versioning is external, through handlers** — `/handlers/sessionv1alpha1/`,
`sessionv1beta1/`, `sessionv1/` — while internal package shape stays stable.

**Storage has no database preference.** The repository pattern carries the
flexibility; Redis is the start, adapters arrive as needed.

**Documentation is living.** The shape is described in "Where things live"
and maintained in the same PR that invalidates a line. `status.md` and
`quality.md` reflect actual code, not stale snapshots. Cite `file:line` when
describing code; verify before asserting; surface gaps honestly. Historical
journey/ADR content lives under `docs/archive/`.

**Standards are tool-enforced, not memorized.** golangci-lint and the git
hooks say what needs fixing; when CI finds a new failure pattern, add its
detection to `scripts/ci-checks.sh` so it is caught locally forever after.

## Testing

- **Uber's gomock** (not mockery); **always test suites**; **real Redis when
  safe** (miniredis). Establish mocks in `SetupTest` with
  `gomock.NewController(s.T())`; use explicit EXPECT matching and complete
  entities so conversions get checked.
- **Mocks live beside their interface**: a `mock/` subdirectory, package
  `<parent>mock` (`charactermock`), file `mock_<interface>.go`, generated by
  the `//go:generate` directive above the interface.
- Coverage measures internal code only (`/gen/`, `/mock/`, `/cmd/` excluded):
  handlers 40–50% (mostly translation), services 80%+ (business logic).
  0% new-code coverage is fine during outside-in contract work.
- Integration tests: `docs/how-to/run-integration-tests`.

## Hard rules

- **Work in a worktree**, one per line of work — never a bare
  `git checkout -b` in the shared clone:
  ```bash
  git fetch origin
  git worktree add .worktrees/feat-character-creation -b feat/character-creation origin/dev
  ```
- **New work bases off `origin/dev`, never `main`.** `dev` is the actual
  current working state; `main` is periodically promoted and can sit tens of
  commits behind in between — including real toolkit dependency bumps and
  fixes. Branching from `main` makes already-resolved work look like fresh
  breakage. Confirm before assuming otherwise:
  `git log origin/main..origin/dev --oneline`.
- **NEVER use `git commit --no-verify`.** The same checks run in CI — fix
  issues locally; if the linter seems wrong, fix the config, not the check.
- **Run `make ci-check` before pushing.** When it finds a failure pattern,
  add the detection to `scripts/ci-checks.sh` — every CI failure becomes a
  local check that catches it forever.
- **`make pre-commit` is check-only** — it never changes tracked files or the
  index. Prepare deliberate source changes with `make fix`; regenerate mocks
  explicitly with `make generate` when interfaces changed; tools install only
  via `make install-tools`.

## Dependency updates

- Protos: `GOPROXY=direct go get github.com/KirkDiggler/rpg-api-protos/gen/go@generated`
- Toolkit: `GOPROXY=direct go get github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e@latest`
- Iterating on local toolkit changes without publish → tag → `go get`:
  `docs/how-to/local-toolkit-override`

## Remember

- Explicit > implicit; simple > complex; rpg-api orchestrates, rpg-toolkit calculates.
- **ALWAYS question data structures** — there are no guarantees we did it correctly.
- **Trust your instincts** — if something feels wrong, it probably is. Verify
  assumptions against actual API responses and data flows.
- **Don't blindly follow existing patterns** — they might be wrong.
- Tests are thorough and "set and forget".