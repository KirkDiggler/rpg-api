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

func TestEnd_Unauthenticated_Errors(t *testing.T) {
	h := &Handler{}
	_, err := h.End(context.Background(), &sessionpb.EndRequest{})
	requireCode(t, err, codes.Unauthenticated)
}

func TestEnd_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().End(gomock.Any(), &sdk.EndInput{Session: "sess-1", Ending: "victory"}).Return(&sdk.EndOutput{
		Outcome: sdk.Outcome{Ending: "victory", At: 99},
	}, nil)

	h := &Handler{manager: mgr}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.End(ctx, &sessionpb.EndRequest{Session: "sess-1", Ending: "victory"})
	require.NoError(t, err)
	require.Equal(t, "victory", resp.GetOutcome().GetEnding())
}

func TestEnd_ManagerError_TranslatesViaErrorTable(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().End(gomock.Any(), gomock.Any()).Return(nil, sdk.ErrNoEnding)

	h := &Handler{manager: mgr}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.End(ctx, &sessionpb.EndRequest{Session: "sess-1", Ending: "bogus"})
	requireCode(t, err, codes.NotFound)
}
