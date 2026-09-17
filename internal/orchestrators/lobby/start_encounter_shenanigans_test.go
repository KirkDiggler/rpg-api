package lobby

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"

	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
)

// start_encounter_shenanigans_test.go is the launch half of rpg-project#454
// and rpg-project#458: what it takes to lean on a monster or talk it round,
// and what the creature DOES about either, have to cross the seam into
// session.Spawn — or the file says something the board never hears.
//
// It is the same claim start_encounter_armed_test.go makes for `actions:`,
// and it needed making again because the failure is silent in every
// direction: a launch that drops an approach list leaves every attempt
// resolving against the derived DC, and one that drops the answer table
// leaves a goblin standing silent where the author wrote it a scene. Nothing
// anywhere says the file was ignored.

// TestSocialApproachesOf_SpellsTheRouteWithoutInterpretingIt pins the one
// piece of new logic at this boundary: field for field, in order, nothing
// read and nothing decided.
func TestSocialApproachesOf_SpellsTheRouteWithoutInterpretingIt(t *testing.T) {
	out := socialApproachesOf([]tkencounter.CheckApproach{
		{Ability: "intimidation", DC: 12},
		{Ability: "str", Tool: "dnd5e:item:brass-knuckles", DC: 15},
	})

	require.Len(t, out, 2, "every authored route crosses, not just the first")
	require.Equal(t, "intimidation", out[0].Ability)
	require.Empty(t, out[0].Tool, "a route that names no tool crosses naming none")
	require.Equal(t, 12, out[0].DC)
	// ORDER IS THE AUTHOR'S and nothing here sorts it. The resolver picks the
	// checker's best route; which one that is depends on the sheet, not on
	// the order — but a converter that reordered would still be rewriting
	// what the author wrote.
	require.Equal(t, "str", out[1].Ability)
	require.Equal(t, "dnd5e:item:brass-knuckles", out[1].Tool)
	require.Equal(t, 15, out[1].DC)
}

// TestSocialApproachesOf_NilStaysNil is the load-bearing case, not an
// edge. Absent does not mean "no check": the rulebook derives one from the
// monster's own passive Insight at threat time, so a goblin is DC 9 and a
// thug DC 10 without anybody authoring a number.
//
// AN EMPTY NON-NIL SLICE WOULD BE A DIFFERENT AND MUCH WORSE CLAIM — a
// monster priced with no way through, which is a threat nothing can ever
// land. The distinction is invisible at a glance and total in effect.
func TestSocialApproachesOf_NilStaysNil(t *testing.T) {
	require.Nil(t, socialApproachesOf(nil))
}

// TestTheLaunchForwardsEveryShenaniganField guards the three assignments the
// tests above cannot reach: a converter that works perfectly is worth nothing
// if StartEncounter never calls it.
//
// STRUCTURAL ON PURPOSE. Observing the authored DC through a live launch
// means driving a real threat — an actor on their turn, in sight of the thug,
// with the action to spend — and the number it would prove is the one Kirk
// reads off the beat on the walk anyway. What this catches instead is the
// regression that walk would not: somebody dropping a field while moving this
// call, which compiles, passes every other test, and silently returns every
// monster to its derived DC.
func TestTheLaunchForwardsEveryShenaniganField(t *testing.T) {
	const launch = "start_encounter_session_stack.go"

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, launch, nil, 0)
	require.NoError(t, err, "parse the launch that spawns every authored monster")

	read := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "monster" {
			read[sel.Sel.Name] = true
		}
		return true
	})

	require.True(t, read["Actions"],
		"the launch reads monster.Actions -- if not, this test is looking at the wrong file")
	require.True(t, read["Intimidate"],
		"the launch dropped monster.Intimidate: every authored DC silently reverts to the derived one")
	require.True(t, read["Persuade"],
		"the launch dropped monster.Persuade: every authored appeal DC silently reverts to the derived one")
	require.True(t, read["Answers"],
		"the launch dropped monster.Answers: the creature says nothing, teaches nothing and never runs")
}

// TestAnswersOf_CarriesEveryKeyAndEntryInTheAuthorsOrder pins the table's own
// crossing (rpg-project#458): key for key, entry for entry, in the order the
// author wrote them.
//
// ORDER IS LOAD-BEARING HERE IN A WAY IT IS NOT FOR APPROACHES. `entry` on the
// answered beat is an INDEX into this list, and a builder highlights that line
// in the file the author is looking at, so a converter that reordered would
// renumber every line of the log against the file it names.
func TestAnswersOf_CarriesEveryKeyAndEntryInTheAuthorsOrder(t *testing.T) {
	out := answersOf(map[string][]tkencounter.Answer{
		"intimidated": {
			{Weight: 70, Say: "Fine, fine! The cellar door is behind the barrels.", Fact: "goblin-cowed"},
			{Weight: 30, Say: "Boss! BOSS!", Flee: true},
		},
		"persuade_failed": {
			{Weight: 1, Say: "Nothing down there, friend. Go right."},
		},
	})

	require.Len(t, out, 2, "every authored outcome key crosses")

	threatened := out["intimidated"]
	require.Len(t, threatened, 2)
	require.Equal(t, 70, threatened[0].Weight)
	require.Equal(t, "Fine, fine! The cellar door is behind the barrels.", threatened[0].Say,
		"the author's line crosses VERBATIM; nothing here composes or trims one")
	require.Equal(t, "goblin-cowed", threatened[0].Fact)
	require.False(t, threatened[0].Flee, "an entry carries exactly one word and this one teaches")
	require.Equal(t, 30, threatened[1].Weight)
	require.True(t, threatened[1].Flee)
	require.Empty(t, threatened[1].Fact, "a creature that runs teaches nobody anything")

	// An entry that only SPEAKS is a legitimate authored entry, not a
	// half-filled one: the goblin's bad directions are a line and nothing
	// else, and the trap they lead to is authored elsewhere as an arrival.
	quiet := out["persuade_failed"]
	require.Len(t, quiet, 1)
	require.Equal(t, "Nothing down there, friend. Go right.", quiet[0].Say)
	require.Empty(t, quiet[0].Fact)
	require.False(t, quiet[0].Flee)
}

// TestAnswersOf_NilStaysNil is the ordinary case and the load-bearing one.
// A creature the author wrote no `on:` for answers nothing and the world rolls
// NO DIE AT ALL -- distinct from an empty non-nil map, which would claim a
// table exists that nothing can ever fire from.
func TestAnswersOf_NilStaysNil(t *testing.T) {
	require.Nil(t, answersOf(nil))
}
