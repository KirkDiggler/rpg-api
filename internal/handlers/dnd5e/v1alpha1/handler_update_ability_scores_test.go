package v1alpha1_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/errors"
	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/orchestrators/character/mock"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/constants"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

type HandlerUpdateAbilityScoresTestSuite struct {
	suite.Suite
	ctrl            *gomock.Controller
	mockCharService *charactermock.MockService
	handler         *v1alpha1.Handler
}

func TestHandlerUpdateAbilityScoresTestSuite(t *testing.T) {
	suite.Run(t, new(HandlerUpdateAbilityScoresTestSuite))
}

func (s *HandlerUpdateAbilityScoresTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockCharService = charactermock.NewMockService(s.ctrl)

	handler, err := v1alpha1.NewHandler(&v1alpha1.HandlerConfig{
		CharacterService: s.mockCharService,
	})
	s.Require().NoError(err)
	s.handler = handler
}

func (s *HandlerUpdateAbilityScoresTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *HandlerUpdateAbilityScoresTestSuite) TestUpdateAbilityScores_WithRollAssignments_Success() {
	ctx := context.Background()
	draftID := "draft-123"

	// Mock draft with ability scores after assignment
	mockDraft := &toolkitchar.DraftData{
		ID:       draftID,
		PlayerID: "player-456",
		Name:     "Test Character",
		AbilityScoreChoice: shared.AbilityScores{
			constants.STR: 16,
			constants.DEX: 14,
			constants.CON: 13,
			constants.INT: 12,
			constants.WIS: 15,
			constants.CHA: 10,
		},
	}

	// Mock roll assignments
	rollAssignments := &character.RollAssignments{
		StrengthRollID:     "roll-1",
		DexterityRollID:    "roll-2",
		ConstitutionRollID: "roll-3",
		IntelligenceRollID: "roll-4",
		WisdomRollID:       "roll-5",
		CharismaRollID:     "roll-6",
	}

	s.mockCharService.EXPECT().
		UpdateAbilityScores(ctx, &character.UpdateAbilityScoresInput{
			DraftID:         draftID,
			RollAssignments: rollAssignments,
		}).
		Return(&character.UpdateAbilityScoresOutput{
			Draft:    mockDraft,
			Warnings: []character.ValidationWarning{},
		}, nil)

	// Call handler
	req := &dnd5ev1alpha1.UpdateAbilityScoresRequest{
		DraftId: draftID,
		ScoresInput: &dnd5ev1alpha1.UpdateAbilityScoresRequest_RollAssignments{
			RollAssignments: &dnd5ev1alpha1.RollAssignments{
				StrengthRollId:     "roll-1",
				DexterityRollId:    "roll-2",
				ConstitutionRollId: "roll-3",
				IntelligenceRollId: "roll-4",
				WisdomRollId:       "roll-5",
				CharismaRollId:     "roll-6",
			},
		},
	}
	resp, err := s.handler.UpdateAbilityScores(ctx, req)

	// Verify response
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().NotNil(resp.Draft)
	s.Equal(draftID, resp.Draft.Id)
	s.Require().NotNil(resp.Draft.AbilityScores)
	s.Equal(int32(16), resp.Draft.AbilityScores.Strength)
	s.Equal(int32(14), resp.Draft.AbilityScores.Dexterity)
	s.Equal(int32(13), resp.Draft.AbilityScores.Constitution)
	s.Equal(int32(12), resp.Draft.AbilityScores.Intelligence)
	s.Equal(int32(15), resp.Draft.AbilityScores.Wisdom)
	s.Equal(int32(10), resp.Draft.AbilityScores.Charisma)
}

func (s *HandlerUpdateAbilityScoresTestSuite) TestUpdateAbilityScores_MissingDraftID() {
	ctx := context.Background()

	req := &dnd5ev1alpha1.UpdateAbilityScoresRequest{
		DraftId: "", // Missing draft ID
		ScoresInput: &dnd5ev1alpha1.UpdateAbilityScoresRequest_RollAssignments{
			RollAssignments: &dnd5ev1alpha1.RollAssignments{
				StrengthRollId:     "roll-1",
				DexterityRollId:    "roll-2",
				ConstitutionRollId: "roll-3",
				IntelligenceRollId: "roll-4",
				WisdomRollId:       "roll-5",
				CharismaRollId:     "roll-6",
			},
		},
	}

	resp, err := s.handler.UpdateAbilityScores(ctx, req)

	s.Require().Error(err)
	s.Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Equal(codes.InvalidArgument, st.Code())
	s.Contains(st.Message(), "draft_id is required")
}

func (s *HandlerUpdateAbilityScoresTestSuite) TestUpdateAbilityScores_MissingRollIDs() {
	ctx := context.Background()

	req := &dnd5ev1alpha1.UpdateAbilityScoresRequest{
		DraftId: "draft-123",
		ScoresInput: &dnd5ev1alpha1.UpdateAbilityScoresRequest_RollAssignments{
			RollAssignments: &dnd5ev1alpha1.RollAssignments{
				StrengthRollId:     "roll-1",
				DexterityRollId:    "", // Missing
				ConstitutionRollId: "roll-3",
				IntelligenceRollId: "roll-4",
				WisdomRollId:       "roll-5",
				CharismaRollId:     "roll-6",
			},
		},
	}

	resp, err := s.handler.UpdateAbilityScores(ctx, req)

	s.Require().Error(err)
	s.Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Equal(codes.InvalidArgument, st.Code())
	s.Contains(st.Message(), "all ability score roll IDs must be provided")
}

func (s *HandlerUpdateAbilityScoresTestSuite) TestUpdateAbilityScores_DraftNotFound() {
	ctx := context.Background()
	draftID := "non-existent"

	s.mockCharService.EXPECT().
		UpdateAbilityScores(ctx, gomock.Any()).
		Return(nil, errors.NotFound("draft not found"))

	req := &dnd5ev1alpha1.UpdateAbilityScoresRequest{
		DraftId: draftID,
		ScoresInput: &dnd5ev1alpha1.UpdateAbilityScoresRequest_RollAssignments{
			RollAssignments: &dnd5ev1alpha1.RollAssignments{
				StrengthRollId:     "roll-1",
				DexterityRollId:    "roll-2",
				ConstitutionRollId: "roll-3",
				IntelligenceRollId: "roll-4",
				WisdomRollId:       "roll-5",
				CharismaRollId:     "roll-6",
			},
		},
	}

	resp, err := s.handler.UpdateAbilityScores(ctx, req)

	s.Require().Error(err)
	s.Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Equal(codes.NotFound, st.Code())
}

func (s *HandlerUpdateAbilityScoresTestSuite) TestUpdateAbilityScores_NoScoresProvided() {
	ctx := context.Background()

	req := &dnd5ev1alpha1.UpdateAbilityScoresRequest{
		DraftId: "draft-123",
		// No scores_input provided
	}

	resp, err := s.handler.UpdateAbilityScores(ctx, req)

	s.Require().Error(err)
	s.Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Equal(codes.InvalidArgument, st.Code())
	s.Contains(st.Message(), "scores_input must be provided")
}

func (s *HandlerUpdateAbilityScoresTestSuite) TestUpdateAbilityScores_ManualAssignment_NotImplemented() {
	ctx := context.Background()

	req := &dnd5ev1alpha1.UpdateAbilityScoresRequest{
		DraftId: "draft-123",
		ScoresInput: &dnd5ev1alpha1.UpdateAbilityScoresRequest_AbilityScores{
			AbilityScores: &dnd5ev1alpha1.AbilityScores{
				Strength:     15,
				Dexterity:    14,
				Constitution: 13,
				Intelligence: 12,
				Wisdom:       10,
				Charisma:     8,
			},
		},
	}

	resp, err := s.handler.UpdateAbilityScores(ctx, req)

	s.Require().Error(err)
	s.Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Equal(codes.Unimplemented, st.Code())
	s.Contains(st.Message(), "manual ability score assignment not yet implemented")
}
