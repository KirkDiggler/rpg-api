//go:build integration
// +build integration

package character_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-api/internal/clients/external"
	"github.com/KirkDiggler/rpg-api/internal/engine/rpgtoolkit"
	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	"github.com/KirkDiggler/rpg-api/internal/pkg/clock"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	characterdraftrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
	"github.com/KirkDiggler/rpg-api/internal/testutils"
	"github.com/KirkDiggler/rpg-toolkit/dice"
	"github.com/KirkDiggler/rpg-toolkit/events"
)

// OrchestratorIntegrationTestSuite tests the orchestrator with real Redis
type OrchestratorIntegrationTestSuite struct {
	suite.Suite

	ctx                context.Context
	orchestrator       *character.Orchestrator
	characterRepo      characterrepo.Repository
	characterDraftRepo characterdraftrepo.Repository
	redisCleanup       func()
}

func TestOrchestratorIntegrationSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	suite.Run(t, new(OrchestratorIntegrationTestSuite))
}

func (s *OrchestratorIntegrationTestSuite) SetupTest() {
	s.ctx = context.Background()

	// Create test Redis client
	redisClient, cleanup := testutils.CreateTestRedisClient(s.T())
	s.redisCleanup = cleanup

	// Create real repositories with test Redis
	charRepo, err := characterrepo.NewRedis(&characterrepo.RedisConfig{
		Client: redisClient,
	})
	s.Require().NoError(err)
	s.characterRepo = charRepo

	draftRepo, err := characterdraftrepo.NewRedis(&characterdraftrepo.Config{
		Clock:       clock.New(),
		IDGenerator: idgen.NewPrefixed("draft-"),
		Client:      redisClient,
	})
	s.Require().NoError(err)
	s.characterDraftRepo = draftRepo

	// Create external client - using real API for integration tests
	client, err := external.New(&external.Config{
		BaseURL:     "https://www.dnd5eapi.co/api/2014/",
		CacheTTL:    24 * time.Hour,
		HTTPTimeout: 30 * time.Second,
	})
	s.Require().NoError(err)

	// Create rpg-toolkit components
	eventBus := events.NewBus()
	diceRoller := dice.DefaultRoller

	// Create engine using rpg-toolkit adapter
	e, err := rpgtoolkit.NewAdapter(&rpgtoolkit.AdapterConfig{
		EventBus:       eventBus,
		DiceRoller:     diceRoller,
		ExternalClient: client,
	})
	s.Require().NoError(err)

	// Create orchestrator with real dependencies
	orchestrator, err := character.New(&character.Config{
		CharacterRepo:      s.characterRepo,
		CharacterDraftRepo: s.characterDraftRepo,
		Engine:             e,
		ExternalClient:     client,
	})
	s.Require().NoError(err)
	s.orchestrator = orchestrator
}

func (s *OrchestratorIntegrationTestSuite) TearDownTest() {
	if s.redisCleanup != nil {
		s.redisCleanup()
	}
}

// TestCompleteCharacterCreationFlow tests the entire character creation workflow
func (s *OrchestratorIntegrationTestSuite) TestCompleteCharacterCreationFlow() {
	playerID := "player-123"

	// Step 1: Create a draft
	createOutput, err := s.orchestrator.CreateDraft(s.ctx, &character.CreateDraftInput{
		PlayerID: playerID,
	})
	s.Require().NoError(err)
	s.Assert().NotEmpty(createOutput.Draft.ID)
	s.Assert().Equal(playerID, createOutput.Draft.PlayerID)
	s.Assert().Equal(int32(0), createOutput.Draft.Progress.CompletionPercentage)

	draftID := createOutput.Draft.ID

	// Step 2: Update name
	nameOutput, err := s.orchestrator.UpdateName(s.ctx, &character.UpdateNameInput{
		DraftID: draftID,
		Name:    "Aragorn",
	})
	s.Require().NoError(err)
	s.Assert().Equal("Aragorn", nameOutput.Draft.Name)
	s.Assert().True(nameOutput.Draft.Progress.HasName())
	s.Assert().Greater(nameOutput.Draft.Progress.CompletionPercentage, int32(0))

	// Step 3: Update race
	raceOutput, err := s.orchestrator.UpdateRace(s.ctx, &character.UpdateRaceInput{
		DraftID: draftID,
		RaceID:  dnd5e.RaceHuman,
	})
	s.Require().NoError(err)
	s.Assert().Equal(dnd5e.RaceHuman, raceOutput.Draft.RaceID)
	s.Assert().True(raceOutput.Draft.Progress.HasRace())

	// Step 4: Update class
	classOutput, err := s.orchestrator.UpdateClass(s.ctx, &character.UpdateClassInput{
		DraftID: draftID,
		ClassID: dnd5e.ClassRanger,
	})
	s.Require().NoError(err)
	s.Assert().Equal(dnd5e.ClassRanger, classOutput.Draft.ClassID)
	s.Assert().True(classOutput.Draft.Progress.HasClass())

	// Step 5: Update background
	bgOutput, err := s.orchestrator.UpdateBackground(s.ctx, &character.UpdateBackgroundInput{
		DraftID:      draftID,
		BackgroundID: dnd5e.BackgroundOutlander,
	})
	s.Require().NoError(err)
	s.Assert().Equal(dnd5e.BackgroundOutlander, bgOutput.Draft.BackgroundID)
	s.Assert().True(bgOutput.Draft.Progress.HasBackground())

	// Step 6: Update ability scores
	abilityOutput, err := s.orchestrator.UpdateAbilityScores(s.ctx, &character.UpdateAbilityScoresInput{
		DraftID: draftID,
		AbilityScores: dnd5e.AbilityScores{
			Strength:     13,
			Dexterity:    15,
			Constitution: 14,
			Intelligence: 12,
			Wisdom:       10,
			Charisma:     8,
		},
	})
	s.Require().NoError(err)
	s.Assert().NotNil(abilityOutput.Draft.AbilityScores)
	s.Assert().True(abilityOutput.Draft.Progress.HasAbilityScores())

	// Step 7: Update skills
	skillsOutput, err := s.orchestrator.UpdateSkills(s.ctx, &character.UpdateSkillsInput{
		DraftID:  draftID,
		SkillIDs: []string{dnd5e.SkillSurvival, dnd5e.SkillNature},
	})
	s.Require().NoError(err)
	// Skills are now handled through ChoiceSelections
	s.Assert().True(skillsOutput.Draft.Progress.HasSkills())

	// Step 8: Validate the draft is ready
	validateOutput, err := s.orchestrator.ValidateDraft(s.ctx, &character.ValidateDraftInput{
		DraftID: draftID,
	})
	s.Require().NoError(err)
	s.Assert().True(validateOutput.IsValid)
	// Note: IsComplete depends on language selection which we're skipping for now

	// Step 9: Finalize the draft
	// Skip for now since we need language selection and the engine needs full implementation
}

// TestDraftPersistence tests that drafts persist in Redis correctly
func (s *OrchestratorIntegrationTestSuite) TestDraftPersistence() {
	playerID := "player-456"

	// Create a draft
	createOutput, err := s.orchestrator.CreateDraft(s.ctx, &character.CreateDraftInput{
		PlayerID: playerID,
		InitialData: &dnd5e.CharacterDraft{
			Name:   "Legolas",
			RaceID: dnd5e.RaceElf,
		},
	})
	s.Require().NoError(err)
	draftID := createOutput.Draft.ID

	// Retrieve the draft
	getOutput, err := s.orchestrator.GetDraft(s.ctx, &character.GetDraftInput{
		DraftID: draftID,
	})
	s.Require().NoError(err)
	s.Assert().Equal("Legolas", getOutput.Draft.Name)
	s.Assert().Equal(dnd5e.RaceElf, getOutput.Draft.RaceID)
	s.Assert().True(getOutput.Draft.Progress.HasName())
	s.Assert().True(getOutput.Draft.Progress.HasRace())
}

// TestSingleDraftPerPlayer tests that creating a new draft replaces the old one
func (s *OrchestratorIntegrationTestSuite) TestSingleDraftPerPlayer() {
	playerID := "player-789"

	// Create first draft
	createOutput1, err := s.orchestrator.CreateDraft(s.ctx, &character.CreateDraftInput{
		PlayerID: playerID,
		InitialData: &dnd5e.CharacterDraft{
			Name: "First Character",
		},
	})
	s.Require().NoError(err)
	firstDraftID := createOutput1.Draft.ID

	// Create second draft for same player
	createOutput2, err := s.orchestrator.CreateDraft(s.ctx, &character.CreateDraftInput{
		PlayerID: playerID,
		InitialData: &dnd5e.CharacterDraft{
			Name: "Second Character",
		},
	})
	s.Require().NoError(err)
	secondDraftID := createOutput2.Draft.ID

	// First draft should no longer exist
	_, err = s.orchestrator.GetDraft(s.ctx, &character.GetDraftInput{
		DraftID: firstDraftID,
	})
	s.Assert().Error(err)

	// Second draft should exist
	getOutput, err := s.orchestrator.GetDraft(s.ctx, &character.GetDraftInput{
		DraftID: secondDraftID,
	})
	s.Require().NoError(err)
	s.Assert().Equal("Second Character", getOutput.Draft.Name)

	// List should only return one draft
	listOutput, err := s.orchestrator.ListDrafts(s.ctx, &character.ListDraftsInput{
		PlayerID: playerID,
	})
	s.Require().NoError(err)
	s.Assert().Len(listOutput.Drafts, 1)
	s.Assert().Equal(secondDraftID, listOutput.Drafts[0].ID)
}

// TestCharacterPersistence tests that finalized characters persist correctly
func (s *OrchestratorIntegrationTestSuite) TestCharacterPersistence() {
	// Create a test character directly in the repository
	char := testutils.CreateTestCharacter("player-999")
	createOutput, err := s.characterRepo.Create(s.ctx, characterrepo.CreateInput{
		Character: char,
	})
	s.Require().NoError(err)

	// Retrieve via orchestrator
	getOutput, err := s.orchestrator.GetCharacter(s.ctx, &character.GetCharacterInput{
		CharacterID: createOutput.Character.ID,
	})
	s.Require().NoError(err)
	s.Assert().Equal("Gandalf the Grey", getOutput.Character.Name)
	s.Assert().Equal(dnd5e.ClassWizard, getOutput.Character.ClassID)

	// List characters for player
	listOutput, err := s.orchestrator.ListCharacters(s.ctx, &character.ListCharactersInput{
		PlayerID: "player-999",
	})
	s.Require().NoError(err)
	s.Assert().Len(listOutput.Characters, 1)
	s.Assert().Equal(createOutput.Character.ID, listOutput.Characters[0].ID)

	// Delete character
	deleteOutput, err := s.orchestrator.DeleteCharacter(s.ctx, &character.DeleteCharacterInput{
		CharacterID: createOutput.Character.ID,
	})
	s.Require().NoError(err)
	s.Assert().Contains(deleteOutput.Message, "deleted successfully")

	// Verify deletion
	_, err = s.orchestrator.GetCharacter(s.ctx, &character.GetCharacterInput{
		CharacterID: createOutput.Character.ID,
	})
	s.Assert().Error(err)
}
