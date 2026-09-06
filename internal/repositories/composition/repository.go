// Package composition persists toolkit compositions by world.
package composition

import (
	"context"

	worldcomposition "github.com/KirkDiggler/rpg-toolkit/world/composition"
)

//go:generate mockgen -destination=mock/mock_repository.go -package=compositionmock github.com/KirkDiggler/rpg-api/internal/repositories/composition Repository

// Repository stores compositions without interpreting their JSON payloads.
type Repository interface {
	Create(context.Context, *CreateInput) (*CreateOutput, error)
	Get(context.Context, *GetInput) (*GetOutput, error)
	List(context.Context, *ListInput) (*ListOutput, error)
}

// CreateInput contains the composition to create.
type CreateInput struct {
	Composition *worldcomposition.Data
}

// CreateOutput contains the persisted composition snapshot.
type CreateOutput struct {
	Composition *worldcomposition.Data
}

// GetInput addresses one composition within a world.
type GetInput struct {
	WorldID string
	ID      string
}

// GetOutput contains the requested composition.
type GetOutput struct {
	Composition *worldcomposition.Data
}

// ListInput scopes compositions to one world.
type ListInput struct {
	WorldID string
}

// ListOutput contains compositions sorted by ID.
type ListOutput struct {
	Compositions []*worldcomposition.Data
}
