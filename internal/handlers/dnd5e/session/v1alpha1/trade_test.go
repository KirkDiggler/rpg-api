package sessionv1alpha1

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"

	"github.com/KirkDiggler/rpg-toolkit/npc"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/equipment"
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

	// The real longsword price, not a hardcoded number -- pins that Give.currency
	// (rpg-toolkit#1534) actually reaches the SDK input untouched.
	price, err := equipment.PriceOf("longsword")
	require.NoError(t, err)

	mgr.EXPECT().
		Trade(gomock.Any(), &sdk.TradeInput{
			Session: "sess-1", Actor: "char-1", Target: "demo-merchant-1", Range: 1,
			Give:    sdk.TradeOffer{Items: []sdk.TradeItem{}, Currency: price},
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
		Give: &sessionpb.TradeOffer{Currency: &sessionpb.Money{Copper: int32(price.Copper)}},
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

// TestTrade_BothGiveAndReceiveItems_IsRefused proves this handler does not
// pre-validate or drop a populated `give` -- it forwards both sides exactly
// as given (design rule 8). Naming items on both sides at once is barter,
// not yet supported (rpg-toolkit#1537 retired the buy-only wave's
// ErrGiveNotSupported now that a populated Give has a legal meaning of its
// own: selling), and that shape refusal is session.Trade's own rule to
// enforce, not a silent correction made at the wire boundary.
func TestTrade_BothGiveAndReceiveItems_IsRefused(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().
		Trade(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in *sdk.TradeInput) (*sdk.TradeOutput, error) {
			require.Len(t, in.Give.Items, 1)
			require.Equal(t, "shield", in.Give.Items[0].ID)
			require.Len(t, in.Receive.Items, 1)
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
	// FailedPrecondition, not InvalidArgument: give and receive are both
	// legal fields on a legal message, refused by this wave's own rule
	// (exactly one direction per call), not a malformed request.
	requireCode(t, err, codes.FailedPrecondition)
}

// TestTrade_Sell_ForwardsGiveItemsAndReceiveCurrency is the sell direction's
// own happy path (rpg-toolkit#1537), the mirror of
// TestTrade_HappyPath_ReturnsDecrementedDescriptor: Give now names the item
// changing hands and Receive carries the payout, and the response's
// inventory reflects the vendor's stock GROWING with a PlayerSold row.
func TestTrade_Sell_ForwardsGiveItemsAndReceiveCurrency(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)

	price, err := equipment.PriceOf("longsword")
	require.NoError(t, err)

	mgr.EXPECT().
		Trade(gomock.Any(), &sdk.TradeInput{
			Session: "sess-1", Actor: "char-1", Target: "demo-merchant-1", Range: 1,
			Give:    sdk.TradeOffer{Items: []sdk.TradeItem{{Type: shared.EquipmentTypeWeapon, ID: "longsword", Quantity: 1}}},
			Receive: sdk.TradeOffer{Items: []sdk.TradeItem{}, Currency: price},
		}).
		Return(&sdk.TradeOutput{
			Descriptor: sdk.WorldNPCDescriptor{
				TargetID:     "demo-merchant-1",
				DisplayName:  "Demo Merchant",
				Capabilities: []npc.Capability{npc.CapabilityVendor},
				CombatPolicy: npc.CombatPolicyNonCombatant,
				// The sold longsword joined the vendor's stock, flagged as a
				// player's sale -- proves AddToVendorStock's row survives to
				// the wire, not just that stock exists.
				Inventory: []npcs.StockEntryView{
					{Type: shared.EquipmentTypeWeapon, ID: "longsword", Name: "Longsword", Mode: npcs.StockModeLimited, Quantity: 1, PlayerSold: true},
				},
			},
			Seq: 8,
		}, nil)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.Trade(ctx, &sessionpb.TradeRequest{
		Session: "sess-1", Actor: "char-1", Target: "demo-merchant-1", Range: 1,
		Give: &sessionpb.TradeOffer{Items: []*sessionpb.TradeItem{
			{EquipmentType: "weapon", EquipmentId: "longsword", Quantity: 1},
		}},
		Receive: &sessionpb.TradeOffer{Currency: &sessionpb.Money{Copper: int32(price.Copper)}},
	})
	require.NoError(t, err)

	d := resp.GetDescriptor_()
	require.Equal(t, uint64(8), resp.GetSeq())
	require.Len(t, d.GetInventory(), 1)
	require.Equal(t, "longsword", d.GetInventory()[0].GetEquipmentId())
	require.True(t, d.GetInventory()[0].GetPlayerSold())
}

// TestTrade_NotInInventory_IsRefused pins selling something the actor
// doesn't hold enough of (rpg-toolkit#1537) -- the sell direction's mirror
// of buy's ErrOutOfStock, same FailedPrecondition bucket.
func TestTrade_NotInInventory_IsRefused(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Trade(gomock.Any(), gomock.Any()).Return(nil, sdk.ErrNotInInventory)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Trade(ctx, &sessionpb.TradeRequest{
		Session: "sess-1", Actor: "char-1", Target: "demo-merchant-1",
		Give: &sessionpb.TradeOffer{Items: []*sessionpb.TradeItem{
			{EquipmentType: "weapon", EquipmentId: "longsword", Quantity: 1},
		}},
		Receive: &sessionpb.TradeOffer{Currency: &sessionpb.Money{Copper: 1}},
	})
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

// TestTrade_WrongPrice_IsRefused pins that an offered price the SDK refuses
// (rpg-toolkit#1534: the server alone decides what's correct, never a
// trusted client amount) translates to FAILED_PRECONDITION -- the same
// well-formed-call-the-world-refuses bucket as ErrOutOfStock above.
func TestTrade_WrongPrice_IsRefused(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Trade(gomock.Any(), gomock.Any()).Return(nil, sdk.ErrWrongPrice)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Trade(ctx, &sessionpb.TradeRequest{
		Session: "sess-1", Actor: "char-1", Target: "demo-merchant-1",
		Give: &sessionpb.TradeOffer{Currency: &sessionpb.Money{Copper: 1}},
		Receive: &sessionpb.TradeOffer{Items: []*sessionpb.TradeItem{
			{EquipmentType: "weapon", EquipmentId: "longsword", Quantity: 1},
		}},
	})
	requireCode(t, err, codes.FailedPrecondition)
}

// TestTrade_InsufficientFunds_IsRefused pins the actor-can't-pay refusal,
// distinct from TestTrade_WrongPrice_IsRefused: the right amount was named,
// the wallet just doesn't hold it.
func TestTrade_InsufficientFunds_IsRefused(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Trade(gomock.Any(), gomock.Any()).Return(nil, sdk.ErrInsufficientFunds)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	price, err := equipment.PriceOf("longsword")
	require.NoError(t, err)
	_, err = h.Trade(ctx, &sessionpb.TradeRequest{
		Session: "sess-1", Actor: "char-1", Target: "demo-merchant-1",
		Give: &sessionpb.TradeOffer{Currency: &sessionpb.Money{Copper: int32(price.Copper)}},
		Receive: &sessionpb.TradeOffer{Items: []*sessionpb.TradeItem{
			{EquipmentType: "weapon", EquipmentId: "longsword", Quantity: 1},
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
