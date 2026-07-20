// Package encounter is the v2 encounter handler.
// project.go translates a toolkit encounter.Data into the v1alpha2
// proto Encounter message for a specific viewer.
package encounter

import (
	"context"
	"sort"
	"time"

	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/encounter/perception"

	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

// ProjectFor builds a proto *encounterv2pb.Encounter for the given viewer from
// the encounter's persisted data. GetEncounter (#500) and StreamEncounter
// snapshot replay (#497) call it so the projection logic lives in exactly one
// place.
//
// The broker is required to rehydrate a live Encounter via LoadFromData
// (needed for SnapshotFor).
//
// charRepo serves two purposes, both by-key lookups against the character
// store rather than anything rules-touching:
//  1. Hydrates the turn's ACTIVE actor so the snapshot's TurnState can carry
//     that actor's economy + available_actions menu at turn start (#601: the
//     client needs the menu to take its FIRST action, before any
//     TurnStateChanged push fires). The toolkit computes the menu
//     (ActorTurnState); rpg-api projects it.
//  2. Resolves EVERY projected player entity's display_name/class_ref
//     (rpg-api#664) via characterIdentityFor — not just the active actor.
//
// nil disables both (tests / non-combat callers): the snapshot then carries
// initiative/active/round only and player entities carry no
// display_name/class_ref, as before #664.
//
// now is passed explicitly so callers in tests can inject a fixed clock.
func ProjectFor(
	ctx context.Context,
	data *tkenc.Data,
	viewer core.PlayerID,
	broker *tkenc.Broker,
	charRepo characterrepo.Repository,
	now time.Time,
) (*encounterv2pb.Encounter, error) {
	// #601 turn-start menu: attach the ACTIVE actor's character blob before
	// LoadFromData so the SDK holds that one character and ActorTurnState below
	// returns its menu + economy. Only the active actor is hydrated — the menu is
	// that actor's private view (Inv 6). No-op when not turn-based, no active
	// actor, or no char store (then the snapshot carries no menu, as before).
	// Other seats stay un-hydrated, so Space.Entities still reads positions/HP
	// from Data directly (not held entities) exactly as before.
	//
	// Audience gate (Copilot #602): the menu/economy is the active actor's PRIVATE
	// view — the same audience-seam the live TurnStateChanged push uses (its
	// audience is the controlling player only). So project it ONLY when this
	// viewer controls the active actor; every other viewer gets the menu-less
	// initiative-only TurnState, so we never leak one player's options/economy to
	// another. The controlling player's own connect/stream is what seeds + shows
	// the menu.
	activeActor := activeActorID(data)
	viewerControlsActiveActor := activeActor != "" && playerSeatForEntity(data, activeActor) == viewer
	var actorHydrated bool
	if charRepo != nil && viewerControlsActiveActor {
		// Ensure the active actor's economy is seeded before reading its menu, so
		// the turn-start snapshot carries economy too — not just the menu. The
		// connect-time snapshot path (StreamEncounter/GetEncounter) does NOT go
		// through the orchestrator's combat-load #598 seeding, so a freshly-
		// entered turn-based encounter's first actor would otherwise be unseeded
		// (nil economy) here. SeedTurnEconomyForData runs the toolkit's own
		// StartTurn (idempotent; no-op once in combat) and persists it — rpg-api
		// authors no economy values, the toolkit owns them (Invariant 2/3).
		if seedErr := SeedTurnEconomyForData(ctx, data, charRepo); seedErr != nil {
			return nil, seedErr
		}
		var attachErr error
		actorHydrated, attachErr = attachActorCharacterData(ctx, data, activeActor, charRepo)
		if attachErr != nil {
			return nil, attachErr
		}
	}

	enc, err := tkenc.LoadFromData(ctx, data, broker)
	if err != nil {
		return nil, err
	}

	// Compute the active actor's turn state (menu + economy) from the held
	// character the toolkit just hydrated. Only when we actually attached the
	// actor's blob: ActorTurnState returns a zero value for an un-held actor, and
	// we want buildTurnState to fall back to the menu-less shape in that case.
	var actorTurnState *tkenc.ActorTurnState
	if actorHydrated {
		ts := enc.ActorTurnState(activeActor)
		actorTurnState = &ts
	}

	snap := enc.SnapshotFor(viewer)

	// Build the Space from the viewer's revealed hexes. Hexes are sorted by
	// (Q,R,S) so the wire output is deterministic across Go map iterations.
	keys := make([]core.Hex, 0, len(snap.RevealedHexes))
	for h := range snap.RevealedHexes {
		keys = append(keys, h)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Q != keys[j].Q {
			return keys[i].Q < keys[j].Q
		}
		if keys[i].R != keys[j].R {
			return keys[i].R < keys[j].R
		}
		return keys[i].S < keys[j].S
	})
	hexes := make([]*encounterv2pb.Hex, 0, len(keys))
	for _, h := range keys {
		hexes = append(hexes, &encounterv2pb.Hex{Position: HexToPosition(h)})
	}

	// Build the set of hexes currently visible to the viewer so we can
	// include other players who are visible right now (not just ever-revealed).
	// This requires the viewer's sight range from the persisted view.
	var visibleNow core.HexSet
	if vp, ok := data.Players[viewer]; ok && vp.View != nil {
		visibleNow = perception.VisibleHexesAt(snap.Position, vp.View.SightRange, enc.Room())
	}

	// Collect entities visible to the viewer. Start with the viewer's own
	// entity (always visible), then add other players whose current position
	// falls within the viewer's sight range. Iterate data.Players in player-ID
	// order so wire output is deterministic across Go map iterations.
	playerIDs := make([]core.PlayerID, 0, len(data.Players))
	for pid := range data.Players {
		playerIDs = append(playerIDs, pid)
	}
	sort.Slice(playerIDs, func(i, j int) bool {
		return string(playerIDs[i]) < string(playerIDs[j])
	})

	var entities []*encounterv2pb.Entity
	for _, pid := range playerIDs {
		pd := data.Players[pid]
		if pid == viewer {
			// Viewer always sees their own entity.
			name, classRef := characterIdentityFor(ctx, charRepo, string(pd.EntityID))
			entities = append(entities, playerEntity(pd, snap.Position, name, classRef))
			continue
		}
		if pd.View == nil {
			continue
		}
		if visibleNow != nil && visibleNow.Has(pd.View.Position) {
			name, classRef := characterIdentityFor(ctx, charRepo, string(pd.EntityID))
			entities = append(entities, playerEntity(pd, pd.View.Position, name, classRef))
		}
	}

	// Append visible monster entities. Iterate data.Monsters in entity-ID order
	// so wire output is deterministic across Go map iterations. LOS-filter:
	// only include monsters whose position is currently visible to the viewer.
	monsterIDs := make([]core.EntityID, 0, len(data.Monsters))
	for mid := range data.Monsters {
		monsterIDs = append(monsterIDs, mid)
	}
	sort.Slice(monsterIDs, func(i, j int) bool {
		return string(monsterIDs[i]) < string(monsterIDs[j])
	})
	for _, mid := range monsterIDs {
		m := data.Monsters[mid]
		if visibleNow == nil || !visibleNow.Has(m.Position) {
			continue
		}
		entities = append(entities, monsterEntity(m))
	}

	return &encounterv2pb.Encounter{
		Id:        string(data.ID),
		Mode:      encounterModeToProto(data.Mode),
		TurnState: buildTurnState(data, actorTurnState),
		Space: &encounterv2pb.Space{
			Hexes:    hexes,
			Walls:    append(wallsToProto(data.Space), doorWallsToProto(data)...),
			Entities: entities,
		},
	}, nil
}

// wallsToProto converts an encounter's persisted room snapshot (rpg-toolkit
// #757/#759's SpaceData) into the wire Wall list. Wave 1 wall visibility is
// whole-room (design doc: "no per-viewer wall reveal yet"), so every viewer's
// snapshot carries every wall unconditionally — unlike Hexes/Entities above,
// this is NOT gated by RevealedHexes/visibleNow. space is nil for encounters
// with no room (InitRoom never called — pre-wave-1 fixtures, non-spatial
// encounters), which projects to an empty Walls list.
func wallsToProto(space *tkenc.SpaceData) []*encounterv2pb.Wall {
	if space == nil {
		return nil
	}
	walls := make([]*encounterv2pb.Wall, 0, len(space.Walls))
	for _, w := range space.Walls {
		kind, ok := wallKindFor(w.BlocksMovement, w.BlocksLoS)
		if !ok {
			continue
		}
		// Wave 1 persists one degenerate (Start == End) segment per
		// discretized wall hex (SpaceData.Walls doc) — From/To both convert
		// the same cube coordinate.
		from := HexToPosition(core.HexFromCube(w.Start))
		to := HexToPosition(core.HexFromCube(w.End))
		walls = append(walls, &encounterv2pb.Wall{From: from, To: to, Kind: kind})
	}
	return walls
}

// doorWallsToProto projects every door in the encounter onto the wire as a
// Wall carrying its entity id (rpg-api-protos#186's additive Wall.id — The
// Dungeon wave 2 Slice 2, rpg-api#676): the design doc's crux resolution
// (§Crux) makes DoorData the single source of door truth, with the wall
// kind/edge as its PROJECTED geometry, never a second source of truth.
// Like wallsToProto, this is unconditional (whole-room wall visibility,
// wave 1's "no per-viewer wall reveal yet") — every viewer's snapshot
// carries every door regardless of RevealedHexes/visibleNow.
//
// The passage edge (design doc §Q2, the web-confirmed 6-door-pair
// multiplicity constraint on an isolated degenerate wall cell): From is
// the door's own cell; To is doorPassageNeighbor's pick — the one
// designated passage edge, so the renderer draws a single door frame on
// that edge instead of an ambiguous isolated-hex wall.
//
// Doors are sorted by id for deterministic wire output across Go map
// iterations (data.Doors is a map), matching this file's Players/Monsters
// sort convention above.
func doorWallsToProto(data *tkenc.Data) []*encounterv2pb.Wall {
	if len(data.Doors) == 0 {
		return nil
	}
	ids := make([]core.EntityID, 0, len(data.Doors))
	for id := range data.Doors {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return string(ids[i]) < string(ids[j]) })

	walls := make([]*encounterv2pb.Wall, 0, len(ids))
	for _, id := range ids {
		door := data.Doors[id]

		kind := encounterv2pb.WallKind_WALL_KIND_DOOR_CLOSED
		if door.Open {
			kind = encounterv2pb.WallKind_WALL_KIND_DOOR_OPEN
		}
		doorID := string(id)
		walls = append(walls, &encounterv2pb.Wall{
			From: HexToPosition(door.Position),
			To:   HexToPosition(doorPassageNeighbor(data.Space, door)),
			Kind: kind,
			Id:   &doorID,
		})
	}
	return walls
}

// doorPassageNeighbor returns the door's designated passage-edge neighbor
// (see doorWallsToProto's doc): the door's own cell belongs to NEITHER
// region (KNOWN TRAP flagged by rpg-toolkit#806's gate note and mirrored
// in start_encounter.go's chamberEntryAnchor — RegionAt(door.Position) is
// always ("", false)), so this walks perception.HexNeighbors (the
// toolkit's canonical six-neighbor set) and returns the first neighbor
// SpaceData.RegionAt tags into ANY region — for Slice 2's single door that
// is always the chamber-1 or chamber-2 side. Never hand-rolled hex-grid
// geometry: HexNeighbors + RegionAt are both toolkit-exposed surface.
// Falls back to the door's own position (a degenerate Start==To segment,
// matching a solid wall's shape) when no tagged neighbor exists — which
// includes a nil space (Copilot review, PR #677: fixtures that AddDoor
// without ever calling InitRoom/InitTwoChamberRoom leave Data.Space nil,
// so a door-only encounter must still project deterministically instead of
// dropping the door from the wire or panicking). SpaceData.RegionAt is
// itself nil-receiver safe (rpg-toolkit#806), so this guard is
// belt-and-suspenders documentation as much as a runtime necessity — it
// makes the nil-space contract explicit here rather than resting on a
// transitive safety property of a function this package doesn't own.
func doorPassageNeighbor(space *tkenc.SpaceData, door *tkenc.DoorData) core.Hex {
	if space == nil {
		return door.Position
	}
	for _, n := range perception.HexNeighbors(door.Position) {
		if _, ok := space.RegionAt(n); ok {
			return n
		}
	}
	return door.Position
}

// wallKindFor maps a wall segment's persisted BlocksMovement/BlocksLoS flags
// (environments.WallSegmentData; wave 1 does not persist a WallType — see
// rebuildRoomFromData's doc in rpg-toolkit/encounter/space.go) onto the
// proto WallKind enum:
//   - blocks both: SOLID (the common case for an interior wall)
//   - blocks movement only: WINDOW (see through, can't walk through)
//   - blocks LoS only: SOLID as a conservative fallback — wave 1's generator
//     (environments.QuickRoom) never produces this combination, but a
//     LoS-blocking segment shouldn't be silently dropped if one ever exists
//   - blocks neither: not a wall worth putting on the wire (ok=false)
func wallKindFor(blocksMovement, blocksLoS bool) (kind encounterv2pb.WallKind, ok bool) {
	switch {
	case blocksMovement && blocksLoS:
		return encounterv2pb.WallKind_WALL_KIND_SOLID, true
	case blocksMovement && !blocksLoS:
		return encounterv2pb.WallKind_WALL_KIND_WINDOW, true
	case !blocksMovement && blocksLoS:
		return encounterv2pb.WallKind_WALL_KIND_SOLID, true
	default:
		return encounterv2pb.WallKind_WALL_KIND_UNSPECIFIED, false
	}
}

// activeActorID returns the entity id of the actor whose turn it is, or "" when
// the encounter is not turn-based or the active index is out of range.
func activeActorID(data *tkenc.Data) core.EntityID {
	if data.Mode != core.ModeTurnBased {
		return ""
	}
	if data.ActiveIdx < 0 || data.ActiveIdx >= len(data.Initiative) {
		return ""
	}
	return data.Initiative[data.ActiveIdx]
}

// buildTurnState returns a populated TurnState only when the encounter is in
// turn-based mode.
//
// Initiative IDs (and the active entity ID) are emitted verbatim — clients
// need them as opaque tokens to render "whose turn" even when the active
// entity is currently outside the viewer's LOS. Per-entity rich data
// (position, hp, monster ref) is still LOS-gated via Space.Entities; the
// initiative roster only exposes ids, which carry no spatial information.
//
// actorTurnState is the active actor's toolkit-computed menu + economy (#601),
// supplied by ProjectFor when it hydrated the active actor. When non-nil it
// populates Economy + AvailableActions field-for-field so the snapshot carries
// the menu at turn start (the client needs it to take its FIRST action, before
// any TurnStateChanged push). nil (NPC turn / no char store / not hydrated)
// leaves them empty — the menu-less initiative-only shape, as before. The
// toolkit computed the menu; rpg-api only projects it.
func buildTurnState(data *tkenc.Data, actorTurnState *tkenc.ActorTurnState) *encounterv2pb.TurnState {
	if data.Mode != core.ModeTurnBased {
		return nil
	}
	order := make([]string, 0, len(data.Initiative))
	for _, eid := range data.Initiative {
		order = append(order, string(eid))
	}
	var active string
	if data.ActiveIdx >= 0 && data.ActiveIdx < len(data.Initiative) {
		active = string(data.Initiative[data.ActiveIdx])
	}
	ts := &encounterv2pb.TurnState{
		InitiativeOrder: order,
		ActiveEntityId:  active,
		Round:           int32(data.Round),
	}
	// #601: project the active actor's economy + menu when available. The
	// rulebook-touching projection lives in turn_state_project.go (the toolkit
	// computes ActorTurnState; this only maps its fields onto the wire).
	if actorTurnState != nil {
		ts.Economy = actorEconomyToProto(actorTurnState.Economy)
		ts.AvailableActions = actorMenuToProto(actorTurnState)
	}
	return ts
}

// playerEntity builds the wire-shape proto Entity for a player seat. The
// viewer's-own and visible-other-player branches differ only in which position
// the caller supplies (snap.Position vs pd.View.Position), so the rest of the
// emit lives here to keep them symmetric and to mirror the monster emit shape.
//
// displayName/classRef are resolved by the caller (characterIdentityFor) and
// passed in rather than looked up here — playerEntity stays a pure builder,
// matching monsterEntity's shape, and both call sites (ProjectFor's snapshot
// loop, entityForID's live-event enrichment) share the exact same resolution
// path (rpg-api#664).
//
// CharacterData.RaceRef remains unset: issue #664 only asked for
// display_name/class_ref (the identity dock's actual read, rpg-dnd5e-web#491)
// — race has no client reader yet, so resolving it now would be speculative
// enrichment with nothing to verify it against. Add it the same way
// (characterIdentityFor already holds the full character record) once a
// caller needs it.
func playerEntity(pd *tkenc.PlayerData, pos core.Hex, displayName string, classRef *encounterv2pb.Ref) *encounterv2pb.Entity {
	e := &encounterv2pb.Entity{
		Id:          string(pd.EntityID),
		Position:    HexToPosition(pos),
		Type:        encounterv2pb.EntityType_ENTITY_TYPE_CHARACTER,
		DisplayName: displayName,
		Hp: &encounterv2pb.HitPoints{
			Current: int32(pd.HP),
			Max:     int32(pd.MaxHP),
		},
		Data: &encounterv2pb.Entity_Character{
			Character: &encounterv2pb.CharacterData{
				PlayerId: string(pd.ID),
				ClassRef: classRef,
			},
		},
		StatusEffects: statusEffectsFrom(pd.ActiveConditions),
	}
	if pd.AC > 0 {
		ac := int32(pd.AC)
		e.ArmorClass = &ac
	}
	return e
}

// characterClassRefType is the proto Ref.Type value for a class identity ref
// (mirrors monsterEntity's "monster" / conditionRefFor's "condition" —
// established Ref-tagging convention in this file, see monsterRefFor).
const characterClassRefType = "class"

// characterIdentityFor resolves a player seat's display name and class ref
// for wire projection (rpg-api#664: real StartEncounter players showed the
// raw entity id instead of "Alice"/"Rogue" because playerEntity never
// populated these fields — issue #511 deferred it "until toolkit PlayerData
// carries class/race info"). That toolkit change never happened and isn't
// needed: a player seat's EntityID IS the character ID
// (start_encounter.go's AddPlayer sets EntityID: core.EntityID(m.CharacterID)),
// so this is a plain by-key lookup against the character store already
// wired into ProjectFor for turn-menu hydration — no rules math, pure
// projection of data rpg-api already owns.
//
// characterID intentionally comes in as a plain string, not core.EntityID —
// the character store's key space is characterrepo's, independent of the
// toolkit's entity-ID type.
//
// Any resolution failure (nil charRepo — tests and any caller that hasn't
// wired one, a NotFound character, or a genuine store error) returns zero
// values rather than propagating an error: display_name/class_ref are
// optional-by-design on the wire (rpg-dnd5e-web#491's dock already falls
// back to the raw entity id with no crash), so a missing/erroring character
// record should degrade the label, not the snapshot or the live event
// carrying it.
//
// One charRepo.Get per projected player per call, no batching — batch seam:
// see rpg-api#666.
func characterIdentityFor(ctx context.Context, charRepo characterrepo.Repository, characterID string) (string, *encounterv2pb.Ref) {
	if charRepo == nil {
		return "", nil
	}
	out, err := charRepo.Get(ctx, characterrepo.GetInput{ID: characterID})
	if err != nil || out == nil || out.Character == nil || out.Character.Data == nil {
		return "", nil
	}
	data := out.Character.Data
	var classRef *encounterv2pb.Ref
	if data.ClassID != "" {
		classRef = &encounterv2pb.Ref{Module: refModuleDnd5e, Type: characterClassRefType, Id: data.ClassID}
	}
	return data.Name, classRef
}

// statusEffectsFrom projects a toolkit ActiveConditions ref list
// (PlayerData.ActiveConditions / MonsterData.ActiveConditions,
// rpg-toolkit#754) onto the wire, matching the live
// translateConditionAppliedEvent's StatusEffect shape (Source: the same
// conditionRefFor parse translate.go's live path already uses) so a
// reconnecting client's snapshot and a continuously-connected client's
// stream agree on shape (rpg-api#651).
//
// ActiveConditions is already filtered toolkit-side (rpg-toolkit#778): it
// excludes conditions attached permanently at character/monster construction
// (class-grant passives, monster traits), since those never fire the live
// ConditionApplied event either — a naive unfiltered projection would badge
// every Monk with "MartialArts" and every goblin with "PackTactics" forever.
// rpg-api does no filtering of its own here; the boundary rule places that
// judgment in the toolkit, which has the rules knowledge to make it (see
// rpg-toolkit#778's investigation trail for why a ref-namespace filter in
// rpg-api specifically does NOT work).
//
// DurationRounds is left unset: ActiveConditions carries only ref strings,
// no duration — the live path's DurationRounds comes from the
// ConditionAppliedEvent's own payload, which snapshot projection has no
// equivalent of. A future toolkit enrichment could add it; nothing here
// depends on that happening.
func statusEffectsFrom(activeConditions []string) []*encounterv2pb.StatusEffect {
	if len(activeConditions) == 0 {
		return nil
	}
	effects := make([]*encounterv2pb.StatusEffect, 0, len(activeConditions))
	for _, ref := range activeConditions {
		effects = append(effects, &encounterv2pb.StatusEffect{
			Source: conditionRefFor(ref),
		})
	}
	return effects
}

// monsterEntity builds the wire-shape proto Entity for a monster, mirroring
// playerEntity's shape (type + hp + armor_class + a Data oneof variant).
// Unlike playerEntity, position is not a separate parameter — a monster's
// current position always lives on its own MonsterData.Position (there is
// no "viewer's own entity uses a different position source" case the way
// players have snap.Position vs pd.View.Position).
//
// This is the SAME builder ProjectFor's snapshot path uses AND (rpg-api#644
// playtest follow-up) the live EntityAppearedEvent translation path uses via
// entityForID — one authoritative place for "how does a MonsterData become a
// wire Entity" prevents the two paths from drifting the way they did before
// this fix (the live path used to hand-build a bare {id, position} Entity
// with no type, HP, or monster ref — see translateEntityAppearedEventWithData's
// doc for the full story).
//
// Entity.DisplayName is deliberately left unset here (rpg-api#664
// scope-decision): unlike a character's Name, "goblin" -> "Goblin" is
// content knowledge, not stored data — resolving it would mean rpg-api
// owning a monster-ref-to-label table, the same content the client already
// resolves for condition refs (see translateConditionAppliedEvent's doc:
// "the web resolves display_name/icon_hint from its own lookup table").
// MonsterData.MonsterRef already carries the structured {module,type,id} the
// client needs to do that lookup itself.
func monsterEntity(m *tkenc.MonsterData) *encounterv2pb.Entity {
	me := &encounterv2pb.Entity{
		Id:       string(m.ID),
		Position: HexToPosition(m.Position),
		Type:     encounterv2pb.EntityType_ENTITY_TYPE_MONSTER,
		Hp: &encounterv2pb.HitPoints{
			Current: int32(m.HP),
			Max:     int32(m.MaxHP),
		},
		Data: &encounterv2pb.Entity_Monster{
			Monster: &encounterv2pb.MonsterData{
				MonsterRef: monsterRefFor(m.MonsterRef),
			},
		},
		StatusEffects: statusEffectsFrom(m.ActiveConditions),
	}
	if m.AC > 0 {
		ac := int32(m.AC)
		me.ArmorClass = &ac
	}
	return me
}

// monsterRefFor builds a proto Ref for a toolkit monster-ref string.
// The toolkit ships a fully-qualified ref (e.g. "dnd5e:monsters:goblin");
// we reuse splitRef from translate.go so the parsing contract is identical
// across the v2 encounter wire (snapshot + live events). Bare strings are
// treated as ids under module=dnd5e, type=monster, mirroring conditionRefFor.
func monsterRefFor(toolkitMonsterRef string) *encounterv2pb.Ref {
	parts := splitRef(toolkitMonsterRef)
	if len(parts) == 3 {
		return &encounterv2pb.Ref{Module: parts[0], Type: parts[1], Id: parts[2]}
	}
	return &encounterv2pb.Ref{Module: refModuleDnd5e, Type: "monster", Id: toolkitMonsterRef}
}
