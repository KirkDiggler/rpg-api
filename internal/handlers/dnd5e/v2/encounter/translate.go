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
	toolkitcore "github.com/KirkDiggler/rpg-toolkit/core"
	"github.com/KirkDiggler/rpg-toolkit/encounter"
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

// refModuleDnd5e is the canonical module string for the dnd5e rulebook. Used
// as the Module field of proto Refs when the toolkit emits bare strings (no
// fully-qualified module:type:id) and the v2 encounter wire needs to attribute
// the ref to a specific rulebook. Future rulebooks would add their own const.
const refModuleDnd5e = "dnd5e"

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
	case *events.ActionResolvedEvent:
		return translateActionResolvedEvent(e, viewer)
	case *events.AttackResolvedEvent:
		// TakeAction wave (#597): the proto now has an AttackResolved variant
		// (protos v0.1.100), so the cause-stage attack event is no longer
		// suppressed — it is translated and sent, carrying hit/miss/crit/roll
		// detail. This is the #594 fix: misses become visible on the wire.
		return translateAttackResolvedEvent(e, viewer)
	case *events.TurnStateChangedEvent:
		return translateTurnStateChangedEvent(e, viewer)
	case *events.DamageDealtEvent:
		return translateDamageDealtEvent(e, viewer, now)
	case *events.ConditionAppliedEvent:
		return translateConditionAppliedEvent(e, viewer, now)
	case *events.ConditionRemovedEvent:
		return translateConditionRemovedEvent(e, viewer, now)
	case *events.ResourceChangedEvent:
		return translateResourceChangedEvent(e, viewer, now)
	case *events.ModeChangedEvent:
		return translateModeChangedEvent(e, viewer, now)
	case *events.TurnStartedEvent:
		return translateTurnStartedEvent(e, viewer, now)
	case *events.TurnEndedEvent:
		return translateTurnEndedEvent(e, viewer, now)
	case *events.EntityDiedEvent:
		return translateEntityDiedEvent(e, viewer, now)
	case *events.DeathSaveRolledEvent:
		return translateDeathSaveRolledEvent(e, viewer, now)
	case *events.EntityStabilizedEvent:
		return translateEntityStabilizedEvent(e, viewer, now)
	case *events.EntityRemovedEvent:
		return translateEntityRemovedEvent(e, viewer, now)
	case *events.EncounterEndedEvent:
		return translateEncounterEndedEvent(e, viewer, now)
	case *events.InputRequiredDeliveredEvent:
		// Wave 2.11d: prompt content lives on encounter Data, not on the
		// event payload. The stream loop calls TranslateInputRequiredDelivered
		// directly with the encounter snapshot loaded; reaching this case
		// from TranslateEvent (which has no data lookup) means the caller
		// should be using the data-aware translator path instead.
		return nil, fmt.Errorf("%w: InputRequiredDeliveredEvent requires data lookup; "+
			"use TranslateInputRequiredDelivered from the stream loop", ErrUnknownEventType)
	default:
		return nil, fmt.Errorf("%w: %T", ErrUnknownEventType, evt)
	}
}

// TranslateInputRequiredDelivered is the data-aware translator for
// InputRequiredDeliveredEvent. The stream loop calls this when it sees the
// event so the prompt content (currently on encounter Data, not the event)
// can be loaded and packaged into the wire shape.
//
// Returns ErrViewerSawNothing when the event's audience does not include
// viewer. Returns ErrEventSuppressed when no pending prompt exists for the
// reactor (a benign race: the prompt cleared between event publish and
// translation).
func TranslateInputRequiredDelivered(
	evt *events.InputRequiredDeliveredEvent,
	viewer core.PlayerID,
	now time.Time,
	pendingPrompt *encounter.PendingReactionPrompt,
) (*encounterv2pb.EncounterEvent, error) {
	if evt.ReactorID != viewer {
		return nil, ErrViewerSawNothing
	}
	if pendingPrompt == nil {
		// Benign race: prompt cleared between publish and translation, OR
		// wrong reactor. Suppress so the stream loop continues quietly.
		return nil, ErrEventSuppressed
	}

	// Build the InputRequired oneof variant for the prompt kind. Wave 2.11d
	// only emits PromptKindReaction; future kinds branch here.
	switch evt.PromptKind {
	case events.PromptKindReaction:
		ref := refStringToProto(pendingPrompt.ConditionRef)
		return &encounterv2pb.EncounterEvent{
			Sequence:  int64(evt.Sequence()), //nolint:gosec // sequence is monotonic; fits int64
			Timestamp: timestamppb.New(now),
			Event: &encounterv2pb.EncounterEvent_InputRequiredDelivered{
				InputRequiredDelivered: &encounterv2pb.InputRequiredDelivered{
					InputRequired: &encounterv2pb.InputRequired{
						Kind: &encounterv2pb.InputRequired_ReactionPrompt{
							ReactionPrompt: &encounterv2pb.ReactionPrompt{
								ReactionRef:           ref,
								TriggerKind:           pendingPrompt.TriggerKind,
								TriggerSourceEntityId: string(pendingPrompt.SourceEntity),
								TargetEntityId:        string(pendingPrompt.ReactorEntityID),
							},
						},
					},
				},
			},
		}, nil
	default:
		return nil, fmt.Errorf("%w: unknown PromptKind %q", ErrUnknownEventType, evt.PromptKind)
	}
}

// refStringToProto parses a "module:type:id" canonical ref string into the
// proto Ref shape. Returns a Ref with the parsed parts; if parsing fails
// (malformed input), returns a Ref carrying the raw id and the dnd5e module
// fallback so the wire never carries an empty ref.
func refStringToProto(s string) *encounterv2pb.Ref {
	module, typ, id := refModuleDnd5e, "", s
	// canonical shape: module:type:id
	first := indexByte(s, ':')
	if first >= 0 {
		module = s[:first]
		rest := s[first+1:]
		second := indexByte(rest, ':')
		if second >= 0 {
			typ = rest[:second]
			id = rest[second+1:]
		}
	}
	return &encounterv2pb.Ref{Module: module, Type: typ, Id: id}
}

// indexByte mirrors strings.IndexByte without pulling the strings package
// just for one helper. Returns -1 if not found.
func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
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
// v1alpha2 EntityAppeared proto envelope, carrying only what the toolkit
// event itself provides (EntityID + Position) — a bare Entity{id, position}
// with Type/Hp/ArmorClass/the Character-or-Monster oneof all left unset.
//
// The live stream (handler.go's translateForStream) does NOT call this —
// rpg-api#644's playtest follow-up replaced that call site with
// translateEntityAppearedEventWithData, which looks the entity up in the
// already-loaded encounter data to populate the fields this function leaves
// bare (that gap — a monster appearing over the wire as type=UNSPECIFIED,
// rendering as a generic capsule instead of its model — was the playtest
// finding). This function remains the TranslateEvent dispatch's fallback for
// any caller with only the bare toolkit event and no data to enrich it with
// (see TestTranslateEvent_EntityAppearedEvent_HappyPath).
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

// translateEntityAppearedEventWithData is the data-aware translator for
// EntityAppearedEvent (rpg-api#644 playtest follow-up). The toolkit event
// carries only an entity ID + position — see translateEntityAppearedEvent's
// doc for why: a deliberate minimal cause-event shape the toolkit uses for
// BOTH "a player becomes visible to another player" (encounter.go's
// applyAndPublishMove) and, since rpg-toolkit#762, "a monster becomes
// visible to the mover" (monsterVisibilityTransitions) — in the monster
// case the toolkit has the full *MonsterData in hand at the publish site but
// deliberately narrows to just ID+Position before constructing the event,
// mirroring the DoorOpened/parallel-HexRevealedEvent cause/effect split
// already established in this file: the toolkit's cause event carries
// identity, the consumer enriches.
//
// entityForID looks the entity up in the already-loaded data to build the
// FULL wire Entity (type + hp + armor_class + character/monster data),
// reusing the exact same playerEntity/monsterEntity builders ProjectFor's
// snapshot path uses — one authoritative place for "how does a
// PlayerData/MonsterData become a wire Entity" so the live and snapshot
// paths cannot drift the way they did before this fix.
//
// data is loaded fresh by the caller (translateForStream) rather than
// threaded through TranslateEvent's generic signature — mirrors the
// InputRequiredDeliveredEvent / InitiativeRolled data-aware branches already
// in this file. Unlike InitiativeRolled (rpg-api#647: the roster value
// doesn't exist anywhere until the SAME in-flight request rolls it), this
// read is NOT racy: the appearing entity's identity was always persisted by
// an earlier, separate, already-completed request (AddPlayer/AddMonster
// inside StartEncounter or devcombat.Inject) — the only call site that can
// ever produce this event is enc.Move(), which always runs against an
// encounter MoveEntity's orchestrator already loaded from the repo. No
// retry needed (contrast loadRolledInitiative below).
func translateEntityAppearedEventWithData(
	e *events.EntityAppearedEvent, viewer core.PlayerID, now time.Time, data *encounter.Data,
) (*encounterv2pb.EncounterEvent, error) {
	if _, ok := e.PerPlayer[viewer]; !ok {
		return nil, ErrViewerSawNothing
	}

	entity := entityForID(data, e.Entity)
	if entity == nil {
		// Defensive: the entity is gone from data between publish and this
		// read (e.g. killed the same tick) — fall back to the bare shape
		// rather than dropping the event outright; the client still learns
		// something appeared, just without enrichment.
		entity = &encounterv2pb.Entity{Id: string(e.Entity), Position: HexToPosition(e.Position)}
	} else {
		// Position must be the EVENT's own reported position, not the
		// entity's current/final stored position from data — these can
		// legitimately differ. The toolkit's ProjectVisibilityTransition can
		// report "appeared" at an INTERMEDIATE hex of a multi-hex
		// pass-through move, not the mover's final resting position (a real
		// case this fix regressed against
		// TestMovementSlicePerViewerProjection_AsymmetricLoS until caught:
		// viewer B, SightRange 1, sees mover A only at the one hex A passes
		// through B's vision, which is not where A ends up after their full
		// move). entityForID's builders (playerEntity/monsterEntity) get
		// every OTHER field right from the authoritative stored data — type,
		// hp, armor_class, character/monster data are not position-scoped
		// the way this is — only Position needs the event's own value.
		entity.Position = HexToPosition(e.Position)
	}

	return &encounterv2pb.EncounterEvent{
		Sequence:  int64(e.Sequence()),
		Timestamp: timestamppb.New(now),
		Event: &encounterv2pb.EncounterEvent_EntityAppeared{
			EntityAppeared: &encounterv2pb.EntityAppeared{
				Entity: entity,
				Reason: "entered LOS",
			},
		},
	}, nil
}

// entityForID looks up id in data's players/monsters and builds the wire
// Entity for it via the same playerEntity/monsterEntity builders ProjectFor
// uses, or nil if id belongs to neither (should not happen for a live
// EntityAppearedEvent, whose Entity always names a combatant already in
// this encounter's Players/Monsters at publish time — see
// translateEntityAppearedEventWithData's doc on why that read isn't racy).
//
// The position passed into playerEntity here is a placeholder — the caller
// (translateEntityAppearedEventWithData) always overwrites .Position with
// the event's own reported position afterward (see its doc for why), so
// this does not need pd.View to be populated to enrich the rest of the
// entity's fields.
func entityForID(data *encounter.Data, id core.EntityID) *encounterv2pb.Entity {
	if data == nil {
		return nil
	}
	if m, ok := data.Monsters[id]; ok {
		return monsterEntity(m)
	}
	if pid := playerSeatForEntity(data, id); pid != "" {
		if pd := data.Players[pid]; pd != nil {
			return playerEntity(pd, core.Hex{})
		}
	}
	return nil
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

// translateActionResolvedEvent maps the toolkit's ActionResolvedEvent — the
// first-class "an action was taken" umbrella beat (North-Star Invariant 9) — to
// the proto ActionResolved envelope. Every player-facing action (attack, Dodge,
// Dash, the Monk bonus strike) emits one. rpg-api projects it field-for-field:
// actor, action_ref, target, economy_consumed; it authors nothing and computes
// nothing (Invariant 2).
//
// The envelope timestamp is the event's game-time OccurredAt (Invariant 5, NOT
// rpg-api's wall clock) and the correlation_id is the event's CorrelationID
// (Invariant 8) so the web reassembles "this damage came from that action" —
// the AttackResolved / EntityDamaged / StatusApplied events the action caused
// all carry the same id.
//
// Per-viewer projection: viewers absent from PerPlayer or with Visible:false
// return ErrViewerSawNothing for the stream loop's silent skip.
func translateActionResolvedEvent(e *events.ActionResolvedEvent, viewer core.PlayerID) (*encounterv2pb.EncounterEvent, error) {
	slice, ok := e.PerPlayer[viewer]
	if !ok || !slice.Visible {
		return nil, ErrViewerSawNothing
	}
	return &encounterv2pb.EncounterEvent{
		Sequence:      int64(e.Sequence()), //nolint:gosec // sequence is monotonic; fits int64
		Timestamp:     timestamppb.New(e.OccurredAt()),
		CorrelationId: string(e.CorrelationID()),
		Event: &encounterv2pb.EncounterEvent_ActionResolved{
			ActionResolved: &encounterv2pb.ActionResolved{
				ActorEntityId:   string(e.ActorID),
				ActionRef:       actionRefToProto(e.ActionRef),
				TargetEntityId:  string(e.TargetID),
				EconomyConsumed: economyConsumedToProto(e.EconomyConsumed),
			},
		},
	}, nil
}

// translateAttackResolvedEvent maps the toolkit's AttackResolvedEvent (the
// cause/roll-detail event) to the proto AttackResolved envelope. TakeAction
// wave (#597): no longer suppressed — it fires on hit AND miss (the #594 fix),
// so the web can render a swing-and-miss. The HP outcome still lives on the
// parallel DamageDealtEvent → EntityDamaged (published on hit only); this event
// carries the to-hit story (hit/crit/roll/bonus/AC).
//
// Timestamp + correlation_id come from the event (Invariants 5, 8) so this
// attack ties to its ActionResolved umbrella and the EntityDamaged it causes.
func translateAttackResolvedEvent(e *events.AttackResolvedEvent, viewer core.PlayerID) (*encounterv2pb.EncounterEvent, error) {
	slice, ok := e.PerPlayer[viewer]
	if !ok || !slice.Visible {
		return nil, ErrViewerSawNothing
	}
	return &encounterv2pb.EncounterEvent{
		Sequence:      int64(e.Sequence()), //nolint:gosec // sequence is monotonic; fits int64
		Timestamp:     timestamppb.New(e.OccurredAt()),
		CorrelationId: string(e.CorrelationID()),
		Event: &encounterv2pb.EncounterEvent_AttackResolved{
			AttackResolved: &encounterv2pb.AttackResolved{
				AttackerEntityId:    string(e.AttackerID),
				TargetEntityId:      string(e.TargetID),
				Hit:                 e.Hit,
				Critical:            e.Critical,
				AttackRoll:          int32(e.AttackRoll),  //nolint:gosec // a d20 roll fits int32
				AttackBonus:         int32(e.AttackBonus), //nolint:gosec // a to-hit modifier fits int32
				TargetAc:            int32(e.TargetAC),    //nolint:gosec // an AC value fits int32
				HasAdvantage:        e.HasAdvantage,
				HasDisadvantage:     e.HasDisadvantage,
				AdvantageSources:    toolkitRefsToProto(e.AdvantageSources),
				DisadvantageSources: toolkitRefsToProto(e.DisadvantageSources),
			},
		},
	}, nil
}

// toolkitRefsToProto projects a slice of the toolkit's structured core.Ref
// (module/type/id) onto the proto Ref triple, verbatim — no rules
// conditionals, no remapping (Invariant 2). Used for AttackResolved's
// advantage/disadvantage source refs, which the rulebook already resolved
// (e.g. refs.Conditions.Dodging()); rpg-api only forwards them. Returns nil
// for an empty/nil input, AND when every entry is nil, so the proto field
// stays unset rather than an allocated-but-empty slice on the wire.
func toolkitRefsToProto(refs []*toolkitcore.Ref) []*encounterv2pb.Ref {
	if len(refs) == 0 {
		return nil
	}
	out := make([]*encounterv2pb.Ref, 0, len(refs))
	for _, r := range refs {
		if r == nil {
			continue
		}
		out = append(out, &encounterv2pb.Ref{
			Module: r.Module,
			Type:   r.Type,
			Id:     r.ID,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// translateTurnStateChangedEvent maps the toolkit's TurnStateChangedEvent — the
// pushed menu/economy refresh (North-Star Invariant 12) — to the proto
// TurnStateChanged envelope. The engine computes the snapshot (economy +
// available menu) from the actor's own rules engine; rpg-api projects it
// field-for-field onto the wire TurnState (Invariant 11). No menu logic, no
// availability decision, no reason string is authored here.
//
// Audience is the actor's own controlling player (turn state is that player's
// private "what can I do now" view); viewers absent or Visible:false return
// ErrViewerSawNothing. correlation_id ties a post-action refresh to the action
// that caused it (empty for turn-start refreshes — Invariant 8).
func translateTurnStateChangedEvent(e *events.TurnStateChangedEvent, viewer core.PlayerID) (*encounterv2pb.EncounterEvent, error) {
	slice, ok := e.PerPlayer[viewer]
	if !ok || !slice.Visible {
		return nil, ErrViewerSawNothing
	}
	return &encounterv2pb.EncounterEvent{
		Sequence:      int64(e.Sequence()), //nolint:gosec // sequence is monotonic; fits int64
		Timestamp:     timestamppb.New(e.OccurredAt()),
		CorrelationId: string(e.CorrelationID()),
		Event: &encounterv2pb.EncounterEvent_TurnStateChanged{
			TurnStateChanged: &encounterv2pb.TurnStateChanged{
				TurnState: turnStateSnapshotToProto(e.State, e.ActorID),
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
	if len(e.Components) > 0 {
		out.DamageBreakdown = make([]*encounterv2pb.DamageComponent, 0, len(e.Components))
		for _, c := range e.Components {
			pb := &encounterv2pb.DamageComponent{
				Source:     c.Source,
				Amount:     int32(c.Amount), //nolint:gosec // damage amounts are bounded game values that fit int32
				IsCritical: c.IsCritical,
			}
			if c.DamageType != "" {
				pb.DamageType = damageRefFor(c.DamageType)
			}
			out.DamageBreakdown = append(out.DamageBreakdown, pb)
		}
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

// translateConditionRemovedEvent maps the toolkit's ConditionRemovedEvent
// (a status clearing — e.g., "dodging" wearing off at the dodger's next
// turn start) to the proto StatusRemoved envelope. Mirrors
// translateConditionAppliedEvent's shape (rpg-api#622): same per-viewer
// visibility check and the same conditionRefFor parsing for ConditionRef.
func translateConditionRemovedEvent(e *events.ConditionRemovedEvent, viewer core.PlayerID, now time.Time) (*encounterv2pb.EncounterEvent, error) {
	slice, ok := e.PerPlayer[viewer]
	if !ok || !slice.Visible {
		return nil, ErrViewerSawNothing
	}
	out := &encounterv2pb.StatusRemoved{
		EntityId:     string(e.TargetID),
		StatusSource: conditionRefFor(e.ConditionRef),
	}
	return &encounterv2pb.EncounterEvent{
		Sequence:  int64(e.Sequence()),
		Timestamp: timestamppb.New(now),
		Event: &encounterv2pb.EncounterEvent_StatusRemoved{
			StatusRemoved: out,
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

// translateInitiativeRolledEvent builds the proto InitiativeRolled envelope
// carrying the encounter's freshly-rolled turn order (rpg-api#644 playtest
// follow-up). Unlike every other translator in this file, it has no
// corresponding toolkit event to translate FROM: rollInitiative (inside
// SetMode, rpg-toolkit encounter/combat.go) mutates data.Initiative in place
// but publishes no dedicated roster event — only ModeChangedEvent and
// TurnStartedEvent fire at the FREE_ROAM->TURN_BASED transition, and neither
// proto carries a roster field (ModeChanged: from/to/reason; TurnStarted:
// entity_id/round — see events.proto). InitiativeRolled{order} is a separate
// message already defined on the wire (events.proto's EncounterEvent oneof,
// field 41) that no rpg-api code populated until now — zero proto changes
// needed.
//
// The stream loop (handler.go's translateForStream) calls this once,
// alongside the normal ModeChanged translation, when it observes the
// transition to TURN_BASED — not on every TurnStartedEvent, which also
// fires on every later per-turn advance and would otherwise resend the same
// static roster every turn. seq is shared with the paired ModeChangedEvent:
// this envelope is that transition's effect, not an independently-caused
// event of its own (mirrors the DoorOpened/parallel-HexRevealedEvent
// cause/effect split elsewhere in this file, except here one toolkit cause
// produces two wire envelopes instead of two toolkit events each producing
// one).
func translateInitiativeRolledEvent(seq uint64, initiative []core.EntityID, now time.Time) *encounterv2pb.EncounterEvent {
	order := make([]string, 0, len(initiative))
	for _, eid := range initiative {
		order = append(order, string(eid))
	}
	return &encounterv2pb.EncounterEvent{
		Sequence:  int64(seq), //nolint:gosec // sequence is monotonic; fits int64
		Timestamp: timestamppb.New(now),
		Event: &encounterv2pb.EncounterEvent_InitiativeRolled{
			InitiativeRolled: &encounterv2pb.InitiativeRolled{
				Order: order,
			},
		},
	}
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

// translateEntityDiedEvent maps the toolkit's EntityDiedEvent (cause /
// narrative) to the proto EntityDied envelope. Per-viewer projection: the
// toolkit publishes a Visible:false slice for viewers who lack LoS to both
// the dying entity and the killer; the stream loop must drop those via
// ErrViewerSawNothing rather than send a Visible:false event downstream
// (no wire shape carries visibility metadata for death).
//
// KillerEntityId is a proto optional field (oneof). The toolkit emits
// KillerID="" when the cause has no single attributable attacker
// (environmental damage, future indirect-kill paths); leave the proto
// field unset in that case so clients can distinguish "killed by X" from
// "killed by something".
func translateEntityDiedEvent(e *events.EntityDiedEvent, viewer core.PlayerID, now time.Time) (*encounterv2pb.EncounterEvent, error) {
	slice, ok := e.PerPlayer[viewer]
	if !ok || !slice.Visible {
		return nil, ErrViewerSawNothing
	}
	out := &encounterv2pb.EntityDied{
		EntityId: string(e.EntityID),
	}
	if e.KillerID != "" {
		k := string(e.KillerID)
		out.KillerEntityId = &k
	}
	return &encounterv2pb.EncounterEvent{
		Sequence:  int64(e.Sequence()),
		Timestamp: timestamppb.New(now),
		Event: &encounterv2pb.EncounterEvent_EntityDied{
			EntityDied: out,
		},
	}, nil
}

// translateDeathSaveRolledEvent maps the toolkit's DeathSaveRolledEvent
// (rpg-toolkit#741) to the proto DeathSaveRolled envelope. Fires on EVERY
// death save roll (or automatic damage-while-unconscious failure — Roll==0)
// so clients can show death saves resolving in real time, not just the
// terminal Stabilized/Dead transition. Same per-viewer projection template
// as translateEntityDiedEvent (LoS to the rolling entity) — no
// killer/causer concept. Every field is copied verbatim per the boundary
// rule; the toolkit has already computed successes/failures/crit state.
func translateDeathSaveRolledEvent(e *events.DeathSaveRolledEvent, viewer core.PlayerID, now time.Time) (*encounterv2pb.EncounterEvent, error) {
	slice, ok := e.PerPlayer[viewer]
	if !ok || !slice.Visible {
		return nil, ErrViewerSawNothing
	}
	return &encounterv2pb.EncounterEvent{
		Sequence:  int64(e.Sequence()),
		Timestamp: timestamppb.New(now),
		Event: &encounterv2pb.EncounterEvent_DeathSaveRolled{
			DeathSaveRolled: &encounterv2pb.DeathSaveRolled{
				EntityId:              string(e.EntityID),
				Roll:                  int32(e.Roll),
				Successes:             int32(e.Successes),
				Failures:              int32(e.Failures),
				IsCriticalFail:        e.IsCriticalFail,
				IsCriticalSuccess:     e.IsCriticalSuccess,
				Stabilized:            e.Stabilized,
				Dead:                  e.Dead,
				RegainedConsciousness: e.RegainedConsciousness,
				HpRestored:            int32(e.HPRestored),
			},
		},
	}, nil
}

// translateEntityStabilizedEvent maps the toolkit's EntityStabilizedEvent
// (rpg-toolkit#741) to the proto EntityStabilized envelope. Fires once, at
// the terminal transition when an unconscious entity accumulates 3
// successful death saves. Mirrors translateEntityDiedEvent's shape — same
// per-viewer projection template, no killer/causer concept since
// stabilization has no attacker.
func translateEntityStabilizedEvent(e *events.EntityStabilizedEvent, viewer core.PlayerID, now time.Time) (*encounterv2pb.EncounterEvent, error) {
	slice, ok := e.PerPlayer[viewer]
	if !ok || !slice.Visible {
		return nil, ErrViewerSawNothing
	}
	return &encounterv2pb.EncounterEvent{
		Sequence:  int64(e.Sequence()),
		Timestamp: timestamppb.New(now),
		Event: &encounterv2pb.EncounterEvent_EntityStabilized{
			EntityStabilized: &encounterv2pb.EntityStabilized{
				EntityId: string(e.EntityID),
			},
		},
	}, nil
}

// translateEntityRemovedEvent maps the toolkit's EntityRemovedEvent
// (effect / state mutation) to the proto EntityRemoved envelope. The
// toolkit's audience for this event is BROADCAST: every viewer in the
// encounter is in PerPlayer with Visible:true so even out-of-LoS clients
// drop the entity from their local mirror. The translator therefore only
// gates on viewer-presence, not Visible — a missing PerPlayer entry would
// indicate a toolkit-side audience bug (the contract is broadcast), and
// returning ErrViewerSawNothing in that defensive case is the safest
// stream-loop behavior.
func translateEntityRemovedEvent(e *events.EntityRemovedEvent, viewer core.PlayerID, now time.Time) (*encounterv2pb.EncounterEvent, error) {
	if _, ok := e.PerPlayer[viewer]; !ok {
		return nil, ErrViewerSawNothing
	}
	return &encounterv2pb.EncounterEvent{
		Sequence:  int64(e.Sequence()),
		Timestamp: timestamppb.New(now),
		Event: &encounterv2pb.EncounterEvent_EntityRemoved{
			EntityRemoved: &encounterv2pb.EntityRemoved{
				EntityId: string(e.EntityID),
				Reason:   e.Reason,
			},
		},
	}, nil
}

// translateEncounterEndedEvent maps the toolkit's EncounterEndedEvent
// (terminal-state transition) to the proto EncounterEnded envelope. Like
// EntityRemoved, the toolkit's audience is BROADCAST — every viewer must
// observe the end so subsequent verbs are gated client-side. Gate only on
// viewer-presence per the broadcast-contract reasoning above.
func translateEncounterEndedEvent(e *events.EncounterEndedEvent, viewer core.PlayerID, now time.Time) (*encounterv2pb.EncounterEvent, error) {
	if _, ok := e.PerPlayer[viewer]; !ok {
		return nil, ErrViewerSawNothing
	}
	return &encounterv2pb.EncounterEvent{
		Sequence:  int64(e.Sequence()),
		Timestamp: timestamppb.New(now),
		Event: &encounterv2pb.EncounterEvent_EncounterEnded{
			EncounterEnded: &encounterv2pb.EncounterEnded{
				Reason: e.Reason,
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
		Module: refModuleDnd5e,
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
	return &encounterv2pb.Ref{Module: refModuleDnd5e, Type: "condition", Id: toolkitConditionRef}
}

// translateResourceChangedEvent maps the toolkit's ResourceChangedEvent to the
// proto ResourceChanged envelope.
//
// Per-viewer projection: viewers absent from PerPlayer or whose Visible is
// false get ErrViewerSawNothing so the stream loop skips them silently.
// The resource_ref is parsed from the canonical "module:type:id" string —
// same pattern as conditionRefFor (e.g. "dnd5e:resources:rage_charges" →
// {module:"dnd5e", type:"resources", id:"rage_charges"}).
func translateResourceChangedEvent(e *events.ResourceChangedEvent, viewer core.PlayerID, now time.Time) (*encounterv2pb.EncounterEvent, error) {
	slice, ok := e.PerPlayer[viewer]
	if !ok || !slice.Visible {
		return nil, ErrViewerSawNothing
	}
	return &encounterv2pb.EncounterEvent{
		Sequence:  int64(e.Sequence()), //nolint:gosec // sequence is monotonic; fits int64
		Timestamp: timestamppb.New(now),
		Event: &encounterv2pb.EncounterEvent_ResourceChanged{
			ResourceChanged: &encounterv2pb.ResourceChanged{
				EntityId:    string(e.EntityID),
				ResourceRef: refStringToProto(e.ResourceRef),
				NewCurrent:  int32(e.NewCurrent), //nolint:gosec // resource values fit int32
				Max:         int32(e.Max),        //nolint:gosec // resource values fit int32
			},
		},
	}, nil
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

// actionRefToProto parses a toolkit action ref string ("module:type:id") into
// the proto Ref triple for the resolved-action events. It carries the toolkit's
// two native namespaces verbatim — "combat_abilities:*" (intent) and
// "actions:*" (doing) — never flattening or remapping them (D13: a remap would
// be an Invariant-2 violation; rpg-api copies the triple through).
//
// Defensive against degenerate input (protos disc-014): a malformed ref with
// short/empty parts (e.g. "::attack") must not produce a bad wire ref. splitRef
// returns nil when the string does NOT have exactly two colons (e.g. "" or
// "attack"); for a string that DOES have two colons but empty parts (e.g.
// "::attack") it returns a 3-element slice with empty members. The guard below
// covers both: it requires len==3 AND all three parts non-empty, so any input
// that isn't a clean triple falls back to treating the whole raw string as the
// id under the dnd5e module. No real caller produces a degenerate ref (the web
// sends full triples), so this is purely a guard, mirroring conditionRefFor /
// monsterRefFor.
func actionRefToProto(toolkitActionRef string) *encounterv2pb.Ref {
	parts := splitRef(toolkitActionRef)
	if len(parts) == 3 && parts[0] != "" && parts[1] != "" && parts[2] != "" {
		return &encounterv2pb.Ref{Module: parts[0], Type: parts[1], Id: parts[2]}
	}
	return &encounterv2pb.Ref{Module: refModuleDnd5e, Type: "action", Id: toolkitActionRef}
}

// economyConsumedToProto projects the toolkit's EconomyConsumed (what an action
// spent off the turn economy) field-for-field onto the proto EconomyConsumed.
// The toolkit owns the deduction (Invariants 2/3); this is a faithful copy —
// no math, no defaulting.
func economyConsumedToProto(c events.EconomyConsumed) *encounterv2pb.EconomyConsumed {
	out := &encounterv2pb.EconomyConsumed{
		Actions:      int32(c.Actions),      //nolint:gosec // bounded turn-economy counters fit int32
		BonusActions: int32(c.BonusActions), //nolint:gosec // bounded turn-economy counters fit int32
		Reactions:    int32(c.Reactions),    //nolint:gosec // bounded turn-economy counters fit int32
		Movement:     int32(c.Movement),     //nolint:gosec // movement in feet fits int32
	}
	if len(c.GrantedConsumed) > 0 {
		out.GrantedConsumed = make(map[string]int32, len(c.GrantedConsumed))
		for k, v := range c.GrantedConsumed {
			out.GrantedConsumed[k] = int32(v) //nolint:gosec // granted-capacity counts fit int32
		}
	}
	return out
}

// turnStateSnapshotToProto projects the toolkit's engine-computed
// TurnStateSnapshot (economy + available menu) onto the proto TurnState. The
// engine authored the snapshot from the actor's own rules engine (Invariant
// 11); rpg-api copies every field through and authors nothing.
//
// The flattened toolkit menu (abilities then granted-capacity actions, already
// ordered by the engine) becomes proto AvailableActions. The economy maps onto
// proto ActionEconomy. ActiveEntityId is set to this actor (the push names whose
// menu/economy refreshed) and Round to the snapshot's TurnNumber. InitiativeOrder
// is NOT part of this per-actor delta — it's the full-encounter roster the
// snapshot path (buildTurnState) owns — so it is left empty on this push; a
// TurnStateChanged carries the actor's menu + economy refresh, not the roster.
func turnStateSnapshotToProto(snap events.TurnStateSnapshot, actorID core.EntityID) *encounterv2pb.TurnState {
	ts := &encounterv2pb.TurnState{
		ActiveEntityId: string(actorID),
		Round:          int32(snap.TurnNumber), //nolint:gosec // a turn counter fits int32
	}
	if snap.InCombat {
		ts.Economy = &encounterv2pb.ActionEconomy{
			ActionsRemaining:      int32(snap.ActionsRemaining),      //nolint:gosec // bounded counters fit int32
			BonusActionsRemaining: int32(snap.BonusActionsRemaining), //nolint:gosec // bounded counters fit int32
			ReactionsRemaining:    int32(snap.ReactionsRemaining),    //nolint:gosec // bounded counters fit int32
			MovementRemaining:     int32(snap.MovementRemaining),     //nolint:gosec // movement in feet fits int32
		}
	}
	if len(snap.Menu) > 0 {
		ts.AvailableActions = make([]*encounterv2pb.AvailableAction, 0, len(snap.Menu))
		for _, m := range snap.Menu {
			ts.AvailableActions = append(ts.AvailableActions, menuEntryToProto(m))
		}
	}
	return ts
}

// menuEntryToProto projects one toolkit MenuEntry onto the proto
// AvailableAction field-for-field: ref, display name, availability + reason,
// economy slot, target kind. The engine authored every field (including the
// effective `Available` flag and the reason string); rpg-api never recomputes
// availability or authors a reason (Invariants 2, 11).
func menuEntryToProto(m events.MenuEntry) *encounterv2pb.AvailableAction {
	return &encounterv2pb.AvailableAction{
		Ref:               actionRefToProto(m.Ref),
		DisplayName:       m.Name,
		Available:         m.Available,
		UnavailableReason: m.Reason,
		EconomySlot:       economySlotToProto(m.EconomySlot),
		TargetKind:        targetKindToProto(m.TargetKind),
	}
}

// economySlotToProto maps the toolkit's economy-slot string (the engine's
// EconomySlot enum, serialized to its string form on the spine) onto the proto
// EconomySlot enum 1:1. Unknown / empty maps to UNSPECIFIED. This is a pure
// enum translation at the wire boundary — not a rules decision.
func economySlotToProto(slot string) encounterv2pb.EconomySlot {
	switch slot {
	case "action":
		return encounterv2pb.EconomySlot_ECONOMY_SLOT_ACTION
	case "bonus_action":
		return encounterv2pb.EconomySlot_ECONOMY_SLOT_BONUS_ACTION
	case "reaction":
		return encounterv2pb.EconomySlot_ECONOMY_SLOT_REACTION
	case "movement":
		return encounterv2pb.EconomySlot_ECONOMY_SLOT_MOVEMENT
	case "free":
		return encounterv2pb.EconomySlot_ECONOMY_SLOT_FREE
	default:
		return encounterv2pb.EconomySlot_ECONOMY_SLOT_UNSPECIFIED
	}
}

// targetKindToProto maps the toolkit's target-kind string (the engine's
// TargetKind enum serialized to its string form) onto the proto TargetKind enum
// 1:1. The proto mirrors the toolkit enum exactly — SELF ≠ NONE is a real
// semantic distinction the toolkit owns (D15), so both are preserved. Unknown /
// empty maps to UNSPECIFIED. Pure enum translation, no rules.
func targetKindToProto(kind string) encounterv2pb.TargetKind {
	switch kind {
	case "self":
		return encounterv2pb.TargetKind_TARGET_KIND_SELF
	case "single_entity":
		return encounterv2pb.TargetKind_TARGET_KIND_SINGLE_ENTITY
	case "position":
		return encounterv2pb.TargetKind_TARGET_KIND_POSITION
	case "area":
		return encounterv2pb.TargetKind_TARGET_KIND_AREA
	case "none":
		return encounterv2pb.TargetKind_TARGET_KIND_NONE
	default:
		return encounterv2pb.TargetKind_TARGET_KIND_UNSPECIFIED
	}
}

// encounterModeToProto maps the toolkit's core.EncounterMode enum to the
// proto EncounterMode enum. ModeUnspecified is treated as FREE_ROAM per
// the toolkit's documented convention (see encounter/core/mode.go: "the
// zero value; treat as ModeFreeRoam unless explicitly set"). Both the
// snapshot projector (ProjectFor) and the ModeChanged event translator
// share this helper so the wire is consistent across snapshot vs.
// live-event paths.
//
// Wave 2.10: core.ModeEnded is the terminal-state value, but the
// v1alpha2 proto has no _ENDED enum variant (the wire signal is the
// dedicated EncounterEnded event, not the mode enum). Map ModeEnded to
// FREE_ROAM so a snapshot of a terminal encounter renders without turn
// structure (no active actor, no initiative) rather than UNSPECIFIED
// which clients would treat as "never set". The EncounterEndedEvent
// remains the canonical signal that combat verbs are now gated.
func encounterModeToProto(m core.EncounterMode) encounterv2pb.EncounterMode {
	switch m {
	case core.ModeTurnBased:
		return encounterv2pb.EncounterMode_ENCOUNTER_MODE_TURN_BASED
	case core.ModeFreeRoam, core.ModeUnspecified, core.ModeEnded:
		return encounterv2pb.EncounterMode_ENCOUNTER_MODE_FREE_ROAM
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
