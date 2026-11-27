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

// A TABLE ROW THAT USES AN ABILITY (rpg-project#1131, slice 3d): a row says
// `activate: <word>`; the site format takes it as one word, the read-back
// says the row, and the rules refuse a word that is no ability. The rules'
// refusal has no place (a compiled row no longer knows its line), so a save
// places it: at every row of the file that says the word, or on no path when
// none can be found. Nothing is stored.

const (
	activateRowKey   = "lieutenant-uses-second-wind-walk"
	activateRowPath0 = "behaviorDefinitions.lieutenant-policy.table.time[0]"
	activateRowPath3 = "behaviorDefinitions.lieutenant-policy.table.time[3]"
	activateRowGood  = "- { activate: second wind }"
	activateRowBad   = "- { activate: secnd wind }"
	activateRowFar   = "- { when: { enemy: far }, activate: secnd wind }"
	activateRowLast  = "- { when: { enemy: reach }, attack: enemy }"
)

// activateRowSite is the fixture with each piece of its text replaced; a
// piece must occur exactly once so a change to the fixture cannot make the
// replacement a no-op.
func activateRowSite(t *testing.T, replace ...string) []byte {
	t.Helper()
	require.Zero(t, len(replace)%2, "from, to pairs")
	raw, err := os.ReadFile(filepath.Join("testdata", "activate-row", "site.yaml"))
	require.NoError(t, err)
	for i := 0; i < len(replace); i += 2 {
		from, to := replace[i], replace[i+1]
		require.Equal(t, 1, bytes.Count(raw, []byte(from)), "precondition: %q is written once", from)
		raw = bytes.Replace(raw, []byte(from), []byte(to), 1)
	}
	return raw
}

func activateRowStatement(rb *readback.ReadBack, path string) (readback.Statement, bool) {
	for _, st := range rb.Statements() {
		if st.Path == path {
			return st, true
		}
	}
	return readback.Statement{}, false
}

func requireConnectsTo(t *testing.T, st readback.Statement, ability, role string) {
	t.Helper()
	for _, c := range st.Connects {
		if c.Kind == readback.ConnAbility && c.ID == ability && c.Role == role {
			return
		}
	}
	require.Failf(t, "no connection", "statement at %q does not connect to ability %q as %q: %+v", st.Path, ability, role, st.Connects)
}

func TestPutTakesATableRowThatActivatesAnAbility(t *testing.T) {
	ctx := context.Background()
	world := dungeonstest.WorldStore(t)
	r := joinShelf(t, world, true)

	for _, validateOnly := range []bool{true, false} {
		out, err := r.Put(ctx, &dungeons.PutInput{Caller: alice, Key: activateRowKey, YAML: activateRowSite(t), ValidateOnly: validateOnly})
		require.NoError(t, err)
		require.Emptyf(t, out.Errors, "validate-only %v: the site is accepted", validateOnly)
		if !validateOnly {
			require.NotNil(t, out.Entry, "the save stored the site")
			continue
		}
		require.NotNil(t, out.ReadBack)
		row, ok := activateRowStatement(out.ReadBack, activateRowPath0)
		require.True(t, ok, "a statement for the row")
		require.Contains(t, row.Sentence, "activates “second wind”")
		requireConnectsTo(t, row, "second wind", readback.RoleActivates)
		block, ok := activateRowStatement(out.ReadBack, "templates.gate-lieutenant.lets")
		require.True(t, ok, "a statement for the stat block")
		requireConnectsTo(t, block, "second wind", readback.RoleLets)
		t.Logf("row:        %s", row.Sentence)
		t.Logf("stat block: %s", block.Sentence)
	}
}

func TestPutRefusesARowWhoseWordIsNoAbilityAtTheRowAndStoresNothing(t *testing.T) {
	ctx := context.Background()
	world := dungeonstest.WorldStore(t)
	r := joinShelf(t, world, true)

	for _, validateOnly := range []bool{true, false} {
		out, err := r.Put(ctx, &dungeons.PutInput{Caller: alice, Key: activateRowKey, YAML: activateRowSite(t, activateRowGood, activateRowBad), ValidateOnly: validateOnly})
		require.NoErrorf(t, err, "validate-only %v: a refusal is an answer, not an error", validateOnly)
		require.Lenf(t, out.Errors, 1, "validate-only %v: %v", validateOnly, out.Errors)
		require.Equal(t, activateRowPath0, out.Errors[0].Path, "at the row")
		require.Contains(t, out.Errors[0].Message, "secnd wind")
		require.Contains(t, out.Errors[0].Message, "second wind", "the words that are abilities are listed")
		require.Nil(t, out.Entry)
		t.Logf("validate-only %v: %q: %s", validateOnly, out.Errors[0].Path, out.Errors[0].Message)
	}
	_, err := world.Get(ctx, &worlddungeons.GetInput{WorldID: alice.WorldID, Key: activateRowKey})
	require.ErrorIs(t, err, worlddungeons.ErrNotFound, "nothing was stored")
}

func TestPutRefusesEveryRowWhoseWordIsNoAbilityInPathOrder(t *testing.T) {
	ctx := context.Background()
	world := dungeonstest.WorldStore(t)
	r := joinShelf(t, world, true)

	raw := activateRowSite(t, activateRowGood, activateRowBad, activateRowLast, activateRowLast+"\n        "+activateRowFar)
	for _, validateOnly := range []bool{true, false} {
		out, err := r.Put(ctx, &dungeons.PutInput{Caller: alice, Key: activateRowKey, YAML: raw, ValidateOnly: validateOnly})
		require.NoErrorf(t, err, "validate-only %v", validateOnly)
		require.Lenf(t, out.Errors, 2, "validate-only %v: %v", validateOnly, out.Errors)
		require.Equal(t, activateRowPath0, out.Errors[0].Path)
		require.Equal(t, activateRowPath3, out.Errors[1].Path)
		require.Equal(t, out.Errors[0].Message, out.Errors[1].Message, "the same message at each place")
		require.Nil(t, out.Entry)
	}
	_, err := world.Get(ctx, &worlddungeons.GetInput{WorldID: alice.WorldID, Key: activateRowKey})
	require.ErrorIs(t, err, worlddungeons.ErrNotFound, "nothing was stored")
}
