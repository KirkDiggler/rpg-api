package character_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/clients/external"
	externalmock "github.com/KirkDiggler/rpg-api/internal/clients/external/mock"
	"github.com/KirkDiggler/rpg-api/internal/errors"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	dicemock "github.com/KirkDiggler/rpg-api/internal/orchestrators/dice/mock"
	idgenmock "github.com/KirkDiggler/rpg-api/internal/pkg/idgen/mock"
	characterrepomock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	draftrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
	draftrepomock "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft/mock"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/class"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/constants"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/race"
)

type OrchestratorTestSuite struct {
	suite.Suite
	ctrl               *gomock.Controller
	mockCharRepo       *characterrepomock.MockRepository
	mockDraftRepo      *draftrepomock.MockRepository
	mockExternal       *externalmock.MockClient
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
	s.mockExternal = externalmock.NewMockClient(s.ctrl)
	s.mockDiceService = dicemock.NewMockService(s.ctrl)
	s.mockIDGenerator = idgenmock.NewMockGenerator(s.ctrl)
	s.ctx = context.Background()

	orchestrator, err := character.New(&character.Config{
		CharacterRepo:      s.mockCharRepo,
		CharacterDraftRepo: s.mockDraftRepo,
		ExternalClient:     s.mockExternal,
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

func (s *OrchestratorTestSuite) TestGetRaceDetails_Success() {
	ctx := context.Background()
	input := &character.GetRaceDetailsInput{
		RaceID: "RACE_DRAGONBORN",
	}

	expectedRaceData := &race.Data{
		ID:   constants.Race("RACE_DRAGONBORN"),
		Name: "Dragonborn",
	}
	expectedUIData := &external.RaceUIData{
		SizeDescription: "Dragonborn are taller and heavier than humans",
	}

	s.mockExternal.EXPECT().
		GetRaceData(ctx, "RACE_DRAGONBORN").
		Return(&external.RaceDataOutput{
			RaceData: expectedRaceData,
			UIData:   expectedUIData,
		}, nil)

	output, err := s.orchestrator.GetRaceDetails(ctx, input)

	s.Require().NoError(err)
	s.Assert().Equal(expectedRaceData, output.RaceData)
	s.Assert().Equal(expectedUIData, output.UIData)
}

func (s *OrchestratorTestSuite) TestGetRaceDetails_EmptyID() {
	ctx := context.Background()
	input := &character.GetRaceDetailsInput{
		RaceID: "",
	}

	output, err := s.orchestrator.GetRaceDetails(ctx, input)

	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().True(errors.IsInvalidArgument(err))
}

func (s *OrchestratorTestSuite) TestGetClassDetails_Success() {
	ctx := context.Background()
	input := &character.GetClassDetailsInput{
		ClassID: "CLASS_WIZARD",
	}

	expectedClassData := &class.Data{
		ID:      constants.Class("CLASS_WIZARD"),
		Name:    "Wizard",
		HitDice: 6,
	}
	expectedUIData := &external.ClassUIData{
		Description: "Wizards are supreme magic-users",
	}

	s.mockExternal.EXPECT().
		GetClassData(ctx, "CLASS_WIZARD").
		Return(&external.ClassDataOutput{
			ClassData: expectedClassData,
			UIData:    expectedUIData,
		}, nil)

	output, err := s.orchestrator.GetClassDetails(ctx, input)

	s.Require().NoError(err)
	s.Assert().Equal(expectedClassData, output.ClassData)
	s.Assert().Equal(expectedUIData, output.UIData)
}

func (s *OrchestratorTestSuite) TestGetClassDetails_EmptyID() {
	ctx := context.Background()
	input := &character.GetClassDetailsInput{
		ClassID: "",
	}

	output, err := s.orchestrator.GetClassDetails(ctx, input)

	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().True(errors.IsInvalidArgument(err))
}

func (s *OrchestratorTestSuite) TestListDrafts_Success() {
	ctx := context.Background()
	input := &character.ListDraftsInput{
		PlayerID: s.testPlayerID,
	}

	// Mock repository call
	s.mockDraftRepo.EXPECT().
		GetByPlayerID(ctx, draftrepo.GetByPlayerIDInput{
			PlayerID: s.testPlayerID,
		}).
		Return(&draftrepo.GetByPlayerIDOutput{
			Draft: s.testDraftData,
		}, nil)

	// Call orchestrator
	output, err := s.orchestrator.ListDrafts(ctx, input)

	// Assert response
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().Len(output.Drafts, 1)
	s.Assert().Equal(s.testDraftData, output.Drafts[0])
	s.Assert().Empty(output.NextPageToken)
}

func (s *OrchestratorTestSuite) TestListDrafts_EmptyPlayerID() {
	ctx := context.Background()
	input := &character.ListDraftsInput{
		PlayerID: "",
	}

	// Call orchestrator
	output, err := s.orchestrator.ListDrafts(ctx, input)

	// Assert error
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().True(errors.IsInvalidArgument(err))
	s.Assert().Contains(err.Error(), "player ID is required")
}

func (s *OrchestratorTestSuite) TestListDrafts_NoDraft() {
	ctx := context.Background()
	input := &character.ListDraftsInput{
		PlayerID: s.testPlayerID,
	}

	// Mock repository call - no draft found
	s.mockDraftRepo.EXPECT().
		GetByPlayerID(ctx, draftrepo.GetByPlayerIDInput{
			PlayerID: s.testPlayerID,
		}).
		Return(nil, errors.NotFound("no draft found"))

	// Call orchestrator
	output, err := s.orchestrator.ListDrafts(ctx, input)

	// Assert response - empty list but no error
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().Empty(output.Drafts)
	s.Assert().Empty(output.NextPageToken)
}

func (s *OrchestratorTestSuite) TestUpdateName_Success() {
	ctx := context.Background()
	newName := "Gimli"
	input := &character.UpdateNameInput{
		DraftID: s.testDraftID,
		Name:    newName,
	}

	// Create a copy of test data with updated name
	updatedDraft := *s.testDraftData
	updatedDraft.Name = newName

	// Mock get call
	s.mockDraftRepo.EXPECT().
		Get(ctx, draftrepo.GetInput{
			ID: s.testDraftID,
		}).
		Return(&draftrepo.GetOutput{
			Draft: s.testDraftData,
		}, nil)

	// Mock update call
	s.mockDraftRepo.EXPECT().
		Update(ctx, draftrepo.UpdateInput{
			Draft: &updatedDraft,
		}).
		Return(&draftrepo.UpdateOutput{
			Draft: &updatedDraft,
		}, nil)

	// Call orchestrator
	output, err := s.orchestrator.UpdateName(ctx, input)

	// Assert response
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().Equal(newName, output.Draft.Name)
	s.Assert().Empty(output.Warnings)
}

func (s *OrchestratorTestSuite) TestUpdateName_EmptyDraftID() {
	ctx := context.Background()
	input := &character.UpdateNameInput{
		DraftID: "",
		Name:    "Gimli",
	}

	// Call orchestrator
	output, err := s.orchestrator.UpdateName(ctx, input)

	// Assert error
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().True(errors.IsInvalidArgument(err))
	s.Assert().Contains(err.Error(), "draft ID is required")
}

func (s *OrchestratorTestSuite) TestUpdateName_EmptyName() {
	ctx := context.Background()
	input := &character.UpdateNameInput{
		DraftID: s.testDraftID,
		Name:    "   ",  // Whitespace only
	}

	// Call orchestrator
	output, err := s.orchestrator.UpdateName(ctx, input)

	// Assert error
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().True(errors.IsInvalidArgument(err))
	s.Assert().Contains(err.Error(), "name is required")
}

func (s *OrchestratorTestSuite) TestUpdateName_DraftNotFound() {
	ctx := context.Background()
	input := &character.UpdateNameInput{
		DraftID: s.testDraftID,
		Name:    "Gimli",
	}

	// Mock get call - draft not found
	s.mockDraftRepo.EXPECT().
		Get(ctx, draftrepo.GetInput{
			ID: s.testDraftID,
		}).
		Return(nil, errors.NotFound("draft not found"))

	// Call orchestrator
	output, err := s.orchestrator.UpdateName(ctx, input)

	// Assert error
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "failed to get draft")
}

func TestOrchestratorSuite(t *testing.T) {
	suite.Run(t, new(OrchestratorTestSuite))
}