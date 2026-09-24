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

// LobbySuite is the shared fixture for every lobby-orchestrator RPC test
// file in this package (create_lobby_test.go, join_lobby_test.go, etc.) —
// one Go type whose methods live across files, avoiding six copies of the
// same Config wiring.
//
// The session SDK is mocked (manager), not built: these tests prove what the
// lobby asks the SDK to do and how it reads the SDK's answer back, not that
// a session really persists — that is the SDK's own suite. The dungeon
// registry is mocked for the same reason: no shipped YAML is loaded and no
// compiled world is constructed here. Both mocks are controller-isolated per
// test, so an SDK or registry call a test did not expect FAILS that test —
// there is no permissive AnyTimes default to hide an unauthorized or
// premature call.
type LobbySuite struct {
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

func (s *LobbySuite) SetupTest() {
	s.ctx = context.Background()
	s.ctrl = gomock.NewController(s.T())
	s.charRepo = charactermock.NewMockRepository(s.ctrl)
	s.manager = lobbymock.NewMockSessionManager(s.ctrl)
	s.registry = dungeonsmock.NewMockRegistry(s.ctrl)
	s.lobbyRepo = lobbyrepo.NewInMemory()
	s.broker = lobbyorch.NewBroker()

	orch, err := lobbyorch.New(&lobbyorch.Config{
		LobbyRepo:            s.lobbyRepo,
		LobbyBroker:          s.broker,
		CharacterRepo:        s.charRepo,
		LobbyIDGenerator:     idgen.NewSequential("lobby"),
		JoinRefGenerator:     idgen.NewSequential("ref"),
		EncounterIDGenerator: idgen.NewSequential("enc"),
		Now:                  func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) },
		SessionManager:       s.manager,
		Dungeons:             s.registry,
	})
	s.Require().NoError(err)
	s.orch = orch
}

func (s *LobbySuite) TearDownTest() {
	s.ctrl.Finish()
}

// expectCharacter arms s.charRepo to return, on the next Get(characterID), a
// character owned by playerID with the given display name and HP.
func (s *LobbySuite) expectCharacter(characterID, playerID, name string, hp, maxHP int) {
	s.charRepo.EXPECT().
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
func (s *LobbySuite) expectCharacterNotFound(characterID string) {
	s.charRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: characterID}).
		Return(nil, apierr.NotFound("character not found"))
}

// newOrchestratorWithLobbyRepo builds a second Orchestrator sharing every
// other suite dependency (broker, character repo, session manager,
// generators) but backed by repo instead of s.lobbyRepo — for tests that
// need to observe behavior when the lobby repository itself misbehaves
// (e.g. a wrapped repo that forces one method to fail).
func (s *LobbySuite) newOrchestratorWithLobbyRepo(repo lobbyrepo.Repository) *lobbyorch.Orchestrator {
	orch, err := lobbyorch.New(&lobbyorch.Config{
		LobbyRepo:            repo,
		LobbyBroker:          s.broker,
		CharacterRepo:        s.charRepo,
		LobbyIDGenerator:     idgen.NewSequential("lobby"),
		JoinRefGenerator:     idgen.NewSequential("ref"),
		EncounterIDGenerator: idgen.NewSequential("enc"),
		Now:                  func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) },
		SessionManager:       s.manager,
		Dungeons:             s.registry,
	})
	s.Require().NoError(err)
	return orch
}

// seedLobby writes data directly to s.lobbyRepo, bypassing CreateLobby /
// JoinLobby (and their character-repo lookups) so tests that exercise a
// LATER RPC (SetReady, LeaveLobby, StartEncounter, SetConnected) can set up
// their starting roster in one call instead of chaining gomock-backed
// CreateLobby/JoinLobby calls.
func (s *LobbySuite) seedLobby(data *lobbyrepo.Data) {
	s.Require().NoError(s.lobbyRepo.Save(s.ctx, data))
}

func TestLobbySuite(t *testing.T) {
	suite.Run(t, new(LobbySuite))
}
