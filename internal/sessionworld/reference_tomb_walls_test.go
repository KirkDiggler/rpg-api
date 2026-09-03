package sessionworld

import (
	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

// crossing is one hex-to-hex step, unordered: a seam is a set of these, and
// which way round a boundary reports its two ends is the compiler's business,
// not this test's.
type crossing struct{ a, b spatial.Position }

// crossingOf normalizes a pair of absolute AXIAL cells into a crossing, in
// coordinate order so the same step compares equal whichever end named it.
func crossingOf(a, b spatial.Position) crossing {
	if a.X > b.X || (a.X == b.X && a.Y > b.Y) {
		a, b = b, a
	}

	return crossing{a: a, b: b}
}

// seamOf turns authored [col,row] pairs into the crossings they name. The
// offset-to-axial half is asked of the toolkit (rpg-toolkit#1150: one basis,
// one place), so a shear correction lands here without this file having an
// opinion about hexes.
func seamOf(pairs [][4]int) map[crossing]bool {
	out := make(map[crossing]bool, len(pairs))
	for _, p := range pairs {
		out[crossingOf(
			tkencounter.HexCellAt(tkencounter.HexesArePointyTop(), p[0], p[1]),
			tkencounter.HexCellAt(tkencounter.HexesArePointyTop(), p[2], p[3]),
		)] = true
	}

	return out
}

// pairFormSeams is the tomb's two seams EXACTLY AS THE DELETED PAIR FORM WROTE
// THEM: the twenty-eight walls it listed plus the two crossings its doors
// stood in, thirty steps in all, transcribed from content/reference-tomb.yaml
// as it stood at origin/dev before this branch re-authored it.
//
// This literal is the regression net (design §7, A6). Slice 2 deletes the pair
// form rather than migrating it, so there is no old compiler left to diff a
// golden against -- byte-identity is gone, and what replaces it is this: the
// thirty crossings a human wrote out one at a time, against the thirty a
// single authored line now derives. Reading the expectation back out of the
// new compiler would assert nothing at all.
var pairFormSeams = [][4]int{
	// The entrance/hall seam, columns 5 and 6.
	{5, 0, 6, 0}, {5, 1, 6, 0}, {5, 1, 6, 1}, {5, 1, 6, 2}, {5, 2, 6, 2},
	{5, 3, 6, 2}, {5, 3, 6, 3}, {5, 3, 6, 4}, {5, 4, 6, 4}, {5, 5, 6, 4},
	{5, 5, 6, 5}, {5, 5, 6, 6}, {5, 6, 6, 6}, {5, 7, 6, 6}, {5, 7, 6, 7},
	// The hall/tomb seam, columns 15 and 16, the same shape ten columns over.
	{15, 0, 16, 0}, {15, 1, 16, 0}, {15, 1, 16, 1}, {15, 1, 16, 2}, {15, 2, 16, 2},
	{15, 3, 16, 2}, {15, 3, 16, 3}, {15, 3, 16, 4}, {15, 4, 16, 4}, {15, 5, 16, 4},
	{15, 5, 16, 5}, {15, 5, 16, 6}, {15, 6, 16, 6}, {15, 7, 16, 6}, {15, 7, 16, 7},
}

// TestTheRewrittenSeamsBlockTheSameCrossings is acceptance A6 at the api seam.
//
// The tomb is re-authored as two lines where it was twenty-eight loose
// crossings and two door edges. This asks the one question that survives the
// pair form's deletion: does the seam still divide the same two rooms in the
// same places? Every crossing the old file named is either walled or a doorway
// in the new atlas, and the new atlas names no crossing the old file did not.
//
// A DOOR MOVED WITHIN THE SEAM, and that is why this compares the UNION rather
// than the walls alone. Under the pair form each door opened a straight
// west-east crossing, whose side midpoint lies half a width from the two
// columns' centers; no thin line passes through a flat-side midpoint (design
// F16), so each door moved one row onto the slanted midpoint the wall actually
// crosses. The set is the same thirty steps; which one of them stands open
// moved by one. The doorway assertions below pin where it moved to, so this
// union test cannot quietly absorb a door wandering off somewhere else.
func (s *ReferenceTombSuite) TestTheRewrittenSeamsBlockTheSameCrossings() {
	atlas, err := s.load().Atlas()
	s.Require().NoError(err)

	got := make(map[crossing]bool, len(atlas.Boundaries)+len(atlas.Doorways))
	for _, b := range atlas.Boundaries {
		got[crossingOf(b.From, b.To)] = true
	}
	for _, d := range atlas.Doorways {
		got[crossingOf(d.From, d.To)] = true
	}

	s.Equal(seamOf(pairFormSeams), got,
		"the two lines must divide the entrance from the hall and the hall from the tomb "+
			"in exactly the places the pair form spelled out")
	s.Len(atlas.Boundaries, 28, "thirty crossings less the two the doors stand in")
	s.Len(atlas.Doorways, 2)
}

// TestEachDoorOpensACrossingBetweenTheSameTwoRooms is A6's second half: a door
// that survived the rewrite still joins the rooms it joined, even though the
// crossing it stands in moved one row (design §7, A6 as amended).
func (s *ReferenceTombSuite) TestEachDoorOpensACrossingBetweenTheSameTwoRooms() {
	atlas, err := s.load().Atlas()
	s.Require().NoError(err)

	rooms := map[tkencounter.DoorID][2]string{}
	where := map[tkencounter.DoorID]crossing{}
	for _, d := range atlas.Doorways {
		rooms[d.Door] = [2]string{s.regionOf(d.From), s.regionOf(d.To)}
		where[d.Door] = crossingOf(d.From, d.To)
	}

	s.Equal([2]string{"entrance", "hall"}, rooms["reference-tomb/entrance-hall"],
		"the first door still opens the entrance onto the hall")
	s.Equal([2]string{"hall", "tomb"}, rooms["reference-tomb/hall-tomb"],
		"and the second still opens the hall onto the tomb")

	// Where each one moved to, as literals: the authored positions are
	// [6,4] offset [-0.25,-0.375] and [16,4] offset [-0.25,0.375], and these
	// are the crossings those side midpoints belong to.
	s.Equal(
		crossingOf(
			tkencounter.HexCellAt(tkencounter.HexesArePointyTop(), 5, 3),
			tkencounter.HexCellAt(tkencounter.HexesArePointyTop(), 6, 4),
		),
		where["reference-tomb/entrance-hall"],
		"the entrance door stands on the slanted crossing [5,3]-[6,4], one row up from the pair form's [5,4]-[6,4]")
	s.Equal(
		crossingOf(
			tkencounter.HexCellAt(tkencounter.HexesArePointyTop(), 15, 5),
			tkencounter.HexCellAt(tkencounter.HexesArePointyTop(), 16, 4),
		),
		where["reference-tomb/hall-tomb"],
		"and the tomb door on [15,5]-[16,4], one row down from [15,4]-[16,4]")
}

// TestTheSeamsAreTwoSegmentsAndNothingIsSealed pins what the lines themselves
// come out as: the presentation half of the same two walls (design §5.2), and
// the price they cost the floor.
//
// NOTHING IS SEALED, which is the whole reason these are quarter lines. A
// quarter line runs a quarter of a hex's width inside the column it cuts and
// shaves five twenty-fourths off the cells either side, which leaves every one
// of them standable at the calibrated fraction. The thick alternative would
// have kept both doors where the pair form had them and sealed four entrance
// cells; this assertion is what makes that choice visible if anybody re-authors
// the seams the other way.
func (s *ReferenceTombSuite) TestTheSeamsAreTwoSegmentsAndNothingIsSealed() {
	atlas, err := s.load().Atlas()
	s.Require().NoError(err)

	s.Require().Len(atlas.Segments, 2, "two authored walls, two lines to draw")

	// Authored [5,7]+[0.25,0.375] to [6,0]+[-0.25,-0.375], and the same shape
	// ten columns over. A side midpoint is exactly half a step from the center
	// it belongs to, which is where the halves come from.
	s.Equal(tkencounter.AxialPointF{Q: 2, R: 7.5}, atlas.Segments[0].From)
	s.Equal(tkencounter.AxialPointF{Q: 6, R: -0.5}, atlas.Segments[0].To)
	s.Equal(tkencounter.AxialPointF{Q: 12, R: 7.5}, atlas.Segments[1].From)
	s.Equal(tkencounter.AxialPointF{Q: 16, R: -0.5}, atlas.Segments[1].To)

	for i, segment := range atlas.Segments {
		s.Zerof(segment.Height, "segment %d authors no height, and 0 means standard rather than flat", i)
	}

	s.Empty(atlas.Sealed, "quarter lines leave every cell they pass standable")
	s.Len(atlas.Cells, 224, "6x8 entrance plus 10x8 hall plus 12x8 tomb, unchanged by the rewrite")
}
