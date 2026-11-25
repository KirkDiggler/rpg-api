# Run — 2026-11-15 — studio today S1 read-back on the check (api)

processed: 2026-11-25T190427Z

Slice [rpg-project#967](https://github.com/KirkDiggler/rpg-project/issues/967) of story [rpg-project#966](https://github.com/KirkDiggler/rpg-project/issues/966). Base: `rpg-api` `dev` at [`20af6ffd`](https://github.com/KirkDiggler/rpg-api/commit/20af6ffd).

## Shipped

- `PutDungeonResponse.read_back` carries the engine's read-back on a clean validate-only check, mapped field for field; none on a save, a refusal or a version 2 file — [rpg-api#1246](https://github.com/KirkDiggler/rpg-api/pull/1246).
- Protos pinned to `generated` at `affa8b8e` (tag `v0.1.278`); no toolkit bump — [rpg-api#1246](https://github.com/KirkDiggler/rpg-api/pull/1246).

## Decided

- The read-back is computed in `storeRegistry.Put` and returned on `PutResult`, not on `Entry`, because `compileEntry` also serves Get, launch, boot and Rename, none of which returns a read-back — [rpg-api#1246](https://github.com/KirkDiggler/rpg-api/pull/1246).
- The toolkit type rides up to the handler and is converted there (`read_back.go`), the same path the atlas takes — [rpg-api#1246](https://github.com/KirkDiggler/rpg-api/pull/1246).
- The read-back rides the validate-only check only, never a save (lead's ruling, reading R25 "carried on the check the studio sends after each edit"; the plan's "or a save" over-reached): the studio holds the read-back from the check before any save, and computing it again on each castle save took the four `TestSharing*` tests from 5.2–6.3 s on dev (run 43636107710) to 10.95–12.65 s (run 43696097575), over the 10 s hard line — [rpg-api#1246](https://github.com/KirkDiggler/rpg-api/pull/1246).
- A read-back failure or panic after a clean compile is logged and answered with no read-back, never a refusal, and a written save stays a success; an unexported seam (`export_test.go`) reaches both branches, because the real read-back never fails on a file that compiled — [review](https://github.com/KirkDiggler/rpg-api/pull/1246#discussion_r4577682955), [08b33ec0](https://github.com/KirkDiggler/rpg-api/commit/08b33ec03e3d57a74f71f76aca38241cbed4edfc).
- The mapping is guarded by comparing the toolkit's JSON with the wire message's protojson (proto names) on the small site; blanking `Section.Title` or `Group.About` fails it — [review](https://github.com/KirkDiggler/rpg-api/pull/1246#discussion_r4577682960).
- A fourth test, `TestPutDungeon_Save_CarriesNoReadBack`, pins that a clean save is stored and carries no read-back while the same file's check carries one — [rpg-api#1246](https://github.com/KirkDiggler/rpg-api/pull/1246).

## Found

- `readback.Of` on the castle costs a median 159 ms in-process; the whole validate-only check is 278 ms with it, about 119 ms without (derived) — [rpg-api#1246](https://github.com/KirkDiggler/rpg-api/pull/1246).
- The castle's encoded check is 415,124 bytes with the read-back and 83,138 without; gzip 46,812 and 12,068 — [rpg-api#1246](https://github.com/KirkDiggler/rpg-api/pull/1246).
- The castle's read-back has 642 statements and nothing in `not_yet_said` — [rpg-api#1246](https://github.com/KirkDiggler/rpg-api/pull/1246).
- A suite that calls `dungeonstest.ShippedStore` first in its binary pays about 7 s under race to compile the whole shelf; a one-file shelf with a small site standing in for the default costs well under a second — [rpg-api#1246](https://github.com/KirkDiggler/rpg-api/pull/1246).

## Mistaken

- I attributed the red CI run at e5fd5bea to existing dev tests (`TestSharing*` over 10 s); it was this PR: each of those tests validates then saves the castle, and the save's read-back doubled their time; nothing compared their CI times with dev's before the claim — [CI run 43696097575](https://github.com/KirkDiggler/rpg-api/actions/runs/43696097575).
- The first PR head let a panic in `readback.Of` escape, after the write on a save, and guarded the mapping with four spot checks a blanked title passed; one review round found both — [review](https://github.com/KirkDiggler/rpg-api/pull/1246#pullrequestreview-5909868827).
- The first version of the suite used the whole shared shelf and broke the castle for the refusal probe, 10 s under race against a 5 s budget; it was cut to 3 s before the PR opened — [rpg-api#1246](https://github.com/KirkDiggler/rpg-api/pull/1246).
