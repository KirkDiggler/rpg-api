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

func TestTurn_Unauthenticated_Errors(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := &Handler{characters: anyMemberOwnedBy(ctrl, "alice"), roster: testRoster()}
	_, err := h.Turn(context.Background(), &sessionpb.TurnRequest{})
	requireCode(t, err, codes.Unauthenticated)
}

func TestTurn_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Turn(gomock.Any(), &sdk.TurnInput{Session: "sess-1", Member: "char-1"}).Return(&sdk.TurnOutput{
		Clock: sdk.ClockTurn, Active: "char-1", Round: 2, Order: []string{"char-1", "goblin-1"},
		Participants: []sdk.Participant{
			{Member: "char-1", Name: "Aldric", Kind: sdk.KindPlayer, Standing: sdk.StandingUp, Active: true},
			{Member: "goblin-1", Name: "goblin-1", Kind: sdk.KindMonster, Standing: sdk.StandingUp},
		},
	}, nil)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice"), roster: testRoster()}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.Turn(ctx, &sessionpb.TurnRequest{Session: "sess-1", Member: "char-1"})
	require.NoError(t, err)
	require.Equal(t, sessionpb.ClockKind_CLOCK_KIND_TURN, resp.GetClock())
	require.Equal(t, "char-1", resp.GetActive())
	require.Equal(t, int32(2), resp.GetRound())

	// Participants carries what a bare id in Order cannot -- name, kind,
	// standing, active -- so a client marks the active row without a lookup.
	require.Len(t, resp.GetParticipants(), 2)
	require.Equal(t, "Aldric", resp.GetParticipants()[0].GetName())
	require.True(t, resp.GetParticipants()[0].GetActive())
	require.Equal(t, sessionpb.Standing_STANDING_UP, resp.GetParticipants()[1].GetStanding())
}

func TestTurn_ManagerError_TranslatesViaErrorTable(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Turn(gomock.Any(), gomock.Any()).Return(nil, sdk.ErrNoMember)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice"), roster: testRoster()}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Turn(ctx, &sessionpb.TurnRequest{Session: "sess-1", Member: "char-1"})
	requireCode(t, err, codes.NotFound)
}
