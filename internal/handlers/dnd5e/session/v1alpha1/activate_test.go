package sessionv1alpha1

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	sessionv1alpha1mock "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1/mock"
)

func TestActivate_Unauthenticated_Errors(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := &Handler{characters: anyMemberOwnedBy(ctrl, "alice")}
	_, err := h.Activate(context.Background(), &sessionpb.ActivateRequest{})
	requireCode(t, err, codes.Unauthenticated)
}

func TestActivate_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Activate(gomock.Any(), &sdk.ActivateInput{
		Session: "sess-1", Member: "char-1", DeclarationID: "decl-rage-1",
	}).Return(&sdk.ActivateOutput{
		Ability: "dnd5e:features:rage",
		Saved:   sdk.SaveReport{Written: []string{"character:char-1"}},
	}, nil)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.Activate(ctx, &sessionpb.ActivateRequest{
		Session: "sess-1", Member: "char-1", DeclarationId: "decl-rage-1",
	})

	require.NoError(t, err)
	require.Equal(t, []string{"character:char-1"}, resp.GetSaved().GetWritten())
}

// The target rides through for the one ability that takes one.
func TestActivate_PassesTheTargetThrough(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Activate(gomock.Any(), &sdk.ActivateInput{
		Session: "sess-1", Member: "char-1", DeclarationID: "decl-help-1", Target: "char-2",
	}).Return(&sdk.ActivateOutput{Ability: "dnd5e:combat_abilities:help"}, nil)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Activate(ctx, &sessionpb.ActivateRequest{
		Session: "sess-1", Member: "char-1", DeclarationId: "decl-help-1", Target: "char-2",
	})
	require.NoError(t, err)
}

// A REFUSAL IS A STATUS CODE, NOT AN EMPTY SUCCESS — and this is the test
// rpg-project#301 §9.1 said neither side had.
//
// ActivateResponse is deliberately thin: no success flag, no error string. So
// an empty success and an empty refusal would be THE SAME BYTES if this
// handler ever returned a response where the SDK gave it an error. The whole
// safety of the thin ack rests on this table.
func TestActivate_EveryRefusalIsAStatusCode(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want codes.Code
	}{
		// The ability said no. A fact about the game — the player picks
		// another verb.
		{"ability refused", sdk.ErrCannotActivate, codes.FailedPrecondition},
		// The offer moved: spent, or the world changed since Afford.
		{"selector went stale", sdk.ErrStaleDeclaration, codes.FailedPrecondition},
		{"not this member's turn", sdk.ErrNotYourTurn, codes.FailedPrecondition},
		{"member is downed", sdk.ErrDowned, codes.FailedPrecondition},
		{"member is not a character", sdk.ErrNotACharacter, codes.FailedPrecondition},
		// A caller mistake — a target named for an ability that takes none.
		// INVALID_ARGUMENT rather than Internal, because a client produced it.
		{"activation is malformed", sdk.ErrBadActivation, codes.InvalidArgument},
		{"no selector echoed", sdk.ErrNoDeclarationID, codes.InvalidArgument},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mgr := sessionv1alpha1mock.NewMockManager(ctrl)
			mgr.EXPECT().Activate(gomock.Any(), gomock.Any()).
				Return(nil, fmt.Errorf("activate: %w", tc.err))

			h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
			ctx := auth.WithPlayerID(context.Background(), "alice")
			resp, err := h.Activate(ctx, &sessionpb.ActivateRequest{
				Session: "sess-1", Member: "char-1", DeclarationId: "decl-1",
			})

			require.Error(t, err)
			require.Nil(t, resp, "a refusal must not come back as a response at all")
			requireCode(t, err, tc.want)
		})
	}
}
