package sessionv1alpha2

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	sessionv1alpha2mock "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha2/mock"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
)

// storyStream captures what StreamEvents sends.
type storyStream struct {
	grpc.ServerStream
	ctx  context.Context
	sent chan *sessionpb.StreamEventsResponse
}

func newStoryStream(ctx context.Context) *storyStream {
	return &storyStream{ctx: ctx, sent: make(chan *sessionpb.StreamEventsResponse, 64)}
}

func (s *storyStream) Context() context.Context { return s.ctx }

func (s *storyStream) Send(m *sessionpb.StreamEventsResponse) error {
	s.sent <- m
	return nil
}

func (s *storyStream) next(t *testing.T) *sessionpb.StreamEventsResponse {
	t.Helper()
	select {
	case m := <-s.sent:
		return m
	case <-time.After(2 * time.Second):
		t.Fatal("nothing sent")
		return nil
	}
}

// storyAct is one decodable act for char-1 with the given record id: a tick,
// so its body converts without any party.
func storyAct(id string) sdk.Act {
	return sdk.Act{
		Session: "sess-1", ID: id, Recipient: "char-1", Family: sdk.FamilyClock,
		Body: sdk.ClockEntry{Mark: sdk.ClockTick, Tick: 1},
	}
}

func storyActs(ids ...string) []sdk.Act {
	out := make([]sdk.Act, len(ids))
	for i, id := range ids {
		out[i] = storyAct(id)
	}
	return out
}

// after is what sdk.Manager.Acts returns from a fixed story: every act whose
// id is after afterID.
func after(all []sdk.Act, afterID string) []sdk.Act {
	var out []sdk.Act
	for _, a := range all {
		if a.ID > afterID {
			out = append(out, a)
		}
	}
	return out
}

type StoryHandlerSuite struct {
	suite.Suite
	ctrl    *gomock.Controller
	manager *sessionv1alpha2mock.MockManager
	gate    *sessionv1alpha2mock.MockGate
	ctx     context.Context
	cancel  context.CancelFunc
}

func TestStoryHandlerSuite(t *testing.T) {
	suite.Run(t, new(StoryHandlerSuite))
}

func (s *StoryHandlerSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.manager = sessionv1alpha2mock.NewMockManager(s.ctrl)
	s.gate = sessionv1alpha2mock.NewMockGate(s.ctrl)
	s.ctx, s.cancel = context.WithCancel(auth.WithPlayerID(context.Background(), "alice"))
}

func (s *StoryHandlerSuite) TearDownTest() { s.cancel() }

func (s *StoryHandlerSuite) handler(broker *sessionorch.ActBroker) *StoryHandler {
	h, err := NewStory(&StoryHandlerConfig{Manager: s.manager, Broker: broker, Gate: s.gate})
	s.Require().NoError(err)
	return h
}

func (s *StoryHandlerSuite) seated() {
	s.gate.EXPECT().CallerMemberSeated(gomock.Any(), "sess-1", "char-1").Return(nil).AnyTimes()
}

func (s *StoryHandlerSuite) run(h *StoryHandler, afterID string) (*storyStream, chan error) {
	stream := newStoryStream(s.ctx)
	done := make(chan error, 1)
	go func() {
		done <- h.StreamEvents(&sessionpb.StreamEventsRequest{Session: "sess-1", Member: "char-1", AfterId: afterID}, stream)
	}()
	return stream, done
}

func (s *StoryHandlerSuite) ended(done chan error) error {
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		s.FailNow("StreamEvents did not end")
		return nil
	}
}

func (s *StoryHandlerSuite) quiet(stream *storyStream) {
	select {
	case m := <-stream.sent:
		s.Failf("an extra message went out", "%v", m)
	case <-time.After(100 * time.Millisecond):
	}
}

// TestStreamEventsReplayThenLive: opened after A, the stream replays B and C,
// then goes live with D. C, folded between the subscribe and the read, also
// arrives live and is not sent twice. The read names the subscription's
// member as the recipient.
func (s *StoryHandlerSuite) TestStreamEventsReplayThenLive() {
	broker := sessionorch.NewActBroker(8)
	s.seated()
	s.manager.EXPECT().Acts(gomock.Any(), &sdk.ActsInput{Session: "sess-1", Member: "char-1", AfterID: "A"}).DoAndReturn(
		func(ctx context.Context, _ *sdk.ActsInput) (*sdk.ActsOutput, error) {
			s.Require().NoError(broker.PublishActs(ctx, storyActs("C", "D")))
			return &sdk.ActsOutput{Acts: storyActs("B", "C")}, nil
		})

	stream, done := s.run(s.handler(broker), "A")
	var got []string
	for i := 0; i < 3; i++ {
		got = append(got, stream.next(s.T()).GetAct().GetId())
	}
	s.Equal([]string{"B", "C", "D"}, got)
	s.quiet(stream)
	s.cancel()
	s.NoError(s.ended(done))
}

// TestStreamEventsLiveSameIDRunWhole: a live same-id run arrives whole. The
// skip is against the replay's end, never the last id sent, so the second
// and third acts of a run are not mistaken for duplicates of the first.
func (s *StoryHandlerSuite) TestStreamEventsLiveSameIDRunWhole() {
	broker := sessionorch.NewActBroker(8)
	s.seated()
	s.manager.EXPECT().Acts(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, _ *sdk.ActsInput) (*sdk.ActsOutput, error) {
			s.Require().NoError(broker.PublishActs(ctx, storyActs("B", "B", "B")))
			return &sdk.ActsOutput{Acts: storyActs("A")}, nil
		})

	stream, done := s.run(s.handler(broker), "")
	var got []string
	var ends []bool
	for i := 0; i < 4; i++ {
		a := stream.next(s.T()).GetAct()
		got = append(got, a.GetId())
		ends = append(ends, a.GetRunEnd())
	}
	s.Equal([]string{"A", "B", "B", "B"}, got)
	s.Equal([]bool{true, false, false, true}, ends, "run_end on the last act of each run, replayed or live")
	s.quiet(stream)
	s.cancel()
	s.NoError(s.ended(done))
}

// TestStreamEventsRestartedFirst: a stale after_id gives `restarted` as the
// first message, then the replay from the oldest retained act.
func (s *StoryHandlerSuite) TestStreamEventsRestartedFirst() {
	broker := sessionorch.NewActBroker(8)
	s.seated()
	s.manager.EXPECT().Acts(gomock.Any(), gomock.Any()).Return(&sdk.ActsOutput{Acts: storyActs("K", "L"), Restarted: true}, nil)

	stream, done := s.run(s.handler(broker), "stale")
	s.NotNil(stream.next(s.T()).GetRestarted(), "restarted is the first message")
	s.Equal("K", stream.next(s.T()).GetAct().GetId())
	s.Equal("L", stream.next(s.T()).GetAct().GetId())
	s.cancel()
	s.NoError(s.ended(done))
}

// TestStreamEventsLaggedLast: when the broker closes the subscription as
// lagged, the stream sends what was queued, then `lagged` naming the last id
// SENT, which ends a whole run, and ends OK with nothing after it. The run
// [C, C, C] did not fit a buffer of 2 beside nothing queued, so no C went
// out, and the client resumes after B.
func (s *StoryHandlerSuite) TestStreamEventsLaggedLast() {
	broker := sessionorch.NewActBroker(2)
	s.seated()
	s.manager.EXPECT().Acts(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, _ *sdk.ActsInput) (*sdk.ActsOutput, error) {
			_ = broker.PublishActs(ctx, storyActs("C", "C", "C")) // overflows: the run is not split
			return &sdk.ActsOutput{Acts: storyActs("A", "B")}, nil
		})

	stream, done := s.run(s.handler(broker), "")
	s.Equal("A", stream.next(s.T()).GetAct().GetId())
	s.Equal("B", stream.next(s.T()).GetAct().GetId())
	lagged := stream.next(s.T()).GetLagged()
	s.Require().NotNil(lagged, "the stream says it lagged")
	s.Equal("B", lagged.GetLastId())
	s.NoError(s.ended(done))
	s.Empty(stream.sent, "lagged is the last message")
}

// TestStoryRequiresSeat: an unseated caller gets PermissionDenied from both
// RPCs, and nothing is read.
func (s *StoryHandlerSuite) TestStoryRequiresSeat() {
	denied := status.Error(codes.PermissionDenied, "not seated")
	s.gate.EXPECT().CallerMemberSeated(gomock.Any(), "sess-1", "char-1").Return(denied).Times(2)
	h := s.handler(sessionorch.NewActBroker(2))

	err := h.StreamEvents(&sessionpb.StreamEventsRequest{Session: "sess-1", Member: "char-1"}, newStoryStream(s.ctx))
	s.Equal(codes.PermissionDenied, status.Code(err))
	_, err = h.GetStory(s.ctx, &sessionpb.GetStoryRequest{Session: "sess-1", Member: "char-1"})
	s.Equal(codes.PermissionDenied, status.Code(err))
}

// TestGetStoryPagesOnIDBoundary: with limit 2 over [A, B, B, C], page 1 is
// [A, B, B] with more true — the page runs past its limit to finish B's run —
// and page 2, after B, is [C] with more false.
func (s *StoryHandlerSuite) TestGetStoryPagesOnIDBoundary() {
	all := storyActs("A", "B", "B", "C")
	s.seated()
	s.manager.EXPECT().Acts(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *sdk.ActsInput) (*sdk.ActsOutput, error) {
			return &sdk.ActsOutput{Acts: after(all, in.AfterID)}, nil
		}).Times(2)
	h := s.handler(sessionorch.NewActBroker(2))

	page, err := h.GetStory(s.ctx, &sessionpb.GetStoryRequest{Session: "sess-1", Member: "char-1", Limit: 2})
	s.Require().NoError(err)
	s.Equal([]string{"A", "B", "B"}, ids(page.GetActs()))
	s.Equal([]bool{true, false, true}, runEnds(page.GetActs()), "a page ends on a run_end")
	s.True(page.GetMore())

	page, err = h.GetStory(s.ctx, &sessionpb.GetStoryRequest{Session: "sess-1", Member: "char-1", AfterId: "B", Limit: 2})
	s.Require().NoError(err)
	s.Equal([]string{"C"}, ids(page.GetActs()))
	s.False(page.GetMore())
}

// TestGetStoryPagesEqualLive: reading the whole story with limit 2, each page
// after the last returned id, gives exactly the acts a live StreamEvents
// subscriber received from the start, compared as protos. The story mixes
// lone acts and same-id runs, including one that straddles every page limit.
func (s *StoryHandlerSuite) TestGetStoryPagesEqualLive() {
	all := storyActs("A", "B", "B", "B", "C", "D", "D", "E")
	all[2].Family, all[2].Body = sdk.FamilyState, sdk.StateChange{
		Party: sdk.Party{Kind: sdk.PartyMember, ID: "char-1"}, Change: sdk.StateConcentrationEnded,
		Concentration: &sdk.ConcentrationEnded{Spell: sdk.SpellRef{Ref: "dnd5e:spells:bless", Name: "Bless"}, Reason: "rest"},
	}
	s.seated()
	s.manager.EXPECT().Acts(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *sdk.ActsInput) (*sdk.ActsOutput, error) {
			return &sdk.ActsOutput{Acts: after(all, in.AfterID)}, nil
		}).AnyTimes()
	h := s.handler(sessionorch.NewActBroker(16))

	stream, done := s.run(h, "")
	live := make([]*sessionpb.Act, 0, len(all))
	for range all {
		live = append(live, stream.next(s.T()).GetAct())
	}
	s.cancel()
	s.NoError(s.ended(done))

	ctx := auth.WithPlayerID(context.Background(), "alice")
	var paged []*sessionpb.Act
	afterID := ""
	for {
		page, err := h.GetStory(ctx, &sessionpb.GetStoryRequest{Session: "sess-1", Member: "char-1", AfterId: afterID, Limit: 2})
		s.Require().NoError(err)
		paged = append(paged, page.GetActs()...)
		if !page.GetMore() {
			break
		}
		afterID = page.GetActs()[len(page.GetActs())-1].GetId()
	}

	s.Require().Len(paged, len(live))
	for i := range live {
		s.True(proto.Equal(live[i], paged[i]), "act %d: live %v, paged %v", i, live[i], paged[i])
	}
}

// TestRefusedActKeepsID: an act the SDK could not decode arrives refused,
// with its id, act and family kept and no body, and the act after it still
// arrives — on the page and on the stream (R22).
func (s *StoryHandlerSuite) TestRefusedActKeepsID() {
	all := storyActs("A", "B", "C")
	all[1].Act, all[1].Verb, all[1].Family = "speak:B", "speak", sdk.FamilyTableAnswer
	all[1].Body, all[1].Undecoded = nil, "answered: payload is not an object"
	s.seated()
	s.manager.EXPECT().Acts(gomock.Any(), gomock.Any()).Return(&sdk.ActsOutput{Acts: all}, nil).Times(2)
	h := s.handler(sessionorch.NewActBroker(8))

	page, err := h.GetStory(s.ctx, &sessionpb.GetStoryRequest{Session: "sess-1", Member: "char-1"})
	s.Require().NoError(err, "one bad act never fails the read")
	s.requireRefusedMiddle(page.GetActs())

	stream, done := s.run(h, "")
	var streamed []*sessionpb.Act
	for i := 0; i < 3; i++ {
		streamed = append(streamed, stream.next(s.T()).GetAct())
	}
	s.requireRefusedMiddle(streamed)
	s.cancel()
	s.NoError(s.ended(done))
}

func (s *StoryHandlerSuite) requireRefusedMiddle(acts []*sessionpb.Act) {
	s.Require().Len(acts, 3)
	s.Equal([]string{"A", "B", "C"}, ids(acts))
	bad := acts[1]
	s.True(bad.GetRefused())
	s.Contains(bad.GetRefusal(), "payload is not an object")
	s.Equal("speak:B", bad.GetAct())
	s.Equal(sessionpb.Family_FAMILY_TABLE_ANSWER, bad.GetFamily())
	s.Nil(bad.GetBody(), "a refused act carries no body")
	s.False(acts[2].GetRefused())
	s.NotNil(acts[2].GetClock(), "the act after it keeps its body")
}

func ids(acts []*sessionpb.Act) []string {
	out := make([]string, len(acts))
	for i, a := range acts {
		out[i] = a.GetId()
	}
	return out
}

// runEnds is each act's run_end, in order.
func runEnds(acts []*sessionpb.Act) []bool {
	out := make([]bool, len(acts))
	for i, a := range acts {
		out[i] = a.GetRunEnd()
	}
	return out
}
