package lobby_test

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	lobbyv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/lobby/v1alpha1"
)

func (s *HandlerSuite) TestSetReady_NoAuth_Unauthenticated() {
	_, err := s.handler.SetReady(context.Background(), &lobbyv1alpha1.SetReadyRequest{
		LobbyId: "lobby-1", Ready: true,
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.Unauthenticated, st.Code())
}

func (s *HandlerSuite) TestSetReady_EmptyLobbyID_InvalidArgument() {
	_, err := s.handler.SetReady(s.ctx, &lobbyv1alpha1.SetReadyRequest{Ready: true})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.InvalidArgument, st.Code())
}

func (s *HandlerSuite) TestSetReady_Success() {
	lobbyID, _ := s.createLobby("alice", "char-alice", "Alice")

	resp, err := s.handler.SetReady(s.ctx, &lobbyv1alpha1.SetReadyRequest{
		LobbyId: lobbyID, Ready: true,
	})
	s.Require().NoError(err)
	s.Require().NotNil(resp)

	data, err := s.lobbyRepo.Get(s.ctx, lobbyID)
	s.Require().NoError(err)
	s.Require().True(data.Members["alice"].IsReady)
}

func (s *HandlerSuite) TestSetReady_LobbyNotFound() {
	_, err := s.handler.SetReady(s.ctx, &lobbyv1alpha1.SetReadyRequest{
		LobbyId: "no-such-lobby", Ready: true,
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.NotFound, st.Code())
}
