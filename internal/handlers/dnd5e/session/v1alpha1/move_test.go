package sessionv1alpha1

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	sessionv1alpha1mock "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1/mock"
)

func TestMove_Unauthenticated_Errors(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := &Handler{characters: anyMemberOwnedBy(ctrl, "alice")}
	_, err := h.Move(context.Background(), &sessionpb.MoveRequest{})
	requireCode(t, err, codes.Unauthenticated)
}

func TestMove_HappyPath_TranslatesPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Move(gomock.Any(), &sdk.MoveInput{
		Session: "sess-1", Member: "char-1", DeclarationID: "decl-move-1",
		Path: []spatial.Position{{X: 1, Y: 1}, {X: 2, Y: 1}},
	}).Return(&sdk.MoveOutput{
		Steps: []sdk.Step{{Position: spatial.Position{X: 1, Y: 1}}, {Position: spatial.Position{X: 2, Y: 1}}},
	}, nil)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.Move(ctx, &sessionpb.MoveRequest{
		Session: "sess-1", Member: "char-1", DeclarationId: "decl-move-1",
		Path: []*sessionpb.Position{{X: 1, Y: 1}, {X: 2, Y: 1}},
	})
	require.NoError(t, err)
	require.Len(t, resp.GetSteps(), 2)
}

func TestMove_ManagerError_TranslatesViaErrorTable(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Move(gomock.Any(), gomock.Any()).Return(nil, sdk.ErrBrokenPath)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Move(ctx, &sessionpb.MoveRequest{Session: "sess-1", Member: "char-1", Path: []*sessionpb.Position{{X: 1, Y: 1}}})
	requireCode(t, err, codes.InvalidArgument)
}

func TestMove_JoinsCombatWithoutInventingAStep(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Move(gomock.Any(), gomock.Any()).Return(&sdk.MoveOutput{JoinedCombat: true, Status: sdk.MovementStopped}, nil)
	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	out, err := h.Move(auth.WithPlayerID(context.Background(), "alice"), &sessionpb.MoveRequest{Session: "sess", Member: "char-1", Path: []*sessionpb.Position{{X: 1}}})
	require.NoError(t, err)
	require.True(t, out.GetJoinedCombat())
	require.Equal(t, sessionpb.MovementStatus_MOVEMENT_STATUS_STOPPED, out.GetStatus())
	require.Empty(t, out.GetSteps())
}

func TestMove_PreservesReactionPause(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Move(gomock.Any(), gomock.Any()).Return(&sdk.MoveOutput{Status: sdk.MovementPaused}, nil)
	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	out, err := h.Move(auth.WithPlayerID(context.Background(), "alice"), &sessionpb.MoveRequest{Session: "sess", Member: "char-1", Path: []*sessionpb.Position{{X: 1}}})
	require.NoError(t, err)
	require.Equal(t, sessionpb.MovementStatus_MOVEMENT_STATUS_PAUSED, out.GetStatus())
	require.False(t, out.GetJoinedCombat())
	require.Empty(t, out.GetSteps())
}

// TestMove_BarredStopsWithTheReason pins rpg-project#842 (R2): a walk a body
// stopped answers, never errs. It crosses STOPPED with the session's reason
// and no step for the bump; the response has no field for the barrier, which
// the `barred` act on the stream names.
func TestMove_BarredStopsWithTheReason(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Move(gomock.Any(), gomock.Any()).Return(&sdk.MoveOutput{
		Status: sdk.MovementStopped, StopReason: "Gate Guard bars the way",
		Barred: &sdk.Barred{Barrier: "gate-1", Cell: spatial.Position{X: 0, Y: 6}, Reason: "Gate Guard bars the way"},
	}, nil)
	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	out, err := h.Move(auth.WithPlayerID(context.Background(), "alice"), &sessionpb.MoveRequest{Session: "sess", Member: "char-1", Path: []*sessionpb.Position{{X: 0, Y: 6}}})
	require.NoError(t, err)
	require.Equal(t, sessionpb.MovementStatus_MOVEMENT_STATUS_STOPPED, out.GetStatus())
	require.Equal(t, "Gate Guard bars the way", out.GetStopReason())
	require.Empty(t, out.GetSteps(), "the bump is no step")
}

// A walk that answers before the world runs (rpg-project#879 R2) says so on
// the wire: continues, the record id the rest follows on the stream, the
// CONTINUING status, and empty save and delivery reports, since both belong
// to the rest of the walk.
func TestMove_ContinuingAnswer_CarriesContinuesAndThrough(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Move(gomock.Any(), gomock.Any()).Return(&sdk.MoveOutput{
		Status:    sdk.MovementContinuing,
		Steps:     []sdk.Step{{Position: spatial.Position{X: 1, Y: 1}}, {Position: spatial.Position{X: 2, Y: 1}}},
		Continues: true,
		Through:   "record-42",
	}, nil)
	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	out, err := h.Move(auth.WithPlayerID(context.Background(), "alice"), &sessionpb.MoveRequest{
		Session: "sess", Member: "char-1",
		Path: []*sessionpb.Position{{X: 1, Y: 1}, {X: 2, Y: 1}, {X: 3, Y: 1}},
	})
	require.NoError(t, err)
	require.True(t, out.GetContinues())
	require.Equal(t, "record-42", out.GetThrough())
	require.Equal(t, sessionpb.MovementStatus_MOVEMENT_STATUS_CONTINUING, out.GetStatus())
	require.Len(t, out.GetSteps(), 2)
	require.Empty(t, out.GetSaved().GetWritten())
	require.Empty(t, out.GetSaved().GetFailed())
	require.False(t, out.GetDelivery().GetFailed())
}

// A whole walk never claims to continue.
func TestMove_WholeAnswer_DoesNotContinue(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Move(gomock.Any(), gomock.Any()).Return(&sdk.MoveOutput{
		Status: sdk.MovementCompleted,
		Saved:  sdk.SaveReport{Written: []string{"encounter:sess"}},
	}, nil)
	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	out, err := h.Move(auth.WithPlayerID(context.Background(), "alice"), &sessionpb.MoveRequest{Session: "sess", Member: "char-1", Path: []*sessionpb.Position{{X: 1}}})
	require.NoError(t, err)
	require.False(t, out.GetContinues())
	require.Empty(t, out.GetThrough())
	require.Equal(t, sessionpb.MovementStatus_MOVEMENT_STATUS_COMPLETED, out.GetStatus())
	require.Equal(t, []string{"encounter:sess"}, out.GetSaved().GetWritten())
}
