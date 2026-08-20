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

func TestGetAtlas_Unauthenticated_Errors(t *testing.T) {
	h := &Handler{}
	_, err := h.GetAtlas(context.Background(), &sessionpb.GetAtlasRequest{})
	requireCode(t, err, codes.Unauthenticated)
}

func TestGetAtlas_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Atlas(gomock.Any(), &sdk.AtlasInput{Session: "sess-1"}).Return(&sdk.Atlas{
		Grid:  sdk.GridSquare,
		Cells: []spatial.Position{{X: 0, Y: 0}},
	}, nil)

	h := &Handler{manager: mgr}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.GetAtlas(ctx, &sessionpb.GetAtlasRequest{Session: "sess-1"})
	require.NoError(t, err)
	require.Len(t, resp.GetCells(), 1)
}

func TestGetAtlas_ManagerError_TranslatesViaErrorTable(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Atlas(gomock.Any(), gomock.Any()).Return(nil, sdk.ErrNoEncounter)

	h := &Handler{manager: mgr}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.GetAtlas(ctx, &sessionpb.GetAtlasRequest{Session: "sess-1"})
	requireCode(t, err, codes.NotFound)
}
