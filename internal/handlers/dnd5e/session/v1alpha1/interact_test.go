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

func TestInteract_Unauthenticated_Errors(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := &Handler{characters: anyMemberOwnedBy(ctrl, "alice"), roster: testRoster()}
	_, err := h.Interact(context.Background(), &sessionpb.InteractRequest{})
	requireCode(t, err, codes.Unauthenticated)
}

func TestInteract_HappyPath_ReturnsDescriptor(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().
		Interact(gomock.Any(), &sdk.InteractInput{Session: "sess-1", Actor: "char-1", Target: "demo-merchant-1", Range: 2}).
		Return(&sdk.InteractOutput{
			Descriptor: sdk.WorldNPCDescriptor{
				TargetID:     "demo-merchant-1",
				Ref:          "dnd5e:npcs:merchant",
				DisplayName:  "Demo Merchant",
				Capabilities: []npc.Capability{npc.CapabilityVendor},
				CombatPolicy: npc.CombatPolicyNonCombatant,
				Inventory: []npcs.StockEntryView{
					{Type: shared.EquipmentTypeWeapon, ID: "longsword", Name: "Longsword", Mode: npcs.StockModeLimited, Quantity: 1},
				},
			},
			Seq: 42,
		}, nil)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice"), roster: testRoster()}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.Interact(ctx, &sessionpb.InteractRequest{
		Session: "sess-1", Actor: "char-1", Target: "demo-merchant-1", Range: 2,
	})
	require.NoError(t, err)

	d := resp.GetDescriptor_()
	require.Equal(t, "demo-merchant-1", d.GetTargetId())
	require.Equal(t, "dnd5e:npcs:merchant", d.GetRef())
	require.Equal(t, "Demo Merchant", d.GetDisplayName())
	require.Equal(t, []string{"vendor"}, d.GetCapabilities())
	require.Equal(t, "non_combatant", d.GetCombatPolicy())
	require.Equal(t, uint64(42), resp.GetSeq())
	require.Len(t, d.GetInventory(), 1)
	require.Equal(t, "longsword", d.GetInventory()[0].GetEquipmentId())
	require.Equal(t, sessionpb.VendorStockMode_VENDOR_STOCK_MODE_LIMITED, d.GetInventory()[0].GetStockMode())
	require.Equal(t, int32(1), d.GetInventory()[0].GetQuantity())
}

func TestInteract_ManagerError_TranslatesViaErrorTable(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Interact(gomock.Any(), gomock.Any()).Return(nil, sdk.ErrOutOfRange)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice"), roster: testRoster()}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Interact(ctx, &sessionpb.InteractRequest{Session: "sess-1", Actor: "char-1", Target: "demo-merchant-1"})
	// FailedPrecondition, not NotFound: the actor and target both exist, the
	// reach just doesn't hold right now -- the same bucket ErrOutOfReach
	// (Attack's own reach refusal) already sits in.
	requireCode(t, err, codes.FailedPrecondition)
}

// TestInteract_ForeignActor_IsRefusedBeforeTheSDK is the entitlement gate's
// sharpest case: Interact's Actor is the acting-as member, and an unchecked
// one here would let a client reach for a world NPC while claiming to be
// anyone. The manager mock expects nothing -- the point is the call never
// reaches the SDK.
func TestInteract_ForeignActor_IsRefusedBeforeTheSDK(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)

	h := &Handler{manager: mgr, characters: ownedCharacterRepo(ctrl, "goblin-1", "someone-else"), roster: testRoster()}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Interact(ctx, &sessionpb.InteractRequest{Session: "sess-1", Actor: "goblin-1", Target: "demo-merchant-1"})
	requireCode(t, err, codes.PermissionDenied)
}

func TestInteract_EmptyActor_IsRefused(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice"), roster: testRoster()}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Interact(ctx, &sessionpb.InteractRequest{Session: "sess-1", Target: "demo-merchant-1"})
	requireCode(t, err, codes.InvalidArgument)
}
