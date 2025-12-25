// Package character provides the interface for character persistence
package character

//go:generate mockgen -destination=mock/mock_repository.go -package=charactermock github.com/KirkDiggler/rpg-api/internal/repositories/character Repository

import (
	"context"

	"github.com/KirkDiggler/rpg-api/internal/entities"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

// Repository defines the interface for character persistence
type Repository interface {
	// Create creates a new character
	// Returns apierr.InvalidArgument for validation failures
	// Returns apierr.AlreadyExists if character with same ID exists
	// Returns apierr.Internal for storage failures
	Create(ctx context.Context, input CreateInput) (*CreateOutput, error)

	// Get retrieves a character by ID
	// Returns apierr.InvalidArgument for empty/invalid IDs
	// Returns apierr.NotFound if character doesn't exist
	// Returns apierr.Internal for storage failures
	Get(ctx context.Context, input GetInput) (*GetOutput, error)

	// Update updates an existing character
	// Returns apierr.InvalidArgument for validation failures
	// Returns apierr.NotFound if character doesn't exist
	// Returns apierr.Internal for storage failures
	Update(ctx context.Context, input UpdateInput) (*UpdateOutput, error)

	// Delete deletes a character by ID
	// Returns apierr.InvalidArgument for empty/invalid IDs
	// Returns apierr.NotFound if character doesn't exist
	// Returns apierr.Internal for storage failures
	Delete(ctx context.Context, input DeleteInput) (*DeleteOutput, error)

	// ListByPlayerID retrieves all characters for a player
	// Returns apierr.InvalidArgument for empty/invalid player IDs
	// Returns apierr.Internal for storage failures
	ListByPlayerID(ctx context.Context, input ListByPlayerIDInput) (*ListByPlayerIDOutput, error)

	// ListBySessionID retrieves all characters in a session
	// Returns apierr.InvalidArgument for empty/invalid session IDs
	// Returns apierr.Internal for storage failures
	ListBySessionID(ctx context.Context, input ListBySessionIDInput) (*ListBySessionIDOutput, error)

	// SetAppearance sets or updates the appearance for a character
	// Returns apierr.InvalidArgument for empty character ID
	// Returns apierr.Internal for storage failures
	SetAppearance(ctx context.Context, input SetAppearanceInput) (*SetAppearanceOutput, error)

	// GetAppearance retrieves the appearance for a character
	// Returns apierr.InvalidArgument for empty character ID
	// Returns nil Appearance (not error) if not set
	// Returns apierr.Internal for storage failures
	GetAppearance(ctx context.Context, input GetAppearanceInput) (*GetAppearanceOutput, error)
}

// CreateInput defines the input for creating a character
type CreateInput struct {
	CharacterData *toolkitchar.Data
}

// CreateOutput defines the output for creating a character
type CreateOutput struct {
	CharacterData *toolkitchar.Data
}

// GetInput defines the input for getting a character
type GetInput struct {
	ID string
}

// GetOutput defines the output for getting a character
type GetOutput struct {
	CharacterData *toolkitchar.Data
}

// UpdateInput defines the input for updating a character
type UpdateInput struct {
	CharacterData *toolkitchar.Data
}

// UpdateOutput defines the output for updating a character
type UpdateOutput struct {
	CharacterData *toolkitchar.Data
}

// DeleteInput defines the input for deleting a character
type DeleteInput struct {
	ID string
}

// DeleteOutput defines the output for deleting a character
type DeleteOutput struct {
	// Empty for now, can be extended later
}

// ListByPlayerIDInput defines the input for listing characters by player
type ListByPlayerIDInput struct {
	PlayerID string
}

// ListByPlayerIDOutput defines the output for listing characters by player
type ListByPlayerIDOutput struct {
	Characters []*toolkitchar.Data
}

// ListBySessionIDInput defines the input for listing characters by session
type ListBySessionIDInput struct {
	SessionID string
}

// ListBySessionIDOutput defines the output for listing characters by session
type ListBySessionIDOutput struct {
	Characters []*toolkitchar.Data
}

// SetAppearanceInput defines the input for setting a character's appearance
type SetAppearanceInput struct {
	CharacterID string
	Appearance  *entities.Appearance
}

// SetAppearanceOutput defines the output for setting a character's appearance
type SetAppearanceOutput struct {
	Appearance *entities.Appearance
}

// GetAppearanceInput defines the input for getting a character's appearance
type GetAppearanceInput struct {
	CharacterID string
}

// GetAppearanceOutput defines the output for getting a character's appearance
type GetAppearanceOutput struct {
	Appearance *entities.Appearance // nil if not set
}
