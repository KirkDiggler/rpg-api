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

func TestUnpack_Unauthenticated_Errors(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := &Handler{characters: anyMemberOwnedBy(ctrl, "alice")}
	_, err := h.Unpack(context.Background(), &sessionpb.UnpackRequest{
		Session: "sess-1", Actor: "char-1", ItemId: "explorers-pack", Quantity: 1,
	})
	requireCode(t, err, codes.Unauthenticated)
}

func TestUnpack_EmptyActor_IsRefused(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Unpack(ctx, &sessionpb.UnpackRequest{Session: "sess-1", ItemId: "explorers-pack", Quantity: 1})
	requireCode(t, err, codes.InvalidArgument)
}

// TestUnpack_ForeignActor_IsRefusedBeforeTheSDK is the entitlement gate's
// sharpest case, same reasoning as every other actor-taking verb's own: an
// unchecked Actor here would let a client unpack a pack while claiming to
// be anyone. The manager mock expects nothing -- the point is the call
// never reaches the SDK.
func TestUnpack_ForeignActor_IsRefusedBeforeTheSDK(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)

	h := &Handler{manager: mgr, characters: ownedCharacterRepo(ctrl, "char-bob", "bob")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Unpack(ctx, &sessionpb.UnpackRequest{
		Session: "sess-1", Actor: "char-bob", ItemId: "explorers-pack", Quantity: 1,
	})
	requireCode(t, err, codes.PermissionDenied)
}

// TestUnpack_HappyPath_RoutesVerbatimAndAcksOnly pins that all four request
// fields reach the SDK unchanged and the response carries only Saved/
// Delivery -- no descriptor and no Seq, Unpack's own shape (rpg-toolkit#1544):
// it never touches the encounter aggregate and has no story beat to number.
func TestUnpack_HappyPath_RoutesVerbatimAndAcksOnly(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Unpack(gomock.Any(), &sdk.UnpackInput{
		Session: "sess-1", Actor: "char-1", ItemID: "explorers-pack", Quantity: 2,
	}).Return(&sdk.UnpackOutput{
		Saved:    sdk.SaveReport{Written: []string{"character:char-1"}},
		Delivery: sdk.DeliveryReport{Events: 1},
	}, nil)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.Unpack(ctx, &sessionpb.UnpackRequest{
		Session: "sess-1", Actor: "char-1", ItemId: "explorers-pack", Quantity: 2,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"character:char-1"}, resp.GetSaved().GetWritten())
	require.Equal(t, int32(1), resp.GetDelivery().GetEvents())
}

func TestUnpack_ManagerError_TranslatesViaErrorTable(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Unpack(gomock.Any(), gomock.Any()).Return(nil, sdk.ErrNotAPack)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Unpack(ctx, &sessionpb.UnpackRequest{
		Session: "sess-1", Actor: "char-1", ItemId: "longsword", Quantity: 1,
	})
	// FailedPrecondition, not NotFound: longsword is a real, resolvable
	// catalog item -- it's simply not a pack.
	requireCode(t, err, codes.FailedPrecondition)
}
