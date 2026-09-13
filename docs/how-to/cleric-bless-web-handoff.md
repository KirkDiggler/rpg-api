# Cleric/Bless API integration and web handoff

API issue #975 consumes published providers after their merges:

| Module | Adopted release |
| --- | --- |
| rpg-toolkit/rulebooks/dnd5e | v0.165.0 (includes acquisition PR #1713) |
| rpg-toolkit/rulebooks/dnd5e/resolution | v0.46.0 |
| rpg-toolkit/rulebooks/dnd5e/encounter | v0.79.0 |
| rpg-toolkit/rulebooks/dnd5e/session | v0.82.0 |
| rpg-api-protos/gen/go | v0.0.0-20260913015835-f97bcce04ae5 (generated after proto PR #333) |

`internal/handlers/dnd5e/session/v1alpha1/convert.go` maps `EventCastMissed`
and `CastMissedBody` to `EVENT_KIND_CAST_MISSED` (31) and `Event.cast_missed`
(37). The body carries actor, named target, and the existing open `SpellRef`.
Both `StreamEvents` and `GetStory` use this converter. They retain the provider's
sequence and recipient identity, including mixed condition results and misses.
No attack roll, AC, inferred reason, or location is added.

`internal/orchestrators/session.Config.StaleTargetPolicy` defaults explicitly to
`session.StaleTargetRefuse`. The production server accepts
`RPG_STALE_TARGET_POLICY=attempt` to opt into paid attempts; unset, empty, or
`refuse` uses refusal. Any other nonempty value fails manager construction.
This is host configuration, never a player cast field. The toolkit validates
targets, remembers locations, decides availability and payment, and emits outcomes.

Existing mappings preserve ordered `CastRequest.targets`, spell references,
declaration costs/target bounds, candidate availability and `Shortfall` text,
healing results/calculations, and condition source IDs. `CastResponse` remains
the persistence/delivery acknowledgement and existing caught-members projection;
named misses appear on the event stream and in story replay.

Character creation now preserves Cleric domain enum selections in both directions.
The provider's level-1 acquisition pool contains Bane, Bless, Command, Cure Wounds,
and Healing Word. Preparation and automatic domain grants remain deferred. Loading
existing characters does not backfill spells. The Character wire still lacks a
finalized subclass field; draft subclass round-tripping and stored toolkit data
retain it.

## Web follow-on after API review and merge

1. Consume the generated TypeScript release containing proto PR #333; verify the
   published package version rather than assuming it matches Go's pseudo-version.
2. Use the existing open spell references for Cleric acquisition and cast rows.
   Keep domain selection in the draft and send the provider-authored choices.
3. Render Bless's ordered target selection from declaration candidates and
   min/max bounds; show provider availability reasons. Do not compute eligibility
   from current visibility or fetch hidden target positions.
4. Handle `cast_missed` in both live and catch-up reducers/narration, e.g.
   “Mercy's Bless missed Ally.” Preserve sequence order among mixed results.
   Use the provided identities and SpellRef; do not render an attack roll.
5. Reuse healing and sourced condition rendering. Do not add missed targets to
   CastResponse or offer a per-cast policy switch.
6. Verify a newly created Cleric can cast Bless, Cure Wounds, and Healing Word
   through the UI; verify reconnect narration and each host policy in a local
   playthrough. API acceptance tests do not establish UI or deployment readiness.

Boundary tests cover actual character creation/persistence, missing domain
conversion, host default/refuse/attempt behavior, stale-target reasons, one-time
payment, ordered mixed outcomes after repository reload, live/story equality,
and both healing spells. Toolkit tests remain authoritative for concentration,
overlap, Bane coexistence, and detailed eligibility rules.
