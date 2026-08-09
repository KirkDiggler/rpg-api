package lobby

// White-box unit tests for resolveDungeonSpec (dungeon_spec.go) —
// package lobby (not lobby_test): resolveDungeonSpec/dungeonSpecs are
// unexported. These are the fast, direct proofs of key-selection/default
// behavior, unknown-key error behavior, and — rpg-api#694 — that the
// crypt key resolves through the toolkit's own tkenc.CryptDungeonParams
// constructor VERBATIM rather than an API-owned duplicate of its region
// widths/heights/connectors/obstacle specs. The black-box
// StartEncounter-level tests in start_encounter_test.go additionally
// prove the resolved params actually reach persisted encounter state,
// obstacles included.
import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/dungeonspec"

	"github.com/KirkDiggler/rpg-api/internal/dungeonregistry"
)

// TestResolveDungeonSpec_ExplicitCryptKey_MatchesToolkitConstructorVerbatim
// is the rpg-api#694 headline proof: resolveDungeonSpec(DungeonKeyCrypt,
// seed) must equal calling tkenc.CryptDungeonParams(seed, ...) directly
// with this package's own connector door IDs — not a hand-rolled region/
// connector/height/theme literal that happens to look similar. Any
// reintroduction of API-owned dimensions (a duplicate width, a stale
// height, a hand-authored Obstacles list) would diverge from this
// equality and fail here first, before any StartEncounter-level test even
// runs.
func TestResolveDungeonSpec_ExplicitCryptKey_MatchesToolkitConstructorVerbatim(t *testing.T) {
	const seed = int64(123)
	key, params, err := resolveDungeonSpec(DungeonKeyCrypt, seed)
	require.NoError(t, err)
	require.Equal(t, DungeonKeyCrypt, key)

	want := tkenc.CryptDungeonParams(seed, cryptDoorEntranceToCorridor, cryptDoorCorridorToBoss)
	require.Equal(t, want, params,
		"resolveDungeonSpec must forward to tkenc.CryptDungeonParams verbatim -- "+
			"rpg-api owns no region widths/heights/connectors/obstacle specs of its own")
}

// TestResolveDungeonSpec_ZeroValue_ResolvesToDefaultCrypt proves the zero
// value DungeonKey resolves to the exact same toolkit-constructed params
// as an explicit DungeonKeyCrypt, for the SAME seed.
func TestResolveDungeonSpec_ZeroValue_ResolvesToDefaultCrypt(t *testing.T) {
	const seed = int64(456)
	keyDefault, paramsDefault, err := resolveDungeonSpec("", seed)
	require.NoError(t, err)
	require.Equal(t, DungeonKeyCrypt, keyDefault, "the zero value must resolve to the named default")

	keyExplicit, paramsExplicit, err := resolveDungeonSpec(DungeonKeyCrypt, seed)
	require.NoError(t, err)
	require.Equal(t, keyExplicit, keyDefault)
	require.Equal(t, paramsExplicit, paramsDefault)
}

// TestResolveDungeonSpec_DifferentSeeds_ThreadSeedIntoParams proves the
// seed argument is actually threaded into the returned params (a builder
// that ignored its seed argument would pass this test's sibling but fail
// here): two different seeds must produce two different
// tkenc.DungeonParams.RandomSeed values, matching what was passed in.
func TestResolveDungeonSpec_DifferentSeeds_ThreadSeedIntoParams(t *testing.T) {
	_, params1, err := resolveDungeonSpec(DungeonKeyCrypt, 1)
	require.NoError(t, err)
	_, params2, err := resolveDungeonSpec(DungeonKeyCrypt, 2)
	require.NoError(t, err)

	require.Equal(t, int64(1), params1.RandomSeed)
	require.Equal(t, int64(2), params2.RandomSeed)
	require.NotEqual(t, params1, params2)
}

// TestResolveDungeonSpec_ExplicitCryptKey_ThreeRegionArchetypeChain keeps
// the pre-#694 structural proof (3-region entrance->corridor->boss chain,
// 2 connectors) alive, now read back off the toolkit-returned params
// instead of an API-owned literal.
func TestResolveDungeonSpec_ExplicitCryptKey_ThreeRegionArchetypeChain(t *testing.T) {
	_, params, err := resolveDungeonSpec(DungeonKeyCrypt, 789)
	require.NoError(t, err)
	require.Equal(t, "crypt", params.Theme)
	require.Len(t, params.Regions, 3, "the crypt spec is a 3-region linear chain")
	require.Len(t, params.Connectors, 2, "an N-region chain needs exactly N-1 connectors")

	archetypes := make([]tkenc.RegionArchetype, len(params.Regions))
	for i, r := range params.Regions {
		archetypes[i] = r.Archetype
	}
	require.Equal(t, []tkenc.RegionArchetype{tkenc.ArchetypeEntrance, tkenc.ArchetypeCorridor, tkenc.ArchetypeBoss}, archetypes,
		"entrance -> corridor -> boss, in that order")
}

func TestResolveDungeonSpec_UnknownKey_ReturnsErrUnknownDungeonKey(t *testing.T) {
	_, _, err := resolveDungeonSpec(DungeonKey("no-such-key"), 0)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrUnknownDungeonKey))
}

// --- Task E2: content-backed dungeon spec resolution ---

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
// unaffected — resolveContentDungeonSpec is what turns this into a
// caller-visible DisabledDungeonKeyError, per-request, not at startup.
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

func TestLoadContentRegistry_UsesStrictCompilationForDraftOnlyRegionFloor(t *testing.T) {
	dir := t.TempDir()
	const source = `version: 1
key: tiny-draft-only
name: Tiny Draft Only
canvas: { width: 3, height: 2, floor_source: regions }
rooms: []
regions: [{ id: tiny, cells: [[0,0], [1,0]] }]
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tiny-draft-only.yaml"), []byte(source), 0o600))
	t.Setenv("RPG_CONTENT_DIR", dir)

	registry, err := LoadContentRegistry()
	require.NoError(t, err)
	entry, ok := registry.Get("tiny-draft-only")
	require.True(t, ok, "strict-invalid content remains discoverable as a disabled key")
	require.Error(t, entry.Err)
	var fieldErr *contentFieldError
	require.ErrorAs(t, entry.Err, &fieldErr)
	require.Equal(t, "canvas.floor_source", fieldErr.Field)
	require.Equal(t, "no floor anchor has a complete same-component party start envelope", fieldErr.Message)
	require.Equal(t, "entrance_unavailable", fieldErr.Code)
}

func TestResolveContentDungeonSpec_NotFound_ReturnsFoundFalse(t *testing.T) {
	o := &Orchestrator{registry: dungeonregistry.New(nil)}
	compiled, found, err := o.resolveContentDungeonSpec(DungeonKey("no-such-key"))
	require.False(t, found)
	require.NoError(t, err)
	require.Equal(t, dungeonspec.CompiledDungeon{}, compiled)
}

func TestResolveContentDungeonSpec_FoundAndValid_ReturnsCompiledNoError(t *testing.T) {
	want := dungeonspec.CompiledDungeon{Params: tkenc.DungeonParams{Theme: "crypt"}}
	o := &Orchestrator{registry: dungeonregistry.New(map[string]dungeonregistry.Entry{
		"reference-tomb": {Compiled: want},
	})}

	compiled, found, err := o.resolveContentDungeonSpec(DungeonKey("reference-tomb"))
	require.True(t, found)
	require.NoError(t, err)
	require.Equal(t, want, compiled)
}

func TestResolveContentDungeonSpec_FoundButDisabled_WrapsStoredError(t *testing.T) {
	cause := errors.New("boom: at least 2 rooms required")
	o := &Orchestrator{registry: dungeonregistry.New(map[string]dungeonregistry.Entry{
		"broken-dungeon": {Err: cause},
	})}

	compiled, found, err := o.resolveContentDungeonSpec(DungeonKey("broken-dungeon"))
	require.True(t, found, "a disabled key is still FOUND -- it must never fall through to the legacy map")
	require.Error(t, err)
	require.Equal(t, dungeonspec.CompiledDungeon{}, compiled)

	var disabledErr *DisabledDungeonKeyError
	require.True(t, errors.As(err, &disabledErr))
	require.Equal(t, DungeonKey("broken-dungeon"), disabledErr.Key)
	require.True(t, errors.Is(err, cause), "Unwrap must expose the stored cause")
}

func TestDisabledDungeonKeyError_ErrorMessageNamesKeyAndCause(t *testing.T) {
	err := &DisabledDungeonKeyError{Key: "broken-dungeon", Cause: errors.New("bad spec")}
	require.Contains(t, err.Error(), "broken-dungeon")
	require.Contains(t, err.Error(), "bad spec")
}
