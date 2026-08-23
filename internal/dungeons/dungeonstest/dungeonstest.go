// Package dungeonstest builds content registries for tests from the shipped
// content tree, so every suite that starts an encounter runs on the same
// reference tomb the server boots with rather than a fixture that could
// drift from it.
package dungeonstest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/KirkDiggler/rpg-api/internal/dungeons"
)

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

	r, err := dungeons.NewFileRegistry(ContentDir(t), false)
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

	r, err := dungeons.NewFileRegistry(dst, true)
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
