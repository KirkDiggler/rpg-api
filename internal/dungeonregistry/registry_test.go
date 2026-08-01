package dungeonregistry_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/dungeonspec"

	"github.com/KirkDiggler/rpg-api/internal/dungeonregistry"
)

// TestNew_NilInitial_EmptyRegistry proves New(nil) is a valid, usable empty
// registry -- cmd/server/server.go should never need a special-case guard
// against a nil initial map.
func TestNew_NilInitial_EmptyRegistry(t *testing.T) {
	r := dungeonregistry.New(nil)
	_, ok := r.Get("anything")
	require.False(t, ok)
	require.Empty(t, r.Keys())
}

// TestGet_EmptyRegistry_ReturnsOkFalse is the base case the plan calls out
// explicitly: Get on an empty registry must report ok=false, not panic or
// return a zero Entry indistinguishable from a real one.
func TestGet_EmptyRegistry_ReturnsOkFalse(t *testing.T) {
	r := dungeonregistry.New(nil)
	entry, ok := r.Get("missing")
	require.False(t, ok)
	require.Equal(t, dungeonregistry.Entry{}, entry)
}

// TestPutThenGet_RoundTripsEntry proves Put followed by Get returns exactly
// what was stored -- the core Registry contract PutDungeon and StartEncounter
// both depend on.
func TestPutThenGet_RoundTripsEntry(t *testing.T) {
	r := dungeonregistry.New(nil)
	want := dungeonregistry.Entry{
		Compiled: dungeonspec.CompiledDungeon{Params: tkenc.DungeonParams{Theme: "crypt"}},
		Name:     "Test Dungeon",
	}
	r.Put("test-key", want)

	got, ok := r.Get("test-key")
	require.True(t, ok)
	require.Equal(t, want, got)
}

// TestPut_OverwritesExistingKey proves a second Put for the same key
// replaces the first -- the no-restart-visibility contract's building
// block: PutDungeon's registry swap must actually take effect, not
// silently no-op on an existing key.
func TestPut_OverwritesExistingKey(t *testing.T) {
	r := dungeonregistry.New(nil)
	r.Put("k", dungeonregistry.Entry{Name: "First"})
	r.Put("k", dungeonregistry.Entry{Name: "Second"})

	got, ok := r.Get("k")
	require.True(t, ok)
	require.Equal(t, "Second", got.Name)
}

// TestKeys_EmptyRegistry_ReturnsEmpty covers ListDungeons' base case: no
// entries, no error, no nil-slice panic risk for a caller ranging over it.
func TestKeys_EmptyRegistry_ReturnsEmpty(t *testing.T) {
	r := dungeonregistry.New(nil)
	require.Empty(t, r.Keys())
}

// TestKeys_SortedByKey pins the deterministic-ordering contract (matching
// content.OverriddenKeys' sort.Strings precedent) -- stable test
// assertions and any future log line depend on this, not just aesthetics.
func TestKeys_SortedByKey(t *testing.T) {
	r := dungeonregistry.New(map[string]dungeonregistry.Entry{
		"zebra":   {Name: "Zebra Dungeon"},
		"alpha":   {Name: "Alpha Dungeon"},
		"mid-key": {Name: "Mid Dungeon"},
	})

	got := r.Keys()
	require.Equal(t, []dungeonregistry.DungeonSummary{
		{Key: "alpha", Name: "Alpha Dungeon"},
		{Key: "mid-key", Name: "Mid Dungeon"},
		{Key: "zebra", Name: "Zebra Dungeon"},
	}, got)
}

// TestKeys_ExcludesLoadErrorEntries proves a disabled (load-failed) key
// never surfaces in the player-facing ListDungeons summary -- offering an
// unplayable dungeon in a dropdown is worse than a temporarily-shorter
// list (plan.md S1, ListDungeons checklist item).
func TestKeys_ExcludesLoadErrorEntries(t *testing.T) {
	r := dungeonregistry.New(map[string]dungeonregistry.Entry{
		"good":   {Name: "Good Dungeon"},
		"broken": {Err: errors.New("boom: at least 2 rooms required")},
	})

	got := r.Keys()
	require.Equal(t, []dungeonregistry.DungeonSummary{{Key: "good", Name: "Good Dungeon"}}, got)
}

// TestRegistry_ConcurrentPutAndGet_NoRace is the plan's explicit -race
// requirement: concurrent Put/Get must never race, proving the RWMutex
// (or equivalent) actually guards every access path, not just the ones
// this package's other tests happen to exercise sequentially.
func TestRegistry_ConcurrentPutAndGet_NoRace(t *testing.T) {
	r := dungeonregistry.New(nil)
	var wg sync.WaitGroup
	const n = 100
	wg.Add(n * 2)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			r.Put("key", dungeonregistry.Entry{Name: "concurrent"})
		}(i)
		go func(i int) {
			defer wg.Done()
			_, _ = r.Get("key")
		}(i)
	}
	wg.Wait()
}
