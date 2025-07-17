package testutils

import (
	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
)

// Draft progress stages for testing
const (
	StageNameComplete   = "name_complete"
	StageRaceComplete   = "race_complete"
	StageClassComplete  = "class_complete"
	StageNearlyComplete = "nearly_complete"

	// TestCharacterName is the default character name for test fixtures
	TestCharacterName = "Thorin Oakenshield"
)

// CreateTestCharacterDraft creates a test character draft with sensible defaults
func CreateTestCharacterDraft(playerID string) *dnd5e.CharacterDraft {
	return &dnd5e.CharacterDraft{
		ID:        "draft-test-001",
		PlayerID:  playerID,
		SessionID: "session-test-001",
		Name:      "Test Character",
		RaceID:    dnd5e.RaceHuman,
		ClassID:   dnd5e.ClassFighter,
		Progress: dnd5e.CreationProgress{
			StepsCompleted:       0,
			CompletionPercentage: 0,
			CurrentStep:          dnd5e.CreationStepName,
		},
	}
}

// CreateTestCharacterDraftWithProgress creates a test draft at various stages of completion
func CreateTestCharacterDraftWithProgress(playerID string, stage string) *dnd5e.CharacterDraft {
	draft := CreateTestCharacterDraft(playerID)

	switch stage {
	case StageNameComplete:
		draft.Name = TestCharacterName
		draft.Progress.SetStep(dnd5e.ProgressStepName, true)
		draft.Progress.CompletionPercentage = 14 // 1/7 steps

	case StageRaceComplete:
		draft.Name = TestCharacterName
		draft.RaceID = dnd5e.RaceDwarf
		draft.SubraceID = dnd5e.SubraceMountainDwarf
		draft.Progress.SetStep(dnd5e.ProgressStepName, true)
		draft.Progress.SetStep(dnd5e.ProgressStepRace, true)
		draft.Progress.CompletionPercentage = 28 // 2/7 steps

	case StageClassComplete:
		draft.Name = TestCharacterName
		draft.RaceID = dnd5e.RaceDwarf
		draft.SubraceID = dnd5e.SubraceMountainDwarf
		draft.ClassID = dnd5e.ClassFighter
		draft.Progress.SetStep(dnd5e.ProgressStepName, true)
		draft.Progress.SetStep(dnd5e.ProgressStepRace, true)
		draft.Progress.SetStep(dnd5e.ProgressStepClass, true)
		draft.Progress.CompletionPercentage = 42 // 3/7 steps

	case StageNearlyComplete:
		draft.Name = TestCharacterName
		draft.RaceID = dnd5e.RaceDwarf
		draft.SubraceID = dnd5e.SubraceMountainDwarf
		draft.ClassID = dnd5e.ClassFighter
		draft.BackgroundID = dnd5e.BackgroundSoldier
		draft.AbilityScores = &dnd5e.AbilityScores{
			Strength:     15,
			Dexterity:    13,
			Constitution: 14,
			Intelligence: 8,
			Wisdom:       12,
			Charisma:     10,
		}
		draft.StartingSkillIDs = []string{dnd5e.SkillAthletics, dnd5e.SkillIntimidation}
		draft.Progress.SetStep(dnd5e.ProgressStepName, true)
		draft.Progress.SetStep(dnd5e.ProgressStepRace, true)
		draft.Progress.SetStep(dnd5e.ProgressStepClass, true)
		draft.Progress.SetStep(dnd5e.ProgressStepBackground, true)
		draft.Progress.SetStep(dnd5e.ProgressStepAbilityScores, true)
		draft.Progress.SetStep(dnd5e.ProgressStepSkills, true)
		draft.Progress.CompletionPercentage = 85 // 6/7 steps
	}

	return draft
}

// CreateTestCharacter creates a fully formed test character
func CreateTestCharacter(playerID string) *dnd5e.Character {
	return &dnd5e.Character{
		ID:               "char-test-001",
		Name:             "Gandalf the Grey",
		Level:            1,
		ExperiencePoints: 0,
		RaceID:           dnd5e.RaceHuman,
		ClassID:          dnd5e.ClassWizard,
		BackgroundID:     dnd5e.BackgroundSage,
		Alignment:        dnd5e.AlignmentNeutralGood,
		AbilityScores: dnd5e.AbilityScores{
			Strength:     8,
			Dexterity:    14,
			Constitution: 13,
			Intelligence: 16,
			Wisdom:       15,
			Charisma:     12,
		},
		CurrentHP: 7, // 6 (wizard d6) + 1 (CON mod)
		TempHP:    0,
		PlayerID:  playerID,
		SessionID: "session-test-001",
	}
}

// CreateTestAbilityScores creates standard array ability scores
func CreateTestAbilityScores() *dnd5e.AbilityScores {
	return &dnd5e.AbilityScores{
		Strength:     15,
		Dexterity:    14,
		Constitution: 13,
		Intelligence: 12,
		Wisdom:       10,
		Charisma:     8,
	}
}
