# Run — 2026-11-12 — Courtyard S4 captured door frames through API

Slice [Project #906](https://github.com/KirkDiggler/rpg-project/issues/906), story [Project #867](https://github.com/KirkDiggler/rpg-project/issues/867).

## Shipped

- API implementation checkpoint: detached frame/span copies in both wire versions and native cold-host capture verification; Web adoption and the real-stack walk remain pending — [continuation](https://github.com/KirkDiggler/rpg-project/issues/906#issuecomment-6844016998).

## Decided

- Use actual generated bindings and pushed provider pseudo-versions because development precedes module publication; no replacement directives or source-copy override — [contract](https://github.com/KirkDiggler/rpg-api-protos/pull/436), [encounter](https://github.com/KirkDiggler/rpg-toolkit/pull/2412), [session](https://github.com/KirkDiggler/rpg-toolkit/pull/2413).
- Copy phase pointer presence, including zero, rather than default it because the translator must not invent an appearance fact — [wire contract](https://github.com/KirkDiggler/rpg-api-protos/pull/436).
- Synchronize the canonical castle from the newly pinned provider because this repository must ship the document the provider's route tests exercised; the two shout annotations are upstream work, not new gameplay choices here — [upstream change](https://github.com/KirkDiggler/rpg-toolkit/pull/2409).

## Found

- The native fixture survives a fresh host with an empty site provider reading captured data from Redis after valid source edits; frame looks, wrapped phases, visual cuts and cells remain unchanged — [provider capture](https://github.com/KirkDiggler/rpg-toolkit/pull/2412).
- Both converter packages pass, `make lint` reports zero issues and final `make test` passes across the API, including cmd/server — [slice](https://github.com/KirkDiggler/rpg-project/issues/906).

## Mistaken

- The first full API gate found its castle copy lacked the two already-tested shout annotations in the new provider pin; copying that exact canonical file restored the equality guard without weakening it or patching gameplay — [upstream change](https://github.com/KirkDiggler/rpg-toolkit/pull/2409).
