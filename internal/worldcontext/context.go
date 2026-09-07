// Package worldcontext carries an API-verified world identity to world-scoped handlers.
package worldcontext

import "context"

type contextKey struct{}

// Value contains the trusted domain identity for the current request.
type Value struct {
	WorldID string
}

// With returns a context carrying the trusted world value.
func With(ctx context.Context, value Value) context.Context {
	return context.WithValue(ctx, contextKey{}, value)
}

// Get returns the trusted world value, when one has been installed.
func Get(ctx context.Context) (Value, bool) {
	value, ok := ctx.Value(contextKey{}).(Value)
	return value, ok
}
