// Package entities defines the core data structures for the RPG API
package entities

import (
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

// CharacterDraft is the API storage wrapper around toolkit draft data.
type CharacterDraft struct {
	Data *toolkitchar.DraftData `json:"data"`
}
