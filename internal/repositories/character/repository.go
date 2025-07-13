package character

//go:generate mockgen -destination=mock/mock_repository.go -package=charactermock github.com/KirkDiggler/rpg-api/internal/repositories/character Repository

import (
	"context"

	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
)

// Repository defines the interface for character persistence
type Repository interface {
	// Create creates a new character
	Create(ctx context.Context, character *dnd5e.Character) error

	// Get retrieves a character by ID
	Get(ctx context.Context, id string) (*dnd5e.Character, error)

	// Update updates an existing character
	Update(ctx context.Context, character *dnd5e.Character) error

	// Delete deletes a character by ID
	Delete(ctx context.Context, id string) error

	// List lists characters with pagination
	List(ctx context.Context, opts ListOptions) (*ListResult, error)

	// GetByPlayerID retrieves all characters for a player
	GetByPlayerID(ctx context.Context, playerID string) ([]*dnd5e.Character, error)

	// GetBySessionID retrieves all characters in a session
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
