package lobby_test

import (
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
)

func (s *LobbySuite) TestCreateLobby_Success_HostSeatedAndWaiting() {
	s.expectCharacter("char-alice", "alice", "Alice", 12, 12)

	out, err := s.orch.CreateLobby(s.ctx, &lobbyorch.CreateLobbyInput{
		PlayerID: "alice", CampaignID: "campaign-1", CharacterID: "char-alice",
	})
	s.Require().NoError(err)
	s.Require().NotEmpty(out.LobbyID)
	s.Require().NotEmpty(out.JoinRef)
	s.Require().Equal("alice", out.HostPlayerID)

	data, err := s.lobbyRepo.Get(s.ctx, out.LobbyID)
	s.Require().NoError(err)
	s.Require().Equal(lobbyrepo.StatusWaiting, data.Status)
	s.Require().Equal("alice", data.HostPlayerID)
	s.Require().Len(data.Members, 1)
	s.Require().True(data.Members["alice"].IsHost)
	s.Require().Equal("Alice", data.Members["alice"].CharacterName)
	s.Require().False(data.Members["alice"].IsReady)
}

func (s *LobbySuite) TestCreateLobby_CharacterOwnedByOtherPlayer_PermissionMismatch() {
	s.expectCharacter("char-bob", "bob", "Bob", 10, 10)

	_, err := s.orch.CreateLobby(s.ctx, &lobbyorch.CreateLobbyInput{
		PlayerID: "alice", CampaignID: "campaign-1", CharacterID: "char-bob",
	})
	s.Require().ErrorIs(err, lobbyorch.ErrCharacterOwnershipMismatch)
}

func (s *LobbySuite) TestCreateLobby_CharacterNotFound() {
	s.expectCharacterNotFound("char-missing")

	_, err := s.orch.CreateLobby(s.ctx, &lobbyorch.CreateLobbyInput{
		PlayerID: "alice", CampaignID: "campaign-1", CharacterID: "char-missing",
	})
	s.Require().ErrorIs(err, lobbyorch.ErrCharacterNotFound)
}

func (s *LobbySuite) TestCreateLobby_NilInput_Errors() {
	_, err := s.orch.CreateLobby(s.ctx, nil)
	s.Require().Error(err)
}
