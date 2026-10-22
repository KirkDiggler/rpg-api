package sessionv1alpha1

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/proto"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

// Non-default values distinguish a missing field, unit conversion, and an
// incorrect dungeon-prefix reconstruction from transparent SDK projection.
func structuralAtlasFixture() sdk.Atlas {
	return sdk.Atlas{
		DungeonKey: "not-the-door-prefix",
		StructuralWalls: []sdk.AtlasStructuralWall{{
			ID: "wall-a", Ref: "content:stone-wall",
			From: sdk.FootprintPoint{X: 13.25, Y: -7.5}, To: sdk.FootprintPoint{X: 21.75, Y: 4.125},
			Height: 8.75, Thickness: 0.66, Elevation: -1.25,
			Openings: []sdk.AtlasStructuralOpening{{ID: "cut-a", Position: 4.5, Width: 2.75}},
			// Deliberately not in coordinate order: the session hands them in
			// line order, and a converter that sorted would reorder them.
			FootprintCells: []spatial.Position{{X: 14, Y: -7}, {X: 13, Y: -5}, {X: 21, Y: 4}},
		}},
		StructuralDoors: []sdk.AtlasStructuralDoor{{
			ID: "keep/gate", Ref: "content:gate",
			From: sdk.FootprintPoint{X: 15, Y: -5}, To: sdk.FootprintPoint{X: 16.5, Y: -2.75},
			Height: 7.5, Thickness: 0.5, Elevation: -0.25,
		}},
	}
}

func expectedStructuralWall() *sessionpb.AtlasStructuralWall {
	return &sessionpb.AtlasStructuralWall{
		Id: "wall-a", Ref: "content:stone-wall",
		From: &sessionpb.FootprintPoint{X: 13.25, Y: -7.5}, To: &sessionpb.FootprintPoint{X: 21.75, Y: 4.125},
		Height: 8.75, Thickness: 0.66, Elevation: -1.25,
		Openings:       []*sessionpb.AtlasStructuralOpening{{Id: "cut-a", Position: 4.5, Width: 2.75}},
		FootprintCells: []*sessionpb.Position{{X: 14, Y: -7}, {X: 13, Y: -5}, {X: 21, Y: 4}},
	}
}

func expectedStructuralDoor() *sessionpb.AtlasStructuralDoor {
	return &sessionpb.AtlasStructuralDoor{
		Id: "keep/gate", Ref: "content:gate",
		From: &sessionpb.FootprintPoint{X: 15, Y: -5}, To: &sessionpb.FootprintPoint{X: 16.5, Y: -2.75},
		Height: 7.5, Thickness: 0.5, Elevation: -0.25,
	}
}

type StructuralConversionSuite struct{ suite.Suite }

func TestStructuralConversionSuite(t *testing.T) { suite.Run(t, new(StructuralConversionSuite)) }

func (s *StructuralConversionSuite) checkRows(walls []*sessionpb.AtlasStructuralWall, doors []*sessionpb.AtlasStructuralDoor) {
	s.Require().Len(walls, 1)
	s.Require().Len(doors, 1)
	s.True(proto.Equal(expectedStructuralWall(), walls[0]), "wall row: %v", walls[0])
	s.True(proto.Equal(expectedStructuralDoor(), doors[0]), "door row: %v", doors[0])
}

func (s *StructuralConversionSuite) TestSnapshotCarriesAllFieldsWithoutChangingUnitsOrIdentity() {
	in := structuralAtlasFixture()
	out := AtlasToProto(&in)
	s.checkRows(out.StructuralWalls, out.StructuralDoors)
	s.Empty(out.RoomSceneJson)
	// Document ownership/input preservation. Current cross-type allocation and
	// value-field copies already preclude pointer aliasing; this is not an
	// independent mutation test of an alias-prone implementation.
	out.StructuralWalls[0].From.X = 999
	out.StructuralWalls[0].Openings[0].Width = 999
	out.StructuralDoors[0].To.Y = 999
	s.Equal(13.25, in.StructuralWalls[0].From.X)
	s.Equal(2.75, in.StructuralWalls[0].Openings[0].Width)
	s.Equal(-2.75, in.StructuralDoors[0].To.Y)
}

// The footing hexes are what a client grades a wall's stretches by (fog of
// war R9), so they cross in the session's line order, cell for cell. The
// same converter serves the atlas and every reveal beat, which checkRows
// covers through the expected row.
func (s *StructuralConversionSuite) TestWallCarriesItsFootprintCellsInLineOrder() {
	in := structuralAtlasFixture()
	got := AtlasToProto(&in).StructuralWalls[0].GetFootprintCells()
	for i, want := range [][2]float64{{14, -7}, {13, -5}, {21, 4}} {
		s.Require().Greaterf(len(got), i, "footprint cell %d is missing", i)
		s.Equal(want, [2]float64{got[i].GetX(), got[i].GetY()}, "footprint cell %d", i)
	}
}

func (s *StructuralConversionSuite) TestTheHostDoesNotRebuildAWithheldCutOrParent() {
	in := structuralAtlasFixture()
	in.StructuralWalls[0].Openings = nil
	in.StructuralDoors = nil
	masked := AtlasToProto(&in)
	s.Require().Len(masked.StructuralWalls, 1)
	s.Empty(masked.StructuralWalls[0].Openings)
	s.Empty(masked.StructuralDoors)
	in = structuralAtlasFixture()
	in.StructuralWalls = nil
	independent := AtlasToProto(&in)
	s.Empty(independent.StructuralWalls)
	s.Require().Len(independent.StructuralDoors, 1)
	s.Equal("keep/gate", independent.StructuralDoors[0].Id)
}

func (s *StructuralConversionSuite) TestLegacyAbsenceAddsNoRows() {
	for _, in := range []*sdk.Atlas{nil, {}} {
		out := AtlasToProto(in)
		s.Empty(out.StructuralWalls)
		s.Empty(out.StructuralDoors)
	}
}
