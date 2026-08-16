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
	m := sdk.Member{ID: "alice", Kind: sdk.KindPlayer, Room: "entrance"}
	got := memberToProto(m)
	require.Equal(t, "alice", got.GetId())
	require.Equal(t, sessionpb.MemberKind_MEMBER_KIND_PLAYER, got.GetKind())
	require.Equal(t, "entrance", got.GetRoom())
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
			{ID: "alice", Room: "vault", Position: spatial.Position{X: 1, Y: 2}},
		},
	}
	got := outcomeToProto(o)
	require.Equal(t, "victory", got.GetEnding())
	require.Equal(t, uint64(42), got.GetAt())
	require.Len(t, got.GetMembers(), 1)
	require.Equal(t, "alice", got.GetMembers()[0].GetId())
	require.Equal(t, "vault", got.GetMembers()[0].GetRoom())
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
	require.Empty(t, got.GetRooms())
	require.Empty(t, got.GetDoorways())
}

func TestAtlasToProto_Populated(t *testing.T) {
	a := &sdk.Atlas{
		Rooms: []sdk.AtlasRoom{
			{
				ID: "entrance", Grid: sdk.GridSquare,
				Origin: spatial.Position{X: 0, Y: 0}, Width: 5, Height: 5,
				Cells:     []spatial.Position{{X: 0, Y: 0}},
				Occluders: []spatial.Position{{X: 1, Y: 1}},
				Boundaries: []sdk.AtlasBoundary{
					{From: spatial.Position{X: 0, Y: 0}, To: spatial.Position{X: 1, Y: 0}, BlocksMovement: true, BlocksLineOfSight: true},
				},
			},
		},
		Doorways: []sdk.AtlasDoorway{
			{Connection: "door-1", From: "entrance", FromCell: spatial.Position{X: 5, Y: 1}, To: "hall", ToCell: spatial.Position{X: 6, Y: 1}},
		},
	}
	got := atlasToProto(a)
	require.Len(t, got.GetRooms(), 1)
	room := got.GetRooms()[0]
	require.Equal(t, "entrance", room.GetId())
	require.Equal(t, sessionpb.GridKind_GRID_KIND_SQUARE, room.GetGrid())
	require.Equal(t, int32(5), room.GetWidth())
	require.Len(t, room.GetCells(), 1)
	require.Len(t, room.GetOccluders(), 1)
	require.Len(t, room.GetBoundaries(), 1)
	require.True(t, room.GetBoundaries()[0].GetBlocksMovement())

	require.Len(t, got.GetDoorways(), 1)
	dw := got.GetDoorways()[0]
	require.Equal(t, "door-1", dw.GetConnection())
	require.Equal(t, "entrance", dw.GetFrom())
	require.Equal(t, "hall", dw.GetTo())
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
