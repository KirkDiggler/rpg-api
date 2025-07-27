package character

import (
	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
	"github.com/KirkDiggler/rpg-toolkit/items"
)

// Adapter adapts our Character entity to implement the rpg-toolkit Character interface
type Adapter struct {
	character      *dnd5e.Character
	proficiencies  []string
	equipmentItems map[string]items.Item // Maps slot names to rpg-toolkit items
	attunedItems   []items.Item
}

// NewAdapter creates a new character adapter
func NewAdapter(character *dnd5e.Character) *Adapter {
	return &Adapter{
		character:      character,
		proficiencies:  []string{}, // Will be populated from character data
		equipmentItems: make(map[string]items.Item),
		attunedItems:   []items.Item{},
	}
}

// GetStrength returns the character's strength score
func (a *Adapter) GetStrength() int {
	if a.character == nil || a.character.AbilityScores.Strength == 0 {
		return 10 // Default ability score
	}
	return int(a.character.AbilityScores.Strength)
}

// GetProficiencies returns the character's equipment proficiencies
func (a *Adapter) GetProficiencies() []string {
	return a.proficiencies
}

// SetProficiencies sets the character's equipment proficiencies
func (a *Adapter) SetProficiencies(proficiencies []string) {
	a.proficiencies = proficiencies
}

// GetEquippedItems returns a map of slot names to equipped items
func (a *Adapter) GetEquippedItems() map[string]items.Item {
	return a.equipmentItems
}

// SetEquippedItems sets the character's equipped items
func (a *Adapter) SetEquippedItems(items map[string]items.Item) {
	a.equipmentItems = items
}

// GetAttunedItems returns the character's attuned items
func (a *Adapter) GetAttunedItems() []items.Item {
	return a.attunedItems
}

// SetAttunedItems sets the character's attuned items
func (a *Adapter) SetAttunedItems(items []items.Item) {
	a.attunedItems = items
}

// GetAttunementLimit returns the character's attunement limit
func (a *Adapter) GetAttunementLimit() int {
	// D&D 5e default is 3
	return 3
}

// GetClass returns the character's class
func (a *Adapter) GetClass() string {
	if a.character == nil {
		return ""
	}
	return a.character.ClassID
}

// GetRace returns the character's race
func (a *Adapter) GetRace() string {
	if a.character == nil {
		return ""
	}
	return a.character.RaceID
}

// GetAlignment returns the character's alignment
func (a *Adapter) GetAlignment() string {
	if a.character == nil {
		return ""
	}
	return a.character.Alignment
}

// GetCharacter returns the underlying character entity
func (a *Adapter) GetCharacter() *dnd5e.Character {
	return a.character
}
