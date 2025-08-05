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

type HandlerFinalizeDraftTestSuite struct {
	suite.Suite
	ctrl            *gomock.Controller
	mockCharService *charactermock.MockService
	handler         *v1alpha1.Handler
	ctx             context.Context
}

func TestHandlerFinalizeDraftTestSuite(t *testing.T) {
	suite.Run(t, new(HandlerFinalizeDraftTestSuite))
}

func (s *HandlerFinalizeDraftTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockCharService = charactermock.NewMockService(s.ctrl)
	s.ctx = context.Background()

	handler, err := v1alpha1.NewHandler(&v1alpha1.HandlerConfig{
		CharacterService: s.mockCharService,
	})
	s.Require().NoError(err)
	s.handler = handler
}

func (s *HandlerFinalizeDraftTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *HandlerFinalizeDraftTestSuite) TestFinalizeDraft_Success() {
	draftID := "draft-123"

	// Mock character data
	mockCharacter := &toolkitchar.Data{
		ID:       "char-123",
		PlayerID: "player-456",
		Name:     "Test Character",
		Level:    1,

		// Race and class info
		RaceID:       constants.RaceHuman,
		ClassID:      constants.ClassFighter,
		BackgroundID: constants.BackgroundSoldier,

		// Ability scores
		AbilityScores: shared.AbilityScores{
			constants.STR: 16,
			constants.DEX: 14,
			constants.CON: 15,
			constants.INT: 10,
			constants.WIS: 12,
			constants.CHA: 8,
		},

		// Hit points
		HitPoints:    12,
		MaxHitPoints: 12,

		// Speed and size
		Speed: 30,
		Size:  "Medium",

		// Skills
		Skills: map[constants.Skill]shared.ProficiencyLevel{
			constants.SkillAthletics:    shared.Proficient,
			constants.SkillIntimidation: shared.Proficient,
		},

		// Saving throws
		SavingThrows: map[constants.Ability]shared.ProficiencyLevel{
			constants.STR: shared.Proficient,
			constants.CON: shared.Proficient,
		},

		// Languages
		Languages: []string{string(constants.LanguageCommon), string(constants.LanguageElvish)},

		// Proficiencies
		Proficiencies: shared.Proficiencies{
			Weapons: []string{"simple", "martial"},
			Armor:   []string{"light", "medium", "heavy", "shields"},
		},

		// Equipment
		Equipment: []string{"longsword", "shield", "chain-mail"},
	}

	s.mockCharService.EXPECT().
		FinalizeDraft(s.ctx, &character.FinalizeDraftInput{
			DraftID: draftID,
		}).
		Return(&character.FinalizeDraftOutput{
			Character:    mockCharacter,
			DraftDeleted: true,
		}, nil)

	// Call handler
	req := &dnd5ev1alpha1.FinalizeDraftRequest{
		DraftId: draftID,
	}
	resp, err := s.handler.FinalizeDraft(s.ctx, req)

	// Verify response
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().NotNil(resp.Character)
	s.Equal("char-123", resp.Character.Id)
	s.Equal("Test Character", resp.Character.Name)
	s.Equal(int32(1), resp.Character.Level)
	s.True(resp.DraftDeleted)

	// Verify metadata
	s.Require().NotNil(resp.Character.Metadata)
	s.Equal("player-456", resp.Character.Metadata.PlayerId)

	// Verify race and class
	s.Equal(dnd5ev1alpha1.Race_RACE_HUMAN, resp.Character.Race)
	s.Equal(dnd5ev1alpha1.Class_CLASS_FIGHTER, resp.Character.Class)
	s.Equal(dnd5ev1alpha1.Background_BACKGROUND_SOLDIER, resp.Character.Background)

	// Verify ability scores
	s.NotNil(resp.Character.AbilityScores)
	s.Equal(int32(16), resp.Character.AbilityScores.Strength)
	s.Equal(int32(14), resp.Character.AbilityScores.Dexterity)
	s.Equal(int32(15), resp.Character.AbilityScores.Constitution)
	s.Equal(int32(10), resp.Character.AbilityScores.Intelligence)
	s.Equal(int32(12), resp.Character.AbilityScores.Wisdom)
	s.Equal(int32(8), resp.Character.AbilityScores.Charisma)

	// Verify combat stats
	s.Require().NotNil(resp.Character.CombatStats)
	s.Equal(int32(12), resp.Character.CombatStats.HitPointMaximum)
	s.Equal(int32(12), resp.Character.CurrentHitPoints)

	// Verify proficiencies
	s.NotNil(resp.Character.Proficiencies)
	s.Len(resp.Character.Proficiencies.Skills, 2)
	s.Len(resp.Character.Proficiencies.SavingThrows, 2)
	s.Len(resp.Character.Proficiencies.Weapons, 2)
	s.Len(resp.Character.Proficiencies.Armor, 4)

	// Verify languages
	s.Len(resp.Character.Languages, 2)
}

func (s *HandlerFinalizeDraftTestSuite) TestFinalizeDraft_MissingDraftID() {
	req := &dnd5ev1alpha1.FinalizeDraftRequest{
		DraftId: "", // Missing draft ID
	}

	resp, err := s.handler.FinalizeDraft(s.ctx, req)

	s.Require().Error(err)
	s.Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Equal(codes.InvalidArgument, st.Code())
	s.Contains(st.Message(), "draft_id is required")
}

func (s *HandlerFinalizeDraftTestSuite) TestFinalizeDraft_DraftNotFound() {
	draftID := "non-existent"

	s.mockCharService.EXPECT().
		FinalizeDraft(s.ctx, &character.FinalizeDraftInput{
			DraftID: draftID,
		}).
		Return(nil, errors.NotFound("draft not found"))

	req := &dnd5ev1alpha1.FinalizeDraftRequest{
		DraftId: draftID,
	}

	resp, err := s.handler.FinalizeDraft(s.ctx, req)

	s.Require().Error(err)
	s.Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Equal(codes.NotFound, st.Code())
}

func (s *HandlerFinalizeDraftTestSuite) TestFinalizeDraft_InvalidDraft() {
	draftID := "draft-123"

	s.mockCharService.EXPECT().
		FinalizeDraft(s.ctx, &character.FinalizeDraftInput{
			DraftID: draftID,
		}).
		Return(nil, errors.InvalidArgument("draft is incomplete: name is required"))

	req := &dnd5ev1alpha1.FinalizeDraftRequest{
		DraftId: draftID,
	}

	resp, err := s.handler.FinalizeDraft(s.ctx, req)

	s.Require().Error(err)
	s.Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Equal(codes.InvalidArgument, st.Code())
	s.Contains(st.Message(), "draft is incomplete")
}

func (s *HandlerFinalizeDraftTestSuite) TestFinalizeDraft_DraftDeleteFailed() {
	draftID := "draft-123"

	// Mock character data
	mockCharacter := &toolkitchar.Data{
		ID:       "char-123",
		PlayerID: "player-456",
		Name:     "Test Character",
		Level:    1,
		RaceID:   constants.RaceHuman,
		ClassID:  constants.ClassFighter,
	}

	s.mockCharService.EXPECT().
		FinalizeDraft(s.ctx, &character.FinalizeDraftInput{
			DraftID: draftID,
		}).
		Return(&character.FinalizeDraftOutput{
			Character:    mockCharacter,
			DraftDeleted: false, // Draft deletion failed
		}, nil)

	// Call handler
	req := &dnd5ev1alpha1.FinalizeDraftRequest{
		DraftId: draftID,
	}
	resp, err := s.handler.FinalizeDraft(s.ctx, req)

	// Should still succeed
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().NotNil(resp.Character)
	s.Equal("char-123", resp.Character.Id)
	s.False(resp.DraftDeleted) // Draft was not deleted
}
