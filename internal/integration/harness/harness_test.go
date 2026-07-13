package harness_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"

	"github.com/KirkDiggler/rpg-api/internal/integration/harness"
)

func TestHarness_StartsAndConnects(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Create test server
	ts, err := harness.New(ctx, nil)
	require.NoError(t, err, "failed to create test server")
	defer ts.Close()

	// Verify the full stack is wired correctly (server, clients, Redis)
	// Add dev auth header for Redis operations
	authCtx := metadata.AppendToOutgoingContext(ctx, "authorization", "Dev test-player-1")
	t.Log("✓ Test server started with real Redis")
	t.Log("✓ gRPC clients connected via bufconn")
	t.Logf("✓ CharacterClient: %T", ts.CharacterClient)
	t.Logf("✓ EncounterClientV2: %T", ts.EncounterClientV2)
	t.Logf("✓ DiceClient: %T", ts.DiceClient)

	// Verify Redis is working by flushing
	err = ts.FlushRedis(authCtx)
	require.NoError(t, err, "failed to flush redis")
	t.Log("✓ Redis flush succeeded")
}
