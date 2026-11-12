# Run — 2026-11-07 — alive, not lagging S10 the act buffer counts commits

processed: 2026-11-12T165555Z

Slice [rpg-project#894](https://github.com/KirkDiggler/rpg-project/issues/894) of story [rpg-project#879](https://github.com/KirkDiggler/rpg-project/issues/879). Base: `rpg-api` `dev` at [`8d8c4ef`](https://github.com/KirkDiggler/rpg-api/commit/8d8c4ef34058bc19bc613a1873e044136b184e6e).

## Shipped

- The act broker queues one commit's acts per recipient as one unit, so its buffer counts commits; `StreamEvents` sends each unit's acts in order through the replay skip — [rpg-api#1199](https://github.com/KirkDiggler/rpg-api/pull/1199).
- The record broker queues one commit's beats per session as one unit the same way; `StreamRecord` loops the unit through its skip — [rpg-api#1199](https://github.com/KirkDiggler/rpg-api/pull/1199).

## Decided

- The default is 64 commits for both brokers, because a verb is one or two commits, so 64 is a client thirty-odd verbs behind, and units are shared slices so a stalled client holds at most 64 commits — [rpg-api#1199](https://github.com/KirkDiggler/rpg-api/pull/1199).
- `SESSION_ACT_BUFFER` keeps its name and now counts commits; nothing outside `rpg-api` set it — [rpg-api#1199](https://github.com/KirkDiggler/rpg-api/pull/1199).
- The record broker gets the same unit and no new env knob, so orchestrator wiring stays untouched while rpg-api#1197 is open — [rpg-api#1199](https://github.com/KirkDiggler/rpg-api/pull/1199).
- The subscription feed is renamed `Acts()` to `Units()` (and `Events()` to `Units()` on the record), so every reader of the old per-act channel fails to compile instead of silently reading units — [rpg-api#1199](https://github.com/KirkDiggler/rpg-api/pull/1199).

## Found

- One `PublishActs` and one `PublishRecord` call is one commit by the SDK's own contract (session v0.166.3 `act.go`, `record.go`), so grouping per call is grouping per commit — [rpg-api#1199](https://github.com/KirkDiggler/rpg-api/pull/1199).
- The record broker had the act broker's flaw: 256 single beats against a record that grows by hundreds of beats per garrison verb, and the web client holds a live `StreamRecord` — [rpg-project#894](https://github.com/KirkDiggler/rpg-project/issues/894).

## Mistaken

- The first lag test's second commit was `[C, C]`, so a `LastID` taken from the unit's first act passed it; the commit is now `[C, C, D]` and the mutant fails — [rpg-api#1199](https://github.com/KirkDiggler/rpg-api/pull/1199).
