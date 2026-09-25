package lobby_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	dungeonsmock "github.com/KirkDiggler/rpg-api/internal/dungeons/mock"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	lobbymock "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby/mock"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

// lobbyFixture is the shared setup and helper set every lobby-orchestrator
// suite composes: the generated SDK mock, the dungeon registry mock, the
// API-owned in-memory lobby repository, the broker, the character mock and the
// deterministic generators.
//
// It deliberately declares NO Test* method. Testify promotes Test* methods
// through embedding, so the shared part is a fixture rather than a suite: a
// suite that composes it inherits the fields and helpers only, leaving
// LobbySuite's lifecycle cases and StartContractSuite's StartEncounter
// launch-contract cases to run under their own Test entry points instead of
// each suite re-running the other's.
//
// The session SDK is mocked (manager), not built: these tests prove what the
// lobby asks the SDK to do and how it reads the SDK's answer back, not that
// a session really persists — that is the SDK's own suite. The dungeon
// registry is mocked for the same reason: no shipped YAML is loaded and no
// compiled world is constructed here. Both mocks are controller-isolated per
// test, so an SDK or registry call a test did not expect FAILS that test —
// there is no permissive AnyTimes default to hide an unauthorized or
// premature call.
type lobbyFixture struct {
	suite.Suite

	ctx       context.Context
	ctrl      *gomock.Controller
	charRepo  *charactermock.MockRepository
	manager   *lobbymock.MockSessionManager
	registry  *dungeonsmock.MockRegistry
	lobbyRepo lobbyrepo.Repository
	broker    *lobbyorch.Broker
	orch      *lobbyorch.Orchestrator
}

func (f *lobbyFixture) SetupTest() {
	f.ctx = context.Background()
	f.ctrl = gomock.NewController(f.T())
	f.charRepo = charactermock.NewMockRepository(f.ctrl)
	f.manager = lobbymock.NewMockSessionManager(f.ctrl)
	f.registry = dungeonsmock.NewMockRegistry(f.ctrl)
	f.lobbyRepo = lobbyrepo.NewInMemory()
	f.broker = lobbyorch.NewBroker()

	orch, err := lobbyorch.New(&lobbyorch.Config{
		LobbyRepo:            f.lobbyRepo,
		LobbyBroker:          f.broker,
		CharacterRepo:        f.charRepo,
		LobbyIDGenerator:     idgen.NewSequential("lobby"),
		JoinRefGenerator:     idgen.NewSequential("ref"),
		EncounterIDGenerator: idgen.NewSequential("enc"),
		Now:                  func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) },
		SessionManager:       f.manager,
		Dungeons:             f.registry,
	})
	f.Require().NoError(err)
	f.orch = orch
}

func (f *lobbyFixture) TearDownTest() {
	f.ctrl.Finish()
}

// expectCharacter arms s.charRepo to return, on the next Get(characterID), a
// character owned by playerID with the given display name and HP.
func (f *lobbyFixture) expectCharacter(characterID, playerID, name string, hp, maxHP int) {
	f.charRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: characterID}).
		Return(&characterrepo.GetOutput{
			Character: &entities.Character{
				Data: &toolkitchar.Data{
					PlayerID: playerID, Name: name, HitPoints: hp, MaxHitPoints: maxHP,
				},
			},
		}, nil)
}

// expectCharacterNotFound arms s.charRepo to return NotFound for characterID.
func (f *lobbyFixture) expectCharacterNotFound(characterID string) {
	f.charRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: characterID}).
		Return(nil, apierr.NotFound("character not found"))
}

// newOrchestratorWithLobbyRepo builds a second Orchestrator sharing every
// other suite dependency (broker, character repo, session manager,
// generators) but backed by repo instead of s.lobbyRepo — for tests that
// need to observe behavior when the lobby repository itself misbehaves
// (e.g. a wrapped repo that forces one method to fail).
func (f *lobbyFixture) newOrchestratorWithLobbyRepo(repo lobbyrepo.Repository) *lobbyorch.Orchestrator {
	orch, err := lobbyorch.New(&lobbyorch.Config{
		LobbyRepo:            repo,
		LobbyBroker:          f.broker,
		CharacterRepo:        f.charRepo,
		LobbyIDGenerator:     idgen.NewSequential("lobby"),
		JoinRefGenerator:     idgen.NewSequential("ref"),
		EncounterIDGenerator: idgen.NewSequential("enc"),
		Now:                  func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) },
		SessionManager:       f.manager,
		Dungeons:             f.registry,
	})
	f.Require().NoError(err)
	return orch
}

// seedLobby writes data directly to s.lobbyRepo, bypassing CreateLobby /
// JoinLobby (and their character-repo lookups) so tests that exercise a
// LATER RPC (SetReady, LeaveLobby, StartEncounter, SetConnected) can set up
// their starting roster in one call instead of chaining gomock-backed
// CreateLobby/JoinLobby calls.
func (f *lobbyFixture) seedLobby(data *lobbyrepo.Data) {
	f.Require().NoError(f.lobbyRepo.Save(f.ctx, data))
}

// LobbySuite runs the lobby-orchestrator RPC lifecycle cases (create, join,
// rebind, ready, leave, presence, active-lobby resume, abandon) declared
// across this package's *_test.go files. It composes lobbyFixture for the
// mocked dependencies and shared helpers; the fixture contributes no cases of
// its own, so TestLobbySuite runs exactly these lifecycle cases.
type LobbySuite struct {
	lobbyFixture
}

func TestLobbySuite(t *testing.T) {
	suite.Run(t, new(LobbySuite))
}
