// Package encounters is the v2 encounter store.
// It holds *encounter.Data values from the rpg-toolkit encounter SDK.
package encounters

import (
	"context"
	"errors"

	"github.com/KirkDiggler/rpg-toolkit/encounter"
)

// ErrNotFound indicates the encounter id does not exist in the store.
var ErrNotFound = errors.New("encounter not found")

// Repository persists *encounter.Data keyed by encounter id.
//
// Persistent implementations MUST round-trip through encounter.Data's
// ToData/LoadFromData pattern on each Save/Get so the toolkit's serialization
// is exercised on every write — catching JSON-level bugs before they reach production.
type Repository interface {
	// Get returns ErrNotFound if id has no stored data.
	Get(ctx context.Context, id string) (*encounter.Data, error)

	// Save replaces the stored data for data.ID.
	Save(ctx context.Context, data *encounter.Data) error
}
