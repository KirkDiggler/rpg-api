package authoringv1alpha1_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	authoringhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/authoring/v1alpha1"
	authoringorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"

	authoringpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/authoring/v1alpha1"
)

// WallGeometryWireSuite is wall-geometry slice 2 (rpg-project#360, rpg-api#899)
// asked at the wire: the real handler over the real orchestrator over the real
// content registry, so what these assertions read is the PutDungeonResponse a
// builder actually receives.
//
// Same shape and the same reason as SceneryWireSuite beside it, one slice on.
type WallGeometryWireSuite struct {
	suite.Suite

	ctx     context.Context
	handler *authoringhandler.Handler
}

func TestWallGeometryWireSuite(t *testing.T) {
	suite.Run(t, new(WallGeometryWireSuite))
}

func (s *WallGeometryWireSuite) SetupTest() {
	s.ctx = auth.WithPlayerID(context.Background(), "alice")

	registry, _ := dungeonstest.Scratch(s.T())
	orch, err := authoringorch.New(&authoringorch.Config{Dungeons: registry})
	s.Require().NoError(err)
	h, err := authoringhandler.New(&authoringhandler.HandlerConfig{Orchestrator: orch})
	s.Require().NoError(err)
	s.handler = h
}

// TestPutDungeon_AHalvedCellIsStillFloorAndStillOwned pins what sealing does
// NOT do to the wire.
//
// A wall drawn through a cell's center takes the cell's footing away and
// nothing else: it keeps its region, and a host must draw it exactly as it
// draws the floor beside it. So it is still in the flat cell list and still in
// its region's cells, and an implementation that "helpfully" dropped a sealed
// cell from either would punch a visible hole in the room.
func (s *WallGeometryWireSuite) TestPutDungeon_AHalvedCellIsStillFloorAndStillOwned() {
	resp, err := s.handler.PutDungeon(s.ctx, &authoringpb.PutDungeonRequest{
		Key: dungeonstest.HalvedRoomKey, Yaml: dungeonstest.HalvedRoomYAML,
	})
	s.Require().NoError(err)
	s.Require().Empty(resp.GetErrors(), "the halved-room file must compile: %v", resp.GetErrors())
	s.Require().NotNil(resp.GetAtlas())

	floor := cellsOf(resp.GetAtlas().GetCells())
	s.Len(floor, 12, "a wall through a cell removes no floor")

	s.Require().Len(resp.GetAtlas().GetRegions(), 1)
	s.Equal("vault", resp.GetAtlas().GetRegions()[0].GetId())
	owned := cellsOf(resp.GetAtlas().GetRegions()[0].GetCells())
	s.Len(owned, 12, "and takes no cell out of the room that owns it")

	for _, c := range dungeonstest.HalvedRoomSealedCells {
		s.True(floor[at(c[0], c[1])], "halved cell %v is on the wire as floor", c)
		s.True(owned[at(c[0], c[1])], "and the vault still claims %v", c)
	}
	for _, c := range dungeonstest.HalvedRoomRegionCells {
		s.True(floor[at(c[0], c[1])], "painted cell %v is on the wire as floor", c)
		s.True(owned[at(c[0], c[1])], "and the vault claims %v", c)
	}

	// And the wall really is drawn through them, which is what makes the two
	// assertions above worth making: every step out of a halved cell into
	// another floor cell is blocked, plus the one crossing the line runs
	// ALONG rather than through -- the flat side between [0,1] and [1,1],
	// whose two cells the line leaves whole. Nine in all. Without this the
	// test would pass just as well on a room with no wall in it.
	blocked := make(map[[2]wireCell]bool, len(resp.GetAtlas().GetBoundaries()))
	for _, b := range resp.GetAtlas().GetBoundaries() {
		from := wireCell{x: b.GetFrom().GetX(), y: b.GetFrom().GetY()}
		to := wireCell{x: b.GetTo().GetX(), y: b.GetTo().GetY()}
		blocked[orderedPair(from, to)] = true
	}
	s.Len(blocked, 9)
	for _, step := range [][4]int{
		// out of [1,0]
		{1, 0, 0, 0}, {1, 0, 2, 0}, {1, 0, 0, 1}, {1, 0, 1, 1},
		// out of [1,2]
		{1, 2, 0, 2}, {1, 2, 2, 2}, {1, 2, 0, 1}, {1, 2, 1, 1},
		// and the side the line lies on
		{0, 1, 1, 1},
	} {
		s.Truef(blocked[orderedPair(at(step[0], step[1]), at(step[2], step[3]))],
			"the step [%d,%d] to [%d,%d] must be blocked", step[0], step[1], step[2], step[3])
	}

	// The two halved cells are the WHOLE of what `sealed` says, and it says it
	// about cells that are also in `cells` and in the vault above: sealed floor
	// is still floor.
	sealed := cellsOf(resp.GetAtlas().GetSealed())
	s.Len(sealed, 2)
	for _, c := range dungeonstest.HalvedRoomSealedCells {
		s.True(sealed[at(c[0], c[1])], "halved cell %v is on the wire as sealed", c)
	}
	s.False(sealed[at(1, 1)],
		"[1,1] is the odd-row cell the line runs ALONGSIDE rather than through, and stays standable")

	// And the line itself, once, as the author drew it: center of [1,0] to
	// center of [1,2], which are the axial cells (1,0) and (0,2) -- an offset
	// grid shears when it becomes axial, and a center carries no half.
	s.Require().Len(resp.GetAtlas().GetSegments(), 1, "one authored wall, one line to draw")
	segment := resp.GetAtlas().GetSegments()[0]
	s.Equal(at(1, 0).x, segment.GetFrom().GetQ())
	s.Equal(at(1, 0).y, segment.GetFrom().GetR())
	s.Equal(at(1, 2).x, segment.GetTo().GetQ())
	s.Equal(at(1, 2).y, segment.GetTo().GetR())
	s.Zero(segment.GetHeight(), "no height authored, and 0 means standard rather than flat")
}

// orderedPair normalizes two cells into a comparable unordered pair: which way
// round a boundary reports its ends is the compiler's business.
func orderedPair(a, b wireCell) [2]wireCell {
	if a.x > b.x || (a.x == b.x && a.y > b.y) {
		a, b = b, a
	}

	return [2]wireCell{a, b}
}

// TestPutDungeon_ThePairFormIsRefusedByName is design F4 at the wire: the pair
// form is DELETED rather than migrated, and a builder who pastes an old file
// gets told which form it wrote rather than a parse error about a list.
//
// The message is the compiler's own, carried through unchanged -- the api
// invents no vocabulary for a dialect it does not own -- and it arrives as a
// compile ERROR on the response rather than a transport failure, because a
// file that will not compile is an answer to PutDungeon, not a broken call.
func (s *WallGeometryWireSuite) TestPutDungeon_ThePairFormIsRefusedByName() {
	resp, err := s.handler.PutDungeon(s.ctx, &authoringpb.PutDungeonRequest{
		Key: "pair-form", Yaml: pairFormDungeon,
	})
	s.Require().NoError(err, "a file that will not compile is an answer, not a transport failure")
	s.Require().Len(resp.GetErrors(), 1)
	s.Contains(resp.GetErrors()[0].GetMessage(), "`edges` is the deleted pair form",
		"the refusal names the form the author wrote")
	s.Contains(resp.GetErrors()[0].GetPath(), "line",
		"and points at the line it read it on")
	s.Nil(resp.GetAtlas(), "nothing compiled, so there is nothing to draw")
}

// pairFormDungeon is a dungeon written the way every file in this repository
// was written before slice 2: walls as a list of crossings between adjacent
// cells. Kept as a literal rather than read from history so the refusal stays
// tested after the last real pair-form file is gone.
const pairFormDungeon = `version: 2
key: pair-form
name: The Old Way
orientation: pointy
void: opaque

regions:
  - id: vault
    name: Vault
    archetype: crypt
    lighting: { intensity: 0.5 }
    cells:
      - [[0,0],[1,0]]
      - [[0,1],[1,1]]

start: [0, 0]

walls:
  - [[0,0],[1,0]]
`
