package sessionv1alpha1

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

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

// A CELL cast carries the cell the caster pointed at, and the handler is the
// only place the wire's Position becomes the SDK's.
//
// The handler does not ask what the cell MEANS. Which shape is aimed, whether
// the caster's own cell is legal, whether a cell was required at all -- every
// one of those is a rule, and session refuses on all of them. This copies a
// reference through, exactly as it already does for a path on Move.
func TestCast_ForwardsTheAimedCell(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Cast(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *sdk.CastInput) (*sdk.CastOutput, error) {
			require.NotNil(t, in.Cell, "a cast that named a cell must reach the SDK carrying it")
			require.Equal(t, spatial.Position{X: 4, Y: 2}, *in.Cell)
			return &sdk.CastOutput{}, nil
		},
	)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Cast(ctx, &sessionpb.CastRequest{
		Session: "sess-1", Member: "bard-1", DeclarationId: "decl-thunderwave-1",
		Cell: &sessionpb.Position{X: 4, Y: 2},
	})
	require.NoError(t, err)
}

// An unset cell stays nil rather than becoming the origin.
//
// (0,0) is a real cell on this grid, so a zero Position cannot stand in for
// "no cell was named" -- the two would be indistinguishable and session's
// refusal for a CELL cast with no cell could never fire. The pointer is what
// makes the absence sayable.
func TestCast_LeavesTheCellNilWhenTheRequestNamesNone(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Cast(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *sdk.CastInput) (*sdk.CastOutput, error) {
			require.Nil(t, in.Cell, "an unset cell is absent, not the origin")
			return &sdk.CastOutput{}, nil
		},
	)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Cast(ctx, &sessionpb.CastRequest{
		Session: "sess-1", Member: "bard-1", DeclarationId: "decl-bane-1", Targets: []string{"goblin-1"},
	})
	require.NoError(t, err)
}

// A cast that answered a menu carries the chosen word to the SDK
// (rpg-project#442, Command).
//
// The handler copies an OPAQUE ID and asks nothing about it. Whether this
// declaration offered a menu at all, whether the id is on it, and what the
// word then does to a creature are rules; session refuses on the first two and
// resolution owns the third. This is the aimed cell's law one field over.
func TestCast_ForwardsTheChosenWord(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Cast(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *sdk.CastInput) (*sdk.CastOutput, error) {
			require.Equal(t, "grovel", in.Option,
				"the word the caster picked must reach the SDK that judges it")
			return &sdk.CastOutput{}, nil
		},
	)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Cast(ctx, &sessionpb.CastRequest{
		Session: "sess-1", Member: "bard-1", DeclarationId: "decl-command-1",
		Targets: []string{"skel-1"}, Option: "grovel",
	})
	require.NoError(t, err)
}
