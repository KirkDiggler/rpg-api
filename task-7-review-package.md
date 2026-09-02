# Task 7 review package — rpg-api#882

## Exact review target

```bash
git diff --stat 54de176..HEAD
git diff --check 54de176..HEAD
git diff 54de176..HEAD -- \
  go.mod go.sum \
  internal/orchestrators/lobby/start_encounter_session_stack.go \
  internal/orchestrators/lobby/start_encounter_session_stack_test.go \
  internal/orchestrators/lobby/abandon_encounter_test.go \
  internal/handlers/dnd5e/lobby/v1alpha1/handler_test.go \
  docs/architecture/components/lobby-service.md docs/status.md \
  task-7-report.md task-7-review-package.md
```

Base is `54de176`, the recorded `origin/dev` base for the isolated worktree. Review the committed `HEAD`; no working-tree patch is part of the package.

## Decision being implemented

Approved spec/implementation decision: rpg-project#343 at `0b3821b`.

- Session `Join`, not rpg-api, owns first-admission normal-rest policy.
- Persisted `EverMembers` is the admission record.
- Session persists the rested character before projection/placement.
- A later Join failure reports durable progress with `SaveError`/`SaveReport`.
- rpg-api keeps the existing `StartSession → Join → Spawn` call sequence and removes its parallel lifecycle loop without recreating preflight.

## Scope inventory

Expected production/dependency diff:

- `go.mod`, `go.sum`: Session `v0.44.0 → v0.45.0`; root D&D remains exactly `v0.126.2`; resolution remains exactly `v0.29.0`.
- `internal/orchestrators/lobby/start_encounter_session_stack.go`: deletion of the complete direct member recovery loop and now-unused imports; no replacement logic.

Expected evidence/docs diff:

- `internal/orchestrators/lobby/start_encounter_session_stack_test.go`: real miniredis Manager/adapter success fixture and discriminating StartSession failure-order fixture.
- `internal/orchestrators/lobby/abandon_encounter_test.go`: directly stale test comment only.
- `internal/handlers/dnd5e/lobby/v1alpha1/handler_test.go`: directly stale mock-wiring comment only.
- `docs/architecture/components/lobby-service.md`, `docs/status.md`: provider-owned first-admission and truthful partial-write posture.
- `task-7-report.md`, `task-7-review-package.md`: evidence artifacts.

Anything else in `54de176..HEAD` is a scope blocker.

## Reviewer acceptance checklist

### Dependency and release boundary

- [ ] `go.mod` resolves root D&D `v0.126.2`, resolution `v0.29.0`, Session `v0.45.0`.
- [ ] No replace/workspace/local-toolkit override exists.
- [ ] No toolkit/proto/web/generated source is changed.

### Production boundary

- [ ] No source occurrence of `LoadFromData`, `events.NewEventBus`, `LongRest`, or `ToData` remains in `start_encounter_session_stack.go`.
- [ ] No API branch inspects HP, death saves, hit dice, spell slots, resources, features, or conditions.
- [ ] No API lifecycle helper, rest flag, event bus, or runtime character was added.
- [ ] `StartSession` still precedes all `Join` calls; all `Join` calls still precede `Spawn`; lobby/roster/publish behavior is unchanged.
- [ ] Existing dungeon-seat capacity validation was not expanded into character/rest preflight.

### Integration evidence

- [ ] Success acceptance travels through Lobby `StartEncounter`, real Session Manager, miniredis repositories, and production character adapter.
- [ ] Fighter/Barbarian assertions cover HP, death saves, exact half hit dice, spell slots, Second Wind, Rage Charges, passive retention, meter reset, temporary removal, metadata, and Appearance.
- [ ] Failure-order acceptance forces a real Manager's miniredis-backed Session/Encounter repository write to fail during StartSession and proves exact character bytes/version/save count unchanged.
- [ ] The failure-order test is discriminating: recorded RED on the old direct loop changed bytes/version and made one character update before StartSession returned its error.

### Failure semantics and docs

- [ ] Docs do not claim an API all-member preflight or rollback.
- [ ] Docs distinguish StartSession-before-Join failure from Session's intentional early character save during first-admission Join.
- [ ] Existing partial-launch/orphan risk remains visible rather than replaced with API rules.

## Validation evidence

Passed:

```text
go test ./internal/orchestrators/lobby -run 'TestSessionStackSuite/(TestStartEncounter_FirstAdmissionPersistsCompleteLongRestOutcomes|TestStartEncounter_StartSessionFailureLeavesCharacterUntouched)' -count=1 -v
go test ./internal/handlers/dnd5e/lobby/... ./internal/orchestrators/lobby ./internal/orchestrators/session ./internal/integration/harness ./internal/integration/session -count=1
go test -short ./... -count=1
./scripts/verify-release-pin.sh
make pre-commit
git diff --check
```

`make pre-commit` includes the repository's `go test -v -short -race` gate.

Known gate requiring reviewer adjudication:

```text
make ci-check
# all tests pass; command exits nonzero only because go generate rewrites the
# pre-existing generated-command comment in:
# internal/handlers/dnd5e/session/v1alpha1/mock/mock_manager.go
```

The exact unrelated generated-only delta is:

```diff
-// mockgen -destination=internal/handlers/dnd5e/session/v1alpha1/mock/mock_manager.go ...
+// mockgen -destination=mock/mock_manager.go ...
```

Task 7 does not change the corresponding generator directive or Manager interface. The generated mock was deliberately left untouched under the task's file authority.

## Worker review findings

- No production boundary, sequence, persistence-envelope, dependency-pin, or test-evidence blocker found in the staged diff.
- No scope widening found.
- Independent reviewer verdict remains the required merge gate; this package does not substitute the implementer's self-review for that verdict.
