# Run — 2026-11-13 — Composable conditions S18 API adoption

Slice [rpg-project#951](https://github.com/KirkDiggler/rpg-project/issues/951), story [#703](https://github.com/KirkDiggler/rpg-project/issues/703). Base: API dev [aaccd36b](https://github.com/KirkDiggler/rpg-api/commit/aaccd36b).

## Shipped

- Adoption checkpoint: API selects released rulebook v0.216.0 and mechanics/effects v0.6.0; encounter v0.204.1, resolution v0.75.1, session v0.192.0 and the generated SDK are unchanged — [scope](https://github.com/KirkDiggler/rpg-project/issues/951).

## Decided

- KirkDiggler directed this wave to adopt its own toolkit release into API rather than wait for unrelated work; this is API dev adoption, not fixture-item acquisition or a production deployment — [direction](https://github.com/KirkDiggler/rpg-project/issues/951).
- No API rules or web translation are added: the API already preserves the provider's reason, and inspected web dev e815a9fe renders the reason through generic authoredWords; no old-reason discriminator exists on that path — [web reader](https://github.com/KirkDiggler/rpg-dnd5e-web/blob/e815a9fe/src/components/session/combat-experience/describeAct.ts#L553-L559).

## Found

- The native cast/turn/attack consumption test observed spell_ended on both live subscribers before the re-pin, then members_empty after it; persisted cleanup and fresh-host GetStory equality also pass. This is a saved-sheet transport fixture, not a new class-acquisition or browser claim — [acceptance](../../internal/integration/session/effects_release_adoption_test.go).
- Full API make test and make ci-check pass, including cmd/server, existing fresh-character maintenance/Raging and castle parity; no parallel content/provider or SDK pin is taken over — [slice gates](https://github.com/KirkDiggler/rpg-project/issues/951).

## Mistaken

- The first combined test/ci-check shell timeout was too short: make test completed successfully, then the wrapper expired during ci-check imports. ci-check was rerun to completion with a bounded longer timeout; no failed check was treated as passing and no checks were weakened — [gate record](https://github.com/KirkDiggler/rpg-project/issues/951).
