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
}

func (s *OrchestratorTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *OrchestratorTestSuite) TestGetDraft_Success() {
	draftID := "draft-123"
	draftData := &toolkitchar.DraftData{
		ID:       draftID,
		PlayerID: "player-456",
		Name:     "Aragorn",
	}

	// Mock repository call
	s.mockDraftRepo.EXPECT().
		Get(s.ctx, draftrepo.GetInput{
			ID: draftID,
		}).
		Return(&draftrepo.GetOutput{
			Draft: draftData,
		}, nil)

	// Call orchestrator
	output, err := s.orchestrator.GetDraft(s.ctx, &character.GetDraftInput{
		DraftID: draftID,
	})

	// Assert response
	s.NoError(err)
	s.NotNil(output)
	s.NotNil(output.Draft)
	s.Equal(draftData, output.Draft)
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

func TestOrchestratorSuite(t *testing.T) {
	suite.Run(t, new(OrchestratorTestSuite))
}