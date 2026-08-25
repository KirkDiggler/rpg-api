package sessionworld

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	tkdungeonspec "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter/dungeonspec"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

type ReferenceTombSuite struct {
	suite.Suite

	tomb *Dungeon
}

func TestReferenceTombSuite(t *testing.T) {
	suite.Run(t, new(ReferenceTombSuite))
}

// referenceTombPath is the shipped tomb, read from the content tree rather
// than embedded: this package holds no content any more (see the package
// comment), and the test compiling the real shipped file is the point.
var referenceTombPath = filepath.Join("..", "..", "content", "reference-tomb.yaml")

func (s *ReferenceTombSuite) SetupTest() {
	raw, err := os.ReadFile(referenceTombPath)
	s.Require().NoError(err, "the shipped tomb must exist at content/reference-tomb.yaml")
	tomb, err := Compile(raw)
	s.Require().NoError(err, "the shipped tomb must compile")
	s.tomb = tomb
}

// TestTheFileNamesItself pins that Compile reports the file's own key: the
// registry stores a file under the key it was Put with and refuses a file
// whose key line disagrees, which only works if the key comes out of the
// compile rather than the filename.
func (s *ReferenceTombSuite) TestTheFileNamesItself() {
	s.Equal("reference-tomb", s.tomb.Key)
	s.NotEmpty(s.tomb.Name)
}

// load rebuilds a live encounter from the world this package produced, which is
// exactly what session.StartSession does with it. Doing it here is how the
// tests ask questions in absolute space -- which region owns a cell, what state
// a door is in -- without this package having to expose either.
func (s *ReferenceTombSuite) load() *tkencounter.Encounter {
	enc, err := tkencounter.LoadEncounter(&tkencounter.LoadEncounterInput{
		Data:       *s.tomb.World,
		Initiative: orderAsGiven{}, Standing: nobodyDown{}, Sight: nobodySees{},
		TurnDriver: tkencounter.PassDriver{}, Striker: tkencounter.RefusingStriker{},
	})
	s.Require().NoError(err, "and the world it produced must be one the composition accepts back")

	return enc
}

func (s *ReferenceTombSuite) regionOf(cell spatial.Position) string {
	region, ok := s.load().RegionAt(cell)
	s.Require().Truef(ok, "cell %v should be floor in some chamber", cell)

	return region
}

// TestTheWorldArrivesEmptyOfMembers pins the decision in Dungeon.World's doc:
// the world is authored content, and everybody who stands in it arrives through
// a session verb.
//
// It is not a stylistic preference. encounter.Join refuses a member already in
// the encounter, so a party baked in here could never be joined -- and Join is
// the only thing that loads a character's sheet. A change that "helpfully"
// seeded the roster at construction would leave every player sheetless and
// every session unjoinable, and this is the test that says so first.
func (s *ReferenceTombSuite) TestTheWorldArrivesEmptyOfMembers() {
	s.Empty(s.tomb.World.Members, "the world is content, not a live encounter")
	s.Empty(s.tomb.World.EverMembers, "and nobody has ever been in it")
}

// TestThePartyComesInWhereTheDungeonSaysItDoes checks the authored start
// resolves to the chamber the author put it in, rather than to whichever
// chamber happens to be first.
func (s *ReferenceTombSuite) TestThePartyComesInWhereTheDungeonSaysItDoes() {
	s.Require().NotEmpty(s.tomb.PartySeats)
	s.Equal("entrance", s.regionOf(s.tomb.PartySeats[0]), "the party enters the entrance")
}

// TestPlacementsAreProjectedThroughTheCompositionNotCopied is the test this
// package exists for.
//
// The tomb's authored start is the absolute offset cell [1,3]. Its axial
// cell is (0,3) -- not (1,3) -- because an offset grid SHEARS when it becomes
// axial. Any change that passes the authored cell straight through, or
// reimplements the conversion by hand instead of asking encounter.HexCellAt,
// produces (1,3) or something else and fails here.
//
// The literal is the point rather than an embarrassment: reading the expected
// value back out of the same projection under test would assert nothing at all.
// It has MOVED TWICE, and that is the literal earning its keep. It was (1,-4)
// until rpg-toolkit#1141 corrected spatial's hex offset schemes (pointy-top
// is odd-r, so the COLUMN shears with the row: q = 1 - (3-1)/2 = 0), then
// (0,-3) until rpg-toolkit#1150 corrected the axial basis (R is cube Z, the
// row itself, so r = 3 -- not -q-r). Each value was read off the corrected
// toolkit, not derived by hand -- this package asks the toolkit for the
// conversion precisely so it never has its own opinion about it.
func (s *ReferenceTombSuite) TestPlacementsAreProjectedThroughTheCompositionNotCopied() {
	authored := spatial.Position{X: 1, Y: 3}
	s.Require().NotEqual(authored, s.tomb.PartySeats[0],
		"an authored cell that survived unchanged means nothing projected it")
	s.Equal(spatial.Position{X: 0, Y: 3}, s.tomb.PartySeats[0],
		"the authored offset [1,3] is the axial cell (0,3)")
}

// TestTheGarrisonHoldsTheHallAndTheCaptainWaitsBeyondIt checks the monsters
// land in the chambers they were authored into -- the second half of the
// projection, and the half a seats-only implementation would leave broken.
func (s *ReferenceTombSuite) TestTheGarrisonHoldsTheHallAndTheCaptainWaitsBeyondIt() {
	s.Require().Len(s.tomb.Monsters, 3, "two skeletons and their captain")

	byRegion := map[string][]Monster{}
	for _, m := range s.tomb.Monsters {
		region := s.regionOf(m.At)
		byRegion[region] = append(byRegion[region], m)
	}

	s.Len(byRegion["hall"], 2, "the garrison holds the hall")
	s.Require().Len(byRegion["tomb"], 1, "and one thing waits past the locked door")
	s.Equal("dnd5e:monsters:skeleton-captain", byRegion["tomb"][0].Ref)
	s.True(byRegion["tomb"][0].Boss, "the captain is the boss")

	for _, m := range byRegion["hall"] {
		s.False(m.Boss, "a garrison skeleton is not the boss")
	}
}

// TestTheAuthorsWordsAboutMonstersSurviveTheCompile pins two authored facts.
// Targeting is still carried-and-not-acted-on (session.SpawnInput has no
// field for it); Boss graduated — the wave its comment waited for is
// rpg-project#268, and TestTheBossFlagBecomesTheDeclaredDoom below pins what
// it became.
func (s *ReferenceTombSuite) TestTheAuthorsWordsAboutMonstersSurviveTheCompile() {
	targeting := map[string]int{}
	bosses := 0
	for _, m := range s.tomb.Monsters {
		targeting[m.Targeting]++
		if m.Boss {
			bosses++
		}
	}

	s.Equal(2, targeting["lowest-health"], "the garrison's authored targeting is carried")
	s.Equal(1, targeting["closest"], "and so is the captain's")
	s.Equal(1, bosses, "exactly one boss")
}

// TestEveryMonsterIsNamedAfterWhatItIs pins the naming rule in
// Monster.MemberID: a client sees the member ID and nothing else, so an opaque
// ordinal would leave a UI unable to tell a skeleton from its captain.
//
// It also pins the per-ref numbering. Numbering across the whole dungeon would
// give the captain "skeleton-captain-3" today and a different number the moment
// a garrison skeleton is added or removed -- a rename of a monster nothing
// about which changed.
func (s *ReferenceTombSuite) TestEveryMonsterIsNamedAfterWhatItIs() {
	byID := map[string]Monster{}
	for _, m := range s.tomb.Monsters {
		s.Require().NotEmptyf(m.MemberID, "monster %s has no member ID", m.Ref)
		s.Require().NotContains(m.MemberID, ":", "a member ID is not a ref")
		_, repeated := byID[m.MemberID]
		s.Require().Falsef(repeated, "member ID %q is used twice", m.MemberID)
		byID[m.MemberID] = m
	}

	s.Contains(byID, "skeleton-1")
	s.Contains(byID, "skeleton-2")
	s.Contains(byID, "skeleton-captain-1", "the captain is numbered within its OWN ref, not across the dungeon")
	s.True(byID["skeleton-captain-1"].Boss)
}

// TestTheBossFlagBecomesTheDeclaredDoom pins what Compile does with the
// authored flag (rpg-project#268): the world is born declaring BOTH its
// endings — withdrawal (external) and the captain's fall (member_down over
// the member ID the launch will spawn him under). The member does not exist
// yet in this empty world, and that is the contract: an ending may name a
// member that joins later, exactly as TriggerReachedPosition's filter may.
func (s *ReferenceTombSuite) TestTheBossFlagBecomesTheDeclaredDoom() {
	endings := s.tomb.World.Endings
	s.Require().Len(endings, 2, "withdrawal and the doom, nothing silent")

	s.Equal("withdrawn", endings[0].Key)
	s.Equal("external", endings[0].Kind)

	s.Equal(EndingBossDown, endings[1].Key)
	s.Equal("member_down", endings[1].Kind)
	s.Equal("skeleton-captain-1", string(endings[1].Member),
		"the doom names the captain by the member ID the launch spawns him under")
}

// TestTheTombIsShutAtDCTwelve pins the authored lock. Twelve is a literal on
// purpose for the reason the projection's literal is: it is the authored fact
// under test, so reading it back out of the compiler would be circular.
func (s *ReferenceTombSuite) TestTheTombIsShutAtDCTwelve() {
	enc := s.load()
	doors := enc.Doors()
	s.Require().Len(doors, 2, "the tomb authors two doorways: an open one into the hall and the locked one into the tomb")

	byID := map[tkencounter.DoorID]tkencounter.Door{}
	for _, d := range doors {
		byID[d.ID] = d
	}
	// Door IDs are minted under the dungeon's key (version 2), so two
	// dungeons in one process cannot collide.
	open, ok := byID["reference-tomb/entrance-hall"]
	s.Require().True(ok, "doors are named <key>/<id>: %v", doors)
	s.Equal(tkencounter.DoorStateKind("open"), open.State.Kind(), "the way into the hall is an open gap")
	locked, ok := byID["reference-tomb/hall-tomb"]
	s.Require().True(ok)
	s.Equal(tkencounter.DoorStateKind("locked"), locked.State.Kind(), "and the tomb starts shut")

	_, err := enc.OpenDoor(&tkencounter.OpenDoorInput{Door: locked.ID})
	s.Require().ErrorIs(err, tkencounter.ErrLocked)
	s.Contains(err.Error(), "DC 12", "and the refusal says what it would take")
}

// TestAPartyCanAllStandSomewhereDistinct checks the seats are usable as a list:
// a four-player party takes the first four and nobody is placed on top of
// anybody. A projection bug that mapped several local cells onto one absolute
// cell would pass every region assertion above and fail here.
func (s *ReferenceTombSuite) TestAPartyCanAllStandSomewhereDistinct() {
	const biggestPartyWorthChecking = 8
	s.Require().GreaterOrEqual(len(s.tomb.PartySeats), biggestPartyWorthChecking)

	seen := map[spatial.Position]bool{}
	for i, seat := range s.tomb.PartySeats[:biggestPartyWorthChecking] {
		s.Falsef(seen[seat], "seat %d repeats cell %v", i, seat)
		seen[seat] = true
		s.Equal("entrance", s.regionOf(seat), "a party stands together at the way in")
	}
}

// monsterFirst is an InitiativeRoller rigged for exactly one purpose: hand
// the skeleton the very first turn regardless of contact-detection order, so
// TestAFightsUnplayedTurnPassesWithoutTouchingTheStriker can force the one
// moment this package's own construction-time TurnDriver/Striker pair can
// ever run -- "first light" (encounter.SetupInput's TurnDriver doc: "a
// fight can form at first light with an unplayed member first in
// initiative"). Not general purpose, and does not need to be: the test that
// uses it only ever has these two members.
type monsterFirst struct{}

func (monsterFirst) RollInitiative(_ []tkencounter.MemberID) ([]tkencounter.MemberID, error) {
	return []tkencounter.MemberID{"skel-1", "fighter"}, nil
}

// allSeeing gives every member a sight range large enough that two adjacent
// members always see each other. nobodySees's range of zero -- this
// package's own construction-time stand-in everywhere else, correct for a
// throwaway placement probe and an empty real world -- would never let a
// fight form at all here, which is exactly wrong for what this test needs
// to prove.
type allSeeing struct{}

func (allSeeing) Sight(members []tkencounter.MemberID) (map[tkencounter.MemberID]int, error) {
	out := make(map[tkencounter.MemberID]int, len(members))
	for _, id := range members {
		out[id] = 1_000_000
	}
	return out, nil
}

// TestAFightsUnplayedTurnPassesWithoutTouchingTheStriker is the acceptance
// proof rpg-project#254 asks this package for: tkencounter.PassDriver{} and
// tkencounter.RefusingStriker{} are not merely types that satisfy
// SetupInput's now-required TurnDriver/Striker fields, they are the RIGHT
// pair together at the one moment this package's own construction-only
// encounters can ever reach a driven monster turn.
//
// A monster and a player start adjacent and in sight of each other, with
// initiative rigged to hand the monster the first turn. That forms the
// fight AND drives the monster's unplayed turn synchronously inside
// NewEncounter -- see [Encounter.form]'s call to driveMonsterTurns --
// before this test's own code runs a single line past construction. If
// PassDriver ever tried to swing, RefusingStriker would refuse with
// ErrRefusingStriker and NewEncounter would return that error instead of an
// encounter, so a bare successful construction already is most of the
// proof; the clock assertion below is the rest of it, confirming the
// skeleton's turn did not just fail to error but actually passed.
func TestAFightsUnplayedTurnPassesWithoutTouchingTheStriker(t *testing.T) {
	enc, err := tkencounter.NewEncounter(&tkencounter.SetupInput{
		Initiative: monsterFirst{},
		Standing:   nobodyDown{},
		Sight:      allSeeing{},
		TurnDriver: tkencounter.PassDriver{},
		Striker:    tkencounter.RefusingStriker{},
		Retention:  tkencounter.RetentionUnbounded,
		Field: tkencounter.FieldInput{
			// Transparent void: a sightline hugging the edge of a sheared
			// rectangle crosses void, and this test is about the turn, not
			// the void.
			Canvas: tkencounter.CanvasInput{Void: tkencounter.VoidIsTransparent(), Orientation: tkencounter.HexesArePointyTop()},
			Regions: []tkencounter.RegionInput{{
				ID: "room", Name: "Room", Archetype: "test", Lighting: &tkencounter.Lighting{Intensity: 1},
				Cells: rectangle(4, 4),
			}},
		},
		Members: []tkencounter.MemberInput{
			{ID: "fighter", Kind: tkencounter.KindPlayer, Position: spatial.Position{X: 0, Y: 0}},
			{ID: "skel-1", Kind: tkencounter.KindMonster, Position: spatial.Position{X: 1, Y: 0}},
		},
		Endings: []tkencounter.EndingInput{{Key: "withdrawn", Trigger: tkencounter.TriggerExternal{}}},
	})
	require.NoError(t, err,
		"constructing the world must still succeed -- if RefusingStriker had been reached this is exactly where it would have failed")

	clock, err := enc.ClockOf(&tkencounter.ClockOfInput{Member: "fighter"})
	require.NoError(t, err)
	require.Equal(t, tkencounter.ClockTurn, clock.Kind, "adjacent and in sight, a fight must have formed at first light")
	require.Equal(t, tkencounter.MemberID("fighter"), clock.Active,
		"the skeleton went first and PassDriver passed its turn immediately -- the turn already belongs to the player nobody has acted for yet")
}

// rectangle paints a width x height block of absolute offset cells starting
// at [0,0] -- the smallest honest region for a test.
func rectangle(width, height int) []spatial.Position {
	out := make([]spatial.Position, 0, width*height)
	for row := 0; row < height; row++ {
		for col := 0; col < width; col++ {
			out = append(out, spatial.Position{X: float64(col), Y: float64(row)})
		}
	}
	return out
}

// twoRegions is a version-2 dungeon with a six-wide entrance and hall, the
// party starting at [1,3], and one monster placed where the test says.
func twoRegions(key string, monsterAt string) string {
	return `
version: 2
key: ` + key + `
name: Two Regions
orientation: pointy
void: opaque
regions:
  - id: entrance
    name: Entrance
    archetype: test
    lighting: { intensity: 1 }
    cells:
      - [[0,0],[1,0],[2,0],[3,0],[4,0],[5,0]]
      - [[0,1],[1,1],[2,1],[3,1],[4,1],[5,1]]
      - [[0,2],[1,2],[2,2],[3,2],[4,2],[5,2]]
      - [[0,3],[1,3],[2,3],[3,3],[4,3],[5,3]]
      - [[0,4],[1,4],[2,4],[3,4],[4,4],[5,4]]
      - [[0,5],[1,5],[2,5],[3,5],[4,5],[5,5]]
  - id: hall
    name: Hall
    archetype: test
    lighting: { intensity: 1 }
    cells:
      - [[6,0],[7,0],[8,0],[9,0],[10,0],[11,0]]
      - [[6,1],[7,1],[8,1],[9,1],[10,1],[11,1]]
      - [[6,2],[7,2],[8,2],[9,2],[10,2],[11,2]]
      - [[6,3],[7,3],[8,3],[9,3],[10,3],[11,3]]
      - [[6,4],[7,4],[8,4],[9,4],[10,4],[11,4]]
      - [[6,5],[7,5],[8,5],[9,5],[10,5],[11,5]]
start: [1, 3]
place:
  - { ref: "dnd5e:monsters:skeleton", at: ` + monsterAt + ` }
`
}

// TestNoSeatIsAlsoSomebodyElsesCell guards a guarantee this package DEPENDS ON
// and does not own: the compiler's seat list excludes cells a monster already
// stands on. If it ever stopped, StartEncounter would seat a player on top of
// a skeleton and the session's Join would refuse with a message that points
// nowhere near the compiler.
//
// The tomb's own garrison is in other chambers, so the tomb alone would never
// notice. This uses a dungeon authored specifically to put a monster in the
// entrance the party starts in.
func TestNoSeatIsAlsoSomebodyElsesCell(t *testing.T) {
	monsterInTheEntrance := twoRegions("seat-collision", "[0, 3]")

	d, err := Compile([]byte(monsterInTheEntrance))
	require.NoError(t, err)
	require.Len(t, d.Monsters, 1)

	for i, seat := range d.PartySeats {
		require.NotEqualf(t, d.Monsters[0].At, seat, "seat %d is the monster's cell", i)
	}
}

// TestAPartyStartOnTopOfAMonsterIsRefusedByTheCompiler is the same concern one
// cell over, and the compiler answers this one with a refusal rather than by
// omitting a seat -- an author who put the party's entry cell under a monster
// made a mistake, and it is named as one.
func TestAPartyStartOnTopOfAMonsterIsRefusedByTheCompiler(t *testing.T) {
	monsterOnTheStart := twoRegions("start-collision", "[1, 3]")

	d, err := Compile([]byte(monsterOnTheStart))
	require.Error(t, err)
	require.Nil(t, d)
}

// TestABrokenSpecIsRefusedRatherThanPartlyBuilt checks the failure direction:
// compile returns an error and no Dungeon, so a caller can never seed a session
// from a dungeon that did not fully compile.
func TestABrokenSpecIsRefusedRatherThanPartlyBuilt(t *testing.T) {
	d, err := Compile([]byte("version: 2\nkey: nope\n"))
	require.Error(t, err)
	require.Nil(t, d)
	require.ErrorIs(t, err, tkdungeonspec.ErrBadSpec)

	var verr *tkdungeonspec.ValidationError
	require.ErrorAs(t, err, &verr, "a validation failure carries every defect with its path")
	require.NotEmpty(t, verr.Errors)
}

// TestVersionOneIsRefusedByName: the deleted dialect is refused, not parsed
// hopefully (rpg-project#256 ruling 4).
func TestVersionOneIsRefusedByName(t *testing.T) {
	d, err := Compile([]byte("version: 1\nkey: old\nvoid: opaque\norientation: pointy\nheight: 8\nstart: [1, 3]\nrooms:\n  - id: a\n    width: 6\n"))
	require.Error(t, err)
	require.Nil(t, d)
	require.ErrorIs(t, err, tkdungeonspec.ErrBadSpec)
}
