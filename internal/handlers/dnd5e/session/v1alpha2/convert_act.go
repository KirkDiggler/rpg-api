package sessionv1alpha2

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
)

// convert_act.go spells the SDK's acts on session v1alpha2 (rpg-project#630,
// event-stream R16, R17, R22, Laws 6 and 7): one function per body, every
// field copied, every closed set mapped through a table that fails closed.
//
// A REFUSED ACT KEEPS ITS ID (R22). An act the SDK could not decode
// (sdk.Act.Undecoded) or that this file cannot spell goes out with its id,
// act, family and segment, refused = true, the reason, and no body. The acts
// around it are untouched, so one bad act fails that act and never the read.

// errUnknownValue is a closed-set value this build has no wire spelling for.
var errUnknownValue = errors.New("no v1alpha2 spelling")

// actToProto spells one act. It always returns an act; the error, when set,
// says why that act went out refused, for the caller to log.
func actToProto(a sdk.Act) (*sessionpb.Act, error) {
	out := &sessionpb.Act{
		Session: a.Session,
		Id:      a.ID,
		At:      a.At,
		Act:     a.Act,
		Verb:    a.Verb,
		Segment: int32(a.Segment), //nolint:gosec // a segment count fits int32
	}
	family, err := lookup(familyToProto, a.Family, "family")
	if err != nil {
		return refuse(out, err)
	}
	out.Family = family
	if a.Undecoded != "" {
		return refuse(out, fmt.Errorf("undecoded: %s", a.Undecoded))
	}
	if err := setBody(out, a.Body); err != nil {
		return refuse(out, err)
	}
	return out, nil
}

// refuse marks out refused with err's reason and clears any body.
func refuse(out *sessionpb.Act, err error) (*sessionpb.Act, error) {
	out.Refused = true
	out.Refusal = err.Error()
	out.Body = nil
	return out, err
}

// setBody spells body into out's oneof.
//
//nolint:gocyclo // one arm per body is the table
func setBody(out *sessionpb.Act, body sdk.ActBody) error {
	var err error
	switch b := body.(type) {
	case sdk.WalkAct:
		var w *sessionpb.WalkAct
		w, err = walkToProto(b)
		out.Body = &sessionpb.Act_Walk{Walk: w}
	case sdk.AttackAct:
		var v *sessionpb.AttackAct
		v, err = attackToProto(b)
		out.Body = &sessionpb.Act_Attack{Attack: v}
	case sdk.CastAct:
		var v *sessionpb.CastAct
		v, err = castToProto(b)
		out.Body = &sessionpb.Act_Cast{Cast: v}
	case sdk.ActivateAct:
		var v *sessionpb.ActivateAct
		v, err = activateToProto(b)
		out.Body = &sessionpb.Act_Activate{Activate: v}
	case sdk.SpeakAct:
		var v *sessionpb.SpeakAct
		v, err = speakToProto(b)
		out.Body = &sessionpb.Act_Speak{Speak: v}
	case sdk.DeathSaveAct:
		var v *sessionpb.DeathSaveAct
		v, err = deathSaveToProto(b)
		out.Body = &sessionpb.Act_DeathSave{DeathSave: v}
	case sdk.LootAct:
		var v *sessionpb.LootAct
		v, err = lootToProto(b)
		out.Body = &sessionpb.Act_Loot{Loot: v}
	case sdk.HoldAct:
		var v *sessionpb.HoldAct
		v, err = holdToProto(b)
		out.Body = &sessionpb.Act_Hold{Hold: v}
	case sdk.DropAct:
		var v *sessionpb.DropAct
		v, err = dropToProto(b)
		out.Body = &sessionpb.Act_Drop{Drop: v}
	case sdk.DoorAct:
		var v *sessionpb.DoorAct
		v, err = doorToProto(b)
		out.Body = &sessionpb.Act_Door{Door: v}
	case sdk.EquipAct:
		var v *sessionpb.EquipAct
		v, err = equipToProto(b)
		out.Body = &sessionpb.Act_Equip{Equip: v}
	case sdk.StayAct:
		var v *sessionpb.StayAct
		v, err = stayToProto(b)
		out.Body = &sessionpb.Act_Stay{Stay: v}
	case sdk.SneakAct:
		var v *sessionpb.SneakAct
		v, err = sneakToProto(b)
		out.Body = &sessionpb.Act_Sneak{Sneak: v}
	case sdk.RestAct:
		var v *sessionpb.RestAct
		v, err = restToProto(b)
		out.Body = &sessionpb.Act_Rest{Rest: v}
	case sdk.TableAnswer:
		var v *sessionpb.TableAnswer
		v, err = tableAnswerToProto(b)
		out.Body = &sessionpb.Act_TableAnswer{TableAnswer: v}
	case sdk.StateChange:
		var v *sessionpb.StateChange
		v, err = stateToProto(b)
		out.Body = &sessionpb.Act_State{State: v}
	case sdk.Perception:
		var v *sessionpb.Perception
		v, err = perceptionToProto(b)
		out.Body = &sessionpb.Act_Perception{Perception: v}
	case sdk.ClockEntry:
		var v *sessionpb.ClockEntry
		v, err = clockToProto(b)
		out.Body = &sessionpb.Act_Clock{Clock: v}
	default:
		return fmt.Errorf("body %T: %w", body, errUnknownValue)
	}
	return err
}

// --- closed sets -----------------------------------------------------------

var familyToProto = map[sdk.Family]sessionpb.Family{
	sdk.FamilyAct:         sessionpb.Family_FAMILY_ACT,
	sdk.FamilyReaction:    sessionpb.Family_FAMILY_REACTION,
	sdk.FamilyTableAnswer: sessionpb.Family_FAMILY_TABLE_ANSWER,
	sdk.FamilyState:       sessionpb.Family_FAMILY_STATE,
	sdk.FamilyPerception:  sessionpb.Family_FAMILY_PERCEPTION,
	sdk.FamilyClock:       sessionpb.Family_FAMILY_CLOCK,
}

var partyKindToProto = map[sdk.PartyKind]sessionpb.PartyKind{
	sdk.PartyMember:  sessionpb.PartyKind_PARTY_KIND_MEMBER,
	sdk.PartyProp:    sessionpb.PartyKind_PARTY_KIND_PROP,
	sdk.PartyFaction: sessionpb.PartyKind_PARTY_KIND_FACTION,
	sdk.PartyWorld:   sessionpb.PartyKind_PARTY_KIND_WORLD,
}

var attackOutcomeToProto = map[sdk.AttackOutcome]sessionpb.AttackOutcome{
	sdk.AttackHit:    sessionpb.AttackOutcome_ATTACK_OUTCOME_HIT,
	sdk.AttackMiss:   sessionpb.AttackOutcome_ATTACK_OUTCOME_MISS,
	sdk.AttackWarded: sessionpb.AttackOutcome_ATTACK_OUTCOME_WARDED,
}

var castOutcomeToProto = map[sdk.CastOutcome]sessionpb.CastOutcome{
	sdk.CastMissed: sessionpb.CastOutcome_CAST_OUTCOME_MISS,
	sdk.CastWarded: sessionpb.CastOutcome_CAST_OUTCOME_WARDED,
	sdk.CastSaved:  sessionpb.CastOutcome_CAST_OUTCOME_SAVED,
	sdk.CastFailed: sessionpb.CastOutcome_CAST_OUTCOME_FAILED,
}

var stateKindToProto = map[sdk.StateKind]sessionpb.StateKind{
	sdk.StateDowned:             sessionpb.StateKind_STATE_KIND_DOWNED,
	sdk.StateStance:             sessionpb.StateKind_STATE_KIND_STANCE,
	sdk.StateConcentrationEnded: sessionpb.StateKind_STATE_KIND_CONCENTRATION_ENDED,
	sdk.StateConditionRemoved:   sessionpb.StateKind_STATE_KIND_CONDITION_REMOVED,
	sdk.StateExperience:         sessionpb.StateKind_STATE_KIND_EXPERIENCE,
}

var senseToProto = map[sdk.Sense]sessionpb.Sense{
	sdk.SenseSight:     sessionpb.Sense_SENSE_SIGHT,
	sdk.SenseHearing:   sessionpb.Sense_SENSE_HEARING,
	sdk.SenseDiscovery: sessionpb.Sense_SENSE_DISCOVERY,
	sdk.SenseReveal:    sessionpb.Sense_SENSE_REVEAL,
	sdk.SenseCells:     sessionpb.Sense_SENSE_CELLS,
}

// endingResultToProto is the declared result of the ending reached
// (rpg-project#733). "" is not in the table: it is an ending that declares
// none, carried as unspecified by endingResult, never an error.
var endingResultToProto = map[sdk.EndingResult]sessionpb.EndingResult{
	sdk.ResultSuccess: sessionpb.EndingResult_ENDING_RESULT_SUCCESS,
	sdk.ResultFailure: sessionpb.EndingResult_ENDING_RESULT_FAILURE,
}

var clockMarkToProto = map[sdk.ClockMark]sessionpb.ClockMark{
	sdk.ClockTick:             sessionpb.ClockMark_CLOCK_MARK_TICK,
	sdk.ClockTurnEnded:        sessionpb.ClockMark_CLOCK_MARK_TURN_ENDED,
	sdk.ClockFightFormed:      sessionpb.ClockMark_CLOCK_MARK_FIGHT_FORMED,
	sdk.ClockFightDissolved:   sessionpb.ClockMark_CLOCK_MARK_FIGHT_DISSOLVED,
	sdk.ClockJoined:           sessionpb.ClockMark_CLOCK_MARK_JOINED,
	sdk.ClockExited:           sessionpb.ClockMark_CLOCK_MARK_EXITED,
	sdk.ClockArrived:          sessionpb.ClockMark_CLOCK_MARK_ARRIVED,
	sdk.ClockSceneOpened:      sessionpb.ClockMark_CLOCK_MARK_SCENE_OPENED,
	sdk.ClockEnded:            sessionpb.ClockMark_CLOCK_MARK_ENDED,
	sdk.ClockWindowOpened:     sessionpb.ClockMark_CLOCK_MARK_WINDOW_OPENED,
	sdk.ClockRollWindowOpened: sessionpb.ClockMark_CLOCK_MARK_ROLL_WINDOW_OPENED,
}

var influenceKindToProto = map[sdk.InfluenceKind]sessionpb.InfluenceKind{
	sdk.InfluenceAdvantage:    sessionpb.InfluenceKind_INFLUENCE_KIND_ADVANTAGE,
	sdk.InfluenceDisadvantage: sessionpb.InfluenceKind_INFLUENCE_KIND_DISADVANTAGE,
}

var damageTypeToProto = map[sdk.DamageType]sessionpb.DamageType{
	sdk.DamageAcid:        sessionpb.DamageType_DAMAGE_TYPE_ACID,
	sdk.DamageBludgeoning: sessionpb.DamageType_DAMAGE_TYPE_BLUDGEONING,
	sdk.DamageCold:        sessionpb.DamageType_DAMAGE_TYPE_COLD,
	sdk.DamageFire:        sessionpb.DamageType_DAMAGE_TYPE_FIRE,
	sdk.DamageForce:       sessionpb.DamageType_DAMAGE_TYPE_FORCE,
	sdk.DamageLightning:   sessionpb.DamageType_DAMAGE_TYPE_LIGHTNING,
	sdk.DamageNecrotic:    sessionpb.DamageType_DAMAGE_TYPE_NECROTIC,
	sdk.DamagePiercing:    sessionpb.DamageType_DAMAGE_TYPE_PIERCING,
	sdk.DamagePoison:      sessionpb.DamageType_DAMAGE_TYPE_POISON,
	sdk.DamagePsychic:     sessionpb.DamageType_DAMAGE_TYPE_PSYCHIC,
	sdk.DamageRadiant:     sessionpb.DamageType_DAMAGE_TYPE_RADIANT,
	sdk.DamageSlashing:    sessionpb.DamageType_DAMAGE_TYPE_SLASHING,
	sdk.DamageThunder:     sessionpb.DamageType_DAMAGE_TYPE_THUNDER,
}

// lookup maps a closed-set value, failing closed on one with no spelling.
func lookup[K ~string, V any](table map[K]V, k K, what string) (V, error) {
	v, ok := table[k]
	if !ok {
		var zero V
		return zero, fmt.Errorf("%s %q: %w", what, string(k), errUnknownValue)
	}
	return v, nil
}

// endingResult maps an ending's declared result; "" is unspecified, because
// an ending may declare none and an ended beat written before results existed
// carries none, so an old run reads no result rather than a wrong one.
func endingResult(r sdk.EndingResult) (sessionpb.EndingResult, error) {
	if r == "" {
		return sessionpb.EndingResult_ENDING_RESULT_UNSPECIFIED, nil
	}
	return lookup(endingResultToProto, r, "ending result")
}

// damageType maps a damage type; "" is the unspecified type, not an error,
// because an attack ref built before damage types were carried names none.
func damageType(t sdk.DamageType) (sessionpb.DamageType, error) {
	if t == "" {
		return sessionpb.DamageType_DAMAGE_TYPE_UNSPECIFIED, nil
	}
	return lookup(damageTypeToProto, t, "damage type")
}

// --- shared messages -------------------------------------------------------

// partyToProto spells a party; the zero party is nil, an absent field.
func partyToProto(p sdk.Party) (*sessionpb.Party, error) {
	if p == (sdk.Party{}) {
		return nil, nil //nolint:nilnil // an absent party is a valid answer
	}
	kind, err := lookup(partyKindToProto, p.Kind, "party kind")
	if err != nil {
		return nil, err
	}
	return &sessionpb.Party{Kind: kind, Id: p.ID}, nil
}

func partiesToProto(ps []sdk.Party) ([]*sessionpb.Party, error) {
	if len(ps) == 0 {
		return nil, nil
	}
	out := make([]*sessionpb.Party, len(ps))
	for i, p := range ps {
		v, err := partyToProto(p)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

// parties is a batch of party conversions that keeps the first error, so a
// body converter reads as a list of fields.
type parties struct{ err error }

func (c *parties) one(p sdk.Party) *sessionpb.Party {
	v, err := partyToProto(p)
	if c.err == nil {
		c.err = err
	}
	return v
}

func (c *parties) many(ps []sdk.Party) []*sessionpb.Party {
	v, err := partiesToProto(ps)
	if c.err == nil {
		c.err = err
	}
	return v
}

// roll spells a roll, keeping the first error (an influence this build
// cannot spell refuses the act rather than vanishing from the card).
func (c *parties) roll(r *sdk.Roll) *sessionpb.Roll {
	v, err := rollToProto(r)
	if c.err == nil {
		c.err = err
	}
	return v
}

func (c *parties) rollValue(r sdk.Roll) *sessionpb.Roll { return c.roll(&r) }

func int32s(in []int) []int32 {
	if len(in) == 0 {
		return nil
	}
	out := make([]int32, len(in))
	for i, v := range in {
		out[i] = int32(v) //nolint:gosec // die faces fit int32
	}
	return out
}

// rollToProto spells a roll, nil for nil. An influence kind this build
// cannot spell is an error.
func rollToProto(r *sdk.Roll) (*sessionpb.Roll, error) {
	if r == nil {
		return nil, nil //nolint:nilnil // an absent roll is a valid answer
	}
	out := &sessionpb.Roll{
		Expression: r.Expression,
		Dice:       int32s(r.Dice),
		Total:      int32(r.Total), //nolint:gosec // a roll total fits int32
		Of:         int32(r.Of),    //nolint:gosec // a table size fits int32
	}
	for _, m := range r.Modifiers {
		out.Modifiers = append(out.Modifiers, modifierToProto(m))
	}
	if r.Against != nil {
		against := int32(*r.Against) //nolint:gosec // a DC or an AC fits int32
		out.Against = &against
		if r.Beaten != nil {
			beaten := *r.Beaten
			out.Beaten = &beaten
		}
	}
	for _, in := range r.Influences {
		kind, err := lookup(influenceKindToProto, in.Kind, "influence kind")
		if err != nil {
			return nil, err
		}
		out.Influences = append(out.Influences, &sessionpb.RollInfluence{
			Kind: kind, SourceRef: in.SourceRef, SourceId: in.SourceID, Name: in.Name, SourceLabel: in.SourceLabel,
		})
	}
	for _, t := range r.Terms {
		out.Terms = append(out.Terms, termToProto(t))
	}
	return out, nil
}

// THE ROLL RECORD'S ONE WIRE SPELLING (rpg-project#643 B and D). A folded
// Roll's terms and modifiers go through termToProto and modifierToProto. A
// damage component carries its raw trace instead, so traceTermToProto and
// sourceModifierToProto first state it as the SDK states a folded term or
// modifier, then spell it the same way: the reroll record, the kept indices
// and every source's ref, id and label cannot differ between the two paths.

// termToProto spells one dice pool with its source and its reroll record.
func termToProto(t sdk.RollDiceTerm) *sessionpb.RollDiceTerm {
	return &sessionpb.RollDiceTerm{
		Count:       int32(t.Count),   //nolint:gosec // a pool's die count fits int32
		DieSize:     int32(t.DieSize), //nolint:gosec // a die size fits int32
		Faces:       int32s(t.Faces),
		Subtract:    t.Subtract,
		SourceRef:   t.SourceRef,
		SourceName:  t.SourceName,
		SourceId:    t.SourceID,
		SourceLabel: t.SourceLabel,
		Rolled:      int32s(t.Rolled),
		KeptIndices: int32s(t.KeptIndices),
		Rerolls:     rerollsToProto(t.Rerolls),
	}
}

// modifierToProto spells one flat contribution with its whole source.
func modifierToProto(m sdk.RollModifier) *sessionpb.RollModifier {
	return &sessionpb.RollModifier{
		Source: m.Source, Value: int32(m.Value), //nolint:gosec // a modifier fits int32
		SourceRef: m.SourceRef, SourceId: m.SourceID, SourceLabel: m.SourceLabel,
	}
}

// traceTermToProto spells a traced roll component's dice (a damage
// component's DiceTrace) as one term: its kept faces, the pool as first
// rolled, the kept indices, its sourced rerolls and its source. Nil when the
// component rolled no dice.
func traceTermToProto(rc sdk.RollComponent) *sessionpb.RollDiceTerm {
	d := rc.Dice
	if d == nil {
		return nil
	}
	kept := d.FinalRolls
	if len(d.KeptIndices) > 0 {
		kept = make([]int, 0, len(d.KeptIndices))
		for _, i := range d.KeptIndices {
			if i >= 0 && i < len(d.FinalRolls) {
				kept = append(kept, d.FinalRolls[i])
			}
		}
	}
	term := sdk.RollDiceTerm{
		Count: len(kept), DieSize: d.DieSize, Faces: kept, Subtract: rc.SubtractDice,
		SourceRef: rc.Source.Ref, SourceName: rc.Source.Name, SourceID: rc.Source.SourceID, SourceLabel: rc.Source.Label,
		Rolled: d.OriginalRolls, KeptIndices: d.KeptIndices,
	}
	for _, r := range d.Rerolls {
		term.Rerolls = append(term.Rerolls, sdk.RollReroll{
			DieIndex: r.DieIndex, Before: r.Before, After: r.After,
			SourceRef: r.Source.Ref, SourceName: r.Source.Name, SourceID: r.Source.SourceID, SourceLabel: r.Source.Label,
		})
	}
	return termToProto(term)
}

// sourceModifierToProto spells a traced component's flat contribution with
// its whole source: name, ref, id and label.
func sourceModifierToProto(src sdk.RollSource, value int) *sessionpb.RollModifier {
	return modifierToProto(sdk.RollModifier{
		Source: src.Name, Value: value, SourceRef: src.Ref, SourceID: src.SourceID, SourceLabel: src.Label,
	})
}

// rerollsToProto spells a term's sourced rerolls, nil for none.
func rerollsToProto(rerolls []sdk.RollReroll) []*sessionpb.RollReroll {
	if len(rerolls) == 0 {
		return nil
	}
	out := make([]*sessionpb.RollReroll, 0, len(rerolls))
	for _, r := range rerolls {
		out = append(out, &sessionpb.RollReroll{
			DieIndex:  int32(r.DieIndex), //nolint:gosec // a die index fits int32
			Before:    int32(r.Before),   //nolint:gosec // a face fits int32
			After:     int32(r.After),    //nolint:gosec // a face fits int32
			SourceRef: r.SourceRef, SourceName: r.SourceName, SourceId: r.SourceID, SourceLabel: r.SourceLabel,
		})
	}
	return out
}

// damageRollToProto spells one damage component's roll as a Roll (R17: every
// die an act carries is said one way). A traced component reads its kept
// faces, its notation's die count and size, and its modifier; Total is the
// trace's own subtotal, signed, plus the modifier. Its dice are one term
// naming the source that rolled them (rpg-project#643 A): which spell or
// weapon a damage part came from rides the roll, as it rode the v1 trace. A
// pre-trace component reads its legacy scalars the same way.
func damageRollToProto(c sdk.DamageComponent) *sessionpb.Roll {
	out := &sessionpb.Roll{}
	rc := c.Roll
	if rc.Dice != nil || rc.Modifier != nil {
		sum := 0
		if d := rc.Dice; d != nil {
			// The one wire spelling of a traced pool: kept faces, the pool as
			// first rolled, kept indices and sourced rerolls (rpg-project#643 B).
			term := traceTermToProto(rc)
			out.Dice = term.GetFaces()
			out.Terms = []*sessionpb.RollDiceTerm{term}
			expression := fmt.Sprintf("%dd%d", term.GetCount(), d.DieSize)
			sum = d.Subtotal
			if rc.SubtractDice {
				expression = "-" + expression
				sum = -sum
			}
			out.Expression = expression
		}
		if m := rc.Modifier; m != nil {
			sum += *m
			// Its source whole: name, ref, id and label (rpg-project#643 D).
			out.Modifiers = []*sessionpb.RollModifier{sourceModifierToProto(rc.Source, *m)}
			out.Expression = joinExpression(out.Expression, *m)
		}
		out.Total = int32(sum) //nolint:gosec // a damage total fits int32
		return out
	}
	// A component persisted before roll traces carries only scalars.
	out.Expression = joinExpression(c.Dice, c.FlatBonus)
	out.Dice = int32s(c.FinalRolls)
	sum := c.FlatBonus
	for _, f := range c.FinalRolls {
		sum += f
	}
	if c.FlatBonus != 0 {
		out.Modifiers = []*sessionpb.RollModifier{{Source: c.Source, Value: int32(c.FlatBonus)}} //nolint:gosec // a modifier fits int32
	}
	out.Total = int32(sum) //nolint:gosec // a damage total fits int32
	return out
}

// damageSourceRef is a damage component's provider ref: the legacy scalar on
// a pre-trace payload, else its roll's source (rpg-project#643 C). The SDK
// fills SourceRef only for payloads written before roll traces. The order
// cannot matter: the SDK refuses a component carrying both shapes, so at most
// one is set.
func damageSourceRef(c sdk.DamageComponent) string {
	if c.SourceRef != "" {
		return c.SourceRef
	}
	return c.Roll.Source.Ref
}

// joinExpression appends a flat modifier to a dice expression: "1d8" and 3
// is "1d8+3"; "" and -1 is "-1"; a zero modifier adds nothing.
func joinExpression(dice string, mod int) string {
	if mod == 0 {
		return dice
	}
	m := strconv.Itoa(mod)
	if mod > 0 && dice != "" {
		m = "+" + m
	}
	return strings.TrimSpace(dice + m)
}

func attackRefToProto(r sdk.AttackRef) (*sessionpb.AttackRef, error) {
	t, err := damageType(r.DamageType)
	if err != nil {
		return nil, err
	}
	return &sessionpb.AttackRef{Ref: r.Ref, Name: r.Name, DamageType: t}, nil
}

func reactionRefToProto(r *sdk.ReactionRef) *sessionpb.ReactionRef {
	if r == nil {
		return nil
	}
	return &sessionpb.ReactionRef{Ref: r.Ref, Name: r.Name}
}

func spellRefToProto(r sdk.SpellRef) *sessionpb.SpellRef {
	return &sessionpb.SpellRef{Ref: r.Ref, Name: r.Name}
}

// --- act bodies ------------------------------------------------------------

func walkToProto(b sdk.WalkAct) (*sessionpb.WalkAct, error) {
	var p parties
	out := &sessionpb.WalkAct{Mover: p.one(b.Mover), Cause: b.Cause}
	for _, s := range b.Path {
		out.Path = append(out.Path, &sessionpb.WalkStep{Cell: positionToProto(s.Cell), Doors: s.Doors})
	}
	return out, p.err
}

func attackToProto(b sdk.AttackAct) (*sessionpb.AttackAct, error) {
	var p parties
	outcome, err := lookup(attackOutcomeToProto, b.Outcome, "attack outcome")
	if err != nil {
		return nil, err
	}
	ref, err := attackRefToProto(b.Attack)
	if err != nil {
		return nil, err
	}
	out := &sessionpb.AttackAct{
		By: p.one(b.By), To: p.one(b.To), Attack: ref, Outcome: outcome, Critical: b.Critical,
		Damage: int32(b.Damage), //nolint:gosec // damage fits int32
		Roll:   p.roll(b.Roll), Reaction: reactionRefToProto(b.Reaction), PresentationId: b.PresentationID,
	}
	for _, c := range b.DamageComponents {
		t, err := damageType(c.DamageType)
		if err != nil {
			return nil, err
		}
		dc := &sessionpb.DamageComponent{
			Source: c.Source, SourceRef: damageSourceRef(c), DamageType: t, Roll: damageRollToProto(c),
		}
		if c.Multiplier != nil {
			m := *c.Multiplier
			dc.Multiplier = &m
		}
		out.DamageComponents = append(out.DamageComponents, dc)
	}
	if w := b.Ward; w != nil {
		out.Ward = &sessionpb.WardSave{Source: p.one(w.Source), Ability: w.Ability, Roll: p.rollValue(w.Roll)}
	}
	if s := b.Sequence; s != nil {
		out.Sequence = &sessionpb.SequenceRef{Ref: s.Ref, Name: s.Name}
	}
	return out, p.err
}

func castToProto(b sdk.CastAct) (*sessionpb.CastAct, error) {
	var p parties
	out := &sessionpb.CastAct{By: p.one(b.By), Spell: spellRefToProto(b.Spell), Targets: p.many(b.Targets)}
	for _, r := range b.Results {
		outcome, err := lookup(castOutcomeToProto, r.Outcome, "cast outcome")
		if err != nil {
			return nil, err
		}
		out.Results = append(out.Results, &sessionpb.CastResult{
			Target: p.one(r.Target), Outcome: outcome, Ability: r.Ability,
			Save: p.roll(r.Save), Source: p.one(r.Source),
		})
	}
	return out, p.err
}

func activateToProto(b sdk.ActivateAct) (*sessionpb.ActivateAct, error) {
	var p parties
	out := &sessionpb.ActivateAct{
		By: p.one(b.By), Ability: &sessionpb.AbilityRef{Ref: b.Ability.Ref, Name: b.Ability.Name}, Target: p.one(b.Target),
	}
	for _, r := range b.Results {
		resultDamage, err := damageType(sdk.DamageType(r.DamageType))
		if err != nil {
			return nil, err
		}
		res := &sessionpb.ActivationResult{
			Kind: r.Kind, Target: p.one(r.Target), Ref: r.Ref, Name: r.Name, SourceId: r.SourceID,
			Amount: int32(r.Amount), Requested: int32(r.Requested), //nolint:gosec // hit points fit int32
			Before: int32(r.Before), After: int32(r.After), //nolint:gosec // hit points fit int32
			Roll: p.roll(r.Roll), DamageType: resultDamage, Detail: r.Detail, Description: r.Description,
			Moved: int32(r.Moved), StoppedBy: r.StoppedBy, //nolint:gosec // a push distance fits int32
		}
		if st := r.Stabilization; st != nil {
			res.Stabilization = &sessionpb.Stabilization{
				Before: st.Before, After: st.After,
				HitPoints: int32(st.HitPoints), Successes: int32(st.Successes), //nolint:gosec // tallies fit int32
				Failures: int32(st.Failures), SuccessesNeeded: int32(st.SuccessesNeeded), //nolint:gosec // tallies fit int32
				FailuresRemaining: int32(st.FailuresRemaining), //nolint:gosec // tallies fit int32
				Stabilized:        st.Stabilized, Dead: st.Dead,
			}
		}
		out.Results = append(out.Results, res)
	}
	for _, s := range b.Saves {
		out.Saves = append(out.Saves, &sessionpb.SaveResult{
			Saver: p.one(s.Saver), Ability: s.Ability, Source: spellRefToProto(s.Source), Roll: p.rollValue(s.Roll),
		})
	}
	return out, p.err
}

func speakToProto(b sdk.SpeakAct) (*sessionpb.SpeakAct, error) {
	var p parties
	out := &sessionpb.SpeakAct{
		By: p.one(b.By), To: p.one(b.To), Option: b.Option, Line: b.Line, Skill: b.Skill, Roll: p.roll(b.Roll),
	}
	if b.Answer != nil {
		answer, err := tableAnswerToProto(*b.Answer)
		if err != nil {
			return nil, err
		}
		out.Answer = answer
	}
	return out, p.err
}

func deathSaveToProto(b sdk.DeathSaveAct) (*sessionpb.DeathSaveAct, error) {
	var p parties
	out := &sessionpb.DeathSaveAct{
		By: p.one(b.By), Roll: p.roll(b.Roll), Outcome: b.Outcome,
		SuccessesAdded: int32(b.SuccessesAdded), FailuresAdded: int32(b.FailuresAdded), //nolint:gosec // tallies fit int32
		Successes: int32(b.Successes), Failures: int32(b.Failures), //nolint:gosec // tallies fit int32
		SuccessesNeeded: int32(b.SuccessesNeeded), FailuresRemaining: int32(b.FailuresRemaining), //nolint:gosec // tallies fit int32
		Stabilized: b.Stabilized, Dead: b.Dead, Recovered: b.Recovered,
		HpRestored:   int32(b.HPRestored), //nolint:gosec // hit points fit int32
		Continuation: b.Continuation, PresentationId: b.PresentationID,
	}
	return out, p.err
}

func lootToProto(b sdk.LootAct) (*sessionpb.LootAct, error) {
	var p parties
	out := &sessionpb.LootAct{By: p.one(b.By), From: p.one(b.From)}
	return out, p.err
}

func holdToProto(b sdk.HoldAct) (*sessionpb.HoldAct, error) {
	var p parties
	out := &sessionpb.HoldAct{By: p.one(b.By), Prop: p.one(b.Prop)}
	return out, p.err
}

func dropToProto(b sdk.DropAct) (*sessionpb.DropAct, error) {
	var p parties
	out := &sessionpb.DropAct{By: p.one(b.By), Prop: p.one(b.Prop), Cell: positionToProto(b.Cell)}
	return out, p.err
}

func doorToProto(b sdk.DoorAct) (*sessionpb.DoorAct, error) {
	var p parties
	out := &sessionpb.DoorAct{By: p.one(b.By), Door: b.Door, State: b.State, Roll: p.roll(b.Roll)}
	return out, p.err
}

func equipToProto(b sdk.EquipAct) (*sessionpb.EquipAct, error) {
	var p parties
	out := &sessionpb.EquipAct{By: p.one(b.By), Slot: b.Slot, Item: b.Item, Change: b.Change}
	return out, p.err
}

func stayToProto(b sdk.StayAct) (*sessionpb.StayAct, error) {
	var p parties
	out := &sessionpb.StayAct{Member: p.one(b.Member), Cause: b.Cause, Why: b.Why}
	return out, p.err
}

func sneakToProto(b sdk.SneakAct) (*sessionpb.SneakAct, error) {
	var p parties
	out := &sessionpb.SneakAct{By: p.one(b.By), Watcher: p.one(b.Watcher), Roll: p.roll(b.Roll)}
	return out, p.err
}

func restToProto(b sdk.RestAct) (*sessionpb.RestAct, error) {
	var p parties
	out := &sessionpb.RestAct{
		By: p.one(b.By), Kind: b.Kind,
		HitPointsRestored: int32(b.HitPointsRestored), HitPoints: int32(b.HitPoints), //nolint:gosec // hit points fit int32
		HitDiceSpent: int32(b.HitDiceSpent), HitDiceReturned: int32(b.HitDiceReturned), //nolint:gosec // hit dice fit int32
		HitDiceRemaining:  int32(b.HitDiceRemaining), //nolint:gosec // hit dice fit int32
		ResourcesRefilled: b.ResourcesRefilled, Roll: p.roll(b.Roll),
	}
	return out, p.err
}

func tableAnswerToProto(b sdk.TableAnswer) (*sessionpb.TableAnswer, error) {
	var p parties
	out := &sessionpb.TableAnswer{
		Who: p.one(b.Who), Option: b.Option, Key: b.Key, Word: b.Word, Selector: b.Selector, Say: b.Say,
		Temperament: b.Temperament, Entry: int32(b.Entry), Roll: p.roll(b.Roll), //nolint:gosec // a table row fits int32
	}
	for _, c := range b.Candidates {
		out.Candidates = append(out.Candidates, &sessionpb.TableCandidate{
			Entry: int32(c.Entry), Weight: int32(c.Weight), //nolint:gosec // table weights fit int32
			Percent: int32(c.Percent), Loaded: int32(c.Loaded), //nolint:gosec // table weights fit int32
		})
	}
	if cue := b.Activity; cue != nil {
		out.Activity = &sessionpb.ActivityCue{Ref: cue.Ref, Cycles: cue.Cycles}
	}
	if d := b.Dealt; d != nil {
		out.Dealt = &sessionpb.Temper{Faction: p.one(d.Faction), Temper: d.Temper, Roll: p.rollValue(d.Roll)}
	}
	return out, p.err
}

func stateToProto(b sdk.StateChange) (*sessionpb.StateChange, error) {
	var p parties
	change, err := lookup(stateKindToProto, b.Change, "state kind")
	if err != nil {
		return nil, err
	}
	out := &sessionpb.StateChange{Party: p.one(b.Party), Change: change}
	if d := b.Downed; d != nil {
		out.Downed = &sessionpb.Downed{Kind: d.Kind}
	}
	if s := b.Stance; s != nil {
		out.Stance = &sessionpb.StanceTurn{Between: p.many(s.Between), Stance: s.Stance, Cause: s.Cause}
	}
	if c := b.Concentration; c != nil {
		out.Concentration = &sessionpb.ConcentrationEnded{Spell: spellRefToProto(c.Spell), Reason: c.Reason}
	}
	if c := b.Condition; c != nil {
		out.Condition = &sessionpb.ConditionLifted{Ref: c.Ref, Name: c.Name, SourceId: c.SourceID, Reason: c.Reason}
	}
	for _, g := range b.Experience {
		out.Experience = append(out.Experience, &sessionpb.ExperienceGrant{
			Character: g.Character, Amount: int32(g.Amount), Total: int32(g.Total), //nolint:gosec // experience fits int32
		})
	}
	return out, p.err
}

func perceptionToProto(b sdk.Perception) (*sessionpb.Perception, error) {
	var p parties
	sense, err := lookup(senseToProto, b.Sense, "sense")
	if err != nil {
		return nil, err
	}
	out := &sessionpb.Perception{Observer: p.one(b.Observer), Sense: sense}
	if s := b.Sighted; s != nil {
		out.Sighted = &sessionpb.SightChange{Gained: s.Gained, Lost: s.Lost, Changed: s.Changed, KnowledgeChanged: s.KnowledgeChanged}
	}
	if h := b.Heard; h != nil {
		out.Heard = &sessionpb.Sound{Kind: h.Kind, Cell: positionToProto(h.Cell), Hops: int32(h.Hops)} //nolint:gosec // hops fit int32
	}
	if d := b.Discovery; d != nil {
		out.Discovery = &sessionpb.DiscoveryCheck{Ability: d.Ability, Roll: p.roll(d.Roll)}
	}
	if r := b.Revealed; r != nil {
		out.Revealed = &sessionpb.SecretRevealed{
			Concealment: r.Concealment, Cells: positionsToProto(r.Cells), Props: propsToProto(r.Props),
			Doors: revealedDoorsToProto(r.Doors), Regions: regionsToProto(r.Regions),
			Boundaries: boundariesToProto(r.Boundaries), Segments: segmentsToProto(r.Segments),
			Sealed:                             positionsToProto(r.Sealed),
			StructuralWalls:                    structuralWallsToProto(r.StructuralWalls),
			StructuralDoors:                    structuralDoorsToProto(r.StructuralDoors),
			StructuralWallOpeningsReplacements: openingReplacementsToProto(r.StructuralWallOpeningsReplacements),
			PropPresentations:                  propPresentationsToProto(r.PropPresentations),
		}
	}
	if c := b.Cells; c != nil {
		out.Cells = &sessionpb.CellsLearned{
			Regions: regionsToProto(c.Regions), Scenery: positionsToProto(c.Scenery), Props: propsToProto(c.Props),
			Boundaries: boundariesToProto(c.Boundaries), Segments: segmentsToProto(c.Segments),
			Sealed: positionsToProto(c.Sealed), Doorways: doorwaysToProto(c.Doorways),
			Placed: placedToProto(c.Placed), Exits: exitsToProto(c.Exits),
			StructuralWalls:                    structuralWallsToProto(c.StructuralWalls),
			StructuralDoors:                    structuralDoorsToProto(c.StructuralDoors),
			StructuralWallOpeningsReplacements: openingReplacementsToProto(c.StructuralWallOpeningsReplacements),
			PropPresentations:                  propPresentationsToProto(c.PropPresentations),
		}
	}
	return out, p.err
}

func clockToProto(b sdk.ClockEntry) (*sessionpb.ClockEntry, error) {
	var p parties
	mark, err := lookup(clockMarkToProto, b.Mark, "clock mark")
	if err != nil {
		return nil, err
	}
	outcome, err := endingResult(b.Outcome)
	if err != nil {
		return nil, err
	}
	out := &sessionpb.ClockEntry{Mark: mark, Tick: b.Tick, Member: p.one(b.Member), Ending: b.Ending, Outcome: outcome}
	if t := b.Turn; t != nil {
		out.Turn = &sessionpb.TurnPassed{Member: p.one(t.Member), Next: p.one(t.Next)}
	}
	if f := b.Fight; f != nil {
		out.Fight = &sessionpb.FightFormed{Order: p.many(f.Order), Surprised: p.many(f.Surprised)}
	}
	if d := b.Dissolved; d != nil {
		out.Dissolved = &sessionpb.FightDissolved{Members: p.many(d.Members), Cause: d.Cause}
	}
	if d := b.Departure; d != nil {
		out.Departure = &sessionpb.Departure{Member: p.one(d.Member), Holding: d.Holding, Exit: d.Exit}
	}
	if a := b.Arrival; a != nil {
		out.Arrival = &sessionpb.Arrival{Id: a.ID, Kind: a.Kind, Cell: positionToProto(a.Cell)}
	}
	if w := b.Window; w != nil {
		win := &sessionpb.WindowOpened{Member: p.one(w.Member), From: positionToProto(w.From), To: positionToProto(w.To)}
		for _, o := range w.Windows {
			win.Windows = append(win.Windows, &sessionpb.WindowOffer{
				Audience: p.one(o.Audience), Reaction: reactionRefToProto(&o.Reaction),
			})
		}
		out.Window = win
	}
	if r := b.RollWindow; r != nil {
		out.RollWindow = &sessionpb.RollWindowOpened{
			PresentationId: r.PresentationID, Audience: p.one(r.Audience),
			Offer: reactionRefToProto(&r.Offer), Roll: p.roll(r.Roll),
		}
	}
	return out, p.err
}
