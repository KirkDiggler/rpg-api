package lobby_test

import (
	"context"
	"testing"
	"time"

	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	lobbyhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/lobby/v1alpha1"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
	rosterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/roster"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"

	lobbyv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/lobby/v1alpha1"
)

// HandlerSuite wires a real lobby orchestrator (in-memory lobby repo, a
// miniredis-backed session.Manager for StartEncounter's sole remaining
// stack) behind the Handler, so these tests exercise proto<->entity
// translation and envelope validation — the orchestrator's own
// business-logic branches are covered by internal/orchestrators/lobby's
// suite, so handler tests don't re-derive them.
type HandlerSuite struct {
	suite.Suite

	ctx       context.Context
	cancel    context.CancelFunc
	ctrl      *gomock.Controller
	charRepo  *charactermock.MockRepository
	lobbyRepo lobbyrepo.Repository
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

	mr := miniredis.RunT(s.T())
	redisClient := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	s.T().Cleanup(func() { _ = redisClient.Close() })
	sessOrch, err := sessionorch.New(sessionorch.Config{
		Redis: redisClient, Characters: s.charRepo, TTL: 24 * time.Hour,
	})
	s.Require().NoError(err)

	orch, err := lobbyorch.New(&lobbyorch.Config{
		LobbyRepo:            s.lobbyRepo,
		LobbyBroker:          s.broker,
		CharacterRepo:        s.charRepo,
		LobbyIDGenerator:     idgen.NewSequential("lobby"),
		JoinRefGenerator:     idgen.NewSequential("ref"),
		EncounterIDGenerator: idgen.NewSequential("enc"),
		Now:                  func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) },
		SessionManager:       sessOrch.Manager,
		Dungeons:             dungeonstest.Shipped(s.T()),
		RosterRepo:           rosterrepo.NewInMemory(),
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

// expectCharacter arms s.charRepo to return a character owned by playerID
// with the given display name and HP on any Get(characterID) call — AnyTimes
// rather than the gomock default of exactly once, because a call this arms
// for may be read more than once (CreateLobby's resolveCharacter, PLUS the
// session SDK's own Join, which loads a character's sheet for placement and
// again for the standing/contact check triggered by arriving in sight of
// another member — an internal call count this package's tests should not
// have to pin).
func (s *HandlerSuite) expectCharacter(characterID, playerID, name string, hp, maxHP int) {
	s.charRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: characterID}).
		Return(&characterrepo.GetOutput{
			Character: &entities.Character{
				Data: &toolkitchar.Data{
					// ID as well as the rest, because a repository that
					// answers Get(id) with a sheet carrying a different id —
					// or none — has violated its contract, and the session
					// SDK now says so rather than attaching whatever came
					// back (rpg-toolkit#1261). This fake had always returned
					// an ID-less sheet; nothing depended on it, which is
					// exactly why nothing noticed.
					ID:       characterID,
					PlayerID: playerID, Name: name, HitPoints: hp, MaxHitPoints: maxHP,
				},
			},
		}, nil).AnyTimes()
}

func (s *HandlerSuite) expectCharacterNotFound(characterID string) {
	s.charRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: characterID}).
		Return(nil, apierr.NotFound("character not found")).AnyTimes()
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
