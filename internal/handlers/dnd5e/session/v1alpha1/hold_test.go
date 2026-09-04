package sessionv1alpha1

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	sessionv1alpha1mock "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1/mock"
)

func TestHold_Unauthenticated_Errors(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := &Handler{characters: anyMemberOwnedBy(ctrl, "alice")}
	_, err := h.Hold(context.Background(), &sessionpb.HoldRequest{
		Session: "sess-1", Member: "char-1", Target: "heirloom",
	})
	requireCode(t, err, codes.Unauthenticated)
}

func TestHold_EmptyMember_IsRefused(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Hold(ctx, &sessionpb.HoldRequest{Session: "sess-1", Target: "heirloom"})
	requireCode(t, err, codes.InvalidArgument)
}

func TestHold_ForeignMember_IsRefusedBeforeTheSDK(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)

	h := &Handler{manager: mgr, characters: ownedCharacterRepo(ctrl, "char-bob", "bob")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Hold(ctx, &sessionpb.HoldRequest{
		Session: "sess-1", Member: "char-bob", Target: "heirloom",
	})
	requireCode(t, err, codes.PermissionDenied)
}

// TestHold_HappyPath_RoutesVerbatimAndAcksOnly pins that all four request
// fields reach the SDK unchanged -- target is the PLACEMENT ID, never a ref,
// and range is the host's truth, forwarded rather than defaulted here.
func TestHold_HappyPath_RoutesVerbatimAndAcksOnly(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Hold(gomock.Any(), &sdk.HoldInput{
		Session: "sess-1", Member: "char-alice", Target: "heirloom", Range: 1,
	}).Return(&sdk.HoldOutput{
		Saved:    sdk.SaveReport{Written: []string{"encounter"}},
		Delivery: sdk.DeliveryReport{Events: 1},
	}, nil)

	h := &Handler{
		manager: mgr,
		characters: charactersOf(ctrl, map[string]rosterCharacter{
			"char-alice": {owner: "alice", name: "Alice", class: "fighter", race: "human"},
		}),
	}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.Hold(ctx, &sessionpb.HoldRequest{
		Session: "sess-1", Member: "char-alice", Target: "heirloom", Range: 1,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"encounter"}, resp.GetSaved().GetWritten())
	require.Equal(t, int32(1), resp.GetDelivery().GetEvents())
}

// TestHold_TheProbeLawSurvivesTranslation is the acceptance row this file
// exists for (design §4.3): for a prop the member cannot see, EVERY refusal
// -- no such prop, not holdable, already held, out of range -- must reach the
// client as the SAME STATUS AND THE SAME MESSAGE, or a guessed id becomes a
// way to map a room nobody has found. "Out of range" about an unseen id
// answers "yes, there is something by that name in a room you have not found"
// as loudly as "not holdable" would.
//
// The composition does the collapsing: all four arrive here as one wrapped
// sdk.ErrNoProp, exactly as [session.Manager.Hold] wraps it. What THIS test
// can still catch is the way rpg-api would break the law -- by making the
// message a function of the request. A handler that answered
// `status.Errorf(codes.NotFound, "no prop %q", req.GetTarget())` would pass
// every other test in this file and fail this one, because the four probes
// below deliberately name four different prop ids.
func TestHold_TheProbeLawSurvivesTranslation(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)

	// Four ids a client might guess at, standing for the four refusals the
	// composition would have made about a prop inside space this member has
	// not been shown. Each returns the sentinel the SDK actually returns for
	// all four, wrapped the way the SDK wraps it.
	probes := []string{"relic", "urn", "heirloom", "chalice"}
	mgr.EXPECT().Hold(gomock.Any(), gomock.Any()).
		Return(nil, fmt.Errorf("hold: %w", sdk.ErrNoProp)).Times(len(probes))

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")

	answers := make([]string, len(probes))
	for i, target := range probes {
		_, err := h.Hold(ctx, &sessionpb.HoldRequest{
			Session: "sess-1", Member: "char-1", Target: target,
		})
		st, ok := status.FromError(err)
		require.True(t, ok, "refusal %q must be a gRPC status", target)
		require.Equal(t, codes.NotFound, st.Code(), "probe %q", target)
		answers[i] = st.Code().String() + "/" + st.Message()
	}

	for i := 1; i < len(answers); i++ {
		require.Equal(t, answers[0], answers[i],
			"every refusal about a prop the member cannot see is the same bytes; probe %q differed",
			probes[i])
	}
	require.NotContains(t, answers[0], probes[0],
		"the refusal must not echo the guessed id back -- that alone would confirm it")
}

// TestHold_AVisibleRefusalIsNamed is the other half of the same law, and the
// reason the test above is not simply "everything is NotFound": a prop the
// member CAN see refuses by name, because there is no secret in a pillar.
func TestHold_AVisibleRefusalIsNamed(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"not holdable", sdk.ErrNotHoldable},
		{"already held", sdk.ErrAlreadyHeld},
		{"out of range", sdk.ErrOutOfRange},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mgr := sessionv1alpha1mock.NewMockManager(ctrl)
			mgr.EXPECT().Hold(gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("hold: %w", tc.err))

			h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
			ctx := auth.WithPlayerID(context.Background(), "alice")
			_, err := h.Hold(ctx, &sessionpb.HoldRequest{
				Session: "sess-1", Member: "char-1", Target: "pillar",
			})
			st, ok := status.FromError(err)
			require.True(t, ok)
			require.Equal(t, codes.FailedPrecondition, st.Code())
			require.Contains(t, st.Message(), tc.err.Error(),
				"a visible prop's refusal says which refusal it was")
		})
	}
}

// TestHold_ResponseCarriesNoOutcome: HoldResponse is the two ack fields.
// What the world needs to know -- the prop off the map, who has it -- is the
// HELD beat's job, and a field here that said it again would be a second
// answer free to disagree with the first.
func TestHold_ResponseCarriesNoOutcome(t *testing.T) {
	typ := reflect.TypeOf(sessionpb.HoldResponse{})
	var exported []string
	for i := 0; i < typ.NumField(); i++ {
		if f := typ.Field(i); f.IsExported() {
			exported = append(exported, f.Name)
		}
	}
	require.ElementsMatch(t, []string{"Saved", "Delivery"}, exported)
}
