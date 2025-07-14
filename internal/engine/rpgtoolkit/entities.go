package rpgtoolkit

import "github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"

// CharacterEntity wraps dnd5e.Character to implement core.Entity interface
type CharacterEntity struct {
	*dnd5e.Character
}

// GetID returns the character's ID
func (c *CharacterEntity) GetID() string {
	return c.ID
}

// GetType returns the entity type for rpg-toolkit
func (c *CharacterEntity) GetType() string {
	return "character"
}

// CharacterDraftEntity wraps dnd5e.CharacterDraft to implement core.Entity interface
type CharacterDraftEntity struct {
	*dnd5e.CharacterDraft
}

// GetID returns the character draft's ID
func (c *CharacterDraftEntity) GetID() string {
	return c.ID
}

// GetType returns the entity type for rpg-toolkit
func (c *CharacterDraftEntity) GetType() string {
	return "character_draft"
}

// wrapCharacter converts a dnd5e.Character to a CharacterEntity
func wrapCharacter(character *dnd5e.Character) *CharacterEntity {
	return &CharacterEntity{Character: character}
}

// wrapCharacterDraft converts a dnd5e.CharacterDraft to a CharacterDraftEntity
func wrapCharacterDraft(draft *dnd5e.CharacterDraft) *CharacterDraftEntity {
	return &CharacterDraftEntity{CharacterDraft: draft}
}
