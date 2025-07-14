package rpgtoolkit

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
)

func TestCharacterEntity(t *testing.T) {
	character := &dnd5e.Character{
		ID:   "char-123",
		Name: "Test Character",
	}

	entity := wrapCharacter(character)

	assert.Equal(t, "char-123", entity.GetID())
	assert.Equal(t, "character", entity.GetType())
	assert.Equal(t, character, entity.Character)
}

func TestCharacterDraftEntity(t *testing.T) {
	draft := &dnd5e.CharacterDraft{
		ID:   "draft-456",
		Name: "Test Draft",
	}

	entity := wrapCharacterDraft(draft)

	assert.Equal(t, "draft-456", entity.GetID())
	assert.Equal(t, "character_draft", entity.GetType())
	assert.Equal(t, draft, entity.CharacterDraft)
}

func TestEntityWrappers(t *testing.T) {
	t.Run("CharacterEntity wrapping", func(t *testing.T) {
		character := &dnd5e.Character{
			ID:      "test-char",
			Name:    "Aragorn",
			Level:   5,
			RaceID:  "human",
			ClassID: "ranger",
		}

		wrapped := &CharacterEntity{Character: character}

		// Test that wrapper maintains access to original data
		assert.Equal(t, "test-char", wrapped.GetID())
		assert.Equal(t, "character", wrapped.GetType())
		assert.Equal(t, "Aragorn", wrapped.Name)
		assert.Equal(t, int32(5), wrapped.Level)
		assert.Equal(t, "human", wrapped.RaceID)
		assert.Equal(t, "ranger", wrapped.ClassID)
	})

	t.Run("CharacterDraftEntity wrapping", func(t *testing.T) {
		abilities := &dnd5e.AbilityScores{
			Strength:     15,
			Dexterity:    14,
			Constitution: 13,
			Intelligence: 12,
			Wisdom:       10,
			Charisma:     8,
		}

		draft := &dnd5e.CharacterDraft{
			ID:            "test-draft",
			Name:          "Legolas",
			RaceID:        "elf",
			ClassID:       "ranger",
			AbilityScores: abilities,
		}

		wrapped := &CharacterDraftEntity{CharacterDraft: draft}

		// Test that wrapper maintains access to original data
		assert.Equal(t, "test-draft", wrapped.GetID())
		assert.Equal(t, "character_draft", wrapped.GetType())
		assert.Equal(t, "Legolas", wrapped.Name)
		assert.Equal(t, "elf", wrapped.RaceID)
		assert.Equal(t, "ranger", wrapped.ClassID)
		assert.Equal(t, abilities, wrapped.AbilityScores)
	})
}
