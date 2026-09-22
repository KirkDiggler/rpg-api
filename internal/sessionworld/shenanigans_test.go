package sessionworld

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

// shenanigans_test.go covers the social verbs' authoring half
// (rpg-project#454, rpg-project#458): a monster placement may price the checks
// a character has to beat to lean on it or talk it round, and say what the
// creature DOES about either -- what it says, what it teaches, and whether it
// runs.
//
// It compiles the REAL shipped files rather than fixtures, for
// armed_minds_test.go's reason: the thing under test is the file the walk
// plays, and a fixture would not be it.
type ShenanigansSuite struct {
	suite.Suite

	minds     *Dungeon
	frontRoom *Dungeon
}

// referenceFrontRoomPath is the front room goblin's own shipped file
// (rpg-project#458). It is a fifth dungeon rather than a fifth room on the
// Three Minds, because that file has no `factions:` at all and a scene where
// nothing attacks you needs a faction graph to say so.
var referenceFrontRoomPath = filepath.Join("..", "..", "content", "reference-front-room.yaml")

func TestShenanigansSuite(t *testing.T) { suite.Run(t, new(ShenanigansSuite)) }

func (s *ShenanigansSuite) SetupTest() {
	s.minds = s.compile(referenceMindsPath)
	s.frontRoom = s.compile(referenceFrontRoomPath)
}

func (s *ShenanigansSuite) compile(path string) *Dungeon {
	s.T().Helper()
	raw, err := os.ReadFile(path) //nolint:gosec // a shipped content path, not user input
	s.Require().NoErrorf(err, "the shipped dungeon must exist at %s", path)
	compiled, err := Compile(raw)
	s.Require().NoErrorf(err, "the shipped dungeon at %s must compile", path)
	return compiled
}

func (s *ShenanigansSuite) monsterOf(d *Dungeon, ref string) Monster {
	s.T().Helper()
	for _, m := range d.Monsters {
		if m.Ref == ref {
			return m
		}
	}
	s.Require().Failf("no such monster", "this dungeon places no %s", ref)
	return Monster{}
}

func (s *ShenanigansSuite) placement(d *Dungeon, id string) Monster {
	s.T().Helper()
	for _, m := range d.Monsters {
		if m.PlacementID == id {
			return m
		}
	}
	s.Require().Failf("no such placement", "this dungeon places nothing with id %q", id)
	return Monster{}
}

// TestTheAuthoredCheckSurvivesTheCompile is the acceptance case for the
// authoring half: a placement that priced `intimidate:` reaches this package
// with the author's own route and number, uninterpreted.
func (s *ShenanigansSuite) TestTheAuthoredCheckSurvivesTheCompile() {
	thug := s.monsterOf(s.minds, "dnd5e:monsters:thug")

	s.Require().Len(thug.Intimidate, 1, "the thug prices exactly one route through")
	s.Equal("intimidation", thug.Intimidate[0].Ability,
		"the author's own skill ref, never mapped to something else on the way")
	s.Equal(12, thug.Intimidate[0].DC,
		"and the author's own number, which is not the thug's derived 10")
}

// TestAnUnpricedPlacementCarriesNothing is the control, and it is the half of
// the design that matters most: absent means DERIVED, not ungated. The
// goblins price nothing, so the field is nil here and the rulebook rolls
// against each one's own passive Insight (DC 9) at threat time. A zero value
// invented on this side would be a DC nobody chose.
func (s *ShenanigansSuite) TestAnUnpricedPlacementCarriesNothing() {
	var checked int
	for _, m := range s.minds.Monsters {
		if m.Ref == "dnd5e:monsters:thug" {
			continue
		}
		s.Nil(m.Intimidate, "%s prices no threat and is checked against its own stat block", m.Ref)
		s.Nil(m.Persuade, "%s prices no appeal and is checked against its own stat block", m.Ref)
		s.Nil(m.Table, "%s carries no authored orders, so its kind's default table speaks alone", m.Ref)
		checked++
	}
	require.Positive(s.T(), checked, "the file must still place monsters the author priced nothing on")
}

// TestTheMindsDungeonAuthorsNoOrders pins what that file does NOT author. The
// `on:` half belongs to a dungeon with a faction graph in it, and the Three
// Minds has none.
//
// NIL IS NOT "DOES NOTHING" ANY MORE (rpg-project#465). Every monster in this
// file is driven by the rulebook's default table for its kind, laid on inside
// session.Spawn where a ref can be resolved; what this asserts is that the
// AUTHOR added nothing over it, which is why the file still reads as a plain
// tomb.
func (s *ShenanigansSuite) TestTheMindsDungeonAuthorsNoOrders() {
	for _, m := range s.minds.Monsters {
		s.Nil(m.Table, "%s", m.Ref)
		s.Empty(m.Temper.Word, "%s is given no temperament, which is a soldier", m.Ref)
		s.Empty(m.Temper.Mix, "%s is in no faction with a mix to be dealt from", m.Ref)
	}
}

// TestTheFrontRoomGoblinPricesBothVerbs is the front room's own acceptance
// case (rpg-project#458). BOTH numbers are the author's and NEITHER is the
// derived one: a goblin's own passive Insight is 9, the threat is priced
// HARDER at 12 and the appeal EASIER at 10.
//
// THE ASYMMETRY IS THE AUTHORING TOOL WORKING. "This one would rather be
// talked to than shouted at" is a sentence about a creature that no stat block
// can express and that the engine now hears.
func (s *ShenanigansSuite) TestTheFrontRoomGoblinPricesBothVerbs() {
	goblin := s.placement(s.frontRoom, "front-goblin")

	s.Require().Len(goblin.Intimidate, 1)
	s.Equal("intimidation", goblin.Intimidate[0].Ability)
	s.Equal(12, goblin.Intimidate[0].DC, "harder than its derived 9")

	s.Require().Len(goblin.Persuade, 1)
	s.Equal("persuasion", goblin.Persuade[0].Ability)
	s.Equal(10, goblin.Persuade[0].DC, "easier than the threat, which is the author saying something")
}

// TestTheFrontRoomTableSurvivesTheCompile is the table's own acceptance case:
// four outcome keys, the weights the author wrote, the lines VERBATIM, and one
// word per entry.
//
// THE LINES ARE ASSERTED IN FULL, not by prefix. `say` is the one field in
// this whole slice the engine is forbidden to compose, and a test that
// matched loosely would not notice a converter that trimmed, escaped or
// re-cased what the author typed.
func (s *ShenanigansSuite) TestTheFrontRoomTableSurvivesTheCompile() {
	answers := s.placement(s.frontRoom, "front-goblin").Table
	s.Require().NotNil(answers,
		"the front room goblin's whole point is its table, and it INHERITS one from its faction")

	// A landed threat: 70/30, and the 30 is the gamble -- the goblin you
	// frightened away is one you can no longer talk to.
	threatened := answers[tkencounter.AnswerIntimidated]
	s.Require().Len(threatened, 2)
	s.Equal(70, threatened[0].Weight)
	s.Equal("Fine! FINE. The cellar door is behind the barrels. Just don't.", threatened[0].Say)
	s.Equal(tkencounter.FactID("goblin-cowed"), threatened[0].Fact)
	s.False(threatened[0].Flee, "an entry carries exactly one word and this one teaches")
	s.Equal(30, threatened[1].Weight)
	s.Equal("Boss! BOSS!", threatened[1].Say)
	s.True(threatened[1].Flee)
	s.Empty(threatened[1].Fact, "a creature that runs teaches nobody anything")

	// A missed threat: one entry, no word. It speaks and nothing else
	// happens, and an omitted weight compiles to 1.
	missed := answers[tkencounter.AnswerIntimidateFailed]
	s.Require().Len(missed, 1)
	s.Equal(1, missed[0].Weight, "an omitted weight is 1, not 0 and not 100")
	s.Equal("Big talk, for someone standing in my doorway.", missed[0].Say)
	s.Empty(missed[0].Fact)
	s.False(missed[0].Flee)

	// A landed appeal: the truth, and NOT `goblin-cowed`. Talking somebody
	// round is not frightening them, and the disposition flips on the
	// threat's fact alone -- a shared fact here would make the two verbs one
	// verb with two names.
	persuaded := answers[tkencounter.AnswerPersuaded]
	s.Require().Len(persuaded, 1)
	s.Equal("Bandits took the cellar. Go left at the rope, and mind the third step.", persuaded[0].Say)
	s.Empty(persuaded[0].Fact, "an appeal that lands teaches the party nothing the world reads")

	// A failed appeal: THE TRAP. The lie is a fact like any other.
	failed := answers[tkencounter.AnswerPersuadeFailed]
	s.Require().Len(failed, 1)
	s.Equal("Cellar's empty, friend. Nothing down there but rats. Straight on through.", failed[0].Say)
	s.Equal(tkencounter.FactID("cellar-is-clear"), failed[0].Fact)
}

// TestTheGoblinsLieIsWhatBringsTheBandits is the scenario's own wiring, and
// the reason it can be tested at all is that a false fact is an ID AND NOTHING
// ELSE. There is no truth bit anywhere -- not on the table, not on the beat --
// so "the goblin lied" is expressed entirely by which placement reads the
// fact it minted.
//
// IT IS ASSERTED AS A JOIN, not as two separate facts about two placements:
// the bug this catches is an author renaming one end and not the other, which
// leaves a lie nobody ever walks into and a trap that never springs.
func (s *ShenanigansSuite) TestTheGoblinsLieIsWhatBringsTheBandits() {
	lie := s.placement(s.frontRoom, "front-goblin").
		Table[tkencounter.AnswerPersuadeFailed][0].Fact
	s.Require().NotEmpty(lie, "the failed appeal must teach something for anything to read")

	for _, id := range []string{"bandit-1", "bandit-2"} {
		bandit := s.placement(s.frontRoom, id)
		arrival, ok := bandit.Arrives.(tkencounter.TriggerFact)
		s.Require().Truef(ok, "%s must arrive on a fact, not on a round or a fall: %T", id, bandit.Arrives)
		s.Equalf(lie, arrival.Fact,
			"%s arrives on the exact fact the goblin's bad directions teach", id)
	}
}

// TestTheGoblinIsNotOnTheBanditsSide pins the scene itself. The goblin and the
// bandits are DIFFERENT factions, which is what lets one stand in front of you
// without a fight while the other two are hostile -- one faction would make
// the neutral goblin hostile the moment the bandits were.
func (s *ShenanigansSuite) TestTheGoblinIsNotOnTheBanditsSide() {
	s.Equal("goblins", s.placement(s.frontRoom, "front-goblin").Faction)
	s.Equal("bandits", s.placement(s.frontRoom, "bandit-1").Faction)
	s.Equal("bandits", s.placement(s.frontRoom, "bandit-2").Faction)
}

// TestTheBanditsArePricedForNothingAndSayNothing is the front room's own
// control: only the GOBLINS are a conversation. A bandit priced with a DC
// would quietly claim you can talk down the ambush you walked into, and one
// with a social key on its table would be a second scene nobody designed.
//
// ITS TABLE IS NOT EMPTY ANY MORE, and that is a different claim: the bandits
// carry a `time` standing order and nothing else (rpg-project#465). What this
// pins is that the standing order did not bring a conversation with it.
func (s *ShenanigansSuite) TestTheBanditsArePricedForNothingAndSayNothing() {
	for _, id := range []string{"bandit-1", "bandit-2"} {
		bandit := s.placement(s.frontRoom, id)
		s.Nilf(bandit.Intimidate, "%s prices no threat", id)
		s.Nilf(bandit.Persuade, "%s prices no appeal", id)
		for _, key := range tkencounter.AnswerKeys {
			s.Nilf(bandit.Table[key], "%s answers nothing to %s", id, key)
		}
	}
}

// TestTheFourGoblinsShareOneTable is the slice's own claim, in the file
// (rpg-project#465): four creatures, one set of orders, and the orders live on
// the FACTION so that sentence is true by construction rather than by an
// author copying a block four times.
//
// IDENTITY, NOT EQUIVALENCE. The compiler lays each placement's own `on:` over
// its faction's, and none of these four writes one — so what every goblin
// carries is the faction's table entry for entry. A test that compared only
// lengths would pass against four blocks that had drifted apart.
func (s *ShenanigansSuite) TestTheFourGoblinsShareOneTable() {
	ids := []string{"front-goblin", "front-goblin-2", "front-goblin-3", "front-goblin-4"}
	first := s.placement(s.frontRoom, ids[0]).Table
	s.Require().NotNil(first)

	for _, id := range ids[1:] {
		s.Equalf(first, s.placement(s.frontRoom, id).Table,
			"%s answers from the same authored table as %s", id, ids[0])
	}
}

// TestOnlyTheNamedGoblinIsPriced is the control beside it. The three siblings
// exist so the walk can watch ONE variable — the die each was dealt — and a
// DC written on any of them would be a second one.
//
// NIL IS DERIVED, NOT UNGATED: each of the three is checked against its own
// stat block's passive Insight, which for a goblin is 9.
func (s *ShenanigansSuite) TestOnlyTheNamedGoblinIsPriced() {
	for _, id := range []string{"front-goblin-2", "front-goblin-3", "front-goblin-4"} {
		sibling := s.placement(s.frontRoom, id)
		s.Nilf(sibling.Intimidate, "%s prices no threat and is checked against its own stat block", id)
		s.Nilf(sibling.Persuade, "%s prices no appeal and is checked against its own stat block", id)
	}
}

// TestNeitherFactionAuthorsATemperamentPerCreature pins the deal (design §3,
// R5): every creature in this file gets its temperament from its faction's
// MIX, and no placement names a word.
//
// THE MIX CROSSES UNDEALT. dungeonspec carries the faction's spread and the
// composition throws it at spawn, with the faction as the die's entity, so the
// streamer sees which goblin came out the coward. A placement that named a
// word would be dealt nothing and raise no beat — which is the right answer
// for an author who has already decided, and the wrong one for this walk.
//
// AND THE PROFILES ARE NOT FILLED HERE. What `coward` MEANS in numbers is
// rulebook content, looked up inside session.Spawn; a profile filled on this
// side would be rpg-api naming a rules value.
func (s *ShenanigansSuite) TestNeitherFactionAuthorsATemperamentPerCreature() {
	want := map[string]map[string]int{
		"front-goblin":   {"coward": 2, "soldier": 1, "aggressive": 1},
		"front-goblin-2": {"coward": 2, "soldier": 1, "aggressive": 1},
		"front-goblin-3": {"coward": 2, "soldier": 1, "aggressive": 1},
		"front-goblin-4": {"coward": 2, "soldier": 1, "aggressive": 1},
		"bandit-1":       {"coward": 1, "soldier": 2, "aggressive": 1},
		"bandit-2":       {"coward": 1, "soldier": 2, "aggressive": 1},
	}
	for id, mix := range want {
		temper := s.placement(s.frontRoom, id).Temper
		s.Equalf(mix, temper.Mix, "%s is dealt from its faction's mix", id)
		s.Emptyf(temper.Word, "%s names no word of its own, or the mix is not dealt for it", id)
		s.Equalf(tkencounter.TemperProfile{}, temper.Profile,
			"%s crosses this package with no profile: what a word means is the rulebook's", id)
	}
}

// TestTheBanditsWalkToTheFrontRoom pins the standing order (rpg-project#465
// §2): an arrival that should go somewhere is a `time` entry with a `when`,
// not a memory and not an `arrived` trigger.
//
// THE CELL IS ASSERTED EXACTLY. `toward: { at: … }` is walked ONTO rather than
// up to, so an author who names an occupied cell has written a walk that
// cannot finish; [3, 3] is chosen clear of the goblin at [4, 3] and the party's
// own seat at [1, 3], and a change to either that forgot this line would be
// invisible on the board until somebody watched a bandit stop short.
//
// IT IS THE LAST LINE, and the band it sits in is the assertion. The five
// above it are the rulebook's default copied in, so the entry that fires when
// a bandit has nobody in sight is this one -- and `enemy: none` is what makes
// the four exclusive bands hand it over.
func (s *ShenanigansSuite) TestTheBanditsWalkToTheFrontRoom() {
	for _, id := range []string{"bandit-1", "bandit-2"} {
		orders := s.placement(s.frontRoom, id).Table[tkencounter.AnswerTime]
		s.Require().NotEmptyf(orders, "%s has a time table", id)

		entry := orders[len(orders)-1]
		s.Require().NotNilf(entry.Toward, "%s walks toward something", id)
		s.Require().NotNilf(entry.Toward.At, "%s walks toward an authored CELL, not a member", id)
		s.Equalf(spatial.Position{X: 3, Y: 3}, *entry.Toward.At,
			"%s walks onto the front room cell the file names", id)
		s.Require().NotNilf(entry.When, "%s only walks when it has nobody to fight", id)
		s.Equalf(tkencounter.EnemyNone, entry.When.Enemy,
			"%s stops walking the moment it is opposed to something it can see", id)
	}
}

// TestTheBanditsCanStillFight is the other half, and it is the reason the
// default's entries are copied onto these two placements at all
// (rpg-project#465 §1, ruled during the build).
//
// A `time` KEY REPLACES THE DEFAULT'S WHOLESALE. There is no merging of entry
// lists, by design — an author never has to reason about what was added to
// what — so a placement that wrote only its standing order would walk west
// beautifully, form a fight the moment it saw somebody, and then stand in it
// with nothing on its table to say about an enemy in reach.
//
// ASSERTED BY BAND, not by entry index. What matters is that every `enemy:`
// band a bandit can be in has an answer, because the bands are exclusive and
// exactly one of them holds at any moment. An entry list that drifted in order
// still passes; one that lost a band does not, which is the failure this
// exists to catch.
func (s *ShenanigansSuite) TestTheBanditsCanStillFight() {
	for _, id := range []string{"bandit-1", "bandit-2"} {
		answered := map[tkencounter.EnemyWord]bool{}
		for _, entry := range s.placement(s.frontRoom, id).Table[tkencounter.AnswerTime] {
			if entry.When != nil && entry.When.Enemy != "" {
				answered[entry.When.Enemy] = true
			}
		}
		for _, band := range tkencounter.EnemyWords {
			s.Truef(answered[band], "%s has an answer for `enemy: %s`", id, band)
		}
	}
}
