package sessionv1alpha1

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	sessionv1alpha1mock "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1/mock"
)

func TestEndTurn_Unauthenticated_Errors(t *testing.T) {
	h := &Handler{}
	_, err := h.EndTurn(context.Background(), &sessionpb.EndTurnRequest{})
	requireCode(t, err, codes.Unauthenticated)
}

func TestEndTurn_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().EndTurn(gomock.Any(), &sdk.EndTurnInput{
		Session: "sess-1", Member: "char-1", DeclarationID: "decl-end-1",
	}).Return(&sdk.EndTurnOutput{
		Next: "goblin-1", RoundWrapped: true, Seq: 6,
	}, nil)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.EndTurn(ctx, &sessionpb.EndTurnRequest{
		Session: "sess-1", Member: "char-1", DeclarationId: "decl-end-1",
	})
	require.NoError(t, err)
	require.Equal(t, "goblin-1", resp.GetNext())
	require.True(t, resp.GetRoundWrapped())
}

func TestEndTurn_ManagerError_TranslatesViaErrorTable(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().EndTurn(gomock.Any(), gomock.Any()).Return(nil, sdk.ErrNotInFight)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.EndTurn(ctx, &sessionpb.EndTurnRequest{Session: "sess-1", Member: "char-1"})
	requireCode(t, err, codes.FailedPrecondition)
}
