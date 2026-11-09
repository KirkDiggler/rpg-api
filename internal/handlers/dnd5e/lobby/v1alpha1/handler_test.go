package lobby_test

import (
	"context"
	"testing"
	"time"

	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	"github.com/KirkDiggler/rpg-api/internal/worldcontext"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	lobbyhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/lobby/v1alpha1"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/races"

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
	charRepo  characterrepo.Repository
	lobbyRepo lobbyrepo.Repository
	broker    *lobbyorch.Broker
	handler   *lobbyhandler.Handler
	shipped   *dungeons.FileRegistry
}

func (s *HandlerSuite) SetupSuite() {
	// Production compiles the immutable shipped shelf once, not per request.
	// Keep that same lifetime here; every case still gets fresh writable world,
	// lobby, character and session stores below, with real compiler/SDK wiring.
	s.shipped = dungeonstest.ShippedStore(s.T())
}

func (s *HandlerSuite) SetupTest() {
	base, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.ctx = worldcontext.With(auth.WithPlayerID(base, "alice"), worldcontext.Value{WorldID: testWorldID})

	s.lobbyRepo = lobbyrepo.NewInMemory()
	s.broker = lobbyorch.NewBroker()

	mr := miniredis.RunT(s.T())
	redisClient := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	s.T().Cleanup(func() { _ = redisClient.Close() })
	// The Redis character repository on the session's own Redis, as the
	// server wires it: the fenced session save lands seated sheets in the
	// same store as the world (rpg-project#879 S12a), so a mocked character
	// store would hold sheets the session never sees.
	charRepo, err := characterrepo.NewRedis(&characterrepo.RedisConfig{Client: redisClient})
	s.Require().NoError(err)
	s.charRepo = charRepo
	sessOrch, err := sessionorch.New(sessionorch.Config{
		Redis: redisClient, Characters: s.charRepo, TTL: 24 * time.Hour,
		PresentationIDs: idgen.NewSequential("presentation"),
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
		Dungeons:             dungeonstest.Join(s.T(), s.shipped, false),
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
}

// seedCharacter stores a character owned by playerID with the given display
// name and HP, replacing any earlier seed of the same id. Level 1 and an id
// on the sheet, because the session SDK refuses a sheet without either
// (rpg-toolkit#1261, rpg-toolkit#1766). A character never seeded is absent,
// which is every not-found case (the demo vendor's lookup included).
func (s *HandlerSuite) seedCharacter(characterID, playerID, name string, hp, maxHP int) {
	char := &entities.Character{Data: &toolkitchar.Data{
		ID: characterID, Level: 1, RaceID: races.Human,
		PlayerID: playerID, Name: name, HitPoints: hp, MaxHitPoints: maxHP,
	}}
	if _, err := s.charRepo.Get(context.Background(), characterrepo.GetInput{ID: characterID}); err == nil {
		_, err = s.charRepo.Update(context.Background(), characterrepo.UpdateInput{Character: char})
		s.Require().NoError(err)
		return
	}
	_, err := s.charRepo.Create(context.Background(), characterrepo.CreateInput{Character: char})
	s.Require().NoError(err)
}

// createLobby drives a real CreateLobby RPC (arming the character-repo mock)
// and returns the resulting lobby_id/join_ref, for tests of the RPCs that
// need an existing lobby.
func (s *HandlerSuite) createLobby(playerID, characterID, name string) (lobbyID, joinRef string) {
	s.seedCharacter(characterID, playerID, name, 12, 12)
	ctx := playerCtx(playerID)
	resp, err := s.handler.CreateLobby(ctx, &lobbyv1alpha1.CreateLobbyRequest{
		CampaignId: "campaign-1", CharacterId: characterID,
	})
	s.Require().NoError(err)
	return resp.GetLobbyId(), resp.GetJoinRef()
}

// testWorldID is the world every caller in this suite asks from.
const testWorldID = "test-world"

// playerCtx is an authenticated player in the suite's world, as the auth
// boundary installs both.
func playerCtx(playerID string) context.Context {
	return worldcontext.With(auth.WithPlayerID(context.Background(), playerID), worldcontext.Value{WorldID: testWorldID})
}

func TestHandlerSuite(t *testing.T) {
	suite.Run(t, new(HandlerSuite))
}
