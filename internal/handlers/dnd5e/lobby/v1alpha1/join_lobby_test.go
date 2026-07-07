package lobby_test

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	lobbyv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/lobby/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
)

func (s *HandlerSuite) TestJoinLobby_NoAuth_Unauthenticated() {
	_, err := s.handler.JoinLobby(context.Background(), &lobbyv1alpha1.JoinLobbyRequest{
		JoinRef: "ref-1", CharacterId: "char-bob",
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.Unauthenticated, st.Code())
}

func (s *HandlerSuite) TestJoinLobby_EmptyJoinRef_InvalidArgument() {
	_, err := s.handler.JoinLobby(s.ctx, &lobbyv1alpha1.JoinLobbyRequest{
		CharacterId: "char-alice",
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.InvalidArgument, st.Code())
}

func (s *HandlerSuite) TestJoinLobby_EmptyCharacterID_InvalidArgument() {
	_, joinRef := s.createLobby("alice", "char-alice", "Alice")

	_, err := s.handler.JoinLobby(s.ctx, &lobbyv1alpha1.JoinLobbyRequest{JoinRef: joinRef})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.InvalidArgument, st.Code())
}

func (s *HandlerSuite) TestJoinLobby_Success_ReturnsFullRoster() {
	lobbyID, joinRef := s.createLobby("alice", "char-alice", "Alice")
	s.expectCharacter("char-bob", "bob", "Bob", 10, 10)

	bobCtx := auth.WithPlayerID(context.Background(), "bob")
	resp, err := s.handler.JoinLobby(bobCtx, &lobbyv1alpha1.JoinLobbyRequest{
		JoinRef: joinRef, CharacterId: "char-bob",
	})
	s.Require().NoError(err)
	s.Require().Equal(lobbyID, resp.GetLobbyId())
	s.Require().Len(resp.GetMembers(), 2)

	var bobMember *lobbyv1alpha1.LobbyMember
	for _, m := range resp.GetMembers() {
		if m.GetPlayerId() == "bob" {
			bobMember = m
		}
	}
	s.Require().NotNil(bobMember)
	s.Require().Equal("Bob", bobMember.GetCharacterName())
	s.Require().False(bobMember.GetIsHost())
}

func (s *HandlerSuite) TestJoinLobby_UnknownJoinRef_NotFound() {
	bobCtx := auth.WithPlayerID(context.Background(), "bob")
	_, err := s.handler.JoinLobby(bobCtx, &lobbyv1alpha1.JoinLobbyRequest{
		JoinRef: "no-such-ref", CharacterId: "char-bob",
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.NotFound, st.Code())
}
