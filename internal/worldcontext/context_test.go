package worldcontext_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/KirkDiggler/rpg-api/internal/worldcontext"
)

func TestWorldContext(t *testing.T) {
	ctx := context.Background()
	_, ok := worldcontext.Get(ctx)
	require.False(t, ok)

	ctx = worldcontext.With(ctx, worldcontext.Value{WorldID: "123456789012345678"})
	value, ok := worldcontext.Get(ctx)
	require.True(t, ok)
	require.Equal(t, "123456789012345678", value.WorldID)
}
