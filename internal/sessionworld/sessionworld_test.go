package sessionworld

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

type ReferenceTombSuite struct {
	suite.Suite

	tomb *Dungeon
}

func TestReferenceTombSuite(t *testing.T) {
	suite.Run(t, new(ReferenceTombSuite))
}

func (s *ReferenceTombSuite) SetupTest() {
	tomb, err := ReferenceTomb()
	s.Require().NoError(err, "the shipped tomb must compile")
	s.tomb = tomb
}

// load rebuilds a live encounter from the world this package produced, which is
// exactly what session.StartSession does with it. Doing it here is how the
// tests ask questions in absolute space -- which region owns a cell, what state
// a door is in -- without this package having to expose either.
func (s *ReferenceTombSuite) load() *tkencounter.Encounter {
	enc, err := tkencounter.LoadEncounter(&tkencounter.LoadEncounterInput{
		Data:       *s.tomb.World,
		Initiative: orderAsGiven{}, Standing: nobodyDown{}, Sight: nobodySees{},
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
// The tomb's authored start is the entrance's local cell [1,3]. Its absolute
// cell is (0,3) -- not (1,3) -- because an offset rectangle SHEARS when it
// becomes axial. Any change that passes the authored cell straight through,
// or reimplements the projection as "local plus origin", produces (1,3) or
// (7,3) and fails here.
//
// The literal is the point rather than an embarrassment: reading the expected
// value back out of the same projection under test would assert nothing at all.
// It has MOVED TWICE, and that is the literal earning its keep. It was (1,-4)
// until rpg-toolkit#1141 corrected spatial's hex offset schemes (pointy-top
// is odd-r, so the COLUMN shears with the row: q = 1 - (3-1)/2 = 0), then
// (0,-3) until rpg-toolkit#1150 corrected the axial basis (R is cube Z, the
// row itself, so r = 3 -- not -q-r). Each value was read off the corrected
// toolkit, not derived by hand -- this package borrows the projection
// precisely so it never has its own opinion about it.
func (s *ReferenceTombSuite) TestPlacementsAreProjectedThroughTheCompositionNotCopied() {
	authored := spatial.Position{X: 1, Y: 3}
	s.Require().NotEqual(authored, s.tomb.PartySeats[0],
		"an authored cell that survived unchanged means nothing projected it")
	s.Equal(spatial.Position{X: 0, Y: 3}, s.tomb.PartySeats[0],
		"the entrance's local [1,3] is the absolute cell (0,3)")
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

// TestTheAuthorsWordsAboutMonstersSurviveTheCompile pins the two facts this
// package carries but cannot yet act on (see Monster.Boss and
// Monster.Targeting). They are asserted so that dropping them is a test
// failure rather than a silent narrowing nobody notices until the wave that
// needs them.
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

// TestTheTombIsShutAtDCTwelve pins the authored lock. Twelve is a literal on
// purpose for the reason the projection's literal is: it is the authored fact
// under test, so reading it back out of the compiler would be circular.
func (s *ReferenceTombSuite) TestTheTombIsShutAtDCTwelve() {
	enc := s.load()
	doors := enc.Doors()
	s.Require().Len(doors, 1, "the tomb authors exactly one door -- the entrance/hall connector is an open gap")
	s.Equal(tkencounter.DoorStateKind("locked"), doors[0].State.Kind(), "and it starts shut")

	_, err := enc.OpenDoor(&tkencounter.OpenDoorInput{Door: doors[0].ID})
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

// TestNoSeatIsAlsoSomebodyElsesCell guards a guarantee this package DEPENDS ON
// and does not own.
//
// projectPlacements puts every party seat and every monster into ONE throwaway
// encounter to read their absolute cells back. That is only safe because the
// compiler's seat list excludes cells that are already occupied -- if it ever
// stopped, this package would start failing at construction with an error about
// a member collision, and nothing about the message would point at the compiler.
//
// The tomb's own garrison is in other chambers, so the tomb alone would never
// notice. This uses a dungeon authored specifically to put a monster in the
// entrance the party starts in.
func TestNoSeatIsAlsoSomebodyElsesCell(t *testing.T) {
	const monsterInTheEntrance = `
version: 1
key: seat-collision
void: opaque
orientation: pointy
height: 6
start: [1, 3]
rooms:
  - id: entrance
    width: 6
    place:
      - { ref: "dnd5e:monsters:skeleton", at: [0, 3] }
  - id: hall
    width: 6
connectors:
  - { from: entrance, to: hall }
`

	d, err := compile([]byte(monsterInTheEntrance))
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
	const monsterOnTheStart = `
version: 1
key: start-collision
void: opaque
orientation: pointy
height: 6
start: [1, 3]
rooms:
  - id: entrance
    width: 6
    place:
      - { ref: "dnd5e:monsters:skeleton", at: [1, 3] }
  - id: hall
    width: 6
connectors:
  - { from: entrance, to: hall }
`

	d, err := compile([]byte(monsterOnTheStart))
	require.Error(t, err)
	require.Nil(t, d)
}

// TestABrokenSpecIsRefusedRatherThanPartlyBuilt checks the failure direction:
// compile returns an error and no Dungeon, so a caller can never seed a session
// from a dungeon that did not fully compile.
func TestABrokenSpecIsRefusedRatherThanPartlyBuilt(t *testing.T) {
	d, err := compile([]byte("version: 1\nkey: nope\n"))
	require.Error(t, err)
	require.Nil(t, d)
}
