package authoringv1alpha1_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"

	authoringpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/authoring/v1alpha1"
	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	authoringhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/authoring/v1alpha1"
	authoringorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"
)

// SceneryWireSuite is wall-geometry slice 1 (rpg-project#360, rpg-api#898)
// asked at the wire, not at a seam behind it: the real handler over the real
// orchestrator over the real content registry, so what these assertions read
// is the PutDungeonResponse a builder actually receives.
//
// HandlerSuite in this package mocks the registry on purpose -- the handler's
// job there is transport, and a real compile would only slow those tests down.
// This suite is the opposite question, and needs the real one: whether the
// bytes an author wrote survive a genuine dungeonspec compile and a genuine
// atlas projection all the way onto the response message.
type SceneryWireSuite struct {
	suite.Suite

	ctx     context.Context
	handler *authoringhandler.Handler
}

func TestSceneryWireSuite(t *testing.T) {
	suite.Run(t, new(SceneryWireSuite))
}

func (s *SceneryWireSuite) SetupTest() {
	s.ctx = auth.WithPlayerID(context.Background(), "alice")

	registry, _ := dungeonstest.Scratch(s.T())
	orch, err := authoringorch.New(&authoringorch.Config{Dungeons: registry})
	s.Require().NoError(err)
	h, err := authoringhandler.New(&authoringhandler.HandlerConfig{Orchestrator: orch})
	s.Require().NoError(err)
	s.handler = h
}

// wireCell is one atlas cell as the proto carries it, comparable so a test can
// ask set questions of a repeated field.
type wireCell struct{ x, y float64 }

// at converts an authored [col,row] offset pair to the wire cell it becomes.
// The offset-to-axial half is asked of the toolkit (rpg-toolkit#1150: one
// basis, one place) and only the proto's own field reading is done here.
func at(col, row int) wireCell {
	c := tkencounter.HexCellAt(tkencounter.HexesArePointyTop(), col, row)
	return wireCell{x: c.X, y: c.Y}
}

func cellsOf(ps []*sessionpb.Position) map[wireCell]bool {
	out := make(map[wireCell]bool, len(ps))
	for _, p := range ps {
		out[wireCell{x: p.GetX(), y: p.GetY()}] = true
	}
	return out
}

// TestPutDungeon_SceneryReachesTheWireAsFloorInNoRegion is rpg-api#898's
// second acceptance stated on the message itself.
//
// Slice 1 adds no field to the wire (wall-geometry design §5.1): scenery rides
// the flat `cells` list, and a cell there that no region lists IS scenery.
// That is the entire contract, so it is the entire assertion -- the strip is
// in cells, the strip is in no region's cells, and the wall between the strip
// and the room is still standing.
func (s *SceneryWireSuite) TestPutDungeon_SceneryReachesTheWireAsFloorInNoRegion() {
	resp, err := s.handler.PutDungeon(s.ctx, &authoringpb.PutDungeonRequest{
		Key: dungeonstest.SceneryStripKey, Yaml: dungeonstest.SceneryStripYAML,
	})
	s.Require().NoError(err)
	s.Require().Empty(resp.GetErrors(), "the scenery file must compile: %v", resp.GetErrors())
	s.Require().NotNil(resp.GetAtlas())

	floor := cellsOf(resp.GetAtlas().GetCells())
	s.Len(floor, 12, "nine cells of vault and three of scenery")

	owned := make(map[wireCell]bool)
	s.Require().Len(resp.GetAtlas().GetRegions(), 1)
	s.Equal("vault", resp.GetAtlas().GetRegions()[0].GetId())
	for c := range cellsOf(resp.GetAtlas().GetRegions()[0].GetCells()) {
		owned[c] = true
	}
	s.Len(owned, 9, "and only the vault's nine are owned")

	for _, c := range dungeonstest.SceneryStripSceneryCells {
		s.True(floor[at(c[0], c[1])], "scenery cell %v is on the wire as floor", c)
		s.False(owned[at(c[0], c[1])], "and no region's cell list claims %v", c)
	}
	for _, c := range dungeonstest.SceneryStripRegionCells {
		s.True(floor[at(c[0], c[1])], "painted cell %v is on the wire as floor", c)
		s.True(owned[at(c[0], c[1])], "and the vault claims %v", c)
	}

	s.Len(resp.GetAtlas().GetBoundaries(), 5,
		"the wall between the room and the strip is still standing: five crossings the two columns share")
}
