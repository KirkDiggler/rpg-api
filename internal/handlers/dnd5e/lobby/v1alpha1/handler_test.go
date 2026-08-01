package lobby_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	lobbyv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/lobby/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	lobbyhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/lobby/v1alpha1"
	encounterhandlerv2 "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v2/encounter"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

// HandlerSuite wires a real lobby orchestrator (in-memory repos) behind the
// Handler, so these tests exercise proto<->entity translation and envelope
// validation — the orchestrator's own business-logic branches are covered by
// internal/orchestrators/lobby's suite, so handler tests don't re-derive them.
type HandlerSuite struct {
	suite.Suite

	ctx       context.Context
	cancel    context.CancelFunc
	ctrl      *gomock.Controller
	charRepo  *charactermock.MockRepository
	lobbyRepo lobbyrepo.Repository
	encRepo   encountersv2.Repository
	broker    *lobbyorch.Broker
	handler   *lobbyhandler.Handler
}

func (s *HandlerSuite) SetupTest() {
	base, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.ctx = auth.WithPlayerID(base, "alice")

	s.ctrl = gomock.NewController(s.T())
	s.charRepo = charactermock.NewMockRepository(s.ctrl)
	s.lobbyRepo = lobbyrepo.NewInMemory()
	s.broker = lobbyorch.NewBroker()

	registry, err := lobbyorch.LoadContentRegistry()
	s.Require().NoError(err)

	s.encRepo = encountersv2.NewInMemory()
	orch, err := lobbyorch.New(&lobbyorch.Config{
		LobbyRepo:              s.lobbyRepo,
		LobbyBroker:            s.broker,
		CharacterRepo:          s.charRepo,
		EncounterRepo:          s.encRepo,
		EncounterBroker:        tkenc.NewBroker(tkenc.NewInMemoryTransport()),
		BuildCharacterResolver: func(_ *tkenc.Data) tkenc.CharacterResolver { return encounterhandlerv2.StubCharacterResolver{} },
		BuildCombatResolver: func(_ *tkenc.Data) tkenc.CombatResolver {
			return nil
		},
		BuildMovementResolver: func(_ *tkenc.Data) tkenc.MovementResolver {
			return nil
		},
		LobbyIDGenerator:     idgen.NewSequential("lobby"),
		JoinRefGenerator:     idgen.NewSequential("ref"),
		EncounterIDGenerator: idgen.NewSequential("enc"),
		Now:                  func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) },
		Registry:             registry,
	})
	s.Require().NoError(err)

	h, err := lobbyhandler.New(&lobbyhandler.HandlerConfig{
		Orchestrator: orch, Broker: s.broker,
		Now: func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) },
	})
	s.Require().NoError(err)
	s.handler = h
}

func (s *HandlerSuite) TearDownTest() {
	s.cancel()
	s.ctrl.Finish()
}

func (s *HandlerSuite) expectCharacter(characterID, playerID, name string, hp, maxHP int) {
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

func (s *HandlerSuite) expectCharacterNotFound(characterID string) {
	s.charRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: characterID}).
		Return(nil, apierr.NotFound("character not found"))
}

// createLobby drives a real CreateLobby RPC (arming the character-repo mock)
// and returns the resulting lobby_id/join_ref, for tests of the RPCs that
// need an existing lobby.
func (s *HandlerSuite) createLobby(playerID, characterID, name string) (lobbyID, joinRef string) {
	s.expectCharacter(characterID, playerID, name, 12, 12)
	ctx := auth.WithPlayerID(context.Background(), playerID)
	resp, err := s.handler.CreateLobby(ctx, &lobbyv1alpha1.CreateLobbyRequest{
		CampaignId: "campaign-1", CharacterId: characterID,
	})
	s.Require().NoError(err)
	return resp.GetLobbyId(), resp.GetJoinRef()
}

func TestHandlerSuite(t *testing.T) {
	suite.Run(t, new(HandlerSuite))
}
