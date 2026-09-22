package sessionworld

// The workshop suite compiles the shared single-room v3 fixture (the same
// file the registry and launch suites play, internal/dungeons/testdata) and
// pins what rpg-api's thin adoption owes it: the file's own identity, the
// authored axial cells — including the NEGATIVE ODD ROW — the three numbers
// per prop the lowering takes out of the authored scene, the declared
// placement, and the refusals. This package
// performs exactly one geometry conversion (cellOf); these tests are what
// prove it once, at the seam, rather than a second algorithm.

import (
	"bytes"
	"math"
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

// sqrt3 is the one irrational the source-to-canonical scale uses: an editor
// hex of circumradius 1 is sqrt(3) editor units across the flats, which is
// exactly one cell's FeetPerCell.
const sqrt3 = 1.7320508075688772935274463415058723669428052538102380135

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

// TestTheThreeNumbersPerPropReachTheField pins what the engine actually
// takes from the World Builder's scene now that it no longer carries it
// (rpg-project#479): for each DECLARED prop, the authored transform's x, z
// and rotationY, joined to the gameplay block's own footprint and its two
// blocking answers. Everything else the author wrote — asset refs, labels,
// parent groups, height scales, the candles' point light, the frame's axis
// words — is content the engine never reads, no longer carries and no longer
// judges. The candles are in the scene and NOT here, because nothing
// declared them.
//
// Expectations are the AUTHORED numbers times the one documented scale, not
// constants copied out of an output: k = FeetPerCell/sqrt(3) feet per source
// unit, origin = (x*k, z*k), facing = -rotationY in degrees, and the name
// swap (spatial's D lies along the facing, so the declared width becomes D
// and the depth becomes W). A regression names which term moved.
func (s *WorkshopSuite) TestTheThreeNumbersPerPropReachTheField() {
	const k = tkencounter.FeetPerCell / sqrt3

	s.Require().Len(s.dungeon.World.Field.Placed, 1,
		"one DECLARED prop: the undeclared candles are dressing, never inferred blocking")
	placed := s.dungeon.World.Field.Placed[0]
	s.Equal(tkencounter.PropID("table"), placed.ID)

	// The three numbers, from `transform: {x: -2.25, z: 1.3, rotationY: 0.37}`.
	s.InDelta(-2.25*k, placed.Placement.Origin.X, 1e-9, "authored x, scaled")
	s.InDelta(1.3*k, placed.Placement.Origin.Y, 1e-9, "authored z becomes the plane's Y")
	s.InDelta(-0.37*180/math.Pi, placed.Placement.Facing, 1e-9,
		"a source yaw arrives as the opposite plane angle")

	// The footprint, from the GAMEPLAY block's own declaration.
	s.InDelta(1.2*k, placed.Placement.Footprint.D, 1e-9, "the declared width lies along the facing")
	s.InDelta(0.5*k, placed.Placement.Footprint.W, 1e-9, "and the declared depth across it")
	s.InDelta(0.1*k, placed.Placement.LocalOffset.X, 1e-9)
	s.InDelta(-0.2*k, placed.Placement.LocalOffset.Y, 1e-9)

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

// TestALegacyV2DungeonPlacesNoProps pins compatibility the other way: the
// shipped tomb is v2, has no scene to lower and no prop declarations,
// compiles exactly as before, and contributes NOTHING to the field's placed
// props — absence stays absence rather than becoming a zero-sized box at the
// origin.
func TestALegacyV2DungeonPlacesNoProps(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "content", "reference-tomb.yaml"))
	require.NoError(t, err, "the shipped tomb must exist")
	dungeon, err := Compile(raw)
	require.NoError(t, err, "the shipped tomb must still compile")
	require.Empty(t, dungeon.World.Field.Placed, "a v2 dungeon declares no placed contributors")
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
