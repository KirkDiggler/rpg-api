# Run — 2026-11-08 — structured machine inspection capture

## Shipped

- Implemented independent structured capture of committed records and development-only client record access on the pushed inspection providers; the playtest reader and live walk remain outstanding — [Machine inspection S2](https://github.com/KirkDiggler/rpg-project/issues/900).

## Decided

- Keep the record broker and capture independent consumers of one SDK batch, because a slow/failed log export must not suppress browser delivery or reinterpret game decisions — [R5–R7](https://github.com/KirkDiggler/rpg-project/pull/902).
- Use a64-commit bounded worker with detached accepted bytes, fail-stop overflow and explicit start/drained/incomplete status; a clean drain covers accepted batches, never invented prior history — [R6–R8](https://github.com/KirkDiggler/rpg-project/pull/902).
- Keep capture lifecycle with the server and leave Orchestrator.Drain/continuation/cache implementation unchanged, because the concurrent resident-instance work owns that path — [coordination](https://github.com/KirkDiggler/rpg-project/issues/879#issuecomment-6725835021).
- Gate both diagnostic RPCs from existing AUTH_DEV_MODE while capturing to stdout in every server mode; ordinary StoryService remains separate — [R7](https://github.com/KirkDiggler/rpg-project/pull/902).

## Found

- Capture tests preserve raw false/zero/empty/large-integer/HTML values, reject bad configuration, copy accepted payloads, avoid blocking on a stalled writer, report overflow/short writes/failures/panics and drain with a deadline — [#900](https://github.com/KirkDiggler/rpg-project/issues/900).
- Real compiler/Redis/handlers/production continuation test captures two original actor configurations across manager reload and source edit, four actual choices, normal guarded completion after early answers, and the same record IDs/fields as GetRecord; no browser subscription is required — [#900](https://github.com/KirkDiggler/rpg-project/issues/900).
- Handler and actual server-registration wiring tests refuse diagnostic reads in release mode before storage/subscription; normal gameplay still reads, and development mode remains inspectable — [#900](https://github.com/KirkDiggler/rpg-project/issues/900).
- Full make test and make ci-check pass after targeted lint corrections; current base includes upstream cold-reload measurement #1203 without changing that measurement or its owner — [#898](https://github.com/KirkDiggler/rpg-project/issues/898).

## Mistaken

- First comprehensive CI check found two shadowed error names, one spelling issue and a test-parser preallocation issue. Full tests had passed; the corrected full CI gate passes without suppressions — [#900](https://github.com/KirkDiggler/rpg-project/issues/900).
