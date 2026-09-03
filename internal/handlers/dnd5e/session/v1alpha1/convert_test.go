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
}

func TestDeclarationToProto_FieldForField(t *testing.T) {
	remaining := 0
	out := declarationToProto(sdk.Declaration{
		Verb: sdk.VerbAttack, Slot: sdk.SlotAction, Available: false, Remaining: &remaining,
		Why:        &sdk.Shortfall{Reason: sdk.ShortfallNoBudget, Currency: sdk.CurrencyAction, Needed: 1, Text: "action: 1 needed, 0 left"},
		ID:         "decl-attack-1",
		Attack:     &sdk.AttackRef{Ref: "dnd5e:weapons:longsword", Name: "Longsword", DamageType: sdk.DamageSlashing},
		TargetKind: sdk.TargetMember,
		Candidates: []sdk.TargetCandidate{
			{Member: "goblin-1", Available: true},
			{Member: "skeleton-1", Available: false, Why: &sdk.Shortfall{Reason: sdk.ShortfallTargetOutOfReach, Text: "target out of reach"}},
		},
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
}

func TestDeclarationToProto_PreservesOptionalAbsenceAndEmptyCandidates(t *testing.T) {
	out := declarationToProto(sdk.Declaration{
		Verb: sdk.VerbEndTurn, Slot: sdk.SlotNone, Available: true, ID: "decl-end-1",
		TargetKind: sdk.TargetNone, Candidates: []sdk.TargetCandidate{},
	})
	require.Nil(t, out.Remaining)
	require.Nil(t, out.Why)
	require.Nil(t, out.Attack)
	require.NotNil(t, out.Candidates)
	require.Empty(t, out.Candidates)
}

// TestDeclarationsToProto_NilOrEmpty_StaysNonNilEmpty pins the "no null,
// just an empty repeated" law: proto has no null-vs-empty distinction on a
// repeated field, so a nil SDK slice must still marshal as "[]", never be
// left nil at the Go level where a naive append-based converter would.
func TestDeclarationsToProto_NilOrEmpty_StaysNonNilEmpty(t *testing.T) {
	out := declarationsToProto(nil)
	require.NotNil(t, out)
	require.Empty(t, out)

	out = declarationsToProto([]sdk.Declaration{})
	require.NotNil(t, out)
	require.Empty(t, out)
}

func TestDeclarationsToProto_Populated(t *testing.T) {
	out := declarationsToProto([]sdk.Declaration{
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
	require.Equal(t, sessionpb.DissolveKind_DISSOLVE_KIND_UNSPECIFIED, dissolveKindToProto(sdk.DissolveKind("bogus")))
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

func TestStrikeDetailToProto_EmptyStaysNonNil(t *testing.T) {
	require.NotNil(t, damageComponentsToProto(nil))
	require.Empty(t, damageComponentsToProto(nil))
	require.NotNil(t, attackModifierSourcesToProto(nil))
	require.Empty(t, attackModifierSourcesToProto(nil))
}

func TestRollTraceConverters_NilSafe(t *testing.T) {
	require.Nil(t, rollSourceToProto(nil))
	require.Nil(t, diceRerollToProto(nil))
	require.Nil(t, diceTraceToProto(nil))
	require.Nil(t, rollComponentToProto(nil))
	require.Nil(t, rollCalculationToProto(nil))
}

func TestRollCalculationToProto_FieldForFieldOrderPresenceAndIsolation(t *testing.T) {
	zero := 0
	negative := -3
	in := &sdk.RollCalculation{
		Components: []sdk.RollComponent{
			{
				Source: sdk.RollSource{
					Ref: "dnd5e:weapons:greatsword", Name: "Greatsword", Label: "primary",
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
				Source:   sdk.RollSource{Ref: "dnd5e:abilities:str", Name: "Strength", Label: "ability modifier"},
				Modifier: &negative,
			},
		},
		Total: 777,
	}

	wantZero := int32(0)
	wantNegative := int32(-3)
	want := &sessionpb.RollCalculation{
		Components: []*sessionpb.RollComponent{
			{
				Source: &sessionpb.RollSource{Ref: "dnd5e:weapons:greatsword", Name: "Greatsword", Label: "primary"},
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
				Source:   &sessionpb.RollSource{Ref: "dnd5e:abilities:str", Name: "Strength", Label: "ability modifier"},
				Modifier: &wantNegative,
			},
		},
		Total: 777,
	}

	got := rollCalculationToProto(in)
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

func TestRollDamageComponentToProto_NewAndLegacyRepresentationsNeverMix(t *testing.T) {
	zeroModifier := 0
	zeroMultiplier := 0.0

	newComponent := damageComponentsToProto([]sdk.DamageComponent{{
		Source: "weapon",
		Roll: sdk.RollComponent{
			Source: sdk.RollSource{Ref: "dnd5e:weapons:greatsword", Name: "Greatsword"},
			Dice: &sdk.DiceTrace{
				Notation: "2d6", DieSize: 6, OriginalRolls: []int{1, 5},
				Rerolls: []sdk.DiceReroll{{
					DieIndex: 0, Before: 1, After: 4,
					Source: sdk.RollSource{
						Ref: "dnd5e:conditions:fighting_style_great_weapon_fighting", Name: "Great Weapon Fighting",
					},
				}},
				FinalRolls: []int{4, 5}, Subtotal: 9,
			},
			Modifier: &zeroModifier,
		},
		DamageType: sdk.DamageSlashing, Multiplier: &zeroMultiplier,
		// A malformed in-memory body can carry both representations even though
		// Session's decoder never creates one. The API still emits only the new
		// representation when Roll is present; it does not merge or validate.
		SourceRef: "legacy-ref", Dice: "legacy-dice", FinalRolls: []int{6}, FlatBonus: 6,
	}})[0]

	require.NotNil(t, newComponent.GetRoll())
	require.Equal(t, []int32{1, 5}, newComponent.GetRoll().GetDice().GetOriginalRolls())
	require.Equal(t, []int32{4, 5}, newComponent.GetRoll().GetDice().GetFinalRolls())
	require.NotNil(t, newComponent.GetRoll().Modifier)
	require.Zero(t, newComponent.GetRoll().GetModifier())
	require.NotNil(t, newComponent.Multiplier)
	require.Zero(t, newComponent.GetMultiplier())
	require.Empty(t, newComponent.GetSourceRef())
	require.Empty(t, newComponent.GetDice())
	require.Nil(t, newComponent.GetFinalRolls())
	require.Zero(t, newComponent.GetFlatBonus())

	legacyComponent := damageComponentsToProto([]sdk.DamageComponent{{
		Source: "weapon", SourceRef: "dnd5e:weapons:longsword", Dice: "1d8",
		FinalRolls: []int{7, 2}, FlatBonus: 3, DamageType: sdk.DamageSlashing,
	}})[0]
	require.Nil(t, legacyComponent.GetRoll())
	require.Equal(t, "dnd5e:weapons:longsword", legacyComponent.GetSourceRef())
	require.Equal(t, "1d8", legacyComponent.GetDice())
	require.Equal(t, []int32{7, 2}, legacyComponent.GetFinalRolls())
	require.Equal(t, int32(3), legacyComponent.GetFlatBonus())
}

func TestRollHealingAppliedToProto_NewAndLegacyRepresentationsNeverMix(t *testing.T) {
	level := 1
	newBody := healingAppliedBodyToProto(&sdk.HealingAppliedBody{
		Target: "alice", Amount: 2, Requested: 7, Roll: 99, Modifier: 98,
		SourceRef: "dnd5e:features:second_wind", SourceName: "Second Wind",
		HPBefore: 8, HPAfter: 10,
		Calculation: &sdk.RollCalculation{
			Components: []sdk.RollComponent{
				{
					Source: sdk.RollSource{Ref: "dnd5e:features:second_wind", Name: "Second Wind"},
					Dice: &sdk.DiceTrace{
						Notation: "1d10", DieSize: 10, OriginalRolls: []int{6}, FinalRolls: []int{6}, Subtotal: 6,
					},
				},
				{
					Source:   sdk.RollSource{Ref: "dnd5e:classes:fighter", Name: "Fighter", Label: "Fighter level"},
					Modifier: &level,
				},
			},
			Total: 7,
		},
	})
	require.NotNil(t, newBody.GetCalculation())
	require.Equal(t, int32(7), newBody.GetCalculation().GetTotal())
	require.Equal(t, "Fighter level", newBody.GetCalculation().GetComponents()[1].GetSource().GetLabel())
	require.Zero(t, newBody.GetRoll(), "new events do not also populate deprecated roll")
	require.Zero(t, newBody.GetModifier(), "new events do not also populate deprecated modifier")

	legacyBody := healingAppliedBodyToProto(&sdk.HealingAppliedBody{
		Target: "alice", Amount: 2, Requested: 7, Roll: 6, Modifier: 1,
		SourceRef: "dnd5e:features:second_wind", SourceName: "Second Wind",
		HPBefore: 8, HPAfter: 10,
	})
	require.Nil(t, legacyBody.GetCalculation())
	require.Equal(t, int32(6), legacyBody.GetRoll())
	require.Equal(t, int32(1), legacyBody.GetModifier())
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
			AdvantageSources: []sdk.AttackModifierSource{
				{SourceRef: "dnd5e:conditions:hidden", SourceID: "char-1"},
			},
			DisadvantageSources: []sdk.AttackModifierSource{
				{SourceRef: "dnd5e:conditions:dodging", SourceID: "goblin-1"},
			},
		},
	}
}

// TestEventsToProto pins GetStory's own slice conversion (get_story.go) to
// the SAME per-event converter StreamEvents uses -- Manager.Story returns
// []sdk.Event since session/v0.23.0 (rpg-toolkit#1213), so there is exactly
// one mapping from sdk.Event to the wire, not a thinner one for catch-up
// (rpg-api-protos#239's own ruling: live and catch-up must be byte-equal for
// the same seq).
func TestEventsToProto(t *testing.T) {
	got := eventsToProto([]sdk.Event{
		{
			Session: "sess-1", Seq: 1, At: 10, Correlation: "corr-1", Recipient: "char-1",
			Kind: sdk.EventTurnEnded, Payload: []byte("p"),
			Body: sdk.TurnEndedBody{Member: "char-1", Next: "char-2"},
			Tags: map[string]string{"k": "v"}, // no wire field yet -- dropped, see eventToProto's own doc
		},
	})
	require.Len(t, got, 1)
	require.Equal(t, "sess-1", got[0].GetSession())
	require.Equal(t, uint64(1), got[0].GetSeq())
	require.Equal(t, "corr-1", got[0].GetCorrelation())
	require.Equal(t, "char-1", got[0].GetRecipient())
	require.Equal(t, sessionpb.EventKind_EVENT_KIND_TURN_ENDED, got[0].GetKind())
	require.Equal(t, "char-2", got[0].GetTurnEnded().GetNext())
}

func TestEventsToProto_Empty(t *testing.T) {
	got := eventsToProto(nil)
	require.Empty(t, got)
}

func TestEventsToProto_RichStruckMatchesDirectConversion(t *testing.T) {
	in := richStruckEvent()
	caughtUp := eventsToProto([]sdk.Event{in})
	require.Len(t, caughtUp, 1)
	require.True(t, proto.Equal(eventToProto(in), caughtUp[0]),
		"GetStory's slice conversion and StreamEvents' direct conversion share one mapping")
}

func TestActivationEventKindsToProto(t *testing.T) {
	require.Equal(t, sessionpb.EventKind_EVENT_KIND_ACTIVATED, eventKindToProto(sdk.EventActivated))
	require.Equal(t, sessionpb.EventKind_EVENT_KIND_ACTIVATION_RESULT, eventKindToProto(sdk.EventActivationResult))
}

// TestActivationEventBodiesToProto pins the thin event boundary: every SDK
// activation body crosses into its matching proto oneof arm without deriving
// identity, arithmetic, or prose from any other field.
func TestActivationEventBodiesToProto(t *testing.T) {
	t.Run("Activated", func(t *testing.T) {
		got := eventToProto(sdk.Event{
			Kind: sdk.EventActivated, Payload: []byte("activated-payload"),
			Body: sdk.ActivatedBody{
				Actor: "alice",
				Ability: sdk.AbilityRef{
					Ref: "dnd5e:features:second_wind", Name: "Second Wind",
				},
				Target: "alice",
			},
		})

		require.Equal(t, []byte("activated-payload"), got.GetPayload())
		activated := got.GetActivated()
		require.NotNil(t, activated)
		require.Equal(t, "alice", activated.GetActor())
		require.Equal(t, "dnd5e:features:second_wind", activated.GetAbility().GetRef())
		require.Equal(t, "Second Wind", activated.GetAbility().GetName())
		require.Equal(t, "alice", activated.GetTarget())
	})

	t.Run("HealingApplied", func(t *testing.T) {
		fighterLevel := 1
		got := eventToProto(sdk.Event{
			Kind: sdk.EventActivationResult,
			Body: sdk.ActivationResultBody{
				Actor: "alice",
				// Roll and Modifier come off Calculation, for the same reason
				// the struck fixture above builds a Roll: a body this SDK
				// produces carries the trace and leaves the two scalars zero.
				HealingApplied: &sdk.HealingAppliedBody{
					Target: "alice", Amount: 2, Requested: 7,
					SourceRef: "dnd5e:features:second_wind", SourceName: "Second Wind",
					HPBefore: 8, HPAfter: 10,
					Calculation: &sdk.RollCalculation{
						Components: []sdk.RollComponent{
							{
								Source: sdk.RollSource{
									Ref: "dnd5e:features:second_wind", Name: "Second Wind",
								},
								Dice: &sdk.DiceTrace{
									Notation: "d10", DieSize: 10,
									OriginalRolls: []int{6}, FinalRolls: []int{6}, Subtotal: 6,
								},
							},
							{
								Source: sdk.RollSource{
									Ref: "dnd5e:classes:fighter", Name: "Fighter", Label: "Fighter level",
								},
								Modifier: &fighterLevel,
							},
						},
						Total: 7,
					},
				},
			},
		})

		result := got.GetActivationResult()
		require.NotNil(t, result)
		require.Equal(t, "alice", result.GetActor())
		healing := result.GetHealingApplied()
		require.NotNil(t, healing)
		require.Equal(t, "alice", healing.GetTarget())
		require.Equal(t, int32(2), healing.GetAmount())
		require.Equal(t, int32(7), healing.GetRequested())
		require.Zero(t, healing.GetRoll())
		require.Zero(t, healing.GetModifier())
		require.NotNil(t, healing.GetCalculation())
		require.Equal(t, int32(7), healing.GetCalculation().GetTotal())
		require.Len(t, healing.GetCalculation().GetComponents(), 2)
		require.Equal(t, "d10", healing.GetCalculation().GetComponents()[0].GetDice().GetNotation())
		require.Equal(t, int32(1), healing.GetCalculation().GetComponents()[1].GetModifier())
		require.Equal(t, "dnd5e:features:second_wind", healing.GetSourceRef())
		require.Equal(t, "Second Wind", healing.GetSourceName())
		require.Equal(t, int32(8), healing.GetHpBefore())
		require.Equal(t, int32(10), healing.GetHpAfter())
		require.Nil(t, result.GetConditionApplied())
		require.Nil(t, result.GetConditionRemoved())
		require.Nil(t, result.GetCapacityGranted())
	})

	t.Run("ConditionApplied", func(t *testing.T) {
		got := eventToProto(sdk.Event{
			Kind: sdk.EventActivationResult,
			Body: sdk.ActivationResultBody{
				Actor: "alice",
				ConditionApplied: &sdk.ConditionAppliedBody{
					Target: "bob", Ref: "dnd5e:conditions:raging", Name: "Raging",
				},
			},
		})

		result := got.GetActivationResult()
		require.NotNil(t, result)
		require.Equal(t, "alice", result.GetActor())
		condition := result.GetConditionApplied()
		require.NotNil(t, condition)
		require.Equal(t, "bob", condition.GetTarget())
		require.Equal(t, "dnd5e:conditions:raging", condition.GetRef())
		require.Equal(t, "Raging", condition.GetName())
		require.Nil(t, result.GetHealingApplied())
		require.Nil(t, result.GetConditionRemoved())
		require.Nil(t, result.GetCapacityGranted())
	})

	t.Run("ConditionRemoved", func(t *testing.T) {
		got := eventToProto(sdk.Event{
			Kind: sdk.EventActivationResult,
			Body: sdk.ActivationResultBody{
				Actor: "alice",
				ConditionRemoved: &sdk.ConditionRemovedBody{
					Target: "bob", Ref: "dnd5e:conditions:hidden", Name: "Hidden", Reason: "revealed",
				},
			},
		})

		result := got.GetActivationResult()
		require.NotNil(t, result)
		require.Equal(t, "alice", result.GetActor())
		condition := result.GetConditionRemoved()
		require.NotNil(t, condition)
		require.Equal(t, "bob", condition.GetTarget())
		require.Equal(t, "dnd5e:conditions:hidden", condition.GetRef())
		require.Equal(t, "Hidden", condition.GetName())
		require.Equal(t, "revealed", condition.GetReason())
		require.Nil(t, result.GetHealingApplied())
		require.Nil(t, result.GetConditionApplied())
		require.Nil(t, result.GetCapacityGranted())
	})

	t.Run("CapacityGranted", func(t *testing.T) {
		got := eventToProto(sdk.Event{
			Kind: sdk.EventActivationResult,
			Body: sdk.ActivationResultBody{
				Actor: "alice",
				CapacityGranted: &sdk.CapacityGrantedBody{
					Member: "alice", Description: "30ft movement",
				},
			},
		})

		result := got.GetActivationResult()
		require.NotNil(t, result)
		require.Equal(t, "alice", result.GetActor())
		capacity := result.GetCapacityGranted()
		require.NotNil(t, capacity)
		require.Equal(t, "alice", capacity.GetMember())
		require.Equal(t, "30ft movement", capacity.GetDescription())
		require.Nil(t, result.GetHealingApplied())
		require.Nil(t, result.GetConditionApplied())
		require.Nil(t, result.GetConditionRemoved())
	})
}

func TestActivationResultVariantConverters_NilSafe(t *testing.T) {
	require.Nil(t, healingAppliedBodyToProto(nil))
	require.Nil(t, conditionAppliedBodyToProto(nil))
	require.Nil(t, conditionRemovedBodyToProto(nil))
	require.Nil(t, capacityGrantedBodyToProto(nil))
}

func TestActivationEventBody_NilOrMalformedStaysNil(t *testing.T) {
	tests := []struct {
		name string
		body sdk.EventBody
	}{
		{name: "nil", body: nil},
		{name: "no result", body: sdk.ActivationResultBody{Actor: "alice"}},
		{
			name: "multiple results",
			body: sdk.ActivationResultBody{
				Actor:            "alice",
				ConditionApplied: &sdk.ConditionAppliedBody{Target: "alice"},
				CapacityGranted:  &sdk.CapacityGrantedBody{Member: "alice"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := eventToProto(sdk.Event{
				Kind: sdk.EventActivationResult, Payload: []byte("passthrough"), Body: tt.body,
			})
			require.Equal(t, sessionpb.EventKind_EVENT_KIND_ACTIVATION_RESULT, got.GetKind())
			require.Equal(t, []byte("passthrough"), got.GetPayload())
			require.Nil(t, got.GetBody())
		})
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
	got := seenToProto(&sdk.Seen{Position: spatial.Position{X: 1, Y: 2}, Standing: sdk.StandingDowned})
	require.Equal(t, sessionpb.Standing_STANDING_DOWNED, got.GetStanding())
}

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
		Member: "char-1", Name: "Aldric", Kind: sdk.KindPlayer, Standing: sdk.StandingUp, Active: true,
	})
	require.Equal(t, "char-1", got.GetMember())
	require.Equal(t, "Aldric", got.GetName())
	require.Equal(t, sessionpb.MemberKind_MEMBER_KIND_PLAYER, got.GetKind())
	require.Equal(t, sessionpb.Standing_STANDING_UP, got.GetStanding())
	require.True(t, got.GetActive())
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

// TestEventToProto_TypedBodies covers ONE ARM PER BODY (rpg-toolkit#941):
// every kind that carries a typed session.EventBody projects onto the
// matching proto oneof member, and Payload keeps riding alongside it --
// Body is an ADDITIONAL carrier, not a replacement for the passthrough law.
func TestEventToProto_TypedBodies(t *testing.T) {
	t.Run("TurnEnded", func(t *testing.T) {
		got := eventToProto(sdk.Event{
			Kind: sdk.EventTurnEnded, Payload: []byte("x"),
			Body: sdk.TurnEndedBody{Member: "char-1", Next: "goblin-1"},
		})
		require.Equal(t, []byte("x"), got.GetPayload())
		require.Equal(t, "char-1", got.GetTurnEnded().GetMember())
		require.Equal(t, "goblin-1", got.GetTurnEnded().GetNext())
	})

	t.Run("Downed", func(t *testing.T) {
		got := eventToProto(sdk.Event{Kind: sdk.EventDowned, Body: sdk.DownedBody{Member: "goblin-1"}})
		require.Equal(t, "goblin-1", got.GetDowned().GetMember())
	})

	t.Run("Struck", func(t *testing.T) {
		s := eventToProto(richStruckEvent()).GetStruck()
		require.NotNil(t, s)
		require.Equal(t, "char-1", s.GetAttacker())
		require.Equal(t, "goblin-1", s.GetTarget())
		require.Equal(t, int32(18), s.GetRoll())
		require.Equal(t, int32(21), s.GetTotal())
		require.Equal(t, int32(13), s.GetAgainst())
		require.Equal(t, int32(6), s.GetDamage())
		require.True(t, s.GetCritical())
		require.Equal(t, "dnd5e:weapons:longsword", s.GetAttack().GetRef())
		require.Equal(t, sessionpb.DamageType_DAMAGE_TYPE_SLASHING, s.GetAttack().GetDamageType())

		require.Len(t, s.GetDamageComponents(), 2)
		weapon := s.GetDamageComponents()[0]
		require.Equal(t, "weapon", weapon.GetSource())
		require.NotNil(t, weapon.GetRoll())
		require.Equal(t, "dnd5e:weapons:longsword", weapon.GetRoll().GetSource().GetRef())
		require.Equal(t, "Longsword", weapon.GetRoll().GetSource().GetName())
		require.Equal(t, "1d8", weapon.GetRoll().GetDice().GetNotation())
		require.Equal(t, []int32{4}, weapon.GetRoll().GetDice().GetFinalRolls())
		require.Empty(t, weapon.GetSourceRef())
		require.Empty(t, weapon.GetDice())
		require.Nil(t, weapon.GetFinalRolls())
		require.Equal(t, sessionpb.DamageType_DAMAGE_TYPE_SLASHING, weapon.GetDamageType())
		require.Nil(t, weapon.Multiplier)

		immunity := s.GetDamageComponents()[1]
		require.Equal(t, "monster_trait", immunity.GetSource())
		require.NotNil(t, immunity.GetRoll())
		require.Equal(t, "dnd5e:monster_traits:immunity", immunity.GetRoll().GetSource().GetRef())
		require.Nil(t, immunity.GetRoll().GetDice())
		require.Nil(t, immunity.GetRoll().Modifier)
		require.Empty(t, immunity.GetSourceRef())
		require.NotNil(t, immunity.Multiplier)
		require.Zero(t, immunity.GetMultiplier())

		require.Len(t, s.GetAdvantageSources(), 1)
		require.Equal(t, "dnd5e:conditions:hidden", s.GetAdvantageSources()[0].GetSourceRef())
		require.Equal(t, "char-1", s.GetAdvantageSources()[0].GetSourceId())
		require.Len(t, s.GetDisadvantageSources(), 1)
		require.Equal(t, "dnd5e:conditions:dodging", s.GetDisadvantageSources()[0].GetSourceRef())
		require.Equal(t, "goblin-1", s.GetDisadvantageSources()[0].GetSourceId())
	})

	t.Run("Missed", func(t *testing.T) {
		got := eventToProto(sdk.Event{
			Kind: sdk.EventMissed,
			Body: sdk.MissedBody{
				Attacker: "char-1", Target: "goblin-1", Roll: 4, Total: 7, Against: 13,
				Attack: sdk.AttackRef{Ref: "dnd5e:weapons:longsword", Name: "Longsword", DamageType: sdk.DamageSlashing},
			},
		})
		m := got.GetMissed()
		require.NotNil(t, m)
		require.Equal(t, int32(4), m.GetRoll())
		require.Equal(t, "dnd5e:weapons:longsword", m.GetAttack().GetRef())
	})

	t.Run("FightStarted", func(t *testing.T) {
		got := eventToProto(sdk.Event{
			Kind: sdk.EventFightStarted,
			Body: sdk.FightStartedBody{Members: []string{"char-1", "goblin-1"}},
		})
		require.Equal(t, []string{"char-1", "goblin-1"}, got.GetFightStarted().GetMembers())
	})

	t.Run("FightEnded", func(t *testing.T) {
		got := eventToProto(sdk.Event{
			Kind: sdk.EventFightEnded,
			Body: sdk.FightEndedBody{Cause: sdk.DissolveByDefeat},
		})
		require.Equal(t, sessionpb.DissolveKind_DISSOLVE_KIND_BY_DEFEAT, got.GetFightEnded().GetCause())
	})

	t.Run("Moved", func(t *testing.T) {
		got := eventToProto(sdk.Event{
			Kind: sdk.EventMoved,
			Body: sdk.MovedBody{Member: "char-1", To: spatial.Position{X: 3, Y: 4}},
		})
		require.Equal(t, "char-1", got.GetMoved().GetMember())
		require.Equal(t, 3.0, got.GetMoved().GetTo().GetX())
		require.Equal(t, 4.0, got.GetMoved().GetTo().GetY())
	})

	t.Run("Joined", func(t *testing.T) {
		got := eventToProto(sdk.Event{
			Kind: sdk.EventJoined,
			Body: sdk.JoinedBody{Member: "char-1"},
		})
		require.Equal(t, "char-1", got.GetJoined().GetMember())
	})

	t.Run("Exited", func(t *testing.T) {
		got := eventToProto(sdk.Event{
			Kind: sdk.EventExited,
			Body: sdk.ExitedBody{Member: "char-1"},
		})
		require.Equal(t, "char-1", got.GetExited().GetMember())
	})

	// DoorRevealed (rpg-project#350/#351): the flat door/state/approaches
	// DoorRevealedBody carries grouped into the same nested DoorInfo shape
	// GetDoors returns, plus the doorways that patch the recipient's cached
	// atlas. A locked door's approaches list rides through; an unlocked one
	// (see the RegionRevealed case below) carries no lock at all.
	t.Run("DoorRevealed", func(t *testing.T) {
		got := eventToProto(sdk.Event{
			Kind: sdk.EventDoorRevealed,
			Body: sdk.DoorRevealedBody{
				Door:  "hall-tomb",
				State: "locked",
				Doorways: []sdk.AtlasDoorway{
					{Door: "hall-tomb", From: spatial.Position{X: 15, Y: 4}, To: spatial.Position{X: 16, Y: 4}},
				},
				Approaches: []sdk.DoorApproach{{Ability: "dex", DC: 12}},
			},
		})
		d := got.GetDoorRevealed()
		require.NotNil(t, d)
		require.Equal(t, "hall-tomb", d.GetDoor().GetDoor())
		require.Equal(t, sessionpb.DoorState_DOOR_STATE_LOCKED, d.GetDoor().GetState())
		require.Len(t, d.GetDoor().GetLock().GetApproaches(), 1)
		require.Equal(t, "dex", d.GetDoor().GetLock().GetApproaches()[0].GetAbility())
		require.Equal(t, int32(12), d.GetDoor().GetLock().GetApproaches()[0].GetDc())
		require.Len(t, d.GetDoorways(), 1)
		require.Equal(t, "hall-tomb", d.GetDoorways()[0].GetConnection())
	})

	// An unlocked reveal carries no lock -- doorRevealedInfoToProto's
	// presence law (Approaches empty means Lock unset), matching
	// doorToProto's own convention field-for-field.
	t.Run("DoorRevealed_Unlocked_CarriesNoLock", func(t *testing.T) {
		got := eventToProto(sdk.Event{
			Kind: sdk.EventDoorRevealed,
			Body: sdk.DoorRevealedBody{Door: "entrance-hall", State: "open"},
		})
		require.Nil(t, got.GetDoorRevealed().GetDoor().GetLock())
	})

	// RegionRevealed (rpg-project#350/#351): the region's whole atlas slice
	// -- region entry, props, and every boundary touching its cells -- reuses
	// atlasRegionToProto and the shared atlasPropsToProto AtlasToProto itself
	// uses, verbatim.
	t.Run("RegionRevealed", func(t *testing.T) {
		got := eventToProto(sdk.Event{
			Kind: sdk.EventRegionRevealed,
			Body: sdk.RegionRevealedBody{
				Region: sdk.AtlasRegion{
					ID: "tomb", Name: "Tomb", Archetype: "crypt",
					Cells:    []spatial.Position{{X: 16, Y: 0}},
					Lighting: sdk.Lighting{Intensity: 0.15},
				},
				Props: []sdk.AtlasProp{
					{Ref: "dnd5e:props:coffin", At: spatial.Position{X: 22, Y: 3}, BlocksMovement: true},
				},
				Boundaries: []sdk.AtlasBoundary{
					{From: spatial.Position{X: 15, Y: 0}, To: spatial.Position{X: 16, Y: 0}, BlocksMovement: true, BlocksLineOfSight: true},
				},
			},
		})
		r := got.GetRegionRevealed()
		require.NotNil(t, r)
		require.Equal(t, "tomb", r.GetRegion().GetId())
		require.Equal(t, "crypt", r.GetRegion().GetArchetype())
		require.Equal(t, 0.15, r.GetRegion().GetLighting().GetIntensity())
		require.Len(t, r.GetProps(), 1)
		require.Equal(t, "dnd5e:props:coffin", r.GetProps()[0].GetRef())
		require.True(t, r.GetProps()[0].GetBlocksMovement())
		require.Len(t, r.GetBoundaries(), 1)
		require.True(t, r.GetBoundaries()[0].GetBlocksLineOfSight())
	})
}

// TestEventToProto_UntypedKind_BodyStaysNilPayloadCarries pins the other
// half of the law: a kind with no typed body member (or a nil Body from the
// SDK) leaves the wire's oneof unset, and payload -- never a decoded body --
// is what a client for one of these kinds reads. EventEnded stands in for
// the kind here -- unlike EventJoined/EventExited (session/v0.24.0), it has
// no typed session.EventBody at all, so this stays a clean "no arm claims
// this kind" case rather than "the SDK happened to hand back a nil body".
func TestEventToProto_UntypedKind_BodyStaysNilPayloadCarries(t *testing.T) {
	got := eventToProto(sdk.Event{Kind: sdk.EventEnded, Payload: []byte("ended-payload"), Body: nil})
	require.Equal(t, []byte("ended-payload"), got.GetPayload())
	require.Nil(t, got.GetTurnEnded())
	require.Nil(t, got.GetDowned())
	require.Nil(t, got.GetStruck())
	require.Nil(t, got.GetMissed())
	require.Nil(t, got.GetFightStarted())
	require.Nil(t, got.GetFightEnded())
	require.Nil(t, got.GetMoved())
	require.Nil(t, got.GetJoined())
	require.Nil(t, got.GetExited())
}

// TestDeclarationToProto_CarriesTheAbility pins the identity half of an
// Activate declaration.
//
// A client renders this VERBATIM — the panel reads ability.name for the
// button's label — so an ability that crossed without one is a button with no
// text. The same presence law Attack keeps: present exactly when the SDK
// carries one, absent otherwise, never defaulted.
func TestDeclarationToProto_CarriesTheAbility(t *testing.T) {
	out := declarationToProto(sdk.Declaration{
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
	out := declarationToProto(sdk.Declaration{
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
