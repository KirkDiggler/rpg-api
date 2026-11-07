package sessionv1alpha1

import (
	"testing"

	"github.com/stretchr/testify/suite"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

type GroundConvertSuite struct{ suite.Suite }

func TestGroundConvertSuite(t *testing.T) { suite.Run(t, new(GroundConvertSuite)) }

func (s *GroundConvertSuite) TestAtlasCarriesBaseAndExplicitPaintWithoutInventingCells() {
	source := &sdk.Atlas{Regions: []sdk.AtlasRegion{
		{ID: "court", Cells: []spatial.Position{{X: 2, Y: -1}}, Ground: &sdk.RegionGround{BaseRef: "grass", Paint: []sdk.GroundPaint{{Ref: "grass", Cells: []spatial.Position{{X: 2, Y: -1}}}}}},
		{ID: "old"},
		{ID: "default", Ground: &sdk.RegionGround{}},
	}}
	out := AtlasToProto(source)
	s.Require().Len(out.Regions, 3)
	ground := out.Regions[0].Ground
	s.Require().NotNil(ground)
	s.Equal("grass", ground.BaseRef)
	s.Require().Len(ground.Paint, 1)
	s.Equal("grass", ground.Paint[0].Ref, "equal-to-base paint is still explicit")
	s.Require().Len(ground.Paint[0].Cells, 1)
	s.Equal(float64(2), ground.Paint[0].Cells[0].X)
	s.Equal(float64(-1), ground.Paint[0].Cells[0].Y)
	s.Nil(out.Regions[1].Ground)
	s.Require().NotNil(out.Regions[2].Ground)
	s.Empty(out.Regions[2].Ground.BaseRef)
	source.Regions[0].Ground.Paint[0].Cells[0].X = 99
	s.Equal(float64(2), ground.Paint[0].Cells[0].X, "wire output is detached from SDK input")
}
