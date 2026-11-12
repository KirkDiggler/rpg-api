# Run — 2026-11-08 — Captured texture choices at the API

processed: 2026-11-12T165555Z

## Shipped

- API checkpoint transcribes captured wall/prop material overrides in snapshot and act geometry. A native-source/Redis/fresh-host test proves original choices survive later source edits; product texture controls and joined play remain outstanding — [Courtyard S3](https://github.com/KirkDiggler/rpg-project/issues/895).

## Decided

- Adopt actual pushed SDK 579875f9 / encounter e3849c88 and CI-generated nested proto module c23b703. No local replacements, SDK generation, resource lookup, new RPC or permission classification — [wire](https://github.com/KirkDiggler/rpg-api-protos/pull/425).

## Found

- Both conversion tests and the native fresh-host acceptance test failed on missing material lists, then pass with field transcription. The saved 04_C choices remain on both owner records after valid source changes to 01_A; floor and layout records remain identical — [S3](https://github.com/KirkDiggler/rpg-project/issues/895).
- Full make test, including cmd/server, completes; final make ci-check passes with zero lint issues and all CI-mode tests. No toolkit game logic was added to API — [S3](https://github.com/KirkDiggler/rpg-project/issues/895).

## Mistaken

- First proto adoption targeted the repository root Go module; tidy correctly removed the unused root requirement. Adopted the actual imported gen/go module at c23b703 and verified go.mod rather than trusting the download message — [S3](https://github.com/KirkDiggler/rpg-project/issues/895).
- The combined full-test/CI command exceeded its 900-second wrapper during CI after make test completed. Separate foreground CI finished, found one test import grouping issue, then passed after targeted goimports; no timeout increase or test removal in repository policy — [S3](https://github.com/KirkDiggler/rpg-project/issues/895).
