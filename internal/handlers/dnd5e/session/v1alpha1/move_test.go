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
		Steps: []sdk.Step{{Position: spatial.Position{X: 1, Y: 1}, Seq: 1}, {Position: spatial.Position{X: 2, Y: 1}, Seq: 2}},
	}, nil)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := authedCtx("alice")
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
	ctx := authedCtx("alice")
	_, err := h.Move(ctx, &sessionpb.MoveRequest{Session: "sess-1", Member: "char-1", Path: []*sessionpb.Position{{X: 1, Y: 1}}})
	requireCode(t, err, codes.InvalidArgument)
}

func TestMove_JoinsCombatWithoutInventingAStep(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Move(gomock.Any(), gomock.Any()).Return(&sdk.MoveOutput{JoinedCombat: true, Status: sdk.MovementStopped}, nil)
	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	out, err := h.Move(authedCtx("alice"), &sessionpb.MoveRequest{Session: "sess", Member: "char-1", Path: []*sessionpb.Position{{X: 1}}})
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
	out, err := h.Move(authedCtx("alice"), &sessionpb.MoveRequest{Session: "sess", Member: "char-1", Path: []*sessionpb.Position{{X: 1}}})
	require.NoError(t, err)
	require.Equal(t, sessionpb.MovementStatus_MOVEMENT_STATUS_PAUSED, out.GetStatus())
	require.False(t, out.GetJoinedCombat())
	require.Empty(t, out.GetSteps())
}
