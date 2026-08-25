package sessionv1alpha1

// doors_handlers_test.go covers the three door RPCs (rpg-project#268):
// GetDoors behind the seated gate GetRoster set, OpenDoor and Unlock behind
// callerActingAs, and the projection of the SDK's door shapes onto the wire
// — the lock rides only while it is real.

import (
	"context"
	"testing"

	"fmt"
	"github.com/stretchr/testify/require"

	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	sessionv1alpha1mock "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1/mock"
)

func TestGetDoors_Unauthenticated_Errors(t *testing.T) {
	h := &Handler{}
	_, err := h.GetDoors(context.Background(), &sessionpb.GetDoorsRequest{Session: "sess-1"})
	requireCode(t, err, codes.Unauthenticated)
}

func TestGetDoors_MissingSession_Errors(t *testing.T) {
	h := &Handler{}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.GetDoors(ctx, &sessionpb.GetDoorsRequest{})
	requireCode(t, err, codes.InvalidArgument)
}

func TestGetDoors_NotSeated_PermissionDenied(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := &Handler{
		roster: tombRoster(t),
		characters: charactersOf(ctrl, map[string][4]string{
			"char-alice": {"alice", "Alice", "fighter", "human"},
			"char-bob":   {"bob", "Bob", "rogue", "elf"},
		}),
	}
	ctx := auth.WithPlayerID(context.Background(), "mallory")
	_, err := h.GetDoors(ctx, &sessionpb.GetDoorsRequest{Session: "sess-1"})
	requireCode(t, err, codes.PermissionDenied)
}

// The projection, pinned: a locked door carries its lock down to the DC (full
// data until v1.0), an open one carries NO lock — unset is "not locked",
// never "lock with DC zero".
func TestGetDoors_ProjectsTheLiveState(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Doors(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *sdk.DoorsInput) (*sdk.DoorsOutput, error) {
			require.Equal(t, "sess-1", in.Session)
			return &sdk.DoorsOutput{Doors: []sdk.Door{
				{ID: "entrance-hall", State: "open"},
				{ID: "hall-tomb", State: "locked", Lock: &sdk.DoorLock{DC: 12, Ability: "dex"}},
			}}, nil
		},
	)

	h := &Handler{
		manager: mgr,
		roster:  tombRoster(t),
		characters: charactersOf(ctrl, map[string][4]string{
			"char-alice": {"alice", "Alice", "fighter", "human"},
			"char-bob":   {"bob", "Bob", "rogue", "elf"},
		}),
	}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.GetDoors(ctx, &sessionpb.GetDoorsRequest{Session: "sess-1"})
	require.NoError(t, err)
	require.Len(t, resp.GetDoors(), 2)

	open := resp.GetDoors()[0]
	require.Equal(t, "entrance-hall", open.GetDoor())
	require.Equal(t, sessionpb.DoorState_DOOR_STATE_OPEN, open.GetState())
	require.Nil(t, open.GetLock(), "an open door carries no lock")

	locked := resp.GetDoors()[1]
	require.Equal(t, "hall-tomb", locked.GetDoor())
	require.Equal(t, sessionpb.DoorState_DOOR_STATE_LOCKED, locked.GetState())
	require.Equal(t, int32(12), locked.GetLock().GetDc())
	require.Equal(t, "dex", locked.GetLock().GetAbility())
}

func TestOpenDoor_Unauthenticated_Errors(t *testing.T) {
	h := &Handler{}
	_, err := h.OpenDoor(context.Background(), &sessionpb.OpenDoorRequest{Session: "sess-1", Member: "char-1", Door: "gate"})
	requireCode(t, err, codes.Unauthenticated)
}

func TestOpenDoor_MissingMember_Errors_NeverCallsManager(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl) // no EXPECT()
	h := &Handler{manager: mgr}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.OpenDoor(ctx, &sessionpb.OpenDoorRequest{Session: "sess-1", Door: "gate"})
	requireCode(t, err, codes.InvalidArgument)
}

func TestOpenDoor_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().OpenDoor(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *sdk.OpenDoorInput) (*sdk.OpenDoorOutput, error) {
			require.Equal(t, "sess-1", in.Session)
			require.Equal(t, "char-1", in.Member)
			require.Equal(t, "gate", in.Door)
			return &sdk.OpenDoorOutput{Door: sdk.Door{ID: "gate", State: "open"}, Seq: 7}, nil
		},
	)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.OpenDoor(ctx, &sessionpb.OpenDoorRequest{Session: "sess-1", Member: "char-1", Door: "gate"})
	require.NoError(t, err)
	require.Equal(t, "gate", resp.GetDoor().GetDoor())
	require.Equal(t, sessionpb.DoorState_DOOR_STATE_OPEN, resp.GetDoor().GetState())
}

// A locked door's refusal is a fiction beat: FAILED_PRECONDITION, with the
// SDK's message — which names the DC — carried whole (rpg-toolkit#1135).
func TestOpenDoor_Locked_FailedPrecondition(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().OpenDoor(gomock.Any(), gomock.Any()).Return(nil, sdkLockedErr())

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.OpenDoor(ctx, &sessionpb.OpenDoorRequest{Session: "sess-1", Member: "char-1", Door: "hall-tomb"})
	requireCode(t, err, codes.FailedPrecondition)
	require.Contains(t, err.Error(), "DC 12", "the refusal names the stakes")
}

func TestUnlock_HappyPath_CarriesTheAttempt(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Unlock(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *sdk.UnlockInput) (*sdk.UnlockOutput, error) {
			require.Equal(t, "sess-1", in.Session)
			require.Equal(t, "char-1", in.Member)
			require.Equal(t, "hall-tomb", in.Door)
			return &sdk.UnlockOutput{
				Beaten: false, Total: 9, DC: 12,
				Door: sdk.Door{ID: "hall-tomb", State: "locked", Lock: &sdk.DoorLock{DC: 12, Ability: "dex"}},
			}, nil
		},
	)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.Unlock(ctx, &sessionpb.UnlockRequest{Session: "sess-1", Member: "char-1", Door: "hall-tomb"})
	require.NoError(t, err, "a failed attempt is an outcome, not an error")
	require.False(t, resp.GetBeaten())
	require.Equal(t, int32(9), resp.GetTotal(), "the roll is public, down to the number")
	require.Equal(t, int32(12), resp.GetDc())
	require.Equal(t, sessionpb.DoorState_DOOR_STATE_LOCKED, resp.GetDoor().GetState(), "unchanged and retryable")
}

func TestUnlock_MissingMember_Errors(t *testing.T) {
	h := &Handler{}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Unlock(ctx, &sessionpb.UnlockRequest{Session: "sess-1", Door: "hall-tomb"})
	requireCode(t, err, codes.InvalidArgument)
}

// sdkLockedErr builds the error shape the SDK really returns for a locked
// refusal: its sentinel, with the composition's DC-naming text.
func sdkLockedErr() error {
	return fmt.Errorf("opendoor: door %q is locked, DC 12: %w", "hall-tomb", sdk.ErrLocked)
}
