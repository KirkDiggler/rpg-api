// Package testutils provides utilities for testing, including Redis test helpers
package testutils

import (
	"context"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"

	"github.com/KirkDiggler/rpg-api/internal/redis"
)

// CreateTestRedisClient creates an in-memory Redis client for testing
func CreateTestRedisClient(t *testing.T) (redis.Client, func()) {
	mr, err := miniredis.Run()
	require.NoError(t, err, "failed to create miniredis")

	client, err := redis.NewClient(mr.Addr(), nil)
	require.NoError(t, err, "failed to create redis client")

	cleanup := func() {
		mr.Close()
	}

	return client, cleanup
}

// CreateTestRedisClientWithContext creates an in-memory Redis client with data population function
func CreateTestRedisClientWithContext(t *testing.T, setupFunc func(mr *miniredis.Miniredis)) (redis.Client, func()) {
	mr, err := miniredis.Run()
	require.NoError(t, err, "failed to create miniredis")

	// Allow test to populate Redis with initial data
	if setupFunc != nil {
		setupFunc(mr)
	}

	client, err := redis.NewClient(mr.Addr(), nil)
	require.NoError(t, err, "failed to create redis client")

	cleanup := func() {
		mr.Close()
	}

	return client, cleanup
}

// FlushTestRedis is currently unimplemented and should not be used.
// Tests should create fresh clients for each test until the flush logic is implemented.
func FlushTestRedis(ctx context.Context, client redis.Client) error {
	// TODO(#47): Implement flush when Redis client supports it
	return errors.New("FlushTestRedis is unimplemented: create fresh clients for each test instead")
}
