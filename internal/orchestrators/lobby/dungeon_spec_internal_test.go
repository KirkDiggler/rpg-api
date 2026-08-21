package lobby

// White-box unit tests for LoadContentRegistry (dungeon_spec.go) — package
// lobby (not lobby_test): the legacy tkenc.DungeonParams builder table and
// its resolveDungeonSpec/resolveContentDungeonSpec/DisabledDungeonKeyError
// machinery were removed with the old encounter stack (rpg-project#227):
// the session stack's StartEncounter never resolves a DungeonKey at all
// (start_encounter_session_stack.go always plays the embedded reference
// tomb). LoadContentRegistry itself survives — ListDungeons and the
// authoring orchestrator both still read the live registry it builds — so
// its own coverage stays here.
import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoadContentRegistry_EmbeddedReferenceTombCompiles proves the M1
// acceptance file (internal/content/dungeons/reference-tomb.yaml) is
// present in LoadContentRegistry's startup registry, compiled cleanly, and
// its declared display Name was captured — no RPG_CONTENT_DIR override
// needed, since reference-tomb ships embedded.
func TestLoadContentRegistry_EmbeddedReferenceTombCompiles(t *testing.T) {
	registry, err := LoadContentRegistry()
	require.NoError(t, err)

	entry, ok := registry.Get("reference-tomb")
	require.True(t, ok, "reference-tomb must be present in the startup registry")
	require.NoError(t, entry.Err, "reference-tomb must compile cleanly — it's the M1 acceptance file")
	require.Len(t, entry.Compiled.Params.Regions, 3, "entrance + hall + tomb (Kirk's live-authored 3-room draft)")
	require.Equal(t, DefaultPartyCap, entry.Compiled.Params.PartyStart.SeatCount,
		"content startup must compile through the toolkit load/config seam at the normal product capacity")
	require.Equal(t, "The Tomb of the Captain", entry.Name, "Name must be captured via dungeonspec.Decode, not left blank")
}

// TestLoadContentRegistry_ContentDirUnreadable_ReturnsError proves an
// unreadable RPG_CONTENT_DIR path fails loudly here — the caller
// (cmd/server/server.go) propagates this as a construction failure,
// never a panic, and lobbyorch.New() is never even reached.
func TestLoadContentRegistry_ContentDirUnreadable_ReturnsError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	t.Setenv("RPG_CONTENT_DIR", missing)

	_, err := LoadContentRegistry()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "RPG_CONTENT_DIR", "message must name the env var an operator set")
}

// TestLoadContentRegistry_BrokenOverrideFile_StoredAsDisabledNotConstructionFailure
// proves the OTHER failure mode: a content file with a valid key: field
// that nonetheless fails dungeonspec.Load (a schema/business-rule
// violation — here, a single-room spec violating the >=2-rooms rule)
// must NOT fail LoadContentRegistry itself; it's stored as a disabled
// (errored) dungeonregistry.Entry for that one key, every other key
// unaffected.
func TestLoadContentRegistry_BrokenOverrideFile_StoredAsDisabledNotConstructionFailure(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "broken-dungeon.yaml"),
		[]byte("version: 1\nkey: broken-dungeon\nname: Broken\nheight: 8\nrooms:\n  - id: only-one\n    archetype: entrance\n    width: 6\n"),
		0o600,
	))
	t.Setenv("RPG_CONTENT_DIR", dir)

	registry, err := LoadContentRegistry()
	require.NoError(t, err, "a schema-invalid content FILE must not fail construction")

	entry, ok := registry.Get("broken-dungeon")
	require.True(t, ok, "the key must still be present -- found, but disabled")
	require.Error(t, entry.Err)

	// The embedded reference-tomb must be unaffected by the other key's
	// failure -- one broken file must never take down the whole registry.
	tombEntry, ok := registry.Get("reference-tomb")
	require.True(t, ok)
	require.NoError(t, tombEntry.Err)
}
