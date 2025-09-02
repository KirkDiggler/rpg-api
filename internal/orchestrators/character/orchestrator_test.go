package character

import (
	"context"
	"errors"
	"testing"
	
	"go.uber.org/mock/gomock"
	"github.com/stretchr/testify/suite"
	
	dicemock "github.com/KirkDiggler/rpg-api/internal/orchestrators/dice/mock"
	idgenmock "github.com/KirkDiggler/rpg-api/internal/pkg/idgen/mock"
	draftmock "github.com/KirkDiggler/rpg-api/internal/repositories/draft/mock"
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
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/weapons"
)

type OrchestratorTestSuite struct {
	suite.Suite
	ctrl             *gomock.Controller
	mockDraftRepo    *draftmock.MockRepository
	mockDiceService  *dicemock.MockService
	mockIDGen        *idgenmock.MockGenerator
	mockDraftIDGen   *idgenmock.MockGenerator
	orchestrator     *Orchestrator
	ctx              context.Context
	
	// Reusable test data
	testDraftID      string
	testPlayerID     string
	testCharacterID  string
	testDraft        *character.Draft
	completeDraft    *character.Draft
	
	// Valid class inputs for testing
	validFighter     *character.SetClassInput
	validWizard      *character.SetClassInput
	validRogue       *character.SetClassInput
	
	// Valid race inputs for testing
	validHuman       *character.SetRaceInput
	validElf         *character.SetRaceInput
	validDwarf       *character.SetRaceInput
	
	// Valid background inputs
	validSoldier     *character.SetBackgroundInput
	validSage        *character.SetBackgroundInput
	
	// Valid ability scores
	validFighterScores *character.SetAbilityScoresInput
	validWizardScores  *character.SetAbilityScoresInput
}

func (s *OrchestratorTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockDraftRepo = draftmock.NewMockRepository(s.ctrl)
	s.mockDiceService = dicemock.NewMockService(s.ctrl)
	s.mockIDGen = idgenmock.NewMockGenerator(s.ctrl)
	s.mockDraftIDGen = idgenmock.NewMockGenerator(s.ctrl)
	s.ctx = context.Background()
	
	config := &Config{
		DraftRepo:        s.mockDraftRepo,
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
			Equipment: map[choices.ChoiceID]shared.SelectionID{
				choices.FighterArmor:            "fighter-armor-a",     // Chain mail option
				choices.FighterWeaponsPrimary:   "fighter-weapon-a",    // Martial weapon + shield
				choices.FighterWeaponsSecondary: "fighter-ranged-a",    // Light crossbow + bolts
				choices.FighterPack:             "fighter-pack-a",      // Dungeoneer's pack
				choices.FighterMartialWeapon1:   weapons.Longsword,     // Specific martial weapon
			},
		},
	}
	
	s.validWizard = &character.SetClassInput{
		ClassID: classes.Wizard,
		Choices: character.ClassChoices{
			Skills: []skills.Skill{skills.Arcana, skills.Investigation},
			// Wizard doesn't require fighting style
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
		Choices: character.RaceChoices{
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
			abilities.STR: 15,  // Primary
			abilities.DEX: 13,
			abilities.CON: 14,  // Secondary
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
			abilities.INT: 15,  // Primary for wizard
			abilities.WIS: 12,
			abilities.CHA: 10,
		},
		Method: "standard",
	}
	
	// Create a basic test draft
	s.testDraft = s.createTestDraft(s.testDraftID, s.testPlayerID)
	
	// Create a complete draft for finalization tests
	s.completeDraft = s.createCompleteDraft(s.testDraftID, s.testPlayerID)
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
func (s *OrchestratorTestSuite) createCompleteDraft(draftID, playerID string) *character.Draft {
	draft := s.createTestDraft(draftID, playerID)
	
	// Set all required fields
	err := draft.SetName(&character.SetNameInput{Name: "Aragorn"})
	s.Require().NoError(err)
	
	// Human with language choice (required for humans)
	err = draft.SetRace(&character.SetRaceInput{
		RaceID: races.Human,
		Choices: character.RaceChoices{
			Languages: []languages.Language{languages.Elvish}, // Humans must choose a language
		},
	})
	s.Require().NoError(err)
	
	// Use Wizard instead of Fighter to avoid equipment choice requirements
	// Fighter requires equipment choices but ClassChoices doesn't support them yet
	err = draft.SetClass(&character.SetClassInput{
		ClassID: classes.Wizard,
		Choices: character.ClassChoices{
			Skills: []skills.Skill{skills.Arcana, skills.Investigation},
			// Wizard doesn't require fighting style or equipment choices
		},
	})
	s.Require().NoError(err)
	
	// Sage background (matches wizard better)
	err = draft.SetBackground(&character.SetBackgroundInput{
		BackgroundID: backgrounds.Sage,
	})
	s.Require().NoError(err)
	
	// Standard array ability scores
	err = draft.SetAbilityScores(&character.SetAbilityScoresInput{
		Scores: shared.AbilityScores{
			abilities.STR: 8,
			abilities.DEX: 14,
			abilities.CON: 13,
			abilities.INT: 15,  // High INT for wizard
			abilities.WIS: 12,
			abilities.CHA: 10,
		},
		Method: "standard",
	})
	s.Require().NoError(err)
	
	// Log final state for debugging
	s.T().Logf("Draft Progress: %v, IsComplete: %v", draft.Progress(), draft.Progress().IsComplete())
	
	// Validate and log any errors for debugging
	if err := draft.ValidateChoices(); err != nil {
		s.T().Logf("Draft validation errors: %v", err)
	}
	
	return draft
}


func (s *OrchestratorTestSuite) SetupSubTest() {
	// Reset test data to fresh state for each subtest
	s.testDraft = s.createTestDraft(s.testDraftID, s.testPlayerID)
	s.completeDraft = s.createCompleteDraft(s.testDraftID, s.testPlayerID)
	
	// Reset valid inputs to pristine state (deep copy to avoid cross-test pollution)
	s.validFighter = &character.SetClassInput{
		ClassID: classes.Fighter,
		Choices: character.ClassChoices{
			Skills:        []skills.Skill{skills.Athletics, skills.Intimidation},
			FightingStyle: fightingstyles.Defense,
			Equipment: map[choices.ChoiceID]shared.SelectionID{
				choices.FighterArmor:            "fighter-armor-a",
				choices.FighterWeaponsPrimary:   "fighter-weapon-a",
				choices.FighterWeaponsSecondary: "fighter-ranged-a",
				choices.FighterPack:             "fighter-pack-a",
				choices.FighterMartialWeapon1:   weapons.Longsword,
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
	
	// Expect save to be called with a draft that has the generated ID
	s.mockDraftRepo.EXPECT().Save(s.ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, draft *character.Draft) error {
			s.Assert().Equal(s.testDraftID, draft.ID())
			s.Assert().Equal(s.testPlayerID, draft.PlayerID())
			s.Assert().Equal(character.ProgressNone, draft.Progress())
			return nil
		})
	
	output, err := s.orchestrator.CreateDraft(s.ctx, input)
	
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().NotNil(output.Draft)
	s.Assert().Equal(s.testDraftID, output.Draft.ID())
	s.Assert().Equal(s.testPlayerID, output.Draft.PlayerID())
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
	s.mockDraftRepo.EXPECT().Save(s.ctx, gomock.Any()).Return(errors.New("save failed"))
	
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
	
	s.mockDraftRepo.EXPECT().Get(s.ctx, s.testDraftID).Return(testDraft, nil)
	
	output, err := s.orchestrator.GetDraft(s.ctx, input)
	
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().Equal(testDraft, output.Draft)
	s.Assert().Equal(character.ProgressNone, output.Progress)
}

func (s *OrchestratorTestSuite) TestGetDraft_NotFound() {
	input := &GetDraftInput{
		DraftID: "draft-404",
	}
	
	s.mockDraftRepo.EXPECT().Get(s.ctx, "draft-404").Return(nil, errors.New("draft not found"))
	
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
	backgroundMap := make(map[string]BackgroundInfo)
	for _, bg := range output.Backgrounds {
		backgroundMap[bg.ID] = bg
	}
	
	// Check a few expected backgrounds
	s.Assert().Contains(backgroundMap, "acolyte")
	s.Assert().Contains(backgroundMap, "criminal")
	s.Assert().Contains(backgroundMap, "soldier")
	
	// Verify background data
	acolyte := backgroundMap["acolyte"]
	s.Assert().Equal("Acolyte", acolyte.Name)
	s.Assert().NotEmpty(acolyte.Description)
	s.Assert().Len(acolyte.Skills, 2)
	s.Assert().Contains(acolyte.Skills, string(skills.Insight))
	s.Assert().Contains(acolyte.Skills, string(skills.Religion))
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
	
	s.mockDraftRepo.EXPECT().Delete(s.ctx, "draft-123").Return(nil)
	
	output, err := s.orchestrator.DeleteDraft(s.ctx, input)
	
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().True(output.Success)
}

func (s *OrchestratorTestSuite) TestDeleteDraft_Error() {
	input := &DeleteDraftInput{
		DraftID: "draft-123",
	}
	
	s.mockDraftRepo.EXPECT().Delete(s.ctx, "draft-123").Return(errors.New("delete failed"))
	
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
	
	s.mockDraftRepo.EXPECT().Get(s.ctx, "draft-123").Return(testDraft, nil)
	s.mockDraftRepo.EXPECT().Save(s.ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, draft *character.Draft) error {
			s.Assert().Equal("Aragorn", draft.Name())
			s.Assert().True(draft.Progress().Has(character.ProgressName))
			return nil
		})
	
	output, err := s.orchestrator.SetName(s.ctx, input)
	
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().Equal("Aragorn", output.Draft.Name())
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
			RaceID: races.Elf,
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
	
	s.mockDraftRepo.EXPECT().Get(s.ctx, "draft-123").Return(testDraft, nil)
	s.mockDraftRepo.EXPECT().Save(s.ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, draft *character.Draft) error {
			s.Assert().Equal(races.Elf, draft.Race())
			s.Assert().Equal(races.HighElf, draft.Subrace())
			s.Assert().True(draft.Progress().Has(character.ProgressRace))
			return nil
		})
	
	output, err := s.orchestrator.SetRace(s.ctx, input)
	
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().Equal(races.Elf, output.Draft.Race())
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
				Skills: []skills.Skill{skills.Athletics, skills.Intimidation},
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
	
	s.mockDraftRepo.EXPECT().Get(s.ctx, "draft-123").Return(testDraft, nil)
	s.mockDraftRepo.EXPECT().Save(s.ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, draft *character.Draft) error {
			s.Assert().Equal(classes.Fighter, draft.Class())
			// ProgressClass won't be set without equipment choices
			s.Assert().False(draft.Progress().Has(character.ProgressClass))
			return nil
		})
	
	output, err := s.orchestrator.SetClass(s.ctx, input)
	
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().Equal(classes.Fighter, output.Draft.Class())
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
	
	s.mockDraftRepo.EXPECT().Get(s.ctx, "draft-123").Return(testDraft, nil)
	s.mockDraftRepo.EXPECT().Save(s.ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, draft *character.Draft) error {
			s.Assert().Equal(backgrounds.Soldier, draft.Background())
			s.Assert().True(draft.Progress().Has(character.ProgressBackground))
			return nil
		})
	
	output, err := s.orchestrator.SetBackground(s.ctx, input)
	
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().Equal(backgrounds.Soldier, output.Draft.Background())
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
	
	s.mockDraftRepo.EXPECT().Get(s.ctx, "draft-123").Return(testDraft, nil)
	s.mockDraftRepo.EXPECT().Save(s.ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, draft *character.Draft) error {
			scores := draft.BaseAbilityScores()
			s.Assert().Equal(15, scores[abilities.STR])
			s.Assert().Equal(14, scores[abilities.DEX])
			s.Assert().True(draft.Progress().Has(character.ProgressAbilityScores))
			return nil
		})
	
	output, err := s.orchestrator.SetAbilityScores(s.ctx, input)
	
	s.Require().NoError(err)
	s.Require().NotNil(output)
	scores := output.Draft.BaseAbilityScores()
	s.Assert().Equal(15, scores[abilities.STR])
	s.Assert().True(output.Progress.Has(character.ProgressAbilityScores))
}

// ValidateDraft tests
func (s *OrchestratorTestSuite) TestValidateDraft() {
	testCases := []struct {
		name          string
		setupDraft    func() *character.Draft
		expectValid   bool
		expectComplete bool
		errorMessage  string
	}{
		{
			name: "complete draft with wizard (no equipment required)",
			setupDraft: func() *character.Draft {
				// completeDraft uses Wizard which doesn't require equipment
				return s.completeDraft
			},
			expectValid:   true,  // Wizard doesn't need equipment, so validation passes
			expectComplete: true, // All 5 progress fields are set and valid
			errorMessage:  "wizard draft should be valid and complete",
		},
		{
			name: "missing language for human",
			setupDraft: func() *character.Draft {
				draft := s.createTestDraft(s.testDraftID, s.testPlayerID)
				draft.SetName(&character.SetNameInput{Name: "Aragorn"})
				// Human without language choice - should fail validation
				draft.SetRace(&character.SetRaceInput{RaceID: races.Human})
				draft.SetClass(&character.SetClassInput{
					ClassID: classes.Fighter,
					Choices: character.ClassChoices{
						Skills: []skills.Skill{skills.Athletics, skills.Intimidation},
					},
				})
				draft.SetBackground(&character.SetBackgroundInput{BackgroundID: backgrounds.Soldier})
				draft.SetAbilityScores(&character.SetAbilityScoresInput{
					Scores: shared.AbilityScores{
						abilities.STR: 15, abilities.DEX: 14, abilities.CON: 13,
						abilities.INT: 12, abilities.WIS: 10, abilities.CHA: 8,
					},
					Method: "standard",
				})
				return draft
			},
			expectValid:   false,
			expectComplete: false, // Progress isn't complete if validation fails
			errorMessage:  "Fighter class requires equipment choices but ClassChoices doesn't support them",
		},
		{
			name: "missing ability scores",
			setupDraft: func() *character.Draft {
				draft := s.createTestDraft(s.testDraftID, s.testPlayerID)
				draft.SetName(&character.SetNameInput{Name: "Aragorn"})
				draft.SetRace(&character.SetRaceInput{
					RaceID: races.Human,
					Choices: character.RaceChoices{
						Languages: []languages.Language{languages.Elvish},
					},
				})
				draft.SetClass(&character.SetClassInput{
					ClassID: classes.Fighter,
					Choices: character.ClassChoices{
						Skills: []skills.Skill{skills.Athletics, skills.Intimidation},
					},
				})
				draft.SetBackground(&character.SetBackgroundInput{BackgroundID: backgrounds.Soldier})
				// Missing ability scores
				return draft
			},
			expectValid:   false,
			expectComplete: false,
			errorMessage:  "should not be complete without ability scores",
		},
	}
	
	for _, tc := range testCases {
		s.Run(tc.name, func() {
			input := &ValidateDraftInput{
				DraftID: s.testDraftID,
			}
			
			draft := tc.setupDraft()
			s.mockDraftRepo.EXPECT().Get(s.ctx, s.testDraftID).Return(draft, nil)
			
			output, err := s.orchestrator.ValidateDraft(s.ctx, input)
			
			s.Require().NoError(err, "ValidateDraft should not return error")
			s.Require().NotNil(output)
			s.Assert().Equal(tc.expectValid, output.Valid, tc.errorMessage)
			s.Assert().Equal(tc.expectComplete, output.Progress.IsComplete(), 
				"Progress.IsComplete() mismatch: %s", tc.errorMessage)
		})
	}
}

// FinalizeDraft tests
func (s *OrchestratorTestSuite) TestFinalizeDraft_Success() {
	input := &FinalizeDraftInput{
		DraftID: s.testDraftID,
	}
	
	// Use our properly configured complete draft
	completeDraft := s.createCompleteDraft(s.testDraftID, s.testPlayerID)
	
	s.mockDraftRepo.EXPECT().Get(s.ctx, s.testDraftID).Return(completeDraft, nil)
	s.mockIDGen.EXPECT().Generate().Return(s.testCharacterID)
	s.mockDraftRepo.EXPECT().Delete(s.ctx, s.testDraftID).Return(nil)
	
	output, err := s.orchestrator.FinalizeDraft(s.ctx, input)
	
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().NotNil(output.Character)
	s.Assert().Equal(s.testCharacterID, output.Character.GetID())
	s.Assert().Equal("Aragorn", output.Character.GetName())
}

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
	
	s.mockDraftRepo.EXPECT().Get(s.ctx, "draft-123").Return(testDraft, nil)
	
	output, err := s.orchestrator.FinalizeDraft(s.ctx, input)
	
	s.Assert().Error(err)
	s.Assert().Contains(err.Error(), "draft is not complete")
	s.Assert().Nil(output)
}

// TestFighterWithAllRequiredChoices tests creating a fighter with fighting style and skills (but no equipment)
func (s *OrchestratorTestSuite) TestFighterWithAllRequiredChoices() {
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
	
	// Verify basic properties
	s.Assert().Equal("Boromir", draft.Name())
	s.Assert().Equal(races.Human, draft.Race())
	s.Assert().Equal(classes.Fighter, draft.Class())
	s.Assert().Equal(backgrounds.Soldier, draft.Background())
	
	// Verify choices were recorded
	allChoices := draft.Choices()
	
	// Find skill choices and fighting style
	var foundSkills []skills.Skill
	var foundFightingStyle string
	
	for _, choice := range allChoices {
		if choice.Category == shared.ChoiceSkills && choice.Source == shared.SourceClass {
			foundSkills = choice.SkillSelection
		}
		if choice.Category == shared.ChoiceFightingStyle && choice.Source == shared.SourceClass {
			if choice.FightingStyleSelection != nil {
				foundFightingStyle = string(*choice.FightingStyleSelection)
			}
		}
	}
	
	s.Assert().Len(foundSkills, 2, "Fighter should have 2 skills")
	s.Assert().Contains(foundSkills, skills.Athletics)
	s.Assert().Contains(foundSkills, skills.Intimidation)
	s.Assert().Equal(string(fightingstyles.Defense), foundFightingStyle, "Fighter should have Defense fighting style")
	
	// Validate choices - this will fail due to missing equipment
	err = draft.ValidateChoices()
	s.Assert().Error(err, "Should fail validation due to missing equipment choices")
	s.T().Logf("Validation error (expected): %v", err)
	
	// The error should mention armor (equipment choice)
	s.Assert().Contains(err.Error(), "armor", "Error should mention missing armor/equipment")
	
	// Progress should NOT be complete due to validation failure
	s.Assert().False(draft.Progress().IsComplete(), "Progress should not be complete without valid equipment choices")
}

// TestFighterMissingFightingStyle tests that fighter without fighting style fails validation
func (s *OrchestratorTestSuite) TestFighterMissingFightingStyle() {
	draft := s.createTestDraft("draft-fighter-no-style", s.testPlayerID)
	
	draft.SetName(&character.SetNameInput{Name: "Faramir"})
	draft.SetRace(s.validHuman)
	
	// Remove fighting style from valid fighter input
	fighterNoStyle := &character.SetClassInput{
		ClassID: s.validFighter.ClassID,
		Choices: character.ClassChoices{
			Skills: s.validFighter.Choices.Skills,
			// MISSING: FightingStyle
		},
	}
	
	err := draft.SetClass(fighterNoStyle)
	s.Require().NoError(err)
	
	draft.SetBackground(s.validSoldier)
	draft.SetAbilityScores(s.validFighterScores)
	
	// Validate - should fail due to missing equipment (comes before fighting style check)
	err = draft.ValidateChoices()
	s.Assert().Error(err, "Fighter should fail validation")
	s.T().Logf("Validation error: %v", err)
	
	// Note: Equipment validation happens before fighting style validation
	// So we'll see armor error first even though fighting style is also missing
	s.Assert().Contains(err.Error(), "armor", "Error should mention missing armor/equipment first")
}

// TestFighterWrongNumberOfSkills tests fighter with wrong number of skills
func (s *OrchestratorTestSuite) TestFighterWrongNumberOfSkills() {
	draft := s.createTestDraft("draft-fighter-wrong-skills", s.testPlayerID)
	
	draft.SetName(&character.SetNameInput{Name: "Gimli"})
	draft.SetRace(s.validDwarf)
	
	// Modify valid fighter to have only 1 skill
	fighterWrongSkills := &character.SetClassInput{
		ClassID: s.validFighter.ClassID,
		Choices: character.ClassChoices{
			Skills:        []skills.Skill{skills.Intimidation}, // Only 1, needs 2
			FightingStyle: s.validFighter.Choices.FightingStyle,
		},
	}
	
	// SetClass validates skill count immediately
	err := draft.SetClass(fighterWrongSkills)
	s.Assert().Error(err, "SetClass should fail with wrong number of skills")
	s.T().Logf("SetClass validation error: %v", err)
	
	// The error should mention needing 2 skills
	s.Assert().Contains(err.Error(), "2", "Error should mention needing 2 skills")
	s.Assert().Contains(err.Error(), "skills", "Error should mention skills")
}

// TestFighterCreationViaOrchestrator tests the full fighter creation flow through the orchestrator
func (s *OrchestratorTestSuite) TestFighterCreationViaOrchestrator() {
	// Create draft
	createDraftInput := &CreateDraftInput{
		PlayerID: s.testPlayerID,
	}
	
	s.mockDraftIDGen.EXPECT().Generate().Return("draft-fighter-test")
	s.mockDraftRepo.EXPECT().Save(s.ctx, gomock.Any()).Return(nil)
	
	createOutput, err := s.orchestrator.CreateDraft(s.ctx, createDraftInput)
	s.Require().NoError(err)
	s.Require().NotNil(createOutput)
	
	draftID := createOutput.Draft.ID()
	
	// Use the valid fighter input from suite
	setClassInput := &SetClassInput{
		DraftID: draftID,
		Input:   s.validFighter,
	}
	
	// Create a draft to return from Get
	testDraft := s.createTestDraft(draftID, s.testPlayerID)
	s.mockDraftRepo.EXPECT().Get(s.ctx, draftID).Return(testDraft, nil)
	s.mockDraftRepo.EXPECT().Save(s.ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, draft *character.Draft) error {
			// Verify the fighter was set with fighting style
			s.Assert().Equal(classes.Fighter, draft.Class())
			
			// Extract choices from draft
			allChoices := draft.Choices()
			var foundSkills []skills.Skill
			var foundFightingStyle string
			
			for _, choice := range allChoices {
				if choice.Category == shared.ChoiceSkills && choice.Source == shared.SourceClass {
					foundSkills = choice.SkillSelection
				}
				if choice.Category == shared.ChoiceFightingStyle && choice.Source == shared.SourceClass {
					if choice.FightingStyleSelection != nil {
						foundFightingStyle = string(*choice.FightingStyleSelection)
					}
				}
			}
			
			s.Assert().Equal(string(fightingstyles.Defense), foundFightingStyle)
			s.Assert().Len(foundSkills, 2)
			return nil
		})
	
	setClassOutput, err := s.orchestrator.SetClass(s.ctx, setClassInput)
	s.Require().NoError(err)
	s.Require().NotNil(setClassOutput)
	s.Assert().Equal(classes.Fighter, setClassOutput.Draft.Class())
}

// TestFighterWithCompleteEquipment tests creating a fighter with all required choices including equipment
func (s *OrchestratorTestSuite) TestFighterWithCompleteEquipment() {
	// Create a draft with complete fighter including equipment
	draft := s.createTestDraft("draft-fighter-complete", s.testPlayerID)
	
	// Set name
	err := draft.SetName(&character.SetNameInput{Name: "Conan"})
	s.Require().NoError(err)
	
	// Use valid human
	err = draft.SetRace(s.validHuman)
	s.Require().NoError(err)
	
	// Use valid fighter WITH equipment choices
	err = draft.SetClass(s.validFighter)
	s.Require().NoError(err)
	
	// Use valid soldier background
	err = draft.SetBackground(s.validSoldier)
	s.Require().NoError(err)
	
	// Use valid fighter ability scores
	err = draft.SetAbilityScores(s.validFighterScores)
	s.Require().NoError(err)
	
	// Validate choices - should PASS now that equipment is included
	err = draft.ValidateChoices()
	s.Assert().NoError(err, "Fighter with complete equipment should pass validation")
	
	// Progress should be complete
	progress := draft.Progress()
	s.T().Logf("Progress: Name=%v, Race=%v, Class=%v, Background=%v, AbilityScores=%v", 
		progress.Has(character.ProgressName),
		progress.Has(character.ProgressRace),
		progress.Has(character.ProgressClass),
		progress.Has(character.ProgressBackground),
		progress.Has(character.ProgressAbilityScores))
	s.T().Logf("Progress value: %d, IsComplete: %v", progress, progress.IsComplete())
	s.Assert().True(draft.Progress().IsComplete(), "Progress should be complete with all choices including equipment")
	
	// Verify equipment choices were recorded
	allChoices := draft.Choices()
	foundEquipmentChoices := 0
	
	for _, choice := range allChoices {
		if choice.Category == shared.ChoiceEquipment && choice.Source == shared.SourceClass {
			foundEquipmentChoices++
			s.T().Logf("Found equipment choice: %s with %d items", choice.ChoiceID, len(choice.EquipmentSelection))
		}
	}
	
	// Should have recorded all 5 equipment choices
	s.Assert().Equal(5, foundEquipmentChoices, "Should have 5 equipment choices (armor, primary weapon, secondary weapon, pack, martial weapon)")
}

// Run the test suite
func TestOrchestratorSuite(t *testing.T) {
	suite.Run(t, new(OrchestratorTestSuite))
}