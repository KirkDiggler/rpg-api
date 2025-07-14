// Package characterdraft defines the interface for character draft persistence
package characterdraft

//go:generate mockgen -destination=mock/mock_repository.go -package=characterdraftmock github.com/KirkDiggler/rpg-api/internal/repositories/character_draft Repository

import (
	"context"

	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
)

// Repository defines the interface for character draft persistence
type Repository interface {
	// Create creates a new character draft
	// Returns errors.InvalidArgument for validation failures
	// Returns errors.AlreadyExists if draft with same ID exists
	// Returns errors.Internal for storage failures
	Create(ctx context.Context, draft *dnd5e.CharacterDraft) error

	// Get retrieves a character draft by ID
	// Returns errors.InvalidArgument for empty/invalid IDs
	// Returns errors.NotFound if draft doesn't exist
	// Returns errors.Internal for storage failures
	Get(ctx context.Context, id string) (*dnd5e.CharacterDraft, error)

	// Update updates an existing character draft
	// Returns errors.InvalidArgument for validation failures
	// Returns errors.NotFound if draft doesn't exist
	// Returns errors.Internal for storage failures
	Update(ctx context.Context, draft *dnd5e.CharacterDraft) error

	// Delete deletes a character draft by ID
	// Returns errors.InvalidArgument for empty/invalid IDs
	// Returns errors.NotFound if draft doesn't exist
	// Returns errors.Internal for storage failures
	Delete(ctx context.Context, id string) error

	// List lists character drafts with pagination
	// Returns errors.InvalidArgument for invalid pagination options
	// Returns errors.Internal for storage failures
	List(ctx context.Context, opts ListOptions) (*ListResult, error)

	// GetByPlayerID retrieves all drafts for a player
	// Returns errors.InvalidArgument for empty/invalid player IDs
	// Returns errors.Internal for storage failures
	GetByPlayerID(ctx context.Context, playerID string) ([]*dnd5e.CharacterDraft, error)

	// GetBySessionID retrieves all drafts in a session
	// Returns errors.InvalidArgument for empty/invalid session IDs
	// Returns errors.Internal for storage failures
	GetBySessionID(ctx context.Context, sessionID string) ([]*dnd5e.CharacterDraft, error)

	// DeleteExpired deletes all drafts past their expiration time
	// Returns errors.Internal for storage failures
	// Returns count of deleted drafts
	DeleteExpired(ctx context.Context) (int64, error)
}

// ListOptions defines options for listing character drafts
type ListOptions struct {
	PageSize  int32
	PageToken string
	PlayerID  string // Optional filter
	SessionID string // Optional filter
}

// ListResult contains the results of a list operation
type ListResult struct {
	Drafts        []*dnd5e.CharacterDraft
	NextPageToken string
	TotalSize     int32
}
