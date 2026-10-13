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

// shenanigans_test.go covers Speak's authoring half (rpg-project#454,
// rpg-project#458, migrated to options by rpg-project#569): a monster
// placement may author the lines a character can say to it, the check each
// line has to beat, and what the creature DOES about it -- what it says, what
// it teaches, and whether it runs.
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
	for _, m := range monstersOf(d) {
		if m.Ref == ref {
			return m
		}
	}
	s.Require().Failf("no such monster", "this dungeon places no %s", ref)
	return Monster{}
}

func (s *ShenanigansSuite) placement(d *Dungeon, id string) Monster {
	s.T().Helper()
	for _, m := range monstersOf(d) {
		if m.PlacementID == id {
			return m
		}
	}
	s.Require().Failf("no such placement", "this dungeon places nothing with id %q", id)
	return Monster{}
}

// optionOf finds one authored line on a compiled placement by its id.
func (s *ShenanigansSuite) optionOf(m Monster, id string) tkencounter.Option {
	s.T().Helper()
	for _, o := range m.Options {
		if o.ID == id {
			return o
		}
	}
	s.Require().Failf("no such option", "%s authors no option %q", m.PlacementID, id)
	return tkencounter.Option{}
}

// TestTheAuthoredCheckSurvivesTheCompile is the acceptance case for the
// authoring half: the thug's one line reaches this package with the author's
// own route and number, uninterpreted (rpg-project#569). It was a priced
// `intimidate:` with no table; it is one option at the same DC and no rows.
func (s *ShenanigansSuite) TestTheAuthoredCheckSurvivesTheCompile() {
	thug := s.monsterOf(s.minds, "dnd5e:monsters:thug")

	s.Require().Len(thug.Options, 1, "the thug authors exactly one line")
	line := thug.Options[0]
	s.Equal("lean-on-it", line.ID)
	s.Equal("Step aside, or I move you.", line.Say, "the line crosses verbatim")
	s.Equal([]tkencounter.CheckApproach{{Ability: "intimidation", DC: 12}}, line.Check,
		"the author's own skill and number, never mapped to something else on the way")
	s.Nil(line.Requires, "the line is offered to everybody")
	s.Empty(line.Success, "a landed threat answers with the deed alone")
	s.Empty(line.Failure)
}

// TestAnUnpricedPlacementCarriesNothing is the control: a creature nobody
// wrote a line for has no options, and it cannot be spoken to at all
// (rpg-project#494 R1). A zero value invented on this side would be a line
// nobody wrote.
func (s *ShenanigansSuite) TestAnUnpricedPlacementCarriesNothing() {
	var checked int
	for _, m := range monstersOf(s.minds) {
		if m.Ref == "dnd5e:monsters:thug" {
			continue
		}
		s.Nil(m.Options, "%s has no lines and cannot be spoken to", m.Ref)
		s.Nil(m.Table, "%s carries no authored orders, so its kind's default table speaks alone", m.Ref)
		checked++
	}
	require.Positive(s.T(), checked, "the file must still place monsters the author wrote no line for")
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
	for _, m := range monstersOf(s.minds) {
		s.Nil(m.Table, "%s", m.Ref)
		s.Empty(m.Temper.Word, "%s is given no temperament, which is a soldier", m.Ref)
		s.Empty(m.Temper.Mix, "%s is in no faction with a mix to be dealt from", m.Ref)
	}
}

// TestTheFrontRoomGoblinOffersTwoLines is the front room's own acceptance
// case (rpg-project#458, migrated to options by rpg-project#569). The threat
// is priced HARDER at 12 and the appeal EASIER at 10 -- the same numbers the
// retired `intimidate:`/`persuade:` keys carried.
//
// THE ASYMMETRY IS THE AUTHORING TOOL WORKING. "This one would rather be
// talked to than shouted at" is a sentence about a creature that no stat block
// can express and that the engine now hears.
func (s *ShenanigansSuite) TestTheFrontRoomGoblinOffersTwoLines() {
	goblin := s.placement(s.frontRoom, "front-goblin")
	s.Require().Len(goblin.Options, 2)

	threat := s.optionOf(goblin, "lean-on-it")
	s.Equal("Where's the cellar? Tell me or you're next.", threat.Say)
	s.Equal([]tkencounter.CheckApproach{{Ability: "intimidation", DC: 12}}, threat.Check)
	s.Nil(threat.Requires)

	appeal := s.optionOf(goblin, "talk-it-round")
	s.Equal("We're not here for you. Where did the bandits go?", appeal.Say)
	s.Equal([]tkencounter.CheckApproach{{Ability: "persuasion", DC: 10}}, appeal.Check,
		"easier than the threat, which is the author saying something")
	s.Nil(appeal.Requires)
}

// TestTheFrontRoomRowsSurviveTheCompile is the rows' own acceptance case: the
// four tables the faction used to carry, now on the option each answered, the
// weights the author wrote, the lines VERBATIM, and one word per entry.
//
// THE LINES ARE ASSERTED IN FULL, not by prefix. `say` is the one field in
// this whole slice the engine is forbidden to compose, and a test that
// matched loosely would not notice a converter that trimmed, escaped or
// re-cased what the author typed.
func (s *ShenanigansSuite) TestTheFrontRoomRowsSurviveTheCompile() {
	goblin := s.placement(s.frontRoom, "front-goblin")
	threat := s.optionOf(goblin, "lean-on-it")
	appeal := s.optionOf(goblin, "talk-it-round")

	// A landed threat: 70/30, and the 30 is the gamble -- the goblin you
	// frightened away is one you can no longer talk to.
	threatened := threat.Success
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
	missed := threat.Failure
	s.Require().Len(missed, 1)
	s.Equal(1, missed[0].Weight, "an omitted weight is 1, not 0 and not 100")
	s.Equal("Big talk, for someone standing in my doorway.", missed[0].Say)
	s.Empty(missed[0].Fact)
	s.False(missed[0].Flee)

	// A landed appeal: the truth, and NOT `goblin-cowed`. Talking somebody
	// round is not frightening them.
	persuaded := appeal.Success
	s.Require().Len(persuaded, 1)
	s.Equal("Bandits took the cellar. Go left at the rope, and mind the third step.", persuaded[0].Say)
	s.Empty(persuaded[0].Fact, "an appeal that lands teaches the party nothing the world reads")

	// A failed appeal: THE TRAP. The lie is a fact like any other.
	failed := appeal.Failure
	s.Require().Len(failed, 1)
	s.Equal(100, failed[0].Weight)
	s.Equal("Cellar's empty, friend. Nothing down there but rats. Straight on through.", failed[0].Say)
	s.Equal(tkencounter.FactID("cellar-is-clear"), failed[0].Fact)
}

// TestTheGoblinsLieIsWhatBringsTheBandits is the scenario's own wiring, and
// the reason it can be tested at all is that a false fact is an ID AND NOTHING
// ELSE. There is no truth bit anywhere -- not on the option, not on the beat --
// so "the goblin lied" is expressed entirely by which placement reads the
// fact it minted.
//
// IT IS ASSERTED AS A JOIN, not as two separate facts about two placements:
// the bug this catches is an author renaming one end and not the other, which
// leaves a lie nobody ever walks into and a trap that never springs.
func (s *ShenanigansSuite) TestTheGoblinsLieIsWhatBringsTheBandits() {
	appeal := s.optionOf(s.placement(s.frontRoom, "front-goblin"), "talk-it-round")
	s.Require().NotEmpty(appeal.Failure, "the failed appeal must have a row for anything to read")
	lie := appeal.Failure[0].Fact
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

// TestTheBanditsSayNothing is the front room's own control: only the GOBLIN
// is a conversation. A bandit with a line would quietly claim you can talk
// down the ambush you walked into, which is a second scene nobody designed.
//
// THEIR TABLE IS NOT EMPTY: the bandits carry a `time` standing order and
// nothing else (rpg-project#465). What this pins is that the standing order
// did not bring a conversation with it.
func (s *ShenanigansSuite) TestTheBanditsSayNothing() {
	for _, id := range []string{"bandit-1", "bandit-2"} {
		bandit := s.placement(s.frontRoom, id)
		s.Nilf(bandit.Options, "%s has no lines", id)
		for key := range bandit.Table {
			s.Equalf(tkencounter.AnswerTime, key, "%s's table answers time and nothing else", id)
		}
	}
}

// TestOnlyTheNamedGoblinHasLines is the control beside the goblin's own case.
// The three siblings exist so the walk can watch ONE variable -- the die each
// was dealt -- and a line written on any of them would be a second one.
//
// THEY ALSO CARRY NO AUTHORED TABLE. The goblins' answers used to be the
// faction's four social `on:` keys, inherited by all four; those keys are
// retired (rpg-project#569) and the answers live on front-goblin's options,
// so each sibling falls through to its kind's default `time` table.
func (s *ShenanigansSuite) TestOnlyTheNamedGoblinHasLines() {
	for _, id := range []string{"front-goblin", "front-goblin-2", "front-goblin-3", "front-goblin-4"} {
		goblin := s.placement(s.frontRoom, id)
		s.Nilf(goblin.Table, "%s authors no table over its kind's default", id)
		if id != "front-goblin" {
			s.Nilf(goblin.Options, "%s has no lines and cannot be spoken to", id)
		}
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
