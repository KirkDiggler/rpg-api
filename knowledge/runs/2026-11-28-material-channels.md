# Run — 2026-11-28 — Courtyard S7 material channel transport

Slice [rpg-project#1192](https://github.com/KirkDiggler/rpg-project/issues/1192), story [867](https://github.com/KirkDiggler/rpg-project/issues/867). Base: API dev at [5ae70188](https://github.com/KirkDiggler/rpg-api/commit/5ae701881f2753e366183a9021a6f17f96cca8b8).

## Shipped

- Nothing merged or deployed by this task; this is a draft transport checkpoint, not browser/asset acceptance — [slice](https://github.com/KirkDiggler/rpg-project/issues/1192).

## Decided

- Keep the newer generated SDK294 already in dev, verified at d4dadb1f0734 and descended from the channel-contract merge, rather than downgrade to293 — [contract PR456](https://github.com/KirkDiggler/rpg-api-protos/pull/456).
- Normally catch Session up to main before consumption; generated Go pins retain rulebook0.248.0/resolution0.86.0 plus Encounter4b9aee80 and Sessionab0d12c9, without replacements or fabricated versions — [Session2623](https://github.com/KirkDiggler/rpg-toolkit/pull/2623).
- One shared leaf converter serves both wire versions; only nil means default, explicit Off/1 remain distinct, and malformed present leaves never become an invented valid appearance — [design](https://github.com/KirkDiggler/rpg-project/blob/design/material-channels/ideas/material-channels/design.md).
- Preserve the historical v0.220.0 oracle; normalize only null channel fields at exact wall/prop owners, with nonempty intent controls proving the comparison still fails for authored changes — [slice](https://github.com/KirkDiggler/rpg-project/issues/1192).
- Repair inherited local gate hygiene separately: trade response alignment, scoped error names and the equivalent context-aware listener call in the opt-in UE test; no skip, timeout, assertion, network policy or gameplay change — [base](https://github.com/KirkDiggler/rpg-api/commit/5ae701881f2753e366183a9021a6f17f96cca8b8).

## Found

- Snapshot/live wall, prop and independent frame converters plus observed roster transport preserve detached channel values; native launch reaches two clients and survives cold-host reads after source edits, while a hidden wall does not hide its separately permitted frame — [acceptance tests](https://github.com/KirkDiggler/rpg-api/blob/feat/material-channels/internal/integration/session/material_channels_acceptance_test.go).
- The historical compiled comparison adds exactly29 null fields: five prop presentations and24 walls; the stored oracle remains untouched — [comparison tests](https://github.com/KirkDiggler/rpg-api/blob/feat/material-channels/internal/notedwords/convert_test.go).
- The complete GOWORK=off make ci-check passes, including normal formatting/import/tidy/lint and race/coverage tests; fixture targets are opaque transport evidence, not provider enrollment or a browser walk — [slice](https://github.com/KirkDiggler/rpg-project/issues/1192).

## Mistaken

- The first dependency attempt selected the older Session base; module resolution and current dev pins exposed the mismatch, corrected by normal Session catch-up and a genuine pushed-commit pin — [Session2623](https://github.com/KirkDiggler/rpg-toolkit/pull/2623).
- The first integration fixture named an unregistered guard and omitted a concealment finding method; ordinary validation refused both, corrected to the registered skeleton and a normal perception approach without changing validators — [acceptance tests](https://github.com/KirkDiggler/rpg-api/blob/feat/material-channels/internal/integration/session/material_channels_acceptance_test.go).
- The first complete gate found feature formatting/shadow/line-length errors, the measured historical absence difference and inherited dev lint failures; the second passed tests but exposed further inherited shadows hidden by the linter's diagnostic count cap, then normal complete gates passed after scoped corrections — [slice](https://github.com/KirkDiggler/rpg-project/issues/1192).
- Earlier Session notes inferred timeout margin from590s package elapsed; TestMain prepares substantial castle fixtures before m.Run, so that inference was wrong; the later611.658s race run passed without timeout changes — [Session record](https://github.com/KirkDiggler/rpg-toolkit/blob/feat/material-channels-session/rulebooks/dnd5e/session/knowledge/runs/2026-11-28-material-channels.md).
