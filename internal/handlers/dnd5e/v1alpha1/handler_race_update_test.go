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
)

type HandlerRaceUpdateTestSuite struct {
	suite.Suite
	ctrl            *gomock.Controller
	mockCharService *charactermock.MockService
	handler         *v1alpha1.Handler
}

func TestHandlerRaceUpdateTestSuite(t *testing.T) {
	suite.Run(t, new(HandlerRaceUpdateTestSuite))
}

func (s *HandlerRaceUpdateTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockCharService = charactermock.NewMockService(s.ctrl)

	handler, err := v1alpha1.NewHandler(&v1alpha1.HandlerConfig{
		CharacterService: s.mockCharService,
	})
	s.Require().NoError(err)
	s.handler = handler
}

func (s *HandlerRaceUpdateTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *HandlerRaceUpdateTestSuite) TestUpdateRace_Success_ReturnsIDInDraft() {
	// GIVEN a draft exists with an ID
	draftID := "draft-123"
	ctx := context.Background()

	req := &dnd5ev1alpha1.UpdateRaceRequest{
		DraftId: draftID,
		Race:    dnd5ev1alpha1.Race_RACE_ELF,
		Subrace: dnd5ev1alpha1.Subrace_SUBRACE_HIGH_ELF,
	}

	// Mock response should include the draft with ID
	mockDraft := &toolkitchar.DraftData{
		ID:       draftID,
		PlayerID: "player-123",
		Name:     "Test Character",
		RaceChoice: toolkitchar.RaceChoice{
			RaceID:    constants.RaceElf,
			SubraceID: constants.SubraceHighElf,
		},
	}

	s.mockCharService.EXPECT().
		UpdateRace(ctx, &character.UpdateRaceInput{
			DraftID:   draftID,
			RaceID:    constants.RaceElf,
			SubraceID: constants.SubraceHighElf,
			Choices:   nil,
		}).
		Return(&character.UpdateRaceOutput{
			Draft:    mockDraft,
			Warnings: []character.ValidationWarning{},
		}, nil)

	// WHEN updating the race
	resp, err := s.handler.UpdateRace(ctx, req)

	// THEN the response should be successful
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().NotNil(resp.Draft)

	// AND the draft should have the ID preserved
	s.Equal(draftID, resp.Draft.Id, "Draft ID should be preserved after update")
	s.Equal("player-123", resp.Draft.PlayerId)
	s.Equal("Test Character", resp.Draft.Name)
	s.Equal(dnd5ev1alpha1.Race_RACE_ELF, resp.Draft.RaceId)
	s.Equal(dnd5ev1alpha1.Subrace_SUBRACE_HIGH_ELF, resp.Draft.SubraceId)
}

func (s *HandlerRaceUpdateTestSuite) TestUpdateRace_WithChoices_Success() {
	// GIVEN a draft exists with choices
	draftID := "draft-456"
	ctx := context.Background()

	req := &dnd5ev1alpha1.UpdateRaceRequest{
		DraftId: draftID,
		Race:    dnd5ev1alpha1.Race_RACE_HALF_ELF,
		RaceChoices: []*dnd5ev1alpha1.ChoiceData{
			{
				ChoiceId: "skill_choice",
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_RACE,
				Selection: &dnd5ev1alpha1.ChoiceData_Skills{
					Skills: &dnd5ev1alpha1.SkillList{
						Skills: []dnd5ev1alpha1.Skill{
							dnd5ev1alpha1.Skill_SKILL_PERCEPTION,
							dnd5ev1alpha1.Skill_SKILL_INVESTIGATION,
						},
					},
				},
			},
		},
	}

	// Mock response with choices
	mockDraft := &toolkitchar.DraftData{
		ID:       draftID,
		PlayerID: "player-456",
		Name:     "Half-Elf Character",
		RaceChoice: toolkitchar.RaceChoice{
			RaceID: constants.RaceHalfElf,
		},
		Choices: []toolkitchar.ChoiceData{
			{
				ChoiceID:       "skill_choice",
				SkillSelection: []constants.Skill{constants.SkillPerception, constants.SkillInvestigation},
			},
		},
	}

	s.mockCharService.EXPECT().
		UpdateRace(ctx, gomock.Any()).
		Return(&character.UpdateRaceOutput{
			Draft:    mockDraft,
			Warnings: []character.ValidationWarning{},
		}, nil)

	// WHEN updating the race with choices
	resp, err := s.handler.UpdateRace(ctx, req)

	// THEN the response should be successful
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().NotNil(resp.Draft)

	// AND the draft should have the ID and choices preserved
	s.Equal(draftID, resp.Draft.Id, "Draft ID should be preserved")
	s.Equal(dnd5ev1alpha1.Race_RACE_HALF_ELF, resp.Draft.RaceId)
	s.Len(resp.Draft.Choices, 1)
}

func (s *HandlerRaceUpdateTestSuite) TestUpdateRace_MissingDraftID_ReturnsError() {
	ctx := context.Background()

	req := &dnd5ev1alpha1.UpdateRaceRequest{
		DraftId: "", // Missing draft ID
		Race:    dnd5ev1alpha1.Race_RACE_HUMAN,
	}

	// Mock the orchestrator to return an error for missing draft ID
	s.mockCharService.EXPECT().
		UpdateRace(ctx, gomock.Any()).
		Return(nil, errors.InvalidArgument("draft ID is required"))

	// WHEN updating race without draft ID
	resp, err := s.handler.UpdateRace(ctx, req)

	// THEN it should return an error
	s.Require().Error(err)
	s.Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Equal(codes.InvalidArgument, st.Code())
}

func (s *HandlerRaceUpdateTestSuite) TestUpdateRace_DraftNotFound_ReturnsNotFound() {
	ctx := context.Background()
	draftID := "non-existent"

	req := &dnd5ev1alpha1.UpdateRaceRequest{
		DraftId: draftID,
		Race:    dnd5ev1alpha1.Race_RACE_DWARF,
	}

	s.mockCharService.EXPECT().
		UpdateRace(ctx, gomock.Any()).
		Return(nil, errors.NotFound("draft not found"))

	// WHEN updating a non-existent draft
	resp, err := s.handler.UpdateRace(ctx, req)

	// THEN it should return not found error
	s.Require().Error(err)
	s.Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Equal(codes.NotFound, st.Code())
}
