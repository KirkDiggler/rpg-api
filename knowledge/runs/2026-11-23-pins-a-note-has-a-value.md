# Run — 2026-11-23 — a note has a value S5 pins

processed: 2026-11-25T190427Z

Slice [rpg-project#1117](https://github.com/KirkDiggler/rpg-project/issues/1117) of story [rpg-project#1007](https://github.com/KirkDiggler/rpg-project/issues/1007). Base: `rpg-api` `dev` at [`4089c360`](https://github.com/KirkDiggler/rpg-api/commit/4089c360).

## Shipped

- The api pins encounter `v0.249.1` and session `v0.208.2`; the castle content is unchanged and still agrees with the toolkit's — [rpg-project#1117](https://github.com/KirkDiggler/rpg-project/issues/1117).

## Decided

- The encounter pin is `v0.249.1`, not the issue's `v0.249.0`, because `v0.249.0` wrote `"As": ""` on every noted requirement and `v0.249.1` omits it when empty — [Amendment 2](https://github.com/KirkDiggler/rpg-project/issues/1117#issuecomment-7090381566).
- The notedwords golden picture is not regenerated; the engine was fixed instead — [Amendment 1](https://github.com/KirkDiggler/rpg-project/issues/1117#issuecomment-7090239340).

## Found

- Rests on F1 to F5 all hold, posted before the build — [rests-on comment](https://github.com/KirkDiggler/rpg-project/issues/1117).
- With the pin at `v0.249.0`, `internal/notedwords` `TestLastWeeksCastleCompilesAsItDid` failed on an added `"As": ""`; blocking gap — [gap](https://github.com/KirkDiggler/rpg-project/issues/1117#issuecomment-7090196837).
- `go.mod` against `dev`: `encounter v0.245.0 -> v0.249.1`, `session v0.208.0 -> v0.208.2`, nothing else — [rpg-project#1117](https://github.com/KirkDiggler/rpg-project/issues/1117).
- At the final head: `go build ./...`, `TestCastleCopiesAgree`, the Gate tests, `internal/notedwords` and `go test ./... -count=1` pass, and `make lint` prints 0 issues; no existing test changed — [rpg-project#1117](https://github.com/KirkDiggler/rpg-project/issues/1117).

## Mistaken

- The toolkit slice added the `As` field with no `omitempty` tag against the module's own rule; fixed in `v0.249.1` by [rpg-toolkit#2541](https://github.com/KirkDiggler/rpg-toolkit/pull/2541), tracked in [rpg-project#1118](https://github.com/KirkDiggler/rpg-project/issues/1118).
