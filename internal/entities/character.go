// Package entities defines the core data structures for the RPG API
package entities

import (
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

// Character is the API storage wrapper around toolkit character data.
type Character struct {
	Data *toolkitchar.Data `json:"data"`
}
