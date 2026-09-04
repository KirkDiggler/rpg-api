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

func (p managerProjector) AtlasOf(ctx context.Context, world *tkencounter.EncounterData) (*sdk.Atlas, error) {
	return p.m.AtlasOf(ctx, &sdk.AtlasOfInput{World: world})
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
// Puts are refused, so nothing a test does can touch the tree.
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

	r, err := dungeons.NewFileRegistry(dst, true, Projector(t))
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
