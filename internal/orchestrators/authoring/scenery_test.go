package authoring_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	authoringorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"
)

// ScenerySuite is rpg-api's half of wall-geometry slice 1 (rpg-project#360,
// rpg-api#898): the toolkit gained a third floor state and rpg-api gained a
// version pin, so what has to be proved here is that NOTHING in this repo
// stands between the author's `scenery:` list and the wire.
//
// It runs on the real registry over a scratch copy of the shipped content --
// a real dungeonspec compile and a real session Manager projecting the atlas
// -- rather than the mocked registry the handler suite uses, because a mock
// would only prove the call happened.
type ScenerySuite struct {
	suite.Suite

	ctx  context.Context
	orch *authoringorch.Orchestrator
}

func TestScenerySuite(t *testing.T) {
	suite.Run(t, new(ScenerySuite))
}

func (s *ScenerySuite) SetupTest() {
	s.ctx = context.Background()

	registry, _ := dungeonstest.Scratch(s.T())
	orch, err := authoringorch.New(&authoringorch.Config{Dungeons: registry})
	s.Require().NoError(err)
	s.orch = orch
}

// put stores the scenery fixture and returns the compiled answer, failing the
// test if the file did not compile -- with the defects named, since "the file
// the pin was bumped for does not compile" is the failure this suite exists
// to make legible.
func (s *ScenerySuite) put() *authoringorch.PutDungeonOutput {
	s.T().Helper()

	out, err := s.orch.PutDungeon(s.ctx, &authoringorch.PutDungeonInput{
		Key:  dungeonstest.SceneryStripKey,
		YAML: []byte(dungeonstest.SceneryStripYAML),
	})
	s.Require().NoError(err)
	s.Require().Empty(out.Errors, "the scenery file must compile: %v", out.Errors)

	return out
}

// cell is the one conversion this test performs: an authored [col,row] offset
// pair to the dungeon-absolute axial cell the atlas draws. Asked of the
// toolkit rather than computed, for the reason HexCellAt is exported at all
// (rpg-toolkit#1150: one basis, one place) -- a test that reimplemented the
// basis would agree with a wrong answer as happily as a right one.
func cell(col, row int) spatial.Position {
	return tkencounter.HexCellAt(tkencounter.HexesArePointyTop(), col, row)
}

// TestPutDungeon_AFileWithSceneryRoundTripsVerbatim is the first half of
// #898's acceptance. The registry never re-marshals a file, and scenery must
// not be the exception: what comes back is the bytes that went in, comments,
// spacing and the `scenery:` block included.
func (s *ScenerySuite) TestPutDungeon_AFileWithSceneryRoundTripsVerbatim() {
	s.put()

	got, err := s.orch.GetDungeon(s.ctx, &authoringorch.GetDungeonInput{Key: dungeonstest.SceneryStripKey})
	s.Require().NoError(err)
	s.Equal(dungeonstest.SceneryStripYAML, string(got.YAML),
		"the stored file is the authored file, byte for byte")
}

// TestPutDungeon_SceneryIsFloorInNoRegion is the second half: the atlas
// PutDungeon answers with -- the same projection GetAtlas serves, produced by
// the session Manager -- carries the strip in the flat Cells list, and no
// region claims it.
//
// This is design §5.1's "no change" stated as a test: a cell in Cells and in
// no region IS scenery, which is the whole of what the wire says about it.
func (s *ScenerySuite) TestPutDungeon_SceneryIsFloorInNoRegion() {
	atlas := s.put().Atlas
	s.Require().NotNil(atlas)

	inCells := make(map[spatial.Position]bool, len(atlas.Cells))
	for _, c := range atlas.Cells {
		inCells[c] = true
	}
	s.Len(atlas.Cells, 12, "nine owned cells and three of scenery")

	owned := make(map[spatial.Position]bool)
	s.Require().Len(atlas.Regions, 1)
	s.Equal("vault", atlas.Regions[0].ID)
	for _, c := range atlas.Regions[0].Cells {
		owned[c] = true
	}
	s.Len(owned, len(dungeonstest.SceneryStripRegionCells), "the vault owns what it was painted with")

	for _, at := range dungeonstest.SceneryStripSceneryCells {
		c := cell(at[0], at[1])
		s.True(inCells[c], "scenery cell %v is floor: it is in Cells", at)
		s.False(owned[c], "scenery cell %v belongs to no region", at)
	}

	for _, at := range dungeonstest.SceneryStripRegionCells {
		c := cell(at[0], at[1])
		s.True(inCells[c], "painted cell %v is floor", at)
		s.True(owned[c], "painted cell %v is the vault's", at)
	}
}

// TestPutDungeon_TheWallOnTheSeamReachesTheAtlas pins the rest of the
// fixture, so a compile that quietly dropped the scenery's neighbors could
// not pass the two tests above by accident: the wall between the vault and
// the strip stands, and the prop the design allows on scenery is there.
func (s *ScenerySuite) TestPutDungeon_TheWallOnTheSeamReachesTheAtlas() {
	atlas := s.put().Atlas
	s.Require().NotNil(atlas)

	s.Len(atlas.Boundaries, 5, "the five crossings the vault's and the strip's columns share")
	s.Require().Len(atlas.Props, 1)
	s.Equal("dnd5e:props:bone-pile", atlas.Props[0].Ref)
	s.Equal(cell(3, 1), atlas.Props[0].At, "a prop may stand on scenery (design §3.1 F2)")
}
