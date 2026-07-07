package lobby_test

import lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"

func (s *LobbySuite) TestSetConnected_FlipsPresence_NotMembership() {
	s.seedWaitingLobby("lobby-c1", "ref-c1", "alice")

	out, err := s.orch.SetConnected(s.ctx, &lobbyorch.SetConnectedInput{
		PlayerID: "alice", LobbyID: "lobby-c1", Connected: true,
	})
	s.Require().NoError(err)
	s.Require().Len(out.Members, 1)
	s.Require().True(out.Members[0].IsConnected)

	data, err := s.lobbyRepo.Get(s.ctx, "lobby-c1")
	s.Require().NoError(err)
	s.Require().True(data.Members["alice"].IsConnected)

	_, err = s.orch.SetConnected(s.ctx, &lobbyorch.SetConnectedInput{
		PlayerID: "alice", LobbyID: "lobby-c1", Connected: false,
	})
	s.Require().NoError(err)
	data, err = s.lobbyRepo.Get(s.ctx, "lobby-c1")
	s.Require().NoError(err)
	s.Require().False(data.Members["alice"].IsConnected)
	s.Require().Contains(data.Members, "alice", "disconnect must not remove the seat")
}

func (s *LobbySuite) TestSetConnected_NotAMember_PermissionDenied() {
	s.seedWaitingLobby("lobby-c2", "ref-c2", "alice")

	_, err := s.orch.SetConnected(s.ctx, &lobbyorch.SetConnectedInput{
		PlayerID: "stranger", LobbyID: "lobby-c2", Connected: true,
	})
	s.Require().ErrorIs(err, lobbyorch.ErrPlayerNotInLobby)
}

func (s *LobbySuite) TestSetConnected_LobbyNotFound() {
	_, err := s.orch.SetConnected(s.ctx, &lobbyorch.SetConnectedInput{
		PlayerID: "alice", LobbyID: "no-such-lobby", Connected: true,
	})
	s.Require().ErrorIs(err, lobbyorch.ErrLobbyNotFound)
}
