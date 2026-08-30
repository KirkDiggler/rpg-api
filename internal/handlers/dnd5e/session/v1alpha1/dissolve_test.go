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

func TestDissolve_Unauthenticated_Errors(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := &Handler{characters: anyMemberOwnedBy(ctrl, "alice"), roster: testRoster()}
	_, err := h.Dissolve(context.Background(), &sessionpb.DissolveRequest{})
	requireCode(t, err, codes.Unauthenticated)
}

func TestDissolve_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Dissolve(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *sdk.DissolveInput) (*sdk.DissolveOutput, error) {
			require.Equal(t, "sess-1", in.Session)
			require.Equal(t, "char-1", in.Member)
			require.Equal(t, sdk.DissolveByDecision, in.Cause.Kind())
			return &sdk.DissolveOutput{Members: []string{"char-1", "goblin-1"}, Cause: sdk.DissolveByDecision, Seq: 11}, nil
		},
	)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice"), roster: testRoster()}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.Dissolve(ctx, &sessionpb.DissolveRequest{
		Session: "sess-1", Member: "char-1", Cause: sessionpb.DissolveKind_DISSOLVE_KIND_BY_DECISION,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"char-1", "goblin-1"}, resp.GetMembers())
	require.Equal(t, sessionpb.DissolveKind_DISSOLVE_KIND_BY_DECISION, resp.GetCause())
}

func TestDissolve_NoCause_InvalidArgument_NeverCallsManager(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl) // no EXPECT() -- must not be called

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice"), roster: testRoster()}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Dissolve(ctx, &sessionpb.DissolveRequest{
		Session: "sess-1", Member: "char-1", Cause: sessionpb.DissolveKind_DISSOLVE_KIND_UNSPECIFIED,
	})
	requireCode(t, err, codes.InvalidArgument)
}

func TestDissolve_ManagerError_TranslatesViaErrorTable(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Dissolve(gomock.Any(), gomock.Any()).Return(nil, sdk.ErrNotInFight)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice"), roster: testRoster()}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Dissolve(ctx, &sessionpb.DissolveRequest{
		Session: "sess-1", Member: "char-1", Cause: sessionpb.DissolveKind_DISSOLVE_KIND_BY_DECISION,
	})
	requireCode(t, err, codes.FailedPrecondition)
}
