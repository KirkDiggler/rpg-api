package authoring_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"
)

func canvasUpdateYAML(key, body string) string {
	return "version: 1\nkey: " + key + "\nname: Canvas update\nheight: 1\ncanvas: { width: 4, height: 2 }\nrooms: []\n" + body
}

func TestPutDungeon_CompleteCandidateMayDeletePriorContentAndShrink(t *testing.T) {
	const key = "complete-candidate-shrink"
	previous := canvasUpdateYAML(key, `place:
  - { ref: dnd5e:props:pillar, at: [3, 0] }
walls:
  - { from: [2, 0], to: [2, 1], kind: solid }
start: [0, 1]
`)
	orch, registry, dir := newTestOrchestrator(t)
	first, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{Key: key, YAML: previous})
	require.NoError(t, err)
	require.True(t, first.Success)

	// The complete replacement explicitly deletes the old placement, wall, and
	// start while shrinking the canvas. No prior compiled occupancy is an input;
	// the candidate succeeds because its own complete source is valid.
	candidate := "version: 1\nkey: " + key + "\nname: Shrunk complete candidate\nheight: 1\ncanvas: { width: 3, height: 2 }\nrooms: []\n"
	out, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{Key: key, YAML: candidate})
	require.NoError(t, err)
	require.True(t, out.Success)
	require.Equal(t, 3, out.FloorPlan.Width)
	require.Empty(t, out.FloorPlan.Edges)

	afterDisk, err := os.ReadFile(filepath.Join(dir, key+".yaml"))
	require.NoError(t, err)
	require.Equal(t, candidate, string(afterDisk))
	afterEntry, ok := registry.Get(key)
	require.True(t, ok)
	require.Equal(t, "Shrunk complete candidate", afterEntry.Name)
}

func TestPutDungeon_ValidateOnlyAndGrowthRefreshRegistryAndReload(t *testing.T) {
	const key = "canvas-reload"
	initial := canvasUpdateYAML(key, `start: [1, 1]
place:
  - { ref: dnd5e:props:altar, at: [1, 0], facing: W }
  - { ref: dnd5e:monsters:skeleton, at: [2, 0] }
walls:
  - { from: [1, 0], to: [1, 1], kind: door }
`)
	orch, registry, dir := newTestOrchestrator(t)
	first, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{Key: key, YAML: initial})
	require.NoError(t, err)
	require.True(t, first.Success)
	beforeDisk, err := os.ReadFile(filepath.Join(dir, key+".yaml"))
	require.NoError(t, err)
	beforeEntry, ok := registry.Get(key)
	require.True(t, ok)

	preview := strings.Replace(initial, "width: 4, height: 2", "width: 5, height: 3", 1)
	previewOut, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{Key: key, YAML: preview, ValidateOnly: true})
	require.NoError(t, err)
	require.True(t, previewOut.Success)
	afterPreviewDisk, err := os.ReadFile(filepath.Join(dir, key+".yaml"))
	require.NoError(t, err)
	require.Equal(t, beforeDisk, afterPreviewDisk)
	afterPreviewEntry, ok := registry.Get(key)
	require.True(t, ok)
	require.Equal(t, beforeEntry, afterPreviewEntry)

	grown, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{Key: key, YAML: preview})
	require.NoError(t, err)
	require.True(t, grown.Success)
	require.Equal(t, buildProviderFloorPlan(t, preview), grown.FloorPlan)

	// Production restart/load/runtime evidence belongs in lobby's
	// TestStartEncounter_CanvasPutDungeonSurvivesProductionReload: it starts
	// from this write-through source via LoadContentRegistry, rather than
	// reconstructing a registry or compiling the bytes in this package.
}
