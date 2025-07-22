// Package builders provides test data builders for creating test fixtures
package builders

import (
	"time"

	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
)

// CharacterDraftBuilder provides a fluent interface for building test CharacterDraft instances
type CharacterDraftBuilder struct {
	draft *dnd5e.CharacterDraft
}

// NewCharacterDraftBuilder creates a new builder with minimal defaults
func NewCharacterDraftBuilder() *CharacterDraftBuilder {
	now := time.Now().Unix()
	return &CharacterDraftBuilder{
		draft: &dnd5e.CharacterDraft{
			ID:        "draft-test-123",
			PlayerID:  "player-test-123",
			CreatedAt: now,
			UpdatedAt: now,
			Progress: dnd5e.CreationProgress{
				StepsCompleted:       0,
				CompletionPercentage: 0,
				CurrentStep:          dnd5e.CreationStepName,
			},
		},
	}
}

// WithID sets the draft ID
func (b *CharacterDraftBuilder) WithID(id string) *CharacterDraftBuilder {
	b.draft.ID = id
	return b
}

// WithPlayerID sets the player ID
func (b *CharacterDraftBuilder) WithPlayerID(playerID string) *CharacterDraftBuilder {
	b.draft.PlayerID = playerID
	return b
}

// WithSessionID sets the session ID
func (b *CharacterDraftBuilder) WithSessionID(sessionID string) *CharacterDraftBuilder {
	b.draft.SessionID = sessionID
	return b
}

// WithName sets the character name and marks the name step as complete
func (b *CharacterDraftBuilder) WithName(name string) *CharacterDraftBuilder {
	b.draft.Name = name
	if name != "" {
		b.draft.Progress.SetStep(dnd5e.ProgressStepName, true)
		b.updateProgress()
	}
	return b
}

// WithRace sets the race and optionally subrace, marking the race step as complete
func (b *CharacterDraftBuilder) WithRace(raceID string, subraceID ...string) *CharacterDraftBuilder {
	b.draft.RaceID = raceID
	if len(subraceID) > 0 {
		b.draft.SubraceID = subraceID[0]
	}
	if raceID != "" {
		b.draft.Progress.SetStep(dnd5e.ProgressStepRace, true)
		b.updateProgress()
	}
	return b
}

// WithClass sets the class and marks the class step as complete
func (b *CharacterDraftBuilder) WithClass(classID string) *CharacterDraftBuilder {
	b.draft.ClassID = classID
	if classID != "" {
		b.draft.Progress.SetStep(dnd5e.ProgressStepClass, true)
		b.updateProgress()
	}
	return b
}

// WithBackground sets the background and marks the background step as complete
func (b *CharacterDraftBuilder) WithBackground(backgroundID string) *CharacterDraftBuilder {
	b.draft.BackgroundID = backgroundID
	if backgroundID != "" {
		b.draft.Progress.SetStep(dnd5e.ProgressStepBackground, true)
		b.updateProgress()
	}
	return b
}

// WithAbilityScores sets the ability scores and marks the ability scores step as complete
func (b *CharacterDraftBuilder) WithAbilityScores(str, dex, con, intel, wis, cha int32) *CharacterDraftBuilder {
	b.draft.AbilityScores = &dnd5e.AbilityScores{
		Strength:     str,
		Dexterity:    dex,
		Constitution: con,
		Intelligence: intel,
		Wisdom:       wis,
		Charisma:     cha,
	}
	b.draft.Progress.SetStep(dnd5e.ProgressStepAbilityScores, true)
	b.updateProgress()
	return b
}

// WithChoiceSelections sets choice selections
func (b *CharacterDraftBuilder) WithChoiceSelections(selections []dnd5e.ChoiceSelection) *CharacterDraftBuilder {
	b.draft.ChoiceSelections = selections
	return b
}

// WithAlignment sets the alignment
func (b *CharacterDraftBuilder) WithAlignment(alignment string) *CharacterDraftBuilder {
	b.draft.Alignment = alignment
	return b
}

// WithProgress sets specific progress values
func (b *CharacterDraftBuilder) WithProgress(
	stepsCompleted uint8, percentage int32, currentStep string,
) *CharacterDraftBuilder {
	b.draft.Progress = dnd5e.CreationProgress{
		StepsCompleted:       stepsCompleted,
		CompletionPercentage: percentage,
		CurrentStep:          currentStep,
	}
	return b
}

// WithHydratedRace adds hydrated race info (for testing hydration)
func (b *CharacterDraftBuilder) WithHydratedRace(race *dnd5e.RaceInfo) *CharacterDraftBuilder {
	b.draft.Race = race
	return b
}

// WithHydratedClass adds hydrated class info (for testing hydration)
func (b *CharacterDraftBuilder) WithHydratedClass(class *dnd5e.ClassInfo) *CharacterDraftBuilder {
	b.draft.Class = class
	return b
}

// WithHydratedBackground adds hydrated background info (for testing hydration)
func (b *CharacterDraftBuilder) WithHydratedBackground(background *dnd5e.BackgroundInfo) *CharacterDraftBuilder {
	b.draft.Background = background
	return b
}

// AsComplete builds a draft with all required fields populated
func (b *CharacterDraftBuilder) AsComplete() *CharacterDraftBuilder {
	return b.
		WithName("Test Character").
		WithRace(dnd5e.RaceHuman).
		WithClass(dnd5e.ClassFighter).
		WithBackground(dnd5e.BackgroundSoldier).
		WithAbilityScores(15, 14, 13, 12, 10, 8).
		WithAlignment(dnd5e.AlignmentLawfulGood)
}

// Build returns the constructed CharacterDraft
func (b *CharacterDraftBuilder) Build() *dnd5e.CharacterDraft {
	return b.draft
}

// BuildData returns the draft converted to CharacterDraftData for repository operations
func (b *CharacterDraftBuilder) BuildData() *dnd5e.CharacterDraftData {
	return dnd5e.FromCharacterDraft(b.draft)
}

// updateProgress recalculates the completion percentage based on completed steps
func (b *CharacterDraftBuilder) updateProgress() {
	// Simple calculation - each step is worth a percentage
	// Total of 8 possible steps (name, race, class, background, abilities, skills, languages, choices)
	completedCount := 0
	for i := uint8(0); i < 8; i++ {
		if b.draft.Progress.StepsCompleted&(1<<i) != 0 {
			completedCount++
		}
	}
	// #nosec G115 - completedCount is bounded by 8, so no overflow is possible
	b.draft.Progress.CompletionPercentage = int32(completedCount * 100 / 8)

	// Update current step based on what's not completed
	switch {
	case !b.draft.Progress.HasName():
		b.draft.Progress.CurrentStep = dnd5e.CreationStepName
	case !b.draft.Progress.HasRace():
		b.draft.Progress.CurrentStep = dnd5e.CreationStepRace
	case !b.draft.Progress.HasClass():
		b.draft.Progress.CurrentStep = dnd5e.CreationStepClass
	case !b.draft.Progress.HasBackground():
		b.draft.Progress.CurrentStep = dnd5e.CreationStepBackground
	case !b.draft.Progress.HasAbilityScores():
		b.draft.Progress.CurrentStep = dnd5e.CreationStepAbilityScores
	case !b.draft.Progress.HasSkills():
		b.draft.Progress.CurrentStep = dnd5e.CreationStepSkills
	default:
		b.draft.Progress.CurrentStep = dnd5e.CreationStepReview
	}
}
