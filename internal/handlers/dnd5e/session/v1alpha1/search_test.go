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

func TestSearch_Unauthenticated_Errors(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := &Handler{characters: anyMemberOwnedBy(ctrl, "alice")}
	_, err := h.Search(context.Background(), &sessionpb.SearchRequest{Session: "sess-1", Member: "char-1", Region: "hall"})
	requireCode(t, err, codes.Unauthenticated)
}

func TestSearch_EmptyMember_IsRefused(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Search(ctx, &sessionpb.SearchRequest{Session: "sess-1", Region: "hall"})
	requireCode(t, err, codes.InvalidArgument)
}

// TestSearch_ForeignMember_IsRefusedBeforeTheSDK mirrors GetAtlas's and
// GetWhere's own sharpest case: Search is a member-taking verb gated by
// callerActingAs like every other one, so a client cannot spend another
// player's search by naming their character. The manager mock expects
// nothing: the point is the call never reaches the SDK at all.
func TestSearch_ForeignMember_IsRefusedBeforeTheSDK(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)

	h := &Handler{manager: mgr, characters: ownedCharacterRepo(ctrl, "char-bob", "bob")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Search(ctx, &sessionpb.SearchRequest{Session: "sess-1", Member: "char-bob", Region: "hall"})
	requireCode(t, err, codes.PermissionDenied)
}

// TestSearch_HappyPath_ReturnsAckOnly pins the routing (session/member/region
// reach the SDK verbatim) and the ack-only translation (out.Saved/out.Delivery
// copied through, nothing else read from the output -- there IS nothing else
// on SearchOutput to read).
func TestSearch_HappyPath_ReturnsAckOnly(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Search(gomock.Any(), &sdk.SearchInput{Session: "sess-1", Member: "char-alice", Region: "hall"}).Return(
		&sdk.SearchOutput{
			Saved:    sdk.SaveReport{Written: []string{"encounter"}},
			Delivery: sdk.DeliveryReport{Events: 1},
		}, nil,
	)

	h := &Handler{
		manager: mgr,
		characters: charactersOf(ctrl, map[string]rosterCharacter{
			"char-alice": {owner: "alice", name: "Alice", class: "fighter", race: "human"},
		}),
	}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.Search(ctx, &sessionpb.SearchRequest{Session: "sess-1", Member: "char-alice", Region: "hall"})
	require.NoError(t, err)
	require.Equal(t, []string{"encounter"}, resp.GetSaved().GetWritten())
	require.Equal(t, int32(1), resp.GetDelivery().GetEvents())
}

// TestSearch_EmptySearch_LooksIdenticalToAFind pins the secrecy law at the
// response level (design.md: "a room with nothing hidden resolves the same
// way as a failed check -- the answer never leaks the question"): the SAME
// handler code path, given a SearchOutput that persisted and delivered
// nothing, returns a response that differs from the happy path ONLY in the
// two fields the SDK itself reported differently -- there is no third field
// a handler could branch on to say "and by the way, nothing was there."
func TestSearch_EmptySearch_LooksIdenticalToAFind(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Search(gomock.Any(), gomock.Any()).Return(&sdk.SearchOutput{}, nil)

	h := &Handler{
		manager: mgr,
		characters: charactersOf(ctrl, map[string]rosterCharacter{
			"char-alice": {owner: "alice", name: "Alice", class: "fighter", race: "human"},
		}),
	}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.Search(ctx, &sessionpb.SearchRequest{Session: "sess-1", Member: "char-alice", Region: "hall"})
	require.NoError(t, err, "an empty region and a failed check both resolve as an ordinary ack, never an error")
	require.Empty(t, resp.GetSaved().GetWritten())
	require.Zero(t, resp.GetDelivery().GetEvents())
}

// TestSearch_ManagerError_TranslatesViaErrorTable exercises Search's own
// refusal, sdk.ErrElsewhere -- the SDK returns it identically whether the
// named region is real-but-elsewhere or does not exist (the probe law), and
// it is FAILED_PRECONDITION here, not NotFound (see errors.go).
func TestSearch_ManagerError_TranslatesViaErrorTable(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Search(gomock.Any(), gomock.Any()).Return(nil, sdk.ErrElsewhere)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Search(ctx, &sessionpb.SearchRequest{Session: "sess-1", Member: "char-1", Region: "hall"})
	requireCode(t, err, codes.FailedPrecondition)
}

// TestSearch_ResponseCarriesNoOutcome makes the secrecy law mechanical at
// the wire type itself, not just at today's handler code: SearchResponse
// has exactly two fields, the ack (saved, delivery). There is no field for
// any future handler change to populate with "was there anything to find
// here" even by accident -- an Outcome/Found/Result field appearing on this
// message should fail this test and force the deliberate design
// conversation the law requires, not slip in as a one-line addition.
func TestSearch_ResponseCarriesNoOutcome(t *testing.T) {
	typ := reflect.TypeOf(sessionpb.SearchResponse{})
	var exported []string
	for i := 0; i < typ.NumField(); i++ {
		if f := typ.Field(i); f.IsExported() {
			exported = append(exported, f.Name)
		}
	}
	require.ElementsMatch(t, []string{"Saved", "Delivery"}, exported)
}
