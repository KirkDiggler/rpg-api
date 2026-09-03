package lobby_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

// LobbySuite is the shared fixture for every lobby-orchestrator RPC test
// file in this package (create_lobby_test.go, join_lobby_test.go, etc.) —
// one Go type whose methods live across files, avoiding six copies of the
// same Config wiring. sessOrch is a real, miniredis-backed session.Manager
// (mirroring internal/integration/session's harness) — StartEncounter's
// sole remaining stack — rather than a mock, because Join genuinely loads
// and reconstitutes a character; a mock EXPECT() would only prove the call
// happened, not that a real sheet round-trips.
type LobbySuite struct {
	suite.Suite

	ctx       context.Context
	ctrl      *gomock.Controller
	charRepo  *charactermock.MockRepository
	lobbyRepo lobbyrepo.Repository
	broker    *lobbyorch.Broker
	sessOrch  *sessionorch.Orchestrator
	orch      *lobbyorch.Orchestrator
}

func (s *LobbySuite) SetupTest() {
	s.ctx = context.Background()
	s.ctrl = gomock.NewController(s.T())
	s.charRepo = charactermock.NewMockRepository(s.ctrl)
	s.lobbyRepo = lobbyrepo.NewInMemory()
	s.broker = lobbyorch.NewBroker()
	s.sessOrch = s.newSessionOrchestrator()

	orch, err := lobbyorch.New(&lobbyorch.Config{
		LobbyRepo:            s.lobbyRepo,
		LobbyBroker:          s.broker,
		CharacterRepo:        s.charRepo,
		LobbyIDGenerator:     idgen.NewSequential("lobby"),
		JoinRefGenerator:     idgen.NewSequential("ref"),
		EncounterIDGenerator: idgen.NewSequential("enc"),
		Now:                  func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) },
		SessionManager:       s.sessOrch.Manager,
		Dungeons:             dungeonstest.Shipped(s.T()),
	})
	s.Require().NoError(err)
	s.orch = orch
}

func (s *LobbySuite) TearDownTest() {
	s.ctrl.Finish()
}

// newSessionOrchestrator builds a fresh miniredis-backed session
// orchestrator sharing s.charRepo as its character store.
func (s *LobbySuite) newSessionOrchestrator() *sessionorch.Orchestrator {
	mr := miniredis.RunT(s.T())
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	s.T().Cleanup(func() { _ = client.Close() })
	sessOrch, err := sessionorch.New(sessionorch.Config{
		Redis: client, Characters: s.charRepo, TTL: 24 * time.Hour,
	})
	s.Require().NoError(err)
	return sessOrch
}

// seedLiveSession starts a real, open session under id on s.sessOrch —
// for tests that need GetMyActiveLobby/AbandonEncounter to see a genuinely
// live session without going through the full StartEncounter flow.
func (s *LobbySuite) seedLiveSession(id string) {
	entry, err := dungeonstest.Shipped(s.T()).Get(s.ctx, dungeons.DefaultKey)
	s.Require().NoError(err)
	_, err = s.sessOrch.Manager.StartSession(s.ctx, &sdk.StartSessionInput{
		Session: id, Encounter: id, World: entry.Dungeon.World,
	})
	s.Require().NoError(err)
}

// seedEndedSession starts then immediately ends a real session under id —
// for tests of the "session exists but is no longer open" case.
func (s *LobbySuite) seedEndedSession(id string) {
	s.seedLiveSession(id)
	_, err := s.sessOrch.Manager.End(s.ctx, &sdk.EndInput{Session: id, Ending: sessionworld.EndingWithdrawn})
	s.Require().NoError(err)
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
		SessionManager:       s.sessOrch.Manager,
		Dungeons:             dungeonstest.Shipped(s.T()),
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
