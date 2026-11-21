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

func TestWait_Unauthenticated_Errors(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := &Handler{characters: anyMemberOwnedBy(ctrl, "alice")}
	_, err := h.Wait(context.Background(), &sessionpb.WaitRequest{Session: "sess-1", Member: "char-1"})
	requireCode(t, err, codes.Unauthenticated)
}

func TestWait_EmptyMember_IsRefused(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Wait(ctx, &sessionpb.WaitRequest{Session: "sess-1"})
	requireCode(t, err, codes.InvalidArgument)
}

// TestWait_ForeignMember_IsRefusedBeforeTheSDK: a client cannot wait as another
// player's character. The manager mock expects nothing.
func TestWait_ForeignMember_IsRefusedBeforeTheSDK(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)

	h := &Handler{manager: mgr, characters: ownedCharacterRepo(ctrl, "char-bob", "bob")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Wait(ctx, &sessionpb.WaitRequest{Session: "sess-1", Member: "char-bob"})
	requireCode(t, err, codes.PermissionDenied)
}

// TestWait_HappyPath_RoutesVerbatimAndAcksOnly: both request fields reach the
// SDK unchanged and the response is the two reports and nothing else.
func TestWait_HappyPath_RoutesVerbatimAndAcksOnly(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Wait(gomock.Any(), &sdk.WaitInput{Session: "sess-1", Member: "char-alice"}).
		Return(&sdk.WaitOutput{
			Saved:    sdk.SaveReport{Written: []string{"encounter"}},
			Delivery: sdk.DeliveryReport{},
		}, nil)

	h := &Handler{
		manager: mgr,
		characters: charactersOf(ctrl, map[string]rosterCharacter{
			"char-alice": {owner: "alice", name: "Alice", class: "fighter", race: "human"},
		}),
	}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.Wait(ctx, &sessionpb.WaitRequest{Session: "sess-1", Member: "char-alice"})
	require.NoError(t, err)
	require.Equal(t, []string{"encounter"}, resp.GetSaved().GetWritten())
	require.False(t, resp.GetDelivery().GetFailed())
}

// TestWait_InAFight_IsFailedPrecondition: waiting is a free-roam act; the
// engine's refusal inside a fight surfaces as FailedPrecondition.
func TestWait_InAFight_IsFailedPrecondition(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Wait(gomock.Any(), &sdk.WaitInput{Session: "sess-1", Member: "char-alice"}).
		Return(nil, fmt.Errorf("wait: %w", sdk.ErrInBubble))

	h := &Handler{
		manager: mgr,
		characters: charactersOf(ctrl, map[string]rosterCharacter{
			"char-alice": {owner: "alice", name: "Alice", class: "fighter", race: "human"},
		}),
	}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Wait(ctx, &sessionpb.WaitRequest{Session: "sess-1", Member: "char-alice"})
	requireCode(t, err, codes.FailedPrecondition)
}
