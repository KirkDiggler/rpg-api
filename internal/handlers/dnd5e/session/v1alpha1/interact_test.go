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

func TestInteract_Unauthenticated_Errors(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := &Handler{characters: anyMemberOwnedBy(ctrl, "alice")}
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
					// A row a player's sale created (rpg-toolkit#1537), to
					// prove PlayerSold survives to the wire alongside price.
					{
						Type: shared.EquipmentTypeWeapon, ID: "greatsword", Name: "Greatsword",
						Mode: npcs.StockModeLimited, Quantity: 1, PlayerSold: true,
					},
				},
			},
		}, nil)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
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
	require.Len(t, d.GetInventory(), 2)
	require.Equal(t, "longsword", d.GetInventory()[0].GetEquipmentId())
	require.Equal(t, sessionpb.VendorStockMode_VENDOR_STOCK_MODE_LIMITED, d.GetInventory()[0].GetStockMode())
	require.Equal(t, int32(1), d.GetInventory()[0].GetQuantity())
	require.False(t, d.GetInventory()[0].GetPlayerSold(), "authored stock, not a player's sale")

	// The price is server-computed via equipment.PriceOf (rpg-toolkit#1534),
	// not carried on npcs.StockEntryView -- computed here from the real
	// catalog rather than hardcoded, so this pins the wiring, not a number.
	wantPrice, err := equipment.PriceOf("longsword")
	require.NoError(t, err)
	require.Equal(t, int32(wantPrice.Copper), d.GetInventory()[0].GetPrice().GetCopper())

	// PlayerSold (rpg-toolkit#1537) survives to the wire, the same
	// carried-straight-across pattern price already proved for this row.
	require.Equal(t, "greatsword", d.GetInventory()[1].GetEquipmentId())
	require.True(t, d.GetInventory()[1].GetPlayerSold())
}

func TestInteract_ManagerError_TranslatesViaErrorTable(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Interact(gomock.Any(), gomock.Any()).Return(nil, sdk.ErrOutOfRange)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
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

	h := &Handler{manager: mgr, characters: ownedCharacterRepo(ctrl, "goblin-1", "someone-else")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Interact(ctx, &sessionpb.InteractRequest{Session: "sess-1", Actor: "goblin-1", Target: "demo-merchant-1"})
	requireCode(t, err, codes.PermissionDenied)
}

func TestInteract_EmptyActor_IsRefused(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Interact(ctx, &sessionpb.InteractRequest{Session: "sess-1", Target: "demo-merchant-1"})
	requireCode(t, err, codes.InvalidArgument)
}

// TestInteract_NamesExactlyOneThing pins rpg-project#959: a request naming
// both a world NPC and a prop, or neither, is refused INVALID_ARGUMENT with a
// sentence before the SDK is asked.
func TestInteract_NamesExactlyOneThing(t *testing.T) {
	for name, req := range map[string]*sessionpb.InteractRequest{
		"both":    {Session: "sess-1", Actor: "char-1", Target: "pedlar", Prop: "grate", OptionId: "to-chapel"},
		"neither": {Session: "sess-1", Actor: "char-1", OptionId: "to-chapel"},
	} {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			h := &Handler{manager: sessionv1alpha1mock.NewMockManager(ctrl), characters: anyMemberOwnedBy(ctrl, "alice")}
			_, err := h.Interact(auth.WithPlayerID(context.Background(), "alice"), req)
			requireCode(t, err, codes.InvalidArgument)
		})
	}
}

// TestInteract_APropDeedCarriesWhatWasDone pins rpg-project#959: the prop
// branch passes the prop, option and revision through, and answers with what
// was done -- no descriptor, the approach only when a check was made, the
// roll only while the check waits.
func TestInteract_APropDeedCarriesWhatWasDone(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	roll := 14
	mgr.EXPECT().Interact(gomock.Any(), &sdk.InteractInput{
		Session: "sess-1", Actor: "char-1", Prop: "altar", OptionID: "examine", Revision: 4,
	}).Return(&sdk.InteractOutput{Done: &sdk.DoneOutput{
		Prop: "altar", OptionID: "examine", Beaten: true, Total: 15, DC: 12,
		Applied: sdk.DoorApproach{Ability: "religion", DC: 12}, Paused: true, Roll: &roll,
	}}, nil)
	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	resp, err := h.Interact(auth.WithPlayerID(context.Background(), "alice"), &sessionpb.InteractRequest{
		Session: "sess-1", Actor: "char-1", Prop: "altar", OptionId: "examine", Revision: 4,
	})
	require.NoError(t, err)
	require.Nil(t, resp.GetDescriptor_(), "a prop is not a world NPC")
	done := resp.GetDone()
	require.Equal(t, "altar", done.GetProp())
	require.Equal(t, "examine", done.GetOptionId())
	require.True(t, done.GetBeaten())
	require.Equal(t, int32(15), done.GetTotal())
	require.Equal(t, int32(12), done.GetDc())
	require.Equal(t, "religion", done.GetApplied().GetAbility())
	require.True(t, done.GetPaused())
	require.Equal(t, int32(14), done.GetRoll())

	mgr.EXPECT().Interact(gomock.Any(), gomock.Any()).Return(&sdk.InteractOutput{Done: &sdk.DoneOutput{Prop: "grate", OptionID: "to-chapel", Beaten: true}}, nil)
	resp, err = h.Interact(auth.WithPlayerID(context.Background(), "alice"), &sessionpb.InteractRequest{
		Session: "sess-1", Actor: "char-1", Prop: "grate", OptionId: "to-chapel",
	})
	require.NoError(t, err)
	require.Nil(t, resp.GetDone().GetApplied(), "no check, no approach")
	require.Nil(t, resp.GetDone().Roll, "no waiting check, no roll")
}

// TestAfford_InteractOptionsCrossOnTheSpeakRow pins rpg-project#959: the
// things to do on props cross on the declaration as the SDK wrote them --
// `do` as do_words, the shortfall as its reason only.
func TestAfford_InteractOptionsCrossOnTheSpeakRow(t *testing.T) {
	rows := interactOptionsToProto([]sdk.InteractOptionRow{
		{Prop: "grate", OptionID: "to-chapel", Do: "You lift the grate and take the way up.", Available: true},
		{Prop: "altar", OptionID: "examine", Do: "You examine the altar.", Interaction: "shrine", Revision: 3, Pending: true, PendingOn: "bob",
			Why: &sdk.Shortfall{Reason: sdk.ShortfallLineReserved, Text: "waits"}},
	})
	require.Len(t, rows, 2)
	require.Equal(t, "grate", rows[0].GetPropId())
	require.Equal(t, "to-chapel", rows[0].GetOptionId())
	require.Equal(t, "You lift the grate and take the way up.", rows[0].GetDoWords())
	require.True(t, rows[0].GetAvailable())
	require.Equal(t, sessionpb.ShortfallReason_SHORTFALL_REASON_UNSPECIFIED, rows[0].GetShortfall())
	require.Equal(t, sessionpb.ShortfallReason_SHORTFALL_REASON_LINE_RESERVED, rows[1].GetShortfall())
	require.Equal(t, "shrine", rows[1].GetInteractionId())
	require.Equal(t, uint64(3), rows[1].GetRevision())
	require.True(t, rows[1].GetPending())
	require.Equal(t, "bob", rows[1].GetPendingOnMemberId())
	require.Nil(t, interactOptionsToProto(nil))
}
