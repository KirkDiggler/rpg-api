// Package entities defines the core data structures for the RPG API
package entities

import (
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

// Character is the API storage wrapper around toolkit character data.
//
// WorldID is API-owned ownership metadata, not part of the toolkit payload:
// it records which world (Discord guild or configured dev world) owns this
// record. The toolkit character.Data is unchanged and never carries it.
type Character struct {
	WorldID string            `json:"world_id"`
	Data    *toolkitchar.Data `json:"data"`
}
