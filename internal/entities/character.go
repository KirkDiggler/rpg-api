// Package entities defines the core data structures for the RPG API
package entities

import (
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

// Character represents a complete character in the rpg-api layer.
// It wraps the toolkit's character data and adds API-specific fields like appearance.
type Character struct {
	Data       *toolkitchar.Data `json:"data"`
	Appearance *Appearance       `json:"appearance,omitempty"`
}
