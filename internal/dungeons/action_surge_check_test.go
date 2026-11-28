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

// ACTION SURGE IS A NAMED ABILITY THAT MUST BE PAID FROM A COUNT
// (rpg-project#1131, slice 5d-3): a stat block that lets a creature use
// Action Surge, and a table row that activates it, are taken when the entry
// carries a count; an entry with no count is refused by the rules at the stat
// block, and nothing is stored. This server has no code behind it: these tests
// pin that the engine's rules reach the author unchanged.

const (
	actionSurgeKey = "lieutenant-surges-walk"
	// the one line of the entry that gives Action Surge its count.
	actionSurgeCountLine = "        uses: { max: 1, refills: { on: [short rest, long rest] } }\n"
)

// actionSurgeSite is the fixture with each piece of its text replaced; a
// piece must occur exactly once so a change to the fixture cannot make the
// replacement a no-op.
func actionSurgeSite(t *testing.T, replace ...string) []byte {
	t.Helper()
	require.Zero(t, len(replace)%2, "from, to pairs")
	raw, err := os.ReadFile(filepath.Join("testdata", "action-surge", "site.yaml"))
	require.NoError(t, err)
	for i := 0; i < len(replace); i += 2 {
		from, to := replace[i], replace[i+1]
		require.Equal(t, 1, bytes.Count(raw, []byte(from)), "precondition: %q is written once", from)
		raw = bytes.Replace(raw, []byte(from), []byte(to), 1)
	}
	return raw
}

func TestPutTakesASiteThatUsesActionSurge(t *testing.T) {
	ctx := context.Background()
	world := dungeonstest.WorldStore(t)
	r := joinShelf(t, world, true)

	for _, validateOnly := range []bool{true, false} {
		out, err := r.Put(ctx, &dungeons.PutInput{Caller: alice, Key: actionSurgeKey, YAML: actionSurgeSite(t), ValidateOnly: validateOnly})
		require.NoError(t, err)
		require.Emptyf(t, out.Errors, "validate-only %v: the site is accepted", validateOnly)
		if !validateOnly {
			require.NotNil(t, out.Entry, "the save stored the site")
			continue
		}
		require.NotNil(t, out.ReadBack)
		row, ok := activateRowStatement(out.ReadBack, activateRowPath0)
		require.True(t, ok, "a statement for the row")
		require.Contains(t, row.Sentence, "activates “action surge”")
		requireConnectsTo(t, row, "action surge", readback.RoleActivates)
		block, ok := activateRowStatement(out.ReadBack, "templates.gate-lieutenant.lets")
		require.True(t, ok, "a statement for the stat block")
		requireConnectsTo(t, block, "action surge", readback.RoleLets)
		t.Logf("row:        %s", row.Sentence)
		t.Logf("stat block: %s", block.Sentence)
	}
}

func TestPutRefusesActionSurgeWithNoCountAtTheStatBlockAndStoresNothing(t *testing.T) {
	ctx := context.Background()
	world := dungeonstest.WorldStore(t)
	r := joinShelf(t, world, true)

	raw := actionSurgeSite(t, actionSurgeCountLine, "")
	for _, validateOnly := range []bool{true, false} {
		out, err := r.Put(ctx, &dungeons.PutInput{Caller: alice, Key: actionSurgeKey, YAML: raw, ValidateOnly: validateOnly})
		require.NoErrorf(t, err, "validate-only %v: a refusal is an answer, not an error", validateOnly)
		for _, e := range out.Errors {
			t.Logf("validate-only %v: %q: %s", validateOnly, e.Path, e.Message)
		}
		require.Lenf(t, out.Errors, 1, "validate-only %v: one error, no knock-on", validateOnly)
		require.Equal(t, statBlockPath, out.Errors[0].Path, "at the stat block")
		require.Contains(t, out.Errors[0].Message, "action surge")
		require.Contains(t, out.Errors[0].Message, "count", "it says the ability needs a count")
		require.Contains(t, out.Errors[0].Message, "uses", "it offers uses")
		require.Contains(t, out.Errors[0].Message, "cost", "it offers a cost")
		require.Nil(t, out.Entry)
	}
	_, err := world.Get(ctx, &worlddungeons.GetInput{WorldID: alice.WorldID, Key: actionSurgeKey})
	require.ErrorIs(t, err, worlddungeons.ErrNotFound, "nothing was stored")
}

func TestPutRefusesARowThatMisspellsActionSurgeAndOffersTheAbilities(t *testing.T) {
	ctx := context.Background()
	world := dungeonstest.WorldStore(t)
	r := joinShelf(t, world, true)

	raw := actionSurgeSite(t, "activate: action surge, weight: 9", "activate: actoin surge, weight: 9")
	for _, validateOnly := range []bool{true, false} {
		out, err := r.Put(ctx, &dungeons.PutInput{Caller: alice, Key: actionSurgeKey, YAML: raw, ValidateOnly: validateOnly})
		require.NoErrorf(t, err, "validate-only %v", validateOnly)
		for _, e := range out.Errors {
			t.Logf("validate-only %v: %q: %s", validateOnly, e.Path, e.Message)
		}
		require.Lenf(t, out.Errors, 1, "validate-only %v", validateOnly)
		require.Equal(t, activateRowPath0, out.Errors[0].Path, "at the row")
		require.Contains(t, out.Errors[0].Message, "actoin surge")
		require.Contains(t, out.Errors[0].Message, "action surge, second wind", "the abilities are offered")
		require.Nil(t, out.Entry)
	}
	_, err := world.Get(ctx, &worlddungeons.GetInput{WorldID: alice.WorldID, Key: actionSurgeKey})
	require.ErrorIs(t, err, worlddungeons.ErrNotFound, "nothing was stored")
}
