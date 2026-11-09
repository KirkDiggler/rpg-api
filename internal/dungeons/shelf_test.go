package dungeons_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	"github.com/KirkDiggler/rpg-api/internal/repositories/worlddungeons"
)

// shelf is the content shelf this package's suites read: content/, the
// retired walk sites under testdata/sites, and the castle kitchen staged as a
// reference. TestMain compiles it ONCE per test binary, as the server
// compiles its content directory once at boot.
//
// Every suite here used to copy and compile the whole shelf per test, and
// one compile is about 5 s under -race: the package spent 199 s, most of it
// recompiling the same files (rpg-project#911). A FileRegistry is read-only
// after construction and the joined registry caches nothing. Get copies an
// entry's YAML bytes; the compiled dungeon and its atlas are shared, read
// only, and no test may mutate them. Each test still gets its own world
// store and projector. The directory stays on disk for the binary, so a test can still
// prove a save never writes it: against shelf.staged, the files as TestMain
// staged them, never against a snapshot of its own, because the directory
// outlives the test and an earlier save of the same bytes would hide a
// write. TestMain fails the binary when the directory changed at all.
//
// A test about construction itself (a broken file, a missing default, the
// seed) builds its own registry over its own directory.
var shelf struct {
	dir    string
	store  *dungeons.FileRegistry
	staged map[string][]byte
}

func TestMain(m *testing.M) {
	os.Exit(runWithShelf(m))
}

func runWithShelf(m *testing.M) int {
	dir, err := os.MkdirTemp("", "dungeons-shelf-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "dungeons shelf: %v\n", err)
		return 1
	}
	defer func() { _ = os.RemoveAll(dir) }()

	if stageErr := stageShelf(dir); stageErr != nil {
		fmt.Fprintf(os.Stderr, "dungeons shelf: %v\n", stageErr)
		return 1
	}
	store, closeFn, err := dungeonstest.CompileShelf(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dungeons shelf: %v\n", err)
		return 1
	}
	defer closeFn()
	staged, err := readShelf(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dungeons shelf: %v\n", err)
		return 1
	}
	shelf.dir, shelf.store, shelf.staged = dir, store, staged

	code := m.Run()
	after, err := readShelf(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dungeons shelf: %v\n", err)
		return 1
	}
	if changed := shelfChanges(staged, after); len(changed) > 0 {
		fmt.Fprintf(os.Stderr, "dungeons shelf: a test wrote the shared content directory (%s); the shipped store has no write path\n",
			strings.Join(changed, ", "))
		return 1
	}

	return code
}

// shelfChanges names every file added, removed or rewritten between two
// readings of the shelf, sorted.
func shelfChanges(before, after map[string][]byte) []string {
	var out []string
	for name, raw := range after {
		was, ok := before[name]
		switch {
		case !ok:
			out = append(out, name+" added")
		case !bytes.Equal(was, raw):
			out = append(out, name+" rewritten")
		}
	}
	for name := range before {
		if _, ok := after[name]; !ok {
			out = append(out, name+" removed")
		}
	}
	sort.Strings(out)

	return out
}

// readShelf is every file in dir by name, with its bytes.
func readShelf(dir string) (map[string][]byte, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]byte, len(entries))
	for _, e := range entries {
		raw, readErr := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // dir is the shelf, e a ReadDir entry
		if readErr != nil {
			return nil, readErr
		}
		out[e.Name()] = raw
	}

	return out, nil
}

// stageShelf copies the shelf's files into dir: every *.yaml directly under
// content/ and testdata/sites (dungeonstest.FixtureContentDir's set), and the
// castle kitchen.
func stageShelf(dir string) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	content, err := dungeons.FindContentDir(wd)
	if err != nil {
		return err
	}
	var files []string
	for _, src := range []string{content, filepath.Join("testdata", "sites")} {
		matches, globErr := filepath.Glob(filepath.Join(src, "*.yaml"))
		if globErr != nil {
			return globErr
		}
		if len(matches) == 0 {
			return fmt.Errorf("no dungeons under %s", src)
		}
		files = append(files, matches...)
	}
	files = append(files, filepath.Join("testdata", dungeonstest.CastleKitchenKey+".yaml"))
	for _, f := range files {
		raw, readErr := os.ReadFile(f) //nolint:gosec // fixed fixture paths
		if readErr != nil {
			return readErr
		}
		//nolint:gosec // dir is a temp dir this file made, f a fixture name
		if writeErr := os.WriteFile(filepath.Join(dir, filepath.Base(f)), raw, 0o600); writeErr != nil {
			return writeErr
		}
	}

	return nil
}

// joinShelf is the two-store Registry over the shared shelf and world: what
// a server booted on the shelf serves. A second call over the same world
// store is that server restarted.
func joinShelf(t testing.TB, world worlddungeons.Repository, authoring bool) dungeons.Registry {
	t.Helper()

	r, err := dungeons.NewRegistry(&dungeons.RegistryConfig{
		Shipped: shelf.store, World: world, Authoring: authoring, Projector: dungeonstest.Projector(t),
	})
	if err != nil {
		t.Fatalf("join shelf: %v", err)
	}

	return r
}
