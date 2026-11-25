# Run — 2026-11-26 — item story S? the API accepts a custom item

Slice [rpg-project#1172](https://github.com/KirkDiggler/rpg-project/issues/1172) of story [rpg-project#850](https://github.com/KirkDiggler/rpg-project/issues/850). Base: `rpg-api` `dev` at [`8c2e3719`](https://github.com/KirkDiggler/rpg-api/commit/8c2e3719f968c23c63e6b01f111e2b182079979a).

## Shipped

- The sheet maps a Custom inventory row with no catalog lookup, under a `site`/`items` ref, and a slot holding a custom item carries the same ref — [rpg-project#1172](https://github.com/KirkDiggler/rpg-project/issues/1172).
- The content check reports each refused custom item at `customItems.<id>` beside a refused status — [rpg-project#1172](https://github.com/KirkDiggler/rpg-project/issues/1172).
- Pins: rulebook v0.235.0, encounter v0.258.1, resolver v0.82.0, session v0.212.0; `go.mod` moves only those four requirement lines — [pins comment](https://github.com/KirkDiggler/rpg-project/issues/1172#issuecomment-7173981998).

## Decided

- The encounter pin is v0.258.1, not v0.258.0, so an empty item leaves nothing in a compiled site (the issue's Amendment 1) — [amendment](https://github.com/KirkDiggler/rpg-project/issues/1172#issuecomment-7174435147).
- A slot's ref is chosen by whether its id is a Custom row's id, never by the slot's name (the issue's Design 3) — [rpg-project#1172](https://github.com/KirkDiggler/rpg-project/issues/1172).

## Found

- F1 to F6 held at `8c2e3719`; the newest encounter tag was v0.258.1 after the fix and the newest session tag is v0.212.0 — [rests-on comment](https://github.com/KirkDiggler/rpg-project/issues/1172#issuecomment-7173981998).
- F7 printed: only `mapEquipment` looks a row id up in the catalog; the v1alpha1 converters, `view.go`, the vendor stock and sandboxseed read rows or slots with no lookup — [rests-on comment](https://github.com/KirkDiggler/rpg-project/issues/1172#issuecomment-7173981998).
- F8 did not hold at encounter v0.258.0: `placed_props.go:184` `Item *ItemGrant` had no omitempty, so `TestLastWeeksCastleCompilesAsItDid` failed on the pins alone with 67 added `"Item": null` lines — [gap](https://github.com/KirkDiggler/rpg-project/issues/1172#issuecomment-7174145331).
- On v0.258.1 the pins alone fail no test, and F8 holds with no edit to either guard or the golden — [amendment](https://github.com/KirkDiggler/rpg-project/issues/1172#issuecomment-7174435147).
- The two proofs bite: a catalog lookup for a Custom row fails the not-worn and worn cloak tests; dropping `checked.Items` fails the content-check test — [rpg-project#1172](https://github.com/KirkDiggler/rpg-project/issues/1172).
- No existing test was changed — [rpg-project#1172](https://github.com/KirkDiggler/rpg-project/issues/1172).
- Whole suite (`go test ./... -count=1`) green at the final head; lint gate is `make lint` (golangci-lint) — [rpg-project#1172](https://github.com/KirkDiggler/rpg-project/issues/1172).

## Mistaken

- F8 claimed the engine's new compiled fields were left out when empty; it had not been checked against the tag, and the gap was caught by running the whole suite rather than the three named packages — [gap](https://github.com/KirkDiggler/rpg-project/issues/1172#issuecomment-7174145331).
