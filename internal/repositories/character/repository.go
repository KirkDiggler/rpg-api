// Package character provides the interface for character persistence
package character

//go:generate mockgen -destination=mock/mock_repository.go -package=charactermock github.com/KirkDiggler/rpg-api/internal/repositories/character Repository

import (
	"context"

	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
)

// Repository defines the interface for character persistence
type Repository interface {
	// Create creates a new character
	// Returns errors.InvalidArgument for validation failures
	// Returns errors.AlreadyExists if character with same ID exists
	// Returns errors.Internal for storage failures
	Create(ctx context.Context, character *dnd5e.Character) error

	// Get retrieves a character by ID
	// Returns errors.InvalidArgument for empty/invalid IDs
	// Returns errors.NotFound if character doesn't exist
	// Returns errors.Internal for storage failures
	Get(ctx context.Context, id string) (*dnd5e.Character, error)

	// Update updates an existing character
	// Returns errors.InvalidArgument for validation failures
	// Returns errors.NotFound if character doesn't exist
	// Returns errors.Internal for storage failures
	Update(ctx context.Context, character *dnd5e.Character) error

	// Delete deletes a character by ID
	// Returns errors.InvalidArgument for empty/invalid IDs
	// Returns errors.NotFound if character doesn't exist
	// Returns errors.Internal for storage failures
	Delete(ctx context.Context, id string) error

	// List lists characters with pagination
	// Returns errors.InvalidArgument for invalid pagination options
	// Returns errors.Internal for storage failures
	List(ctx context.Context, opts ListOptions) (*ListResult, error)

	// GetByPlayerID retrieves all characters for a player
	// Returns errors.InvalidArgument for empty/invalid player IDs
	// Returns errors.Internal for storage failures
	GetByPlayerID(ctx context.Context, playerID string) ([]*dnd5e.Character, error)

	// GetBySessionID retrieves all characters in a session
	// Returns errors.InvalidArgument for empty/invalid session IDs
	// Returns errors.Internal for storage failures
	GetBySessionID(ctx context.Context, sessionID string) ([]*dnd5e.Character, error)
}

// ListOptions defines options for listing characters
type ListOptions struct {
	PageSize  int32
	PageToken string
	PlayerID  string // Optional filter
	SessionID string // Optional filter
}

// ListResult contains the results of a list operation
type ListResult struct {
	Characters    []*dnd5e.Character
	NextPageToken string
	TotalSize     int32
}
