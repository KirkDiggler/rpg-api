# Run — 2026-11-07 — canonical behavior API integration

Part of [live adoption #866](https://github.com/KirkDiggler/rpg-project/issues/866), under [story #859](https://github.com/KirkDiggler/rpg-project/issues/859).

## Shipped

- Prepared real pushed encounter/session pins, rebuilt current-format content, and API source/persistence/continuation proofs; the wave remains unmerged and the encounter provider remains held — [early-look receipt](https://github.com/KirkDiggler/rpg-project/issues/866#issuecomment-6711912823).

## Decided

- Kept all policy and execution in toolkit: API changes are dependency pins, authored content and tests, with no new RPC or host-owned runner — [consumer plan](https://github.com/KirkDiggler/rpg-project/pull/877).
- Rebuilt the camp with explicit shared combat configuration, retaining its floor, actor seats, letter cell and actual letter/kill/reinforcement journeys; its old thin seams become native structural walls/openings, not a legacy policy adapter — [R9](https://github.com/KirkDiggler/rpg-project/pull/877#issuecomment-6698028412).
- Replaced obsolete byte parity with the toolkit's v2 camp example by an explicit roster-and-reserve policy check plus the real handler journeys; the historical front-room policy now has a launch-refusal test, while canonical content proves resolved-key propagation — [R9](https://github.com/KirkDiggler/rpg-project/pull/877#issuecomment-6698028412).
- The camp walk finishes its actual path if sight forms a fight before the crossing, ends the spent turn normally before requesting full movement legs, and reads both observed cell props and placed footprints; no sheet reset, fabricated movement or world-truth visibility read — [#866](https://github.com/KirkDiggler/rpg-project/issues/866).
- Kept nonblocking placed-letter cells eligible as native party seats while excluding every reserved creature cell; retired the old atomic-prop seat-count assumption rather than changing placement rules — [#866](https://github.com/KirkDiggler/rpg-project/issues/866).

## Found

- Full owning-repository `make test` and `make ci-check` pass, including the rebuilt camp and current-source lobby path; the latter verifies formatting, imports, tidy, lint and race tests, while `make test` also covers `cmd/server` — [#866](https://github.com/KirkDiggler/rpg-project/issues/866).
- Actual Attack emits the qualifying clang: the nearby guard investigates without advancing its unfinished duty, the farther guard continues independently, and fresh managers over Redis resume the original long leg; the exact script is ten d100 choices, one d20 attack and one d8 damage — [#866](https://github.com/KirkDiggler/rpg-project/issues/866).
- An authored dual-context patrol triggers a real opportunity-reaction pause through EndTurn; Redis preserves its selected claim and 20 feet, source recompilation changes no saved configuration, and React/Hold reaches the original goal once with no extra selection — [#866](https://github.com/KirkDiggler/rpg-project/issues/866).
- API repository overlays dropping the held claim or resetting duty progress on load fail actual assertions, including continuation and final positions; original repository bytes remain unchanged — [#866](https://github.com/KirkDiggler/rpg-project/issues/866).
- The camp's kill acceptance now restricts reinforcement arrivals to new acts sharing the killing blow's cause, rather than counting all earlier arrivals and accidentally depending on whether the letter had arrived — [#866](https://github.com/KirkDiggler/rpg-project/issues/866).
- The rebuilt camp atlas is identical under the pre-adoption released projector (policy-only fields omitted) and the new complete source: 112 cells, three regions, two structural walls, two structural doors and one exit; no projection code changed — [#866](https://github.com/KirkDiggler/rpg-project/issues/866).
- Kirk confirmed both independent goblin patrols on the isolated early-look stack; that confirms the visible cadence, not merge approval or the remaining QA/review — [operator receipt](https://github.com/KirkDiggler/rpg-project/issues/866#issuecomment-6711996296).

## Mistaken

- The first rebuilt camp used unsupported `open: true`; the compiler refused it and the source now uses `closed: false`; this was a fixture-authoring error, not a production regression — [#866](https://github.com/KirkDiggler/rpg-project/issues/866).
- Initial tests still assumed legacy prop projection, a full movement budget after a partial walk, and exactly three historical arrivals; native projection, real turn payment and same-blow evidence corrected those assumptions without weakening the gameplay contract — [#866](https://github.com/KirkDiggler/rpg-project/issues/866).
- A first mutation attempt targeted the module cache, which Go overlays refuse; that tool failure is not mutation evidence. Subsequent overlays target the API's own repository transport and fail real assertions — [#866](https://github.com/KirkDiggler/rpg-project/issues/866).
- The untracked local walk file entered the shipped-atlas golden scan; gate runs temporarily park only that file and restore it byte-identically, rather than committing a golden for local content — [early-look receipt](https://github.com/KirkDiggler/rpg-project/issues/866#issuecomment-6711912823).
