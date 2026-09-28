package lobby_test

import (
	"errors"
	"fmt"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
)

// The Status paths below are scripted through s.manager: GetMyActiveLobby is
// a read, so its whole session dependency is one Status call, and the SDK's
// liveness verdict is the input under test rather than something this suite
// has to make true by starting a real session. Any test that does NOT arm a
// Status expectation (waiting or missing lobby, stale index) proves its
// "Status is not consulted" claim by construction: the controller-isolated
// mock fails the test on an unexpected call.

func (s *LobbySuite) TestGetMyActiveLobby_NoActiveLobby_ReturnsEmptyOutput() {
	out, err := s.orch.GetMyActiveLobby(s.ctx, &lobbyorch.GetMyActiveLobbyInput{PlayerID: "stranger"})
	s.Require().NoError(err)
	s.Require().Empty(out.LobbyID)
	s.Require().Empty(out.EncounterID)
	s.Require().Equal(lobbyrepo.StatusUnspecified, out.Status)
}

func (s *LobbySuite) TestGetMyActiveLobby_WaitingLobby_ReturnsLobbyOnly() {
	s.seedLobby(&lobbyrepo.Data{
		ID: "lobby-g1", HostPlayerID: "alice", Status: lobbyrepo.StatusWaiting,
		Members:     map[string]*lobbyrepo.Member{"alice": {PlayerID: "alice", IsHost: true}},
		MemberOrder: []string{"alice"},
	})

	out, err := s.orch.GetMyActiveLobby(s.ctx, &lobbyorch.GetMyActiveLobbyInput{PlayerID: "alice"})
	s.Require().NoError(err)
	s.Require().Equal("lobby-g1", out.LobbyID)
	s.Require().Empty(out.EncounterID)
	s.Require().Equal(lobbyrepo.StatusWaiting, out.Status)
}

func (s *LobbySuite) TestGetMyActiveLobby_StartedWithLiveEncounter_ReturnsBothIDs() {
	s.manager.EXPECT().
		Status(s.ctx, &sdk.StatusInput{Session: "enc-live"}).
		Return(&sdk.Status{Open: true}, nil)
	s.seedLobby(&lobbyrepo.Data{
		ID: "lobby-g2", HostPlayerID: "alice", Status: lobbyrepo.StatusStarted, EncounterID: "enc-live",
		Members:     map[string]*lobbyrepo.Member{"alice": {PlayerID: "alice", IsHost: true}},
		MemberOrder: []string{"alice"},
	})

	out, err := s.orch.GetMyActiveLobby(s.ctx, &lobbyorch.GetMyActiveLobbyInput{PlayerID: "alice"})
	s.Require().NoError(err)
	s.Require().Equal("lobby-g2", out.LobbyID)
	s.Require().Equal("enc-live", out.EncounterID)
	s.Require().Equal(lobbyrepo.StatusStarted, out.Status)
}

func (s *LobbySuite) TestGetMyActiveLobby_StartedWithTerminalEncounter_ReturnsEmptyOutput() {
	s.manager.EXPECT().
		Status(s.ctx, &sdk.StatusInput{Session: "enc-ended"}).
		Return(&sdk.Status{Open: false}, nil)
	s.seedLobby(&lobbyrepo.Data{
		ID: "lobby-g3", HostPlayerID: "alice", Status: lobbyrepo.StatusStarted, EncounterID: "enc-ended",
		Members:     map[string]*lobbyrepo.Member{"alice": {PlayerID: "alice", IsHost: true}},
		MemberOrder: []string{"alice"},
	})

	out, err := s.orch.GetMyActiveLobby(s.ctx, &lobbyorch.GetMyActiveLobbyInput{PlayerID: "alice"})
	s.Require().NoError(err)
	s.Require().Empty(out.LobbyID, "a STARTED lobby whose session has ended has nothing to resume")
	s.Require().Empty(out.EncounterID)
}

func (s *LobbySuite) TestGetMyActiveLobby_StartedWithMissingSession_ReturnsEmptyOutput() {
	// Started lobby whose session record is simply gone (e.g. expired) —
	// same "nothing to resume" outcome as an explicitly ended one. The SDK
	// reports the missing session, this package treats it as a normal empty
	// answer rather than an error.
	s.manager.EXPECT().
		Status(s.ctx, &sdk.StatusInput{Session: "enc-missing"}).
		Return(nil, fmt.Errorf("provider: %w", sdk.ErrNoSession))
	s.seedLobby(&lobbyrepo.Data{
		ID: "lobby-g4", HostPlayerID: "alice", Status: lobbyrepo.StatusStarted, EncounterID: "enc-missing",
		Members:     map[string]*lobbyrepo.Member{"alice": {PlayerID: "alice", IsHost: true}},
		MemberOrder: []string{"alice"},
	})

	out, err := s.orch.GetMyActiveLobby(s.ctx, &lobbyorch.GetMyActiveLobbyInput{PlayerID: "alice"})
	s.Require().NoError(err)
	s.Require().Empty(out.LobbyID)
	s.Require().Empty(out.EncounterID)
}

func (s *LobbySuite) TestGetMyActiveLobby_StartedWithMissingEncounter_ReturnsEmptyOutput() {
	// The session exists but the encounter record it references does not.
	// Distinct SDK sentinel (ErrNoEncounter), same caller-visible outcome:
	// the lobby is a dead husk, so the whole Output is zeroed.
	s.manager.EXPECT().
		Status(s.ctx, &sdk.StatusInput{Session: "enc-dangling"}).
		Return(nil, fmt.Errorf("provider: %w", sdk.ErrNoEncounter))
	s.seedLobby(&lobbyrepo.Data{
		ID: "lobby-g6", HostPlayerID: "alice", Status: lobbyrepo.StatusStarted, EncounterID: "enc-dangling",
		Members:     map[string]*lobbyrepo.Member{"alice": {PlayerID: "alice", IsHost: true}},
		MemberOrder: []string{"alice"},
	})

	out, err := s.orch.GetMyActiveLobby(s.ctx, &lobbyorch.GetMyActiveLobbyInput{PlayerID: "alice"})
	s.Require().NoError(err)
	s.Require().Empty(out.LobbyID)
	s.Require().Empty(out.EncounterID)
}

// TestGetMyActiveLobby_StatusFails_ReturnsWrappedError is the non-sentinel
// control: an SDK failure this package does NOT recognize as "no such
// session" must surface as an error, not be folded into the empty-success
// answer above. A transient provider outage reported to the client as
// "you have no active lobby" would strand the player outside an encounter
// that is still running.
func (s *LobbySuite) TestGetMyActiveLobby_StatusFails_ReturnsWrappedError() {
	boom := errors.New("provider unavailable")
	s.manager.EXPECT().
		Status(s.ctx, &sdk.StatusInput{Session: "enc-boom"}).
		Return(nil, boom)
	s.seedLobby(&lobbyrepo.Data{
		ID: "lobby-g7", HostPlayerID: "alice", Status: lobbyrepo.StatusStarted, EncounterID: "enc-boom",
		Members:     map[string]*lobbyrepo.Member{"alice": {PlayerID: "alice", IsHost: true}},
		MemberOrder: []string{"alice"},
	})

	out, err := s.orch.GetMyActiveLobby(s.ctx, &lobbyorch.GetMyActiveLobbyInput{PlayerID: "alice"})
	s.Require().ErrorIs(err, boom, "an unrecognized SDK failure must survive the wrapping")
	s.Require().Nil(out, "there is no empty-success fallback for an arbitrary Status failure")
}

func (s *LobbySuite) TestGetMyActiveLobby_PlayerNoLongerMember_SelfHealsStaleIndex() {
	// Set up a lobby where bob is a member (Save writes bob's index entry),
	// then Save the SAME lobby again with bob removed from Members. Save
	// only adds/refreshes index entries for players CURRENTLY in Members
	// (see Repository.Save's doc comment) — it never removes a departed
	// player's entry — so this reproduces exactly the window LeaveLobby's
	// best-effort ClearPlayerIndex (leave_lobby.go) can leave behind if that
	// cleanup call fails: an index entry pointing at a lobby the player is
	// no longer actually a member of.
	s.seedLobby(&lobbyrepo.Data{
		ID: "lobby-g5", HostPlayerID: "alice", Status: lobbyrepo.StatusWaiting,
		Members: map[string]*lobbyrepo.Member{
			"alice": {PlayerID: "alice", IsHost: true},
			"bob":   {PlayerID: "bob"},
		},
		MemberOrder: []string{"alice", "bob"},
	})
	s.seedLobby(&lobbyrepo.Data{
		ID: "lobby-g5", HostPlayerID: "alice", Status: lobbyrepo.StatusWaiting,
		Members:     map[string]*lobbyrepo.Member{"alice": {PlayerID: "alice", IsHost: true}},
		MemberOrder: []string{"alice"},
	})

	out, err := s.orch.GetMyActiveLobby(s.ctx, &lobbyorch.GetMyActiveLobbyInput{PlayerID: "bob"})
	s.Require().NoError(err)
	s.Require().Empty(out.LobbyID, "a stale index entry pointing at a lobby the player is no longer a member of must resolve to no active lobby")
	s.Require().Empty(out.EncounterID)

	_, err = s.lobbyRepo.GetByPlayerID(s.ctx, "bob")
	s.Require().Error(err, "the stale index entry must be self-healed (cleared) by the read path, not left for TTL to eventually reap")
	s.Require().ErrorIs(err, lobbyrepo.ErrNotFound)
}

func (s *LobbySuite) TestGetMyActiveLobby_NilInput_Errors() {
	_, err := s.orch.GetMyActiveLobby(s.ctx, nil)
	s.Require().Error(err)
}

func (s *LobbySuite) TestGetMyActiveLobby_EmptyPlayerID_Errors() {
	// An empty PlayerID must fail loudly, not silently resolve to "no active
	// lobby" — that would mask a caller bug (e.g. an auth-context wiring
	// mistake upstream) as a legitimate empty result.
	_, err := s.orch.GetMyActiveLobby(s.ctx, &lobbyorch.GetMyActiveLobbyInput{PlayerID: ""})
	s.Require().Error(err)
}
