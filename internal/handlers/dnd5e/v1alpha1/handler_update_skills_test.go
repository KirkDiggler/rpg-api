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

type HandlerUpdateSkillsTestSuite struct {
	suite.Suite
	ctrl            *gomock.Controller
	mockCharService *charactermock.MockService
	handler         *v1alpha1.Handler
	ctx             context.Context
}

func TestHandlerUpdateSkillsTestSuite(t *testing.T) {
	suite.Run(t, new(HandlerUpdateSkillsTestSuite))
}

func (s *HandlerUpdateSkillsTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockCharService = charactermock.NewMockService(s.ctrl)
	s.ctx = context.Background()

	handler, err := v1alpha1.NewHandler(&v1alpha1.HandlerConfig{
		CharacterService: s.mockCharService,
	})
	s.Require().NoError(err)
	s.handler = handler
}

func (s *HandlerUpdateSkillsTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *HandlerUpdateSkillsTestSuite) TestUpdateSkills_Success() {
	draftID := "draft-123"
	playerID := "player-456"

	// Mock draft with skill selection
	mockDraft := &toolkitchar.DraftData{
		ID:       draftID,
		PlayerID: playerID,
		Name:     "Test Character",
		ClassChoice: toolkitchar.ClassChoice{
			ClassID: constants.ClassFighter,
		},
		Choices: []toolkitchar.ChoiceData{
			{
				Category:       shared.ChoiceSkills,
				Source:         shared.SourceClass,
				ChoiceID:       "class_skills",
				SkillSelection: []constants.Skill{constants.SkillAthletics, constants.SkillIntimidation},
			},
		},
	}

	s.mockCharService.EXPECT().
		UpdateSkills(s.ctx, &character.UpdateSkillsInput{
			DraftID:  draftID,
			SkillIDs: []string{string(constants.SkillAthletics), string(constants.SkillIntimidation)},
		}).
		Return(&character.UpdateSkillsOutput{
			Draft:    mockDraft,
			Warnings: []character.ValidationWarning{},
		}, nil)

	// Call handler
	req := &dnd5ev1alpha1.UpdateSkillsRequest{
		DraftId: draftID,
		Skills: []dnd5ev1alpha1.Skill{
			dnd5ev1alpha1.Skill_SKILL_ATHLETICS,
			dnd5ev1alpha1.Skill_SKILL_INTIMIDATION,
		},
	}
	resp, err := s.handler.UpdateSkills(s.ctx, req)

	// Verify response
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().NotNil(resp.Draft)
	s.Equal(draftID, resp.Draft.Id)
	s.Empty(resp.Warnings)
}

func (s *HandlerUpdateSkillsTestSuite) TestUpdateSkills_MissingDraftID() {
	req := &dnd5ev1alpha1.UpdateSkillsRequest{
		DraftId: "", // Missing draft ID
		Skills: []dnd5ev1alpha1.Skill{
			dnd5ev1alpha1.Skill_SKILL_ATHLETICS,
		},
	}

	resp, err := s.handler.UpdateSkills(s.ctx, req)

	s.Require().Error(err)
	s.Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Equal(codes.InvalidArgument, st.Code())
	s.Contains(st.Message(), "draft_id is required")
}

func (s *HandlerUpdateSkillsTestSuite) TestUpdateSkills_NoSkills() {
	req := &dnd5ev1alpha1.UpdateSkillsRequest{
		DraftId: "draft-123",
		Skills:  []dnd5ev1alpha1.Skill{}, // No skills
	}

	resp, err := s.handler.UpdateSkills(s.ctx, req)

	s.Require().Error(err)
	s.Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Equal(codes.InvalidArgument, st.Code())
	s.Contains(st.Message(), "at least one skill must be selected")
}

func (s *HandlerUpdateSkillsTestSuite) TestUpdateSkills_InvalidSkill() {
	req := &dnd5ev1alpha1.UpdateSkillsRequest{
		DraftId: "draft-123",
		Skills: []dnd5ev1alpha1.Skill{
			dnd5ev1alpha1.Skill_SKILL_UNSPECIFIED, // Invalid skill
		},
	}

	resp, err := s.handler.UpdateSkills(s.ctx, req)

	s.Require().Error(err)
	s.Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Equal(codes.InvalidArgument, st.Code())
	s.Contains(st.Message(), "invalid skill")
}

func (s *HandlerUpdateSkillsTestSuite) TestUpdateSkills_DraftNotFound() {
	draftID := "non-existent"

	s.mockCharService.EXPECT().
		UpdateSkills(s.ctx, gomock.Any()).
		Return(nil, errors.NotFound("draft not found"))

	req := &dnd5ev1alpha1.UpdateSkillsRequest{
		DraftId: draftID,
		Skills: []dnd5ev1alpha1.Skill{
			dnd5ev1alpha1.Skill_SKILL_ATHLETICS,
		},
	}

	resp, err := s.handler.UpdateSkills(s.ctx, req)

	s.Require().Error(err)
	s.Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Equal(codes.NotFound, st.Code())
}

func (s *HandlerUpdateSkillsTestSuite) TestUpdateSkills_WithWarnings() {
	draftID := "draft-123"
	playerID := "player-456"

	// Mock draft
	mockDraft := &toolkitchar.DraftData{
		ID:       draftID,
		PlayerID: playerID,
		Name:     "Test Character",
		Choices: []toolkitchar.ChoiceData{
			{
				Category:       shared.ChoiceSkills,
				Source:         shared.SourceClass,
				ChoiceID:       "class_skills",
				SkillSelection: []constants.Skill{constants.SkillAthletics},
			},
		},
	}

	s.mockCharService.EXPECT().
		UpdateSkills(s.ctx, gomock.Any()).
		Return(&character.UpdateSkillsOutput{
			Draft: mockDraft,
			Warnings: []character.ValidationWarning{
				{
					Field:   "skills",
					Message: "You selected fewer skills than available",
					Type:    "info",
				},
			},
		}, nil)

	req := &dnd5ev1alpha1.UpdateSkillsRequest{
		DraftId: draftID,
		Skills: []dnd5ev1alpha1.Skill{
			dnd5ev1alpha1.Skill_SKILL_ATHLETICS,
		},
	}

	resp, err := s.handler.UpdateSkills(s.ctx, req)

	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().Len(resp.Warnings, 1)
	s.Equal("skills", resp.Warnings[0].Field)
	s.Equal("You selected fewer skills than available", resp.Warnings[0].Message)
}