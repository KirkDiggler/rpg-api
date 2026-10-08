package lobby_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	tkdungeonspec "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter/dungeonspec"
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
// It composes lobbyFixture for the shared dependencies — the generated SDK
// mock, the dungeon registry mock, the API-owned in-memory lobby repository,
// the broker and the deterministic generators — and adds no production
// behavior of its own. The fixture declares no Test* method, so
// TestStartContractSuite runs exactly this file's launch-contract cases and the
// failure cases in start_encounter_failure_test.go, never LobbySuite's
// lifecycle cases. Both mocks are controller-isolated per test, so an SDK or
// registry call a case did not expect FAILS that case.
type StartContractSuite struct {
	lobbyFixture
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
// from. The spec is an opaque, empty dungeonspec.Compiled: Launch takes the very
// pointer the registry handed the launch (asserted with Same) — this host
// re-projects nothing from it — and nothing here runs the toolkit's compiler.
// PartySeats is read only for the demo vendor's cell.
func contractEntry(key string) *dungeons.Entry {
	return &dungeons.Entry{
		Key:   key,
		Atlas: &sdk.Atlas{},
		Dungeon: &sessionworld.Dungeon{
			Key:        key,
			Name:       "Contract Hall",
			World:      &tkencounter.EncounterData{},
			PartySeats: []spatial.Position{{X: 7, Y: -3}, {X: -2, Y: 5}},
			Spec:       &tkdungeonspec.Compiled{Key: key},
		},
	}
}

// contractParty is the party in seat order: lobby MemberOrder is bob then
// alice, so bob's char-b takes the first seat.
var contractParty = []string{"char-b", "char-a"}

// armContractLaunch arms the registry lookup and every SDK verb in the exact
// order the launch must call them, and returns a pointer the Launch callback
// writes the generated session id into.
func (s *StartContractSuite) armContractLaunch(entry *dungeons.Entry, key string, withVendor bool) *string {
	sessionID := new(string)

	calls := []any{
		// 1. The registry is asked for the RESOLVED key.
		s.registry.EXPECT().Get(s.ctx, key).Return(entry, nil),
		// 2. ONE Launch: the API-minted session id, the resolved key, the very
		// compiled spec the entry holds, and the party in seat order.
		s.manager.EXPECT().Launch(s.ctx, gomock.Any()).DoAndReturn(
			func(_ context.Context, in *sdk.LaunchInput) (*sdk.LaunchOutput, error) {
				s.Same(entry.Dungeon.Spec, in.Dungeon,
					"Launch gets the compiled spec the registry entry holds, not a re-projection")
				s.Equal(entry.Key, in.DungeonKey, "and the key this launch resolved")
				s.NotEmpty(in.Session, "the API mints the session id itself")
				s.Equal(contractParty, in.Party, "the party in lobby MemberOrder, which is seat order")
				*sessionID = in.Session
				return &sdk.LaunchOutput{Session: in.Session}, nil
			},
		),
	}

	// 3. The reference tomb additionally places the temporary demo vendor,
	// AFTER the launch; any other key places nobody, and an unexpected
	// PlaceNPC call fails the case because no expectation is armed.
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

	sessionID := s.armContractLaunch(entry, key, false)

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
// default-only demo-vendor placement AFTER the launch.
func (s *StartContractSuite) TestStartEncounter_ExplicitDefaultKey_ResolvesTheTombAndPlacesTheVendor() {
	entry := contractEntry(dungeons.DefaultKey)

	s.seedLobby(readyContractLobby())
	sessionID := s.armContractLaunch(entry, dungeons.DefaultKey, true)

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: contractLobbyID,
		DungeonKey: lobbyorch.DungeonKey(dungeons.DefaultKey),
	})
	s.Require().NoError(err)
	s.Equal(*sessionID, out.EncounterID)
}

// TestStartEncounter_OmittedKey_ResolvesTheTombAndPlacesTheVendor pins the
// empty-request path: no key at all resolves to the default tomb, and the
// RESOLVED key — not the empty request — is what Launch records.
func (s *StartContractSuite) TestStartEncounter_OmittedKey_ResolvesTheTombAndPlacesTheVendor() {
	entry := contractEntry(dungeons.DefaultKey)

	s.seedLobby(readyContractLobby())
	sessionID := s.armContractLaunch(entry, dungeons.DefaultKey, true)

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: contractLobbyID,
	})
	s.Require().NoError(err)
	s.Equal(*sessionID, out.EncounterID)
}
