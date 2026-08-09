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
	"github.com/KirkDiggler/rpg-toolkit/encounter/dungeonspec"
)

// These mock-injected fixtures prove only API seam behavior: lifecycle input,
// verbatim mapping, exact error transport, and nonmutation. They do not prove
// that the unreleased toolkit provider computes ring topology or mechanics.

func waveAOrchestrator(t *testing.T, compiler authoring.Compiler) (*authoring.Orchestrator, *dungeonregistry.Registry, string) {
	t.Helper()
	registry := dungeonregistry.New(nil)
	dir := t.TempDir()
	orch, err := authoring.New(&authoring.Config{
		Registry: registry, ContentDir: dir, PartyStartSeatCount: 4, Compiler: compiler,
	})
	require.NoError(t, err)
	return orch, registry, dir
}

func TestPutDungeon_WaveASeamMapsInjectedRingProjectionVerbatim(t *testing.T) {
	ctrl := gomock.NewController(t)
	compiler := authoringmock.NewMockCompiler(ctrl)
	orch, registry, dir := waveAOrchestrator(t, compiler)
	source := []byte("version: 1\nkey: ring-room\ncanvas: { floor_source: regions }\n")
	entrance := &authoring.FloorPlanCell{Column: 1, Row: 1}
	plan := &authoring.FloorPlan{
		FloorSource: authoring.FloorSourceRegions,
		FloorCells: []authoring.FloorPlanCell{
			{Column: 1, Row: 1}, {Column: 1, Row: 2}, {Column: 1, Row: 3},
			{Column: 2, Row: 1}, {Column: 2, Row: 3},
			{Column: 3, Row: 1}, {Column: 3, Row: 2}, {Column: 3, Row: 3},
		},
		Regions: []authoring.FloorPlanRegion{{
			ID: "ring", Cells: []authoring.FloorPlanCell{
				{Column: 1, Row: 1}, {Column: 1, Row: 2}, {Column: 1, Row: 3},
				{Column: 2, Row: 1}, {Column: 2, Row: 3},
				{Column: 3, Row: 1}, {Column: 3, Row: 2}, {Column: 3, Row: 3},
			},
		}},
		Entrance: entrance,
		Edges: []authoring.FloorPlanEdge{
			// Reversed interior-hole pair and off-canvas pair prove the API
			// neither reorients nor clips provider truth.
			{From: authoring.FloorPlanCell{Column: 2, Row: 2}, To: authoring.FloorPlanCell{Column: 2, Row: 1}, Kind: authoring.FloorPlanEdgeKindSolid},
			{From: authoring.FloorPlanCell{Column: 1, Row: 1}, To: authoring.FloorPlanCell{Column: 0, Row: 1}, Kind: authoring.FloorPlanEdgeKindSolid},
		},
	}
	compiler.EXPECT().CompileDungeon(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *authoring.CompileDungeonInput) (*authoring.CompileDungeonOutput, error) {
			require.Equal(t, source, in.Source)
			require.Equal(t, authoring.CompileModeDraft, in.Mode)
			require.Equal(t, 4, in.PartyStartSeatCount)
			return &authoring.CompileDungeonOutput{FloorPlan: plan}, nil
		})

	out, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{
		Key: "ring-room", YAML: string(source), ValidateOnly: true,
	})
	require.NoError(t, err)
	require.True(t, out.Success)
	require.Same(t, plan, out.FloorPlan)
	require.Equal(t, authoring.FloorSourceRegions, out.FloorPlan.FloorSource)
	require.Equal(t, plan.FloorCells, out.FloorPlan.FloorCells)
	require.Equal(t, plan.Regions, out.FloorPlan.Regions)
	require.Same(t, entrance, out.FloorPlan.Entrance)
	require.Equal(t, plan.Edges, out.FloorPlan.Edges)
	require.Empty(t, registry.Keys())
	entries, readErr := os.ReadDir(dir)
	require.NoError(t, readErr)
	require.Empty(t, entries)
}

func TestPutDungeon_WaveASeamCarriesTinyDraftAndStrictFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	compiler := authoringmock.NewMockCompiler(ctrl)
	orch, registry, dir := waveAOrchestrator(t, compiler)
	source := "version: 1\nkey: tiny-draft\ncanvas: { floor_source: regions }\n"
	tiny := &authoring.FloorPlan{
		FloorSource: authoring.FloorSourceRegions,
		FloorCells:  []authoring.FloorPlanCell{{Column: 1, Row: 1}, {Column: 1, Row: 2}},
		Edges: []authoring.FloorPlanEdge{{
			From: authoring.FloorPlanCell{Column: 1, Row: 1},
			To:   authoring.FloorPlanCell{Column: 0, Row: 1},
			Kind: authoring.FloorPlanEdgeKindSolid,
		}},
	}
	gomock.InOrder(
		compiler.EXPECT().CompileDungeon(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, in *authoring.CompileDungeonInput) (*authoring.CompileDungeonOutput, error) {
				require.Equal(t, authoring.CompileModeDraft, in.Mode)
				return &authoring.CompileDungeonOutput{FloorPlan: tiny}, nil
			}),
		compiler.EXPECT().CompileDungeon(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, in *authoring.CompileDungeonInput) (*authoring.CompileDungeonOutput, error) {
				require.Equal(t, authoring.CompileModeStrict, in.Mode)
				return &authoring.CompileDungeonOutput{FieldErrors: []authoring.FieldError{{
					Field: "canvas.floor_source", Code: "party-cap", Message: "floor has no complete PartyCap seating envelope",
				}}}, nil
			}),
	)

	preview, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{Key: "tiny-draft", YAML: source, ValidateOnly: true})
	require.NoError(t, err)
	require.True(t, preview.Success)
	require.Nil(t, preview.FloorPlan.Entrance, "absent is not synthesized as [0,0]")

	strict, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{Key: "tiny-draft", YAML: source})
	require.NoError(t, err)
	require.False(t, strict.Success)
	require.Equal(t, []authoring.FieldError{{
		Field: "canvas.floor_source", Code: "party-cap", Message: "floor has no complete PartyCap seating envelope",
	}}, strict.FieldErrors)
	require.Empty(t, registry.Keys())
	_, statErr := os.Stat(filepath.Join(dir, "tiny-draft.yaml"))
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestPutDungeon_WaveASeamCarriesDisconnectedDraftAndStrictFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	compiler := authoringmock.NewMockCompiler(ctrl)
	registry := dungeonregistry.New(map[string]dungeonregistry.Entry{
		"two-islands": {Compiled: dungeonspec.CompiledDungeon{}, Name: "Prior"},
	})
	dir := t.TempDir()
	priorSource := []byte("prior authoritative source")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "two-islands.yaml"), priorSource, 0o600))
	orch, err := authoring.New(&authoring.Config{Registry: registry, ContentDir: dir, PartyStartSeatCount: 4, Compiler: compiler})
	require.NoError(t, err)
	source := "version: 1\nkey: two-islands\ncanvas: { floor_source: regions }\n"
	islands := &authoring.FloorPlan{
		FloorSource: authoring.FloorSourceRegions,
		FloorCells: []authoring.FloorPlanCell{
			{Column: 0, Row: 0}, {Column: 0, Row: 1}, {Column: 1, Row: 0}, {Column: 1, Row: 1}, {Column: 4, Row: 4},
		},
		Entrance: &authoring.FloorPlanCell{Column: 0, Row: 0},
	}
	gomock.InOrder(
		compiler.EXPECT().CompileDungeon(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, in *authoring.CompileDungeonInput) (*authoring.CompileDungeonOutput, error) {
				require.Equal(t, authoring.CompileModeDraft, in.Mode)
				return &authoring.CompileDungeonOutput{FloorPlan: islands}, nil
			}),
		compiler.EXPECT().CompileDungeon(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, in *authoring.CompileDungeonInput) (*authoring.CompileDungeonOutput, error) {
				require.Equal(t, authoring.CompileModeStrict, in.Mode)
				return &authoring.CompileDungeonOutput{FieldErrors: []authoring.FieldError{{
					Field: "regions[1].cells", Code: "disconnected-floor", Message: "floor cell is outside the entrance component",
				}}}, nil
			}),
	)

	preview, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{Key: "two-islands", YAML: source, ValidateOnly: true})
	require.NoError(t, err)
	require.True(t, preview.Success)
	require.Equal(t, islands.FloorCells, preview.FloorPlan.FloorCells)

	strict, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{Key: "two-islands", YAML: source})
	require.NoError(t, err)
	require.False(t, strict.Success)
	require.Equal(t, "regions[1].cells", strict.FieldErrors[0].Field)
	afterSource, readErr := os.ReadFile(filepath.Join(dir, "two-islands.yaml"))
	require.NoError(t, readErr)
	require.Equal(t, priorSource, afterSource)
	entry, ok := registry.Get("two-islands")
	require.True(t, ok)
	require.Equal(t, "Prior", entry.Name)
}
