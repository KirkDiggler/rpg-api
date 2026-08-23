---
name: authoring service + dungeon content registry
description: internal/dungeons (the file registry every authored dungeon lives in) and AuthoringService v1alpha1 (PutDungeon / GetDungeon) on the session stack
updated: 2026-08-23
confidence: medium-high — rpg-api#806 branch on toolkit T1/T2/T3 pseudo-versions (encounter b955743, session bec5ff2); registry, handler and lobby contracts verified by passing unit + integration suites under -race; no browser walk yet
---

# authoring service + dungeon content registry

The dungeon builder's seam (rpg-project#256 under journey #169, design in
`rpg-project/ideas/dungeon-builder/design.md`): write a world file, get back
the world the game will play. Two pieces, both thin:

```
internal/dungeons/                           the content registry (FileRegistry)
internal/orchestrators/authoring/            PutDungeon / GetDungeon business logic
internal/handlers/dnd5e/authoring/v1alpha1/  proto <-> input, status mapping
content/reference-tomb.yaml                  the shipped dungeon
```

## The registry (`internal/dungeons`)

`NewFileRegistry(dir, authoring, projector)` reads every `*.yaml` under `dir`
(`RPG_CONTENT_DIR`, default `./content`) and compiles each once through
`internal/sessionworld.Compile` — the toolkit's
`rulebooks/dnd5e/encounter/dungeonspec`, never a local copy of its geometry.
Construction **fails** (naming the file) when a file does not compile, when a
file's `key:` line disagrees with its filename, when two files claim one key,
or when `reference-tomb` is absent. A dungeon that silently vanished from the
picker would be a worse failure than a server that refuses to boot and says
why. Each entry's `Atlas` comes from the `AtlasProjector` — production wires
`session.Manager.AtlasOf` (the same validation-load path as `StartSession`
and the same projection `Manager.Atlas` uses), so `PutDungeon`'s atlas and
the started game's `GetAtlas` have one producer. A world that compiled but
will not load is a boot refusal / `Internal`, never a `FieldError`: the
stack disagreed with itself, the author did nothing.

`Registry` is the interface the lobby and authoring orchestrators see:

- `List` — every `{Key, Name}`, sorted by key.
- `Get(key)` — the `Entry` (`Key`, `Name`, verbatim `YAML`, compiled
  `Dungeon`, `Atlas`) or `ErrNotFound`.
- `Put(PutInput{Key, YAML, ValidateOnly})` — compile → (unless validate-only)
  write `dir/<key>.yaml` through a same-directory temp file + rename → swap
  the entry under the registry mutex. Puts are serialized **per key**
  (different keys do not contend). A file that does not compile is a
  `PutResult.Errors` list, not an error. `ErrInvalidKey` (outside
  `[a-z0-9-]`), `ErrKeyMismatch` (request key ≠ file's `key:`), and
  `ErrAuthoringDisabled` (registry constructed read-only) are errors.

The registry never re-marshals: `GetDungeon` hands back the bytes that were
`Put`, comments and spacing included (the builder's byte-identity round trip,
design §5).

`internal/dungeons/dungeonstest` builds registries for tests: `Shipped(t)`
(read-only over the repo's `content/`) and `Scratch(t)` (a temp copy with
authoring on).

## The gate

`cmd/server` constructs the registry **unconditionally** — the tomb must load
with authoring off because `StartEncounter` with no `dungeon_key` plays it.
`AuthoringService` is registered only when `RPG_AUTHORING_ENABLED=1`, which
then requires `RPG_CONTENT_DIR` to be set explicitly (writing into an
implicit `./content` is not something to do by accident). With the gate off a
client sees `Unimplemented` for both RPCs — the proto's documented way to
tell "authoring is off" from "server unreachable"; the web's Home-button probe
is `GetDungeon("reference-tomb")`.

| Variable | Effect |
|---|---|
| `RPG_CONTENT_DIR` | directory the registry loads and writes; default `content` (the Docker image ships the repo's `content/` at `/home/appuser/content`) |
| `RPG_AUTHORING_ENABLED=1` | registers `AuthoringService`; requires `RPG_CONTENT_DIR` |

## Transport rules (the proto's, enforced in the handler)

- A request that cannot name its target — empty key, bad charset, key/file
  mismatch — is a gRPC `InvalidArgument`, no body.
- A well-formed request whose file does not compile is **OK** with
  `errors` populated and `atlas` unset. `validate_only` never refuses a
  half-drawn map.
- `errors` empty ⇒ compiled; `atlas` set (see the gap below); stored unless
  `validate_only`.
- `GetDungeon` unknown key → `NotFound`.
- Both RPCs require an authenticated caller; no per-player ownership exists on
  a dungeon yet (rpg-api#803 tracks verb authorization generally).

`PutDungeonResponse.atlas` is produced by the **same** `AtlasToProto` the
session handler's `GetAtlas` uses (exported for exactly this reason): one
producer of the wire atlas, so the builder has no second geometry to keep in
step with the game.

## Toolkit pins

Built on plan items T1/T2 (`rpg-toolkit` `feat/256-regions-dungeonspec-v2`,
encounter `v0.30.9-0.20260823055203-b955743ff1a3`) and T3
(`feat/256-atlas-regions`, session `v0.21.12-0.20260823060107-bec5ff238f27`)
as pseudo-versions; the merge re-pins to their tags. What they gave this
component: dungeonspec **version 2** (regions, walls, doors, absolute
`place`; `Validate` returns every defect with its YAML path, `Decode`
defects carry `line N`), `Seat.At`/`MonsterPlacement.At` as absolute offset
cells (so `sessionworld` asks `encounter.HexCellAt` for the axial cell and
the throwaway-encounter projection is gone), `session.Atlas.Regions`, and
`Manager.AtlasOf`. `content/reference-tomb.yaml` is T2's golden v2 file —
the same 224 cells, boundaries, doorways and props version 1 produced, with
three regions added.

## Tests that gate

- `internal/dungeons/registry_test.go` — boot refuses a non-compiling file
  naming it; name/key mismatch refused; default dungeon required; validate-only
  never writes; Put then Get returns bytes unchanged (and a fresh registry over
  the directory sees them); compile failure is a body; key mismatch /
  charset / authoring-off refusals; overwrite leaves no temp files; 16
  concurrent Puts on one key serialize under `-race` and the final file is one
  writer's file whole.
- `internal/handlers/dnd5e/authoring/v1alpha1/handler_test.go` — the transport
  rules above over a mocked registry.
- `internal/orchestrators/lobby/start_encounter_session_stack_test.go` —
  unknown `dungeon_key` refused before any write; explicit default key is the
  tomb; a dungeon that arrived through `Put` starts, its `GetAtlas` carries
  its regions and is **cell-for-cell the atlas `Put` answered** (one
  producer), with doors minted under the authored key; `ListDungeons` reads
  the registry.
- `internal/integration/session/acceptance_test.go` — the session acceptance
  loop re-authored on regions (absolute offset cells, declared seam walls,
  open doors on the edges they leave out, `encounter.HexCellAt` for every
  cell the verbs speak).

## Related

- [`lobby-service.md`](./lobby-service.md) — `dungeon_key` routing and `ListDungeons`.
- [`authoring.md`](./authoring.md) — the deleted old-dialect predecessor (history).
- `rpg-project/ideas/dungeon-builder/{design,plan}.md` — design and the per-module plan.
