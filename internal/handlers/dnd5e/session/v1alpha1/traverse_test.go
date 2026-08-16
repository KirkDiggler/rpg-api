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

func TestTraverse_Unauthenticated_Errors(t *testing.T) {
	h := &Handler{}
	_, err := h.Traverse(context.Background(), &sessionpb.TraverseRequest{})
	requireCode(t, err, codes.Unauthenticated)
}

func TestTraverse_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Traverse(gomock.Any(), &sdk.TraverseInput{
		Session: "sess-1", Member: "char-1", Connection: "door-1",
	}).Return(&sdk.TraverseOutput{FromRoom: "entrance", ToRoom: "hall", Seq: 4}, nil)

	h := &Handler{manager: mgr}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.Traverse(ctx, &sessionpb.TraverseRequest{Session: "sess-1", Member: "char-1", Connection: "door-1"})
	require.NoError(t, err)
	require.Equal(t, "entrance", resp.GetFromRoom())
	require.Equal(t, "hall", resp.GetToRoom())
}

func TestTraverse_ManagerError_TranslatesViaErrorTable(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Traverse(gomock.Any(), gomock.Any()).Return(nil, sdk.ErrNoConnection)

	h := &Handler{manager: mgr}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Traverse(ctx, &sessionpb.TraverseRequest{Session: "sess-1", Member: "char-1", Connection: "bogus"})
	requireCode(t, err, codes.InvalidArgument)
}
