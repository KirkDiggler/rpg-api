package conversion_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/clients/external"
	externalmock "github.com/KirkDiggler/rpg-api/internal/clients/external/mock"
	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
	"github.com/KirkDiggler/rpg-api/internal/services/conversion"
)

type DraftConverterTestSuite struct {
	suite.Suite
	ctrl           *gomock.Controller
	mockExternal   *externalmock.MockClient
	converter      conversion.DraftConverter
	ctx            context.Context
}

func (s *DraftConverterTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockExternal = externalmock.NewMockClient(s.ctrl)
	s.ctx = context.Background()

	converter, err := conversion.NewDraftConverter(&conversion.DraftConverterConfig{
		ExternalClient: s.mockExternal,
	})
	s.Require().NoError(err)
	s.converter = converter
}

func (s *DraftConverterTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *DraftConverterTestSuite) TestNewDraftConverter() {
	s.Run("nil config returns error", func() {
		_, err := conversion.NewDraftConverter(nil)
		s.Error(err)
		s.Contains(err.Error(), "config is required")
	})

	s.Run("missing external client returns error", func() {
		_, err := conversion.NewDraftConverter(&conversion.DraftConverterConfig{})
		s.Error(err)
		s.Contains(err.Error(), "external client is required")
	})

	s.Run("valid config creates converter", func() {
		converter, err := conversion.NewDraftConverter(&conversion.DraftConverterConfig{
			ExternalClient: s.mockExternal,
		})
		s.NoError(err)
		s.NotNil(converter)
	})
}

func (s *DraftConverterTestSuite) TestToCharacterDraft() {
	s.Run("nil input returns nil", func() {
		result := s.converter.ToCharacterDraft(nil)
		s.Nil(result)
	})

	s.Run("converts all fields correctly", func() {
		data := &dnd5e.CharacterDraftData{
			ID:           "draft-123",
			PlayerID:     "player-456",
			SessionID:    "session-789",
			Name:         "Test Character",
			RaceID:       dnd5e.RaceHuman,
			SubraceID:    "",
			ClassID:      dnd5e.ClassFighter,
			BackgroundID: dnd5e.BackgroundSoldier,
			AbilityScores: &dnd5e.AbilityScores{
				Strength:     16,
				Dexterity:    14,
				Constitution: 15,
				Intelligence: 10,
				Wisdom:       12,
				Charisma:     8,
			},
			Alignment: dnd5e.AlignmentLawfulGood,
			ChoiceSelections: []dnd5e.ChoiceSelection{
				{
					ChoiceID:     "fighting-style",
					ChoiceType:   dnd5e.ChoiceTypeFeat,
					Source:       dnd5e.ChoiceSourceClass,
					SelectedKeys: []string{"defense"},
				},
			},
			Progress: dnd5e.CreationProgress{
				StepsCompleted:       15,
				CompletionPercentage: 75,
				CurrentStep:          dnd5e.CreationStepSkills,
			},
			ExpiresAt: 1234567890,
			CreatedAt: 1234567800,
			UpdatedAt: 1234567850,
		}

		result := s.converter.ToCharacterDraft(data)

		s.NotNil(result)
		s.Equal(data.ID, result.ID)
		s.Equal(data.PlayerID, result.PlayerID)
		s.Equal(data.SessionID, result.SessionID)
		s.Equal(data.Name, result.Name)
		s.Equal(data.RaceID, result.RaceID)
		s.Equal(data.SubraceID, result.SubraceID)
		s.Equal(data.ClassID, result.ClassID)
		s.Equal(data.BackgroundID, result.BackgroundID)
		s.Equal(data.AbilityScores, result.AbilityScores)
		s.Equal(data.Alignment, result.Alignment)
		s.Equal(data.ChoiceSelections, result.ChoiceSelections)
		s.Equal(data.Progress, result.Progress)
		s.Equal(data.ExpiresAt, result.ExpiresAt)
		s.Equal(data.CreatedAt, result.CreatedAt)
		s.Equal(data.UpdatedAt, result.UpdatedAt)

		// Info objects should be nil
		s.Nil(result.Race)
		s.Nil(result.Subrace)
		s.Nil(result.Class)
		s.Nil(result.Background)
	})
}

func (s *DraftConverterTestSuite) TestFromCharacterDraft() {
	s.Run("nil input returns nil", func() {
		result := s.converter.FromCharacterDraft(nil)
		s.Nil(result)
	})

	s.Run("strips info objects and converts correctly", func() {
		draft := &dnd5e.CharacterDraft{
			ID:           "draft-123",
			PlayerID:     "player-456",
			SessionID:    "session-789",
			Name:         "Test Character",
			RaceID:       dnd5e.RaceHuman,
			SubraceID:    "",
			ClassID:      dnd5e.ClassFighter,
			BackgroundID: dnd5e.BackgroundSoldier,
			AbilityScores: &dnd5e.AbilityScores{
				Strength:     16,
				Dexterity:    14,
				Constitution: 15,
				Intelligence: 10,
				Wisdom:       12,
				Charisma:     8,
			},
			Alignment: dnd5e.AlignmentLawfulGood,
			ChoiceSelections: []dnd5e.ChoiceSelection{
				{
					ChoiceID:     "fighting-style",
					ChoiceType:   dnd5e.ChoiceTypeFeat,
					Source:       dnd5e.ChoiceSourceClass,
					SelectedKeys: []string{"defense"},
				},
			},
			Progress: dnd5e.CreationProgress{
				StepsCompleted:       15,
				CompletionPercentage: 75,
				CurrentStep:          dnd5e.CreationStepSkills,
			},
			ExpiresAt: 1234567890,
			CreatedAt: 1234567800,
			UpdatedAt: 1234567850,
			// These should be stripped
			Race: &dnd5e.RaceInfo{
				ID:   dnd5e.RaceHuman,
				Name: "Human",
			},
			Class: &dnd5e.ClassInfo{
				ID:   dnd5e.ClassFighter,
				Name: "Fighter",
			},
		}

		result := s.converter.FromCharacterDraft(draft)

		s.NotNil(result)
		s.Equal(draft.ID, result.ID)
		s.Equal(draft.PlayerID, result.PlayerID)
		s.Equal(draft.SessionID, result.SessionID)
		s.Equal(draft.Name, result.Name)
		s.Equal(draft.RaceID, result.RaceID)
		s.Equal(draft.SubraceID, result.SubraceID)
		s.Equal(draft.ClassID, result.ClassID)
		s.Equal(draft.BackgroundID, result.BackgroundID)
		s.Equal(draft.AbilityScores, result.AbilityScores)
		s.Equal(draft.Alignment, result.Alignment)
		s.Equal(draft.ChoiceSelections, result.ChoiceSelections)
		s.Equal(draft.Progress, result.Progress)
		s.Equal(draft.ExpiresAt, result.ExpiresAt)
		s.Equal(draft.CreatedAt, result.CreatedAt)
		s.Equal(draft.UpdatedAt, result.UpdatedAt)
	})
}

func (s *DraftConverterTestSuite) TestHydrateDraft() {
	s.Run("nil input returns nil", func() {
		result, err := s.converter.HydrateDraft(s.ctx, nil)
		s.NoError(err)
		s.Nil(result)
	})

	s.Run("empty draft returns copy without errors", func() {
		draft := &dnd5e.CharacterDraft{
			ID:   "draft-123",
			Name: "Test",
		}

		result, err := s.converter.HydrateDraft(s.ctx, draft)
		s.NoError(err)
		s.NotNil(result)
		s.Equal(draft.ID, result.ID)
		s.Equal(draft.Name, result.Name)
		s.Nil(result.Race)
		s.Nil(result.Class)
		s.Nil(result.Background)
	})

	s.Run("hydrates race successfully", func() {
		draft := &dnd5e.CharacterDraft{
			ID:     "draft-123",
			RaceID: dnd5e.RaceElf,
		}

		raceData := &external.RaceData{
			ID:          dnd5e.RaceElf,
			Name:        "Elf",
			Description: "Elves are magical",
			Speed:       30,
			Size:        "medium",
			AbilityBonuses: map[string]int32{
				"dexterity": 2,
			},
			Traits: []*external.TraitData{
				{
					Name:        "Darkvision",
					Description: "60 feet",
				},
			},
		}

		s.mockExternal.EXPECT().
			GetRaceData(s.ctx, dnd5e.RaceElf).
			Return(raceData, nil)

		result, err := s.converter.HydrateDraft(s.ctx, draft)
		s.NoError(err)
		s.NotNil(result)
		s.NotNil(result.Race)
		s.Equal(dnd5e.RaceElf, result.Race.ID)
		s.Equal("Elf", result.Race.Name)
		s.Len(result.Race.Traits, 1)
	})

	s.Run("hydrates class successfully", func() {
		draft := &dnd5e.CharacterDraft{
			ID:      "draft-123",
			ClassID: dnd5e.ClassFighter,
		}

		classData := &external.ClassData{
			ID:               dnd5e.ClassFighter,
			Name:             "Fighter",
			Description:      "Masters of combat",
			HitDice:          "1d10",
			PrimaryAbilities: []string{"Strength", "Dexterity"},
			SavingThrows:     []string{"Strength", "Constitution"},
			SkillsCount:      2,
			AvailableSkills:  []string{"Athletics", "Intimidation"},
		}

		s.mockExternal.EXPECT().
			GetClassData(s.ctx, dnd5e.ClassFighter).
			Return(classData, nil)

		result, err := s.converter.HydrateDraft(s.ctx, draft)
		s.NoError(err)
		s.NotNil(result)
		s.NotNil(result.Class)
		s.Equal(dnd5e.ClassFighter, result.Class.ID)
		s.Equal("Fighter", result.Class.Name)
		s.Equal("1d10", result.Class.HitDice)
	})

	s.Run("returns error on race fetch failure", func() {
		draft := &dnd5e.CharacterDraft{
			ID:     "draft-123",
			RaceID: dnd5e.RaceElf,
		}

		s.mockExternal.EXPECT().
			GetRaceData(s.ctx, dnd5e.RaceElf).
			Return(nil, errors.New("API error"))

		result, err := s.converter.HydrateDraft(s.ctx, draft)
		s.Error(err)
		s.Contains(err.Error(), "failed to get race data")
		s.Nil(result)
	})

	s.Run("hydrates all fields successfully", func() {
		draft := &dnd5e.CharacterDraft{
			ID:           "draft-123",
			RaceID:       dnd5e.RaceElf,
			SubraceID:    dnd5e.SubraceWoodElf,
			ClassID:      dnd5e.ClassRanger,
			BackgroundID: dnd5e.BackgroundOutlander,
		}

		// Mock race data with subraces
		raceData := &external.RaceData{
			ID:   dnd5e.RaceElf,
			Name: "Elf",
			Subraces: []*external.SubraceData{
				{
					ID:          dnd5e.SubraceWoodElf,
					Name:        "Wood Elf",
					Description: "Swift and stealthy",
					AbilityBonuses: map[string]int32{
						"wisdom": 1,
					},
				},
			},
		}

		classData := &external.ClassData{
			ID:      dnd5e.ClassRanger,
			Name:    "Ranger",
			HitDice: "1d10",
		}

		backgroundData := &external.BackgroundData{
			ID:                 dnd5e.BackgroundOutlander,
			Name:               "Outlander",
			Description:        "You grew up in the wilds",
			SkillProficiencies: []string{"Athletics", "Survival"},
			Languages:          1,
		}

		s.mockExternal.EXPECT().
			GetRaceData(s.ctx, dnd5e.RaceElf).
			Return(raceData, nil)

		s.mockExternal.EXPECT().
			GetClassData(s.ctx, dnd5e.ClassRanger).
			Return(classData, nil)

		s.mockExternal.EXPECT().
			GetBackgroundData(s.ctx, dnd5e.BackgroundOutlander).
			Return(backgroundData, nil)

		result, err := s.converter.HydrateDraft(s.ctx, draft)
		s.NoError(err)
		s.NotNil(result)

		// Verify all fields are hydrated
		s.NotNil(result.Race)
		s.Equal("Elf", result.Race.Name)

		s.NotNil(result.Subrace)
		s.Equal("Wood Elf", result.Subrace.Name)

		s.NotNil(result.Class)
		s.Equal("Ranger", result.Class.Name)

		s.NotNil(result.Background)
		s.Equal("Outlander", result.Background.Name)
	})
}

func TestDraftConverterTestSuite(t *testing.T) {
	suite.Run(t, new(DraftConverterTestSuite))
}