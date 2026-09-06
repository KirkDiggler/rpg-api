package compositionv1alpha1

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	compositionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/api/composition/v1alpha1"

	worldcomposition "github.com/KirkDiggler/rpg-toolkit/world/composition"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	compositionservice "github.com/KirkDiggler/rpg-api/internal/services/composition"
	compositionmock "github.com/KirkDiggler/rpg-api/internal/services/composition/mock"
)

type HandlerSuite struct {
	suite.Suite
	service *compositionmock.MockService
	handler *Handler
	ctx     context.Context
}

func (s *HandlerSuite) SetupTest() {
	ctrl := gomock.NewController(s.T())
	s.service = compositionmock.NewMockService(ctrl)
	handler, err := New(&HandlerConfig{
		Service:          s.service,
		WorldID:          "test-world",
		AuthoringEnabled: true,
	})
	s.Require().NoError(err)
	s.handler = handler
	s.ctx = auth.WithPlayerID(context.Background(), "player-1")
}

func (s *HandlerSuite) TestCreateMapsRequestAndResponse() {
	data := &worldcomposition.Data{
		ID:      "composition-1",
		WorldID: "test-world",
		JSON:    json.RawMessage(`{"name":"proof composition"}`),
	}
	s.service.EXPECT().Create(gomock.Any(), &compositionservice.CreateInput{
		PlayerID: "player-1",
		WorldID:  "test-world",
		JSON:     json.RawMessage(`{"name":"proof composition"}`),
	}).Return(&compositionservice.CreateOutput{Composition: data}, nil)

	response, err := s.handler.CreateComposition(s.ctx, &compositionpb.CreateCompositionRequest{
		WorldId: "test-world",
		Json:    `{"name":"proof composition"}`,
	})
	s.Require().NoError(err)
	s.Equal("composition-1", response.GetComposition().GetId())
	s.Equal("test-world", response.GetComposition().GetWorldId())
	s.JSONEq(`{"name":"proof composition"}`, response.GetComposition().GetJson())
}

func (s *HandlerSuite) TestGetAndListMapRequestsAndResponses() {
	first := &worldcomposition.Data{ID: "composition-1", WorldID: "test-world", JSON: json.RawMessage(`{"name":"first"}`)}
	second := &worldcomposition.Data{ID: "composition-2", WorldID: "test-world", JSON: json.RawMessage(`{"name":"second"}`)}
	s.service.EXPECT().Get(gomock.Any(), &compositionservice.GetInput{
		PlayerID:      "player-1",
		WorldID:       "test-world",
		CompositionID: "composition-1",
	}).Return(&compositionservice.GetOutput{Composition: first}, nil)

	got, err := s.handler.GetComposition(s.ctx, &compositionpb.GetCompositionRequest{
		WorldId: "test-world",
		Id:      "composition-1",
	})
	s.Require().NoError(err)
	s.Equal("composition-1", got.GetComposition().GetId())
	s.JSONEq(`{"name":"first"}`, got.GetComposition().GetJson())

	s.service.EXPECT().List(gomock.Any(), &compositionservice.ListInput{
		PlayerID: "player-1",
		WorldID:  "test-world",
	}).Return(&compositionservice.ListOutput{Compositions: []*worldcomposition.Data{first, second}}, nil)
	listed, err := s.handler.ListCompositions(s.ctx, &compositionpb.ListCompositionsRequest{WorldId: "test-world"})
	s.Require().NoError(err)
	s.Len(listed.GetCompositions(), 2)
	s.Equal("composition-1", listed.GetCompositions()[0].GetId())
	s.Equal("composition-2", listed.GetCompositions()[1].GetId())
}

func (s *HandlerSuite) TestMissingPlayerAndWorldMismatchRefuseBeforeService() {
	calls := []func(context.Context, string) error{
		func(ctx context.Context, worldID string) error {
			_, err := s.handler.CreateComposition(ctx, &compositionpb.CreateCompositionRequest{WorldId: worldID, Json: `{}`})
			return err
		},
		func(ctx context.Context, worldID string) error {
			_, err := s.handler.GetComposition(ctx, &compositionpb.GetCompositionRequest{WorldId: worldID, Id: "composition-1"})
			return err
		},
		func(ctx context.Context, worldID string) error {
			_, err := s.handler.ListCompositions(ctx, &compositionpb.ListCompositionsRequest{WorldId: worldID})
			return err
		},
	}

	for _, call := range calls {
		s.Equal(codes.Unauthenticated, status.Code(call(context.Background(), "test-world")))
		s.Equal(codes.PermissionDenied, status.Code(call(s.ctx, "another-world")))
		s.Equal(codes.InvalidArgument, status.Code(call(s.ctx, "")))
	}
}

func (s *HandlerSuite) TestCreateHonorsAuthoringGateAndValidatesJSON() {
	disabled, err := New(&HandlerConfig{Service: s.service, WorldID: "test-world"})
	s.Require().NoError(err)
	_, err = disabled.CreateComposition(s.ctx, &compositionpb.CreateCompositionRequest{WorldId: "test-world", Json: `{}`})
	s.Equal(codes.FailedPrecondition, status.Code(err))

	_, err = s.handler.CreateComposition(s.ctx, &compositionpb.CreateCompositionRequest{WorldId: "test-world"})
	s.Equal(codes.InvalidArgument, status.Code(err))
	_, err = s.handler.CreateComposition(s.ctx, &compositionpb.CreateCompositionRequest{WorldId: "test-world", Json: `{`})
	s.Equal(codes.InvalidArgument, status.Code(err))
}

func (s *HandlerSuite) TestValidatesGetID() {
	_, err := s.handler.GetComposition(s.ctx, &compositionpb.GetCompositionRequest{WorldId: "test-world"})
	s.Equal(codes.InvalidArgument, status.Code(err))
}

func (s *HandlerSuite) TestMapsServiceErrorsToGRPC() {
	s.service.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil, apierr.AlreadyExists("duplicate"))
	_, err := s.handler.CreateComposition(s.ctx, &compositionpb.CreateCompositionRequest{WorldId: "test-world", Json: `{}`})
	s.Equal(codes.AlreadyExists, status.Code(err))

	s.service.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, apierr.NotFound("missing"))
	_, err = s.handler.GetComposition(s.ctx, &compositionpb.GetCompositionRequest{WorldId: "test-world", Id: "missing"})
	s.Equal(codes.NotFound, status.Code(err))

	s.service.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, errors.New("storage failed"))
	_, err = s.handler.ListCompositions(s.ctx, &compositionpb.ListCompositionsRequest{WorldId: "test-world"})
	s.Equal(codes.Internal, status.Code(err))
}

func (s *HandlerSuite) TestMissingServiceOutputsReturnInternal() {
	s.service.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil, nil)
	_, err := s.handler.CreateComposition(s.ctx, &compositionpb.CreateCompositionRequest{WorldId: "test-world", Json: `{}`})
	s.Equal(codes.Internal, status.Code(err))

	s.service.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, nil)
	_, err = s.handler.GetComposition(s.ctx, &compositionpb.GetCompositionRequest{WorldId: "test-world", Id: "composition-1"})
	s.Equal(codes.Internal, status.Code(err))

	s.service.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, nil)
	_, err = s.handler.ListCompositions(s.ctx, &compositionpb.ListCompositionsRequest{WorldId: "test-world"})
	s.Equal(codes.Internal, status.Code(err))
}

func TestHandlerSuite(t *testing.T) {
	suite.Run(t, new(HandlerSuite))
}

func TestNewValidation(t *testing.T) {
	t.Parallel()

	_, err := New(nil)
	require.Error(t, err)

	_, err = New(&HandlerConfig{})
	require.Error(t, err)

	ctrl := gomock.NewController(t)
	_, err = New(&HandlerConfig{Service: compositionmock.NewMockService(ctrl)})
	require.Error(t, err)
}
