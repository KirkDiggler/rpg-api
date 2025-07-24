package builders

import (
	"time"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

// ToolkitDraftDataBuilder provides a fluent interface for building test character.DraftData instances
type ToolkitDraftDataBuilder struct {
	data *character.DraftData
}

// NewToolkitDraftDataBuilder creates a new builder with minimal defaults
func NewToolkitDraftDataBuilder() *ToolkitDraftDataBuilder {
	now := time.Now()
	return &ToolkitDraftDataBuilder{
		data: &character.DraftData{
			ID:            "draft-test-123",
			PlayerID:      "player-test-123",
			Choices:       make(map[shared.ChoiceCategory]any),
			ProgressFlags: 0,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
	}
}

// WithID sets the draft ID
func (b *ToolkitDraftDataBuilder) WithID(id string) *ToolkitDraftDataBuilder {
	b.data.ID = id
	return b
}

// WithPlayerID sets the player ID
func (b *ToolkitDraftDataBuilder) WithPlayerID(playerID string) *ToolkitDraftDataBuilder {
	b.data.PlayerID = playerID
	return b
}

// WithName sets the character name
func (b *ToolkitDraftDataBuilder) WithName(name string) *ToolkitDraftDataBuilder {
	b.data.Name = name
	b.data.Choices[shared.ChoiceName] = name
	b.data.ProgressFlags |= character.ProgressName
	return b
}

// WithRace sets the race choice
func (b *ToolkitDraftDataBuilder) WithRace(raceID, subraceID string) *ToolkitDraftDataBuilder {
	b.data.Choices[shared.ChoiceRace] = character.RaceChoice{
		RaceID:    raceID,
		SubraceID: subraceID,
	}
	b.data.ProgressFlags |= character.ProgressRace
	return b
}

// WithClass sets the class choice
func (b *ToolkitDraftDataBuilder) WithClass(classID string) *ToolkitDraftDataBuilder {
	b.data.Choices[shared.ChoiceClass] = classID
	b.data.ProgressFlags |= character.ProgressClass
	return b
}

// WithBackground sets the background choice
func (b *ToolkitDraftDataBuilder) WithBackground(backgroundID string) *ToolkitDraftDataBuilder {
	b.data.Choices[shared.ChoiceBackground] = backgroundID
	b.data.ProgressFlags |= character.ProgressBackground
	return b
}

// WithAbilityScores sets the ability scores
func (b *ToolkitDraftDataBuilder) WithAbilityScores(scores shared.AbilityScores) *ToolkitDraftDataBuilder {
	b.data.Choices[shared.ChoiceAbilityScores] = scores
	b.data.ProgressFlags |= character.ProgressAbilityScores
	return b
}

// WithSkills sets the skill choices
func (b *ToolkitDraftDataBuilder) WithSkills(skills []string) *ToolkitDraftDataBuilder {
	b.data.Choices[shared.ChoiceSkills] = skills
	b.data.ProgressFlags |= character.ProgressSkills
	return b
}

// WithTimestamps sets created and updated timestamps
func (b *ToolkitDraftDataBuilder) WithTimestamps(created, updated time.Time) *ToolkitDraftDataBuilder {
	b.data.CreatedAt = created
	b.data.UpdatedAt = updated
	return b
}

// AsComplete creates a complete draft ready for finalization
func (b *ToolkitDraftDataBuilder) AsComplete() *ToolkitDraftDataBuilder {
	return b.
		WithName("Test Character").
		WithRace("human", "").
		WithClass("fighter").
		WithBackground("soldier").
		WithAbilityScores(shared.AbilityScores{
			Strength:     15,
			Dexterity:    14,
			Constitution: 13,
			Intelligence: 12,
			Wisdom:       10,
			Charisma:     8,
		}).
		WithSkills([]string{"athletics", "intimidation"})
}

// Build returns the constructed DraftData
func (b *ToolkitDraftDataBuilder) Build() *character.DraftData {
	return b.data
}