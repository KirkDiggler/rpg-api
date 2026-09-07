package lobby_test

import (
	"testing"
	"time"

	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.uber.org/mock/gomock"

	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
)

// newTestSessionManager builds a miniredis-backed session.Manager for tests
// that only need lobbyorch.New to construct successfully — nothing in this
// file exercises StartEncounter itself (see start_encounter_session_stack_
// test.go for that), so a bare Manager over a fresh, empty Redis instance is
// enough to satisfy Config.SessionManager's now-required presence.
func newTestSessionManager(t *testing.T, charRepo characterrepo.Repository) *sessionorch.Orchestrator {
	t.Helper()
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	sessOrch, err := sessionorch.New(sessionorch.Config{
		Redis: client, Characters: charRepo, TTL: 24 * time.Hour,
		PresentationIDs: idgen.NewSequential("presentation"),
	})
	require.NoError(t, err)
	return sessOrch
}

// TestNew_NegativePartyCap_ReturnsError proves Config.PartyCap < 0 is
// rejected at construction rather than silently making every JoinLobby fail
// (a negative cap makes len(members) >= partyCap always true).
func TestNew_NegativePartyCap_ReturnsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	charRepo := charactermock.NewMockRepository(ctrl)
	sessOrch := newTestSessionManager(t, charRepo)
	_, err := lobbyorch.New(&lobbyorch.Config{
		LobbyRepo:            lobbyrepo.NewInMemory(),
		LobbyBroker:          lobbyorch.NewBroker(),
		CharacterRepo:        charRepo,
		LobbyIDGenerator:     idgen.NewSequential("lobby"),
		JoinRefGenerator:     idgen.NewSequential("ref"),
		EncounterIDGenerator: idgen.NewSequential("enc"),
		SessionManager:       sessOrch.Manager,
		Dungeons:             dungeonstest.Shipped(t),
		PartyCap:             -1,
	})
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
// in this file builds on, customizing just the one field under test.
func baseTestConfig(t *testing.T) *lobbyorch.Config {
	t.Helper()
	ctrl := gomock.NewController(t)
	charRepo := charactermock.NewMockRepository(ctrl)
	sessOrch := newTestSessionManager(t, charRepo)
	return &lobbyorch.Config{
		LobbyRepo:            lobbyrepo.NewInMemory(),
		LobbyBroker:          lobbyorch.NewBroker(),
		CharacterRepo:        charRepo,
		LobbyIDGenerator:     idgen.NewSequential("lobby"),
		JoinRefGenerator:     idgen.NewSequential("ref"),
		EncounterIDGenerator: idgen.NewSequential("enc"),
		SessionManager:       sessOrch.Manager,
		Dungeons:             dungeonstest.Shipped(t),
	}
}
