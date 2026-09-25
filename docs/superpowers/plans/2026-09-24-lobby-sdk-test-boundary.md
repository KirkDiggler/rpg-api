# Lobby SDK Test Boundary Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans for parent execution, or superpowers:subagent-driven-development only if the operator selects delegation. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make lobby unit tests prove exact SDK calls and API side effects without running session rules.

**Architecture:** Add a consumer-owned six-method SessionManager interface satisfied directly by the real SDK manager. Use generated gomock outcomes, the existing dungeon registry mock, and API-owned in-memory lobby storage/broker. Preserve gameplay and all unique real-stack coverage pending its separately tracked migration.

**Tech Stack:** Go (go.mod targets 1.25.14), canonical session SDK v0.109.0 types, Uber gomock, testify suites, existing in-memory lobby repository and broker.

**Spec:** `docs/superpowers/specs/2026-09-24-lobby-sdk-test-boundary-design.md` (approved by Kirk).

## Global Constraints

- Work only in `/home/kirk/game-dev/rpg-api/.worktrees/1046-lobby-sdk-tests`, branch `refactor/1046-lobby-sdk-tests`, issue #1046; base `9ef6c52e`.
- No toolkit/proto changes, gameplay decisions, new rollback mechanism, generic engine service, character cleanup, or automatic merge are authorized here.
- No broad test-file deletion, test skips, or weaker assertions merely to get a green run.
- The real manager still passes directly from existing server/harness construction; there is no adapter implementation, second manager, or change to provider pins.
- Kirk selected subagent-driven execution after reviewing this plan. Use sequential writers and fresh task reviewers; children never delegate.
- API creates the encounter ID; do not infer that StartSession's output controls it. SDK return values intentionally unused by lobby remain unused.
- Keep Go source formatting, generated-mock freshness, and repository gates. Never bypass pre-commit or restore unrelated work.

## Review Focus

1. Defaulted dungeon key: SDK sees the resolved key, vendor placement happens only for that key (Task 3).
2. Two monsters/two members: all spawns precede every join; member-map iteration must not decide seat order (Task 3).
3. Wrapped SDK sentinels: `errors.Is` behavior survives API wrapping, and ordinary errors do not become missing-session success (Tasks 2, 4).
4. Partial provider success: later failure emits no success event and triggers no invented rollback (Task 4).
5. Slice presence and duplicate runtime IDs: absent social routes stay absent, and collisions never reach SDK (Tasks 3, 4).

---

## Prerequisite: repair the local verification tool, not repository requirements

The baseline focused command passed before changes:

```sh
GOWORK=off go test ./internal/orchestrators/lobby ./internal/handlers/dnd5e/lobby/...
```

`make pre-commit` stopped at lint: installed golangci-lint was built with Go 1.24,
less than target 1.25.14. Evidence: `/tmp/1046-design-precommit.log`. No gate was
bypassed; design and plan are uncommitted.

- [ ] Inspect `go version`, `command -v golangci-lint`, `golangci-lint version`, and `.golangci.yml` before selecting a compatible version. Do not downgrade go.mod or rewrite lint policy.
- [ ] Use a compatible explicitly installed linter, preferably scoped to a local tool directory and process PATH so other sessions' binaries remain untouched. Tool version selection must match the checked-in config generation; stop and report if a compatible tool cannot be obtained. Do not run the broad `make install-tools` target merely to refresh all tools.
- [ ] Re-run `GOWORK=off make pre-commit` before claiming the environment is green or making the first commit. Separate pre-existing unrelated failures from this change; do not repair adjacent features under #1046.

## File map

Create:
- `internal/orchestrators/lobby/session_manager.go`: consumer interface, compatibility assertion, generation directive.
- `internal/orchestrators/lobby/mock/mock_session_manager.go`: generated implementation.
- `internal/orchestrators/lobby/start_encounter_contract_test.go`: suite, synthetic registry fixtures, success/order assertions.
- `internal/orchestrators/lobby/start_encounter_failure_test.go`: gate/failure tests and fault-injecting lobby repository wrapper.

Modify:
- `internal/orchestrators/lobby/orchestrator.go`: dependency types only.
- `internal/orchestrators/lobby/orchestrator_test.go`: constructor fixture no longer constructs an SDK runtime.
- `internal/orchestrators/lobby/lobby_test.go`: shared isolated fixture.
- `internal/orchestrators/lobby/get_my_active_lobby_test.go`, `abandon_encounter_test.go`: exact Status/End expectations.
- `internal/orchestrators/lobby/start_encounter_session_stack_test.go`: remove only four replaced gate cases named in Task 5.
- `docs/architecture/components/lobby-service.md`: boundary, ordering, dependencies, verification commands.
- `docs/status.md`, `docs/quality.md`: update only lobby-specific claims affected by this change after reading their current contents.

Keep production `start_encounter_session_stack.go`, `get_my_active_lobby.go`, and
`abandon_encounter.go` behavior unchanged. If a new test exposes a behavior defect,
stop, report it, and separate the fix rather than silently changing this contract.

## Task 1: introduce the narrow dependency with a compile-time test

**Interfaces:** Produces `lobby.SessionManager`; `Config.SessionManager` and the
stored `sessionManager` use it. `*sdk.Manager` remains directly assignable.

- [ ] Add the following constructor test in `orchestrator_test.go`, importing the future mock as `lobbymock`:

```go
func TestNew_AcceptsSessionManagerMock(t *testing.T) {
    cfg := baseTestConfig(t)
    cfg.SessionManager = lobbymock.NewMockSessionManager(gomock.NewController(t))
    got, err := lobbyorch.New(cfg)
    require.NoError(t, err)
    require.NotNil(t, got)
}
```

- [ ] Run `GOWORK=off go test ./internal/orchestrators/lobby -run TestNew_AcceptsSessionManagerMock -count=1`; record the expected missing mock/type compilation failure.
- [ ] Create `session_manager.go`:

```go
package lobby

import (
    "context"
    sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

//go:generate mockgen -destination=mock/mock_session_manager.go -package=lobbymock github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby SessionManager

// SessionManager is the session SDK surface consumed by lobby orchestration.
type SessionManager interface {
    StartSession(context.Context, *sdk.StartSessionInput) (*sdk.StartSessionOutput, error)
    Spawn(context.Context, *sdk.SpawnInput) (*sdk.SpawnOutput, error)
    Join(context.Context, *sdk.JoinInput) (*sdk.JoinOutput, error)
    PlaceNPC(context.Context, *sdk.PlaceNPCInput) (*sdk.PlaceNPCOutput, error)
    Status(context.Context, *sdk.StatusInput) (*sdk.Status, error)
    End(context.Context, *sdk.EndInput) (*sdk.EndOutput, error)
}

var _ SessionManager = (*sdk.Manager)(nil)
```

- [ ] Replace the two `*sdk.Manager` field types in `orchestrator.go` with `SessionManager`; remove its unused SDK import. Keep existing nil validation, field names, constructor behavior, and production call sites.
- [ ] Generate using `GOWORK=off go generate ./internal/orchestrators/lobby`. Do not hand-edit generated code.
- [ ] In `orchestrator_test.go`, delete `newTestSessionManager`; change `baseTestConfig` to use `lobbymock.NewMockSessionManager(ctrl)` and `dungeonsmock.NewMockRegistry(ctrl)`. Build the negative-party-cap case from `baseTestConfig`, setting `PartyCap=-1`. Remove miniredis/session-orchestrator/time imports that become unused.
- [ ] Run `GOWORK=off go test ./internal/orchestrators/lobby -run TestNew -count=1` and `GOWORK=off go build ./...`. Expected: pass with both generated and real managers assignable.
- [ ] Run repository gates before committing these files with `refactor(lobby): depend on the consumed session SDK interface`.

## Task 2: isolate shared lobby lifecycle tests

**Interfaces:** Consumes `SessionManager` and generated `MockSessionManager`.
Produces shared fixture fields `manager *lobbymock.MockSessionManager` and
`registry *dungeonsmock.MockRegistry` on the existing `LobbySuite`.

- [ ] Replace `sessOrch` in `LobbySuite.SetupTest` with the SDK mock and add the registry mock. Pass these through both constructors in `lobby_test.go`. Keep the existing character mock, broker, in-memory repository, deterministic IDs, and `seedLobby` helper. Delete real-session constructors and live/ended-session helpers.
- [ ] Run `GOWORK=off go test ./internal/orchestrators/lobby -run TestLobbySuite -count=1`; the tests still using deleted live/ended helpers must fail compilation. This is a fixture migration, not a claim of a newly failing behavioral regression.
- [ ] Replace each real-session setup with per-test expectations. For live status:

```go
s.manager.EXPECT().Status(s.ctx, &sdk.StatusInput{Session: "enc-live"}).
    Return(&sdk.Status{Open: true}, nil)
```

For terminal status use `Open: false`; for missing state use wrapped
`fmt.Errorf("provider: %w", sdk.ErrNoSession)` and `sdk.ErrNoEncounter`. Keep
assertions about lobby/encounter IDs, empty outputs, and player-index repair.
Add an arbitrary error case using `boom := errors.New("provider unavailable")`
and `s.ErrorIs(err, boom)`; no empty-success fallback.

- [ ] Change abandonment's helper to seed only a Started lobby, not a real session. Set exact expectations per test:

```go
s.manager.EXPECT().End(s.ctx, &sdk.EndInput{
    Session: encID, Ending: sessionworld.EndingWithdrawn,
}).Return(&sdk.EndOutput{}, nil)
```

Add individual wrapped `ErrNoSession`, `ErrNoEncounter`, `ErrClosed` cases mapping
to `ErrEncounterAlreadyEnded`, and an arbitrary failure that remains `errors.Is`
matchable. Snapshot the lobby before/after to prove no lobby mutation. Leave
SDK expectations absent in unauthorized/waiting/missing-lobby cases.

- [ ] For abandon-then-resume, use ordered End(success), Status(Open=false)
expectations; do not assert toolkit's state transition. Verify no lobby broker
event from abandonment by subscribing before the call and checking the buffered
channel synchronously after return (no sleeps).
- [ ] Run `GOWORK=off go test -race ./internal/orchestrators/lobby -run TestLobbySuite -count=1` and the full lobby package. Expected: unchanged API results without real SDK setup in this suite.
- [ ] Gate and commit as `test(lobby): script SDK status and abandonment outcomes`.

## Task 3: exact launch contract and publication ordering

**Interfaces:** Create `StartContractSuite` in the external `lobby_test` package,
embedding `LobbySuite` for SetupTest and its dependency fields. Register it with
`func TestStartContractSuite(t *testing.T) { suite.Run(t, new(StartContractSuite)) }`.
Use existing `seedLobby(*lobbyrepo.Data)` and `newOrchestratorWithLobbyRepo`.

- [ ] Add a test-only `readyContractLobby() *lobbyrepo.Data` returning:

```go
return &lobbyrepo.Data{
    ID: "lobby-contract", JoinRef: "ref-contract", HostPlayerID: "alice",
    Status: lobbyrepo.StatusWaiting,
    Members: map[string]*lobbyrepo.Member{
        "alice": {PlayerID: "alice", CharacterID: "char-a", IsHost: true, IsReady: true},
        "bob": {PlayerID: "bob", CharacterID: "char-b", IsReady: true},
    },
    MemberOrder: []string{"bob", "alice"},
}
```

- [ ] Add `contractEntry(key string) *dungeons.Entry` returning a canonical
Entry with `Key:key`, `Atlas:&sdk.Atlas{}`, and a Dungeon containing an opaque
`&tkencounter.EncounterData{}`, seats `{X:7,Y:-3}` and `{X:-2,Y:5}`, and two monsters
(`guard-a`, `guard-b`) with distinct literal refs/positions. Neither world nor
monster refs need to be executable by toolkit: this test never calls it.

- [ ] Add the happy-path test with exact StartSession, Spawn, and Join inputs,
using `gomock.InOrder`. StartSession must receive the very same world pointer
(check `s.Same` inside `DoAndReturn`) and resolved key. Capture its generated
Session ID and use it for later exact field assertions. Return empty valid
SDK outputs. The joins must be `char-b` at seat 0 then `char-a` at seat 1.

```go
s.manager.EXPECT().StartSession(s.ctx, gomock.Any()).DoAndReturn(
    func(_ context.Context, in *sdk.StartSessionInput) (*sdk.StartSessionOutput, error) {
        s.Same(entry.Dungeon.World, in.World)
        s.Equal(entry.Key, in.Dungeon)
        s.NotEmpty(in.Session)
        s.Equal(in.Session, in.Encounter)
        sessionID = in.Session
        return &sdk.StartSessionOutput{}, nil
    },
)
```

`entry` is the local result of `contractEntry("custom-dungeon")`; define
`sessionID := ""` before expectations. Inside Spawn/Join callbacks compare the
full input to explicitly built SDK inputs carrying `sessionID`. Do not use
production converters to compute expected converted values.

- [ ] Populate one monster with holdings `[]string{"dungeon/letter"}`, actions
`[]string{"weapon-b", "weapon-a"}`, faction `"guards"`, and the following social
and arrival fixture/expectation pairs (do not call production converters to
construct the expectations):

```go
// Authored fixture:
Intimidate: []tkencounter.CheckApproach{
    {Ability: "intimidation", DC: 12},
    {Ability: "str", Tool: "dnd5e:item:brass-knuckles", DC: 15},
},
Persuade: []tkencounter.CheckApproach{{Ability: "cha", DC: 17}},
Arrives: tkencounter.TriggerRound{Round: 6},
// Expected SDK input:
Intimidate: []sdk.DoorApproach{
    {Ability: "intimidation", DC: 12},
    {Ability: "str", Tool: "dnd5e:item:brass-knuckles", DC: 15},
},
Persuade: []sdk.DoorApproach{{Ability: "cha", DC: 17}},
Arrives: sdk.ArrivesAtRound{Round: 6},
```

Keep the second monster's optional values nil; add a separate explicitly-empty
slice case. For table and temper, reuse nonzero canonical fixture values from
the pinned provider's table tests without executing provider constructors; assert
full equality with the fixture, not that those values cause particular behavior.

- [ ] Add a repository wrapper in `start_encounter_failure_test.go`:

```go
type observedLobbyRepository struct {
    lobbyrepo.Repository
    saveCalls int
    beforeSave func(*lobbyrepo.Data)
    saveErr error
}

func (r *observedLobbyRepository) Save(ctx context.Context, data *lobbyrepo.Data) error {
    r.saveCalls++
    if r.beforeSave != nil { r.beforeSave(data) }
    if r.saveErr != nil { return r.saveErr }
    return r.Repository.Save(ctx, data)
}
```

Seed the underlying repository before wrapping it so setup writes do not count.
Subscribe to `s.broker` for `lobby-contract`; set `beforeSave` to assert Started,
the captured ID, and an empty event channel. After the successful launch, assert
one save, exact persisted status/ID, exact output ID, one EncounterStarted event,
and no second event:

```go
select {
case event := <-sub.Events():
    s.Equal(lobbyorch.EventKindEncounterStarted, event.Kind)
    s.Equal(sessionID, event.EncounterStarted.EncounterID)
default:
    s.FailNow("successful launch did not publish EncounterStarted")
}
select {
case event := <-sub.Events(): s.Failf("unexpected extra event", "%+v", event)
default:
}
```

- [ ] Add explicit-default and omitted-key cases expecting registry lookup and
StartSession.Dungeon=`dungeons.DefaultKey`, plus PlaceNPC only after the joins.
Assert `demo-merchant-1`, same session ID, seat-0 X+1/Y position, non-nil NPC
payload; do not inspect prices/stock or call Interact/Trade. The custom-key test
has no PlaceNPC expectation. Add the demo-vendor member-ID collision case in
Task 4. Do not add Facing forwarding to production code.
- [ ] Run `GOWORK=off go test ./internal/orchestrators/lobby -run TestStartContractSuite -count=1`. Existing behavior may pass immediately; prove sensitivity by temporarily introducing one narrowly scoped fault at a time (join order or resolved key or publish-before-save), observe the named assertion fail, then undo only that exact edit. Never use a broad checkout/reset. Re-run green and inspect the diff.
- [ ] Gate and commit as `test(lobby): pin launch inputs and save-before-publish ordering`.

## Task 4: isolate refusal and partial-failure behavior

**Interfaces:** Consumes StartContractSuite, readyContractLobby, contractEntry,
and observedLobbyRepository from Task 3; no production interfaces are added.

- [ ] Add individual suite cases for nil input, missing lobby, already-started,
non-host, not-ready, unknown dungeon, insufficient seats, duplicate party IDs,
party/monster collision, and party/demo-vendor collision. Seed and mutate the
literal lobby/entry fixtures; expect registry Get only where the gate is after
lookup. Do not set any SDK expectations. Assert error, zero final Save attempts,
unchanged stored lobby, and an empty subscribed broker channel. For missing
lobby, assert it is still absent instead of comparing a nonexistent snapshot.
- [ ] Add arbitrary repository Get and registry Get error cases. For Get use a
small wrapper embedding `lobbyrepo.Repository` with only Get overridden:

```go
type failingLobbyGet struct {
    lobbyrepo.Repository
    err error
}
func (r failingLobbyGet) Get(context.Context, string) (*lobbyrepo.Data, error) {
    return nil, r.err
}
```

Use `newOrchestratorWithLobbyRepo` to inject it and assert `ErrorIs` against the
original failure. No SDK or final-save expectation is allowed.
- [ ] Add tests for StartSession failure, first/second Spawn failure, first/second
Join failure, default-key PlaceNPC failure, and final Save failure. Build ordered
expectations only through the failing call. For example, StartSession failure:

```go
boom := errors.New("provider unavailable")
s.registry.EXPECT().Get(s.ctx, "custom-dungeon").Return(entry, nil)
s.manager.EXPECT().StartSession(s.ctx, gomock.Any()).Return(nil, boom)
out, err := orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
    PlayerID: "alice", LobbyID: "lobby-contract", DungeonKey: "custom-dungeon",
})
s.Nil(out)
s.ErrorIs(err, boom)
s.Zero(repo.saveCalls)
```

Here `entry := contractEntry("custom-dungeon")`, `repo` is the seeded underlying
repository wrapped in `observedLobbyRepository`, and `orch :=
s.newOrchestratorWithLobbyRepo(repo)`. For failures after earlier SDK success,
return concrete empty success outputs for prior calls, then `nil, boom` from the
failing call. No End/cleanup call is expected; adding a rollback would fail the
mock. Use `saveErr:boom` only for the final Save-failure case and assert one
attempt, unchanged backing store, and no event.
- [ ] Set the second monster's `Arrives` to `tkencounter.TriggerExternal{}`
(the unsupported arrival already covered in `arrival_test.go`). Expect only
StartSession and the first Spawn to succeed. Assert the error contains
`TriggerExternal`, no second Spawn or Join occurs, no final Save is attempted,
and no success event is emitted. Keep the existing pure conversion test too.
- [ ] Run `GOWORK=off go test -race ./internal/orchestrators/lobby -run TestStartContractSuite -count=1`. Prove at least the stop-after-error and no-event-on-save-failure assertions fail under narrow temporary fault injection, restoring those edits precisely before continuing. Confirm no gameplay/provider state assertions were added.
- [ ] Gate and commit as `test(lobby): cover refusal and partial-launch failure contracts`.

## Task 5: remove the four replaced gate tests and document the boundary

**Interfaces:** No new code interfaces; consumes the verified contracts above.

- [ ] In `start_encounter_session_stack_test.go`, remove only:
  - `TestStartEncounter_NotHost_Errors`
  - `TestStartEncounter_NotAllReady_Errors`
  - `TestStartEncounter_LobbyNotFound_Errors`
  - `TestStartEncounter_UnknownDungeonKeyIsRefused`

Delete imports only when now unused. Do not delete helpers still used by the
remaining real-stack suite. Each removed assertion must match a Task 4 case.
- [ ] Keep all trading, unpacking, rest, reload, geometry, and broader content
cases active. Record them as retained pending #1049, not as an approved permanent
smoke suite. Keep pure arrival/social conversion tests.
- [ ] Update `docs/architecture/components/lobby-service.md`: describe the
six-method interface and generated mock; state StartSession → all Spawn calls →
all Join calls → default vendor → Save → Publish; correct the registry and three
ID-generator dependency description. Replace its claim that all lobby fixtures
use miniredis with separate isolated and real-stack commands.
- [ ] Read `docs/status.md` and `docs/quality.md`, then revise only stale
lobby-specific testing/boundary claims. Do not claim every API test is isolated or
that integration migration is finished. Record this exact disposition table:

| Old coverage | Replacement/disposition |
| --- | --- |
| Shared live/ended fixtures | Mocked Status/End with current API expectations retained |
| Constructor real SDK setup | Compile-time manager compatibility + constructor mock tests |
| Four real-stack gate cases named above | StartContractSuite gate cases with no SDK/no-write/no-event assertions |
| Other real-stack lobby cases | Retained unchanged pending #1049 ownership mapping |

- [ ] Run final checks from this worktree:

```sh
GOWORK=off go generate ./internal/orchestrators/lobby
GOWORK=off go test -race ./internal/orchestrators/lobby ./internal/handlers/dnd5e/lobby/... -count=1
GOWORK=off go build ./...
GOWORK=off make pre-commit
GOWORK=off make ci-check
git diff --check
```

Check mock regeneration determinism with a second generation and comparison
against the first result; do not interpret unrelated worktree dirt as mock drift.
Coordinate any Docker-backed integration gate with the single-runner rule. Do
not run `make test-integration` blindly against the shared Docker daemon.
- [ ] Gate and commit as `docs(lobby): record isolated SDK test ownership and dispositions`.
- [ ] Publish the draft PR on the first push, targeting `dev`, with `Closes #1046`,
actual gate output, retained-test risks, and independent review explicitly pending.
Use the Platform signature with freshly confirmed authenticated login. Do not
close #1045 or mark unrelated cleanup children complete. No automatic merge.

## Handoff and approval

Recommended execution: parent/native in this session, because all five tasks
modify one tightly coupled lobby fixture and interface. Independent review is a
separate required gate before merge-ready, not permission to delegate now.

Kirk selected subagent-driven execution after this plan was presented. Use the
available subagent protocol rather than launching external agents directly.
