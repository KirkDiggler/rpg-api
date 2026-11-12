# Run — 2026-11-06 — Courtyard S2 API ground

processed: 2026-11-12T165555Z

Slice [#874](https://github.com/KirkDiggler/rpg-project/issues/874), story [#867](https://github.com/KirkDiggler/rpg-project/issues/867). Base: `dev` at `590a201`.

## Shipped

- API checkpoint maps captured region ground into both knowledge snapshots and discovery acts, with no new RPC, permission classification or game logic; joined stack acceptance remains separate — [S2](https://github.com/KirkDiggler/rpg-project/issues/874).

- Added the Studio-exported courtyard ground to the canonical castle shelf copy and adopted the actual pushed encounter/session castle checkpoints; existing source geometry, gameplay data and other regions are unchanged — [S2](https://github.com/KirkDiggler/rpg-project/issues/874).

## Decided

- Adopt generated protos `dea142c77291` from v0.1.258 and the real pushed SDK/encounter pseudo-versions; no workspace replacements or locally generated bindings — [merged wire contract](https://github.com/KirkDiggler/rpg-api-protos/pull/421).

- Keep the API shelf identical to the toolkit's canonical castle rather than decorate source at runtime; the existing TestCastleCopiesAgree guard verifies that contract against the actual pinned module — [S2](https://github.com/KirkDiggler/rpg-project/issues/874).

## Found

- Tests first showed missing ground in snapshot and perception mappings; focused tests now pass. A Redis-backed GetKnowledge test reopens the run through a fresh host after editing/recompiling source and still reads the original ground — [S2](https://github.com/KirkDiggler/rpg-project/issues/874).
- `make test` passed, including cmd/server, and the final `make ci-check` passed with zero lint issues and all CI-mode tests passing — [S2](https://github.com/KirkDiggler/rpg-project/issues/874).

- The before-edit castle SHA-256 matched the operator deployment mount; only its courtyard ground changed, and the operator's mounted source was left untouched. The new shelf agrees with the re-pinned toolkit fixture — [S2](https://github.com/KirkDiggler/rpg-project/issues/874).

- Captured the updated castle atlas with unchanged reviewed projector `2355058` and the new authored source; structural comparison found only the courtyard ground field added, preserving every other atlas fact — [S2](https://github.com/KirkDiggler/rpg-project/issues/874).

- Full `make test`, including cmd/server, and `make ci-check` pass with the folded source and actual pushed provider pins — [S2](https://github.com/KirkDiggler/rpg-project/issues/874).

## Mistaken

- The raw JSON atlas capture lacked the repository's final newline; the EOF gate caught it and normalization changed no parsed atlas value — [S2](https://github.com/KirkDiggler/rpg-project/issues/874).
- The first castle-fold gate correctly rejected the old appearance-free atlas fixture. It was recaptured from the trusted prior projector and checked for the single intended difference, not blindly regenerated from changed projection code — [S2](https://github.com/KirkDiggler/rpg-project/issues/874).

- The first complete CI check found two old-style octal permission literals in the new test; both are corrected to `0o600`, without changing test behavior — [S2](https://github.com/KirkDiggler/rpg-project/issues/874).
