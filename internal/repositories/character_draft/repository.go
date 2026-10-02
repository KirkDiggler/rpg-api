// Package characterdraft defines the interface for character draft persistence
package characterdraft

//go:generate mockgen -destination=mock/mock_repository.go -package=characterdraftmock github.com/KirkDiggler/rpg-api/internal/repositories/character_draft Repository

import (
	"context"

	"github.com/KirkDiggler/rpg-api/internal/entities"
)

// Repository defines the interface for character draft persistence.
// It implements a world-scoped, single-draft-per-player pattern: each
// (world, player) pair points to one draft at a time.
//
// WorldID is mandatory and never inferred from context inside this adapter.
// A draft's world and player ownership are immutable once created.
type Repository interface {
	// Create creates or replaces a (world, player)'s character draft. The
	// draft's WorldID and Data.PlayerID are mandatory ownership metadata.
	// Returns apierr.InvalidArgument for validation failures (including an
	// empty or missing world).
	// Returns apierr.Internal for storage failures.
	Create(ctx context.Context, input CreateInput) (*CreateOutput, error)

	// Get retrieves a character draft by world and ID.
	// Returns apierr.InvalidArgument for an empty world or ID.
	// Returns apierr.NotFound if the draft doesn't exist in that world.
	// Returns apierr.Internal for storage failures or corrupt ownership.
	Get(ctx context.Context, input GetInput) (*GetOutput, error)

	// GetByPlayerID retrieves the player's single draft in a world.
	// Returns apierr.InvalidArgument for an empty world or player ID.
	// Returns apierr.NotFound if the player has no draft in that world.
	// Returns apierr.Internal for storage failures or corrupt ownership.
	GetByPlayerID(ctx context.Context, input GetByPlayerIDInput) (*GetByPlayerIDOutput, error)

	// Update replaces an existing character draft. World and player ownership
	// are immutable: an update that changes the stored player is rejected.
	// Returns apierr.InvalidArgument for validation failures or an ownership
	// change.
	// Returns apierr.NotFound if the draft doesn't exist in that world.
	// Returns apierr.Internal for storage failures or corrupt ownership.
	Update(ctx context.Context, input UpdateInput) (*UpdateOutput, error)

	// Delete deletes a character draft and its world-scoped player mapping.
	// Returns apierr.InvalidArgument for an empty world or ID.
	// Returns apierr.NotFound if the draft doesn't exist in that world.
	// Returns apierr.Internal for storage failures or corrupt ownership.
	Delete(ctx context.Context, input DeleteInput) (*DeleteOutput, error)
}

// CreateInput defines the input for creating a character draft. The draft's
// WorldID and Data.PlayerID are mandatory.
type CreateInput struct {
	Draft *entities.CharacterDraft
}

// CreateOutput defines the output for creating a character draft
type CreateOutput struct {
	Draft *entities.CharacterDraft
}

// GetInput defines the input for getting a character draft
type GetInput struct {
	WorldID string
	ID      string
}

// GetOutput defines the output for getting a character draft
type GetOutput struct {
	Draft *entities.CharacterDraft
}

// GetByPlayerIDInput defines the input for getting a player's draft
type GetByPlayerIDInput struct {
	WorldID  string
	PlayerID string
}

// GetByPlayerIDOutput defines the output for getting a player's draft
type GetByPlayerIDOutput struct {
	Draft *entities.CharacterDraft
}

// UpdateInput defines the input for updating a character draft. The draft's
// WorldID and Data.PlayerID must match the stored record.
type UpdateInput struct {
	Draft *entities.CharacterDraft
}

// UpdateOutput defines the output for updating a character draft
type UpdateOutput struct {
	Draft *entities.CharacterDraft
}

// DeleteInput defines the input for deleting a character draft
type DeleteInput struct {
	WorldID string
	ID      string
}

// DeleteOutput defines the output for deleting a character draft
type DeleteOutput struct {
	// Empty for now, can be extended later
}
