# Lobby SDK test boundary

Status: approved by Kirk in this session on 2026-09-24. Kirk subsequently
selected subagent-driven execution of the implementation plan.

Owning issue: [rpg-api#1046](https://github.com/KirkDiggler/rpg-api/issues/1046).
Parent: [rpg-api#1045](https://github.com/KirkDiggler/rpg-api/issues/1045).
Baseline: `origin/dev` at `9ef6c52e`, fetched 2026-09-24.

## Intent and success

Kirk's direction is that API tests prove storage, conversion, and correct toolkit
invocation, not toolkit rules. This pilot removes real session setup from lobby
unit tests and makes launch orchestration independently testable. It does not
change gameplay, persistence semantics, lobby authorization, or the SDK.

Success means a lobby test can supply an opaque compiled world, script SDK
outcomes, and prove exact inputs, required call ordering, and API side effects
without constructing a playable character, running combat, or rolling dice.

The proposal deliberately preserves existing real-stack coverage whose unique
assertions have not yet been mapped to another owner. Retaining these during
migration does not ratify them as permanent API tests.

## Approach selected for review

Use one consumer-owned six-method interface, matching the existing session
handler pattern. Alternatives rejected:

- Deleting the real-stack tests first would remove evidence before establishing
  the replacement API contracts.
- A generic toolkit facade or interfaces for every toolkit object would enlarge
  the architecture without helping this consumer.

### Production boundary

Add `internal/orchestrators/lobby/session_manager.go` with `SessionManager`:

```go
type SessionManager interface {
    StartSession(context.Context, *sdk.StartSessionInput) (*sdk.StartSessionOutput, error)
    Spawn(context.Context, *sdk.SpawnInput) (*sdk.SpawnOutput, error)
    Join(context.Context, *sdk.JoinInput) (*sdk.JoinOutput, error)
    PlaceNPC(context.Context, *sdk.PlaceNPCInput) (*sdk.PlaceNPCOutput, error)
    Status(context.Context, *sdk.StatusInput) (*sdk.Status, error)
    End(context.Context, *sdk.EndInput) (*sdk.EndOutput, error)
}
```

These are all six SDK methods called by non-test lobby code at the baseline.
Use canonical SDK types and `var _ SessionManager = (*sdk.Manager)(nil)`.
Change `Config.SessionManager` and the stored dependency to this interface.
The real manager still passes directly from existing server/harness construction;
there is no adapter implementation, second manager, or change to provider pins.

Generate `mock/mock_session_manager.go` using the repository's gomock convention
and a `go:generate` directive adjacent to the interface. A missing interface
value remains a construction error. Callers must supply a valid non-nil manager;
do not introduce reflection or a generic dependency-validation framework for
Go's typed-nil interface corner case.

## Unit fixture

Convert `LobbySuite` and constructor tests to the generated SDK mock. Keep the
existing API-owned in-memory lobby repository and broker where useful, and the
character repository mock for ownership/name lookups. Use the existing dungeon
registry mock with hand-constructed `dungeons.Entry`/`sessionworld.Dungeon` data.
Do not load shipped YAML or construct a session orchestrator for these tests.

Replace `seedLiveSession`/`seedEndedSession` helpers with explicit per-test SDK
expectations. Avoid permissive shared `AnyTimes` defaults: an unauthorized or
premature SDK call must fail the test.

Use input equality or explicit field matchers, including the original context
where forwarding it is the claim. Empty but valid SDK output structs are enough
when the orchestrator intentionally ignores the output. In particular,
StartEncounter generates the encounter ID itself; it does not adopt an ID from
StartSession's response. Tests must preserve this actual contract.

## Contracts to prove

### StartEncounter

Use a synthetic non-default dungeon for the main case, with two monsters and
two party members whose order is deliberately distinguishable.

1. Resolve the requested registry key; empty input resolves `dungeons.DefaultKey`.
2. Supply the exact compiled world and resolved key to StartSession with the
   API-generated session/encounter ID.
3. Spawn every monster in authored order before the first party Join.
4. Forward IDs, refs, positions, holdings, faction, actions, converted arrival
   and social approaches, and unchanged table/temper values. Preserve meaningful
   nil/empty distinctions; do not derive behavior from these values.
5. Join members in lobby `MemberOrder` at the corresponding authored seats.
6. For the default key only, preserve the existing demo-vendor PlaceNPC call
   after the joins. Assert session/member/position and a supplied NPC payload,
   not prices, stock, trade legality, or unpacking results. The production
   `npcs.NewMerchant` construction remains unchanged; removing that temporary
   product fixture belongs to #903, not this interface refactor.
7. Save the lobby as Started with the generated ID, then publish exactly one
   EncounterStarted event carrying that ID, and return the same ID.

Order assertions cover meaningful dependencies, not every incidental helper
call. Use a small recording/failing lobby-repository wrapper to assert Save and
publication behavior against the real in-memory repository/broker.

The new Facing field is carried in sessionworld but is not currently forwarded
by this launch. Do not silently add a new production forwarding behavior or
claim coverage of it in this behavior-preserving slice. Record such a provider/
consumer gap separately if it still exists when implementation starts.

### Refusals and failures

Cover missing lobby, already-started lobby, non-host, not-ready member, missing
registry entry, insufficient seats, and duplicate runtime member IDs. These
must produce no SDK call, final lobby save, or EncounterStarted event.

Inject failures at registry lookup, StartSession, Spawn (including after one
successful spawn), Join (including after one successful join), PlaceNPC, and
final lobby Save. Verify contextual wrapping preserves `errors.Is`, later SDK
operations stop, and no success event is emitted. The persisted lobby remains
Waiting when its final Save never succeeds, subject to the repository's current
save contract.

This does NOT assert rollback of prior SDK operations. Partial sessions and
provider-owned first-admission persistence retain their current semantics
(#800/#831). Tests prove the host stops calling and does not announce success;
they do not simulate or infer provider storage internals.

### GetMyActiveLobby and AbandonEncounter

Script Status with Open=true, Open=false, missing-session/missing-encounter
sentinels (including wrapped errors), and an arbitrary failure. Verify the exact
encounter ID and current empty-output/error behavior. Waiting/missing/stale-index
paths do not invoke Status. Keep API-owned index repair tests.

Script End with success, missing/closed sentinels, and arbitrary failures. Verify
the exact session and withdrawn ending, host/readiness gates, unchanged lobby
storage, and existing error translation. An abandon-then-resume test scripts End
and subsequent Status; it does not re-test whether toolkit End closes a session.

## Initial old-test disposition

| Existing coverage | Pilot disposition |
| --- | --- |
| `lobby_test.go` shared fixture | Replace real manager/miniredis/shipped registry with SDK and registry mocks. |
| `orchestrator_test.go` constructor cases | Replace real manager setup; preserve constructor validation assertions. |
| `get_my_active_lobby_test.go` | Preserve all API cases, replace live/ended setup with exact Status expectations, extend error cases. |
| `abandon_encounter_test.go` | Preserve host/lifecycle/error cases with exact End/Status expectations. |
| Launch missing-lobby/non-host/not-ready/unknown-key tests in `start_encounter_session_stack_test.go` | Replace with isolated gate tests after matching their API assertions. |
| `arrival_test.go` and pure social-approach conversion tests | Keep; they test API-owned spelling/presence. |
| Existing broader launch/wire/reload/content suites | Keep active pending assertion-level classification; new exact-call tests do not automatically replace their end-to-end claims. |
| Trade/sell/unpack/rest/loot mechanics embedded in launch suites | Keep active temporarily; route removal/provider equivalence to #1049. Do not add more. |

This is a bounded pilot, not completion of the entire test-ownership cleanup.
A disposition record in the implementation PR will name every test actually
removed and its replacement. No broad test-file deletion, test skips, or weaker
assertions merely to get a green run.

## Documentation and verification

Update the owning lobby component document to describe this interface and the
separate isolated/real-stack tests. Correct nearby stale claims about launch
ordering (the code spawns before joining), registry dependencies, and required
ID generators. Do not rewrite unrelated architecture history.

Baseline executed successfully before any source changes:

```sh
GOWORK=off go test ./internal/orchestrators/lobby ./internal/handlers/dnd5e/lobby/...
```

Implementation evidence must include:

- Red/green tests for exact-call, gate, ordering, and error behavior.
- Focused lobby and lobby-handler race tests, with mock regeneration checked.
- `go build ./...` to verify concrete production manager wiring.
- Repository `make pre-commit` and `make ci-check` gates with actual results.
- Existing real-stack lobby tests still active and passing, not silently skipped.

No Docker-backed integration run is necessary just to review this document.
If implementation needs one, coordinate the repository's single-runner rule.

## Scope and next gate

No toolkit/proto changes, gameplay decisions, new rollback mechanism, generic
engine service, character cleanup, or automatic merge are authorized here.

Written-design approval is recorded above. Kirk selected subagent-driven
execution after receiving the implementation plan; execution now follows that
plan with sequential writers and fresh task reviewers.
