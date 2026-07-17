package lobby_test

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	lobbyv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/lobby/v1alpha1"
)

func (s *HandlerSuite) TestGetMyActiveLobby_Unauthenticated() {
	_, err := s.handler.GetMyActiveLobby(context.Background(), &lobbyv1alpha1.GetMyActiveLobbyRequest{})
	s.Require().Error(err)
	s.Require().Equal(codes.Unauthenticated, status.Code(err))
}

func (s *HandlerSuite) TestGetMyActiveLobby_NoActiveLobby_ReturnsEmptyResponse() {
	resp, err := s.handler.GetMyActiveLobby(s.ctx, &lobbyv1alpha1.GetMyActiveLobbyRequest{})
	s.Require().NoError(err)
	s.Require().Empty(resp.GetLobbyId())
	s.Require().Empty(resp.GetEncounterId())
	s.Require().Equal(lobbyv1alpha1.LobbyStatus_LOBBY_STATUS_UNSPECIFIED, resp.GetLobbyStatus())
}

func (s *HandlerSuite) TestGetMyActiveLobby_WaitingLobby_TranslatesStatusAndID() {
	lobbyID, _ := s.createLobby("alice", "char-alice", "Alice")

	resp, err := s.handler.GetMyActiveLobby(s.ctx, &lobbyv1alpha1.GetMyActiveLobbyRequest{})
	s.Require().NoError(err)
	s.Require().Equal(lobbyID, resp.GetLobbyId())
	s.Require().Empty(resp.GetEncounterId())
	s.Require().Equal(lobbyv1alpha1.LobbyStatus_LOBBY_STATUS_WAITING, resp.GetLobbyStatus())
}
