package sessionworld

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// shenanigans_test.go covers the first shenanigan's authoring half
// (rpg-project#454): a monster placement may price the check a character has
// to beat to frighten it, and say what the world learns when one lands.
//
// It compiles the REAL content/reference-minds.yaml rather than a fixture,
// for armed_minds_test.go's reason: the thing under test is the file the
// walk plays, and a fixture would not be it.
type ShenanigansSuite struct {
	suite.Suite

	minds *Dungeon
}

func TestShenanigansSuite(t *testing.T) { suite.Run(t, new(ShenanigansSuite)) }

func (s *ShenanigansSuite) SetupTest() {
	raw, err := os.ReadFile(referenceMindsPath)
	s.Require().NoError(err, "the shipped minds dungeon must exist at content/reference-minds.yaml")
	minds, err := Compile(raw)
	s.Require().NoError(err, "the shipped minds dungeon must compile")
	s.minds = minds
}

func (s *ShenanigansSuite) monster(ref string) Monster {
	s.T().Helper()
	for _, m := range s.minds.Monsters {
		if m.Ref == ref {
			return m
		}
	}
	s.Require().Failf("no such monster", "the minds dungeon places no %s", ref)
	return Monster{}
}

// TestTheAuthoredCheckSurvivesTheCompile is the acceptance case for the
// authoring half: a placement that priced `intimidate:` reaches this package
// with the author's own route and number, uninterpreted.
func (s *ShenanigansSuite) TestTheAuthoredCheckSurvivesTheCompile() {
	thug := s.monster("dnd5e:monsters:thug")

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
		s.Empty(m.OnIntimidated, "%s plants no fact, so cowing it teaches the world nothing", m.Ref)
		checked++
	}
	require.Positive(s.T(), checked, "the file must still place monsters the author priced nothing on")
}

// TestNoPlacementInTheMindsDungeonPlantsAFact pins what this file does NOT
// author. The `on: { intimidated: … }` half belongs to the raider camp's
// dispositions, not to a tomb with no factions in it, and the walk for this
// slice is the mind's half alone.
func (s *ShenanigansSuite) TestNoPlacementInTheMindsDungeonPlantsAFact() {
	for _, m := range s.minds.Monsters {
		s.Empty(m.OnIntimidated, "%s", m.Ref)
	}
}
