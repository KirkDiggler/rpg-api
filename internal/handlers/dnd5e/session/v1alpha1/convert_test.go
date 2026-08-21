package sessionv1alpha1

import (
	"testing"

	"github.com/stretchr/testify/require"

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
	require.Equal(t, sessionpb.GridKind_GRID_KIND_SQUARE, gridKindToProto(sdk.GridSquare))
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
	out := atlasToProto(&sdk.Atlas{Grid: sdk.GridHex, Layout: sdk.HexLayoutPointyTop})
	require.Equal(t, sessionpb.GridKind_GRID_KIND_HEX, out.GetGrid())
	require.Equal(t, sessionpb.HexLayout_HEX_LAYOUT_POINTY_TOP, out.GetLayout())
}

func TestClockKindToProto(t *testing.T) {
	require.Equal(t, sessionpb.ClockKind_CLOCK_KIND_WORLD, clockKindToProto(sdk.ClockWorld))
	require.Equal(t, sessionpb.ClockKind_CLOCK_KIND_TURN, clockKindToProto(sdk.ClockTurn))
	require.Equal(t, sessionpb.ClockKind_CLOCK_KIND_UNSPECIFIED, clockKindToProto(sdk.ClockKind("bogus")))
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
	got := atlasToProto(nil)
	require.NotNil(t, got)
	require.Empty(t, got.GetCells())
	require.Empty(t, got.GetDoorways())
}

func TestAtlasToProto_Populated(t *testing.T) {
	a := &sdk.Atlas{
		Grid:  sdk.GridSquare,
		Cells: []spatial.Position{{X: 0, Y: 0}, {X: 1, Y: 0}},
		Props: []sdk.AtlasProp{
			// A pillar: blocks both. And a pile of bones: blocks NEITHER --
			// walked through and seen over. The second one is the case the
			// old Occluders list could not express at all, and the reason
			// this conversion carries two independent bools instead of
			// membership in a single list.
			{Ref: "pillar", At: spatial.Position{X: 1, Y: 1}, BlocksMovement: true, BlocksLineOfSight: true},
			{Ref: "bones", At: spatial.Position{X: 2, Y: 2}, BlocksMovement: false, BlocksLineOfSight: false},
		},
		Boundaries: []sdk.AtlasBoundary{
			{From: spatial.Position{X: 0, Y: 0}, To: spatial.Position{X: 1, Y: 0}, BlocksMovement: true, BlocksLineOfSight: true},
		},
		Doorways: []sdk.AtlasDoorway{
			{Connection: "door-1", From: spatial.Position{X: 5, Y: 1}, To: spatial.Position{X: 6, Y: 1}},
		},
	}
	got := atlasToProto(a)
	require.Equal(t, sessionpb.GridKind_GRID_KIND_SQUARE, got.GetGrid())
	require.Len(t, got.GetCells(), 2)
	require.Len(t, got.GetProps(), 2)

	pillar := got.GetProps()[0]
	require.Equal(t, "pillar", pillar.GetRef())
	require.Equal(t, 1.0, pillar.GetAt().GetX())
	require.True(t, pillar.GetBlocksMovement())
	require.True(t, pillar.GetBlocksLineOfSight())

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

	require.Len(t, got.GetBoundaries(), 1)
	require.True(t, got.GetBoundaries()[0].GetBlocksMovement())

	require.Len(t, got.GetDoorways(), 1)
	dw := got.GetDoorways()[0]
	require.Equal(t, "door-1", dw.GetConnection())
	require.Equal(t, 5.0, dw.GetFrom().GetX())
	require.Equal(t, 6.0, dw.GetTo().GetX())
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

func TestStoryEntriesToProto(t *testing.T) {
	got := storyEntriesToProto([]sdk.StoryEntry{
		{Seq: 1, At: 10, Correlation: "corr-1", Tags: map[string]string{"k": "v"}, Payload: []byte("p")},
	})
	require.Len(t, got, 1)
	require.Equal(t, uint64(1), got[0].GetSeq())
	require.Equal(t, "corr-1", got[0].GetCorrelation())
	require.Equal(t, map[string]string{"k": "v"}, got[0].GetTags())
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
