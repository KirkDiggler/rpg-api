package builders

import (
	"time"

	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
)

// CharacterDraftDataBuilder provides a fluent interface for building test CharacterDraftData instances
type CharacterDraftDataBuilder struct {
	data *dnd5e.CharacterDraftData
}

// NewCharacterDraftDataBuilder creates a new builder with minimal defaults
func NewCharacterDraftDataBuilder() *CharacterDraftDataBuilder {
	now := time.Now().Unix()
	return &CharacterDraftDataBuilder{
		data: &dnd5e.CharacterDraftData{
			ID:        "draft-data-test-123",
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

// NewCharacterDraftDataBuilderFromDraft creates a builder from an existing CharacterDraft
func NewCharacterDraftDataBuilderFromDraft(draft *dnd5e.CharacterDraft) *CharacterDraftDataBuilder {
	return &CharacterDraftDataBuilder{
		data: dnd5e.FromCharacterDraft(draft),
	}
}

// WithID sets the draft ID
func (b *CharacterDraftDataBuilder) WithID(id string) *CharacterDraftDataBuilder {
	b.data.ID = id
	return b
}

// WithPlayerID sets the player ID
func (b *CharacterDraftDataBuilder) WithPlayerID(playerID string) *CharacterDraftDataBuilder {
	b.data.PlayerID = playerID
	return b
}

// WithName sets the character name
func (b *CharacterDraftDataBuilder) WithName(name string) *CharacterDraftDataBuilder {
	b.data.Name = name
	return b
}

// WithTimestamps sets created and updated timestamps
func (b *CharacterDraftDataBuilder) WithTimestamps(created, updated int64) *CharacterDraftDataBuilder {
	b.data.CreatedAt = created
	b.data.UpdatedAt = updated
	return b
}

// WithExpiration sets the expiration timestamp
func (b *CharacterDraftDataBuilder) WithExpiration(expiresAt int64) *CharacterDraftDataBuilder {
	b.data.ExpiresAt = expiresAt
	return b
}

// AsMinimal returns a builder with only required fields
func (b *CharacterDraftDataBuilder) AsMinimal() *CharacterDraftDataBuilder {
	return b.WithPlayerID("player-minimal")
}

// Build returns the constructed CharacterDraftData
func (b *CharacterDraftDataBuilder) Build() *dnd5e.CharacterDraftData {
	return b.data
}
