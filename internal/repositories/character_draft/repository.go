// Package characterdraft defines the interface for character draft persistence
package characterdraft

//go:generate mockgen -destination=mock/mock_repository.go -package=characterdraftmock github.com/KirkDiggler/rpg-api/internal/repositories/character_draft Repository

import (
	"context"

	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
)

// Repository defines the interface for character draft persistence
// Implements a single-draft-per-player pattern for simplicity
type Repository interface {
	// Create creates or replaces a player's character draft
	// Returns errors.InvalidArgument for validation failures
	// Returns errors.Internal for storage failures
	Create(ctx context.Context, input CreateInput) (*CreateOutput, error)

	// Get retrieves a character draft by ID
	// Returns errors.InvalidArgument for empty/invalid IDs
	// Returns errors.NotFound if draft doesn't exist
	// Returns errors.Internal for storage failures
	Get(ctx context.Context, input GetInput) (*GetOutput, error)

	// GetByPlayerID retrieves the player's single draft
	// Returns errors.InvalidArgument for empty/invalid player IDs
	// Returns errors.NotFound if player has no draft
	// Returns errors.Internal for storage failures
	GetByPlayerID(ctx context.Context, input GetByPlayerIDInput) (*GetByPlayerIDOutput, error)

	// Update updates an existing character draft
	// Returns errors.InvalidArgument for validation failures
	// Returns errors.NotFound if draft doesn't exist
	// Returns errors.Internal for storage failures
	Update(ctx context.Context, input UpdateInput) (*UpdateOutput, error)

	// Delete deletes a character draft by ID
	// Returns errors.InvalidArgument for empty/invalid IDs
	// Returns errors.NotFound if draft doesn't exist
	// Returns errors.Internal for storage failures
	Delete(ctx context.Context, input DeleteInput) (*DeleteOutput, error)
}

// CreateInput defines the input for creating a character draft
type CreateInput struct {
	Draft *dnd5e.CharacterDraft
}

// CreateOutput defines the output for creating a character draft
type CreateOutput struct {
	Draft *dnd5e.CharacterDraft
}

// GetInput defines the input for getting a character draft
type GetInput struct {
	ID string
}

// GetOutput defines the output for getting a character draft
type GetOutput struct {
	Draft *dnd5e.CharacterDraft
}

// GetByPlayerIDInput defines the input for getting a player's draft
type GetByPlayerIDInput struct {
	PlayerID string
}

// GetByPlayerIDOutput defines the output for getting a player's draft
type GetByPlayerIDOutput struct {
	Draft *dnd5e.CharacterDraft
}

// UpdateInput defines the input for updating a character draft
type UpdateInput struct {
	Draft *dnd5e.CharacterDraft
}

// UpdateOutput defines the output for updating a character draft
type UpdateOutput struct {
	Draft *dnd5e.CharacterDraft
}

// DeleteInput defines the input for deleting a character draft
type DeleteInput struct {
	ID string
}

// DeleteOutput defines the output for deleting a character draft
type DeleteOutput struct {
	// Empty for now, can be extended later
}
