package lobby_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"

	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	"github.com/KirkDiggler/rpg-api/internal/entities"
	encounterhandlerv2 "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v2/encounter"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
)

// SessionStackSuite proves StartEncounter's new-stack branch in isolation:
// a real session.Manager (miniredis-backed, mirroring
// internal/integration/session's harness) rather than the mocked character
// repo the rest of this package's suites use, because the new stack's Join
// genuinely loads and reconstitutes a character -- a mock EXPECT() would
// only prove the call happened, not that a real sheet round-trips.
type SessionStackSuite struct {
	suite.Suite

	ctx       context.Context
	charRepo  characterrepo.Repository
	lobbyRepo lobbyrepo.Repository
	broker    *lobbyorch.Broker
	sessOrch  *sessionorch.Orchestrator
	orch      *lobbyorch.Orchestrator
}

func (s *SessionStackSuite) SetupTest() {
	s.ctx = context.Background()

	mr := miniredis.RunT(s.T())
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	s.T().Cleanup(func() { _ = client.Close() })

	charRepo, err := characterrepo.NewRedis(&characterrepo.RedisConfig{Client: client})
	s.Require().NoError(err)
	s.charRepo = charRepo

	sessOrch, err := sessionorch.New(sessionorch.Config{Redis: client, Characters: charRepo, TTL: 24 * time.Hour})
	s.Require().NoError(err)
	s.sessOrch = sessOrch

	s.lobbyRepo = lobbyrepo.NewInMemory()
	s.broker = lobbyorch.NewBroker()
	encBroker := tkenc.NewBroker(tkenc.NewInMemoryTransport())

	registry, err := lobbyorch.LoadContentRegistry()
	s.Require().NoError(err)

	orch, err := lobbyorch.New(&lobbyorch.Config{
		LobbyRepo:              s.lobbyRepo,
		LobbyBroker:            s.broker,
		CharacterRepo:          s.charRepo,
		EncounterRepo:          encountersv2.NewInMemory(),
		EncounterBroker:        encBroker,
		BuildCharacterResolver: func(_ *tkenc.Data) tkenc.CharacterResolver { return encounterhandlerv2.StubCharacterResolver{} },
		BuildCombatResolver:    func(_ *tkenc.Data) tkenc.CombatResolver { return nil },
		BuildMovementResolver:  func(_ *tkenc.Data) tkenc.MovementResolver { return nil },
		LobbyIDGenerator:       idgen.NewSequential("lobby"),
		JoinRefGenerator:       idgen.NewSequential("ref"),
		EncounterIDGenerator:   idgen.NewSequential("enc"),
		Registry:               registry,
		// The one field under test: presence routes StartEncounter to the
		// new stack for every call this suite makes.
		SessionManager: sessOrch.Manager,
	})
	s.Require().NoError(err)
	s.orch = orch
}

func TestSessionStackSuite(t *testing.T) {
	suite.Run(t, new(SessionStackSuite))
}

func (s *SessionStackSuite) seedCharacter(id, playerID, name string) {
	_, err := s.charRepo.Create(s.ctx, characterrepo.CreateInput{
		Character: &entities.Character{Data: &tkcharacter.Data{
			ID: id, PlayerID: playerID, Name: name, Level: 1,
			HitPoints: 10, MaxHitPoints: 10, ArmorClass: 10,
		}},
	})
	s.Require().NoError(err)
}

func (s *SessionStackSuite) seedReadyLobby(id, host string, others ...string) {
	members := map[string]*lobbyrepo.Member{
		host: {PlayerID: host, CharacterID: "char-" + host, IsHost: true, IsReady: true},
	}
	order := make([]string, 0, 1+len(others))
	order = append(order, host)
	for _, p := range others {
		members[p] = &lobbyrepo.Member{PlayerID: p, CharacterID: "char-" + p, IsReady: true}
		order = append(order, p)
	}
	s.Require().NoError(s.lobbyRepo.Save(s.ctx, &lobbyrepo.Data{
		ID: id, HostPlayerID: host, Status: lobbyrepo.StatusWaiting,
		Members: members, MemberOrder: order,
	}))
}

func (s *SessionStackSuite) TestStartEncounter_BuildsAGenuineNewStackSession() {
	s.seedCharacter("char-alice", "alice", "Alice")
	s.seedCharacter("char-bob", "bob", "Bob")
	s.seedReadyLobby("lobby-1", "alice", "bob")

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-1",
	})
	s.Require().NoError(err)
	s.Require().NotEmpty(out.EncounterID)

	// The session genuinely exists on the new stack: open, and both
	// characters are members the manager itself can report on.
	status, err := s.sessOrch.Manager.Status(s.ctx, &sdk.StatusInput{Session: out.EncounterID})
	s.Require().NoError(err)
	s.True(status.Open)

	view, err := s.sessOrch.Manager.View(s.ctx, &sdk.ViewInput{Session: out.EncounterID, Member: "char-alice"})
	s.Require().NoError(err)
	// The skeleton was placed away from the party's entry wall
	// deliberately (builtInMonsterPosition) so joining alone should not
	// yet reveal it -- this call succeeding at all (no ErrNoMember) is
	// the load-bearing assertion: alice is a real member of a real session.
	_ = view

	lobbyData, err := s.lobbyRepo.Get(s.ctx, "lobby-1")
	s.Require().NoError(err)
	s.Equal(lobbyrepo.StatusStarted, lobbyData.Status)
	s.Equal(out.EncounterID, lobbyData.EncounterID)
}

func (s *SessionStackSuite) TestStartEncounter_SpawnsThePlaceholderMonster() {
	s.seedCharacter("char-alice", "alice", "Alice")
	s.seedReadyLobby("lobby-1", "alice")

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-1",
	})
	s.Require().NoError(err)

	// Turn works for ANY member regardless of combat/equipment state
	// (design: "asked of a member, never of the session") -- the cleanest
	// available proof the skeleton is a real member of a real session
	// without depending on Attack's own equipment/combat preconditions.
	// ErrNoMember here would mean nothing was ever spawned.
	//
	// The clock is ClockTurn, not ClockWorld: the placeholder room has no
	// occluders (see builtInSessionWorld's doc), so the party and the
	// monster see each other -- and the fight starts itself -- the moment
	// both are placed. Real authored content would give a party room to
	// explore before engaging; this fixed world does not, which is a
	// consequence worth knowing about rather than a bug this test should
	// paper over.
	turn, err := s.sessOrch.Manager.Turn(s.ctx, &sdk.TurnInput{
		Session: out.EncounterID, Member: "skeleton-1",
	})
	s.Require().NoError(err)
	s.Equal(sdk.ClockTurn, turn.Clock, "no occluders in the placeholder room: sight forms the fight immediately")
}

func (s *SessionStackSuite) TestStartEncounter_NotHost_Errors() {
	s.seedCharacter("char-alice", "alice", "Alice")
	s.seedCharacter("char-bob", "bob", "Bob")
	s.seedReadyLobby("lobby-1", "alice", "bob")

	_, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "bob", LobbyID: "lobby-1",
	})
	s.Require().ErrorIs(err, lobbyorch.ErrNotHost)
}

func (s *SessionStackSuite) TestStartEncounter_NotAllReady_Errors() {
	s.seedCharacter("char-alice", "alice", "Alice")
	members := map[string]*lobbyrepo.Member{
		"alice": {PlayerID: "alice", CharacterID: "char-alice", IsHost: true, IsReady: false},
	}
	s.Require().NoError(s.lobbyRepo.Save(s.ctx, &lobbyrepo.Data{
		ID: "lobby-1", HostPlayerID: "alice", Status: lobbyrepo.StatusWaiting,
		Members: members, MemberOrder: []string{"alice"},
	}))

	_, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-1",
	})
	s.Require().ErrorIs(err, lobbyorch.ErrNotAllReady)
}

func (s *SessionStackSuite) TestStartEncounter_LobbyNotFound_Errors() {
	_, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "does-not-exist",
	})
	s.Require().ErrorIs(err, lobbyorch.ErrLobbyNotFound)
}
