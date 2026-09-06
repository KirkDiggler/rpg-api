package composition

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	worldcomposition "github.com/KirkDiggler/rpg-toolkit/world/composition"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	idgenmock "github.com/KirkDiggler/rpg-api/internal/pkg/idgen/mock"
	compositionrepo "github.com/KirkDiggler/rpg-api/internal/repositories/composition"
	compositionrepomock "github.com/KirkDiggler/rpg-api/internal/repositories/composition/mock"
	compositionservice "github.com/KirkDiggler/rpg-api/internal/services/composition"
)

type OrchestratorSuite struct {
	suite.Suite
	repository  *compositionrepomock.MockRepository
	idGenerator *idgenmock.MockGenerator
	service     compositionservice.Service
}

func (s *OrchestratorSuite) SetupTest() {
	ctrl := gomock.NewController(s.T())
	s.repository = compositionrepomock.NewMockRepository(ctrl)
	s.idGenerator = idgenmock.NewMockGenerator(ctrl)
	service, err := New(&Config{Repository: s.repository, IDGenerator: s.idGenerator})
	s.Require().NoError(err)
	s.service = service
}

func (s *OrchestratorSuite) TestCreateMintsIDAndPersistsSnapshot() {
	input := &compositionservice.CreateInput{
		PlayerID: "player-1",
		WorldID:  "world-1",
		JSON:     json.RawMessage(`{"name":"library chair"}`),
	}
	expected := &worldcomposition.Data{
		ID:      "composition-1",
		WorldID: "world-1",
		JSON:    json.RawMessage(`{"name":"library chair"}`),
	}

	s.idGenerator.EXPECT().Generate().Return("composition-1")
	s.repository.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, repoInput *compositionrepo.CreateInput) (*compositionrepo.CreateOutput, error) {
			s.Equal(expected, repoInput.Composition)
			s.NotSame(&input.JSON[0], &repoInput.Composition.JSON[0])
			return &compositionrepo.CreateOutput{Composition: expected}, nil
		},
	)

	output, err := s.service.Create(context.Background(), input)
	s.Require().NoError(err)
	s.Equal(expected, output.Composition)
}

func (s *OrchestratorSuite) TestGetAndListUseTypedRepositoryInputs() {
	composition := &worldcomposition.Data{ID: "composition-1", WorldID: "world-1", JSON: json.RawMessage(`{}`)}
	s.repository.EXPECT().Get(gomock.Any(), &compositionrepo.GetInput{
		WorldID: "world-1",
		ID:      "composition-1",
	}).Return(&compositionrepo.GetOutput{Composition: composition}, nil)

	got, err := s.service.Get(context.Background(), &compositionservice.GetInput{
		PlayerID:      "player-1",
		WorldID:       "world-1",
		CompositionID: "composition-1",
	})
	s.Require().NoError(err)
	s.Equal(composition, got.Composition)

	s.repository.EXPECT().List(gomock.Any(), &compositionrepo.ListInput{WorldID: "world-1"}).
		Return(&compositionrepo.ListOutput{Compositions: []*worldcomposition.Data{composition}}, nil)
	listed, err := s.service.List(context.Background(), &compositionservice.ListInput{
		PlayerID: "player-1",
		WorldID:  "world-1",
	})
	s.Require().NoError(err)
	s.Equal([]*worldcomposition.Data{composition}, listed.Compositions)
}

func (s *OrchestratorSuite) TestRepositoryErrorsKeepTheirAPIClassification() {
	s.idGenerator.EXPECT().Generate().Return("composition-1")
	s.repository.EXPECT().Create(gomock.Any(), gomock.Any()).
		Return(nil, apierr.AlreadyExists("duplicate"))

	_, err := s.service.Create(context.Background(), &compositionservice.CreateInput{
		PlayerID: "player-1",
		WorldID:  "world-1",
		JSON:     json.RawMessage(`{}`),
	})
	s.Require().Error(err)
	s.True(apierr.IsAlreadyExists(err), "got %v", err)

	s.repository.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, apierr.NotFound("missing"))
	_, err = s.service.Get(context.Background(), &compositionservice.GetInput{
		PlayerID:      "player-1",
		WorldID:       "world-1",
		CompositionID: "missing",
	})
	s.Require().Error(err)
	s.True(apierr.IsNotFound(err), "got %v", err)

	s.repository.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, errors.New("redis unavailable"))
	_, err = s.service.List(context.Background(), &compositionservice.ListInput{PlayerID: "player-1", WorldID: "world-1"})
	s.Require().Error(err)
	s.True(apierr.IsInternal(err), "got %v", err)
}

func (s *OrchestratorSuite) TestRejectsInvalidInputsBeforeRepositoryAccess() {
	createInputs := []*compositionservice.CreateInput{
		nil,
		{WorldID: "world-1", JSON: json.RawMessage(`{}`)},
		{PlayerID: "player-1", JSON: json.RawMessage(`{}`)},
		{PlayerID: "player-1", WorldID: "world-1"},
		{PlayerID: "player-1", WorldID: "world-1", JSON: json.RawMessage(`{`)},
	}
	for _, input := range createInputs {
		_, err := s.service.Create(context.Background(), input)
		s.Require().Error(err)
	}

	getInputs := []*compositionservice.GetInput{
		nil,
		{WorldID: "world-1", CompositionID: "composition-1"},
		{PlayerID: "player-1", CompositionID: "composition-1"},
		{PlayerID: "player-1", WorldID: "world-1"},
	}
	for _, input := range getInputs {
		_, err := s.service.Get(context.Background(), input)
		s.Require().Error(err)
	}

	listInputs := []*compositionservice.ListInput{
		nil,
		{WorldID: "world-1"},
		{PlayerID: "player-1"},
	}
	for _, input := range listInputs {
		_, err := s.service.List(context.Background(), input)
		s.Require().Error(err)
	}
}

func (s *OrchestratorSuite) TestRejectsEmptyGeneratedIDAndMissingRepositoryOutputs() {
	s.idGenerator.EXPECT().Generate().Return("")
	_, err := s.service.Create(context.Background(), &compositionservice.CreateInput{
		PlayerID: "player-1", WorldID: "world-1", JSON: json.RawMessage(`{}`),
	})
	s.Require().Error(err)
	s.True(apierr.IsInternal(err), "got %v", err)

	s.repository.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, nil)
	_, err = s.service.Get(context.Background(), &compositionservice.GetInput{
		PlayerID: "player-1", WorldID: "world-1", CompositionID: "composition-1",
	})
	s.Require().Error(err)
	s.True(apierr.IsInternal(err), "got %v", err)

	s.repository.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, nil)
	_, err = s.service.List(context.Background(), &compositionservice.ListInput{PlayerID: "player-1", WorldID: "world-1"})
	s.Require().Error(err)
	s.True(apierr.IsInternal(err), "got %v", err)
}

func TestOrchestratorSuite(t *testing.T) {
	suite.Run(t, new(OrchestratorSuite))
}

func TestNewValidation(t *testing.T) {
	t.Parallel()

	_, err := New(nil)
	require.Error(t, err)

	_, err = New(&Config{})
	require.Error(t, err)

	ctrl := gomock.NewController(t)
	_, err = New(&Config{Repository: compositionrepomock.NewMockRepository(ctrl)})
	require.Error(t, err)
}
