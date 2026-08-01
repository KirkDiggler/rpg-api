package lobby

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/KirkDiggler/rpg-api/internal/dungeonregistry"
)

// TestListDungeons_EmptyRegistry_ReturnsEmptyList is ListDungeons' base
// case (plan.md S1's ListDungeons test list): no entries, no error.
func TestListDungeons_EmptyRegistry_ReturnsEmptyList(t *testing.T) {
	o := &Orchestrator{registry: dungeonregistry.New(nil)}
	out, err := o.ListDungeons(context.Background(), &ListDungeonsInput{})
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Empty(t, out.Dungeons)
}

// TestListDungeons_ExcludesLoadErrorKeys proves a populated registry
// returns summaries for every SUCCESSFULLY COMPILED key only — a
// load-error (disabled) key must never surface in ListDungeons' output,
// matching resolveContentDungeonSpec's own treatment of a broken key as
// FOUND-but-disabled for StartEncounter rather than silently absent:
// ListDungeons still omits it from the player-facing list, since offering
// an unplayable dungeon in a dropdown is worse than a temporarily-shorter
// list.
func TestListDungeons_ExcludesLoadErrorKeys(t *testing.T) {
	o := &Orchestrator{registry: dungeonregistry.New(map[string]dungeonregistry.Entry{
		"reference-tomb": {Name: "The Tomb of the Captain"},
		"fog-lab":        {Name: "Fog Lab"},
		"broken-dungeon": {Err: errors.New("boom: at least 2 rooms required")},
	})}

	out, err := o.ListDungeons(context.Background(), &ListDungeonsInput{})
	require.NoError(t, err)
	require.Equal(t, []dungeonregistry.DungeonSummary{
		{Key: "fog-lab", Name: "Fog Lab"},
		{Key: "reference-tomb", Name: "The Tomb of the Captain"},
	}, out.Dungeons, "sorted by key, disabled key excluded")
}
