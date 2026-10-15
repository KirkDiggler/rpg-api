package sessionv1alpha1

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/proto"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

// CellsRevealedConversionSuite pins fog of war's learning beat on the wire
// (rpg-project#586 R1): CELLS_REVEALED copies the session's body and derives
// nothing.
type CellsRevealedConversionSuite struct{ suite.Suite }

func TestCellsRevealedConversionSuite(t *testing.T) { suite.Run(t, new(CellsRevealedConversionSuite)) }

func cellsRevealedFixture() sdk.CellsRevealedBody {
	rows := structuralAtlasFixture()
	return sdk.CellsRevealedBody{
		// Two PARTIAL regions: each names only the cells newly learned.
		Regions: []sdk.AtlasRegion{
			{ID: "hall", Name: "Hall", Archetype: "crypt", Cells: []spatial.Position{{X: 3, Y: 1}, {X: 4, Y: 1}}, Lighting: sdk.Lighting{Intensity: 0.4}},
			{ID: "tomb", Cells: []spatial.Position{{X: 9, Y: -2}}},
		},
		Scenery:           []spatial.Position{{X: 5, Y: 1}},
		Props:             []sdk.AtlasProp{{ID: "coffin", Ref: "dnd5e:props:coffin", At: spatial.Position{X: 4, Y: 1}}},
		Sealed:            []spatial.Position{{X: 4, Y: 1}},
		Placed:            []sdk.AtlasPlacedProp{{ID: "books", Cells: []spatial.Position{{X: 3, Y: 1}}, BlocksMovement: true}},
		Exits:             []sdk.AtlasExit{{ID: "stair", At: spatial.Position{X: 9, Y: -2}}},
		Boundaries:        []sdk.AtlasBoundary{{From: spatial.Position{X: 4, Y: 1}, To: spatial.Position{X: 5, Y: 1}}},
		Segments:          []sdk.AtlasSegment{{From: sdk.AxialPointF{Q: 2, R: 7.5}, To: sdk.AxialPointF{Q: 6, R: -0.5}}},
		Doorways:          []sdk.AtlasDoorway{{Door: "keep/gate", From: spatial.Position{X: 4, Y: 1}, To: spatial.Position{X: 5, Y: 1}}},
		PropPresentations: []sdk.PropPresentation{{ID: "books", Ref: "dnd5e:props:books", HeightScale: 1.2}},
		StructuralWalls:   rows.StructuralWalls,
		StructuralDoors:   rows.StructuralDoors,
		StructuralWallOpeningsReplacements: []sdk.StructuralWallOpeningsReplacement{
			{WallID: "wall-a", Openings: rows.StructuralWalls[0].Openings}, {WallID: "wall-b"},
		},
	}
}

func (s *CellsRevealedConversionSuite) TestKindAndBodyCrossTheWire() {
	out, err := eventToProto(sdk.Event{Kind: sdk.EventCellsRevealed, Recipient: "alice", Body: cellsRevealedFixture()})
	s.Require().NoError(err)
	s.Equal(sessionpb.EventKind_EVENT_KIND_CELLS_REVEALED, out.Kind)
	s.Nil(out.GetRoomRevealed(), "a cells reveal never rides the deprecated room arm") //nolint:staticcheck // SA1019: asserting the deprecated arm is empty
	s.Require().NotNil(out.GetCellsRevealed())
}

func (s *CellsRevealedConversionSuite) TestTwoPartialRegionsConvertFieldForField() {
	out, err := eventToProto(sdk.Event{Kind: sdk.EventCellsRevealed, Body: cellsRevealedFixture()})
	s.Require().NoError(err)
	got := out.GetCellsRevealed().GetRegions()
	s.Require().Len(got, 2, "both partial regions arrive, unmerged -- the client unions by id")
	s.True(proto.Equal(&sessionpb.AtlasRegion{
		Id: "hall", Name: "Hall", Archetype: "crypt",
		Cells:    []*sessionpb.Position{{X: 3, Y: 1}, {X: 4, Y: 1}},
		Lighting: &sessionpb.Lighting{Intensity: 0.4},
	}, got[0]), "hall: %v", got[0])
	s.Equal("tomb", got[1].Id)
	s.Require().Len(got[1].Cells, 1)
	s.True(proto.Equal(&sessionpb.Position{X: 9, Y: -2}, got[1].Cells[0]))
}

func (s *CellsRevealedConversionSuite) TestStructuralRowsAndPresentationsConvertFieldForField() {
	out, err := eventToProto(sdk.Event{Kind: sdk.EventCellsRevealed, Body: cellsRevealedFixture()})
	s.Require().NoError(err)
	body := out.GetCellsRevealed()
	s.Require().Len(body.StructuralWalls, 1)
	s.True(proto.Equal(expectedStructuralWall(), body.StructuralWalls[0]), "wall row: %v", body.StructuralWalls[0])
	s.Require().Len(body.StructuralDoors, 1)
	s.True(proto.Equal(expectedStructuralDoor(), body.StructuralDoors[0]), "door row: %v", body.StructuralDoors[0])
	s.Require().Len(body.StructuralWallOpeningsReplacements, 2, "a default replacement is a clear and must not drop")
	s.True(proto.Equal(&sessionpb.StructuralWallOpeningsReplacement{WallId: "wall-a", Openings: expectedStructuralWall().Openings}, body.StructuralWallOpeningsReplacements[0]))
	s.Equal("wall-b", body.StructuralWallOpeningsReplacements[1].WallId)
	s.Empty(body.StructuralWallOpeningsReplacements[1].Openings)
	s.Require().Len(body.PropPresentations, 1)
	s.Equal("books", body.PropPresentations[0].Id)
	s.Equal(1.2, body.PropPresentations[0].HeightScale)
}

// TestEveryOtherRowReadsAsTheAtlasDoes: scenery, props, sealed, placed,
// exits, boundaries, segments and doorways cross through the SAME row
// converters the GetAtlas snapshot uses, so a patched cache and a fresh atlas
// cannot disagree on a row's shape.
func (s *CellsRevealedConversionSuite) TestEveryOtherRowReadsAsTheAtlasDoes() {
	in := cellsRevealedFixture()
	out, err := eventToProto(sdk.Event{Kind: sdk.EventCellsRevealed, Body: in})
	s.Require().NoError(err)
	body := out.GetCellsRevealed()
	atlas := AtlasToProto(&sdk.Atlas{Cells: in.Scenery, Props: in.Props, Sealed: in.Sealed, Placed: in.Placed,
		Exits: in.Exits, Boundaries: in.Boundaries, Segments: in.Segments, Doorways: in.Doorways})
	s.True(proto.Equal(&sessionpb.GetAtlasResponse{
		Cells: body.Scenery, Props: body.Props, Sealed: body.Sealed, Placed: body.Placed,
		Exits: body.Exits, Boundaries: body.Boundaries, Segments: body.Segments, Doorways: body.Doorways,
	}, &sessionpb.GetAtlasResponse{
		Cells: atlas.Cells, Props: atlas.Props, Sealed: atlas.Sealed, Placed: atlas.Placed,
		Exits: atlas.Exits, Boundaries: atlas.Boundaries, Segments: atlas.Segments, Doorways: atlas.Doorways,
	}))
	s.Len(body.Scenery, 1)
	s.Len(body.Props, 1)
	s.Len(body.Placed, 1)
	s.Len(body.Exits, 1)
	s.Len(body.Boundaries, 1)
	s.Len(body.Segments, 1)
	s.Len(body.Doorways, 1)
}
