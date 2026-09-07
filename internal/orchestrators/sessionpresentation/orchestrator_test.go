package sessionpresentation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	repository "github.com/KirkDiggler/rpg-api/internal/repositories/sessionpresentation"
)

type OrchestratorSuite struct {
	suite.Suite
	ctx  context.Context
	repo *fakeRepository
	svc  Service
}

func (s *OrchestratorSuite) SetupTest() {
	s.ctx = context.Background()
	s.repo = &fakeRepository{}
	s.svc = New(s.repo)
}

func (s *OrchestratorSuite) TestPublish_NormalizesDraftBindsServerFieldsAndReturnsAcceptedPlan() {
	draft := validTwoBodyDraft()
	expectedDraft, err := ValidateDraft(&draft)
	s.Require().NoError(err)
	expectedPlan := Plan{
		SchemaVersion:       expectedDraft.SchemaVersion,
		Session:             "session-1",
		PresentationID:      expectedDraft.PresentationID,
		AuthoritySeq:        expectedDraft.AuthoritySeq,
		Roller:              "member-1",
		Attempt:             expectedDraft.Attempt,
		PhysicsSchema:       expectedDraft.PhysicsSchema,
		ColliderFingerprint: bytes.Clone(expectedDraft.ColliderFingerprint),
		Bodies:              append([]BodyInitial(nil), expectedDraft.Bodies...),
		Contacts:            append([]ContactCheckpoint(nil), expectedDraft.Contacts...),
		Terminal:            append([]BodyTerminal(nil), expectedDraft.Terminal...),
	}
	expectedPayload := mustMarshalPlan(s.T(), expectedPlan)
	s.repo.publishFn = func(_ context.Context, input *repository.PublishInput) (*repository.PublishOutput, error) {
		s.Require().Equal("session-1", input.Session)
		s.Require().Equal(expectedDraft.PresentationID, input.PresentationID)
		s.Require().Equal(expectedDraft.Attempt, input.Attempt)
		s.Require().Equal(expectedPayload, input.Payload)
		return &repository.PublishOutput{Payload: bytes.Clone(input.Payload)}, nil
	}

	out, err := s.svc.Publish(s.ctx, &PublishInput{Session: "session-1", Member: "member-1", Draft: draft})
	s.Require().NoError(err)
	s.Require().Equal(expectedPlan, out.Plan)
}

func (s *OrchestratorSuite) TestPublish_RejectsFinalPlanLargerThan64KiBWithoutCallingRepository() {
	draft := validDraft()
	var called atomic.Bool
	s.repo.publishFn = func(_ context.Context, _ *repository.PublishInput) (*repository.PublishOutput, error) {
		called.Store(true)
		return nil, nil
	}

	_, err := s.svc.Publish(s.ctx, &PublishInput{
		Session: string(bytes.Repeat([]byte("s"), maxEncodedPayloadBytes)),
		Member:  "member-1",
		Draft:   draft,
	})
	s.Require().Error(err)
	s.Require().ErrorIs(err, ErrInvalidPlan)
	s.Require().False(called.Load())
}

func (s *OrchestratorSuite) TestPublish_PropagatesRepositoryConflict() {
	draft := validDraft()
	s.repo.publishFn = func(_ context.Context, _ *repository.PublishInput) (*repository.PublishOutput, error) {
		return nil, repository.ErrConflict
	}

	_, err := s.svc.Publish(s.ctx, &PublishInput{Session: "session-1", Member: "member-1", Draft: draft})
	s.Require().ErrorIs(err, ErrConflict)
}

func (s *OrchestratorSuite) TestSubscribe_DropsMalformedAndInvalidPayloadsAndDeliversValidPlans() {
	payloads := make(chan []byte, 3)
	s.repo.subscribeFn = func(_ context.Context, input *repository.SubscribeInput) (repository.Subscription, error) {
		s.Require().Equal("session-1", input.Session)
		return &fakeSubscription{payloads: payloads}, nil
	}
	sub, err := s.svc.Subscribe(s.ctx, &SubscribeInput{Session: "session-1", Member: "member-1"})
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = sub.Close() })

	payloads <- []byte("{not-json")
	invalidPlan := validPlanForSession("session-1", "member-1")
	invalidPlan.Terminal = invalidPlan.Terminal[:0]
	payloads <- mustMarshalPlan(s.T(), invalidPlan)
	validPlan := validPlanForSession("session-1", "member-1")
	payloads <- mustMarshalPlan(s.T(), validPlan)

	select {
	case plan, ok := <-sub.Plans():
		s.Require().True(ok)
		s.Require().Equal(validPlan, plan)
	case <-time.After(2 * time.Second):
		s.T().Fatal("timed out waiting for valid plan")
	}
}

func (s *OrchestratorSuite) TestSubscribe_MapsRepositoryErrClosed() {
	s.repo.subscribeFn = func(_ context.Context, _ *repository.SubscribeInput) (repository.Subscription, error) {
		return nil, repository.ErrClosed
	}

	_, err := s.svc.Subscribe(s.ctx, &SubscribeInput{Session: "session-1", Member: "member-1"})
	s.Require().ErrorIs(err, ErrClosed)
}

func (s *OrchestratorSuite) TestSubscribe_CloseClosesPlanChannelAndPropagatesErrClosed() {
	payloads := make(chan []byte)
	fakeSub := &fakeSubscription{payloads: payloads, closeErrAfterFirst: repository.ErrClosed}
	s.repo.subscribeFn = func(_ context.Context, _ *repository.SubscribeInput) (repository.Subscription, error) {
		return fakeSub, nil
	}

	sub, err := s.svc.Subscribe(s.ctx, &SubscribeInput{Session: "session-1", Member: "member-1"})
	s.Require().NoError(err)
	s.Require().NoError(sub.Close())
	waitForPlanChannelClosed(s.T(), sub.Plans())
	s.Require().ErrorIs(sub.Close(), ErrClosed)
}

func TestOrchestratorSuite(t *testing.T) {
	suite.Run(t, new(OrchestratorSuite))
}

type fakeRepository struct {
	publishFn   func(context.Context, *repository.PublishInput) (*repository.PublishOutput, error)
	subscribeFn func(context.Context, *repository.SubscribeInput) (repository.Subscription, error)
}

func (f *fakeRepository) Publish(ctx context.Context, input *repository.PublishInput) (*repository.PublishOutput, error) {
	if f.publishFn != nil {
		return f.publishFn(ctx, input)
	}
	return nil, errors.New("unexpected Publish call")
}

func (f *fakeRepository) Subscribe(ctx context.Context, input *repository.SubscribeInput) (repository.Subscription, error) {
	if f.subscribeFn != nil {
		return f.subscribeFn(ctx, input)
	}
	return nil, errors.New("unexpected Subscribe call")
}

type fakeSubscription struct {
	payloads           chan []byte
	closeCalls         atomic.Int32
	closeErrAfterFirst error
}

func (f *fakeSubscription) Payloads() <-chan []byte {
	return f.payloads
}

func (f *fakeSubscription) Close() error {
	if f.closeCalls.Add(1) == 1 {
		close(f.payloads)
		return nil
	}
	if f.closeErrAfterFirst != nil {
		return f.closeErrAfterFirst
	}
	return nil
}

func mustMarshalPlan(t *testing.T, plan Plan) []byte {
	t.Helper()
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(plan); err != nil {
		t.Fatalf("marshal plan: %v", err)
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte{'\n'})
}

func validPlanForSession(sessionID, memberID string) Plan {
	draft := validTwoBodyDraft()
	return Plan{
		SchemaVersion:       draft.SchemaVersion,
		Session:             sessionID,
		PresentationID:      draft.PresentationID,
		AuthoritySeq:        draft.AuthoritySeq,
		Roller:              memberID,
		Attempt:             draft.Attempt,
		PhysicsSchema:       draft.PhysicsSchema,
		ColliderFingerprint: bytes.Clone(draft.ColliderFingerprint),
		Bodies:              append([]BodyInitial(nil), draft.Bodies...),
		Contacts:            append([]ContactCheckpoint(nil), draft.Contacts...),
		Terminal:            append([]BodyTerminal(nil), draft.Terminal...),
	}
}

func waitForPlanChannelClosed(t *testing.T, ch <-chan Plan) {
	t.Helper()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected closed plan channel")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for closed plan channel")
	}
}
