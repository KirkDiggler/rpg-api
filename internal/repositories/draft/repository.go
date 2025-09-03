package draft

import (
	"context"
	
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

//go:generate mockgen -destination=mock/mock_repository.go -package=draftmock github.com/KirkDiggler/rpg-api/internal/repositories/draft Repository

// Repository defines the interface for draft storage
// It uses DraftData for serialization but returns Draft domain objects
type Repository interface {
	// Save creates or updates a draft (converts to DraftData internally)
	Save(ctx context.Context, draft *character.Draft) error
	
	// Get retrieves a draft by ID (loads from DraftData internally)
	Get(ctx context.Context, id string) (*character.Draft, error)
	
	// Delete removes a draft
	Delete(ctx context.Context, id string) error
	
	// List retrieves drafts for a player
	List(ctx context.Context, input *ListInput) (*ListOutput, error)
}

// ListInput defines parameters for listing drafts
type ListInput struct {
	PlayerID string
	Limit    int
	Offset   int
}

// ListOutput contains the list results
type ListOutput struct {
	Drafts []*character.Draft
	Total  int
}