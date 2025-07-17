// Package dicesession provides repository interface and types for dice roll sessions
package dicesession

import (
	"context"
	"time"
)

//go:generate mockgen -destination=mock/mock_repository.go -package=dicesessionmock github.com/KirkDiggler/rpg-api/internal/repositories/dice_session Repository

// DiceSession represents a collection of dice rolls grouped by entity and context
type DiceSession struct {
	// Entity that owns these rolls (e.g., "char_draft_123", "char_789")
	EntityID string

	// Context for grouping related rolls (e.g., "ability_scores", "combat_round_1")
	Context string

	// The actual dice rolls in this session
	Rolls []DiceRoll

	// When this session was created
	CreatedAt time.Time

	// When this session expires
	ExpiresAt time.Time
}

// DiceRoll represents a single dice roll result
type DiceRoll struct {
	// Unique identifier for this roll within the session
	RollID string

	// Dice notation that was rolled (e.g., "4d6", "1d20+5")
	Notation string

	// Individual dice values that were rolled
	Dice []int32

	// Final result after applying modifiers
	Total int32

	// Any dice that were dropped (for "drop lowest" etc.)
	Dropped []int32

	// Human-readable description of the roll
	Description string

	// Raw dice total before modifiers
	DiceTotal int32

	// Modifier applied to get final total
	Modifier int32
}

// CreateInput contains parameters for creating a dice session
type CreateInput struct {
	EntityID string
	Context  string
	Rolls    []DiceRoll
	TTL      time.Duration // How long the session should live
}

// CreateOutput contains the result of creating a dice session
type CreateOutput struct {
	Session *DiceSession
}

// GetInput contains parameters for retrieving a dice session
type GetInput struct {
	EntityID string
	Context  string
}

// GetOutput contains the result of retrieving a dice session
type GetOutput struct {
	Session *DiceSession
}

// DeleteInput contains parameters for deleting a dice session
type DeleteInput struct {
	EntityID string
	Context  string
}

// DeleteOutput contains the result of deleting a dice session
type DeleteOutput struct {
	RollsDeleted int32
}

// Repository defines the interface for dice session storage operations
type Repository interface {
	// Create stores a new dice session with the specified TTL
	Create(ctx context.Context, input CreateInput) (*CreateOutput, error)

	// Get retrieves a dice session by entity ID and context
	Get(ctx context.Context, input GetInput) (*GetOutput, error)

	// Delete removes a dice session
	Delete(ctx context.Context, input DeleteInput) (*DeleteOutput, error)

	// Update replaces an existing dice session (used for adding rolls)
	Update(ctx context.Context, session *DiceSession) error
}
