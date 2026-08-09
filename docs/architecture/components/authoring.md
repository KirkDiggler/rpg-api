---
name: authoring durability
description: PutDungeon per-key transaction ordering and authored-source crash guarantees
updated: 2026-08-09
confidence: high — fault injection, race stress, and production-loader restart tests
---

# Authoring durability

`PutDungeon` is a thin consumer of `dungeonspec`: rpg-api passes each complete
source standalone to `CompileDungeon`, maps the provider `FloorPlan` and exact
field errors, and owns only persistence plus live-registry orchestration. It does
not interpret geometry, shrink meaning, topology, or rules.

## Per-key transaction

Every call, including `validate_only`, takes the dungeon key's lock before
standalone candidate compilation. A mutating call holds that lock through:

1. one toolkit `CompileDungeon` call over the complete source;
2. durable source replacement; and
3. live registry replacement.

Thus two calls for one key have one linear order without passing prior compiled
state into either candidate. Locks are per key and reference-counted; unrelated
keys proceed concurrently and unused lock entries are removed.

## Filesystem and crash contract

The supported deployment contract is Linux/POSIX storage implementing atomic
same-directory `rename(2)`, file `fsync(2)`, and directory `fsync(2)` semantics.
The update writes a hidden same-directory temporary file, applies mode `0600`,
checks the full write, syncs and closes the temporary file, opens the containing
directory, renames the temporary file over the live YAML, and syncs the
directory before replacing the registry or acknowledging success. Temporary
names do not end in `.yaml`/`.yml`, so the production loader never indexes an
orphan left by a process crash.

The guarantees are deliberately stated by crash point rather than called simply
“atomic”:

- Before rename (create/chmod/write/short-write/file-sync/close/open-directory or
  rename failure): the original pathname and registry entry remain unchanged;
  normal error cleanup removes the temporary file.
- Crash after the temporary-file sync and before rename: the original pathname
  remains; an unindexed temporary file may remain.
- Crash after rename but before the directory sync completes: restart may see
  either the complete prior file or the complete candidate file, never torn or
  mixed bytes. No success has been acknowledged yet.
- After directory sync succeeds: the candidate pathname is durable. Only then is
  the in-memory registry replaced and the RPC allowed to succeed.

A directory-sync error after a successful rename is necessarily indeterminate:
the RPC fails and the registry is not advanced, but the current pathname may
already expose the complete candidate. The code does not claim rollback after
the rename commit point; retrying the authored update or restarting through the
production loader reconciles from durable source. This exceptional storage
failure is distinct from the injected pre-rename failures above.

On process restart, `lobby.LoadContentRegistry` reads `RPG_CONTENT_DIR` and
recompiles the committed YAML. The live registry is therefore a cache of the
durable authored source, not a separate source of truth.
