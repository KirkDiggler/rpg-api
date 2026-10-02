# Character Repository

The Redis adapter stores `entities.Character`, a wrapper around canonical toolkit
`character.Data` plus API-owned `WorldID` ownership metadata. It does not load a
playable character or calculate equipment effects.

## Current contract

Every operation is world-scoped. `WorldID` is mandatory and is never inferred from
context inside the adapter: callers pass the trusted world they already resolved
(read/delete/patch carry it explicitly; create/update take it from the wrapper's
`WorldID`).

- `Create` requires a supplied ID and non-empty `WorldID`, refuses an existing
  record in the **same world**, and adds the world-scoped player index when
  PlayerID is nonempty. The same ID may exist independently in another world.
  Characters have no TTL.
- `Get` returns a deserialized record plus an opaque version of its stored bytes.
  A missing world-scoped key is `NotFound`; an empty world is `InvalidArgument`.
- `Update` replaces an existing record. **World and player ownership are immutable:**
  an update that changes the stored PlayerID (including clearing it or assigning it
  for the first time) is rejected with `InvalidArgument` and writes nothing. Because
  ownership cannot move, the player index is never rewritten by Update. It is a
  full-record update, not a compare-and-swap operation.
- `Delete` removes an existing record and its world-scoped player-index membership.
- `ListByPlayerID` and `ListBySessionID` resolve members from their respective
  world-scoped Redis sets. Missing record IDs are lazily removed; malformed records
  return an error. A resolved record whose stored world disagrees with the key, or
  (for the player list) whose player disagrees with the index, is reported as
  `Internal` storage corruption instead of projected — and the poisoning index
  member is deliberately **not** deleted. List order is not promised.
- `PatchEquipment` changes only supplied EquipmentSlots and cached ArmorClass. WATCH
  guards the transaction. Stale equipment returns ABORTED; a changed version with the
  same equipment returns the latest record with Applied=false so the caller can
  reproject; success returns the patched record/version. No AC calculation occurs here.
  Ownership fields are never touched.

## Ownership validation

`Get` (and therefore `Update`/`Delete`/`PatchEquipment`/index resolution, which read
through it) validates the stored envelope before projecting it:

- `nil` toolkit `Data` is `Internal` corruption, not an empty success or a panic.
- stored `WorldID` different from the requested world is `Internal` corruption.

## Redis keys

World IDs are canonical decimal guild-shaped identifiers; record, player and session
IDs never contain the `:` delimiter, so each tuple is unambiguous.

```text
character:{worldID}:{id}                    JSON record, no TTL
character:player:{worldID}:{playerID}       maintained set of character IDs
character:session:{worldID}:{sessionID}     read-side set of character IDs
```

**Legacy keys are not read.** The former ownerless `character:{id}` and
`character:player:{playerID}` keys are neither migrated nor used as a fallback on a
world-scoped miss; they are ignored until the authorized cutover.

**Correction to the historical README:** character CRUD does not write or migrate
session-index membership. The former SessionID example and claim of automatic session
index maintenance did not describe this adapter. Tests seed that index explicitly
when exercising its existing list/cleanup path, rather than inventing a writer.

## Evidence

`redis_equipment_test.go` retains the non-equipment-revision/reprojection and
stale-equipment refusal regressions. `redis_contract_test.go` adds populated
persistence, player-index lifecycle, detached reads, successful equipment-patch
preservation, validation, malformed data, and bounded storage failures using
miniredis. `world_test.go` adds same-player A/B list isolation, foreign-world direct
operation refusals, immutable ownership, legacy-key non-fallback, poisoned
index/ownership corruption, and byte-for-byte foreign-record preservation.

See [the method inventory](../../../docs/quality/repository-contracts.md) for exact
coverage, known limits, and #141 reconciliation. In particular, these tests do not
establish atomic create-if-absent under concurrent writers, arbitrary transaction
rollback, or real concurrent WATCH retry/exhaustion behavior.

### Equipment-bound conditions

`PatchEquipmentInput.Conditions` optionally carries the toolkit's post-equipment
condition blobs. The same full-record expected version guards slots, derived AC
and conditions together. A stale version must be reprojected; a nil conditions
pointer preserves them, while a present empty slice explicitly clears them.
No HP, resource or action-economy field is written by this patch.
