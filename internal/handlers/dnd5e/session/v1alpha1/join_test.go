package sessionv1alpha1

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	sessionv1alpha1mock "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1/mock"
)

func TestJoin_Unauthenticated_Errors(t *testing.T) {
	h := &Handler{}
	_, err := h.Join(context.Background(), &sessionpb.JoinRequest{})
	requireCode(t, err, codes.Unauthenticated)
}

func TestJoin_HappyPath_TranslatesRequestAndResponse(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Join(gomock.Any(), &sdk.JoinInput{
		Session:  "sess-1",
		Member:   "char-1",
		Position: positionFromProto(&sessionpb.Position{X: 1, Y: 2}),
	}).Return(&sdk.JoinOutput{
		Member:    sdk.Member{ID: "char-1", Kind: sdk.KindPlayer, Position: spatial.Position{X: 1, Y: 2}},
		Character: &sdk.CharacterState{ID: "char-1", Name: "Alice"},
		Seq:       5,
		Saved:     sdk.SaveReport{Written: []string{"encounter"}},
	}, nil)

	h := &Handler{manager: mgr}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.Join(ctx, &sessionpb.JoinRequest{
		Session:  "sess-1",
		Member:   "char-1",
		Position: &sessionpb.Position{X: 1, Y: 2},
	})
	require.NoError(t, err)
	require.Equal(t, "char-1", resp.GetMember().GetId())
	require.Equal(t, "Alice", resp.GetCharacter().GetName())
	require.Equal(t, uint64(5), resp.GetSeq())
}

func TestJoin_ManagerError_TranslatesViaErrorTable(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Join(gomock.Any(), gomock.Any()).Return(nil, sdk.ErrNoSession)

	h := &Handler{manager: mgr}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Join(ctx, &sessionpb.JoinRequest{Session: "sess-1", Member: "char-1"})
	requireCode(t, err, codes.NotFound)
}

// requireCode is a small shared assertion helper for verb handler tests:
// every handler's error path goes through statusError, so the useful thing
// to assert is the resulting gRPC code, not the exact message.
func requireCode(t *testing.T, err error, want codes.Code) {
	t.Helper()
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, want, st.Code())
}
