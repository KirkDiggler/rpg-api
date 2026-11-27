# Run — 2026-11-26 — Item story S-words the api

Slice [rpg-project#1190](https://github.com/KirkDiggler/rpg-project/issues/1190) of story [rpg-project#850](https://github.com/KirkDiggler/rpg-project/issues/850). Base: `rpg-api` `dev` at [`6f13cf71`](https://github.com/KirkDiggler/rpg-api/commit/6f13cf71).

## Shipped

- `GetItemWords` served: orchestrator method, handler copying every field with `With` recursive, role policy entry at Build — [rpg-project#1190](https://github.com/KirkDiggler/rpg-project/issues/1190).
- Pins moved: `rulebooks/dnd5e` v0.236.0 to v0.237.0 and `rpg-api-protos/gen/go` to 645a10bfa373; encounter, resolution and session unchanged — [rpg-project#1190](https://github.com/KirkDiggler/rpg-project/issues/1190).
- A section after "The status words" in the authoring service doc — [rpg-project#1190](https://github.com/KirkDiggler/rpg-project/issues/1190).

## Decided

- The status suite's reflection helpers moved unchanged into `words_reflect_test.go` as a `wordsReflect` suite both embed, because Test 1 needs them and the issue allows that one move — [rpg-project#1190](https://github.com/KirkDiggler/rpg-project/issues/1190).

## Found

- F1 to F4 hold; F2 prints 16; F3 prints Bounds :58, Value :67, Word :78, Words :116; F4 at role_policy.go:24 — [fact comment](https://github.com/KirkDiggler/rpg-project/issues/1190#issuecomment-7185324718).
- F5 prints requireSame :112, requireValue :126, hasAny :161; `snake` lives in machine_words_test.go:277, same package — [fact comment](https://github.com/KirkDiggler/rpg-project/issues/1190#issuecomment-7185324718).
- F6: after the pins `go build ./...` passes; the handler embeds `UnimplementedAuthoringServiceServer` which answers the new method until it is written — [fact comment](https://github.com/KirkDiggler/rpg-project/issues/1190#issuecomment-7185324718).
- `go mod tidy` moved only the two pinned lines — [rpg-project#1190](https://github.com/KirkDiggler/rpg-project/issues/1190).
- `charges.max` in the issue is the word `max` of place `charges`; the table also has a word `charges` in places `item` and `cost`, so the test finds a word by place and name — [rpg-project#1190](https://github.com/KirkDiggler/rpg-project/issues/1190).
- Proof one, `With` not copied: Test 1 fails with `Words.Values.With: length` expected 1 actual 0 — [rpg-project#1190](https://github.com/KirkDiggler/rpg-project/issues/1190).
- Proof two, `NoMax` not copied: Test 1 fails with `Words.Values.With.Bounds.NoMax` expected true actual false — [rpg-project#1190](https://github.com/KirkDiggler/rpg-project/issues/1190).
- New test times: TestItemWordsSuite 0.00s, TestItemWordsRequireBuildPermission under 0.01s — [rpg-project#1190](https://github.com/KirkDiggler/rpg-project/issues/1190).

## Mistaken

- The first draft of the content test looked up `charges.max` as a word named that; the table names it `max` in place `charges`, and the test failed until it looked up by place — [rpg-project#1190](https://github.com/KirkDiggler/rpg-project/issues/1190).
