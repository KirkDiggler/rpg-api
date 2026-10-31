package dungeons_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/KirkDiggler/rpg-api/internal/dungeons"
)

// TestLoadReport_String pins the boot line's format after "content
// registry: ". The benchmark harness parses it as the "registry compile ms"
// row (rpg-project#740), so a reword must fail here first.
func TestLoadReport_String(t *testing.T) {
	r := dungeons.LoadReport{Files: 3, Elapsed: 12345678 * time.Nanosecond}
	require.Equal(t, "compiled 3 files in 12.35ms", r.String())
}
