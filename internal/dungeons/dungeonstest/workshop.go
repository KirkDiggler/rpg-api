package dungeonstest

import (
	"os"
	"path/filepath"
	"testing"
)

// The single-room fixtures (internal/dungeons/testdata): shared YAML, one
// copy, so no Go test file carries the authored source as a raw string. The
// `play.standing` value these rooms declare is the provider-owned contract
// literal and is deliberately NOT respelled here.
const (
	// WorkshopRoomKey is the full room's own `key:` line: the grouped raised
	// prop, the supported lit decor, the explicit declarations, two placed
	// skeletons (one on a negative odd axial row) and the party start.
	WorkshopRoomKey = "workshop-room"

	// WorkshopOneSeatKey is the one-seat variant's own `key:` line: the same
	// room reduced to a single walkable hex, so the launch's seat-capacity
	// gate has exactly one seat.
	WorkshopOneSeatKey = "workshop-one-seat"
)

// WorkshopRoomYAML is the full authored room, read from the shared fixture
// so every suite plays the same bytes.
func WorkshopRoomYAML(t testing.TB) []byte {
	t.Helper()
	return roomFixture(t, WorkshopRoomKey)
}

// WorkshopOneSeatYAML is the one-seat variant, read from the same fixture
// directory.
func WorkshopOneSeatYAML(t testing.TB) []byte {
	t.Helper()
	return roomFixture(t, WorkshopOneSeatKey)
}

// roomFixture reads one testdata room by the key its file is named for --
// any single-room dialect, since the reader only names the file.
// The directory is anchored on the repo root through the content tree
// (ContentDir's own walk), not on the caller's cwd: a suite in ANY package
// runs this helper with its own working directory, and a relative "../"
// from there names whatever happens to sit beside that package.
func roomFixture(t testing.TB, key string) []byte {
	t.Helper()

	root := filepath.Dir(ContentDir(t))
	path := filepath.Clean(filepath.Join(root, "internal", "dungeons", "testdata", key+".yaml"))
	raw, err := os.ReadFile(path) //nolint:gosec // path is the repo's own testdata dir joined with a named constant key
	if err != nil {
		t.Fatalf("dungeonstest: read the shared single-room fixture %s: %v", key, err)
	}

	return raw
}
