package lobby

import (
	"testing"

	"github.com/stretchr/testify/require"

	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// TestArrivalOf_IsSpellingOnly pins the launch's translation of a compiled
// arrival predicate into the session seam's sealed Arrival (rpg-project#375
// step B): one arm per form of the grammar, values carried through
// unchanged, nil for a monster placed at once, and a form this launch does
// not know refused by type rather than spawned as if never in reserve.
func TestArrivalOf_IsSpellingOnly(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   tkencounter.Trigger
		want sdk.Arrival
	}{
		{"placed at once", nil, nil},
		{"round", tkencounter.TriggerRound{Round: 6}, sdk.ArrivesAtRound{Round: 6}},
		{"down", tkencounter.TriggerMemberDown{Member: "chief"}, sdk.ArrivesOnFall{Member: "chief"}},
		{"fact", tkencounter.TriggerFact{Fact: "saved-wiseman"}, sdk.ArrivesOnFact{Fact: "saved-wiseman"}},
		{"stance", tkencounter.TriggerStance{
			Between: [2]tkencounter.FactionID{"raiders", tkencounter.FactionParty}, Stance: tkencounter.StanceNeutral,
		}, sdk.ArrivesOnStance{Between: [2]string{"raiders", "party"}, Stance: "neutral"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := arrivalOf(tc.in)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}

	_, err := arrivalOf(tkencounter.TriggerExternal{})
	require.Error(t, err, "an ending's trigger is not an arrival, and the launch says so by type")
	require.Contains(t, err.Error(), "TriggerExternal")
}
