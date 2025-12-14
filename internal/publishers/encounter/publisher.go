// Package encounter provides the interface for encounter event publishing
package encounter

//go:generate mockgen -destination=mock/mock_publisher.go -package=publishermock github.com/KirkDiggler/rpg-api/internal/publishers/encounter Publisher

import (
	"context"

	"github.com/KirkDiggler/rpg-api/internal/entities"
)

// Publisher defines the interface for publishing encounter events to subscribers
// Purpose: Enable real-time multiplayer synchronization via pub/sub pattern
type Publisher interface {
	// Publish publishes an event to all subscribers of the encounter
	// Returns apierr.InvalidArgument for nil events or empty encounter IDs
	// Returns apierr.Internal for publishing failures
	Publish(ctx context.Context, input *PublishInput) (*PublishOutput, error)

	// Subscribe subscribes to events for a specific encounter
	// Returns a channel that will receive events and an error channel
	// The event channel will be closed when the context is canceled or Unsubscribe is called
	// Returns apierr.InvalidArgument for empty encounter IDs
	// Returns apierr.Internal for subscription failures
	Subscribe(ctx context.Context, input *SubscribeInput) (*SubscribeOutput, error)

	// Unsubscribe unsubscribes from encounter events
	// Closes the event channel associated with the subscription ID
	// Returns apierr.InvalidArgument for empty subscription IDs
	// Returns apierr.NotFound if subscription doesn't exist
	Unsubscribe(ctx context.Context, input *UnsubscribeInput) (*UnsubscribeOutput, error)
}

// PublishInput defines the input for publishing an event
type PublishInput struct {
	EncounterID string                   `json:"encounter_id"`
	Event       *entities.EncounterEvent `json:"event"`
}

// PublishOutput defines the output for publishing an event
type PublishOutput struct {
	// Empty for now, can be extended with confirmation details
}

// SubscribeInput defines the input for subscribing to encounter events
type SubscribeInput struct {
	EncounterID string `json:"encounter_id"`
}

// SubscribeOutput defines the output for subscribing to encounter events
type SubscribeOutput struct {
	SubscriptionID string                          `json:"subscription_id"` // Unique ID for this subscription
	Events         <-chan *entities.EncounterEvent `json:"-"`               // Channel for receiving events
	Errors         <-chan error                    `json:"-"`               // Channel for subscription errors
}

// UnsubscribeInput defines the input for unsubscribing from encounter events
type UnsubscribeInput struct {
	SubscriptionID string `json:"subscription_id"`
}

// UnsubscribeOutput defines the output for unsubscribing from encounter events
type UnsubscribeOutput struct {
	// Empty for now, can be extended with cleanup details
}
