// Package monsterfixture provides shared monster.Data fixtures for
// hand-built tkenc.MonsterInput test fixtures across rpg-api's test
// suites — the encounter/v2 orchestrator, encounter/v2 handler, and
// integration packages all need the same shape.
//
// rpg-toolkit#895's no-fallback rider makes AddMonster reject empty
// DataJSON (encounter.ErrMonsterDataRequired) — before the rider, NPCAct
// silently degraded to npcActScripted's minimal closest-player attack for
// a monster with no DataJSON; that fallback is deleted, so a fixture with
// none can no longer join an encounter at all. GoblinDataJSON gives every
// such fixture a real, rehydratable registry monster instead of repeating
// the marshal boilerplate at each call site (mirrors the toolkit's own
// encounter_test package helper, testGoblinDataJSON).
package monsterfixture

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	// NewGoblin moved out of monster into monster/monsters at dnd5e v0.97.0
	// (rpg-toolkit's composable-attack-damage-provider migration,
	// rpg-toolkit#1146 / ADR-0040): the bare monster package holds only the
	// runtime Monster type and Config now, and per-species factories live in
	// this sibling package.
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster/monsters"
)

// GoblinDataJSON returns valid, rehydratable monster.Data JSON for a real
// registry goblin with the given entity id, for use as
// tkenc.MonsterInput.DataJSON in hand-built test fixtures.
func GoblinDataJSON(tb testing.TB, id string) []byte {
	tb.Helper()
	b, err := json.Marshal(monsters.NewGoblin(id).ToData())
	require.NoError(tb, err)
	return b
}
