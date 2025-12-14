package auth

import "context"

type playerIDKey struct{}

// GetPlayerID retrieves the player ID from the context.
// Returns empty string if no player ID is set.
func GetPlayerID(ctx context.Context) string {
	if id, ok := ctx.Value(playerIDKey{}).(string); ok {
		return id
	}
	return ""
}

// WithPlayerID returns a new context with the player ID set.
func WithPlayerID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, playerIDKey{}, id)
}
