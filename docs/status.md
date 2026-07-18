---
name: rpg-api status
description: Where we are with rpg-api — active work, paused, known rough edges, per-subsystem confidence
updated: 2026-07-18
confidence: high — Wave 2 Monk entries verified against passing integration tests; #636 entry verified against passing unit + integration tests; #642 v1alpha1 encounter stack deletion verified against passing build/vet/test/lint; #644 The Dungeon wave 1 (api) verified against passing unit + stress-run (50x) integration tests; #650 toolkit seam adoption (InitiativeRolled event + room-aware spawn) verified against passing unit/integration/-race full suite; #651 ActiveConditions projection verified against passing unit + integration (10x -race) + full suite; #656 movement-truncation fix verified against an isolated toolkit-level repro, a new RPC-level regression test (10x -race), and the full suite
---

# rpg-api: Where We Are

This is a living doc. Edit it in the same PR that invalidates a line. Don't let it rot.

## Active work

**Wave-close blocker: stale character combat economy blocked all movement on fresh
encounters (rpg-api#644, 2026-07-15)** — found live on the dev stack: every `MoveEntity`
on a brand-new `FREE_ROAM` encounter failed `insufficient movement remaining: Nft, 0ft
remaining`, even on the very first move. Root cause was NOT a code regression in this
wave's commits (confirmed: none of them touch `MoveEntity`, the orchestrator, or
character hydration) — `character.Character.ActionEconomy`
(`rpg-toolkit/rulebooks/dnd5e/character`) has no encounter scoping; it is a flat field on
the character record. rpg-toolkit's `Move()` budget gate treats any non-nil
`ActionEconomy` as "in combat, enforce the budget" (`InCombat() == ActionEconomy != nil`),
and `ExitCombat()` — the toolkit's own API for clearing it, whose doc literally says "call
this when the encounter ends" — was never called anywhere: not in rpg-api, not even
inside rpg-toolkit's own encounter package (which owns `EncounterEndedEvent` but never
wires `ExitCombat` to it). A character that ever finished a combat (e.g. an earlier
playtest session) carries its depleted economy — `movement_remaining: 0` — into every
SUBSEQUENT encounter it's added to, forever, until something explicitly clears it.
`seedMemberCombatSnapshot`'s new `clearStaleActionEconomy`
(`internal/orchestrators/lobby/character.go`) fixes this at `StartEncounter` — the sole
path a new encounter comes into existence, so at that exact moment no member's character
can legitimately be mid-turn in the encounter about to exist, making it safe to
unconditionally clear. `SeedTurnEconomyForData` (`hydrate_players.go`) could not have
fixed this instead — it deliberately treats any non-nil economy as "already legitimately
seeded, leave it alone" (correct for its own mid-encounter job, unable to distinguish
live-and-depleted from stale-and-abandoned). **This is a defensive backstop, not the
complete fix — genuinely open follow-up**: rpg-api's encounter-end handling (or the
toolkit) should call `ExitCombat()` when an encounter properly ends, so a character never
carries stale economy into the next one in the first place.

**The Dungeon wave 1 (api half) — walls on the wire + real goblins seeded at StartEncounter (rpg-api#644, 2026-07-15)** —
`StartEncounter` (`internal/orchestrators/lobby/start_encounter.go`) now calls
`enc.InitRoom(20, 20, environments.PatternRandom)` right after `tkenc.New` (before any
`AddPlayer`/`AddMonster`, both of which consult `e.room` internally) and seeds
`goblinCount` (2) real goblins (`monster.NewGoblin`, DataJSON-carrying, identical shape
to the devseed/`devcombat.Inject` goblins) before persisting. Bumped
`rpg-toolkit/encounter` to v0.25.0 and added `rpg-toolkit/tools/spawn` v0.2.0
(rpg-toolkit#759). The projector (`internal/handlers/dnd5e/v2/encounter/project.go`'s
`ProjectFor`) now populates `Space.Walls` from the encounter's persisted room snapshot —
whole-room visibility for wave 1 (not LOS-gated like `Hexes`/`Entities`); zero proto
changes (`Wall`/`WallKind` already existed on the wire, unpopulated).

Goblin placement is verified NOT visible to any player at spawn — `AddMonster`
inline-checks combat entry (rpg-toolkit#759's `checkCombatEntry`), so a goblin seeded
within sight would flip the encounter to `TURN_BASED` immediately, violating the design
bar ("walk into a room, the fight starts" — combat starts on a `Move` that forms sight,
never at spawn). ~~**Known toolkit gap, not fixed here:** `tools/spawn`'s own
`BasicSpawnEngine` position search (`findValidPosition`, and the constraint-aware
`ConstraintSolver.FindValidPositions`) never consults the real `spatial.Room` this wave
introduced — no `room.CanPlaceEntity`/`room.IsLineOfSightBlocked` calls anywhere in that
path — and its optional `LineOfSight` constraint is a Euclidean-distance stub
(`constraints.go`'s `hasLineOfSight`, doc-flagged as a Phase-3 placeholder). `SpawnConfig`
also has no fixed/target-position injection point. `StartEncounter` still constructs and
calls `spawn.BasicSpawnEngine.PopulateRoom` (wired to `enc.RoomOrchestrator()`, exercising
rpg-toolkit#757/#759's `getRoomFromSpatial`/`placeEntityInRoom` fix from a real caller),
but discards its returned position in favor of one computed from the toolkit's own
wall-aware `perception.CanSeeAt` — see `seedGoblins`'/`safeGoblinHexes`' doc comments in
`start_encounter.go` for the full reasoning. A toolkit follow-up issue for
fixed-position spawn support is recommended before wave 2 needs dynamic (non-fixed-2)
monster placement through this engine.~~ **RESOLVED by rpg-api#650 (2026-07-17):**
rpg-toolkit#760 (`tools/spawn` v0.3.0) made the position search itself room-aware and
added `EntityGroup.PositionOracle` — a caller predicate ANDed into the search.
`seedGoblins` now expresses the out-of-sight requirement as a `PositionOracle` built on
`perception.CanSeeAt` and uses the engine's own returned position directly; `safeGoblinHexes`
and `placementProbe` are deleted. One new toolkit gap found and routed around while
migrating: `BasicSelectablesRegistry.GetEntities` samples WITH replacement
(`selectables_registry.go`), so a single 2-entity table asked for 2 could return the same
goblin twice — invisible to the old discard-and-recompute code, a real bug once positions
are correlated back to specific goblins by ID. Fixed in rpg-api by giving each goblin its
own single-entity table/`EntityGroup` (these are individually pre-identified fixed
entities, not a random pick from a pool, so this is the more honest modeling, not a
workaround) — no toolkit issue filed for the sampling-with-replacement behavior itself
since rpg-api no longer depends on it, but any future caller wanting N>1 *distinct* random
picks from a shared table would hit the same gap.

Verified via `internal/orchestrators/lobby/start_encounter_test.go`'s
`TestStartEncounter_WalledRoomAndGoblins_CombatStartsOnSightedMove`: real `StartEncounter`
call → walled room + 2 goblins persisted, `Mode == FREE_ROAM` at spawn (not
`TURN_BASED`), then a rehydrated `enc.Move` onto a goblin's hex flips `Mode ==
TURN_BASED` **by rule** (the toolkit's own inline `checkCombatEntry`, not anything
rpg-api triggers) — no `devseed --inject-combat` involved. Stress-run 50x clean (room
generation and goblin placement are both randomized). This retires this doc's
"Known gap" note below about the combat-entry trigger being "future room/encounter-
design work" — see the correction inline. The full MCP playtest (web step 9, wave-1 step
9 in `ideas/the-dungeon/design.md`) still closes the wave; this PR is the api half only
(design doc steps 7-8).

~~**Playtest follow-up: live initiative roster broadcast on combat entry (rpg-api#644,
2026-07-15)** — the connect-time snapshot's `TurnState.InitiativeOrder` was the only
place a client ever learned the turn order; a mid-stream `FREE_ROAM` → `TURN_BASED`
transition (the toolkit's `rollInitiative`, inside `SetMode`) published no roster of its
own, so a client watching the fight start live saw an empty initiative list until its
next full snapshot. `internal/handlers/dnd5e/v2/encounter/handler.go`'s stream loop
synthesized a proto `InitiativeRolled` envelope (`order=[]string`, `EncounterEvent` oneof
field 41 — already defined on the wire, unused until now, zero proto changes) and sent
it right after the `ModeChanged` translation whenever a `ModeChangedEvent` transitioned
`To==TURN_BASED`; `translateForStream` returned a slice so one broker event could produce
two wire envelopes. Broadcast to every connected viewer, not just whoever moved.

**Known rough edge: the roster read races the orchestrator's save (rpg-api#647)** — every
combat-capable orchestrator verb follows a mutate-then-persist pattern where the toolkit
SDK call (e.g. `MoveEntity`'s `enc.Move(...)`) mutates state AND synchronously publishes
its broker events *before returning*, and only once it returns does the orchestrator
`Save` the result. The stream goroutine that wakes on the just-published `ModeChangedEvent`
could therefore call `h.encRepo.Get` before that `Save` landed, reading the pre-transition
(empty-`Initiative`) snapshot — reproduced reliably under `go test -race -count=10` before
the fix, intermittent without `-race`. Worked around with a bounded retry
(`loadRolledInitiative`, up to 15 attempts / 10ms apart / ~150ms worst case) rather than a
longer fixed delay, since the real `Save` was the very next thing the orchestrator did.~~
**RESOLVED by rpg-api#650 (2026-07-17): the toolkit publishes `InitiativeRolledEvent`
directly now (rpg-toolkit#765, PR #771 — the seam was introduced in encounter v0.28.0;
this repo pins whatever is current, see go.mod), sequenced between `ModeChanged` and
`TurnStarted` with its own real sequence number, broadcast to all players — no read-back,
no synthesis, no retry. Both the synthesis branch and `loadRolledInitiative` are deleted;
`translateInitiativeRolledEvent` is now a plain `TranslateEvent` case.** This closes ONE of
rpg-api#647's two known manifestations, not the issue itself: #647 still tracks the general
mutate-then-persist race class (the `InputRequiredDeliveredEvent` prompt-lookup path still
has it, untested under sustained `-race` load) and a related, distinct gap found during the
reconnect-fidelity wave investigation — `persistWithCharacterData`'s two sequential,
non-atomic Redis writes (character store, then encounter snapshot) — both still open.

**Playtest follow-up: live EntityAppeared carries entity type (rpg-api#644, 2026-07-15)** —
a monster (or player) becoming visible via a live `Move` (rpg-toolkit#762's monster
`EntityAppeared` emission) projected on the wire as `type=UNSPECIFIED` — a goblin rendered
as a generic capsule instead of its model. The toolkit's `EntityAppearedEvent` deliberately
carries only an entity ID + position (same minimal cause-event shape as the
DoorOpened/HexRevealed split already in this file), so the live translator built a bare
`{id, position}` Entity, leaving `type`/`hp`/`armor_class`/the character-or-monster oneof
unset. `handler.go`'s stream loop now branches on `EntityAppearedEvent` and calls the new
`translateEntityAppearedEventWithData`, which looks the entity up in freshly-loaded data
and builds the full Entity via `playerEntity`/`monsterEntity` — the monster-entity-building
code that used to live inline in `ProjectFor` is now `monsterEntity`, extracted so the
snapshot and live paths share one authoritative builder instead of drifting the way they
did before this fix (existing `TestProjectSuite` snapshot tests pin the extraction as
behavior-preserving). Unlike the `InitiativeRolled` read above, this one is NOT exposed to
the rpg-api#647 race class — the appearing entity's identity was always persisted by an
earlier, separate, already-completed request (`AddPlayer`/`AddMonster` inside
`StartEncounter` or `devcombat.Inject`); no retry needed. One subtlety caught by
`TestMovementSlicePerViewerProjection_AsymmetricLoS` while building this: the Entity's
`Position` must stay the EVENT's own reported position, not the entity's current stored
position — `ProjectVisibilityTransition` can report "appeared" at an intermediate hex of a
multi-hex pass-through move, not the mover's final resting position. **Same rough-edge
family as #647, worth flagging next to it:** unlike `ModeChanged` (once per fight),
`EntityAppeared` can now fire on every LOS-changing move during active combat
(rpg-toolkit#762's own scope includes "a player rounding a corner mid-fight") — if the
per-event repo read ever shows up as a real cost, the natural follow-up is caching the
connect-time snapshot's Players/Monsters in the stream goroutine's own state instead of
round-tripping Redis per appearance; not built now, flagged here so it's discoverable
rather than re-derived.

**Active battlefield conditions survive reconnect (rpg-api#651, 2026-07-17)** —
`PlayerData.ActiveConditions` / `MonsterData.ActiveConditions` (rpg-toolkit#754,
encounter v0.29.0+) are now projected into `Entity.status_effects` at snapshot time,
in the shared `playerEntity`/`monsterEntity` builders (`project.go`'s
`statusEffectsFrom`), matching the live `translateConditionAppliedEvent`'s
`StatusEffect{Source: conditionRefFor(...)}` shape exactly (via the same
`conditionRefFor` helper) so a reconnecting client's snapshot and a
continuously-connected client's stream agree on wire shape. `ActiveConditions`
itself needed no filtering on the rpg-api side: it's already narrowed toolkit-side
(rpg-toolkit#778, encounter v0.29.1) to exclude conditions attached permanently at
character/monster construction (class-grant passives like a Monk's `MartialArts`,
monster traits like `PackTactics`) — a namespace filter (`dnd5e:conditions:*` vs
`dnd5e:monster_traits:*`) provably could not make this distinction (`MartialArts`
and `Raging` are both `dnd5e:conditions:*`; see rpg-toolkit#778's investigation
trail), so rpg-api trusts the toolkit's attachment-provenance-aware filter
completely rather than re-deriving it.

Verified end to end via `integration_conditions_projection_test.go`
(`ConditionsProjectionIntegrationSuite`): a raging character's connect snapshot
carries the rage status effect with zero live events involved (no verb called in
the test — the character fixture starts already-raging, mirroring
rpg-toolkit's own `active_conditions_test.go` "NoVerbCalled" pattern, which is
the honest shape of "hydrated after a restart"); a Monk fixture with a real,
round-tripped `MartialArts` condition proves the provenance-chain filter holds
end to end (confirmed as a true positive, not an empty pipe — verified the
condition genuinely round-trips into `character.Data.Conditions` while
`ActiveConditions` still excludes it); and ending an encounter (via a real kill,
not a synthetic monster-removal shortcut) sweeps the rage badge from the
post-end snapshot, pinning the `checkEncounterEnd`/`endCombatForPlayers`
(rpg-toolkit#767/#752) sweep-before-persist ordering.

**Known rough edge: `ActiveConditions` can lag a just-activated feature until the
next combat-capable verb — root cause is toolkit#691, rpg-api's projection is
faithful to what's persisted:** `Encounter.ActivateFeature` deliberately does not
use the shared #689 hydration cascade
(`internal/orchestrators/encounter/v2/activate_feature.go`'s doc comment; tracked as
the OPEN toolkit#691, to avoid a double-subscribe collision with `ActivateFeature`'s
own `CharDataJSON` self-load — the #684 class). `ProjectFor` — the function behind
both `StreamEncounter`'s connect snapshot and `GetEncounter` — never calls
`enc.ToData()` itself (confirmed by direct read: zero `ToData()`/`Save()` calls in
`project.go`); it projects `PlayerData.ActiveConditions` exactly as last persisted,
for every viewer including the active actor. So a feature activated via
`ActivateFeature` IS correctly persisted to the character store immediately, but
does NOT appear on any snapshot — not even the activating character's own
reconnect — until some OTHER combat-capable verb (`MoveEntity`, `TakeAction`,
anything using the #689 `WithCharacterData: true` cascade) runs: that cascade
attaches and holds EVERY player's fresh character blob, not just the acting one
(`attachPlayerCharacterData` loops `data.Players` unconditionally), so its own
persist backfills `ActiveConditions` for every held player as a side effect, not
just whoever the verb targeted. The precise trigger map is "activate a feature,
then read a snapshot with zero combat-capable verb calls in between" — connecting
does not itself sync, so there is no self-heal via reconnect alone. Not fixed
here — out of scope for #651, which is the projection layer; the fix belongs in
toolkit#691.

**Wave-close blocker: MoveEntity silently no-op'd in StartEncounter-created
encounters (rpg-api#656, 2026-07-18)** — found by the reconnect-fidelity wave's
closing playtest: a `MoveEntity` RPC on a fresh, wall-free-nearby, lobby-started
encounter returned success but the entity never moved (`actualPath` a single
zero-length step). Root cause, confirmed by an isolated reproduction (no gRPC,
no Redis — a standalone Go program calling the toolkit directly) then verified
against the exact bug-report hex: `StartEncounter` spawns the party at raw cube
coordinate `{0,0,0}`. `core.Hex.ToPosition()` maps that DIRECTLY to the room's
offset-coordinate origin `(0,0)` — `InitRoom`'s room (`environments.QuickRoom`)
spans offset `[0,roomWidth) x [0,roomHeight)` with no auto-centering, so the
party spawns at the room's CORNER, not its center. The toolkit's `Move()`
unconditionally truncates the requested path the instant a step's offset
position falls outside those bounds (`space.go`'s `truncateAtWall`,
indistinguishable from hitting a real wall — rpg-toolkit#757), and roughly half
of the six hex movement directions from a corner spawn immediately do. This
predates #650/#655 entirely — it has existed since `InitRoom`'s introduction in
wave-1 (rpg-api#644) and was never triggered because earlier playtests happened
to move in one of the safe directions; the coverage hole is that every existing
integration test seeds encounters directly via `tkenc.New`/`AddPlayer`
(bypassing `StartEncounter`'s `InitRoom` + corner-spawn combination entirely),
so none of them could have caught a movement-direction-specific bug. Fixed by
spawning the party at the room's geometric center (`roomCenterHex()`,
`start_encounter.go`) instead of raw cube-origin — verified the fix directly in
the same isolated reproduction before touching production code. Closing the
loop required a NEW integration test exercising the actual failing path
(`LobbyService.StartEncounter` RPC → `EncounterService.MoveEntity` RPC,
`lobby_start_then_move_test.go`) — the class of test #656 asked for by name.
Fixing the corner-to-center spawn move also exposed a second, pre-existing,
narrower issue: `TestPartyAssembles_FourPlayers_CreateJoinReadyStart`'s mutual-
visibility assertion (every party member must see every OTHER member's exact
spawn hex) happened to hold at the room's corner for this room's random wall
layout but not at the center, where one wall sits between two spawn hexes.
That assertion was always stronger than its own stated purpose (guarding
rpg-api#632 — SightRange never being seeded) requires; wall-free line-of-sight
between every pair of spawn positions was never actually guaranteed by the
room generator, at the corner or anywhere else. Loosened to check SightRange
is seeded and RevealedHexes covers a real radius (rather than the mover's own
single hex), which is what #632 needs guarded, without re-introducing
wall-placement fragility. Not pursued: a wall-aware, verified-safe party
spawn search (mirroring `seedGoblins`' `perception.CanSeeAt`-verified
placement) — out of scope for this fix; flagged here if mutual spawn
visibility ever becomes a real product requirement.

**The live v1alpha1 encounter stack is DELETED (rpg-api#642, 2026-07-13)** —
the audit-flagged registered-and-serving `dnd5e.api.v1alpha1.EncounterService`
(`cmd/server/server.go:233`), its 5,844-line `internal/orchestrators/encounter/
orchestrator.go`, the v1-only entities (`entities.Dungeon`, the v1 combat
domain model, `entity_state.go`/`encounter_state_builder.go` proto converters),
and the v1-only repos/publisher/processor (`repositories/dungeons`,
`repositories/encounterlog`, the v1 root of `repositories/encounters`,
`publishers/encounter`, `processors/event`) are all gone — rpg-api's twin of
rpg-dnd5e-web#448's clean slate. The web made zero v1alpha1 encounter RPC
calls (confirmed before deletion), so this was pure dead-weight removal, not a
behavior change. `internal/components/dungeon` (audit debt #5 — belongs in
rpg-toolkit under the Boundary Rule) was explicitly left untouched: it still
has a live consumer in `internal/components/spawner` and its own toolkit
tests, so it did not become fully dead as a side effect.
**This retires every "Known rough edge" and "Upcoming work" item below that
named the deleted orchestrator, handler, or repos** — see the strikethrough
notes in those sections rather than assuming they still apply. It also moots
the six open Round 2 coordinate-space PRs (#459–#468, see "Paused / on
hold"): they all patched a file that no longer exists.

**NPC-first initiative no longer stalls the encounter (rpg-api#636, 2026-07-11)** —
`Orchestrator.EndTurn`'s NPC dispatch loop only ever ran as the tail of an `EndTurn`
call — but when a TURN_BASED encounter's very FIRST active actor is an NPC (today:
`devseed --inject-combat` rolling the goblin first; the future real combat-entry
trigger doing the same), there is no preceding `EndTurn` to chain off of: every player
is correctly locked out (not their turn) and nothing ever calls `NPCAct`. The
encounter stalled forever.

Fix: the NPC dispatch loop is factored out of `EndTurn` into a shared
`Orchestrator.driveNPCChain` (`internal/orchestrators/encounter/v2/end_turn.go`) that
owns every persist on every exit path, so both `EndTurn` and a new
`Orchestrator.DriveStalledNPCTurn` (`drive_npc.go`) can run the identical
NPCAct+EndTurn cycle. `DriveStalledNPCTurn` is the "combat-entry kick": it loads the
encounter and, if the active actor is an NPC, drives it (and any chained NPC turns
after it) until a player is active, the encounter ends, or the chain cap is reached.
`StreamEncounter` and `GetEncounter` (`handler.go`, `get.go`) both call it — as
best-effort, error-swallowing self-heals — at the top of every connect-time read,
since those are the only "first touch" available for an out-of-process write like
today's devseed injection (the server never otherwise observes combat starting).
`StreamEncounter` kicks BEFORE `Subscribe` (not after) so the kick's own NPCAct/
EndTurn broker events fan out live to already-connected viewers without also
replaying as a redundant post-snapshot event for the connecting client itself — see
the handler's doc comment for the full ordering rationale.

Concurrency: up to N clients can hit `StreamEncounter`/`GetEncounter` for the same
encounter at once (e.g. four players reconnecting together), and the toolkit's own
"am I still the active actor" guard inside `NPCAct` only protects the in-memory copy
the CALLER loaded — it does not protect across independently loaded snapshots. A new
per-encounter `keyedMutex` (`keyed_mutex.go`, duplicated from the lobby
orchestrator's identical single-process pattern rather than shared across packages)
single-flights `DriveStalledNPCTurn` per encounter ID: only the first caller does
real work; every other concurrent caller blocks, then re-loads the
already-persisted post-drive state and no-ops.

The future real (server-side, in-process) combat-entry trigger does NOT need the
`StreamEncounter`/`GetEncounter` kick at all — it can (and should) call
`driveNPCChain` directly right after its own `SetMode`, since it IS the server
observing combat start. The kick exists specifically for paths that bypass normal
server verbs, today's devseed injection being the only one.

`devcombat.Inject` gained a `ForceNPCFirst` input (`internal/pkg/devcombat/inject.go`)
that deterministically reorders the freshly-rolled Initiative so the injected goblin
leads it, instead of depending on a lucky roll to reproduce the stall — exposed as
`devseed --inject-combat --inject-combat-npc-first` for manual playtest repro, and
used by the new `TestPartyAssembles_FourPlayers_ThenCombatEntry_
NPCFirstInitiative_StreamDrivesNPCTurn` integration test
(`internal/integration/lobby_v1alpha1_test.go`), which drives the fix through the
real `StreamEncounter` RPC end-to-end (not a direct toolkit/repo manipulation).

**LobbyService v1alpha1 — party assembly + sole encounter construction path (rpg-api#629, 2026-07-07)** —
New `internal/handlers/dnd5e/lobby/v1alpha1` → `internal/orchestrators/lobby` →
`internal/repositories/lobby` vertical, layered from day one (Chapter-1 discipline, not
the handler-package shape #616 flags). Six RPCs: `CreateLobby`, `JoinLobby` (idempotent,
rebinds `character_id`), `SetReady`, `LeaveLobby` (host migrates to oldest remaining
member), `StartEncounter` (host-only, all-ready gated, atomic member snapshot via a
per-lobby `sync.Mutex`), `StreamLobby` (snapshot-first, presence keyed off subscribe/
disconnect — a dropped stream is NOT `LeaveLobby`). Redis-backed repo (`lobby:` prefix
+ `lobby:joinref:` secondary index, 24h TTL) plus an in-memory variant for tests. A
small lobby-scoped `Broker` (`internal/orchestrators/lobby/broker.go`) fans out
`StreamLobby` events — deliberately simpler than the toolkit's per-viewer encounter
broker since a lobby roster has no line-of-sight concept.

**`StartEncounter` subsumes the deleted `EncounterService.CreateEncounter` RPC**
(v1alpha2, `dnd5e/api/v1alpha2/encounter`): it is now the SOLE path an encounter comes
into existence. It builds the toolkit encounter, seeds each ready member's HP from the
character store (generalizing the old handler-layer `seedPlayerHP` — rpg-api#612 — to
N members), persists once to `enc:v2:<id>`, transitions the lobby to STARTED, and only
then publishes `EncounterStarted` (persist-then-emit is load-bearing). The old
`internal/handlers/dnd5e/v2/encounter/create.go` + its HP-seed test are deleted;
`internal/integration/encounter_v2_test.go`'s old `TestCreateEncounter_Basic` is
replaced by the new `internal/integration/lobby_v1alpha1_test.go`'s 4-player
create/join/ready/start flow (the boarded "Party Assembles" gate test, umbrella
KirkDiggler/rpg-project#81). devseed is unaffected — it always wrote straight to Redis,
bypassing the RPC surface.

Known gap: `StartEncounter`'s proto contract carries no `initial_mode` field (the old
`CreateEncounter` could start TURN_BASED directly) — every lobby-constructed encounter
starts FREE_ROAM. No production caller depended on the old field; flagged as a design
gap, not fixed here. `devseed --inject-combat` (below) gives local/MCP playtesting a way
to add a monster and flip TURN_BASED on a lobby-started encounter without a proto change.
**Correction (rpg-api#644, 2026-07-15): the real combat-entry trigger has landed** — see
"The Dungeon wave 1" entry above. `StartEncounter` now seeds real goblins into a real
walled room and combat starts by rule (a `Move` that forms sight), not just via
`devseed --inject-combat`; the dev-inject tool remains for playtest control (design doc:
"the inject tool stays for playtest control"), not as the only path anymore.

**A real fight on GameView — combat-ready lobby members + devseed injection (rpg-api#634, 2026-07-11)** —
`StartEncounter` now seeds each member's `tkenc.PlayerInput.AC` honestly from
`character.Data.ArmorClass` (`seedMemberCombatSnapshot`, `internal/orchestrators/lobby/
character.go`) — the character is already fetched for HP. `AttackBonus`/`DamageDice`/
`DamageType` stay zero-value: a stored character carries no precomputed field for them
(they're derived at attack time from equipped weapon + ability scores + proficiency
bonus — real rules math rpg-api must not duplicate). This used to be a hard block: the
toolkit's `isPlayerCombatant` gate rejected TakeAction for every lobby-created player
(`ErrNonCombatant: missing HP/AC/DamageDice`) even though the v2 encounter
orchestrator's existing `characterData.Attach` cascade (`hydrate_players.go`) already
rehydrates every lobby-created player from the character store — keyed off
`PlayerData.EntityID`, which `StartEncounter` already sets — on every combat-capable RPC
(TakeAction/EndTurn/MoveEntity/SubmitCheckReaction). The real `Dnd5eCombatResolver`
ignores the flat AC/DamageDice snapshot entirely once a seat is hydrated (it drives
damage off the held `*character.Character`'s real weapon via
`Character.WeaponForActionRef`); the flat snapshot only feeds the stand-in fallback for
an un-hydrated seat. **Fixed in rpg-toolkit encounter v0.24.4** (rpg-toolkit#750/#751,
merged 2026-07-11): `isPlayerCombatant` (now an `*Encounter` method) treats an actually
HELD seat (`e.heldCharacter(...) != nil`) as combat-ready, independent of the flat
snapshot — deliberately not `len(DataJSON) > 0`, since DataJSON being set doesn't mean a
seat has been hydrated (only a `LoadFromData` round-trip's cascade does that; a Copilot
review catch on the toolkit PR). `go.mod` bumped to `v0.24.4`; the integration test
proving the full path end-to-end (`TestPartyAssembles_FourPlayers_ThenCombatEntry_
AttackResolves` in `internal/integration/lobby_v1alpha1_test.go`) now passes for real, no
skip, no replace directive.

`cmd/devseed` gains `--inject-combat --encounter-id=<id>`: loads the EXISTING encounter
(does not rebuild it), adds a goblin (`monster.NewGoblin`, same DataJSON pattern every
other devseed fixture uses), flips to TURN_BASED (skipped if already turn-based, so
re-running mid-fight just adds another goblin rather than erroring on the toolkit's
redundant-`SetMode` rejection), and saves. The actual load/AddMonster/SetMode/save logic
lives in `internal/pkg/devcombat` (`Inject`, Input/Output-typed) rather than in
`cmd/devseed/main.go` directly — `package main` can't be imported, and both the CLI flag
and `internal/integration/lobby_v1alpha1_test.go`'s combat-entry test call the identical
code path. Known acceptable gap, documented in `devcombat.Inject`'s doc comment: a
monster added this way doesn't get the `monstertraits.LoadMonsterConditions`/OA-reaction
wiring the toolkit's `LoadFromData` cascade applies to monsters present at load time —
cosmetic for a first fight; the goblin is fully rehydrated (OA included) on the next
combat-capable RPC's own load.

**Test harness gap fixed alongside #634**: `internal/integration/harness`'s v1alpha2
`EncounterService` handler was wired with no `CombatResolverConfig`/`CharacterRepo` at
all (unlike `cmd/server/server.go`'s production wiring), so the `characterData.Attach`
hydration cascade silently never ran in ANY existing integration test — every prior
combat test worked only because it seeded explicit `AttackBonus`/`DamageDice` snapshot
fields directly, never needing hydration. Fixed to mirror production; verified the full
`internal/integration/...` suite still passes.

**Chapter 1 (Architecture Honesty) — v2 encounter orchestrator carve COMPLETE (#582, 2026-05-31)** —
The v2 encounter vertical now runs through a clean `internal/orchestrators/encounter/v2`
orchestrator: one method per RPC, each doing exactly one `load → toolkit-verb → persist`.
The handler-package `Runner` + scattered inline verb bodies are retired. EndTurn was the
final verb (step 7): `Orchestrator.EndTurn` owns the NPC-dispatch loop, the (post-#689 clean)
turn-end reset via `enc.EndTurn`, and the pause-for-reaction prompt bookkeeping. The handler
`end_turn.go` is now the ~25-line canonical shape (auth + envelope + `EndTurnInput` + delegate
+ `endTurnStatusError`). The orchestrator stays rulebook-free: the one rulebook-touching piece
on the NPC pause path — marshaling the opaque `*combat.AttackContext` — reuses the existing
injected `ReactionResume.MarshalAttackContext` adapter (the same seam TakeAction phase-1 uses).
Depguard now denies `rulebooks/dnd5e/combat` in the guarded handler + orchestrator files (the
last `combat` import left those files with the EndTurn carve), locking the boundary shut.
Behavior parity is proven by the unchanged handler `TestEndTurn_*` suite; new orchestrator-level
`EndTurnSuite` + `EndTurnPauseSuite` cover load/verb/persist + the prompt-bookkeeping helpers.
**Note:** the cross-RPC reaction wire-pause remains DEFERRED — the NPC reaction step stays
internal (resolved via the already-carved `SubmitReactionCheck` single-RPC). #582 closes after
the playtest sign-off.

**Chapter 2 Wave 2 (monk) — Entity.armor_class populated in v2 snapshot (2026-05-29)** —
`ProjectFor` now sets `Entity.armor_class` for both players (`PlayerData.AC`) and monsters
(`MonsterData.AC`) when the value is non-zero. Charli's AC=15 (UnarmoredDefense) and the
goblin's AC=15 flow through the snapshot wire so the playtest harness can render them (closes #562).
Two new `ProjectSuite` tests: `TestProjectFor_PlayerEntityCarriesArmorClass` (charli AC=15)
and an assertion on `TestProjectFor_TurnBased_EmitsModeTurnStateAndMonsters` (goblin AC=15).

**Chapter 2 Wave 2 (monk) — devseed fixture + v2 unarmed-strike integration test (2026-05-28)** —
`--fixture=wave-2-monk` seeds charli (L1 monk, DEX 16/WIS 14/CON 14, HP 10, AC 15, no weapon,
MartialArts + UnarmoredDefense conditions persisted) + goblin adjacent into `enc:v2:dev-encounter`
(closes #559). `TestMonkUnarmedIntegrationSuite` proves the full MartialArts chain via the real
`TakeAction` RPC: unarmed strike uses DEX mod (+3) not STR (+0), 1d4 damage die applies, and
the `dnd5e:abilities:dex` source appears in `DamageDealtEvent.Components` (closes #560).

Root bug fixed: `buildEncounterCharacterRegistryFromResolved` was calling `reg.Add` (weapons)
but never `reg.AddAbilityScores`. Without ability scores in the registry, `MartialArtsCondition`
silently no-ops on `onDamageChain`. Fix adds `characterToGamectxAbilityScores` helper and two
`AddAbilityScores` calls (attacker + target). No toolkit changes needed.

**Chapter 2 Wave 1 (rogue) — devseed fixture + L1 SA integration test (2026-05-27)** —
`--fixture=wave-1-rogue` seeds alice (L1 rogue, 1d6 SneakAttack, HP 10) + bob (L1 barbarian) +
goblin into `enc:v2:dev-encounter` (closes #551). `TestSneakAttackIntegrationL1Suite` proves the
full SA chain (gamectx wiring, once-per-turn enforcement, turn-end reset) end-to-end via the real
`TakeAction` RPC for L1 alice with ally-adjacent bob — same 5-test scenario as the existing L2 suite,
parameterized by `rogueLevel` (closes #552).

**Wave 2.11e rpg-api — PR open (2026-05-25)** — MovementResolver wiring: OA-class
reactions end-to-end in both movement directions (player retreats past NPC → NPC OA;
NPC retreats past player → player OA) plus Disengage suppression. Pins
`encounter@v0.14.0` + `rulebooks/dnd5e@v0.59.0`.

What landed on the rpg-api side (issue #539):

- `Dnd5eMovementResolver` (`dnd5e_movement_resolver.go`) — implements
  `tkenc.MovementResolver` via `combat.MoveEntity` per step. Builds
  `encounterHexRoom` + `CombatantRegistry` + `gamectx` per step so the OA
  chain (MovementChain → OpportunityAttackCondition → triggerOpportunityAttack →
  combat.ResolveAttack) runs with correct spatial + readiness context.
- `encounterHexRoom.MoveEntity` (`hex_room.go`) — write path added; mutates
  `data.Players[*].View.Position` / `data.Monsters[*].Position` per step so
  successive ResolveStep calls see updated positions.
- `encounterHexRoom.GetGrid()` returns `SquareGrid` (required by
  `OpportunityAttackCondition.isLeavingMyThreatRange`).
- `applyMonsterReactionConditions` (`reaction_conditions.go`) — wires OA
  condition on monsters at rehydration time (without this, NPC-direction OA
  never fires because the condition's bus subscription never installs).
- `HandlerConfig.MovementResolverConfig` + `buildMovementResolver` — wires the
  resolver at all 7 `LoadFromData` / `New` sites in the handler.
- 3 integration tests (`integration_movement_oa_test.go`):
  - `TestPlayerOA_OnNPCFleeing` — goblin retreats via `MoveNPCSteps`, alice's OA fires, goblin HP drops.
  - `TestNPCOA_OnPlayerFleeing` — alice retreats via `MoveEntity` RPC, goblin's OA fires, `DamageDealtEvent` verified.
  - `TestRogueDisengage_NoOAFiresOnMovement` — alice activates `BonusDisengage` on the encounter's bus, retreats, no OA fires. Documents cross-RPC persistence gap.

Known follow-ups (filed separately, out of Wave 2.11e scope):
- Disengage cross-RPC persistence (condition evaporates on LoadFromData per-RPC bus reconstruction).
- Player-pause reactions / Sentinel (rpg-toolkit#665).
- `v2 ActivateCombatAbility` RPC for Disengage.
- Weapon-aware OA attacker (`triggerOpportunityAttack` uses unarmed strike placeholder).

**Earlier active state (still relevant):**

**MOOT as of #642 (2026-07-13):** all six PRs below patch
`internal/orchestrators/encounter/orchestrator.go`, `dungeon_mapper.go`, and
related v1-only files, all of which are now deleted. None of these branches
apply cleanly against current main and none should be merged — the fixes they
carry either need to be re-derived against the v2 stack (if the underlying bug
still exists there) or are simply moot. Left listed here for the historical
record rather than silently dropped; closing them is a follow-up action, not
done in this PR.

| PR | Branch | Status | Notes |
|----|--------|--------|-------|
| #459 | `fix/458-room-origins-monster-turns` | Open — MOOT | targeted the deleted orchestrator.go |
| #461 | `fix/open-door-monster-absolute-positions` | Open — MOOT | targeted the deleted orchestrator.go |
| #463 | `fix/462-room-layout-wall-absolute-positions` | Open — MOOT | targeted the deleted orchestrator.go |
| #466 | `fix/open-door-room-data-persist` | Open — MOOT | targeted the deleted orchestrator.go |
| #467 | `fix/room2-missing-perimeter-walls-465` | Open — MOOT | targeted the deleted dungeon_mapper.go |
| #468 | `round2/multi-room-dungeon` | Open — MOOT, was CI FAILING | Consolidated branch; targeted deleted files |

## Recently landed (since Round 1, highlights)

- **Unified entity state** (PR #453, 2026-04-05) — `Entities` map added to
  `EncounterData`; `BuildEncounterStateData` snapshot builder; all events
  wire entity state through.
- **Entity state wired to events** (PR #454, 2026-04-05) — `AttackResolved`,
  `TurnEnded`, `FeatureActivated`, `MovementCompleted`, room reveal events all
  carry `EntityStateData`. Copilot feedback addressed post-merge.
- **Integration test: OpenDoor → RoomRevealed stream** (commit `176b501`) —
  first integration test proving the full event-stream pipeline for multi-room
  reveal.
- **Room layout protos wired through encounter state builder** (commit `87d015c`)
  — `RoomLayout` now returned in `GetEncounterState`.
- **Round 1: Monk kills monster** — all issues merged by 2026-04-05; combat
  loop, action economy, monster turns, event publishing all working end-to-end
  for a single-room dungeon.

## Paused / on hold

- **Round 2 consolidated branch (#468)** — MOOT as of #642: targeted the now-deleted
  v1 orchestrator. See the PR table above.
- ~~**Debug-walls theme** — `dungeon_mapper.go:44` had a hardcoded
  `ThemeDebugWalls` where `ThemeCrypt` should be.~~ Moot: `dungeon_mapper.go` was
  deleted with the v1 orchestrator (#642).
- ~~**PlayerDisconnected** — the streaming handler exits but the `PlayerDisconnected`
  orchestrator method is never called, so no encounter state cleanup fires.~~
  Moot: the v1 handler and orchestrator that owned this method are deleted (#642).
  If the v2 path needs equivalent cleanup, it's a fresh gap, not a carried-over one.
- **Spell/trait enum conversions** — multiple `TODO` comments in
  `handlers/.../character/converters.go` where `SPELL_UNSPECIFIED` and
  `TRAIT_UNSPECIFIED` are returned because proto enums are not yet mapped.
- **REFACTOR_PLAN.md** (repo root) — equipment-choice business logic still lives
  in the handler/external client instead of the orchestrator. Flagged but
  deferred since early in the project.

## Known rough edges

### ~~Boundary violations — proto types in the orchestrator~~ RESOLVED by deletion (#642)

`internal/orchestrators/encounter/orchestrator.go` and `service.go` — the files
this section described — are deleted. The proto-contaminated Input/Output types
died with them. The v2 orchestrator (`internal/orchestrators/encounter/v2/`)
never imported proto in the first place.

### Boundary violation — dungeon component belongs in toolkit

`internal/components/dungeon/` implements procedural dungeon generation
(room shapes, wall perimeters, door spawning, hex-grid layouts, monster placement).
This is game logic, not data orchestration. The CLAUDE.md boundary rule is explicit:
"If it's a game mechanic or calculation → rpg-toolkit." The component is correctly
identified as toolkit-bound in project memory but has not moved.

Affected path: `internal/components/dungeon/`

### ~~Boundary violation — toolkit types leak into encounter handler~~ RESOLVED by deletion (#642)

`internal/handlers/dnd5e/v1alpha1/encounter/handler.go` — the file this
section described — is deleted along with the rest of the v1alpha1
EncounterService.

### ~~Coordinate-space fragility~~ RESOLVED by deletion (#642)

The bug class (room-local coordinates treated as absolute dungeon-space
coordinates) lived entirely in the now-deleted `DungeonStart`, `OpenDoor`,
`buildRoomLayoutProto`, entity map, and `mergeNewRoomMonsters` functions of the
v1 orchestrator. Related PRs #459, #461, #463, #466, #467 are moot — see
"Paused / on hold" above. If the v2 path develops an equivalent bug class, it's
a fresh problem against fresh code, not a continuation of this one.

### ~~Encounter + dungeon repositories are in-memory only~~ RESOLVED by deletion (#642)

`internal/repositories/encounters/inmemory.go` (v1 root) and
`internal/repositories/dungeons/inmemory.go` are deleted. The surviving
`internal/repositories/encounters/v2/` has both Redis and in-memory
implementations, and is the one wired into production (`cmd/server/server.go`,
24h TTL).

### ~~Orchestrator size and complexity~~ RESOLVED by deletion (#642)

`internal/orchestrators/encounter/orchestrator.go` (5,844 lines) is deleted.
The v2 orchestrator (`internal/orchestrators/encounter/v2/`) is one file per
RPC, each doing exactly one `load → toolkit-verb → persist` — the opposite
shape of the file this section described.

### ~~Publish failures silently swallowed in event processor~~ RESOLVED by deletion (#642)

`internal/processors/event/processor.go` is deleted — it had zero consumers
left once the v1 orchestrator and handler were gone. The v2 path publishes
through the toolkit's own `tkenc.Broker`, a different mechanism not affected
by this gap.

### Handler TODO cluster in character handler/converters

`internal/handlers/dnd5e/v1alpha1/character/handler.go` has 8 TODO comments.
`internal/handlers/dnd5e/v1alpha1/character/converters.go` has 27 TODO
comments (verified by grep on docs/honest-status-snapshot branch — original draft said 20). Most are about incomplete proto enum mappings (spells, traits, tool
proficiencies, subraces, languages). Some responses return stub/zero values
today.

### ~~Debug theme not restored~~ RESOLVED by deletion (#642)

`internal/orchestrators/encounter/dungeon_mapper.go` is deleted with the rest
of the v1 orchestrator.

## v1alpha2 encounter service (Wave 2.5 slice 1 — superseded)

Walking skeleton wired through the rpg-toolkit `encounter` SDK. `MoveEntity` and
`StreamEncounter` implemented; all other RPCs return `codes.Unimplemented`. Per-viewer
event projection works end-to-end (verified by `internal/integration/encounter_v2_test.go`).

**Stale note (2026-07-07), now resolved (2026-07-13):** this section predated the many
verbs that shipped since (TakeAction, EndTurn, Interact, SubmitCheck, SetReactionReady,
ActivateFeature all exist now — see `internal/orchestrators/encounter/v2/`) and the
removal of `CreateEncounter` (see the LobbyService entry above — `StartEncounter` on
`LobbyService v1alpha1` is the sole construction path now). The "deletion deferred until
web migrates" note below is also resolved: #642 deleted the v1alpha1 encounter path
outright, confirmed via grep that the web made zero v1alpha1 encounter RPC calls. A full
content refresh of this section (it still describes a walking-skeleton state years out of
date) remains a separate cleanup, not done in this PR.

Known gaps: rpg-toolkit#629 (LoS-loss events when entity moves out of viewer range — slice
1 wave goal uses mutual LoS so this doesn't bite). `SnapshotDelivered.encounter` proto
field is empty for slice 1; toolkit `Snapshot` shape will be mapped when slice 2 needs it.

## Per-subsystem confidence

See [quality.md](quality.md) for grade and rationale.

| Subsystem | Confidence |
|---|---|
| Character handler | Medium — large converter surface with many TODO stubs |
| Encounter v2 handler/orchestrator | Medium-high — one file per RPC, load→verb→persist, no proto leakage; see quality.md |
| Character orchestrator | Medium-high — smaller, well-tested |
| Dungeon component | Medium — good tests, wrong repo; toolkit boundary violation |
| Spawner component | Medium — thin, functional |
| Encounter repository v2 (Redis + in-memory) | Medium-high — persistent backend, tested, production-wired |
| Character repository (Redis) | Medium-high — only persistent game-state store predating the v2 vertical |
| Integration test harness | Medium-high — good coverage of happy paths across character/lobby/v2 encounter |
| Services layer (sandboxroom) | Low — sparse; most business logic lives in orchestrators |

~~Encounter handler | Encounter orchestrator | Event processor | Redis publisher |
Encounter repository (in-memory) | Dungeon repository (in-memory) | Encounter log
repository (in-memory)~~ — all deleted with the v1alpha1 stack (#642).

### Lint — 29 pre-existing violations (was 70, before #642)

`make pre-commit` fails on lint with 29 pre-existing violations as of 2026-07-13,
down from 70 before the v1alpha1 encounter stack deletion (#642) — 41 fewer, mostly
`goconst` (44→19, duplicated proto/action-id string literals lived almost entirely
in the deleted orchestrator) plus a handful of `unused`/`gocritic`/`revive` findings
that died with their files. All tests pass. Every remaining issue was verified
present (same description, shifted line number) in the pre-deletion baseline — this
PR introduced zero new lint issues. Categories:

- **goconst (19)** — magic string literals repeated 3+ times in dungeon toolkit and character converters
- **govet (3)** — error variable shadowing in harness and integration helpers
- **unconvert (3)** — unnecessary string() conversions in character handler
- **staticcheck (1)** — `grpc.DialContext` deprecated in test harness
- **errcheck (2)** — unchecked errors in harness.Close()
- **unused (1)** — dead helper in `internal/integration/encounter_v2_test.go`

## Upcoming work

- ~~**Start fresh from main for Round 2**~~ — moot: the six coordinate fixes
  targeted the deleted v1 orchestrator (#642). See "Paused / on hold."
- **Move dungeon component to rpg-toolkit** — tracked decision, not yet an
  open issue. Unaffected by #642 — `internal/components/dungeon` was
  deliberately left untouched (still has a live consumer in
  `internal/components/spawner`).
- ~~**Redis implementations for encounter + dungeon repos**~~ — moot: the
  in-memory-only v1 repos are deleted; `repositories/encounters/v2` already
  has a Redis implementation and is what's wired into production.
- ~~**Eliminate proto types from orchestrator**~~ — moot: the orchestrator
  that had this problem is deleted; the v2 orchestrator never imported proto.
- **Character proto enum gaps** — spells, traits, subraces, languages all
  return stub values today. Unaffected by #642 (v1alpha1 CharacterService
  was never in scope for this deletion).
- ~~**PlayerDisconnected orchestrator hookup**~~ — moot: the handler/orchestrator
  pair that had this gap is deleted.
- ~~**Revert `ThemeDebugWalls` to `ThemeCrypt`**~~ — moot: `dungeon_mapper.go`
  is deleted.

## Related references

- [rpg-project/CLAUDE.md](https://github.com/KirkDiggler/rpg-project/blob/main/CLAUDE.md) — cross-repo Boundary Rule
- [Project board #10](https://github.com/users/KirkDiggler/projects/10)
- [REFACTOR_PLAN.md](../REFACTOR_PLAN.md) — equipment-choice refactor deferred
- [docs/architecture/](architecture/) — existing architecture docs
