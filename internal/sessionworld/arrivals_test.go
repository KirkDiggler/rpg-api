package sessionworld

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

// arrivals_test.go pins step B of the hold-out at this seam (rpg-project#375,
// design §3.7, R6, R10): what a placement in reserve costs this package,
// which cells a party may not be seated on, and that an ending spelled out
// in the file is the scenario's own.

// TestArrivalsAreCarriedToTheSeam is Faction's test asked of Arrives: a
// MONSTER's predicate is about the member and is hand-carried to the seam
// for the launch to forward; a PROP's rides Compiled.Field on the PropInput
// with no line here. The chief's fall is spelled as the composition's own
// Trigger naming the member id the chief spawns under -- the reason an
// authored id is the member id in the first place.
func TestArrivalsAreCarriedToTheSeam(t *testing.T) {
	camp, spec := compileRaiderCamp(t)

	byPlacement := map[string]Monster{}
	for _, m := range camp.Monsters {
		byPlacement[m.PlacementID] = m
	}
	for _, id := range []string{"reinforcement-1", "reinforcement-2", "reinforcement-3"} {
		m, placed := byPlacement[id]
		require.True(t, placed, "the author placed %s", id)
		require.Equal(t, tkencounter.TriggerMemberDown{Member: "chief"}, m.Arrives,
			"%s waits on the chief's fall, by the id the chief spawns under", id)
		require.Equal(t, "raiders", m.Faction, "and arrives as a raider")
	}
	require.Nil(t, byPlacement["chief"].Arrives, "the chief stands there from the first frame")
	require.Nil(t, byPlacement["scout"].Arrives, "and so does the scout")
	for _, want := range spec.Monsters {
		require.Equal(t, want.Arrives, byPlacement[want.ID].Arrives,
			"placement %q carries the predicate the compiler gave it, verbatim", want.ID)
	}

	// The letter's arrival rode the field: no line of this package put it
	// there, and the composition holds the prop in reserve until round 6.
	var letter *tkencounter.PropData
	for i := range camp.World.Field.Props {
		if camp.World.Field.Props[i].ID == "letter" {
			letter = &camp.World.Field.Props[i]
		}
	}
	require.NotNil(t, letter, "the letter is in the world's field, in reserve")
	require.NotNil(t, letter.Arrives)
	require.Equal(t, "round", letter.Arrives.Kind)
	require.Equal(t, 6, letter.Arrives.Round, "and not before")
}

// TestAnArrivalsCellIsNobodysSeat: a placement in reserve has no cell yet,
// but the cell it will arrive at is spoken for -- the compiler leaves it
// out of the seats, so a fourth player is never sat where a zombie is about
// to stand. The first seat is still the authored start.
func TestAnArrivalsCellIsNobodysSeat(t *testing.T) {
	camp, spec := compileRaiderCamp(t)
	orientation := spec.Field.Canvas.Orientation

	seats := map[spatial.Position]bool{}
	for _, s := range camp.PartySeats {
		seats[s] = true
	}
	require.Equal(t, cellOf(orientation, spatial.Position{X: 0, Y: 3}), camp.PartySeats[0], "the authored start comes first")
	for _, arrival := range [][2]int{{1, 4}, {2, 4}, {1, 5}} {
		cell := cellOf(orientation, spatial.Position{X: float64(arrival[0]), Y: float64(arrival[1])})
		require.False(t, seats[cell], "[%d,%d] is where a reinforcement arrives, and nobody's seat", arrival[0], arrival[1])
	}
	require.Len(t, camp.PartySeats, 20, "the gate's floor, less the cells that are spoken for")
}

// TestAnEndingAuthoredInTheFileIsTheScenariosOwn is R10 at this seam: the
// scenario binding is SUGAR. `scenarios: { hold-out: { convince: raiders } }`
// and `endings: [{ id: hold-out, when: { stance: { between: [raiders,
// party], is: neutral } } }]` compile to the same world, ending for ending
// -- dungeonspec compiles the spelled-out form to the Trigger the scenario
// package constructs, and this package declares both lists the same way.
// A scenario package with nothing left to do is the north star's own test.
func TestAnEndingAuthoredInTheFileIsTheScenariosOwn(t *testing.T) {
	raw, err := os.ReadFile(raiderCampPath)
	require.NoError(t, err)
	const sugar = "scenarios:\n  hold-out: { convince: raiders }\n"
	require.Contains(t, string(raw), sugar, "the fixture's binding must be where this test expects it")

	canonical, err := Compile(raw)
	require.NoError(t, err)

	spelled := strings.Replace(string(raw), sugar,
		"endings:\n  - { id: hold-out, when: { stance: { between: [raiders, party], is: neutral } } }\n", 1)
	authored, err := Compile([]byte(spelled))
	require.NoError(t, err, "the spelled-out ending compiles")
	require.Equal(t, canonical.World.Endings, authored.World.Endings,
		"the sugar and the spelling declare the same endings to the composition")

	// And a name of the author's own, so the wire's `ended` beat names it.
	turned := strings.Replace(string(raw), sugar,
		"endings:\n  - { id: turned, when: { stance: { between: [raiders, party], is: neutral } } }\n", 1)
	own, err := Compile([]byte(turned))
	require.NoError(t, err)
	keys := make([]string, 0, len(own.World.Endings))
	for _, e := range own.World.Endings {
		keys = append(keys, e.Key)
	}
	require.Equal(t, []string{EndingWithdrawn, "turned"}, keys, "withdrawal always, plus the file's own ending")
	require.Equal(t, "stance", own.World.Endings[1].Kind)
}
