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
	// The first shenanigan (rpg-project#454). Afford emits this row on the
	// turn clock unconditionally, the way it emits VerbMove, so unmapped it
	// would not be one wrong label but the whole Intimidate affordance
	// missing: a client drops a verb it cannot name rather than drawing it
	// wrong.
	require.Equal(t, sessionpb.Verb_VERB_INTIMIDATE, verbToProto(sdk.VerbIntimidate))
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
		MinTargets: 1,
		MaxTargets: 3,
		Cost: []sdk.CostComponent{
			{Currency: sdk.CurrencyAction, Needed: 1},
			{Currency: sdk.CurrencyCharges, Needed: 1, Label: "1st-level Spell Slots"},
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
	require.Equal(t, int32(1), out.GetMinTargets())
	require.Equal(t, int32(3), out.GetMaxTargets())
	require.Equal(t, []*sessionpb.CostComponent{
		{Currency: sessionpb.Currency_CURRENCY_ACTION, Needed: 1},
		{Currency: sessionpb.Currency_CURRENCY_CHARGES, Needed: 1, Label: "1st-level Spell Slots"},
	}, out.GetCost())
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
	require.Equal(t, sessionpb.DissolveKind_DISSOLVE_KIND_BY_DEFEAT, dissolveKindToProto(sdk.DissolveByDefeat))
	// The camp turning (rpg-project#375): a missing case here would reach a
	// client as a fight that ended for no stated reason.
	require.Equal(t, sessionpb.DissolveKind_DISSOLVE_KIND_BY_STANCE, dissolveKindToProto(sdk.DissolveByStance))
	require.Equal(t, sessionpb.DissolveKind_DISSOLVE_KIND_UNSPECIFIED, dissolveKindToProto(sdk.DissolveKind("bogus")))
}

// TestPlacementKindToProto: the session's closed placement vocabulary onto
// the wire's enum, by name; a kind this build's protos cannot name is
// UNSPECIFIED, the wire's word for a producer defect, never a guess.
func TestPlacementKindToProto(t *testing.T) {
	require.Equal(t, sessionpb.PlacementKind_PLACEMENT_KIND_MONSTER, placementKindToProto(sdk.PlacementMonster))
	require.Equal(t, sessionpb.PlacementKind_PLACEMENT_KIND_PROP, placementKindToProto(sdk.PlacementProp))
	require.Equal(t, sessionpb.PlacementKind_PLACEMENT_KIND_UNSPECIFIED, placementKindToProto(sdk.PlacementKind("bogus")))
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

func TestStrikeDetailToProto_EmptyStaysNonNil(t *testing.T) {
	components, err := damageComponentsToProto(nil)
	require.NoError(t, err)
	require.NotNil(t, components)
	require.Empty(t, components)
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

func TestRolledEventBodiesProjectCalculation(t *testing.T) {
	calculation := &sdk.RollCalculation{Total: 11, Components: []sdk.RollComponent{{
		Source: sdk.RollSource{Ref: "dnd5e:spells:bane", Name: "Bane", SourceID: "bard-1"},
	}}}

	for _, body := range []sdk.EventBody{
		sdk.DeathSaveBody{Calculation: calculation},
		sdk.StruckBody{Calculation: calculation},
		sdk.MissedBody{Calculation: calculation},
		sdk.SavedBody{Calculation: calculation},
	} {
		event := &sessionpb.Event{}
		setEventBody(event, body)
		var got *sessionpb.RollCalculation
		switch {
		case event.GetDeathSaveRolled() != nil:
			got = event.GetDeathSaveRolled().GetCalculation()
		case event.GetStruck() != nil:
			got = event.GetStruck().GetCalculation()
		case event.GetMissed() != nil:
			got = event.GetMissed().GetCalculation()
		case event.GetSaved() != nil:
			got = event.GetSaved().GetCalculation()
		}
		require.NotNil(t, got)
		require.Equal(t, int32(11), got.GetTotal())
		require.Equal(t, "bard-1", got.GetComponents()[0].GetSource().GetSourceId())
	}
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

// A RULE THIS BUILD CANNOT NAME IS REFUSED, never demoted to UNSPECIFIED.
// Elven Accuracy and the reroll feats are named in the design and deliberately
// absent from the wire; the day one ships against an api that was not rebuilt,
// a client would otherwise draw three faces with no reason beside them.
func TestKeepRuleToProto_RefusesARuleThisBuildCannotName(t *testing.T) {
	for _, rule := range []sdk.KeepRule{sdk.KeepRule("elven-accuracy"), sdk.KeepRule("")} {
		_, err := keepRuleToProto(rule)
		require.Error(t, err)
		require.Contains(t, err.Error(), string(rule))
	}

	// The refusal reaches the caller rather than stopping at the helper: a
	// beat carrying an unnameable rule does not become an event at all.
	err := setEventBody(&sessionpb.Event{}, sdk.IntimidatedBody{
		Actor: "char-1", Target: "goblin-1", DC: 13, Total: 7, Beaten: false,
		Calculation: d20WithKeep([]int{7, 18}, []int{0}, &sdk.DiceKeep{
			Rule:    sdk.KeepRule("elven-accuracy"),
			Imposed: []sdk.RollSource{{Ref: "dnd5e:rules:untrained", Name: "Untrained", SourceID: "char-1"}},
		}),
	})
	require.Error(t, err)
}

// THE CHECK BEATS CARRY THE WHOLE ROLL (rpg-project#462, R4 and R5). Each of
// these used to reach the wire as {dc, total, beaten} or {roll, total}, so an
// untrained character threw two dice and the table saw one number.
func TestCheckBeatsCarryTheCalculation(t *testing.T) {
	untrained := &sdk.DiceKeep{
		Rule:    sdk.KeepDisadvantage,
		Imposed: []sdk.RollSource{{Ref: "dnd5e:rules:untrained", Name: "Untrained", SourceID: "char-1"}},
	}

	for _, tc := range []struct {
		name string
		body sdk.EventBody
		read func(*sessionpb.Event) *sessionpb.RollCalculation
	}{
		{
			name: "intimidated",
			body: sdk.IntimidatedBody{
				Actor: "char-1", Target: "goblin-1", DC: 13, Total: 7,
				Calculation: d20WithKeep([]int{7, 18}, []int{0}, untrained),
			},
			read: func(e *sessionpb.Event) *sessionpb.RollCalculation {
				return e.GetIntimidated().GetCalculation()
			},
		},
		{
			name: "persuaded",
			body: sdk.PersuadedBody{
				Actor: "char-1", Target: "goblin-1", DC: 13, Total: 7,
				Calculation: d20WithKeep([]int{7, 18}, []int{0}, untrained),
			},
			read: func(e *sessionpb.Event) *sessionpb.RollCalculation {
				return e.GetPersuaded().GetCalculation()
			},
		},
		{
			name: "door changed by an unlock attempt",
			body: sdk.DoorBody{
				Door: "door-1", State: "locked", Actor: "char-1", DC: 15, Total: 7,
				Calculation: d20WithKeep([]int{7, 18}, []int{0}, untrained),
			},
			read: func(e *sessionpb.Event) *sessionpb.RollCalculation {
				return e.GetDoor().GetCalculation()
			},
		},
		{
			name: "the paused roll window",
			body: sdk.RollWindowOpenedBody{
				Audience: "char-1", Roll: 7, Total: 7,
				Offer:       sdk.ReactionRef{Ref: "dnd5e:features:bardic_inspiration", Name: "Bardic Inspiration"},
				Calculation: d20WithKeep([]int{7, 18}, []int{0}, untrained),
			},
			read: func(e *sessionpb.Event) *sessionpb.RollCalculation {
				return e.GetRollWindowOpened().GetCalculation()
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event := &sessionpb.Event{}
			require.NoError(t, setEventBody(event, tc.body))

			got := tc.read(event)
			require.NotNil(t, got, "the beat carries the roll, not three numbers")

			dice := got.GetComponents()[0].GetDice()
			require.Equal(t, "2d20", dice.GetNotation())
			require.Equal(t, []int32{7, 18}, dice.GetFinalRolls(), "both faces cross, not the settled one")
			require.Equal(t, []int32{0}, dice.GetKeptIndices())
			require.Equal(t, sessionpb.KeepRule_KEEP_RULE_DISADVANTAGE, dice.GetKeep().GetRule())
			require.Equal(t, "Untrained", dice.GetKeep().GetImposed()[0].GetName(),
				"the word the log prints comes down from the server")
			require.Equal(t, "char-1", dice.GetKeep().GetImposed()[0].GetSourceId())
			require.Empty(t, dice.GetKeep().GetGranted())
		})
	}
}

// A BEAT THAT RECORDED NO ARITHMETIC CARRIES NONE. A door somebody walked
// through was not a check, and an older beat written before this slice has no
// calculation to carry; both leave the field unset rather than crossing with
// a zero-valued calculation a reader would have to tell apart from a real one.
func TestBeatsWithoutArithmeticLeaveTheCalculationUnset(t *testing.T) {
	event := &sessionpb.Event{}
	require.NoError(t, setEventBody(event, sdk.DoorBody{Door: "door-1", State: "open"}))
	require.Nil(t, event.GetDoor().GetCalculation())

	event = &sessionpb.Event{}
	require.NoError(t, setEventBody(event, sdk.IntimidatedBody{Actor: "char-1", Target: "goblin-1", DC: 13, Total: 7}))
	require.Nil(t, event.GetIntimidated().GetCalculation())
}

// THE STRUCK BEAT WRITES NEITHER DEPRECATED LIST (rpg-project#462, R1), even
// when the swing's d20 was decided by a rule. A reader that still consults
// them finds nothing, which is the point: there is one place to read the
// attribution and it is the pool the rule decided.
func TestStruckBeatWritesNoParallelSourceLists(t *testing.T) {
	event := &sessionpb.Event{}
	require.NoError(t, setEventBody(event, sdk.StruckBody{
		Attacker: "char-1", Target: "goblin-1", Roll: 18, Total: 21, Against: 13, Damage: 6,
		Calculation: d20WithKeep([]int{7, 18}, []int{1}, &sdk.DiceKeep{
			Rule: sdk.KeepAdvantage,
			Granted: []sdk.RollSource{{
				Ref: "dnd5e:features:reckless_attack", Name: "Reckless Attack", SourceID: "char-1",
			}},
		}),
	}))

	struck := event.GetStruck()
	require.Empty(t, struck.GetAdvantageSources())    //nolint:staticcheck // Pins that the deprecated field stays empty.
	require.Empty(t, struck.GetDisadvantageSources()) //nolint:staticcheck // Pins that the deprecated field stays empty.
	require.Equal(t, sessionpb.KeepRule_KEEP_RULE_ADVANTAGE,
		struck.GetCalculation().GetComponents()[0].GetDice().GetKeep().GetRule())
	require.Equal(t, "Reckless Attack",
		struck.GetCalculation().GetComponents()[0].GetDice().GetKeep().GetGranted()[0].GetName())
}

func TestCastBodyToProto_CanonicalTargetsAndFaithfulLegacyProjection(t *testing.T) {
	for _, tc := range []struct {
		name       string
		targets    []string
		wantTarget string
	}{
		{name: "one target preserves scalar", targets: []string{"goblin-1"}, wantTarget: "goblin-1"},
		{name: "multiple targets never choose a representative", targets: []string{"goblin-1", "goblin-2"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event := &sessionpb.Event{}
			setEventBody(event, sdk.CastBody{
				Actor: "bard-1", Spell: sdk.SpellRef{Ref: "dnd5e:spells:bane", Name: "Bane"}, Targets: tc.targets,
			})

			require.Equal(t, tc.targets, event.GetCast().GetTargets())
			require.Equal(t, tc.wantTarget, event.GetCast().GetTarget())
		})
	}
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

func TestRollDamageComponentToProto_NewAndLegacyRepresentationsNeverMix(t *testing.T) {
	zeroModifier := 0
	zeroMultiplier := 0.0

	newComponents, err := damageComponentsToProto([]sdk.DamageComponent{{
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
	}})
	require.NoError(t, err)
	newComponent := newComponents[0]

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

	legacyComponents, err := damageComponentsToProto([]sdk.DamageComponent{{
		Source: "weapon", SourceRef: "dnd5e:weapons:longsword", Dice: "1d8",
		FinalRolls: []int{7, 2}, FlatBonus: 3, DamageType: sdk.DamageSlashing,
	}})
	require.NoError(t, err)
	legacyComponent := legacyComponents[0]
	require.Nil(t, legacyComponent.GetRoll())
	require.Equal(t, "dnd5e:weapons:longsword", legacyComponent.GetSourceRef())
	require.Equal(t, "1d8", legacyComponent.GetDice())
	require.Equal(t, []int32{7, 2}, legacyComponent.GetFinalRolls())
	require.Equal(t, int32(3), legacyComponent.GetFlatBonus())
}

func TestRollHealingAppliedToProto_NewAndLegacyRepresentationsNeverMix(t *testing.T) {
	level := 1
	newBody, err := healingAppliedBodyToProto(&sdk.HealingAppliedBody{
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
	require.NoError(t, err)
	require.NotNil(t, newBody.GetCalculation())
	require.Equal(t, int32(7), newBody.GetCalculation().GetTotal())
	require.Equal(t, "Fighter level", newBody.GetCalculation().GetComponents()[1].GetSource().GetLabel())
	require.Zero(t, newBody.GetRoll(), "new events do not also populate deprecated roll")
	require.Zero(t, newBody.GetModifier(), "new events do not also populate deprecated modifier")

	legacyBody, err := healingAppliedBodyToProto(&sdk.HealingAppliedBody{
		Target: "alice", Amount: 2, Requested: 7, Roll: 6, Modifier: 1,
		SourceRef: "dnd5e:features:second_wind", SourceName: "Second Wind",
		HPBefore: 8, HPAfter: 10,
	})
	require.NoError(t, err)
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

// TestEventsToProto pins GetStory's own slice conversion (get_story.go) to
// the SAME per-event converter StreamEvents uses -- Manager.Story returns
// []sdk.Event since session/v0.23.0 (rpg-toolkit#1213), so there is exactly
// one mapping from sdk.Event to the wire, not a thinner one for catch-up
// (rpg-api-protos#239's own ruling: live and catch-up must be byte-equal for
// the same seq).
func TestEventsToProto(t *testing.T) {
	got := mustEventsToProto(t, []sdk.Event{
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
	got := mustEventsToProto(t, nil)
	require.Empty(t, got)
}

func TestEventsToProto_RichStruckMatchesDirectConversion(t *testing.T) {
	in := richStruckEvent()
	caughtUp := mustEventsToProto(t, []sdk.Event{in})
	require.Len(t, caughtUp, 1)
	require.True(t, proto.Equal(mustEventToProto(t, in), caughtUp[0]),
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
		got := mustEventToProto(t, sdk.Event{
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
		got := mustEventToProto(t, sdk.Event{
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
		got := mustEventToProto(t, sdk.Event{
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
		got := mustEventToProto(t, sdk.Event{
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
		got := mustEventToProto(t, sdk.Event{
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

	// A creature the blast MOVED is one more delivered effect, and it reaches
	// a client the same way damage does: as an arm of ActivationResult on the
	// stream, not as a field on the cast's own acknowledgement.
	//
	// It is not the movement. Every cell crossed is already its own beat with
	// the cause on it; this is the line that says how far and what got in the
	// way, which a reader would otherwise have to reconstruct by correlating
	// those beats by hand.
	t.Run("MoveImposed", func(t *testing.T) {
		got := mustEventToProto(t, sdk.Event{
			Kind: sdk.EventActivationResult,
			Body: sdk.ActivationResultBody{
				Actor: "bard-1",
				MoveImposed: &sdk.MoveImposedBody{
					Target: "skeleton-1", SourceRef: "dnd5e:spells:thunderwave", SourceName: "Thunderwave",
					MovedCells: 1, StoppedBy: "a pillar",
				},
			},
		})

		result := got.GetActivationResult()
		require.NotNil(t, result)
		require.Equal(t, "bard-1", result.GetActor())
		moved := result.GetMoveImposed()
		require.NotNil(t, moved)
		require.Equal(t, "skeleton-1", moved.GetTarget())
		require.Equal(t, int32(1), moved.GetMovedCells())
		require.Equal(t, "a pillar", moved.GetStoppedBy())
		require.Nil(t, result.GetHealingApplied())
		require.Nil(t, result.GetConditionApplied())
		require.Nil(t, result.GetDamageApplied())
	})

	// ZERO CELLS IS A RESULT, NOT AN ABSENCE. A creature already against the
	// wall is pushed nowhere, and the caster is owed that sentence: the blast
	// landed and the wall is why nothing moved. The wire field is unset at
	// zero by proto3's own rules, so the arm itself has to be present for a
	// client to tell "pushed nowhere" from "not pushed".
	t.Run("MoveImposed pinned against the fold", func(t *testing.T) {
		got := mustEventToProto(t, sdk.Event{
			Kind: sdk.EventActivationResult,
			Body: sdk.ActivationResultBody{
				Actor: "bard-1",
				MoveImposed: &sdk.MoveImposedBody{
					Target: "skeleton-2", SourceRef: "dnd5e:spells:thunderwave", SourceName: "Thunderwave",
					MovedCells: 0, StoppedBy: "a wall",
				},
			},
		})

		moved := got.GetActivationResult().GetMoveImposed()
		require.NotNil(t, moved, "a push that moved nobody is still a push that happened")
		require.Equal(t, int32(0), moved.GetMovedCells())
		require.Equal(t, "a wall", moved.GetStoppedBy())
	})
}

func TestActivationResultVariantConverters_NilSafe(t *testing.T) {
	_, err := healingAppliedBodyToProto(nil)
	require.Error(t, err, "a result arm that is not there is a caller defect, not an empty heal")
	_, err = damageAppliedBodyToProto(nil)
	require.Error(t, err)
	require.Nil(t, conditionAppliedBodyToProto(nil))
	require.Nil(t, conditionRemovedBodyToProto(nil))
	require.Nil(t, capacityGrantedBodyToProto(nil))
	require.Nil(t, moveImposedBodyToProto(nil))
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
		{
			name: "a moved result beside another result",
			body: sdk.ActivationResultBody{
				Actor:       "alice",
				MoveImposed: &sdk.MoveImposedBody{Target: "bob"},
				DamageApplied: &sdk.DamageAppliedBody{
					Target: "bob", SourceRef: "dnd5e:spells:thunderwave",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mustEventToProto(t, sdk.Event{
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
	got := declarationToProto(sdk.Declaration{
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

// TestEventToProto_TypedBodies covers ONE ARM PER BODY (rpg-toolkit#941):
// every kind that carries a typed session.EventBody projects onto the
// matching proto oneof member, and Payload keeps riding alongside it --
// Body is an ADDITIONAL carrier, not a replacement for the passthrough law.
func TestEventToProto_TypedBodies(t *testing.T) {
	t.Run("TurnEnded", func(t *testing.T) {
		got := mustEventToProto(t, sdk.Event{
			Kind: sdk.EventTurnEnded, Payload: []byte("x"),
			Body: sdk.TurnEndedBody{Member: "char-1", Next: "goblin-1"},
		})
		require.Equal(t, []byte("x"), got.GetPayload())
		require.Equal(t, "char-1", got.GetTurnEnded().GetMember())
		require.Equal(t, "goblin-1", got.GetTurnEnded().GetNext())
	})

	t.Run("Downed", func(t *testing.T) {
		got := mustEventToProto(t, sdk.Event{Kind: sdk.EventDowned, Body: sdk.DownedBody{Member: "goblin-1"}})
		require.Equal(t, "goblin-1", got.GetDowned().GetMember())
	})

	// The fall's receipt (rpg-project#496, R5). Kind and body are asserted
	// together here because they were written together: a kind this build did
	// not know would demote to EVENT_KIND_UNKNOWN and take the body with it,
	// and no RPC exists to ask what the demotion swallowed.
	t.Run("ExperienceGained", func(t *testing.T) {
		got := mustEventToProto(t, sdk.Event{
			Kind: sdk.EventExperienceGained,
			Body: sdk.ExperienceGainedBody{
				Member: "goblin-1",
				Grants: []sdk.ExperienceGrant{
					{Character: "alice", Amount: 25, Total: 325},
					{Character: "bob", Amount: 25, Total: 25},
				},
			},
		})
		require.Equal(t, sessionpb.EventKind_EVENT_KIND_EXPERIENCE_GAINED, got.GetKind())

		x := got.GetExperienceGained()
		require.NotNil(t, x)
		require.Equal(t, "goblin-1", x.GetMember(),
			"member is the CAUSE -- the monster that fell -- never one of the paid")

		// Everyone on the roster rides every copy of the beat, in the SDK's
		// own order: this converter sorts nothing and drops nobody, so a
		// client can narrate the whole party's share from one beat.
		require.Len(t, x.GetGrants(), 2)
		require.Equal(t, "alice", x.GetGrants()[0].GetCharacter())
		require.Equal(t, int32(25), x.GetGrants()[0].GetAmount())
		require.Equal(t, int32(325), x.GetGrants()[0].GetTotal(),
			"total is the sheet AFTER the grant, not the share again")
		require.Equal(t, "bob", x.GetGrants()[1].GetCharacter())
		require.Equal(t, int32(25), x.GetGrants()[1].GetAmount())
		require.Equal(t, int32(25), x.GetGrants()[1].GetTotal(),
			"a first kill leaves a total that happens to equal the share -- still the total")
	})

	t.Run("Struck", func(t *testing.T) {
		s := mustEventToProto(t, richStruckEvent()).GetStruck()
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

		// The deprecated lists are never written, even though the wire still
		// carries the fields for payloads persisted before the keep record
		// existed. Two places to read one fact is two places that can
		// disagree (rpg-project#462, R1).
		require.Empty(t, s.GetAdvantageSources())    //nolint:staticcheck // Pins that the deprecated field stays empty.
		require.Empty(t, s.GetDisadvantageSources()) //nolint:staticcheck // Pins that the deprecated field stays empty.

		keep := s.GetCalculation().GetComponents()[0].GetDice().GetKeep()
		require.NotNil(t, keep, "the swing's attribution rides the pool it decided")
		require.Equal(t, sessionpb.KeepRule_KEEP_RULE_CANCELLED, keep.GetRule())
		require.Equal(t, "dnd5e:conditions:hidden", keep.GetGranted()[0].GetRef())
		require.Equal(t, "Hidden", keep.GetGranted()[0].GetName())
		require.Equal(t, "char-1", keep.GetGranted()[0].GetSourceId())
		require.Equal(t, "dnd5e:conditions:dodging", keep.GetImposed()[0].GetRef())
		require.Equal(t, "Dodging", keep.GetImposed()[0].GetName())
		require.Equal(t, "goblin-1", keep.GetImposed()[0].GetSourceId())
	})

	t.Run("Missed", func(t *testing.T) {
		got := mustEventToProto(t, sdk.Event{
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
		got := mustEventToProto(t, sdk.Event{
			Kind: sdk.EventFightStarted,
			Body: sdk.FightStartedBody{Members: []string{"char-1", "goblin-1"}},
		})
		require.Equal(t, []string{"char-1", "goblin-1"}, got.GetFightStarted().GetMembers())
	})

	t.Run("FightEnded", func(t *testing.T) {
		got := mustEventToProto(t, sdk.Event{
			Kind: sdk.EventFightEnded,
			Body: sdk.FightEndedBody{Cause: sdk.DissolveByDefeat},
		})
		require.Equal(t, sessionpb.DissolveKind_DISSOLVE_KIND_BY_DEFEAT, got.GetFightEnded().GetCause())
	})

	t.Run("FightEnded by stance", func(t *testing.T) {
		got := mustEventToProto(t, sdk.Event{
			Kind: sdk.EventFightEnded,
			Body: sdk.FightEndedBody{Cause: sdk.DissolveByStance},
		})
		require.Equal(t, sessionpb.DissolveKind_DISSOLVE_KIND_BY_STANCE, got.GetFightEnded().GetCause())
	})

	// The arrival beat (rpg-project#375 step B, design §6): which placement
	// entered the run, what it is -- a closed enum, mapped by name -- and
	// the cell it stands on now.
	t.Run("Arrived", func(t *testing.T) {
		got := mustEventToProto(t, sdk.Event{
			Kind: sdk.EventArrived,
			Body: sdk.ArrivedBody{ID: "reinforcement-1", Kind: sdk.PlacementMonster, Cell: spatial.Position{X: 1, Y: 4}},
		})
		require.Equal(t, sessionpb.EventKind_EVENT_KIND_ARRIVED, got.GetKind())
		require.Equal(t, "reinforcement-1", got.GetArrived().GetId())
		require.Equal(t, sessionpb.PlacementKind_PLACEMENT_KIND_MONSTER, got.GetArrived().GetKind())
		require.Equal(t, 1.0, got.GetArrived().GetCell().GetX())
		require.Equal(t, 4.0, got.GetArrived().GetCell().GetY())

		prop := mustEventToProto(t, sdk.Event{
			Kind: sdk.EventArrived,
			Body: sdk.ArrivedBody{ID: "letter", Kind: sdk.PlacementProp, Cell: spatial.Position{X: 0, Y: 3}},
		})
		require.Equal(t, sessionpb.PlacementKind_PLACEMENT_KIND_PROP, prop.GetArrived().GetKind(),
			"a prop is not a member, and the client branches on it")
	})

	// The stance beat (rpg-project#375, design §6): kind and body, verbatim
	// -- the pair as the session sorted it, the stance as the author's word,
	// and the composition's own sentence for why (rpg-api-protos#354).
	t.Run("StanceChanged", func(t *testing.T) {
		got := mustEventToProto(t, sdk.Event{
			Kind: sdk.EventStanceChanged,
			Body: sdk.StanceChangedBody{
				Between: []string{"party", "raiders"}, Stance: "hostile", Cause: "attacked by alice",
			},
		})
		require.Equal(t, sessionpb.EventKind_EVENT_KIND_STANCE_CHANGED, got.GetKind())
		require.Equal(t, []string{"party", "raiders"}, got.GetStanceChanged().GetBetween())
		require.Equal(t, "hostile", got.GetStanceChanged().GetStance())
		require.Equal(t, "attacked by alice", got.GetStanceChanged().GetCause(),
			"the sentence the composition wrote, not one composed here")
	})

	// A pair that a mind's knowledge turned carries no sentence, and the
	// converter leaves it that way. Substituting a default would hand every
	// client a reason no author wrote; the emptiness is what tells a reader
	// to say only that the pair turned.
	t.Run("StanceChangedWithoutACause", func(t *testing.T) {
		got := mustEventToProto(t, sdk.Event{
			Kind: sdk.EventStanceChanged,
			Body: sdk.StanceChangedBody{Between: []string{"party", "raiders"}, Stance: "neutral"},
		})
		require.Equal(t, "neutral", got.GetStanceChanged().GetStance())
		require.Empty(t, got.GetStanceChanged().GetCause(),
			"the hold-out beat writes no cause, and nothing here invents one")
	})

	t.Run("Moved", func(t *testing.T) {
		got := mustEventToProto(t, sdk.Event{
			Kind: sdk.EventMoved,
			Body: sdk.MovedBody{Member: "char-1", To: spatial.Position{X: 3, Y: 4}},
		})
		require.Equal(t, "char-1", got.GetMoved().GetMember())
		require.Equal(t, 3.0, got.GetMoved().GetTo().GetX())
		require.Equal(t, 4.0, got.GetMoved().GetTo().GetY())
	})

	t.Run("Joined", func(t *testing.T) {
		got := mustEventToProto(t, sdk.Event{
			Kind: sdk.EventJoined,
			Body: sdk.JoinedBody{Member: "char-1"},
		})
		require.Equal(t, "char-1", got.GetJoined().GetMember())
	})

	t.Run("Exited", func(t *testing.T) {
		got := mustEventToProto(t, sdk.Event{
			Kind: sdk.EventExited,
			Body: sdk.ExitedBody{Member: "char-1"},
		})
		require.Equal(t, "char-1", got.GetExited().GetMember())
	})

	// ConcealmentRevealed (design rpg-project#490, E4): ONE beat for one
	// authored secret, where DoorRevealed and RegionRevealed were two. Every
	// field the SDK body carries has to reach the wire, because this payload
	// IS the client's patch for two cached reads -- a list dropped here
	// leaves a revealed room drawn with no walls, or a door on the map that
	// no verb can open, and neither failure looks like a converter bug.
	//
	// THE DOORWAYS COME OFF THE DOORS. The SDK hangs each door's edges on the
	// door; the wire carries one flat doorway list beside the door list,
	// because those are the two shapes GetDoors and GetAtlas answer in. A
	// wide door's edges arrive together and each names its own door.
	t.Run("ConcealmentRevealed_CarriesEveryField", func(t *testing.T) {
		got := mustEventToProto(t, sdk.Event{
			Kind: sdk.EventConcealmentRevealed,
			Body: sdk.ConcealmentRevealedBody{
				Concealment: "reference-tomb/vault",
				Cells:       []spatial.Position{{X: 28, Y: 2}, {X: 29, Y: 2}},
				Props: []sdk.AtlasProp{{
					ID: "heirloom", Ref: "dnd5e:props:coffin", Holdable: true,
					At: spatial.Position{X: 28, Y: 2}, BlocksMovement: true,
				}},
				Doors: []sdk.RevealedDoor{{
					Door: "reference-tomb/vault-door", State: "locked",
					Doorways: []sdk.AtlasDoorway{
						{Door: "reference-tomb/vault-door", From: spatial.Position{X: 27, Y: 2}, To: spatial.Position{X: 28, Y: 2}},
						{Door: "reference-tomb/vault-door", From: spatial.Position{X: 27, Y: 3}, To: spatial.Position{X: 28, Y: 3}},
					},
					Approaches: []sdk.DoorApproach{{Ability: "dex", Tool: "thieves-tools", DC: 12}},
				}},
				Regions: []sdk.AtlasRegion{{
					ID: "vault", Name: "The Vault", Archetype: "crypt",
					Cells:    []spatial.Position{{X: 28, Y: 2}, {X: 29, Y: 2}},
					Lighting: sdk.Lighting{Intensity: 0.15},
				}},
				Boundaries: []sdk.AtlasBoundary{{
					From: spatial.Position{X: 27, Y: 4}, To: spatial.Position{X: 28, Y: 4},
					BlocksMovement: true, BlocksLineOfSight: true, Height: 0.7,
				}},
				Segments: []sdk.AtlasSegment{{
					From: sdk.AxialPointF{Q: 27.25, R: 7.375}, To: sdk.AxialPointF{Q: 27.75, R: 0.625}, Height: 0.7,
				}},
				Sealed: []spatial.Position{{X: 29, Y: 2}},
			},
		})

		require.Equal(t, sessionpb.EventKind_EVENT_KIND_CONCEALMENT_REVEALED, got.GetKind())
		c := got.GetConcealmentRevealed()
		require.NotNil(t, c, "the one reveal kind carries the one reveal body")
		require.Equal(t, "reference-tomb/vault", c.GetConcealment(),
			"the secret's own id, which is what a client keys its patch on")

		// THE FLOOR IT HID, in order, and the one cell of it nobody stands on.
		// Sealed is a subset of cells rather than a second list of places, so
		// a client applying the patch cannot end up with a cell it has sealed
		// and never added.
		require.Equal(t, [][2]float64{{28, 2}, {29, 2}}, projected(c.GetCells(), wireXY))
		require.Equal(t, [][2]float64{{29, 2}}, projected(c.GetSealed(), wireXY))

		// EVERY REGION IT TOUCHED, WHOLE -- archetype and lighting with it, so
		// a room withheld entirely is dressed and lit the moment it arrives
		// instead of rendering as bare floor until the next GetAtlas.
		regions := c.GetRegions()
		require.Equal(t, []string{"vault"},
			projected(regions, func(r *sessionpb.AtlasRegion) string { return r.GetId() }))
		require.Equal(t, "The Vault", regions[0].GetName())
		require.Equal(t, "crypt", regions[0].GetArchetype())
		require.Equal(t, 0.15, regions[0].GetLighting().GetIntensity())
		require.Equal(t, [][2]float64{{28, 2}, {29, 2}}, projected(regions[0].GetCells(), wireXY))

		// THE PROPS STANDING ON THAT FLOOR, through the same converter
		// AtlasToProto uses, holdability and all.
		props := c.GetProps()
		require.Equal(t, []string{"heirloom"},
			projected(props, func(p *sessionpb.AtlasProp) string { return p.GetId() }))
		require.Equal(t, "dnd5e:props:coffin", props[0].GetRef())
		require.True(t, props[0].GetHoldable())
		require.True(t, props[0].GetBlocksMovement())

		// THE DOOR, in the nested DoorInfo shape GetDoors answers in, its
		// lock's approaches riding with it.
		doors := c.GetDoors()
		require.Equal(t, []string{"reference-tomb/vault-door"},
			projected(doors, func(d *sessionpb.DoorInfo) string { return d.GetDoor() }))
		require.Equal(t, sessionpb.DoorState_DOOR_STATE_LOCKED, doors[0].GetState())
		approaches := doors[0].GetLock().GetApproaches()
		require.Equal(t, []string{"dex"},
			projected(approaches, func(a *sessionpb.CheckApproach) string { return a.GetAbility() }))
		require.Equal(t, "thieves-tools", approaches[0].GetTool())
		require.Equal(t, int32(12), approaches[0].GetDc())

		// ITS EDGES, FLATTENED OUT of the door and onto the wire's own doorway
		// list -- both of them, each still naming the door it belongs to.
		doorways := c.GetDoorways()
		require.Equal(t, []string{"reference-tomb/vault-door", "reference-tomb/vault-door"},
			projected(doorways, func(d *sessionpb.AtlasDoorway) string { return d.GetConnection() }))
		require.Equal(t, [][2]float64{{27, 2}, {27, 3}},
			projected(doorways, func(d *sessionpb.AtlasDoorway) [2]float64 { return wireXY(d.GetFrom()) }))
		require.Equal(t, [][2]float64{{28, 2}, {28, 3}},
			projected(doorways, func(d *sessionpb.AtlasDoorway) [2]float64 { return wireXY(d.GetTo()) }))

		// THE MECHANICAL TRUTH AT THE REVEALED SEAMS, and the walls as the
		// author drew them. Boundaries say what may cross; segments are the
		// line a client draws, in the fractional axial frame the atlas already
		// carries them in.
		boundaries := c.GetBoundaries()
		require.Equal(t, [][2]float64{{27, 4}},
			projected(boundaries, func(b *sessionpb.AtlasBoundary) [2]float64 { return wireXY(b.GetFrom()) }))
		require.Equal(t, [][2]float64{{28, 4}},
			projected(boundaries, func(b *sessionpb.AtlasBoundary) [2]float64 { return wireXY(b.GetTo()) }))
		require.True(t, boundaries[0].GetBlocksMovement())
		require.True(t, boundaries[0].GetBlocksLineOfSight())
		require.Equal(t, float32(0.7), boundaries[0].GetHeight())

		segments := c.GetSegments()
		require.Equal(t, [][2]float64{{27.25, 7.375}},
			projected(segments, func(s *sessionpb.AtlasSegment) [2]float64 { return wireQR(s.GetFrom()) }))
		require.Equal(t, [][2]float64{{27.75, 0.625}},
			projected(segments, func(s *sessionpb.AtlasSegment) [2]float64 { return wireQR(s.GetTo()) }))
		require.Equal(t, float32(0.7), segments[0].GetHeight())
	})

	// An unlocked door carries no lock -- revealedDoorToProto's presence law
	// (Approaches empty means Lock unset), matching doorToProto's own
	// convention field-for-field rather than re-deriving it from State.
	t.Run("ConcealmentRevealed_UnlockedDoorCarriesNoLock", func(t *testing.T) {
		got := mustEventToProto(t, sdk.Event{
			Kind: sdk.EventConcealmentRevealed,
			Body: sdk.ConcealmentRevealedBody{
				Concealment: "reference-tomb/vault",
				Doors:       []sdk.RevealedDoor{{Door: "entrance-hall", State: "open"}},
			},
		})
		require.Nil(t, got.GetConcealmentRevealed().GetDoors()[0].GetLock())
	})

	// A CELL-LESS CONCEALMENT IS LEGAL and means a hidden crossing: a door
	// alone, hiding no floor. Empty cells here is an answer rather than a
	// gap, and the doors are the whole patch -- which is the shape the v2
	// lowering gives a concealed door touching no concealed region.
	t.Run("ConcealmentRevealed_CellLessIsADoorAlone", func(t *testing.T) {
		got := mustEventToProto(t, sdk.Event{
			Kind: sdk.EventConcealmentRevealed,
			Body: sdk.ConcealmentRevealedBody{
				Concealment: "reference-tomb/hidden-crossing",
				Doors: []sdk.RevealedDoor{{
					Door: "reference-tomb/hidden-crossing", State: "closed",
					Doorways: []sdk.AtlasDoorway{
						{Door: "reference-tomb/hidden-crossing", From: spatial.Position{X: 4, Y: 1}, To: spatial.Position{X: 5, Y: 1}},
					},
				}},
			},
		})
		c := got.GetConcealmentRevealed()
		require.Empty(t, c.GetCells(), "a hidden crossing hides no floor")
		require.Empty(t, c.GetRegions(), "and so trims no region to put back")
		require.Equal(t, []string{"reference-tomb/hidden-crossing"},
			projected(c.GetDoors(), func(d *sessionpb.DoorInfo) string { return d.GetDoor() }))
		require.Equal(t, []string{"reference-tomb/hidden-crossing"},
			projected(c.GetDoorways(), func(d *sessionpb.AtlasDoorway) string { return d.GetConnection() }))
	})

	// Sighted: a change in ONE recipient's own perception, passed through as
	// member ids and nothing else. The names are the whole body on purpose --
	// what the recipient now perceives about them is already answered,
	// member-scoped, by GetView, and minting it here would be a second
	// computation of that same answer.
	t.Run("Sighted_CarriesNamesVerbatim", func(t *testing.T) {
		got := mustEventToProto(t, sdk.Event{
			Kind: sdk.EventSighted,
			Body: sdk.SightedBody{Gained: []string{"goblin-2", "orc-1"}, Lost: []string{"wolf-3"}},
		})
		require.Equal(t, sessionpb.EventKind_EVENT_KIND_SIGHTED, got.GetKind())

		sighted := got.GetSighted()
		require.NotNil(t, sighted)
		require.Equal(t, []string{"goblin-2", "orc-1"}, sighted.GetGained(),
			"member ids verbatim, in the order the session settled them")
		require.Equal(t, []string{"wolf-3"}, sighted.GetLost())
	})

	// EITHER HALF MAY BE ABSENT, and the seam does not invent the other. A
	// client reads an empty Gained as "nobody arrived" rather than wondering
	// whether the question was asked -- so this side must not turn an absent
	// list into a present empty one, nor the reverse.
	t.Run("Sighted_TheHalfThatDidNotHappenStaysAbsent", func(t *testing.T) {
		arrived := mustEventToProto(t, sdk.Event{
			Kind: sdk.EventSighted,
			Body: sdk.SightedBody{Gained: []string{"goblin-2"}},
		}).GetSighted()
		require.Equal(t, []string{"goblin-2"}, arrived.GetGained())
		require.Empty(t, arrived.GetLost(), "nobody left, so nothing is named as leaving")

		departed := mustEventToProto(t, sdk.Event{
			Kind: sdk.EventSighted,
			Body: sdk.SightedBody{Lost: []string{"wolf-3"}},
		}).GetSighted()
		require.Empty(t, departed.GetGained())
		require.Equal(t, []string{"wolf-3"}, departed.GetLost())
	})

	// NO ASSET REF IS MINTED HERE, unlike the equipment this seam does mint
	// for (assetref.Item). This beat names MEMBERS, not items -- ids the
	// client already holds from its roster -- so there is nothing in the
	// rules' vocabulary needing translation into the manifest's. Pinned so a
	// future "be consistent, namespace everything" pass has to argue with a
	// test.
	// THE THIRD LIST: a peer still in view whose appearance moved under the
	// recipient. It crosses beside the two transitions rather than instead of
	// them, because one pass can carry all three.
	t.Run("Sighted_CarriesTheChangedHalf", func(t *testing.T) {
		sighted := mustEventToProto(t, sdk.Event{
			Kind: sdk.EventSighted,
			Body: sdk.SightedBody{Changed: []string{"goblin-2"}},
		}).GetSighted()
		require.Equal(t, []string{"goblin-2"}, sighted.GetChanged())
		require.Empty(t, sighted.GetGained(), "nobody arrived — it was already in view")
		require.Empty(t, sighted.GetLost())

		all := mustEventToProto(t, sdk.Event{
			Kind: sdk.EventSighted,
			Body: sdk.SightedBody{
				Gained: []string{"orc-1"}, Lost: []string{"wolf-3"}, Changed: []string{"goblin-2"},
			},
		}).GetSighted()
		require.Equal(t, []string{"orc-1"}, all.GetGained())
		require.Equal(t, []string{"wolf-3"}, all.GetLost())
		require.Equal(t, []string{"goblin-2"}, all.GetChanged(),
			"three independent lists, and one pass can carry all of them")
	})

	// AND IT SAYS NOTHING ABOUT WHAT CHANGED. There is no item, slot or verb
	// anywhere on this body — only a name. Pinned so the next person tempted
	// to "just include the weapon, the client needs it anyway" has to argue
	// with a test rather than with a comment: the fact is exactly what an
	// illusion must be able to lie about, and a fact on the wire is true for
	// everybody by construction.
	t.Run("Sighted_SaysWhoChangedAndNeverWhat", func(t *testing.T) {
		sighted := mustEventToProto(t, sdk.Event{
			Kind: sdk.EventSighted,
			Body: sdk.SightedBody{Changed: []string{"goblin-2"}},
		}).GetSighted()

		require.Equal(t, []string{"goblin-2"}, sighted.GetChanged())
		require.Equal(t, 3, sighted.ProtoReflect().Descriptor().Fields().Len(),
			"gained, lost, changed — and nowhere to put an item")
	})

	t.Run("Sighted_MemberIdsAreNotAssetRefs", func(t *testing.T) {
		sighted := mustEventToProto(t, sdk.Event{
			Kind: sdk.EventSighted,
			Body: sdk.SightedBody{Gained: []string{"goblin-2"}},
		}).GetSighted()
		require.Equal(t, "goblin-2", sighted.GetGained()[0],
			"a member id crosses as itself, not as dnd5e:item:goblin-2")
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
	got := mustEventToProto(t, sdk.Event{Kind: sdk.EventEnded, Payload: []byte("ended-payload"), Body: nil})
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
	out := declarationToProto(sdk.Declaration{
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
	out := declarationToProto(sdk.Declaration{Verb: sdk.VerbMove, ID: "decl-move-1"})
	require.Nil(t, out.GetReaction())
}

// TestDeclarationToProto_CarriesTheSpell covers the CAST row (rpg-project#405).
// The verb alone cannot say which cantrip a row casts -- one verb compiles one
// row per castable cantrip -- so this field is what lets a dock label the
// button "Vicious Mockery" instead of "Cast".
func TestDeclarationToProto_CarriesTheSpell(t *testing.T) {
	out := declarationToProto(sdk.Declaration{
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
	out := declarationToProto(sdk.Declaration{Verb: sdk.VerbMove, ID: "decl-move-2"})
	require.Nil(t, out.GetSpell())
}

// TestEventWindowOpened_ReachesTheWireTyped is the beat half of the done-when:
// the fight paused, and the log says whose step, between which cells, who is
// being asked, and with what.
func TestEventWindowOpened_ReachesTheWireTyped(t *testing.T) {
	got := mustEventToProto(t, sdk.Event{
		Session: "sess-1",
		Kind:    sdk.EventWindowOpened,
		Body: sdk.WindowOpenedBody{
			Mover:    "skel-1",
			From:     spatial.Position{X: 3, Y: 4},
			To:       spatial.Position{X: 4, Y: 4},
			Audience: []string{"char-1", "char-2"},
			Reaction: sdk.ReactionRef{
				Ref: "dnd5e:conditions:opportunity_attack", Name: "Opportunity Attack",
			},
		},
	})

	require.Equal(t, sessionpb.EventKind_EVENT_KIND_WINDOW_OPENED, got.GetKind())
	w := got.GetWindowOpened()
	require.NotNil(t, w, "the kind and the body arm are one-to-one")
	require.Equal(t, "skel-1", w.GetMover())
	// The mover is STANDING ON From: the step is announced and not taken,
	// which is the whole reason reach can still be checked against them.
	require.Equal(t, float64(3), w.GetFrom().GetX())
	require.Equal(t, float64(4), w.GetTo().GetX())
	require.Equal(t, []string{"char-1", "char-2"}, w.GetAudience())
	require.Equal(t, "Opportunity Attack", w.GetReaction().GetName())
}

// TestStruckAndMissedCarryTheReaction closes the gap protos#258 opened and
// nothing filled: the wire field existed, the encounter recorded the identity,
// and the beat arrived saying nothing about why a fighter swung on a
// skeleton's turn.
func TestStruckAndMissedCarryThePresentationToken(t *testing.T) {
	const token = "presentation_2f1c8b4a-0d6e-4a1b-9c3f-5e7a1b2c3d4e"

	struck := mustEventToProto(t, sdk.Event{Kind: sdk.EventStruck, Body: sdk.StruckBody{
		Attacker: "char-1", Target: "skel-1", PresentationID: token,
	}}).GetStruck()
	require.Equal(t, token, struck.GetPresentationId())

	missed := mustEventToProto(t, sdk.Event{Kind: sdk.EventMissed, Body: sdk.MissedBody{
		Attacker: "char-1", Target: "skel-1", PresentationID: token,
	}}).GetMissed()
	require.Equal(t, token, missed.GetPresentationId())

	// A beat recorded before the field existed carries nothing, and empty is
	// the truth: the client reads it as "this roll has no shared presentation"
	// and narrates the swing alone rather than treating it as an error.
	old := mustEventToProto(t, sdk.Event{Kind: sdk.EventStruck, Body: sdk.StruckBody{
		Attacker: "char-1", Target: "skel-1",
	}}).GetStruck()
	require.Empty(t, old.GetPresentationId())
}

func TestStruckAndMissedCarryTheReaction(t *testing.T) {
	oa := &sdk.ReactionRef{Ref: "dnd5e:conditions:opportunity_attack", Name: "Opportunity Attack"}

	struck := mustEventToProto(t, sdk.Event{Kind: sdk.EventStruck, Body: sdk.StruckBody{
		Attacker: "char-1", Target: "skel-1", Reaction: oa,
	}}).GetStruck()
	require.Equal(t, "Opportunity Attack", struck.GetReaction().GetName())

	missed := mustEventToProto(t, sdk.Event{Kind: sdk.EventMissed, Body: sdk.MissedBody{
		Attacker: "char-1", Target: "skel-1", Reaction: oa,
	}}).GetMissed()
	require.Equal(t, "Opportunity Attack", missed.GetReaction().GetName())

	// An ordinary swing on the actor's own turn was taken as nothing, and
	// absent is the truth. False-vs-absent is the whole point of the field.
	plain := mustEventToProto(t, sdk.Event{Kind: sdk.EventStruck, Body: sdk.StruckBody{
		Attacker: "char-1", Target: "skel-1",
	}}).GetStruck()
	require.Nil(t, plain.GetReaction())
}

// TestEventRollWindowOpened_ReachesTheWireTyped is rpg-project#398's beat: a
// d20 is on the table, the fight is waiting on the member who rolled it, and
// the log says who is asked, what they hold and the two numbers they decide
// with.
//
// A SECOND KIND, not a second shape of WINDOW_OPENED. The movement window
// carries a mover and two cells; this one has neither, and the assertion that
// the movement body is absent is what keeps the two from being conflated.
func TestEventRollWindowOpened_ReachesTheWireTyped(t *testing.T) {
	got := mustEventToProto(t, sdk.Event{
		Session: "sess-1",
		Kind:    sdk.EventRollWindowOpened,
		Body: sdk.RollWindowOpenedBody{
			PresentationID: "opaque~d20",
			Audience:       "alice",
			Offer: sdk.ReactionRef{
				Ref: "dnd5e:conditions:inspired", Name: "Bardic Inspiration",
			},
			Roll:  15,
			Total: 20,
		},
	})

	require.Equal(t, sessionpb.EventKind_EVENT_KIND_ROLL_WINDOW_OPENED, got.GetKind())
	w := got.GetRollWindowOpened()
	require.NotNil(t, w, "the kind and the body arm are one-to-one")
	require.Equal(t, "alice", w.GetAudience(), "the audience is the roller, and one member")
	require.Equal(t, "opaque~d20", w.GetPresentationId())
	require.Equal(t, "dnd5e:conditions:inspired", w.GetOffer().GetRef())
	require.Equal(t, "Bardic Inspiration", w.GetOffer().GetName(), "the server authors the label")
	require.Equal(t, int32(15), w.GetRoll())
	require.Equal(t, int32(20), w.GetTotal())
	require.Nil(t, got.GetWindowOpened(),
		"a post-roll window is not a movement window: no mover, no cells, and no arm pretending otherwise")
}

// TestEventToProto_CarriesTheConcentrationBreak covers BOTH halves of the new
// beat (rpg-project#407, R10) in one assertion set, because either half alone
// still compiles and still ships a broken beat: an unmapped kind demotes to
// EVENT_KIND_UNKNOWN at the default arm, and an unmapped body simply stays
// nil. A break that arrives as a beat the client cannot read is exactly the
// silence the dedicated kind exists to prevent.
func TestEventToProto_CarriesTheConcentrationBreak(t *testing.T) {
	out := mustEventToProto(t, sdk.Event{
		Session: "sess-1",
		Seq:     7,
		Kind:    sdk.EventConcentrationEnded,
		Body: sdk.ConcentrationEndedBody{
			Caster: "bard-1",
			Spell: sdk.SpellRef{
				Ref: "dnd5e:spells:hold-person", Name: "Hold Person",
			},
			Reason: "damage",
		},
	})

	require.Equal(t, sessionpb.EventKind_EVENT_KIND_CONCENTRATION_ENDED, out.GetKind())

	body := out.GetConcentrationEnded()
	require.NotNil(t, body, "the kind converted but the body did not; the beat is unreadable")
	require.Equal(t, "bard-1", body.GetCaster())
	require.Equal(t, "dnd5e:spells:hold-person", body.GetSpell().GetRef())
	require.Equal(t, "Hold Person", body.GetSpell().GetName(),
		"the content authors the label; a client never derives it from the ref")
	require.Equal(t, "damage", body.GetReason(),
		"the rulebook's own word, copied verbatim rather than classified here")
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
	out := declarationToProto(sdk.Declaration{
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
	out := declarationToProto(sdk.Declaration{
		Verb:       sdk.VerbCast,
		Slot:       sdk.SlotAction,
		Available:  true,
		ID:         "decl-cast-2",
		TargetKind: sdk.TargetMember,
		Spell:      &sdk.SpellRef{Ref: "dnd5e:spells:vicious-mockery", Name: "Vicious Mockery"},
	})
	require.Empty(t, out.GetOptions(), "a spell with no menu offers no word to choose")
}

// TestConditionAppliedToProto_NamesWhoIsResponsible is the walk finding of
// 2026-09-12: the raw activation-result payload carried source_id and the
// typed conditionApplied.sourceId beside it was empty, because this converter
// copied three of the body's four fields.
//
// THE SOURCE IS PART OF THE CONDITION'S ADDRESS, not decoration. Two casters
// can each land Bane on the same fighter, and target+ref alone cannot tell
// those instances apart -- which is exactly what a client needs to say whose
// Command holds a creature, and which of two Banes ended when one caster's
// concentration broke.
func TestConditionAppliedToProto_NamesWhoIsResponsible(t *testing.T) {
	got := mustEventToProto(t, sdk.Event{
		Kind: sdk.EventActivationResult,
		Body: sdk.ActivationResultBody{
			Actor: "char_bard",
			ConditionApplied: &sdk.ConditionAppliedBody{
				Target:   "zombie-1",
				Ref:      "dnd5e:conditions:commanded",
				Name:     "the commanded condition",
				SourceID: "char_bard",
			},
		},
	})

	condition := got.GetActivationResult().GetConditionApplied()
	require.NotNil(t, condition)
	require.Equal(t, "char_bard", condition.GetSourceId(),
		"the typed field must say what the raw payload already said")
	require.Equal(t, "zombie-1", condition.GetTarget())
	require.Equal(t, "dnd5e:conditions:commanded", condition.GetRef())
	require.Equal(t, "the commanded condition", condition.GetName())
}

// The half that keeps the field honest: a body with no source leaves the wire
// field empty rather than borrowing the actor beside it.
//
// The actor is who ACTED and the source is who the condition answers to, and
// they are the same id often enough that filling one from the other would look
// right for a long time. A condition applied by no one -- terrain, a trap the
// rulebook does not attribute -- would then be blamed on whoever was standing
// there.
func TestConditionAppliedToProto_LeavesAnUnattributedConditionUnattributed(t *testing.T) {
	got := mustEventToProto(t, sdk.Event{
		Kind: sdk.EventActivationResult,
		Body: sdk.ActivationResultBody{
			Actor: "char_bard",
			ConditionApplied: &sdk.ConditionAppliedBody{
				Target: "zombie-1", Ref: "dnd5e:conditions:prone", Name: "Prone",
			},
		},
	})
	require.Empty(t, got.GetActivationResult().GetConditionApplied().GetSourceId())
}

// The removal beat carries the same address for the same reason, and dropped
// it in the same way. Without it "a Bane ended on the fighter" cannot say
// WHICH Bane, so a client holding two would have to guess which row to strike.
func TestConditionRemovedToProto_NamesWhoIsResponsible(t *testing.T) {
	got := mustEventToProto(t, sdk.Event{
		Kind: sdk.EventActivationResult,
		Body: sdk.ActivationResultBody{
			Actor: "char_bard",
			ConditionRemoved: &sdk.ConditionRemovedBody{
				Target:   "fighter",
				Ref:      "dnd5e:conditions:baned",
				Name:     "Bane",
				Reason:   "concentration ended",
				SourceID: "char_bard",
			},
		},
	})

	condition := got.GetActivationResult().GetConditionRemoved()
	require.NotNil(t, condition)
	require.Equal(t, "char_bard", condition.GetSourceId(),
		"which instance ended is the source plus the target plus the ref")
	require.Equal(t, "concentration ended", condition.GetReason())
}

// TestIntimidatedBodyToProto_CarriesTheWholeRollBothWays is the beat half of
// the first shenanigan (rpg-project#454), and the missed case is the one that
// matters most: IntimidateResponse carries no beaten, total or dc, so this
// beat is the ONLY account of the roll and the actor reads it here like every
// other witness. A body that failed to cross would leave the person who threw
// the die with nothing at all.
//
// FALSE IS AN ANSWER. The SDK writes beaten, dc and total with no omitempty
// precisely because `beaten: false` is the whole content of a missed threat,
// and this converter must not reintroduce the absence its author removed.
func TestIntimidatedBodyToProto_CarriesTheWholeRollBothWays(t *testing.T) {
	for _, tc := range []struct {
		name   string
		beaten bool
		total  int
	}{
		{name: "beaten", beaten: true, total: 14},
		{name: "missed", beaten: false, total: 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event := &sessionpb.Event{}
			setEventBody(event, sdk.IntimidatedBody{
				Actor: "char-alice", Target: "goblin-2", DC: 9, Total: tc.total, Beaten: tc.beaten,
			})

			got := event.GetIntimidated()
			require.NotNil(t, got, "a threat that reached this seam must reach the wire")
			require.Equal(t, "char-alice", got.GetActor())
			require.Equal(t, "goblin-2", got.GetTarget())
			require.Equal(t, int32(9), got.GetDc())
			require.Equal(t, int32(tc.total), got.GetTotal())
			require.Equal(t, tc.beaten, got.GetBeaten())
		})
	}
}

// TestIntimidatedBodyToProto_BeatenIsCopiedNeverDerived pins the reading as
// the provider's. A 20 the rulebook says did not beat a DC 9 crosses as
// beaten=false, because the day a rule changes what beating a DC means, every
// reader that compared total against dc would be wrong at once -- the law
// Saved.succeeded and DoorChanged.beaten already keep.
func TestIntimidatedBodyToProto_BeatenIsCopiedNeverDerived(t *testing.T) {
	event := &sessionpb.Event{}
	setEventBody(event, sdk.IntimidatedBody{
		Actor: "char-alice", Target: "goblin-2", DC: 9, Total: 20, Beaten: false,
	})
	require.False(t, event.GetIntimidated().GetBeaten())
	require.Equal(t, int32(20), event.GetIntimidated().GetTotal())
	require.Equal(t, int32(9), event.GetIntimidated().GetDc())
}

// TestEventIntimidatedKindToProto is the kind half. An unmapped kind does not
// fail -- it demotes to EVENT_KIND_UNKNOWN and the body stays nil -- so a
// missing arm here would lose the die for the whole table rather than degrade
// one field of it.
func TestEventIntimidatedKindToProto(t *testing.T) {
	require.Equal(t,
		sessionpb.EventKind_EVENT_KIND_INTIMIDATED,
		eventKindToProto(sdk.EventIntimidated))
}

// mustEventToProto is eventToProto for a scene that pins a PROJECTION rather
// than the refusal (rpg-project#458). The converter grew an error return when
// the `answered` beat arrived, because an outcome word this build cannot spell
// must not be demoted to ANSWER_WORD_UNSPECIFIED -- which is the wire's way of
// saying "the creature only spoke", a positive and false claim.
//
// THE REFUSAL HAS ITS OWN SCENES and they call the converter directly. Every
// other scene in this file is about a body that cannot fail, so it says so
// here once instead of forty-eight times, and a body that starts failing
// silently fails the scene that was not asking about it -- which is what a
// t.Fatal here is for.
func mustEventToProto(t *testing.T, e sdk.Event) *sessionpb.Event {
	t.Helper()
	evt, err := eventToProto(e)
	require.NoError(t, err, "this scene pins a projection, not a refusal")
	return evt
}

// mustEventsToProto is mustEventToProto's plural, for the catch-up read.
func mustEventsToProto(t *testing.T, es []sdk.Event) []*sessionpb.Event {
	t.Helper()
	out, err := eventsToProto(es)
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
