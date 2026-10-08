package sessionworld

import (
	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

// Monster is a test-side reading of one compiled placement, in the cell frame
// the session speaks. The package forwards Spec whole and re-projects nothing
// (rpg-project#542); these content tests still pin what the authored files
// compile to, so they read the toolkit's placements through this one helper.
type Monster struct {
	Ref         string
	MemberID    string
	At          spatial.Position
	Facing      string
	Boss        bool
	Targeting   string
	Actions     []string
	PlacementID string
	Holds       []string
	Faction     string
	Arrives     tkencounter.Trigger
	Intimidate  []tkencounter.CheckApproach
	Persuade    []tkencounter.CheckApproach
	Table       tkencounter.Table
	Temper      tkencounter.Temper
}

// monstersOf reads every compiled placement of d, in the compiler's order.
func monstersOf(d *Dungeon) []Monster {
	out := make([]Monster, len(d.Spec.Monsters))
	orientation := d.Spec.Field.Canvas.Orientation
	for i, m := range d.Spec.Monsters {
		out[i] = Monster{
			Ref: m.Ref, MemberID: m.MemberID, At: cellOf(orientation, m.At), Facing: m.Facing,
			Boss: m.Boss, Targeting: m.Targeting, Actions: m.Actions, PlacementID: m.ID,
			Holds: m.Holds, Faction: m.Faction, Arrives: m.Arrives,
			Intimidate: m.Intimidate, Persuade: m.Persuade, Table: m.Table, Temper: m.Temper,
		}
	}
	return out
}
