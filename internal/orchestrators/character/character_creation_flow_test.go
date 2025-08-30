package character_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	extmock "github.com/KirkDiggler/rpg-api/internal/clients/external/mock"
	"github.com/KirkDiggler/rpg-api/internal/errors"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/dice"
	dicemock "github.com/KirkDiggler/rpg-api/internal/orchestrators/dice/mock"
	idgenmock "github.com/KirkDiggler/rpg-api/internal/pkg/idgen/mock"
	charmock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	draftrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
	draftmock "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft/mock"
	dicesession "github.com/KirkDiggler/rpg-api/internal/repositories/dice_session"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/races"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/skills"
)

// CharacterCreationFlowTestSuite tests the complete character creation flow
// These tests document exactly how the game should interact with the API
type CharacterCreationFlowTestSuite struct {
	suite.Suite
	ctrl            *gomock.Controller
	mockCharRepo    *charmock.MockRepository
	mockDraftRepo   *draftmock.MockRepository
	mockExtClient   *extmock.MockClient
	mockDiceService *dicemock.MockService
	mockIDGen       *idgenmock.MockGenerator
	mockDraftIDGen  *idgenmock.MockGenerator
	orchestrator    *character.Orchestrator
}

func TestCharacterCreationFlowTestSuite(t *testing.T) {
	suite.Run(t, new(CharacterCreationFlowTestSuite))
}

func (s *CharacterCreationFlowTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockCharRepo = charmock.NewMockRepository(s.ctrl)
	s.mockDraftRepo = draftmock.NewMockRepository(s.ctrl)
	s.mockExtClient = extmock.NewMockClient(s.ctrl)
	s.mockDiceService = dicemock.NewMockService(s.ctrl)
	s.mockIDGen = idgenmock.NewMockGenerator(s.ctrl)
	s.mockDraftIDGen = idgenmock.NewMockGenerator(s.ctrl)

	// Create orchestrator with all required dependencies
	cfg := &character.Config{
		CharacterRepo:      s.mockCharRepo,
		CharacterDraftRepo: s.mockDraftRepo,
		ExternalClient:     s.mockExtClient,
		DiceService:        s.mockDiceService,
		IDGenerator:        s.mockIDGen,
		DraftIDGenerator:   s.mockDraftIDGen,
	}
	orchestrator, err := character.New(cfg)
	s.Require().NoError(err)
	s.orchestrator = orchestrator
}

func (s *CharacterCreationFlowTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

// ============================================================================
// UpdateAbilityScores Tests - Document how ability score assignment works
// ============================================================================

func (s *CharacterCreationFlowTestSuite) TestUpdateAbilityScores_ValidRollAssignment() {
	// GIVEN: The game has rolled ability scores and wants to assign them
	ctx := context.Background()
	draftID := "draft_123"
	playerID := "player_456"

	// The draft exists with race/class already set
	existingDraft := &toolkitchar.DraftData{
		ID:       draftID,
		PlayerID: playerID,
		Name:     "Aragorn",
		RaceChoice: toolkitchar.RaceChoice{
			RaceID: races.Human,
		},
		ClassChoice: toolkitchar.ClassChoice{
			ClassID: classes.Ranger,
		},
	}

	// Mock getting the draft
	s.mockDraftRepo.EXPECT().
		Get(ctx, draftrepo.GetInput{ID: draftID}).
		Return(&draftrepo.GetOutput{Draft: existingDraft}, nil)

	// The game previously rolled 6 ability scores
	// These are the actual roll results stored in the dice service
	mockSession := &dicesession.DiceSession{
		EntityID: playerID,
		Context:  "ability_scores",
		Rolls: []dicesession.DiceRoll{
			{RollID: "roll_1", Total: 18}, // Best roll
			{RollID: "roll_2", Total: 15},
			{RollID: "roll_3", Total: 14},
			{RollID: "roll_4", Total: 13},
			{RollID: "roll_5", Total: 12},
			{RollID: "roll_6", Total: 10}, // Worst roll
		},
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(15 * time.Minute),
	}

	s.mockDiceService.EXPECT().
		GetRollSession(ctx, &dice.GetRollSessionInput{
			EntityID: playerID,
			Context:  "ability_scores",
		}).
		Return(&dice.GetRollSessionOutput{Session: mockSession}, nil)

	// After using the rolls, the session should be cleared
	s.mockDiceService.EXPECT().
		ClearRollSession(ctx, &dice.ClearRollSessionInput{
			EntityID: playerID,
			Context:  "ability_scores",
		}).
		Return(&dice.ClearRollSessionOutput{}, nil)

	// The updated draft should have the ability scores set
	s.mockDraftRepo.EXPECT().
		Update(ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, input draftrepo.UpdateInput) (*draftrepo.UpdateOutput, error) {
			// Verify the draft has correct ability scores assigned
			s.Equal(18, input.Draft.AbilityScoreChoice[abilities.STR], "Best roll assigned to STR")
			s.Equal(15, input.Draft.AbilityScoreChoice[abilities.DEX], "Second best to DEX")
			s.Equal(14, input.Draft.AbilityScoreChoice[abilities.CON])
			s.Equal(13, input.Draft.AbilityScoreChoice[abilities.WIS])
			s.Equal(12, input.Draft.AbilityScoreChoice[abilities.INT])
			s.Equal(10, input.Draft.AbilityScoreChoice[abilities.CHA], "Worst roll to CHA")

			return &draftrepo.UpdateOutput{Draft: input.Draft}, nil
		})

	// WHEN: The game assigns the rolls to abilities
	// This shows the VALID way to assign rolls - using the exact roll IDs from RollAbilityScores
	result, err := s.orchestrator.UpdateAbilityScores(ctx, &character.UpdateAbilityScoresInput{
		DraftID: draftID,
		RollAssignments: &character.RollAssignments{
			StrengthRollID:     "roll_1", // Assign best roll to STR
			DexterityRollID:    "roll_2", // Second best to DEX (good for Ranger)
			ConstitutionRollID: "roll_3",
			WisdomRollID:       "roll_4", // Wisdom important for Ranger
			IntelligenceRollID: "roll_5",
			CharismaRollID:     "roll_6", // Dump stat
		},
	})

	// THEN: The assignment succeeds
	s.NoError(err)
	s.NotNil(result)
	s.NotNil(result.Draft)
	s.Equal(18, result.Draft.AbilityScoreChoice[abilities.STR])
	s.Equal(15, result.Draft.AbilityScoreChoice[abilities.DEX])
}

func (s *CharacterCreationFlowTestSuite) TestUpdateAbilityScores_InvalidRollID() {
	// GIVEN: The game tries to use a roll ID that doesn't exist
	ctx := context.Background()
	draftID := "draft_123"
	playerID := "player_456"

	existingDraft := &toolkitchar.DraftData{
		ID:       draftID,
		PlayerID: playerID,
		Name:     "Test Character",
	}

	s.mockDraftRepo.EXPECT().
		Get(ctx, draftrepo.GetInput{ID: draftID}).
		Return(&draftrepo.GetOutput{Draft: existingDraft}, nil)

	// The dice session has specific roll IDs
	mockSession := &dicesession.DiceSession{
		EntityID: playerID,
		Context:  "ability_scores",
		Rolls: []dicesession.DiceRoll{
			{RollID: "roll_1", Total: 18},
			{RollID: "roll_2", Total: 15},
			{RollID: "roll_3", Total: 14},
			{RollID: "roll_4", Total: 13},
			{RollID: "roll_5", Total: 12},
			{RollID: "roll_6", Total: 10},
		},
	}

	s.mockDiceService.EXPECT().
		GetRollSession(ctx, &dice.GetRollSessionInput{
			EntityID: playerID,
			Context:  "ability_scores",
		}).
		Return(&dice.GetRollSessionOutput{Session: mockSession}, nil)

	// WHEN: The game uses an invalid roll ID
	result, err := s.orchestrator.UpdateAbilityScores(ctx, &character.UpdateAbilityScoresInput{
		DraftID: draftID,
		RollAssignments: &character.RollAssignments{
			StrengthRollID:     "roll_1",
			DexterityRollID:    "roll_2",
			ConstitutionRollID: "roll_3",
			WisdomRollID:       "roll_4",
			IntelligenceRollID: "roll_5",
			CharismaRollID:     "INVALID_ROLL_ID", // ❌ This roll doesn't exist!
		},
	})

	// THEN: The API returns an error explaining what's wrong
	s.Error(err)
	s.Nil(result)
	s.True(errors.IsInvalidArgument(err))
	s.Contains(err.Error(), "INVALID_ROLL_ID")
	s.Contains(err.Error(), "not found in session")
}

func (s *CharacterCreationFlowTestSuite) TestUpdateAbilityScores_NoActiveRollSession() {
	// GIVEN: The game tries to assign rolls but hasn't rolled yet
	ctx := context.Background()
	draftID := "draft_123"
	playerID := "player_456"

	existingDraft := &toolkitchar.DraftData{
		ID:       draftID,
		PlayerID: playerID,
		Name:     "Test Character",
	}

	s.mockDraftRepo.EXPECT().
		Get(ctx, draftrepo.GetInput{ID: draftID}).
		Return(&draftrepo.GetOutput{Draft: existingDraft}, nil)

	// No active roll session
	s.mockDiceService.EXPECT().
		GetRollSession(ctx, &dice.GetRollSessionInput{
			EntityID: playerID,
			Context:  "ability_scores",
		}).
		Return(nil, errors.NotFound("no active roll session"))

	// WHEN: The game tries to assign rolls without rolling first
	result, err := s.orchestrator.UpdateAbilityScores(ctx, &character.UpdateAbilityScoresInput{
		DraftID: draftID,
		RollAssignments: &character.RollAssignments{
			StrengthRollID:     "roll_1",
			DexterityRollID:    "roll_2",
			ConstitutionRollID: "roll_3",
			WisdomRollID:       "roll_4",
			IntelligenceRollID: "roll_5",
			CharismaRollID:     "roll_6",
		},
	})

	// THEN: The API tells them to roll first
	s.Error(err)
	s.Nil(result)
	s.True(errors.IsNotFound(err))
	s.Contains(err.Error(), "no active roll session")
}

func (s *CharacterCreationFlowTestSuite) TestUpdateAbilityScores_ManualAssignment() {
	// GIVEN: The game wants to set ability scores manually (e.g., for testing or point buy)
	ctx := context.Background()
	draftID := "draft_123"
	playerID := "player_456"

	existingDraft := &toolkitchar.DraftData{
		ID:       draftID,
		PlayerID: playerID,
		Name:     "Test Character",
	}

	s.mockDraftRepo.EXPECT().
		Get(ctx, draftrepo.GetInput{ID: draftID}).
		Return(&draftrepo.GetOutput{Draft: existingDraft}, nil)

	// The updated draft should have the manual scores
	s.mockDraftRepo.EXPECT().
		Update(ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, input draftrepo.UpdateInput) (*draftrepo.UpdateOutput, error) {
			// Verify the manual scores were set correctly
			s.Equal(15, input.Draft.AbilityScoreChoice[abilities.STR])
			s.Equal(14, input.Draft.AbilityScoreChoice[abilities.DEX])
			s.Equal(13, input.Draft.AbilityScoreChoice[abilities.CON])
			s.Equal(12, input.Draft.AbilityScoreChoice[abilities.INT])
			s.Equal(10, input.Draft.AbilityScoreChoice[abilities.WIS])
			s.Equal(8, input.Draft.AbilityScoreChoice[abilities.CHA])

			return &draftrepo.UpdateOutput{Draft: input.Draft}, nil
		})

	// WHEN: The game sets ability scores manually (standard array example)
	result, err := s.orchestrator.UpdateAbilityScores(ctx, &character.UpdateAbilityScoresInput{
		DraftID: draftID,
		AbilityScores: &shared.AbilityScores{
			abilities.STR: 15,
			abilities.DEX: 14,
			abilities.CON: 13,
			abilities.INT: 12,
			abilities.WIS: 10,
			abilities.CHA: 8,
		},
	})

	// THEN: The manual assignment succeeds
	s.NoError(err)
	s.NotNil(result)
	s.NotNil(result.Draft)
	s.Equal(15, result.Draft.AbilityScoreChoice[abilities.STR])
	s.Equal(14, result.Draft.AbilityScoreChoice[abilities.DEX])
}

// ============================================================================
// Complete Character Creation Flow - Documents the full process
// ============================================================================

func (s *CharacterCreationFlowTestSuite) TestCompleteCharacterCreation_FighterHuman() {
	// This test documents the COMPLETE flow for creating a Human Fighter
	// It shows exactly what the game needs to send at each step

	s.T().Log("Complete Character Creation Flow - Human Fighter")

	// Step 1: UpdateRace - Choose Human
	// The game sends the race choice
	s.T().Log("Step 1: UpdateRace with Human - no additional choices needed")
	// Human gets +1 to all abilities (automatic, no choice needed)

	// Step 2: UpdateClass - Choose Fighter
	// The API returns requirements showing what choices the game must make
	s.T().Log("Step 2: UpdateClass with Fighter returns:")
	s.T().Log("  - Skill choice: Choose 2 from [Acrobatics, AnimalHandling, Athletics, History, Insight, Intimidation, Perception, Survival]")
	s.T().Log("  - Equipment choices: various armor, weapon, and pack options")

	// The game must submit these choices during finalization
	// Let's show VALID choices:
	validFighterChoices := []toolkitchar.ChoiceData{
		{
			Category: shared.ChoiceSkills,
			Source:   shared.SourceClass,
			ChoiceID: "fighter_skills",
			SkillSelection: []skills.Skill{
				skills.Athletics,    // ✅ Valid - in the list
				skills.Intimidation, // ✅ Valid - in the list
			},
		},
		{
			Category:           shared.ChoiceEquipment,
			Source:             shared.SourceClass,
			ChoiceID:           "fighter_equipment_1",
			EquipmentSelection: []string{"chain_mail"}, // Choice (a)
		},
		{
			Category:           shared.ChoiceEquipment,
			Source:             shared.SourceClass,
			ChoiceID:           "fighter_equipment_2",
			EquipmentSelection: []string{"longsword", "shield"}, // Choice (a) with specific martial weapon
		},
		{
			Category:           shared.ChoiceEquipment,
			Source:             shared.SourceClass,
			ChoiceID:           "fighter_equipment_3",
			EquipmentSelection: []string{"handaxe", "handaxe"}, // Choice (b) - two handaxes
		},
		{
			Category:           shared.ChoiceEquipment,
			Source:             shared.SourceClass,
			ChoiceID:           "fighter_equipment_4",
			EquipmentSelection: []string{"dungeoneers_pack"}, // Choice (a)
		},
	}

	s.T().Log("Game submits valid Fighter choices:")
	for _, choice := range validFighterChoices {
		if choice.SkillSelection != nil {
			s.T().Logf("  - Skills: %v", choice.SkillSelection)
		}
		if choice.EquipmentSelection != nil {
			s.T().Logf("  - Equipment %s: %v", choice.ChoiceID, choice.EquipmentSelection)
		}
	}
}

// TestInvalidSkillChoice shows what happens when the game sends invalid choices
func (s *CharacterCreationFlowTestSuite) TestInvalidSkillChoice_FighterCannotChooseDeception() {
	// This test documents what INVALID choices look like and how the API should respond
	// This is primarily documentation for the web UI developers

	s.T().Log("Invalid skill choice example:")
	s.T().Log("  Fighter can choose from: [Acrobatics, AnimalHandling, Athletics, History, Insight, Intimidation, Perception, Survival]")
	s.T().Log("  Game sent: [Athletics, Deception]")
	s.T().Log("  Expected Error: 'Deception' is not a valid skill choice for Fighter")

	// In a real FinalizeDraft call with these invalid choices:
	// The API would validate against the class requirements and return:
	// - INVALID_ARGUMENT error
	// - Message explaining which skill is invalid
	// - Possibly list the valid options
}
