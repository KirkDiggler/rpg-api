package lobby_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	dungeonsmock "github.com/KirkDiggler/rpg-api/internal/dungeons/mock"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	lobbymock "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby/mock"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
)

// TestNew_AcceptsSessionManagerMock proves Config.SessionManager accepts the
// consumer-owned SessionManager interface, so constructor tests can supply the
// generated SDK mock instead of building a real session.Manager over Redis.
func TestNew_AcceptsSessionManagerMock(t *testing.T) {
	cfg := baseTestConfig(t)
	cfg.SessionManager = lobbymock.NewMockSessionManager(gomock.NewController(t))
	got, err := lobbyorch.New(cfg)
	require.NoError(t, err)
	require.NotNil(t, got)
}

// TestNew_NegativePartyCap_ReturnsError proves Config.PartyCap < 0 is
// rejected at construction rather than silently making every JoinLobby fail
// (a negative cap makes len(members) >= partyCap always true).
func TestNew_NegativePartyCap_ReturnsError(t *testing.T) {
	cfg := baseTestConfig(t)
	cfg.PartyCap = -1
	_, err := lobbyorch.New(cfg)
	require.Error(t, err)
}

// TestNew_SessionManagerRequired proves Config.SessionManager is a required
// dependency: with the old encounter stack removed (rpg-project#227), the
// session stack is StartEncounter's ONLY implementation, not an opt-in
// coexistence branch — a lobbyorch.New() call site that forgets to wire one
// must fail construction loudly, exactly like every other required Config
// field.
func TestNew_SessionManagerRequired(t *testing.T) {
	cfg := baseTestConfig(t)
	cfg.SessionManager = nil
	_, err := lobbyorch.New(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SessionManager")
}

// baseTestConfig returns the required-fields-only Config every New() test
// in this file builds on, customizing just the one field under test. Both
// mocks are controller-isolated per call so a test that never exercises an
// SDK method still fails if lobbyorch.New touches one unexpectedly.
func baseTestConfig(t *testing.T) *lobbyorch.Config {
	t.Helper()
	ctrl := gomock.NewController(t)
	charRepo := charactermock.NewMockRepository(ctrl)
	return &lobbyorch.Config{
		LobbyRepo:            lobbyrepo.NewInMemory(),
		LobbyBroker:          lobbyorch.NewBroker(),
		CharacterRepo:        charRepo,
		LobbyIDGenerator:     idgen.NewSequential("lobby"),
		JoinRefGenerator:     idgen.NewSequential("ref"),
		EncounterIDGenerator: idgen.NewSequential("enc"),
		SessionManager:       lobbymock.NewMockSessionManager(ctrl),
		Dungeons:             dungeonsmock.NewMockRegistry(ctrl),
	}
}
