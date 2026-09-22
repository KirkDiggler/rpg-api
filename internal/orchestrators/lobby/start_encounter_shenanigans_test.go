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
	require.True(t, read["Table"],
		"the launch dropped monster.Table: the creature answers nothing the author wrote and fights "+
			"only from its kind's default")
	require.True(t, read["Temper"],
		"the launch dropped monster.Temper: four goblins off one mix all come out soldiers")
}

// TestTheLaunchHandsTheTableAndTemperOverUNCONVERTED pins the shape of the
// two fields the table brought (rpg-project#465): they are the composition's
// own types on BOTH sides of session.Spawn, so the launch assigns them and
// converts nothing.
//
// THE CONVERTER THIS REPLACED IS THE POINT. `answersOf` existed to respell the
// composition's Answer as the session seam's own, key for key and entry for
// entry, and every word the author's grammar grew had to be added to it or it
// silently crossed as an entry that only speaks. SpawnInput now takes
// encounter.Table directly, so there is nothing to keep in step — and the way
// this stays true is that no such function exists to drift.
//
// STRUCTURAL FOR TestTheLaunchForwardsEveryShenaniganField's REASON: what
// would break this is somebody reintroducing a fold at this boundary, which
// compiles and passes every behavioral test in the package.
func TestTheLaunchHandsTheTableAndTemperOverUNCONVERTED(t *testing.T) {
	const launch = "start_encounter_session_stack.go"

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, launch, nil, 0)
	require.NoError(t, err, "parse the launch that spawns every authored monster")

	var tableExpr, temperExpr ast.Expr
	ast.Inspect(file, func(n ast.Node) bool {
		kv, ok := n.(*ast.KeyValueExpr)
		if !ok {
			return true
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			return true
		}
		switch key.Name {
		case "Table":
			tableExpr = kv.Value
		case "Temper":
			temperExpr = kv.Value
		}
		return true
	})

	require.NotNil(t, tableExpr, "the launch sets SpawnInput.Table")
	require.NotNil(t, temperExpr, "the launch sets SpawnInput.Temper")

	// A SELECTOR, NEVER A CALL. `monster.Table` is the whole right-hand side;
	// a `somethingOf(monster.Table)` here is a second reader of the author's
	// grammar and is exactly what this refuses.
	require.IsType(t, &ast.SelectorExpr{}, tableExpr,
		"SpawnInput.Table is hand-carried off the placement, not passed through a converter")
	require.IsType(t, &ast.SelectorExpr{}, temperExpr,
		"SpawnInput.Temper is hand-carried off the placement, not passed through a converter")
}
