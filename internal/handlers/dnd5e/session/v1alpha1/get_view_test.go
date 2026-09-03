package sessionv1alpha1

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	sessionv1alpha1mock "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1/mock"
)

func TestGetView_Unauthenticated_Errors(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := &Handler{characters: anyMemberOwnedBy(ctrl, "alice")}
	_, err := h.GetView(context.Background(), &sessionpb.GetViewRequest{})
	requireCode(t, err, codes.Unauthenticated)
}

func TestGetView_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().View(gomock.Any(), &sdk.ViewInput{Session: "sess-1", Member: "char-1"}).Return(
		[]sdk.Sighting{{Subject: "goblin-1"}}, nil,
	)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.GetView(ctx, &sessionpb.GetViewRequest{Session: "sess-1", Member: "char-1"})
	require.NoError(t, err)
	require.Len(t, resp.GetSightings(), 1)
}

func TestGetView_ManagerError_TranslatesViaErrorTable(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().View(gomock.Any(), gomock.Any()).Return(nil, sdk.ErrNoMember)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.GetView(ctx, &sessionpb.GetViewRequest{Session: "sess-1", Member: "bogus"})
	requireCode(t, err, codes.NotFound)
}
