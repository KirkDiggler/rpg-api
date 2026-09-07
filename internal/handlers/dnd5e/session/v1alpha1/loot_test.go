package sessionv1alpha1

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	sessionv1alpha1mock "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1/mock"
)

func TestLoot_Unauthenticated_Errors(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := &Handler{characters: anyMemberOwnedBy(ctrl, "alice")}
	_, err := h.Loot(context.Background(), &sessionpb.LootRequest{
		Session: "sess-1", Member: "char-1", Target: "captain-1",
	})
	requireCode(t, err, codes.Unauthenticated)
}

func TestLoot_EmptyMember_IsRefused(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Loot(ctx, &sessionpb.LootRequest{Session: "sess-1", Target: "captain-1"})
	requireCode(t, err, codes.InvalidArgument)
}

// TestLoot_ForeignMember_IsRefusedBeforeTheSDK: Loot is a member-taking verb
// gated by callerActingAs like every other one, so a client cannot loot as
// another player's character. The manager mock expects nothing -- the point
// is the call never reaches the SDK.
func TestLoot_ForeignMember_IsRefusedBeforeTheSDK(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)

	h := &Handler{manager: mgr, characters: ownedCharacterRepo(ctrl, "char-bob", "bob")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Loot(ctx, &sessionpb.LootRequest{
		Session: "sess-1", Member: "char-bob", Target: "captain-1",
	})
	requireCode(t, err, codes.PermissionDenied)
}

// TestLoot_HappyPath_RoutesVerbatimAndAcksOnly pins both halves of this
// handler's whole job: all four request fields reach the SDK unchanged
// (range included, which is the host's truth and is forwarded, never
// defaulted here), and the response is the two reports and nothing else.
func TestLoot_HappyPath_RoutesVerbatimAndAcksOnly(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Loot(gomock.Any(), &sdk.LootInput{
		Session: "sess-1", Member: "char-alice", Target: "skeleton-captain-1", Range: 2,
	}).Return(&sdk.LootOutput{
		Saved:    sdk.SaveReport{Written: []string{"encounter"}},
		Delivery: sdk.DeliveryReport{Events: 2},
	}, nil)

	h := &Handler{
		manager: mgr,
		characters: charactersOf(ctrl, map[string]rosterCharacter{
			"char-alice": {owner: "alice", name: "Alice", class: "fighter", race: "human"},
		}),
	}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.Loot(ctx, &sessionpb.LootRequest{
		Session: "sess-1", Member: "char-alice", Target: "skeleton-captain-1", Range: 2,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"encounter"}, resp.GetSaved().GetWritten())
	require.Equal(t, int32(2), resp.GetDelivery().GetEvents())
}

// TestLoot_AnEmptyBodyAnswersLikeTheCaptain is design P3 at the response
// level: the affordance must not say which body carries intel. Given two
// SDK outputs that are byte-identical, the handler produces byte-identical
// responses -- there is no third field it could branch on to say "and this
// one had something." The SDK reporting the same thing for both is the
// toolkit's half; this is rpg-api's.
func TestLoot_AnEmptyBodyAnswersLikeTheCaptain(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	ack := &sdk.LootOutput{
		Saved:    sdk.SaveReport{Written: []string{"encounter"}},
		Delivery: sdk.DeliveryReport{Events: 1},
	}
	mgr.EXPECT().Loot(gomock.Any(), gomock.Any()).Return(ack, nil).Times(2)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")

	captain, err := h.Loot(ctx, &sessionpb.LootRequest{
		Session: "sess-1", Member: "char-1", Target: "skeleton-captain-1",
	})
	require.NoError(t, err)
	empty, err := h.Loot(ctx, &sessionpb.LootRequest{
		Session: "sess-1", Member: "char-1", Target: "skeleton-2",
	})
	require.NoError(t, err)
	require.Equal(t, captain.String(), empty.String(),
		"a body with the run's only secret and a body with nothing answer with the same bytes")
}

// TestLoot_NotDown_IsFailedPrecondition exercises Loot's own ordinary
// refusal: the target is a real member who is standing up. Ordinary rather
// than probe-scoped on purpose -- a body is visible, and there is no secret
// in whether somebody is on the floor.
func TestLoot_NotDown_IsFailedPrecondition(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Loot(gomock.Any(), gomock.Any()).Return(nil, sdk.ErrNotDown)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Loot(ctx, &sessionpb.LootRequest{
		Session: "sess-1", Member: "char-1", Target: "skeleton-2",
	})
	requireCode(t, err, codes.FailedPrecondition)
}

// TestLoot_ResponseCarriesNoOutcome makes design P3 mechanical at the wire
// type itself rather than only at today's handler code: LootResponse has
// exactly the two ack fields. A Found/Transferred/Count field appearing on
// this message should fail this test and force the deliberate design
// conversation, not slip in as a one-line addition. SearchResponse's own
// pin, one verb over, for the same law.
func TestLoot_ResponseCarriesNoOutcome(t *testing.T) {
	typ := reflect.TypeOf(sessionpb.LootResponse{})
	var exported []string
	for i := 0; i < typ.NumField(); i++ {
		if f := typ.Field(i); f.IsExported() {
			exported = append(exported, f.Name)
		}
	}
	require.ElementsMatch(t, []string{"Saved", "Delivery"}, exported)
}
