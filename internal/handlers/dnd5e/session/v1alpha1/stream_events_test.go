package sessionv1alpha1

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

// capturingStream satisfies grpc.ServerStreamingServer[sessionpb.Event]
// (sessionpb.SessionService_StreamEventsServer) for unit tests, mirroring the
// v2 encounter handler's own capturingStream (handler_test.go).
type capturingStream struct {
	grpc.ServerStream
	ctx  context.Context
	sent chan *sessionpb.Event
}

func newCapturingStream(ctx context.Context) *capturingStream {
	return &capturingStream{ctx: ctx, sent: make(chan *sessionpb.Event, 16)}
}

func (s *capturingStream) Context() context.Context { return s.ctx }

func (s *capturingStream) Send(evt *sessionpb.Event) error {
	select {
	case s.sent <- evt:
		return nil
	default:
		return fmt.Errorf("capturingStream buffer full")
	}
}

func (s *capturingStream) WaitForSend(t *testing.T, timeout time.Duration) *sessionpb.Event {
	t.Helper()
	select {
	case evt := <-s.sent:
		return evt
	case <-time.After(timeout):
		t.Fatalf("no event received within %s", timeout)
		return nil
	}
}

func ownedCharacterRepo(ctrl *gomock.Controller, member, playerID string) characterrepo.Repository {
	repo := charactermock.NewMockRepository(ctrl)
	repo.EXPECT().Get(gomock.Any(), characterrepo.GetInput{ID: member}).Return(
		&characterrepo.GetOutput{Character: &entities.Character{Data: &tkcharacter.Data{ID: member, PlayerID: playerID}}}, nil,
	).AnyTimes()
	return repo
}

func TestStreamEvents_Unauthenticated_Errors(t *testing.T) {
	h := &Handler{}
	stream := newCapturingStream(context.Background())
	err := h.StreamEvents(&sessionpb.StreamEventsRequest{}, stream)
	requireCode(t, err, codes.Unauthenticated)
}

func TestStreamEvents_EmptyMember_InvalidArgument(t *testing.T) {
	h := &Handler{}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	stream := newCapturingStream(ctx)
	err := h.StreamEvents(&sessionpb.StreamEventsRequest{Session: "sess-1"}, stream)
	requireCode(t, err, codes.InvalidArgument)
}

func TestStreamEvents_CallerDoesNotOwnMember_PermissionDenied(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := &Handler{characters: ownedCharacterRepo(ctrl, "char-1", "bob"), broker: sessionorch.NewBroker()}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	stream := newCapturingStream(ctx)
	err := h.StreamEvents(&sessionpb.StreamEventsRequest{Session: "sess-1", Member: "char-1"}, stream)
	requireCode(t, err, codes.PermissionDenied)
}

func TestStreamEvents_ForwardsPublishedEventsVerbatim(t *testing.T) {
	ctrl := gomock.NewController(t)
	broker := sessionorch.NewBroker()
	h := &Handler{characters: ownedCharacterRepo(ctrl, "char-1", "alice"), broker: broker}

	ctx, cancel := context.WithCancel(auth.WithPlayerID(context.Background(), "alice"))
	defer cancel()
	stream := newCapturingStream(ctx)

	done := make(chan error, 1)
	go func() {
		done <- h.StreamEvents(&sessionpb.StreamEventsRequest{Session: "sess-1", Member: "char-1"}, stream)
	}()

	// The handler's Subscribe call happens inside that goroutine, so there is
	// no signal here for exactly when it has run. Publish is best-effort and
	// silently drops to a not-yet-subscribed recipient (by contract -- see
	// broker.go), so republish on a short tick until the event shows up in
	// stream.sent rather than assuming one publish lands.
	got := waitForPublishedEvent(t, broker, stream, sdk.Event{
		Session: "sess-1", Recipient: "char-1", Seq: 1, Kind: sdk.EventMoved,
	})
	require.Equal(t, uint64(1), got.GetSeq())
	require.Equal(t, sessionpb.EventKind_EVENT_KIND_MOVED, got.GetKind())
	require.Equal(t, "char-1", got.GetRecipient())

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("StreamEvents did not return after context cancellation")
	}
}

func TestStreamEvents_DoesNotReceiveEventsAddressedToOthers(t *testing.T) {
	ctrl := gomock.NewController(t)
	broker := sessionorch.NewBroker()
	h := &Handler{characters: ownedCharacterRepo(ctrl, "char-1", "alice"), broker: broker}

	ctx, cancel := context.WithCancel(auth.WithPlayerID(context.Background(), "alice"))
	defer cancel()
	stream := newCapturingStream(ctx)

	done := make(chan error, 1)
	go func() {
		done <- h.StreamEvents(&sessionpb.StreamEventsRequest{Session: "sess-1", Member: "char-1"}, stream)
	}()

	// Confirm the subscription is live (via a control event this recipient
	// DOES own) before asserting the negative -- otherwise "nothing arrived"
	// would be indistinguishable from "hadn't subscribed yet".
	waitForPublishedEvent(t, broker, stream, sdk.Event{
		Session: "sess-1", Recipient: "char-1", Seq: 1, Kind: sdk.EventMoved,
	})

	broker.Publish(context.Background(), []sdk.Event{ //nolint:errcheck // Publish never errors (best-effort by contract)
		{Session: "sess-1", Recipient: "someone-else", Seq: 2, Kind: sdk.EventMoved},
	})

	select {
	case evt := <-stream.sent:
		t.Fatalf("must not receive an event addressed to a different recipient, got seq %d", evt.GetSeq())
	case <-time.After(100 * time.Millisecond):
	}

	cancel()
	<-done
}

// waitForPublishedEvent republishes evt on a short tick until it appears in
// stream.sent or the test times out. Necessary because the subscriber this
// helper is waiting on is registered inside a goroutine this test does not
// otherwise synchronize with, and Publish silently drops to a recipient with
// no live subscription yet (best-effort by contract, broker.go) rather than
// erroring -- so a single publish racing the Subscribe call is not reliable.
func waitForPublishedEvent(t *testing.T, broker *sessionorch.Broker, stream *capturingStream, evt sdk.Event) *sessionpb.Event {
	t.Helper()
	deadline := time.After(2 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case got := <-stream.sent:
			return got
		case <-ticker.C:
			broker.Publish(context.Background(), []sdk.Event{evt}) //nolint:errcheck // best-effort by contract
		case <-deadline:
			t.Fatal("event never arrived on the stream")
			return nil
		}
	}
}
