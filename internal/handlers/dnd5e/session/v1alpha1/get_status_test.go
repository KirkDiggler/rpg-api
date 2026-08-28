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

func TestGetStatus_Unauthenticated_Errors(t *testing.T) {
	h := &Handler{}
	_, err := h.GetStatus(context.Background(), &sessionpb.GetStatusRequest{})
	requireCode(t, err, codes.Unauthenticated)
}

func TestGetStatus_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Status(gomock.Any(), &sdk.StatusInput{Session: "sess-1"}).Return(&sdk.Status{Open: true}, nil)

	h := &Handler{manager: mgr}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.GetStatus(ctx, &sessionpb.GetStatusRequest{Session: "sess-1"})
	require.NoError(t, err)
	require.True(t, resp.GetOpen())
}

func TestGetStatus_ManagerError_TranslatesViaErrorTable(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Status(gomock.Any(), gomock.Any()).Return(nil, sdk.ErrNoSession)

	h := &Handler{manager: mgr}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.GetStatus(ctx, &sessionpb.GetStatusRequest{Session: "bogus"})
	requireCode(t, err, codes.NotFound)
}
