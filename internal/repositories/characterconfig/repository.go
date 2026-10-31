// Package characterconfig is the character configuration store: what a player
// chooses for a character across every session (rpg-project#753 R2).
//
// It is keyed by character, never by session or player. Character verbs write
// it under the character's own guard; sessions only read it, at each write
// verb, so no session lock needs to reach wider than its own session. Today it
// holds one preference, discovery sharing; others join it with their own use
// cases (server-at-scale O4).
package characterconfig

//go:generate mockgen -destination=mock/mock_repository.go -package=characterconfigmock github.com/KirkDiggler/rpg-api/internal/repositories/characterconfig Repository

import (
	"context"
	"errors"
)

// ErrCharacterIDRequired is returned when an input names no character.
var ErrCharacterIDRequired = errors.New("character config: character id is required")

// ErrDataRequired is returned when Put is given no configuration to store.
var ErrDataRequired = errors.New("character config: data is required")

// Data is one character's configuration. A nil preference means the player
// has never chosen it; each reader applies its own default.
type Data struct {
	// DiscoverySharing is whether this character's automatic discovery
	// results are shared with the party. Explicit false is private.
	DiscoverySharing *bool `json:"discovery_sharing"`
}

// GetInput names the character to read.
type GetInput struct {
	CharacterID string
}

// GetOutput is the character's configuration. Data is never nil: a character
// with nothing stored has every preference unchosen.
type GetOutput struct {
	Data *Data
}

// PutInput replaces the character's configuration.
type PutInput struct {
	CharacterID string
	Data        *Data
}

// PutOutput is the configuration as stored.
type PutOutput struct {
	Data *Data
}

// Repository persists character configuration.
type Repository interface {
	// Get reads the configuration. A character with nothing stored reads as
	// Data with every preference unchosen, never as an error.
	Get(ctx context.Context, input *GetInput) (*GetOutput, error)

	// Put replaces the configuration. Callers hold the character's guard
	// across their read and Put, so a read-modify-write is not lost.
	Put(ctx context.Context, input *PutInput) (*PutOutput, error)
}
