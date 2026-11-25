# Run — 2026-11-23 — a wake says whom S-pins the engine pins

processed: 2026-11-25T190427Z

Slice [rpg-project#1129](https://github.com/KirkDiggler/rpg-project/issues/1129) of story [rpg-project#1123](https://github.com/KirkDiggler/rpg-project/issues/1123). Base: `rpg-api` `dev` at [`246af973`](https://github.com/KirkDiggler/rpg-api/commit/246af973).

## Shipped

- `rulebooks/dnd5e/encounter` v0.249.1 to v0.252.0 and `rulebooks/dnd5e/session` v0.208.2 to v0.208.5 as one set in one commit; no content change — [rpg-project#1129](https://github.com/KirkDiggler/rpg-project/issues/1129).

## Decided

- No existing test was edited: every test passed unchanged, so the allowed machine-words edit was not used — [rpg-project#1129](https://github.com/KirkDiggler/rpg-project/issues/1129).

## Found

- F1 to F5 all hold: F1 printed the four dnd5e pins at 0.226.0/0.249.1/0.80.0/0.208.2; F2 printed `1` and `v0.252.0`; F3 printed the one test line and an empty castle diff; F4 printed the handler line; F5 printed nothing — [Rests on comment](https://github.com/KirkDiggler/rpg-project/issues/1129#issuecomment-7095133636).
- go.mod diff: only the encounter and session requirement lines moved — [rpg-project#1129](https://github.com/KirkDiggler/rpg-project/issues/1129).
- `TestCastleCopiesAgree`, `notedwords` and the `Gate|ContentHash` session tests pass with no edit; `go test ./... -count=1` and `make lint` (0 issues) pass at the final head — [rpg-project#1129](https://github.com/KirkDiggler/rpg-project/issues/1129).

## Mistaken

- The first whole-suite run failed in `internal/integration/character` with "failed to ping redis" because the shared test redis container was gone (a parallel job); the rerun passed with no code change — [rpg-project#1129](https://github.com/KirkDiggler/rpg-project/issues/1129).
