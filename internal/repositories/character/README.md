# Character Repository

The Redis adapter stores `entities.Character`, a wrapper around canonical toolkit
`character.Data`. It does not load a playable character or calculate equipment effects.

## Current contract

- `Create` requires a supplied ID, refuses an existing record, and adds the player index
  when PlayerID is nonempty. Characters have no TTL.
- `Get` returns a deserialized record plus an opaque version of its stored bytes.
- `Update` replaces an existing record and moves/removes/adds its **player** index when
  PlayerID changes. It is a full-record update, not a compare-and-swap operation.
- `Delete` removes an existing record and its player-index membership.
- `ListByPlayerID` and `ListBySessionID` resolve members from their respective Redis sets.
  Missing record IDs are lazily removed; malformed records return an error. List order
  is not promised.
- `PatchEquipment` changes only supplied EquipmentSlots and cached ArmorClass. WATCH
  guards the transaction. Stale equipment returns ABORTED; a changed version with the
  same equipment returns the latest record with Applied=false so the caller can
  reproject; success returns the patched record/version. No AC calculation occurs here.

## Redis keys

```text
character:{id}                 JSON record, no TTL
character:player:{playerID}     maintained set of character IDs
character:session:{sessionID}   legacy read-side set of character IDs
```

**Correction to the historical README:** character CRUD does not write or migrate
session-index membership. The former SessionID example and claim of automatic session
index maintenance did not describe the current adapter. Tests seed that index explicitly
when exercising its existing list/cleanup path, rather than inventing a writer.

## Evidence

`redis_equipment_test.go` retains the non-equipment-revision/reprojection and
stale-equipment refusal regressions. `redis_contract_test.go` adds populated persistence,
player-index lifecycle, detached reads, successful equipment-patch preservation,
validation, malformed data, and bounded storage failures using miniredis.

See [the method inventory](../../../docs/quality/repository-contracts.md) for exact
coverage, known limits, and #141 reconciliation. In particular, these tests do not
establish atomic create-if-absent under concurrent writers, arbitrary transaction
rollback, or real concurrent WATCH retry/exhaustion behavior.
