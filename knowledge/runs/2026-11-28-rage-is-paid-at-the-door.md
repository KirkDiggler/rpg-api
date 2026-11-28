# Run — 2026-11-28 — rage is paid at the one gate S7f pins (api)

Slice [rpg-project#1283](https://github.com/KirkDiggler/rpg-project/issues/1283) of story [rpg-project#1131](https://github.com/KirkDiggler/rpg-project/issues/1131). Base: `rpg-api` `dev` at [`b4138430`](https://github.com/KirkDiggler/rpg-api/commit/b4138430).

## Shipped

- `go.mod` and `go.sum`: rules v0.249.0 → v0.251.0, resolution v0.86.1 → v0.87.1, session v0.223.1 → v0.223.2; the encounter pin stays v0.267.0, no production file changed — [3af9a9a2](https://github.com/KirkDiggler/rpg-api/commit/3af9a9a2).
- Two acceptance tests in `barbarian_rage_door_acceptance_test.go`: a second Rage while raging, and a self ability (Rage, Second Wind) named with a target — [b9b86f01](https://github.com/KirkDiggler/rpg-api/commit/b9b86f01).

## Decided

- The second Rage is sent with the selector the panel offers after the first, not a crafted fresh-turn state, because the issue says to build from the existing helpers — [rpg-project#1283](https://github.com/KirkDiggler/rpg-project/issues/1283).
- No guard allow-list entry was added, because no guard failed at the pins — [first comment](https://github.com/KirkDiggler/rpg-project/issues/1283#issuecomment-7234884240).

## Found

- The suite is green at the old pins (44 packages ok), after the pins alone (44 ok, no existing test failed or edited) and at the final head (44 ok) — [first comment](https://github.com/KirkDiggler/rpg-project/issues/1283#issuecomment-7234884240).
- F4 (`-run 'Rag|Activat|Barbarian'`, top-level passes): 5 at the old pins, 5 after the pins, 6 at the final head (the one top-level test added counts; the other new test's name does not match the filter) — [first comment](https://github.com/KirkDiggler/rpg-project/issues/1283#issuecomment-7234884240).
- A second Rage while raging is refused ABORTED, "activate: declaration is stale": the bonus action is spent, so the offer's selector is stale; charges stay 1, bonus actions 0, actions 1, one raging status. The existing `TestAcceptance_RageSpendsACharge` only asserted an error; the new test pins code, message and the stored sheet — [b9b86f01](https://github.com/KirkDiggler/rpg-api/commit/b9b86f01).
- The malformed request over the wire, Second Wind with a target named: before INTERNAL, "activate: resolution: invalid action definition: dnd5e:features:second_wind lands on its user and names nobody else; got targets [skel-1]"; after INVALID_ARGUMENT, "activate: activation is invalid: resolution: invalid action definition: dnd5e:features:second_wind lands on its user and names nobody else; got targets [skel-1]" — [first comment](https://github.com/KirkDiggler/rpg-project/issues/1283#issuecomment-7234884240).
- Rage with a target named: the code stays INVALID_ARGUMENT, the message changes from "activation is invalid: resolution: invalid activation: dnd5e:features:rage takes no target, but \"skel-1\" was named" to "...invalid action definition: dnd5e:features:rage lands on its user and names nobody else; got targets [skel-1]"; no existing test pinned the old message — [first comment](https://github.com/KirkDiggler/rpg-project/issues/1283#issuecomment-7234884240).
- `make lint` reports only the four known issues in `zz_ue_spike_test.go` — [rpg-project#1283](https://github.com/KirkDiggler/rpg-project/issues/1283).
- The proof bites: after a refused second Rage, moving her charges to 0 fails the new test (expected 1, actual 0) — [b9b86f01](https://github.com/KirkDiggler/rpg-api/commit/b9b86f01).

## Mistaken

- `/home` hit 100% during `make lint` (a 102 GB Go build cache, "no space left on device"); `go clean -cache` freed it and the lint was re-run clean of that — [rpg-project#1283](https://github.com/KirkDiggler/rpg-project/issues/1283).
