package lobby_test

import (
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
)

func (s *LobbySuite) seedWaitingLobby(id, joinRef, host string) {
	s.seedLobby(&lobbyrepo.Data{
		ID: id, JoinRef: joinRef, CampaignID: "campaign-1",
		HostPlayerID: host, Status: lobbyrepo.StatusWaiting,
		Members: map[string]*lobbyrepo.Member{
			host: {PlayerID: host, CharacterID: "char-" + host, CharacterName: host, IsHost: true},
		},
		MemberOrder: []string{host},
	})
}

func (s *LobbySuite) TestJoinLobby_NewMember_AddsToRoster() {
	s.seedWaitingLobby("lobby-1", "ref-1", "alice")
	s.expectCharacter("char-bob", "bob", "Bob", 10, 10)

	out, err := s.orch.JoinLobby(s.ctx, &lobbyorch.JoinLobbyInput{
		PlayerID: "bob", JoinRef: "ref-1", CharacterID: "char-bob",
	})
	s.Require().NoError(err)
	s.Require().Equal("lobby-1", out.LobbyID)
	s.Require().Len(out.Members, 2)

	data, err := s.lobbyRepo.Get(s.ctx, "lobby-1")
	s.Require().NoError(err)
	s.Require().Contains(data.Members, "bob")
	s.Require().False(data.Members["bob"].IsHost)
	s.Require().Equal([]string{"alice", "bob"}, data.MemberOrder)
}

func (s *LobbySuite) TestJoinLobby_ExistingMember_RebindsCharacter() {
	s.seedWaitingLobby("lobby-2", "ref-2", "alice")
	s.expectCharacter("char-alice-2", "alice", "Alice Prime", 14, 14)

	out, err := s.orch.JoinLobby(s.ctx, &lobbyorch.JoinLobbyInput{
		PlayerID: "alice", JoinRef: "ref-2", CharacterID: "char-alice-2",
	})
	s.Require().NoError(err)
	s.Require().Len(out.Members, 1, "rebind must not add a duplicate seat")

	data, err := s.lobbyRepo.Get(s.ctx, "lobby-2")
	s.Require().NoError(err)
	s.Require().Len(data.Members, 1)
	s.Require().Equal("char-alice-2", data.Members["alice"].CharacterID)
	s.Require().Equal("Alice Prime", data.Members["alice"].CharacterName)
	s.Require().True(data.Members["alice"].IsHost, "rebind must not disturb host status")
}

func (s *LobbySuite) TestJoinLobby_UnknownJoinRef_NotFound() {
	_, err := s.orch.JoinLobby(s.ctx, &lobbyorch.JoinLobbyInput{
		PlayerID: "bob", JoinRef: "no-such-ref", CharacterID: "char-bob",
	})
	s.Require().ErrorIs(err, lobbyorch.ErrLobbyNotFound)
}

func (s *LobbySuite) TestJoinLobby_LobbyStarted_LateJoinFailedPrecondition() {
	s.seedLobby(&lobbyrepo.Data{
		ID: "lobby-3", JoinRef: "ref-3", HostPlayerID: "alice",
		Status:      lobbyrepo.StatusStarted,
		EncounterID: "enc-3",
		Members: map[string]*lobbyrepo.Member{
			"alice": {PlayerID: "alice", IsHost: true},
		},
		MemberOrder: []string{"alice"},
	})

	_, err := s.orch.JoinLobby(s.ctx, &lobbyorch.JoinLobbyInput{
		PlayerID: "bob", JoinRef: "ref-3", CharacterID: "char-bob",
	})
	s.Require().ErrorIs(err, lobbyorch.ErrLobbyAlreadyStarted)
}

func (s *LobbySuite) TestJoinLobby_LobbyFull_FailedPrecondition() {
	s.seedLobby(&lobbyrepo.Data{
		ID: "lobby-4", JoinRef: "ref-4", HostPlayerID: "p1",
		Status: lobbyrepo.StatusWaiting,
		Members: map[string]*lobbyrepo.Member{
			"p1": {PlayerID: "p1", IsHost: true},
			"p2": {PlayerID: "p2"},
			"p3": {PlayerID: "p3"},
			"p4": {PlayerID: "p4"},
		},
		MemberOrder: []string{"p1", "p2", "p3", "p4"},
	})
	s.expectCharacter("char-p5", "p5", "P5", 10, 10)

	_, err := s.orch.JoinLobby(s.ctx, &lobbyorch.JoinLobbyInput{
		PlayerID: "p5", JoinRef: "ref-4", CharacterID: "char-p5",
	})
	s.Require().ErrorIs(err, lobbyorch.ErrLobbyFull)
}

func (s *LobbySuite) TestJoinLobby_CharacterOwnedByOtherPlayer_PermissionMismatch() {
	s.seedWaitingLobby("lobby-5", "ref-5", "alice")
	s.expectCharacter("char-carol", "carol", "Carol", 10, 10)

	_, err := s.orch.JoinLobby(s.ctx, &lobbyorch.JoinLobbyInput{
		PlayerID: "bob", JoinRef: "ref-5", CharacterID: "char-carol",
	})
	s.Require().ErrorIs(err, lobbyorch.ErrCharacterOwnershipMismatch)
}
