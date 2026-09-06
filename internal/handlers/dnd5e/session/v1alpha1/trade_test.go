package sessionv1alpha1

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"

	"github.com/KirkDiggler/rpg-toolkit/npc"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/npcs"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	sessionv1alpha1mock "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1/mock"
)

func TestTrade_Unauthenticated_Errors(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := &Handler{characters: anyMemberOwnedBy(ctrl, "alice")}
	_, err := h.Trade(context.Background(), &sessionpb.TradeRequest{})
	requireCode(t, err, codes.Unauthenticated)
}

func TestTrade_HappyPath_ReturnsDecrementedDescriptor(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().
		Trade(gomock.Any(), &sdk.TradeInput{
			Session: "sess-1", Actor: "char-1", Target: "demo-merchant-1", Range: 1,
			Give:    sdk.TradeOffer{Items: []sdk.TradeItem{}},
			Receive: sdk.TradeOffer{Items: []sdk.TradeItem{{Type: shared.EquipmentTypeWeapon, ID: "longsword", Quantity: 1}}},
		}).
		Return(&sdk.TradeOutput{
			Descriptor: sdk.WorldNPCDescriptor{
				TargetID:     "demo-merchant-1",
				DisplayName:  "Demo Merchant",
				Capabilities: []npc.Capability{npc.CapabilityVendor},
				CombatPolicy: npc.CombatPolicyNonCombatant,
				// The longsword is gone from stock after the buy -- the
				// caller refreshes its display from this response without a
				// second Interact round trip.
				Inventory: []npcs.StockEntryView{
					{Type: shared.EquipmentTypeWeapon, ID: "longbow", Name: "Longbow", Mode: npcs.StockModeLimited, Quantity: 1},
				},
			},
			Seq: 7,
		}, nil)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.Trade(ctx, &sessionpb.TradeRequest{
		Session: "sess-1", Actor: "char-1", Target: "demo-merchant-1", Range: 1,
		Receive: &sessionpb.TradeOffer{Items: []*sessionpb.TradeItem{
			{EquipmentType: "weapon", EquipmentId: "longsword", Quantity: 1},
		}},
	})
	require.NoError(t, err)

	d := resp.GetDescriptor_()
	require.Equal(t, "demo-merchant-1", d.GetTargetId())
	require.Equal(t, uint64(7), resp.GetSeq())
	require.Len(t, d.GetInventory(), 1, "the bought longsword is gone, the longbow remains")
	require.Equal(t, "longbow", d.GetInventory()[0].GetEquipmentId())
}

// TestTrade_GiveIsForwardedUntouched proves this handler does not
// pre-validate or drop a populated `give`: it hands the offer to
// session.Trade exactly as the wire carried it, and returns whatever that
// verb says about it (design rule 8).
//
// Give is a real offer now -- selling arrived in rpg-toolkit#1535 -- so what
// makes a populated one legal or not is the SDK's rule about the trade, not a
// shape this boundary is allowed to correct. The refusal below stands in for
// any of them; what the test pins is that the shield reached the SDK intact
// and the answer came back as FailedPrecondition rather than being turned
// into a nil check here.
func TestTrade_GiveIsForwardedUntouched(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().
		Trade(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in *sdk.TradeInput) (*sdk.TradeOutput, error) {
			require.Len(t, in.Give.Items, 1)
			require.Equal(t, "shield", in.Give.Items[0].ID)
			return nil, sdk.ErrInvalidTradeOffer
		})

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Trade(ctx, &sessionpb.TradeRequest{
		Session: "sess-1", Actor: "char-1", Target: "demo-merchant-1",
		Give: &sessionpb.TradeOffer{Items: []*sessionpb.TradeItem{
			{EquipmentType: "armor", EquipmentId: "shield", Quantity: 1},
		}},
		Receive: &sessionpb.TradeOffer{Items: []*sessionpb.TradeItem{
			{EquipmentType: "weapon", EquipmentId: "longsword", Quantity: 1},
		}},
	})
	// FailedPrecondition, not InvalidArgument: give is a legal field on a
	// legal message, refused by the SDK's own rule about the offer, not a
	// malformed request.
	requireCode(t, err, codes.FailedPrecondition)
}

func TestTrade_ManagerError_TranslatesViaErrorTable(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Trade(gomock.Any(), gomock.Any()).Return(nil, sdk.ErrOutOfStock)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Trade(ctx, &sessionpb.TradeRequest{
		Session: "sess-1", Actor: "char-1", Target: "demo-merchant-1",
		Receive: &sessionpb.TradeOffer{Items: []*sessionpb.TradeItem{
			{EquipmentType: "weapon", EquipmentId: "longsword", Quantity: 99},
		}},
	})
	requireCode(t, err, codes.FailedPrecondition)
}

// TestTrade_ForeignActor_IsRefusedBeforeTheSDK is the entitlement gate's
// sharpest case, same reasoning as Interact's own: Actor is the acting-as
// member, and an unchecked one here would let a client buy an item while
// claiming to be anyone. The manager mock expects nothing -- the point is
// the call never reaches the SDK.
func TestTrade_ForeignActor_IsRefusedBeforeTheSDK(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)

	h := &Handler{manager: mgr, characters: ownedCharacterRepo(ctrl, "goblin-1", "someone-else")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Trade(ctx, &sessionpb.TradeRequest{
		Session: "sess-1", Actor: "goblin-1", Target: "demo-merchant-1",
		Receive: &sessionpb.TradeOffer{Items: []*sessionpb.TradeItem{
			{EquipmentType: "weapon", EquipmentId: "longsword", Quantity: 1},
		}},
	})
	requireCode(t, err, codes.PermissionDenied)
}

func TestTrade_EmptyActor_IsRefused(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Trade(ctx, &sessionpb.TradeRequest{Session: "sess-1", Target: "demo-merchant-1"})
	requireCode(t, err, codes.InvalidArgument)
}
