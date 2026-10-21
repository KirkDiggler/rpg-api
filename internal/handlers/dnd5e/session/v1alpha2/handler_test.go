package sessionv1alpha2

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	sessionv1alpha2mock "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha2/mock"
	sessionaccess "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/sessionaccess"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
)

// recordStream captures what StreamRecord sends.
type recordStream struct {
	grpc.ServerStream
	ctx  context.Context
	sent chan *sessionpb.StreamRecordResponse
}

func newRecordStream(ctx context.Context) *recordStream {
	return &recordStream{ctx: ctx, sent: make(chan *sessionpb.StreamRecordResponse, 64)}
}

func (s *recordStream) Context() context.Context { return s.ctx }

func (s *recordStream) Send(m *sessionpb.StreamRecordResponse) error {
	s.sent <- m
	return nil
}

func (s *recordStream) next(t *testing.T) *sessionpb.StreamRecordResponse {
	t.Helper()
	select {
	case m := <-s.sent:
		return m
	case <-time.After(2 * time.Second):
		t.Fatal("nothing sent")
		return nil
	}
}

func beat(id string) sdk.RecordEvent {
	return sdk.RecordEvent{Session: "sess-1", ID: id, Beat: "moved", Act: "move:01HF", Fields: json.RawMessage(`{"beat":"moved"}`)}
}

func beats(ids ...string) []sdk.RecordEvent {
	out := make([]sdk.RecordEvent, len(ids))
	for i, id := range ids {
		out[i] = beat(id)
	}
	return out
}

type RecordHandlerSuite struct {
	suite.Suite
	ctrl    *gomock.Controller
	manager *sessionv1alpha2mock.MockManager
	gate    *sessionv1alpha2mock.MockGate
	ctx     context.Context
	cancel  context.CancelFunc
}

func TestRecordHandlerSuite(t *testing.T) {
	suite.Run(t, new(RecordHandlerSuite))
}

func (s *RecordHandlerSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.manager = sessionv1alpha2mock.NewMockManager(s.ctrl)
	s.gate = sessionv1alpha2mock.NewMockGate(s.ctrl)
	s.ctx, s.cancel = context.WithCancel(auth.WithPlayerID(context.Background(), "alice"))
}

func (s *RecordHandlerSuite) TearDownTest() { s.cancel() }

func (s *RecordHandlerSuite) handler(broker *sessionorch.RecordBroker) *Handler {
	h, err := New(&HandlerConfig{Manager: s.manager, Broker: broker, Gate: s.gate})
	s.Require().NoError(err)
	return h
}

func (s *RecordHandlerSuite) seated() {
	s.gate.EXPECT().CallerMemberSeated(gomock.Any(), "sess-1", "char-1").Return(nil)
}

func (s *RecordHandlerSuite) run(h *Handler, after string) (*recordStream, chan error) {
	stream := newRecordStream(s.ctx)
	done := make(chan error, 1)
	go func() {
		done <- h.StreamRecord(&sessionpb.StreamRecordRequest{Session: "sess-1", Member: "char-1", AfterId: after}, stream)
	}()
	return stream, done
}

func (s *RecordHandlerSuite) ended(done chan error) error {
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		s.FailNow("StreamRecord did not end")
		return nil
	}
}

// TestStreamRecordCatchUpThenLive: opened after beat 2 of 5, the stream
// replays 3 to 5, then goes live with 6 — and beat 5, which also arrives
// live because it was written between the subscribe and the read, is not
// sent twice.
func (s *RecordHandlerSuite) TestStreamRecordCatchUpThenLive() {
	broker := sessionorch.NewRecordBroker(8)
	s.seated()
	s.manager.EXPECT().Record(gomock.Any(), &sdk.RecordReadInput{Session: "sess-1", AfterID: "id-02"}).DoAndReturn(
		func(ctx context.Context, _ *sdk.RecordReadInput) (*sdk.RecordReadOutput, error) {
			// Subscribed before the read: these are queued, not missed.
			s.Require().NoError(broker.PublishRecord(ctx, beats("id-05", "id-06")))
			return &sdk.RecordReadOutput{Events: beats("id-03", "id-04", "id-05")}, nil
		})

	stream, done := s.run(s.handler(broker), "id-02")
	var got []string
	for i := 0; i < 4; i++ {
		got = append(got, stream.next(s.T()).GetEvent().GetId())
	}
	s.Equal([]string{"id-03", "id-04", "id-05", "id-06"}, got)
	select {
	case m := <-stream.sent:
		s.Failf("a duplicate went out", "%v", m)
	case <-time.After(100 * time.Millisecond):
	}
	s.cancel()
	s.NoError(s.ended(done))
}

// TestStreamRecordLaggedEndsWithStatus: a subscription the broker closes as
// lagged delivers what was queued, then RecordLagged with the last id sent,
// as its last message, and the stream ends OK.
func (s *RecordHandlerSuite) TestStreamRecordLaggedEndsWithStatus() {
	broker := sessionorch.NewRecordBroker(2)
	s.seated()
	s.manager.EXPECT().Record(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, _ *sdk.RecordReadInput) (*sdk.RecordReadOutput, error) {
			_ = broker.PublishRecord(ctx, beats("id-01", "id-02", "id-03")) // the third overflows
			return &sdk.RecordReadOutput{}, nil
		})

	stream, done := s.run(s.handler(broker), "")
	s.Equal("id-01", stream.next(s.T()).GetEvent().GetId())
	s.Equal("id-02", stream.next(s.T()).GetEvent().GetId())
	lagged := stream.next(s.T()).GetLagged()
	s.Require().NotNil(lagged, "the stream says it lagged")
	s.Equal("id-02", lagged.GetLastId())
	s.NoError(s.ended(done))
	s.Empty(stream.sent, "lagged is the last message")
}

// TestStreamRecordOlderThanRetentionEndsLagged: a resume point the record
// no longer reaches is answered as a lag at the requested id, so the client's
// GetRecord reports the restart.
func (s *RecordHandlerSuite) TestStreamRecordOlderThanRetentionEndsLagged() {
	s.seated()
	s.manager.EXPECT().Record(gomock.Any(), gomock.Any()).Return(
		&sdk.RecordReadOutput{Events: beats("id-09"), Restarted: true}, nil)

	stream, done := s.run(s.handler(sessionorch.NewRecordBroker(8)), "id-01")
	lagged := stream.next(s.T()).GetLagged()
	s.Require().NotNil(lagged)
	s.Equal("id-01", lagged.GetLastId())
	s.NoError(s.ended(done))
}

// TestStreamRecordRequiresSeat: a caller not seated as the member is refused
// PermissionDenied, the same code v1alpha1's StreamEvents gives, through the
// same sessionaccess gate.
func (s *RecordHandlerSuite) TestStreamRecordRequiresSeat() {
	chars := charactermock.NewMockRepository(s.ctrl)
	chars.EXPECT().Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).Return(
		&characterrepo.GetOutput{Character: &entities.Character{Data: &tkcharacter.Data{ID: "char-1", PlayerID: "alice"}}}, nil)
	gate, err := sessionaccess.New(chars, rosterOf("char-2"))
	s.Require().NoError(err)
	h, err := New(&HandlerConfig{Manager: s.manager, Broker: sessionorch.NewRecordBroker(8), Gate: gate})
	s.Require().NoError(err)

	err = h.StreamRecord(&sessionpb.StreamRecordRequest{Session: "sess-1", Member: "char-1"}, newRecordStream(s.ctx))
	s.Equal(codes.PermissionDenied, status.Code(err))
}

// TestGetRecordRestarted: a restarted read returns from the oldest retained
// beat with restarted set.
func (s *RecordHandlerSuite) TestGetRecordRestarted() {
	s.seated()
	s.manager.EXPECT().Record(gomock.Any(), &sdk.RecordReadInput{Session: "sess-1", AfterID: "id-01", Limit: 50}).Return(
		&sdk.RecordReadOutput{Events: beats("id-07", "id-08"), Restarted: true}, nil)

	out, err := s.handler(sessionorch.NewRecordBroker(8)).GetRecord(s.ctx,
		&sessionpb.GetRecordRequest{Session: "sess-1", Member: "char-1", AfterId: "id-01", Limit: 50})
	s.Require().NoError(err)
	s.True(out.GetRestarted())
	s.Require().Len(out.GetEvents(), 2)
	s.Equal("id-07", out.GetEvents()[0].GetId())
	s.Equal("move:01HF", out.GetEvents()[0].GetAct())
	s.Equal("moved", out.GetEvents()[0].GetBeat())
}

// TestGetRecordRequiresSeat: the gate's refusal is the RPC's answer.
func (s *RecordHandlerSuite) TestGetRecordRequiresSeat() {
	s.gate.EXPECT().CallerMemberSeated(gomock.Any(), "sess-1", "char-1").Return(status.Error(codes.PermissionDenied, "not seated"))
	_, err := s.handler(sessionorch.NewRecordBroker(8)).GetRecord(s.ctx,
		&sessionpb.GetRecordRequest{Session: "sess-1", Member: "char-1"})
	s.Equal(codes.PermissionDenied, status.Code(err))
}

// TestRecordFieldsToStruct: nested objects and arrays survive, and an
// integer reads back as a number.
func (s *RecordHandlerSuite) TestRecordFieldsToStruct() {
	evt := beat("id-01")
	evt.Fields = json.RawMessage(`{"beat":"struck","amount":6,"targets":["gate-1"],"attack":{"roll":14,"mods":[{"src":"str","v":3}]}}`)
	out, err := recordEventToProto(evt)
	s.Require().NoError(err)
	fields := out.GetFields().AsMap()
	s.Equal(float64(6), fields["amount"])
	s.Equal([]any{"gate-1"}, fields["targets"])
	attack := fields["attack"].(map[string]any)
	s.Equal(float64(14), attack["roll"])
	s.Equal("str", attack["mods"].([]any)[0].(map[string]any)["src"])
}

// TestUnreadableFieldsStillSend: a payload that is not a JSON object fails
// its fields, not the beat: id, beat and act still go out.
func (s *RecordHandlerSuite) TestUnreadableFieldsStillSend() {
	evt := beat("id-01")
	evt.Fields = json.RawMessage(`[1,2]`)
	out, err := recordEventToProto(evt)
	s.Error(err)
	s.Equal("id-01", out.GetId())
	s.Equal("moved", out.GetBeat())
	s.Nil(out.GetFields())
}

// TestNewRefusesMissingDependencies is the constructor's contract.
func (s *RecordHandlerSuite) TestNewRefusesMissingDependencies() {
	broker := sessionorch.NewRecordBroker(1)
	for name, cfg := range map[string]*HandlerConfig{
		"nil":     nil,
		"manager": {Broker: broker, Gate: s.gate},
		"broker":  {Manager: s.manager, Gate: s.gate},
		"gate":    {Manager: s.manager, Broker: broker},
	} {
		_, err := New(cfg)
		s.Error(err, name)
	}
}

// rosterOf answers every roster read with these players seated.
type fakeRoster struct{ members []string }

func rosterOf(members ...string) *fakeRoster { return &fakeRoster{members: members} }

func (f *fakeRoster) Roster(_ context.Context, _ *sdk.RosterInput) (*sdk.RosterOutput, error) {
	out := &sdk.RosterOutput{}
	for _, m := range f.members {
		out.Members = append(out.Members, sdk.PublicMember{ID: m, Kind: sdk.KindPlayer})
	}
	return out, nil
}
