package sessionworld

// The workshop suite compiles the shared single-room v3 fixture (the same
// file the registry and launch suites play, internal/dungeons/testdata) and
// pins what rpg-api's thin adoption owes it: the file's own identity, the
// authored axial cells — including the NEGATIVE ODD ROW — the complete scene
// riding the field, the declared placement, and the refusals. This package
// performs exactly one geometry conversion (cellOf); these tests are what
// prove it once, at the seam, rather than a second algorithm.

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/stretchr/testify/require"

	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	tkdungeonspec "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter/dungeonspec"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

// workshopFixturePath is the shared fixture, read rather than embedded: the
// registry and launch suites play these bytes and so must this package, or a
// "fixture" nobody plays could drift.
var workshopFixturePath = filepath.Join("..", "dungeons", "testdata", "workshop-room.yaml")

type WorkshopSuite struct {
	suite.Suite

	raw     []byte
	dungeon *Dungeon
}

func TestWorkshopSuite(t *testing.T) { suite.Run(t, new(WorkshopSuite)) }

func (s *WorkshopSuite) SetupTest() {
	raw, err := os.ReadFile(workshopFixturePath)
	s.Require().NoError(err, "the shared workshop fixture must exist")
	dungeon, err := Compile(raw)
	s.Require().NoError(err, "the authored room must compile")
	s.raw = raw
	s.dungeon = dungeon
}

// TestTheFileNamesItself pins the metadata rpg-api reports: the root key and
// the VISUAL SCENE's name are the provider's published identity.
func (s *WorkshopSuite) TestTheFileNamesItself() {
	s.Equal("workshop-room", s.dungeon.Key)
	s.Equal("Workshop", s.dungeon.Name)
}

// TestThePartyAndTheGarrisonStandWhereTheyWereAuthored pins the one
// conversion this package performs: authored axial cells in, dungeon-absolute
// axial cells out, including the negative odd row. The party seats are
// derived — the authored cell first, then the region's other free cells.
func (s *WorkshopSuite) TestThePartyAndTheGarrisonStandWhereTheyWereAuthored() {
	s.Require().NotEmpty(s.dungeon.PartySeats)
	s.Equal(spatial.Position{X: 0, Y: 0}, s.dungeon.PartySeats[0],
		"the authored party start is the first seat")
	s.Len(s.dungeon.PartySeats, 6,
		"eight authored hexes minus the two monster cells, nearest first")

	byID := make(map[string]Monster, len(s.dungeon.Monsters))
	for _, m := range s.dungeon.Monsters {
		byID[m.MemberID] = m
	}
	s.Require().Len(byID, 2, "each authored placement keeps its own stable id")
	s.Equal("dnd5e:monsters:skeleton", byID["skeleton-a"].Ref)
	s.Equal(spatial.Position{X: 2, Y: 0}, byID["skeleton-a"].At, "the authored cell, axial")

	// THE NEGATIVE ODD ROW: authored axial (q=1, r=-1) crosses the one
	// offset conversion and the member stands on exactly that cell. An
	// even-only conversion would land it one column off.
	s.Equal(spatial.Position{X: 1, Y: -1}, byID["skeleton-b"].At,
		"authored axial (q=1, r=-1) arrives as itself — a negative odd row survives cellOf")
}

// TestTheSceneRidesTheFieldWhole pins the retention half of the adoption:
// the compiled field carries the complete presentation the author wrote —
// frame and workspace declarations, the grouped raised prop with its height
// scale, the supported lit decor, the fractional/negative source transforms —
// as doubles, untouched. Gameplay truth beside it: the declared table
// placement with both blocking answers.
func (s *WorkshopSuite) TestTheSceneRidesTheFieldWhole() {
	scene := s.dungeon.World.Field.RoomScene
	s.Require().NotNil(scene, "a v3 room's scene rides the field")
	s.Equal(1, scene.Version)
	s.Equal("world-xz", scene.Frame.HorizontalPlane)
	s.Equal("world-y-up", scene.Frame.VerticalAxis)
	s.Equal("world-scene-unit", scene.Frame.DistanceUnit)
	s.Equal(float64(1), scene.Frame.HexRadius)
	s.Equal("owner-local-xz", scene.Frame.FootprintFrame)
	s.Equal(float64(6), scene.Workspace.HexRadius)
	s.Equal(float64(12), scene.Workspace.HorizontalLimit)

	s.Equal("scene-1", scene.Scene.ID)
	s.Equal("Workshop", scene.Scene.Name)
	s.Require().Len(scene.Scene.Items, 2)
	table := scene.Scene.Items[0]
	s.Equal("table", table.ID)
	s.Equal(tkencounter.RoomSceneKindProp, table.Kind)
	s.Equal("dnd5e:props:torture-table", table.AssetRef)
	s.InDelta(-2.25, table.Transform.X, 1e-9, "fractional negative transform, a double")
	s.InDelta(1.3, table.Transform.Z, 1e-9)
	s.InDelta(0.37, table.Transform.RotationY, 1e-9)
	s.Require().NotNil(table.HeightScale)
	s.InDelta(1.5, *table.HeightScale, 1e-9, "the raised prop's height scale rides")
	s.Equal("furniture", table.ParentID, "the raised prop is grouped")

	candles := scene.Scene.Items[1]
	s.Equal("candles", candles.ID)
	s.Equal("table", candles.SupportID, "the decor is supported by the prop")
	s.Require().NotNil(candles.PointLight)
	s.True(candles.PointLight.Enabled)
	s.Equal("#ff9d52", candles.PointLight.Color)
	s.InDelta(1.1, candles.PointLight.Intensity, 1e-9)
	s.InDelta(2.6, candles.PointLight.Range, 1e-9)
	s.InDelta(0.5, candles.PointLight.Offset.Y, 1e-9)

	s.Require().Len(scene.Scene.Groups, 1)
	s.Equal("furniture", scene.Scene.Groups[0].ID)
	s.InDelta(-2.175, scene.Scene.Groups[0].Transform.X, 1e-9)

	s.Require().Len(s.dungeon.World.Field.Placed, 1, "one declared placement")
	placed := s.dungeon.World.Field.Placed[0]
	s.Equal(tkencounter.PropID("table"), placed.ID)
	s.True(placed.BlocksMovement, "the table is walked around")
	s.False(placed.BlocksLineOfSight, "and seen over, as declared")
}

// TestTheImplicitRegionIsOneBrightCryptRoom pins the compiled presentation
// facts the atlas serves: one implicit region under the authored id, crypt
// archetype, bright lighting.
func (s *WorkshopSuite) TestTheImplicitRegionIsOneBrightCryptRoom() {
	s.Require().Len(s.dungeon.World.Field.Regions, 1)
	region := s.dungeon.World.Field.Regions[0]
	s.Equal("room-1-region", region.ID)
	s.Equal("crypt", region.Archetype)
	s.Require().NotNil(region.Lighting)
	s.Require().NotNil(region.Lighting.Intensity)
	s.InDelta(1.0, *region.Lighting.Intensity, 1e-9, "bright is intensity 1")
}

// TestALegacyV2DungeonKeepsNoRoomScene pins compatibility the other way: the
// shipped tomb predates room scenes, compiles exactly as before, and its
// world carries NO room scene — absence stays absence on every carrier.
func TestALegacyV2DungeonKeepsNoRoomScene(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "content", "reference-tomb.yaml"))
	require.NoError(t, err, "the shipped tomb must exist")
	dungeon, err := Compile(raw)
	require.NoError(t, err, "the shipped tomb must still compile")
	require.Nil(t, dungeon.World.Field.RoomScene,
		"legacy v2 content has no room scene, and none is invented")
	require.Empty(t, dungeon.World.Field.Placed, "and no placed contributors either")
}

// TestAnUnspellableStandingIsRefusedByName pins the strict decode: a play
// value this build does not support is a validation error naming the path —
// never a fallback to a guessed default or a partial world. The variant is
// built from the shared fixture by prefix surgery so the provider-owned
// contract literal lives in exactly one place (the fixture file).
func (s *WorkshopSuite) TestAnUnspellableStandingIsRefusedByName() {
	bad := bytes.Replace(s.raw, []byte("standing: cent"), []byte("standing: center"), 1)
	s.Require().NotEqual(s.raw, bad, "the fixture's play line must be where this test expects it")

	_, err := Compile(bad)
	s.Require().Error(err)
	s.ErrorIs(err, tkdungeonspec.ErrBadSpec, "a bad spec is refused as a bad spec")
	var verr *tkdungeonspec.ValidationError
	s.Require().ErrorAs(err, &verr)
	s.Require().NotEmpty(verr.Errors)
	s.Equal("play.standing", verr.Errors[0].Path, "the refusal names the play field")
}

// TestAnUnknownMonsterRefIsRefusedAtCompile pins the preflight: content
// resolution happens BEFORE acceptance, so a placement naming a ref the
// rulebook does not have fails the compile with a document-level validation
// error — and nothing half-built escapes.
func (s *WorkshopSuite) TestAnUnknownMonsterRefIsRefusedAtCompile() {
	bad := bytes.Replace(s.raw, []byte("dnd5e:monsters:skeleton"), []byte("dnd5e:monsters:not-real"), 1)
	s.Require().NotEqual(s.raw, bad)

	_, err := Compile(bad)
	s.Require().Error(err)
	s.ErrorIs(err, tkdungeonspec.ErrBadSpec)
	var verr *tkdungeonspec.ValidationError
	s.Require().ErrorAs(err, &verr)
	s.Require().NotEmpty(verr.Errors)
	s.Contains(verr.Errors[0].Message, "references unknown monster",
		"the refusal names the placement and the ref")
}
