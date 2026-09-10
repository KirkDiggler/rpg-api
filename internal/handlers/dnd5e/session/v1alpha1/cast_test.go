package sessionv1alpha1

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	sessionv1alpha1mock "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1/mock"
)

func TestCast_ForwardsCanonicalTargetsWithoutAliasing(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	requestTargets := []string{"goblin-1", "goblin-2"}
	mgr.EXPECT().Cast(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *sdk.CastInput) (*sdk.CastOutput, error) {
			require.Empty(t, in.Target)
			require.Equal(t, requestTargets, in.Targets)
			in.Targets[0] = "mutated-by-manager"
			return &sdk.CastOutput{}, nil
		},
	)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Cast(ctx, &sessionpb.CastRequest{
		Session: "sess-1", Member: "bard-1", DeclarationId: "decl-bane-1", Targets: requestTargets,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"goblin-1", "goblin-2"}, requestTargets,
		"the handler must not expose the request's backing array to the provider")
}

func TestCast_ForwardsDeprecatedScalarForProviderNormalization(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Cast(gomock.Any(), &sdk.CastInput{
		Session: "sess-1", Member: "bard-1", DeclarationID: "decl-bane-1", Target: "goblin-1",
	}).Return(&sdk.CastOutput{}, nil)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Cast(ctx, &sessionpb.CastRequest{
		Session: "sess-1", Member: "bard-1", DeclarationId: "decl-bane-1", Target: "goblin-1",
	})
	require.NoError(t, err)
}
