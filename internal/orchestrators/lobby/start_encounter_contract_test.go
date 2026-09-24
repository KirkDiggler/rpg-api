package lobby_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
)

// StartContractSuite pins StartEncounter's exact launch contract: the compiled
// world and resolved key it hands StartSession, the ordered SDK verbs it calls,
// the exact input each verb receives, and the save-then-publish ordering of the
// lobby record and the EncounterStarted event.
//
// It embeds LobbySuite for the shared fixture — the generated SDK mock, the
// dungeon registry mock, the API-owned in-memory lobby repository, the broker
// and the deterministic generators — and adds no production behavior of its
// own. Both mocks are controller-isolated per test, so an SDK or registry call
// a case did not expect FAILS that case.
type StartContractSuite struct {
	LobbySuite
}

func TestStartContractSuite(t *testing.T) {
	suite.Run(t, new(StartContractSuite))
}

// contractLobbyID is the lobby id every contract launch seeds.
const contractLobbyID = "lobby-contract"

// contractVendorMemberID mirrors the launch's own temporary demo-vendor id.
// Repeated here as a literal rather than read from production so the
// expectation is a claim about the wire, not a restatement of the constant.
const contractVendorMemberID = "demo-merchant-1"

// The two guards in the contract dungeon. Literal refs, never executed by a
// toolkit loader: this test never spawns into a real session, so the refs exist
// only to make the launch's per-monster forwarding distinguishable.
const (
	contractGuardARef = "dnd5e:monsters:contract-guard-a"
	contractGuardBRef = "dnd5e:monsters:contract-guard-b"
)

// contractTable and contractTemper are canonical NONZERO values copied from the
// pinned toolkit's own table tests — encounter's answer_test.go goblin row
// ({Weight, Say, Fact}) and worldtime_test.go's faction mix. They are handed to
// Spawn verbatim: this launch owns no table or temper conversion, so the test
// asserts the authored values arrive whole, not that they cause any behavior.
// Nothing here runs a provider constructor.
var contractTable = tkencounter.Table{
	tkencounter.AnswerIntimidated: {{Weight: 1, Say: "Fine.", Fact: "guards-alerted"}},
}

var contractTemper = tkencounter.Temper{
	Mix: map[string]int{"coward": 1, "soldier": 2, "aggressive": 1},
	Profiles: map[string]tkencounter.TemperProfile{
		"coward":     {Attack: 50, Toward: 50, Away: 300, Flee: 300, Hold: 100},
		"soldier":    {Attack: 100, Toward: 100, Away: 100, Flee: 100, Hold: 100},
		"aggressive": {Attack: 300, Toward: 300, Away: 25, Flee: 25, Hold: 50},
	},
}

// readyContractLobby is the settled, all-ready party every contract launch
// starts from: alice hosts and is ready, bob is ready, and MemberOrder is
// deliberately bob-then-alice so the join order is distinguishable from any
// map iteration order.
func readyContractLobby() *lobbyrepo.Data {
	return &lobbyrepo.Data{
		ID: "lobby-contract", JoinRef: "ref-contract", HostPlayerID: "alice",
		Status: lobbyrepo.StatusWaiting,
		Members: map[string]*lobbyrepo.Member{
			"alice": {PlayerID: "alice", CharacterID: "char-a", IsHost: true, IsReady: true},
			"bob":   {PlayerID: "bob", CharacterID: "char-b", IsReady: true},
		},
		MemberOrder: []string{"bob", "alice"},
	}
}

// contractEntry is the canonical compiled dungeon every contract case launches
// from. The world is an opaque, empty EncounterData: StartSession takes the very
// pointer the registry handed the launch (asserted with Same), and nothing here
// runs the toolkit's compiler or loads shipped YAML. The two monsters carry
// distinct literal refs and cells only so the launch's per-monster forwarding is
// distinguishable; neither ref needs to be executable because this test never
// spawns into a real session.
func contractEntry(key string) *dungeons.Entry {
	return &dungeons.Entry{
		Key:   key,
		Atlas: &sdk.Atlas{},
		Dungeon: &sessionworld.Dungeon{
			Key:        key,
			Name:       "Contract Hall",
			World:      &tkencounter.EncounterData{},
			PartySeats: []spatial.Position{{X: 7, Y: -3}, {X: -2, Y: 5}},
			Monsters: []sessionworld.Monster{
				{
					MemberID: "guard-a",
					Ref:      contractGuardARef,
					At:       spatial.Position{X: 1, Y: 2},
					Holds:    []string{"dungeon/letter"},
					Actions:  []string{"weapon-b", "weapon-a"},
					Faction:  "guards",
					Intimidate: []tkencounter.CheckApproach{
						{Ability: "intimidation", DC: 12},
						{Ability: "str", Tool: "dnd5e:item:brass-knuckles", DC: 15},
					},
					Persuade: []tkencounter.CheckApproach{{Ability: "cha", DC: 17}},
					Arrives:  tkencounter.TriggerRound{Round: 6},
					Table:    contractTable,
					Temper:   contractTemper,
				},
				{
					MemberID: "guard-b",
					Ref:      contractGuardBRef,
					At:       spatial.Position{X: -4, Y: 0},
				},
			},
		},
	}
}

// contractMonsterSpawns is the exact Spawn sequence the launch must send, in
// authored order: guard-a first, carrying every optional field the fixture
// authored, then guard-b with every optional field nil. Session is filled from
// the id the launch generated — which is why the sequence is assembled per call
// rather than written into the gomock expectation up front.
//
// The social approaches and the arrival are spelled here as SDK values, NOT run
// through the production converters: an expectation built by the code under test
// would agree with a broken conversion by sharing it.
func contractMonsterSpawns(sessionID string) []*sdk.SpawnInput {
	return []*sdk.SpawnInput{
		{
			Session:  sessionID,
			ID:       "guard-a",
			Ref:      contractGuardARef,
			Position: spatial.Position{X: 1, Y: 2},
			Holds:    []string{"dungeon/letter"},
			Actions:  []string{"weapon-b", "weapon-a"},
			Intimidate: []sdk.DoorApproach{
				{Ability: "intimidation", DC: 12},
				{Ability: "str", Tool: "dnd5e:item:brass-knuckles", DC: 15},
			},
			Persuade: []sdk.DoorApproach{{Ability: "cha", DC: 17}},
			Table:    contractTable,
			Temper:   contractTemper,
			Faction:  "guards",
			Arrives:  sdk.ArrivesAtRound{Round: 6},
		},
		{
			Session:  sessionID,
			ID:       "guard-b",
			Ref:      contractGuardBRef,
			Position: spatial.Position{X: -4, Y: 0},
		},
	}
}

// contractJoins is the exact party arrival: lobby MemberOrder is bob then alice,
// so bob's char-b takes seat 0 ({7,-3}) and alice's char-a seat 1 ({-2,5}).
// Session is filled from the generated id.
var contractJoins = []sdk.JoinInput{
	{Member: "char-b", Position: spatial.Position{X: 7, Y: -3}},
	{Member: "char-a", Position: spatial.Position{X: -2, Y: 5}},
}

// armContractLaunch arms the registry lookup and every SDK verb in the exact
// order the launch must call them, and returns a pointer the StartSession
// callback writes the generated session id into. expectSpawns supplies the
// exact Spawn sequence to expect (one per entry monster, authored order), so a
// case that authors different monsters hands in its own literal sequence rather
// than reusing a converter this test would then be testing against itself.
func (s *StartContractSuite) armContractLaunch(
	entry *dungeons.Entry, key string, withVendor bool,
	expectSpawns func(sessionID string) []*sdk.SpawnInput,
) *string {
	sessionID := new(string)

	calls := []any{
		// 1. The registry is asked for the RESOLVED key, and its entry is what
		// the launch builds the session from: the compiled world by pointer and
		// the key it was stored under.
		s.registry.EXPECT().Get(s.ctx, key).Return(entry, nil),
		// 2. StartSession receives the API-minted session id as both the session
		// and its encounter, the resolved key, and the very world pointer the
		// entry holds.
		s.manager.EXPECT().StartSession(s.ctx, gomock.Any()).DoAndReturn(
			func(_ context.Context, in *sdk.StartSessionInput) (*sdk.StartSessionOutput, error) {
				s.Same(entry.Dungeon.World, in.World,
					"StartSession gets the compiled world the registry entry holds, not a copy")
				s.Equal(entry.Key, in.Dungeon, "and the key this launch resolved")
				s.NotEmpty(in.Session, "the API mints the session id itself; the SDK's answer does not name it")
				s.Equal(in.Session, in.Encounter, "one generated id names both the session and its encounter")
				*sessionID = in.Session
				return &sdk.StartSessionOutput{}, nil
			},
		),
	}

	// 3. Every monster is spawned in authored order, before any party member
	// arrives. The whole input is compared to the literal expected input.
	for i := range entry.Dungeon.Monsters {
		index := i
		calls = append(calls, s.manager.EXPECT().Spawn(s.ctx, gomock.Any()).DoAndReturn(
			func(_ context.Context, in *sdk.SpawnInput) (*sdk.SpawnOutput, error) {
				s.Equal(expectSpawns(*sessionID)[index], in,
					"monster %d's Spawn input must be exactly what the entry authored", index)
				return &sdk.SpawnOutput{}, nil
			},
		))
	}

	// 4. The party joins last, in lobby MemberOrder, on the authored seats.
	for i := range contractJoins {
		index := i
		calls = append(calls, s.manager.EXPECT().Join(s.ctx, gomock.Any()).DoAndReturn(
			func(_ context.Context, in *sdk.JoinInput) (*sdk.JoinOutput, error) {
				expected := contractJoins[index]
				expected.Session = *sessionID
				s.Equal(&expected, in,
					"join %d must be the authored member on the authored seat", index)
				return &sdk.JoinOutput{}, nil
			},
		))
	}

	// 5. The reference tomb additionally places the temporary demo vendor,
	// AFTER the joins; any other key places nobody, and an unexpected PlaceNPC
	// call fails the case because no expectation is armed.
	if withVendor {
		calls = append(calls, s.manager.EXPECT().PlaceNPC(s.ctx, gomock.Any()).DoAndReturn(
			func(_ context.Context, in *sdk.PlaceNPCInput) (*sdk.PlaceNPCOutput, error) {
				s.Equal(*sessionID, in.Session, "the vendor joins the session this launch started")
				s.Equal(contractVendorMemberID, in.Member)
				s.Equal(spatial.Position{X: 8, Y: -3}, in.Position,
					"the vendor stands one hex east of the party's first seat")
				s.NotNil(in.NPC, "the vendor's payload is supplied, not resolved from a ref")
				return &sdk.PlaceNPCOutput{}, nil
			},
		))
	}

	gomock.InOrder(calls...)
	return sessionID
}

// TestStartEncounter_CustomKey_PinsLaunchInputsAndSaveThenPublish is the contract
// for a named dungeon: every SDK verb gets the exact input the fixture authored,
// in the order the launch promises, and the lobby record is saved as Started
// before exactly one EncounterStarted event is published.
func (s *StartContractSuite) TestStartEncounter_CustomKey_PinsLaunchInputsAndSaveThenPublish() {
	const key = "custom-dungeon"

	entry := contractEntry(key)

	// The lobby is seeded through the underlying repository BEFORE it is
	// wrapped, so the only Save the wrapper counts is the launch's own.
	s.seedLobby(readyContractLobby())
	observed := &observedLobbyRepository{Repository: s.lobbyRepo}
	orch := s.newOrchestratorWithLobbyRepo(observed)

	sub, err := s.broker.Subscribe(contractLobbyID)
	s.Require().NoError(err)
	defer func() { _ = sub.Close() }()

	sessionID := s.armContractLaunch(entry, key, false, contractMonsterSpawns)

	observed.beforeSave = func(data *lobbyrepo.Data) {
		// The record is already Started with the generated id when it is
		// written, and the event channel is still empty at save time.
		s.Equal(lobbyrepo.StatusStarted, data.Status)
		s.Equal(*sessionID, data.EncounterID)
		select {
		case evt := <-sub.Events():
			s.Failf("event published before the lobby was saved", "%+v", evt)
		default:
		}
	}

	out, err := orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: contractLobbyID, DungeonKey: lobbyorch.DungeonKey(key),
	})
	s.Require().NoError(err)
	s.Equal(*sessionID, out.EncounterID, "the output carries the id the launch minted")

	s.Equal(1, observed.saveCalls, "the launch saves the lobby exactly once")
	stored, err := s.lobbyRepo.Get(s.ctx, contractLobbyID)
	s.Require().NoError(err)
	s.Equal(lobbyrepo.StatusStarted, stored.Status)
	s.Equal(*sessionID, stored.EncounterID, "the persisted record names the started encounter")

	// Exactly one event, carrying the same id, and nothing after it. Checked
	// synchronously against the buffered channel: Publish is synchronous
	// fan-out, so an empty channel right now is proof, with no sleep.
	select {
	case event := <-sub.Events():
		s.Require().Equal(lobbyorch.EventKindEncounterStarted, event.Kind)
		s.Require().NotNil(event.EncounterStarted)
		s.Equal(*sessionID, event.EncounterStarted.EncounterID)
	default:
		s.FailNow("successful launch did not publish EncounterStarted")
	}
	select {
	case event := <-sub.Events():
		s.Failf("unexpected extra event", "%+v", event)
	default:
	}
}

// TestStartEncounter_ExplicitDefaultKey_ResolvesTheTombAndPlacesTheVendor pins
// the explicit default half of key resolution: naming the default key still
// resolves to the same tomb, records the same resolved key, and takes the
// default-only demo-vendor placement AFTER the party has joined.
func (s *StartContractSuite) TestStartEncounter_ExplicitDefaultKey_ResolvesTheTombAndPlacesTheVendor() {
	entry := contractEntry(dungeons.DefaultKey)

	s.seedLobby(readyContractLobby())
	sessionID := s.armContractLaunch(entry, dungeons.DefaultKey, true, contractMonsterSpawns)

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: contractLobbyID,
		DungeonKey: lobbyorch.DungeonKey(dungeons.DefaultKey),
	})
	s.Require().NoError(err)
	s.Equal(*sessionID, out.EncounterID)
}

// TestStartEncounter_OmittedKey_ResolvesTheTombAndPlacesTheVendor pins the
// empty-request path: no key at all resolves to the default tomb, and the
// RESOLVED key — not the empty request — is what StartSession records.
func (s *StartContractSuite) TestStartEncounter_OmittedKey_ResolvesTheTombAndPlacesTheVendor() {
	entry := contractEntry(dungeons.DefaultKey)

	s.seedLobby(readyContractLobby())
	sessionID := s.armContractLaunch(entry, dungeons.DefaultKey, true, contractMonsterSpawns)

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: contractLobbyID,
	})
	s.Require().NoError(err)
	s.Equal(*sessionID, out.EncounterID)
}

// TestStartEncounter_ExplicitlyEmptySlices_ArriveEmptyNotNil pins the other side
// of the nil/empty distinction: an author's explicitly empty list is forwarded
// as an explicitly empty list, never collapsed to nil. The SDK reads nil as
// "derive one from the stat block", so the two are different claims. Every
// optional slice on the first monster is authored empty here; the second keeps
// the nil case covered by the other cases in this file.
func (s *StartContractSuite) TestStartEncounter_ExplicitlyEmptySlices_ArriveEmptyNotNil() {
	const key = "empty-slices"

	entry := contractEntry(key)
	entry.Dungeon.Monsters[0].Holds = []string{}
	entry.Dungeon.Monsters[0].Actions = []string{}
	entry.Dungeon.Monsters[0].Intimidate = []tkencounter.CheckApproach{}
	entry.Dungeon.Monsters[0].Persuade = []tkencounter.CheckApproach{}

	s.seedLobby(readyContractLobby())
	expectSpawns := func(sessionID string) []*sdk.SpawnInput {
		spawns := contractMonsterSpawns(sessionID)
		spawns[0].Holds = []string{}
		spawns[0].Actions = []string{}
		spawns[0].Intimidate = []sdk.DoorApproach{}
		spawns[0].Persuade = []sdk.DoorApproach{}
		return spawns
	}
	sessionID := s.armContractLaunch(entry, key, false, expectSpawns)

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: contractLobbyID, DungeonKey: lobbyorch.DungeonKey(key),
	})
	s.Require().NoError(err)
	s.Equal(*sessionID, out.EncounterID)
}
