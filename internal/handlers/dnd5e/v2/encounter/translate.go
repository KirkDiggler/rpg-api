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

// ErrEventSuppressed is returned when the translator deliberately drops a
// toolkit event that has no proto wire shape. Used for cause-stage combat
// events (AttackResolvedEvent) where the proto only carries the effect
// (EntityDamaged). The stream loop must errors.Is-check and continue
// silently — neither logging (it's expected) nor sending.
//
// This differs from ErrViewerSawNothing (per-viewer empty slice) and
// ErrUnknownEventType (translator gap to log) — it's a deliberate design
// choice baked in per pat-v2-combat-event-translator.
var ErrEventSuppressed = errors.New("event deliberately suppressed by translator")

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
	case *events.AttackResolvedEvent:
		// Cause-stage event with no proto wire shape (proto designers chose
		// EntityDamaged as the canonical wire shape; AttackResolved is
		// narration/animation hooks the web doesn't render in this wave).
		// Suppress so the stream loop neither logs nor sends.
		return nil, ErrEventSuppressed
	case *events.DamageDealtEvent:
		return translateDamageDealtEvent(e, viewer, now)
	case *events.ConditionAppliedEvent:
		return translateConditionAppliedEvent(e, viewer, now)
	case *events.ModeChangedEvent:
		return translateModeChangedEvent(e, viewer, now)
	case *events.TurnStartedEvent:
		return translateTurnStartedEvent(e, viewer, now)
	case *events.TurnEndedEvent:
		return translateTurnEndedEvent(e, viewer, now)
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

// translateDamageDealtEvent maps the toolkit's DamageDealtEvent (effect of
// an attack) to the proto EntityDamaged envelope. Per
// pat-v2-combat-event-translator, this is the canonical wire shape for HP
// outcomes — the cause-stage AttackResolvedEvent is suppressed.
//
// Per-viewer projection: viewers absent from PerPlayer don't see the event;
// viewers with Visible:false return ErrViewerSawNothing for stream-loop
// silent skip. The toolkit's publish-side already gated audience by LoS to
// attacker OR target so a Visible:true entry means the viewer should see
// the wire-shape event.
func translateDamageDealtEvent(e *events.DamageDealtEvent, viewer core.PlayerID, now time.Time) (*encounterv2pb.EncounterEvent, error) {
	slice, ok := e.PerPlayer[viewer]
	if !ok || !slice.Visible {
		return nil, ErrViewerSawNothing
	}
	out := &encounterv2pb.EntityDamaged{
		EntityId: string(e.TargetID),
		Amount:   int32(e.Amount),
		HpAfter: &encounterv2pb.HitPoints{
			Current: int32(e.HPAfter),
			Max:     int32(e.MaxHP),
		},
	}
	if e.DamageType != "" {
		out.DamageType = damageRefFor(e.DamageType)
	}
	if e.SourceID != "" {
		s := string(e.SourceID)
		out.SourceEntityId = &s
	}
	return &encounterv2pb.EncounterEvent{
		Sequence:  int64(e.Sequence()),
		Timestamp: timestamppb.New(now),
		Event: &encounterv2pb.EncounterEvent_EntityDamaged{
			EntityDamaged: out,
		},
	}, nil
}

// translateConditionAppliedEvent maps the toolkit's ConditionAppliedEvent
// (effect of an action that applies a status — e.g., "poisoned" from a
// poisoned-attack) to the proto StatusApplied envelope.
//
// The toolkit publishes ConditionRef as the toolkit's three-part-ref string
// (e.g. "dnd5e:conditions:poisoned"). For Wave 2.8, the rpg-api translator
// emits the StatusEffect.Source as a Ref with the same id portion; the web
// resolves display_name / icon_hint from its own lookup table. Future waves
// can plumb richer status metadata when toolkit ships it.
func translateConditionAppliedEvent(e *events.ConditionAppliedEvent, viewer core.PlayerID, now time.Time) (*encounterv2pb.EncounterEvent, error) {
	slice, ok := e.PerPlayer[viewer]
	if !ok || !slice.Visible {
		return nil, ErrViewerSawNothing
	}
	status := &encounterv2pb.StatusEffect{
		Source: conditionRefFor(e.ConditionRef),
	}
	if e.DurationRounds > 0 {
		d := int32(e.DurationRounds)
		status.DurationRounds = &d
	}
	out := &encounterv2pb.StatusApplied{
		EntityId: string(e.TargetID),
		Status:   status,
	}
	if e.SourceID != "" {
		s := string(e.SourceID)
		out.SourceEntityId = &s
	}
	return &encounterv2pb.EncounterEvent{
		Sequence:  int64(e.Sequence()),
		Timestamp: timestamppb.New(now),
		Event: &encounterv2pb.EncounterEvent_StatusApplied{
			StatusApplied: out,
		},
	}, nil
}

// translateModeChangedEvent maps the toolkit's ModeChangedEvent to the
// proto ModeChanged envelope. Audience is implicit (every player sees mode
// flips), so we only check viewer-presence; Visible flag is always true on
// the toolkit side.
func translateModeChangedEvent(e *events.ModeChangedEvent, viewer core.PlayerID, now time.Time) (*encounterv2pb.EncounterEvent, error) {
	if _, ok := e.PerPlayer[viewer]; !ok {
		return nil, ErrViewerSawNothing
	}
	return &encounterv2pb.EncounterEvent{
		Sequence:  int64(e.Sequence()),
		Timestamp: timestamppb.New(now),
		Event: &encounterv2pb.EncounterEvent_ModeChanged{
			ModeChanged: &encounterv2pb.ModeChanged{
				From:   encounterModeToProto(e.From),
				To:     encounterModeToProto(e.To),
				Reason: e.Reason,
			},
		},
	}, nil
}

// translateTurnStartedEvent maps the toolkit's TurnStartedEvent to the proto
// TurnStarted envelope. Round is 1-indexed in both shapes.
func translateTurnStartedEvent(e *events.TurnStartedEvent, viewer core.PlayerID, now time.Time) (*encounterv2pb.EncounterEvent, error) {
	if _, ok := e.PerPlayer[viewer]; !ok {
		return nil, ErrViewerSawNothing
	}
	return &encounterv2pb.EncounterEvent{
		Sequence:  int64(e.Sequence()),
		Timestamp: timestamppb.New(now),
		Event: &encounterv2pb.EncounterEvent_TurnStarted{
			TurnStarted: &encounterv2pb.TurnStarted{
				EntityId: string(e.ActorID),
				Round:    int32(e.Round),
			},
		},
	}, nil
}

// translateTurnEndedEvent maps the toolkit's TurnEndedEvent to the proto
// TurnEnded envelope. Audience is implicit (every player sees turn
// transitions); viewer must be in PerPlayer.
func translateTurnEndedEvent(e *events.TurnEndedEvent, viewer core.PlayerID, now time.Time) (*encounterv2pb.EncounterEvent, error) {
	if _, ok := e.PerPlayer[viewer]; !ok {
		return nil, ErrViewerSawNothing
	}
	return &encounterv2pb.EncounterEvent{
		Sequence:  int64(e.Sequence()),
		Timestamp: timestamppb.New(now),
		Event: &encounterv2pb.EncounterEvent_TurnEnded{
			TurnEnded: &encounterv2pb.TurnEnded{
				EntityId: string(e.ActorID),
			},
		},
	}, nil
}

// damageRefFor builds a proto Ref for a toolkit damage-type string. The
// toolkit emits bare strings (e.g. "slashing", "fire", "untyped"); the proto
// wire shape uses a Ref triple. For Wave 2.8 we hardwire module=dnd5e and
// type=damage; future modules can route off the toolkit string when they
// ship.
func damageRefFor(toolkitDamageType string) *encounterv2pb.Ref {
	return &encounterv2pb.Ref{
		Module: "dnd5e",
		Type:   "damage",
		Id:     toolkitDamageType,
	}
}

// conditionRefFor builds a proto Ref for a toolkit condition-ref string.
// The toolkit ships a fully-qualified ref (e.g. "dnd5e:conditions:poisoned");
// we parse it back into module/type/id. Bare strings are treated as ids
// under module=dnd5e, type=condition.
func conditionRefFor(toolkitConditionRef string) *encounterv2pb.Ref {
	parts := splitRef(toolkitConditionRef)
	if len(parts) == 3 {
		return &encounterv2pb.Ref{Module: parts[0], Type: parts[1], Id: parts[2]}
	}
	return &encounterv2pb.Ref{Module: "dnd5e", Type: "condition", Id: toolkitConditionRef}
}

// splitRef splits a "module:type:id" string into its three parts. Returns
// nil when the input doesn't have exactly two colons. Callers should treat
// the nil return as "not a fully-qualified ref" and fall back to defaults.
func splitRef(s string) []string {
	parts := []string{}
	start := 0
	colons := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ':' {
			parts = append(parts, s[start:i])
			start = i + 1
			colons++
		}
	}
	if colons != 2 {
		return nil
	}
	parts = append(parts, s[start:])
	return parts
}

// encounterModeToProto maps the toolkit's core.EncounterMode enum to the
// proto EncounterMode enum. The toolkit's ModeUnspecified maps to
// ENCOUNTER_MODE_UNSPECIFIED; FreeRoam → FREE_ROAM; TurnBased → TURN_BASED.
func encounterModeToProto(m core.EncounterMode) encounterv2pb.EncounterMode {
	switch m {
	case core.ModeFreeRoam:
		return encounterv2pb.EncounterMode_ENCOUNTER_MODE_FREE_ROAM
	case core.ModeTurnBased:
		return encounterv2pb.EncounterMode_ENCOUNTER_MODE_TURN_BASED
	}
	return encounterv2pb.EncounterMode_ENCOUNTER_MODE_UNSPECIFIED
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
