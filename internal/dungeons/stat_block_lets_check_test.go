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

// THE CONTENT CHECK ON A STAT BLOCK THAT SAYS WHAT ITS CREATURE CAN USE
// (rpg-project#1131, slice 2e): a stat block's `lets` is taken by the site
// format, the read-back and the rules; the rules refuse a stat block they
// cannot run when the site is checked or saved, at `templates.<id>.lets`, and
// nothing is stored.

const (
	statBlockLetsKey  = "lieutenant-holds-second-wind-walk"
	statBlockLetsPath = "templates.gate-lieutenant.lets"
)

// statBlockLetsSite is the fixture with one piece of its text replaced; the
// text must occur exactly once so a change to the fixture cannot make the
// replacement a no-op.
func statBlockLetsSite(t *testing.T, from, to string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "stat-block-lets", "site.yaml"))
	require.NoError(t, err)
	if from == "" {
		return raw
	}
	require.Equal(t, 1, bytes.Count(raw, []byte(from)), "precondition: %q is written once", from)
	return bytes.Replace(raw, []byte(from), []byte(to), 1)
}

func statBlockLetsStatements(rb *readback.ReadBack, kind readback.Kind) []readback.Statement {
	var out []readback.Statement
	for _, sec := range rb.Sections {
		for _, g := range sec.Groups {
			for _, st := range g.Statements {
				if st.Kind == kind {
					out = append(out, st)
				}
			}
		}
	}
	return out
}

func TestPutTakesAStatBlockThatSaysWhatItsCreatureCanUse(t *testing.T) {
	ctx := context.Background()
	world := dungeonstest.WorldStore(t)
	r := joinShelf(t, world, true)

	for _, validateOnly := range []bool{true, false} {
		out, err := r.Put(ctx, &dungeons.PutInput{Caller: alice, Key: statBlockLetsKey, YAML: statBlockLetsSite(t, "", ""), ValidateOnly: validateOnly})
		require.NoError(t, err)
		require.Emptyf(t, out.Errors, "validate-only %v: the site is accepted", validateOnly)
		if validateOnly {
			// A save carries no read-back (PutResult.ReadBack): the studio holds
			// the check's own.
			require.NotNil(t, out.ReadBack)
			got := statBlockLetsStatements(out.ReadBack, readback.KindTemplateLets)
			require.Len(t, got, 1, "one statement of what the stat block can use")
			require.Equal(t, statBlockLetsPath, got[0].Path)
			t.Logf("read-back: %s", got[0].Sentence)
		} else {
			require.NotNil(t, out.Entry, "the save stored the site")
		}
	}
}

// The site format judges the shape and the rules judge the words: a `lets`
// that is not a list is refused by the format at the entry, and the rules do
// not say a second sentence about it.
func TestPutRefusesALetsThatIsNotAListAtTheEntry(t *testing.T) {
	ctx := context.Background()
	world := dungeonstest.WorldStore(t)
	r := joinShelf(t, world, true)

	raw, err := os.ReadFile(filepath.Join("testdata", "stat-block-lets", "site.yaml"))
	require.NoError(t, err)
	list := "    lets:\n      - use: second wind\n        with: { level: 1 }\n        uses: { max: 1, refills: { on: [short rest, long rest] } }\n"
	require.Equal(t, 1, bytes.Count(raw, []byte(list)), "precondition: the lets list is written once")
	yaml := bytes.Replace(raw, []byte(list), []byte("    lets: second wind\n"), 1)

	for _, validateOnly := range []bool{true, false} {
		out, putErr := r.Put(ctx, &dungeons.PutInput{Caller: alice, Key: statBlockLetsKey, YAML: yaml, ValidateOnly: validateOnly})
		require.NoErrorf(t, putErr, "validate-only %v", validateOnly)
		require.Nil(t, out.Entry)
		var atEntry []dungeons.FieldError
		for _, e := range out.Errors {
			if e.Path == statBlockLetsPath {
				atEntry = append(atEntry, e)
			}
		}
		require.Lenf(t, atEntry, 1, "validate-only %v: exactly one error at the entry, from the site format: %v", validateOnly, out.Errors)
		require.Contains(t, atEntry[0].Message, "must be a list")
		for _, e := range out.Errors {
			t.Logf("validate-only %v: %q: %s", validateOnly, e.Path, e.Message)
		}
	}
	_, err = world.Get(ctx, &worlddungeons.GetInput{WorldID: alice.WorldID, Key: statBlockLetsKey})
	require.ErrorIs(t, err, worlddungeons.ErrNotFound, "nothing was stored")
}
