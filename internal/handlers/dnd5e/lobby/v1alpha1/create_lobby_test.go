package lobby_test

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	lobbyv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/lobby/v1alpha1"
)

func (s *HandlerSuite) TestCreateLobby_NoAuth_Unauthenticated() {
	_, err := s.handler.CreateLobby(context.Background(), &lobbyv1alpha1.CreateLobbyRequest{
		CampaignId: "campaign-1", CharacterId: "char-alice",
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.Unauthenticated, st.Code())
}

func (s *HandlerSuite) TestCreateLobby_EmptyCampaignID_InvalidArgument() {
	_, err := s.handler.CreateLobby(s.ctx, &lobbyv1alpha1.CreateLobbyRequest{
		CharacterId: "char-alice",
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.InvalidArgument, st.Code())
}

func (s *HandlerSuite) TestCreateLobby_EmptyCharacterID_InvalidArgument() {
	_, err := s.handler.CreateLobby(s.ctx, &lobbyv1alpha1.CreateLobbyRequest{
		CampaignId: "campaign-1",
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.InvalidArgument, st.Code())
}

func (s *HandlerSuite) TestCreateLobby_Success_ReturnsLobbyIdentity() {
	s.expectCharacter("char-alice", "alice", "Alice", 12, 12)

	resp, err := s.handler.CreateLobby(s.ctx, &lobbyv1alpha1.CreateLobbyRequest{
		CampaignId: "campaign-1", CharacterId: "char-alice",
	})
	s.Require().NoError(err)
	s.Require().NotEmpty(resp.GetLobbyId())
	s.Require().NotEmpty(resp.GetJoinRef())
	s.Require().Equal("alice", resp.GetHostPlayerId())
}

func (s *HandlerSuite) TestCreateLobby_CharacterOwnershipMismatch_PermissionDenied() {
	s.expectCharacter("char-bob", "bob", "Bob", 10, 10)

	_, err := s.handler.CreateLobby(s.ctx, &lobbyv1alpha1.CreateLobbyRequest{
		CampaignId: "campaign-1", CharacterId: "char-bob",
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.PermissionDenied, st.Code())
}

func (s *HandlerSuite) TestCreateLobby_CharacterNotFound_NotFound() {
	s.expectCharacterNotFound("char-missing")

	_, err := s.handler.CreateLobby(s.ctx, &lobbyv1alpha1.CreateLobbyRequest{
		CampaignId: "campaign-1", CharacterId: "char-missing",
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.NotFound, st.Code())
}
