package sessionworld

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
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
		s.Nil(m.Answers, "%s answers nothing, so the world rolls no die about it", m.Ref)
		checked++
	}
	require.Positive(s.T(), checked, "the file must still place monsters the author priced nothing on")
}

// TestTheMindsDungeonAnswersNothing pins what that file does NOT author. The
// `on:` half belongs to a dungeon with a faction graph in it, and the Three
// Minds has none -- the walk for THAT slice is the mind's half alone.
func (s *ShenanigansSuite) TestTheMindsDungeonAnswersNothing() {
	for _, m := range s.minds.Monsters {
		s.Nil(m.Answers, "%s", m.Ref)
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
	answers := s.placement(s.frontRoom, "front-goblin").Answers
	s.Require().NotNil(answers, "the front room goblin's whole point is its table")

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
		Answers[tkencounter.AnswerPersuadeFailed][0].Fact
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

// TestTheBanditsArePricedForNothingAndAnswerNothing is the front room's own
// control: only ONE creature in this file is a conversation. A bandit that
// arrived with a table would be a second scene nobody designed, and one priced
// with a DC would quietly claim you can talk down the ambush you walked into.
func (s *ShenanigansSuite) TestTheBanditsArePricedForNothingAndAnswerNothing() {
	for _, id := range []string{"bandit-1", "bandit-2"} {
		bandit := s.placement(s.frontRoom, id)
		s.Nilf(bandit.Intimidate, "%s prices no threat", id)
		s.Nilf(bandit.Persuade, "%s prices no appeal", id)
		s.Nilf(bandit.Answers, "%s answers nothing", id)
	}
}
