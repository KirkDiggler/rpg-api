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

func TestGetSeat_Unauthenticated_Errors(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := &Handler{characters: anyMemberOwnedBy(ctrl, "alice")}
	_, err := h.GetSeat(context.Background(), &sessionpb.GetSeatRequest{Character: "char-1"})
	requireCode(t, err, codes.Unauthenticated)
}

func TestGetSeat_EmptyCharacter_InvalidArgument(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := &Handler{manager: sessionv1alpha1mock.NewMockManager(ctrl), characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.GetSeat(ctx, &sessionpb.GetSeatRequest{})
	requireCode(t, err, codes.InvalidArgument)
}

func TestGetSeat_Seated_NamesTheSession(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Seat(gomock.Any(), &sdk.SeatInput{Character: "char-1"}).
		Return(&sdk.SeatOutput{Seat: &sdk.SeatData{Character: "char-1", Session: "sess-7"}}, nil)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.GetSeat(ctx, &sessionpb.GetSeatRequest{Character: "char-1"})
	require.NoError(t, err)
	require.Equal(t, "sess-7", resp.GetSession())
}

func TestGetSeat_NoSeat_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Seat(gomock.Any(), gomock.Any()).Return(nil, sdk.ErrNoSeat)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.GetSeat(ctx, &sessionpb.GetSeatRequest{Character: "char-1"})
	requireCode(t, err, codes.NotFound)
	require.Nil(t, resp)
}
