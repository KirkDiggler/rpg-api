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
	require.Equal(t, authoring.FloorPlanCell{Column: 0, Row: 4}, omitted.FloorPlan.Entrance)
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
	require.Equal(t, authoring.FloorPlanCell{Column: 12, Row: 4}, out.FloorPlan.Entrance,
		"FloorPlan.entrance is the toolkit-resolved authored anchor, not an entrance-room assumption")

	entry, ok := registry.Get(key)
	require.True(t, ok)
	require.NotNil(t, entry.Compiled.Params.PartyStart.Anchor)
	require.Equal(t, 4, entry.Compiled.Params.PartyStart.SeatCount,
		"authoring must compile against the normal product party capacity")

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
	require.Equal(t, authoring.FloorPlanCell{Column: 0, Row: 4}, fp.Entrance)

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

	expected := expectedShowcaseGeneratedEdges(t)
	require.Len(t, expected, 196, "fixture must list the full canonical sequence")
	require.Len(t, out.FloorPlan.Edges, 196, "toolkit physical-edge count")
	require.Equal(t, expected, out.FloorPlan.Edges,
		"the API must project the complete toolkit-produced physical-edge sequence verbatim")
}

// expectedShowcaseGeneratedEdges is a literal, ordered snapshot of every
// physical edge produced by rpg-toolkit PR #876 at 7b58d3f for the deterministic
// showcase spec. Each record includes both endpoints, kind, and door identity;
// equality above deliberately catches missing, extra, reordered, or changed
// edges rather than merely proving representative examples exist.
func expectedShowcaseGeneratedEdges(t *testing.T) []authoring.FloorPlanEdge {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("testdata", "showcase_generated_edges.txt"))
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	edges := make([]authoring.FloorPlanEdge, 0, len(lines))
	for _, line := range lines {
		fields := strings.Split(line, ":")
		require.Len(t, fields, 4, "expected edge record %q", line)

		var from, to authoring.FloorPlanCell
		_, err = fmt.Sscanf(fields[0], "%d,%d", &from.Column, &from.Row)
		require.NoError(t, err, "expected edge source %q", line)
		_, err = fmt.Sscanf(fields[1], "%d,%d", &to.Column, &to.Row)
		require.NoError(t, err, "expected edge destination %q", line)

		edges = append(edges, authoring.FloorPlanEdge{
			From:   from,
			To:     to,
			Kind:   authoring.FloorPlanEdgeKind(fields[2]),
			DoorID: fields[3],
		})
	}
	return edges
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
	require.Len(t, entry.Compiled.Params.Regions, 2)
}
