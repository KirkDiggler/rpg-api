package sessionv1alpha1

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	sessionv1alpha1mock "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1/mock"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// TestSetSneaking_ACallerCannotSetAnotherMembersMode: naming a character the
// caller does not own is refused before the SDK is reached (no SetSneaking
// expectation, so a forwarded call fails the controller).
func TestSetSneaking_ACallerCannotSetAnotherMembersMode(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	h := &Handler{manager: mgr, characters: ownedBy(ctrl, map[string]string{"char-a": "alice"})}
	ctx := auth.WithPlayerID(context.Background(), "mallory")

	out, err := h.SetSneaking(ctx, &sessionpb.SetSneakingRequest{Session: "sess-1", Member: "char-a", Sneaking: proto.Bool(true)})
	require.Error(t, err)
	require.Nil(t, out)
}

// TestSetSneaking_ACallerNotSeatedAsTheMemberIsRefused: owning the character
// is not enough; the caller must be seated in this session as that member.
func TestSetSneaking_ACallerNotSeatedAsTheMemberIsRefused(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Roster(gomock.Any(), gomock.Any()).Return(nil, sdk.ErrNotSeated)
	h := &Handler{manager: mgr, characters: ownedBy(ctrl, map[string]string{"char-a": "alice"})}
	ctx := auth.WithPlayerID(context.Background(), "alice")

	_, err := h.SetSneaking(ctx, &sessionpb.SetSneakingRequest{Session: "sess-1", Member: "char-a", Sneaking: proto.Bool(true)})
	require.Error(t, err)
}

// TestSetSneaking_AnOmittedModeIsRefused: the request's optional field
// distinguishes "stop" (false) from "said nothing"; nothing is defaulted.
func TestSetSneaking_AnOmittedModeIsRefused(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	seatedAs(mgr, "alice", "char-a")
	h := &Handler{manager: mgr, characters: ownedBy(ctrl, map[string]string{"char-a": "alice"})}
	ctx := auth.WithPlayerID(context.Background(), "alice")

	_, err := h.SetSneaking(ctx, &sessionpb.SetSneakingRequest{Session: "sess-1", Member: "char-a"})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

// TestSetSneaking_ForwardsTheCallersOwnModeAndAnswersTheReports: the seated
// owner's declaration reaches the SDK as given, and the SDK's mode and reports
// come back converted, nothing added.
func TestSetSneaking_ForwardsTheCallersOwnModeAndAnswersTheReports(t *testing.T) {
	for _, sneaking := range []bool{true, false} {
		ctrl := gomock.NewController(t)
		mgr := sessionv1alpha1mock.NewMockManager(ctrl)
		seatedAs(mgr, "alice", "char-a")
		mgr.EXPECT().SetSneaking(gomock.Any(), &sdk.SetSneakingInput{Session: "sess-1", Member: "char-a", Sneaking: sneaking}).
			Return(&sdk.SetSneakingOutput{
				Sneaking: sneaking,
				Saved:    sdk.SaveReport{Written: []string{"encounter"}, Failed: []string{"exploration"}},
				Delivery: sdk.DeliveryReport{Events: 2, Failed: true},
			}, nil)
		h := &Handler{manager: mgr, characters: ownedBy(ctrl, map[string]string{"char-a": "alice"})}
		ctx := auth.WithPlayerID(context.Background(), "alice")

		resp, err := h.SetSneaking(ctx, &sessionpb.SetSneakingRequest{Session: "sess-1", Member: "char-a", Sneaking: proto.Bool(sneaking)})
		require.NoError(t, err)
		require.Equal(t, sneaking, resp.GetSneaking())
		require.Equal(t, []string{"encounter"}, resp.GetSaved().GetWritten())
		require.Equal(t, []string{"exploration"}, resp.GetSaved().GetFailed())
		require.Equal(t, int32(2), resp.GetDelivery().GetEvents())
		require.True(t, resp.GetDelivery().GetFailed())
	}
}

// TestSetSneaking_AnSDKRefusalIsAStatus: a refusal from the composition is
// mapped, not swallowed into an empty response.
func TestSetSneaking_AnSDKRefusalIsAStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	seatedAs(mgr, "alice", "char-a")
	mgr.EXPECT().SetSneaking(gomock.Any(), gomock.Any()).Return(nil, sdk.ErrNotSeated)
	h := &Handler{manager: mgr, characters: ownedBy(ctrl, map[string]string{"char-a": "alice"})}
	ctx := auth.WithPlayerID(context.Background(), "alice")

	out, err := h.SetSneaking(ctx, &sessionpb.SetSneakingRequest{Session: "sess-1", Member: "char-a", Sneaking: proto.Bool(true)})
	require.Error(t, err)
	require.Nil(t, out)
}

func TestSnuckBodyProjectsOntoTheWire(t *testing.T) {
	for _, tc := range []struct {
		name    string
		watcher string
		beaten  bool
	}{
		{name: "a watcher the mover sees, beaten", watcher: "goblin-1", beaten: true},
		{name: "a watcher the mover cannot see stays empty, not beaten", watcher: "", beaten: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, sessionpb.EventKind_EVENT_KIND_SNUCK, eventKindToProto(sdk.EventSnuck))

			event := &sessionpb.Event{}
			require.NoError(t, setEventBody(event, sdk.SnuckBody{
				Member: "char-1", Watcher: tc.watcher, DC: 12, Total: 15, Beaten: tc.beaten,
				Calculation: d20WithKeep([]int{15}, []int{0}, nil),
			}))

			got := event.GetSnuck()
			require.NotNil(t, got, "the snuck body must ride its own oneof arm")
			require.Equal(t, "char-1", got.GetMember())
			require.Equal(t, tc.watcher, got.GetWatcher())
			require.Equal(t, int32(12), got.GetDc())
			require.Equal(t, int32(15), got.GetTotal())
			require.Equal(t, tc.beaten, got.GetBeaten(), "the rulebook's verdict is carried, never recomputed from total and dc")
			require.NotNil(t, got.GetCalculation(), "the roll's arithmetic crosses")
			require.Equal(t, []int32{15}, got.GetCalculation().GetComponents()[0].GetDice().GetFinalRolls())
		})
	}
}

func TestSnuckBodyWithoutArithmeticLeavesTheCalculationUnset(t *testing.T) {
	event := &sessionpb.Event{}
	require.NoError(t, setEventBody(event, sdk.SnuckBody{Member: "char-1", DC: 12, Total: 9}))
	require.Nil(t, event.GetSnuck().GetCalculation())
}

func TestViewCarriesTheViewersSneakingMode(t *testing.T) {
	require.True(t, viewToProto(&sdk.ViewOutput{Sneaking: true}).GetSneaking())
	require.False(t, viewToProto(&sdk.ViewOutput{}).GetSneaking())
}
