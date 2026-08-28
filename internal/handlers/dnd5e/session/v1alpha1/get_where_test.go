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

func TestGetWhere_Unauthenticated_Errors(t *testing.T) {
	h := &Handler{}
	_, err := h.GetWhere(context.Background(), &sessionpb.GetWhereRequest{})
	requireCode(t, err, codes.Unauthenticated)
}

func TestGetWhere_HappyPath_ReturnsAbsoluteCell(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Where(gomock.Any(), &sdk.WhereInput{Session: "sess-1", Member: "char-1"}).Return(
		&sdk.WhereOutput{Position: spatial.Position{X: 7, Y: 3}}, nil,
	)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.GetWhere(ctx, &sessionpb.GetWhereRequest{Session: "sess-1", Member: "char-1"})
	require.NoError(t, err)
	// The exact cell, not merely a non-nil position: this read exists so a cold
	// client can recover where it stands, and a response that carried the wrong
	// coordinates would satisfy any weaker assertion while putting the player
	// somewhere they are not.
	require.Equal(t, float64(7), resp.GetPosition().GetX())
	require.Equal(t, float64(3), resp.GetPosition().GetY())
}

func TestGetWhere_ManagerError_TranslatesViaErrorTable(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Where(gomock.Any(), gomock.Any()).Return(nil, sdk.ErrNoMember)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.GetWhere(ctx, &sessionpb.GetWhereRequest{Session: "sess-1", Member: "char-1"})
	// NotFound, not InvalidArgument: the caller named a member the session does
	// not have, which the error table buckets with "you named something that
	// does not exist" rather than with a malformed request.
	requireCode(t, err, codes.NotFound)
}

// TestGetWhere_ForeignMember_IsRefusedBeforeTheSDK is the sharpest case for the
// entitlement gate, and the reason it is not merely an authz nicety.
//
// session.Where answers for whatever member ID it is handed -- it checks only
// that the ID is non-empty and in the encounter, because the toolkit cannot
// know who is asking. So an unchecked member here would let a client read the
// cell of anything it can name: a party member across the map, or a MONSTER
// whose ID it learned from a story beat. That is precisely the unperceived-
// roster leak the toolkit refused to build a batch positions read for
// (rpg-toolkit#1051) -- reintroduced one ID at a time.
//
// The manager mock expects NOTHING: the point is that the call never reaches
// the SDK at all.
func TestGetWhere_ForeignMember_IsRefusedBeforeTheSDK(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)

	h := &Handler{manager: mgr, characters: ownedCharacterRepo(ctrl, "goblin-1", "someone-else")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.GetWhere(ctx, &sessionpb.GetWhereRequest{Session: "sess-1", Member: "goblin-1"})
	requireCode(t, err, codes.PermissionDenied)
}

func TestGetWhere_EmptyMember_IsRefused(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.GetWhere(ctx, &sessionpb.GetWhereRequest{Session: "sess-1"})
	requireCode(t, err, codes.InvalidArgument)
}
