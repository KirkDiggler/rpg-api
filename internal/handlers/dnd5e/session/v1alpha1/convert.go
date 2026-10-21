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

// watchZonesToProto carries the session's red floor onto the wire
// (rpg-project#619, sneaking R15): one WatchZone per watcher, in the order
// the session answered, each cell through positionToProto exactly as
// visible_cells is. The api computes no zone; an empty answer stays empty.
func watchZonesToProto(in []sdk.WatchZone) []*sessionpb.WatchZone {
	if len(in) == 0 {
		return nil
	}
	out := make([]*sessionpb.WatchZone, 0, len(in))
	for _, z := range in {
		w := &sessionpb.WatchZone{Watcher: z.Watcher}
		for _, c := range z.Cells {
			w.Cells = append(w.Cells, positionToProto(c))
		}
		out = append(out, w)
	}
	return out
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
	// Facing is a compass bearing (0 = map north = -Y of the footprint
	// plane, 90 = +X, clockwise) and the wire speaks the same frame, so the
	// number passes through unchanged (rpg-project#582, sneaking R13). Nil is
	// testimony that predates facing, not "facing north": it stays unset. The
	// value is copied so the wire message never aliases the toolkit's Seen.
	if s.Facing != nil {
		facing := *s.Facing
		out.FacingDegrees = &facing
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
		Passage:    passageToProto(s.Passage),
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

// spellRefToProto mirrors AbilityRef's shape one content type over: the full
// ref for correlation and an icon table, and a name the content authored.
// A reader never derives the name from the ref, and never branches on the ref
// to decide what a spell does.
func spellRefToProto(s sdk.SpellRef) *sessionpb.SpellRef {
	return &sessionpb.SpellRef{Ref: s.Ref, Name: s.Name}
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
// UNSET (rpg-project#479). The key identifies source; it does not authorize a
// gameplay client to fetch the complete authored document. Permitted fixed
// structure and opaque content references now arrive on the SDK atlas and
// existing reveal bodies. This converter copies those records, never derives
// visibility or fetches source to fill missing data.
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
		Placed:            atlasPlacedPropsToProto(a.Placed),
		StructuralWalls:   atlasStructuralWallsToProto(a.StructuralWalls),
		StructuralDoors:   atlasStructuralDoorsToProto(a.StructuralDoors),
		PropPresentations: propPresentationsToProto(a.PropPresentations),
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
	// VerbSpeak (rpg-project#569), which replaced the Intimidate and
	// Persuade rows. LOAD-BEARING FOR THE DOCK, not a completeness sweep:
	// leaving it unmapped would label the row VERB_UNSPECIFIED -- and a client
	// drops a verb it cannot name rather than showing it wrong, so the menu
	// would never appear. Afford emits it on BOTH clocks (rpg-project#458 R3):
	// free on the world clock, the action on the turn clock.
	case sdk.VerbSpeak:
		return sessionpb.Verb_VERB_SPEAK
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
// targetCandidateToProto mirrors one candidate. Effects are per-target
// answers to the actor's declaration rows; HeldEffects are full rows for
// effects THAT target holds (R18). The two lists cross separately and are
// never joined here or with the declaration's rows.
func targetCandidateToProto(c sdk.TargetCandidate) *sessionpb.TargetCandidate {
	return &sessionpb.TargetCandidate{
		Member:      c.Member,
		Available:   c.Available,
		Why:         shortfallToProto(c.Why),
		Effects:     targetEffectsToProto(c.Effects),
		HeldEffects: effectRowsToProto(c.HeldEffects),
	}
}

func targetCandidatesToProto(cs []sdk.TargetCandidate) []*sessionpb.TargetCandidate {
	out := make([]*sessionpb.TargetCandidate, len(cs))
	for i, candidate := range cs {
		out[i] = targetCandidateToProto(candidate)
	}
	return out
}

// effectStateToProto mirrors session.EffectState onto the wire enum. An
// unrecognized string reaches UNSPECIFIED, a producer defect.
func effectStateToProto(s sdk.EffectState) sessionpb.EffectState {
	switch s {
	case sdk.EffectApplies:
		return sessionpb.EffectState_EFFECT_STATE_APPLIES
	case sdk.EffectDoesNotApply:
		return sessionpb.EffectState_EFFECT_STATE_DOES_NOT_APPLY
	case sdk.EffectDepends:
		return sessionpb.EffectState_EFFECT_STATE_DEPENDS
	case sdk.EffectUnavailable:
		return sessionpb.EffectState_EFFECT_STATE_UNAVAILABLE
	default:
		return sessionpb.EffectState_EFFECT_STATE_UNSPECIFIED
	}
}

// effectParticipationToProto mirrors session.EffectParticipation onto the wire
// enum. An unrecognized string reaches UNSPECIFIED, a producer defect.
func effectParticipationToProto(p sdk.EffectParticipation) sessionpb.EffectParticipation {
	switch p {
	case sdk.ContributesNow:
		return sessionpb.EffectParticipation_EFFECT_PARTICIPATION_CONTRIBUTES_NOW
	case sdk.LaterChoice:
		return sessionpb.EffectParticipation_EFFECT_PARTICIPATION_LATER_CHOICE
	default:
		return sessionpb.EffectParticipation_EFFECT_PARTICIPATION_UNSPECIFIED
	}
}

// effectRowsToProto mirrors a declaration's effect rows in the SDK's order.
// The API names no effect and rules on none: nothing is filtered, sorted or
// derived, and a row's name, description, reason and benefit are the
// toolkit's own text. Make-then-map, so an empty SDK answer stays non-nil.
func effectRowsToProto(rows []sdk.EffectRow) []*sessionpb.EffectRow {
	out := make([]*sessionpb.EffectRow, len(rows))
	for i, row := range rows {
		out[i] = &sessionpb.EffectRow{
			Id:            row.ID,
			Ref:           row.Ref,
			Name:          row.Name,
			Description:   row.Description,
			State:         effectStateToProto(row.State),
			Reason:        row.Reason,
			Participation: effectParticipationToProto(row.Participation),
			Benefit:       row.Benefit,
		}
	}
	return out
}

// targetEffectsToProto mirrors one candidate's per-target answers. Each
// replaces the declaration row with the same id on the client; this converter
// neither joins nor checks them against the declaration's rows.
func targetEffectsToProto(effects []sdk.TargetEffect) []*sessionpb.TargetEffect {
	out := make([]*sessionpb.TargetEffect, len(effects))
	for i, effect := range effects {
		out[i] = &sessionpb.TargetEffect{
			Id:      effect.ID,
			State:   effectStateToProto(effect.State),
			Reason:  effect.Reason,
			Benefit: effect.Benefit,
		}
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
		out[i] = &sessionpb.CastOption{Id: option.ID, Label: option.Label, Description: option.Description}
	}
	return out
}

// speakOptionsToProto mirrors the Speak row's menu field for field. Available
// is the SDK's own answer, copied (every row listed today is available: a line
// whose `requires` the speaker does not meet is left off, not listed
// unavailable). Shortfall is the line's own reason, present exactly when the
// SDK set Why. Nil in is nil out, so a non-Speak row carries no menu.
//
// EXACTLY ONE TARGET ARM, refused the way spokeToProto refuses it. A row
// naming both or neither is a producer defect; crossing it as an empty member
// id, or dropping one target, would offer the player a line said to nobody or
// to the wrong thing.
func speakOptionsToProto(rows []sdk.SpeakOptionRow) ([]*sessionpb.SpeakOption, error) {
	if rows == nil {
		return nil, nil
	}
	out := make([]*sessionpb.SpeakOption, len(rows))
	for i, row := range rows {
		option := &sessionpb.SpeakOption{
			OptionId:  row.OptionID,
			Line:      row.Line,
			Available: row.Available,
		}
		switch {
		case row.Target != "" && row.Prop == "":
			option.Target = &sessionpb.SpeakOption_MemberId{MemberId: row.Target}
		case row.Prop != "" && row.Target == "":
			option.Target = &sessionpb.SpeakOption_PropId{PropId: row.Prop}
		default:
			return nil, fmt.Errorf("speak option %q names target %q and prop %q; exactly one must be set",
				row.OptionID, row.Target, row.Prop)
		}
		if row.Why != nil {
			option.Shortfall = shortfallReasonToProto(row.Why.Reason)
		}
		out[i] = option
	}
	return out, nil
}

// footprintShapeToProto mirrors the provider's closed outline vocabulary.
// An unrecognized value reaches UNSPECIFIED, a producer defect.
func footprintShapeToProto(shape sdk.FootprintShape) sessionpb.FootprintShape {
	switch shape {
	case sdk.FootprintShapeRadius:
		return sessionpb.FootprintShape_FOOTPRINT_SHAPE_RADIUS
	case sdk.FootprintShapeTriangle:
		return sessionpb.FootprintShape_FOOTPRINT_SHAPE_TRIANGLE
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
func declarationToProto(d sdk.Declaration) (*sessionpb.Declaration, error) {
	speakOptions, err := speakOptionsToProto(d.SpeakOptions)
	if err != nil {
		return nil, err
	}
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
		Options: castOptionsToProto(d.Options),
		// THE SPEAK ROW'S MENU (rpg-project#569): one entry per (target,
		// line) pair. Empty on every other verb. A Speak request brings back
		// the option id AND the same target, since an id alone names nothing.
		SpeakOptions: speakOptions,
		Footprint:    footprintToProto(d.Footprint),
		// WHAT BEARS ON THIS ROW, as the toolkit answered it. Rows never grant
		// or refuse the declaration; Available and Why above stay the only gate.
		Effects: effectRowsToProto(d.Effects),
	}
	out.Information = actionInformationToProto(d.Information)
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
	return out, nil
}

// shortfallReasonToProto mirrors session.ShortfallReason onto the wire enum.
// An unrecognized string reaches UNSPECIFIED, a producer defect.
func shortfallReasonToProto(r sdk.ShortfallReason) sessionpb.ShortfallReason {
	switch r {
	case sdk.ShortfallNoBudget:
		return sessionpb.ShortfallReason_SHORTFALL_REASON_NO_BUDGET
	case sdk.ShortfallNotYourTurn:
		return sessionpb.ShortfallReason_SHORTFALL_REASON_NOT_YOUR_TURN
	// A Speak row with nothing on its menu (rpg-project#569): nobody in
	// sight offers this speaker a line.
	case sdk.ShortfallNoOption:
		return sessionpb.ShortfallReason_SHORTFALL_REASON_NO_OPTION
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

// declarationsToProto mirrors a Declarations list. A nil or empty input
// becomes a non-nil, zero-length slice, matching the SDK's own "empty IS the
// answer" law for the world clock (AffordOutput.Declarations never marshals
// as null) -- the same make-then-loop shape every other list converter in
// this file keeps, so an empty result already comes out non-nil for free.
//
// ONE MALFORMED ROW FAILS THE WHOLE LIST, as one bad beat fails a story read:
// a panel missing a row it was offered is a wrong panel, not a smaller one.
func declarationsToProto(ds []sdk.Declaration) ([]*sessionpb.Declaration, error) {
	out := make([]*sessionpb.Declaration, len(ds))
	for i, d := range ds {
		decl, err := declarationToProto(d)
		if err != nil {
			return nil, fmt.Errorf("declaration %q: %w", d.ID, err)
		}
		out[i] = decl
	}
	return out, nil
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

func passageToProto(p sdk.Passage) sessionpb.Passage {
	switch p {
	case sdk.PassageBlocked:
		return sessionpb.Passage_PASSAGE_BLOCKED
	case sdk.PassagePassThrough:
		return sessionpb.Passage_PASSAGE_PASS_THROUGH
	case sdk.PassageStandable:
		return sessionpb.Passage_PASSAGE_STANDABLE
	default:
		return sessionpb.Passage_PASSAGE_UNSPECIFIED
	}
}

// actionInformationToProto copies one declaration's information field for
// field: the owner's description verbatim and the rendered detail rows in the
// order the toolkit gave them. Nil stays nil, so an action with no
// information is absent on the wire, and nothing here is looked up,
// formatted, or computed.
func actionInformationToProto(info *sdk.ActionInformation) *sessionpb.ActionInformation {
	if info == nil {
		return nil
	}
	out := &sessionpb.ActionInformation{Description: info.Description}
	if len(info.Details) > 0 {
		out.Details = make([]*sessionpb.ActionInformationDetail, len(info.Details))
		for i, d := range info.Details {
			out.Details[i] = &sessionpb.ActionInformationDetail{Label: d.Label, Value: d.Value}
		}
	}
	return out
}
