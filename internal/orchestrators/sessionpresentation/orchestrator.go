package sessionpresentation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	repository "github.com/KirkDiggler/rpg-api/internal/repositories/sessionpresentation"
)

func New(repo repository.Repository) Service {
	return &orchestrator{repo: repo}
}

type orchestrator struct {
	repo repository.Repository
}

func (o *orchestrator) Publish(ctx context.Context, input *PublishInput) (*PublishOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("publish input is required: %w", ErrInvalidPlan)
	}
	if input.Session == "" {
		return nil, fmt.Errorf("publish session is required: %w", ErrInvalidPlan)
	}
	if input.Member == "" {
		return nil, fmt.Errorf("publish member is required: %w", ErrInvalidPlan)
	}

	normalizedDraft, err := ValidateDraft(&input.Draft)
	if err != nil {
		return nil, err
	}
	_, payload, err := bindPlan(input.Session, input.Member, normalizedDraft)
	if err != nil {
		return nil, err
	}

	accepted, err := o.repo.Publish(ctx, &repository.PublishInput{
		Session:        input.Session,
		PresentationID: normalizedDraft.PresentationID,
		Attempt:        normalizedDraft.Attempt,
		Payload:        payload,
	})
	if err != nil {
		if errors.Is(err, repository.ErrConflict) {
			return nil, ErrConflict
		}
		return nil, err
	}

	acceptedPlan, err := decodePlanPayload(accepted.Payload)
	if err != nil {
		return nil, fmt.Errorf("accepted plan payload invalid: %w", err)
	}
	if acceptedPlan.Session != input.Session || acceptedPlan.Roller != input.Member {
		return nil, fmt.Errorf("accepted plan payload mismatched publish identity: %w", ErrInvalidPlan)
	}

	return &PublishOutput{Plan: acceptedPlan}, nil
}

func (o *orchestrator) Subscribe(ctx context.Context, input *SubscribeInput) (Subscription, error) {
	if input == nil {
		return nil, fmt.Errorf("subscribe input is required: %w", ErrInvalidPlan)
	}
	if input.Session == "" {
		return nil, fmt.Errorf("subscribe session is required: %w", ErrInvalidPlan)
	}
	if input.Member == "" {
		return nil, fmt.Errorf("subscribe member is required: %w", ErrInvalidPlan)
	}

	inner, err := o.repo.Subscribe(ctx, &repository.SubscribeInput{Session: input.Session})
	if err != nil {
		return nil, err
	}

	subscription := &serviceSubscription{
		inner: inner,
		plans: make(chan Plan, 1),
		stop:  make(chan struct{}),
		done:  make(chan struct{}),
	}
	go subscription.run(ctx, input.Session, input.Member)
	return subscription, nil
}

type serviceSubscription struct {
	inner repository.Subscription
	plans chan Plan
	stop  chan struct{}
	done  chan struct{}

	closeMu sync.Mutex
	closed  bool
}

func (s *serviceSubscription) Plans() <-chan Plan {
	return s.plans
}

func (s *serviceSubscription) Close() error {
	s.closeMu.Lock()
	if s.closed {
		s.closeMu.Unlock()
		return ErrClosed
	}
	s.closed = true
	close(s.stop)
	s.closeMu.Unlock()

	err := s.inner.Close()
	<-s.done
	if err != nil {
		if errors.Is(err, repository.ErrClosed) {
			return ErrClosed
		}
		return err
	}
	return nil
}

func (s *serviceSubscription) run(ctx context.Context, sessionID, memberID string) {
	defer close(s.done)
	defer close(s.plans)

	for payload := range s.inner.Payloads() {
		plan, err := decodePlanPayload(payload)
		if err != nil {
			slog.WarnContext(ctx, "session presentation: dropping malformed subscription payload",
				"session", sessionID,
				"member", memberID,
				"error", err,
			)
			continue
		}
		if plan.Session != sessionID {
			slog.WarnContext(ctx, "session presentation: dropping payload for unexpected session",
				"session", sessionID,
				"member", memberID,
				"payload_session", plan.Session,
			)
			continue
		}

		select {
		case s.plans <- plan:
		case <-ctx.Done():
			return
		case <-s.stop:
			return
		}
	}
}

func bindPlan(sessionID, memberID string, draft Draft) (Plan, []byte, error) {
	plan := Plan{
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
	payload, err := marshalDeterministicPlan(plan)
	if err != nil {
		return Plan{}, nil, fmt.Errorf("marshal plan: %w", ErrInvalidPlan)
	}
	if len(payload) > maxEncodedPayloadBytes {
		return Plan{}, nil, fmt.Errorf("encoded plan too large: %w", ErrInvalidPlan)
	}
	return plan, payload, nil
}

func decodePlanPayload(payload []byte) (Plan, error) {
	var plan Plan
	if err := json.Unmarshal(payload, &plan); err != nil {
		return Plan{}, fmt.Errorf("decode plan payload: %w", ErrInvalidPlan)
	}
	validated, err := validatePlan(&plan)
	if err != nil {
		return Plan{}, err
	}
	return validated, nil
}

func validatePlan(plan *Plan) (Plan, error) {
	if plan == nil {
		return Plan{}, fmt.Errorf("plan is required: %w", ErrInvalidPlan)
	}
	if plan.Session == "" {
		return Plan{}, fmt.Errorf("plan session is required: %w", ErrInvalidPlan)
	}
	if plan.Roller == "" {
		return Plan{}, fmt.Errorf("plan roller is required: %w", ErrInvalidPlan)
	}

	draft, err := ValidateDraft(&Draft{
		SchemaVersion:       plan.SchemaVersion,
		PresentationID:      plan.PresentationID,
		AuthoritySeq:        plan.AuthoritySeq,
		Attempt:             plan.Attempt,
		PhysicsSchema:       plan.PhysicsSchema,
		ColliderFingerprint: plan.ColliderFingerprint,
		Bodies:              plan.Bodies,
		Contacts:            plan.Contacts,
		Terminal:            plan.Terminal,
	})
	if err != nil {
		return Plan{}, err
	}

	validated, payload, err := bindPlan(plan.Session, plan.Roller, draft)
	if err != nil {
		return Plan{}, err
	}
	if len(payload) > maxEncodedPayloadBytes {
		return Plan{}, fmt.Errorf("encoded plan too large: %w", ErrInvalidPlan)
	}
	return validated, nil
}

func marshalDeterministicPlan(plan Plan) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(plan); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte{'\n'}), nil
}
