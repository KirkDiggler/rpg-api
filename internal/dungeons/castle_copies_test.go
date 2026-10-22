package dungeons_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
)

// encounterModule is the toolkit module whose dungeonspec testdata holds the
// castle its route tests play.
const encounterModule = "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"

// TestCastleCopiesAgree is THE QA CASTLE, ONE CASTLE (rpg-project#639): the
// shipped the-mad-king and the toolkit's castle — the copy the toolkit's
// `TestMadKingScript*` route tests play, read from the encounter module this
// one pins — are the same document apart from comments. A content change
// lands in the toolkit's copy first, under its tests, and is copied here; a
// shipped file that drifts from what was tested fails here.
func TestCastleCopiesAgree(t *testing.T) {
	out, err := exec.CommandContext(context.Background(), "go", "list", "-m", "-f", "{{.Dir}}", encounterModule).Output()
	require.NoError(t, err, "the encounter module is pinned")
	toolkit, err := os.ReadFile(filepath.Join(strings.TrimSpace(string(out)),
		"dungeonspec", "testdata", "world-builder-v4-castle.yaml"))
	require.NoError(t, err)
	shipped, err := os.ReadFile(filepath.Join(dungeonstest.ContentDir(t), "the-mad-king.yaml"))
	require.NoError(t, err)

	// Decoded, so the comments — which differ, because the toolkit's name its
	// own tests — are not compared and every value is.
	var want, got any
	require.NoError(t, yaml.Unmarshal(toolkit, &want))
	require.NoError(t, yaml.Unmarshal(shipped, &got))
	require.Equal(t, want, got, "content/the-mad-king.yaml is the toolkit's castle apart from comments")
}
