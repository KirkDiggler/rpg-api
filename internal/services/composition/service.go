// Package composition defines the world composition service contract.
package composition

import (
	"context"
	"encoding/json"

	worldcomposition "github.com/KirkDiggler/rpg-toolkit/world/composition"
)

//go:generate mockgen -destination=mock/mock_service.go -package=compositionmock github.com/KirkDiggler/rpg-api/internal/services/composition Service

// Service creates and reads immutable world composition snapshots.
type Service interface {
	Create(context.Context, *CreateInput) (*CreateOutput, error)
	Get(context.Context, *GetInput) (*GetOutput, error)
	List(context.Context, *ListInput) (*ListOutput, error)
}

// CreateInput contains one authenticated composition creation request.
type CreateInput struct {
	PlayerID string
	WorldID  string
	JSON     json.RawMessage
}

// CreateOutput contains the newly persisted composition.
type CreateOutput struct {
	Composition *worldcomposition.Data
}

// GetInput identifies one composition for an authenticated caller.
type GetInput struct {
	PlayerID      string
	WorldID       string
	CompositionID string
}

// GetOutput contains the requested composition.
type GetOutput struct {
	Composition *worldcomposition.Data
}

// ListInput selects one world's compositions for an authenticated caller.
type ListInput struct {
	PlayerID string
	WorldID  string
}

// ListOutput contains the world's compositions.
type ListOutput struct {
	Compositions []*worldcomposition.Data
}
