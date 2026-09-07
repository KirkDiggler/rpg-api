package sessionv1alpha1

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/KirkDiggler/rpg-api/internal/auth"
	sessionv1alpha1mock "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1/mock"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
)

func TestNew_MissingManager_Errors(t *testing.T) {
	ctrl := gomock.NewController(t)
	_, err := New(&HandlerConfig{
		Broker:     sessionorch.NewBroker(),
		Characters: charactermock.NewMockRepository(ctrl),
	})
	require.Error(t, err)
}

func TestNew_MissingBroker_Errors(t *testing.T) {
	ctrl := gomock.NewController(t)
	_, err := New(&HandlerConfig{
		Manager:    sessionv1alpha1mock.NewMockManager(ctrl),
		Characters: charactermock.NewMockRepository(ctrl),
	})
	require.Error(t, err)
}

func TestNew_MissingCharacters_Errors(t *testing.T) {
	ctrl := gomock.NewController(t)
	_, err := New(&HandlerConfig{
		Manager: sessionv1alpha1mock.NewMockManager(ctrl),
		Broker:  sessionorch.NewBroker(),
	})
	require.Error(t, err)
}

func TestNew_WithoutRosterDependency_Succeeds(t *testing.T) {
	ctrl := gomock.NewController(t)
	h, err := New(&HandlerConfig{
		Manager:    sessionv1alpha1mock.NewMockManager(ctrl),
		Broker:     sessionorch.NewBroker(),
		Characters: charactermock.NewMockRepository(ctrl),
	})
	require.NoError(t, err)
	require.NotNil(t, h)
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

func TestAccessGate_LiteralHandlerBuildsSharedAccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := &Handler{
		manager:    sessionv1alpha1mock.NewMockManager(ctrl),
		characters: charactermock.NewMockRepository(ctrl),
	}

	gate, err := h.accessGate()
	require.NoError(t, err)
	require.NotNil(t, gate)
}
