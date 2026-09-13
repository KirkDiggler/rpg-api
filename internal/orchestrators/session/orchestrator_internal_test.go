package session

import (
	"context"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultPresentationIDsUseOneSeparator(t *testing.T) {
	id := newDefaultPresentationIDs().Generate()

	require.Regexp(t,
		regexp.MustCompile(`^presentation_[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`),
		id,
	)
	require.NotContains(t, id, "presentation-_",
		"idgen.UUIDGenerator already inserts the separator after its prefix")
}

// The cache hands each session its own driver, and hands the same session the
// same one every time.
//
// BOTH HALVES ARE THE CLAIM. Distinct across sessions is the point of the
// seam: a driver is stateful, member ids are authored per dungeon rather than
// minted per run, and one driver for every session gave two parties in the
// same tomb one skeleton's assigned mind and names (rpg-api#980's caveat,
// rpg-toolkit#1734). Stable within a session is what makes it a driver rather
// than a fresh brain per verb -- a cache that rebuilt on every ask would pass
// the first half and forget everything a session ever learned.
func TestTurnDriverCacheHandsOneDriverPerSession(t *testing.T) {
	cache := newTurnDriverCache()
	ctx := context.Background()

	first, err := cache.DriverFor(ctx, "sess-a")
	require.NoError(t, err)
	require.NotNil(t, first)

	second, err := cache.DriverFor(ctx, "sess-b")
	require.NoError(t, err)
	require.NotNil(t, second)

	// INTERFACE IDENTITY, compared with == rather than require.Same or
	// require.Equal. The SDK hands back a small value type rather than a
	// pointer, so Same cannot take it; and two freshly built drivers are
	// structurally identical -- empty maps either way -- so a deep comparison
	// would call them equal and prove the opposite of what is meant here.
	require.False(t, first == second, "two sessions, two drivers")

	again, err := cache.DriverFor(ctx, "sess-a")
	require.NoError(t, err)
	require.True(t, first == again, "and one session keeps the driver it was given")
}

// Nothing is built until a session asks, and every session that asks is kept.
//
// The entries are not evicted, which is a decision rather than an oversight
// (see turnDriverCache's own doc). This pins the shape that decision is about:
// what the map holds is exactly the sessions that have been asked about, so
// whoever adds eviction later can see what they are evicting from.
func TestTurnDriverCacheBuildsOnFirstSightAndKeepsWhatItBuilt(t *testing.T) {
	cache := newTurnDriverCache()
	require.Empty(t, cache.drivers, "an empty cache builds nothing in advance")

	ctx := context.Background()
	for _, id := range []string{"sess-a", "sess-b", "sess-a"} {
		_, err := cache.DriverFor(ctx, id)
		require.NoError(t, err)
	}

	require.Len(t, cache.drivers, 2, "two sessions were asked about, twice over for one of them")
}
