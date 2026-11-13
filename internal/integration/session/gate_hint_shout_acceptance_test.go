package session_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	storypb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	"github.com/KirkDiggler/rpg-api/internal/auth"
)

// theGuardsHint is gate-1's answer to `friends-inside`, shouted
// (the-mad-king.yaml, rpg-project#936).
const theGuardsHint = "The troupe? Aye, they’re inside. If one of them comes back and speaks for you, in you go."

// toTheEastYard is the walk from just inside the gate to the east yard
// (cube 4,0): courtyard floor out of gate-1's sight with the game's real
// sight. The engine's own route for it, in offset cells.
func toTheEastYard() []*sessionpb.Position {
	return []*sessionpb.Position{pbAt(1, 4), pbAt(1, 3), pbAt(2, 2), pbAt(2, 1), pbAt(3, 0), pbAt(4, 0)}
}

// toTheWestYard is the walk from just inside the gate to the west yard
// (cube -12,2): courtyard floor IN LINE with the open gate, which sees
// gate-1 with the game's real sight.
func toTheWestYard() []*sessionpb.Position {
	path := make([]*sessionpb.Position, 0, 13)
	for col := -1; col >= -10; col-- {
		path = append(path, pbAt(col, 5))
	}
	return append(path, pbAt(-10, 4), pbAt(-11, 3), pbAt(-11, 2))
}

// hintInTheYard is the admitted scene at each their own (so what alice is
// sent is her own ears', rpg-project#935): alice walked through the gate and
// along path into the yard, bob come up beside the gate; bob asks gate-1
// after his friends. It returns the harness and alice's versions of the hint.
func hintInTheYard(t *testing.T, run string, path []*sessionpb.Position) (*acceptanceHarness, []*storypb.Act) {
	t.Helper()
	dice := &gateDice{}
	dice.face.Store(20)
	h := admittedWithBobOnTheRoadFrom(t, run, dice, eachTheirOwn(t, madKingRaw(t)))
	walk(t, h, run, []*sessionpb.Position{pbAt(-1, 8), besideTheGate(), pastTheGate(), pbAt(0, 5)}, "alice through the gate")
	in := walkAs(t, h, run, "alice", "player-alice", path)
	require.Equal(t, sessionpb.MovementStatus_MOVEMENT_STATUS_COMPLETED, in.GetStatus(), "alice into the yard: %s", in.GetStopReason())
	bobUpToTheGate(t, h, run)

	acts := watchActs(auth.WithPlayerID(context.Background(), "player-alice"), t, h, run, "alice", func() {
		_, err := sayTo(t, h, run, "bob", "player-bob", gatePosted, friendsLine)
		require.NoError(t, err, "bob asks after his friends")
	})
	var hints []*storypb.Act
	for _, a := range acts {
		if a.GetSpeech().GetWords() == theGuardsHint {
			require.Empty(t, a.GetPerceived().GetVia(), "her own ears, at each their own")
			hints = append(hints, a)
		}
	}
	require.Len(t, hints, 1, "alice, in the yard, catches the shouted hint")
	return h, hints
}

// TestAcceptance_TheGuardsHintIsShoutedToTheYard is rpg-project#936 on the
// server the api serves: alice, admitted, stands in the east yard out of
// gate-1's sight; bob, the straggler, asks gate-1 after his friends. Gate-1
// shouts the hint, and alice catches the words without the name: a Speech
// with the words, no speaker, one room off, by hearing alone. Whom it was
// said to is bob: from the east yard she sees him in the gate's opening,
// though not gate-1 beside him (R4: the spoken-to is named exactly when the
// reader sees them). She walks back, vouches, and bob is admitted.
func TestAcceptance_TheGuardsHintIsShoutedToTheYard(t *testing.T) {
	h, hints := hintInTheYard(t, "gate-hint-run", toTheEastYard())
	sp := hints[0].GetSpeech()
	require.Nil(t, sp.GetSpeaker(), "without the name: she does not see the guard")
	require.Equal(t, "bob", sp.GetSpokenTo().GetId(), "she sees bob in the opening, so she is told he was answered")
	require.Equal(t, storypb.Loudness_LOUDNESS_SHOUT, sp.GetLoudness())
	require.NotNil(t, sp.Hops, "an unseen speaker is placed by rooms")
	require.Equal(t, int32(1), sp.GetHops(), "one room off")
	require.Equal(t, []storypb.Sense{storypb.Sense_SENSE_HEARING}, hints[0].GetPerceived().GetSenses())

	back := walkAs(t, h, "gate-hint-run", "alice", "player-alice", []*sessionpb.Position{pbAt(3, 0), pbAt(2, 1), pbAt(2, 2), pbAt(1, 3), pbAt(1, 4), pbAt(0, 5)})
	require.Equal(t, sessionpb.MovementStatus_MOVEMENT_STATUS_COMPLETED, back.GetStatus(), "alice walks back: %s", back.GetStopReason())
	vouchFromTheRoad(t, h, "gate-hint-run")
}

// TestAcceptance_TheGuardsHintNamesHimInViewOfTheGate is the mirror: from
// the west yard, in line with the open gate, alice SEES gate-1 with the
// game's real sight, so the hint reaches her with his name and no distance.
// She does not see bob from there, so whom it was said to is not named.
func TestAcceptance_TheGuardsHintNamesHimInViewOfTheGate(t *testing.T) {
	_, hints := hintInTheYard(t, "gate-hint-view-run", toTheWestYard())
	sp := hints[0].GetSpeech()
	require.Equal(t, gatePosted, sp.GetSpeaker().GetId(), "she sees gate-1 through the open gate")
	require.Nil(t, sp.GetSpokenTo(), "bob, out of her sight, is not named")
	require.Equal(t, []storypb.Sense{storypb.Sense_SENSE_SIGHT, storypb.Sense_SENSE_HEARING}, hints[0].GetPerceived().GetSenses())
	require.Nil(t, sp.Hops, "a speaker seen is no distance")
	require.Equal(t, storypb.Loudness_LOUDNESS_SHOUT, sp.GetLoudness())
}
