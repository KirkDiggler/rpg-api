// Package character provides the interface for character persistence
package character

//go:generate mockgen -destination=mock/mock_repository.go -package=charactermock github.com/KirkDiggler/rpg-api/internal/repositories/character Repository

import (
	"context"

	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

// Repository defines the interface for character persistence
type Repository interface {
	// Create creates a new character
	// Returns errors.InvalidArgument for validation failures
	// Returns errors.AlreadyExists if character with same ID exists
	// Returns errors.Internal for storage failures
	Create(ctx context.Context, input CreateInput) (*CreateOutput, error)

	// Get retrieves a character by ID
	// Returns errors.InvalidArgument for empty/invalid IDs
	// Returns errors.NotFound if character doesn't exist
	// Returns errors.Internal for storage failures
	Get(ctx context.Context, input GetInput) (*GetOutput, error)

	// Update updates an existing character
	// Returns errors.InvalidArgument for validation failures
	// Returns errors.NotFound if character doesn't exist
	// Returns errors.Internal for storage failures
	Update(ctx context.Context, input UpdateInput) (*UpdateOutput, error)

	// Delete deletes a character by ID
	// Returns errors.InvalidArgument for empty/invalid IDs
	// Returns errors.NotFound if character doesn't exist
	// Returns errors.Internal for storage failures
	Delete(ctx context.Context, input DeleteInput) (*DeleteOutput, error)

	// ListByPlayerID retrieves all characters for a player
	// Returns errors.InvalidArgument for empty/invalid player IDs
	// Returns errors.Internal for storage failures
	ListByPlayerID(ctx context.Context, input ListByPlayerIDInput) (*ListByPlayerIDOutput, error)

	// ListBySessionID retrieves all characters in a session
	// Returns errors.InvalidArgument for empty/invalid session IDs
	// Returns errors.Internal for storage failures
	ListBySessionID(ctx context.Context, input ListBySessionIDInput) (*ListBySessionIDOutput, error)

	// GetEquipmentSlots retrieves equipment slot assignments for a character
	// Returns errors.InvalidArgument for empty/invalid character IDs
	// Returns errors.NotFound if character doesn't exist
	// Returns errors.Internal for storage failures
	GetEquipmentSlots(ctx context.Context, input GetEquipmentSlotsInput) (*GetEquipmentSlotsOutput, error)

	// SetEquipmentSlot sets an item to a specific equipment slot
	// Returns errors.InvalidArgument for validation failures
	// Returns errors.NotFound if character doesn't exist
	// Returns errors.Internal for storage failures
	SetEquipmentSlot(ctx context.Context, input SetEquipmentSlotInput) (*SetEquipmentSlotOutput, error)

	// ClearEquipmentSlot clears a specific equipment slot
	// Returns errors.InvalidArgument for empty/invalid parameters
	// Returns errors.Internal for storage failures
	ClearEquipmentSlot(ctx context.Context, input ClearEquipmentSlotInput) (*ClearEquipmentSlotOutput, error)
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

// EquipmentSlots represents the equipment slots for a character
type EquipmentSlots struct {
	MainHand    string `json:"main_hand,omitempty"`
	OffHand     string `json:"off_hand,omitempty"`
	Armor       string `json:"armor,omitempty"`
	Shield      string `json:"shield,omitempty"`
	Ring1       string `json:"ring1,omitempty"`
	Ring2       string `json:"ring2,omitempty"`
	Amulet      string `json:"amulet,omitempty"`
	Boots       string `json:"boots,omitempty"`
	Gloves      string `json:"gloves,omitempty"`
	Helmet      string `json:"helmet,omitempty"`
	Belt        string `json:"belt,omitempty"`
	Cloak       string `json:"cloak,omitempty"`
}

// GetEquipmentSlotsInput defines the input for getting equipment slots
type GetEquipmentSlotsInput struct {
	CharacterID string
}

// GetEquipmentSlotsOutput defines the output for getting equipment slots
type GetEquipmentSlotsOutput struct {
	EquipmentSlots *EquipmentSlots
}

// SetEquipmentSlotInput defines the input for setting an equipment slot
type SetEquipmentSlotInput struct {
	CharacterID string
	Slot        string
	ItemID      string
}

// SetEquipmentSlotOutput defines the output for setting an equipment slot
type SetEquipmentSlotOutput struct {
	PreviousItemID string // Item that was previously in the slot, if any
}

// ClearEquipmentSlotInput defines the input for clearing an equipment slot
type ClearEquipmentSlotInput struct {
	CharacterID string
	Slot        string
}

// ClearEquipmentSlotOutput defines the output for clearing an equipment slot
type ClearEquipmentSlotOutput struct {
	ClearedItemID string // Item that was cleared from the slot, if any
}
