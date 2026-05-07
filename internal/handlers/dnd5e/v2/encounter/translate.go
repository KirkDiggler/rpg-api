// Package encounter is the v2 encounter handler — wire shim over the
// rpg-toolkit encounter SDK. translate.go converts toolkit events into
// v1alpha2 proto envelopes per viewer.
package encounter

import (
	"errors"
	"fmt"
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
	// range over keys (the hex), not values (struct{}).
	hexes := make([]*encounterv2pb.Hex, 0, len(slice.Hexes))
	for h := range slice.Hexes {
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
