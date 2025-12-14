package auth_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/KirkDiggler/rpg-api/internal/auth"
)

func TestGetPlayerID_ReturnsEmptyWhenNotSet(t *testing.T) {
	ctx := context.Background()

	playerID := auth.GetPlayerID(ctx)

	assert.Empty(t, playerID)
}

func TestGetPlayerID_ReturnsIDWhenSet(t *testing.T) {
	ctx := context.Background()
	ctx = auth.WithPlayerID(ctx, "discord-user-123")

	playerID := auth.GetPlayerID(ctx)

	assert.Equal(t, "discord-user-123", playerID)
}

func TestWithPlayerID_PreservesExistingContext(t *testing.T) {
	type testKey struct{}
	ctx := context.WithValue(context.Background(), testKey{}, "existing-value")
	ctx = auth.WithPlayerID(ctx, "player-456")

	// Both values should be accessible
	assert.Equal(t, "player-456", auth.GetPlayerID(ctx))
	assert.Equal(t, "existing-value", ctx.Value(testKey{}))
}
