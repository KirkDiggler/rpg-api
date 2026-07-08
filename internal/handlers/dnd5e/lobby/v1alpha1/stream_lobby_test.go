package lobby_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	lobbyv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/lobby/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
)

func (s *HandlerSuite) TestStreamLobby_NoAuth_Unauthenticated() {
	stream := newCapturingStream(context.Background())
	err := s.handler.StreamLobby(&lobbyv1alpha1.StreamLobbyRequest{LobbyId: "lobby-1"}, stream)
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.Unauthenticated, st.Code())
}

func (s *HandlerSuite) TestStreamLobby_EmptyLobbyID_InvalidArgument() {
	stream := newCapturingStream(s.ctx)
	err := s.handler.StreamLobby(&lobbyv1alpha1.StreamLobbyRequest{}, stream)
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.InvalidArgument, st.Code())
}

func (s *HandlerSuite) TestStreamLobby_SendsSnapshotFirst_AndFlipsPresence() {
	lobbyID, _ := s.createLobby("alice", "char-alice", "Alice")

	ctx, cancel := context.WithCancel(s.ctx)
	stream := newCapturingStream(ctx)
	go func() {
		_ = s.handler.StreamLobby(&lobbyv1alpha1.StreamLobbyRequest{LobbyId: lobbyID}, stream)
	}()

	first := stream.WaitForSend(s.T(), 2*time.Second)
	snap := first.GetSnapshot()
	s.Require().NotNil(snap, "first event must be Snapshot")
	s.Require().Len(snap.GetMembers(), 1)
	s.Require().True(snap.GetMembers()[0].GetIsConnected(), "subscribe must flip is_connected true")

	cancel() // disconnect

	s.Require().Eventually(func() bool {
		data, err := s.lobbyRepo.Get(context.Background(), lobbyID)
		return err == nil && !data.Members["alice"].IsConnected
	}, 2*time.Second, 10*time.Millisecond, "disconnect must flip is_connected back false")

	data, err := s.lobbyRepo.Get(context.Background(), lobbyID)
	s.Require().NoError(err)
	s.Require().Contains(data.Members, "alice", "disconnect must not remove the seat")
}

func (s *HandlerSuite) TestStreamLobby_ForwardsBrokerEvents() {
	lobbyID, joinRef := s.createLobby("alice", "char-alice", "Alice")

	stream := newCapturingStream(s.ctx)
	go func() {
		_ = s.handler.StreamLobby(&lobbyv1alpha1.StreamLobbyRequest{LobbyId: lobbyID}, stream)
	}()
	_ = stream.WaitForSend(s.T(), 2*time.Second) // snapshot
	// StreamLobby's own SetConnected(true) publishes MemberConnectionChanged
	// for alice to every subscriber of this lobby, including the subscription
	// just created — it lands in the buffer ahead of the forward loop and is
	// delivered right after the snapshot. Drain it before asserting on bob's
	// join.
	self := stream.WaitForSend(s.T(), 2*time.Second)
	s.Require().NotNil(self.GetMemberConnectionChanged())

	s.expectCharacter("char-bob", "bob", "Bob", 10, 10)
	bobCtx := auth.WithPlayerID(context.Background(), "bob")
	_, err := s.handler.JoinLobby(bobCtx, &lobbyv1alpha1.JoinLobbyRequest{
		JoinRef: joinRef, CharacterId: "char-bob",
	})
	s.Require().NoError(err)

	got := stream.WaitForSend(s.T(), 2*time.Second)
	s.Require().NotNil(got.GetMemberJoined(), "alice's stream must see bob's join as a broker event")
	s.Require().Equal("bob", got.GetMemberJoined().GetMember().GetPlayerId())
}

// capturingStream satisfies lobbyv1alpha1.LobbyService_StreamLobbyServer for
// unit tests, mirroring the v2 encounter handler's capturingStream
// (internal/handlers/dnd5e/v2/encounter/handler_test.go).
type capturingStream struct {
	grpc.ServerStream
	ctx  context.Context
	sent chan *lobbyv1alpha1.LobbyEvent
}

func newCapturingStream(ctx context.Context) *capturingStream {
	return &capturingStream{ctx: ctx, sent: make(chan *lobbyv1alpha1.LobbyEvent, 16)}
}

func (s *capturingStream) Context() context.Context { return s.ctx }

func (s *capturingStream) Send(evt *lobbyv1alpha1.LobbyEvent) error {
	select {
	case s.sent <- evt:
		return nil
	default:
		return fmt.Errorf("capturingStream buffer full (16 events undrained); test should drain or grow buffer")
	}
}

func (s *capturingStream) WaitForSend(t *testing.T, timeout time.Duration) *lobbyv1alpha1.LobbyEvent {
	t.Helper()
	select {
	case evt := <-s.sent:
		return evt
	case <-time.After(timeout):
		t.Fatalf("no event received within %s", timeout)
		return nil
	}
}
