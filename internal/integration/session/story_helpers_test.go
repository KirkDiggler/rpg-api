package session_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	storypb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	sessionstory "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha2"
	sessionaccess "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/sessionaccess"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

// newStoryHandler is the v1alpha2 StoryService over orch, gated as
// production gates it: the act stream the scenes read since the v1 event
// stream was deleted (rpg-project#631).
func newStoryHandler(t *testing.T, orch *sessionorch.Orchestrator, charRepo characterrepo.Repository) *sessionstory.StoryHandler {
	t.Helper()
	access, err := sessionaccess.New(charRepo, orch.Manager)
	require.NoError(t, err)
	story, err := sessionstory.NewStory(&sessionstory.StoryHandlerConfig{
		Manager: orch.Manager, Broker: orch.ActBroker, Gate: access,
	})
	require.NoError(t, err)
	return story
}

// storyOf is every act member has, through the real StoryService GetStory,
// read page by page from the start.
func storyOf(ctx context.Context, t *testing.T, story *sessionstory.StoryHandler, session, member string) []*storypb.Act {
	t.Helper()
	var acts []*storypb.Act
	after := ""
	for {
		page, err := story.GetStory(ctx, &storypb.GetStoryRequest{Session: session, Member: member, AfterId: after})
		require.NoError(t, err)
		acts = append(acts, page.GetActs()...)
		if !page.GetMore() || len(page.GetActs()) == 0 {
			return acts
		}
		after = page.GetActs()[len(page.GetActs())-1].GetId()
	}
}

// storyAfter is the acts member has after the act id after.
func storyAfter(ctx context.Context, t *testing.T, story *sessionstory.StoryHandler, session, member, after string) []*storypb.Act {
	t.Helper()
	var acts []*storypb.Act
	for {
		page, err := story.GetStory(ctx, &storypb.GetStoryRequest{Session: session, Member: member, AfterId: after})
		require.NoError(t, err)
		acts = append(acts, page.GetActs()...)
		if !page.GetMore() || len(page.GetActs()) == 0 {
			return acts
		}
		after = page.GetActs()[len(page.GetActs())-1].GetId()
	}
}

// actStream records, in order, every message a real StoryService
// StreamEvents call sends: the production path minus the network hop.
type actStream struct {
	grpc.ServerStream
	ctx context.Context

	mu       sync.Mutex
	acts     []*storypb.Act
	lagged   *storypb.StoryLagged
	messages int
}

func newActStream(ctx context.Context) *actStream { return &actStream{ctx: ctx} }

func (s *actStream) Context() context.Context { return s.ctx }

func (s *actStream) Send(msg *storypb.StreamEventsResponse) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages++
	switch item := msg.GetItem().(type) {
	case *storypb.StreamEventsResponse_Act:
		s.acts = append(s.acts, item.Act)
	case *storypb.StreamEventsResponse_Lagged:
		s.lagged = item.Lagged
	}
	return nil
}

func (s *actStream) snapshot() []*storypb.Act {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*storypb.Act(nil), s.acts...)
}

func (s *actStream) laggedAt() *storypb.StoryLagged {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lagged
}

// settle waits until the stream's act count stops growing for a short
// stable window, then returns the settled acts; it fails after timeout.
func (s *actStream) settle(t *testing.T, timeout time.Duration) []*storypb.Act {
	t.Helper()
	const stableRounds = 5
	deadline := time.Now().Add(timeout)
	last, stable := -1, 0
	for time.Now().Before(deadline) {
		cur := len(s.snapshot())
		if cur == last {
			stable++
			if stable >= stableRounds {
				return s.snapshot()
			}
		} else {
			stable = 0
		}
		last = cur
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("act stream never settled within %s (stuck at %d acts)", timeout, last)
	return nil
}

// watchActs is the acts member receives live, through a real StoryService
// StreamEvents subscription, while do runs. The stream replays the story
// first (the handler subscribes before it replays, so a settled replay means
// the subscription is live); only what arrives after that baseline is
// returned.
func watchActs(ctx context.Context, t *testing.T, h *acceptanceHarness, session, member string, do func()) []*storypb.Act {
	t.Helper()
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream := newActStream(streamCtx)
	done := make(chan error, 1)
	go func() {
		done <- h.story.StreamEvents(&storypb.StreamEventsRequest{Session: session, Member: member}, stream)
	}()
	baseline := len(stream.settle(t, 2*time.Second))
	do()
	live := stream.settle(t, 2*time.Second)[baseline:]
	require.Nil(t, stream.laggedAt(), "a watched stream must not lag")
	return live
}

// watchCastActs is watchActs on the cast scenes' session.
func watchCastActs(ctx context.Context, t *testing.T, h *acceptanceHarness, member string, do func()) []*storypb.Act {
	t.Helper()
	return watchActs(ctx, t, h, castSessionID, member, do)
}

// requireLiveIsStory fails unless live is exactly the tail of member's
// GetStory: the live stream and the catch-up read are one projection, act
// for act (what the v1 scenes asserted between StreamEvents and GetStory).
func requireLiveIsStory(ctx context.Context, t *testing.T, h *acceptanceHarness, session, member string, live []*storypb.Act) {
	t.Helper()
	require.NotEmpty(t, live, "nothing arrived live")
	story := storyOf(ctx, t, h.story, session, member)
	require.GreaterOrEqual(t, len(story), len(live))
	tail := story[len(story)-len(live):]
	for i := range live {
		require.True(t, proto.Equal(live[i], tail[i]), "live act %d (%s) differs from GetStory's", i, live[i].GetId())
	}
}
