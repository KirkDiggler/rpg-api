package character_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/errors"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	externalmock "github.com/KirkDiggler/rpg-api/internal/clients/external/mock"
	dicemock "github.com/KirkDiggler/rpg-api/internal/orchestrators/dice/mock"
	idgenmock "github.com/KirkDiggler/rpg-api/internal/pkg/idgen/mock"
	characterrepomock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	draftrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
	draftrepomock "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft/mock"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

type OrchestratorTestSuite struct {
	suite.Suite
	ctrl               *gomock.Controller
	mockCharRepo       *characterrepomock.MockRepository
	mockDraftRepo      *draftrepomock.MockRepository
	mockExternalClient *externalmock.MockClient
	mockDiceService    *dicemock.MockService
	mockIDGenerator    *idgenmock.MockGenerator
	orchestrator       *character.Orchestrator
	ctx                context.Context
	
	// Test data
	testDraftData    *toolkitchar.DraftData
	testDraftID      string
	testPlayerID     string
}

func (s *OrchestratorTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockCharRepo = characterrepomock.NewMockRepository(s.ctrl)
	s.mockDraftRepo = draftrepomock.NewMockRepository(s.ctrl)
	s.mockExternalClient = externalmock.NewMockClient(s.ctrl)
	s.mockDiceService = dicemock.NewMockService(s.ctrl)
	s.mockIDGenerator = idgenmock.NewMockGenerator(s.ctrl)
	s.ctx = context.Background()

	orchestrator, err := character.New(&character.Config{
		CharacterRepo:      s.mockCharRepo,
		CharacterDraftRepo: s.mockDraftRepo,
		ExternalClient:     s.mockExternalClient,
		DiceService:        s.mockDiceService,
		IDGenerator:        s.mockIDGenerator,
	})
	s.Require().NoError(err)
	s.orchestrator = orchestrator
	
	// Initialize test data
	s.setupTestData()
}

func (s *OrchestratorTestSuite) SetupSubTest() {
	// Reset test data to clean state for each subtest
	s.setupTestData()
}

func (s *OrchestratorTestSuite) setupTestData() {
	s.testDraftID = "draft-123"
	s.testPlayerID = "player-456"
	s.testDraftData = &toolkitchar.DraftData{
		ID:       s.testDraftID,
		PlayerID: s.testPlayerID,
		Name:     "Aragorn",
	}
}

func (s *OrchestratorTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *OrchestratorTestSuite) TestGetDraft_Success() {
	// Mock repository call
	s.mockDraftRepo.EXPECT().
		Get(s.ctx, draftrepo.GetInput{
			ID: s.testDraftID,
		}).
		Return(&draftrepo.GetOutput{
			Draft: s.testDraftData,
		}, nil)

	// Call orchestrator
	output, err := s.orchestrator.GetDraft(s.ctx, &character.GetDraftInput{
		DraftID: s.testDraftID,
	})

	// Assert response
	s.NoError(err)
	s.NotNil(output)
	s.NotNil(output.Draft)
	s.Equal(s.testDraftData, output.Draft)
}

func (s *OrchestratorTestSuite) TestGetDraft_EmptyID() {
	// Call orchestrator with empty ID
	output, err := s.orchestrator.GetDraft(s.ctx, &character.GetDraftInput{
		DraftID: "",
	})

	// Assert error
	s.Error(err)
	s.Nil(output)
	s.True(errors.IsInvalidArgument(err))
	s.Contains(err.Error(), "draft ID is required")
}

func (s *OrchestratorTestSuite) TestGetDraft_NotFound() {
	draftID := "draft-notfound"

	// Mock repository call
	s.mockDraftRepo.EXPECT().
		Get(s.ctx, draftrepo.GetInput{
			ID: draftID,
		}).
		Return(nil, errors.NotFound("draft not found"))

	// Call orchestrator
	output, err := s.orchestrator.GetDraft(s.ctx, &character.GetDraftInput{
		DraftID: draftID,
	})

	// Assert error
	s.Error(err)
	s.Nil(output)
	s.Contains(err.Error(), "failed to get draft")
}

func (s *OrchestratorTestSuite) TestCreateDraft_Success() {
	// Generate test ID
	generatedID := "draft-generated-123"
	s.mockIDGenerator.EXPECT().
		Generate().
		Return(generatedID)

	// Mock repository call
	s.mockDraftRepo.EXPECT().
		Create(s.ctx, draftrepo.CreateInput{
			Draft: &toolkitchar.DraftData{
				ID:       generatedID,
				PlayerID: s.testPlayerID,
			},
		}).
		Return(&draftrepo.CreateOutput{
			Draft: &toolkitchar.DraftData{
				ID:       generatedID,
				PlayerID: s.testPlayerID,
				Name:     "",
			},
		}, nil)

	// Call orchestrator
	output, err := s.orchestrator.CreateDraft(s.ctx, &character.CreateDraftInput{
		PlayerID: s.testPlayerID,
	})

	// Assert response
	s.NoError(err)
	s.NotNil(output)
	s.NotNil(output.Draft)
	s.Equal(generatedID, output.Draft.ID)
	s.Equal(s.testPlayerID, output.Draft.PlayerID)
}

func (s *OrchestratorTestSuite) TestCreateDraft_WithInitialData() {
	generatedID := "draft-generated-456"
	initialName := "Legolas"
	
	s.mockIDGenerator.EXPECT().
		Generate().
		Return(generatedID)

	// Mock repository call with initial data
	s.mockDraftRepo.EXPECT().
		Create(s.ctx, draftrepo.CreateInput{
			Draft: &toolkitchar.DraftData{
				ID:       generatedID,
				PlayerID: s.testPlayerID,
				Name:     initialName,
			},
		}).
		Return(&draftrepo.CreateOutput{
			Draft: &toolkitchar.DraftData{
				ID:       generatedID,
				PlayerID: s.testPlayerID,
				Name:     initialName,
			},
		}, nil)

	// Call orchestrator with initial data
	output, err := s.orchestrator.CreateDraft(s.ctx, &character.CreateDraftInput{
		PlayerID: s.testPlayerID,
		InitialData: &toolkitchar.DraftData{
			Name: initialName,
		},
	})

	// Assert response
	s.NoError(err)
	s.NotNil(output)
	s.NotNil(output.Draft)
	s.Equal(generatedID, output.Draft.ID)
	s.Equal(s.testPlayerID, output.Draft.PlayerID)
	s.Equal(initialName, output.Draft.Name)
}

func (s *OrchestratorTestSuite) TestCreateDraft_EmptyPlayerID() {
	// Call orchestrator with empty player ID
	output, err := s.orchestrator.CreateDraft(s.ctx, &character.CreateDraftInput{
		PlayerID: "",
	})

	// Assert error
	s.Error(err)
	s.Nil(output)
	s.True(errors.IsInvalidArgument(err))
	s.Contains(err.Error(), "player ID is required")
}

func (s *OrchestratorTestSuite) TestCreateDraft_RepositoryError() {
	generatedID := "draft-generated-789"
	
	s.mockIDGenerator.EXPECT().
		Generate().
		Return(generatedID)

	// Mock repository error
	s.mockDraftRepo.EXPECT().
		Create(s.ctx, gomock.Any()).
		Return(nil, errors.Internal("database error"))

	// Call orchestrator
	output, err := s.orchestrator.CreateDraft(s.ctx, &character.CreateDraftInput{
		PlayerID: s.testPlayerID,
	})

	// Assert error
	s.Error(err)
	s.Nil(output)
	s.Contains(err.Error(), "failed to create draft")
}

func TestOrchestratorSuite(t *testing.T) {
	suite.Run(t, new(OrchestratorTestSuite))
}