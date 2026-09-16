package sessionworld

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
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

// TestTheAuthoredCheckCannotReachTheRunYet is the gap, pinned where it is
// visible rather than left as a comment nobody runs.
//
// session.SpawnInput is the only way a monster's static facts cross into the
// live encounter, and on the pinned toolkit branch it has no Intimidate and
// no OnIntimidated field -- so the launch has nothing to forward, and the
// thug's authored DC 12 above loses to its derived 10 in an actual fight.
// The composition end is built (encounter.MemberInput.Intimidate exists and
// session.Manager.Intimidate reads it at roll time); the missing link is one
// field on SpawnInput plus one line in session's own Join call
// (rpg-toolkit#1790).
//
// THIS TEST IS MEANT TO FAIL WHEN THAT LANDS. Its failure is the signal to
// forward both fields from internal/orchestrators/lobby's StartEncounter
// beside Actions and Faction, and then to delete this test -- not to widen
// it. Until then it stops "the author can price a threat" from being claimed
// end to end when only half of it is true.
func (s *ShenanigansSuite) TestTheAuthoredCheckCannotReachTheRunYet() {
	typ := reflect.TypeOf(sdk.SpawnInput{})
	for _, name := range []string{"Intimidate", "OnIntimidated"} {
		_, found := typ.FieldByName(name)
		s.Falsef(found,
			"session.SpawnInput grew %s: forward it from the launch beside Actions and delete this test", name)
	}
}

// TestTheLaunchStillForwardsNothingShenaniganish is the same gap asked of the
// caller rather than the type, because a field can exist and go unread. It
// parses the launch's own source for the two field names, which is a direct
// answer to "does StartEncounter hand these to Spawn" independent of whether
// the SDK has somewhere to put them.
func (s *ShenanigansSuite) TestTheLaunchStillForwardsNothingShenaniganish() {
	const launch = "../orchestrators/lobby/start_encounter_session_stack.go"

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, launch, nil, 0)
	s.Require().NoError(err, "parse the launch that spawns every authored monster")

	mentioned := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "monster" {
			mentioned[sel.Sel.Name] = true
		}
		return true
	})

	s.True(mentioned["Actions"], "the launch reads monster.Actions -- if not, this test is looking at the wrong file")
	s.False(mentioned["Intimidate"],
		"the launch reads monster.Intimidate: the gap closed, so drop this test and the sibling above")
	s.False(mentioned["OnIntimidated"], "same, for the world half")
}
