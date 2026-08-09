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
	tkencevents "github.com/KirkDiggler/rpg-toolkit/encounter/events"
	"github.com/KirkDiggler/rpg-toolkit/encounter/perception"
	"github.com/KirkDiggler/rpg-toolkit/events"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
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
//     (rpg-api#664) and equipment display fields (rpg-api#680) via
//     characterDataFor — not just the active actor.
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

	// The Space's hex records are built AFTER the entity pass below, because a
	// record carries its own occupants and those must come from the same pass
	// that decides which entities are disclosed. Two independent filters could
	// disagree and leave a placement pointing at an entity the viewer was never
	// told about; one pass cannot.

	// Build the set of hexes currently visible to the viewer so we can
	// include other players who are visible right now (not just ever-revealed).
	// This requires the viewer's sight range from the persisted view.
	var visibleNow core.HexSet
	if vp, ok := data.Players[viewer]; ok && vp.View != nil {
		visibleNow = perception.VisibleHexesAt(snap.Position, vp.View.SightRange, enc.Room())
	}

	// entities is still rpg-api's own pass (the toolkit only tracks entity
	// IDs in a placement, never full disclosed identity/hp/equipment — that
	// stays rpg-api's job). placements fed the retired toolkitKnowledge
	// stand-in and has no other caller here now; enc.KnownHexes(viewer)
	// below already carries each hex's Contents.
	entities, _ := disclosedEntities(ctx, charRepo, data, viewer, snap, visibleNow)

	// rpg-toolkit#851: the toolkit now IS the source of viewer knowledge —
	// enc.KnownHexes(viewer) replaces the toolkitKnowledge stand-in
	// (knowledge_adapter.go, deleted) that used to hand-assemble this from
	// RevealedHexes + a locally-rebuilt edges/placements index. A remembered
	// hex's Edges/Contents now come from the toolkit's own persisted
	// observations, not this package's own approximation. rpg-api#733
	// deleted knowledge.go's ViewerKnowledge/DiffKnowledge seam (it had no
	// production caller — the live-path fix that would have used it instead
	// re-states enc.KnownHexes(viewer) wholesale; see
	// translateMoveEventWithData in translate.go), so enc is called directly
	// here rather than through an interface.
	return &encounterv2pb.Encounter{
		Id:        string(data.ID),
		Mode:      encounterModeToProto(data.Mode),
		TurnState: buildTurnState(data, actorTurnState),
		Space: &encounterv2pb.Space{
			Hexes:    hexRecordsToProto(enc.KnownHexes(viewer)),
			Entities: entities,
			Zones:    zonesToProto(enc.AuthorizedZones(viewer)),
			Theme:    themeToProto(data.Space),
		},
	}, nil
}

// disclosedEntities is THE authorization filter for "which entities does
// viewer get told about, and where": viewer's own entity (always, at
// snap.Position); other players whose View.Position falls within
// visibleNow; monsters within visibleNow; static obstacles within
// snap.RevealedHexes (sticky — once revealed, obstacles stay disclosed even
// outside current LOS, unlike combatants). Iteration order is sorted by id
// within each category (players, then monsters, then obstacles) so both the
// entities slice and each placements[hex] slice are deterministic across
// Go's randomized map iteration.
//
// This is ProjectFor's own disclosure pass, factored out so a second caller
// can ask the exact same question without a second, independently-drifting
// filter (translate.go's translateEntityDisappearedEventWithData: when an
// entity leaves a hex that remains visible to the viewer, the hex's
// remaining Contents must be rebuilt from every OTHER entity actually
// standing there, not just the departed one subtracted from stale data —
// this is the one place that answer comes from). placements[hex] is exactly
// the Contents a HexRecord at hex should carry.
func disclosedEntities(
	ctx context.Context,
	charRepo characterrepo.Repository,
	data *tkenc.Data,
	viewer core.PlayerID,
	snap tkenc.Snapshot,
	visibleNow core.HexSet,
) (entities []*encounterv2pb.Entity, placements map[core.Hex][]perception.Placement) {
	playerIDs := make([]core.PlayerID, 0, len(data.Players))
	for pid := range data.Players {
		playerIDs = append(playerIDs, pid)
	}
	sort.Slice(playerIDs, func(i, j int) bool {
		return string(playerIDs[i]) < string(playerIDs[j])
	})

	placements = make(map[core.Hex][]perception.Placement)
	// placements records WHERE each disclosed entity is. Position left Entity
	// with rpg-api-protos#197: the hex states occupancy, so a second copy on
	// the entity could only agree redundantly or disagree dangerously.
	disclose := func(at core.Hex, placement perception.Placement, e *encounterv2pb.Entity) {
		entities = append(entities, e)
		placements[at] = append(placements[at], placement)
	}

	for _, pid := range playerIDs {
		pd := data.Players[pid]
		if pid == viewer {
			// Viewer always sees their own entity.
			name, classRef, equip := characterDataFor(ctx, charRepo, string(pd.EntityID))
			disclose(snap.Position, perception.Placement{EntityID: pd.EntityID}, playerEntity(pd, name, classRef, equip))
			continue
		}
		if pd.View == nil {
			continue
		}
		if visibleNow != nil && visibleNow.Has(pd.View.Position) {
			name, classRef, equip := characterDataFor(ctx, charRepo, string(pd.EntityID))
			disclose(pd.View.Position, perception.Placement{EntityID: pd.EntityID}, playerEntity(pd, name, classRef, equip))
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
		disclose(m.Position, perception.Placement{EntityID: m.ID, Offset: m.Offset}, monsterEntity(m))
	}

	// Static obstacles are sticky exploration data, not currently-visible
	// combatants: once their hex is revealed, include them on snapshots even
	// when it is outside the viewer's current line of sight. Their stable IDs
	// provide deterministic ordering across the toolkit's placement slice.
	if data.Space != nil {
		obstacles := make([]tkenc.ObstacleData, 0, len(data.Space.Obstacles))
		for _, obstacle := range data.Space.Obstacles {
			if snap.RevealedHexes.Has(obstacle.Position) {
				obstacles = append(obstacles, obstacle)
			}
		}
		sort.Slice(obstacles, func(i, j int) bool {
			return string(obstacles[i].ID) < string(obstacles[j].ID)
		})
		for _, obstacle := range obstacles {
			disclose(obstacle.Position, perception.Placement{
				EntityID: obstacle.ID, Facing: obstacle.Facing, Offset: obstacle.Offset,
			}, obstacleEntity(obstacle))
		}
	}

	return entities, placements
}

// hexRecordsToProto is the one conversion point from viewer knowledge to the
// wire. Records are sorted by (Q,R,S) so output is deterministic across Go's
// randomized map iteration.
func hexRecordsToProto(known map[core.Hex]perception.HexObservation) []*encounterv2pb.HexRecord {
	obs := make([]perception.HexObservation, 0, len(known))
	for _, o := range known {
		obs = append(obs, o)
	}
	sortObservations(obs)

	out := make([]*encounterv2pb.HexRecord, 0, len(obs))
	for _, o := range obs {
		out = append(out, observationToProto(o))
	}
	return out
}

// knownHexesToProto converts a MoveEvent's own MoverKnownHexes
// (rpg-api#737) into wire HexRecords. tkencevents.KnownHex is a flat mirror
// of perception.HexObservation (the encounter/events package cannot import
// perception — see KnownHex's doc for the cycle), so this converts back to
// perception.HexObservation field-for-field and reuses observationToProto
// rather than duplicating its proto-building logic. No sort here: the
// toolkit's own knownHexesToEvents already sorts by (Q,R,S) before this
// slice is published, so it arrives deterministic.
func knownHexesToProto(known []tkencevents.KnownHex) []*encounterv2pb.HexRecord {
	out := make([]*encounterv2pb.HexRecord, 0, len(known))
	for _, kh := range known {
		out = append(out, observationToProto(knownHexToObservation(kh)))
	}
	return out
}

func knownHexToObservation(kh tkencevents.KnownHex) perception.HexObservation {
	edges := make([]perception.Edge, 0, len(kh.Edges))
	for _, e := range kh.Edges {
		edges = append(edges, perception.Edge{
			From:           e.From,
			To:             e.To,
			BlocksMovement: e.BlocksMovement,
			BlocksLoS:      e.BlocksLoS,
			DoorID:         e.DoorID,
			DoorOpen:       e.DoorOpen,
			DoorLocked:     e.DoorLocked,
		})
	}
	contents := make([]perception.Placement, 0, len(kh.Contents))
	for _, c := range kh.Contents {
		contents = append(contents, perception.Placement{
			EntityID: c.EntityID,
			Facing:   c.Facing,
			Offset:   c.Offset,
		})
	}
	return perception.HexObservation{
		Position: kh.Position,
		State:    perception.KnowledgeState(kh.State),
		Terrain:  perception.TerrainKind(kh.Terrain),
		ZoneID:   kh.ZoneID,
		Edges:    edges,
		Contents: contents,
	}
}

// sortObservations keeps wire output deterministic. Go randomizes map
// iteration, so without this the same knowledge would serialize differently
// run to run — flaky golden tests, and a diff that looks like a change when
// nothing moved. (Formerly shared with the now-deleted DiffKnowledge in
// knowledge.go; hexRecordsToProto is its only caller now.)
func sortObservations(obs []perception.HexObservation) {
	sort.Slice(obs, func(i, j int) bool {
		a, b := obs[i].Position, obs[j].Position
		if a.Q != b.Q {
			return a.Q < b.Q
		}
		if a.R != b.R {
			return a.R < b.R
		}
		return a.S < b.S
	})
}

func observationToProto(o perception.HexObservation) *encounterv2pb.HexRecord {
	rec := &encounterv2pb.HexRecord{
		Position: HexToPosition(o.Position),
		State:    knowledgeStateToProto(o.State),
		Terrain:  terrainToProto(o.Terrain),
		ZoneId:   o.ZoneID,
	}
	for _, e := range o.Edges {
		if w := edgeToProto(e); w != nil {
			rec.Edges = append(rec.Edges, w)
		}
	}
	// Contents is TOTAL on a visible record: an empty list is a positive claim
	// that the hex is empty, which is what deletes a remembered occupant on
	// re-sight without a forget message.
	for _, p := range o.Contents {
		rec.Contents = append(rec.Contents, &encounterv2pb.Placement{
			EntityId: string(p.EntityID),
			// Preserve optional presence verbatim: nil means no authored override,
			// while a non-nil pointer to E remains an explicit zero on the wire.
			Facing: p.Facing,
			// Offset is mechanically inert provider truth. Copy all three world-axis
			// components exactly; never rotate or reinterpret them from Facing.
			Offset: toProtoRuntimePlacementOffset(p.Offset),
		})
	}
	return rec
}

func toProtoRuntimePlacementOffset(offset *core.PlacementOffset) *dnd5ev1alpha1.PlacementOffset {
	if offset == nil {
		return nil
	}
	return &dnd5ev1alpha1.PlacementOffset{X: offset[0], Y: offset[1], Z: offset[2]}
}

func knowledgeStateToProto(s perception.KnowledgeState) encounterv2pb.HexState {
	switch s {
	case perception.KnowledgeStateVisible:
		return encounterv2pb.HexState_HEX_STATE_VISIBLE
	case perception.KnowledgeStateRemembered:
		return encounterv2pb.HexState_HEX_STATE_REMEMBERED
	case perception.KnowledgeStateUnspecified:
		return encounterv2pb.HexState_HEX_STATE_UNSPECIFIED
	default:
		return encounterv2pb.HexState_HEX_STATE_UNSPECIFIED
	}
}

func terrainToProto(t perception.TerrainKind) encounterv2pb.TerrainType {
	switch t {
	case perception.TerrainKindFloor:
		return encounterv2pb.TerrainType_TERRAIN_TYPE_FLOOR
	case perception.TerrainKindRough:
		return encounterv2pb.TerrainType_TERRAIN_TYPE_ROUGH
	case perception.TerrainKindDifficult:
		return encounterv2pb.TerrainType_TERRAIN_TYPE_DIFFICULT
	case perception.TerrainKindVoid:
		return encounterv2pb.TerrainType_TERRAIN_TYPE_VOID
	case perception.TerrainKindWater:
		return encounterv2pb.TerrainType_TERRAIN_TYPE_WATER
	case perception.TerrainKindUnspecified:
		return encounterv2pb.TerrainType_TERRAIN_TYPE_UNSPECIFIED
	default:
		return encounterv2pb.TerrainType_TERRAIN_TYPE_UNSPECIFIED
	}
}

// edgeToProto maps an observed barrier onto the wire. Doors keep their id so a
// click can become an InteractRequest; a segment that blocks nothing is not a
// wall worth sending, matching wallKindFor's ok=false case.
func edgeToProto(e perception.Edge) *encounterv2pb.Wall {
	w := &encounterv2pb.Wall{
		From: HexToPosition(e.From),
		To:   HexToPosition(e.To),
	}
	if e.DoorID != "" {
		id := e.DoorID
		w.Id = &id
		switch {
		case e.DoorOpen:
			w.Kind = encounterv2pb.WallKind_WALL_KIND_DOOR_OPEN
		case e.DoorLocked:
			w.Kind = encounterv2pb.WallKind_WALL_KIND_DOOR_LOCKED
		default:
			w.Kind = encounterv2pb.WallKind_WALL_KIND_DOOR_CLOSED
		}
		return w
	}
	kind, ok := wallKindFor(e.BlocksMovement, e.BlocksLoS)
	if !ok {
		return nil
	}
	w.Kind = kind
	return w
}

// zonesToProto translates only the toolkit-authorized Zone facts for this
// viewer. The toolkit has already selected observed scopes and required
// ancestors; rpg-api must not inspect SpaceData or reconstruct membership.
func zonesToProto(zones []tkenc.Zone) []*encounterv2pb.Zone {
	if len(zones) == 0 {
		return nil
	}
	out := make([]*encounterv2pb.Zone, 0, len(zones))
	for _, zone := range zones {
		pbZone := &encounterv2pb.Zone{Id: zone.ID, ParentId: zone.ParentID}
		if zone.Name != nil {
			pbZone.Name = *zone.Name
		}
		if zone.Archetype != nil {
			pbZone.Archetype = string(*zone.Archetype)
		}
		out = append(out, pbZone)
	}
	return out
}

// themeToProto projects SpaceData.Theme onto the wire verbatim (rpg-
// api#687) — opaque cosmetic metadata the toolkit documents as "never
// interpreted here" (SpaceData.Theme's doc); rpg-api mirrors that boundary
// and never interprets or defaults it either. nil space (no room) falls
// back to "", the same absent-metadata shape as an explicit empty Theme.
func themeToProto(space *tkenc.SpaceData) string {
	if space == nil {
		return ""
	}
	return space.Theme
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
// displayName/classRef/equip are all resolved by the caller (characterDataFor)
// and passed in rather than looked up here — playerEntity stays a pure
// builder, matching monsterEntity's shape, and both call sites (ProjectFor's
// snapshot loop, entityForID's live-event enrichment) share the exact same
// resolution path (rpg-api#664, rpg-api#680).
//
// equip carries the equipped/inventory/slots/armor_class_detail/
// main_hand_damage fields, all sourced from the toolkit's EquipmentView,
// never computed here. nil (no character store wired, or the lookup/load
// failed) leaves those fields unset — degrade the snapshot, don't fail it,
// matching identity's fallback.
//
// CharacterData.RaceRef remains unset: issue #664 only asked for
// display_name/class_ref (the identity dock's actual read, rpg-dnd5e-web#491)
// — race has no client reader yet, so resolving it now would be speculative
// enrichment with nothing to verify it against. Add it the same way
// (characterDataFor already holds the full character record) once a
// caller needs it.
func playerEntity(
	pd *tkenc.PlayerData,
	displayName string,
	classRef *encounterv2pb.Ref,
	equip *encounterv2pb.CharacterData,
) *encounterv2pb.Entity {
	cd := &encounterv2pb.CharacterData{
		PlayerId: string(pd.ID),
		ClassRef: classRef,
	}
	if equip != nil {
		cd.Equipped = equip.Equipped
		cd.Inventory = equip.Inventory
		cd.Slots = equip.Slots
		cd.ArmorClassDetail = equip.ArmorClassDetail
		cd.MainHandDamage = equip.MainHandDamage
	}
	e := &encounterv2pb.Entity{
		Id:          string(pd.EntityID),
		Type:        encounterv2pb.EntityType_ENTITY_TYPE_CHARACTER,
		DisplayName: displayName,
		Hp: &encounterv2pb.HitPoints{
			Current: int32(pd.HP),
			Max:     int32(pd.MaxHP),
		},
		Data:          &encounterv2pb.Entity_Character{Character: cd},
		StatusEffects: statusEffectsFrom(pd.ActiveConditions),
	}
	// Entity.armor_class must stay in sync with armor_class_detail.total
	// (rpg-api#680 gate: the two duplicate the same total by design — see
	// CharacterData.armor_class_detail's doc comment in types.proto). The
	// EquipmentView total is the real toolkit-computed EffectiveAC(), so it
	// wins over the encounter snapshot's own cached pd.AC whenever it's
	// available — pd.AC is only a fallback for when the character store
	// wasn't reachable (see equip's doc above).
	switch {
	case cd.ArmorClassDetail != nil:
		ac := cd.ArmorClassDetail.Total
		e.ArmorClass = &ac
	case pd.AC > 0:
		ac := int32(pd.AC)
		e.ArmorClass = &ac
	}
	return e
}

// characterClassRefType is the proto Ref.Type value for a class identity ref
// (mirrors monsterEntity's "monster" / conditionRefFor's "condition" —
// established Ref-tagging convention in this file, see monsterRefFor).
const characterClassRefType = "class"

// characterDataFor resolves a player seat's full wire-projected character
// data with a single character-store lookup: display name + class ref
// (rpg-api#664: real StartEncounter players showed the raw entity id
// instead of "Alice"/"Rogue" because playerEntity never populated these
// fields — issue #511 deferred it "until toolkit PlayerData carries
// class/race info"; that toolkit change never happened and isn't needed —
// a player seat's EntityID IS the character ID, start_encounter.go's
// AddPlayer sets EntityID: core.EntityID(m.CharacterID)) plus the
// equipment display fields (rpg-api#680: equipped/inventory/slots/
// armor_class_detail/main_hand_damage, composed from the toolkit's
// EquipmentView — a display projection, not rules math run in rpg-api).
// Both were originally separate lookups (characterIdentityFor +
// characterEquipmentFor); merged into one Get so adding the equipment
// projection didn't double the per-player query count.
//
// characterID intentionally comes in as a plain string, not core.EntityID —
// the character store's key space is characterrepo's, independent of the
// toolkit's entity-ID type.
//
// Any resolution failure (nil charRepo — tests and any caller that hasn't
// wired one, a NotFound character, a genuine store error, or a toolkit
// load failure) returns zero values rather than propagating an error:
// every field here is optional-by-design on the wire (rpg-dnd5e-web#491's
// identity dock already falls back to the raw entity id with no crash),
// so a missing/erroring character record should degrade the projection,
// not fail the snapshot or the live event carrying it.
//
// One charRepo.Get per projected player per call, no batching — batch seam:
// see rpg-api#666.
func characterDataFor(
	ctx context.Context,
	charRepo characterrepo.Repository,
	characterID string,
) (name string, classRef *encounterv2pb.Ref, equip *encounterv2pb.CharacterData) {
	if charRepo == nil {
		return "", nil, nil
	}
	out, err := charRepo.Get(ctx, characterrepo.GetInput{ID: characterID})
	if err != nil || out == nil || out.Character == nil || out.Character.Data == nil {
		return "", nil, nil
	}
	data := out.Character.Data
	if data.ClassID != "" {
		classRef = &encounterv2pb.Ref{Module: refModuleDnd5e, Type: characterClassRefType, Id: data.ClassID}
	}

	char, err := tkcharacter.LoadFromData(ctx, data, events.NewEventBus())
	if err != nil {
		return data.Name, classRef, nil
	}
	return data.Name, classRef, BuildEquipmentCharacterData(char.EquipmentView(ctx))
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
		Id:   string(m.ID),
		Type: encounterv2pb.EntityType_ENTITY_TYPE_MONSTER,
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

func obstacleEntity(obstacle tkenc.ObstacleData) *encounterv2pb.Entity {
	return &encounterv2pb.Entity{
		Id:   string(obstacle.ID),
		Type: encounterv2pb.EntityType_ENTITY_TYPE_OBSTACLE,
		Data: &encounterv2pb.Entity_Obstacle{
			Obstacle: &encounterv2pb.ObstacleData{
				ObstacleRef:       obstacleRefFor(obstacle.Ref),
				BlocksMovement:    obstacle.BlocksMovement,
				BlocksLineOfSight: obstacle.BlocksLoS,
			},
		},
	}
}

func obstacleRefFor(toolkitObstacleRef string) *encounterv2pb.Ref {
	parts := splitRef(toolkitObstacleRef)
	if len(parts) == 3 {
		return &encounterv2pb.Ref{Module: parts[0], Type: parts[1], Id: parts[2]}
	}
	return &encounterv2pb.Ref{Module: refModuleDnd5e, Type: "obstacle", Id: toolkitObstacleRef}
}
