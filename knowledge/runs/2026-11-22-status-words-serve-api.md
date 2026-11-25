# Run — 2026-11-22 — Status effect powers on a player S-words the api

processed: 2026-11-25T190427Z

Slice [rpg-project#1097](https://github.com/KirkDiggler/rpg-project/issues/1097) of story [rpg-project#1036](https://github.com/KirkDiggler/rpg-project/issues/1036). Base: `rpg-api` `dev` at [`560b1ee1`](https://github.com/KirkDiggler/rpg-api/commit/560b1ee1).

## Shipped

- `GetStatusWords` and `SayStatus` served: orchestrator methods, handlers, conversion copying every field, role policy entries at Build — [rpg-project#1097](https://github.com/KirkDiggler/rpg-project/issues/1097).
- Pins moved: `rulebooks/dnd5e` v0.224.0 to v0.226.0 and `rpg-api-protos/gen/go` to 45b0314d255f; encounter, session and resolution unchanged — [rpg-project#1097](https://github.com/KirkDiggler/rpg-project/issues/1097).
- A section beside "The machine words" in the authoring service doc — [rpg-project#1097](https://github.com/KirkDiggler/rpg-project/issues/1097).

## Decided

- A refusal crosses as one FieldError with the rules' Path and Says untouched, an empty Path staying empty, because the api describes nothing — [rpg-project#1097](https://github.com/KirkDiggler/rpg-project/issues/1097).
- A non-refusal error from status.Say is returned as an error, never an empty sentence, because the api fails closed — [rpg-project#1097](https://github.com/KirkDiggler/rpg-project/issues/1097).
- No wire-name exceptions are needed, so the new reflection test uses the plain snake_case of each Go field — [rpg-project#1097](https://github.com/KirkDiggler/rpg-project/issues/1097).

## Found

- A1 holds (go.mod lines 19-22 at v0.224.0 / v0.239.0 / v0.78.0 / v0.206.0); A2 holds (orchestrator :362-394, handler :31-47, role policy :24); A3 holds (role_policy_test.go:33); A4 holds (machine_words_test.go wireNames :32, requireSame :220) — [fact comment](https://github.com/KirkDiggler/rpg-project/issues/1097#issuecomment-7069727588).
- A5 re-run after the pins: `go build ./...` exit 0; `internal/dungeons` and `authoring/...` ok, `internal/auth` failed only on the two unclassified RPCs, as the issue says — [rpg-project#1097](https://github.com/KirkDiggler/rpg-project/issues/1097).
- `go mod tidy` moved only the two requirement lines; `go list -m` prints encounter v0.239.0, session v0.206.0, resolution v0.78.0 — [rpg-project#1097](https://github.com/KirkDiggler/rpg-project/issues/1097).
- New test times: TestStatusWordsSuite 0.00s (all four tests), TestStatusWordsRequireBuildPermission 0.00s — [rpg-project#1097](https://github.com/KirkDiggler/rpg-project/issues/1097).
- A status JSON without `again` is refused at path `again`, so a refusal test must write `again` to reach its own row — [rpg-project#1097](https://github.com/KirkDiggler/rpg-project/issues/1097).

## Mistaken

- The first refusal test inputs omitted the required `again`, so both failed on path `again`; the inputs now write it and the test passes — [rpg-project#1097](https://github.com/KirkDiggler/rpg-project/issues/1097).
