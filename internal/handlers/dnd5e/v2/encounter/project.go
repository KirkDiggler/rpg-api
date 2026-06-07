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
// the encounter's persisted data. CreateEncounter calls it today; GetEncounter
// (#500) and StreamEncounter snapshot replay (#497) reuse it so the projection
// logic lives in exactly one place.
//
// The broker is required to rehydrate a live Encounter via LoadFromData
// (needed for SnapshotFor).
//
// charRepo is used ONLY to hydrate the turn's ACTIVE actor so the snapshot's
// TurnState can carry that actor's economy + available_actions menu at turn
// start (#601: the client needs the menu to take its FIRST action, before any
// TurnStateChanged push fires). nil disables menu hydration (tests / non-combat
// callers): the snapshot then carries initiative/active/round only, as before.
// The toolkit computes the menu (ActorTurnState); rpg-api projects it.
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
		visibleNow = perception.VisibleHexesAt(snap.Position, vp.View.SightRange)
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
			entities = append(entities, playerEntity(pd, snap.Position))
			continue
		}
		if pd.View == nil {
			continue
		}
		if visibleNow != nil && visibleNow.Has(pd.View.Position) {
			entities = append(entities, playerEntity(pd, pd.View.Position))
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
		}
		if m.AC > 0 {
			ac := int32(m.AC)
			me.ArmorClass = &ac
		}
		entities = append(entities, me)
	}

	return &encounterv2pb.Encounter{
		Id:        string(data.ID),
		Mode:      encounterModeToProto(data.Mode),
		TurnState: buildTurnState(data, actorTurnState),
		Space: &encounterv2pb.Space{
			Hexes:    hexes,
			Entities: entities,
		},
	}, nil
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
// CharacterData currently sets only PlayerId. ClassRef/RaceRef are deferred
// until toolkit PlayerData carries class/race info; see issue #511 for the
// rationale (we don't want to couple ProjectFor to the character orchestrator
// for a thin enrichment).
func playerEntity(pd *tkenc.PlayerData, pos core.Hex) *encounterv2pb.Entity {
	e := &encounterv2pb.Entity{
		Id:       string(pd.EntityID),
		Position: HexToPosition(pos),
		Type:     encounterv2pb.EntityType_ENTITY_TYPE_CHARACTER,
		Hp: &encounterv2pb.HitPoints{
			Current: int32(pd.HP),
			Max:     int32(pd.MaxHP),
		},
		Data: &encounterv2pb.Entity_Character{
			Character: &encounterv2pb.CharacterData{
				PlayerId: string(pd.ID),
			},
		},
	}
	if pd.AC > 0 {
		ac := int32(pd.AC)
		e.ArmorClass = &ac
	}
	return e
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
