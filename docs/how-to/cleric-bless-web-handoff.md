# Cleric/Bless API integration and web handoff

API issue #975 introduced the consumer; follow-up #978 audits native creation
and private projection. Current published provider pins (including later dev updates):

| Module | Adopted release |
| --- | --- |
| rpg-toolkit/rulebooks/dnd5e | v0.165.1 (acquisition #1713 and strict Cleric projection #1721) |
| rpg-toolkit/rulebooks/dnd5e/resolution | v0.47.0 |
| rpg-toolkit/rulebooks/dnd5e/encounter | v0.80.0 |
| rpg-toolkit/rulebooks/dnd5e/session | v0.83.0 |
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

## Native creation follow-up (#978)

`ListClasses` and `GetClassDetails("cleric")` now use the same class projection:
provider domain names, descriptions, selection level, resolved choice overlays,
and spellcasting metadata. Overlay `SubclassInfo.additional_choices` by choice ID
over the base class choices. Saved drafts include `class_info`, their domain, and
the correct SPELLS/CANTRIPS categories, so a fresh client can resume and resubmit.
Spellcasting ability, counts and first-level slots come directly from ClassData;
the choice requirements are authoritative for acquisition. A zero static
`spells_known` value does not erase the Cleric's five-spell acquisition pool.

No new proto is required for these repairs. Published Go SDK `f97bcce04ae5`
(TypeScript v0.1.189) already contains these catalog/draft fields, owner-private
CharacterData, open SpellRefs, healing/sourced conditions, and CastMissed.
Preparation and automatic domain spell grants remain outside this work.

The native acceptance helper selects Life Domain from the returned catalog,
creates and saves all choices, reloads the draft through a fresh service,
resubmits the returned choices, and finalizes without seeded character data.
The session tests join that character in world mode and start combat by spawning
a perceived enemy. World exploration has no available Cast offers under the
current provider clock; combat is required for this playthrough.

Real Bless, Cure Wounds and Healing Word then pass at the API boundary. Tests
assert slot spending, action/bonus-action spending, ordered targets, healing and
condition sources, and equality of streamed events with Story after reopening
the host. The same native character covers default/refuse/attempt stale-target
policy, refusal without spending, paid misses, and ordered mixed outcomes.
These in-process handler/Redis tests do not establish browser or deployment
readiness. The web playthrough must be repeated against the resulting API build.

### Private projection fixed; remaining acquisition limitation

Published root v0.165.1 is exactly toolkit PR #1721's merge commit
`9ec6aae85f3a6125fabc2ea486b7457078bdef6c`. It fixes the strict Cleric resource
owner catalog and condition sources tracked in toolkit #1720. The native
`TestAcceptance_NativeClericOwnerPrivateData` now passes through the real owner
handler. No permissive loader, API-authored owner catalog, or local module
replacement is used.

- **Knowledge Domain acquisition:** the provider advertises additional skill
  and language requirements, but its ClassChoices input has no grouped extra
  skill/language fields and SetClass validates the base skill count. Its language
  requirement also has nil options meaning any language. The API preserves the
  advertised requirements; it does not invent choices or hide the domain.
  This is separate provider work. Native acceptance currently covers Life Domain.

The private-view regression was observed failing with v0.165.0 and passing with
v0.165.1. Full repository gate results are recorded in the API PR. Browser and
deployment acceptance remain a web follow-on.

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
