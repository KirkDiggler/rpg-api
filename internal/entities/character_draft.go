// Package entities defines the core data structures for the RPG API
package entities

import (
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

// CharacterDraft is the API storage wrapper around toolkit draft data.
//
// WorldID is API-owned ownership metadata, not part of the toolkit payload:
// it records which world (Discord guild or configured dev world) owns this
// draft. The toolkit character.DraftData is unchanged and never carries it.
type CharacterDraft struct {
	WorldID string                 `json:"world_id"`
	Data    *toolkitchar.DraftData `json:"data"`
}
