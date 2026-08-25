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

// gridKindToProto mirrors session.GridKind. Hex is the only kind a map can
// report since rpg-project#256 (square fields were deleted with rooms); the
// wire enum keeps SQUARE for its own history, and nothing here can produce it.
func gridKindToProto(k sdk.GridKind) sessionpb.GridKind {
	switch k {
	case sdk.GridHex:
		return sessionpb.GridKind_GRID_KIND_HEX
	default:
		return sessionpb.GridKind_GRID_KIND_UNSPECIFIED
	}
}

// hexLayoutToProto mirrors session.HexLayout (session/v0.20.0,
// rpg-toolkit#1140). The empty value is a square map, which carries no layout
// by law; it reaches the wire as UNSPECIFIED rather than a guess, because a
// square map that said pointy-top would be a client believing something that
// cannot be true about its grid.
func hexLayoutToProto(l sdk.HexLayout) sessionpb.HexLayout {
	switch l {
	case sdk.HexLayoutPointyTop:
		return sessionpb.HexLayout_HEX_LAYOUT_POINTY_TOP
	case sdk.HexLayoutFlatTop:
		return sessionpb.HexLayout_HEX_LAYOUT_FLAT_TOP
	default:
		return sessionpb.HexLayout_HEX_LAYOUT_UNSPECIFIED
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
	case sdk.DissolveByDefeat:
		// Arrived at session/v0.15.0. Missing this case would not fail
		// anything -- it would report UNSPECIFIED, so a fight that ended
		// because the last skeleton dropped would reach a client as a fight
		// that ended for no stated reason, which is a producer defect by this
		// enum's own definition.
		return sessionpb.DissolveKind_DISSOLVE_KIND_BY_DEFEAT
	default:
		return sessionpb.DissolveKind_DISSOLVE_KIND_UNSPECIFIED
	}
}

// dissolveCauseFromProto builds the SDK's sealed DissolveCause from the wire
// enum. An unspecified or future-unknown value is refused with the same
// ErrNoCause the SDK returns for a missing cause, rather than guessed at.
//
// BY_DEFEAT is ACCEPTED here rather than refused, and the choice is deliberate.
// A caller cannot honestly declare it -- defeat is something the world notices
// at a sight refresh, and this verb IS the decision -- so the temptation is to
// reject it as a lie. But the SDK made it a NO-OP on purpose: handing in the
// wrong cause does not fail the call and does not change the outcome, because
// the answer reports what actually happened. Refusing here would give rpg-api a
// stricter contract than the package it transcribes, over a distinction the SDK
// deliberately declined to enforce -- design rule 1's "no vocabulary of our
// own" running in the subtractive direction.
func dissolveCauseFromProto(k sessionpb.DissolveKind) (sdk.DissolveCause, error) {
	switch k {
	case sessionpb.DissolveKind_DISSOLVE_KIND_BY_DECISION:
		return sdk.ByDecision(), nil
	case sessionpb.DissolveKind_DISSOLVE_KIND_BY_DEFEAT:
		return sdk.ByDefeat(), nil
	default:
		return nil, fmt.Errorf("dissolve: unrecognized cause %v: %w", k, sdk.ErrNoCause)
	}
}

func memberToProto(m sdk.Member) *sessionpb.Member {
	return &sessionpb.Member{
		Id:       m.ID,
		Kind:     memberKindToProto(m.Kind),
		Position: positionToProto(m.Position),
	}
}

func memberOutcomeToProto(m sdk.MemberOutcome) *sessionpb.MemberOutcome {
	return &sessionpb.MemberOutcome{
		Id:       m.ID,
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

// seenToProto mirrors the sight channel's typed Seen sub-struct (ADR-0041,
// rpg-toolkit#1157, session v0.21.2). A nil Seen -- channel provenance or
// payload decoding didn't hold sight, or (on Report) the payload simply
// didn't decode as one -- stays unset on the wire rather than a zero-valued
// Seen, matching discoveriesToProto's own "absent means nothing to report"
// convention on this seam.
//
// Standing rides along (rpg-toolkit#1137, rpg-project#249): sight-channel
// knowledge, not roster truth, which is why it lives here and not on a
// roster read this seam deliberately lacks -- see Participant.
func seenToProto(s *sdk.Seen) *sessionpb.Seen {
	if s == nil {
		return nil
	}
	return &sessionpb.Seen{Position: positionToProto(s.Position), Standing: standingToProto(s.Standing)}
}

// standingToProto mirrors session.Standing onto the wire enum. Two values,
// not a bool: DOWNED is at zero hit points and out of the fight, distinct
// from PRONE (a posture condition this seam never gates on, Kirk's ruling
// rpg-toolkit#1084) -- see session.Standing's own doc. An unrecognized
// string reaches UNSPECIFIED, a producer defect.
func standingToProto(s sdk.Standing) sessionpb.Standing {
	switch s {
	case sdk.StandingUp:
		return sessionpb.Standing_STANDING_UP
	case sdk.StandingDowned:
		return sessionpb.Standing_STANDING_DOWNED
	default:
		return sessionpb.Standing_STANDING_UNSPECIFIED
	}
}

func reportToProto(r sdk.Report) *sessionpb.Report {
	return &sessionpb.Report{Subject: r.Subject, Payload: r.Payload, Seen: seenToProto(r.Seen)}
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

// sightingToProto mirrors session.Sighting field-for-field, including Name
// (rpg-toolkit#1137, rpg-project#249): anything an observer can sight, they
// can name, so a client labels what it draws without a second lookup
// (rpg-dnd5e-web#564) — and Kind (rpg-toolkit#1230), for the same reason:
// a client routes a player subject to a player model instead of guessing a
// monster ref from the subject id (rpg-dnd5e-web#792).
func sightingToProto(s sdk.Sighting) *sessionpb.Sighting {
	return &sessionpb.Sighting{
		Subject:    s.Subject,
		Name:       s.Name,
		Kind:       memberKindToProto(s.Kind),
		Payload:    s.Payload,
		Channel:    s.Channel,
		At:         s.At,
		CurrentVia: s.CurrentVia,
		Status:     s.Status,
		Seen:       seenToProto(s.Seen),
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

func atlasDoorwayToProto(d sdk.AtlasDoorway) *sessionpb.AtlasDoorway {
	return &sessionpb.AtlasDoorway{
		Connection: d.Door,
		From:       positionToProto(d.From),
		To:         positionToProto(d.To),
	}
}

func atlasDoorwaysToProto(ds []sdk.AtlasDoorway) []*sessionpb.AtlasDoorway {
	out := make([]*sessionpb.AtlasDoorway, len(ds))
	for i, d := range ds {
		out[i] = atlasDoorwayToProto(d)
	}
	return out
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
	// No EventTraversed case: session/v0.18.0 retired the kind because the
	// composition stopped emitting the beat -- a doorway crossing is written
	// like any other step (rpg-toolkit#1048, #1059). A client that draws a
	// doorway differently derives it from GetAtlasResponse.doorways, which
	// lists every crossable pair.
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
	case sdk.EventDowned:
		return sessionpb.EventKind_EVENT_KIND_DOWNED
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
// inspects them (design rule 4's corollary). Body is projected separately
// by setEventBody, once the spine is built, so the passthrough law above
// stays true of payload even as body stops being the only carrier.
//
// e.Tags (session/v0.23.0, rpg-toolkit#1213) has NO wire counterpart yet --
// dnd5e.api.session.v1alpha1.Event carries no tags field -- so it is
// deliberately dropped here rather than smuggled onto payload or forced into
// a field that does not exist. This is the same converter GetStory now runs
// its catch-up through (get_story.go, rpg-api-protos#239), so both paths
// drop it identically; adding a wire `tags` field is a proto change, not
// something this function can paper over on its own.
func eventToProto(e sdk.Event) *sessionpb.Event {
	evt := &sessionpb.Event{
		Session:     e.Session,
		Seq:         e.Seq,
		At:          e.At,
		Correlation: e.Correlation,
		Recipient:   e.Recipient,
		Kind:        eventKindToProto(e.Kind),
		Payload:     e.Payload,
	}
	setEventBody(evt, e.Body)
	return evt
}

// eventsToProto mirrors a []sdk.Event slice -- GetStory's own use of the
// same eventToProto StreamEvents sends through one at a time, so catch-up
// and live delivery share one projection all the way to the wire.
func eventsToProto(es []sdk.Event) []*sessionpb.Event {
	out := make([]*sessionpb.Event, len(es))
	for i, e := range es {
		out[i] = eventToProto(e)
	}
	return out
}

// setEventBody projects the SDK's typed session.EventBody onto the proto
// oneof body -- ONE ARM PER BODY (rpg-toolkit#941, rpg-project#249),
// never a body built from payload bytes: every arm reads only the SDK's own
// typed fields, the same decode the SDK already did once, not a second
// encoding of it.
//
// evt.Body stays nil for a kind with no typed body member (ENDED,
// SCENE_OPENED, TICK, UNKNOWN) and for a beat this build's decoder did not
// recognize -- session.Event.Body is nil in exactly those cases, so the
// default arm below is correct by construction, not a fallback that papers
// over an unhandled case. payload stays the passthrough carrier for every
// one of those kinds.
//
// JoinedBody/ExitedBody (session/v0.24.0, rpg-toolkit#1217, rpg-project#260
// slice 4) carry the arriving/departing member -- the same field the wire
// Joined/Exited messages added at protos v0.1.136 (oneof tags 17/18), so
// GetStory gets both free through this same converter.
func setEventBody(evt *sessionpb.Event, body sdk.EventBody) {
	switch b := body.(type) {
	case sdk.TurnEndedBody:
		evt.Body = &sessionpb.Event_TurnEnded{TurnEnded: &sessionpb.TurnEnded{Member: b.Member, Next: b.Next}}
	case sdk.DownedBody:
		evt.Body = &sessionpb.Event_Downed{Downed: &sessionpb.Downed{Member: b.Member}}
	case sdk.StruckBody:
		evt.Body = &sessionpb.Event_Struck{Struck: &sessionpb.Struck{
			Attacker:            b.Attacker,
			Target:              b.Target,
			Roll:                int32(b.Roll),
			Total:               int32(b.Total),
			Against:             int32(b.Against),
			Damage:              int32(b.Damage),
			Attack:              attackRefToProto(b.Attack),
			Critical:            b.Critical,
			DamageComponents:    damageComponentsToProto(b.DamageComponents),
			AdvantageSources:    attackModifierSourcesToProto(b.AdvantageSources),
			DisadvantageSources: attackModifierSourcesToProto(b.DisadvantageSources),
		}}
	case sdk.MissedBody:
		evt.Body = &sessionpb.Event_Missed{Missed: &sessionpb.Missed{
			Attacker: b.Attacker,
			Target:   b.Target,
			Roll:     int32(b.Roll),
			Total:    int32(b.Total),
			Against:  int32(b.Against),
			Attack:   attackRefToProto(b.Attack),
		}}
	case sdk.FightStartedBody:
		evt.Body = &sessionpb.Event_FightStarted{FightStarted: &sessionpb.FightStarted{Members: b.Members}}
	case sdk.FightEndedBody:
		evt.Body = &sessionpb.Event_FightEnded{FightEnded: &sessionpb.FightEnded{Cause: dissolveKindToProto(b.Cause)}}
	case sdk.MovedBody:
		evt.Body = &sessionpb.Event_Moved{Moved: &sessionpb.Moved{Member: b.Member, To: positionToProto(b.To)}}
	case sdk.JoinedBody:
		evt.Body = &sessionpb.Event_Joined{Joined: &sessionpb.Joined{Member: b.Member}}
	case sdk.ExitedBody:
		evt.Body = &sessionpb.Event_Exited{Exited: &sessionpb.Exited{Member: b.Member}}
	default:
		// nil (no typed body for this kind) or a body type this build does
		// not recognize: leave evt.Body nil. payload stays the passthrough
		// carrier.
	}
}

// AtlasToProto mirrors the ONE-MAP Atlas (design §0, live as of session
// v0.12.0): a flat set of cells, the things standing on them, the walls
// between them, and every doorway -- not a list of rooms with anchors and
// spans a client would have to reassemble.
//
// Props replaced a bare `occluders` coordinate list at session/v0.18.0
// (rpg-toolkit#1130). Both blocking answers are carried verbatim rather than
// collapsed back into "does it block sight": the old field could not say a
// pillar from a statue, and it gave ONE answer to TWO independent questions --
// a coffin is walked around but seen over, a pile of bones is neither. Copying
// the bools straight across is the whole job here; deciding anything about them
// would be this layer inventing world state.
//
// Exported because it has two callers that MUST agree: GetAtlas, and the
// AuthoringService's PutDungeon, whose answer is the same message so the
// builder has no second geometry to keep in step with the game
// (rpg-project#256, design §3a).
//
// Regions (GetAtlasResponse.regions) are copied cell for cell: they are
// already absolute axial in the same frame as Cells, so nothing here converts
// anything — the one place cells become axial is the toolkit's.
func AtlasToProto(a *sdk.Atlas) *sessionpb.GetAtlasResponse {
	if a == nil {
		return &sessionpb.GetAtlasResponse{}
	}
	cells := make([]*sessionpb.Position, len(a.Cells))
	for i, c := range a.Cells {
		cells[i] = positionToProto(c)
	}
	props := make([]*sessionpb.AtlasProp, len(a.Props))
	for i, prop := range a.Props {
		props[i] = &sessionpb.AtlasProp{
			Ref:               prop.Ref,
			At:                positionToProto(prop.At),
			BlocksMovement:    prop.BlocksMovement,
			BlocksLineOfSight: prop.BlocksLineOfSight,
			Facing:            prop.Facing,
			OffsetX:           float32(prop.Offset[0]),
			OffsetY:           float32(prop.Offset[1]),
		}
	}
	return &sessionpb.GetAtlasResponse{
		Grid:       gridKindToProto(a.Grid),
		Layout:     hexLayoutToProto(a.Layout),
		Cells:      cells,
		Props:      props,
		Boundaries: atlasBoundariesToProto(a.Boundaries),
		Doorways:   atlasDoorwaysToProto(a.Doorways),
		Regions:    atlasRegionsToProto(a.Regions),
	}
}

// atlasRegionToProto mirrors session.AtlasRegion: a named set of absolute
// cells plus the per-area facts it carries. Archetype and lighting are copied
// verbatim — an archetype never decides mechanics, and intensity is a world
// fact, not a render hint (design §3b).
func atlasRegionToProto(r sdk.AtlasRegion) *sessionpb.AtlasRegion {
	cells := make([]*sessionpb.Position, len(r.Cells))
	for i, c := range r.Cells {
		cells[i] = positionToProto(c)
	}
	return &sessionpb.AtlasRegion{
		Id:        r.ID,
		Name:      r.Name,
		Cells:     cells,
		Archetype: r.Archetype,
		Lighting:  &sessionpb.Lighting{Intensity: r.Lighting.Intensity},
	}
}

func atlasRegionsToProto(rs []sdk.AtlasRegion) []*sessionpb.AtlasRegion {
	out := make([]*sessionpb.AtlasRegion, len(rs))
	for i, r := range rs {
		out[i] = atlasRegionToProto(r)
	}
	return out
}

// whereToProto mirrors session.WhereOutput onto the wire GetWhereResponse.
func whereToProto(w *sdk.WhereOutput) *sessionpb.GetWhereResponse {
	if w == nil {
		return &sessionpb.GetWhereResponse{}
	}
	return &sessionpb.GetWhereResponse{Position: positionToProto(w.Position)}
}

// verbToProto mirrors session.Verb onto the wire enum. VerbMove joined
// VerbAttack at protos v0.1.131 (rpg-toolkit#1169) -- Afford already emits
// VerbMove declarations on the turn clock unconditionally (session's own
// affordMove), so leaving it unmapped here would silently mislabel every one
// of them VERB_UNSPECIFIED rather than a producer defect this handler forgot
// to update for; a verb this build genuinely does not recognize (in
// principle only a future SDK value) still maps to VERB_UNSPECIFIED, since
// the SDK's own Verb is a closed enum with no unknown-but-delivered case the
// way EventKind carries.
func verbToProto(v sdk.Verb) sessionpb.Verb {
	switch v {
	case sdk.VerbAttack:
		return sessionpb.Verb_VERB_ATTACK
	case sdk.VerbMove:
		return sessionpb.Verb_VERB_MOVE
	default:
		return sessionpb.Verb_VERB_UNSPECIFIED
	}
}

// slotToProto mirrors session.Slot onto the wire enum.
//
// SlotNone ("") maps to the EXPLICIT SLOT_NONE, never left to fall through to
// SLOT_UNSPECIFIED -- the one law this converter exists to keep. A
// declaration that lights no economy shape (Extra Attack's second swing,
// spending a banked attack rather than an action/bonus/reaction) is a FACT
// about the price, the same way Declaration.Affordable=false is an answer and
// not an absence (types.go). Collapsing it into UNSPECIFIED would tell a
// client this layer forgot to set a slot, when the SDK answered on purpose.
// Only a slot string this build does not recognize reaches UNSPECIFIED, a
// producer defect.
func slotToProto(s sdk.Slot) sessionpb.Slot {
	switch s {
	case sdk.SlotNone:
		return sessionpb.Slot_SLOT_NONE
	case sdk.SlotAction:
		return sessionpb.Slot_SLOT_ACTION
	case sdk.SlotBonus:
		return sessionpb.Slot_SLOT_BONUS
	case sdk.SlotReaction:
		return sessionpb.Slot_SLOT_REACTION
	default:
		return sessionpb.Slot_SLOT_UNSPECIFIED
	}
}

// declarationToProto mirrors session.Declaration field-for-field (design rule
// 3): verb, slot, affordable, shortfall -- no omitempty on Affordable or
// Shortfall on the wire side either, the same false-vs-absent law the SDK's
// own doc keeps for both fields.
//
// Target and Why are both POINTER-TO-POINTER copies (rpg-toolkit#1010,
// rpg-project#249): the SDK already keeps absent distinct from a present
// zero value the same way the wire does, so there is nothing to normalise --
// nil stays nil, a set value crosses as itself.
//
// Remaining is NOT wired here: it lands with the separate Move-on-clock wave
// (rpg-toolkit#1169, branch feat/session-move-clock), still WIP as of this
// PR -- out of scope here on purpose, to keep one wave on one branch.
func declarationToProto(d sdk.Declaration) *sessionpb.Declaration {
	return &sessionpb.Declaration{
		Verb:       verbToProto(d.Verb),
		Slot:       slotToProto(d.Slot),
		Affordable: d.Affordable,
		Shortfall:  d.Shortfall,
		Target:     d.Target,
		Why:        shortfallToProto(d.Why),
	}
}

// shortfallReasonToProto mirrors session.ShortfallReason onto the wire enum.
// An unrecognized string reaches UNSPECIFIED, a producer defect.
func shortfallReasonToProto(r sdk.ShortfallReason) sessionpb.ShortfallReason {
	switch r {
	case sdk.ShortfallNoBudget:
		return sessionpb.ShortfallReason_SHORTFALL_REASON_NO_BUDGET
	case sdk.ShortfallNotYourTurn:
		return sessionpb.ShortfallReason_SHORTFALL_REASON_NOT_YOUR_TURN
	case sdk.ShortfallNoTargetInReach:
		return sessionpb.ShortfallReason_SHORTFALL_REASON_NO_TARGET_IN_REACH
	case sdk.ShortfallDowned:
		return sessionpb.ShortfallReason_SHORTFALL_REASON_DOWNED
	case sdk.ShortfallUnreadable:
		return sessionpb.ShortfallReason_SHORTFALL_REASON_UNREADABLE
	default:
		return sessionpb.ShortfallReason_SHORTFALL_REASON_UNSPECIFIED
	}
}

// currencyToProto mirrors session.Currency onto the wire enum. NOT Slot,
// although three values coincide -- Slot says which shape a declaration
// lights, Currency says which ledger a refusal drained, and movement is a
// ledger with no shape (see types.proto's Currency doc). An unrecognized
// string reaches UNSPECIFIED, a producer defect.
func currencyToProto(c sdk.Currency) sessionpb.Currency {
	switch c {
	case sdk.CurrencyAction:
		return sessionpb.Currency_CURRENCY_ACTION
	case sdk.CurrencyBonus:
		return sessionpb.Currency_CURRENCY_BONUS
	case sdk.CurrencyReaction:
		return sessionpb.Currency_CURRENCY_REACTION
	case sdk.CurrencyMovement:
		return sessionpb.Currency_CURRENCY_MOVEMENT
	default:
		return sessionpb.Currency_CURRENCY_UNSPECIFIED
	}
}

// shortfallToProto mirrors session.Shortfall. Present exactly when the SDK
// set it -- nil in, nil out -- matching Declaration.why's presence law: PRESENT
// EXACTLY WHEN affordable == false (rpg-toolkit#1010).
func shortfallToProto(s *sdk.Shortfall) *sessionpb.Shortfall {
	if s == nil {
		return nil
	}
	return &sessionpb.Shortfall{
		Reason:   shortfallReasonToProto(s.Reason),
		Currency: currencyToProto(s.Currency),
		Needed:   int32(s.Needed),
		Left:     int32(s.Left),
		Text:     s.Text,
	}
}

// damageTypeToProto mirrors session.DamageType onto the wire enum -- a
// CLOSED Go type to a closed enum, never a string round-trip (rpg-project#249
// §6, Kirk): an unrecognized value reaches UNSPECIFIED, the same producer-
// defect treatment every other closed enum in this file gets, rather than
// inventing a thirteen-plus-one vocabulary of our own.
func damageTypeToProto(d sdk.DamageType) sessionpb.DamageType {
	switch d {
	case sdk.DamageAcid:
		return sessionpb.DamageType_DAMAGE_TYPE_ACID
	case sdk.DamageBludgeoning:
		return sessionpb.DamageType_DAMAGE_TYPE_BLUDGEONING
	case sdk.DamageCold:
		return sessionpb.DamageType_DAMAGE_TYPE_COLD
	case sdk.DamageFire:
		return sessionpb.DamageType_DAMAGE_TYPE_FIRE
	case sdk.DamageForce:
		return sessionpb.DamageType_DAMAGE_TYPE_FORCE
	case sdk.DamageLightning:
		return sessionpb.DamageType_DAMAGE_TYPE_LIGHTNING
	case sdk.DamageNecrotic:
		return sessionpb.DamageType_DAMAGE_TYPE_NECROTIC
	case sdk.DamagePiercing:
		return sessionpb.DamageType_DAMAGE_TYPE_PIERCING
	case sdk.DamagePoison:
		return sessionpb.DamageType_DAMAGE_TYPE_POISON
	case sdk.DamagePsychic:
		return sessionpb.DamageType_DAMAGE_TYPE_PSYCHIC
	case sdk.DamageRadiant:
		return sessionpb.DamageType_DAMAGE_TYPE_RADIANT
	case sdk.DamageSlashing:
		return sessionpb.DamageType_DAMAGE_TYPE_SLASHING
	case sdk.DamageThunder:
		return sessionpb.DamageType_DAMAGE_TYPE_THUNDER
	default:
		return sessionpb.DamageType_DAMAGE_TYPE_UNSPECIFIED
	}
}

func damageComponentsToProto(in []sdk.DamageComponent) []*sessionpb.DamageComponent {
	out := make([]*sessionpb.DamageComponent, len(in))
	for i, component := range in {
		rolls := make([]int32, len(component.FinalRolls))
		for j, roll := range component.FinalRolls {
			rolls[j] = int32(roll)
		}
		var multiplier *float64
		if component.Multiplier != nil {
			value := *component.Multiplier
			multiplier = &value
		}
		out[i] = &sessionpb.DamageComponent{
			Source: component.Source, SourceRef: component.SourceRef, Dice: component.Dice,
			FinalRolls: rolls, FlatBonus: int32(component.FlatBonus),
			DamageType: damageTypeToProto(component.DamageType), Multiplier: multiplier,
		}
	}
	return out
}

func attackModifierSourcesToProto(in []sdk.AttackModifierSource) []*sessionpb.AttackModifierSource {
	out := make([]*sessionpb.AttackModifierSource, len(in))
	for i, source := range in {
		out[i] = &sessionpb.AttackModifierSource{
			SourceRef: source.SourceRef,
			SourceId:  source.SourceID,
		}
	}
	return out
}

// attackRefToProto mirrors session.AttackRef field-for-field (rpg-toolkit#866):
// what was swung, always populated -- AttackOutput.Attack and the Struck/
// Missed event bodies carry it as a value, never a pointer, so this always
// returns a non-nil message.
func attackRefToProto(a sdk.AttackRef) *sessionpb.AttackRef {
	return &sessionpb.AttackRef{
		Ref:        a.Ref,
		Name:       a.Name,
		DamageType: damageTypeToProto(a.DamageType),
	}
}

// participantToProto mirrors session.Participant field-for-field
// (rpg-toolkit#1137, rpg-project#249): everything a bare id in `order`
// cannot carry -- name, kind, standing, and whether this is the active
// member's turn.
func participantToProto(p sdk.Participant) *sessionpb.Participant {
	return &sessionpb.Participant{
		Member:   p.Member,
		Name:     p.Name,
		Kind:     memberKindToProto(p.Kind),
		Standing: standingToProto(p.Standing),
		Active:   p.Active,
	}
}

// participantsToProto mirrors a Participants list. A nil or empty input
// becomes a non-nil, zero-length slice, the same make-then-loop convention
// declarationsToProto keeps -- TurnResponse.participants is empty (not null)
// on the world clock, same law as Declarations.
func participantsToProto(ps []sdk.Participant) []*sessionpb.Participant {
	out := make([]*sessionpb.Participant, len(ps))
	for i, p := range ps {
		out[i] = participantToProto(p)
	}
	return out
}

// declarationsToProto mirrors a Declarations list. A nil or empty input
// becomes a non-nil, zero-length slice, matching the SDK's own "empty IS the
// answer" law for the world clock (AffordOutput.Declarations never marshals
// as null) -- the same make-then-loop shape every other list converter in
// this file keeps, so an empty result already comes out non-nil for free.
func declarationsToProto(ds []sdk.Declaration) []*sessionpb.Declaration {
	out := make([]*sessionpb.Declaration, len(ds))
	for i, d := range ds {
		out[i] = declarationToProto(d)
	}
	return out
}
