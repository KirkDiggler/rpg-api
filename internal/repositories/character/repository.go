// Package character provides the interface for character persistence
package character

//go:generate mockgen -destination=mock/mock_repository.go -package=charactermock github.com/KirkDiggler/rpg-api/internal/repositories/character Repository

import (
	"context"
	"encoding/json"

	"github.com/KirkDiggler/rpg-api/internal/entities"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

// Repository defines the interface for character persistence.
//
// Every operation is world-scoped. WorldID is mandatory and never inferred
// from context inside this adapter: callers pass the trusted world they
// already resolved. Reads, deletes and patches carry WorldID explicitly; create
// and update take it from the stored wrapper's WorldID field. A record's world
// and player ownership are immutable once created.
type Repository interface {
	// Create creates a new character in the world named by Character.WorldID.
	// Returns apierr.InvalidArgument for validation failures (including an
	// empty or missing world).
	// Returns apierr.AlreadyExists if a character with the same ID exists in
	// the same world.
	// Returns apierr.Internal for storage failures.
	Create(ctx context.Context, input CreateInput) (*CreateOutput, error)

	// Get retrieves a character by world and ID.
	// Returns apierr.InvalidArgument for an empty world or ID.
	// Returns apierr.NotFound if the character doesn't exist in that world.
	// Returns apierr.Internal for storage failures or a stored record whose
	// ownership metadata contradicts the requested world.
	Get(ctx context.Context, input GetInput) (*GetOutput, error)

	// Update replaces an existing character. Player and world ownership are
	// immutable: an update that changes the stored player is rejected.
	// Returns apierr.InvalidArgument for validation failures or an ownership
	// change.
	// Returns apierr.NotFound if the character doesn't exist in that world.
	// Returns apierr.Internal for storage failures or corrupt ownership.
	Update(ctx context.Context, input UpdateInput) (*UpdateOutput, error)

	// PatchEquipment atomically changes only equipment slots and cached armor
	// class on the latest record. A stale equipment expectation is aborted;
	// an unrelated revision is returned without a write so the caller can
	// strictly reproject it before retrying. Ownership is never changed.
	PatchEquipment(ctx context.Context, input PatchEquipmentInput) (*PatchEquipmentOutput, error)

	// Delete deletes a character by world and ID.
	// Returns apierr.InvalidArgument for an empty world or ID.
	// Returns apierr.NotFound if the character doesn't exist in that world.
	// Returns apierr.Internal for storage failures or corrupt ownership.
	Delete(ctx context.Context, input DeleteInput) (*DeleteOutput, error)

	// ListByPlayerID retrieves all characters a player owns in a world.
	// Index entries whose record is missing are cleaned up; an entry whose
	// record belongs to a different world or player is reported as storage
	// corruption rather than projected.
	// Returns apierr.InvalidArgument for an empty world or player ID.
	// Returns apierr.Internal for storage failures.
	ListByPlayerID(ctx context.Context, input ListByPlayerIDInput) (*ListByPlayerIDOutput, error)

	// ListBySessionID retrieves all characters indexed for a session in a
	// world. Index entries whose record is missing are cleaned up; an entry
	// whose record belongs to a different world is reported as storage
	// corruption rather than projected.
	// Returns apierr.InvalidArgument for an empty world or session ID.
	// Returns apierr.Internal for storage failures.
	ListBySessionID(ctx context.Context, input ListBySessionIDInput) (*ListBySessionIDOutput, error)
}

// CreateInput defines the input for creating a character. The character's
// WorldID field is mandatory ownership metadata.
type CreateInput struct {
	Character *entities.Character
}

// CreateOutput defines the output for creating a character
type CreateOutput struct {
	Character *entities.Character
}

// GetInput defines the input for getting a character
type GetInput struct {
	WorldID string
	ID      string
}

// GetOutput defines the output for getting a character
type GetOutput struct {
	Character *entities.Character
	Version   string
}

// UpdateInput defines the input for updating a character. The character's
// WorldID and player ownership must match the stored record.
type UpdateInput struct {
	Character *entities.Character
}

// UpdateOutput defines the output for updating a character
type UpdateOutput struct {
	Character *entities.Character
}

// PatchEquipmentInput contains the optimistic revision/equipment expectation
// and equipment-derived fields the repository is permitted to change.
type PatchEquipmentInput struct {
	WorldID                string
	CharacterID            string
	ExpectedVersion        string
	ExpectedEquipmentSlots tkcharacter.EquipmentSlots
	EquipmentSlots         tkcharacter.EquipmentSlots
	ArmorClass             int
	// Conditions is the toolkit's post-equipment state. Nil preserves conditions;
	// a present empty slice clears them. The expected version protects concurrent
	// combat changes before this replacement is accepted.
	Conditions *[]json.RawMessage
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
	WorldID string
	ID      string
}

// DeleteOutput defines the output for deleting a character
type DeleteOutput struct {
	// Empty for now, can be extended later
}

// ListByPlayerIDInput defines the input for listing characters by player
type ListByPlayerIDInput struct {
	WorldID  string
	PlayerID string
}

// ListByPlayerIDOutput defines the output for listing characters by player
type ListByPlayerIDOutput struct {
	Characters []*entities.Character
}

// ListBySessionIDInput defines the input for listing characters by session
type ListBySessionIDInput struct {
	WorldID   string
	SessionID string
}

// ListBySessionIDOutput defines the output for listing characters by session
type ListBySessionIDOutput struct {
	Characters []*entities.Character
}
