package lobby_test

import (
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
)

func (s *LobbySuite) TestLeaveLobby_NonHostLeaves_RemovesSeat() {
	s.seedLobby(&lobbyrepo.Data{
		ID: "lobby-l1", HostPlayerID: "alice", Status: lobbyrepo.StatusWaiting,
		Members: map[string]*lobbyrepo.Member{
			"alice": {PlayerID: "alice", IsHost: true},
			"bob":   {PlayerID: "bob"},
		},
		MemberOrder: []string{"alice", "bob"},
	})

	_, err := s.orch.LeaveLobby(s.ctx, &lobbyorch.LeaveLobbyInput{PlayerID: "bob", LobbyID: "lobby-l1"})
	s.Require().NoError(err)

	data, err := s.lobbyRepo.Get(s.ctx, "lobby-l1")
	s.Require().NoError(err)
	s.Require().NotContains(data.Members, "bob")
	s.Require().Equal([]string{"alice"}, data.MemberOrder)
	s.Require().Equal("alice", data.HostPlayerID, "host must not change when a non-host leaves")
}

func (s *LobbySuite) TestLeaveLobby_HostLeaves_MigratesToOldestRemaining() {
	s.seedLobby(&lobbyrepo.Data{
		ID: "lobby-l2", HostPlayerID: "alice", Status: lobbyrepo.StatusWaiting,
		Members: map[string]*lobbyrepo.Member{
			"alice": {PlayerID: "alice", IsHost: true},
			"bob":   {PlayerID: "bob"},
			"carol": {PlayerID: "carol"},
		},
		MemberOrder: []string{"alice", "bob", "carol"},
	})

	_, err := s.orch.LeaveLobby(s.ctx, &lobbyorch.LeaveLobbyInput{PlayerID: "alice", LobbyID: "lobby-l2"})
	s.Require().NoError(err)

	data, err := s.lobbyRepo.Get(s.ctx, "lobby-l2")
	s.Require().NoError(err)
	s.Require().NotContains(data.Members, "alice")
	s.Require().Equal("bob", data.HostPlayerID, "oldest remaining member (bob) must become host")
	s.Require().True(data.Members["bob"].IsHost)
	s.Require().False(data.Members["carol"].IsHost)
}

func (s *LobbySuite) TestLeaveLobby_LastMemberLeaves_EmptyLobbyPersists() {
	s.seedLobby(&lobbyrepo.Data{
		ID: "lobby-l3", HostPlayerID: "alice", Status: lobbyrepo.StatusWaiting,
		Members:     map[string]*lobbyrepo.Member{"alice": {PlayerID: "alice", IsHost: true}},
		MemberOrder: []string{"alice"},
	})

	_, err := s.orch.LeaveLobby(s.ctx, &lobbyorch.LeaveLobbyInput{PlayerID: "alice", LobbyID: "lobby-l3"})
	s.Require().NoError(err)

	data, err := s.lobbyRepo.Get(s.ctx, "lobby-l3")
	s.Require().NoError(err, "an empty lobby is still saved, not deleted — it expires via TTL")
	s.Require().Empty(data.Members)
	s.Require().Empty(data.MemberOrder)
}

func (s *LobbySuite) TestLeaveLobby_NotAMember_PermissionDenied() {
	s.seedWaitingLobby("lobby-l4", "ref-l4", "alice")

	_, err := s.orch.LeaveLobby(s.ctx, &lobbyorch.LeaveLobbyInput{PlayerID: "stranger", LobbyID: "lobby-l4"})
	s.Require().ErrorIs(err, lobbyorch.ErrPlayerNotInLobby)
}

func (s *LobbySuite) TestLeaveLobby_LobbyStarted_FailedPrecondition() {
	s.seedLobby(&lobbyrepo.Data{
		ID: "lobby-l5", HostPlayerID: "alice", Status: lobbyrepo.StatusStarted,
		Members:     map[string]*lobbyrepo.Member{"alice": {PlayerID: "alice", IsHost: true}},
		MemberOrder: []string{"alice"},
	})

	_, err := s.orch.LeaveLobby(s.ctx, &lobbyorch.LeaveLobbyInput{PlayerID: "alice", LobbyID: "lobby-l5"})
	s.Require().ErrorIs(err, lobbyorch.ErrLobbyAlreadyStarted)
}
