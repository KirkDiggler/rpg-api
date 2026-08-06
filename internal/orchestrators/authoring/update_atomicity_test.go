package authoring_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/KirkDiggler/rpg-api/internal/dungeonregistry"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"
	"github.com/KirkDiggler/rpg-toolkit/encounter/dungeonspec"
)

func canvasUpdateYAML(key, body string) string {
	return "version: 1\nkey: " + key + "\nname: Canvas update\nheight: 1\ncanvas: { width: 4, height: 2 }\nrooms: []\n" + body
}

func TestPutDungeon_CanvasShrinkFailuresAreAtomic(t *testing.T) {
	cases := []struct {
		name, previousBody, candidate string
		wantError                     string
	}{
		{"placement", "place:\n  - { ref: dnd5e:props:pillar, at: [3, 0] }\n", "canvas: { width: 3, height: 2 }\nrooms: []\n", "place[0]"},
		{"wall endpoint", "walls:\n  - { from: [2, 0], to: [2, 1], kind: solid }\n", "canvas: { width: 4, height: 1 }\nrooms: []\n", "walls[0]"},
		{"start", "start: [0, 1]\n", "canvas: { width: 4, height: 1 }\nrooms: []\n", "start"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			orch, registry, dir := newTestOrchestrator(t)
			key := "atomic-" + strings.ReplaceAll(test.name, " ", "-")
			previous := canvasUpdateYAML(key, test.previousBody)
			first, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{Key: key, YAML: previous})
			require.NoError(t, err)
			require.True(t, first.Success)
			beforeDisk, err := os.ReadFile(filepath.Join(dir, key+".yaml"))
			require.NoError(t, err)
			beforeEntry, ok := registry.Get(key)
			require.True(t, ok)

			candidate := "version: 1\nkey: " + key + "\nname: Canvas update\nheight: 1\n" + test.candidate
			out, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{Key: key, YAML: candidate})
			require.NoError(t, err)
			require.False(t, out.Success)
			require.Contains(t, out.FieldError, test.wantError)
			afterDisk, err := os.ReadFile(filepath.Join(dir, key+".yaml"))
			require.NoError(t, err)
			require.Equal(t, beforeDisk, afterDisk)
			afterEntry, ok := registry.Get(key)
			require.True(t, ok)
			require.Equal(t, beforeEntry, afterEntry)
		})
	}
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

	// Simulate process restart: disk is the source; a new registry and a new
	// orchestrator receive a freshly compiled opaque toolkit value.
	raw, err := os.ReadFile(filepath.Join(dir, key+".yaml"))
	require.NoError(t, err)
	require.Equal(t, preview, string(raw))
	compiled, err := dungeonspec.LoadWithConfig(raw, dungeonspec.LoadConfig{PartyStartSeatCount: 4})
	require.NoError(t, err)
	freshRegistry := dungeonregistry.New(map[string]dungeonregistry.Entry{key: {Compiled: compiled, Name: "Canvas update"}})
	fresh, err := authoring.New(&authoring.Config{Registry: freshRegistry, ContentDir: dir, PartyStartSeatCount: 4})
	require.NoError(t, err)
	reloaded, err := fresh.PutDungeon(context.Background(), &authoring.PutDungeonInput{Key: key, YAML: preview, ValidateOnly: true})
	require.NoError(t, err)
	require.True(t, reloaded.Success)
	require.Equal(t, grown.FloorPlan, reloaded.FloorPlan)
	require.Contains(t, string(raw), "dnd5e:props:altar")
	require.Contains(t, string(raw), "facing: W")
	require.Contains(t, string(raw), "dnd5e:monsters:skeleton")
	require.Contains(t, string(raw), "start: [1, 1]")
	require.Contains(t, reloaded.FloorPlan.Edges, authoring.FloorPlanEdge{
		From: authoring.FloorPlanCell{Column: 1, Row: 0}, To: authoring.FloorPlanCell{Column: 1, Row: 1},
		Kind: authoring.FloorPlanEdgeKindDoor, DoorID: "canvas-reload-authored-door-1--2-1--1--1-0",
	})
}
