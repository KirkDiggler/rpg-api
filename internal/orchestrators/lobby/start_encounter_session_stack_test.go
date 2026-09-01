package lobby_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"

	coreResources "github.com/KirkDiggler/rpg-toolkit/core/resources"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/conditions"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	dnd5eResources "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/resources"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/saves"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
	rosterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/roster"
)

// SessionStackSuite proves StartEncounter's new-stack branch in isolation:
// a real session.Manager (miniredis-backed, mirroring
// internal/integration/session's harness) rather than the mocked character
// repo the rest of this package's suites use, because the new stack's Join
// genuinely loads and reconstitutes a character -- a mock EXPECT() would
// only prove the call happened, not that a real sheet round-trips.
type SessionStackSuite struct {
	suite.Suite

	ctx        context.Context
	charRepo   characterrepo.Repository
	lobbyRepo  lobbyrepo.Repository
	broker     *lobbyorch.Broker
	sessOrch   *sessionorch.Orchestrator
	orch       *lobbyorch.Orchestrator
	rosterRepo rosterrepo.Repository
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
	s.rosterRepo = rosterrepo.NewInMemory()

	orch, err := lobbyorch.New(&lobbyorch.Config{
		LobbyRepo:            s.lobbyRepo,
		LobbyBroker:          s.broker,
		CharacterRepo:        s.charRepo,
		LobbyIDGenerator:     idgen.NewSequential("lobby"),
		JoinRefGenerator:     idgen.NewSequential("ref"),
		EncounterIDGenerator: idgen.NewSequential("enc"),
		SessionManager:       sessOrch.Manager,
		Dungeons:             dungeonstest.Shipped(s.T()),
		RosterRepo:           s.rosterRepo,
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

// TestStartEncounter_WritesTheRosterRow pins the launch-written roster
// (rpg-project#264, ideas/characters/presentation): the one moment that knows
// every member and every authored spawn persists identity facts for GetRoster
// to read back. Player rows are id-only (name and refs are read fresh from the
// character record at serve time — pinned by the ABSENCE of a stored name
// here); monster rows carry the authored ref and the name the spawn itself
// reported, so the roster can never drift from what sightings call them.
func (s *SessionStackSuite) TestStartEncounter_WritesTheRosterRow() {
	s.seedCharacter("char-alice", "alice", "Alice")
	s.seedCharacter("char-bob", "bob", "Bob")
	s.seedReadyLobby("lobby-1", "alice", "bob")

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-1",
	})
	s.Require().NoError(err)

	row, err := s.rosterRepo.Get(s.ctx, out.EncounterID)
	s.Require().NoError(err)
	s.Equal(out.EncounterID, row.EncounterID)

	players := make([]rosterrepo.Member, 0)
	monsters := make([]rosterrepo.Member, 0)
	for _, m := range row.Members {
		switch m.Kind {
		case rosterrepo.KindPlayer:
			players = append(players, m)
		case rosterrepo.KindMonster:
			monsters = append(monsters, m)
		default:
			s.Failf("kind", "member %q has unspecified kind", m.ID)
		}
	}

	s.Equal([]rosterrepo.Member{
		{ID: "char-alice", Kind: rosterrepo.KindPlayer},
		{ID: "char-bob", Kind: rosterrepo.KindPlayer},
	}, players, "player rows are identity-only, in join order, with nothing stored that the character record owns")

	s.Require().NotEmpty(monsters, "the authored tomb has a garrison; its spawns must be on the roster")
	for _, m := range monsters {
		s.NotEmpty(m.Ref, "monster %q must carry its authored ref", m.ID)
		s.NotEmpty(m.Name, "monster %q must carry the spawn-reported name", m.ID)
	}
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

	atlas, err := s.sessOrch.Manager.Atlas(s.ctx, &sdk.AtlasInput{Session: out.EncounterID, Member: "char-alice"})
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

// TestStartEncounter_UnknownDungeonKeyIsRefused pins design §3c: a key the
// registry does not have is ErrDungeonNotFound, never silently the tomb. The
// lobby is untouched -- still WAITING, no encounter -- because the refusal
// happens before anything is written.
func (s *SessionStackSuite) TestStartEncounter_UnknownDungeonKeyIsRefused() {
	s.seedCharacter("char-alice", "alice", "Alice")
	s.seedReadyLobby("lobby-1", "alice")

	_, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-1", DungeonKey: "nope",
	})
	s.Require().ErrorIs(err, lobbyorch.ErrDungeonNotFound)

	data, err := s.lobbyRepo.Get(s.ctx, "lobby-1")
	s.Require().NoError(err)
	s.Equal(lobbyrepo.StatusWaiting, data.Status, "nothing was written")
	s.Empty(data.EncounterID)
}

// TestStartEncounter_ExplicitDefaultKeyIsTheTomb: naming the tomb and naming
// nothing are the same dungeon.
func (s *SessionStackSuite) TestStartEncounter_ExplicitDefaultKeyIsTheTomb() {
	s.seedCharacter("char-alice", "alice", "Alice")
	s.seedReadyLobby("lobby-1", "alice")

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-1", DungeonKey: lobbyorch.DungeonKey(dungeons.DefaultKey),
	})
	s.Require().NoError(err)

	_, err = s.sessOrch.Manager.Turn(s.ctx, &sdk.TurnInput{Session: out.EncounterID, Member: "skeleton-captain-1"})
	s.Require().NoError(err, "the captain is in it, so it is the tomb")
}

// TestStartEncounter_PlaysADungeonTheAuthorPut is the builder's loop from the
// lobby's side: a dungeon that arrived through Put (not the shipped tree) is
// the one a session starts on, and its atlas reaches the wire.
func (s *SessionStackSuite) TestStartEncounter_PlaysADungeonTheAuthorPut() {
	registry, _ := dungeonstest.Scratch(s.T())
	tomb, err := registry.Get(s.ctx, dungeons.DefaultKey)
	s.Require().NoError(err)
	crypt := bytes.Replace(tomb.YAML, []byte("key: reference-tomb"), []byte("key: crypt"), 1)
	res, err := registry.Put(s.ctx, &dungeons.PutInput{Key: "crypt", YAML: crypt})
	s.Require().NoError(err)
	s.Require().Empty(res.Errors)

	orch, err := lobbyorch.New(&lobbyorch.Config{
		LobbyRepo:            s.lobbyRepo,
		LobbyBroker:          s.broker,
		CharacterRepo:        s.charRepo,
		LobbyIDGenerator:     idgen.NewSequential("lobby"),
		JoinRefGenerator:     idgen.NewSequential("ref"),
		EncounterIDGenerator: idgen.NewSequential("enc"),
		SessionManager:       s.sessOrch.Manager,
		Dungeons:             registry,
		RosterRepo:           rosterrepo.NewInMemory(),
	})
	s.Require().NoError(err)

	s.seedCharacter("char-alice", "alice", "Alice")
	s.seedReadyLobby("lobby-1", "alice")

	out, err := orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-1", DungeonKey: "crypt",
	})
	s.Require().NoError(err)

	atlas, err := s.sessOrch.Manager.Atlas(s.ctx, &sdk.AtlasInput{Session: out.EncounterID, Member: "char-alice"})
	s.Require().NoError(err)
	s.NotEmpty(atlas.Cells, "the authored dungeon's floor reached the wire")
	s.Require().Len(atlas.Regions, 3, "and its regions, with what they carry")
	s.Equal("crypt", atlas.Regions[0].Archetype)
	s.Equal(res.Entry.Atlas.Cells, atlas.Cells,
		"PutDungeon's atlas and the started session's GetAtlas are the same cells -- one producer (design §3a)")
	s.Equal(res.Entry.Atlas.Regions, atlas.Regions, "and the same regions")
	s.Require().NotEmpty(atlas.Doorways)
	s.True(strings.HasPrefix(atlas.Doorways[0].Door, "crypt"), "doorway %q is minted under the authored key, not the tomb's", atlas.Doorways[0].Door)
}

func (s *SessionStackSuite) TestListDungeons_ReadsTheRegistry() {
	out, err := s.orch.ListDungeons(s.ctx, &lobbyorch.ListDungeonsInput{})
	s.Require().NoError(err)
	s.Require().Len(out.Dungeons, 1)
	s.Equal(dungeons.DefaultKey, out.Dungeons[0].Key)
}

// TestStartEncounter_LaunchRestoresEveryMemberFully pins Kirk's ruling from
// the 2026-08-24 walk (rpg-project#253, rpg-api#828): launch is an arcade
// run start, so a member limping in at 2 HP and a member who died in a
// prior run are both seated at full HP with death-save state cleared —
// and the restore is PERSISTED, not just seated (the repo record is what
// every later reload reads).
func (s *SessionStackSuite) TestStartEncounter_LaunchRestoresEveryMemberFully() {
	// host arrives wounded (2 HP of 10), guest arrives dead (0 HP, 3 fails).
	_, err := s.charRepo.Create(s.ctx, characterrepo.CreateInput{
		Character: &entities.Character{Data: &tkcharacter.Data{
			ID: "char-p1", PlayerID: "p1", Name: "Wounded", Level: 1,
			HitPoints: 2, MaxHitPoints: 10, ArmorClass: 10,
		}},
	})
	s.Require().NoError(err)
	unconscious, err := json.Marshal(conditions.UnconsciousData{
		Ref:      refs.Conditions.Unconscious(),
		MemberID: "char-p2",
		Failures: 3,
		Dead:     true,
	})
	s.Require().NoError(err)
	_, err = s.charRepo.Create(s.ctx, characterrepo.CreateInput{
		Character: &entities.Character{Data: &tkcharacter.Data{
			ID: "char-p2", PlayerID: "p2", Name: "Dead", Level: 1,
			HitPoints: 0, MaxHitPoints: 12, ArmorClass: 10,
			DeathSaveState: &saves.DeathSaveState{Failures: 3, Dead: true},
			Conditions:     []json.RawMessage{unconscious},
			Resources: map[coreResources.ResourceKey]tkcharacter.RecoverableResourceData{
				// ResetType matters now: LongRest restores only resources that
				// reset on a long or short rest (character/character.go), where
				// the retired RestoreForLaunch refilled every entry regardless.
				// Real rage charges are always created ResetLongRest
				// (draft.go's initializeClassResources) -- this fixture matches
				// what a real barbarian's persisted data actually carries.
				dnd5eResources.RageCharges: {Current: 0, Maximum: 2, ResetType: coreResources.ResetLongRest},
			},
		}},
	})
	s.Require().NoError(err)
	s.seedReadyLobby("lobby-restore", "p1", "p2")

	_, err = s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "p1", LobbyID: "lobby-restore",
	})
	s.Require().NoError(err)

	got, err := s.charRepo.Get(s.ctx, characterrepo.GetInput{ID: "char-p1"})
	s.Require().NoError(err)
	s.Equal(10, got.Character.Data.HitPoints, "a wounded member launches at full HP")

	got, err = s.charRepo.Get(s.ctx, characterrepo.GetInput{ID: "char-p2"})
	s.Require().NoError(err)
	s.Equal(12, got.Character.Data.HitPoints, "a dead member launches at full HP")
	// Character.LongRest -- the toolkit's own now-authoritative launch
	// mechanism (rpg-toolkit#1376 retired the Data-only RestoreForLaunch this
	// once read nil from) -- clears death-save state to a fresh zero-value
	// struct rather than nil; both mean "no death save in progress", but the
	// wire/storage representation changed with the mechanism.
	s.Equal(&saves.DeathSaveState{}, got.Character.Data.DeathSaveState,
		"death-save state is reset, not nil, after a launch long rest")
	s.Empty(got.Character.Data.Conditions, "the Unconscious blob is stripped, not left to re-hydrate")
	s.Equal(2, got.Character.Data.Resources[dnd5eResources.RageCharges].Current,
		"spent resource pools refill at launch")
}
