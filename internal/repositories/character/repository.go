// Package character provides the interface for character persistence
package character

//go:generate mockgen -destination=mock/mock_repository.go -package=charactermock github.com/KirkDiggler/rpg-api/internal/repositories/character Repository

import (
	"context"

	"github.com/KirkDiggler/rpg-api/internal/entities"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
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

	// PatchEquipment atomically changes only equipment slots and cached armor
	// class on the latest record. A stale equipment expectation is aborted;
	// an unrelated revision is returned without a write so the caller can
	// strictly reproject it before retrying.
	PatchEquipment(ctx context.Context, input PatchEquipmentInput) (*PatchEquipmentOutput, error)

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
}

// CreateInput defines the input for creating a character
type CreateInput struct {
	Character *entities.Character
}

// CreateOutput defines the output for creating a character
type CreateOutput struct {
	Character *entities.Character
}

// GetInput defines the input for getting a character
type GetInput struct {
	ID string
}

// GetOutput defines the output for getting a character
type GetOutput struct {
	Character *entities.Character
	Version   string
}

// UpdateInput defines the input for updating a character
type UpdateInput struct {
	Character *entities.Character
}

// UpdateOutput defines the output for updating a character
type UpdateOutput struct {
	Character *entities.Character
}

// PatchEquipmentInput contains the optimistic revision/equipment expectation
// and the only two fields the repository is permitted to change.
type PatchEquipmentInput struct {
	CharacterID            string
	ExpectedVersion        string
	ExpectedEquipmentSlots tkcharacter.EquipmentSlots
	EquipmentSlots         tkcharacter.EquipmentSlots
	ArmorClass             int
}

// PatchEquipmentOutput contains the actual latest persisted entity. Applied is
// false only when a non-equipment revision requires caller reprojection.
type PatchEquipmentOutput struct {
	Character *entities.Character
	Version   string
	Applied   bool
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
	Characters []*entities.Character
}

// ListBySessionIDInput defines the input for listing characters by session
type ListBySessionIDInput struct {
	SessionID string
}

// ListBySessionIDOutput defines the output for listing characters by session
type ListBySessionIDOutput struct {
	Characters []*entities.Character
}
