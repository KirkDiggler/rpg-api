package sessionv1alpha1

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

func TestPosition_RoundTrips(t *testing.T) {
	sdkPos := spatial.Position{X: 3.5, Y: -2}
	pbPos := positionToProto(sdkPos)
	require.Equal(t, 3.5, pbPos.GetX())
	require.Equal(t, -2.0, pbPos.GetY())
	require.Equal(t, sdkPos, positionFromProto(pbPos))
}

func TestPositionFromProto_Nil_ReturnsZeroValue(t *testing.T) {
	require.Equal(t, spatial.Position{}, positionFromProto(nil))
}

// The optional form must keep absence and the origin apart.
//
// If an unset field collapsed to (0,0) the two would be the same value on the
// way in, and every rule below that refuses a missing cell would instead be
// handed the middle of the map. The pointer is the only thing carrying that
// distinction across the boundary.
func TestPositionPtrFromProto_SeparatesAbsenceFromTheOrigin(t *testing.T) {
	require.Nil(t, positionPtrFromProto(nil), "an unset field is absent")

	atOrigin := positionPtrFromProto(&sessionpb.Position{X: 0, Y: 0})
	require.NotNil(t, atOrigin, "the origin is a cell somebody can point at")
	require.Equal(t, spatial.Position{X: 0, Y: 0}, *atOrigin)

	elsewhere := positionPtrFromProto(&sessionpb.Position{X: 4, Y: -2})
	require.NotNil(t, elsewhere)
	require.Equal(t, spatial.Position{X: 4, Y: -2}, *elsewhere)
}

func TestMemberKindToProto(t *testing.T) {
	require.Equal(t, sessionpb.MemberKind_MEMBER_KIND_PLAYER, memberKindToProto(sdk.KindPlayer))
	require.Equal(t, sessionpb.MemberKind_MEMBER_KIND_MONSTER, memberKindToProto(sdk.KindMonster))
	require.Equal(t, sessionpb.MemberKind_MEMBER_KIND_UNSPECIFIED, memberKindToProto(sdk.MemberKind("bogus")))
}

func TestGridKindToProto(t *testing.T) {
	require.Equal(t, sessionpb.GridKind_GRID_KIND_HEX, gridKindToProto(sdk.GridHex))
	require.Equal(t, sessionpb.GridKind_GRID_KIND_UNSPECIFIED, gridKindToProto(sdk.GridKind("bogus")))
}

// TestHexLayoutToProto covers both enum values (SDK -> proto is the only
// direction that exists: the wire never sends a layout back), for the reason
// TestGridKindToProto does: a mapping hard-coded to pointy would pass the
// tomb and mislabel every flat-top map in the game.
func TestHexLayoutToProto(t *testing.T) {
	require.Equal(t, sessionpb.HexLayout_HEX_LAYOUT_POINTY_TOP, hexLayoutToProto(sdk.HexLayoutPointyTop))
	require.Equal(t, sessionpb.HexLayout_HEX_LAYOUT_FLAT_TOP, hexLayoutToProto(sdk.HexLayoutFlatTop))
	require.Equal(t, sessionpb.HexLayout_HEX_LAYOUT_UNSPECIFIED, hexLayoutToProto(sdk.HexLayout("")),
		"a square map carries no layout, and the wire says UNSPECIFIED rather than guessing")
}

// TestAtlasToProto_CarriesLayout pins the field the first client had to
// measure a bounding box to recover (rpg-toolkit#1140). Copying it across is
// the whole job; omitting it would compile and draw the tomb sideways.
func TestAtlasToProto_CarriesLayout(t *testing.T) {
	out := AtlasToProto(&sdk.Atlas{Grid: sdk.GridHex, Layout: sdk.HexLayoutPointyTop})
	require.Equal(t, sessionpb.GridKind_GRID_KIND_HEX, out.GetGrid())
	require.Equal(t, sessionpb.HexLayout_HEX_LAYOUT_POINTY_TOP, out.GetLayout())
}

func TestClockKindToProto(t *testing.T) {
	require.Equal(t, sessionpb.ClockKind_CLOCK_KIND_WORLD, clockKindToProto(sdk.ClockWorld))
	require.Equal(t, sessionpb.ClockKind_CLOCK_KIND_TURN, clockKindToProto(sdk.ClockTurn))
	require.Equal(t, sessionpb.ClockKind_CLOCK_KIND_UNSPECIFIED, clockKindToProto(sdk.ClockKind("bogus")))
}

func TestVerbToProto(t *testing.T) {
	require.Equal(t, sessionpb.Verb_VERB_ATTACK, verbToProto(sdk.VerbAttack))
	require.Equal(t, sessionpb.Verb_VERB_MOVE, verbToProto(sdk.VerbMove))
	require.Equal(t, sessionpb.Verb_VERB_END_TURN, verbToProto(sdk.VerbEndTurn))
	// Unmapped, this would label every activation a barbarian can reach
	// VERB_UNSPECIFIED — not one mislabelled row but six, on the panel
	// rpg-project#300 exists to fill.
	require.Equal(t, sessionpb.Verb_VERB_ACTIVATE, verbToProto(sdk.VerbActivate))
	require.Equal(t, sessionpb.Verb_VERB_DEATH_SAVE, verbToProto(sdk.VerbDeathSave))
	// The first verb offered off the member's own turn (rpg-project#316 rung
	// 3). Unmapped, the one row a frozen Afford returns -- the ONLY row a
	// client can act on while the fight waits -- would arrive UNSPECIFIED,
	// and the panel would have nothing to draw but a dead table.
	require.Equal(t, sessionpb.Verb_VERB_REACT, verbToProto(sdk.VerbReact))
	// Speak (rpg-project#569), which replaced the Intimidate and Persuade
	// rows. Unmapped it would not be one wrong label but the whole menu
	// missing: a client drops a verb it cannot name rather than drawing it
	// wrong.
	require.Equal(t, sessionpb.Verb_VERB_SPEAK, verbToProto(sdk.VerbSpeak))
	require.Equal(t, sessionpb.Verb_VERB_UNSPECIFIED, verbToProto(sdk.Verb("bogus")))
}

// TestSlotToProto pins the one law this converter exists to keep: SlotNone
// ("") is an EXPLICIT SLOT_NONE, never left to fall through to
// SLOT_UNSPECIFIED. A declaration that lights no economy shape (a banked
// Extra Attack swing) is a fact about the price, not a producer defect --
// only an unrecognized slot string reaches UNSPECIFIED.
func TestSlotToProto(t *testing.T) {
	require.Equal(t, sessionpb.Slot_SLOT_NONE, slotToProto(sdk.SlotNone))
	require.Equal(t, sessionpb.Slot_SLOT_ACTION, slotToProto(sdk.SlotAction))
	require.Equal(t, sessionpb.Slot_SLOT_BONUS, slotToProto(sdk.SlotBonus))
	require.Equal(t, sessionpb.Slot_SLOT_REACTION, slotToProto(sdk.SlotReaction))
	require.Equal(t, sessionpb.Slot_SLOT_UNSPECIFIED, slotToProto(sdk.Slot("bogus")))
}

func TestTargetKindToProto(t *testing.T) {
	require.Equal(t, sessionpb.TargetKind_TARGET_KIND_NONE, targetKindToProto(sdk.TargetNone))
	require.Equal(t, sessionpb.TargetKind_TARGET_KIND_MEMBER, targetKindToProto(sdk.TargetMember))
	require.Equal(t, sessionpb.TargetKind_TARGET_KIND_PATH, targetKindToProto(sdk.TargetPath))
	require.Equal(t, sessionpb.TargetKind_TARGET_KIND_AREA, targetKindToProto(sdk.TargetArea))
	require.Equal(t, sessionpb.TargetKind_TARGET_KIND_CELL, targetKindToProto(sdk.TargetCell))
	require.Equal(t, sessionpb.TargetKind_TARGET_KIND_UNSPECIFIED, targetKindToProto(sdk.TargetKind("bogus")))
}

func TestTargetCandidateToProto_FieldForField(t *testing.T) {
	got := targetCandidateToProto(sdk.TargetCandidate{
		Member: "skeleton-1", Available: false,
		Why: &sdk.Shortfall{Reason: sdk.ShortfallTargetOutOfReach, Text: "target out of reach"},
	})
	require.Equal(t, "skeleton-1", got.GetMember())
	require.False(t, got.GetAvailable())
	require.Equal(t, sessionpb.ShortfallReason_SHORTFALL_REASON_TARGET_OUT_OF_REACH, got.GetWhy().GetReason())
	require.Equal(t, "target out of reach", got.GetWhy().GetText())
	require.NotNil(t, got.Effects, "no target answers stays non-nil empty")
	require.Empty(t, got.Effects)
	require.NotNil(t, got.HeldEffects, "no held rows stays non-nil empty")
	require.Empty(t, got.HeldEffects)
}

// TestTargetCandidateToProto_CarriesHeldEffects pins R18 across the boundary:
// the rows a target holds cross as full rows, every field, in the SDK's
// order, and stay apart from the per-target answers to the actor's rows.
func TestTargetCandidateToProto_CarriesHeldEffects(t *testing.T) {
	got := targetCandidateToProto(sdk.TargetCandidate{
		Member: "goblin-2", Available: true,
		Effects: []sdk.TargetEffect{
			{ID: "effect-a", State: sdk.EffectApplies, Reason: "answer a"},
		},
		HeldEffects: []sdk.EffectRow{
			{
				ID: "held-b", Ref: "ref:b", Name: "name b", Description: "description b",
				State: sdk.EffectDepends, Reason: "reason b",
				Participation: sdk.LaterChoice, Benefit: "benefit b",
			},
			{
				ID: "held-a", Ref: "ref:a", Name: "name a", Description: "description a",
				State: sdk.EffectApplies, Reason: "reason a",
				Participation: sdk.ContributesNow, Benefit: "benefit a",
			},
		},
	})
	require.Equal(t, []*sessionpb.EffectRow{
		{
			Id: "held-b", Ref: "ref:b", Name: "name b", Description: "description b",
			State: sessionpb.EffectState_EFFECT_STATE_DEPENDS, Reason: "reason b",
			Participation: sessionpb.EffectParticipation_EFFECT_PARTICIPATION_LATER_CHOICE, Benefit: "benefit b",
		},
		{
			Id: "held-a", Ref: "ref:a", Name: "name a", Description: "description a",
			State: sessionpb.EffectState_EFFECT_STATE_APPLIES, Reason: "reason a",
			Participation: sessionpb.EffectParticipation_EFFECT_PARTICIPATION_CONTRIBUTES_NOW, Benefit: "benefit a",
		},
	}, got.GetHeldEffects(), "every field, in the SDK's order")
	require.Equal(t, []*sessionpb.TargetEffect{
		{Id: "effect-a", State: sessionpb.EffectState_EFFECT_STATE_APPLIES, Reason: "answer a"},
	}, got.GetEffects(), "per-target answers are not joined with held rows")
}

// TestTargetCandidateToProto_HeldEffectsUnknownEnumsAreUnspecified: a held
// row goes through the shared row converter, so an unrecognized state or
// participation reaches UNSPECIFIED (a producer defect the client refuses),
// never a guessed value.
func TestTargetCandidateToProto_HeldEffectsUnknownEnumsAreUnspecified(t *testing.T) {
	got := targetCandidateToProto(sdk.TargetCandidate{
		Member: "goblin-2",
		HeldEffects: []sdk.EffectRow{
			{ID: "held-x", State: sdk.EffectState("bogus"), Participation: sdk.EffectParticipation("bogus")},
		},
	})
	require.Len(t, got.GetHeldEffects(), 1)
	require.Equal(t, sessionpb.EffectState_EFFECT_STATE_UNSPECIFIED, got.GetHeldEffects()[0].GetState())
	require.Equal(t, sessionpb.EffectParticipation_EFFECT_PARTICIPATION_UNSPECIFIED, got.GetHeldEffects()[0].GetParticipation())
}

func TestTargetCandidateToProto_CarriesTargetEffects(t *testing.T) {
	got := targetCandidateToProto(sdk.TargetCandidate{
		Member: "goblin-2", Available: true,
		Effects: []sdk.TargetEffect{
			{ID: "effect-b", State: sdk.EffectDepends, Reason: "reason b"},
			{ID: "effect-a", State: sdk.EffectApplies, Reason: "reason a", Benefit: "benefit a"},
		},
	})
	require.Equal(t, []*sessionpb.TargetEffect{
		{Id: "effect-b", State: sessionpb.EffectState_EFFECT_STATE_DEPENDS, Reason: "reason b"},
		{Id: "effect-a", State: sessionpb.EffectState_EFFECT_STATE_APPLIES, Reason: "reason a", Benefit: "benefit a"},
	}, got.GetEffects(), "every field, in the SDK's order")
}

func TestEffectStateToProto(t *testing.T) {
	require.Equal(t, sessionpb.EffectState_EFFECT_STATE_APPLIES, effectStateToProto(sdk.EffectApplies))
	require.Equal(t, sessionpb.EffectState_EFFECT_STATE_DOES_NOT_APPLY, effectStateToProto(sdk.EffectDoesNotApply))
	require.Equal(t, sessionpb.EffectState_EFFECT_STATE_DEPENDS, effectStateToProto(sdk.EffectDepends))
	require.Equal(t, sessionpb.EffectState_EFFECT_STATE_UNAVAILABLE, effectStateToProto(sdk.EffectUnavailable))
}

func TestEffectStateToProto_UnknownIsUnspecified(t *testing.T) {
	require.Equal(t, sessionpb.EffectState_EFFECT_STATE_UNSPECIFIED, effectStateToProto(sdk.EffectState("bogus")))
	require.Equal(t, sessionpb.EffectState_EFFECT_STATE_UNSPECIFIED, effectStateToProto(""))
}

func TestEffectParticipationToProto(t *testing.T) {
	require.Equal(t, sessionpb.EffectParticipation_EFFECT_PARTICIPATION_CONTRIBUTES_NOW, effectParticipationToProto(sdk.ContributesNow))
	require.Equal(t, sessionpb.EffectParticipation_EFFECT_PARTICIPATION_LATER_CHOICE, effectParticipationToProto(sdk.LaterChoice))
}

func TestEffectParticipationToProto_UnknownIsUnspecified(t *testing.T) {
	require.Equal(t, sessionpb.EffectParticipation_EFFECT_PARTICIPATION_UNSPECIFIED, effectParticipationToProto(sdk.EffectParticipation("bogus")))
	require.Equal(t, sessionpb.EffectParticipation_EFFECT_PARTICIPATION_UNSPECIFIED, effectParticipationToProto(""))
}

func TestDeclarationToProto_CarriesEffectRows(t *testing.T) {
	out := mustDeclarationToProto(t, sdk.Declaration{
		Verb: sdk.VerbAttack, Slot: sdk.SlotAction, Available: true, ID: "decl-attack-1",
		TargetKind: sdk.TargetMember,
		Effects: []sdk.EffectRow{
			{
				ID: "row-2", Ref: "ref-2", Name: "name 2", Description: "description 2",
				State: sdk.EffectDepends, Reason: "reason 2", Participation: sdk.ContributesNow,
			},
			{
				ID: "row-1", Ref: "ref-1", Name: "name 1", Description: "description 1",
				State: sdk.EffectApplies, Reason: "reason 1", Participation: sdk.LaterChoice, Benefit: "benefit 1",
			},
		},
	})
	require.Equal(t, []*sessionpb.EffectRow{
		{
			Id: "row-2", Ref: "ref-2", Name: "name 2", Description: "description 2",
			State: sessionpb.EffectState_EFFECT_STATE_DEPENDS, Reason: "reason 2",
			Participation: sessionpb.EffectParticipation_EFFECT_PARTICIPATION_CONTRIBUTES_NOW,
		},
		{
			Id: "row-1", Ref: "ref-1", Name: "name 1", Description: "description 1",
			State: sessionpb.EffectState_EFFECT_STATE_APPLIES, Reason: "reason 1",
			Participation: sessionpb.EffectParticipation_EFFECT_PARTICIPATION_LATER_CHOICE, Benefit: "benefit 1",
		},
	}, out.GetEffects(), "every field, in the SDK's order")
}

func TestDeclarationToProto_FieldForField(t *testing.T) {
	remaining := 0
	out := mustDeclarationToProto(t, sdk.Declaration{
		Verb: sdk.VerbAttack, Slot: sdk.SlotAction, Available: false, Remaining: &remaining,
		Why:        &sdk.Shortfall{Reason: sdk.ShortfallNoBudget, Currency: sdk.CurrencyAction, Needed: 1, Text: "action: 1 needed, 0 left"},
		ID:         "decl-attack-1",
		Attack:     &sdk.AttackRef{Ref: "dnd5e:weapons:longsword", Name: "Longsword", DamageType: sdk.DamageSlashing},
		TargetKind: sdk.TargetMember,
		Candidates: []sdk.TargetCandidate{
			{Member: "goblin-1", Available: true, Effects: []sdk.TargetEffect{{ID: "row-1", State: sdk.EffectApplies, Benefit: "benefit"}}},
			{Member: "skeleton-1", Available: false, Why: &sdk.Shortfall{Reason: sdk.ShortfallTargetOutOfReach, Text: "target out of reach"}},
		},
		MinTargets: 1,
		MaxTargets: 3,
		Cost: []sdk.CostComponent{
			{Currency: sdk.CurrencyAction, Needed: 1},
			{Currency: sdk.CurrencyCharges, Needed: 1, Label: "1st-level Spell Slots"},
		},
		Effects: []sdk.EffectRow{{
			ID: "row-1", Ref: "ref-1", Name: "name", Description: "description",
			State: sdk.EffectDepends, Reason: "reason", Participation: sdk.ContributesNow,
		}},
	})
	require.Equal(t, sessionpb.Verb_VERB_ATTACK, out.GetVerb())
	require.Equal(t, sessionpb.Slot_SLOT_ACTION, out.GetSlot())
	require.False(t, out.GetAvailable())
	require.NotNil(t, out.Remaining)
	require.Zero(t, out.GetRemaining(), "present zero must not collapse into absence")
	require.Equal(t, "action: 1 needed, 0 left", out.GetWhy().GetText())
	require.Equal(t, "decl-attack-1", out.GetId())
	require.Equal(t, "dnd5e:weapons:longsword", out.GetAttack().GetRef())
	require.Equal(t, "Longsword", out.GetAttack().GetName())
	require.Equal(t, sessionpb.DamageType_DAMAGE_TYPE_SLASHING, out.GetAttack().GetDamageType())
	require.Equal(t, sessionpb.TargetKind_TARGET_KIND_MEMBER, out.GetTargetKind())
	require.Len(t, out.GetCandidates(), 2)
	require.True(t, out.GetCandidates()[0].GetAvailable(), "candidate availability is independent of the declaration")
	require.Nil(t, out.GetCandidates()[0].GetWhy())
	require.False(t, out.GetCandidates()[1].GetAvailable())
	require.Equal(t, sessionpb.ShortfallReason_SHORTFALL_REASON_TARGET_OUT_OF_REACH, out.GetCandidates()[1].GetWhy().GetReason())
	require.Equal(t, int32(1), out.GetMinTargets())
	require.Equal(t, int32(3), out.GetMaxTargets())
	require.Equal(t, []*sessionpb.CostComponent{
		{Currency: sessionpb.Currency_CURRENCY_ACTION, Needed: 1},
		{Currency: sessionpb.Currency_CURRENCY_CHARGES, Needed: 1, Label: "1st-level Spell Slots"},
	}, out.GetCost())
	require.Equal(t, []*sessionpb.EffectRow{{
		Id: "row-1", Ref: "ref-1", Name: "name", Description: "description",
		State: sessionpb.EffectState_EFFECT_STATE_DEPENDS, Reason: "reason",
		Participation: sessionpb.EffectParticipation_EFFECT_PARTICIPATION_CONTRIBUTES_NOW,
	}}, out.GetEffects())
	require.Equal(t, []*sessionpb.TargetEffect{
		{Id: "row-1", State: sessionpb.EffectState_EFFECT_STATE_APPLIES, Benefit: "benefit"},
	}, out.GetCandidates()[0].GetEffects(), "a target answer rides only its own candidate")
	require.Empty(t, out.GetCandidates()[1].GetEffects())
}

func TestDeclarationToProto_PreservesOptionalAbsenceAndEmptyCandidates(t *testing.T) {
	out := mustDeclarationToProto(t, sdk.Declaration{
		Verb: sdk.VerbEndTurn, Slot: sdk.SlotNone, Available: true, ID: "decl-end-1",
		TargetKind: sdk.TargetNone, Candidates: []sdk.TargetCandidate{},
	})
	require.Nil(t, out.Remaining)
	require.Nil(t, out.Why)
	require.Nil(t, out.Attack)
	require.NotNil(t, out.Candidates)
	require.Empty(t, out.Candidates)
	require.NotNil(t, out.Effects, "no effect rows stays non-nil empty")
	require.Empty(t, out.Effects)
}

// TestDeclarationsToProto_NilOrEmpty_StaysNonNilEmpty pins the "no null,
// just an empty repeated" law: proto has no null-vs-empty distinction on a
// repeated field, so a nil SDK slice must still marshal as "[]", never be
// left nil at the Go level where a naive append-based converter would.
func TestDeclarationsToProto_NilOrEmpty_StaysNonNilEmpty(t *testing.T) {
	out := mustDeclarationsToProto(t, nil)
	require.NotNil(t, out)
	require.Empty(t, out)

	out = mustDeclarationsToProto(t, []sdk.Declaration{})
	require.NotNil(t, out)
	require.Empty(t, out)
}

func TestDeclarationsToProto_Populated(t *testing.T) {
	out := mustDeclarationsToProto(t, []sdk.Declaration{
		{Verb: sdk.VerbAttack, Slot: sdk.SlotNone, Available: true},
	})
	require.Len(t, out, 1)
	require.Equal(t, sessionpb.Slot_SLOT_NONE, out[0].GetSlot())
	require.True(t, out[0].GetAvailable())
}

func TestDissolveCauseFromProto_ByDecision(t *testing.T) {
	cause, err := dissolveCauseFromProto(sessionpb.DissolveKind_DISSOLVE_KIND_BY_DECISION)
	require.NoError(t, err)
	require.Equal(t, sdk.DissolveByDecision, cause.Kind())
}

func TestDissolveCauseFromProto_Unspecified_ReturnsErrNoCause(t *testing.T) {
	_, err := dissolveCauseFromProto(sessionpb.DissolveKind_DISSOLVE_KIND_UNSPECIFIED)
	require.ErrorIs(t, err, sdk.ErrNoCause)
}

func TestDissolveKindToProto(t *testing.T) {
	require.Equal(t, sessionpb.DissolveKind_DISSOLVE_KIND_BY_DECISION, dissolveKindToProto(sdk.DissolveByDecision))
	require.Equal(t, sessionpb.DissolveKind_DISSOLVE_KIND_BY_DEFEAT, dissolveKindToProto(sdk.DissolveByDefeat))
	// The camp turning (rpg-project#375): a missing case here would reach a
	// client as a fight that ended for no stated reason.
	require.Equal(t, sessionpb.DissolveKind_DISSOLVE_KIND_BY_STANCE, dissolveKindToProto(sdk.DissolveByStance))
	require.Equal(t, sessionpb.DissolveKind_DISSOLVE_KIND_UNSPECIFIED, dissolveKindToProto(sdk.DissolveKind("bogus")))
}

// TestDissolveCauseFromProto_ByStanceIsAcceptedLikeByDefeat pins the inbound
// half of the third cause: not a caller's to declare honestly, and accepted
// anyway for BY_DEFEAT's reason -- the SDK answers with what the composition
// actually did, whatever was handed in.
func TestDissolveCauseFromProto_ByStanceIsAcceptedLikeByDefeat(t *testing.T) {
	cause, err := dissolveCauseFromProto(sessionpb.DissolveKind_DISSOLVE_KIND_BY_STANCE)
	require.NoError(t, err)
	require.Equal(t, sdk.DissolveByStance, cause.Kind())

	_, err = dissolveCauseFromProto(sessionpb.DissolveKind_DISSOLVE_KIND_UNSPECIFIED)
	require.ErrorIs(t, err, sdk.ErrNoCause, "and nothing is guessed at")
}

func TestMemberToProto(t *testing.T) {
	m := sdk.Member{ID: "alice", Kind: sdk.KindPlayer, Position: spatial.Position{X: 3, Y: 4}}
	got := memberToProto(m)
	require.Equal(t, "alice", got.GetId())
	require.Equal(t, sessionpb.MemberKind_MEMBER_KIND_PLAYER, got.GetKind())
	require.Equal(t, 3.0, got.GetPosition().GetX())
	require.Equal(t, 4.0, got.GetPosition().GetY())
}

func TestCharacterStateToProto_Nil(t *testing.T) {
	require.Nil(t, characterStateToProto(nil))
}

func TestCharacterStateToProto_Populated(t *testing.T) {
	c := &sdk.CharacterState{ID: "char-1", Name: "Alice", Level: 3, Speed: 30, HitPoints: 20, MaxHitPoints: 24, ArmorClass: 15, ProficiencyBonus: 2}
	got := characterStateToProto(c)
	require.Equal(t, "char-1", got.GetId())
	require.Equal(t, "Alice", got.GetName())
	require.Equal(t, int32(3), got.GetLevel())
	require.Equal(t, int32(30), got.GetSpeed())
	require.Equal(t, int32(20), got.GetHitPoints())
	require.Equal(t, int32(24), got.GetMaxHitPoints())
	require.Equal(t, int32(15), got.GetArmorClass())
	require.Equal(t, int32(2), got.GetProficiencyBonus())
}

func TestDiscoveriesToProto_Nil_StaysNil(t *testing.T) {
	require.Nil(t, discoveriesToProto(nil))
}

func TestDiscoveriesToProto_Populated(t *testing.T) {
	in := map[string]sdk.Discovery{
		"alice": {
			FirstContact: []sdk.Report{{Subject: "goblin", Payload: []byte("x")}},
			Refreshed:    []string{"bob"},
			Faded:        []string{"carol"},
		},
	}
	got := discoveriesToProto(in)
	require.Len(t, got, 1)
	d := got["alice"]
	require.Len(t, d.GetFirstContact(), 1)
	require.Equal(t, "goblin", d.GetFirstContact()[0].GetSubject())
	require.Equal(t, []byte("x"), d.GetFirstContact()[0].GetPayload())
	require.Equal(t, []string{"bob"}, d.GetRefreshed())
	require.Equal(t, []string{"carol"}, d.GetFaded())
}

func TestOutcomeToProto_Nil(t *testing.T) {
	require.Nil(t, outcomeToProto(nil))
}

func TestOutcomeToProto_Populated(t *testing.T) {
	o := &sdk.Outcome{
		Ending: "victory",
		At:     42,
		Members: []sdk.MemberOutcome{
			{ID: "alice", Position: spatial.Position{X: 1, Y: 2}},
		},
	}
	got := outcomeToProto(o)
	require.Equal(t, "victory", got.GetEnding())
	require.Equal(t, uint64(42), got.GetAt())
	require.Len(t, got.GetMembers(), 1)
	require.Equal(t, "alice", got.GetMembers()[0].GetId())
	require.Equal(t, 1.0, got.GetMembers()[0].GetPosition().GetX())
	require.Equal(t, 2.0, got.GetMembers()[0].GetPosition().GetY())
}

func TestFormedToProto_Nil(t *testing.T) {
	require.Nil(t, formedToProto(nil))
}

func TestFormedToProto_Populated(t *testing.T) {
	f := &sdk.Formed{Order: []string{"alice", "goblin"}, Surprised: []string{"goblin"}, Seq: 7}
	got := formedToProto(f)
	require.Equal(t, []string{"alice", "goblin"}, got.GetOrder())
	require.Equal(t, []string{"goblin"}, got.GetSurprised())
	require.Equal(t, uint64(7), got.GetSeq())
}

func TestAtlasToProto_Nil(t *testing.T) {
	got := AtlasToProto(nil)
	require.NotNil(t, got)
	require.Empty(t, got.GetCells())
	require.Empty(t, got.GetDoorways())
}

func TestAtlasToProto_Populated(t *testing.T) {
	a := &sdk.Atlas{
		Grid:  sdk.GridHex,
		Cells: []spatial.Position{{X: 0, Y: 0}, {X: 1, Y: 0}},
		Props: []sdk.AtlasProp{
			// A pillar: blocks both. And a pile of bones: blocks NEITHER --
			// walked through and seen over. The second one is the case the
			// old Occluders list could not express at all, and the reason
			// this conversion carries two independent bools instead of
			// membership in a single list.
			{Ref: "pillar", At: spatial.Position{X: 1, Y: 1}, BlocksMovement: true, BlocksLineOfSight: true},
			{Ref: "bones", At: spatial.Position{X: 2, Y: 2}, BlocksMovement: false, BlocksLineOfSight: false},
			// Faced and offset (rpg-project#261): the authored word and the
			// authored nudge must reach the wire verbatim -- no angle math,
			// no snapping, no interpretation at this layer.
			{Ref: "brazier", At: spatial.Position{X: 3, Y: 3}, Facing: "ne", Offset: [3]float64{0.2, -0.1, 0.6}},
		},
		Boundaries: []sdk.AtlasBoundary{
			{From: spatial.Position{X: 0, Y: 0}, To: spatial.Position{X: 1, Y: 0}, BlocksMovement: true, BlocksLineOfSight: true},
			// Raised (rpg-project#273): the authored multiplier crosses
			// verbatim.
			{From: spatial.Position{X: 0, Y: 1}, To: spatial.Position{X: 1, Y: 1}, BlocksMovement: true, BlocksLineOfSight: true, Height: 2.5},
		},
		Doorways: []sdk.AtlasDoorway{
			{Door: "door-1", From: spatial.Position{X: 5, Y: 1}, To: spatial.Position{X: 6, Y: 1}},
		},
		Regions: []sdk.AtlasRegion{
			{ID: "hall", Name: "Hall", Cells: []spatial.Position{{X: 0, Y: 0}, {X: 1, Y: 0}}, Archetype: "crypt", Lighting: sdk.Lighting{Intensity: 0.4}},
			// Zero intensity is dark, which is an answer, not an absence: it
			// must reach the wire as a Lighting{0}, never a nil.
			{ID: "pit", Cells: []spatial.Position{{X: 2, Y: 2}}, Archetype: "cave", Lighting: sdk.Lighting{Intensity: 0}},
		},
	}
	got := AtlasToProto(a)
	require.Equal(t, sessionpb.GridKind_GRID_KIND_HEX, got.GetGrid())
	require.Len(t, got.GetCells(), 2)
	require.Len(t, got.GetProps(), 3)

	pillar := got.GetProps()[0]
	require.Equal(t, "pillar", pillar.GetRef())
	require.Equal(t, 1.0, pillar.GetAt().GetX())
	require.True(t, pillar.GetBlocksMovement())
	require.True(t, pillar.GetBlocksLineOfSight())
	require.Empty(t, pillar.GetFacing())
	require.Zero(t, pillar.GetOffsetX())
	require.Zero(t, pillar.GetOffsetY())
	require.Zero(t, pillar.GetOffsetZ())

	// The discriminating half. Both answers must arrive as FALSE rather than
	// as a prop that simply is not in a list: "blocks neither" and "nobody
	// said" are different facts, and collapsing them is exactly what the old
	// occluders field did. A conversion that dropped these bools, or that
	// filtered non-blocking props out entirely, passes every assertion above
	// and fails here.
	bones := got.GetProps()[1]
	require.Equal(t, "bones", bones.GetRef())
	require.False(t, bones.GetBlocksMovement())
	require.False(t, bones.GetBlocksLineOfSight())
	// Neither pillar nor bones authored a facing/offset -- "said nothing"
	// must arrive as the zero value, not as some other default.
	require.Empty(t, bones.GetFacing())
	require.Zero(t, bones.GetOffsetX())
	require.Zero(t, bones.GetOffsetY())
	require.Zero(t, bones.GetOffsetZ())

	brazier := got.GetProps()[2]
	require.Equal(t, "brazier", brazier.GetRef())
	require.Equal(t, "ne", brazier.GetFacing())
	require.InDelta(t, 0.2, brazier.GetOffsetX(), 1e-6)
	require.InDelta(t, -0.1, brazier.GetOffsetY(), 1e-6)
	// The raised third component (rpg-project#272): the authored height
	// above the floor crosses verbatim, and "authored nothing" arrives
	// as 0 (the pillar/bones assertions above).
	require.InDelta(t, 0.6, brazier.GetOffsetZ(), 1e-6)

	require.Len(t, got.GetBoundaries(), 2)
	require.True(t, got.GetBoundaries()[0].GetBlocksMovement())
	// No authored wall height = 0 on the wire: the reader renders the
	// STANDARD height and never multiplies by the raw value; the authored
	// multiplier crosses verbatim (rpg-project#273).
	require.Zero(t, got.GetBoundaries()[0].GetHeight())
	require.InDelta(t, 2.5, got.GetBoundaries()[1].GetHeight(), 1e-6)

	require.Len(t, got.GetDoorways(), 1)
	dw := got.GetDoorways()[0]
	require.Equal(t, "door-1", dw.GetConnection())
	require.Equal(t, 5.0, dw.GetFrom().GetX())
	require.Equal(t, 6.0, dw.GetTo().GetX())

	// Regions (rpg-project#256): copied cell for cell, archetype and
	// intensity verbatim.
	require.Len(t, got.GetRegions(), 2)
	hall := got.GetRegions()[0]
	require.Equal(t, "hall", hall.GetId())
	require.Equal(t, "Hall", hall.GetName())
	require.Equal(t, "crypt", hall.GetArchetype())
	require.Len(t, hall.GetCells(), 2)
	require.Equal(t, 1.0, hall.GetCells()[1].GetX())
	require.InDelta(t, 0.4, hall.GetLighting().GetIntensity(), 1e-9)
	pit := got.GetRegions()[1]
	require.NotNil(t, pit.GetLighting(), "dark is Lighting{0}, not a missing block")
	require.Equal(t, 0.0, pit.GetLighting().GetIntensity())
}

// TestAtlasToProto_DungeonKeyNamesTheContentAndTheSceneIsGone pins what
// replaced the scene carriage (rpg-project#479): the atlas names the content
// key its world was compiled from, the client fetches the room's appearance
// by that key, and the deprecated `room_scene_json` is left EMPTY by this
// converter on every path.
//
// The old field is still on the wire and must stay unset rather than
// repurposed: a client on the previous build reads an empty scene and draws
// the map alone, which is exactly what it did for every world authored
// before scenes existed. Filling it with anything -- a key, a placeholder,
// a re-marshaled fragment -- would make an old client draw a room nobody
// authored.
func TestAtlasToProto_DungeonKeyNamesTheContentAndTheSceneIsGone(t *testing.T) {
	got := AtlasToProto(&sdk.Atlas{
		Grid:       sdk.GridHex,
		Cells:      []spatial.Position{{X: 0, Y: 0}},
		DungeonKey: "workshop-room",
	})
	require.Equal(t, "workshop-room", got.GetDungeonKey(),
		"the key crosses verbatim -- it is what a client fetches the scene by")
	require.Empty(t, got.GetRoomSceneJson(),
		"the deprecated scene field is never filled, not even when a key is present")

	keyless := AtlasToProto(&sdk.Atlas{Grid: sdk.GridHex, Cells: []spatial.Position{{X: 0, Y: 0}}})
	require.Empty(t, keyless.GetDungeonKey(),
		"a world launched under no key names none rather than inventing a default")
	require.Empty(t, keyless.GetRoomSceneJson())
}

func TestWhereToProto_Nil(t *testing.T) {
	got := whereToProto(nil)
	require.NotNil(t, got)
}

func TestWhereToProto_Populated(t *testing.T) {
	got := whereToProto(&sdk.WhereOutput{Position: spatial.Position{X: 7, Y: 8}})
	require.Equal(t, 7.0, got.GetPosition().GetX())
	require.Equal(t, 8.0, got.GetPosition().GetY())
}

func TestSaveReportToProto(t *testing.T) {
	got := saveReportToProto(sdk.SaveReport{Written: []string{"encounter"}, Failed: []string{"character"}})
	require.Equal(t, []string{"encounter"}, got.GetWritten())
	require.Equal(t, []string{"character"}, got.GetFailed())
}

func TestDeliveryReportToProto(t *testing.T) {
	got := deliveryReportToProto(sdk.DeliveryReport{Events: 3, Failed: true})
	require.Equal(t, int32(3), got.GetEvents())
	require.True(t, got.GetFailed())
}

// A CALCULATION IS OPTIONAL AND ITS ABSENCE IS THE ANSWER; a trace, a
// component and a keep record are not, and asking this seam to convert one
// that is not there is a caller defect it refuses rather than answering with
// a nil nobody can tell from an unset field.
func TestRollTraceConverters_NilHandling(t *testing.T) {
	require.Nil(t, rollSourceToProto(nil))
	require.Nil(t, diceRerollToProto(nil))

	_, err := diceTraceToProto(nil)
	require.Error(t, err)
	_, err = rollComponentToProto(nil)
	require.Error(t, err)
	_, err = diceKeepToProto(nil)
	require.Error(t, err)

	calculation, err := rollCalculationToProto(nil)
	require.NoError(t, err)
	require.Nil(t, calculation, "a body that recorded no arithmetic leaves the wire field unset")
}

// d20WithKeep is one settled d20 pool carrying a keep record, the shape every
// scene below is about. The faces are the argument: a rule that kept one of
// two is the whole reason the record exists.
func d20WithKeep(faces []int, kept []int, keep *sdk.DiceKeep) *sdk.RollCalculation {
	subtotal := 0
	for _, index := range kept {
		subtotal = faces[index]
	}
	if len(kept) == 0 {
		subtotal = faces[0]
	}
	notation := "1d20"
	if len(faces) > 1 {
		notation = "2d20"
	}
	return &sdk.RollCalculation{
		Total: subtotal,
		Components: []sdk.RollComponent{{
			Source: sdk.RollSource{Ref: "dnd5e:skills:intimidation", Name: "Intimidation", SourceID: "char-1"},
			Dice: &sdk.DiceTrace{
				Notation: notation, DieSize: 20,
				OriginalRolls: faces, FinalRolls: faces, KeptIndices: kept,
				Subtotal: subtotal, Keep: keep,
			},
		}},
	}
}

// THE KEEP RECORD IS WHY A FACE COUNTED, and every field of it crosses: the
// rule as an enum this build knows, and both source lists with the entity
// that brought each one. The web draws a die in its owner's style from
// source_id, so dropping it would leave the client guessing from the beat's
// actor -- which is the client calculating (rpg-project#462, R7).
func TestDiceKeepToProto_CarriesEveryRuleAndBothSourceLists(t *testing.T) {
	help := sdk.RollSource{
		Ref: "dnd5e:actions:help", Name: "Help", Label: "ally", SourceID: "alice",
	}
	untrained := sdk.RollSource{
		Ref: "dnd5e:rules:untrained", Name: "Untrained", Label: "skill", SourceID: "char-1",
	}

	for _, tc := range []struct {
		name        string
		keep        sdk.DiceKeep
		wantRule    sessionpb.KeepRule
		wantGranted int
		wantImposed int
	}{
		{
			name:        "advantage names who granted it",
			keep:        sdk.DiceKeep{Rule: sdk.KeepAdvantage, Granted: []sdk.RollSource{help}},
			wantRule:    sessionpb.KeepRule_KEEP_RULE_ADVANTAGE,
			wantGranted: 1,
		},
		{
			name:        "disadvantage names who imposed it",
			keep:        sdk.DiceKeep{Rule: sdk.KeepDisadvantage, Imposed: []sdk.RollSource{untrained}},
			wantRule:    sessionpb.KeepRule_KEEP_RULE_DISADVANTAGE,
			wantImposed: 1,
		},
		{
			name: "cancellation names both sides",
			keep: sdk.DiceKeep{
				Rule:    sdk.KeepCancelled,
				Granted: []sdk.RollSource{help},
				Imposed: []sdk.RollSource{untrained},
			},
			wantRule:    sessionpb.KeepRule_KEEP_RULE_CANCELLED,
			wantGranted: 1,
			wantImposed: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := diceKeepToProto(&tc.keep)
			require.NoError(t, err)
			require.Equal(t, tc.wantRule, got.GetRule())
			require.Len(t, got.GetGranted(), tc.wantGranted)
			require.Len(t, got.GetImposed(), tc.wantImposed)

			for i, want := range tc.keep.Granted {
				require.Equal(t, want.Ref, got.GetGranted()[i].GetRef())
				require.Equal(t, want.Name, got.GetGranted()[i].GetName())
				require.Equal(t, want.Label, got.GetGranted()[i].GetLabel())
				require.Equal(t, want.SourceID, got.GetGranted()[i].GetSourceId(),
					"the entity whose rule threw the die is what a tray draws its style from")
			}
			for i, want := range tc.keep.Imposed {
				require.Equal(t, want.Ref, got.GetImposed()[i].GetRef())
				require.Equal(t, want.Name, got.GetImposed()[i].GetName())
				require.Equal(t, want.Label, got.GetImposed()[i].GetLabel())
				require.Equal(t, want.SourceID, got.GetImposed()[i].GetSourceId())
			}
		})
	}
}

// A STRAIGHT ROLL CARRIES NO RECORD, and the unset field is the answer rather
// than a defaulted one. UNSPECIFIED would be this seam claiming a rule exists
// that it could not name.
func TestDiceTraceToProto_StraightRollLeavesKeepUnset(t *testing.T) {
	got, err := diceTraceToProto(&sdk.DiceTrace{
		Notation: "1d20", DieSize: 20,
		OriginalRolls: []int{11}, FinalRolls: []int{11}, Subtotal: 11,
	})
	require.NoError(t, err)
	require.Nil(t, got.GetKeep(), "nobody touched this pool and the zero value says so")
	require.Empty(t, got.GetKeptIndices())
}

func TestRollCalculationToProto_FieldForFieldOrderPresenceAndIsolation(t *testing.T) {
	zero := 0
	negative := -3
	in := &sdk.RollCalculation{
		Components: []sdk.RollComponent{
			{
				Source: sdk.RollSource{
					Ref: "dnd5e:weapons:greatsword", Name: "Greatsword", Label: "primary", SourceID: "fighter-1",
				},
				Dice: &sdk.DiceTrace{
					Notation: "2d6", DieSize: 6,
					OriginalRolls: []int{1, 5},
					Rerolls: []sdk.DiceReroll{
						{
							DieIndex: 0, Before: 1, After: 4,
							Source: sdk.RollSource{
								Ref:  "dnd5e:conditions:fighting_style_great_weapon_fighting",
								Name: "Great Weapon Fighting", Label: "first reroll",
							},
						},
						{
							DieIndex: 1, Before: 5, After: 6,
							Source: sdk.RollSource{Ref: "test:conditions:second", Name: "Second", Label: "second reroll"},
						},
					},
					FinalRolls: []int{4, 6}, KeptIndices: []int{1, 0},
					Subtotal: 91,
				},
				Modifier: &zero,
			},
			{
				Source:       sdk.RollSource{Ref: "dnd5e:abilities:str", Name: "Strength", Label: "ability modifier"},
				Modifier:     &negative,
				SubtractDice: true,
			},
		},
		Total: 777,
	}

	wantZero := int32(0)
	wantNegative := int32(-3)
	want := &sessionpb.RollCalculation{
		Components: []*sessionpb.RollComponent{
			{
				Source: &sessionpb.RollSource{Ref: "dnd5e:weapons:greatsword", Name: "Greatsword", Label: "primary", SourceId: "fighter-1"},
				Dice: &sessionpb.DiceTrace{
					Notation: "2d6", DieSize: 6,
					OriginalRolls: []int32{1, 5},
					Rerolls: []*sessionpb.DiceReroll{
						{
							DieIndex: 0, Before: 1, After: 4,
							Source: &sessionpb.RollSource{
								Ref:  "dnd5e:conditions:fighting_style_great_weapon_fighting",
								Name: "Great Weapon Fighting", Label: "first reroll",
							},
						},
						{
							DieIndex: 1, Before: 5, After: 6,
							Source: &sessionpb.RollSource{Ref: "test:conditions:second", Name: "Second", Label: "second reroll"},
						},
					},
					FinalRolls: []int32{4, 6}, KeptIndices: []int32{1, 0},
					Subtotal: 91,
				},
				Modifier: &wantZero,
			},
			{
				Source:       &sessionpb.RollSource{Ref: "dnd5e:abilities:str", Name: "Strength", Label: "ability modifier"},
				Modifier:     &wantNegative,
				SubtractDice: true,
			},
		},
		Total: 777,
	}

	got, err := rollCalculationToProto(in)
	require.NoError(t, err)
	require.True(t, proto.Equal(want, got), "the converter copies every authored field without recomputing totals")
	require.NotNil(t, got.GetComponents()[0].Modifier, "a present zero modifier stays present")
	require.Equal(t, []int32{1, 0}, got.GetComponents()[0].GetDice().GetKeptIndices(), "producer order is preserved")
	require.Equal(t, int32(91), got.GetComponents()[0].GetDice().GetSubtotal(), "the authoritative subtotal is not resummed")
	require.Equal(t, int32(777), got.GetTotal(), "the authoritative total is not recalculated")

	got.Components[0].Source.Name = "mutated"
	got.Components[0].Dice.OriginalRolls[0] = 99
	got.Components[0].Dice.Rerolls[0].Source.Label = "mutated"
	*got.Components[0].Modifier = 9
	require.Equal(t, "Greatsword", in.Components[0].Source.Name)
	require.Equal(t, []int{1, 5}, in.Components[0].Dice.OriginalRolls)
	require.Equal(t, "first reroll", in.Components[0].Dice.Rerolls[0].Source.Label)
	require.Zero(t, *in.Components[0].Modifier, "the proto owns its optional scalar")
}

func richStruckEvent() sdk.Event {
	immunity := 0.0
	return sdk.Event{
		Kind: sdk.EventStruck,
		Body: sdk.StruckBody{
			Attacker: "char-1", Target: "goblin-1", Roll: 18, Total: 21, Against: 13, Damage: 6,
			Attack:   sdk.AttackRef{Ref: "dnd5e:weapons:longsword", Name: "Longsword", DamageType: sdk.DamageSlashing},
			Critical: true,
			// Roll-shaped facts live in Roll since session/v0.50.0
			// (toolkit#1470); the scalars beside it are a legacy READ path a
			// newly produced body never fills, so a fixture that used them
			// would be testing a decode of old storage rather than the
			// conversion this file is about.
			DamageComponents: []sdk.DamageComponent{
				{
					Source: "weapon",
					Roll: sdk.RollComponent{
						Source: sdk.RollSource{Ref: "dnd5e:weapons:longsword", Name: "Longsword"},
						Dice: &sdk.DiceTrace{
							Notation: "1d8", DieSize: 8,
							OriginalRolls: []int{4}, FinalRolls: []int{4}, Subtotal: 4,
						},
					},
					DamageType: sdk.DamageSlashing,
				},
				{
					Source: "monster_trait",
					Roll: sdk.RollComponent{
						Source: sdk.RollSource{Ref: "dnd5e:monster_traits:immunity", Name: "Immunity"},
					},
					DamageType: sdk.DamageSlashing, Multiplier: &immunity,
				},
			},
			// THE ATTRIBUTION IS ON THE D20 IT DECIDED (rpg-project#462, R1).
			// The body's parallel AdvantageSources/DisadvantageSources are
			// gone from the SDK; the rules that met over this swing ride the
			// first component's own keep record, naming both the rule and the
			// entity that brought it. Hidden granted and Dodging imposed, so
			// the pool was rolled straight and the record says why -- the one
			// case the older lists could not represent at all.
			Calculation: &sdk.RollCalculation{
				Total: 21,
				Components: []sdk.RollComponent{{
					Source: sdk.RollSource{
						Ref: "dnd5e:weapons:longsword", Name: "Longsword", SourceID: "char-1",
					},
					Dice: &sdk.DiceTrace{
						Notation: "1d20", DieSize: 20,
						OriginalRolls: []int{18}, FinalRolls: []int{18}, Subtotal: 18,
						Keep: &sdk.DiceKeep{
							Rule: sdk.KeepCancelled,
							Granted: []sdk.RollSource{{
								Ref: "dnd5e:conditions:hidden", Name: "Hidden", SourceID: "char-1",
							}},
							Imposed: []sdk.RollSource{{
								Ref: "dnd5e:conditions:dodging", Name: "Dodging", SourceID: "goblin-1",
							}},
						},
					},
				}},
			},
		},
	}
}

func TestStepsToProto(t *testing.T) {
	got := stepsToProto([]sdk.Step{{Position: spatial.Position{X: 1, Y: 1}, Seq: 5}})
	require.Len(t, got, 1)
	require.Equal(t, uint64(5), got[0].GetSeq())
	require.Equal(t, 1.0, got[0].GetPosition().GetX())
}

func TestSightingsToProto(t *testing.T) {
	got := sightingsToProto([]sdk.Sighting{
		{Subject: "goblin", Payload: []byte("x"), Channel: "sight", At: 3, CurrentVia: []string{"sight"}, Status: "live"},
	})
	require.Len(t, got, 1)
	require.Equal(t, "goblin", got[0].GetSubject())
	require.Equal(t, "live", got[0].GetStatus())
}

// TestSightingToProto_WithSeen and TestSightingToProto_WithoutSeen cover
// ADR-0041's typed sight-channel position (rpg-toolkit#1157, session
// v0.21.2): Seen is present iff channel provenance AND payload decoding both
// held, so a converter that always sets or always omits it passes only one
// of these two -- the discriminator the pair exists to catch. Payload stays
// asserted alongside so a converter that stopped copying the passthrough
// bytes while wiring in Seen would also fail here.
func TestSightingToProto_WithSeen(t *testing.T) {
	got := sightingToProto(sdk.Sighting{
		Subject: "skeleton-1",
		Payload: []byte("x"),
		Channel: "sight",
		Seen:    &sdk.Seen{Position: spatial.Position{X: 4, Y: 6}},
	})
	require.Equal(t, []byte("x"), got.GetPayload())
	require.NotNil(t, got.GetSeen())
	require.Equal(t, 4.0, got.GetSeen().GetPosition().GetX())
	require.Equal(t, 6.0, got.GetSeen().GetPosition().GetY())
}

func TestSightingToProto_WithoutSeen(t *testing.T) {
	got := sightingToProto(sdk.Sighting{Subject: "skeleton-1", Payload: []byte("x"), Channel: "memory"})
	require.Nil(t, got.GetSeen())
}

// TestReportToProto_WithSeen and TestReportToProto_WithoutSeen cover the
// same field on Report -- the shape Discovery.first_contact carries a
// Report in (discoveryToProto delegates to reportToProto per entry, so this
// exercises the Discovery path too without a fixture matrix).
func TestReportToProto_WithSeen(t *testing.T) {
	got := reportToProto(sdk.Report{
		Subject: "skeleton-1",
		Payload: []byte("x"),
		Seen:    &sdk.Seen{Position: spatial.Position{X: 4, Y: 6}},
	})
	require.Equal(t, []byte("x"), got.GetPayload())
	require.NotNil(t, got.GetSeen())
	require.Equal(t, 4.0, got.GetSeen().GetPosition().GetX())
	require.Equal(t, 6.0, got.GetSeen().GetPosition().GetY())
}

func TestReportToProto_WithoutSeen(t *testing.T) {
	got := reportToProto(sdk.Report{Subject: "skeleton-1", Payload: []byte("x")})
	require.Nil(t, got.GetSeen())
}

// TestStandingToProto covers both values plus the unrecognized fallback --
// two values, not a bool, and the fallback proves an unrecognized string
// reaches UNSPECIFIED rather than being guessed at.
func TestStandingToProto(t *testing.T) {
	require.Equal(t, sessionpb.Standing_STANDING_UP, standingToProto(sdk.StandingUp))
	require.Equal(t, sessionpb.Standing_STANDING_DOWNED, standingToProto(sdk.StandingDowned))
	require.Equal(t, sessionpb.Standing_STANDING_UNSPECIFIED, standingToProto(sdk.Standing("bogus")))
}

// TestSeenToProto_CarriesStanding pins the field ADR-0041's sight channel
// grew alongside Position (rpg-toolkit#1137): Seen is sight-channel
// knowledge, not a roster read, so Standing belongs here.
func TestSeenToProto_CarriesStanding(t *testing.T) {
	for _, tc := range []struct {
		standing sdk.Standing
		want     sessionpb.Standing
	}{
		{sdk.StandingUp, sessionpb.Standing_STANDING_UP},
		{sdk.StandingDowned, sessionpb.Standing_STANDING_DOWNED},
	} {
		t.Run(string(tc.standing), func(t *testing.T) {
			got := seenToProto(&sdk.Seen{Position: spatial.Position{X: 1, Y: 2}, Standing: &tc.standing})
			require.Equal(t, tc.want, got.GetStanding())
		})
	}
}

func TestSeenToProto_UnobservedStandingIsNotGuessed(t *testing.T) {
	got := seenToProto(&sdk.Seen{Position: spatial.Position{X: 1, Y: 2}})
	require.NotNil(t, got)
	require.Equal(t, sessionpb.Standing_STANDING_UNSPECIFIED, got.GetStanding())
	require.Equal(t, 1.0, got.GetPosition().GetX())
	require.Equal(t, 2.0, got.GetPosition().GetY())
}

// TestSeenToProto_Facing pins rpg-project#582 (sneaking R13): an observed
// facing crosses the wire as the same compass bearing the toolkit holds, and
// an unobserved one stays unset rather than becoming "facing north".
func TestSeenToProto_Facing(t *testing.T) {
	for _, tc := range []struct {
		name   string
		facing *float64
		want   *float64
	}{
		{name: "west passes through", facing: ptrFloat(270), want: ptrFloat(270)},
		{name: "north is a present zero", facing: ptrFloat(0), want: ptrFloat(0)},
		{name: "fraction survives", facing: ptrFloat(359.5), want: ptrFloat(359.5)},
		{name: "unobserved stays unset", facing: nil, want: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := seenToProto(&sdk.Seen{Position: spatial.Position{X: 1, Y: 2}, Facing: tc.facing})
			require.NotNil(t, got)
			if tc.want == nil {
				require.Nil(t, got.FacingDegrees)
				return
			}
			require.NotNil(t, got.FacingDegrees)
			require.Equal(t, *tc.want, got.GetFacingDegrees())
		})
	}
}

func ptrFloat(v float64) *float64 { return &v }

// TestSightingToProto_CarriesName pins rpg-dnd5e-web#564: names, not ids --
// anything an observer can sight, they can name.
func TestSightingToProto_CarriesName(t *testing.T) {
	got := sightingToProto(sdk.Sighting{Subject: "skeleton-1", Name: "skeleton-1"})
	require.Equal(t, "skeleton-1", got.GetName())
}

// TestSightingToProto_CarriesKind pins rpg-dnd5e-web#792: kind, like name,
// is not a perception question -- a sighted player projects as PLAYER so a
// client draws a player model, never a guessed monster ref.
func TestSightingToProto_CarriesKind(t *testing.T) {
	got := sightingToProto(sdk.Sighting{Subject: "char-123", Kind: sdk.KindPlayer})
	require.Equal(t, sessionpb.MemberKind_MEMBER_KIND_PLAYER, got.GetKind())

	got = sightingToProto(sdk.Sighting{Subject: "skeleton-1", Kind: sdk.KindMonster})
	require.Equal(t, sessionpb.MemberKind_MEMBER_KIND_MONSTER, got.GetKind())
}

// TestDamageTypeToProto covers all thirteen values plus the unrecognized
// fallback. A closed Go type to a closed enum, never a string round-trip
// (rpg-project#249 §6, Kirk) -- the fallback proves an unrecognized value
// reaches UNSPECIFIED rather than being coerced from its string form.
func TestDamageTypeToProto(t *testing.T) {
	tests := []struct {
		in   sdk.DamageType
		want sessionpb.DamageType
	}{
		{sdk.DamageAcid, sessionpb.DamageType_DAMAGE_TYPE_ACID},
		{sdk.DamageBludgeoning, sessionpb.DamageType_DAMAGE_TYPE_BLUDGEONING},
		{sdk.DamageCold, sessionpb.DamageType_DAMAGE_TYPE_COLD},
		{sdk.DamageFire, sessionpb.DamageType_DAMAGE_TYPE_FIRE},
		{sdk.DamageForce, sessionpb.DamageType_DAMAGE_TYPE_FORCE},
		{sdk.DamageLightning, sessionpb.DamageType_DAMAGE_TYPE_LIGHTNING},
		{sdk.DamageNecrotic, sessionpb.DamageType_DAMAGE_TYPE_NECROTIC},
		{sdk.DamagePiercing, sessionpb.DamageType_DAMAGE_TYPE_PIERCING},
		{sdk.DamagePoison, sessionpb.DamageType_DAMAGE_TYPE_POISON},
		{sdk.DamagePsychic, sessionpb.DamageType_DAMAGE_TYPE_PSYCHIC},
		{sdk.DamageRadiant, sessionpb.DamageType_DAMAGE_TYPE_RADIANT},
		{sdk.DamageSlashing, sessionpb.DamageType_DAMAGE_TYPE_SLASHING},
		{sdk.DamageThunder, sessionpb.DamageType_DAMAGE_TYPE_THUNDER},
		{sdk.DamageType("bogus"), sessionpb.DamageType_DAMAGE_TYPE_UNSPECIFIED},
	}
	for _, tt := range tests {
		t.Run(string(tt.in), func(t *testing.T) {
			require.Equal(t, tt.want, damageTypeToProto(tt.in))
		})
	}
}

func TestAttackRefToProto(t *testing.T) {
	got := attackRefToProto(sdk.AttackRef{Ref: "dnd5e:weapons:longsword", Name: "Longsword", DamageType: sdk.DamageSlashing})
	require.Equal(t, "dnd5e:weapons:longsword", got.GetRef())
	require.Equal(t, "Longsword", got.GetName())
	require.Equal(t, sessionpb.DamageType_DAMAGE_TYPE_SLASHING, got.GetDamageType())
}

func TestShortfallReasonToProto(t *testing.T) {
	tests := []struct {
		in   sdk.ShortfallReason
		want sessionpb.ShortfallReason
	}{
		{sdk.ShortfallNoBudget, sessionpb.ShortfallReason_SHORTFALL_REASON_NO_BUDGET},
		{sdk.ShortfallNotYourTurn, sessionpb.ShortfallReason_SHORTFALL_REASON_NOT_YOUR_TURN},
		{sdk.ShortfallNoTargetInReach, sessionpb.ShortfallReason_SHORTFALL_REASON_NO_TARGET_IN_REACH},
		{sdk.ShortfallDowned, sessionpb.ShortfallReason_SHORTFALL_REASON_DOWNED},
		{sdk.ShortfallUnreadable, sessionpb.ShortfallReason_SHORTFALL_REASON_UNREADABLE},
		{sdk.ShortfallTargetOutOfReach, sessionpb.ShortfallReason_SHORTFALL_REASON_TARGET_OUT_OF_REACH},
		// The ability's own precondition refusing (rpg-project#300). Distinct
		// from NO_BUDGET on purpose: nothing ran out, and telling a raging
		// barbarian to come back next turn is the wrong sentence.
		{sdk.ShortfallUnavailable, sessionpb.ShortfallReason_SHORTFALL_REASON_UNAVAILABLE},
		// The freeze (rpg-project#316 rung 3). It is what EVERY other verb
		// says while somebody is being asked whether they react, so leaving
		// it unmapped would blank the reason on the whole panel at the one
		// moment a player most needs to be told why nothing works.
		{sdk.ShortfallWindowOpen, sessionpb.ShortfallReason_SHORTFALL_REASON_WINDOW_OPEN},
		// A Speak row nobody in sight offers a line on (rpg-project#569).
		{sdk.ShortfallNoOption, sessionpb.ShortfallReason_SHORTFALL_REASON_NO_OPTION},
		{sdk.ShortfallReason("bogus"), sessionpb.ShortfallReason_SHORTFALL_REASON_UNSPECIFIED},
	}
	for _, tt := range tests {
		t.Run(string(tt.in), func(t *testing.T) {
			require.Equal(t, tt.want, shortfallReasonToProto(tt.in))
		})
	}
}

// TestCurrencyToProto pins the four values -- and, by its own doc, this is
// NOT Slot: Currency names which ledger a refusal drained, not which shape a
// declaration lights.
func TestCurrencyToProto(t *testing.T) {
	tests := []struct {
		in   sdk.Currency
		want sessionpb.Currency
	}{
		{sdk.CurrencyAction, sessionpb.Currency_CURRENCY_ACTION},
		{sdk.CurrencyBonus, sessionpb.Currency_CURRENCY_BONUS},
		{sdk.CurrencyReaction, sessionpb.Currency_CURRENCY_REACTION},
		{sdk.CurrencyMovement, sessionpb.Currency_CURRENCY_MOVEMENT},
		// The fifth (rpg-project#300): a ledger that ran out which is not one
		// of the turn's three. WHICH resource is named only in the shortfall's
		// text — this seam does not enumerate the rulebook's resource keys.
		{sdk.CurrencyCharges, sessionpb.Currency_CURRENCY_CHARGES},
		{sdk.Currency("bogus"), sessionpb.Currency_CURRENCY_UNSPECIFIED},
	}
	for _, tt := range tests {
		t.Run(string(tt.in), func(t *testing.T) {
			require.Equal(t, tt.want, currencyToProto(tt.in))
		})
	}
}

func TestShortfallToProto_Nil(t *testing.T) {
	require.Nil(t, shortfallToProto(nil))
}

func TestShortfallToProto_Populated(t *testing.T) {
	got := shortfallToProto(&sdk.Shortfall{
		Reason: sdk.ShortfallNoBudget, Currency: sdk.CurrencyAction, Needed: 1, Left: 0,
		Text: "action: 1 needed, 0 left",
	})
	require.Equal(t, sessionpb.ShortfallReason_SHORTFALL_REASON_NO_BUDGET, got.GetReason())
	require.Equal(t, sessionpb.Currency_CURRENCY_ACTION, got.GetCurrency())
	require.Equal(t, int32(1), got.GetNeeded())
	require.Equal(t, int32(0), got.GetLeft())
	require.Equal(t, "action: 1 needed, 0 left", got.GetText())
}

func TestParticipantToProto(t *testing.T) {
	got := participantToProto(sdk.Participant{
		Member: "char-1", Name: "Aldric", Kind: sdk.KindPlayer, Standing: sdk.StandingDowned, Active: true,
		LifeState:  sdk.LifeStateDying,
		DeathSaves: &sdk.DeathSaveProgress{Successes: 1, Failures: 2, SuccessesNeeded: 2, FailuresRemaining: 1},
	})
	require.Equal(t, "char-1", got.GetMember())
	require.Equal(t, "Aldric", got.GetName())
	require.Equal(t, sessionpb.MemberKind_MEMBER_KIND_PLAYER, got.GetKind())
	require.Equal(t, sessionpb.Standing_STANDING_DOWNED, got.GetStanding())
	require.True(t, got.GetActive())
	require.Equal(t, sessionpb.LifeState_LIFE_STATE_DYING, got.GetLifeState())
	require.Equal(t, int32(1), got.GetDeathSaves().GetSuccesses())
	require.Equal(t, int32(2), got.GetDeathSaves().GetFailures())
	require.Equal(t, int32(2), got.GetDeathSaves().GetSuccessesNeeded())
	require.Equal(t, int32(1), got.GetDeathSaves().GetFailuresRemaining())
}

func TestDeclarationToProto_CarriesDeathSaveIdentity(t *testing.T) {
	got := mustDeclarationToProto(t, sdk.Declaration{
		Verb: sdk.VerbDeathSave, Slot: sdk.SlotNone, Available: true,
		ID: "save-selector", TargetKind: sdk.TargetNone,
		DeathSave: &sdk.DeathSaveRef{Name: "Death Saving Throw"},
	})
	require.Equal(t, sessionpb.Verb_VERB_DEATH_SAVE, got.GetVerb())
	require.Equal(t, sessionpb.Slot_SLOT_NONE, got.GetSlot())
	require.Equal(t, sessionpb.TargetKind_TARGET_KIND_NONE, got.GetTargetKind())
	require.Equal(t, "Death Saving Throw", got.GetDeathSave().GetName())
	require.Nil(t, got.GetAttack())
	require.Nil(t, got.GetAbility())
}

// TestParticipantsToProto_NilOrEmpty_StaysNonNilEmpty mirrors
// TestDeclarationsToProto_NilOrEmpty_StaysNonNilEmpty's law: empty IS the
// answer on the world clock, never a null the wire cannot distinguish from
// "the server didn't say".
func TestParticipantsToProto_NilOrEmpty_StaysNonNilEmpty(t *testing.T) {
	out := participantsToProto(nil)
	require.NotNil(t, out)
	require.Empty(t, out)

	out = participantsToProto([]sdk.Participant{})
	require.NotNil(t, out)
	require.Empty(t, out)
}

func TestParticipantsToProto_Populated(t *testing.T) {
	out := participantsToProto([]sdk.Participant{
		{Member: "char-1", Name: "Aldric", Kind: sdk.KindPlayer, Standing: sdk.StandingUp, Active: true},
		{Member: "skeleton-1", Name: "skeleton-1", Kind: sdk.KindMonster, Standing: sdk.StandingDowned},
	})
	require.Len(t, out, 2)
	require.Equal(t, "Aldric", out[0].GetName())
	require.True(t, out[0].GetActive())
	require.Equal(t, sessionpb.Standing_STANDING_DOWNED, out[1].GetStanding())
	require.False(t, out[1].GetActive())
}

// TestDeclarationToProto_CarriesTheAbility pins the identity half of an
// Activate declaration.
//
// A client renders this VERBATIM — the panel reads ability.name for the
// button's label — so an ability that crossed without one is a button with no
// text. The same presence law Attack keeps: present exactly when the SDK
// carries one, absent otherwise, never defaulted.
func TestDeclarationToProto_CarriesTheAbility(t *testing.T) {
	out := mustDeclarationToProto(t, sdk.Declaration{
		Verb:      sdk.VerbActivate,
		Slot:      sdk.SlotBonus,
		Available: true,
		ID:        "v1.rage-selector",
		Ability:   &sdk.AbilityRef{Ref: "dnd5e:features:rage", Name: "Rage"},
	})

	require.Equal(t, sessionpb.Verb_VERB_ACTIVATE, out.GetVerb())
	require.Equal(t, sessionpb.Slot_SLOT_BONUS, out.GetSlot())
	require.NotNil(t, out.GetAbility())
	require.Equal(t, "dnd5e:features:rage", out.GetAbility().GetRef())
	require.Equal(t, "Rage", out.GetAbility().GetName())
	require.Nil(t, out.GetAttack(), "an activation carries no attack identity")
}

// And absent stays absent. Attack, Move and EndTurn carry no ability, and a
// defaulted empty AbilityRef would tell a client the producer forgot rather
// than that this verb has no such identity.
func TestDeclarationToProto_AttackCarriesNoAbility(t *testing.T) {
	out := mustDeclarationToProto(t, sdk.Declaration{
		Verb:   sdk.VerbAttack,
		Slot:   sdk.SlotAction,
		Attack: &sdk.AttackRef{Ref: "dnd5e:weapons:greataxe", Name: "Greataxe"},
	})

	require.Nil(t, out.GetAbility())
	require.NotNil(t, out.GetAttack())
}

// A charge shortfall crosses whole: the reason, the currency that is not one
// of the turn's three, the figures, and the ability's own words — which carry
// the one fact the structure deliberately does not, namely WHICH resource.
func TestShortfallToProto_ChargesCrossWhole(t *testing.T) {
	out := shortfallToProto(&sdk.Shortfall{
		Reason:   sdk.ShortfallNoBudget,
		Currency: sdk.CurrencyCharges,
		Needed:   1,
		Left:     0,
		Text:     "no rage uses remaining",
	})

	require.Equal(t, sessionpb.ShortfallReason_SHORTFALL_REASON_NO_BUDGET, out.GetReason())
	require.Equal(t, sessionpb.Currency_CURRENCY_CHARGES, out.GetCurrency())
	require.Equal(t, int32(1), out.GetNeeded())
	require.Equal(t, int32(0), out.GetLeft())
	require.Equal(t, "no rage uses remaining", out.GetText())
}

// TestReactionRefToProto pins the presence law: a reaction identity is a
// POINTER on Struck, Missed and a Declaration, and absent means "not taken as
// a reaction" rather than "taken as one whose name we lost". A zeroed message
// on the wire would read as the second, and a client drawing "reacted with
// ..." would label an ordinary swing.
func TestReactionRefToProto(t *testing.T) {
	got := reactionRefToProto(&sdk.ReactionRef{
		Ref: "dnd5e:conditions:opportunity_attack", Name: "Opportunity Attack",
	})
	require.Equal(t, "dnd5e:conditions:opportunity_attack", got.GetRef())
	require.Equal(t, "Opportunity Attack", got.GetName())

	require.Nil(t, reactionRefToProto(nil), "absent must stay absent, never a zeroed message")
}

// TestDeclarationToProto_CarriesTheReaction covers the REACT row Afford
// returns while a window is open (rpg-project#316 rung 3): what the member is
// being asked to react with, and the mover as the sole candidate. The two
// choices are NOT here -- strike and hold are implied by the verb and travel
// as ReactChoice on the request.
func TestDeclarationToProto_CarriesTheReaction(t *testing.T) {
	out := mustDeclarationToProto(t, sdk.Declaration{
		Verb:       sdk.VerbReact,
		Slot:       sdk.SlotReaction,
		Available:  true,
		ID:         "decl-react-1",
		TargetKind: sdk.TargetMember,
		Candidates: []sdk.TargetCandidate{{Member: "skel-1", Available: true}},
		Reaction: &sdk.ReactionRef{
			Ref: "dnd5e:conditions:opportunity_attack", Name: "Opportunity Attack",
		},
	})

	require.Equal(t, sessionpb.Verb_VERB_REACT, out.GetVerb())
	require.Equal(t, sessionpb.Slot_SLOT_REACTION, out.GetSlot())
	require.Equal(t, "Opportunity Attack", out.GetReaction().GetName())
	require.Len(t, out.GetCandidates(), 1)
	require.Equal(t, "skel-1", out.GetCandidates()[0].GetMember())
}

// A row that is not a reaction carries no reaction, and this is the half that
// makes the field mean something: every OTHER declaration Afford compiles is
// on the same wire message.
func TestDeclarationToProto_OmitsTheReactionOnAnOrdinaryRow(t *testing.T) {
	out := mustDeclarationToProto(t, sdk.Declaration{Verb: sdk.VerbMove, ID: "decl-move-1"})
	require.Nil(t, out.GetReaction())
}

// TestDeclarationToProto_CarriesTheSpell covers the CAST row (rpg-project#405).
// The verb alone cannot say which cantrip a row casts -- one verb compiles one
// row per castable cantrip -- so this field is what lets a dock label the
// button "Vicious Mockery" instead of "Cast".
func TestDeclarationToProto_CarriesTheSpell(t *testing.T) {
	out := mustDeclarationToProto(t, sdk.Declaration{
		Verb:       sdk.VerbCast,
		Slot:       sdk.SlotAction,
		Available:  true,
		ID:         "decl-cast-1",
		TargetKind: sdk.TargetMember,
		Candidates: []sdk.TargetCandidate{{Member: "skel-1", Available: true}},
		Spell: &sdk.SpellRef{
			Ref: "dnd5e:spells:vicious-mockery", Name: "Vicious Mockery",
		},
	})

	require.Equal(t, sessionpb.Verb_VERB_CAST, out.GetVerb())
	require.Equal(t, sessionpb.Slot_SLOT_ACTION, out.GetSlot(),
		"a cantrip costs one action, and the row shows the price the door charges")
	require.Equal(t, "dnd5e:spells:vicious-mockery", out.GetSpell().GetRef())
	require.Equal(t, "Vicious Mockery", out.GetSpell().GetName(),
		"the content authors the label; a client never derives it from the ref")
}

// The other half, and the one that makes the field mean anything: a row that
// is not a cast carries NO spell. A zeroed SpellRef here would read as a
// spell nobody named, on every Move and Attack row on the same panel.
func TestDeclarationToProto_OmitsTheSpellOnAnOrdinaryRow(t *testing.T) {
	out := mustDeclarationToProto(t, sdk.Declaration{Verb: sdk.VerbMove, ID: "decl-move-2"})
	require.Nil(t, out.GetSpell())
}

// TestParticipantToProto_CarriesConcentrating pins R11's one bool
// (rpg-project#407). Unfilled, every roster row reads as nobody
// concentrating, and the break beat above lands on a table that was never
// shown the setup.
func TestParticipantToProto_CarriesConcentrating(t *testing.T) {
	holding := participantToProto(sdk.Participant{Member: "bard-1", Concentrating: true})
	require.True(t, holding.GetConcentrating())

	// The other half, and the one that makes the flag mean anything: a member
	// holding nothing says so, rather than every row reading alike.
	idle := participantToProto(sdk.Participant{Member: "fighter-1"})
	require.False(t, idle.GetConcentrating())
}

// sdkTargetKinds is every selector shape the SDK declares.
//
// MAINTAINED BY HAND, and that is the honest limitation: Go cannot enumerate a
// string-const type, so adding a kind to the SDK without adding it here leaves
// this list short. It is still worth having — it is the only place that states
// what the full set is — but the guarantee lives in the test below it, which
// needs no maintenance at all.
var sdkTargetKinds = []sdk.TargetKind{
	sdk.TargetNone,
	sdk.TargetMember,
	sdk.TargetPath,
	sdk.TargetArea,
	sdk.TargetCell,
}

// TestEverySDKTargetKindReachesTheWire asserts no selector shape degrades to
// UNSPECIFIED on its way to a client.
//
// An SDK kind with no case in targetKindToProto falls to UNSPECIFIED, and the
// client has no branch for that: the row draws, the click does nothing, and
// nothing is logged anywhere. That is not hypothetical — it is exactly what
// TARGET_KIND_AREA did before rpg-api-protos#322, and it cost a walk to find.
func TestEverySDKTargetKindReachesTheWire(t *testing.T) {
	for _, kind := range sdkTargetKinds {
		t.Run(string(kind), func(t *testing.T) {
			require.NotEqual(t, sessionpb.TargetKind_TARGET_KIND_UNSPECIFIED,
				targetKindToProto(kind),
				"%q reaches the client as UNSPECIFIED, which it cannot dispatch on", kind)
		})
	}
}

// TestEveryProtoTargetKindIsProducedBySomeSDKKind is the half that needs no
// maintenance.
//
// The proto enum is GENERATED, so its value set is enumerable at runtime
// through TargetKind_name. A value declared on the wire that nothing can
// produce is a contract the seam cannot honor — either a dead value, or a
// mapping somebody forgot. Either way this fails without anyone remembering to
// update a list.
func TestEveryProtoTargetKindIsProducedBySomeSDKKind(t *testing.T) {
	produced := make(map[sessionpb.TargetKind]sdk.TargetKind, len(sdkTargetKinds))
	for _, kind := range sdkTargetKinds {
		produced[targetKindToProto(kind)] = kind
	}

	for value, name := range sessionpb.TargetKind_name {
		kind := sessionpb.TargetKind(value)
		if kind == sessionpb.TargetKind_TARGET_KIND_UNSPECIFIED {
			continue
		}
		require.Containsf(t, produced, kind,
			"%s is declared in the proto and no SDK target kind maps to it", name)
	}
}

// TestTargetKindAreaCrossesTheSeam pins the value this walk was about, by name
// rather than only by the sweeps above.
func TestTargetKindAreaCrossesTheSeam(t *testing.T) {
	require.Equal(t, sessionpb.TargetKind_TARGET_KIND_AREA, targetKindToProto(sdk.TargetArea))
}

// TestTargetKindCellCrossesTheSeam pins the shape an AIMED cast announces.
//
// CELL is the kind a client cannot guess at. AREA fires the moment it is
// armed; CELL has to wait for a ground click, and a client that reads
// UNSPECIFIED here would fire the cube at nothing. The sweeps above would
// catch it, this says which value it is.
func TestTargetKindCellCrossesTheSeam(t *testing.T) {
	require.Equal(t, sessionpb.TargetKind_TARGET_KIND_CELL, targetKindToProto(sdk.TargetCell))
}

// sdkUnresolvedReasons is every reason the SDK declares. Hand-kept, with the
// same limitation sdkTargetKinds has and the same mechanical partner below.
var sdkUnresolvedReasons = []sdk.UnresolvedReason{
	sdk.UnresolvedNoSheet,
}

// TestEveryProtoUnresolvedReasonIsProduced needs no list maintained.
//
// The proto enum is generated, so its values are enumerable at runtime. A wire
// value nothing can produce is a contract the seam cannot honor — and this is
// the guard TargetKind did not have, which is why an area cast reached a client
// as UNSPECIFIED and cost a walk to find.
func TestEveryProtoUnresolvedReasonIsProduced(t *testing.T) {
	produced := make(map[sessionpb.UnresolvedReason]sdk.UnresolvedReason, len(sdkUnresolvedReasons))
	for _, reason := range sdkUnresolvedReasons {
		produced[unresolvedReasonToProto(reason)] = reason
	}

	for value, name := range sessionpb.UnresolvedReason_name {
		reason := sessionpb.UnresolvedReason(value)
		if reason == sessionpb.UnresolvedReason_UNRESOLVED_REASON_UNSPECIFIED {
			continue
		}
		require.Containsf(t, produced, reason,
			"%s is declared in the proto and no SDK reason maps to it", name)
	}
}

// TestACaughtMemberCrossesTheSeamWhole asserts member, kind and reason all
// survive the conversion.
//
// The whole point of the field: a shopkeeper standing in a thunderclap must
// reach the client as somebody, not as the absence of a target. Member, kind
// and reason all have to survive, or the client can see that SOMETHING was
// caught without being able to say what or why.
func TestACaughtMemberCrossesTheSeamWhole(t *testing.T) {
	got := caughtMembersToProto([]sdk.CaughtMember{{
		Member: "demo-merchant-1", Kind: sdk.KindWorld, Reason: sdk.UnresolvedNoSheet,
	}})

	require.Len(t, got, 1)
	require.Equal(t, "demo-merchant-1", got[0].GetMember())
	require.Equal(t, sessionpb.MemberKind_MEMBER_KIND_WORLD, got[0].GetKind())
	require.Equal(t, sessionpb.UnresolvedReason_UNRESOLVED_REASON_NO_SHEET, got[0].GetReason())
}

// TestCatchingNobodyIsNilRatherThanEmpty — most casts catch nobody this way and
// every cast that is not an area catches nobody at all, so an empty slice would
// be a second way of saying the same nothing.
func TestCatchingNobodyIsNilRatherThanEmpty(t *testing.T) {
	require.Nil(t, caughtMembersToProto(nil))
	require.Nil(t, caughtMembersToProto([]sdk.CaughtMember{}))
}

// TestDeclarationToProto_CarriesTheMenuInContentOrder covers Command's cast
// menu (rpg-project#442). The words are an input the request brings back, not
// a row per word, so one offer lists what may be chosen and the client draws
// that list.
//
// ORDER IS THE CONTENT'S. Approach, Flee, Grovel is how the spell authored
// them, and a converter that sorted or regrouped would be editing a spell's
// own presentation from four layers away.
func TestDeclarationToProto_CarriesTheMenuInContentOrder(t *testing.T) {
	out := mustDeclarationToProto(t, sdk.Declaration{
		Verb:       sdk.VerbCast,
		Slot:       sdk.SlotAction,
		Available:  true,
		ID:         "decl-command-1",
		TargetKind: sdk.TargetMember,
		Spell:      &sdk.SpellRef{Ref: "dnd5e:spells:command", Name: "Command"},
		Options: []sdk.CastOption{
			{ID: "approach", Label: "Approach"},
			{ID: "flee", Label: "Flee"},
			{ID: "grovel", Label: "Grovel"},
		},
	})

	require.Equal(t, []*sessionpb.CastOption{
		{Id: "approach", Label: "Approach"},
		{Id: "flee", Label: "Flee"},
		{Id: "grovel", Label: "Grovel"},
	}, out.GetOptions(), "the whole menu crosses in the order the spell authored")
}

// The other half, and the one that makes the menu mean something: a row that
// offers no choice lists nothing.
//
// A row listing options REQUIRES one back and a row listing none REFUSES one,
// so an invented entry here would make session refuse a cast this layer had
// promised was answerable. Every verb but Cast and every spell but Command is
// on this side of the law today.
func TestDeclarationToProto_CarriesNoMenuOnARowThatOffersNoChoice(t *testing.T) {
	out := mustDeclarationToProto(t, sdk.Declaration{
		Verb:       sdk.VerbCast,
		Slot:       sdk.SlotAction,
		Available:  true,
		ID:         "decl-cast-2",
		TargetKind: sdk.TargetMember,
		Spell:      &sdk.SpellRef{Ref: "dnd5e:spells:vicious-mockery", Name: "Vicious Mockery"},
	})
	require.Empty(t, out.GetOptions(), "a spell with no menu offers no word to choose")
}

// mustDeclarationToProto is declarationToProto for a scene that pins a
// projection rather than the malformed-menu refusal.
func mustDeclarationToProto(t *testing.T, d sdk.Declaration) *sessionpb.Declaration {
	t.Helper()
	out, err := declarationToProto(d)
	require.NoError(t, err, "this scene pins a projection, not a refusal")
	return out
}

// mustDeclarationsToProto is mustDeclarationToProto's plural.
func mustDeclarationsToProto(t *testing.T, ds []sdk.Declaration) []*sessionpb.Declaration {
	t.Helper()
	out, err := declarationsToProto(ds)
	require.NoError(t, err, "this scene pins a projection, not a refusal")
	return out
}

// projected maps a wire list through one fact about each element, so a test
// can assert a repeated field by what it carries -- ids, coordinates -- in one
// comparison that pins CONTENT AND ORDER together. Asserting a length beside
// the elements would be a second, weaker statement of the same thing, and the
// kind that gets bumped rather than read when the list changes.
func projected[T any, R any](in []T, fact func(T) R) []R {
	out := make([]R, len(in))
	for i, v := range in {
		out[i] = fact(v)
	}
	return out
}

// wireXY and wireQR read one wire point as a comparable pair. Position is
// dungeon-absolute cell space; AxialPoint is the fractional axial frame a wall
// segment's ends live in. They are deliberately separate: the two frames are
// not interchangeable and a test that mixed them would compare a cell against
// a wall corner.
func wireXY(p *sessionpb.Position) [2]float64 { return [2]float64{p.GetX(), p.GetY()} }

func wireQR(p *sessionpb.AxialPoint) [2]float64 { return [2]float64{p.GetQ(), p.GetR()} }

// TestAtlasToProto_APlacedFootprintCrossesTheSeamWhole is the wire half of
// rpg-api-protos#351: every field of one placement reaches GetAtlasResponse,
// and none of them is derived here.
//
// EVERY NUMBER IS DIFFERENT, on purpose. Width and depth are the pair a
// converter is most likely to swap -- the authored dialect really does swap
// those names one layer down -- and a rectangle three feet across by seven
// deep fails loudly where a square one would pass a converter that drew the
// table turned ninety degrees. The same holds for the origin against the
// local offset, and for x against y inside each of them.
//
// MUTATION-CHECKED on `cells`, which is the field the wire exists for:
// dropping the `Cells:` line from atlasPlacedPropToProto fails the cells
// assertions below, and dropping the whole `Placed:` line from AtlasToProto
// fails the length assertion.
func TestAtlasToProto_APlacedFootprintCrossesTheSeamWhole(t *testing.T) {
	got := AtlasToProto(&sdk.Atlas{
		Grid:  sdk.GridHex,
		Cells: []spatial.Position{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}},
		Placed: []sdk.AtlasPlacedProp{{
			ID: "long-table",
			Placement: sdk.FootprintPlacement{
				Width:       3,
				Depth:       7,
				Origin:      sdk.FootprintPoint{X: 11, Y: 13},
				Facing:      29,
				LocalOffset: sdk.FootprintPoint{X: 2, Y: -5},
			},
			BlocksMovement:    true,
			BlocksLineOfSight: false,
			Holdable:          true,
			Cells:             []spatial.Position{{X: 1, Y: 0}, {X: 1, Y: 1}},
		}},
	})

	require.Len(t, got.GetPlaced(), 1)
	placed := got.GetPlaced()[0]
	require.Equal(t, "long-table", placed.GetId(),
		"the author's name is the only handle Hold can use")

	// The pose. Across-the-facing is 3 and along-it is 7; a converter that
	// re-swapped the pair the authored dialect already swapped fails here.
	pose := placed.GetPlacement()
	require.NotNil(t, pose, "a placement always carries its rectangle")
	require.Equal(t, 3.0, pose.GetWidth(), "width lies ACROSS the facing")
	require.Equal(t, 7.0, pose.GetDepth(), "depth lies ALONG it")
	require.Equal(t, 29.0, pose.GetFacing(), "degrees from east, verbatim -- nothing snaps to a hex edge")
	require.Equal(t, 11.0, pose.GetOrigin().GetX())
	require.Equal(t, 13.0, pose.GetOrigin().GetY())
	require.Equal(t, 2.0, pose.GetLocalOffset().GetX())
	require.Equal(t, -5.0, pose.GetLocalOffset().GetY(),
		"the offset is a real nudge in the placement's own axes, not decoration")

	// The two blocking answers are independent, and this table is the pair
	// that proves it: walked around, seen over. A converter that copied one
	// bool into both passes neither assertion.
	require.True(t, placed.GetBlocksMovement())
	require.False(t, placed.GetBlocksLineOfSight())
	require.True(t, placed.GetHoldable(),
		"the author's flag, verbatim -- a client offers Hold only where it is true")

	// THE FIELD THE WIRE EXISTS FOR. Standing is a trace of the rectangle
	// against the floor, and it is carried so nobody runs that geometry a
	// second time. Both cells, in the atlas's own order.
	require.Len(t, placed.GetCells(), 2, "the rectangle stands on two cells")
	require.Equal(t, 1.0, placed.GetCells()[0].GetX())
	require.Equal(t, 0.0, placed.GetCells()[0].GetY())
	require.Equal(t, 1.0, placed.GetCells()[1].GetX())
	require.Equal(t, 1.0, placed.GetCells()[1].GetY())
}

// TestAtlasToProto_PlacedAbsenceIsFalseAndEmpty pins the two shapes that a
// converter is tempted to spell as "nothing": a map with no placements at
// all, and a placement nobody declared anything about.
//
// FALSE IS AN ANSWER on all three flags. A rectangle nobody declared holdable
// is scenery, and a rectangle nobody declared blocking is walked through and
// seen over -- those are facts the author gave, not facts nobody gave, and a
// converter that filtered such an entry out of the list would tell a client
// the thing is not on the floor.
//
// An empty list is the repeated field's own empty, matching every other list
// on this message rather than inventing a nil-versus-empty distinction the
// wire cannot carry anyway.
func TestAtlasToProto_PlacedAbsenceIsFalseAndEmpty(t *testing.T) {
	bare := AtlasToProto(&sdk.Atlas{Grid: sdk.GridHex, Cells: []spatial.Position{{X: 0, Y: 0}}})
	require.Empty(t, bare.GetPlaced(), "a map with no placements carries an empty list")

	quiet := AtlasToProto(&sdk.Atlas{
		Grid:   sdk.GridHex,
		Cells:  []spatial.Position{{X: 0, Y: 0}},
		Placed: []sdk.AtlasPlacedProp{{ID: "rug", Cells: []spatial.Position{{X: 0, Y: 0}}}},
	})
	require.Len(t, quiet.GetPlaced(), 1,
		"a placement nobody declared anything about is still standing there")
	rug := quiet.GetPlaced()[0]
	require.False(t, rug.GetBlocksMovement())
	require.False(t, rug.GetBlocksLineOfSight())
	require.False(t, rug.GetHoldable())
	require.NotNil(t, rug.GetPlacement(),
		"a zero pose is a real pose -- a rectangle anchored at the origin, facing east")
	require.NotNil(t, rug.GetPlacement().GetOrigin(),
		"the source carries a VALUE, so there is no absence here to translate")
	require.NotNil(t, rug.GetPlacement().GetLocalOffset())
	require.Len(t, rug.GetCells(), 1)
}

// TestAtlasToProto_PlacedIsNotFoldedIntoProps pins the one collapse that
// would look tidy and lose the run: the two lists are different KINDS of
// thing, and a placement has no cell to be "at".
//
// A converter that appended placements to `props` would hand a client a prop
// carrying no ref -- nothing to draw -- standing at whichever cell somebody
// picked out of its footprint, and the client's Hold offer would then be
// keyed off a list whose ids the engine's prop verbs do not answer to.
func TestAtlasToProto_PlacedIsNotFoldedIntoProps(t *testing.T) {
	got := AtlasToProto(&sdk.Atlas{
		Grid:  sdk.GridHex,
		Cells: []spatial.Position{{X: 0, Y: 0}},
		Props: []sdk.AtlasProp{{ID: "urn", Ref: "dnd5e:props:urn", At: spatial.Position{X: 0, Y: 0}}},
		Placed: []sdk.AtlasPlacedProp{{
			ID:    "bookcase",
			Cells: []spatial.Position{{X: 0, Y: 0}},
		}},
	})

	require.Len(t, got.GetProps(), 1, "the cell prop is alone on its own list")
	require.Equal(t, "urn", got.GetProps()[0].GetId())
	require.Len(t, got.GetPlaced(), 1, "and the rectangle is alone on its own")
	require.Equal(t, "bookcase", got.GetPlaced()[0].GetId())
}

func TestDeclarationToProto_CarriesActionInformationFieldForField(t *testing.T) {
	base := func() sdk.Declaration {
		return sdk.Declaration{
			Verb: sdk.VerbCast, Slot: sdk.SlotAction, Available: true, ID: "decl-info-1",
			TargetKind: sdk.TargetMember,
			Effects: []sdk.EffectRow{{
				ID: "row-1", Ref: "ref-1", Name: "name 1", Description: "effect description",
				State: sdk.EffectApplies, Participation: sdk.ContributesNow,
			}},
		}
	}
	info := &sdk.ActionInformation{
		Description: "Owner prose, verbatim.",
		Details: []sdk.ActionInformationDetail{
			{Label: "Range", Value: "60 ft"},
			{Label: "Base damage", Value: "1d8 + STR modifier (+3) · Bludgeoning"},
			{Label: "Components", Value: "V, S"},
		},
	}
	wantInfo := &sessionpb.ActionInformation{
		Description: "Owner prose, verbatim.",
		Details: []*sessionpb.ActionInformationDetail{
			{Label: "Range", Value: "60 ft"},
			{Label: "Base damage", Value: "1d8 + STR modifier (+3) · Bludgeoning"},
			{Label: "Components", Value: "V, S"},
		},
	}

	t.Run("available offer", func(t *testing.T) {
		d := base()
		d.Information = info
		d.Options = []sdk.CastOption{
			{ID: "approach", Label: "Approach", Description: "Walk toward the caster."},
			{ID: "flee", Label: "Flee"},
		}
		out := mustDeclarationToProto(t, d)
		require.Equal(t, wantInfo.GetDescription(), out.GetInformation().GetDescription())
		require.Equal(t, wantInfo.GetDetails(), out.GetInformation().GetDetails(), "details cross in the renderer's order")
		require.Equal(t, []*sessionpb.CastOption{
			{Id: "approach", Label: "Approach", Description: "Walk toward the caster."},
			{Id: "flee", Label: "Flee", Description: ""},
		}, out.GetOptions(), "nested choice text crosses; an empty description is not fabricated")
		require.Len(t, out.GetEffects(), 1, "effect rows are untouched")
		require.Equal(t, "effect description", out.GetEffects()[0].GetDescription())
	})

	t.Run("unavailable offer carries the same information", func(t *testing.T) {
		d := base()
		d.Available = false
		d.Why = &sdk.Shortfall{}
		d.Information = info
		out := mustDeclarationToProto(t, d)
		require.False(t, out.GetAvailable())
		require.Equal(t, wantInfo.GetDescription(), out.GetInformation().GetDescription())
		require.Equal(t, wantInfo.GetDetails(), out.GetInformation().GetDetails())
	})

	t.Run("nil information stays absent", func(t *testing.T) {
		out := mustDeclarationToProto(t, base())
		require.Nil(t, out.Information)
	})

	t.Run("description only has no fabricated details", func(t *testing.T) {
		d := base()
		d.Information = &sdk.ActionInformation{Description: "Only prose."}
		out := mustDeclarationToProto(t, d)
		require.Equal(t, "Only prose.", out.GetInformation().GetDescription())
		require.Empty(t, out.GetInformation().GetDetails())
	})

	t.Run("details only has an empty description", func(t *testing.T) {
		d := base()
		d.Information = &sdk.ActionInformation{Details: []sdk.ActionInformationDetail{{Label: "Range", Value: "5 ft"}}}
		out := mustDeclarationToProto(t, d)
		require.Empty(t, out.GetInformation().GetDescription())
		require.Equal(t, []*sessionpb.ActionInformationDetail{{Label: "Range", Value: "5 ft"}}, out.GetInformation().GetDetails())
	})

	t.Run("refusal and selector are identical with and without information", func(t *testing.T) {
		with := base()
		with.Available = false
		with.Why = &sdk.Shortfall{}
		with.Information = info
		without := with
		without.Information = nil
		a, b := mustDeclarationToProto(t, with), mustDeclarationToProto(t, without)
		a.Information = nil
		require.True(t, proto.Equal(a, b), "information changes nothing else on the row")
	})
}

// TestGetView_WatchZonesConvertWatcherForWatcherCellForCell: the red floor
// (rpg-project#619, sneaking R15) is the session's answer carried unchanged.
// Each zone keeps its watcher and its cells in order, through the same
// position converter visible_cells uses; an empty answer sends no zones.
func TestGetView_WatchZonesConvertWatcherForWatcherCellForCell(t *testing.T) {
	type zone struct {
		Watcher string
		Cells   []spatial.Position
	}
	cases := []struct {
		name string
		in   []sdk.WatchZone
		want []zone
	}{
		{
			name: "two zones",
			in: []sdk.WatchZone{
				{Watcher: "monster:knight-1", Cells: []spatial.Position{{X: 2, Y: -1}, {X: 3, Y: -1}, {X: 3, Y: 0}}},
				{Watcher: "monster:knight-2", Cells: []spatial.Position{{X: -4, Y: 5}}},
			},
			want: []zone{
				{Watcher: "monster:knight-1", Cells: []spatial.Position{{X: 2, Y: -1}, {X: 3, Y: -1}, {X: 3, Y: 0}}},
				{Watcher: "monster:knight-2", Cells: []spatial.Position{{X: -4, Y: 5}}},
			},
		},
		{
			// A watcher the viewer holds live whose zone is empty still names
			// itself; the zone carries no cells.
			name: "a zone of zero cells",
			in:   []sdk.WatchZone{{Watcher: "monster:knight-1", Cells: []spatial.Position{}}},
			want: []zone{{Watcher: "monster:knight-1"}},
		},
		{name: "empty list", in: []sdk.WatchZone{}, want: nil},
		{name: "nil list", in: nil, want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := viewToProto(&sdk.ViewOutput{WatchZones: tc.in})
			var got []zone
			for _, z := range resp.GetWatchZones() {
				g := zone{Watcher: z.GetWatcher()}
				for _, c := range z.GetCells() {
					g.Cells = append(g.Cells, positionFromProto(c))
				}
				got = append(got, g)
			}
			require.Equal(t, tc.want, got)
		})
	}
}

// TestGetView_WatchZoneCellsReadTheirLiteralWireFields: the round trip above
// reads cells back through positionFromProto, so a swap made in both
// converters would pass it. This reads the proto message's own fields: the
// session's X=2, Y=-1 must arrive as x=2, y=-1 on the wire.
func TestGetView_WatchZoneCellsReadTheirLiteralWireFields(t *testing.T) {
	resp := viewToProto(&sdk.ViewOutput{WatchZones: []sdk.WatchZone{
		{Watcher: "monster:knight-1", Cells: []spatial.Position{{X: 2, Y: -1}}},
	}})
	require.NotEmpty(t, resp.GetWatchZones())
	z := resp.GetWatchZones()[0]
	require.Equal(t, "monster:knight-1", z.GetWatcher())
	require.NotEmpty(t, z.GetCells())
	require.Equal(t, 2.0, z.GetCells()[0].GetX())
	require.Equal(t, -1.0, z.GetCells()[0].GetY())
}
