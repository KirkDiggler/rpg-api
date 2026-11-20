package dungeons_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	"github.com/KirkDiggler/rpg-api/internal/repositories/worlddungeons"
)

// The holding words (rpg-project#1005) renamed content keys: a post's
// `pass: { recognized: … }` is `pass: { noted: … }` now, and the engine
// refuses the old word with a sentence naming the new one. These tests pin
// what THIS api does with a dungeon still written in the old words, wherever
// it sits: the decision is the engine's refusal, and the api's part is where
// that refusal lands. Nothing here changes that behavior; it records it.

// oldWordsRefusal is the engine's sentence an operator or author reads.
const oldWordsRefusal = "`recognized` is now `noted`"

// oldWordsCastle is the shipped castle under key with one post's pass
// written in the old word: last week's file, as far as the compiler cares.
func oldWordsCastle(t *testing.T, key string) []byte {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(dungeonstest.ContentDir(t), dungeons.DefaultKey+".yaml")) //nolint:gosec // the shipped content dir
	require.NoError(t, err)
	if key != dungeons.DefaultKey {
		raw = dungeonstest.AsKey(t, raw, dungeons.DefaultKey, key)
	}
	old := bytes.Replace(raw, []byte("pass: { noted: admitted }"), []byte("pass: { recognized: admitted }"), 1)
	require.NotEqual(t, raw, old, "the castle no longer holds a `pass: { noted: admitted }` to write back in the old word")

	return old
}

// TestOldWords_MountedDefaultStopsTheBoot: a mounted content directory whose
// the-mad-king.yaml was seeded by an earlier image keeps it — SeedShipped
// never overwrites — and NewFileRegistry then refuses the whole boot, naming
// the file, the line and the new word.
func TestOldWords_MountedDefaultStopsTheBoot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, dungeons.DefaultKey+".yaml")
	require.NoError(t, os.WriteFile(path, oldWordsCastle(t, dungeons.DefaultKey), 0o600))

	seeded, err := dungeons.SeedShipped(dir, dungeonstest.ContentDir(t))
	require.NoError(t, err)
	require.NotContains(t, seeded, dungeons.DefaultKey, "the operator's own default is left as it is")

	_, err = dungeons.NewFileRegistry(dir, dungeonstest.Projector(t))
	require.Error(t, err, "an old-words default does not load")
	require.Contains(t, err.Error(), path+" does not compile")
	require.Contains(t, err.Error(), oldWordsRefusal)
}

// TestOldWords_MountedSiblingStopsTheBoot: any old-words file in the mounted
// directory stops the boot the same way, beside a current default. It is not
// skipped and not served.
func TestOldWords_MountedSiblingStopsTheBoot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "my-castle.yaml")
	require.NoError(t, os.WriteFile(path, oldWordsCastle(t, "my-castle"), 0o600))
	_, err := dungeons.SeedShipped(dir, dungeonstest.ContentDir(t))
	require.NoError(t, err)

	_, err = dungeons.NewFileRegistry(dir, dungeonstest.Projector(t))
	require.Error(t, err, "one old-words file refuses the whole shelf")
	require.Contains(t, err.Error(), path+" does not compile")
	require.Contains(t, err.Error(), oldWordsRefusal)
}

// TestOldWords_SavedWorldDungeonListsOpensButDoesNotPlay: an author's
// dungeon saved in the world store before the rename is still listed (List
// never compiles), opens as saved (Source never compiles), and is refused by
// the compiling read every play goes through, with the engine's sentence.
func TestOldWords_SavedWorldDungeonListsOpensButDoesNotPlay(t *testing.T) {
	ctx := context.Background()
	world := dungeonstest.WorldStore(t)
	_, err := world.Put(ctx, &worlddungeons.PutInput{
		WorldID: "w", Owner: "alice", Key: "my-castle", Name: "My Castle", YAML: oldWordsCastle(t, "my-castle"),
	})
	require.NoError(t, err)
	// The package's shared shelf (shelf_test.go), compiled once per binary:
	// compiling the shipped content again here was most of this test's time.
	r := joinShelf(t, world, false)
	caller := dungeons.Caller{WorldID: "w", PlayerID: "alice"}

	list, err := r.List(ctx, caller)
	require.NoError(t, err)
	keys := make([]string, 0, len(list))
	for _, s := range list {
		keys = append(keys, s.Key)
	}
	require.Contains(t, keys, "my-castle", "listed: the list reads the stored name, it does not compile")

	opened, err := r.Source(ctx, &dungeons.SourceInput{Caller: caller, Key: "my-castle"})
	require.NoError(t, err, "opened: the author's read does not compile")
	require.Equal(t, oldWordsCastle(t, "my-castle"), opened.YAML, "byte for byte as saved")

	_, err = r.Get(ctx, caller, "my-castle")
	require.Error(t, err)
	require.Contains(t, err.Error(), `dungeon "my-castle" in world "w" no longer compiles`)
	require.Contains(t, err.Error(), oldWordsRefusal)
}
