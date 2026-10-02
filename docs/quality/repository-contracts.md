# Repository contract coverage (#1047)

Baseline: `origin/dev` at `120e7e83` (includes lobby pilot #1052), then the
world-owned-character storage slice (#522 S1).
This is a method-level inventory, not a coverage-percentage or all-repositories claim.
All new fixtures are hand-authored records using canonical toolkit data types. No
character loader, combat, spell, equipment projection, dice roller, or dungeon compiler
runs to produce expected values. Arbitrary supplied AC and roll totals are preserved,
not recalculated. Tests exercise the real adapters with miniredis.

## Method inventory

| Adapter / methods | Evidence in tests | Explicit limits |
| --- | --- | --- |
| Character `Create`, `Get` | Populated record equality (resources, conditions, economy, equipment, optional appearance), stable version, duplicate refusal without overwrite/reindex, detached nested reads, no TTL, world-scoped keys, empty-world rejection, foreign-world miss | JSON representation follows canonical tags, not a new universal nil/empty guarantee |
| Character `Update`, `Delete` | Read-after-write equality, changed version, immutable player ownership (clearing/assigning a different player rejected without writing), world-scoped player-index membership removal (raw set checked immediately after Delete, before lazy list cleanup), neighbor preservation, missing/invalid records | General Update remains full-record last-writer behavior, not optimistic concurrency; null-Data stored envelopes are now INTERNAL, not a panic (#1057) |
| Character `ListByPlayerID`, `ListBySessionID` | World/index isolation, record resolution, missing-index empty result, stale-ID cleanup, decode failure propagation, detached listed records, poisoned index (foreign player or foreign world) reported as INTERNAL without projection or deletion | Session index is seeded directly: character CRUD does **not** maintain it; no membership writer is invented here |
| Character `PatchEquipment` | Successful patch changes only supplied EquipmentSlots/ArmorClass, full other-data equality, returned/stored version agreement, caller mutation isolation, missing/invalid/syntactically malformed record and transaction errors; INTERNAL refusal of null Data or foreign stored world without writing | Existing `redis_equipment_test.go` retains unrelated-revision/no-write/retry and stale-equipment ABORTED cases; real concurrent WATCH invalidation/retry exhaustion is not newly proved |
| Draft `Create`, `Get`, `GetByPlayerID` | Populated data/choices/ability scores, generated or supplied ID, world+player replacement deletes old record, another player's or another world's record survives, detached reads, missing/malformed records, mapping that resolves to a foreign player reported as INTERNAL | Existing `redis_appearance_test.go` retains full appearance and present-zero optional-pointer evidence; supplied-ID collisions within one world still overwrite a record (#1058) |
| Draft `Update`, `Delete` | Same-owner update persists and refreshes 24-hour TTL; reads do not refresh; expiry makes record missing; GetByPlayerID lazily clears stale mapping; delete clears record/mapping without touching neighbor; player/world ownership immutable | Owner reassignment is rejected, not supported by mapping maintenance; player mapping itself has no TTL |
| Dice `Create`, `Get` | Supplied roll arrays/totals/metadata, nil versus empty dropped lists, timestamps, detached reads, world/entity/context isolation, default/custom Redis expiry, application-clock expiry cleanup, decode/read errors, foreign-stored-world INTERNAL refusal | No notation parsing or roll legality. Delimiter-bearing key components are not covered; no encoding/validation redesign |
| Dice `Update`, `Delete` | Full replacement on an existing key, remaining rather than restarted lifetime, past-expiry refusal, refusal to overwrite a stored envelope with contradictory ownership, deletion count after a successful count-read and idempotent missing deletion, foreign-world delete does not touch the other world's key, write failures | Exact-deadline resurrection defect is **open #1055**, not blessed by a green test; no claim that Update refuses a missing key or validates roll immutability; Delete's count is best-effort (a failed pre-count Get followed by successful DEL returns zero, not a count error) |
| Session `SaveSession`, `GetSession` | Populated key/cursors/NPC sheet/window payload round trip, detached nested reads, overwrite/removal of fields, separate IDs and encounter namespace, configurable TTL refresh and zero-TTL permanence, invalid save, malformed JSON and storage errors | SDK structural/rule validation is deliberately not performed; this is serialization, not a playable-session acceptance test |
| Encounter `SaveEncounter`, `GetEncounter` | Populated clock maps, members/cells, scenery, outcome, retention and ever-members; zero position pointer versus absent; overwrite, independent keys, detached reads, TTL/expiry, invalid save, marshal/decode errors, storage errors | Representative populated fields, not a promise that every future toolkit field is individually populated; provider loading/game rules stay in toolkit |

Test locations:
- `internal/repositories/character/redis_contract_test.go` — `CharacterRedisContractSuite`
- `internal/repositories/character/world_test.go` — `TestSamePlayerSeparateWorldLists`, `TestForeignWorldDirectOperationsDoNotWrite`, `TestOwnershipCannotMove`, `TestLegacyKeysAreNotFallback`, `TestCorruptOwnershipDoesNotLeak`
- `internal/repositories/character_draft/redis_contract_test.go` — `DraftRedisContractSuite`
- `internal/repositories/character_draft/world_test.go` — `TestDraftReplacementStaysInWorld`, `TestForeignWorldDirectOperationsDoNotWrite`, `TestDraftOwnershipCannotMove`, `TestDraftLegacyKeysAreNotFallback`, `TestDraftCorruptOwnershipDoesNotLeak`
- `internal/repositories/dice_session/redis_contract_test.go` — `DiceRedisContractSuite`
- `internal/repositories/dice_session/world_test.go` — `TestAbilityRollsStayInWorld`, `TestDiceForeignWorldDeleteDoesNotTouchOtherWorld`, `TestDiceLegacyKeysAreNotFallback`, `TestDiceCorruptOwnershipDoesNotLeak`
- `internal/orchestrators/session/redis_repos{,_contract}_test.go` — `RedisReposTestSuite`

Malformed-payload tests primarily cover invalid JSON syntax; semantic envelope
validation is not generally guaranteed (see #1057 below). Missing character/draft/dice
records keep API not-found classification. Session and
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

## Review corrections and newly recorded gaps

The independent reviewer found Delete's player-index assertion could pass even without
Delete's SRem: ListByPlayerID lazily repaired the missing write before asserting the
result. The test now reads the raw set immediately after Delete, before listing. The
parent repeated the missing-SRem overlay: the old suite passed, and the corrected test
fails with the stale `char-a` member. This is a correction to the evidence, not a
production fix.

[rpg-api#1057](https://github.com/KirkDiggler/rpg-api/issues/1057) records the separate
semantic corruption gap: store `character:char-a = {"data":null}`. The world-owned
storage slice (#522 S1) now rejects that envelope as INTERNAL from the shared decode
path used by Get/Update/Delete/PatchEquipment/index resolution, so Update no longer
panics at `redis.go` reading `existing.Data.PlayerID`. No claim is made that all
syntactically valid corrupt records are rejected.

[rpg-api#1058](https://github.com/KirkDiggler/rpg-api/issues/1058) records the draft
supplied-ID collision: create the same supplied ID for owner-a and then owner-b in
**one world**; both player mappings resolve to owner-b's overwritten record. The
isolation tests use **distinct** IDs and separate worlds; they do not prove in-world
collision safety. Generated IDs normally avoid this case, but the repository also
accepts supplied IDs; its policy needs a separate production decision.

Dice Delete's RollsDeleted is best-effort: if Get fails while counting (including a
malformed stored payload), successful DEL still returns success with zero. Only DEL's
own failure is propagated. The tests of successful count reads do not prove an exact
count when that read fails. This behavior is documented, not redesigned here.

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
