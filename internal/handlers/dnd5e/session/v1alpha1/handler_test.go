package sessionv1alpha1

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	sessionv1alpha1mock "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1/mock"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	rosterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/roster"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

func TestNew_MissingManager_Errors(t *testing.T) {
	ctrl := gomock.NewController(t)
	_, err := New(&HandlerConfig{
		Broker:     sessionorch.NewBroker(),
		Characters: charactermock.NewMockRepository(ctrl),
		Roster:     rosterrepo.NewInMemory(),
	})
	require.Error(t, err)
}

func TestNew_MissingBroker_Errors(t *testing.T) {
	ctrl := gomock.NewController(t)
	_, err := New(&HandlerConfig{
		Manager:    sessionv1alpha1mock.NewMockManager(ctrl),
		Characters: charactermock.NewMockRepository(ctrl),
		Roster:     rosterrepo.NewInMemory(),
	})
	require.Error(t, err)
}

func TestNew_MissingCharacters_Errors(t *testing.T) {
	ctrl := gomock.NewController(t)
	_, err := New(&HandlerConfig{
		Manager: sessionv1alpha1mock.NewMockManager(ctrl),
		Broker:  sessionorch.NewBroker(),
		Roster:  rosterrepo.NewInMemory(),
	})
	require.Error(t, err)
}

func TestNew_MissingRoster_Errors(t *testing.T) {
	ctrl := gomock.NewController(t)
	_, err := New(&HandlerConfig{
		Manager:    sessionv1alpha1mock.NewMockManager(ctrl),
		Broker:     sessionorch.NewBroker(),
		Characters: charactermock.NewMockRepository(ctrl),
	})
	require.Error(t, err)
}

func TestNew_NilConfig_Errors(t *testing.T) {
	_, err := New(nil)
	require.Error(t, err)
}

func TestNew_EverythingSupplied_Succeeds(t *testing.T) {
	ctrl := gomock.NewController(t)
	h, err := New(&HandlerConfig{
		Manager:    sessionv1alpha1mock.NewMockManager(ctrl),
		Broker:     sessionorch.NewBroker(),
		Characters: charactermock.NewMockRepository(ctrl),
		Roster:     rosterrepo.NewInMemory(),
	})
	require.NoError(t, err)
	require.NotNil(t, h)
}

func TestAuthenticatedPlayerID_Present(t *testing.T) {
	ctx := auth.WithPlayerID(context.Background(), "alice")
	got, err := authenticatedPlayerID(ctx)
	require.NoError(t, err)
	require.Equal(t, "alice", got)
}

func TestAuthenticatedPlayerID_Absent_ReturnsUnauthenticated(t *testing.T) {
	_, err := authenticatedPlayerID(context.Background())
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.Unauthenticated, st.Code())
}

func TestVerifyMemberOwnership_CallerOwnsMember_Succeeds(t *testing.T) {
	ctrl := gomock.NewController(t)
	charRepo := charactermock.NewMockRepository(ctrl)
	charRepo.EXPECT().Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).Return(
		&characterrepo.GetOutput{Character: &entities.Character{Data: &tkcharacter.Data{ID: "char-1", PlayerID: "alice"}}}, nil,
	)
	h := &Handler{characters: charRepo}

	err := h.verifyMemberOwnership(context.Background(), "alice", "char-1")
	require.NoError(t, err)
}

func TestVerifyMemberOwnership_CallerDoesNotOwnMember_ReturnsPermissionDenied(t *testing.T) {
	ctrl := gomock.NewController(t)
	charRepo := charactermock.NewMockRepository(ctrl)
	charRepo.EXPECT().Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).Return(
		&characterrepo.GetOutput{Character: &entities.Character{Data: &tkcharacter.Data{ID: "char-1", PlayerID: "bob"}}}, nil,
	)
	h := &Handler{characters: charRepo}

	err := h.verifyMemberOwnership(context.Background(), "alice", "char-1")
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.PermissionDenied, st.Code())
}

func TestVerifyMemberOwnership_MemberNotFound_ReturnsNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	charRepo := charactermock.NewMockRepository(ctrl)
	charRepo.EXPECT().Get(gomock.Any(), characterrepo.GetInput{ID: "missing"}).Return(
		nil, status.Error(codes.NotFound, "not found"),
	)
	h := &Handler{characters: charRepo}

	err := h.verifyMemberOwnership(context.Background(), "alice", "missing")
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.NotFound, st.Code())
}
