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
	ctrl := gomock.NewController(t)
	h := &Handler{characters: anyMemberOwnedBy(ctrl, "alice")}
	_, err := h.GetAtlas(context.Background(), &sessionpb.GetAtlasRequest{})
	requireCode(t, err, codes.Unauthenticated)
}

func TestGetAtlas_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Atlas(gomock.Any(), &sdk.AtlasInput{Session: "sess-1", Member: "char-1"}).Return(&sdk.Atlas{
		Grid:  sdk.GridHex,
		Cells: []spatial.Position{{X: 0, Y: 0}},
	}, nil)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.GetAtlas(ctx, &sessionpb.GetAtlasRequest{Session: "sess-1", Member: "char-1"})
	require.NoError(t, err)
	require.Len(t, resp.GetCells(), 1)
}

func TestGetAtlas_ManagerError_TranslatesViaErrorTable(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Atlas(gomock.Any(), gomock.Any()).Return(nil, sdk.ErrNoEncounter)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.GetAtlas(ctx, &sessionpb.GetAtlasRequest{Session: "sess-1", Member: "char-1"})
	requireCode(t, err, codes.NotFound)
}

// TestGetAtlas_ForeignMember_IsRefusedBeforeTheSDK mirrors GetWhere's own
// sharpest case: concealment makes the atlas a member-scoped answer
// (rpg-api-protos#266), so an unchecked member here would let a client read
// another member's revealed structure by naming their ID. The manager mock
// expects nothing: the point is the call never reaches the SDK at all.
func TestGetAtlas_ForeignMember_IsRefusedBeforeTheSDK(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)

	h := &Handler{manager: mgr, characters: ownedCharacterRepo(ctrl, "goblin-1", "someone-else")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.GetAtlas(ctx, &sessionpb.GetAtlasRequest{Session: "sess-1", Member: "goblin-1"})
	requireCode(t, err, codes.PermissionDenied)
}

func TestGetAtlas_EmptyMember_IsRefused(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.GetAtlas(ctx, &sessionpb.GetAtlasRequest{Session: "sess-1"})
	requireCode(t, err, codes.InvalidArgument)
}
