package lobby_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/dungeonregistry"
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
	sessOrch, err := sessionorch.New(sessionorch.Config{Redis: client, Characters: charRepo, TTL: 24 * time.Hour})
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
		Registry:             dungeonregistry.New(nil),
		SessionManager:       sessOrch.Manager,
		PartyCap:             -1,
	})
	require.Error(t, err)
}

// TestNew_RegistryRequired proves Config.Registry is a required dependency
// — a lobbyorch.New() call site (production or test) that forgets to wire
// the shared registry must fail construction loudly, the same posture
// every other required Config field in this constructor already has.
// Content-dir loading itself has moved to LoadContentRegistry, called
// BEFORE New() now (see TestLoadContentRegistry_ContentDirUnreadable_ReturnsError
// in dungeon_spec_internal_test.go for that construction-time failure
// mode's own coverage) — New() itself no longer reads RPG_CONTENT_DIR at
// all.
func TestNew_RegistryRequired(t *testing.T) {
	cfg := baseTestConfig(t)
	cfg.Registry = nil
	_, err := lobbyorch.New(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Registry")
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
// Registry is built via LoadContentRegistry() against whatever
// RPG_CONTENT_DIR is set in the calling test's env at the time this runs
// (unset in most tests — just the embedded specs) — callers that need a
// specific RPG_CONTENT_DIR override must t.Setenv it BEFORE calling this.
func baseTestConfig(t *testing.T) *lobbyorch.Config {
	t.Helper()
	ctrl := gomock.NewController(t)
	charRepo := charactermock.NewMockRepository(ctrl)
	registry, err := lobbyorch.LoadContentRegistry()
	require.NoError(t, err)
	sessOrch := newTestSessionManager(t, charRepo)
	return &lobbyorch.Config{
		LobbyRepo:            lobbyrepo.NewInMemory(),
		LobbyBroker:          lobbyorch.NewBroker(),
		CharacterRepo:        charRepo,
		LobbyIDGenerator:     idgen.NewSequential("lobby"),
		JoinRefGenerator:     idgen.NewSequential("ref"),
		EncounterIDGenerator: idgen.NewSequential("enc"),
		Registry:             registry,
		SessionManager:       sessOrch.Manager,
	}
}

// TestLoadContentRegistry_ContentDirUnreadable_FailsBeforeNewIsEverReached
// pins that an unreadable RPG_CONTENT_DIR fails LOUDLY at LoadContentRegistry
// — before lobbyorch.New() is ever called — matching the construction-time
// posture the rest of this file's Config-validation tests assume. Full
// LoadContentRegistry coverage (embedded content, disabled keys) lives in
// dungeon_spec_internal_test.go; this is the "New() is never reached" half
// pinned at this file's own layer.
func TestLoadContentRegistry_ContentDirUnreadable_FailsBeforeNewIsEverReached(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	t.Setenv("RPG_CONTENT_DIR", missing)
	_, err := lobbyorch.LoadContentRegistry()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "RPG_CONTENT_DIR")
}
