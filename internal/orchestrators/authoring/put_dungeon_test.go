package authoring_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/KirkDiggler/rpg-toolkit/encounter/dungeonspec"

	"github.com/KirkDiggler/rpg-api/internal/dungeonregistry"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"
)

// validDungeonYAML is the shared minimal-but-real fixture every "well-
// formed spec" test in this file uses: entrance (width 6) -> boss (width
// 8, height 8 => primary axis 8 > bossAxisMin 6), boss pinned at [4,2]
// (row 2, not doorRow=4; col 4 in [0,8)). key is templated in so each test
// can target its own registry/content-dir key without YAML drift.
func validDungeonYAML(key string) string {
	return fmt.Sprintf(`version: 1
key: %s
name: Test Dungeon
height: 8
rooms:
  - id: entrance
    archetype: entrance
    width: 6
  - id: boss
    archetype: boss
    width: 8
    boss: { ref: "dnd5e:monsters:skeleton-captain", at: [4, 2] }
connectors:
  - { from: entrance, to: boss }
`, key)
}

// invalidDungeonYAML fails dungeonspec.Load (Validate: fewer than the
// minimum 2 rooms) -- a well-formed REQUEST whose CONTENT is the problem,
// distinct from every malformed-request case below.
const invalidDungeonYAML = `version: 1
key: broken
name: Broken
height: 8
rooms:
  - id: only-one
    archetype: entrance
    width: 6
`

// showcaseDungeonYAML is the deterministic three-room Slice #176 fixture.
// It deliberately follows dungeon-content/showcase.yaml's authored region
// shape while staying self-contained as a repository test fixture.
const showcaseDungeonYAML = `version: 1
key: showcase
name: The Shrine Hall
height: 8
rooms:
  - id: antechamber
    archetype: entrance
    width: 6
  - id: shrine
    archetype: chamber
    width: 14
  - id: vault
    archetype: boss
    width: 8
    boss: { ref: "dnd5e:monsters:skeleton-captain", at: [5, 5] }
connectors:
  - { from: antechamber, to: shrine }
  - { from: shrine, to: vault }
`

func newTestOrchestrator(t *testing.T) (*authoring.Orchestrator, *dungeonregistry.Registry, string) {
	t.Helper()
	dir := t.TempDir()
	registry := dungeonregistry.New(nil)
	orch, err := authoring.New(&authoring.Config{
		Registry:            registry,
		ContentDir:          dir,
		PartyStartSeatCount: 4,
	})
	require.NoError(t, err)
	return orch, registry, dir
}

// TestPutDungeon_ProjectsSemanticRegionsFromProviderVerbatim covers the API
// adapter boundary rather than dungeonspec's provider test: the production
// PutDungeon path must preserve declaration order, canonical cells, and
// optional parent presence without interpreting region semantics.
func TestPutDungeon_ProjectsSemanticRegionsFromProviderVerbatim(t *testing.T) {
	const yamlText = `version: 1
key: semantic-regions
name: Semantic Regions
canvas: { width: 3, height: 2 }
rooms: []
regions:
  - id: outer
    cells: [[0,0], [0,1]]
  - id: inner
    cells: [[0,0]]
  - id: empty
    cells: []
`
	orch, _, _ := newTestOrchestrator(t)

	out, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{
		Key: "semantic-regions", YAML: yamlText, ValidateOnly: true,
	})
	require.NoError(t, err)
	require.True(t, out.Success)
	require.NotNil(t, out.FloorPlan)

	regions := out.FloorPlan.Regions
	require.Len(t, regions, 3)
	require.Equal(t, "outer", regions[0].ID)
	require.Equal(t, []authoring.FloorPlanCell{{Column: 0, Row: 0}, {Column: 0, Row: 1}}, regions[0].Cells)
	require.Nil(t, regions[0].ParentID)
	require.Equal(t, "inner", regions[1].ID)
	require.Equal(t, []authoring.FloorPlanCell{{Column: 0, Row: 0}}, regions[1].Cells)
	require.NotNil(t, regions[1].ParentID)
	require.Equal(t, "outer", *regions[1].ParentID)
	require.Equal(t, "empty", regions[2].ID)
	require.Empty(t, regions[2].Cells)
	require.Nil(t, regions[2].ParentID)
}

// --- case 1: ContentDir == "" at New -> construction error ---

func TestNew_ContentDirEmpty_ReturnsConstructionError(t *testing.T) {
	_, err := authoring.New(&authoring.Config{Registry: dungeonregistry.New(nil), ContentDir: ""})
	require.Error(t, err)
}

func TestNew_RegistryNil_ReturnsConstructionError(t *testing.T) {
	_, err := authoring.New(&authoring.Config{Registry: nil, ContentDir: t.TempDir()})
	require.Error(t, err)
}

// --- case 2: key/YAML key: mismatch -> malformed-request result ---

func TestPutDungeon_KeyYAMLMismatch_MalformedRequestNoSideEffects(t *testing.T) {
	orch, registry, dir := newTestOrchestrator(t)

	out, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{
		Key:  "request-key",
		YAML: validDungeonYAML("declared-key-does-not-match"),
	})
	require.Nil(t, out)
	require.Error(t, err)

	var malformed *authoring.MalformedRequestError
	require.ErrorAs(t, err, &malformed, "must be a *MalformedRequestError, not a generic error")

	_, ok := registry.Get("request-key")
	require.False(t, ok, "registry must be untouched")
	_, ok = registry.Get("declared-key-does-not-match")
	require.False(t, ok, "registry must be untouched")

	entries, readErr := os.ReadDir(dir)
	require.NoError(t, readErr)
	require.Empty(t, entries, "no file must be written for a malformed request")
}

// --- case 3: key charset violation -> malformed-request result, rejected
// before any decode/compile ---

func TestPutDungeon_KeyCharsetViolation_MalformedRequestBeforeDecode(t *testing.T) {
	orch, registry, dir := newTestOrchestrator(t)

	out, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{
		Key: "My Dungeon!",
		// Deliberately garbage YAML -- if the charset check didn't run
		// FIRST (before any decode/compile), this would fail with a
		// content error instead of a malformed-request one, and this
		// test would catch that ordering bug.
		YAML: "not: valid: dungeonspec: at: all",
	})
	require.Nil(t, out)
	require.Error(t, err)

	var malformed *authoring.MalformedRequestError
	require.ErrorAs(t, err, &malformed)

	require.Empty(t, registry.Keys())
	entries, readErr := os.ReadDir(dir)
	require.NoError(t, readErr)
	require.Empty(t, entries)
}

// --- case 4: invalid YAML (fails dungeonspec.Load) -> NOT InvalidArgument;
// success=false, one field error, floor_plan unset, no side effects ---

func TestPutDungeon_InvalidYAML_ContentFailureNotMalformedRequest(t *testing.T) {
	orch, registry, dir := newTestOrchestrator(t)

	out, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{
		Key:  "broken",
		YAML: invalidDungeonYAML,
	})
	require.NoError(t, err, "a content-failed compile must not surface as a Go error at all")
	require.NotNil(t, out)

	// Distinguishable in TYPE/SHAPE from case 2/3's malformed-request
	// result: PutDungeon returned (out, nil), not (nil, err) — the type
	// itself (a *PutDungeonOutput, never a *MalformedRequestError) is the
	// proof a naive implementation that collapsed both failure classes
	// into one path would fail here.
	require.False(t, out.Success)
	require.NotEmpty(t, out.FieldError, "exactly one message -- FieldError is a single string, not a slice, by construction")
	require.Nil(t, out.FloorPlan)

	_, ok := registry.Get("broken")
	require.False(t, ok)
	entries, readErr := os.ReadDir(dir)
	require.NoError(t, readErr)
	require.Empty(t, entries)
}

// --- case 5: write-through failure -> RPC fails, registry provably
// unchanged (the ordering test a naive registry-before-write
// implementation fails) ---

func TestPutDungeon_WriteThroughFails_RegistryUntouched(t *testing.T) {
	dir := t.TempDir()
	// Read+execute only, no write -- os.WriteFile into this dir fails
	// with a permission error. Restored before TempDir's own cleanup
	// runs, or removal would fail too.
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	registry := dungeonregistry.New(nil)
	orch, err := authoring.New(&authoring.Config{Registry: registry, ContentDir: dir})
	require.NoError(t, err)

	out, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{
		Key:  "write-fail",
		YAML: validDungeonYAML("write-fail"),
	})
	require.Nil(t, out)
	require.Error(t, err, "an otherwise-valid spec must still fail the RPC when write-through fails")

	var malformed *authoring.MalformedRequestError
	require.False(t, errors.As(err, &malformed), "a write-through failure is a server-side error, not a malformed request")

	_, ok := registry.Get("write-fail")
	require.False(t, ok, "write-then-swap ordering: a write failure must leave the registry untouched")
}

// --- case 6: validate_only=true -> FloorPlan populated (entrance against
// a KNOWN fixture value, not just non-zero), nothing persisted ---

// TestPutDungeon_OmittedAndNullStartKeepToolkitGeneratedAnchor proves the
// optional start field keeps its specified null semantics at the API boundary:
// omitted and explicit null reach the toolkit as the same generated-anchor
// request, rather than an API-generated coordinate.
func TestPutDungeon_OmittedAndNullStartKeepToolkitGeneratedAnchor(t *testing.T) {
	orch, _, _ := newTestOrchestrator(t)

	omitted, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{
		Key:          "start-omitted",
		YAML:         validDungeonYAML("start-omitted"),
		ValidateOnly: true,
	})
	require.NoError(t, err)

	nullYAML := strings.Replace(validDungeonYAML("start-null"), "height: 8\n", "height: 8\nstart: null\n", 1)
	nullStart, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{
		Key:          "start-null",
		YAML:         nullYAML,
		ValidateOnly: true,
	})
	require.NoError(t, err)

	require.True(t, omitted.Success)
	require.True(t, nullStart.Success)
	require.Equal(t, omitted.FloorPlan.Entrance, nullStart.FloorPlan.Entrance,
		"omitted and null start must preserve the same toolkit-resolved generated anchor")
	require.Equal(t, &authoring.FloorPlanCell{Column: 0, Row: 4}, omitted.FloorPlan.Entrance)
}

// TestPutDungeon_AuthoredDoorRowStartUsesToolkitAnchorAndFourSeatConfig proves
// that an authored absolute start is valid on a semantic room's door row, is
// reflected verbatim in FloorPlan.entrance, and is compiled with the normal
// product's four-seat reservation. The companion invalid placed-prop request
// confirms the door-row exception belongs to start only and is surfaced from
// toolkit validation rather than recreated here.
func TestPutDungeon_AuthoredDoorRowStartUsesToolkitAnchorAndFourSeatConfig(t *testing.T) {
	orch, registry, _ := newTestOrchestrator(t)
	const key = "authored-door-row-start"
	yaml := strings.Replace(validDungeonYAML(key), "height: 8\n", "height: 8\nstart: [12, 4]\n", 1)

	out, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{Key: key, YAML: yaml})
	require.NoError(t, err)
	require.True(t, out.Success)
	require.Equal(t, &authoring.FloorPlanCell{Column: 12, Row: 4}, out.FloorPlan.Entrance,
		"FloorPlan.entrance is the toolkit-resolved authored anchor, not an entrance-room assumption")

	_, ok := registry.Get(key)
	require.True(t, ok)

	blockedYAML := strings.Replace(validDungeonYAML("door-row-prop"), "width: 6\n", "width: 6\n    place:\n      - { ref: \"dnd5e:props:pillar\", at: [1, 4] }\n", 1)
	blocked, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{
		Key:          "door-row-prop",
		YAML:         blockedYAML,
		ValidateOnly: true,
	})
	require.NoError(t, err)
	require.False(t, blocked.Success)
	require.Contains(t, blocked.FieldError, "reserved row",
		"ordinary placement legality must be surfaced from toolkit validation")
}

func TestPutDungeon_ValidateOnly_PopulatesFloorPlanWithoutPersisting(t *testing.T) {
	orch, registry, dir := newTestOrchestrator(t)

	out, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{
		Key:          "preview-key",
		YAML:         validDungeonYAML("preview-key"),
		ValidateOnly: true,
	})
	require.NoError(t, err)
	require.NotNil(t, out)
	require.True(t, out.Success)
	require.Empty(t, out.FieldError)
	require.NotNil(t, out.FloorPlan)

	fp := out.FloorPlan
	require.Equal(t, 8, fp.Height)
	require.Equal(t, 4, fp.DoorRow, "height/2")
	require.Len(t, fp.Rooms, 2)
	require.Equal(t, authoring.FloorPlanRoom{ID: "entrance", Archetype: "entrance", Width: 6, StartColumn: 0}, fp.Rooms[0])
	require.Equal(t, authoring.FloorPlanRoom{ID: "boss", Archetype: "boss", Width: 8, StartColumn: 7}, fp.Rooms[1])
	require.Len(t, fp.Connectors, 1)
	require.Equal(t, "preview-key-door-entrance-boss", fp.Connectors[0].DoorID)
	require.False(t, fp.Connectors[0].Locked)
	require.Equal(t, "entrance", fp.Connectors[0].FromRoomID)
	require.Equal(t, "boss", fp.Connectors[0].ToRoomID)
	require.Equal(t, 6, fp.Connectors[0].Column)

	// Entrance is a KNOWN fixture value ([0,0] is both a plausible real
	// entrance AND the proto/struct zero value, so a bug leaving it unset
	// would pass a weaker "non-zero" assertion) -- region 0 (entrance)
	// always starts at column 0 in this compiler, and the entrance cell
	// sits at that region's near edge, doorRow (verified against
	// rpg-toolkit's dungeon.go generateDungeonLayout: entrance is always
	// offset-coordinate {X: 0, Y: doorRow}).
	require.Equal(t, &authoring.FloorPlanCell{Column: 0, Row: 4}, fp.Entrance)

	_, ok := registry.Get("preview-key")
	require.False(t, ok, "validate_only must not mutate the registry")
	entries, readErr := os.ReadDir(dir)
	require.NoError(t, readErr)
	require.Empty(t, entries, "validate_only must not write a file")
}

// --- case 7: validate_only=false -> file written (new key: <key>.yaml;
// existing key: the ORIGINATING file), registry updated, and a second Put
// for the SAME key overwrites rather than shadowing (file count must not
// grow) ---

func TestPutDungeon_ShowcaseProjectsToolkitGeneratedEdgesVerbatim(t *testing.T) {
	orch, _, _ := newTestOrchestrator(t)

	out, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{
		Key:          "showcase",
		YAML:         showcaseDungeonYAML,
		ValidateOnly: true,
	})
	require.NoError(t, err)
	require.True(t, out.Success)

	expected := buildProviderFloorPlan(t, showcaseDungeonYAML)
	require.Equal(t, expected, out.FloorPlan,
		"the API must project every toolkit-produced field in provider order")
	require.Len(t, out.FloorPlan.Edges, 196, "toolkit physical-edge count")
	require.Contains(t, out.FloorPlan.Edges, authoring.FloorPlanEdge{
		From: authoring.FloorPlanCell{Column: 6, Row: 4}, To: authoring.FloorPlanCell{Column: 7, Row: 4},
		Kind: authoring.FloorPlanEdgeKindDoor, DoorID: "showcase-door-antechamber-shrine",
	}, "provider connector door fact must survive the API mapping")
}

// authoredEdgesDungeonYAML uses rpg-project#179's corrected pointy-top odd-q
// specimen pairs. The former [7,1]-[8,0] and [7,3]-[8,2] serializer pairs are
// deliberately not used: they are non-adjacent under the one canonical
// coordinate convention and must remain toolkit field errors.
func authoredEdgesDungeonYAML(key string) string {
	return strings.Replace(validDungeonYAML(key), "connectors:\n", `walls:
  - { from: [7, 1], to: [8, 1], kind: solid }
  - { from: [7, 3], to: [8, 3], kind: door }
connectors:
`, 1)
}

// TestPutDungeon_ProjectsToolkitCombinedGeneratedAndAuthoredEdgesVerbatim
// compares the API response to BuildFloorPlan's complete canonical projection.
// It also pins representative generated and authored facts without sorting or
// normalizing either list in the consumer test.
func TestPutDungeon_ProjectsToolkitCombinedGeneratedAndAuthoredEdgesVerbatim(t *testing.T) {
	const key = "combined-authored-edges"
	yaml := authoredEdgesDungeonYAML(key)
	expected := buildProviderFloorPlan(t, yaml)

	orch, _, _ := newTestOrchestrator(t)
	out, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{
		Key:          key,
		YAML:         yaml,
		ValidateOnly: true,
	})
	require.NoError(t, err)
	require.True(t, out.Success)
	require.Equal(t, expected, out.FloorPlan,
		"FloorPlan must match the toolkit's complete canonical projection")

	var authoredSolid, authoredDoor *authoring.FloorPlanEdge
	for index := range out.FloorPlan.Edges {
		edge := &out.FloorPlan.Edges[index]
		if edge.From == (authoring.FloorPlanCell{Column: 7, Row: 1}) && edge.To == (authoring.FloorPlanCell{Column: 8, Row: 1}) {
			authoredSolid = edge
		}
		if edge.From == (authoring.FloorPlanCell{Column: 7, Row: 3}) && edge.To == (authoring.FloorPlanCell{Column: 8, Row: 3}) {
			authoredDoor = edge
		}
	}
	require.NotNil(t, authoredSolid, "canonical authored solid must be present")
	require.Equal(t, authoring.FloorPlanEdgeKindSolid, authoredSolid.Kind)
	require.Empty(t, authoredSolid.DoorID)
	require.NotNil(t, authoredDoor, "canonical authored door must be present")
	require.Equal(t, authoring.FloorPlanEdgeKindDoor, authoredDoor.Kind)
	require.Equal(t, "combined-authored-edges-authored-door-7--7-0--8--7--1", authoredDoor.DoorID)
}

// TestPutDungeon_AuthoredDoorIDIsStableAcrossReversedYAML keeps identity at
// the toolkit boundary: reversed author input names the same undirected edge,
// and rpg-api forwards the one stable toolkit door ID rather than deriving or
// rewriting it.
func TestPutDungeon_AuthoredDoorIDIsStableAcrossReversedYAML(t *testing.T) {
	const key = "stable-authored-door"
	forward := authoredEdgesDungeonYAML(key)
	reversed := strings.Replace(forward,
		"- { from: [7, 3], to: [8, 3], kind: door }",
		"- { from: [8, 3], to: [7, 3], kind: door }", 1)

	orch, _, _ := newTestOrchestrator(t)
	forwardOut, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{Key: key, YAML: forward, ValidateOnly: true})
	require.NoError(t, err)
	reversedOut, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{Key: key, YAML: reversed, ValidateOnly: true})
	require.NoError(t, err)

	forwardDoorID := authoredDoorID(t, forwardOut.FloorPlan.Edges)
	reversedDoorID := authoredDoorID(t, reversedOut.FloorPlan.Edges)
	require.Equal(t, forwardDoorID, reversedDoorID,
		"reversing an authored edge must not change its stable toolkit ID")
}

// TestPutDungeon_AuthoredWallFieldErrorsPassThroughToolkitVerbatim protects
// the author-feedback boundary: rpg-api returns the toolkit's field-specific
// validation text without replacing it with API geometry judgment.
func TestPutDungeon_AuthoredWallFieldErrorsPassThroughToolkitVerbatim(t *testing.T) {
	const key = "bad-odd-q-authored-edge"
	yaml := strings.Replace(validDungeonYAML(key), "connectors:\n", `walls:
  - { from: [7, 1], to: [8, 0], kind: solid }
connectors:
`, 1)
	_, toolkitErr := dungeonspec.LoadWithConfig([]byte(yaml), dungeonspec.LoadConfig{PartyStartSeatCount: 4})
	require.Error(t, toolkitErr)
	require.ErrorContains(t, toolkitErr, "walls[0]: endpoints must be adjacent pointy-top odd-q floor hexes")

	orch, registry, dir := newTestOrchestrator(t)
	out, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{Key: key, YAML: yaml, ValidateOnly: true})
	require.NoError(t, err)
	require.False(t, out.Success)
	require.Nil(t, out.FloorPlan)
	require.Equal(t, toolkitErr.Error(), out.FieldError)
	require.Empty(t, registry.Keys())
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries)
}

// TestPutDungeon_OmittedAndNullWallsRemainGeneratedOnly pairs with the #176
// literal generated-edge snapshot immediately above: that snapshot pins the
// old sequence, while this test proves omitted and null walls keep the same
// generated-only behavior. No authored edge or door identity may be invented
// by the API.
func TestPutDungeon_OmittedAndNullWallsRemainGeneratedOnly(t *testing.T) {
	orch, _, _ := newTestOrchestrator(t)
	const key = "walls-omitted"
	omitted, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{
		Key:          key,
		YAML:         validDungeonYAML(key),
		ValidateOnly: true,
	})
	require.NoError(t, err)
	nullWallsYAML := strings.Replace(validDungeonYAML(key), "connectors:\n", "walls: null\nconnectors:\n", 1)
	nullWalls, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{
		Key:          key,
		YAML:         nullWallsYAML,
		ValidateOnly: true,
	})
	require.NoError(t, err)

	require.True(t, omitted.Success)
	require.True(t, nullWalls.Success)
	require.Equal(t, omitted.FloorPlan.Edges, nullWalls.FloorPlan.Edges)
	for _, edge := range omitted.FloorPlan.Edges {
		require.NotContains(t, edge.DoorID, "authored-door")
	}
}

func authoredDoorID(t *testing.T, edges []authoring.FloorPlanEdge) string {
	t.Helper()
	for _, edge := range edges {
		if strings.Contains(edge.DoorID, "-authored-door-") {
			return edge.DoorID
		}
	}
	t.Fatal("combined FloorPlan edges had no authored door")
	return ""
}

func buildProviderFloorPlan(t *testing.T, yaml string) *authoring.FloorPlan {
	t.Helper()
	compiled, err := dungeonspec.LoadWithConfig([]byte(yaml), dungeonspec.LoadConfig{PartyStartSeatCount: 4})
	require.NoError(t, err)
	providerPlan, err := dungeonspec.BuildFloorPlan(context.Background(), dungeonspec.BuildFloorPlanInput{Compiled: compiled, Seed: 1})
	require.NoError(t, err)
	plan := &authoring.FloorPlan{
		FloorSource: authoring.FloorSourceBounds,
		Rooms:       make([]authoring.FloorPlanRoom, len(providerPlan.Rooms)), Connectors: make([]authoring.FloorPlanConnector, len(providerPlan.Connectors)),
		Width: providerPlan.Width, Height: providerPlan.Height, DoorRow: providerPlan.DoorRow,
		FloorCells: make([]authoring.FloorPlanCell, len(providerPlan.FloorCells)),
		Entrance:   &authoring.FloorPlanCell{Column: providerPlan.Entrance.Column, Row: providerPlan.Entrance.Row},
		Edges:      make([]authoring.FloorPlanEdge, len(providerPlan.Edges)),
	}
	for index, room := range providerPlan.Rooms {
		plan.Rooms[index] = authoring.FloorPlanRoom{ID: room.ID, Archetype: room.Archetype, Width: room.Width, StartColumn: room.StartColumn}
	}
	for index, connector := range providerPlan.Connectors {
		plan.Connectors[index] = authoring.FloorPlanConnector{DoorID: connector.DoorID, Locked: connector.Locked, FromRoomID: connector.FromRoomID, ToRoomID: connector.ToRoomID, Column: connector.Column}
	}
	for index, cell := range providerPlan.FloorCells {
		plan.FloorCells[index] = authoring.FloorPlanCell{Column: cell.Column, Row: cell.Row}
	}
	for index, edge := range providerPlan.Edges {
		plan.Edges[index] = authoring.FloorPlanEdge{From: authoring.FloorPlanCell{Column: edge.From.Column, Row: edge.From.Row}, To: authoring.FloorPlanCell{Column: edge.To.Column, Row: edge.To.Row}, Kind: authoring.FloorPlanEdgeKind(edge.Kind), DoorID: edge.DoorID}
	}
	return plan
}

func TestPutDungeon_Persists_NewKeyWritesKeyYAML(t *testing.T) {
	orch, registry, dir := newTestOrchestrator(t)

	out, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{
		Key:  "persist-key",
		YAML: validDungeonYAML("persist-key"),
	})
	require.NoError(t, err)
	require.True(t, out.Success)
	require.NotNil(t, out.FloorPlan)

	entry, ok := registry.Get("persist-key")
	require.True(t, ok)
	require.NoError(t, entry.Err)
	require.Equal(t, "Test Dungeon", entry.Name)

	raw, readErr := os.ReadFile(filepath.Join(dir, "persist-key.yaml"))
	require.NoError(t, readErr)
	require.Equal(t, validDungeonYAML("persist-key"), string(raw))
}

func TestPutDungeon_SecondPutSameKey_OverwritesOriginatingFile_FileCountStable(t *testing.T) {
	orch, registry, dir := newTestOrchestrator(t)

	_, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{
		Key:  "overwrite-key",
		YAML: validDungeonYAML("overwrite-key"),
	})
	require.NoError(t, err)

	entriesAfterFirst, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entriesAfterFirst, 1)

	// A DIFFERENT valid YAML for the SAME key -- same 2-room shape (still
	// passes Validate) but a different declared name, so the overwrite is
	// observable in the registry, not just file content.
	secondYAML := `version: 1
key: overwrite-key
name: Renamed Dungeon
height: 8
rooms:
  - id: entrance
    archetype: entrance
    width: 6
  - id: boss
    archetype: boss
    width: 8
    boss: { ref: "dnd5e:monsters:skeleton-captain", at: [4, 2] }
connectors:
  - { from: entrance, to: boss }
`
	out, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{
		Key:  "overwrite-key",
		YAML: secondYAML,
	})
	require.NoError(t, err)
	require.True(t, out.Success)

	entriesAfterSecond, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entriesAfterSecond, 1, "overwriting the same key must not grow the file count (shadow-file prevention)")
	require.Equal(t, entriesAfterFirst[0].Name(), entriesAfterSecond[0].Name(), "must overwrite the SAME originating file")

	entry, ok := registry.Get("overwrite-key")
	require.True(t, ok)
	require.Equal(t, "Renamed Dungeon", entry.Name, "registry must reflect the SECOND put, not the first")
}

// --- case 8: no-restart visibility -- PutDungeon succeeds, then a direct
// Registry.Get (standing in for the next StartEncounter) sees the new
// compiled spec, same process, no restart ---

func TestPutDungeon_NoRestartVisibility_RegistryGetSeesNewSpecImmediately(t *testing.T) {
	orch, registry, _ := newTestOrchestrator(t)

	_, ok := registry.Get("live-key")
	require.False(t, ok, "sanity: key must not pre-exist")

	out, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{
		Key:  "live-key",
		YAML: validDungeonYAML("live-key"),
	})
	require.NoError(t, err)
	require.True(t, out.Success)

	entry, ok := registry.Get("live-key")
	require.True(t, ok, "the SAME registry pointer must see the new spec with no restart")
	require.NoError(t, entry.Err)
	require.Equal(t, buildProviderFloorPlan(t, validDungeonYAML("live-key")), out.FloorPlan)
}
