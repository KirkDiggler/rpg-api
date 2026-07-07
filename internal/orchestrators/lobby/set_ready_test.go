package lobby_test

import (
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
)

func (s *LobbySuite) TestSetReady_TogglesReadyFlag() {
	s.seedWaitingLobby("lobby-r1", "ref-r1", "alice")

	_, err := s.orch.SetReady(s.ctx, &lobbyorch.SetReadyInput{
		PlayerID: "alice", LobbyID: "lobby-r1", Ready: true,
	})
	s.Require().NoError(err)

	data, err := s.lobbyRepo.Get(s.ctx, "lobby-r1")
	s.Require().NoError(err)
	s.Require().True(data.Members["alice"].IsReady)

	_, err = s.orch.SetReady(s.ctx, &lobbyorch.SetReadyInput{
		PlayerID: "alice", LobbyID: "lobby-r1", Ready: false,
	})
	s.Require().NoError(err)
	data, err = s.lobbyRepo.Get(s.ctx, "lobby-r1")
	s.Require().NoError(err)
	s.Require().False(data.Members["alice"].IsReady)
}

func (s *LobbySuite) TestSetReady_NotAMember_PermissionDenied() {
	s.seedWaitingLobby("lobby-r2", "ref-r2", "alice")

	_, err := s.orch.SetReady(s.ctx, &lobbyorch.SetReadyInput{
		PlayerID: "stranger", LobbyID: "lobby-r2", Ready: true,
	})
	s.Require().ErrorIs(err, lobbyorch.ErrPlayerNotInLobby)
}

func (s *LobbySuite) TestSetReady_LobbyNotFound() {
	_, err := s.orch.SetReady(s.ctx, &lobbyorch.SetReadyInput{
		PlayerID: "alice", LobbyID: "no-such-lobby", Ready: true,
	})
	s.Require().ErrorIs(err, lobbyorch.ErrLobbyNotFound)
}

func (s *LobbySuite) TestSetReady_LobbyStarted_FailedPrecondition() {
	s.seedLobby(&lobbyrepo.Data{
		ID: "lobby-r3", HostPlayerID: "alice", Status: lobbyrepo.StatusStarted,
		Members:     map[string]*lobbyrepo.Member{"alice": {PlayerID: "alice", IsHost: true}},
		MemberOrder: []string{"alice"},
	})

	_, err := s.orch.SetReady(s.ctx, &lobbyorch.SetReadyInput{
		PlayerID: "alice", LobbyID: "lobby-r3", Ready: true,
	})
	s.Require().ErrorIs(err, lobbyorch.ErrLobbyAlreadyStarted)
}
