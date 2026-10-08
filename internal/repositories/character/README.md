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
- No armour class is stored: an old record carrying `armor_class` loads with the key
  ignored and drops it on the next write (rpg-project#538 R13).
- Equipment has no write of its own. The session SDK's Equip/Unequip verbs change a sheet
  and save it through `Update` (via the session orchestrator's `CharacterRepository`
  adapter) under the guard the character's seat decides (rpg-project#542), so the former
  `PatchEquipment` compare-and-swap, its version check and its retry loop are gone.

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

`redis_contract_test.go` covers populated persistence, player-index lifecycle, detached
reads, validation, malformed data, and bounded storage failures using miniredis.

See [the method inventory](../../../docs/quality/repository-contracts.md) for exact
coverage, known limits, and #141 reconciliation. In particular, these tests do not
establish atomic create-if-absent under concurrent writers, arbitrary transaction
rollback, or real concurrent WATCH retry/exhaustion behavior.
