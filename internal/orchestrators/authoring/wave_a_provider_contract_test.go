package authoring_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/dungeonregistry"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"
	authoringmock "github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring/mock"
)

const waveARingYAML = `version: 1
key: ring-room
name: Ring Room
canvas: { width: 5, height: 5, floor_source: regions }
rooms: []
regions:
  - id: ring
    cells: [[1,1], [1,2], [1,3], [2,1], [2,3], [3,1], [3,2], [3,3]]
`

func waveAOrchestrator(t *testing.T) (*authoring.Orchestrator, *dungeonregistry.Registry, string) {
	t.Helper()
	registry := dungeonregistry.New(nil)
	dir := t.TempDir()
	orch, err := authoring.New(&authoring.Config{
		Registry: registry, ContentDir: dir, PartyStartSeatCount: 4,
	})
	require.NoError(t, err)
	return orch, registry, dir
}

func TestPutDungeon_ValidateOnlyUsesDraftCompilerAndHasNoSideEffects(t *testing.T) {
	ctrl := gomock.NewController(t)
	compiler := authoringmock.NewMockCompiler(ctrl)
	registry := dungeonregistry.New(nil)
	dir := t.TempDir()
	orch, err := authoring.New(&authoring.Config{
		Registry: registry, ContentDir: dir, PartyStartSeatCount: 4, Compiler: compiler,
	})
	require.NoError(t, err)

	const source = "version: 1\nkey: draft-seam\n"
	compiler.EXPECT().CompileDungeon(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *authoring.CompileDungeonInput) (*authoring.CompileDungeonOutput, error) {
			require.Equal(t, []byte(source), in.Source)
			require.Equal(t, authoring.CompileModeDraft, in.Mode)
			require.Equal(t, 4, in.PartyStartSeatCount)
			return &authoring.CompileDungeonOutput{FloorPlan: &authoring.FloorPlan{
				FloorSource: authoring.FloorSourceRegions,
				FloorCells:  []authoring.FloorPlanCell{{Column: 0, Row: 0}, {Column: 4, Row: 4}},
				Entrance:    nil,
			}}, nil
		},
	)

	out, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{
		Key: "draft-seam", YAML: source, ValidateOnly: true,
	})
	require.NoError(t, err)
	require.True(t, out.Success)
	require.Nil(t, out.FloorPlan.Entrance, "draft nil entrance must remain absent")
	require.Len(t, out.FloorPlan.FloorCells, 2, "draft may return a structurally valid non-runnable mask")
	require.Empty(t, registry.Keys(), "validate-only must not swap the registry")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries, "validate-only must not write source")
}

func TestPutDungeon_WriteUsesStrictCompilerAndPreservesPreviousStateOnFieldErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	compiler := authoringmock.NewMockCompiler(ctrl)
	registry := dungeonregistry.New(nil)
	dir := t.TempDir()
	const (
		key            = "strict-seam"
		previousSource = "version: 1\nkey: strict-seam\nname: Previous\n"
		candidate      = "version: 1\nkey: strict-seam\nname: Candidate\n"
	)
	require.NoError(t, os.WriteFile(filepath.Join(dir, key+".yaml"), []byte(previousSource), 0o600))
	registry.Put(key, dungeonregistry.Entry{Name: "Previous"})
	orch, err := authoring.New(&authoring.Config{
		Registry: registry, ContentDir: dir, PartyStartSeatCount: 4, Compiler: compiler,
	})
	require.NoError(t, err)

	providerErrors := []authoring.FieldError{{
		Field: "regions[1].cells", Message: "provider message stays opaque", Code: "duplicate_region",
	}}
	compiler.EXPECT().CompileDungeon(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *authoring.CompileDungeonInput) (*authoring.CompileDungeonOutput, error) {
			require.Equal(t, []byte(candidate), in.Source)
			require.Equal(t, authoring.CompileModeStrict, in.Mode)
			return &authoring.CompileDungeonOutput{FieldErrors: providerErrors}, nil
		},
	)

	out, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{Key: key, YAML: candidate})
	require.NoError(t, err)
	require.False(t, out.Success)
	require.Equal(t, providerErrors, out.FieldErrors, "field/message/code must pass through without reinterpretation")
	committed, err := os.ReadFile(filepath.Join(dir, key+".yaml"))
	require.NoError(t, err)
	require.Equal(t, previousSource, string(committed), "strict failure must preserve prior source")
	entry, ok := registry.Get(key)
	require.True(t, ok)
	require.Equal(t, "Previous", entry.Name, "strict failure must preserve prior registry entry")
}

func TestPutDungeon_WaveARealProviderProjectsRingAndCompleteEnvelope(t *testing.T) {
	orch, registry, dir := waveAOrchestrator(t)
	out, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{
		Key: "ring-room", YAML: waveARingYAML, ValidateOnly: true,
	})
	require.NoError(t, err)
	require.True(t, out.Success)
	require.Equal(t, authoring.FloorSourceRegions, out.FloorPlan.FloorSource)
	require.Equal(t, 5, out.FloorPlan.Width)
	require.Equal(t, 5, out.FloorPlan.Height)
	require.Equal(t, []authoring.FloorPlanCell{
		{Column: 1, Row: 1}, {Column: 1, Row: 2}, {Column: 1, Row: 3},
		{Column: 2, Row: 1}, {Column: 2, Row: 3},
		{Column: 3, Row: 1}, {Column: 3, Row: 2}, {Column: 3, Row: 3},
	}, out.FloorPlan.FloorCells)
	require.Equal(t, &authoring.FloorPlanCell{Column: 1, Row: 1}, out.FloorPlan.Entrance)
	require.Equal(t, out.FloorPlan.FloorCells, out.FloorPlan.Regions[0].Cells)
	require.Len(t, out.FloorPlan.Edges, 28)

	center := authoring.FloorPlanCell{Column: 2, Row: 2}
	centerEdges := 0
	for _, edge := range out.FloorPlan.Edges {
		if edge.From == center || edge.To == center {
			centerEdges++
		}
		require.Equal(t, authoring.FloorPlanEdgeKindSolid, edge.Kind)
	}
	require.Equal(t, 6, centerEdges, "the provider's complete hole envelope must pass through unchanged")
	require.Empty(t, registry.Keys())
	entries, readErr := os.ReadDir(dir)
	require.NoError(t, readErr)
	require.Empty(t, entries)
}

func TestPutDungeon_WaveARealProviderPreservesOffCanvasEndpoints(t *testing.T) {
	orch, _, _ := waveAOrchestrator(t)
	const source = `version: 1
key: rim
name: Rim
canvas: { width: 2, height: 2, floor_source: regions }
rooms: []
regions: [{ id: rim, cells: [[0,0]] }]
`
	out, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{Key: "rim", YAML: source, ValidateOnly: true})
	require.NoError(t, err)
	require.True(t, out.Success)
	require.Len(t, out.FloorPlan.Edges, 6)
	offCanvas := 0
	for _, edge := range out.FloorPlan.Edges {
		for _, endpoint := range []authoring.FloorPlanCell{edge.From, edge.To} {
			if endpoint.Column < 0 || endpoint.Row < 0 || endpoint.Column >= 2 || endpoint.Row >= 2 {
				offCanvas++
			}
		}
	}
	require.Positive(t, offCanvas)
}

func TestPutDungeon_WaveARealProviderDraftAndStrictLifecycle(t *testing.T) {
	orch, registry, dir := waveAOrchestrator(t)
	const tiny = `version: 1
key: tiny-draft
name: Tiny Draft
canvas: { width: 3, height: 2, floor_source: regions }
rooms: []
regions: [{ id: tiny, cells: [[0,0], [1,0]] }]
`
	preview, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{Key: "tiny-draft", YAML: tiny, ValidateOnly: true})
	require.NoError(t, err)
	require.True(t, preview.Success)
	require.Nil(t, preview.FloorPlan.Entrance, "nil entrance must not become [0,0]")

	strict, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{Key: "tiny-draft", YAML: tiny})
	require.NoError(t, err)
	require.False(t, strict.Success)
	require.Equal(t, []authoring.FieldError{{
		Field: "canvas.floor_source", Message: "no floor anchor has a complete same-component party start envelope", Code: "entrance_unavailable",
	}}, strict.FieldErrors)
	require.Empty(t, registry.Keys())
	_, statErr := os.Stat(filepath.Join(dir, "tiny-draft.yaml"))
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestPutDungeon_WaveARealProviderDisconnectedDraftCannotPersistStrict(t *testing.T) {
	orch, registry, dir := waveAOrchestrator(t)
	const source = `version: 1
key: two-islands
name: Two Islands
canvas: { width: 6, height: 3, floor_source: regions }
rooms: []
regions:
  - { id: large, cells: [[0,0], [0,1], [1,0], [1,1]] }
  - { id: island, cells: [[5,2]] }
`
	preview, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{Key: "two-islands", YAML: source, ValidateOnly: true})
	require.NoError(t, err)
	require.True(t, preview.Success)
	require.Len(t, preview.FloorPlan.FloorCells, 5)

	strict, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{Key: "two-islands", YAML: source})
	require.NoError(t, err)
	require.False(t, strict.Success)
	require.Equal(t, []authoring.FieldError{{
		Field: "canvas.floor_source", Message: "region-union floor must be connected in strict mode", Code: "floor_disconnected",
	}}, strict.FieldErrors)
	require.Empty(t, registry.Keys())
	_, statErr := os.Stat(filepath.Join(dir, "two-islands.yaml"))
	require.ErrorIs(t, statErr, os.ErrNotExist)

	// Removing the disconnected island from the complete candidate succeeds.
	// API never supplies the failed candidate or prior compiled state back to
	// the provider, so this is an ordinary strict standalone replacement.
	const connected = `version: 1
key: two-islands
name: One Connected Island
canvas: { width: 6, height: 3, floor_source: regions }
rooms: []
regions:
  - { id: large, cells: [[0,0], [0,1], [1,0], [1,1]] }
`
	repaired, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{Key: "two-islands", YAML: connected})
	require.NoError(t, err)
	require.True(t, repaired.Success)
	require.Len(t, repaired.FloorPlan.FloorCells, 4)
	require.Equal(t, &authoring.FloorPlanCell{Column: 0, Row: 0}, repaired.FloorPlan.Entrance)
	require.FileExists(t, filepath.Join(dir, "two-islands.yaml"))
	entry, ok := registry.Get("two-islands")
	require.True(t, ok)
	require.NoError(t, entry.Err)
	require.Equal(t, "One Connected Island", entry.Name)
}

func TestPutDungeon_WaveARealProviderCarriesExactIndexedValidationPaths(t *testing.T) {
	orch, _, _ := waveAOrchestrator(t)
	const duplicateEmpty = `version: 1
key: exact-path
name: Exact Path
canvas: { width: 2, height: 2, floor_source: regions }
rooms: []
regions:
  - { id: first, cells: [] }
  - { id: second, cells: [] }
`
	out, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{
		Key: "exact-path", YAML: duplicateEmpty, ValidateOnly: true,
	})
	require.NoError(t, err)
	require.False(t, out.Success)
	require.Len(t, out.FieldErrors, 1)
	require.Equal(t, "regions[1].cells", out.FieldErrors[0].Field)
	require.Equal(t, "duplicate_region", out.FieldErrors[0].Code)
}

func TestPutDungeon_WaveARealProviderPersistsCompleteStrictCandidate(t *testing.T) {
	orch, registry, dir := waveAOrchestrator(t)
	out, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{Key: "ring-room", YAML: waveARingYAML})
	require.NoError(t, err)
	require.True(t, out.Success)
	require.Equal(t, authoring.FloorSourceRegions, out.FloorPlan.FloorSource)
	committed, readErr := os.ReadFile(filepath.Join(dir, "ring-room.yaml"))
	require.NoError(t, readErr)
	require.Equal(t, waveARingYAML, string(committed))
	entry, ok := registry.Get("ring-room")
	require.True(t, ok)
	require.NoError(t, entry.Err)
	require.Equal(t, "Ring Room", entry.Name)
}
