# Run — 2026-11-07 — alive, not lagging S1 api: a Move answers before the world runs

processed: 2026-11-12T165555Z

Slice [rpg-project#890](https://github.com/KirkDiggler/rpg-project/issues/890) of story [rpg-project#879](https://github.com/KirkDiggler/rpg-project/issues/879). Base: `rpg-api` `dev` at [`8d8c4ef3`](https://github.com/KirkDiggler/rpg-api/commit/8d8c4ef3). Built on session v0.167.0, encounter v0.182.0 and protos v0.1.260.

## Shipped

- `InProcessContinuationRunner` runs the rest of a continued Move on its own goroutine under `context.WithoutCancel`. It recovers panics, logs failures and degraded reports with session and verb, and drains on shutdown. Run is refused once Drain has started. — [rpg-api#1197](https://github.com/KirkDiggler/rpg-api/pull/1197)
- `New` wires the runner into `sdk.Config.Continuations`. `Config.Continuations` injects a runner, `Config.AnswerWhole` hands the SDK none, and New refuses both together. — [rpg-api#1197](https://github.com/KirkDiggler/rpg-api/pull/1197)
- The server drains the runner after either stop path, with its own 10 s budget. — [rpg-api#1197](https://github.com/KirkDiggler/rpg-api/pull/1197)
- Move maps `continues`, `through` and `MOVEMENT_STATUS_CONTINUING`. — [rpg-api#1197](https://github.com/KirkDiggler/rpg-api/pull/1197)

## Decided

- `AnswerWhole` was chosen over a scripted inline runner for tests that assert a whole walk's answer. Any runner still answers `Continues` with an empty `Saved`; only a nil runner reproduces the whole answer. — [rpg-api#1197](https://github.com/KirkDiggler/rpg-api/pull/1197)
- Only the shared acceptance harness sets `AnswerWhole`. Every other constructor keeps the production runner, because their Moves never pay a round or never read the store right after. — [rpg-api#1197](https://github.com/KirkDiggler/rpg-api/pull/1197)
- Drain gets its own budget rather than the shutdown context, because the forced-stop path arrives with that context spent. — [review on rpg-api#1197](https://github.com/KirkDiggler/rpg-api/pull/1197)
- Run refuses after Drain starts rather than admitting and waiting. A continuation admitted during shutdown can be cut off by the process exit mid-save. Refused, the SDK fails the verb with nothing saved. It also removes a WaitGroup add-during-wait race. — [review on rpg-api#1197](https://github.com/KirkDiggler/rpg-api/pull/1197)
- A failed, panicked or degraded continuation is logged only. Telling the client needs a wire shape, which is a follow-up. — [rpg-api#1198](https://github.com/KirkDiggler/rpg-api/issues/1198)

## Found

- The first SDK cut handed `Run` its own verb context. A runner that continued with `context.WithoutCancel(runCtx)`, as the doc said, made the context its own parent, and `Value` overflowed the stack. The SDK test continued with `context.Background()` and missed it. Fixed in session [`c99e117c`](https://github.com/KirkDiggler/rpg-toolkit/commit/c99e117c).
- Three acceptance scenes assert on a whole walk's answer: the walk into the tomb forming a fight, the decision walk-away, and the gate troupe crossing. With the runner on, they answer after 5 of 19 steps. — [rpg-api#1197](https://github.com/KirkDiggler/rpg-api/pull/1197)
- The protos tag's commit on `main` has no `gen/go` module. The pin is the `generated` branch commit `d83fdfa5` above it. — [rpg-api#1197](https://github.com/KirkDiggler/rpg-api/pull/1197)

## Mistaken

- The continuation goroutine shipped at `d27ccfa` without a `recover`. A panic in the raise or the save, once one caller's Internal error, would have crashed the server. Review caught it, and a test now proves the process lives and the guard comes back. — [review on rpg-api#1197](https://github.com/KirkDiggler/rpg-api/pull/1197)
- At `d27ccfa` the `ContinuationReport` was discarded, so a save or delivery failure after the answer was silent. It is now logged. — [review on rpg-api#1197](https://github.com/KirkDiggler/rpg-api/pull/1197)

— cross-team agent, on behalf of KirkDiggler
