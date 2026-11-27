package dungeons_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	"github.com/KirkDiggler/rpg-api/internal/repositories/worlddungeons"
)

// A ROW'S WHEN READS ITS CREATURE'S OWN HIT POINTS (rpg-project#1131, slice 4d):
// an author writes `hit_points: { at_most: 8 }` (a number of hit points) or
// `{ at_most: 30% }` (a share of the maximum). The site format takes either,
// the read-back says each with its unit, and a shape that is neither is refused
// at the row when the site is checked or saved. Nothing is stored.

const (
	hitPointsKey   = "lieutenant-heals-when-hurt-walk"
	hitPointsPath0 = "behaviorDefinitions.lieutenant-policy.table.time[0]"
	hitPointsPath1 = "behaviorDefinitions.lieutenant-policy.table.time[1]"
	hitPointsPath2 = "behaviorDefinitions.lieutenant-policy.table.time[2]"
	hitPointsPath3 = "behaviorDefinitions.lieutenant-policy.table.time[3]"
	// a decode refusal is placed at the table, with the line of the row's text
	hitPointsTableLine = "behaviorDefinitions.lieutenant-policy.table.line 1"
	hitPointsShare     = "{ at_most: 30% }"
	hitPointsAway      = "- { when: { hit_points: { at_most: 5 } }, away: enemy }"
)

// hitPointsSite is the fixture with each piece of its text replaced; a piece
// must occur exactly once so a change to the fixture cannot make the
// replacement a no-op.
func hitPointsSite(t *testing.T, replace ...string) []byte {
	t.Helper()
	require.Zero(t, len(replace)%2, "from, to pairs")
	raw, err := os.ReadFile(filepath.Join("testdata", "hit-points-when", "site.yaml"))
	require.NoError(t, err)
	for i := 0; i < len(replace); i += 2 {
		from, to := replace[i], replace[i+1]
		require.Equal(t, 1, bytes.Count(raw, []byte(from)), "precondition: %q is written once", from)
		raw = bytes.Replace(raw, []byte(from), []byte(to), 1)
	}
	return raw
}

func TestPutTakesARowThatReadsItsCreaturesHitPointsAsANumberAndAShare(t *testing.T) {
	ctx := context.Background()
	world := dungeonstest.WorldStore(t)
	r := joinShelf(t, world, true)

	for _, validateOnly := range []bool{true, false} {
		out, err := r.Put(ctx, &dungeons.PutInput{Caller: alice, Key: hitPointsKey, YAML: hitPointsSite(t), ValidateOnly: validateOnly})
		require.NoError(t, err)
		require.Emptyf(t, out.Errors, "validate-only %v: the site is accepted", validateOnly)
		if !validateOnly {
			require.NotNil(t, out.Entry, "the save stored the site")
			continue
		}
		require.NotNil(t, out.ReadBack)

		share, ok := activateRowStatement(out.ReadBack, hitPointsPath0)
		require.True(t, ok, "a statement for the first row")
		require.Contains(t, share.Sentence, "hit points are at most 30% of its maximum")
		require.Contains(t, share.Sentence, "activates “second wind”")

		number, ok := activateRowStatement(out.ReadBack, hitPointsPath1)
		require.True(t, ok, "a statement for the second row")
		require.Contains(t, number.Sentence, "hit points are at most 5")
		require.NotContains(t, number.Sentence, "of its maximum", "a bare number is hit points, not a share")

		for _, p := range []string{hitPointsPath0, hitPointsPath1, hitPointsPath2, hitPointsPath3} {
			st, found := activateRowStatement(out.ReadBack, p)
			require.Truef(t, found, "a statement at %s", p)
			t.Logf("%s: %s", p, st.Sentence)
		}
	}
}

func TestPutRefusesAHitPointsShapeThatIsNeitherANumberNorAShareAtTheRowAndStoresNothing(t *testing.T) {
	cases := []struct {
		name     string
		replace  []string
		contains []string
	}{
		{"a share of nothing", []string{hitPointsShare, "{ at_most: 0% }"}, []string{"whole percent", "1%", "100%"}},
		{"no hit points", []string{hitPointsAway, "- { when: { hit_points: { at_most: 0 } }, away: enemy }"}, []string{"1 or more"}},
		{"a word", []string{hitPointsShare, "{ at_most: low }"}, []string{"whole number of hit points", "whole percent"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			world := dungeonstest.WorldStore(t)
			r := joinShelf(t, world, true)

			for _, validateOnly := range []bool{true, false} {
				out, err := r.Put(ctx, &dungeons.PutInput{Caller: alice, Key: hitPointsKey, YAML: hitPointsSite(t, tc.replace...), ValidateOnly: validateOnly})
				require.NoErrorf(t, err, "validate-only %v: a refusal is an answer, not an error", validateOnly)
				require.NotEmptyf(t, out.Errors, "validate-only %v", validateOnly)
				for _, want := range tc.contains {
					require.Contains(t, out.Errors[0].Message, want)
				}
				require.Equal(t, hitPointsTableLine, out.Errors[0].Path)
				// The site format refuses the row, so the table is not decoded and the
				// rules then find the recipe's pick has no table: one consequence,
				// said once, and nothing else.
				require.Lenf(t, out.Errors, 2, "validate-only %v: the refusal and its one consequence: %v", validateOnly, out.Errors)
				require.Contains(t, out.Errors[1].Message, `picks absent table "time"`)
				require.Nil(t, out.Entry)
				for _, e := range out.Errors {
					t.Logf("validate-only %v: path %q: %s", validateOnly, e.Path, e.Message)
				}
			}
			_, err := world.Get(ctx, &worlddungeons.GetInput{WorldID: alice.WorldID, Key: hitPointsKey})
			require.ErrorIs(t, err, worlddungeons.ErrNotFound, "nothing was stored")
		})
	}
}
