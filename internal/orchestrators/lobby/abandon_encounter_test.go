package lobby_test

import (
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
)

// seedStartedLobbyWithLiveSession builds a STARTED lobby whose session is a
// real, open one on s.sessOrch (the reference tomb — see seedLiveSession).
// Returns the session/encounter id.
func (s *LobbySuite) seedStartedLobbyWithLiveSession(lobbyID, host, hostCharacterID, encID string) string {
	s.seedLiveSession(encID)
	s.seedLobby(&lobbyrepo.Data{
		ID: lobbyID, HostPlayerID: host, Status: lobbyrepo.StatusStarted, EncounterID: encID,
		Members:     map[string]*lobbyrepo.Member{host: {PlayerID: host, CharacterID: hostCharacterID, IsHost: true}},
		MemberOrder: []string{host},
	})
	return encID
}

func (s *LobbySuite) TestAbandonEncounter_Success_EndsEncounter() {
	s.seedStartedLobbyWithLiveSession("lobby-a1", "alice", "char-alice", "enc-a1")

	_, err := s.orch.AbandonEncounter(s.ctx, &lobbyorch.AbandonEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-a1",
	})
	s.Require().NoError(err)

	// GetMyActiveLobby's own liveness check (get_my_active_lobby.go) is the
	// honest cross-check that the session actually closed: it queries the
	// SAME session.Manager.Status this test would otherwise duplicate.
	out, err := s.orch.GetMyActiveLobby(s.ctx, &lobbyorch.GetMyActiveLobbyInput{PlayerID: "alice"})
	s.Require().NoError(err)
	s.Require().Empty(out.LobbyID, "the session must persist as ended")
}

func (s *LobbySuite) TestAbandonEncounter_NotHost_PermissionDenied() {
	s.seedStartedLobbyWithLiveSession("lobby-a3", "alice", "char-alice", "enc-a3")
	lobbyData, err := s.lobbyRepo.Get(s.ctx, "lobby-a3")
	s.Require().NoError(err)
	lobbyData.Members["bob"] = &lobbyrepo.Member{PlayerID: "bob", CharacterID: "char-bob"}
	lobbyData.MemberOrder = append(lobbyData.MemberOrder, "bob")
	s.Require().NoError(s.lobbyRepo.Save(s.ctx, lobbyData))

	_, err = s.orch.AbandonEncounter(s.ctx, &lobbyorch.AbandonEncounterInput{
		PlayerID: "bob", LobbyID: "lobby-a3",
	})
	s.Require().ErrorIs(err, lobbyorch.ErrNotHost)
}

func (s *LobbySuite) TestAbandonEncounter_LobbyNotFound() {
	_, err := s.orch.AbandonEncounter(s.ctx, &lobbyorch.AbandonEncounterInput{
		PlayerID: "alice", LobbyID: "no-such-lobby",
	})
	s.Require().ErrorIs(err, lobbyorch.ErrLobbyNotFound)
}

func (s *LobbySuite) TestAbandonEncounter_WaitingLobby_ErrLobbyNotStarted() {
	s.seedLobby(&lobbyrepo.Data{
		ID: "lobby-a4", HostPlayerID: "alice", Status: lobbyrepo.StatusWaiting,
		Members:     map[string]*lobbyrepo.Member{"alice": {PlayerID: "alice", IsHost: true}},
		MemberOrder: []string{"alice"},
	})

	_, err := s.orch.AbandonEncounter(s.ctx, &lobbyorch.AbandonEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-a4",
	})
	s.Require().ErrorIs(err, lobbyorch.ErrLobbyNotStarted)
}

// TestAbandonEncounter_AlreadyEnded_ErrEncounterAlreadyEnded is the
// negative control: a second abandon attempt against a session that
// already ended (via the first abandon) must reject, not silently
// re-transition or double-publish.
func (s *LobbySuite) TestAbandonEncounter_AlreadyEnded_ErrEncounterAlreadyEnded() {
	s.seedStartedLobbyWithLiveSession("lobby-a5", "alice", "char-alice", "enc-a5")

	_, err := s.orch.AbandonEncounter(s.ctx, &lobbyorch.AbandonEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-a5",
	})
	s.Require().NoError(err)

	_, err = s.orch.AbandonEncounter(s.ctx, &lobbyorch.AbandonEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-a5",
	})
	s.Require().ErrorIs(err, lobbyorch.ErrEncounterAlreadyEnded)
}

// TestAbandonEncounter_ThenGetMyActiveLobby_ReturnsEmpty proves the
// integration half: abandon a live session -> GetMyActiveLobby reports
// nothing to resume into, exercising the SAME session.Manager.Status this
// package's own get_my_active_lobby.go queries, now end-to-end against a
// real abandoned session.
//
// This does NOT re-prove the arcade-recovery chain (a fresh
// StartEncounter reviving a dead character at full HP/rage charges): that
// lives on the session stack since rpg-api#828 (RestoreForLaunch at
// launch seating) and is proven by the SessionStackSuite's own
// TestStartEncounter_LaunchRestoresEveryMemberFully, not here.
func (s *LobbySuite) TestAbandonEncounter_ThenGetMyActiveLobby_ReturnsEmpty() {
	s.seedStartedLobbyWithLiveSession("lobby-a6", "alice", "char-alice", "enc-a6")

	_, err := s.orch.AbandonEncounter(s.ctx, &lobbyorch.AbandonEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-a6",
	})
	s.Require().NoError(err)

	out, err := s.orch.GetMyActiveLobby(s.ctx, &lobbyorch.GetMyActiveLobbyInput{PlayerID: "alice"})
	s.Require().NoError(err)
	s.Require().Empty(out.LobbyID, "abandoned lobby must not resolve as an active lobby")
	s.Require().Empty(out.EncounterID)
	s.Require().Equal(lobbyrepo.StatusUnspecified, out.Status)
}
