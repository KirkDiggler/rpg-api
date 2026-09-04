package sessionv1alpha1

import (
	"fmt"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/npcs"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
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
	case sdk.KindWorld:
		return sessionpb.MemberKind_MEMBER_KIND_WORLD
	default:
		return sessionpb.MemberKind_MEMBER_KIND_UNSPECIFIED
	}
}

// vendorStockModeToProto mirrors npcs.StockMode onto the wire enum. An
// unrecognized value reaches UNSPECIFIED, a producer defect.
func vendorStockModeToProto(m npcs.StockMode) sessionpb.VendorStockMode {
	switch m {
	case npcs.StockModeLimited:
		return sessionpb.VendorStockMode_VENDOR_STOCK_MODE_LIMITED
	case npcs.StockModeUnlimited:
		return sessionpb.VendorStockMode_VENDOR_STOCK_MODE_UNLIMITED
	default:
		return sessionpb.VendorStockMode_VENDOR_STOCK_MODE_UNSPECIFIED
	}
}

// vendorStockEntryToProto mirrors one resolved vendor stock row. Quantity is
// meaningful only when Mode is LIMITED (the wire message's own doc); an
// unlimited entry's Quantity is the toolkit's own zero value and stays
// unset here rather than carried as a meaningless zero.
func vendorStockEntryToProto(e npcs.StockEntryView) *sessionpb.VendorStockEntry {
	out := &sessionpb.VendorStockEntry{
		EquipmentType: string(e.Type),
		EquipmentId:   e.ID,
		DisplayName:   e.Name,
		StockMode:     vendorStockModeToProto(e.Mode),
	}
	if e.Mode == npcs.StockModeLimited {
		quantity := int32(e.Quantity)
		out.Quantity = &quantity
	}
	return out
}

func vendorStockEntriesToProto(es []npcs.StockEntryView) []*sessionpb.VendorStockEntry {
	out := make([]*sessionpb.VendorStockEntry, len(es))
	for i, e := range es {
		out[i] = vendorStockEntryToProto(e)
	}
	return out
}

// worldNPCDescriptorToProto mirrors session.WorldNPCDescriptor field-for-
// field: capabilities and combat_policy cross as plain strings, the same
// open-vocabulary convention the toolkit's own npc.Capability/CombatPolicy
// types already use, so this seam invents no vocabulary of its own.
func worldNPCDescriptorToProto(d sdk.WorldNPCDescriptor) *sessionpb.WorldNPCDescriptor {
	capabilities := make([]string, len(d.Capabilities))
	for i, c := range d.Capabilities {
		capabilities[i] = string(c)
	}
	return &sessionpb.WorldNPCDescriptor{
		TargetId:     d.TargetID,
		Ref:          d.Ref,
		DisplayName:  d.DisplayName,
		Capabilities: capabilities,
		CombatPolicy: string(d.CombatPolicy),
		Inventory:    vendorStockEntriesToProto(d.Inventory),
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

func lifeStateToProto(s sdk.LifeState) sessionpb.LifeState {
	switch s {
	case sdk.LifeStateConscious:
		return sessionpb.LifeState_LIFE_STATE_CONSCIOUS
	case sdk.LifeStateDying:
		return sessionpb.LifeState_LIFE_STATE_DYING
	case sdk.LifeStateStabilized:
		return sessionpb.LifeState_LIFE_STATE_STABILIZED
	case sdk.LifeStateDead:
		return sessionpb.LifeState_LIFE_STATE_DEAD
	case sdk.LifeStateDefeated:
		return sessionpb.LifeState_LIFE_STATE_DEFEATED
	default:
		return sessionpb.LifeState_LIFE_STATE_UNSPECIFIED
	}
}

func deathSaveProgressToProto(p *sdk.DeathSaveProgress) *sessionpb.DeathSaveProgress {
	if p == nil {
		return nil
	}
	return &sessionpb.DeathSaveProgress{
		Successes:         int32(p.Successes),
		Failures:          int32(p.Failures),
		SuccessesNeeded:   int32(p.SuccessesNeeded),
		FailuresRemaining: int32(p.FailuresRemaining),
		Stabilized:        p.Stabilized,
		Dead:              p.Dead,
	}
}

func deathSaveOutcomeToProto(o sdk.DeathSaveOutcome) sessionpb.DeathSaveOutcome {
	switch o {
	case sdk.DeathSaveOutcomeSuccess:
		return sessionpb.DeathSaveOutcome_DEATH_SAVE_OUTCOME_SUCCESS
	case sdk.DeathSaveOutcomeFailure:
		return sessionpb.DeathSaveOutcome_DEATH_SAVE_OUTCOME_FAILURE
	case sdk.DeathSaveOutcomeCriticalFail:
		return sessionpb.DeathSaveOutcome_DEATH_SAVE_OUTCOME_CRITICAL_FAILURE
	case sdk.DeathSaveOutcomeStabilized:
		return sessionpb.DeathSaveOutcome_DEATH_SAVE_OUTCOME_STABILIZED
	case sdk.DeathSaveOutcomeDead:
		return sessionpb.DeathSaveOutcome_DEATH_SAVE_OUTCOME_DEAD
	case sdk.DeathSaveOutcomeRecovered:
		return sessionpb.DeathSaveOutcome_DEATH_SAVE_OUTCOME_RECOVERED
	default:
		return sessionpb.DeathSaveOutcome_DEATH_SAVE_OUTCOME_UNSPECIFIED
	}
}

func deathSaveContinuationToProto(c sdk.DeathSaveContinuation) sessionpb.DeathSaveContinuation {
	switch c {
	case sdk.DeathSaveContinuationEndTurn:
		return sessionpb.DeathSaveContinuation_DEATH_SAVE_CONTINUATION_END_TURN
	case sdk.DeathSaveContinuationKeepTurn:
		return sessionpb.DeathSaveContinuation_DEATH_SAVE_CONTINUATION_KEEP_TURN
	case sdk.DeathSaveContinuationAlreadyAdvanced:
		return sessionpb.DeathSaveContinuation_DEATH_SAVE_CONTINUATION_ALREADY_ADVANCED
	default:
		return sessionpb.DeathSaveContinuation_DEATH_SAVE_CONTINUATION_UNSPECIFIED
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
		// The authored wall-height multiplier, verbatim (rpg-project#273);
		// 0 = not authored — a reader renders the STANDARD height and
		// never multiplies by the raw value.
		Height: float32(b.Height),
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

// atlasSegmentToProto mirrors one authored wall AS THE LINE IT IS. Both ends
// are already fractional axial in the atlas's own frame -- the same frame every
// cell on the wire lives in -- so nothing here converts anything, for the
// reason AtlasToProto's own doc gives: a hex is embedded in the plane in
// exactly one place, and it is not this one.
//
// PRESENTATION, BESIDE THE MECHANICAL TRUTH, NOT INSTEAD OF IT. Boundaries and
// doorways are unchanged and remain what a member may and may not do; this is
// the line those crossings came from, which a client draws instead of chaining
// them back into runs under a straightness tolerance. A door's gap is the
// client's own arithmetic from the doorway it already has.
func atlasSegmentToProto(s sdk.AtlasSegment) *sessionpb.AtlasSegment {
	return &sessionpb.AtlasSegment{
		From: &sessionpb.AxialPoint{Q: s.From.Q, R: s.From.R},
		To:   &sessionpb.AxialPoint{Q: s.To.Q, R: s.To.R},
		// Narrowed to the SAME width AtlasBoundary.height crosses on, so a
		// client never compares a float32 0.7 against a float64 0.7. 0 = not
		// authored = standard height, the same contract as the boundary's.
		Height: float32(s.Height),
	}
}

func atlasSegmentsToProto(ss []sdk.AtlasSegment) []*sessionpb.AtlasSegment {
	out := make([]*sessionpb.AtlasSegment, len(ss))
	for i, s := range ss {
		out[i] = atlasSegmentToProto(s)
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
	case sdk.EventDoor:
		return sessionpb.EventKind_EVENT_KIND_DOOR
	case sdk.EventMissed:
		return sessionpb.EventKind_EVENT_KIND_MISSED
	case sdk.EventActivated:
		return sessionpb.EventKind_EVENT_KIND_ACTIVATED
	case sdk.EventActivationResult:
		return sessionpb.EventKind_EVENT_KIND_ACTIVATION_RESULT
	case sdk.EventDeathSave:
		return sessionpb.EventKind_EVENT_KIND_DEATH_SAVE_ROLLED
	case sdk.EventDoorRevealed:
		return sessionpb.EventKind_EVENT_KIND_DOOR_REVEALED
	case sdk.EventRegionRevealed:
		return sessionpb.EventKind_EVENT_KIND_REGION_REVEALED
	// Holdings (rpg-project#368). Each kind is a STATEMENT -- looted, held,
	// dropped -- because a verb and a beat are named by what the record will
	// say. Nothing here says "took": Take is reserved for the act that lands
	// a thing in inventory (design R10).
	case sdk.EventLooted:
		return sessionpb.EventKind_EVENT_KIND_LOOTED
	case sdk.EventHeld:
		return sessionpb.EventKind_EVENT_KIND_HELD
	case sdk.EventDropped:
		return sessionpb.EventKind_EVENT_KIND_DROPPED
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
// evt.Body stays nil for a kind with no typed body member (SCENE_OPENED,
// TICK, UNKNOWN) and for a beat this build's decoder did not
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
	case sdk.DeathSaveBody:
		evt.Body = &sessionpb.Event_DeathSaveRolled{DeathSaveRolled: &sessionpb.DeathSaveRolled{
			Actor: b.Actor, Roll: int32(b.Roll), Outcome: deathSaveOutcomeToProto(b.Outcome),
			SuccessesAdded: int32(b.SuccessesAdded), FailuresAdded: int32(b.FailuresAdded),
			Successes: int32(b.Successes), Failures: int32(b.Failures),
			SuccessesNeeded: int32(b.SuccessesNeeded), FailuresRemaining: int32(b.FailuresRemaining),
			Stabilized: b.Stabilized, Dead: b.Dead, Recovered: b.Recovered,
			HpRestored: int32(b.HPRestored), Continuation: deathSaveContinuationToProto(b.Continuation),
			PresentationId: b.PresentationID,
		}}
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
	case sdk.ActivatedBody:
		evt.Body = &sessionpb.Event_Activated{Activated: activatedBodyToProto(b)}
	case sdk.ActivationResultBody:
		if result := activationResultBodyToProto(b); result != nil {
			evt.Body = &sessionpb.Event_ActivationResult{ActivationResult: result}
		}
	case sdk.FightStartedBody:
		evt.Body = &sessionpb.Event_FightStarted{FightStarted: &sessionpb.FightStarted{Members: b.Members}}
	case sdk.FightEndedBody:
		evt.Body = &sessionpb.Event_FightEnded{FightEnded: &sessionpb.FightEnded{Cause: dissolveKindToProto(b.Cause)}}
	case sdk.MovedBody:
		evt.Body = &sessionpb.Event_Moved{Moved: &sessionpb.Moved{Member: b.Member, To: positionToProto(b.To)}}
	case sdk.JoinedBody:
		evt.Body = &sessionpb.Event_Joined{Joined: &sessionpb.Joined{Member: b.Member}}
	case sdk.ExitedBody:
		// Holding and Exit (rpg-project#368) carry what left with them and
		// the authored way out they left through. Both are ordinarily
		// empty and empty is the TRUTH rather than "unknown": most
		// departures carry nothing, and a departure from a cell nobody
		// authored as an exit used no exit. A carrier who leaves from
		// anywhere else DROPS what they hold, so this list is never the
		// silent deletion of a holding -- the DROPPED beat says where it
		// landed.
		//
		// PROPS ONLY, and this is the wire's half of design P3: intel is a
		// holding too, and it never appears here or anywhere else, so a
		// departure carrying nothing but knowledge is indistinguishable
		// from one carrying nothing at all.
		evt.Body = &sessionpb.Event_Exited{Exited: &sessionpb.Exited{
			Member: b.Member, Holding: b.Holding, Exit: b.Exit,
		}}
	case sdk.LootedBody:
		// Looter and body, and deliberately nothing about what moved: the
		// beat is identical for a body that carried the run's only secret
		// and one that carried nothing (design P3). What actually moved
		// reaches the looter alone, as their own DOOR_REVEALED.
		evt.Body = &sessionpb.Event_Looted{Looted: &sessionpb.Looted{
			Looter: b.Looter, Body: b.Body,
		}}
	case sdk.HeldBody:
		// To everyone present: an object leaving the floor folds on the
		// TRUTH GRAIN, so every recipient's atlas loses the prop and a
		// client patches its cached map by removing this id -- the
		// load-once, beat-refreshed law running subtractively, where
		// DOOR_REVEALED runs it additively.
		evt.Body = &sessionpb.Event_Held{Held: &sessionpb.Held{
			Holder: b.Holder, Prop: b.Prop,
		}}
	case sdk.DroppedBody:
		// The inverse patch: the prop reappears at `at` for everyone
		// present. Not a player verb -- a drop is what happens when a
		// carrier leaves from anywhere but the scenario's bound exit
		// (design R9), which is what stops a carrier walking off with the
		// only win in the run.
		evt.Body = &sessionpb.Event_Dropped{Dropped: &sessionpb.Dropped{
			Member: b.Member, Prop: b.Prop, At: positionToProto(b.At),
		}}
	case sdk.EndedBody:
		evt.Body = &sessionpb.Event_Ended{Ended: &sessionpb.Ended{Ending: b.Ending}}
	case sdk.DoorBody:
		evt.Body = &sessionpb.Event_Door{Door: &sessionpb.DoorChanged{
			Door:   b.Door,
			State:  doorStateToProto(b.State),
			Actor:  b.Actor,
			Dc:     int32(b.DC),
			Total:  int32(b.Total),
			Beaten: b.Beaten,
		}}
	case sdk.DoorRevealedBody:
		// No Boundaries here: DoorRevealedBody carries none (the masquerade-
		// wall replacement the wire's own field is for is not yet a field
		// this SDK version produces), so the wire field is left unset rather
		// than populated from something this body does not have -- verbatim
		// translation of what the SDK actually sends, not an invented value.
		evt.Body = &sessionpb.Event_DoorRevealed{DoorRevealed: &sessionpb.DoorRevealed{
			Door:     doorRevealedInfoToProto(b),
			Doorways: atlasDoorwaysToProto(b.Doorways),
		}}
	case sdk.RegionRevealedBody:
		// Segments and Sealed do NOT mean the same shape of thing here, and a
		// client that treats them alike will draw the wrong room. Both are
		// carried verbatim; neither is recomputed here.
		//
		// SEGMENTS IS A DIFFERENCE, and adds: the walls this recipient did not
		// have and now does. A wall already presented to them for any reason --
		// the seam their own concealed door hides in, or one footing on floor
		// they can already see -- is deliberately absent, because it is not news
		// and they are already drawing it. So this is append-to-cache, never
		// replace-for-region. It HAS to be a difference: a segment carries no
		// footprint on purpose, so there is no way to ask which cells a wall
		// stands on without leaking what the doorway list withholds. No wall
		// ever leaves, so the atlas after a reveal is the atlas before it union
		// this.
		//
		// SEALED IS SCOPED, AND REPLACES, and the scoping is load-bearing rather
		// than a style choice: a client swaps out the revealed region's cells
		// and keeps every other sealed cell it had. Cells LEAVE this list. A
		// non-knower's sealed list already holds some of the hidden room's own
		// cells -- the footing of the walls presented to them (design C18),
		// which reaches them as ownerless floor, and ownerless floor is floor
		// nobody stands on -- and the moment the room is theirs those same cells
		// are ordinary standable floor. A client that appended would leave a
		// room it can see permanently unwalkable at its edges. So the atlas
		// after a reveal is (the atlas before it, less the revealed region's
		// cells) union this, which is why the beat carries the region's cells
		// beside it. A difference could only ever add, and this field has to be
		// able to take away.
		sealed := make([]*sessionpb.Position, len(b.Sealed))
		for i, c := range b.Sealed {
			sealed[i] = positionToProto(c)
		}
		evt.Body = &sessionpb.Event_RegionRevealed{RegionRevealed: &sessionpb.RegionRevealed{
			Region:     atlasRegionToProto(b.Region),
			Props:      atlasPropsToProto(b.Props),
			Boundaries: atlasBoundariesToProto(b.Boundaries),
			Segments:   atlasSegmentsToProto(b.Segments),
			Sealed:     sealed,
		}}
	default:
		// nil (no typed body for this kind) or a body type this build does
		// not recognize: leave evt.Body nil. payload stays the passthrough
		// carrier.
	}
}

// activatedBodyToProto trusts Session's bodyFor validation of the required
// actor and ability identity because the API only converts SDK-authored bodies;
// ActivationResult retains extra defensive oneof counting for its result arms.
func activatedBodyToProto(body sdk.ActivatedBody) *sessionpb.Activated {
	return &sessionpb.Activated{
		Actor: body.Actor, Ability: abilityRefToProto(body.Ability), Target: body.Target,
	}
}

// activationResultBodyToProto preserves the SDK's one-result invariant. A
// nil or malformed decoded SDK body has no wire body rather than an arbitrary
// first arm; payload remains untouched on the enclosing Event.
func activationResultBodyToProto(body sdk.ActivationResultBody) *sessionpb.ActivationResult {
	result := &sessionpb.ActivationResult{Actor: body.Actor}
	populated := 0
	if body.HealingApplied != nil {
		populated++
		result.Result = &sessionpb.ActivationResult_HealingApplied{
			HealingApplied: healingAppliedBodyToProto(body.HealingApplied),
		}
	}
	if body.ConditionApplied != nil {
		populated++
		result.Result = &sessionpb.ActivationResult_ConditionApplied{
			ConditionApplied: conditionAppliedBodyToProto(body.ConditionApplied),
		}
	}
	if body.ConditionRemoved != nil {
		populated++
		result.Result = &sessionpb.ActivationResult_ConditionRemoved{
			ConditionRemoved: conditionRemovedBodyToProto(body.ConditionRemoved),
		}
	}
	if body.CapacityGranted != nil {
		populated++
		result.Result = &sessionpb.ActivationResult_CapacityGranted{
			CapacityGranted: capacityGrantedBodyToProto(body.CapacityGranted),
		}
	}
	if populated != 1 {
		return nil
	}
	return result
}

// healingAppliedBodyToProto mirrors a heal onto the wire without deriving one
// representation from the other. New bodies carry Calculation only; legacy
// Story records retain their deprecated Roll and Modifier scalars.
func healingAppliedBodyToProto(body *sdk.HealingAppliedBody) *sessionpb.HealingApplied {
	if body == nil {
		return nil
	}
	out := &sessionpb.HealingApplied{
		Target: body.Target, Amount: int32(body.Amount), Requested: int32(body.Requested),
		SourceRef: body.SourceRef, SourceName: body.SourceName,
		HpBefore: int32(body.HPBefore), HpAfter: int32(body.HPAfter),
	}
	if body.Calculation != nil {
		// New bodies populate only Calculation. Its total is authoritative;
		// neither Requested nor the deprecated scalars are derived from it.
		out.Calculation = rollCalculationToProto(body.Calculation)
	} else {
		// Legacy bodies retain exactly the two deprecated scalar fields and do
		// not gain a fabricated calculation.
		out.Roll = int32(body.Roll)         //nolint:staticcheck // Required read compatibility for pre-trace Story records.
		out.Modifier = int32(body.Modifier) //nolint:staticcheck // Required read compatibility for pre-trace Story records.
	}
	return out
}

func conditionAppliedBodyToProto(body *sdk.ConditionAppliedBody) *sessionpb.ConditionApplied {
	if body == nil {
		return nil
	}
	return &sessionpb.ConditionApplied{Target: body.Target, Ref: body.Ref, Name: body.Name}
}

func conditionRemovedBodyToProto(body *sdk.ConditionRemovedBody) *sessionpb.ConditionRemoved {
	if body == nil {
		return nil
	}
	return &sessionpb.ConditionRemoved{
		Target: body.Target, Ref: body.Ref, Name: body.Name, Reason: body.Reason,
	}
}

func capacityGrantedBodyToProto(body *sdk.CapacityGrantedBody) *sessionpb.CapacityGranted {
	if body == nil {
		return nil
	}
	return &sessionpb.CapacityGranted{Member: body.Member, Description: body.Description}
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
	props := atlasPropsToProto(a.Props)
	// Sealed cells are cells: same absolute frame, same converter, and every
	// one of them is in Cells above as well. Sealed floor is still floor --
	// drawn and lit like the floor beside it -- and this list only says whose
	// feet may not go there.
	sealed := make([]*sessionpb.Position, len(a.Sealed))
	for i, c := range a.Sealed {
		sealed[i] = positionToProto(c)
	}
	return &sessionpb.GetAtlasResponse{
		Grid:       gridKindToProto(a.Grid),
		Layout:     hexLayoutToProto(a.Layout),
		Cells:      cells,
		Props:      props,
		Boundaries: atlasBoundariesToProto(a.Boundaries),
		Doorways:   atlasDoorwaysToProto(a.Doorways),
		Regions:    atlasRegionsToProto(a.Regions),
		Segments:   atlasSegmentsToProto(a.Segments),
		Sealed:     sealed,
		Exits:      atlasExitsToProto(a.Exits),
	}
}

// atlasExitsToProto mirrors the authored ways out (rpg-project#368, design
// §5's wire paragraph): an id and a cell, the same for every member the way
// `start` is, so a map can DRAW the way out.
//
// EXITS DO NOT GATE ANYTHING HERE. Leave is offered everywhere and the server
// decides what a departure means -- a departure from the vault has to remain
// possible, because dropping what you carry when you leave from the wrong
// place (design R9) is the rule that stops a carrier walking off with the
// run. This list is for drawing, never for deciding.
func atlasExitsToProto(es []sdk.AtlasExit) []*sessionpb.AtlasExit {
	out := make([]*sessionpb.AtlasExit, len(es))
	for i, e := range es {
		out[i] = &sessionpb.AtlasExit{Id: e.ID, At: positionToProto(e.At)}
	}

	return out
}

// atlasPropToProto mirrors one session.AtlasProp -- shared by AtlasToProto and
// RegionRevealed's props, which the SDK's own doc promises carries them
// "exactly as GetAtlasResponse.props would" (rpg-project#350/#351).
func atlasPropToProto(prop sdk.AtlasProp) *sessionpb.AtlasProp {
	return &sessionpb.AtlasProp{
		// ID and Holdable (rpg-project#368, design §5). The id is the
		// author's `place[].id` and is the ONLY name a Hold request can
		// use, so a prop carrying none is one no verb can name -- which is
		// the author's decision, not this converter's, and the empty string
		// carries it forward honestly. Holdable is structure on the truth
		// grain: a holdable thing looks holdable, so a client offers Hold
		// where it is true and never guesses from a ref or an id.
		Id:                prop.ID,
		Holdable:          prop.Holdable,
		Ref:               prop.Ref,
		At:                positionToProto(prop.At),
		BlocksMovement:    prop.BlocksMovement,
		BlocksLineOfSight: prop.BlocksLineOfSight,
		Facing:            prop.Facing,
		OffsetX:           float32(prop.Offset[0]),
		OffsetY:           float32(prop.Offset[1]),
		OffsetZ:           float32(prop.Offset[2]),
	}
}

func atlasPropsToProto(ps []sdk.AtlasProp) []*sessionpb.AtlasProp {
	out := make([]*sessionpb.AtlasProp, len(ps))
	for i, p := range ps {
		out[i] = atlasPropToProto(p)
	}
	return out
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

// VerbActivate joins them at protos v0.1.144 (rpg-project#300), and it is the
// first verb whose declarations arrive MANY PER MEMBER rather than one. Leaving
// it unmapped would label every activation a barbarian can reach as
// VERB_UNSPECIFIED -- not one mislabelled row but six, on the panel this slice
// exists to fill.
//
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
	case sdk.VerbEndTurn:
		return sessionpb.Verb_VERB_END_TURN
	case sdk.VerbActivate:
		return sessionpb.Verb_VERB_ACTIVATE
	case sdk.VerbDeathSave:
		return sessionpb.Verb_VERB_DEATH_SAVE
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
// about the price, the same way Declaration.Available=false is an answer and
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

// targetKindToProto mirrors the SDK's closed selector-shape enum. Unknown
// values are producer defects and therefore reach UNSPECIFIED rather than
// being guessed from the declaration's other fields.
func targetKindToProto(k sdk.TargetKind) sessionpb.TargetKind {
	switch k {
	case sdk.TargetNone:
		return sessionpb.TargetKind_TARGET_KIND_NONE
	case sdk.TargetMember:
		return sessionpb.TargetKind_TARGET_KIND_MEMBER
	case sdk.TargetPath:
		return sessionpb.TargetKind_TARGET_KIND_PATH
	default:
		return sessionpb.TargetKind_TARGET_KIND_UNSPECIFIED
	}
}

// targetCandidateToProto mirrors one ruled candidate. Candidate availability
// and refusal are independent from the declaration-level gate and are copied
// only from this candidate.
func targetCandidateToProto(c sdk.TargetCandidate) *sessionpb.TargetCandidate {
	return &sessionpb.TargetCandidate{
		Member:    c.Member,
		Available: c.Available,
		Why:       shortfallToProto(c.Why),
	}
}

func targetCandidatesToProto(cs []sdk.TargetCandidate) []*sessionpb.TargetCandidate {
	out := make([]*sessionpb.TargetCandidate, len(cs))
	for i, candidate := range cs {
		out[i] = targetCandidateToProto(candidate)
	}
	return out
}

// declarationToProto mirrors the SDK's compiled declaration field-for-field.
// It neither derives availability nor transforms selectors: opaque IDs, full
// attack refs, target shape, and every independently ruled candidate cross
// unchanged. Optional fields preserve presence, including a present zero
// Remaining value, and repeated candidates use make-then-map so a non-nil
// empty SDK answer remains non-nil empty in Go.
func declarationToProto(d sdk.Declaration) *sessionpb.Declaration {
	out := &sessionpb.Declaration{
		Verb:       verbToProto(d.Verb),
		Slot:       slotToProto(d.Slot),
		Available:  d.Available,
		Why:        shortfallToProto(d.Why),
		Id:         d.ID,
		TargetKind: targetKindToProto(d.TargetKind),
		Candidates: targetCandidatesToProto(d.Candidates),
	}
	if d.Remaining != nil {
		remaining := int32(*d.Remaining)
		out.Remaining = &remaining
	}
	if d.Attack != nil {
		out.Attack = attackRefToProto(*d.Attack)
	}
	if d.Ability != nil {
		out.Ability = abilityRefToProto(*d.Ability)
	}
	if d.DeathSave != nil {
		out.DeathSave = deathSaveRefToProto(*d.DeathSave)
	}
	return out
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
	case sdk.ShortfallTargetOutOfReach:
		return sessionpb.ShortfallReason_SHORTFALL_REASON_TARGET_OUT_OF_REACH
	case sdk.ShortfallUnavailable:
		// The ability's own precondition refusing -- already raging, already
		// at full hit points. NOT a budget, so no currency is populated, and
		// collapsing it into NO_BUDGET would tell a raging barbarian to come
		// back next turn.
		return sessionpb.ShortfallReason_SHORTFALL_REASON_UNAVAILABLE
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
	case sdk.CurrencyCharges:
		// Charges of a named feature resource -- rage uses, Second Wind uses.
		// A ledger that ran out, just not one of the turn's three; WHICH one
		// is named only in the shortfall's text, because this seam does not
		// enumerate the rulebook's resource keys.
		return sessionpb.Currency_CURRENCY_CHARGES
	default:
		return sessionpb.Currency_CURRENCY_UNSPECIFIED
	}
}

// shortfallToProto mirrors session.Shortfall. Present exactly when the SDK
// set it -- nil in, nil out -- matching Declaration.why's presence law: PRESENT
// EXACTLY WHEN Available is false (rpg-toolkit#1010).
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

// rollSourceToProto copies the provider-authored identity without parsing its
// ref or deriving a display label.
func rollSourceToProto(source *sdk.RollSource) *sessionpb.RollSource {
	if source == nil {
		return nil
	}
	return &sessionpb.RollSource{Ref: source.Ref, Name: source.Name, Label: source.Label}
}

// diceRerollToProto copies one sourced replacement. Ordering is owned by the
// caller's DiceTrace and is preserved by diceTraceToProto.
func diceRerollToProto(reroll *sdk.DiceReroll) *sessionpb.DiceReroll {
	if reroll == nil {
		return nil
	}
	return &sessionpb.DiceReroll{
		DieIndex: int32(reroll.DieIndex),
		Before:   int32(reroll.Before),
		After:    int32(reroll.After),
		Source:   rollSourceToProto(&reroll.Source),
	}
}

func intsToInt32s(values []int) []int32 {
	out := make([]int32, len(values))
	for i, value := range values {
		out[i] = int32(value)
	}
	return out
}

// diceTraceToProto copies the complete physical dice history field-for-field.
// Subtotal is authoritative and is never recomputed from the face lists.
func diceTraceToProto(trace *sdk.DiceTrace) *sessionpb.DiceTrace {
	if trace == nil {
		return nil
	}
	rerolls := make([]*sessionpb.DiceReroll, len(trace.Rerolls))
	for i := range trace.Rerolls {
		rerolls[i] = diceRerollToProto(&trace.Rerolls[i])
	}
	return &sessionpb.DiceTrace{
		Notation:      trace.Notation,
		DieSize:       int32(trace.DieSize),
		OriginalRolls: intsToInt32s(trace.OriginalRolls),
		Rerolls:       rerolls,
		FinalRolls:    intsToInt32s(trace.FinalRolls),
		KeptIndices:   intsToInt32s(trace.KeptIndices),
		Subtotal:      int32(trace.Subtotal),
	}
}

// rollComponentToProto preserves optional modifier presence, including a
// present zero. Dice and source are independently copied and never aliased.
func rollComponentToProto(component *sdk.RollComponent) *sessionpb.RollComponent {
	if component == nil {
		return nil
	}
	out := &sessionpb.RollComponent{
		Source: rollSourceToProto(&component.Source),
		Dice:   diceTraceToProto(component.Dice),
	}
	if component.Modifier != nil {
		modifier := int32(*component.Modifier)
		out.Modifier = &modifier
	}
	return out
}

// rollCalculationToProto preserves component production order and copies the
// producer's authoritative total without validation or arithmetic.
func rollCalculationToProto(calculation *sdk.RollCalculation) *sessionpb.RollCalculation {
	if calculation == nil {
		return nil
	}
	components := make([]*sessionpb.RollComponent, len(calculation.Components))
	for i := range calculation.Components {
		components[i] = rollComponentToProto(&calculation.Components[i])
	}
	return &sessionpb.RollCalculation{Components: components, Total: int32(calculation.Total)}
}

// hasRollComponent reports which of DamageComponent's two SDK read shapes is
// populated. Session's strict decoder guarantees exactly one representation;
// this converter only selects its carrier and neither validates nor merges it.
func hasRollComponent(component *sdk.RollComponent) bool {
	if component == nil {
		return false
	}
	return component.Source.Ref != "" || component.Source.Name != "" || component.Source.Label != "" ||
		component.Dice != nil || component.Modifier != nil
}

func damageComponentsToProto(in []sdk.DamageComponent) []*sessionpb.DamageComponent {
	out := make([]*sessionpb.DamageComponent, len(in))
	for i := range in {
		component := &in[i]
		var multiplier *float64
		if component.Multiplier != nil {
			value := *component.Multiplier
			multiplier = &value
		}
		converted := &sessionpb.DamageComponent{
			Source: component.Source, DamageType: damageTypeToProto(component.DamageType),
			Multiplier: multiplier,
		}
		if hasRollComponent(&component.Roll) {
			// New bodies populate only Roll. Deprecated scalar fields stay empty,
			// even if a malformed in-memory value also happens to carry them.
			converted.Roll = rollComponentToProto(&component.Roll)
		} else {
			// Legacy bodies populate only their deprecated scalars. In
			// particular, no roll trace is fabricated from final faces.
			converted.SourceRef = component.SourceRef                 //nolint:staticcheck // Required pre-trace Story read compatibility.
			converted.Dice = component.Dice                           //nolint:staticcheck // Required pre-trace Story read compatibility.
			converted.FinalRolls = intsToInt32s(component.FinalRolls) //nolint:staticcheck // Required pre-trace Story read compatibility.
			converted.FlatBonus = int32(component.FlatBonus)          //nolint:staticcheck // Required pre-trace Story read compatibility.
		}
		out[i] = converted
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

// abilityRefToProto mirrors the sole public identity of a compiled Activate
// declaration. Present exactly when the SDK carries one, absent otherwise --
// the same presence law declarationToProto keeps for Attack, and for the same
// reason: a client renders this verbatim, so an ability with no name is a
// button with no label rather than a defaulted one.
func abilityRefToProto(a sdk.AbilityRef) *sessionpb.AbilityRef {
	return &sessionpb.AbilityRef{Ref: a.Ref, Name: a.Name}
}

func deathSaveRefToProto(d sdk.DeathSaveRef) *sessionpb.DeathSaveRef {
	return &sessionpb.DeathSaveRef{Name: d.Name}
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
		Member:     p.Member,
		Name:       p.Name,
		Kind:       memberKindToProto(p.Kind),
		Standing:   standingToProto(p.Standing),
		Active:     p.Active,
		LifeState:  lifeStateToProto(p.LifeState),
		DeathSaves: deathSaveProgressToProto(p.DeathSaves),
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

// doorStateToProto mirrors the SDK's string door state onto the wire enum.
// A state this build does not recognize maps to UNSPECIFIED, the same
// delivered-not-guessed posture eventKindToProto takes.
func doorStateToProto(s string) sessionpb.DoorState {
	switch s {
	case "open":
		return sessionpb.DoorState_DOOR_STATE_OPEN
	case "closed":
		return sessionpb.DoorState_DOOR_STATE_CLOSED
	case "locked":
		return sessionpb.DoorState_DOOR_STATE_LOCKED
	default:
		return sessionpb.DoorState_DOOR_STATE_UNSPECIFIED
	}
}

// doorApproachToProto mirrors one accepted route through a lock (the
// multi-approach ruling, rpg-project#350): an ability/skill ref, an optional
// tool, and this route's own DC -- forcing a door and picking its lock need
// not cost the same, so the DC lives per approach, not per lock.
func doorApproachToProto(a sdk.DoorApproach) *sessionpb.CheckApproach {
	return &sessionpb.CheckApproach{Ability: a.Ability, Tool: a.Tool, Dc: int32(a.DC)}
}

func doorApproachesToProto(as []sdk.DoorApproach) []*sessionpb.CheckApproach {
	out := make([]*sessionpb.CheckApproach, len(as))
	for i, a := range as {
		out[i] = doorApproachToProto(a)
	}
	return out
}

// doorRevealedInfoToProto groups DoorRevealedBody's flat door/state/
// approaches into the wire's nested DoorInfo -- the same shape GetDoors
// already returns for this door, per DoorRevealed.door's own doc ("exactly
// as this recipient's GetDoors would now list it"). Approaches is present
// only while the door is locked (DoorRevealedBody's own field law), so a
// non-empty list is the presence signal for Lock, matching doorToProto's
// "Lock unset is not locked" convention field-for-field rather than
// re-deriving it from State.
func doorRevealedInfoToProto(b sdk.DoorRevealedBody) *sessionpb.DoorInfo {
	out := &sessionpb.DoorInfo{Door: b.Door, State: doorStateToProto(b.State)}
	if len(b.Approaches) > 0 {
		out.Lock = &sessionpb.DoorLock{Approaches: doorApproachesToProto(b.Approaches)}
	}
	return out
}

// doorToProto mirrors one live door. The lock rides only while it is real —
// DoorInfo.lock unset is "not locked", never a lock with zero approaches --
// and its approaches list is copied verbatim (rpg-project#350's dialect: a
// lock is a set of accepted routes, not one ability and one DC).
func doorToProto(d sdk.Door) *sessionpb.DoorInfo {
	out := &sessionpb.DoorInfo{Door: d.ID, State: doorStateToProto(d.State)}
	if d.Lock != nil {
		out.Lock = &sessionpb.DoorLock{Approaches: doorApproachesToProto(d.Lock.Approaches)}
	}
	return out
}

// tradeItemFromProto mirrors one wire TradeItem onto the SDK's shape. The
// equipment type crosses as the same plain string vendorStockEntryToProto
// already carries the other direction -- no second equipment-type mapping.
func tradeItemFromProto(i *sessionpb.TradeItem) sdk.TradeItem {
	return sdk.TradeItem{
		Type:     shared.EquipmentType(i.GetEquipmentType()),
		ID:       i.GetEquipmentId(),
		Quantity: int(i.GetQuantity()),
	}
}

// tradeOfferFromProto mirrors one wire TradeOffer. A nil proto offer (the
// field unset) becomes the zero TradeOffer -- an empty Items slice, which is
// exactly what an omitted `give` on the wire means (session.TradeInput's own
// doc: Give must be empty this wave; the SDK's own ErrGiveNotSupported
// refusal is what tells a caller who sent one anyway, not a nil check here).
func tradeOfferFromProto(o *sessionpb.TradeOffer) sdk.TradeOffer {
	items := make([]sdk.TradeItem, len(o.GetItems()))
	for i, it := range o.GetItems() {
		items[i] = tradeItemFromProto(it)
	}
	return sdk.TradeOffer{Items: items}
}
