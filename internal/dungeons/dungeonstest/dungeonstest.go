// Package dungeonstest builds content registries for tests from the shipped
// content tree plus the retired walk sites under
// internal/dungeons/testdata/sites, so every suite that starts an encounter
// runs on the same castle the server boots with, and a suite pinned to a
// retired site still finds it without the site shipping (rpg-api#1205).
package dungeonstest

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"

	tkdungeonspec "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter/dungeonspec"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/repositories/worlddungeons"
)

// Caller is the one caller test registries are asked as: one player in one
// world. A test about who may see or write what builds its own callers.
var Caller = dungeons.Caller{WorldID: "test-world", PlayerID: "test-author"}

// Projector is a dungeons.AtlasProjector over a real, miniredis-backed
// session Manager — the same Manager.AtlasOf production wires, so a test
// registry's atlases are the ones a session would serve. Prefer
// ProjectorFor when the test already has a Manager, so the registry and the
// sessions it starts share one.
func Projector(t testing.TB) dungeons.AtlasProjector {
	t.Helper()

	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	chars, err := characterrepo.NewRedis(&characterrepo.RedisConfig{Client: client})
	if err != nil {
		t.Fatalf("dungeonstest: character repo: %v", err)
	}
	orch, err := sessionorch.New(sessionorch.Config{
		Redis: client, Characters: chars, TTL: time.Hour,
		PresentationIDs: idgen.NewSequential("presentation"),
	})
	if err != nil {
		t.Fatalf("dungeonstest: session orchestrator: %v", err)
	}

	return ProjectorFor(orch.Manager)
}

// ProjectorFor adapts a session Manager to dungeons.AtlasProjector.
func ProjectorFor(m *sdk.Manager) dungeons.AtlasProjector { return managerProjector{m} }

type managerProjector struct{ m *sdk.Manager }

func (p managerProjector) AtlasOf(
	ctx context.Context, key string, dungeon *tkdungeonspec.Compiled,
) (*sdk.Atlas, error) {
	return p.m.AtlasOf(ctx, &sdk.AtlasOfInput{Dungeon: dungeon, DungeonKey: key})
}

// ContentDir locates the repo's content/ directory by walking up from the
// working directory (go test runs each package in its own directory).
func ContentDir(t testing.TB) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("dungeonstest: getwd: %v", err)
	}
	dir, findErr := dungeons.FindContentDir(wd)
	if findErr != nil {
		t.Fatal(findErr)
	}

	return dir
}

// SitesDir is internal/dungeons/testdata/sites: the walk sites a finished
// slice verified, kept for the suites that still play them. The image ships
// only content/, so nothing here reaches a server (rpg-api#1205).
func SitesDir(t testing.TB) string {
	t.Helper()

	return filepath.Join(filepath.Dir(ContentDir(t)), "internal", "dungeons", "testdata", "sites")
}

// SitePath is one retired walk site's file under SitesDir, by key.
func SitePath(t testing.TB, key string) string {
	t.Helper()

	return filepath.Join(SitesDir(t), key+".yaml")
}

// FixtureContentDir is a temp directory holding the shipped content/ files
// and every walk site under SitesDir: the shelf a test registry loads. A
// test about what ships reads ContentDir instead.
func FixtureContentDir(t testing.TB) string {
	t.Helper()

	dst := t.TempDir()
	for _, src := range []string{ContentDir(t), SitesDir(t)} {
		entries, err := os.ReadDir(src)
		if err != nil {
			t.Fatalf("dungeonstest: read %s: %v", src, err)
		}
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
				continue
			}
			copyFile(t, filepath.Join(src, e.Name()), filepath.Join(dst, e.Name()))
		}
	}

	return dst
}

// ShippedStore returns the shipped store over FixtureContentDir: the real
// content/ plus the retired walk sites. It is never written: the shipped
// store has no write path.
func ShippedStore(t testing.TB) *dungeons.FileRegistry {
	t.Helper()

	r, err := dungeons.NewFileRegistry(FixtureContentDir(t), Projector(t))
	if err != nil {
		t.Fatalf("dungeonstest: %v", err)
	}

	return r
}

// Shipped returns a read-only Registry over FixtureContentDir and
// an empty world store: every key resolves to its reference. WRITES are
// refused; a validate-only Put is answered, because a grade writes nothing
// (rpg-project#481).
func Shipped(t testing.TB) dungeons.Registry {
	t.Helper()

	return Join(t, ShippedStore(t), false)
}

// Scratch copies the shipped content into a temp directory and returns a
// Registry over it and an in-memory world store with authoring ON, for tests
// that Put. Saves land in the world store, never in the directory.
func Scratch(t testing.TB) (dungeons.Registry, string) {
	t.Helper()

	return scratch(t, true)
}

// ScratchReadOnly is Scratch with authoring OFF: the shape the server boots
// with when RPG_AUTHORING_ENABLED is unset.
func ScratchReadOnly(t testing.TB) (dungeons.Registry, string) {
	t.Helper()

	return scratch(t, false)
}

// ScratchDir copies the shipped content and the retired walk sites into a
// temp directory and returns it (FixtureContentDir).
func ScratchDir(t testing.TB) string {
	t.Helper()

	return FixtureContentDir(t)
}

// WorldStore returns a world store over its own miniredis.
func WorldStore(t testing.TB) worlddungeons.Repository {
	t.Helper()

	server := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	repo, err := worlddungeons.NewRedis(&worlddungeons.RedisConfig{Client: client})
	if err != nil {
		t.Fatalf("dungeonstest: %v", err)
	}

	return repo
}

// scratch copies the shipped content into a temp directory and joins a
// registry over it.
func scratch(t testing.TB, authoring bool) (dungeons.Registry, string) {
	t.Helper()

	dst := ScratchDir(t)
	shipped, err := dungeons.NewFileRegistry(dst, Projector(t))
	if err != nil {
		t.Fatalf("dungeonstest: %v", err)
	}

	return Join(t, shipped, authoring), dst
}

// Join builds the two-store Registry over shipped and a fresh world store.
func Join(t testing.TB, shipped *dungeons.FileRegistry, authoring bool) dungeons.Registry {
	t.Helper()

	r, err := dungeons.NewRegistry(&dungeons.RegistryConfig{
		Shipped:   shipped,
		World:     WorldStore(t),
		Authoring: authoring,
		Projector: Projector(t),
	})
	if err != nil {
		t.Fatalf("dungeonstest: %v", err)
	}

	return r
}

// copyFile copies one content file into the scratch directory.
func copyFile(t testing.TB, src, dst string) {
	t.Helper()

	raw, err := os.ReadFile(filepath.Clean(src)) //nolint:gosec // src is a ReadDir entry under the repo content dir
	if err != nil {
		t.Fatalf("dungeonstest: read %s: %v", src, err)
	}
	writeFile(t, dst, raw)
}

// writeFile writes one content file into a scratch directory.
func writeFile(t testing.TB, dst string, raw []byte) {
	t.Helper()

	if err := os.WriteFile(dst, raw, 0o600); err != nil { //nolint:gosec // dst is t.TempDir() + a content filename
		t.Fatalf("dungeonstest: write %s: %v", dst, err)
	}
}

// ShippedCount is how many dungeons a test registry's shelf holds
// (FixtureContentDir: content/ plus the retired walk sites) — what a picker
// over the shipped registry must list.
//
// COUNTED, never asserted as a literal: the content tree grows (it gained
// the heirloom fixture with rpg-project#368), and a hard-coded 1 turns every
// piece of new content into a test failure that says nothing about the
// content. A test that wants to know a SPECIFIC dungeon is there names it.
func ShippedCount(t testing.TB) int {
	t.Helper()

	n := 0
	for _, dir := range []string{ContentDir(t), SitesDir(t)} {
		n += countYAML(t, dir)
	}
	if n == 0 {
		t.Fatal("dungeonstest: the content directory holds no dungeons at all")
	}

	return n
}

// countYAML counts the dungeon files directly in dir.
func countYAML(t testing.TB, dir string) int {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("dungeonstest: read %s: %v", dir, err)
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".yaml" {
			n++
		}
	}

	return n
}

// AsKey is raw with its root `key: <from>` line renamed to `key: <to>`: the
// same document as an author's own copy. A shipped key is a reference, so a
// test that saves or grades a shipped file does it under a key of its own.
func AsKey(t testing.TB, raw []byte, from, to string) []byte {
	t.Helper()

	old := []byte("\nkey: " + from + "\n")
	if !bytes.Contains(raw, old) {
		t.Fatalf("dungeonstest: no root `key: %s` line", from)
	}

	return bytes.Replace(raw, old, []byte("\nkey: "+to+"\n"), 1)
}

// ListedReferenceKeys is every shipped key a list shows, sorted: the
// single-room references (root version 3 or more). The version 2 files stay
// resolvable by key and are left out (design R23).
func ListedReferenceKeys(t testing.TB) []string {
	t.Helper()

	shipped := ShippedStore(t)
	all, err := shipped.List(context.Background())
	if err != nil {
		t.Fatalf("dungeonstest: %v", err)
	}
	keys := make([]string, 0, len(all))
	for _, sum := range all {
		e, getErr := shipped.Get(context.Background(), sum.Key)
		if getErr != nil {
			t.Fatalf("dungeonstest: %v", getErr)
		}
		if e.SingleRoom {
			keys = append(keys, sum.Key)
		}
	}

	return keys
}
