package lobby_test

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	lobbyv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/lobby/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
)

func (s *HandlerSuite) TestLeaveLobby_NoAuth_Unauthenticated() {
	_, err := s.handler.LeaveLobby(context.Background(), &lobbyv1alpha1.LeaveLobbyRequest{
		LobbyId: "lobby-1",
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.Unauthenticated, st.Code())
}

func (s *HandlerSuite) TestLeaveLobby_EmptyLobbyID_InvalidArgument() {
	_, err := s.handler.LeaveLobby(s.ctx, &lobbyv1alpha1.LeaveLobbyRequest{})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.InvalidArgument, st.Code())
}

func (s *HandlerSuite) TestLeaveLobby_Success_HostMigrates() {
	lobbyID, joinRef := s.createLobby("alice", "char-alice", "Alice")
	s.expectCharacter("char-bob", "bob", "Bob", 10, 10)
	bobCtx := auth.WithPlayerID(context.Background(), "bob")
	_, err := s.handler.JoinLobby(bobCtx, &lobbyv1alpha1.JoinLobbyRequest{
		JoinRef: joinRef, CharacterId: "char-bob",
	})
	s.Require().NoError(err)

	resp, err := s.handler.LeaveLobby(s.ctx, &lobbyv1alpha1.LeaveLobbyRequest{LobbyId: lobbyID})
	s.Require().NoError(err)
	s.Require().NotNil(resp)

	data, err := s.lobbyRepo.Get(s.ctx, lobbyID)
	s.Require().NoError(err)
	s.Require().NotContains(data.Members, "alice")
	s.Require().True(data.Members["bob"].IsHost, "bob must become host after alice (host) leaves")
}

func (s *HandlerSuite) TestLeaveLobby_LobbyNotFound() {
	_, err := s.handler.LeaveLobby(s.ctx, &lobbyv1alpha1.LeaveLobbyRequest{LobbyId: "no-such-lobby"})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.NotFound, st.Code())
}
