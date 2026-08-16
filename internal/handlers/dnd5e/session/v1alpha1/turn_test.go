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

func TestTurn_Unauthenticated_Errors(t *testing.T) {
	h := &Handler{}
	_, err := h.Turn(context.Background(), &sessionpb.TurnRequest{})
	requireCode(t, err, codes.Unauthenticated)
}

func TestTurn_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Turn(gomock.Any(), &sdk.TurnInput{Session: "sess-1", Member: "char-1"}).Return(&sdk.TurnOutput{
		Clock: sdk.ClockTurn, Active: "char-1", Round: 2, Order: []string{"char-1", "goblin-1"},
	}, nil)

	h := &Handler{manager: mgr}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.Turn(ctx, &sessionpb.TurnRequest{Session: "sess-1", Member: "char-1"})
	require.NoError(t, err)
	require.Equal(t, sessionpb.ClockKind_CLOCK_KIND_TURN, resp.GetClock())
	require.Equal(t, "char-1", resp.GetActive())
	require.Equal(t, int32(2), resp.GetRound())
}

func TestTurn_ManagerError_TranslatesViaErrorTable(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Turn(gomock.Any(), gomock.Any()).Return(nil, sdk.ErrNoMember)

	h := &Handler{manager: mgr}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Turn(ctx, &sessionpb.TurnRequest{Session: "sess-1", Member: "char-1"})
	requireCode(t, err, codes.NotFound)
}
