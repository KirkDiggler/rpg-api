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

func (s *LobbySuite) TestGetMyActiveLobby_StartedWithEndedEncounter_ReturnsEmptyOutput() {
	s.seedLobby(&lobbyrepo.Data{
		ID: "lobby-g3", HostPlayerID: "alice", Status: lobbyrepo.StatusStarted, EncounterID: "enc-ended",
		Members:     map[string]*lobbyrepo.Member{"alice": {PlayerID: "alice", IsHost: true}},
		MemberOrder: []string{"alice"},
	})
	s.Require().NoError(s.encRepo.Save(s.ctx, &tkenc.Data{ID: core.EncounterID("enc-ended"), Mode: core.ModeEnded}))

	out, err := s.orch.GetMyActiveLobby(s.ctx, &lobbyorch.GetMyActiveLobbyInput{PlayerID: "alice"})
	s.Require().NoError(err)
	s.Require().Empty(out.LobbyID, "a STARTED lobby whose encounter has ended has nothing to resume")
	s.Require().Empty(out.EncounterID)
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

func (s *LobbySuite) TestGetMyActiveLobby_NilInput_Errors() {
	_, err := s.orch.GetMyActiveLobby(s.ctx, nil)
	s.Require().Error(err)
}
