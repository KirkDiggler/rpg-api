# Repository contract coverage (#1047)

Baseline: `origin/dev` at `120e7e83` (includes lobby pilot #1052).
This is a method-level inventory, not a coverage-percentage or all-repositories claim.
All new fixtures are hand-authored records using canonical toolkit data types. No
character loader, combat, spell, equipment projection, dice roller, or dungeon compiler
runs to produce expected values. Arbitrary supplied AC and roll totals are preserved,
not recalculated. Tests exercise the real adapters with miniredis.

## Method inventory

| Adapter / methods | Evidence in tests | Explicit limits |
| --- | --- | --- |
| Character `Create`, `Get` | Populated record equality (resources, conditions, economy, equipment, optional appearance), stable version, duplicate refusal without overwrite/reindex, detached nested reads, no TTL | JSON representation follows canonical tags, not a new universal nil/empty guarantee |
| Character `Update`, `Delete` | Read-after-write equality, changed version, player-index movement/removal/assignment, neighbor preservation, missing/invalid records | General Update remains full-record last-writer behavior, not optimistic concurrency |
| Character `ListByPlayerID`, `ListBySessionID` | Index-key isolation, record resolution, missing-index empty result, stale-ID cleanup, decode failure propagation, detached listed records | Session index is seeded directly: character CRUD does **not** maintain it; no membership writer is invented here |
| Character `PatchEquipment` | Successful patch changes only supplied EquipmentSlots/ArmorClass, full other-data equality, returned/stored version agreement, caller mutation isolation, missing/invalid/malformed record and transaction errors | Existing `redis_equipment_test.go` retains unrelated-revision/no-write/retry and stale-equipment ABORTED cases; real concurrent WATCH invalidation/retry exhaustion is not newly proved |
| Draft `Create`, `Get`, `GetByPlayerID` | Populated data/choices/ability scores, generated or supplied ID, player replacement deletes old record, another player's record survives, detached reads, missing/malformed records | Existing `redis_appearance_test.go` retains full appearance and present-zero optional-pointer evidence |
| Draft `Update`, `Delete` | Same-owner update persists and refreshes 24-hour TTL; reads do not refresh; expiry makes record missing; GetByPlayerID lazily clears stale mapping; delete clears record/mapping without touching neighbor | Owner reassignment is not supported by mapping maintenance in Update and is not asserted as a feature; player mapping itself has no TTL |
| Dice `Create`, `Get` | Supplied roll arrays/totals/metadata, nil versus empty dropped lists, timestamps, detached reads, entity/context isolation for ordinary IDs, default/custom Redis expiry, application-clock expiry cleanup, decode/read errors | No notation parsing or roll legality. Delimiter-bearing key components are not covered; no encoding/validation redesign |
| Dice `Update`, `Delete` | Full replacement on an existing key, remaining rather than restarted lifetime, past-expiry refusal, deletion count and idempotent missing deletion, write failures | Exact-deadline resurrection defect is **open #1055**, not blessed by a green test; no claim that Update refuses a missing key or validates roll immutability |
| Session `SaveSession`, `GetSession` | Populated key/cursors/NPC sheet/window payload round trip, detached nested reads, overwrite/removal of fields, separate IDs and encounter namespace, configurable TTL refresh and zero-TTL permanence, invalid save, malformed JSON and storage errors | SDK structural/rule validation is deliberately not performed; this is serialization, not a playable-session acceptance test |
| Encounter `SaveEncounter`, `GetEncounter` | Populated clock maps, members/cells, scenery, outcome, retention and ever-members; zero position pointer versus absent; overwrite, independent keys, detached reads, TTL/expiry, invalid save, marshal/decode errors, storage errors | Representative populated fields, not a promise that every future toolkit field is individually populated; provider loading/game rules stay in toolkit |

Test locations:
- `internal/repositories/character/redis_contract_test.go` — `CharacterRedisContractSuite`
- `internal/repositories/character_draft/redis_contract_test.go` — `DraftRedisContractSuite`
- `internal/repositories/dice_session/redis_contract_test.go` — `DiceRedisContractSuite`
- `internal/orchestrators/session/redis_repos{,_contract}_test.go` — `RedisReposTestSuite`

Missing character/draft/dice records keep API not-found classification. Session and
encounter misses retain `errors.Is(err, sdk.ErrNotFound)`. Syntax and injected storage
errors retain their underlying cause and are not mislabeled missing. Fault hooks fail
only a Redis write boundary, after preparatory reads have succeeded. Their unchanged
record checks prove pre-execution failures do not claim success; they do **not** claim
Redis rolls back arbitrary partially executed transactions. Closed-client tests cover
read-side failures without network retry sleeps. Clocks use miniredis FastForward and,
for dice, an independently controlled application clock.

## Verification and test sensitivity

Focused command (also run with `-race`):

```sh
GOWORK=off go test ./internal/repositories/character ./internal/repositories/character_draft ./internal/repositories/dice_session ./internal/orchestrators/session -count=1
```

Six temporary Go-overlay mutations each produced assertion failures in the new or
strengthened tests: erase resources during equipment patch, omit old-player-index
removal, fail to refresh the draft's full lifetime, restart rather than preserve dice
remaining TTL, drop nested session fields, and drop encounter fields. Overlays did not
change tracked production files. Applicable readiness gates remain `make pre-commit`
and `make ci-check`; this document does not substitute for their PR evidence.

## Known gap found by the expiry probe

[rpg-api#1055](https://github.com/KirkDiggler/rpg-api/issues/1055) records a failing
controlled-clock probe: Create for one minute; retain the record; move the application
clock exactly to ExpiresAt and expire the Redis key; Update returns nil and recreates
the key with TTL 0. Update's strict `After` check lets equality become Redis's
no-expiration SET. This tests-only change neither fixes it nor commits a passing test
that ratifies it. Negative TTL policy and the Get equality boundary remain part of
that follow-up's decision. Past-expiry rejection and normal remaining-TTL tests do not
establish correctness at equality.

## Historical #141 reconciliation

The `redis_test.go`, `redis_toolkit_test.go`, and `integration_test.go` named in #141
are absent from the current character repository at the baseline. There is no disabled
file to re-enable. Its surviving equipment-storage concern is now covered by the two
existing conflict tests plus the successful populated patch, validation, missing,
malformed, and write-failure contracts above. This does not prove old deleted cases
were all duplicated; it inventories current obligations instead. #141 is linked for
human disposition, not automatically closed by #1047.

No integration/gameplay test is removed in this slice. #1049 must still map each
assertion to its owner before deleting the old playthrough-based evidence.
