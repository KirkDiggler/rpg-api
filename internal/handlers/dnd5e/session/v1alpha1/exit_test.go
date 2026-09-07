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

func TestExit_Unauthenticated_Errors(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := &Handler{characters: anyMemberOwnedBy(ctrl, "alice")}
	_, err := h.Exit(context.Background(), &sessionpb.ExitRequest{})
	requireCode(t, err, codes.Unauthenticated)
}

func TestExit_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Exit(gomock.Any(), &sdk.ExitInput{Session: "sess-1", Member: "char-1"}).Return(&sdk.ExitOutput{
		Outcome: sdk.MemberOutcome{ID: "char-1", Position: spatial.Position{X: 1, Y: 1}},
		Seq:     3,
	}, nil)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.Exit(ctx, &sessionpb.ExitRequest{Session: "sess-1", Member: "char-1"})
	require.NoError(t, err)
	require.Equal(t, "char-1", resp.GetOutcome().GetId())
	require.Equal(t, uint64(3), resp.GetSeq())
}

func TestExit_ManagerError_TranslatesViaErrorTable(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Exit(gomock.Any(), gomock.Any()).Return(nil, sdk.ErrNoMember)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Exit(ctx, &sessionpb.ExitRequest{Session: "sess-1", Member: "char-1"})
	requireCode(t, err, codes.NotFound)
}
