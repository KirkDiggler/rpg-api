package lobby_test

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	lobbyv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/lobby/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
)

func (s *HandlerSuite) TestStartEncounter_NoAuth_Unauthenticated() {
	_, err := s.handler.StartEncounter(context.Background(), &lobbyv1alpha1.StartEncounterRequest{
		LobbyId: "lobby-1",
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.Unauthenticated, st.Code())
}

func (s *HandlerSuite) TestStartEncounter_EmptyLobbyID_InvalidArgument() {
	_, err := s.handler.StartEncounter(s.ctx, &lobbyv1alpha1.StartEncounterRequest{})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.InvalidArgument, st.Code())
}

func (s *HandlerSuite) TestStartEncounter_NotAllReady_FailedPrecondition() {
	lobbyID, _ := s.createLobby("alice", "char-alice", "Alice")

	_, err := s.handler.StartEncounter(s.ctx, &lobbyv1alpha1.StartEncounterRequest{LobbyId: lobbyID})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.FailedPrecondition, st.Code())
}

func (s *HandlerSuite) TestStartEncounter_NotHost_PermissionDenied() {
	lobbyID, joinRef := s.createLobby("alice", "char-alice", "Alice")
	s.expectCharacter("char-bob", "bob", "Bob", 10, 10)
	bobCtx := auth.WithPlayerID(context.Background(), "bob")
	_, err := s.handler.JoinLobby(bobCtx, &lobbyv1alpha1.JoinLobbyRequest{
		JoinRef: joinRef, CharacterId: "char-bob",
	})
	s.Require().NoError(err)

	_, err = s.handler.StartEncounter(bobCtx, &lobbyv1alpha1.StartEncounterRequest{LobbyId: lobbyID})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.PermissionDenied, st.Code())
}

func (s *HandlerSuite) TestStartEncounter_Success_ReturnsEncounterID() {
	lobbyID, _ := s.createLobby("alice", "char-alice", "Alice")
	_, err := s.handler.SetReady(s.ctx, &lobbyv1alpha1.SetReadyRequest{LobbyId: lobbyID, Ready: true})
	s.Require().NoError(err)

	s.expectCharacter("char-alice", "alice", "Alice", 12, 12)
	resp, err := s.handler.StartEncounter(s.ctx, &lobbyv1alpha1.StartEncounterRequest{LobbyId: lobbyID})
	s.Require().NoError(err)
	s.Require().NotEmpty(resp.GetEncounterId())
}

// TestStartEncounter_UnknownDungeonKey_NotFound pins the wire mapping of
// design §3c: a dungeon_key the registry does not have is NotFound, never
// silently the tomb.
func (s *HandlerSuite) TestStartEncounter_UnknownDungeonKey_NotFound() {
	lobbyID, _ := s.createLobby("alice", "char-alice", "Alice")
	_, err := s.handler.SetReady(s.ctx, &lobbyv1alpha1.SetReadyRequest{LobbyId: lobbyID, Ready: true})
	s.Require().NoError(err)

	_, err = s.handler.StartEncounter(s.ctx, &lobbyv1alpha1.StartEncounterRequest{
		LobbyId: lobbyID, DungeonKey: "nope",
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.NotFound, st.Code())
}
