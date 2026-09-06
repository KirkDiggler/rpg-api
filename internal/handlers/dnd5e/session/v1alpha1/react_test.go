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

func TestReact_Unauthenticated_Errors(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := &Handler{characters: anyMemberOwnedBy(ctrl, "alice")}
	_, err := h.React(context.Background(), &sessionpb.ReactRequest{})
	requireCode(t, err, codes.Unauthenticated)
}

// Both answers reach the SDK as themselves. HOLD is the one that would be
// silently produced by a defaulted converter, so it is tested as loudly as
// STRIKE: holding is a choice a player made, not the absence of one.
func TestReact_BothChoicesTravelVerbatim(t *testing.T) {
	cases := []struct {
		name string
		in   sessionpb.ReactChoice
		want sdk.ReactChoice
	}{
		{"strike", sessionpb.ReactChoice_REACT_CHOICE_STRIKE, sdk.ReactStrike},
		{"hold", sessionpb.ReactChoice_REACT_CHOICE_HOLD, sdk.ReactHold},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mgr := sessionv1alpha1mock.NewMockManager(ctrl)
			mgr.EXPECT().React(gomock.Any(), &sdk.ReactInput{
				Session: "sess-1", Member: "char-1", DeclarationID: "decl-react-1", Choice: tc.want,
			}).Return(&sdk.ReactOutput{
				Saved: sdk.SaveReport{Written: []string{"session:sess-1"}},
			}, nil)

			h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
			ctx := auth.WithPlayerID(context.Background(), "alice")
			resp, err := h.React(ctx, &sessionpb.ReactRequest{
				Session: "sess-1", Member: "char-1", DeclarationId: "decl-react-1", Choice: tc.in,
			})

			require.NoError(t, err)
			require.Equal(t, []string{"session:sess-1"}, resp.GetSaved().GetWritten())
		})
	}
}

// AN UNSET CHOICE IS INVALID_ARGUMENT AND NEVER REACHES THE SDK, and the
// manager mock with no expectations is the real assertion: gomock fails on any
// call, so a handler that defaulted the empty enum to hold -- declining a
// swing the player never declined -- fails here rather than in a fight.
func TestReact_AnUnsetChoiceIsRefusedBeforeTheSDK(t *testing.T) {
	for _, choice := range []sessionpb.ReactChoice{
		sessionpb.ReactChoice_REACT_CHOICE_UNSPECIFIED,
		sessionpb.ReactChoice(99), // a value this build never posed
	} {
		t.Run(choice.String(), func(t *testing.T) {
			ctrl := gomock.NewController(t)
			h := &Handler{
				manager:    sessionv1alpha1mock.NewMockManager(ctrl),
				characters: anyMemberOwnedBy(ctrl, "alice"),
			}
			ctx := auth.WithPlayerID(context.Background(), "alice")
			resp, err := h.React(ctx, &sessionpb.ReactRequest{
				Session: "sess-1", Member: "char-1", DeclarationId: "decl-react-1", Choice: choice,
			})

			require.Nil(t, resp)
			requireCode(t, err, codes.InvalidArgument)
		})
	}
}

// ReactResponse is thin -- saved and delivery, no success flag -- so an empty
// success and an empty refusal would be the same bytes if a refusal ever came
// back as a response. This table is what makes the thin ack safe, and it is
// where the three window sentinels prove they land in different buckets: a
// stale window, somebody else's window, and the freeze itself must not all
// read alike to a client.
func TestReact_EveryRefusalIsAStatusCode(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want codes.Code
	}{
		// The question is no longer being asked -- answered already, or the
		// fight moved on. A second click on a REACT button.
		{"window is gone", sdk.ErrNoWindow, codes.FailedPrecondition},
		// The window is real and open and belongs to somebody else. NOT
		// FailedPrecondition: this is a reach for what is not yours.
		{"somebody else's window", sdk.ErrNotAudience, codes.PermissionDenied},
		{"choice was never posed", sdk.ErrNotOffered, codes.FailedPrecondition},
		// React is the one verb that reaches a frozen session, but the
		// freeze can still refuse it -- and every OTHER verb sees this one
		// while a window is open.
		{"the freeze", sdk.ErrWindowOpen, codes.FailedPrecondition},
		{"no selector echoed", sdk.ErrNoDeclarationID, codes.InvalidArgument},
		{"no such session", sdk.ErrNoSession, codes.NotFound},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mgr := sessionv1alpha1mock.NewMockManager(ctrl)
			mgr.EXPECT().React(gomock.Any(), gomock.Any()).
				Return(nil, fmt.Errorf("react: %w", tc.err))

			h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
			ctx := auth.WithPlayerID(context.Background(), "alice")
			resp, err := h.React(ctx, &sessionpb.ReactRequest{
				Session: "sess-1", Member: "char-1", DeclarationId: "decl-react-1",
				Choice: sessionpb.ReactChoice_REACT_CHOICE_STRIKE,
			})

			require.Error(t, err)
			require.Nil(t, resp, "a refusal must not come back as a response at all")
			requireCode(t, err, tc.want)
		})
	}
}

// The freeze names WHO the table is waiting for. WindowOpenError is
// ErrWindowOpen's detail, and a host that dropped it would tell every other
// player only that they cannot move.
func TestReact_TheFreezeNamesWhoIsBeingWaitedOn(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().React(gomock.Any(), gomock.Any()).Return(nil, &sdk.WindowOpenError{
		Windows:   []string{"window-1"},
		Audiences: []string{"char-2"},
	})

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.React(ctx, &sessionpb.ReactRequest{
		Session: "sess-1", Member: "char-1", DeclarationId: "decl-react-1",
		Choice: sessionpb.ReactChoice_REACT_CHOICE_HOLD,
	})

	requireCode(t, err, codes.FailedPrecondition)
	require.Contains(t, err.Error(), "char-2")
}
