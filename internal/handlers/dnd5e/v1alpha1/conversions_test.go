package v1alpha1_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/orchestrators/character/mock"
)

// ConversionsTestSuite tests that data flows correctly through the handler conversions
type ConversionsTestSuite struct {
	suite.Suite
	ctrl            *gomock.Controller
	mockCharService *charactermock.MockService
	handler         *v1alpha1.Handler
	ctx             context.Context
}

func (s *ConversionsTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockCharService = charactermock.NewMockService(s.ctrl)

	handler, err := v1alpha1.NewHandler(&v1alpha1.HandlerConfig{
		CharacterService: s.mockCharService,
	})
	s.Require().NoError(err)
	s.handler = handler
	s.ctx = context.Background()
}

func (s *ConversionsTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

// TestCreateDraftDataFlow verifies CharacterDraftData flows correctly through the handler
func (s *ConversionsTestSuite) TestCreateDraftDataFlow() {
	s.Run("full CharacterDraftData creates draft with all fields", func() {
		// Input request with all fields populated
		request := &dnd5ev1alpha1.CreateDraftRequest{
			PlayerId:  "player-123",
			SessionId: "session-456",
			InitialData: &dnd5ev1alpha1.CharacterDraftData{
				Name:       "Aragorn",
				Race:       dnd5ev1alpha1.Race_RACE_HUMAN,
				Class:      dnd5ev1alpha1.Class_CLASS_RANGER,
				Background: dnd5ev1alpha1.Background_BACKGROUND_OUTLANDER,
				AbilityScores: &dnd5ev1alpha1.AbilityScores{
					Strength:     15,
					Dexterity:    16,
					Constitution: 14,
					Intelligence: 12,
					Wisdom:       13,
					Charisma:     10,
				},
				Alignment: dnd5ev1alpha1.Alignment_ALIGNMENT_CHAOTIC_GOOD,
				Choices: []*dnd5ev1alpha1.ChoiceSelection{
					{
						ChoiceId:     "variant-feat",
						ChoiceType:   dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_FEAT,
						Source:       dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_RACE,
						SelectedKeys: []string{"alert"},
					},
				},
			},
		}

		// Mock the service to verify it receives correctly converted data
		s.mockCharService.EXPECT().
			CreateDraft(s.ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, input *character.CreateDraftInput) (*character.CreateDraftOutput, error) {
				// Verify the conversions happened correctly
				s.Equal("player-123", input.PlayerID)
				s.Equal("session-456", input.SessionID)
				s.NotNil(input.InitialData)
				s.Equal("Aragorn", input.InitialData.Name)
				s.Equal(dnd5e.RaceHuman, input.InitialData.RaceID)
				s.Equal(dnd5e.ClassRanger, input.InitialData.ClassID)
				s.Equal(dnd5e.BackgroundOutlander, input.InitialData.BackgroundID)
				s.Equal(dnd5e.AlignmentChaoticGood, input.InitialData.Alignment)

				// Verify ability scores
				s.NotNil(input.InitialData.AbilityScores)
				s.Equal(int32(15), input.InitialData.AbilityScores.Strength)
				s.Equal(int32(16), input.InitialData.AbilityScores.Dexterity)

				// Verify choices
				s.Len(input.InitialData.ChoiceSelections, 1)
				s.Equal("variant-feat", input.InitialData.ChoiceSelections[0].ChoiceID)
				s.Equal(dnd5e.ChoiceTypeFeat, input.InitialData.ChoiceSelections[0].ChoiceType)

				// Return a draft that tests the reverse conversion
				return &character.CreateDraftOutput{
					Draft: &dnd5e.CharacterDraft{
						ID:               "draft-789",
						PlayerID:         input.PlayerID,
						SessionID:        input.SessionID,
						Name:             input.InitialData.Name,
						RaceID:           input.InitialData.RaceID,
						ClassID:          input.InitialData.ClassID,
						BackgroundID:     input.InitialData.BackgroundID,
						Alignment:        input.InitialData.Alignment,
						AbilityScores:    input.InitialData.AbilityScores,
						ChoiceSelections: input.InitialData.ChoiceSelections,
						Progress: dnd5e.CreationProgress{
							StepsCompleted:       31, // Name(1) + Race(2) + Class(4) + Background(8) + AbilityScores(16) = 31
							CompletionPercentage: 71,
							CurrentStep:          dnd5e.CreationStepSkills,
						},
						CreatedAt: 1234567800,
						UpdatedAt: 1234567850,
					},
				}, nil
			})

		// Call the handler
		resp, err := s.handler.CreateDraft(s.ctx, request)
		s.NoError(err)
		s.NotNil(resp)
		s.NotNil(resp.Draft)

		// Verify the response has all fields properly converted back to proto
		s.Equal("draft-789", resp.Draft.Id)
		s.Equal("player-123", resp.Draft.PlayerId)
		s.Equal("session-456", resp.Draft.SessionId)
		s.Equal("Aragorn", resp.Draft.Name)

		// Verify enums are converted back correctly
		s.Equal(dnd5ev1alpha1.Alignment_ALIGNMENT_CHAOTIC_GOOD, resp.Draft.Alignment)

		// Verify ability scores
		s.NotNil(resp.Draft.AbilityScores)
		s.Equal(int32(15), resp.Draft.AbilityScores.Strength)
		s.Equal(int32(16), resp.Draft.AbilityScores.Dexterity)

		// Verify choices are converted back
		s.Len(resp.Draft.Choices, 1)
		s.Equal("variant-feat", resp.Draft.Choices[0].ChoiceId)
		s.Equal(dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_FEAT, resp.Draft.Choices[0].ChoiceType)
		s.Equal(dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_RACE, resp.Draft.Choices[0].Source)

		// Verify progress
		s.NotNil(resp.Draft.Progress)
		s.True(resp.Draft.Progress.HasName)
		s.True(resp.Draft.Progress.HasRace)
		s.True(resp.Draft.Progress.HasClass)
		s.True(resp.Draft.Progress.HasBackground)
		s.True(resp.Draft.Progress.HasAbilityScores)
		s.Equal(int32(71), resp.Draft.Progress.CompletionPercentage)
		s.Equal(dnd5ev1alpha1.CreationStep_CREATION_STEP_SKILLS, resp.Draft.Progress.CurrentStep)

		// Verify metadata
		s.NotNil(resp.Draft.Metadata)
		s.Equal(int64(1234567800), resp.Draft.Metadata.CreatedAt)
		s.Equal(int64(1234567850), resp.Draft.Metadata.UpdatedAt)
		// Discord fields are not used in the API service
		s.Equal("", resp.Draft.Metadata.DiscordChannelId)
		s.Equal("", resp.Draft.Metadata.DiscordMessageId)
	})

	s.Run("CharacterDraftData with ability score choices", func() {
		request := &dnd5ev1alpha1.CreateDraftRequest{
			PlayerId: "player-123",
			InitialData: &dnd5ev1alpha1.CharacterDraftData{
				Name: "Half-Elf Character",
				Choices: []*dnd5ev1alpha1.ChoiceSelection{
					{
						ChoiceId:   "half-elf-ability",
						ChoiceType: dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_UNSPECIFIED,
						Source:     dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_RACE,
						AbilityScoreChoices: []*dnd5ev1alpha1.AbilityScoreChoice{
							{
								Ability: dnd5ev1alpha1.Ability_ABILITY_STRENGTH,
								Bonus:   1,
							},
							{
								Ability: dnd5ev1alpha1.Ability_ABILITY_WISDOM,
								Bonus:   1,
							},
						},
					},
				},
			},
		}

		s.mockCharService.EXPECT().
			CreateDraft(s.ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, input *character.CreateDraftInput) (*character.CreateDraftOutput, error) {
				// Verify ability score choices are converted correctly
				s.Len(input.InitialData.ChoiceSelections, 1)
				choice := input.InitialData.ChoiceSelections[0]
				s.Len(choice.AbilityScoreChoices, 2)
				s.Equal(dnd5e.AbilityStrength, choice.AbilityScoreChoices[0].Ability)
				s.Equal(int32(1), choice.AbilityScoreChoices[0].Bonus)
				s.Equal(dnd5e.AbilityWisdom, choice.AbilityScoreChoices[1].Ability)
				s.Equal(int32(1), choice.AbilityScoreChoices[1].Bonus)

				// Return the draft with the same choices to test conversion back
				return &character.CreateDraftOutput{
					Draft: &dnd5e.CharacterDraft{
						ID:               "draft-half-elf",
						Name:             input.InitialData.Name,
						ChoiceSelections: input.InitialData.ChoiceSelections,
					},
				}, nil
			})

		resp, err := s.handler.CreateDraft(s.ctx, request)
		s.NoError(err)

		// Verify ability score choices are converted back to proto correctly
		s.Len(resp.Draft.Choices, 1)
		s.Len(resp.Draft.Choices[0].AbilityScoreChoices, 2)
		s.Equal(dnd5ev1alpha1.Ability_ABILITY_STRENGTH, resp.Draft.Choices[0].AbilityScoreChoices[0].Ability)
		s.Equal(int32(1), resp.Draft.Choices[0].AbilityScoreChoices[0].Bonus)
		s.Equal(dnd5ev1alpha1.Ability_ABILITY_WISDOM, resp.Draft.Choices[0].AbilityScoreChoices[1].Ability)
		s.Equal(int32(1), resp.Draft.Choices[0].AbilityScoreChoices[1].Bonus)
	})

	s.Run("unspecified enums are handled correctly", func() {
		request := &dnd5ev1alpha1.CreateDraftRequest{
			PlayerId: "player-123",
			InitialData: &dnd5ev1alpha1.CharacterDraftData{
				Name:       "Test Character",
				Race:       dnd5ev1alpha1.Race_RACE_UNSPECIFIED,
				Class:      dnd5ev1alpha1.Class_CLASS_UNSPECIFIED,
				Background: dnd5ev1alpha1.Background_BACKGROUND_UNSPECIFIED,
				Alignment:  dnd5ev1alpha1.Alignment_ALIGNMENT_UNSPECIFIED,
			},
		}

		s.mockCharService.EXPECT().
			CreateDraft(s.ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, input *character.CreateDraftInput) (*character.CreateDraftOutput, error) {
				// Verify unspecified enums convert to empty strings
				s.Empty(input.InitialData.RaceID)
				s.Empty(input.InitialData.ClassID)
				s.Empty(input.InitialData.BackgroundID)
				s.Empty(input.InitialData.Alignment)

				return &character.CreateDraftOutput{
					Draft: &dnd5e.CharacterDraft{
						ID:   "draft-test",
						Name: input.InitialData.Name,
						// Leave enums empty to test conversion back
					},
				}, nil
			})

		resp, err := s.handler.CreateDraft(s.ctx, request)
		s.NoError(err)

		// Verify empty strings convert back to nil info objects
		s.Nil(resp.Draft.Race)
		s.Nil(resp.Draft.Class)
		s.Nil(resp.Draft.Background)
		// Alignment is still a direct field, not an info object
		s.Equal(dnd5ev1alpha1.Alignment_ALIGNMENT_UNSPECIFIED, resp.Draft.Alignment)
	})
}

// TestDraftHydration verifies that hydrated drafts include full info objects
func (s *ConversionsTestSuite) TestDraftHydration() {
	s.Run("hydrated draft includes race, class, and background info", func() {
		request := &dnd5ev1alpha1.GetDraftRequest{
			DraftId: "draft-123",
		}

		// Mock returns a hydrated draft
		s.mockCharService.EXPECT().
			GetDraft(s.ctx, &character.GetDraftInput{DraftID: "draft-123"}).
			Return(&character.GetDraftOutput{
				Draft: &dnd5e.CharacterDraft{
					ID:           "draft-123",
					Name:         "Legolas",
					RaceID:       dnd5e.RaceElf,
					SubraceID:    dnd5e.SubraceWoodElf,
					ClassID:      dnd5e.ClassRanger,
					BackgroundID: dnd5e.BackgroundOutlander,
					// These info objects would be populated by hydrateDraft
					Race: &dnd5e.RaceInfo{
						ID:          dnd5e.RaceElf,
						Name:        "Elf",
						Description: "Elves are a magical people",
						Speed:       30,
						Size:        "medium",
						AbilityBonuses: map[string]int32{
							"dexterity": 2,
						},
						Traits: []dnd5e.RacialTrait{
							{
								Name:        "Darkvision",
								Description: "You can see in dim light within 60 feet",
							},
						},
					},
					Subrace: &dnd5e.SubraceInfo{
						ID:          dnd5e.SubraceWoodElf,
						Name:        "Wood Elf",
						Description: "Wood elves are swift",
						AbilityBonuses: map[string]int32{
							"wisdom": 1,
						},
					},
					Class: &dnd5e.ClassInfo{
						ID:                       dnd5e.ClassRanger,
						Name:                     "Ranger",
						Description:              "Warriors of the wilderness",
						HitDie:                   "1d10",
						PrimaryAbilities:         []string{"Dexterity", "Wisdom"},
						SavingThrowProficiencies: []string{"Strength", "Dexterity"},
						SkillChoicesCount:        3,
						AvailableSkills:          []string{"Animal Handling", "Athletics", "Insight"},
					},
					Background: &dnd5e.BackgroundInfo{
						ID:                 dnd5e.BackgroundOutlander,
						Name:               "Outlander",
						Description:        "You grew up in the wilds",
						SkillProficiencies: []string{"Athletics", "Survival"},
						Languages:          []string{dnd5e.LanguageElvish},
					},
				},
			}, nil)

		resp, err := s.handler.GetDraft(s.ctx, request)
		s.NoError(err)
		s.NotNil(resp.Draft)

		// Verify Race info is included in response
		s.NotNil(resp.Draft.Race)
		s.Equal("RACE_ELF", resp.Draft.Race.Id)
		s.Equal("Elf", resp.Draft.Race.Name)
		s.Equal("Elves are a magical people", resp.Draft.Race.Description)
		s.Equal(int32(30), resp.Draft.Race.Speed)
		s.Equal(dnd5ev1alpha1.Size_SIZE_MEDIUM, resp.Draft.Race.Size)
		s.Len(resp.Draft.Race.AbilityBonuses, 1)
		s.Equal(int32(2), resp.Draft.Race.AbilityBonuses["dexterity"])
		s.Len(resp.Draft.Race.Traits, 1)
		s.Equal("Darkvision", resp.Draft.Race.Traits[0].Name)

		// Verify Subrace info
		s.NotNil(resp.Draft.Subrace)
		s.Equal("Wood Elf", resp.Draft.Subrace.Name)
		s.Len(resp.Draft.Subrace.AbilityBonuses, 1)
		s.Equal(int32(1), resp.Draft.Subrace.AbilityBonuses["wisdom"])

		// Verify Class info
		s.NotNil(resp.Draft.Class)
		s.Equal("Ranger", resp.Draft.Class.Name)
		s.Equal("1d10", resp.Draft.Class.HitDie)
		s.Equal([]string{"Dexterity", "Wisdom"}, resp.Draft.Class.PrimaryAbilities)
		s.Equal(int32(3), resp.Draft.Class.SkillChoicesCount)

		// Verify Background info
		s.NotNil(resp.Draft.Background)
		if resp.Draft.Background != nil {
			s.T().Logf("Background: %+v", resp.Draft.Background)
		}
		s.Equal("Outlander", resp.Draft.Background.Name)
		s.Equal([]string{"Athletics", "Survival"}, resp.Draft.Background.SkillProficiencies)
		s.Len(resp.Draft.Background.Languages, 1)
		s.Equal(dnd5ev1alpha1.Language_LANGUAGE_ELVISH, resp.Draft.Background.Languages[0])
	})

	s.Run("non-hydrated draft has nil info objects", func() {
		request := &dnd5ev1alpha1.GetDraftRequest{
			DraftId: "draft-456",
		}

		// Mock returns a non-hydrated draft (just IDs, no info objects)
		s.mockCharService.EXPECT().
			GetDraft(s.ctx, &character.GetDraftInput{DraftID: "draft-456"}).
			Return(&character.GetDraftOutput{
				Draft: &dnd5e.CharacterDraft{
					ID:           "draft-456",
					Name:         "Gimli",
					RaceID:       dnd5e.RaceDwarf,
					ClassID:      dnd5e.ClassFighter,
					BackgroundID: dnd5e.BackgroundSoldier,
					// Info objects are nil
				},
			}, nil)

		resp, err := s.handler.GetDraft(s.ctx, request)
		s.NoError(err)
		s.NotNil(resp.Draft)

		// Verify info objects are nil when not hydrated
		s.Nil(resp.Draft.Race)
		s.Nil(resp.Draft.Class)
		s.Nil(resp.Draft.Background)

		// But the basic data should still be there
		s.Equal("draft-456", resp.Draft.Id)
		s.Equal("Gimli", resp.Draft.Name)
	})
}

// TestProgressTracking verifies progress is calculated and tracked correctly
func (s *ConversionsTestSuite) TestProgressTracking() {
	s.Run("progress reflects completed steps accurately", func() {
		testCases := []struct {
			name     string
			draft    *dnd5e.CharacterDraft
			expected struct {
				hasName          bool
				hasRace          bool
				hasClass         bool
				hasBackground    bool
				hasAbilityScores bool
				hasSkills        bool
				percentage       int32
				currentStep      dnd5ev1alpha1.CreationStep
			}
		}{
			{
				name: "empty draft",
				draft: &dnd5e.CharacterDraft{
					ID: "draft-empty",
					Progress: dnd5e.CreationProgress{
						StepsCompleted:       0,
						CompletionPercentage: 0,
						CurrentStep:          dnd5e.CreationStepName,
					},
				},
				expected: struct {
					hasName          bool
					hasRace          bool
					hasClass         bool
					hasBackground    bool
					hasAbilityScores bool
					hasSkills        bool
					percentage       int32
					currentStep      dnd5ev1alpha1.CreationStep
				}{
					hasName:          false,
					hasRace:          false,
					hasClass:         false,
					hasBackground:    false,
					hasAbilityScores: false,
					hasSkills:        false,
					percentage:       0,
					currentStep:      dnd5ev1alpha1.CreationStep_CREATION_STEP_NAME,
				},
			},
			{
				name: "name and race completed",
				draft: &dnd5e.CharacterDraft{
					ID:     "draft-partial",
					Name:   "Test Character",
					RaceID: dnd5e.RaceHuman,
					Progress: dnd5e.CreationProgress{
						StepsCompleted:       3, // Name(1) + Race(2) = 3
						CompletionPercentage: 28,
						CurrentStep:          dnd5e.CreationStepClass,
					},
				},
				expected: struct {
					hasName          bool
					hasRace          bool
					hasClass         bool
					hasBackground    bool
					hasAbilityScores bool
					hasSkills        bool
					percentage       int32
					currentStep      dnd5ev1alpha1.CreationStep
				}{
					hasName:          true,
					hasRace:          true,
					hasClass:         false,
					hasBackground:    false,
					hasAbilityScores: false,
					hasSkills:        false,
					percentage:       28,
					currentStep:      dnd5ev1alpha1.CreationStep_CREATION_STEP_CLASS,
				},
			},
			{
				name: "fully completed draft",
				draft: &dnd5e.CharacterDraft{
					ID:           "draft-complete",
					Name:         "Complete Character",
					RaceID:       dnd5e.RaceElf,
					ClassID:      dnd5e.ClassWizard,
					BackgroundID: dnd5e.BackgroundSage,
					AbilityScores: &dnd5e.AbilityScores{
						Strength:     10,
						Dexterity:    14,
						Constitution: 12,
						Intelligence: 18,
						Wisdom:       13,
						Charisma:     11,
					},
					Progress: dnd5e.CreationProgress{
						StepsCompleted:       63, // Name(1) + Race(2) + Class(4) + Background(8) + AbilityScores(16) + Skills(32) = 63
						CompletionPercentage: 100,
						CurrentStep:          dnd5e.CreationStepReview,
					},
				},
				expected: struct {
					hasName          bool
					hasRace          bool
					hasClass         bool
					hasBackground    bool
					hasAbilityScores bool
					hasSkills        bool
					percentage       int32
					currentStep      dnd5ev1alpha1.CreationStep
				}{
					hasName:          true,
					hasRace:          true,
					hasClass:         true,
					hasBackground:    true,
					hasAbilityScores: true,
					hasSkills:        true,
					percentage:       100,
					currentStep:      dnd5ev1alpha1.CreationStep_CREATION_STEP_REVIEW,
				},
			},
		}

		for _, tc := range testCases {
			s.Run(tc.name, func() {
				request := &dnd5ev1alpha1.GetDraftRequest{
					DraftId: tc.draft.ID,
				}

				s.mockCharService.EXPECT().
					GetDraft(s.ctx, &character.GetDraftInput{DraftID: tc.draft.ID}).
					Return(&character.GetDraftOutput{Draft: tc.draft}, nil)

				resp, err := s.handler.GetDraft(s.ctx, request)
				s.NoError(err)
				s.NotNil(resp.Draft.Progress)

				// Verify progress tracking
				s.Equal(tc.expected.hasName, resp.Draft.Progress.HasName)
				s.Equal(tc.expected.hasRace, resp.Draft.Progress.HasRace)
				s.Equal(tc.expected.hasClass, resp.Draft.Progress.HasClass)
				s.Equal(tc.expected.hasBackground, resp.Draft.Progress.HasBackground)
				s.Equal(tc.expected.hasAbilityScores, resp.Draft.Progress.HasAbilityScores)
				s.Equal(tc.expected.hasSkills, resp.Draft.Progress.HasSkills)
				s.Equal(tc.expected.percentage, resp.Draft.Progress.CompletionPercentage)
				s.Equal(tc.expected.currentStep, resp.Draft.Progress.CurrentStep)
			})
		}
	})
}

// TestValidationWarnings verifies that validation warnings are properly converted
func (s *ConversionsTestSuite) TestValidationWarnings() {
	s.Run("UpdateName returns validation warnings", func() {
		request := &dnd5ev1alpha1.UpdateNameRequest{
			DraftId: "draft-123",
			Name:    "X", // Very short name
		}

		s.mockCharService.EXPECT().
			UpdateName(s.ctx, gomock.Any()).
			Return(&character.UpdateNameOutput{
				Draft: &dnd5e.CharacterDraft{
					ID:   "draft-123",
					Name: "X",
				},
				Warnings: []character.ValidationWarning{
					{
						Type:    "suboptimal_choice",
						Field:   "name",
						Message: "Character name is very short",
					},
					{
						Type:    "missing_required",
						Field:   "race",
						Message: "Race is required to continue",
					},
				},
			}, nil)

		resp, err := s.handler.UpdateName(s.ctx, request)
		s.NoError(err)

		// Verify warnings are converted
		s.Len(resp.Warnings, 2)
		s.Equal("suboptimal_choice", resp.Warnings[0].Type)
		s.Equal("name", resp.Warnings[0].Field)
		s.Equal("Character name is very short", resp.Warnings[0].Message)

		s.Equal("missing_required", resp.Warnings[1].Type)
		s.Equal("race", resp.Warnings[1].Field)
		s.Equal("Race is required to continue", resp.Warnings[1].Message)
	})
}

func TestConversionsTestSuite(t *testing.T) {
	suite.Run(t, new(ConversionsTestSuite))
}
