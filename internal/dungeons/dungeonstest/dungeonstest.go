// Package dungeonstest builds content registries for tests from the shipped
// content tree, so every suite that starts an encounter runs on the same
// reference tomb the server boots with rather than a fixture that could
// drift from it.
package dungeonstest

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"

	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

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
	ctx context.Context, key string, world *tkencounter.EncounterData,
) (*sdk.Atlas, error) {
	return p.m.AtlasOf(ctx, &sdk.AtlasOfInput{World: world, Dungeon: key})
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

// Shipped returns a read-only registry over the real content/ directory.
// WRITES are refused, so nothing a test does can touch the tree; a
// validate-only Put is answered, because a grade writes nothing
// (rpg-project#481). A test that wants a read-only registry it may Put to
// freely wants ScratchReadOnly.
func Shipped(t testing.TB) *dungeons.FileRegistry {
	t.Helper()

	r, err := dungeons.NewFileRegistry(ContentDir(t), false, Projector(t))
	if err != nil {
		t.Fatalf("dungeonstest: %v", err)
	}

	return r
}

// Scratch copies the shipped content into a temp directory and returns a
// registry over it with authoring ON, for tests that Put.
func Scratch(t testing.TB) (*dungeons.FileRegistry, string) {
	t.Helper()

	return scratch(t, true)
}

// ScratchReadOnly is Scratch with authoring OFF: the same temp copy of the
// shipped content, behind a registry that refuses to store anything — the
// shape the server boots with when RPG_AUTHORING_ENABLED is unset.
//
// A COPY rather than Shipped's real content/ directory, deliberately: a test
// that proves the write refusal should not be the one test whose failure
// writes into the repo's own content tree.
func ScratchReadOnly(t testing.TB) (*dungeons.FileRegistry, string) {
	t.Helper()

	return scratch(t, false)
}

// scratch copies the shipped content into a temp directory and opens a
// registry over it.
func scratch(t testing.TB, authoring bool) (*dungeons.FileRegistry, string) {
	t.Helper()

	src := ContentDir(t)
	dst := t.TempDir()
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

	r, err := dungeons.NewFileRegistry(dst, authoring, Projector(t))
	if err != nil {
		t.Fatalf("dungeonstest: %v", err)
	}

	return r, dst
}

// copyFile copies one content file into the scratch directory.
func copyFile(t testing.TB, src, dst string) {
	t.Helper()

	raw, err := os.ReadFile(filepath.Clean(src)) //nolint:gosec // src is a ReadDir entry under the repo content dir
	if err != nil {
		t.Fatalf("dungeonstest: read %s: %v", src, err)
	}
	if err := os.WriteFile(dst, raw, 0o600); err != nil { //nolint:gosec // dst is t.TempDir() + a content filename
		t.Fatalf("dungeonstest: write %s: %v", dst, err)
	}
}

// ShippedCount is how many dungeons the content/ directory holds — what a
// picker over the shipped registry must list.
//
// COUNTED, never asserted as a literal: the content tree grows (it gained
// the heirloom fixture with rpg-project#368), and a hard-coded 1 turns every
// piece of new content into a test failure that says nothing about the
// content. A test that wants to know a SPECIFIC dungeon is there names it.
func ShippedCount(t testing.TB) int {
	t.Helper()

	entries, err := os.ReadDir(ContentDir(t))
	if err != nil {
		t.Fatalf("dungeonstest: read content dir: %v", err)
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".yaml" {
			n++
		}
	}
	if n == 0 {
		t.Fatal("dungeonstest: the content directory holds no dungeons at all")
	}

	return n
}
