package encounter

import (
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
)

// toolkitKnowledge is a STAND-IN implementation of ViewerKnowledge built from
// what the toolkit exposes today. Delete it when rpg-toolkit#851 lands and the
// toolkit implements ViewerKnowledge directly.
//
// It exists so the handler can be written, wired and tested against the real
// interface now rather than after the toolkit changes — the same order the
// concept used for the proto. Everything downstream of ViewerKnowledge is
// production code; only this file is temporary.
//
// It is constructed per viewer, from values ProjectFor has already computed, so
// KnownHexes ignores its argument. A real implementation keys off the viewer.
type toolkitKnowledge struct {
	data     *tkenc.Data
	revealed core.HexSet
	visible  core.HexSet
	// placements is the viewer's authorized occupancy, built by the same pass
	// that builds the disclosed entity list. Sharing that one pass is what
	// guarantees a placement never references an entity the viewer was not
	// told about — the fail-closed property, held by construction rather than
	// by two filters agreeing.
	placements map[core.Hex][]Placement
	// edges is every wall/door segment indexed by the hex it sits on.
	edges map[core.Hex][]Edge
}

// KnownHexes reports the viewer's knowledge.
//
// KNOWN LIMITATION, and the reason rpg-toolkit#851 exists: the toolkit's memory
// is perception.View.RevealedHexes, a set of coordinates with no payload. There
// is no stored observation to recover, so a remembered hex carries only what is
// genuinely static — its position and zone — and NOT its edges.
//
// That is deliberate under-disclosure. Filling those from current world truth
// would be the leak this whole contract removes: a viewer would see a door
// someone else opened while they were away. Omitting is wrong in the safe
// direction, and it resolves the moment the toolkit stores observations.
//
// Static obstacles stay in remembered contents because they were genuinely
// observed when the hex was revealed and cannot change — the existing sticky
// behavior, which is contract-correct rather than a leak.
func (t *toolkitKnowledge) KnownHexes(_ core.PlayerID) map[core.Hex]HexObservation {
	out := make(map[core.Hex]HexObservation, len(t.revealed))
	for h := range t.revealed {
		obs := HexObservation{
			Position: h,
			State:    KnowledgeStateRemembered,
			// Nothing in the toolkit sources terrain; the pre-fog projection
			// never set the wire field either. Named in the interface so the
			// toolkit knows to supply it.
			Terrain:  TerrainKindUnspecified,
			Contents: t.placements[h],
		}
		if zone, ok := t.data.Space.RegionAt(h); ok {
			obs.ZoneID = zone
		}
		if t.visible.Has(h) {
			obs.State = KnowledgeStateVisible
			obs.Edges = t.edges[h]
		}
		out[h] = obs
	}
	return out
}

var _ ViewerKnowledge = (*toolkitKnowledge)(nil)

// edgesByHex indexes every wall and door segment by the hex it belongs to, so a
// hex record can carry its own edges instead of the client re-associating a
// global list.
//
// This replaces the unconditional whole-room wall exposure that wallsToProto
// and doorWallsToProto performed — their own comments admitted "every viewer's
// snapshot carries every door regardless of RevealedHexes". Indexing here and
// attaching only to visible hexes is what closes that.
func edgesByHex(data *tkenc.Data) map[core.Hex][]Edge {
	out := make(map[core.Hex][]Edge)
	if data == nil {
		return out
	}

	if data.Space != nil {
		for _, w := range data.Space.Walls {
			if !w.BlocksMovement && !w.BlocksLoS {
				// Not a barrier worth putting on the wire, matching
				// wallKindFor's ok=false case.
				continue
			}
			from := core.HexFromCube(w.Start)
			out[from] = append(out[from], Edge{
				From:           from,
				To:             core.HexFromCube(w.End),
				BlocksMovement: w.BlocksMovement,
				BlocksLoS:      w.BlocksLoS,
			})
		}
	}

	for id, door := range data.Doors {
		if door == nil {
			continue
		}
		out[door.Position] = append(out[door.Position], Edge{
			From: door.Position,
			To:   doorPassageNeighbor(data.Space, door),
			// A closed door blocks both; an open one blocks neither. The
			// client reads DoorOpen/DoorLocked, so these flags describe the
			// barrier rather than re-deriving the door's kind.
			BlocksMovement: !door.Open,
			BlocksLoS:      !door.Open,
			DoorID:         string(id),
			DoorOpen:       door.Open,
			DoorLocked:     door.Locked,
		})
	}
	return out
}
