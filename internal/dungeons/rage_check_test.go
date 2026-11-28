package dungeons_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter/dungeonspec/readback"

	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	"github.com/KirkDiggler/rpg-api/internal/repositories/worlddungeons"
)

// RAGE IS A NAMED ABILITY CONTENT CAN WRITE (rpg-project#1131, slice 8f): a
// stat block and an item may write `use: rage` with `damage_bonus: <a whole
// number of at least 1>`, and a table row may activate it. The by-level form
// (`damage_bonus: barbarian level`) is the barbarian's own and is refused.
// This server has no code behind it: these tests pin that the engine's rules
// reach the author unchanged.

const (
	rageKey         = "lieutenant-rages-walk"
	rageItemKey     = "rage-item-site"
	rageBonusLine   = "with: { damage_bonus: 2 }"
	rageByLevelLine = "with: { damage_bonus: barbarian level }"
	rageByLevelSays = `rage damage_bonus is "barbarian level"; it is a whole number of at least 1`
	rageItemPath    = "customItems.lieutenants-cloak"
)

// rageSite reads a fixture of this slice with each piece of its text replaced;
// a piece must occur exactly once so a change to the fixture cannot make the
// replacement a no-op.
func rageSite(t *testing.T, dir string, replace ...string) []byte {
	t.Helper()
	require.Zero(t, len(replace)%2, "from, to pairs")
	raw, err := os.ReadFile(filepath.Join("testdata", dir, "site.yaml"))
	require.NoError(t, err)
	for i := 0; i < len(replace); i += 2 {
		from, to := replace[i], replace[i+1]
		require.Equal(t, 1, bytes.Count(raw, []byte(from)), "precondition: %q is written once", from)
		raw = bytes.Replace(raw, []byte(from), []byte(to), 1)
	}
	return raw
}

// requireRageRefusedAt puts the site on validate-only and on save and asks for
// the one error the rules give, at the path, saying want; nothing is stored.
func requireRageRefusedAt(t *testing.T, key string, raw []byte, path, want string) {
	t.Helper()
	ctx := context.Background()
	world := dungeonstest.WorldStore(t)
	r := joinShelf(t, world, true)
	for _, validateOnly := range []bool{true, false} {
		out, err := r.Put(ctx, &dungeons.PutInput{Caller: alice, Key: key, YAML: raw, ValidateOnly: validateOnly})
		require.NoErrorf(t, err, "validate-only %v: a refusal is an answer, not an error", validateOnly)
		for _, e := range out.Errors {
			t.Logf("validate-only %v: %q: %s", validateOnly, e.Path, e.Message)
		}
		require.Lenf(t, out.Errors, 1, "validate-only %v: one error, no knock-on", validateOnly)
		require.Equal(t, path, out.Errors[0].Path)
		require.Contains(t, out.Errors[0].Message, want)
		require.Nil(t, out.Entry)
	}
	_, err := world.Get(ctx, &worlddungeons.GetInput{WorldID: alice.WorldID, Key: key})
	require.ErrorIs(t, err, worlddungeons.ErrNotFound, "nothing was stored")
}

func TestPutTakesASiteWhereACreatureRages(t *testing.T) {
	ctx := context.Background()
	world := dungeonstest.WorldStore(t)
	r := joinShelf(t, world, true)

	for _, validateOnly := range []bool{true, false} {
		out, err := r.Put(ctx, &dungeons.PutInput{Caller: alice, Key: rageKey, YAML: rageSite(t, "rage"), ValidateOnly: validateOnly})
		require.NoError(t, err)
		require.Emptyf(t, out.Errors, "validate-only %v: the site is accepted", validateOnly)
		if !validateOnly {
			require.NotNil(t, out.Entry, "the save stored the site")
			continue
		}
		require.NotNil(t, out.ReadBack)
		row, ok := activateRowStatement(out.ReadBack, activateRowPath0)
		require.True(t, ok, "a statement for the row")
		require.Contains(t, row.Sentence, "activates “rage”")
		requireConnectsTo(t, row, "rage", readback.RoleActivates)
		block, ok := activateRowStatement(out.ReadBack, "templates.gate-lieutenant.lets")
		require.True(t, ok, "a statement for the stat block")
		requireConnectsTo(t, block, "rage", readback.RoleLets)
		t.Logf("row:        %s", row.Sentence)
		t.Logf("stat block: %s", block.Sentence)
	}
}

func TestPutRefusesRageByLevelAtTheStatBlockAndStoresNothing(t *testing.T) {
	requireRageRefusedAt(t, rageKey, rageSite(t, "rage", rageBonusLine, rageByLevelLine), statBlockPath, rageByLevelSays)
}

func TestPutRefusesRageWithNoDamageBonusAtTheStatBlockAndStoresNothing(t *testing.T) {
	requireRageRefusedAt(t, rageKey, rageSite(t, "rage", "        "+rageBonusLine+"\n", ""), statBlockPath, "damage_bonus")
}

func TestPutTakesAnItemThatLetsAWearerRage(t *testing.T) {
	ctx := context.Background()
	world := dungeonstest.WorldStore(t)
	r := joinShelf(t, world, true)

	for _, validateOnly := range []bool{true, false} {
		out, err := r.Put(ctx, &dungeons.PutInput{Caller: alice, Key: rageItemKey, YAML: rageSite(t, "rage-item"), ValidateOnly: validateOnly})
		require.NoError(t, err)
		require.Emptyf(t, out.Errors, "validate-only %v: the site is accepted", validateOnly)
		if !validateOnly {
			require.NotNil(t, out.Entry, "the save stored the site")
		}
	}
}

func TestPutRefusesRageByLevelAtTheItemAndStoresNothing(t *testing.T) {
	requireRageRefusedAt(t, rageItemKey, rageSite(t, "rage-item", rageBonusLine, rageByLevelLine), rageItemPath, rageByLevelSays)
}

func TestPutRefusesRageWithNoDamageBonusAtTheItemAndStoresNothing(t *testing.T) {
	requireRageRefusedAt(t, rageItemKey, rageSite(t, "rage-item", "        "+rageBonusLine+"\n", ""), rageItemPath, "damage_bonus")
}

func TestPutRefusesARowThatMisspellsRageAndOffersTheAbilities(t *testing.T) {
	requireRageRefusedAt(t, rageKey, rageSite(t, "rage", "activate: rage }", "activate: rge }"), activateRowPath0, "action surge, rage, second wind")
}
