# Run — 2026-11-06 — Courtyard S2 API ground

Slice [#874](https://github.com/KirkDiggler/rpg-project/issues/874), story [#867](https://github.com/KirkDiggler/rpg-project/issues/867). Base: `dev` at `590a201`.

## Shipped

- API checkpoint maps captured region ground into both knowledge snapshots and discovery acts, with no new RPC, permission classification or game logic; joined stack acceptance remains separate — [S2](https://github.com/KirkDiggler/rpg-project/issues/874).

## Decided

- Adopt generated protos `dea142c77291` from v0.1.258 and the real pushed SDK/encounter pseudo-versions; no workspace replacements or locally generated bindings — [merged wire contract](https://github.com/KirkDiggler/rpg-api-protos/pull/421).

## Found

- Tests first showed missing ground in snapshot and perception mappings; focused tests now pass. A Redis-backed GetKnowledge test reopens the run through a fresh host after editing/recompiling source and still reads the original ground — [S2](https://github.com/KirkDiggler/rpg-project/issues/874).
- `make test` passed, including cmd/server, and the final `make ci-check` passed with zero lint issues and all CI-mode tests passing — [S2](https://github.com/KirkDiggler/rpg-project/issues/874).

## Mistaken

- The first complete CI check found two old-style octal permission literals in the new test; both are corrected to `0o600`, without changing test behavior — [S2](https://github.com/KirkDiggler/rpg-project/issues/874).
