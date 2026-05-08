// Package encounter is the v2 encounter handler — wire shim over the
// rpg-toolkit encounter SDK. translate.go converts toolkit events into
// v1alpha2 proto envelopes per viewer.
package encounter

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/encounter/events"
)

// ErrViewerSawNothing is returned when an event was delivered to a viewer
// but the viewer's per-player slice contains no visible content. The
// stream loop should errors.Is-check and continue silently.
var ErrViewerSawNothing = errors.New("viewer saw nothing in this event")

// ErrUnknownEventType is returned when the translator has no mapping for
// the given toolkit event type. The stream loop should log + continue.
var ErrUnknownEventType = errors.New("translator has no mapping for this event type")

// HexToPosition maps a toolkit cube hex (Q,R,S) to a proto Position (x,y,z).
// The proto invariant x+y+z=0 is preserved by construction.
func HexToPosition(h core.Hex) *encounterv2pb.Position {
	return &encounterv2pb.Position{X: int32(h.Q), Y: int32(h.R), Z: int32(h.S)}
}

// TranslateEvent maps a toolkit EncounterEvent to a v1alpha2 envelope from
// the perspective of viewer. Returns:
//   - (*EncounterEvent, nil) for normal translation
//   - (nil, ErrViewerSawNothing) when viewer had nothing visible in evt
//   - (nil, ErrUnknownEventType) when evt's concrete type has no mapping
//   - (nil, otherErr) for genuine bugs the caller should escalate
func TranslateEvent(evt events.EncounterEvent, viewer core.PlayerID, now time.Time) (*encounterv2pb.EncounterEvent, error) {
	switch e := evt.(type) {
	case *events.MoveEvent:
		return translateMoveEvent(e, viewer, now)
	case *events.HexRevealedEvent:
		return translateHexRevealedEvent(e, viewer, now)
	case *events.EntityAppearedEvent:
		return translateEntityAppearedEvent(e, viewer, now)
	case *events.EntityDisappearedEvent:
		return translateEntityDisappearedEvent(e, viewer, now)
	case *events.DoorOpenedEvent:
		return translateDoorOpenedEvent(e, viewer, now)
	default:
		return nil, fmt.Errorf("%w: %T", ErrUnknownEventType, evt)
	}
}

func translateMoveEvent(e *events.MoveEvent, viewer core.PlayerID, now time.Time) (*encounterv2pb.EncounterEvent, error) {
	slice, ok := e.PerPlayer[viewer]
	if !ok || len(slice.SeenSegments) == 0 {
		return nil, ErrViewerSawNothing
	}
	// Use the per-viewer SeenSegments (not the full e.Path) so the wire
	// reflects the viewer's reality — the toolkit broker delivered this
	// event because the viewer saw at least one segment, but only those
	// segments belong on the wire (per spec: per-viewer reality).
	path := make([]*encounterv2pb.Position, 0, len(slice.SeenSegments))
	for _, h := range slice.SeenSegments {
		path = append(path, HexToPosition(h))
	}
	return &encounterv2pb.EncounterEvent{
		Sequence:  int64(e.Sequence()),
		Timestamp: timestamppb.New(now),
		Event: &encounterv2pb.EncounterEvent_EntityMoved{
			EntityMoved: &encounterv2pb.EntityMoved{
				EntityId:   string(e.Mover),
				From:       path[0],
				To:         path[len(path)-1],
				ActualPath: path,
			},
		},
	}, nil
}

func translateHexRevealedEvent(e *events.HexRevealedEvent, viewer core.PlayerID, now time.Time) (*encounterv2pb.EncounterEvent, error) {
	slice, ok := e.PerPlayer[viewer]
	if !ok || len(slice.Hexes) == 0 {
		return nil, ErrViewerSawNothing
	}
	// HexRevealedSlice.Hexes is core.HexSet which is map[Hex]struct{} —
	// range over keys (the hex), not values (struct{}). Sort by (Q, R, S)
	// so wire output is deterministic; map iteration order is randomized
	// in Go and would otherwise create flaky tests / golden comparisons.
	keys := make([]core.Hex, 0, len(slice.Hexes))
	for h := range slice.Hexes {
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
	return &encounterv2pb.EncounterEvent{
		Sequence:  int64(e.Sequence()),
		Timestamp: timestamppb.New(now),
		Event: &encounterv2pb.EncounterEvent_GeometryRevealed{
			GeometryRevealed: &encounterv2pb.GeometryRevealed{
				Hexes: hexes,
			},
		},
	}, nil
}

// translateEntityAppearedEvent maps the toolkit's EntityAppearedEvent to the
// v1alpha2 EntityAppeared proto envelope. The proto carries an Entity message
// (id + position) plus a free-form reason string. The toolkit event provides
// only EntityID + Position; the wire form populates a minimal Entity{id,
// position} and leaves richer fields (type, display_name, hp, etc.) for the
// web to look up from its snapshot. See gap note in EntityDisappeared below.
func translateEntityAppearedEvent(e *events.EntityAppearedEvent, viewer core.PlayerID, now time.Time) (*encounterv2pb.EncounterEvent, error) {
	if _, ok := e.PerPlayer[viewer]; !ok {
		return nil, ErrViewerSawNothing
	}
	return &encounterv2pb.EncounterEvent{
		Sequence:  int64(e.Sequence()),
		Timestamp: timestamppb.New(now),
		Event: &encounterv2pb.EncounterEvent_EntityAppeared{
			EntityAppeared: &encounterv2pb.EntityAppeared{
				Entity: &encounterv2pb.Entity{
					Id:       string(e.Entity),
					Position: HexToPosition(e.Position),
				},
				Reason: "entered LOS",
			},
		},
	}, nil
}

// translateEntityDisappearedEvent maps the toolkit's EntityDisappearedEvent to
// the v1alpha2 EntityDisappeared proto envelope. The proto's last_known_position
// field carries the viewer's per-stream last-seen hex (different viewers may
// have last-seen the entity at different hexes during pass-through), allowing
// the web to render "freeze marker at last-seen hex" without client-side
// game-state tracking. Per-viewer correctness comes from picking
// e.PerPlayer[viewer] inside the handler's per-stream translator call.
func translateEntityDisappearedEvent(e *events.EntityDisappearedEvent, viewer core.PlayerID, now time.Time) (*encounterv2pb.EncounterEvent, error) {
	lastKnown, ok := e.PerPlayer[viewer]
	if !ok {
		return nil, ErrViewerSawNothing
	}
	return &encounterv2pb.EncounterEvent{
		Sequence:  int64(e.Sequence()),
		Timestamp: timestamppb.New(now),
		Event: &encounterv2pb.EncounterEvent_EntityDisappeared{
			EntityDisappeared: &encounterv2pb.EntityDisappeared{
				EntityId:          string(e.Entity),
				Reason:            "left LOS",
				LastKnownPosition: HexToPosition(lastKnown),
			},
		},
	}, nil
}

// translateDoorOpenedEvent maps the toolkit's DoorOpenedEvent to the
// v1alpha2 DoorOpened proto envelope. Wave 2.7 deliberately splits the
// cause/effect events: this envelope carries only the door identity (the
// "what happened"); the parallel HexRevealedEvent published alongside
// OpenDoor delivers the geometry deltas through GeometryRevealed (the
// "what changed"). See rpg-toolkit/encounter/events/hex_revealed.go for
// the rationale and Wave 2.7 plan for the API-side decision to mirror
// that split — do NOT combine them here.
//
// revealed_walls / removed_walls are left empty: walls are not modeled in
// the v0.2.0 toolkit. The proto carries them for forward compatibility
// when the toolkit grows wall geometry.
func translateDoorOpenedEvent(e *events.DoorOpenedEvent, viewer core.PlayerID, now time.Time) (*encounterv2pb.EncounterEvent, error) {
	slice, ok := e.PerPlayer[viewer]
	if !ok || !slice.Visible {
		return nil, ErrViewerSawNothing
	}
	return &encounterv2pb.EncounterEvent{
		Sequence:  int64(e.Sequence()),
		Timestamp: timestamppb.New(now),
		Event: &encounterv2pb.EncounterEvent_DoorOpened{
			DoorOpened: &encounterv2pb.DoorOpened{
				DoorEntityId: string(e.DoorID),
				// revealed_hexes / revealed_walls / removed_walls are
				// intentionally empty: the parallel HexRevealedEvent
				// (translated to GeometryRevealed) carries the geometry
				// delta. Cause/effect events stay separate on the wire.
			},
		},
	}, nil
}

// TranslateSnapshot wraps a projected proto Encounter in a synthetic
// SnapshotDelivered proto event. Sequence 0 marks it as pre-history; delta
// events start at 1. The Encounter field is populated via ProjectFor before
// this call — the handler owns that call so it can reuse the result for
// BuildReplayEvents without a second projection.
func TranslateSnapshot(pbEncounter *encounterv2pb.Encounter, now time.Time) *encounterv2pb.EncounterEvent {
	return &encounterv2pb.EncounterEvent{
		Sequence:  0,
		Timestamp: timestamppb.New(now),
		Event: &encounterv2pb.EncounterEvent_SnapshotDelivered{
			SnapshotDelivered: &encounterv2pb.SnapshotDelivered{
				Encounter: pbEncounter,
			},
		},
	}
}

// BuildReplayEvents synthesizes the initial EntityAppeared and GeometryRevealed
// events from the already-projected proto Encounter and the viewer's Snapshot.
// These are sent immediately after SnapshotDelivered so a freshly-connected
// client sees the full current state before any live broker event arrives.
//
// Entity replay: one EntityAppeared per entity in pbEncounter.Space.Entities.
// Geometry replay: one GeometryRevealed carrying pbEncounter.Space.Hexes, or
// nothing if Hexes is empty. ProjectFor already produced both in deterministic
// order, so this function is a pure translation — no sorting or projection.
func BuildReplayEvents(
	pbEncounter *encounterv2pb.Encounter,
	now time.Time,
) []*encounterv2pb.EncounterEvent {
	var out []*encounterv2pb.EncounterEvent

	space := pbEncounter.GetSpace()
	if space == nil {
		return out
	}

	// EntityAppeared for each entity visible to the viewer (including the viewer's
	// own entity so the client can render its initial position).
	for _, entity := range space.GetEntities() {
		out = append(out, &encounterv2pb.EncounterEvent{
			Sequence:  0,
			Timestamp: timestamppb.New(now),
			Event: &encounterv2pb.EncounterEvent_EntityAppeared{
				EntityAppeared: &encounterv2pb.EntityAppeared{
					Entity: entity,
					Reason: "initial state",
				},
			},
		})
	}

	// GeometryRevealed for the viewer's revealed hex set.
	if hexes := space.GetHexes(); len(hexes) > 0 {
		out = append(out, &encounterv2pb.EncounterEvent{
			Sequence:  0,
			Timestamp: timestamppb.New(now),
			Event: &encounterv2pb.EncounterEvent_GeometryRevealed{
				GeometryRevealed: &encounterv2pb.GeometryRevealed{
					Hexes: hexes,
				},
			},
		})
	}

	return out
}
