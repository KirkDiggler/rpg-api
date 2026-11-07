# 2026-11-07 — current composed Raging in native readers

## Shipped

- Adopt pushed session 2909290a, resolution 9425c9da and rulebook 3e771a63; metadata conversion carries canonical origin ref and registry description while preserving raw condition bytes — [converter](../../internal/handlers/dnd5e/v1alpha1/character/converters.go).
- Replace obsolete concrete saved-data fixtures; real handler activation, Redis persistence, four domain-specific action rows and first-boundary reload are exercised together — [native integration test](../../internal/integration/session/composed_raging_acceptance_test.go).

## Decided

- Map registry Detail into the existing Condition.notes display field and composed Origin.Ref into Source; neither time remaining nor a damage bonus is calculated here. Other condition formats retain their source field — [converter seam test](../../internal/handlers/dnd5e/v1alpha1/character/composed_raging_test.go).
- At the first checkpoint, source-ID mapping awaited proto#422 publication. The operator merged it; generated release v0.1.259 (7dcb1b2) is now pinned and EffectRow.SourceID maps directly to source_id. Isolated boot, operator look, walk, independent review and toolkit release-tag adoption remain outstanding — [S12](https://github.com/KirkDiggler/rpg-project/issues/884).

## Found

- After generated SDK adoption and source-ID mapping, full make test passed in 426.37 seconds and make ci-check passed in 491.72 seconds; no test set was narrowed — [gate definitions](../../Makefile).
- Proto #422 merged as 1fc63b4; publication CI 42422302400 passed and v0.1.259 points to generated commit 7dcb1b2. The Go submodule resolves that published commit to v0.0.0-20261107224950-7dcb1b2ebf4c; no generated code was authored locally — [publication](https://github.com/KirkDiggler/rpg-api-protos/actions/runs/42422302400).
- Declaration/held-row converter assertions and all four native Raging row assertions failed on empty source IDs, then passed after the direct mapping. Source entity identity is not substituted for row identity or recipient — [wire projection tests](../../internal/handlers/dnd5e/session/v1alpha1/convert_test.go).

- The converter test failed because source and description were empty, then passed after direct metadata projection. The native integration test observes fresh → raging progress in Redis without renewed grace/payment — [tests](../../internal/handlers/dnd5e/v1alpha1/character/composed_raging_test.go).
- Full make test passed, including cmd/server. make ci-check passed in 483.95 seconds after import formatting; the earlier combined command reached its 600-second bound after make test had completed, not a passing ci-check — [gate definitions](../../Makefile).

## Mistaken

- The first new converter fixture called ToJSON on the factory output instead of its Condition; that compile error was corrected before observing the meaningful missing-metadata RED. A new integration file also needed the repository's goimports grouping — [fixture](../../internal/handlers/dnd5e/v1alpha1/character/composed_raging_test.go).

— cross-team agent, on behalf of KirkDiggler
