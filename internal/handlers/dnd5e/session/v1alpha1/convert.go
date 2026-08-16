package sessionv1alpha1

import (
	"fmt"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// This file is the whole of the proto <-> SDK translation for shared types
// (design rule 1: field-for-field, no invented vocabulary). Every verb
// handler composes these converters rather than repeating field mapping.

// positionToProto mirrors spatial.Position onto the wire Position.
func positionToProto(p spatial.Position) *sessionpb.Position {
	return &sessionpb.Position{X: p.X, Y: p.Y}
}

// positionFromProto mirrors the wire Position onto spatial.Position. A nil
// proto position (unset field) becomes the zero Position -- callers that
// require a position validate its presence themselves via the SDK's own
// ErrBadPosition/ErrNilInput, not here.
func positionFromProto(p *sessionpb.Position) spatial.Position {
	if p == nil {
		return spatial.Position{}
	}
	return spatial.Position{X: p.GetX(), Y: p.GetY()}
}

func memberKindToProto(k sdk.MemberKind) sessionpb.MemberKind {
	switch k {
	case sdk.KindPlayer:
		return sessionpb.MemberKind_MEMBER_KIND_PLAYER
	case sdk.KindMonster:
		return sessionpb.MemberKind_MEMBER_KIND_MONSTER
	default:
		return sessionpb.MemberKind_MEMBER_KIND_UNSPECIFIED
	}
}

func gridKindToProto(k sdk.GridKind) sessionpb.GridKind {
	switch k {
	case sdk.GridSquare:
		return sessionpb.GridKind_GRID_KIND_SQUARE
	case sdk.GridHex:
		return sessionpb.GridKind_GRID_KIND_HEX
	default:
		return sessionpb.GridKind_GRID_KIND_UNSPECIFIED
	}
}

func clockKindToProto(k sdk.ClockKind) sessionpb.ClockKind {
	switch k {
	case sdk.ClockWorld:
		return sessionpb.ClockKind_CLOCK_KIND_WORLD
	case sdk.ClockTurn:
		return sessionpb.ClockKind_CLOCK_KIND_TURN
	default:
		return sessionpb.ClockKind_CLOCK_KIND_UNSPECIFIED
	}
}

func dissolveKindToProto(k sdk.DissolveKind) sessionpb.DissolveKind {
	switch k {
	case sdk.DissolveByDecision:
		return sessionpb.DissolveKind_DISSOLVE_KIND_BY_DECISION
	default:
		return sessionpb.DissolveKind_DISSOLVE_KIND_UNSPECIFIED
	}
}

// dissolveCauseFromProto builds the SDK's sealed DissolveCause from the wire
// enum. The SDK's DissolveKind vocabulary has exactly one member today
// (ByDecision); an unspecified or future-unknown value is refused at the
// handler with the same ErrNoCause the SDK itself would return for a missing
// cause, rather than guessed at.
func dissolveCauseFromProto(k sessionpb.DissolveKind) (sdk.DissolveCause, error) {
	switch k {
	case sessionpb.DissolveKind_DISSOLVE_KIND_BY_DECISION:
		return sdk.ByDecision(), nil
	default:
		return nil, fmt.Errorf("dissolve: unrecognized cause %v: %w", k, sdk.ErrNoCause)
	}
}

func memberToProto(m sdk.Member) *sessionpb.Member {
	return &sessionpb.Member{
		Id:   m.ID,
		Kind: memberKindToProto(m.Kind),
		Room: m.Room,
	}
}

func memberOutcomeToProto(m sdk.MemberOutcome) *sessionpb.MemberOutcome {
	return &sessionpb.MemberOutcome{
		Id:       m.ID,
		Room:     m.Room,
		Position: positionToProto(m.Position),
	}
}

func memberOutcomesToProto(ms []sdk.MemberOutcome) []*sessionpb.MemberOutcome {
	out := make([]*sessionpb.MemberOutcome, len(ms))
	for i, m := range ms {
		out[i] = memberOutcomeToProto(m)
	}
	return out
}

func characterStateToProto(c *sdk.CharacterState) *sessionpb.CharacterState {
	if c == nil {
		return nil
	}
	return &sessionpb.CharacterState{
		Id:               c.ID,
		Name:             c.Name,
		Level:            int32(c.Level),
		Speed:            int32(c.Speed),
		HitPoints:        int32(c.HitPoints),
		MaxHitPoints:     int32(c.MaxHitPoints),
		ArmorClass:       int32(c.ArmorClass),
		ProficiencyBonus: int32(c.ProficiencyBonus),
	}
}

func reportToProto(r sdk.Report) *sessionpb.Report {
	return &sessionpb.Report{Subject: r.Subject, Payload: r.Payload}
}

func discoveryToProto(d sdk.Discovery) *sessionpb.Discovery {
	firstContact := make([]*sessionpb.Report, len(d.FirstContact))
	for i, r := range d.FirstContact {
		firstContact[i] = reportToProto(r)
	}
	return &sessionpb.Discovery{
		FirstContact: firstContact,
		Refreshed:    d.Refreshed,
		Faded:        d.Faded,
	}
}

// discoveriesToProto mirrors a per-observer Discovery map. A nil input (no
// observer saw anything new) becomes a nil map, not an empty one -- the SDK's
// own JoinOutput/MoveOutput/... doc treats an absent key as "saw nothing new",
// and proto's map encoding already round-trips nil-vs-empty identically, so
// there is nothing to normalise here.
func discoveriesToProto(d map[string]sdk.Discovery) map[string]*sessionpb.Discovery {
	if d == nil {
		return nil
	}
	out := make(map[string]*sessionpb.Discovery, len(d))
	for k, v := range d {
		out[k] = discoveryToProto(v)
	}
	return out
}

func sightingToProto(s sdk.Sighting) *sessionpb.Sighting {
	return &sessionpb.Sighting{
		Subject:    s.Subject,
		Payload:    s.Payload,
		Channel:    s.Channel,
		At:         s.At,
		CurrentVia: s.CurrentVia,
		Status:     s.Status,
	}
}

func sightingsToProto(ss []sdk.Sighting) []*sessionpb.Sighting {
	out := make([]*sessionpb.Sighting, len(ss))
	for i, s := range ss {
		out[i] = sightingToProto(s)
	}
	return out
}

func stepToProto(s sdk.Step) *sessionpb.Step {
	return &sessionpb.Step{Position: positionToProto(s.Position), Seq: s.Seq}
}

func stepsToProto(ss []sdk.Step) []*sessionpb.Step {
	out := make([]*sessionpb.Step, len(ss))
	for i, s := range ss {
		out[i] = stepToProto(s)
	}
	return out
}

func outcomeToProto(o *sdk.Outcome) *sessionpb.Outcome {
	if o == nil {
		return nil
	}
	return &sessionpb.Outcome{
		Ending:  o.Ending,
		At:      o.At,
		Members: memberOutcomesToProto(o.Members),
	}
}

func formedToProto(f *sdk.Formed) *sessionpb.Formed {
	if f == nil {
		return nil
	}
	return &sessionpb.Formed{
		Order:     f.Order,
		Surprised: f.Surprised,
		Seq:       f.Seq,
	}
}

func saveReportToProto(r sdk.SaveReport) *sessionpb.SaveReport {
	return &sessionpb.SaveReport{Written: r.Written, Failed: r.Failed}
}

func deliveryReportToProto(r sdk.DeliveryReport) *sessionpb.DeliveryReport {
	return &sessionpb.DeliveryReport{Events: int32(r.Events), Failed: r.Failed}
}

func storyEntryToProto(e sdk.StoryEntry) *sessionpb.StoryEntry {
	return &sessionpb.StoryEntry{
		Seq:         e.Seq,
		At:          e.At,
		Correlation: e.Correlation,
		Tags:        e.Tags,
		Payload:     e.Payload,
	}
}

func storyEntriesToProto(es []sdk.StoryEntry) []*sessionpb.StoryEntry {
	out := make([]*sessionpb.StoryEntry, len(es))
	for i, e := range es {
		out[i] = storyEntryToProto(e)
	}
	return out
}

func atlasBoundaryToProto(b sdk.AtlasBoundary) *sessionpb.AtlasBoundary {
	return &sessionpb.AtlasBoundary{
		From:              positionToProto(b.From),
		To:                positionToProto(b.To),
		BlocksMovement:    b.BlocksMovement,
		BlocksLineOfSight: b.BlocksLineOfSight,
	}
}

func atlasBoundariesToProto(bs []sdk.AtlasBoundary) []*sessionpb.AtlasBoundary {
	out := make([]*sessionpb.AtlasBoundary, len(bs))
	for i, b := range bs {
		out[i] = atlasBoundaryToProto(b)
	}
	return out
}

func atlasRoomToProto(r sdk.AtlasRoom) *sessionpb.AtlasRoom {
	cells := make([]*sessionpb.Position, len(r.Cells))
	for i, c := range r.Cells {
		cells[i] = positionToProto(c)
	}
	occluders := make([]*sessionpb.Position, len(r.Occluders))
	for i, c := range r.Occluders {
		occluders[i] = positionToProto(c)
	}
	return &sessionpb.AtlasRoom{
		Id:         r.ID,
		Grid:       gridKindToProto(r.Grid),
		Origin:     positionToProto(r.Origin),
		Width:      int32(r.Width),
		Height:     int32(r.Height),
		Cells:      cells,
		Occluders:  occluders,
		Boundaries: atlasBoundariesToProto(r.Boundaries),
	}
}

func atlasDoorwayToProto(d sdk.AtlasDoorway) *sessionpb.AtlasDoorway {
	return &sessionpb.AtlasDoorway{
		Connection: d.Connection,
		From:       d.From,
		FromCell:   positionToProto(d.FromCell),
		To:         d.To,
		ToCell:     positionToProto(d.ToCell),
	}
}

// eventKindToProto mirrors sdk.EventKind onto the wire enum. A kind this
// build does not recognize -- either the SDK's own EventUnknown (a beat the
// TOOLKIT did not recognize, delivered on purpose) or, in principle, some
// future SDK value this handler package has not been updated for -- maps to
// EVENT_KIND_UNKNOWN rather than EVENT_KIND_UNSPECIFIED. The two mean
// different things: UNSPECIFIED is a producer defect (this code failed to
// set a kind), UNKNOWN is "delivered, but not interpretable by this
// version" -- exactly the SDK's own delivered-not-dropped rule, and exactly
// what an unrecognized kind IS, never a defect.
func eventKindToProto(k sdk.EventKind) sessionpb.EventKind {
	switch k {
	case sdk.EventMoved:
		return sessionpb.EventKind_EVENT_KIND_MOVED
	case sdk.EventTraversed:
		return sessionpb.EventKind_EVENT_KIND_TRAVERSED
	case sdk.EventJoined:
		return sessionpb.EventKind_EVENT_KIND_JOINED
	case sdk.EventExited:
		return sessionpb.EventKind_EVENT_KIND_EXITED
	case sdk.EventEnded:
		return sessionpb.EventKind_EVENT_KIND_ENDED
	case sdk.EventSceneOpened:
		return sessionpb.EventKind_EVENT_KIND_SCENE_OPENED
	case sdk.EventTick:
		return sessionpb.EventKind_EVENT_KIND_TICK
	case sdk.EventTurnEnded:
		return sessionpb.EventKind_EVENT_KIND_TURN_ENDED
	case sdk.EventFightStarted:
		return sessionpb.EventKind_EVENT_KIND_FIGHT_STARTED
	case sdk.EventFightEnded:
		return sessionpb.EventKind_EVENT_KIND_FIGHT_ENDED
	case sdk.EventStruck:
		return sessionpb.EventKind_EVENT_KIND_STRUCK
	case sdk.EventMissed:
		return sessionpb.EventKind_EVENT_KIND_MISSED
	default:
		return sessionpb.EventKind_EVENT_KIND_UNKNOWN
	}
}

// eventToProto mirrors session.Event field-for-field (design rule 3):
// session, seq, at, correlation, recipient, kind, payload. The payload is
// passthrough -- rpg-api round-trips these bytes and never builds or
// inspects them (design rule 4's corollary).
func eventToProto(e sdk.Event) *sessionpb.Event {
	return &sessionpb.Event{
		Session:     e.Session,
		Seq:         e.Seq,
		At:          e.At,
		Correlation: e.Correlation,
		Recipient:   e.Recipient,
		Kind:        eventKindToProto(e.Kind),
		Payload:     e.Payload,
	}
}

func atlasToProto(a *sdk.Atlas) *sessionpb.GetAtlasResponse {
	if a == nil {
		return &sessionpb.GetAtlasResponse{}
	}
	rooms := make([]*sessionpb.AtlasRoom, len(a.Rooms))
	for i, r := range a.Rooms {
		rooms[i] = atlasRoomToProto(r)
	}
	doorways := make([]*sessionpb.AtlasDoorway, len(a.Doorways))
	for i, d := range a.Doorways {
		doorways[i] = atlasDoorwayToProto(d)
	}
	return &sessionpb.GetAtlasResponse{Rooms: rooms, Doorways: doorways}
}
