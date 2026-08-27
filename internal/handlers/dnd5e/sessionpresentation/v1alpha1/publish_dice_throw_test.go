package sessionpresentationv1alpha1

import (
	"errors"

	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	presentationpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/presentation/v1alpha1"
	orchsessionpresentation "github.com/KirkDiggler/rpg-api/internal/orchestrators/sessionpresentation"
	sessionpresentationmock "github.com/KirkDiggler/rpg-api/internal/orchestrators/sessionpresentation/mock"
)

func (s *HandlerSuite) TestNew_RequiresServiceAndAccess() {
	_, err := New(nil)
	s.Require().Error(err)

	_, err = New(&HandlerConfig{})
	s.Require().Error(err)

	_, err = New(&HandlerConfig{Service: sessionpresentationmock.NewMockService(s.ctrl)})
	s.Require().Error(err)
}

func (s *HandlerSuite) TestPublishDiceThrow_AccessRunsBeforeServiceAndConvertsExactly() {
	order := &callOrderRecorder{}
	access := s.ownedAccess(order)
	service := sessionpresentationmock.NewMockService(s.ctrl)
	service.EXPECT().Publish(s.ctx, &orchsessionpresentation.PublishInput{
		Session: s.sessionID,
		Member:  s.memberID,
		Draft:   s.testDomainDraft(),
	}).DoAndReturn(func(_ interface{}, _ *orchsessionpresentation.PublishInput) (*orchsessionpresentation.PublishOutput, error) {
		order.Add("service.Publish")
		return &orchsessionpresentation.PublishOutput{Plan: s.testDomainPlan()}, nil
	})

	h := s.newHandler(service, access)
	resp, err := h.PublishDiceThrow(s.ctx, &presentationpb.PublishDiceThrowRequest{
		Session: s.sessionID,
		Member:  s.memberID,
		Draft:   s.testProtoDraft(),
	})
	s.Require().NoError(err)
	s.True(proto.Equal(s.testProtoPlan(), resp.GetPlan()), "expected exact published plan conversion")
	s.Equal([]string{"characters.Get", "roster.Get", "service.Publish"}, order.Steps())
}

func (s *HandlerSuite) TestPublishDiceThrow_ServiceErrorsMapToStatuses() {
	tests := []struct {
		name string
		err  error
		want codes.Code
	}{
		{name: "invalid", err: orchsessionpresentation.ErrInvalidPlan, want: codes.InvalidArgument},
		{name: "conflict", err: orchsessionpresentation.ErrConflict, want: codes.AlreadyExists},
		{name: "internal", err: errors.New("storage blew up"), want: codes.Internal},
	}

	for _, tc := range tests {
		s.Run(tc.name, func() {
			access := s.ownedAccess(nil)
			service := sessionpresentationmock.NewMockService(s.ctrl)
			service.EXPECT().Publish(s.ctx, gomock.Any()).Return(nil, tc.err)

			h := s.newHandler(service, access)
			_, err := h.PublishDiceThrow(s.ctx, &presentationpb.PublishDiceThrowRequest{
				Session: s.sessionID,
				Member:  s.memberID,
				Draft:   s.testProtoDraft(),
			})
			s.Require().Error(err)
			s.Equal(tc.want, status.Code(err))
		})
	}
}

func (s *HandlerSuite) TestPublishDiceThrow_NilOutputIsInternal() {
	access := s.ownedAccess(nil)
	service := sessionpresentationmock.NewMockService(s.ctrl)
	service.EXPECT().Publish(s.ctx, gomock.Any()).Return(nil, nil)

	h := s.newHandler(service, access)
	_, err := h.PublishDiceThrow(s.ctx, &presentationpb.PublishDiceThrowRequest{
		Session: s.sessionID,
		Member:  s.memberID,
		Draft:   s.testProtoDraft(),
	})
	s.Require().Error(err)
	s.Equal(codes.Internal, status.Code(err))
}
