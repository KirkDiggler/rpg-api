package lobby

// White-box unit tests for resolveDungeonSpec (dungeon_spec.go) —
// package lobby (not lobby_test): dungeonSpec/resolveDungeonSpec are
// unexported. These are the fast, direct proofs of key-selection/default
// behavior and unknown-key error behavior (rpg-api#688); the black-box
// StartEncounter-level tests in start_encounter_test.go additionally prove
// the resolved spec actually reaches the toolkit unmodified.

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
)

func TestResolveDungeonSpec_ZeroValue_ResolvesToDefaultCrypt(t *testing.T) {
	key, spec, err := resolveDungeonSpec("")
	require.NoError(t, err)
	require.Equal(t, DungeonKeyCrypt, key, "the zero value must resolve to the named default")
	require.Equal(t, dungeonSpecs[DungeonKeyCrypt], spec)
}

func TestResolveDungeonSpec_ExplicitCryptKey_ReturnsCryptSpec(t *testing.T) {
	key, spec, err := resolveDungeonSpec(DungeonKeyCrypt)
	require.NoError(t, err)
	require.Equal(t, DungeonKeyCrypt, key)
	require.Equal(t, "crypt", spec.theme)
	require.Len(t, spec.regions, 3, "the crypt spec is a 3-region linear chain")
	require.Len(t, spec.connectors, 2, "an N-region chain needs exactly N-1 connectors")

	archetypes := make([]tkenc.RegionArchetype, len(spec.regions))
	for i, r := range spec.regions {
		archetypes[i] = r.Archetype
	}
	require.Equal(t, []tkenc.RegionArchetype{tkenc.ArchetypeEntrance, tkenc.ArchetypeCorridor, tkenc.ArchetypeBoss}, archetypes,
		"entrance -> corridor -> boss, in that order")
}

func TestResolveDungeonSpec_UnknownKey_ReturnsErrUnknownDungeonKey(t *testing.T) {
	_, _, err := resolveDungeonSpec(DungeonKey("no-such-key"))
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrUnknownDungeonKey))
}
