# Sanctuary development test build (#1006)

This branch intentionally consumes the four **open, unmerged** toolkit PRs at
their exact pushed commits, as requested for local testing. These are not
released toolkit tags. No filesystem replacements or Go workspace are used.

| Owning module | PR | Exact development version |
| --- | --- | --- |
| rulebooks/dnd5e | #1811 | v0.180.1-0.20260918033336-bd1fe77202b4 |
| rulebooks/dnd5e/resolution | #1812 | v0.53.1-0.20260918033440-e3c885da0e7c |
| rulebooks/dnd5e/encounter | #1813 | v0.88.1-0.20260917214116-6b4ac354edaf |
| rulebooks/dnd5e/session | #1814 | v0.95.2-0.20260918033557-6566013f1070 |

The session PR's go.mod already names this same combination. Requirements are
pinned together (using `go mod edit -require=...`, then `go mod tidy`) to avoid
resolving a nested module from another module's feature commit.
Merged protos #345 are in SDK v0.1.201, generated Go submodule
`v0.0.0-20260918041943-0655dbbca56c`. Its generated code also contains the additive
creature-table contract; this branch does not adopt those toolkit PRs or API #1005.
Existing Answered verb/beaten fields remain populated for this provider's contract.

## What is testable

- Native Cleric creation includes the provider's new sixth spell, Sanctuary.
- Cast Sanctuary through the existing Cast declaration; the provider spends
  the bonus action and slot and owns concentration.
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
Full checks: `make pre-commit` and `make ci-check`. The current release-pin
script rejects local replaces/workspaces; passing it does **not** turn these
pseudo-versions into released tags.

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
error. This API does not bypass that limitation. Before release adoption,
merge/release the providers through their normal workflow, replace all four
development pins with the verified tags, and rerun validation. No automatic merge.
