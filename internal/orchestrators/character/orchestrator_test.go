package character

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	rpgerrors "github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/dice"
	dicemock "github.com/KirkDiggler/rpg-api/internal/orchestrators/dice/mock"
	idgenmock "github.com/KirkDiggler/rpg-api/internal/pkg/idgen/mock"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	characterdraft "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
	draftmock "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft/mock"
	dicesession "github.com/KirkDiggler/rpg-api/internal/repositories/dice_session"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/backgrounds"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character/choices"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/fightingstyles"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/languages"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/races"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/skills"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/spells"
)

type OrchestratorTestSuite struct {
	suite.Suite
	ctrl              *gomock.Controller
	mockDraftRepo     *draftmock.MockRepository
	mockCharacterRepo *charactermock.MockRepository
	mockDiceService   *dicemock.MockService
	mockIDGen         *idgenmock.MockGenerator
	mockDraftIDGen    *idgenmock.MockGenerator
	orchestrator      *Orchestrator
	ctx               context.Context

	// Reusable test data
	testDraftID     string
	testPlayerID    string
	testCharacterID string
	testDraft       *character.Draft

	// Valid class inputs for testing
	validFighter *character.SetClassInput
	validWizard  *character.SetClassInput
	validRogue   *character.SetClassInput

	// Valid race inputs for testing
	validHuman *character.SetRaceInput
	validElf   *character.SetRaceInput
	validDwarf *character.SetRaceInput

	// Valid background inputs
	validSoldier *character.SetBackgroundInput
	validSage    *character.SetBackgroundInput

	// Valid ability scores
	validFighterScores *character.SetAbilityScoresInput
	validWizardScores  *character.SetAbilityScoresInput
}

func (s *OrchestratorTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockDraftRepo = draftmock.NewMockRepository(s.ctrl)
	s.mockCharacterRepo = charactermock.NewMockRepository(s.ctrl)
	s.mockDiceService = dicemock.NewMockService(s.ctrl)
	s.mockIDGen = idgenmock.NewMockGenerator(s.ctrl)
	s.mockDraftIDGen = idgenmock.NewMockGenerator(s.ctrl)
	s.ctx = context.Background()

	config := &Config{
		DraftRepo:        s.mockDraftRepo,
		CharacterRepo:    s.mockCharacterRepo,
		DiceService:      s.mockDiceService,
		IDGenerator:      s.mockIDGen,
		DraftIDGenerator: s.mockDraftIDGen,
	}

	var err error
	s.orchestrator, err = New(config)
	s.Require().NoError(err)

	// Initialize test data
	s.testDraftID = "draft-123"
	s.testPlayerID = "player-456"
	s.testCharacterID = "char-789"

	// Initialize valid class inputs
	s.validFighter = &character.SetClassInput{
		ClassID: classes.Fighter,
		Choices: character.ClassChoices{
			Skills:        []skills.Skill{skills.Athletics, skills.Intimidation},
			FightingStyle: fightingstyles.Defense,
			Equipment: []character.EquipmentChoiceSelection{
				{ChoiceID: choices.FighterArmor, OptionID: "fighter-armor-a"},
				{
					ChoiceID:           choices.FighterWeaponsPrimary,
					OptionID:           "fighter-weapon-a",
					CategorySelections: []shared.EquipmentID{"longsword"}, // Option A: martial weapon + shield
				},
				{ChoiceID: choices.FighterWeaponsSecondary, OptionID: "fighter-ranged-a"},
				{ChoiceID: choices.FighterPack, OptionID: "fighter-pack-a"},
			},
		},
	}

	s.validWizard = &character.SetClassInput{
		ClassID: classes.Wizard,
		Choices: character.ClassChoices{
			Skills: []skills.Skill{skills.Arcana, skills.Investigation},
			// Wizard needs cantrips, spells, and equipment
			Cantrips: []spells.Spell{spells.FireBolt, spells.MageHand, spells.Light},
			Spells: []spells.Spell{
				spells.MagicMissile, spells.Shield,
				spells.Sleep, spells.CharmPerson,
				spells.DetectMagic, spells.Identify,
			},
			Equipment: []character.EquipmentChoiceSelection{
				{ChoiceID: choices.WizardWeaponsPrimary, OptionID: "wizard-weapon-a"},
				{ChoiceID: choices.WizardFocus, OptionID: "wizard-focus-a"},
				{ChoiceID: choices.WizardPack, OptionID: "wizard-pack-a"},
			},
		},
	}

	s.validRogue = &character.SetClassInput{
		ClassID: classes.Rogue,
		Choices: character.ClassChoices{
			Skills: []skills.Skill{skills.Stealth, skills.SleightOfHand, skills.Deception, skills.Acrobatics},
			// Rogue gets 4 skills
		},
	}

	// Initialize valid race inputs
	s.validHuman = &character.SetRaceInput{
		RaceID: races.Human,
		Choices: character.RaceChoices{
			Languages: []languages.Language{languages.Elvish}, // Humans must choose 1 language
		},
	}

	s.validElf = &character.SetRaceInput{
		RaceID:    races.Elf,
		SubraceID: races.HighElf,
		Choices:   character.RaceChoices{
			// High elves might have additional choices
		},
	}

	s.validDwarf = &character.SetRaceInput{
		RaceID:    races.Dwarf,
		SubraceID: races.MountainDwarf,
		// Mountain dwarves don't require additional choices
	}

	// Initialize valid background inputs
	s.validSoldier = &character.SetBackgroundInput{
		BackgroundID: backgrounds.Soldier,
	}

	s.validSage = &character.SetBackgroundInput{
		BackgroundID: backgrounds.Sage,
	}

	// Initialize valid ability scores
	s.validFighterScores = &character.SetAbilityScoresInput{
		Scores: shared.AbilityScores{
			abilities.STR: 15, // Primary
			abilities.DEX: 13,
			abilities.CON: 14, // Secondary
			abilities.INT: 10,
			abilities.WIS: 12,
			abilities.CHA: 8,
		},
		Method: "standard",
	}

	s.validWizardScores = &character.SetAbilityScoresInput{
		Scores: shared.AbilityScores{
			abilities.STR: 8,
			abilities.DEX: 14,
			abilities.CON: 13,
			abilities.INT: 15, // Primary for wizard
			abilities.WIS: 12,
			abilities.CHA: 10,
		},
		Method: "standard",
	}

	// Create a basic test draft
	s.testDraft = s.createTestDraft(s.testDraftID, s.testPlayerID)

	// Create a complete draft for finalization tests
	// Don't create completeDraft anymore - not needed for orchestration tests
}

// createTestDraft creates a basic draft for testing
func (s *OrchestratorTestSuite) createTestDraft(draftID, playerID string) *character.Draft {
	config := &character.DraftConfig{
		ID:       draftID,
		PlayerID: playerID,
	}
	draft, err := character.NewDraft(config)
	s.Require().NoError(err)
	return draft
}

// createCompleteDraft creates a draft with all 5 progress fields set (name, race, class, background, ability scores)
// NOTE: This draft uses Wizard to avoid equipment choice requirements. Fighter would require equipment choices
// which we handle separately in specific fighter tests.

func (s *OrchestratorTestSuite) SetupSubTest() {
	// Reset test data to fresh state for each subtest
	s.testDraft = s.createTestDraft(s.testDraftID, s.testPlayerID)
	// Don't create completeDraft anymore - not needed for orchestration tests

	// Reset valid inputs to pristine state (deep copy to avoid cross-test pollution)
	s.validFighter = &character.SetClassInput{
		ClassID: classes.Fighter,
		Choices: character.ClassChoices{
			Skills:        []skills.Skill{skills.Athletics, skills.Intimidation},
			FightingStyle: fightingstyles.Defense,
			Equipment: []character.EquipmentChoiceSelection{
				{ChoiceID: choices.FighterArmor, OptionID: "fighter-armor-a"},
				{
					ChoiceID:           choices.FighterWeaponsPrimary,
					OptionID:           "fighter-weapon-a",
					CategorySelections: []shared.EquipmentID{"longsword"}, // Option A: martial weapon + shield
				},
				{ChoiceID: choices.FighterWeaponsSecondary, OptionID: "fighter-ranged-a"},
				{ChoiceID: choices.FighterPack, OptionID: "fighter-pack-a"},
			},
		},
	}

	s.validHuman = &character.SetRaceInput{
		RaceID: races.Human,
		Choices: character.RaceChoices{
			Languages: []languages.Language{languages.Elvish},
		},
	}
}

func (s *OrchestratorTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

// Test Config validation
func (s *OrchestratorTestSuite) TestNew_InvalidConfig() {
	testCases := []struct {
		name   string
		config *Config
	}{
		{
			name:   "nil config",
			config: nil,
		},
		{
			name: "missing draft repo",
			config: &Config{
				DiceService:      s.mockDiceService,
				IDGenerator:      s.mockIDGen,
				DraftIDGenerator: s.mockDraftIDGen,
			},
		},
		{
			name: "missing dice service",
			config: &Config{
				DraftRepo:        s.mockDraftRepo,
				IDGenerator:      s.mockIDGen,
				DraftIDGenerator: s.mockDraftIDGen,
			},
		},
		{
			name: "missing ID generator",
			config: &Config{
				DraftRepo:        s.mockDraftRepo,
				DiceService:      s.mockDiceService,
				DraftIDGenerator: s.mockDraftIDGen,
			},
		},
		{
			name: "missing draft ID generator",
			config: &Config{
				DraftRepo:   s.mockDraftRepo,
				DiceService: s.mockDiceService,
				IDGenerator: s.mockIDGen,
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			orch, err := New(tc.config)
			s.Assert().Error(err)
			s.Assert().Nil(orch)
		})
	}
}

// CreateDraft tests
func (s *OrchestratorTestSuite) TestCreateDraft_Success() {
	input := &CreateDraftInput{
		PlayerID: s.testPlayerID,
	}

	s.mockDraftIDGen.EXPECT().Generate().Return(s.testDraftID)

	// Expect Create to be called with a draft that has the generated ID
	s.mockDraftRepo.EXPECT().Create(s.ctx, gomock.Any()).DoAndReturn(
		func(_ context.Context, input characterdraft.CreateInput) (*characterdraft.CreateOutput, error) {
			s.Assert().Equal(s.testDraftID, input.Draft.Data.ID)
			s.Assert().Equal(s.testPlayerID, input.Draft.Data.PlayerID)
			s.Assert().Equal(character.ProgressNone, input.Draft.Data.Progress)
			return &characterdraft.CreateOutput{Draft: input.Draft}, nil
		})

	output, err := s.orchestrator.CreateDraft(s.ctx, input)

	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().NotNil(output.Draft)
	s.Assert().Equal(s.testDraftID, output.Draft.ID)
	s.Assert().Equal(s.testPlayerID, output.Draft.PlayerID)
}

func (s *OrchestratorTestSuite) TestCreateDraft_InvalidInput() {
	testCases := []struct {
		name  string
		input *CreateDraftInput
	}{
		{
			name:  "nil input",
			input: nil,
		},
		{
			name:  "empty player ID",
			input: &CreateDraftInput{PlayerID: ""},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			output, err := s.orchestrator.CreateDraft(s.ctx, tc.input)
			s.Assert().Error(err)
			s.Assert().Nil(output)
		})
	}
}

func (s *OrchestratorTestSuite) TestCreateDraft_SaveError() {
	input := &CreateDraftInput{
		PlayerID: "player-123",
	}

	s.mockDraftIDGen.EXPECT().Generate().Return("draft-456")
	s.mockDraftRepo.EXPECT().Create(s.ctx, gomock.Any()).Return(nil, errors.New("save failed"))

	output, err := s.orchestrator.CreateDraft(s.ctx, input)

	s.Assert().Error(err)
	s.Assert().Nil(output)
}

// GetDraft tests
func (s *OrchestratorTestSuite) TestGetDraft_Success() {
	input := &GetDraftInput{
		DraftID: s.testDraftID,
	}

	// Use the reusable test draft
	testDraft := s.createTestDraft(s.testDraftID, s.testPlayerID)

	s.mockDraftRepo.EXPECT().Get(s.ctx, gomock.Any()).DoAndReturn(
		func(_ context.Context, input characterdraft.GetInput) (*characterdraft.GetOutput, error) {
			s.Assert().Equal(s.testDraftID, input.ID)
			return &characterdraft.GetOutput{Draft: &entities.CharacterDraft{Data: testDraft.ToData()}}, nil
		})

	output, err := s.orchestrator.GetDraft(s.ctx, input)

	s.Require().NoError(err)
	s.Require().NotNil(output)
	// output.Draft is CharacterDraft - compare the data inside
	s.Assert().Equal(testDraft.ToData(), output.Draft.Data)
	// Test draft has no progress set
	s.Assert().Equal(character.ProgressNone, output.Progress)
}

func (s *OrchestratorTestSuite) TestGetDraft_NotFound() {
	input := &GetDraftInput{
		DraftID: "draft-404",
	}

	s.mockDraftRepo.EXPECT().Get(s.ctx, gomock.Any()).Return(nil, errors.New("draft not found"))

	output, err := s.orchestrator.GetDraft(s.ctx, input)

	s.Assert().Error(err)
	s.Assert().Nil(output)
}

func (s *OrchestratorTestSuite) TestGetDraft_InvalidInput() {
	testCases := []struct {
		name  string
		input *GetDraftInput
	}{
		{
			name:  "nil input",
			input: nil,
		},
		{
			name:  "empty draft ID",
			input: &GetDraftInput{DraftID: ""},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			output, err := s.orchestrator.GetDraft(s.ctx, tc.input)
			s.Assert().Error(err)
			s.Assert().Nil(output)
		})
	}
}

// ListBackgrounds tests
func (s *OrchestratorTestSuite) TestListBackgrounds_Success() {
	input := &ListBackgroundsInput{}

	output, err := s.orchestrator.ListBackgrounds(s.ctx, input)

	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().NotEmpty(output.Backgrounds)

	// Verify we have expected backgrounds
	backgroundMap := make(map[backgrounds.Background]*backgrounds.Data)
	for _, bg := range output.Backgrounds {
		backgroundMap[bg.ID] = bg
	}

	// Check a few expected backgrounds
	s.Assert().Contains(backgroundMap, backgrounds.Acolyte)
	s.Assert().Contains(backgroundMap, backgrounds.Criminal)
	s.Assert().Contains(backgroundMap, backgrounds.Soldier)

	// Verify background data
	acolyte := backgroundMap[backgrounds.Acolyte]
	s.Assert().Equal("Acolyte", acolyte.Name())
	s.Assert().NotEmpty(acolyte.Description())
	s.Assert().Len(acolyte.Skills, 2)
	s.Assert().Contains(acolyte.Skills, skills.Insight)
	s.Assert().Contains(acolyte.Skills, skills.Religion)
}

func (s *OrchestratorTestSuite) TestListBackgrounds_NilInput() {
	// Should handle nil input gracefully
	output, err := s.orchestrator.ListBackgrounds(s.ctx, nil)

	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().NotEmpty(output.Backgrounds)
}

// DeleteDraft tests
func (s *OrchestratorTestSuite) TestDeleteDraft_Success() {
	input := &DeleteDraftInput{
		DraftID: "draft-123",
	}

	s.mockDraftRepo.EXPECT().Delete(s.ctx, gomock.Any()).DoAndReturn(
		func(_ context.Context, input characterdraft.DeleteInput) (*characterdraft.DeleteOutput, error) {
			s.Assert().Equal("draft-123", input.ID)
			return &characterdraft.DeleteOutput{}, nil
		})

	output, err := s.orchestrator.DeleteDraft(s.ctx, input)

	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().True(output.Success)
}

func (s *OrchestratorTestSuite) TestDeleteDraft_Error() {
	input := &DeleteDraftInput{
		DraftID: "draft-123",
	}

	s.mockDraftRepo.EXPECT().Delete(s.ctx, gomock.Any()).Return(nil, errors.New("delete failed"))

	output, err := s.orchestrator.DeleteDraft(s.ctx, input)

	s.Assert().Error(err)
	s.Assert().Nil(output)
}

// SetName tests
func (s *OrchestratorTestSuite) TestSetName_Success() {
	input := &SetNameInput{
		DraftID: "draft-123",
		Name:    "Aragorn",
	}

	// Create a test draft
	testDraftConfig := &character.DraftConfig{
		ID:       "draft-123",
		PlayerID: "player-456",
	}
	testDraft, err := character.NewDraft(testDraftConfig)
	s.Require().NoError(err)

	s.mockDraftRepo.EXPECT().Get(s.ctx, gomock.Any()).DoAndReturn(
		func(_ context.Context, input characterdraft.GetInput) (*characterdraft.GetOutput, error) {
			s.Assert().Equal("draft-123", input.ID)
			return &characterdraft.GetOutput{Draft: &entities.CharacterDraft{Data: testDraft.ToData()}}, nil
		})
	s.mockDraftRepo.EXPECT().Update(s.ctx, gomock.Any()).DoAndReturn(
		func(_ context.Context, input characterdraft.UpdateInput) (*characterdraft.UpdateOutput, error) {
			s.Assert().Equal("Aragorn", input.Draft.Data.Name)
			s.Assert().True(input.Draft.Data.Progress.Has(character.ProgressName))
			return &characterdraft.UpdateOutput{Draft: input.Draft}, nil
		})

	output, err := s.orchestrator.SetName(s.ctx, input)

	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().Equal("Aragorn", output.Draft.Name)
	s.Assert().True(output.Progress.Has(character.ProgressName))
}

func (s *OrchestratorTestSuite) TestSetName_InvalidInput() {
	testCases := []struct {
		name  string
		input *SetNameInput
	}{
		{
			name:  "nil input",
			input: nil,
		},
		{
			name:  "empty draft ID",
			input: &SetNameInput{DraftID: "", Name: "Aragorn"},
		},
		{
			name:  "empty name",
			input: &SetNameInput{DraftID: "draft-123", Name: ""},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			output, err := s.orchestrator.SetName(s.ctx, tc.input)
			s.Assert().Error(err)
			s.Assert().Nil(output)
		})
	}
}

// SetRace tests
func (s *OrchestratorTestSuite) TestSetRace_Success() {
	input := &SetRaceInput{
		DraftID: "draft-123",
		Input: &character.SetRaceInput{
			RaceID:    races.Elf,
			SubraceID: races.HighElf,
			Choices: character.RaceChoices{
				Languages: []languages.Language{languages.Elvish},
			},
		},
	}

	// Create a test draft
	testDraftConfig := &character.DraftConfig{
		ID:       "draft-123",
		PlayerID: "player-456",
	}
	testDraft, err := character.NewDraft(testDraftConfig)
	s.Require().NoError(err)

	s.mockDraftRepo.EXPECT().Get(s.ctx, gomock.Any()).DoAndReturn(
		func(_ context.Context, input characterdraft.GetInput) (*characterdraft.GetOutput, error) {
			s.Assert().Equal("draft-123", input.ID)
			return &characterdraft.GetOutput{Draft: &entities.CharacterDraft{Data: testDraft.ToData()}}, nil
		})
	s.mockDraftRepo.EXPECT().Update(s.ctx, gomock.Any()).DoAndReturn(
		func(_ context.Context, input characterdraft.UpdateInput) (*characterdraft.UpdateOutput, error) {
			s.Assert().Equal(races.Elf, input.Draft.Data.Race)
			s.Assert().Equal(races.HighElf, input.Draft.Data.Subrace)
			s.Assert().True(input.Draft.Data.Progress.Has(character.ProgressRace))
			return &characterdraft.UpdateOutput{Draft: input.Draft}, nil
		})

	output, err := s.orchestrator.SetRace(s.ctx, input)

	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().Equal(races.Elf, output.Draft.Race)
	s.Assert().True(output.Progress.Has(character.ProgressRace))
}

func (s *OrchestratorTestSuite) TestSetRace_InvalidInput() {
	testCases := []struct {
		name  string
		input *SetRaceInput
	}{
		{
			name:  "nil input",
			input: nil,
		},
		{
			name:  "empty draft ID",
			input: &SetRaceInput{DraftID: "", Input: &character.SetRaceInput{}},
		},
		{
			name:  "nil race input",
			input: &SetRaceInput{DraftID: "draft-123", Input: nil},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			output, err := s.orchestrator.SetRace(s.ctx, tc.input)
			s.Assert().Error(err)
			s.Assert().Nil(output)
		})
	}
}

// SetClass tests
func (s *OrchestratorTestSuite) TestSetClass_Success() {
	input := &SetClassInput{
		DraftID: "draft-123",
		Input: &character.SetClassInput{
			ClassID: classes.Fighter,
			Choices: character.ClassChoices{
				Skills:        []skills.Skill{skills.Athletics, skills.Intimidation},
				FightingStyle: fightingstyles.Defense,
			},
		},
	}

	// Create a test draft
	testDraftConfig := &character.DraftConfig{
		ID:       "draft-123",
		PlayerID: "player-456",
	}
	testDraft, err := character.NewDraft(testDraftConfig)
	s.Require().NoError(err)

	s.mockDraftRepo.EXPECT().Get(s.ctx, gomock.Any()).DoAndReturn(
		func(_ context.Context, input characterdraft.GetInput) (*characterdraft.GetOutput, error) {
			s.Assert().Equal("draft-123", input.ID)
			return &characterdraft.GetOutput{Draft: &entities.CharacterDraft{Data: testDraft.ToData()}}, nil
		})
	s.mockDraftRepo.EXPECT().Update(s.ctx, gomock.Any()).DoAndReturn(
		func(_ context.Context, input characterdraft.UpdateInput) (*characterdraft.UpdateOutput, error) {
			s.Assert().Equal(classes.Fighter, input.Draft.Data.Class)
			// ProgressClass won't be set without equipment choices
			s.Assert().False(input.Draft.Data.Progress.Has(character.ProgressClass))
			return &characterdraft.UpdateOutput{Draft: input.Draft}, nil
		})

	output, err := s.orchestrator.SetClass(s.ctx, input)

	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().Equal(classes.Fighter, output.Draft.Class)
	// ProgressClass won't be set without equipment choices
	s.Assert().False(output.Progress.Has(character.ProgressClass))
}

// SetBackground tests
func (s *OrchestratorTestSuite) TestSetBackground_Success() {
	input := &SetBackgroundInput{
		DraftID: "draft-123",
		Input: &character.SetBackgroundInput{
			BackgroundID: backgrounds.Soldier,
			Choices: character.BackgroundChoices{
				Languages: []languages.Language{languages.Orc, languages.Goblin},
			},
		},
	}

	// Create a test draft
	testDraftConfig := &character.DraftConfig{
		ID:       "draft-123",
		PlayerID: "player-456",
	}
	testDraft, err := character.NewDraft(testDraftConfig)
	s.Require().NoError(err)

	s.mockDraftRepo.EXPECT().Get(s.ctx, gomock.Any()).DoAndReturn(
		func(_ context.Context, input characterdraft.GetInput) (*characterdraft.GetOutput, error) {
			s.Assert().Equal("draft-123", input.ID)
			return &characterdraft.GetOutput{Draft: &entities.CharacterDraft{Data: testDraft.ToData()}}, nil
		})
	s.mockDraftRepo.EXPECT().Update(s.ctx, gomock.Any()).DoAndReturn(
		func(_ context.Context, input characterdraft.UpdateInput) (*characterdraft.UpdateOutput, error) {
			s.Assert().Equal(backgrounds.Soldier, input.Draft.Data.Background)
			s.Assert().True(input.Draft.Data.Progress.Has(character.ProgressBackground))
			return &characterdraft.UpdateOutput{Draft: input.Draft}, nil
		})

	output, err := s.orchestrator.SetBackground(s.ctx, input)

	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().Equal(backgrounds.Soldier, output.Draft.Background)
	s.Assert().True(output.Progress.Has(character.ProgressBackground))
}

// SetAbilityScores tests
func (s *OrchestratorTestSuite) TestSetAbilityScores_Success() {
	input := &SetAbilityScoresInput{
		DraftID: "draft-123",
		Input: &character.SetAbilityScoresInput{
			Scores: shared.AbilityScores{
				abilities.STR: 15,
				abilities.DEX: 14,
				abilities.CON: 13,
				abilities.INT: 12,
				abilities.WIS: 10,
				abilities.CHA: 8,
			},
			Method: "standard",
		},
	}

	// Create a test draft
	testDraftConfig := &character.DraftConfig{
		ID:       "draft-123",
		PlayerID: "player-456",
	}
	testDraft, err := character.NewDraft(testDraftConfig)
	s.Require().NoError(err)

	s.mockDraftRepo.EXPECT().Get(s.ctx, gomock.Any()).DoAndReturn(
		func(_ context.Context, input characterdraft.GetInput) (*characterdraft.GetOutput, error) {
			s.Assert().Equal("draft-123", input.ID)
			return &characterdraft.GetOutput{Draft: &entities.CharacterDraft{Data: testDraft.ToData()}}, nil
		})
	s.mockDraftRepo.EXPECT().Update(s.ctx, gomock.Any()).DoAndReturn(
		func(_ context.Context, input characterdraft.UpdateInput) (*characterdraft.UpdateOutput, error) {
			scores := input.Draft.Data.BaseAbilityScores
			s.Assert().Equal(15, scores[abilities.STR])
			s.Assert().Equal(14, scores[abilities.DEX])
			s.Assert().True(input.Draft.Data.Progress.Has(character.ProgressAbilityScores))
			return &characterdraft.UpdateOutput{Draft: input.Draft}, nil
		})

	output, err := s.orchestrator.SetAbilityScores(s.ctx, input)

	s.Require().NoError(err)
	s.Require().NotNil(output)
	scores := output.Draft.BaseAbilityScores
	s.Assert().Equal(15, scores[abilities.STR])
	s.Assert().True(output.Progress.Has(character.ProgressAbilityScores))
}

// ValidateDraft tests
func (s *OrchestratorTestSuite) TestValidateDraft_ReturnsToolkitResult() {
	input := &ValidateDraftInput{
		DraftID: s.testDraftID,
	}

	// Use any draft - we're just testing that orchestrator calls toolkit
	testDraft := s.createTestDraft(s.testDraftID, s.testPlayerID)
	s.mockDraftRepo.EXPECT().Get(s.ctx, gomock.Any()).DoAndReturn(
		func(_ context.Context, input characterdraft.GetInput) (*characterdraft.GetOutput, error) {
			s.Assert().Equal(s.testDraftID, input.ID)
			return &characterdraft.GetOutput{Draft: &entities.CharacterDraft{Data: testDraft.ToData()}}, nil
		})

	output, err := s.orchestrator.ValidateDraft(s.ctx, input)

	// Just verify the orchestrator retrieves the draft and returns toolkit's validation
	// We're not testing what the validation says - that's the toolkit's job
	s.Require().NoError(err, "ValidateDraft should not return error")
	s.Require().NotNil(output)
	s.Assert().NotNil(output.Progress, "Should return progress")
}

// FinalizeDraft tests
func (s *OrchestratorTestSuite) TestFinalizeDraft_IncompleteError() {
	input := &FinalizeDraftInput{
		DraftID: "draft-123",
	}

	// Create an incomplete test draft
	testDraftConfig := &character.DraftConfig{
		ID:       "draft-123",
		PlayerID: "player-456",
	}
	testDraft, err := character.NewDraft(testDraftConfig)
	s.Require().NoError(err)

	// Only set name - not complete
	testDraft.SetName(&character.SetNameInput{Name: "Aragorn"})

	s.mockDraftRepo.EXPECT().Get(s.ctx, gomock.Any()).DoAndReturn(
		func(_ context.Context, input characterdraft.GetInput) (*characterdraft.GetOutput, error) {
			s.Assert().Equal("draft-123", input.ID)
			return &characterdraft.GetOutput{Draft: &entities.CharacterDraft{Data: testDraft.ToData()}}, nil
		})

	output, err := s.orchestrator.FinalizeDraft(s.ctx, input)

	s.Assert().Error(err)
	s.Assert().Contains(err.Error(), "draft is incomplete")
	s.Assert().Nil(output)
}

// TestFighterSetClass tests that we properly save a fighter class to the repository
func (s *OrchestratorTestSuite) TestFighterSetClass() {
	// Create a draft and set it up with valid fighter
	draft := s.createTestDraft("draft-fighter-test", s.testPlayerID)

	// Set name
	err := draft.SetName(&character.SetNameInput{Name: "Boromir"})
	s.Require().NoError(err)

	// Use valid human
	err = draft.SetRace(s.validHuman)
	s.Require().NoError(err)

	// Use fighter with skills and fighting style but NO equipment
	fighterNoEquipment := &character.SetClassInput{
		ClassID: classes.Fighter,
		Choices: character.ClassChoices{
			Skills:        []skills.Skill{skills.Athletics, skills.Intimidation},
			FightingStyle: fightingstyles.Defense,
			// NO Equipment - this should cause validation to fail
		},
	}
	err = draft.SetClass(fighterNoEquipment)
	s.Require().NoError(err)

	// Use valid soldier background
	err = draft.SetBackground(s.validSoldier)
	s.Require().NoError(err)

	// Use valid fighter ability scores
	err = draft.SetAbilityScores(s.validFighterScores)
	s.Require().NoError(err)

	// Just verify the data was set correctly - toolkit handles validation
	s.Assert().Equal("Boromir", draft.Name())
	s.Assert().Equal(races.Human, draft.Race())
	s.Assert().Equal(classes.Fighter, draft.Class())
	s.Assert().Equal(backgrounds.Soldier, draft.Background())
}

// TestFighterCreationViaOrchestrator tests the full fighter creation flow through the orchestrator
func (s *OrchestratorTestSuite) TestFighterCreationViaOrchestrator() {
	// Create draft
	createDraftInput := &CreateDraftInput{
		PlayerID: s.testPlayerID,
	}

	s.mockDraftIDGen.EXPECT().Generate().Return("draft-fighter-test")
	s.mockDraftRepo.EXPECT().Create(s.ctx, gomock.Any()).Return(&characterdraft.CreateOutput{Draft: &entities.CharacterDraft{Data: &character.DraftData{ID: "draft-fighter-test", PlayerID: s.testPlayerID}}}, nil)

	createOutput, err := s.orchestrator.CreateDraft(s.ctx, createDraftInput)
	s.Require().NoError(err)
	s.Require().NotNil(createOutput)

	draftID := createOutput.Draft.ID

	// Use the valid fighter input from suite
	setClassInput := &SetClassInput{
		DraftID: draftID,
		Input:   s.validFighter,
	}

	// Create a draft to return from Get
	testDraft := s.createTestDraft(draftID, s.testPlayerID)
	s.mockDraftRepo.EXPECT().Get(s.ctx, gomock.Any()).DoAndReturn(
		func(_ context.Context, input characterdraft.GetInput) (*characterdraft.GetOutput, error) {
			s.Assert().Equal(draftID, input.ID)
			return &characterdraft.GetOutput{Draft: &entities.CharacterDraft{Data: testDraft.ToData()}}, nil
		})
	s.mockDraftRepo.EXPECT().Update(s.ctx, gomock.Any()).DoAndReturn(
		func(_ context.Context, input characterdraft.UpdateInput) (*characterdraft.UpdateOutput, error) {
			// Verify the fighter was set with fighting style
			s.Assert().Equal(classes.Fighter, input.Draft.Data.Class)

			// Extract choices from draft
			allChoices := input.Draft.Data.Choices
			var foundSkills []skills.Skill
			var foundFightingStyle string

			for _, choice := range allChoices {
				if choice.Category == shared.ChoiceSkills && choice.Source == shared.SourceClass {
					foundSkills = choice.SkillSelection
				}
				if choice.Category == shared.ChoiceFightingStyle && choice.Source == shared.SourceClass {
					if choice.FightingStyleSelection != nil {
						foundFightingStyle = *choice.FightingStyleSelection
					}
				}
			}

			s.Assert().Equal(fightingstyles.Defense, foundFightingStyle)
			s.Assert().Len(foundSkills, 2)
			return &characterdraft.UpdateOutput{Draft: input.Draft}, nil
		})

	setClassOutput, err := s.orchestrator.SetClass(s.ctx, setClassInput)
	s.Require().NoError(err)
	s.Require().NotNil(setClassOutput)
	s.Assert().Equal(classes.Fighter, setClassOutput.Draft.Class)
}

// TestGetRequirements tests getting requirements for character creation
func (s *OrchestratorTestSuite) TestGetRequirements_Success() {
	tests := []struct {
		name     string
		input    *GetRequirementsInput
		wantReqs bool
	}{
		{
			name: "fighter requirements",
			input: &GetRequirementsInput{
				Class: classes.Fighter,
				Level: 1,
			},
			wantReqs: true,
		},
		{
			name: "wizard requirements",
			input: &GetRequirementsInput{
				Class: classes.Wizard,
				Level: 1,
			},
			wantReqs: true,
		},
		{
			name: "human race requirements",
			input: &GetRequirementsInput{
				Race: races.Human,
			},
			wantReqs: true,
		},
		{
			name: "fighter and human combined",
			input: &GetRequirementsInput{
				Class: classes.Fighter,
				Race:  races.Human,
				Level: 1,
			},
			wantReqs: true,
		},
		{
			name:     "empty requirements",
			input:    &GetRequirementsInput{},
			wantReqs: false,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			result, err := s.orchestrator.GetRequirements(context.Background(), tt.input)
			s.NoError(err)
			s.NotNil(result)
			s.NotNil(result.Requirements)

			if tt.wantReqs {
				// Check that we got some requirements
				hasReqs := result.Requirements.Skills != nil ||
					result.Requirements.Languages != nil ||
					result.Requirements.Equipment != nil ||
					result.Requirements.Expertise != nil ||
					result.Requirements.FightingStyle != nil
				s.True(hasReqs, "expected to have some requirements")
			}
		})
	}
}

func (s *OrchestratorTestSuite) TestGetRequirements_Errors() {
	result, err := s.orchestrator.GetRequirements(context.Background(), nil)
	s.Error(err)
	s.Nil(result)
	s.Contains(err.Error(), "input is required")
}

// TestListRaces tests listing available races
func (s *OrchestratorTestSuite) TestListRaces() {
	result, err := s.orchestrator.ListRaces(context.Background(), &ListRacesInput{})
	s.NoError(err)
	s.NotNil(result)
	s.NotNil(result.Races)
	// Should return actual races from toolkit
	s.Greater(len(result.Races), 0, "Should return at least one race")
}

// TestListClasses tests listing available classes
func (s *OrchestratorTestSuite) TestListClasses() {
	result, err := s.orchestrator.ListClasses(context.Background(), &ListClassesInput{})
	s.NoError(err)
	s.NotNil(result)
	s.NotNil(result.Classes)
	// Should return actual classes from toolkit
	s.Greater(len(result.Classes), 0, "Should return at least one class")
}

// TestRollAbilityScores tests rolling ability scores
func (s *OrchestratorTestSuite) TestRollAbilityScores() {
	// Mock dice service expectations
	s.mockDiceService.EXPECT().
		RollAbilityScores(gomock.Any(), gomock.Any()).
		Return(&dice.RollAbilityScoresOutput{
			Rolls: []*dicesession.DiceRoll{
				{RollID: "roll-1", Total: 16, Dice: []int{6, 5, 5, 3}, Dropped: []int{3}},
				{RollID: "roll-2", Total: 14, Dice: []int{5, 5, 4, 2}, Dropped: []int{2}},
				{RollID: "roll-3", Total: 13, Dice: []int{4, 4, 5, 1}, Dropped: []int{1}},
				{RollID: "roll-4", Total: 12, Dice: []int{4, 4, 4, 3}, Dropped: []int{3}},
				{RollID: "roll-5", Total: 11, Dice: []int{4, 4, 3, 2}, Dropped: []int{2}},
				{RollID: "roll-6", Total: 9, Dice: []int{3, 3, 3, 2}, Dropped: []int{2}},
			},
			Session: &dicesession.DiceSession{
				EntityID: "draft-123",
				Context:  "ability_scores",
			},
		}, nil)

	result, err := s.orchestrator.RollAbilityScores(context.Background(), &RollAbilityScoresInput{
		DraftID: "draft-123",
		Method:  "standard",
	})

	s.NoError(err)
	s.NotNil(result)
	s.Equal(6, len(result.Rolls))
	s.Equal("draft-123:ability_scores", result.SessionID)

	// Verify first roll details
	s.Equal(16, result.Rolls[0].Total)
	s.Equal([]int{6, 5, 5, 3}, result.Rolls[0].Dice)
	s.Equal([]int{3}, result.Rolls[0].Dropped)
}

func (s *OrchestratorTestSuite) TestDeleteCharacter_Success() {
	characterID := "char-123"

	// Mock the repository delete call
	s.mockCharacterRepo.EXPECT().
		Delete(s.ctx, characterrepo.DeleteInput{
			ID: characterID,
		}).
		Return(&characterrepo.DeleteOutput{}, nil)

	// Call DeleteCharacter
	output, err := s.orchestrator.DeleteCharacter(s.ctx, &DeleteCharacterInput{
		CharacterID: characterID,
	})

	// Assert success
	s.NoError(err)
	s.NotNil(output)
}

func (s *OrchestratorTestSuite) TestDeleteCharacter_InvalidInput() {
	testCases := []struct {
		name  string
		input *DeleteCharacterInput
		error string
	}{
		{
			name:  "nil input",
			input: nil,
			error: "input is required",
		},
		{
			name:  "empty character ID",
			input: &DeleteCharacterInput{CharacterID: ""},
			error: "character ID is required",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			output, err := s.orchestrator.DeleteCharacter(s.ctx, tc.input)
			s.Error(err)
			s.Contains(err.Error(), tc.error)
			s.Nil(output)
		})
	}
}

func (s *OrchestratorTestSuite) TestDeleteCharacter_NotFound() {
	characterID := "char-404"

	// Mock the repository delete call to return not found
	s.mockCharacterRepo.EXPECT().
		Delete(s.ctx, characterrepo.DeleteInput{
			ID: characterID,
		}).
		Return(nil, rpgerrors.NotFound("character not found"))

	// Call DeleteCharacter
	output, err := s.orchestrator.DeleteCharacter(s.ctx, &DeleteCharacterInput{
		CharacterID: characterID,
	})

	// Assert error
	s.Error(err)
	s.Contains(err.Error(), "failed to delete character")
	s.Nil(output)
}

func (s *OrchestratorTestSuite) TestDeleteCharacter_RepositoryError() {
	characterID := "char-123"

	// Mock the repository delete call to return an error
	s.mockCharacterRepo.EXPECT().
		Delete(s.ctx, characterrepo.DeleteInput{
			ID: characterID,
		}).
		Return(nil, rpgerrors.Internal("database connection failed"))

	// Call DeleteCharacter
	output, err := s.orchestrator.DeleteCharacter(s.ctx, &DeleteCharacterInput{
		CharacterID: characterID,
	})

	// Assert error
	s.Error(err)
	s.Contains(err.Error(), "failed to delete character")
	s.Nil(output)
}

// Run the test suite
func TestOrchestratorSuite(t *testing.T) {
	suite.Run(t, new(OrchestratorTestSuite))
}
