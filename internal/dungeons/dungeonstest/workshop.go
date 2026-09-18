package dungeonstest

import (
	"os"
	"path/filepath"
	"testing"

	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
)

// The single-room v3 fixtures (internal/dungeons/testdata): shared YAML, one
// copy, so no Go test file carries the authored source as a raw string. The
// `play.standing` value these rooms declare is the provider-owned contract
// literal and is deliberately NOT respelled here.
const (
	// WorkshopRoomKey is the full room's own `key:` line: the grouped raised
	// prop, the supported lit decor, the explicit declarations, two placed
	// skeletons (one on a negative odd axial row) and the party start.
	WorkshopRoomKey = "workshop-room"

	// WorkshopOneSeatKey is the one-seat variant's own `key:` line: the same
	// room reduced to a single walkable hex, so the launch's seat-capacity
	// gate has exactly one seat.
	WorkshopOneSeatKey = "workshop-one-seat"
)

// WorkshopRoomYAML is the full authored room, read from the shared fixture
// so every suite plays the same bytes.
func WorkshopRoomYAML(t testing.TB) []byte {
	t.Helper()
	return roomFixture(t, WorkshopRoomKey)
}

// WorkshopOneSeatYAML is the one-seat variant, read from the same fixture
// directory.
func WorkshopOneSeatYAML(t testing.TB) []byte {
	t.Helper()
	return roomFixture(t, WorkshopOneSeatKey)
}

// roomFixture reads one testdata room by the key its file is named for.
// The directory is anchored on the repo root through the content tree
// (ContentDir's own walk), not on the caller's cwd: a suite in ANY package
// runs this helper with its own working directory, and a relative "../"
// from there names whatever happens to sit beside that package.
func roomFixture(t testing.TB, key string) []byte {
	t.Helper()

	root := filepath.Dir(ContentDir(t))
	path := filepath.Clean(filepath.Join(root, "internal", "dungeons", "testdata", key+".yaml"))
	raw, err := os.ReadFile(path) //nolint:gosec // path is the repo's own testdata dir joined with a named constant key
	if err != nil {
		t.Fatalf("dungeonstest: read the shared v3 fixture %s: %v", key, err)
	}

	return raw
}

// WorkshopRoomScene is the FULL source graph the workshop fixture authors,
// spelled here once as an independent expected value: not decoded out of any
// converter's output, so a test that compares an atlas against it proves the
// whole scene crossed, value by value — the grouped raised table (with its
// height scale and parent group), the supported lit candles (support, light
// offset/color/intensity/range), the fractional/negative transforms, and the
// frame and workspace declarations. Doubles stay doubles.
func WorkshopRoomScene() tkencounter.RoomScenePresentation {
	const furnitureGroupID = "furniture"
	heightScale := 1.5
	return tkencounter.RoomScenePresentation{
		Version: 1,
		Frame: tkencounter.RoomSceneFrame{
			HorizontalPlane: "world-xz",
			VerticalAxis:    "world-y-up",
			DistanceUnit:    "world-scene-unit",
			HexRadius:       1,
			FootprintFrame:  "owner-local-xz",
		},
		Workspace: tkencounter.RoomSceneWorkspace{HexRadius: 6, HorizontalLimit: 12},
		Scene: tkencounter.RoomVisualScene{
			Version: 1, ID: "scene-1", Name: "Workshop",
			Items: []tkencounter.RoomSceneItem{
				{
					Kind:     tkencounter.RoomSceneKindProp,
					ID:       "table",
					Label:    "Table",
					AssetRef: "dnd5e:props:torture-table",
					Transform: tkencounter.RoomSceneTransform{
						X: -2.25, Y: 0, Z: 1.3, RotationY: 0.37,
					},
					HeightScale: &heightScale,
					ParentID:    furnitureGroupID,
				},
				{
					Kind:     tkencounter.RoomSceneKindProp,
					ID:       "candles",
					Label:    "Candles",
					AssetRef: "dnd5e:props:candles",
					Transform: tkencounter.RoomSceneTransform{
						X: -2.1, Y: 1.2, Z: 1.25, RotationY: 0.37,
					},
					ParentID:  furnitureGroupID,
					SupportID: "table",
					PointLight: &tkencounter.RoomSceneLight{
						Enabled:   true,
						Offset:    tkencounter.RoomSceneOffset{X: 0, Y: 0.5, Z: 0},
						Color:     "#ff9d52",
						Intensity: 1.1,
						Range:     2.6,
					},
				},
			},
			Groups: []tkencounter.RoomSceneGroup{
				{
					Kind:  tkencounter.RoomSceneKindGroup,
					ID:    "furniture",
					Label: "Furniture",
					Transform: tkencounter.RoomSceneTransform{
						X: -2.175, Y: 0.6, Z: 1.275, RotationY: 0.37,
					},
				},
			},
		},
	}
}
