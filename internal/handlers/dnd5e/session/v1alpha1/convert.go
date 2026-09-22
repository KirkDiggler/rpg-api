package sessionv1alpha1

import (
	"errors"
	"fmt"
	"math"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/currency"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/equipment"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/npcs"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	"github.com/KirkDiggler/rpg-api/internal/converters/assetref"

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

// positionPtrFromProto mirrors an OPTIONAL wire Position onto a pointer, which
// is what the SDK takes wherever a position may legitimately be absent.
//
// A nil proto position stays nil rather than becoming the origin, and that is
// the whole reason this exists beside positionFromProto: (0,0) is a real cell
// on this grid, so the zero Position cannot also mean "none was named". Fold
// the two together and a cast that pointed at nothing becomes a cast that
// pointed at the middle of the map, which no layer below could ever refuse.
// Whether absence is LEGAL here is the SDK's ruling, not this converter's.
func positionPtrFromProto(p *sessionpb.Position) *spatial.Position {
	if p == nil {
		return nil
	}
	pos := positionFromProto(p)
	return &pos
}

// moneyToProto mirrors currency.Money onto the wire Money.
func moneyToProto(m currency.Money) *sessionpb.Money {
	return &sessionpb.Money{Copper: int32(m.Copper)}
}

// moneyFromProto mirrors the wire Money onto currency.Money. A nil proto
// Money (unset field) becomes the zero Money -- the same nil-safety
// convention positionFromProto keeps for its own zero value.
func moneyFromProto(m *sessionpb.Money) currency.Money {
	return currency.Money{Copper: int(m.GetCopper())}
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

// vendorStockEntryToProto mirrors one resolved vendor stock row, and resolves
// its unit price via equipment.PriceOf (rpg-toolkit#1534) -- a pure toolkit
// lookup composed here, not a rule computed by this handler (design rule 8):
// npcs.StockEntryView itself carries no price (the toolkit's own
// TestVendorViewUsesResolvedEquipmentWithoutPrices pins that), and
// VendorStockEntry.price's own wire doc says it is "server-computed
// (equipment.PriceOf)" -- this is that computation. Quantity is meaningful
// only when Mode is LIMITED (the wire message's own doc); an unlimited
// entry's Quantity is the toolkit's own zero value and stays unset here
// rather than carried as a meaningless zero.
//
// PriceOf can fail only for a catalog entry whose Cost string does not parse
// (an ErrBadCost-class data defect, rpg-toolkit#1524's own doc) -- never for
// an unknown id, because e already came from a stock entry the toolkit
// itself resolved a Name for from that same catalog.
func vendorStockEntryToProto(e npcs.StockEntryView) (*sessionpb.VendorStockEntry, error) {
	price, err := equipment.PriceOf(e.ID)
	if err != nil {
		return nil, fmt.Errorf("vendor stock entry %q: %w", e.ID, err)
	}
	out := &sessionpb.VendorStockEntry{
		EquipmentType: string(e.Type),
		EquipmentId:   e.ID,
		DisplayName:   e.Name,
		StockMode:     vendorStockModeToProto(e.Mode),
		Price:         moneyToProto(price),
		// PlayerSold (rpg-toolkit#1537) is carried straight across, unlike
		// Price -- a plain bool the toolkit already resolved, not a lookup
		// this handler performs. Display treatment is the client's call
		// (both the toolkit's and the wire message's own doc say so).
		PlayerSold: e.PlayerSold,
	}
	if e.Mode == npcs.StockModeLimited {
		quantity := int32(e.Quantity)
		out.Quantity = &quantity
	}
	return out, nil
}

func vendorStockEntriesToProto(es []npcs.StockEntryView) ([]*sessionpb.VendorStockEntry, error) {
	out := make([]*sessionpb.VendorStockEntry, len(es))
	for i, e := range es {
		converted, err := vendorStockEntryToProto(e)
		if err != nil {
			return nil, err
		}
		out[i] = converted
	}
	return out, nil
}

// worldNPCDescriptorToProto mirrors session.WorldNPCDescriptor field-for-
// field: capabilities and combat_policy cross as plain strings, the same
// open-vocabulary convention the toolkit's own npc.Capability/CombatPolicy
// types already use, so this seam invents no vocabulary of its own. Fallible
// only because vendorStockEntriesToProto is (see its own doc).
func worldNPCDescriptorToProto(d sdk.WorldNPCDescriptor) (*sessionpb.WorldNPCDescriptor, error) {
	capabilities := make([]string, len(d.Capabilities))
	for i, c := range d.Capabilities {
		capabilities[i] = string(c)
	}
	inventory, err := vendorStockEntriesToProto(d.Inventory)
	if err != nil {
		return nil, err
	}
	return &sessionpb.WorldNPCDescriptor{
		TargetId:     d.TargetID,
		Ref:          d.Ref,
		DisplayName:  d.DisplayName,
		Capabilities: capabilities,
		CombatPolicy: string(d.CombatPolicy),
		Inventory:    inventory,
	}, nil
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

// placementKindToProto maps the session's closed placement vocabulary onto
// the wire's enum. An unrecognized kind maps to UNSPECIFIED, which the wire
// defines as a producer defect: the session grew a kind of placement this
// build's protos cannot name, and saying so beats calling it a monster.
func placementKindToProto(k sdk.PlacementKind) sessionpb.PlacementKind {
	switch k {
	case sdk.PlacementMonster:
		return sessionpb.PlacementKind_PLACEMENT_KIND_MONSTER
	case sdk.PlacementProp:
		return sessionpb.PlacementKind_PLACEMENT_KIND_PROP
	default:
		return sessionpb.PlacementKind_PLACEMENT_KIND_UNSPECIFIED
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
	case sdk.DissolveByStance:
		// The third cause (rpg-project#375, design §3.5/§6, R1): the two
		// sides stopped being sides. Same trap as BY_DEFEAT above -- a
		// missing case reports the camp turning as a fight that ended for
		// no stated reason.
		return sessionpb.DissolveKind_DISSOLVE_KIND_BY_STANCE
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
//
// BY_STANCE is accepted by the same argument (rpg-project#375): the wire's
// own doc calls it "like BY_DEFEAT, not honestly declarable by a caller",
// and the SDK treats it the same way -- the verb's answer is causeOf(what
// the composition actually did), whatever was handed in.
func dissolveCauseFromProto(k sessionpb.DissolveKind) (sdk.DissolveCause, error) {
	switch k {
	case sessionpb.DissolveKind_DISSOLVE_KIND_BY_DECISION:
		return sdk.ByDecision(), nil
	case sessionpb.DissolveKind_DISSOLVE_KIND_BY_DEFEAT:
		return sdk.ByDefeat(), nil
	case sessionpb.DissolveKind_DISSOLVE_KIND_BY_STANCE:
		return sdk.ByStance(), nil
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
	out := &sessionpb.Seen{
		Position:  positionToProto(s.Position),
		Equipment: seenEquipmentToProto(s.Equipment),
	}
	// Nil is unobserved, not StandingUp. The wire enum remains UNSPECIFIED
	// unless the provider supplied an observed value.
	if s.Standing != nil {
		out.Standing = standingToProto(*s.Standing)
	}
	return out
}

// seenEquipmentToProto mirrors what a subject was observed holding, minting the
// asset identity a client keys a model off (rpg-toolkit#1615).
//
// ABSENT STAYS ABSENT, and that is the whole point of the message being a
// message. Nil means the hands were not observed — nothing with a sheet behind
// it, or testimony older than the field — and it must not become an empty
// SeenEquipment on the wire, because a client reading that would draw somebody
// whose hands nobody looked at as somebody standing there unarmed.
//
// An observed-empty hand is the other claim and survives as an empty string:
// the message is present, the hand is not holding anything. assetref.Item keeps
// it empty rather than minting an unrenderable "dnd5e:item:".
func seenEquipmentToProto(e *sdk.SeenEquipment) *sessionpb.SeenEquipment {
	if e == nil {
		return nil
	}
	return &sessionpb.SeenEquipment{
		MainHand: assetref.Item(e.MainHand),
		OffHand:  assetref.Item(e.OffHand),
	}
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
//
// # Stance, per viewer, carried and never derived
//
// `Sighting.stance` (rpg-api-protos#340, rpg-project#458) is what THIS VIEWER
// believes the subject's stance toward them to be, so the ring under a token
// is a belief rather than the roster's truth. The composition answers it
// through `encounter.BelievedStance`; the session seam carries it beside Name
// and Kind, and this converter copies it.
//
// EMPTY CROSSES AS EMPTY, and that is the load-bearing case rather than an
// edge. The seam leaves it empty when the run cannot answer — a subject who is
// not a member, or one in no faction at all, which a world NPC is — and the
// wire's own doc defines empty as "the observer has no word for it". Mapping
// that to "neutral" would be this seam inventing a belief nobody holds, and a
// client would draw a confident ring around a creature whose side is simply
// unknown. The fallback belongs to the client, which has the roster's faction
// color to fall back TO; this seam has nothing to fall back to and must not
// pretend otherwise.
//
// NOTHING IS DERIVED HERE EITHER. Filling it from the roster's faction would
// make a per-viewer belief field carry shared truth, and the first `pretend`
// would have to UNDO a lie this converter told rather than simply start
// telling a different truth.
func sightingToProto(s sdk.Sighting) *sessionpb.Sighting {
	return &sessionpb.Sighting{
		Subject:    s.Subject,
		Name:       s.Name,
		Kind:       memberKindToProto(s.Kind),
		Stance:     s.Stance,
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

func sightAreaToProto(a sdk.SightArea) *sessionpb.SightArea {
	return &sessionpb.SightArea{
		Id:         a.ID,
		Name:       a.Name,
		SourceRef:  a.Ref,
		Center:     positionToProto(a.Center),
		RadiusFeet: int32(a.RadiusFeet),
	}
}

func sightAreasToProto(areas []sdk.SightArea) []*sessionpb.SightArea {
	out := make([]*sessionpb.SightArea, len(areas))
	for i, area := range areas {
		out[i] = sightAreaToProto(area)
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
	// The fall's receipt (rpg-project#496, R5). It lands beside the beat that
	// causes it, and in the same change as its body below for the cast door's
	// reason: an unmapped kind demotes to EVENT_KIND_UNKNOWN with a nil body,
	// and this beat is the ONLY account anyone gets that the party was paid.
	// Experience is read-only over the wire (R4) -- there is no RPC a client
	// could call to ask what it missed -- so a demoted arm would leave a
	// client reconstructing the grant from the monster's worth and the
	// roster, and a reconstruction lies the moment either one moves.
	case sdk.EventExperienceGained:
		return sessionpb.EventKind_EVENT_KIND_EXPERIENCE_GAINED
	case sdk.EventFightEnded:
		return sessionpb.EventKind_EVENT_KIND_FIGHT_ENDED
	case sdk.EventStruck:
		return sessionpb.EventKind_EVENT_KIND_STRUCK
	case sdk.EventDoor:
		return sessionpb.EventKind_EVENT_KIND_DOOR
	// The first shenanigan (rpg-project#454). BOTH ARMS LAND IN THE SAME
	// CHANGE as the body below, for the cast door's stated reason: an
	// unmapped kind does not fail, it demotes to EVENT_KIND_UNKNOWN and its
	// body stays nil -- and this beat is the ONLY account of its roll, since
	// IntimidateResponse deliberately carries no beaten, total or dc. A
	// demoted arm would lose the die for the whole table, the actor
	// included, rather than degrade one field of it.
	case sdk.EventIntimidated:
		return sessionpb.EventKind_EVENT_KIND_INTIMIDATED
	// The front room goblin (rpg-project#458). EventPersuaded is the threat
	// beat's twin and takes its reasoning whole. EventAnswered is the second
	// roll -- the WORLD's, on the author's table -- and it is the arm that
	// would be missed most quietly: the creature's line, the fact it taught
	// and whether it bolted all ride this one body, so a demotion would leave
	// a goblin running out of the room with nothing in the log saying why.
	case sdk.EventPersuaded:
		return sessionpb.EventKind_EVENT_KIND_PERSUADED
	case sdk.EventAnswered:
		return sessionpb.EventKind_EVENT_KIND_ANSWERED
	// The creature's table (rpg-project#465). EventTempered lands in the same
	// change as its body below, on EventAnswered's argument: a dealt
	// temperament is rolled ONCE, at the door, and this beat is the only
	// account of that roll anyone ever gets. Demoted, the streamer would watch
	// four goblins off one sheet behave differently with nothing in the log
	// saying why.
	case sdk.EventTempered:
		return sessionpb.EventKind_EVENT_KIND_TEMPERED
	// A ROUTED WALK THAT MOVED NOBODY (rpg-project#465, from Kirk's walk).
	// Here for EventTempered's reason and one of its own: the world clock
	// charges a round per driven creature whether or not anybody moves, so
	// demoted this beat would leave a spent round narrated as nothing at all
	// -- and a reader could not tell a creature nobody asked from one sent
	// somewhere it could not reach.
	case sdk.EventStayed:
		return sessionpb.EventKind_EVENT_KIND_STAYED
	case sdk.EventMissed:
		return sessionpb.EventKind_EVENT_KIND_MISSED
	case sdk.EventCastMissed:
		return sessionpb.EventKind_EVENT_KIND_CAST_MISSED
	case sdk.EventWarded:
		return sessionpb.EventKind_EVENT_KIND_WARDED
	case sdk.EventCastWarded:
		return sessionpb.EventKind_EVENT_KIND_CAST_WARDED
	case sdk.EventActivated:
		return sessionpb.EventKind_EVENT_KIND_ACTIVATED
	case sdk.EventActivationResult:
		return sessionpb.EventKind_EVENT_KIND_ACTIVATION_RESULT
	case sdk.EventDeathSave:
		return sessionpb.EventKind_EVENT_KIND_DEATH_SAVE_ROLLED
	// ONE NOUN, ONE REVEAL (design rpg-project#490, E4). The SDK retired
	// EventDoorRevealed and EventRegionRevealed: one authored secret was
	// split across two kinds because the engine hid doors and regions with
	// two separate flags, and a concealment is the one thing an author
	// hides. Its cells, props, member doors, boundaries, walls and touched
	// regions arrive on this one beat instead. The older wire kinds stay on
	// the proto, deprecated; nothing maps to them any more.
	case sdk.EventConcealmentRevealed:
		return sessionpb.EventKind_EVENT_KIND_CONCEALMENT_REVEALED
	case sdk.EventSighted:
		return sessionpb.EventKind_EVENT_KIND_SIGHTED
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
	case sdk.EventStanceChanged:
		return sessionpb.EventKind_EVENT_KIND_STANCE_CHANGED
	case sdk.EventArrived:
		return sessionpb.EventKind_EVENT_KIND_ARRIVED
	case sdk.EventWindowOpened:
		return sessionpb.EventKind_EVENT_KIND_WINDOW_OPENED
	// The post-roll window (rpg-project#398). A SECOND KIND rather than a
	// second shape of the first: WindowOpened is movement-shaped -- mover,
	// from, to, all load-bearing -- and a window opened on a d20 has no
	// mover and no cells, so widening it would put three zero values that
	// lie on every post-roll beat.
	case sdk.EventRollWindowOpened:
		return sessionpb.EventKind_EVENT_KIND_ROLL_WINDOW_OPENED
	// The cast door (rpg-project#405). TWO KINDS rather than one, and neither
	// reuses an existing body: DeathSaveRolled is death-shaped -- stabilized,
	// dead, hp_restored, a continuation -- whose zero values would lie on
	// every ordinary save, and Door carries total/dc/beaten but is a door.
	//
	// Both arms land HERE, in the same change as the bodies below. An
	// unmapped kind does not fail, it demotes to EVENT_KIND_UNKNOWN at the
	// default arm and its body stays nil, so a cast would reach the client as
	// a beat that happened and could not be read.
	case sdk.EventCast:
		return sessionpb.EventKind_EVENT_KIND_CAST
	case sdk.EventSaved:
		return sessionpb.EventKind_EVENT_KIND_SAVED
	// A concentration broke (rpg-project#407, R10). A DEDICATED KIND rather
	// than something a reader infers from the condition-removed beats that
	// follow it: three of the six reasons a concentration ends produce no
	// check at all, and the removals land on OTHER members' sheets, where an
	// unexplained drop reads as random. This beat is the sentence that makes
	// those removals mean something.
	//
	// Both arms land HERE and in setEventBody below in the same change, for
	// the reason the cast door's did: an unmapped kind demotes to
	// EVENT_KIND_UNKNOWN with a nil body, so a break would arrive as a beat
	// that happened and could not be read.
	case sdk.EventConcentrationEnded:
		return sessionpb.EventKind_EVENT_KIND_CONCENTRATION_ENDED
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
//
// IT RETURNS AN ERROR AS OF rpg-project#458, from setEventBody's one refusing
// arm. The spine above never fails -- it is field-for-field copying and a kind
// lookup that demotes -- so an error here means one beat's BODY carries a
// value this build cannot spell on the wire, and the caller's job is to say so
// rather than to send the beat with that value quietly replaced.
func eventToProto(e sdk.Event) (*sessionpb.Event, error) {
	evt := &sessionpb.Event{
		Session:     e.Session,
		Seq:         e.Seq,
		At:          e.At,
		Correlation: e.Correlation,
		Recipient:   e.Recipient,
		Kind:        eventKindToProto(e.Kind),
		Payload:     e.Payload,
	}
	if err := setEventBody(evt, e.Body); err != nil {
		return nil, fmt.Errorf("event seq %d kind %q: %w", e.Seq, e.Kind, err)
	}
	return evt, nil
}

// eventsToProto mirrors a []sdk.Event slice -- GetStory's own use of the
// same eventToProto StreamEvents sends through one at a time, so catch-up
// and live delivery share one projection all the way to the wire.
//
// ONE BAD BEAT FAILS THE WHOLE READ, deliberately. A catch-up that silently
// dropped the beat it could not spell would hand a client a story with a hole
// in it and no seq gap to notice -- and this read exists precisely so a
// reconnecting client can trust that what it got is what happened.
func eventsToProto(es []sdk.Event) ([]*sessionpb.Event, error) {
	out := make([]*sessionpb.Event, len(es))
	for i, e := range es {
		converted, err := eventToProto(e)
		if err != nil {
			return nil, err
		}
		out[i] = converted
	}
	return out, nil
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
// IT RETURNS AN ERROR AS OF rpg-project#458, and exactly one arm can produce
// one. An `answered` beat's WORD says what the creature did, and this build
// knows two of them; the enum's own comment says it grows a value per slice.
// The day the toolkit ships `alarm` against an api that was not rebuilt, the
// choice here is between a wire value that says "the creature only spoke" --
// a positive, false claim about a creature that in fact ran for the guards --
// and a refusal. It refuses. See answerWordToProto.
func setEventBody(evt *sessionpb.Event, body sdk.EventBody) error {
	switch b := body.(type) {
	case sdk.TurnEndedBody:
		evt.Body = &sessionpb.Event_TurnEnded{TurnEnded: &sessionpb.TurnEnded{Member: b.Member, Next: b.Next}}
	case sdk.DownedBody:
		evt.Body = &sessionpb.Event_Downed{Downed: &sessionpb.Downed{Member: b.Member}}
	case sdk.ExperienceGainedBody:
		evt.Body = &sessionpb.Event_ExperienceGained{ExperienceGained: &sessionpb.ExperienceGained{
			// Member is the CAUSE, not a recipient: the fallen monster whose
			// worth this paid, the same id the downed beat beside it carries.
			// The recipients are in Grants, all of them, on every copy of the
			// beat -- the SDK body is the whole grant rather than one
			// reader's slice of it, so this converter has nothing to filter.
			Member: b.Member,
			Grants: experienceGrantsToProto(b.Grants),
		}}
	case sdk.DeathSaveBody:
		calculation, err := rollCalculationToProto(b.Calculation)
		if err != nil {
			return err
		}
		evt.Body = &sessionpb.Event_DeathSaveRolled{DeathSaveRolled: &sessionpb.DeathSaveRolled{
			Actor: b.Actor, Roll: int32(b.Roll), Outcome: deathSaveOutcomeToProto(b.Outcome),
			SuccessesAdded: int32(b.SuccessesAdded), FailuresAdded: int32(b.FailuresAdded),
			Successes: int32(b.Successes), Failures: int32(b.Failures),
			SuccessesNeeded: int32(b.SuccessesNeeded), FailuresRemaining: int32(b.FailuresRemaining),
			Stabilized: b.Stabilized, Dead: b.Dead, Recovered: b.Recovered,
			HpRestored: int32(b.HPRestored), Continuation: deathSaveContinuationToProto(b.Continuation),
			PresentationId: b.PresentationID,
			Calculation:    calculation,
		}}
	case sdk.StruckBody:
		calculation, err := rollCalculationToProto(b.Calculation)
		if err != nil {
			return err
		}
		damageComponents, err := damageComponentsToProto(b.DamageComponents)
		if err != nil {
			return err
		}
		evt.Body = &sessionpb.Event_Struck{Struck: &sessionpb.Struck{
			Attacker:         b.Attacker,
			Target:           b.Target,
			Roll:             int32(b.Roll),
			Total:            int32(b.Total),
			Against:          int32(b.Against),
			Damage:           int32(b.Damage),
			Attack:           attackRefToProto(b.Attack),
			Critical:         b.Critical,
			DamageComponents: damageComponents,
			// advantage_sources and disadvantage_sources ARE DEPRECATED AND
			// STAY EMPTY (rpg-project#462, R1). They were the older, narrower
			// spelling of what the d20's own keep record now carries, refs and
			// ids with no name and no cancellation; the SDK body dropped them
			// outright. Filling both would give a client two places to read one
			// fact and let them disagree with the dice they describe. The
			// attribution is on Calculation's first component, on the pool it
			// actually decided.
			//
			// Why this swing happened out of turn, when it did
			// (rpg-project#316). The field has been on the wire since
			// protos#258 and had nothing to copy until session's body
			// carried the identity; absent on every ordinary swing, which
			// is the truth rather than a gap.
			Reaction: reactionRefToProto(b.Reaction),
			// Same token the attacker got back on AttackResponse, so this
			// recipient can name the same roll the attacker is presenting.
			PresentationId: b.PresentationID,
			Calculation:    calculation,
		}}
	case sdk.CastMissedBody:
		evt.Body = &sessionpb.Event_CastMissed{CastMissed: &sessionpb.CastMissed{
			Actor: b.Actor, Target: b.Target, Spell: spellRefToProto(b.Spell),
		}}
	case sdk.WardedBody:
		calculation, err := rollCalculationToProto(b.Calculation)
		if err != nil {
			return err
		}
		evt.Body = &sessionpb.Event_Warded{Warded: &sessionpb.Warded{
			Attacker: b.Attacker, Target: b.Target, Attack: attackRefToProto(b.Attack),
			Source: b.Source, Ability: b.Ability, Roll: int32(b.Roll), Total: int32(b.Total),
			Dc: int32(b.DC), Calculation: calculation,
		}}
	case sdk.CastWardedBody:
		calculation, err := rollCalculationToProto(b.Calculation)
		if err != nil {
			return err
		}
		evt.Body = &sessionpb.Event_CastWarded{CastWarded: &sessionpb.CastWarded{
			Actor: b.Actor, Target: b.Target, Spell: spellRefToProto(b.Spell),
			Source: b.Source, Ability: b.Ability, Roll: int32(b.Roll), Total: int32(b.Total),
			Dc: int32(b.DC), Calculation: calculation,
		}}
	case sdk.MissedBody:
		calculation, err := rollCalculationToProto(b.Calculation)
		if err != nil {
			return err
		}
		evt.Body = &sessionpb.Event_Missed{Missed: &sessionpb.Missed{
			Attacker: b.Attacker,
			Target:   b.Target,
			Roll:     int32(b.Roll),
			Total:    int32(b.Total),
			Against:  int32(b.Against),
			Attack:   attackRefToProto(b.Attack),
			Reaction: reactionRefToProto(b.Reaction),
			// See the struck case: one shared token per swing.
			PresentationId: b.PresentationID,
			Calculation:    calculation,
		}}
	case sdk.ActivatedBody:
		evt.Body = &sessionpb.Event_Activated{Activated: activatedBodyToProto(b)}
	case sdk.ActivationResultBody:
		result, err := activationResultBodyToProto(b)
		if err != nil {
			return err
		}
		if result != nil {
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
		// reaches the looter alone, as their own CONCEALMENT_REVEALED.
		evt.Body = &sessionpb.Event_Looted{Looted: &sessionpb.Looted{
			Looter: b.Looter, Body: b.Body,
		}}
	case sdk.SightedBody:
		// PASSED THROUGH, NAMES AND NOTHING ELSE -- and the nothing else is
		// the design rather than an omission this seam should correct. What
		// the recipient now perceives about these members (cell, standing,
		// what is in their hands) is already answered, member-scoped, by
		// GetView; minting it here would be a SECOND computation of that
		// same answer, and two computations of one truth is how a patch and
		// a projection learn to disagree. The client re-reads its view.
		//
		// No assetref minting either, for the same reason. This beat names
		// MEMBERS, not items -- ids the client already holds from its
		// roster -- so there is nothing here in the rules' vocabulary that
		// needs turning into the manifest's.
		//
		// ALL THREE LISTS CROSS THE SAME WAY. `changed` is a peer still in
		// view whose appearance moved under the recipient; it is neither
		// arriving nor leaving, and it is deliberately not accompanied by
		// WHAT changed. Saying a weapon was drawn would hand this recipient
		// a fact rather than the news that their own view is stale, and the
		// fact is exactly what an illusion has to be able to lie about.
		evt.Body = &sessionpb.Event_Sighted{Sighted: &sessionpb.Sighted{
			Gained: b.Gained, Lost: b.Lost, Changed: b.Changed,
		}}
	case sdk.StanceChangedBody:
		// Verbatim (rpg-project#375, design §6): the pair as the session
		// sorted it, and the stance as the author's own word -- a client
		// maps the word to a color the way it maps Ended.ending to a
		// sentence. Reaches every recipient the session addressed it to,
		// monsters included; nothing on this side narrows the audience.
		//
		// THE CAUSE CROSSES WHOLE, AND SO DOES ITS ABSENCE
		// (rpg-api-protos#354). The SDK records WHY a pair turned --
		// "attacked by alice", "the fall of scout", "round 3 started"
		// (rpg-project#493, R2 and R3) -- and the wire now has a `cause`
		// to put it in. It is copied, never composed: the sentence is the
		// composition's, the way the stance is the author's word, so a
		// phrase written here would be this layer narrating.
		//
		// EMPTY IS COPIED TOO, and deliberately not filled in. A pair also
		// turns because a faction's mind came to know the fact the author
		// named -- the hold-out beat -- and that fold writes no sentence,
		// because nobody decided one. Substituting a default here would
		// hand every client a reason no author wrote, and the emptiness is
		// what tells a reader to say only that the pair turned.
		evt.Body = &sessionpb.Event_StanceChanged{StanceChanged: &sessionpb.StanceChanged{
			Between: b.Between, Stance: b.Stance, Cause: b.Cause,
		}}
	case sdk.ArrivedBody:
		// A reserved placement entered the run (rpg-project#375 step B,
		// design §6): which one, what it is, and where it stands now.
		// Physical state like HELD/DROPPED, so every recipient hears it
		// and patches its map additively. The kind is a CLOSED enum on
		// the wire because a client branches on it -- a prop is not a
		// member -- and it maps by name, never by position.
		evt.Body = &sessionpb.Event_Arrived{Arrived: &sessionpb.Arrived{
			Id: b.ID, Kind: placementKindToProto(b.Kind), Cell: positionToProto(b.Cell),
		}}
	case sdk.HeldBody:
		// To everyone present: an object leaving the floor folds on the
		// TRUTH GRAIN, so every recipient's atlas loses the prop and a
		// client patches its cached map by removing this id -- the
		// load-once, beat-refreshed law running subtractively, where
		// CONCEALMENT_REVEALED runs it additively.
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
	case sdk.IntimidatedBody:
		// A threat, landed or missed (rpg-project#454). VERBATIM, and
		// BEATEN IS COPIED rather than derived here from total against dc --
		// the law Saved.succeeded and DoorChanged.beaten already keep, for
		// the same reason: the day a rule changes what beating a DC means,
		// every reader that derived it would be wrong at once.
		//
		// EVERY FIELD CROSSES ON A MISS TOO. The SDK writes beaten, dc and
		// total with no omitempty precisely because `beaten: false` is the
		// whole content of a missed threat, and this seam must not
		// reintroduce the absence its author removed.
		//
		// NOTHING ABOUT THE CONSEQUENCE, and the wire has no field for one.
		// A beaten threat lands a deed on the witnesses and what it is worth
		// is the threatened creature's mind's to decide; a client narrates
		// that from the creature's next turn. Deliberately unlike
		// DoorChanged below, which can report the state its own check
		// produced -- there is no state here to report yet.
		//
		// THE WHOLE ROLL RIDES WITH THE VERDICT (rpg-project#462). dc, total
		// and beaten are the outcome; the calculation is what was thrown to
		// get there -- both d20 faces when a rule decided between them, and
		// the keep record naming the rule and who brought it. An untrained
		// character threw two dice and this seam used to publish one number,
		// so the house rule shipped applied and invisible. It is ABSENT on a
		// beat that recorded no arithmetic, which is the truth rather than a
		// zero-valued calculation a reader would have to tell apart.
		calculation, err := rollCalculationToProto(b.Calculation)
		if err != nil {
			return err
		}
		evt.Body = &sessionpb.Event_Intimidated{Intimidated: &sessionpb.Intimidated{
			Actor:       b.Actor,
			Target:      b.Target,
			Dc:          int32(b.DC),
			Total:       int32(b.Total),
			Beaten:      b.Beaten,
			Calculation: calculation,
		}}
	case sdk.PersuadedBody:
		// An appeal, landed or missed (rpg-project#458). The threat body's
		// twin, field for field and law for law -- beaten is COPIED and never
		// derived from total against dc, and every field crosses on a miss
		// too, because `beaten: false` is the whole content of a failed
		// appeal and the `persuade_failed` table fired on it.
		//
		// A SEPARATE ARM RATHER THAN ONE SHARED "SOCIAL CHECK" BODY, which is
		// the wire's own decision (rpg-api-protos#340) and not this
		// converter's to relitigate: a consumer switching on Event.body gets a
		// typed verb out of the arm it matched, and a shared body would make
		// it branch twice -- once to find the arm, once to read a
		// discriminator.
		//
		// It carries the whole roll for the threat body's reason, above.
		calculation, err := rollCalculationToProto(b.Calculation)
		if err != nil {
			return err
		}
		evt.Body = &sessionpb.Event_Persuaded{Persuaded: &sessionpb.Persuaded{
			Actor:       b.Actor,
			Target:      b.Target,
			Dc:          int32(b.DC),
			Total:       int32(b.Total),
			Beaten:      b.Beaten,
			Calculation: calculation,
		}}
	case sdk.AnsweredBody:
		answered, err := answeredToProto(b)
		if err != nil {
			return err
		}
		evt.Body = &sessionpb.Event_Answered{Answered: answered}
	case sdk.TemperedBody:
		// WHICH TEMPERAMENT A FACTION'S MIX DEALT ONE CREATURE, at the door
		// (rpg-project#465 §3, R5). Four goblins handed one table and one mix
		// answer it four different ways, and this is the beat that lets the
		// table see which one came out the coward.
		//
		// THE DIE NAMES ITS ENTITY, as every pool on this seam does: the mix
		// belongs to the FACTION, so the faction threw, and `member` is who
		// the word landed on. Both cross; neither is derived from the other.
		//
		// AN AUTHORED `temper:` RAISES NO BEAT AT ALL. Nothing was rolled, so
		// there is nothing to show -- that creature's word still reaches every
		// reader on Answered.temper, on every pick it makes.
		temper, err := temperToProto(b.Temper)
		if err != nil {
			return err
		}
		evt.Body = &sessionpb.Event_Tempered{Tempered: &sessionpb.Tempered{
			Member:  b.Member,
			Temper:  temper,
			Roll:    int32(b.Roll),
			Of:      int32(b.Of),
			Faction: b.Faction,
		}}
	case sdk.StayedBody:
		// THE WHOLE ACCOUNT OF A SPENT ROUND IN WHICH NOTHING MOVED
		// (rpg-project#465). The creature was sent somewhere -- by its own
		// table, or by whatever else routes a walk -- and the route came back
		// with no path, so the round ended on the cell it started on.
		//
		// EVERY FIELD CROSSES VERBATIM AND NOTHING IS DERIVED. `cause` is the
		// engine's own `<module>:<type>:<id>` reference, so a creature walking
		// under its own orders is distinguishable from one being shoved or
		// commanded, and this seam neither parses it nor decides anything from
		// it.
		//
		// AN EMPTY `why` IS AN ANSWER AND IS SENT AS ONE. The route had
		// nowhere strictly nearer to offer, which is its own reason rather
		// than a blocker it could name, and it is the commonest case. Nothing
		// here substitutes a sentence for it: composing "no path" on this side
		// would be the api narrating, and a client reading empty as "the
		// producer forgot" would be reading a fact as a defect.
		//
		// NO CELLS, because nothing moved. Where the creature stands is what
		// the roster and the atlas already answer, and a position here would
		// be a second copy of a fact this beat did not change.
		evt.Body = &sessionpb.Event_Stayed{Stayed: &sessionpb.Stayed{
			Member: b.Member,
			Cause:  b.Cause,
			Why:    b.Why,
		}}
	default:
		return setWorldEventBody(evt, body)
	}
	return nil
}

// setWorldEventBody carries the second half of the same type switch: the
// world's own beats -- doors, regions, windows -- and the spell beats that
// close a cast.
//
// THE SPLIT IS A COMPLEXITY BOUNDARY AND NOTHING ELSE. One switch over every
// body kind outgrew the cyclomatic limit the day the creature's table and the
// ward slice each brought their own arms, and no arm changed in the move. A
// body kind is recognized here or in setEventBody and never in both, and one
// this build does not recognize still leaves evt.Body nil so the payload
// stays the passthrough carrier.
func setWorldEventBody(evt *sessionpb.Event, body sdk.EventBody) error {
	switch b := body.(type) {
	case sdk.DoorBody:
		// A DOOR THAT CHANGED BECAUSE SOMEBODY ROLLED is a check beat and
		// carries the whole roll (rpg-project#462, R4). A door that opened
		// because somebody walked through it recorded no arithmetic and
		// carries none -- the SDK body leaves Calculation nil on every
		// non-attempt beat and the wire field stays unset, exactly as dc,
		// total and actor already do there.
		calculation, err := rollCalculationToProto(b.Calculation)
		if err != nil {
			return err
		}
		evt.Body = &sessionpb.Event_Door{Door: &sessionpb.DoorChanged{
			Door:        b.Door,
			State:       doorStateToProto(b.State),
			Actor:       b.Actor,
			Dc:          int32(b.DC),
			Total:       int32(b.Total),
			Beaten:      b.Beaten,
			Calculation: calculation,
		}}
	// ONE NOUN, ONE REVEAL (design rpg-project#490, E4). What used to arrive
	// as DoorRevealedBody and RegionRevealedBody -- two kinds, two payloads,
	// one authored secret -- is one concealment now: the cells it hid, the
	// props standing on them, the doors it hid with their own edges, the
	// boundaries and walls at the revealed seams, the sealed cells inside it,
	// and every region it touched. The engine hid doors and regions with two
	// separate flags and so split one secret across two beats; it hides a
	// concealment with one primitive and sends one. The SDK produces neither
	// older body any more, so neither is converted here; the two wire kinds
	// stay on the proto, deprecated, and retire when no client reads them.
	//
	// EVERY FIELD IS CARRIED VERBATIM. The composition derives this body from
	// the recipient's own Atlas and Doors answers, so the patch and the map
	// cannot disagree -- and this converter's whole contribution to that is
	// not getting in the way.
	case sdk.ConcealmentRevealedBody:
		// DOORS AND DOORWAYS SPLIT ON THE WIRE where the SDK nests them. The
		// body hangs each door's edges off the door; the proto carries one
		// flat door list and one flat doorway list, which are the two shapes
		// GetDoors and GetAtlas already answer in and therefore the two
		// caches a client patches. Nothing is lost in the flatten: every
		// AtlasDoorway names its own door (AtlasDoorway.connection), so the
		// grouping is reconstructible from the flat list. A footprint door
		// contributes none -- it stands in no crossing (rpg-project#485) --
		// and that is an honest empty, not a dropped field.
		doors := make([]*sessionpb.DoorInfo, len(b.Doors))
		doorways := make([]sdk.AtlasDoorway, 0, len(b.Doors))
		for i, d := range b.Doors {
			doors[i] = revealedDoorToProto(d)
			doorways = append(doorways, d.Doorways...)
		}

		// Cells adds; the rest are three different shapes of thing and a
		// client that treats them alike will draw the wrong room. All of them
		// are carried verbatim; none is recomputed here.
		//
		// SEGMENTS IS A DIFFERENCE, and adds: the walls this recipient did not
		// have and now does. A wall already presented to them for any reason --
		// the seam a concealed door hides in, or one footing on floor they can
		// already see -- is deliberately absent, because it is not news and
		// they are already drawing it. So this is append-to-cache, never
		// replace. It HAS to be a difference: a segment carries no footprint on
		// purpose, so there is no way to ask which cells a wall stands on
		// without leaking what the doorway list withholds. No wall ever leaves,
		// so the atlas after a reveal is the atlas before it union this.
		//
		// SEALED IS SCOPED, AND REPLACES, and the scoping is load-bearing rather
		// than a style choice: a client swaps out the arriving cells' sealed
		// entries and keeps every other sealed cell it had. Cells LEAVE this
		// list. A non-knower's sealed list already holds some of the hidden
		// room's own cells -- the footing of the walls presented to them (design
		// C18), which reaches them as ownerless floor, and ownerless floor is
		// floor nobody stands on -- and the moment the secret is theirs those
		// same cells are ordinary standable floor. A client that appended would
		// leave a room it can see permanently unwalkable at its edges. So the
		// atlas after a reveal is (the atlas before it, less the concealment's
		// cells) union this, which is why the beat carries those cells beside
		// it. A difference could only ever add, and this field has to be able
		// to take away.
		//
		// REGIONS REPLACES ENTRY BY ENTRY, for a reason of its own: what an
		// unaware recipient held was not a shorter LIST, it was a shorter
		// REGION. A region is a name over a set of cells, and a concealment's
		// cells cannot appear in it without naming the secret, so a touched
		// region reached them trimmed, or did not reach them at all. Merging
		// would leave the trim sitting beside the truth. Each entry arrives
		// whole, archetype and lighting with it, which is how a room withheld
		// entirely gets dressed and lit the moment it arrives.
		cells := make([]*sessionpb.Position, len(b.Cells))
		for i, c := range b.Cells {
			cells[i] = positionToProto(c)
		}
		sealed := make([]*sessionpb.Position, len(b.Sealed))
		for i, c := range b.Sealed {
			sealed[i] = positionToProto(c)
		}
		evt.Body = &sessionpb.Event_ConcealmentRevealed{ConcealmentRevealed: &sessionpb.ConcealmentRevealed{
			Concealment: b.Concealment,
			Cells:       cells,
			Props:       atlasPropsToProto(b.Props),
			Doors:       doors,
			Doorways:    atlasDoorwaysToProto(doorways),
			Boundaries:  atlasBoundariesToProto(b.Boundaries),
			Segments:    atlasSegmentsToProto(b.Segments),
			Sealed:      sealed,
			Regions:     atlasRegionsToProto(b.Regions),
		}}
	case sdk.WindowOpenedBody:
		// The fight stopped to ask somebody something (rpg-project#316 rung
		// 3). Everything a client needs to draw the pause: whose step it
		// was, the two cells it stopped between -- the mover is standing on
		// From, because the step is announced and NOT taken -- who is being
		// asked, and what they are being asked to react with.
		//
		// AUDIENCE IS A LIST AND REACTION IS NOT, verbatim from the SDK.
		// One step asks every player reactor at once (ruling R3) and today
		// exactly one reaction can reach a movement fold, so the asymmetry
		// is the SDK's own and this converter neither flattens nor fans it
		// out. Nothing here says which OPTIONS were posed: strike and hold
		// are implied by the verb, and the answer travels as ReactChoice.
		//
		// Reaction is a value on this body, not a pointer as it is on
		// Struck/Missed -- a window that named no reaction could not have
		// been posed -- so it always converts to a non-nil message.
		evt.Body = &sessionpb.Event_WindowOpened{WindowOpened: &sessionpb.WindowOpened{
			Audience: b.Audience,
			Mover:    b.Mover,
			From:     positionToProto(b.From),
			To:       positionToProto(b.To),
			Reaction: reactionRefToProto(&b.Reaction),
		}}
	case sdk.CastBody:
		// Targets is canonical and request ordered. The deprecated scalar is
		// written only when it is a faithful projection of exactly one target;
		// a multi-target cast never invents a representative.
		targets := append([]string(nil), b.Targets...)
		legacyTarget := ""
		if len(targets) == 1 {
			legacyTarget = targets[0]
		}
		evt.Body = &sessionpb.Event_Cast{Cast: &sessionpb.Cast{
			Actor:   b.Actor,
			Spell:   spellRefToProto(b.Spell),
			Target:  legacyTarget, //nolint:staticcheck // Faithful compatibility projection for exactly one target.
			Targets: targets,
		}}
	case sdk.SavedBody:
		calculation, err := rollCalculationToProto(b.Calculation)
		if err != nil {
			return err
		}
		// The whole of one saving throw. SUCCEEDED IS COPIED, never derived
		// here from total against dc -- the rulebook classifies its own roll,
		// the law DeathSaveRolled.outcome already keeps, so the day beating a
		// DC means something new every reader is not wrong at once.
		evt.Body = &sessionpb.Event_Saved{Saved: &sessionpb.Saved{
			Saver:       b.Saver,
			Ability:     b.Ability,
			Roll:        int32(b.Roll),
			Total:       int32(b.Total),
			Dc:          int32(b.DC),
			Succeeded:   b.Succeeded,
			Source:      spellRefToProto(b.Source),
			Calculation: calculation,
		}}
	case sdk.ConcentrationEndedBody:
		// Who lost what, and why. THE REASON IS AN OPEN STRING and this
		// converter copies it verbatim -- the vocabulary is the rulebook's
		// ("damage", "recast", "duration", "combat_end", "spell_ended",
		// "caster_down") and it grows with the rulebook, so a closed set here
		// would have to be widened in three modules every time a spell learns
		// a new way to end.
		//
		// NO SAVE AND NO REMOVALS ride this body, exactly as the SDK's own
		// shape has none. The failed check travels beside it as EventSaved
		// and each stripped condition as its own ActivationResult beat, in
		// one train from one interaction; repeating them here would give a
		// client two places to read one fact.
		evt.Body = &sessionpb.Event_ConcentrationEnded{ConcentrationEnded: &sessionpb.ConcentrationEnded{
			Caster: b.Caster,
			Spell:  spellRefToProto(b.Spell),
			Reason: b.Reason,
		}}
	case sdk.RollWindowOpenedBody:
		// A roll stopped to ask (rpg-project#398). The d20 is already on the
		// table and the fight is waiting on the one member who rolled it.
		//
		// AUDIENCE IS A SINGLE MEMBER HERE, not a list as it is above, and
		// the asymmetry is the SDK's own: a movement fold asks every player
		// reactor at once, while this slice poses one window to the roller
		// and to nobody else. An offer whose audience is not the roller is
		// refused below the seam rather than posed to somebody no freeze was
		// designed for, so this converter never sees a second name.
		//
		// THE TARGET'S AC IS NOT ON THIS BEAT and there is no field for it.
		// Roll and total are what the player decides with; whether the swing
		// lands is what they are deciding about, and the struck or missed
		// beat says it AFTER the answer.
		//
		// Offer is a value on this body, not a pointer -- a window that
		// named nothing to spend could not have been posed -- so it always
		// converts to a non-nil message.
		//
		// THE PAUSED WINDOW IS WHERE AN UNTRAINED ROLL IS FIRST SEEN
		// (rpg-project#462, R5). calculation has been on the wire since the
		// window shipped and had nothing to copy, because the SDK body had no
		// field for it; it does now. Roll and Total are the two scalars the
		// player decides with, and the calculation is the pair of faces they
		// are deciding ABOUT -- a player asked to spend a Bardic Inspiration
		// die on a roll they can only see one face of is being asked blind.
		calculation, err := rollCalculationToProto(b.Calculation)
		if err != nil {
			return err
		}
		evt.Body = &sessionpb.Event_RollWindowOpened{RollWindowOpened: &sessionpb.RollWindowOpened{
			PresentationId: b.PresentationID,
			Audience:       b.Audience,
			Offer:          reactionRefToProto(&b.Offer),
			Roll:           int32(b.Roll),
			Total:          int32(b.Total),
			Calculation:    calculation,
		}}
	default:
		// nil (no typed body for this kind) or a body type this build does
		// not recognize: leave evt.Body nil. payload stays the passthrough
		// carrier.
	}
	return nil
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
func activationResultBodyToProto(body sdk.ActivationResultBody) (*sessionpb.ActivationResult, error) {
	result := &sessionpb.ActivationResult{Actor: body.Actor}
	populated := 0
	if body.HealingApplied != nil {
		populated++
		healing, err := healingAppliedBodyToProto(body.HealingApplied)
		if err != nil {
			return nil, err
		}
		result.Result = &sessionpb.ActivationResult_HealingApplied{HealingApplied: healing}
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
	// Damage is a RESULT ARM rather than a beat of its own (rpg-project#405
	// R7): Vicious Mockery's 1d4 is a thing an effect delivered, exactly like
	// the condition beside it, and ActivationResult already carries delivered
	// effects. It counts toward the one-arm invariant like every other arm,
	// so a malformed body with two results still produces no wire body at all.
	if body.DamageApplied != nil {
		populated++
		damage, err := damageAppliedBodyToProto(body.DamageApplied)
		if err != nil {
			return nil, err
		}
		result.Result = &sessionpb.ActivationResult_DamageApplied{DamageApplied: damage}
	}
	// A creature the effect MOVED is a result arm for the same reason damage
	// is one: a push is a thing an effect delivered, and ActivationResult is
	// already where delivered effects are read. It is deliberately NOT the
	// movement -- every cell crossed is its own beat carrying the cause, and
	// those are what a client animates. This is the one line saying how far
	// and what stopped it. It counts toward the one-arm invariant like every
	// other arm.
	if body.MoveImposed != nil {
		populated++
		result.Result = &sessionpb.ActivationResult_MoveImposed{
			MoveImposed: moveImposedBodyToProto(body.MoveImposed),
		}
	}
	if body.Stabilized != nil {
		populated++
		b := body.Stabilized
		result.Result = &sessionpb.ActivationResult_Stabilized{
			Stabilized: &sessionpb.Stabilized{
				Target: b.Target, SourceRef: b.SourceRef, SourceName: b.SourceName,
				Before: lifeStateToProto(b.Before), After: lifeStateToProto(b.After),
				HitPoints: int32(b.HitPoints), Progress: deathSaveProgressToProto(&b.Progress),
			},
		}
	}
	if populated != 1 {
		return nil, nil
	}
	return result, nil
}

// moveImposedBodyToProto mirrors an imposed move field-for-field.
//
// The wire message is narrower than the SDK body: SourceRef and SourceName
// have no field on it, so what moved the creature does not cross here. That is
// a gap in the contract rather than something this converter may paper over --
// deriving a name from the ref, or borrowing the enclosing cast's, would be
// inventing a fact the provider authored elsewhere. Recorded on the PR.
func moveImposedBodyToProto(body *sdk.MoveImposedBody) *sessionpb.MoveImposed {
	if body == nil {
		return nil
	}

	return &sessionpb.MoveImposed{
		Target: body.Target,
		// ALWAYS WRITTEN, zero included. A creature pinned against a wall is
		// pushed nowhere, and that is an outcome the caster is owed. proto3
		// leaves the field off the wire at zero, which is exactly why the ARM
		// has to be present: its presence is what says a push happened.
		MovedCells: int32(body.MovedCells),
		StoppedBy:  body.StoppedBy,
	}
}

// healingAppliedBodyToProto mirrors a heal onto the wire without deriving one
// representation from the other. New bodies carry Calculation only; legacy
// Story records retain their deprecated Roll and Modifier scalars.
func healingAppliedBodyToProto(body *sdk.HealingAppliedBody) (*sessionpb.HealingApplied, error) {
	if body == nil {
		return nil, errors.New("healing applied body is required")
	}
	out := &sessionpb.HealingApplied{
		Target: body.Target, Amount: int32(body.Amount), Requested: int32(body.Requested),
		SourceRef: body.SourceRef, SourceName: body.SourceName,
		HpBefore: int32(body.HPBefore), HpAfter: int32(body.HPAfter),
	}
	if body.Calculation != nil {
		// New bodies populate only Calculation. Its total is authoritative;
		// neither Requested nor the deprecated scalars are derived from it.
		calculation, err := rollCalculationToProto(body.Calculation)
		if err != nil {
			return nil, err
		}
		out.Calculation = calculation
	} else {
		// Legacy bodies retain exactly the two deprecated scalar fields and do
		// not gain a fabricated calculation.
		out.Roll = int32(body.Roll)         //nolint:staticcheck // Required read compatibility for pre-trace Story records.
		out.Modifier = int32(body.Modifier) //nolint:staticcheck // Required read compatibility for pre-trace Story records.
	}
	return out, nil
}

// damageAppliedBodyToProto mirrors damage HealingApplied's way, and the
// asymmetry between them is deliberate: a heal carries deprecated Roll and
// Modifier scalars for Story records written before roll traces existed, and
// NOTHING EVER WROTE A DAMAGE RESULT before them, so there is no legacy shape
// to read and Calculation is the only representation of the dice.
func damageAppliedBodyToProto(body *sdk.DamageAppliedBody) (*sessionpb.DamageApplied, error) {
	if body == nil {
		return nil, errors.New("damage applied body is required")
	}

	calculation, err := rollCalculationToProto(body.Calculation)
	if err != nil {
		return nil, err
	}

	return &sessionpb.DamageApplied{
		Target:     body.Target,
		Amount:     int32(body.Amount),
		Requested:  int32(body.Requested),
		DamageType: damageTypeToProto(body.DamageType),
		SourceRef:  body.SourceRef,
		SourceName: body.SourceName,
		HpBefore:   int32(body.HPBefore),
		HpAfter:    int32(body.HPAfter),
		// The 1d4's own face, so a client can show the roll rather than only
		// what it totalled.
		Calculation: calculation,
		// COPIED FROM THE BODY, never derived from SourceRef. The rulebook
		// authors what kind of damage a spell deals -- psychic, for Vicious
		// Mockery -- and a client that read the spell's ref to decide would
		// be deriving 5e, which is the whole thing content refs prevent. The
		// same converter the strike path's components already run through.
	}, nil
}

// spellRefToProto mirrors AbilityRef's shape one content type over: the full
// ref for correlation and an icon table, and a name the content authored.
// A reader never derives the name from the ref, and never branches on the ref
// to decide what a spell does.
func spellRefToProto(s sdk.SpellRef) *sessionpb.SpellRef {
	return &sessionpb.SpellRef{Ref: s.Ref, Name: s.Name}
}

// conditionAppliedBodyToProto mirrors the attach beat, SOURCE INCLUDED.
//
// Target plus ref is not the condition's address -- the source is the third
// part of it. Two casters can each land Bane on the same fighter, and the
// rulebook keeps those apart; a beat that named only the target and the ref
// would collapse them, so a client could not say whose Command holds a
// creature, and a later removal beat would have two rows it might mean.
//
// The source is copied and never inferred. ActivationResultBody.Actor is who
// ACTED and SourceID is who the condition answers to; they are the same id
// often enough that filling one from the other would look right for a long
// time, and would then blame a trap's condition on whoever was standing there.
// An unattributed condition stays unattributed.
func conditionAppliedBodyToProto(body *sdk.ConditionAppliedBody) *sessionpb.ConditionApplied {
	if body == nil {
		return nil
	}
	return &sessionpb.ConditionApplied{
		Target: body.Target, Ref: body.Ref, Name: body.Name, SourceId: body.SourceID,
	}
}

// conditionRemovedBodyToProto mirrors the detach beat, and carries the source
// for the reason its twin above does: WHICH instance ended is the target, the
// ref and the caster together. A client holding two Banes on one fighter
// strikes the wrong row without it.
func conditionRemovedBodyToProto(body *sdk.ConditionRemovedBody) *sessionpb.ConditionRemoved {
	if body == nil {
		return nil
	}
	return &sessionpb.ConditionRemoved{
		Target: body.Target, Ref: body.Ref, Name: body.Name, Reason: body.Reason,
		SourceId: body.SourceID,
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
// # Two lists of things, because there are two kinds of thing
//
// `props` are the things standing on A CELL, naming content they draw as.
// `placed` are the authored FOOTPRINTS -- rectangles drawn at an angle, big
// enough to stand on several cells or small enough to stand on none of their
// centers, naming no content at all (rpg-api-protos#351). The two never share
// an id and neither is derivable from the other. A placement carries the cells
// the engine says it stands on, and this layer copies that list rather than
// tracing the rectangle a second time; see atlasPlacedPropToProto.
//
// Exported because it has two callers that MUST agree: GetAtlas, and the
// AuthoringService's PutDungeon, whose answer is the same message so the
// builder has no second geometry to keep in step with the game
// (rpg-project#256, design §3a).
//
// Regions (GetAtlasResponse.regions) are copied cell for cell: they are
// already absolute axial in the same frame as Cells, so nothing here converts
// anything — the one place cells become axial is the toolkit's.
//
// # The map names its dungeon; it no longer carries the room's picture
//
// `dungeon_key` is the content key this world was compiled from, copied
// across verbatim. `room_scene_json` is deprecated and DELIBERATELY LEFT
// UNSET (rpg-project#479): what a room looks like is the World Builder's
// content, and a client that wants it fetches the authored file by this key
// through the ungated AuthoringService.GetDungeon and reads the scene with
// the codec that owns one. The engine stopped carrying that document at
// encounter v0.93.0, so there is no longer anything on the atlas to copy
// into the old field -- and a field nothing can fill is left empty rather
// than filled with something invented here.
//
// An empty key is the honest absence, not a default: it means the session was
// launched from a world its host assembled rather than from a registry entry,
// which is what every session written before the key existed is. A client
// seeing it empty fetches nothing and draws what the map alone says.
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
		Start:      atlasStartToProto(a.Start),
		DungeonKey: a.DungeonKey,
		// The authored rectangles standing on the map, beside -- never
		// inside -- the cell props above. A client draws a placement from
		// this list rather than from the World Builder's own scene bytes,
		// because the scene is the author's whole drawing and says nothing
		// about the run: whether the thing has arrived, whether somebody is
		// already carrying it, or whether this member can see where it
		// stands.
		Placed: atlasPlacedPropsToProto(a.Placed),
	}
}

// atlasStartToProto mirrors where the party came in and which way they were
// looking (rpg-project#374), or NOTHING when the dungeon declares none.
//
// # Absence is a third answer, not a zero value
//
// A nil start is not "the party arrives at [0,0] facing nowhere" -- that is a
// real dungeon somebody could author, and a zero-valued message here would be
// indistinguishable from it. Both sides of this conversion spell absence with
// a pointer for exactly that reason, so the honest translation of nil is an
// omitted field.
//
// It is reachable, and not only from authoring: dungeonspec REFUSES a file
// with no `start:` ("the dungeon does not say where the party starts"), so no
// authored dungeon lands here nil. What does is a STORED ENCOUNTER written
// before starts were carried -- the toolkit made its own StartData a pointer
// with omitempty precisely so such a blob loads with none. Every session
// already in Redis is one, which makes this the first thing to break on the
// next deploy if it were translated as a zero.
//
// # The facing is a word, carried verbatim
//
// One of eight true-compass names, or empty when the author stated none.
// Empty is a FACT here rather than a gap -- it means open the camera however
// it opened before -- and nothing in this function invents, defaults or
// validates the word. The vocabulary is the authoring dialect's to check and
// the client's to turn into an angle; a second check here would be a second
// place it could drift from the first.
func atlasStartToProto(start *sdk.AtlasStart) *sessionpb.AtlasStart {
	if start == nil {
		return nil
	}

	return &sessionpb.AtlasStart{
		At:     positionToProto(start.At),
		Facing: start.Facing,
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
// ConcealmentRevealed's props, which the SDK's own doc promises carries them
// "exactly as GetAtlasResponse.props would" (rpg-project#350/#351, #490).
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

// atlasPlacedPropToProto mirrors one session.AtlasPlacedProp -- a rectangle
// somebody drew on the map, the two blocking answers its author gave it,
// whether it can be picked up, and the cells the engine says it stands on
// (rpg-api-protos#351).
//
// A DIFFERENT KIND OF THING FROM AtlasProp, not a better one. A prop occupies
// A CELL and names content it draws as; a placement occupies AN AREA and names
// no content at all -- it is the geometry a door, a table or a bookcase was
// drawn as, and the World Builder owns what it looks like. The two lists never
// share an id, so a client may key them together.
//
// # Absence is the whole vocabulary, and this converter adds no flag to it
//
// A placement is off this list when it is still in reserve, when somebody is
// holding it (for everyone at once -- a thing leaving the floor is not a
// secret), and when this recipient cannot see the floor it stands on. All
// three are the SDK's answers, decided before the list reaches here, and this
// layer copies the list it was given. A flag saying "held" or "hidden" would
// be a second answer to a question absence already answers, and a client
// drawing flagged entries would draw furniture nobody can reach.
//
// # Cells is copied, never re-measured
//
// Standing is a trace of the rectangle against every cell of the floor, and
// it is the SAME SET Hold's reach judges. A seam that rasterized the box a
// second time here would be a second geometry one layer before the client's,
// disagreeing at exactly the edges that matter -- where a table's corner
// clips a hex -- and an offer this list produced could then contradict the
// refusal the engine gives.
func atlasPlacedPropToProto(p sdk.AtlasPlacedProp) *sessionpb.AtlasPlacedProp {
	cells := make([]*sessionpb.Position, len(p.Cells))
	for i, c := range p.Cells {
		cells[i] = positionToProto(c)
	}

	return &sessionpb.AtlasPlacedProp{
		Id:                p.ID,
		Placement:         footprintPlacementToProto(p.Placement),
		BlocksMovement:    p.BlocksMovement,
		BlocksLineOfSight: p.BlocksLineOfSight,
		Holdable:          p.Holdable,
		Cells:             cells,
	}
}

func atlasPlacedPropsToProto(ps []sdk.AtlasPlacedProp) []*sessionpb.AtlasPlacedProp {
	out := make([]*sessionpb.AtlasPlacedProp, len(ps))
	for i, p := range ps {
		out[i] = atlasPlacedPropToProto(p)
	}

	return out
}

// footprintPlacementToProto mirrors session.FootprintPlacement field for
// field: the rectangle's size in feet, where it is anchored, which way it
// faces, and how far it is nudged inside its own axes.
//
// NOTHING IS SWAPPED, SNAPPED OR SCALED HERE. Width lies ACROSS the facing and
// Depth ALONG it; the authored dialect performs that name swap at the
// construction boundary, long before this seam, and re-swapping it here would
// draw every door turned ninety degrees. Facing is degrees from east and any
// finite angle is legal -- zero is due east, a real facing, never "not
// authored".
//
// The two points are always written, because the source carries VALUES and a
// value type has no absence to translate. A zero origin is a real anchor and a
// zero local offset is the pose most placements hold (the rectangle centered on
// its origin), so an omitted message here would spell a fact the SDK never
// said.
func footprintPlacementToProto(p sdk.FootprintPlacement) *sessionpb.FootprintPlacement {
	return &sessionpb.FootprintPlacement{
		Width:       p.Width,
		Depth:       p.Depth,
		Origin:      footprintPointToProto(p.Origin),
		Facing:      p.Facing,
		LocalOffset: footprintPointToProto(p.LocalOffset),
	}
}

// footprintPointToProto mirrors session.FootprintPoint: one point on the
// map's continuous plane, IN FEET.
//
// DELIBERATELY NOT positionToProto's target. A Position is a cell coordinate
// in the atlas's own frame, where 1 means one cell along an axis; these are
// feet on the plane the engine traces rectangles in. The same spot carries
// different numbers in the two frames and neither converts without the
// layout, so sending one where the other belongs puts a table five times too
// far out. The wire mints a separate message for exactly that reason, and
// this converter is the only thing that fills it.
func footprintPointToProto(p sdk.FootprintPoint) *sessionpb.FootprintPoint {
	return &sessionpb.FootprintPoint{X: p.X, Y: p.Y}
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
	case sdk.VerbReact:
		return sessionpb.Verb_VERB_REACT
	// VerbCast (rpg-project#405). Afford compiles ONE ROW PER CASTABLE
	// CANTRIP -- many per member, the way VerbActivate arrives -- so leaving
	// it unmapped would label every Cast row a bard can reach
	// VERB_UNSPECIFIED, and the dock drops a verb it cannot name rather than
	// showing it wrong. That is the whole panel this slice exists to fill.
	case sdk.VerbCast:
		return sessionpb.Verb_VERB_CAST
	// VerbIntimidate (rpg-project#454). LOAD-BEARING FOR THE DOCK, not a
	// completeness sweep: leaving it unmapped would label every threat a
	// member can make VERB_UNSPECIFIED -- and a client drops a verb it cannot
	// name rather than showing it wrong, so the row would simply never appear.
	//
	// ON BOTH CLOCKS AS OF rpg-project#458 R3, which corrects what this
	// comment used to say. Afford emitted this row on the TURN clock
	// unconditionally, the way it emits VerbMove, and returned an empty list
	// on the world clock. It now emits both social rows on the world clock
	// too, at no cost -- not a discount, but Move's own rule: the world clock
	// has no economy to fall short of. So this arm is what a player standing
	// in a front room with no fight in it reads.
	case sdk.VerbIntimidate:
		return sessionpb.Verb_VERB_INTIMIDATE
	// VerbPersuade (rpg-project#458). Intimidate's reason, and one more that
	// is new with it: Afford emits BOTH social rows on the WORLD clock as well
	// as the turn clock (R3), so this is the row a player sees standing in a
	// front room where no fight exists. Unmapped, the one panel this slice
	// exists to fill would come up empty.
	case sdk.VerbPersuade:
		return sessionpb.Verb_VERB_PERSUADE
	default:
		return sessionpb.Verb_VERB_UNSPECIFIED
	}
}

// answeredToProto projects one Answered beat: the world's own roll on the
// author's table, with every number it was made of.
//
// SPLIT OUT OF setEventBody, which is one arm per body and nothing else. This
// arm grew three refusing conversions and a conditional pair with the creature's
// table (rpg-project#465), and a switch that big stops being readable as a list
// of bodies — which is the one thing it has to stay.
func answeredToProto(b sdk.AnsweredBody) (*sessionpb.Answered, error) {
	// THE WORLD'S OWN ROLL ON THE AUTHOR'S TABLE (rpg-project#458 R1,
	// rpg-project#465 §6). Two things reach this one body: a social
	// verdict's answer -- the player's check published `intimidated` or
	// `persuaded`, and then the world rolled -- and a creature spending
	// one turn's worth of doing, on its turn in a fight or a round of the
	// world clock. One table, one roll, one shape; `key` says which.
	//
	// EVERY FIELD CROSSES, none of them derived. `entry: 0` is an answer
	// (the author's first line fired), `beaten: false` is an answer (the
	// failure table was read), and an empty `say` is an answer (the author
	// wrote the creature no line). The SDK writes all of them without
	// omitempty for exactly that reason and this seam must not reintroduce
	// the absence its author removed.
	//
	// THE ARITHMETIC RIDES ALONG, NOT THE RESULT ALONE. `candidates`
	// carries every entry whose `when` held with its authored weight, its
	// temperament factor and their product, so a debug log can redo the
	// engine's sum rather than take `of` on trust. Empty is a real answer:
	// nothing was eligible, the creature held, and the beat says so.
	word, err := answerWordToProto(b.Word)
	if err != nil {
		return nil, err
	}
	key, err := answerKeyToProto(b.Key)
	if err != nil {
		return nil, err
	}
	temper, err := temperToProto(b.Temper)
	if err != nil {
		return nil, err
	}
	answered := &sessionpb.Answered{
		Creature: b.Creature,
		Roll:     int32(b.Roll),
		Of:       int32(b.Of),
		Entry:    int32(b.Entry),
		Word:     word,
		Say:      b.Say,
		// WHICH KEY THE WORLD ROLLED ON -- the one field that replaces the
		// deprecated `verb`/`beaten` pair, which between them could spell
		// exactly the four social outcomes and had no way at all to say
		// "it was this creature's turn".
		Key: key,
		// The loaded table as rolled, and the word that loaded it. Temper
		// is on every pick deliberately: a reader holding one beat can say
		// why the coward ran without joining back to the TEMPERED beat
		// that dealt it, and a creature whose temperament was AUTHORED
		// raised no such beat to join to.
		Candidates: answerCandidatesToProto(b.Candidates),
		Temper:     temper,
		// `fact` IS DELIBERATELY NOT SET, ruled by Kirk on rpg-project#458
		// after the contract had already made room for it.
		//
		// A FACT IS PER-OBSERVER KNOWLEDGE AND THIS BEAT IS BROADCAST. It
		// goes to every witness of the creature, and what any one of them
		// then KNOWS is the intel log's answer, held per observer and
		// reachable only through a read that is entitled to it. Putting the
		// id on a broadcast beat would hand the whole table a fact the
		// world may have taught only some of them, and there is no second
		// field that could take it back.
		//
		// NOTHING IS LOST. The story has the author's `say` line, which is
		// what a player actually receives; the consequence arrives on its
		// own terms as a STANCE_CHANGED or an arrival. `b.Fact` is read
		// and dropped here exactly as the verdict fields are on the
		// response one file over.
	}
	// THE DEPRECATED PAIR IS STILL FILLED, BUT ONLY WHERE IT CAN TELL THE
	// TRUTH (rpg-project#465). A reader written against the shipped shape
	// keeps working on all four social keys, and on `time` both stay unset
	// -- because no verb spoke and no check was beaten, and a zero Verb
	// beside `beaten: false` would read as a threat that failed. Leaving
	// them out is the honest account of a turn; filling them in would be a
	// sentence about an event that never happened.
	//
	// BEATEN IS REPEATED FROM THE CHECK BEAT ON PURPOSE where it applies.
	// It says which table `entry` indexes into, and pairing two beats to
	// find out would assume an ordering the stream does not promise.
	if answerKeyIsSocial(key) {
		// The seam's own Verb, mapped through the one table every other
		// verb on the wire goes through. An unrecognized verb here
		// DEGRADES to UNSPECIFIED rather than refusing, unlike the word
		// and the key above, and the difference is what each field claims:
		// this pair is deprecated and a client reads `key` instead, while
		// a word demoted to UNSPECIFIED would assert that the creature
		// merely spoke.
		//nolint:staticcheck // Deprecated by `key`, and deliberately still
		// written: the contract keeps both filled on every social key so a
		// reader built against the shipped shape goes on working.
		answered.Verb = verbToProto(sdk.Verb(b.Verb))
		//nolint:staticcheck // Deprecated by `key`; see the line above.
		answered.Beaten = b.Beaten
	}

	return answered, nil
}

// answerWordToProto names WHAT A CREATURE DID when the world rolled the
// author's table (rpg-project#458, extended by rpg-project#465).
//
// IT REFUSES AN UNKNOWN WORD RATHER THAN DEMOTING IT, and that is the whole
// reason it returns an error at all. The enum's own comment says it grows one
// value per slice: `alarm`, `lure`, `pretend` and `patrol` are named in the
// design and deliberately absent from the wire. So a word this build does not
// know is not a field it can degrade -- ANSWER_WORD_UNSPECIFIED is the wire's
// way of saying "an entry that only speaks", which is a positive claim, and
// sending it about a creature that actually ran for the guards would have
// every client narrate the wrong scene with nothing anywhere saying so. The
// refusal is loud, it names the word, and the fix is to rebuild this seam
// against the toolkit that grew it.
//
// SIX WORDS NOW, AND THE FOUR THAT ARRIVED ARE TIME WORDS. `fact` and `flee`
// are what a creature does when a social verb resolves against it; `hold`,
// `attack`, `toward` and `away` are what it does with one turn's worth of
// doing, on its turn in a fight or a round of the world clock. WHICH WORD IS
// LEGAL ON WHICH KEY IS NOT THIS FUNCTION'S QUESTION and deliberately not a
// second enum: the table's compiler refuses `attack` under `intimidated`, so
// a word on the wrong key never reaches this seam.
//
// EMPTY IS NOT UNKNOWN. An author may write an entry that only speaks, and the
// SDK carries that as an empty Word; UNSPECIFIED is its exact wire spelling.
func answerWordToProto(word string) (sessionpb.AnswerWord, error) {
	switch word {
	case "":
		return sessionpb.AnswerWord_ANSWER_WORD_UNSPECIFIED, nil
	case "fact":
		return sessionpb.AnswerWord_ANSWER_WORD_FACT, nil
	case "flee":
		return sessionpb.AnswerWord_ANSWER_WORD_FLEE, nil
	case "hold":
		return sessionpb.AnswerWord_ANSWER_WORD_HOLD, nil
	case "attack":
		return sessionpb.AnswerWord_ANSWER_WORD_ATTACK, nil
	case "toward":
		return sessionpb.AnswerWord_ANSWER_WORD_TOWARD, nil
	case "away":
		return sessionpb.AnswerWord_ANSWER_WORD_AWAY, nil
	default:
		return sessionpb.AnswerWord_ANSWER_WORD_UNSPECIFIED,
			fmt.Errorf("answered beat carries outcome word %q, which this build cannot name on the wire", word)
	}
}

// answerKeyToProto names WHICH TABLE the world rolled on (rpg-project#465 §2)
// -- the author's own spelling under `on:`, carried so the beat stands alone.
//
// IT REFUSES ANYTHING BUT THE FIVE, answerWordToProto's law for
// answerWordToProto's reason. The design seals the trigger vocabulary and says
// it grows one word per use case, so a sixth key means this seam is older than
// the toolkit that produced the beat. ANSWER_KEY_UNSPECIFIED is the zero this
// file's every enum keeps for "the producer failed", and publishing it about a
// creature that answered a real trigger would be exactly that claim.
//
// EMPTY REFUSES TOO, and this is the one place that differs from the word
// above. An empty Word is an author's real choice -- an entry that only speaks
// -- but there is no such thing as a pick on no key: the SDK fills this on
// every beat it writes, and empty means a beat stored by the build BEFORE the
// table existed, which this seam cannot honestly spell. Loud is the point:
// pre-release, a run that old is a run nobody is playing.
func answerKeyToProto(key string) (sessionpb.AnswerKey, error) {
	switch key {
	case "intimidated":
		return sessionpb.AnswerKey_ANSWER_KEY_INTIMIDATED, nil
	case "intimidate_failed":
		return sessionpb.AnswerKey_ANSWER_KEY_INTIMIDATE_FAILED, nil
	case "persuaded":
		return sessionpb.AnswerKey_ANSWER_KEY_PERSUADED, nil
	case "persuade_failed":
		return sessionpb.AnswerKey_ANSWER_KEY_PERSUADE_FAILED, nil
	case "time":
		return sessionpb.AnswerKey_ANSWER_KEY_TIME, nil
	default:
		return sessionpb.AnswerKey_ANSWER_KEY_UNSPECIFIED,
			fmt.Errorf("answered beat carries key %q, which this build cannot name on the wire", key)
	}
}

// answerKeyIsSocial says whether a key had a VERB and a CHECK behind it --
// which is the whole question `verb` and `beaten` answer, and the reason those
// two are filled on four keys and left unset on the fifth (rpg-project#465).
//
// A `time` pick has neither: nothing spoke and nothing was beaten. Writing
// VERB_UNSPECIFIED and `beaten: false` there would spell "a threat that
// failed", a sentence about an event that never happened, which is precisely
// what deprecated the pair.
func answerKeyIsSocial(key sessionpb.AnswerKey) bool {
	switch key {
	case sessionpb.AnswerKey_ANSWER_KEY_INTIMIDATED,
		sessionpb.AnswerKey_ANSWER_KEY_INTIMIDATE_FAILED,
		sessionpb.AnswerKey_ANSWER_KEY_PERSUADED,
		sessionpb.AnswerKey_ANSWER_KEY_PERSUADE_FAILED:
		return true
	case sessionpb.AnswerKey_ANSWER_KEY_UNSPECIFIED, sessionpb.AnswerKey_ANSWER_KEY_TIME:
		return false
	default:
		return false
	}
}

// temperToProto names A CREATURE'S TEMPERAMENT -- the weight profile loading
// its die (rpg-project#465 §3).
//
// EMPTY IS TEMPER_NONE, EXPLICITLY, never left to fall through to
// UNSPECIFIED. Having no temperament is a real answer about a creature, and
// this is the law slotToProto keeps one screen down for the same reason: the
// zero on this seam means the producer failed, so a creature nobody gave a
// word to needs a word of its own.
//
// NONE IS NOT SOLDIER, and collapsing them would lose the only thing they do
// not share. They multiply identically -- every factor 100 -- but SOLDIER
// means a placement or a faction's mix NAMED that word and NONE means nobody
// did, and a reader asking whether anybody chose it must be able to tell.
//
// AN UNKNOWN WORD REFUSES. Not UNSPECIFIED, which would confess this build's
// failure in a field a client reads as the world's answer, and above all not
// NONE, which would publish "this creature has no temperament" about a
// creature whose author gave it one -- every pick it ever makes would read as
// a soldier's, and nothing anywhere would say the word had been dropped.
func temperToProto(word string) (sessionpb.Temper, error) {
	switch word {
	case "":
		return sessionpb.Temper_TEMPER_NONE, nil
	case "soldier":
		return sessionpb.Temper_TEMPER_SOLDIER, nil
	case "coward":
		return sessionpb.Temper_TEMPER_COWARD, nil
	case "aggressive":
		return sessionpb.Temper_TEMPER_AGGRESSIVE, nil
	default:
		return sessionpb.Temper_TEMPER_UNSPECIFIED,
			fmt.Errorf("beat carries temperament %q, which this build cannot name on the wire", word)
	}
}

// answerCandidatesToProto mirrors THE LOADED TABLE AS ROLLED: every entry
// whose `when` held, in the author's order, with the arithmetic that put it on
// the die (rpg-project#465 §6).
//
// FIELD FOR FIELD, NOTHING DERIVED. `loaded` is weight x percent and this seam
// copies the engine's own product rather than recomputing it -- a converter
// that multiplied here could disagree with the sum on `of` and there would be
// no way to tell which of the two was the roll that happened.
//
// EMPTY IN, EMPTY OUT, and empty is a real answer: a `time` roll where no
// entry's `when` held put nothing on the table, so the creature held and the
// beat says so with no candidates at all.
func answerCandidatesToProto(cs []sdk.AnswerCandidate) []*sessionpb.AnswerCandidate {
	if len(cs) == 0 {
		return nil
	}
	out := make([]*sessionpb.AnswerCandidate, len(cs))
	for i, c := range cs {
		out[i] = &sessionpb.AnswerCandidate{
			Entry:   int32(c.Entry),
			Weight:  int32(c.Weight),
			Percent: int32(c.Percent),
			Loaded:  int32(c.Loaded),
		}
	}
	return out
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
	case sdk.TargetArea:
		return sessionpb.TargetKind_TARGET_KIND_AREA
	case sdk.TargetCell:
		return sessionpb.TargetKind_TARGET_KIND_CELL
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

func costComponentsToProto(cost []sdk.CostComponent) []*sessionpb.CostComponent {
	out := make([]*sessionpb.CostComponent, len(cost))
	for i, component := range cost {
		out[i] = &sessionpb.CostComponent{
			Currency: currencyToProto(component.Currency),
			Needed:   int32(component.Needed),
			Label:    component.Label,
		}
	}
	return out
}

// castOptionsToProto mirrors a cast row's menu -- Command's "Approach",
// "Flee", "Grovel" -- in the content's own order, which is the order a picker
// draws.
//
// NOTHING IS SORTED, GROUPED OR INFERRED. The order and the label are the
// spell's own presentation, authored beside the id; a converter that ranked
// them would be editing a spell from four layers away, and a client that
// derived a label from an id would be deriving 5e. The id is opaque here and
// stays opaque all the way back in on CastRequest.option.
//
// Make-then-map for the reason the candidate list uses it: a non-nil empty SDK
// answer stays non-nil empty in Go. On the wire an empty menu and an absent
// one are the same thing, and both say the same sentence -- this row offers no
// choice and will refuse one.
func castOptionsToProto(options []sdk.CastOption) []*sessionpb.CastOption {
	out := make([]*sessionpb.CastOption, len(options))
	for i, option := range options {
		out[i] = &sessionpb.CastOption{Id: option.ID, Label: option.Label}
	}
	return out
}

// footprintShapeToProto mirrors the provider's closed outline vocabulary.
// An unrecognized value reaches UNSPECIFIED, a producer defect.
func footprintShapeToProto(shape sdk.FootprintShape) sessionpb.FootprintShape {
	switch shape {
	case sdk.FootprintShapeRadius:
		return sessionpb.FootprintShape_FOOTPRINT_SHAPE_RADIUS
	case sdk.FootprintShapeBox:
		return sessionpb.FootprintShape_FOOTPRINT_SHAPE_BOX
	default:
		return sessionpb.FootprintShape_FOOTPRINT_SHAPE_UNSPECIFIED
	}
}

// footprintOriginToProto mirrors the provider's closed anchor vocabulary.
// An unrecognized value reaches UNSPECIFIED, a producer defect.
func footprintOriginToProto(origin sdk.FootprintOrigin) sessionpb.FootprintOrigin {
	switch origin {
	case sdk.FootprintOriginCaster:
		return sessionpb.FootprintOrigin_FOOTPRINT_ORIGIN_CASTER
	case sdk.FootprintOriginCasterEdge:
		return sessionpb.FootprintOrigin_FOOTPRINT_ORIGIN_CASTER_EDGE
	case sdk.FootprintOriginPoint:
		return sessionpb.FootprintOrigin_FOOTPRINT_ORIGIN_POINT
	default:
		return sessionpb.FootprintOrigin_FOOTPRINT_ORIGIN_UNSPECIFIED
	}
}

// footprintToProto copies optional provider-authored area presentation without
// deriving geometry or interpreting spell identity.
func footprintToProto(footprint *sdk.Footprint) *sessionpb.Footprint {
	if footprint == nil {
		return nil
	}
	// Like UNSPECIFIED for an unknown enum, zero is an invalid present extent.
	// Preserve that producer defect instead of wrapping or clamping an
	// unrepresentable size into an apparently usable outline.
	var sizeFeet int32
	if footprint.SizeFeet > 0 && int64(footprint.SizeFeet) <= math.MaxInt32 {
		sizeFeet = int32(footprint.SizeFeet)
	}
	return &sessionpb.Footprint{
		Shape:    footprintShapeToProto(footprint.Shape),
		SizeFeet: sizeFeet,
		Origin:   footprintOriginToProto(footprint.Origin),
	}
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
		MinTargets: int32(d.MinTargets),
		MaxTargets: int32(d.MaxTargets),
		Cost:       costComponentsToProto(d.Cost),
		// WHAT THE REQUEST MUST BRING BACK, when the spell asks a question
		// before it goes. A row listing options REQUIRES one on
		// CastRequest.option and a row listing none REFUSES one, which is the
		// law TargetKind already keeps for the aimed cell: the offer says what
		// the answer needs, rather than the client assembling a second
		// selector out of rows.
		Options:   castOptionsToProto(d.Options),
		Footprint: footprintToProto(d.Footprint),
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
	if d.Reaction != nil {
		out.Reaction = reactionRefToProto(d.Reaction)
	}
	// WHICH CANTRIP this row casts. Present on every VerbCast declaration and
	// absent from every other, which is the same presence law Attack and
	// Ability keep one field up: a dock says "Vicious Mockery" rather than
	// "Cast" because one verb compiles one row per castable cantrip, and the
	// verb alone cannot tell them apart.
	//
	// The SDK's own pointer decides, not the verb: a zeroed SpellRef on a
	// non-cast row would read as a spell nobody named.
	if d.Spell != nil {
		out.Spell = spellRefToProto(*d.Spell)
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
	case sdk.ShortfallWindowOpen:
		// The freeze (rpg-project#316 rung 3). Somebody at this table is
		// being asked whether they react, and until that answer arrives
		// nothing else may move -- so every other verb comes back
		// unavailable for this one reason, on this member's own turn as
		// much as on anybody else's. NOT NOT_YOUR_TURN, which would tell a
		// player to wait for a clock that is not what is holding them.
		return sessionpb.ShortfallReason_SHORTFALL_REASON_WINDOW_OPEN
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
	return &sessionpb.RollSource{
		Ref: source.Ref, Name: source.Name, Label: source.Label, SourceId: source.SourceID,
	}
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

// rollSourcesToProto copies a list of provider-authored identities in the
// order the producer wrote them. Each entry is independently copied, never
// aliased into the SDK's own slice.
func rollSourcesToProto(sources []sdk.RollSource) []*sessionpb.RollSource {
	out := make([]*sessionpb.RollSource, len(sources))
	for i := range sources {
		out[i] = rollSourceToProto(&sources[i])
	}
	return out
}

// keepRuleToProto names WHY one face of a pool counted and the others did not
// (rpg-project#462, R1/R2).
//
// IT REFUSES A RULE THIS BUILD CANNOT NAME rather than demoting it, the same
// law answerWordToProto keeps and for the same reason. KEEP_RULE_UNSPECIFIED
// is not "some rule we could not read": a pool nothing touched carries no
// DiceKeep at all, so UNSPECIFIED would be a positive claim that a keep record
// exists and says nothing. Sending it about a pool a real rule decided would
// have every client draw two faces with no reason beside them and nothing
// anywhere saying the reason was dropped. The refusal is loud, it names the
// rule, and the fix is to rebuild this seam against the toolkit that grew it.
//
// AN EMPTY RULE IS REFUSED TOO. Unlike the answered beat's word, where empty
// is an authored answer, a keep record with no rule could not have decided
// anything -- absence of a rule is spelled by the absence of the record.
func keepRuleToProto(rule sdk.KeepRule) (sessionpb.KeepRule, error) {
	switch rule {
	case sdk.KeepAdvantage:
		return sessionpb.KeepRule_KEEP_RULE_ADVANTAGE, nil
	case sdk.KeepDisadvantage:
		return sessionpb.KeepRule_KEEP_RULE_DISADVANTAGE, nil
	case sdk.KeepCancelled:
		return sessionpb.KeepRule_KEEP_RULE_CANCELLED, nil
	default:
		return sessionpb.KeepRule_KEEP_RULE_UNSPECIFIED,
			fmt.Errorf("dice pool carries keep rule %q, which this build cannot name on the wire", string(rule))
	}
}

// diceKeepToProto copies the record that decided KeptIndices, and who brought
// it. Granted and imposed cross exactly as the producer listed them; this seam
// never reads them to decide what the rule was, because the rule is its own
// field (rpg-project#462, R1).
//
// It is called only for a pool that HAS a record -- diceTraceToProto keeps the
// nil test, because a straight roll's unset field is the truth and not a
// conversion this function has to invent an answer for.
func diceKeepToProto(keep *sdk.DiceKeep) (*sessionpb.DiceKeep, error) {
	if keep == nil {
		return nil, errors.New("dice keep record is required")
	}
	rule, err := keepRuleToProto(keep.Rule)
	if err != nil {
		return nil, err
	}
	return &sessionpb.DiceKeep{
		Rule:    rule,
		Granted: rollSourcesToProto(keep.Granted),
		Imposed: rollSourcesToProto(keep.Imposed),
	}, nil
}

// diceTraceToProto copies the complete physical dice history field-for-field.
// Subtotal is authoritative and is never recomputed from the face lists.
//
// KEEP IS UNSET WHEN NOTHING TOUCHED THE POOL, and that is the whole zero
// value law of the record: a straight roll kept every face, nobody decided it,
// and the absent field says exactly that. A client reading two faces and no
// keep record is reading a producer defect, not advantage.
func diceTraceToProto(trace *sdk.DiceTrace) (*sessionpb.DiceTrace, error) {
	if trace == nil {
		return nil, errors.New("dice trace is required")
	}
	rerolls := make([]*sessionpb.DiceReroll, len(trace.Rerolls))
	for i := range trace.Rerolls {
		rerolls[i] = diceRerollToProto(&trace.Rerolls[i])
	}
	out := &sessionpb.DiceTrace{
		Notation:      trace.Notation,
		DieSize:       int32(trace.DieSize),
		OriginalRolls: intsToInt32s(trace.OriginalRolls),
		Rerolls:       rerolls,
		FinalRolls:    intsToInt32s(trace.FinalRolls),
		KeptIndices:   intsToInt32s(trace.KeptIndices),
		Subtotal:      int32(trace.Subtotal),
	}
	if trace.Keep != nil {
		keep, err := diceKeepToProto(trace.Keep)
		if err != nil {
			return nil, err
		}
		out.Keep = keep
	}
	return out, nil
}

// rollComponentToProto preserves optional modifier presence, including a
// present zero. Dice and source are independently copied and never aliased.
// A component with no dice is a flat modifier and its wire Dice stays unset.
func rollComponentToProto(component *sdk.RollComponent) (*sessionpb.RollComponent, error) {
	if component == nil {
		return nil, errors.New("roll component is required")
	}
	out := &sessionpb.RollComponent{
		Source:       rollSourceToProto(&component.Source),
		SubtractDice: component.SubtractDice,
	}
	if component.Dice != nil {
		dice, err := diceTraceToProto(component.Dice)
		if err != nil {
			return nil, err
		}
		out.Dice = dice
	}
	if component.Modifier != nil {
		modifier := int32(*component.Modifier)
		out.Modifier = &modifier
	}
	return out, nil
}

// rollCalculationToProto preserves component production order and copies the
// producer's authoritative total without validation or arithmetic.
//
// A NIL CALCULATION IS AN ANSWER, not a failure: a body that recorded no
// arithmetic leaves the wire field unset, the same presence law every other
// optional projection in this file keeps. The error channel reports exactly
// one thing -- a keep rule this build cannot name.
func rollCalculationToProto(calculation *sdk.RollCalculation) (*sessionpb.RollCalculation, error) {
	if calculation == nil {
		return nil, nil
	}
	components := make([]*sessionpb.RollComponent, len(calculation.Components))
	for i := range calculation.Components {
		component, err := rollComponentToProto(&calculation.Components[i])
		if err != nil {
			return nil, err
		}
		components[i] = component
	}
	return &sessionpb.RollCalculation{Components: components, Total: int32(calculation.Total)}, nil
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

func damageComponentsToProto(in []sdk.DamageComponent) ([]*sessionpb.DamageComponent, error) {
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
			roll, err := rollComponentToProto(&component.Roll)
			if err != nil {
				return nil, err
			}
			converted.Roll = roll
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
	return out, nil
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

// reactionRefToProto mirrors what a beat or an offer was taken AS: the
// opportunity attack that let a fighter swing on somebody else's turn
// (rpg-project#316). Pointer in and pointer out, because absence is the
// common case and it MEANS something -- an ordinary swing on the actor's own
// turn was taken as nothing, and a zeroed ReactionRef on the wire would read
// as a reaction with no name rather than as no reaction.
func reactionRefToProto(r *sdk.ReactionRef) *sessionpb.ReactionRef {
	if r == nil {
		return nil
	}
	return &sessionpb.ReactionRef{Ref: r.Ref, Name: r.Name}
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
		// Whether this member is holding a spell together right now
		// (rpg-project#407, R11). FOR THE PEOPLE WHO CANNOT SEE THE SHEET:
		// the caster reads its own concentrating condition off its own
		// status, and a creature carrying a spell's effect learns the caster
		// from that effect's own source. What the rest of the table cannot
		// otherwise learn is that a member whose sheet they do not hold is
		// concentrating at all -- and a concentration-ended beat about a
		// member whose state was never visible is a beat with no setup.
		//
		// ONE BOOL AND NOTHING MORE, mirroring Active: no spell, no ref, no
		// remaining duration. Which spell somebody is holding is a fact their
		// own sheet answers, and a roster row that named it would publish the
		// caster's hand to the room.
		Concentrating: p.Concentrating,
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

// experienceGrantsToProto mirrors one grant's per-character shares
// (rpg-project#496, R5), keeping the SDK's own sort by character -- this
// converter reorders nothing, so two clients reading the same beat narrate
// the party in the same order.
//
// The amounts widen int -> int32 like every other count on this wire. A share
// is a monster's authored worth divided among the players and a total is what
// one sheet holds, so neither can approach that ceiling in a game anyone
// plays; the SDK, not this file, is where a number that could would be caught.
func experienceGrantsToProto(gs []sdk.ExperienceGrant) []*sessionpb.ExperienceGrant {
	out := make([]*sessionpb.ExperienceGrant, len(gs))
	for i, g := range gs {
		out[i] = &sessionpb.ExperienceGrant{
			Character: g.Character,
			Amount:    int32(g.Amount),
			Total:     int32(g.Total),
		}
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

// revealedDoorToProto groups one session.RevealedDoor's flat door/state/
// approaches into the wire's nested DoorInfo -- the same shape GetDoors
// already returns for this door, per ConcealmentRevealed.doors's own doc
// ("exactly as this recipient's GetDoors would now list them"). Approaches
// is present only while the door is locked (RevealedDoor's own field law),
// so a non-empty list is the presence signal for Lock, matching
// doorToProto's "Lock unset is not locked" convention field-for-field
// rather than re-deriving it from State.
//
// THE DOORWAYS DO NOT RIDE WITH IT. DoorInfo has no doorway field -- a
// door's edges belong to the atlas, not to the door -- so the caller
// flattens them into ConcealmentRevealed.doorways, which is the list a
// client's cached atlas takes them from.
func revealedDoorToProto(d sdk.RevealedDoor) *sessionpb.DoorInfo {
	out := &sessionpb.DoorInfo{Door: d.Door, State: doorStateToProto(d.State)}
	if len(d.Approaches) > 0 {
		out.Lock = &sessionpb.DoorLock{Approaches: doorApproachesToProto(d.Approaches)}
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
// field unset) becomes the zero TradeOffer -- an empty Items slice and zero
// Currency, which is exactly what an omitted `give`/`receive` on the wire
// means (session.TradeInput's own doc: Give must be empty this wave; the
// SDK's own ErrGiveNotSupported refusal is what tells a caller who sent one
// anyway, not a nil check here). On `give`, Currency is the payment
// (rpg-toolkit#1534) -- carried untouched, never adjusted or defaulted here:
// session.Trade alone decides whether it matches the required price
// (ErrWrongPrice).
func tradeOfferFromProto(o *sessionpb.TradeOffer) sdk.TradeOffer {
	items := make([]sdk.TradeItem, len(o.GetItems()))
	for i, it := range o.GetItems() {
		items[i] = tradeItemFromProto(it)
	}
	return sdk.TradeOffer{Items: items, Currency: moneyFromProto(o.GetCurrency())}
}

// caughtMembersToProto carries the members an area cast reached and the engine
// could not resolve against.
//
// NIL IN, NIL OUT. Most casts catch nobody this way and every cast that is not
// an area catches nobody at all, so an empty slice would be a second way of
// saying the same nothing.
func caughtMembersToProto(caught []sdk.CaughtMember) []*sessionpb.CaughtMember {
	if len(caught) == 0 {
		return nil
	}
	out := make([]*sessionpb.CaughtMember, 0, len(caught))
	for _, member := range caught {
		out = append(out, &sessionpb.CaughtMember{
			Member: member.Member,
			Kind:   memberKindToProto(member.Kind),
			Reason: unresolvedReasonToProto(member.Reason),
		})
	}
	return out
}

// unresolvedReasonToProto mirrors the SDK's closed reason enum.
//
// An unknown value reaches UNSPECIFIED rather than being guessed, the way every
// other closed enum here does — and TestEveryProtoUnresolvedReasonIsProduced
// keeps that from silently swallowing a new one, which is the failure this
// package has already paid for once with TargetKind.
func unresolvedReasonToProto(r sdk.UnresolvedReason) sessionpb.UnresolvedReason {
	switch r {
	case sdk.UnresolvedNoSheet:
		return sessionpb.UnresolvedReason_UNRESOLVED_REASON_NO_SHEET
	default:
		return sessionpb.UnresolvedReason_UNRESOLVED_REASON_UNSPECIFIED
	}
}
