// Package encounter is the v2 encounter handler.
// project.go translates a toolkit encounter.Data into the v1alpha2
// proto Encounter message for a specific viewer.
package encounter

import (
	"sort"
	"strings"
	"time"

	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/encounter/perception"
)

// ProjectFor builds a proto *encounterv2pb.Encounter for the given viewer from
// the encounter's persisted data. CreateEncounter calls it today; GetEncounter
// (#500) and StreamEncounter snapshot replay (#497) will reuse it so the
// projection logic lives in exactly one place.
//
// The broker is required to rehydrate a live Encounter via LoadFromData
// (needed for SnapshotFor).
//
// now is passed explicitly so callers in tests can inject a fixed clock.
func ProjectFor(
	data *tkenc.Data,
	viewer core.PlayerID,
	broker *tkenc.Broker,
	now time.Time,
) (*encounterv2pb.Encounter, error) {
	enc, err := tkenc.LoadFromData(data, broker)
	if err != nil {
		return nil, err
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
			entities = append(entities, &encounterv2pb.Entity{
				Id:       string(pd.EntityID),
				Position: HexToPosition(snap.Position),
			})
			continue
		}
		if pd.View == nil {
			continue
		}
		if visibleNow != nil && visibleNow.Has(pd.View.Position) {
			entities = append(entities, &encounterv2pb.Entity{
				Id:       string(pd.EntityID),
				Position: HexToPosition(pd.View.Position),
			})
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
		entities = append(entities, &encounterv2pb.Entity{
			Id:       string(m.ID),
			Position: HexToPosition(m.Position),
			Type:     encounterv2pb.EntityType_ENTITY_TYPE_MONSTER,
			Hp: &encounterv2pb.HitPoints{
				Current: int32(m.HP),
				Max:     int32(m.MaxHP),
			},
			Data: &encounterv2pb.Entity_Monster{
				Monster: &encounterv2pb.MonsterData{
					MonsterRef: parseMonsterRef(m.MonsterRef),
				},
			},
		})
	}

	return &encounterv2pb.Encounter{
		Id:        string(data.ID),
		Mode:      mapMode(data.Mode),
		TurnState: buildTurnState(data),
		Space: &encounterv2pb.Space{
			Hexes:    hexes,
			Entities: entities,
		},
	}, nil
}

// mapMode converts a toolkit core.EncounterMode to the v1alpha2 wire enum.
// ModeUnspecified (the zero value) is treated as FREE_ROAM per the toolkit's
// documented convention (see encounter/core/mode.go).
func mapMode(m core.EncounterMode) encounterv2pb.EncounterMode {
	switch m {
	case core.ModeTurnBased:
		return encounterv2pb.EncounterMode_ENCOUNTER_MODE_TURN_BASED
	case core.ModeFreeRoam:
		return encounterv2pb.EncounterMode_ENCOUNTER_MODE_FREE_ROAM
	case core.ModeUnspecified:
		// Toolkit convention: treat unspecified as free-roam on the wire so
		// late-joining clients don't see UNSPECIFIED for an exploration session
		// that simply never set Mode explicitly.
		return encounterv2pb.EncounterMode_ENCOUNTER_MODE_FREE_ROAM
	default:
		return encounterv2pb.EncounterMode_ENCOUNTER_MODE_UNSPECIFIED
	}
}

// buildTurnState returns a populated TurnState only when the encounter is in
// turn-based mode. ActionEconomy and AvailableActions are server-only state
// for this PR; emitting them is tracked as a follow-up.
func buildTurnState(data *tkenc.Data) *encounterv2pb.TurnState {
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
	return &encounterv2pb.TurnState{
		InitiativeOrder: order,
		ActiveEntityId:  active,
		Round:           int32(data.Round),
	}
}

// parseMonsterRef splits a toolkit monster ref string like
// "dnd5e:monsters:goblin" into a proto Ref{Module, Type, Id}. If the string
// is malformed (fewer than 3 colon-separated parts), the whole value is
// passed through as Id so the wire still carries something identifying.
func parseMonsterRef(ref string) *encounterv2pb.Ref {
	parts := strings.SplitN(ref, ":", 3)
	if len(parts) < 3 {
		return &encounterv2pb.Ref{Id: ref}
	}
	return &encounterv2pb.Ref{
		Module: parts[0],
		Type:   parts[1],
		Id:     parts[2],
	}
}
