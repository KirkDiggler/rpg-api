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
	"testing"

	"github.com/stretchr/testify/require"

	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
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
