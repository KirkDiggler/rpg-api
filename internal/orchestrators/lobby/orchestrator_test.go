package lobby_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.uber.org/mock/gomock"

	encounterhandlerv2 "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v2/encounter"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
)

// TestNew_NegativePartyCap_ReturnsError proves Config.PartyCap < 0 is
// rejected at construction rather than silently making every JoinLobby fail
// (a negative cap makes len(members) >= partyCap always true).
func TestNew_NegativePartyCap_ReturnsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	_, err := lobbyorch.New(&lobbyorch.Config{
		LobbyRepo:         lobbyrepo.NewInMemory(),
		LobbyBroker:       lobbyorch.NewBroker(),
		CharacterRepo:     charactermock.NewMockRepository(ctrl),
		EncounterRepo:     encountersv2.NewInMemory(),
		EncounterBroker:   tkenc.NewBroker(tkenc.NewInMemoryTransport()),
		CharacterResolver: encounterhandlerv2.StubCharacterResolver{},
		BuildCombatResolver: func(_ *tkenc.Data) tkenc.CombatResolver {
			return nil
		},
		BuildMovementResolver: func(_ *tkenc.Data) tkenc.MovementResolver {
			return nil
		},
		LobbyIDGenerator:     idgen.NewSequential("lobby"),
		JoinRefGenerator:     idgen.NewSequential("ref"),
		EncounterIDGenerator: idgen.NewSequential("enc"),
		PartyCap:             -1,
	})
	require.Error(t, err)
}

// TestNew_ContentDirUnreadable_ReturnsConstructionError proves Task E2's
// content-dir wiring end to end: an RPG_CONTENT_DIR pointing at a path
// that can't be read must fail lobbyorch.New() loudly, not panic and not
// silently degrade to embedded-only content — the same "operator/config
// mistake must fail construction" posture Task E2b applies to
// RPG_DUNGEON_KEY. Also pins the message content (Copilot PR #710 review):
// names RPG_CONTENT_DIR (the env var an operator actually set), and drops
// the "lobby orchestrator:" prefix that would double up once
// cmd/server/server.go's own lobbyorch.New call site wraps this error with
// fmt.Errorf("lobby orchestrator: %w", err) — the same class of doubled
// prefix Task E3's rider fixed for validateDungeonKeyOverride's errors.
func TestNew_ContentDirUnreadable_ReturnsConstructionError(t *testing.T) {
	t.Setenv("RPG_CONTENT_DIR", filepath.Join(t.TempDir(), "does-not-exist"))

	ctrl := gomock.NewController(t)
	_, err := lobbyorch.New(&lobbyorch.Config{
		LobbyRepo:         lobbyrepo.NewInMemory(),
		LobbyBroker:       lobbyorch.NewBroker(),
		CharacterRepo:     charactermock.NewMockRepository(ctrl),
		EncounterRepo:     encountersv2.NewInMemory(),
		EncounterBroker:   tkenc.NewBroker(tkenc.NewInMemoryTransport()),
		CharacterResolver: encounterhandlerv2.StubCharacterResolver{},
		BuildCombatResolver: func(_ *tkenc.Data) tkenc.CombatResolver {
			return nil
		},
		BuildMovementResolver: func(_ *tkenc.Data) tkenc.MovementResolver {
			return nil
		},
		LobbyIDGenerator:     idgen.NewSequential("lobby"),
		JoinRefGenerator:     idgen.NewSequential("ref"),
		EncounterIDGenerator: idgen.NewSequential("enc"),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "RPG_CONTENT_DIR", "message must name the env var an operator set")
	assert.NotContains(t, err.Error(), "lobby orchestrator", "must not carry a prefix that would double up when server.go wraps it")
}

// baseTestConfig returns the required-fields-only Config every New() test
// in this file builds on, customizing just the one field under test.
func baseTestConfig(t *testing.T) *lobbyorch.Config {
	t.Helper()
	ctrl := gomock.NewController(t)
	return &lobbyorch.Config{
		LobbyRepo:         lobbyrepo.NewInMemory(),
		LobbyBroker:       lobbyorch.NewBroker(),
		CharacterRepo:     charactermock.NewMockRepository(ctrl),
		EncounterRepo:     encountersv2.NewInMemory(),
		EncounterBroker:   tkenc.NewBroker(tkenc.NewInMemoryTransport()),
		CharacterResolver: encounterhandlerv2.StubCharacterResolver{},
		BuildCombatResolver: func(_ *tkenc.Data) tkenc.CombatResolver {
			return nil
		},
		BuildMovementResolver: func(_ *tkenc.Data) tkenc.MovementResolver {
			return nil
		},
		LobbyIDGenerator:     idgen.NewSequential("lobby"),
		JoinRefGenerator:     idgen.NewSequential("ref"),
		EncounterIDGenerator: idgen.NewSequential("enc"),
	}
}

// --- Task E2b: RPG_DUNGEON_KEY dev-loop override, validated at construction ---

// TestNew_DungeonKeyOverrideMustResolve proves an override key that
// resolves NOWHERE — not content, not the legacy dungeonSpecs map — fails
// construction loudly, per the plan's own test sketch. Also pins the
// message content: names RPG_DUNGEON_KEY (the env var an operator
// actually set), not Config.DungeonKeyOverride (the Go field), and — once
// wrapped by cmd/server/server.go's own "lobby orchestrator: %w" at the
// real New() call site — must never double up to "lobby orchestrator:
// lobby orchestrator: ...".
func TestNew_DungeonKeyOverrideMustResolve(t *testing.T) {
	cfg := baseTestConfig(t)
	cfg.DungeonKeyOverride = "atlantis"
	_, err := lobbyorch.New(cfg)
	require.Error(t, err, "an override key resolving nowhere (content or legacy) must fail construction loudly")
	assert.Contains(t, err.Error(), "RPG_DUNGEON_KEY", "message must name the env var an operator set")
	assert.NotContains(t, err.Error(), "lobby orchestrator", "must not carry a prefix that would double up when server.go wraps it")
}

// TestNew_DungeonKeyOverrideRejectsDisabledContentKey proves an override
// pointing at a content key that itself failed dungeonspec.Load at
// startup (contentSpecs[key].err != nil) must ALSO fail construction —
// not silently accepted just because the key string exists in the map.
func TestNew_DungeonKeyOverrideRejectsDisabledContentKey(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "broken-dungeon.yaml"),
		[]byte("version: 1\nkey: broken-dungeon\nname: Broken\nheight: 8\nrooms:\n"+
			"  - id: only-one\n    archetype: entrance\n    width: 6\n"),
		0o600,
	))
	t.Setenv("RPG_CONTENT_DIR", dir)

	cfg := baseTestConfig(t)
	cfg.DungeonKeyOverride = "broken-dungeon"
	_, err := lobbyorch.New(cfg)
	require.Error(t, err, "an override pointing at a DISABLED content key must fail construction, not defer to first request")
	assert.Contains(t, err.Error(), "RPG_DUNGEON_KEY")
	assert.NotContains(t, err.Error(), "lobby orchestrator")
}

// TestNew_DungeonKeyOverrideEmptyIsANoOp proves DungeonKeyOverride: ""
// (today's real deployments — RPG_DUNGEON_KEY unset) constructs exactly
// as before this task. Zero player-facing change is the whole point.
func TestNew_DungeonKeyOverrideEmptyIsANoOp(t *testing.T) {
	cfg := baseTestConfig(t)
	cfg.DungeonKeyOverride = ""
	_, err := lobbyorch.New(cfg)
	require.NoError(t, err)
}

// TestNew_DungeonKeyOverride_ResolvesViaContentRegistry proves the
// content-registry resolution half explicitly: reference-tomb ships
// embedded (Task E1), so an override naming it must construct cleanly
// with no RPG_CONTENT_DIR needed.
func TestNew_DungeonKeyOverride_ResolvesViaContentRegistry(t *testing.T) {
	cfg := baseTestConfig(t)
	cfg.DungeonKeyOverride = "reference-tomb"
	_, err := lobbyorch.New(cfg)
	require.NoError(t, err)
}

// TestNew_DungeonKeyOverride_ResolvesViaLegacyMap proves the OTHER
// resolution half: an override naming a legacy dungeonSpecs key (crypt)
// must also construct cleanly.
func TestNew_DungeonKeyOverride_ResolvesViaLegacyMap(t *testing.T) {
	cfg := baseTestConfig(t)
	cfg.DungeonKeyOverride = string(lobbyorch.DungeonKeyCrypt)
	_, err := lobbyorch.New(cfg)
	require.NoError(t, err)
}
