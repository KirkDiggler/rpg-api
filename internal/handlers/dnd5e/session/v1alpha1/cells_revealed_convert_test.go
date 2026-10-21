package sessionv1alpha1

import (
	"testing"

	"github.com/stretchr/testify/suite"

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
