package lobby_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-toolkit/core"
	coreResources "github.com/KirkDiggler/rpg-toolkit/core/resources"
	"github.com/KirkDiggler/rpg-toolkit/dice"
	"github.com/KirkDiggler/rpg-toolkit/npc"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/ammunition"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/backgrounds"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/conditions"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/currency"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/customization"
	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/equipment"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/features"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/npcs"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/races"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	dnd5eResources "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/resources"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/saves"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/weapons"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
)

// SessionStackSuite proves StartEncounter's new-stack branch in isolation:
// a real session.Manager (miniredis-backed, mirroring
// internal/integration/session's harness) rather than the mocked character
// repo the rest of this package's suites use, because the new stack's Join
// genuinely loads and reconstitutes a character -- a mock EXPECT() would
// only prove the call happened, not that a real sheet round-trips.
type SessionStackSuite struct {
	suite.Suite

	ctx         context.Context
	redisClient *goredis.Client
	charRepo    characterrepo.Repository
	lobbyRepo   lobbyrepo.Repository
	broker      *lobbyorch.Broker
	sessOrch    *sessionorch.Orchestrator
	orch        *lobbyorch.Orchestrator
}

func (s *SessionStackSuite) SetupTest() {
	s.ctx = context.Background()

	mr := miniredis.RunT(s.T())
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	s.T().Cleanup(func() { _ = client.Close() })
	s.redisClient = client

	charRepo, err := characterrepo.NewRedis(&characterrepo.RedisConfig{Client: client})
	s.Require().NoError(err)
	s.charRepo = charRepo

	sessOrch, err := sessionorch.New(sessionorch.Config{
		Redis: client, Characters: charRepo, TTL: 24 * time.Hour,
		PresentationIDs: idgen.NewSequential("presentation"),
	})
	s.Require().NoError(err)
	s.sessOrch = sessOrch

	s.lobbyRepo = lobbyrepo.NewInMemory()
	s.broker = lobbyorch.NewBroker()

	orch, err := lobbyorch.New(&lobbyorch.Config{
		LobbyRepo:            s.lobbyRepo,
		LobbyBroker:          s.broker,
		CharacterRepo:        s.charRepo,
		LobbyIDGenerator:     idgen.NewSequential("lobby"),
		JoinRefGenerator:     idgen.NewSequential("ref"),
		EncounterIDGenerator: idgen.NewSequential("enc"),
		SessionManager:       sessOrch.Manager,
		Dungeons:             dungeonstest.Shipped(s.T()),
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

// seedRichCharacter is seedCharacter with a funded wallet, for the one scene
// that buys something. Separate rather than a parameter on seedCharacter, so
// the twenty scenes that never trade keep saying what they mean: a level 1
// character with nothing in particular.
func (s *SessionStackSuite) seedRichCharacter(id, playerID, name string, purse currency.Money) {
	_, err := s.charRepo.Create(s.ctx, characterrepo.CreateInput{
		Character: &entities.Character{Data: &tkcharacter.Data{
			ID: id, PlayerID: playerID, Name: name, Level: 1,
			HitPoints: 10, MaxHitPoints: 10, ArmorClass: 10,
			Wallet: purse,
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

// TestStartEncounter_SDKRosterIsAuthoritative proves launch does not create a
// second roster record: the Session SDK reports the players and authored
// monsters that StartEncounter joined and spawned.
func (s *SessionStackSuite) TestStartEncounter_SDKRosterIsAuthoritative() {
	s.seedCharacter("char-alice", "alice", "Alice")
	s.seedCharacter("char-bob", "bob", "Bob")
	s.seedReadyLobby("lobby-1", "alice", "bob")

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-1",
	})
	s.Require().NoError(err)

	roster, err := s.sessOrch.Manager.Roster(s.ctx, &sdk.RosterInput{
		Session: out.EncounterID, Player: "alice",
	})
	s.Require().NoError(err)
	s.Require().NotNil(roster)

	players := make([]string, 0)
	monsters := make([]string, 0)
	for _, member := range roster.Members {
		switch member.Kind {
		case sdk.KindPlayer:
			players = append(players, member.ID)
		case sdk.KindMonster:
			monsters = append(monsters, member.ID)
		default:
			s.Failf("kind", "member %q has unspecified kind", member.ID)
		}
	}
	s.Equal([]string{"char-alice", "char-bob"}, players)
	s.Require().NotEmpty(monsters, "the authored tomb has a garrison")
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

// TestStartEncounter_GetAtlasServesTheSeamsAsTwoLines is rpg-api#899's
// acceptance at the seam it names: not PutDungeon's compiled answer, but the
// atlas a STARTED SESSION serves to a member -- what GetAtlas returns.
//
// Two authored walls, two lines, and nothing sealed. The tomb's seams are
// quarter lines: they run a quarter of a hex's width inside the column they
// cut and leave every cell either side standable, which is what makes them the
// right default and also means nothing shipped exercises sealing (the
// halved-room fixture at the authoring wire does that).
//
// The endpoints are literals for the reason every other projection literal in
// this repository is one: reading them back out of the projection under test
// would assert nothing. A side midpoint is exactly half a step from the center
// it belongs to, which is where the halves come from.
func (s *SessionStackSuite) TestStartEncounter_GetAtlasServesTheSeamsAsTwoLines() {
	s.seedCharacter("char-alice", "alice", "Alice")
	s.seedReadyLobby("lobby-1", "alice")

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-1",
	})
	s.Require().NoError(err)

	atlas, err := s.sessOrch.Manager.Atlas(s.ctx, &sdk.AtlasInput{Session: out.EncounterID, Member: "char-alice"})
	s.Require().NoError(err)

	s.Require().Len(atlas.Segments, 2, "the entrance seam and the tomb seam, one line each")
	s.Equal(sdk.AxialPointF{Q: 2, R: 7.5}, atlas.Segments[0].From)
	s.Equal(sdk.AxialPointF{Q: 6, R: -0.5}, atlas.Segments[0].To)
	s.Equal(sdk.AxialPointF{Q: 12, R: 7.5}, atlas.Segments[1].From)
	s.Equal(sdk.AxialPointF{Q: 16, R: -0.5}, atlas.Segments[1].To)
	for i, segment := range atlas.Segments {
		s.Zerof(segment.Height, "segment %d authors no height, and 0 means standard rather than flat", i)
	}

	s.Empty(atlas.Sealed, "quarter lines leave every cell they pass standable")

	// And the doors are still the ways through, on the crossings the lines
	// actually cross: one row along from where the deleted pair form had them,
	// between the same two rooms either side.
	where := map[string][2]spatial.Position{}
	for _, d := range atlas.Doorways {
		where[d.Door] = [2]spatial.Position{d.From, d.To}
	}
	s.Require().Len(where, 2)
	s.Equal(
		[2]spatial.Position{
			tkencounter.HexCellAt(tkencounter.HexesArePointyTop(), 5, 3),
			tkencounter.HexCellAt(tkencounter.HexesArePointyTop(), 6, 4),
		},
		where["reference-tomb/entrance-hall"],
		"the entrance door opens the slanted crossing [5,3]-[6,4]")
	s.Equal(
		[2]spatial.Position{
			tkencounter.HexCellAt(tkencounter.HexesArePointyTop(), 15, 5),
			tkencounter.HexCellAt(tkencounter.HexesArePointyTop(), 16, 4),
		},
		where["reference-tomb/hall-tomb"],
		"and the tomb door [15,5]-[16,4]")
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

// TestStartEncounter_DemoVendorIsPlacedAndInteractable is #903 Phase 1's own
// definition of done, proven directly against the real Manager (no mocks,
// no proto layer): the reference tomb's launch places the temporary demo
// vendor, and a seated player can Interact with it and get back its known
// stock.
func (s *SessionStackSuite) TestStartEncounter_DemoVendorIsPlacedAndInteractable() {
	s.seedCharacter("char-alice", "alice", "Alice")
	s.seedReadyLobby("lobby-1", "alice")

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-1",
	})
	s.Require().NoError(err)

	interacted, err := s.sessOrch.Manager.Interact(s.ctx, &sdk.InteractInput{
		Session: out.EncounterID, Actor: "char-alice", Target: "demo-merchant-1",
	})
	s.Require().NoError(err)

	d := interacted.Descriptor
	s.Equal("demo-merchant-1", d.TargetID)
	s.Equal("Demo Merchant", d.DisplayName)
	s.Contains(d.Capabilities, npc.CapabilityVendor)
	s.Equal(npc.CombatPolicyNonCombatant, d.CombatPolicy)

	s.Require().Len(d.Inventory, 3, "longsword, longbow, a bundle of arrows")
	byID := make(map[string]npcs.StockEntryView, len(d.Inventory))
	for _, entry := range d.Inventory {
		byID[entry.ID] = entry
	}
	s.Equal(npcs.StockModeLimited, byID[weapons.Longsword].Mode)
	s.Equal(npcs.StockModeLimited, byID[weapons.Longbow].Mode)
	s.Equal(npcs.StockModeUnlimited, byID[ammunition.Arrows20].Mode)
}

// TestStartEncounter_TradeBuysFromTheDemoVendor is rpg-project#369/#370's
// own definition of done for this wave, proven directly against the real
// Manager (no mocks): a seated player can Trade for one item and actually
// receive it -- the character's stored inventory gains it, and the vendor's
// stock line for it drops by the same amount.
func (s *SessionStackSuite) TestStartEncounter_TradeBuysFromTheDemoVendor() {
	// A buyer with coin in the purse. Selling arrived with session v0.62.0
	// (rpg-toolkit#1535) and brought server-side price validation with it:
	// a buy names its own payment and the server checks it against the
	// catalog, so a character with an empty wallet can no longer buy
	// anything and this scene has to fund one.
	price, err := equipment.PriceOf(weapons.Longsword)
	s.Require().NoError(err, "the catalog has to price what this scene buys")
	s.seedRichCharacter("char-alice", "alice", "Alice", currency.Money{Copper: price.Copper * 2})
	s.seedReadyLobby("lobby-1", "alice")

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-1",
	})
	s.Require().NoError(err)

	// The price is ASKED OF THE CATALOG rather than written out here. The
	// server computes the same number and refuses anything else
	// (ErrWrongPrice), so a literal in this test would be a second copy of
	// a price it does not own — and a silent failure the day the longsword
	// is repriced.
	traded, err := s.sessOrch.Manager.Trade(s.ctx, &sdk.TradeInput{
		Session: out.EncounterID, Actor: "char-alice", Target: "demo-merchant-1",
		Give: sdk.TradeOffer{Currency: price},
		Receive: sdk.TradeOffer{Items: []sdk.TradeItem{
			{Type: shared.EquipmentTypeWeapon, ID: weapons.Longsword, Quantity: 1},
		}},
	})
	s.Require().NoError(err)

	// The vendor's own limited stock line for the longsword is gone --
	// bought the whole quantity it had (1) -- while the longbow and arrows
	// are untouched.
	byID := make(map[string]npcs.StockEntryView, len(traded.Descriptor.Inventory))
	for _, entry := range traded.Descriptor.Inventory {
		byID[entry.ID] = entry
	}
	s.Require().Len(traded.Descriptor.Inventory, 2, "the bought longsword's line is gone, longbow and arrows remain")
	_, stillHasLongsword := byID[weapons.Longsword]
	s.False(stillHasLongsword)
	s.Equal(npcs.StockModeLimited, byID[weapons.Longbow].Mode)

	// And the buyer's own stored character record actually gained it --
	// the point of the whole verb, not just a descriptor that says so.
	got, err := s.charRepo.Get(s.ctx, characterrepo.GetInput{ID: "char-alice"})
	s.Require().NoError(err)
	s.Contains(got.Character.Data.Inventory, tkcharacter.InventoryItemData{
		Type: shared.EquipmentTypeWeapon, ID: weapons.Longsword, Quantity: 1,
	})
}

// lobbyOver is SetupTest's orchestrator over a different content registry,
// for the tests that play a dungeon the author Put rather than a shipped one.
func (s *SessionStackSuite) lobbyOver(registry dungeons.Registry) *lobbyorch.Orchestrator {
	s.T().Helper()

	orch, err := lobbyorch.New(&lobbyorch.Config{
		LobbyRepo:            s.lobbyRepo,
		LobbyBroker:          s.broker,
		CharacterRepo:        s.charRepo,
		LobbyIDGenerator:     idgen.NewSequential("lobby"),
		JoinRefGenerator:     idgen.NewSequential("ref"),
		EncounterIDGenerator: idgen.NewSequential("enc"),
		SessionManager:       s.sessOrch.Manager,
		Dungeons:             registry,
	})
	s.Require().NoError(err)

	return orch
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

	orch := s.lobbyOver(registry)

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

// TestStartEncounter_GetAtlasServesSceneryAsFloorNobodyOwns is rpg-api#898's
// second acceptance at the seam it names: not PutDungeon's compiled answer,
// but the atlas a STARTED SESSION serves to a member -- what GetAtlas returns.
//
// Scenery rides the flat Cells list and nothing else says it (wall-geometry
// design §5.1, "no change" on the wire for slice 1). So the whole claim is
// this: the strip is in Cells, and the strip is in no region's cells.
func (s *SessionStackSuite) TestStartEncounter_GetAtlasServesSceneryAsFloorNobodyOwns() {
	registry, _ := dungeonstest.Scratch(s.T())
	res, err := registry.Put(s.ctx, &dungeons.PutInput{
		Key: dungeonstest.SceneryStripKey, YAML: []byte(dungeonstest.SceneryStripYAML),
	})
	s.Require().NoError(err)
	s.Require().Empty(res.Errors, "the scenery file must compile: %v", res.Errors)

	s.seedCharacter("char-alice", "alice", "Alice")
	s.seedReadyLobby("lobby-1", "alice")

	out, err := s.lobbyOver(registry).StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-1", DungeonKey: dungeonstest.SceneryStripKey,
	})
	s.Require().NoError(err)

	atlas, err := s.sessOrch.Manager.Atlas(s.ctx, &sdk.AtlasInput{Session: out.EncounterID, Member: "char-alice"})
	s.Require().NoError(err)

	floor := make(map[spatial.Position]bool, len(atlas.Cells))
	for _, c := range atlas.Cells {
		floor[c] = true
	}
	owned := make(map[spatial.Position]bool)
	for _, r := range atlas.Regions {
		for _, c := range r.Cells {
			owned[c] = true
		}
	}
	s.Len(atlas.Cells, 12, "nine cells of vault and three of scenery")
	s.Len(owned, 9, "and only the vault's nine are owned")

	for _, at := range dungeonstest.SceneryStripSceneryCells {
		c := tkencounter.HexCellAt(tkencounter.HexesArePointyTop(), at[0], at[1])
		s.True(floor[c], "scenery cell %v reached the wire as floor", at)
		s.False(owned[c], "and no region claims it")
	}
}

func (s *SessionStackSuite) TestListDungeons_ReadsTheRegistry() {
	out, err := s.orch.ListDungeons(s.ctx, &lobbyorch.ListDungeonsInput{})
	s.Require().NoError(err)

	keys := make([]string, 0, len(out.Dungeons))
	for _, d := range out.Dungeons {
		keys = append(keys, d.Key)
	}
	// Every shipped dungeon, counted rather than spelled: the content tree
	// gained the heirloom fixture with rpg-project#368, and a literal here
	// would turn each new piece of content into a failure that says nothing
	// about the content.
	s.Contains(keys, dungeons.DefaultKey)
	s.Len(keys, dungeonstest.ShippedCount(s.T()))
}

// TestStartEncounter_FirstAdmissionPersistsCompleteLongRestOutcomes is the
// API consumer proof for Session Join's first-admission policy. Both records
// cross the real miniredis-backed character repository adapter and the real
// Session Manager; the assertions read the repository after Lobby
// StartEncounter rather than calling a rest helper or inspecting API rules.
func (s *SessionStackSuite) TestStartEncounter_FirstAdmissionPersistsCompleteLongRestOutcomes() {
	fighter, fighterAppearance := s.spentFighter("char-p1", "p1")
	barbarian, barbarianAppearance := s.spentBarbarian("char-p2", "p2")
	for _, character := range []*entities.Character{fighter, barbarian} {
		_, err := s.charRepo.Create(s.ctx, characterrepo.CreateInput{Character: character})
		s.Require().NoError(err)
	}
	s.seedReadyLobby("lobby-rest", "p1", "p2")

	_, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "p1", LobbyID: "lobby-rest",
	})
	s.Require().NoError(err)

	fighterRecord, err := s.charRepo.Get(s.ctx, characterrepo.GetInput{ID: "char-p1"})
	s.Require().NoError(err)
	gotFighter := fighterRecord.Character.Data
	s.Nil(gotFighter.ActionEconomy, "first-admission long rest clears stale action economy")
	s.Equal(36, gotFighter.HitPoints)
	s.Equal(36, gotFighter.MaxHitPoints)
	s.Equal(&saves.DeathSaveState{}, gotFighter.DeathSaveState)
	s.Equal(tkcharacter.RecoverableResourceData{
		Current: 2, Maximum: 4, ResetType: coreResources.ResetLongRest,
	}, gotFighter.Resources[dnd5eResources.HitDice], "exactly half of four spent hit dice recover")
	s.Equal(tkcharacter.SpellSlotData{Max: 3, Used: 0}, gotFighter.SpellSlots[1])

	var secondWind features.SecondWindData
	s.Require().NoError(json.Unmarshal(effectWithRef(s.T(), gotFighter.Features, refs.Features.SecondWind()), &secondWind))
	s.Equal(1, secondWind.Uses, "feature-owned Second Wind hears the normal rest event")
	s.Equal(1, secondWind.MaxUses)

	var defense conditions.FightingStyleDefenseData
	s.Require().NoError(json.Unmarshal(
		effectWithRef(s.T(), gotFighter.Conditions, refs.Conditions.FightingStyleDefense()), &defense))
	s.Equal("char-p1", defense.MemberID, "the retained passive survives")
	var opportunity conditions.OpportunityAttackConditionData
	s.Require().NoError(json.Unmarshal(
		effectWithRef(s.T(), gotFighter.Conditions, refs.Conditions.OpportunityAttack()), &opportunity))
	s.False(opportunity.UsedThisTurn, "the retained reaction meter resets")
	s.Nil(effectWithRefOrNil(gotFighter.Conditions, refs.Conditions.Prone()),
		"the temporary condition removes itself")
	s.Equal(backgrounds.Soldier, gotFighter.BackgroundID)
	s.Equal(fighter.Data.CreatedAt, gotFighter.CreatedAt)
	s.Equal(fighterAppearance, fighterRecord.Character.Data.Appearance)

	barbarianRecord, err := s.charRepo.Get(s.ctx, characterrepo.GetInput{ID: "char-p2"})
	s.Require().NoError(err)
	gotBarbarian := barbarianRecord.Character.Data
	s.Equal(45, gotBarbarian.HitPoints)
	s.Equal(45, gotBarbarian.MaxHitPoints)
	s.Equal(&saves.DeathSaveState{}, gotBarbarian.DeathSaveState)
	s.Equal(tkcharacter.RecoverableResourceData{
		Current: 3, Maximum: 4, ResetType: coreResources.ResetLongRest,
	}, gotBarbarian.Resources[dnd5eResources.HitDice], "exactly half of four hit dice are restored")
	s.Equal(tkcharacter.RecoverableResourceData{
		Current: 2, Maximum: 2, ResetType: coreResources.ResetLongRest,
	}, gotBarbarian.Resources[dnd5eResources.RageCharges], "spent Rage charges refill")

	var unarmoredDefense conditions.UnarmoredDefenseData
	s.Require().NoError(json.Unmarshal(
		effectWithRef(s.T(), gotBarbarian.Conditions, refs.Conditions.UnarmoredDefense()), &unarmoredDefense))
	s.Equal("char-p2", unarmoredDefense.MemberID, "the Barbarian passive survives")
	s.Nil(effectWithRefOrNil(gotBarbarian.Conditions, refs.Conditions.Raging()),
		"Raging ends on the normal long rest")
	s.Equal(backgrounds.Outlander, gotBarbarian.BackgroundID)
	s.Equal(barbarian.Data.CreatedAt, gotBarbarian.CreatedAt)
	s.Equal(barbarianAppearance, barbarianRecord.Character.Data.Appearance)
}

// TestStartEncounter_StartSessionFailureLeavesCharacterUntouched proves the
// launch ordering itself. StartSession is forced to fail through a real
// Manager's repository seam. The character bytes, optimistic version, and
// adapter save count must remain unchanged because Join has not begun. This is
// discriminating against the retired API loop, which wrote the rested record
// before it attempted StartSession.
func (s *SessionStackSuite) TestStartEncounter_StartSessionFailureLeavesCharacterUntouched() {
	fighter, _ := s.spentFighter("char-p1", "p1")
	_, err := s.charRepo.Create(s.ctx, characterrepo.CreateInput{Character: fighter})
	s.Require().NoError(err)
	s.seedReadyLobby("lobby-order", "p1")

	beforeRecord, err := s.charRepo.Get(s.ctx, characterrepo.GetInput{ID: "char-p1"})
	s.Require().NoError(err)
	beforeBytes, err := s.redisClient.Get(s.ctx, "character:char-p1").Bytes()
	s.Require().NoError(err)

	countedCharacters := &countingCharacterRepository{Repository: s.charRepo}
	failedStores := &failingStartRepositories{
		SessionRepository:   sessionorch.NewSessionRepository(s.redisClient, 24*time.Hour),
		EncounterRepository: sessionorch.NewEncounterRepository(s.redisClient, 24*time.Hour),
	}
	manager, err := sdk.NewManager(&sdk.Config{
		PresentationIDs: idgen.NewSequential("presentation"),
		Sessions:        failedStores, Encounters: failedStores,
		Characters: sessionorch.NewCharacterRepository(countedCharacters),
		Events:     sdk.DiscardEvents{}, Dice: &dice.CryptoRoller{}, TurnDriver: sdk.Behavior(),
	})
	s.Require().NoError(err)
	orch, err := lobbyorch.New(&lobbyorch.Config{
		LobbyRepo: s.lobbyRepo, LobbyBroker: s.broker, CharacterRepo: countedCharacters,
		LobbyIDGenerator: idgen.NewSequential("lobby"), JoinRefGenerator: idgen.NewSequential("ref"),
		EncounterIDGenerator: idgen.NewSequential("enc"), SessionManager: manager,
		Dungeons: dungeonstest.Shipped(s.T()),
	})
	s.Require().NoError(err)

	_, err = orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "p1", LobbyID: "lobby-order",
	})
	s.Require().ErrorIs(err, errStartRepository)

	afterRecord, err := s.charRepo.Get(s.ctx, characterrepo.GetInput{ID: "char-p1"})
	s.Require().NoError(err)
	afterBytes, err := s.redisClient.Get(s.ctx, "character:char-p1").Bytes()
	s.Require().NoError(err)
	s.Equal(beforeBytes, afterBytes, "StartSession failed before Join, so stored bytes cannot change")
	s.Equal(beforeRecord.Version, afterRecord.Version, "the repository version cannot advance")
	s.Zero(countedCharacters.updates, "the character adapter cannot save before Join")
}

func (s *SessionStackSuite) spentFighter(id, playerID string) (*entities.Character, *customization.Appearance) {
	secondWind, err := json.Marshal(features.SecondWindData{
		Ref: refs.Features.SecondWind(), ID: id + "-second-wind", Name: "Second Wind",
		Level: 4, CharacterID: id, Uses: 0, MaxUses: 1,
	})
	s.Require().NoError(err)
	defense, err := json.Marshal(conditions.FightingStyleDefenseData{
		Ref: refs.Conditions.FightingStyleDefense(), MemberID: id,
	})
	s.Require().NoError(err)
	opportunity, err := (&conditions.OpportunityAttackCondition{
		MemberID: id, UsedThisTurn: true,
	}).ToJSON()
	s.Require().NoError(err)
	prone, err := conditions.NewProneCondition(id).ToJSON()
	s.Require().NoError(err)

	createdAt := time.Date(2026, time.August, 14, 9, 30, 0, 0, time.UTC)
	color := uint32(0x8A4B2A)
	roughness := float32(0.35)
	appearance := &customization.Appearance{Hair: &customization.HairCustomization{
		Scalp:     &customization.StyleSelection{Kind: customization.StyleSelectionStyle, StyleRef: "dnd5e:hair:short"},
		ColorSRGB: &color, Roughness: &roughness,
	}}
	return &entities.Character{Data: &tkcharacter.Data{
		ID: id, PlayerID: playerID, Name: "Spent Fighter",
		Level: 4, ProficiencyBonus: 2, RaceID: races.Human, ClassID: classes.Fighter,
		BackgroundID: backgrounds.Soldier,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 16, abilities.DEX: 14, abilities.CON: 14,
			abilities.INT: 10, abilities.WIS: 12, abilities.CHA: 8,
		},
		HitPoints: 7, MaxHitPoints: 36, ArmorClass: 16,
		ActionEconomy: &tkcharacter.ActionEconomyData{
			TurnNumber: 1, ActionsRemaining: 0, BonusActionsRemaining: 0,
			ReactionsRemaining: 1, MovementRemaining: 10,
			Granted: map[tkcharacter.GrantedActionKey]int{tkcharacter.GrantedAttacks: 1},
		},
		DeathSaveState: &saves.DeathSaveState{Successes: 1, Failures: 2, Stabilized: true, Dead: true},
		SpellSlots:     map[int]tkcharacter.SpellSlotData{1: {Max: 3, Used: 3}},
		Resources: map[coreResources.ResourceKey]tkcharacter.RecoverableResourceData{
			dnd5eResources.HitDice: {Current: 0, Maximum: 4, ResetType: coreResources.ResetLongRest},
		},
		Features:   []json.RawMessage{secondWind},
		Conditions: []json.RawMessage{defense, opportunity, prone},
		CreatedAt:  createdAt,
		Appearance: appearance,
	}}, appearance
}

func (s *SessionStackSuite) spentBarbarian(id, playerID string) (*entities.Character, *customization.Appearance) {
	unarmoredDefense, err := json.Marshal(conditions.UnarmoredDefenseData{
		Ref: refs.Conditions.UnarmoredDefense(), Type: string(conditions.UnarmoredDefenseBarbarian),
		MemberID: id, Source: refs.Classes.Barbarian().String(),
	})
	s.Require().NoError(err)
	raging, err := json.Marshal(conditions.RagingData{
		Ref: refs.Conditions.Raging(), CharacterID: id, DamageBonus: 2, Level: 4,
		Source: refs.Features.Rage().String(), SawTurnEnd: true, TurnsActive: 2,
	})
	s.Require().NoError(err)

	createdAt := time.Date(2026, time.August, 15, 10, 45, 0, 0, time.UTC)
	color := uint32(0x24150D)
	roughness := float32(0.8)
	appearance := &customization.Appearance{Hair: &customization.HairCustomization{
		Scalp: &customization.StyleSelection{Kind: customization.StyleSelectionNone},
		FacialHair: &customization.StyleSelection{
			Kind: customization.StyleSelectionStyle, StyleRef: "dnd5e:facial-hair:braided-beard",
		},
		ColorSRGB: &color, Roughness: &roughness,
	}}
	return &entities.Character{Data: &tkcharacter.Data{
		ID: id, PlayerID: playerID, Name: "Spent Barbarian",
		Level: 4, ProficiencyBonus: 2, RaceID: races.Dwarf, ClassID: classes.Barbarian,
		BackgroundID: backgrounds.Outlander,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 16, abilities.DEX: 14, abilities.CON: 16,
			abilities.INT: 8, abilities.WIS: 12, abilities.CHA: 10,
		},
		HitPoints: 0, MaxHitPoints: 45, ArmorClass: 15,
		DeathSaveState: &saves.DeathSaveState{Failures: 3, Dead: true},
		Resources: map[coreResources.ResourceKey]tkcharacter.RecoverableResourceData{
			dnd5eResources.HitDice:     {Current: 1, Maximum: 4, ResetType: coreResources.ResetLongRest},
			dnd5eResources.RageCharges: {Current: 0, Maximum: 2, ResetType: coreResources.ResetLongRest},
		},
		Conditions: []json.RawMessage{unarmoredDefense, raging},
		CreatedAt:  createdAt,
		Appearance: appearance,
	}}, appearance
}

func effectWithRef(t *testing.T, blobs []json.RawMessage, want *core.Ref) json.RawMessage {
	t.Helper()
	if got := effectWithRefOrNil(blobs, want); got != nil {
		return got
	}
	t.Fatalf("effect %s not found", want.String())
	return nil
}

func effectWithRefOrNil(blobs []json.RawMessage, want *core.Ref) json.RawMessage {
	for _, raw := range blobs {
		var envelope struct {
			Ref core.Ref `json:"ref"`
		}
		if json.Unmarshal(raw, &envelope) == nil && envelope.Ref.Equals(want) {
			return raw
		}
	}
	return nil
}

var errStartRepository = errors.New("start repository unavailable")

type failingStartRepositories struct {
	sdk.SessionRepository
	sdk.EncounterRepository
}

func (*failingStartRepositories) SaveSession(context.Context, *sdk.SessionData) error {
	return errStartRepository
}

func (*failingStartRepositories) SaveEncounter(context.Context, string, *tkencounter.EncounterData) error {
	return errStartRepository
}

type countingCharacterRepository struct {
	characterrepo.Repository
	updates int
}

func (r *countingCharacterRepository) Update(
	ctx context.Context, input characterrepo.UpdateInput,
) (*characterrepo.UpdateOutput, error) {
	r.updates++
	return r.Repository.Update(ctx, input)
}

// TestStartEncounter_ASpawnedMonsterCarriesTheRecordItWasGiven is the
// launch's half of path 2 (rpg-project#372): the author declares an intel
// record and places it in a monster with `holds:`, and the monster the lobby
// spawns carries it.
//
// PROVEN BY LOOTING, not by reading the input back. Who carries intel never
// reaches a wire, an atlas or a beat (slice 2 design P3) -- that is the whole
// point of the fact -- so the only honest question to ask is the one a player
// asks: loot the body and see whether the door arrives.
func (s *SessionStackSuite) TestStartEncounter_ASpawnedMonsterCarriesTheRecordItWasGiven() {
	after := s.lootTheAuthoredCaptain(dungeonstest.HeirloomVaultYAML)
	s.Len(after.Doorways, 1, "what the record reveals reached the looter")
}

// TestStartEncounter_AMonsterHoldingNothingCarriesNothing is the control the
// test above needs, and a separate method rather than a subtest so each runs
// on its own SetupTest: without it, "the looter's map gained a doorway" could
// have been the launch revealing the vault to anybody who looted anything.
//
// The ONE difference is the `holds:` list, struck out of the same file. The
// record itself stays declared, which is the sharper control: a dungeon can
// author knowledge nobody carries, and that has to reveal nothing.
func (s *SessionStackSuite) TestStartEncounter_AMonsterHoldingNothingCarriesNothing() {
	empty := strings.Replace(dungeonstest.HeirloomVaultYAML, ", holds: [vault-map]", "", 1)
	s.Require().NotEqual(dungeonstest.HeirloomVaultYAML, empty,
		"the fixture's holds line must be where this test expects it")

	after := s.lootTheAuthoredCaptain(empty)
	s.Empty(after.Doorways, "a body holding nothing transfers nothing")
}

// lootTheAuthoredCaptain plays one authored dungeon through the real launch,
// downs the garrison it spawned, loots the captain, and returns the looter's
// map afterwards.
//
// # Why this builds its own session stack
//
// The captain spawns ALIVE, in sight of a party that has already joined, so
// a fight forms inside StartEncounter and the monster takes driven turns
// against a level-3 fighter. On the suite's own stack that is REAL dice, and
// the scene became a coin toss: caught here as a genuine flake, where a run
// in which alice went down closed the encounter and Loot refused with
// ErrClosed before the scene's actual question was asked.
//
// The roller below always answers the LOWEST face, so no attack ever lands.
// That is the honest fix rather than a retry: this test is about whether the
// launch forwards a knowledge link, and who wins a fight is not its
// question. The fight itself is proven with the maximum-face roller in
// internal/integration/session.
func (s *SessionStackSuite) lootTheAuthoredCaptain(authored string) *sdk.Atlas {
	s.T().Helper()

	registry, _ := dungeonstest.Scratch(s.T())
	res, err := registry.Put(s.ctx, &dungeons.PutInput{
		Key: dungeonstest.HeirloomVaultKey, YAML: []byte(authored),
	})
	s.Require().NoError(err)
	s.Require().Empty(res.Errors, "the heirloom fixture must compile: %v", res.Errors)

	sessOrch, err := sessionorch.New(sessionorch.Config{
		Redis: s.redisClient, Characters: s.charRepo, TTL: 24 * time.Hour,
		PresentationIDs: idgen.NewSequential("presentation"), Dice: alwaysTheLowestFace{},
	})
	s.Require().NoError(err)

	orch, err := lobbyorch.New(&lobbyorch.Config{
		LobbyRepo:            s.lobbyRepo,
		LobbyBroker:          s.broker,
		CharacterRepo:        s.charRepo,
		LobbyIDGenerator:     idgen.NewSequential("lobby"),
		JoinRefGenerator:     idgen.NewSequential("ref"),
		EncounterIDGenerator: idgen.NewSequential("enc"),
		SessionManager:       sessOrch.Manager,
		Dungeons:             registry,
	})
	s.Require().NoError(err)

	s.seedCharacter("char-alice", "alice", "Alice")
	s.seedReadyLobby("lobby-1", "alice")

	out, err := orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-1", DungeonKey: dungeonstest.HeirloomVaultKey,
	})
	s.Require().NoError(err)

	before, err := sessOrch.Manager.Atlas(s.ctx,
		&sdk.AtlasInput{Session: out.EncounterID, Member: "char-alice"})
	s.Require().NoError(err)
	s.Require().Empty(before.Doorways, "nobody has searched: the way in is on no map")

	// A body is a body because its sheet says so: the session's standing
	// seam reads these, and Loot refuses a member still on their feet.
	sessions := sessionorch.NewSessionRepository(s.redisClient, time.Hour)
	stored, err := sessions.GetSession(s.ctx, out.EncounterID)
	s.Require().NoError(err)
	s.Require().NotEmpty(stored.NPCs, "the launch must have spawned the authored garrison")
	for i := range stored.NPCs {
		stored.NPCs[i].HitPoints = 0
	}
	s.Require().NoError(sessions.SaveSession(s.ctx, stored))

	_, err = sessOrch.Manager.Loot(s.ctx, &sdk.LootInput{
		Session: out.EncounterID, Member: "char-alice",
		Target: dungeonstest.HeirloomCaptainMemberID, Range: 4,
	})
	s.Require().NoError(err)

	after, err := sessOrch.Manager.Atlas(s.ctx,
		&sdk.AtlasInput{Session: out.EncounterID, Member: "char-alice"})
	s.Require().NoError(err)

	return after
}

// alwaysTheLowestFace is a Roller that never lets an attack land, so a scene
// about something other than combat is not decided by dice.
type alwaysTheLowestFace struct{}

func (alwaysTheLowestFace) Roll(_ context.Context, _ int) (int, error) { return 1, nil }

// TestStartEncounter_ACharacterSharingAMonstersIdIsRefusedBeforeAnyWrite is
// the launch's half of one-id-one-member (rpg-project#375): a named
// placement joins under its own id now, so the heirloom's captain is the
// member `captain` — and a character who happens to be called that cannot
// share a run with him. Refused BEFORE anything is written, naming both,
// rather than at the captain's Spawn halfway through, where the session
// would report the duplicate as "no such member".
func (s *SessionStackSuite) TestStartEncounter_ACharacterSharingAMonstersIdIsRefusedBeforeAnyWrite() {
	s.seedCharacter(dungeonstest.HeirloomCaptainMemberID, "cap", "Cap")
	s.Require().NoError(s.lobbyRepo.Save(s.ctx, &lobbyrepo.Data{
		ID: "lobby-1", HostPlayerID: "cap", Status: lobbyrepo.StatusWaiting,
		Members: map[string]*lobbyrepo.Member{"cap": {
			PlayerID: "cap", CharacterID: dungeonstest.HeirloomCaptainMemberID, IsHost: true, IsReady: true,
		}},
		MemberOrder: []string{"cap"},
	}))

	_, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "cap", LobbyID: "lobby-1", DungeonKey: "reference-tomb-heirloom",
	})
	s.Require().Error(err)
	s.Contains(err.Error(), `"captain" is claimed twice`)
	s.Contains(err.Error(), `character "captain" of player "cap"`, "one claimant, as the lobby knows it")
	s.Contains(err.Error(), `monster "captain" (dnd5e:monsters:skeleton-captain)`, "the other, as the dungeon knows it")

	lobbyData, err := s.lobbyRepo.Get(s.ctx, "lobby-1")
	s.Require().NoError(err)
	s.Equal(lobbyrepo.StatusWaiting, lobbyData.Status, "nothing was written: the lobby is where it was")
	s.Empty(lobbyData.EncounterID)
}

// TestStartEncounter_TheHeirloomsCaptainSpawnsUnderHisAuthoredId is the
// positive half, at the seam that matters: the launch spawns a named
// placement under the author's id, so the file's word for him is the run's.
func (s *SessionStackSuite) TestStartEncounter_TheHeirloomsCaptainSpawnsUnderHisAuthoredId() {
	s.seedCharacter("char-alice", "alice", "Alice")
	s.seedReadyLobby("lobby-1", "alice")

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-1", DungeonKey: "reference-tomb-heirloom",
	})
	s.Require().NoError(err)

	_, err = s.sessOrch.Manager.Turn(s.ctx, &sdk.TurnInput{
		Session: out.EncounterID, Member: dungeonstest.HeirloomCaptainPlacementID,
	})
	s.Require().NoError(err, "the captain is a member under the id the author gave him")
	_, err = s.sessOrch.Manager.Turn(s.ctx, &sdk.TurnInput{
		Session: out.EncounterID, Member: "skeleton-captain-1",
	})
	s.Require().ErrorIs(err, sdk.ErrNoMember, "and under nothing else")
}

// TestStartEncounter_TheCampLaunchesWithItsChiefAsTheMind is the hold-out's
// launch (rpg-project#375): the raider camp starts, every monster enters the
// faction the author placed it in, and the chief -- the faction's MIND --
// is a member under the id the file names him by. The composition refuses a
// mind that arrives in any faction but its own, so this passing at all is
// the proof that the launch forwards `faction` verbatim; the roster is where
// the side shows, since a placement row says nothing about sides.
func (s *SessionStackSuite) TestStartEncounter_TheCampLaunchesWithItsChiefAsTheMind() {
	s.seedCharacter("char-alice", "alice", "Alice")
	s.seedReadyLobby("lobby-1", "alice")

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-1", DungeonKey: "reference-raider-camp",
	})
	s.Require().NoError(err, "the camp launches: its mind entered its own faction")

	roster, err := s.sessOrch.Manager.Roster(s.ctx, &sdk.RosterInput{
		Session: out.EncounterID, Player: "alice",
	})
	s.Require().NoError(err)
	factions := map[string]string{}
	for _, m := range roster.Members {
		factions[m.ID] = m.Faction
	}
	s.Equal("raiders", factions["chief"], "the chief is in the raiders, under the id the file names as their mind")
	s.Equal("raiders", factions["scout"], "and so is the scout")
	s.Equal("party", factions["char-alice"], "the party is the party")

	_, err = s.sessOrch.Manager.Turn(s.ctx, &sdk.TurnInput{Session: out.EncounterID, Member: "chief"})
	s.Require().NoError(err, "the chief is a member of the run")
}
