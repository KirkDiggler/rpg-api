// Package equipment provides the interface for equipment persistence
package equipment

//go:generate mockgen -destination=mock/mock_repository.go -package=equipmentmock github.com/KirkDiggler/rpg-api/internal/repositories/equipment Repository

import (
	"context"

	// NOTE: These types currently live in our entities package but will eventually
	// migrate to rpg-toolkit when it implements equipment rules. The repository
	// interface is designed to make this migration seamless - just update imports.
	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
)

// Repository defines the interface for equipment persistence
type Repository interface {
	// Get retrieves equipment data for a character
	// Returns errors.InvalidArgument for empty/invalid character IDs
	// Returns errors.NotFound if no equipment data exists
	// Returns errors.Internal for storage failures
	Get(ctx context.Context, input GetInput) (*GetOutput, error)

	// Update updates equipment data for a character
	// Creates equipment data if it doesn't exist
	// Returns errors.InvalidArgument for validation failures
	// Returns errors.Internal for storage failures
	Update(ctx context.Context, input UpdateInput) (*UpdateOutput, error)

	// Delete deletes all equipment data for a character
	// Returns errors.InvalidArgument for empty/invalid character IDs
	// Returns errors.NotFound if no equipment data exists
	// Returns errors.Internal for storage failures
	Delete(ctx context.Context, input DeleteInput) (*DeleteOutput, error)
}

// GetInput defines the input for getting equipment
type GetInput struct {
	CharacterID string
}

// GetOutput defines the output for getting equipment
type GetOutput struct {
	CharacterID    string
	EquipmentSlots *dnd5e.EquipmentSlots
	Inventory      []dnd5e.InventoryItem
	Encumbrance    *dnd5e.EncumbranceInfo
}

// UpdateInput defines the input for updating equipment
type UpdateInput struct {
	CharacterID    string
	EquipmentSlots *dnd5e.EquipmentSlots
	Inventory      []dnd5e.InventoryItem
	Encumbrance    *dnd5e.EncumbranceInfo
}

// UpdateOutput defines the output for updating equipment
type UpdateOutput struct {
	CharacterID    string
	EquipmentSlots *dnd5e.EquipmentSlots
	Inventory      []dnd5e.InventoryItem
	Encumbrance    *dnd5e.EncumbranceInfo
}

// DeleteInput defines the input for deleting equipment
type DeleteInput struct {
	CharacterID string
}

// DeleteOutput defines the output for deleting equipment
type DeleteOutput struct {
	// Empty for now, can be extended later
}
