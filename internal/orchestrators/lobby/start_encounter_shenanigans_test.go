package lobby

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"

	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
)

// start_encounter_shenanigans_test.go is the launch half of rpg-project#454:
// what it takes to frighten a monster, and what the world learns if somebody
// does, have to cross the seam into session.Spawn — or the file says
// something the board never hears.
//
// It is the same claim start_encounter_armed_test.go makes for `actions:`,
// and it needed making twice because the failure is silent in both
// directions: a launch that drops the list leaves every threat resolving
// against the derived DC, and nothing anywhere says the authored number was
// ignored.

// TestIntimidateApproachesOf_SpellsTheRouteWithoutInterpretingIt pins the one
// piece of new logic at this boundary: field for field, in order, nothing
// read and nothing decided.
func TestIntimidateApproachesOf_SpellsTheRouteWithoutInterpretingIt(t *testing.T) {
	out := intimidateApproachesOf([]tkencounter.CheckApproach{
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

// TestIntimidateApproachesOf_NilStaysNil is the load-bearing case, not an
// edge. Absent does not mean "no check": the rulebook derives one from the
// monster's own passive Insight at threat time, so a goblin is DC 9 and a
// thug DC 10 without anybody authoring a number.
//
// AN EMPTY NON-NIL SLICE WOULD BE A DIFFERENT AND MUCH WORSE CLAIM — a
// monster priced with no way through, which is a threat nothing can ever
// land. The distinction is invisible at a glance and total in effect.
func TestIntimidateApproachesOf_NilStaysNil(t *testing.T) {
	require.Nil(t, intimidateApproachesOf(nil))
}

// TestTheLaunchForwardsBothShenaniganFields guards the two assignments the
// test above cannot reach: a converter that works perfectly is worth nothing
// if StartEncounter never calls it.
//
// STRUCTURAL ON PURPOSE. Observing the authored DC through a live launch
// means driving a real threat — an actor on their turn, in sight of the thug,
// with the action to spend — and the number it would prove is the one Kirk
// reads off the beat on the walk anyway. What this catches instead is the
// regression that walk would not: somebody dropping a field while moving this
// call, which compiles, passes every other test, and silently returns every
// monster to its derived DC.
func TestTheLaunchForwardsBothShenaniganFields(t *testing.T) {
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
	require.True(t, read["OnIntimidated"],
		"the launch dropped monster.OnIntimidated: cowing a monster silently teaches the world nothing")
}
