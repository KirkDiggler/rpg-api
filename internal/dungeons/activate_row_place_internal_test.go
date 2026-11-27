package dungeons

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
)

// THE PLACING OF A REFUSED ACTIVATION (rpg-project#1131, slice 3d): the
// session's refusal names a word and no place; contentRefusals places it at the
// rows that say it, and when no row can be found, or the read-back cannot read
// the file, it still reports the refusal, once, on no path.

func activateRowCompiled(t *testing.T) (d *sessionworld.Dungeon, good, bad []byte) {
	t.Helper()
	good, err := os.ReadFile(filepath.Join("testdata", "activate-row", "site.yaml"))
	require.NoError(t, err)
	const from = "- { activate: second wind }"
	require.Equal(t, 1, bytes.Count(good, []byte(from)), "precondition: the row is written once")
	bad = bytes.Replace(good, []byte(from), []byte("- { activate: secnd wind }"), 1)
	d, err = sessionworld.Compile(bad)
	require.NoError(t, err)
	return d, good, bad
}

func TestARefusedActivationWithNoPlaceIsStillReportedOnNoPath(t *testing.T) {
	d, good, bad := activateRowCompiled(t)

	// The reference: the file that says the word is placed at its row.
	placed, err := contentRefusals(d.Spec, bad)
	require.NoError(t, err)
	require.Len(t, placed, 1)
	require.Equal(t, "behaviorDefinitions.lieutenant-policy.table.time[0]", placed[0].Path)
	message := placed[0].Message

	cases := []struct {
		name string
		raw  []byte
	}{
		// the read-back reads the file and no statement connects to the word:
		// the file as saved never writes `secnd wind`.
		{"no statement connects to the word", good},
		// the read-back cannot read the bytes: it errors, and no panic escapes.
		{"bytes the read-back cannot read", []byte("not: [a site")},
		{"no bytes at all", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got []FieldError
			require.NotPanics(t, func() {
				var gotErr error
				got, gotErr = contentRefusals(d.Spec, c.raw)
				require.NoError(t, gotErr)
			})
			require.Len(t, got, 1, "the refusal is reported once, never dropped")
			require.Empty(t, got[0].Path)
			require.Equal(t, message, got[0].Message)
			t.Logf("%q: %s", got[0].Path, got[0].Message)
		})
	}
}
