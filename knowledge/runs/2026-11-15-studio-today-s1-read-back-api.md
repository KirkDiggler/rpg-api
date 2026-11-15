# Run — 2026-11-15 — studio today S1 read-back on the check (api)

Slice [rpg-project#967](https://github.com/KirkDiggler/rpg-project/issues/967) of story [rpg-project#966](https://github.com/KirkDiggler/rpg-project/issues/966). Base: `rpg-api` `dev` at [`20af6ffd`](https://github.com/KirkDiggler/rpg-api/commit/20af6ffd).

## Shipped

- `PutDungeonResponse.read_back` carries the engine's read-back on a clean compile, validate-only or save, mapped field for field; none on a refusal or a version 2 file — [rpg-api#1246](https://github.com/KirkDiggler/rpg-api/pull/1246).
- Protos pinned to `generated` at `affa8b8e` (tag `v0.1.278`); no toolkit bump — [rpg-api#1246](https://github.com/KirkDiggler/rpg-api/pull/1246).

## Decided

- The read-back is computed in `storeRegistry.Put` and returned on `PutResult`, not on `Entry`, because `compileEntry` also serves Get, launch, boot and Rename, none of which returns a read-back — [rpg-api#1246](https://github.com/KirkDiggler/rpg-api/pull/1246).
- The toolkit type rides up to the handler and is converted there (`read_back.go`), the same path the atlas takes — [rpg-api#1246](https://github.com/KirkDiggler/rpg-api/pull/1246).
- On a save the read-back is computed after the write succeeds, so a refused write (authoring off, owned by another) pays nothing for it — [rpg-api#1246](https://github.com/KirkDiggler/rpg-api/pull/1246).
- A read-back failure after a clean compile is a `slog.WarnContext` line and no read-back, never a refusal (plan Task 2 behaviour); it has no test because only a test seam could reach it — [rpg-api#1246](https://github.com/KirkDiggler/rpg-api/pull/1246).
- A fourth test, `TestPutDungeon_Save_CarriesReadBack`, covers the save half the plan's behaviour names and its three tests do not — [rpg-api#1246](https://github.com/KirkDiggler/rpg-api/pull/1246).

## Found

- `readback.Of` on the castle costs a median 159 ms in-process; the whole validate-only check is 278 ms with it, about 119 ms without (derived) — [rpg-api#1246](https://github.com/KirkDiggler/rpg-api/pull/1246).
- The castle's encoded check is 415,124 bytes with the read-back and 83,138 without; gzip 46,812 and 12,068 — [rpg-api#1246](https://github.com/KirkDiggler/rpg-api/pull/1246).
- The castle's read-back has 642 statements and nothing in `not_yet_said` — [rpg-api#1246](https://github.com/KirkDiggler/rpg-api/pull/1246).
- A suite that calls `dungeonstest.ShippedStore` first in its binary pays about 7 s under race to compile the whole shelf; a one-file shelf with a small site standing in for the default costs well under a second — [rpg-api#1246](https://github.com/KirkDiggler/rpg-api/pull/1246).

## Mistaken

- The first version of the suite used the whole shared shelf and broke the castle for the refusal probe, 10 s under race against a 5 s budget; it was cut to 3 s before the PR opened — [rpg-api#1246](https://github.com/KirkDiggler/rpg-api/pull/1246).
