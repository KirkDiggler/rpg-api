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

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
)

// GoblinDataJSON returns valid, rehydratable monster.Data JSON for a real
// registry goblin with the given entity id, for use as
// tkenc.MonsterInput.DataJSON in hand-built test fixtures.
func GoblinDataJSON(tb testing.TB, id string) []byte {
	tb.Helper()
	b, err := json.Marshal(monster.NewGoblin(id).ToData())
	require.NoError(tb, err)
	return b
}
