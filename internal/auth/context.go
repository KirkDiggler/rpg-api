package auth

import "context"

type (
	playerIDKey    struct{}
	requestAuthKey struct{}
)

type requestAuth struct {
	scheme string
	value  string
}

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

func withRequestAuth(ctx context.Context, value *requestAuth) context.Context {
	return context.WithValue(ctx, requestAuthKey{}, value)
}

func getRequestAuth(ctx context.Context) (*requestAuth, bool) {
	value, ok := ctx.Value(requestAuthKey{}).(*requestAuth)
	return value, ok && value != nil
}

func withoutRequestAuth(ctx context.Context) context.Context {
	return context.WithValue(ctx, requestAuthKey{}, (*requestAuth)(nil))
}
