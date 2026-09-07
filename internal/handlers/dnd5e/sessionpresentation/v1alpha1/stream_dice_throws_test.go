package sessionpresentationv1alpha1

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	presentationpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/presentation/v1alpha1"
	orchsessionpresentation "github.com/KirkDiggler/rpg-api/internal/orchestrators/sessionpresentation"
	sessionpresentationmock "github.com/KirkDiggler/rpg-api/internal/orchestrators/sessionpresentation/mock"
)

func (s *HandlerSuite) TestStreamDiceThrows_AccessRunsBeforeSubscribeAndForwardsExactPlans() {
	order := &callOrderRecorder{}
	access := s.ownedAccess(order)
	service := sessionpresentationmock.NewMockService(s.ctrl)
	plans := make(chan orchsessionpresentation.Plan, 1)
	sub := &fakeSubscription{plans: plans}
	subscribed := make(chan struct{})
	service.EXPECT().Subscribe(gomock.Any(), &orchsessionpresentation.SubscribeInput{Session: s.sessionID, Member: s.memberID}).DoAndReturn(
		func(_ context.Context, _ *orchsessionpresentation.SubscribeInput) (orchsessionpresentation.Subscription, error) {
			order.Add("service.Subscribe")
			close(subscribed)
			return sub, nil
		},
	)

	stream := newFakePlanStream(s.ctx)
	h := s.newHandler(service, access)
	done := make(chan error, 1)
	go func() {
		done <- h.StreamDiceThrows(&presentationpb.StreamDiceThrowsRequest{Session: s.sessionID, Member: s.memberID}, stream)
	}()

	select {
	case <-subscribed:
	case <-time.After(2 * time.Second):
		s.T().Fatal("timed out waiting for subscribe")
	}
	plans <- s.testDomainPlan()
	close(plans)

	got := stream.WaitForSend(s.T())
	s.True(proto.Equal(s.testProtoPlan(), got), "expected exact streamed plan conversion")
	s.Require().NoError(<-done)
	s.Equal([]string{"characters.Get", "roster.Roster", "service.Subscribe"}, order.Steps())
	s.Equal(int32(1), sub.closeCalls.Load())
}

func (s *HandlerSuite) TestStreamDiceThrows_SubscribeErrorsMapToInternal() {
	tests := []struct {
		name        string
		err         error
		wantMessage string
	}{
		{name: "closed", err: fmt.Errorf("broker detail leaked: %w", orchsessionpresentation.ErrClosed), wantMessage: "session presentation unavailable"},
		{name: "internal", err: errors.New("subscribe failed"), wantMessage: "session presentation unavailable"},
	}

	for _, tc := range tests {
		s.Run(tc.name, func() {
			access := s.ownedAccess(nil)
			service := sessionpresentationmock.NewMockService(s.ctrl)
			service.EXPECT().Subscribe(gomock.Any(), gomock.Any()).Return(nil, tc.err)

			h := s.newHandler(service, access)
			err := h.StreamDiceThrows(&presentationpb.StreamDiceThrowsRequest{Session: s.sessionID, Member: s.memberID}, newFakePlanStream(s.ctx))
			s.Require().Error(err)
			st := status.Convert(err)
			s.Equal(codes.Internal, st.Code())
			s.Equal(tc.wantMessage, st.Message())
		})
	}
}

func (s *HandlerSuite) TestStreamDiceThrows_AccessRefusalSkipsSubscribe() {
	access := s.foreignAccess()
	service := sessionpresentationmock.NewMockService(s.ctrl)
	service.EXPECT().Subscribe(gomock.Any(), gomock.Any()).Times(0)

	h := s.newHandler(service, access)
	err := h.StreamDiceThrows(&presentationpb.StreamDiceThrowsRequest{Session: s.sessionID, Member: s.memberID}, newFakePlanStream(s.ctx))
	s.Require().Error(err)
	s.Equal(codes.PermissionDenied, status.Code(err))
}

func (s *HandlerSuite) TestStreamDiceThrows_CancellationReturnsNilAndClosesSubscription() {
	access := s.ownedAccess(nil)
	service := sessionpresentationmock.NewMockService(s.ctrl)
	plans := make(chan orchsessionpresentation.Plan)
	sub := &fakeSubscription{plans: plans}
	subscribed := make(chan struct{})
	service.EXPECT().Subscribe(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ *orchsessionpresentation.SubscribeInput) (orchsessionpresentation.Subscription, error) {
			close(subscribed)
			return sub, nil
		},
	)

	ctx, cancel := context.WithCancel(s.ctx)
	stream := newFakePlanStream(ctx)
	h := s.newHandler(service, access)
	done := make(chan error, 1)
	go func() {
		done <- h.StreamDiceThrows(&presentationpb.StreamDiceThrowsRequest{Session: s.sessionID, Member: s.memberID}, stream)
	}()

	select {
	case <-subscribed:
	case <-time.After(2 * time.Second):
		s.T().Fatal("timed out waiting for subscribe")
	}
	cancel()

	select {
	case err := <-done:
		s.Require().NoError(err)
	case <-time.After(2 * time.Second):
		s.T().Fatal("timed out waiting for cancellation return")
	}
	s.Equal(int32(1), sub.closeCalls.Load())
}

func (s *HandlerSuite) TestStreamDiceThrows_SendFailureReturnsTransportError() {
	access := s.ownedAccess(nil)
	service := sessionpresentationmock.NewMockService(s.ctrl)
	plans := make(chan orchsessionpresentation.Plan, 1)
	sub := &fakeSubscription{plans: plans}
	subscribed := make(chan struct{})
	service.EXPECT().Subscribe(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ *orchsessionpresentation.SubscribeInput) (orchsessionpresentation.Subscription, error) {
			close(subscribed)
			return sub, nil
		},
	)

	sendErr := errors.New("transport send failed")
	stream := newFakePlanStream(s.ctx)
	stream.sendErr = sendErr
	h := s.newHandler(service, access)
	done := make(chan error, 1)
	go func() {
		done <- h.StreamDiceThrows(&presentationpb.StreamDiceThrowsRequest{Session: s.sessionID, Member: s.memberID}, stream)
	}()

	select {
	case <-subscribed:
	case <-time.After(2 * time.Second):
		s.T().Fatal("timed out waiting for subscribe")
	}
	plans <- s.testDomainPlan()

	select {
	case err := <-done:
		s.ErrorIs(err, sendErr)
	case <-time.After(2 * time.Second):
		s.T().Fatal("timed out waiting for send failure")
	}
	s.Equal(int32(1), sub.closeCalls.Load())
}

func (s *HandlerSuite) TestStreamDiceThrows_NilSubscriptionIsInternal() {
	access := s.ownedAccess(nil)
	service := sessionpresentationmock.NewMockService(s.ctrl)
	service.EXPECT().Subscribe(gomock.Any(), gomock.Any()).Return(nil, nil)

	h := s.newHandler(service, access)
	err := h.StreamDiceThrows(&presentationpb.StreamDiceThrowsRequest{Session: s.sessionID, Member: s.memberID}, newFakePlanStream(s.ctx))
	s.Require().Error(err)
	s.Equal(codes.Internal, status.Code(err))
}
