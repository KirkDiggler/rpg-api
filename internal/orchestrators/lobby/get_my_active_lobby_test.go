package lobby_test

import (
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
)

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
	s.seedLobby(&lobbyrepo.Data{
		ID: "lobby-g2", HostPlayerID: "alice", Status: lobbyrepo.StatusStarted, EncounterID: "enc-live",
		Members:     map[string]*lobbyrepo.Member{"alice": {PlayerID: "alice", IsHost: true}},
		MemberOrder: []string{"alice"},
	})
	s.Require().NoError(s.encRepo.Save(s.ctx, &tkenc.Data{ID: core.EncounterID("enc-live"), Mode: core.ModeFreeRoam}))

	out, err := s.orch.GetMyActiveLobby(s.ctx, &lobbyorch.GetMyActiveLobbyInput{PlayerID: "alice"})
	s.Require().NoError(err)
	s.Require().Equal("lobby-g2", out.LobbyID)
	s.Require().Equal("enc-live", out.EncounterID)
	s.Require().Equal(lobbyrepo.StatusStarted, out.Status)
}

func (s *LobbySuite) TestGetMyActiveLobby_StartedWithTerminalEncounter_ReturnsEmptyOutput() {
	for _, terminal := range []string{"all-hostiles-defeated victory", "tpk defeat"} {
		s.Run(terminal, func() {
			s.seedLobby(&lobbyrepo.Data{
				ID: "lobby-g3-" + terminal, HostPlayerID: "alice", Status: lobbyrepo.StatusStarted, EncounterID: "enc-ended-" + terminal,
				Members:     map[string]*lobbyrepo.Member{"alice": {PlayerID: "alice", IsHost: true}},
				MemberOrder: []string{"alice"},
			})
			s.Require().NoError(s.encRepo.Save(s.ctx, &tkenc.Data{ID: core.EncounterID("enc-ended-" + terminal), Mode: core.ModeEnded}))

			out, err := s.orch.GetMyActiveLobby(s.ctx, &lobbyorch.GetMyActiveLobbyInput{PlayerID: "alice"})
			s.Require().NoError(err)
			s.Require().Empty(out.LobbyID, "a STARTED lobby whose encounter has ended has nothing to resume")
			s.Require().Empty(out.EncounterID)
		})
	}
}

func (s *LobbySuite) TestGetMyActiveLobby_StartedWithMissingEncounter_ReturnsEmptyOutput() {
	// Started lobby whose encounter record is simply gone (e.g. expired) —
	// same "nothing to resume" outcome as an explicitly ended one.
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
