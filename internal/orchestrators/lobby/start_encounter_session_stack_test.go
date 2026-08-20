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

	// This call succeeding at all (no ErrNoMember) is the load-bearing
	// assertion: alice is a real member of a real session.
	_, err = s.sessOrch.Manager.View(s.ctx, &sdk.ViewInput{Session: out.EncounterID, Member: "char-alice"})
	s.Require().NoError(err)

	lobbyData, err := s.lobbyRepo.Get(s.ctx, "lobby-1")
	s.Require().NoError(err)
	s.Equal(lobbyrepo.StatusStarted, lobbyData.Status)
	s.Equal(out.EncounterID, lobbyData.EncounterID)
}

// TestStartEncounter_SeatsTheTombsWholeGarrison checks that starting on the new
// stack seeds the AUTHORED dungeon, not a stand-in: both hall skeletons and the
// captain behind the locked door are real members of a real session.
//
// Turn is the probe because it works for ANY member regardless of combat or
// equipment state ("asked of a member, never of the session"), so it proves
// membership without depending on Attack's own preconditions. ErrNoMember for
// any of these three would mean the tomb's roster did not arrive.
//
// Every one of them is on the WORLD clock. That is the whole point of seeding a
// real dungeon rather than a single room: the party comes in at the entrance,
// the garrison holds the hall behind a wall, and sight has a range of four
// cells since session/v0.18.0 -- so nobody is in contact and there is a dungeon
// to walk before there is a fight. Asserted rather than relaxed to "either
// clock is fine", because whether a session opens in combat is the single most
// visible thing about the world it opens in.
func (s *SessionStackSuite) TestStartEncounter_SeatsTheTombsWholeGarrison() {
	s.seedCharacter("char-alice", "alice", "Alice")
	s.seedReadyLobby("lobby-1", "alice")

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-1",
	})
	s.Require().NoError(err)

	for _, member := range []string{"skeleton-1", "skeleton-2", "skeleton-captain-1"} {
		turn, terr := s.sessOrch.Manager.Turn(s.ctx, &sdk.TurnInput{
			Session: out.EncounterID, Member: member,
		})
		s.Require().NoErrorf(terr, "%s should be a member of the seeded tomb", member)
		s.Equalf(sdk.ClockWorld, turn.Clock, "%s should not be in a fight yet", member)
	}
}

// TestStartEncounter_ThePartyEntersOutOfSightOfTheGarrison is the same fact from
// the party's side, and the one a player would notice: alice arrives and can
// see none of the three monsters.
//
// It is a separate assertion from the clock above rather than a restatement.
// The clock says no fight formed; this says the fog of war is genuinely drawn,
// which is what the walls between the chambers are FOR. A tomb compiled without
// its seams would leave the entrance and hall one open space, and this is the
// test that would notice.
func (s *SessionStackSuite) TestStartEncounter_ThePartyEntersOutOfSightOfTheGarrison() {
	s.seedCharacter("char-alice", "alice", "Alice")
	s.seedCharacter("char-bob", "bob", "Bob")
	s.seedReadyLobby("lobby-1", "alice", "bob")

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-1",
	})
	s.Require().NoError(err)

	view, err := s.sessOrch.Manager.View(s.ctx, &sdk.ViewInput{
		Session: out.EncounterID, Member: "char-alice",
	})
	s.Require().NoError(err)

	subjects := make([]string, 0, len(view))
	for _, sighting := range view {
		subjects = append(subjects, sighting.Subject)
	}

	// POSITIVE CONTROL FIRST, because the assertion that follows is a loop over
	// this slice and an empty one would satisfy it without proving anything at
	// all. Bob is seated beside alice at the way in, so if this read works she
	// sees him -- and only then does not seeing the garrison mean the walls and
	// the sight range are doing the work.
	s.Require().Contains(subjects, "char-bob", "alice sees the party member standing next to her")

	for _, subject := range subjects {
		s.NotContainsf(subject, "skeleton",
			"the garrison is behind a wall and out of range; alice should not see %s", subject)
	}
}

// TestStartEncounter_TheTombReachesTheWire checks the seeded world through the
// same read a client uses, because that is the only place the tomb becoming
// real actually matters. Everything above could pass with a world nobody could
// draw.
//
// The coffin is the interesting prop rather than a random one: it is the tomb's
// single authored exception, walked around but SEEN OVER, and it is exactly the
// distinction the atlas gained props to express (it used to be a bare occluder
// coordinate, indistinguishable from a pillar).
func (s *SessionStackSuite) TestStartEncounter_TheTombReachesTheWire() {
	s.seedCharacter("char-alice", "alice", "Alice")
	s.seedReadyLobby("lobby-1", "alice")

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-1",
	})
	s.Require().NoError(err)

	atlas, err := s.sessOrch.Manager.Atlas(s.ctx, &sdk.AtlasInput{Session: out.EncounterID})
	s.Require().NoError(err)

	s.NotEmpty(atlas.Cells, "the map has floor")
	s.NotEmpty(atlas.Boundaries, "and walls between its chambers -- rooms no longer imply them")
	s.NotEmpty(atlas.Doorways, "and ways through those walls")

	var coffin *sdk.AtlasProp
	for i := range atlas.Props {
		if atlas.Props[i].Ref == "dnd5e:props:coffin" {
			coffin = &atlas.Props[i]
			break
		}
	}
	s.Require().NotNil(coffin, "the tomb's coffin reached the wire")
	s.True(coffin.BlocksMovement, "a coffin is walked around")
	s.False(coffin.BlocksLineOfSight, "and seen over")
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
