# Sanctuary development test build (#1006)

This branch adopts the published Sanctuary provider releases. All four toolkit
PRs are merged; no development toolkit pins, filesystem replacements, or Go
workspace are used.

| Owning module | PR | Released version |
| --- | --- | --- |
| rulebooks/dnd5e | #1811 | v0.181.0 |
| rulebooks/dnd5e/resolution | #1812 | v0.54.0 |
| rulebooks/dnd5e/encounter | #1813 | v0.89.0 |
| rulebooks/dnd5e/session | #1814 | v0.96.0 |

Session v0.96.0 resolves to #1814 merge
`9cee65ab71691587654fadc55f88750807c13baf` and pins this released combination.
Merged protos #345 are in SDK v0.1.201, generated Go submodule
`v0.0.0-20260918041943-0655dbbca56c`. Its generated code also contains the additive
creature-table contract; this branch does not adopt those toolkit PRs or API #1005.
Existing Answered verb/beaten fields remain populated for this provider's contract.

## What is testable

- Native Cleric creation includes the provider's new sixth spell, Sanctuary.
- Cast Sanctuary through the existing Cast declaration; the provider spends
  the bonus action and slot and owns concentration.
- Sanctuary immediately applies its anti-recast cooldown to the recipient. Any
  caster is blocked from applying Sanctuary again for 20 recipient turn ends;
  the cooldown survives ward loss and its countdown persists across reloads.
  The target picker exposes the refusal, and a forced recast spends nothing.
  Passing a ward save does not grant the attacker immunity.
- Failed ward saves on attacks map to Warded events and AttackResponse's
  warded/warded_by fields. These are the attacker's saves, not attack rolls.
- Failed ward saves on hostile casts map to CastWarded and CastResponse's
  ordered warded_targets. Both events retain source, target, ability, roll,
  total, DC and sourced calculation through the shared live/Story converter.
- Integration tests exercise a newly created Cleric, actual Sanctuary casts,
  warded weapon attacks and hostile Sacred Flame, payment and reloaded Story
  equality. Casting Bane from that same warding caster would replace Sanctuary
  concentration, so Sacred Flame is used for the isolated ward test.

Focused test: `go test ./internal/handlers/dnd5e/session/v1alpha1 ./internal/integration/session -run Sanctuary -count=1`.
Full checks: `make pre-commit` and `make ci-check`.

## Local image and integration

The local image is tagged `rpg-api:sanctuary-1006`. Build from a clean checkout
of this branch with `docker build -t rpg-api:sanctuary-1006 .`; an archive of the
commit avoids carrying worktree Git metadata into the build context.
Use this image only in the intended disposable development Compose project.
This task builds the image but does not replace any running service.
The API PR records the exact built commit and local image ID.

Web must consume SDK v0.1.201 and render Warded/CastWarded separately from
ordinary attack misses and target saves. A successful API test is not browser
acceptance. Existing characters are not backfilled with Sanctuary; create a new
Cleric or explicitly document any isolated fixture setup.

Provider caveat from #1812: a nested Sanctuary save colliding with an optional
offer (such as Resistance) currently reports the provider's cannot-be-suspended
error. This API does not bypass that limitation. All four providers are now adopted by release tag. No automatic merge.

The local image and running browser stack were built from the earlier, tested
provider commits at API `d1717ba`; changing release pins in this PR does not
rebuild or replace that running stack. The user observed recipient immunity in
Story and confirmed that no eligible targets prevented recasting. Full timer
expiry was covered by provider regression tests, not manual browser acceptance.
