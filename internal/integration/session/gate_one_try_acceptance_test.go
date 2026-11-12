package session_test

// gate_one_try_acceptance_test.go is one try at the straggler's claim
// (rpg-project#932) and a roll is kept (rpg-project#938) on the server the
// api serves, for two players: alice is admitted while bob is down the road,
// and bob makes his own case at the gate.

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-toolkit/core"
	"github.com/KirkDiggler/rpg-toolkit/play/intel"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/conditions"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
)

// The gate guards' line for a stranger asking after their friends, and what
// a guard holds about a straggler he turned away (the-mad-king.yaml,
// rpg-project#932).
const (
	friendsLine     = "friends-inside"
	turnedAwayTopic = "turned-away"
)

// gateDice rolls every d20 at its face — 20 until a test turns it down —
// and every other die at 1, as [standAsideDice] does.
type gateDice struct{ face atomic.Int32 }

// Roll throws face on a d20 and 1 on anything else.
func (d *gateDice) Roll(_ context.Context, size int) (int, error) {
	if size == 20 {
		return int(d.face.Load()), nil
	}
	return 1, nil
}

// admittedWithBobComingUp is alice admitted and in the courtyard, the
// captain gone in, and bob walked up the road to beside the gate; every
// d20 after it shows face.
func admittedWithBobComingUp(t *testing.T, run string, guided bool, face int32) *acceptanceHarness {
	t.Helper()
	dice := &gateDice{}
	dice.face.Store(20)
	h := admittedWithBobOnTheRoadRolling(t, run, dice)
	if guided {
		giveGuidance(t, h, "bob")
	}
	walk(t, h, run, []*sessionpb.Position{pbAt(-1, 8), besideTheGate(), pastTheGate(), pbAt(0, 5)}, "alice into the courtyard")
	bobUpToTheGate(t, h, run)
	dice.face.Store(face)
	return h
}

// holds is whether guard holds topic about member in the stored world.
func holds(t *testing.T, h *acceptanceHarness, run, guard, member, topic string) bool {
	t.Helper()
	world, err := sessionorch.NewEncounterRepository(h.redis, 0).GetEncounter(context.Background(), run)
	require.NoError(t, err)
	_, ok := world.Perception.Intel.Holdings[core.EntityID("member|"+guard)][intel.Subject("member|"+member)][intel.Topic(topic)]
	return ok
}

// bobHoldsGuidance is whether bob's stored sheet still holds the Guidance
// die.
func bobHoldsGuidance(t *testing.T, h *acceptanceHarness) bool {
	t.Helper()
	stored, err := h.charRepo.Get(context.Background(), characterrepo.GetInput{ID: "bob"})
	require.NoError(t, err)
	held, err := conditions.HoldsRef(stored.Character.Data.Conditions, refs.Conditions.Guided())
	require.NoError(t, err)
	return held
}

// TestAcceptance_AStragglerHasOneTry: bob makes his own case and misses.
// Both gate guards hold that they turned him away, so the case is gone from
// his row on both while their other lines stay (he sights them both); the
// line asking after his friends is offered and answered; alice comes back
// and vouches, and bob walks through.
func TestAcceptance_AStragglerHasOneTry(t *testing.T) {
	const run = "gate-one-try-run"
	h := admittedWithBobComingUp(t, run, false, 2)
	_, err := sayTo(t, h, run, "bob", "player-bob", gatePosted, stragglerLine)
	require.NoError(t, err, "bob makes his own case")
	require.True(t, holds(t, h, run, gatePosted, "bob", turnedAwayTopic), "the miss: gate-1 turned bob away")
	require.True(t, holds(t, h, run, gateLoop, "bob", turnedAwayTopic), "and gate-2 heard it")
	for _, guard := range []string{gatePosted, gateLoop} {
		require.True(t, offers(t, h, run, "bob", "player-bob", guard, friendsLine), "%s still speaks to bob", guard)
		require.False(t, offers(t, h, run, "bob", "player-bob", guard, stragglerLine), "one try: the case is gone on %s", guard)
	}
	_, err = sayTo(t, h, run, "bob", "player-bob", gatePosted, friendsLine)
	require.NoError(t, err, "bob asks after his friends and is answered")

	vouchFromTheRoad(t, h, run)
}

// TestAcceptance_AStragglerWhoWalksAwayKeepsTheRoll: bob holds a Guidance
// die, so his case pauses on it, with a total below the mark. He walks back
// down the road without answering. That Move settles the case as rolled:
// both guards turned him away, his die is still his, his question is gone,
// and a later answer to it is refused as there is no window.
//
// The staged case (an answer reaching a hold abandoned behind the session's
// back, answered roll_kept) is not staged here: the api offers no honest way
// to write the encounter record from outside a verb. It is covered by the
// session's castle tests and by the handler's mapping of roll_kept.
func TestAcceptance_AStragglerWhoWalksAwayKeepsTheRoll(t *testing.T) {
	const run = "gate-roll-kept-run"
	h := admittedWithBobComingUp(t, run, true, 2)
	claim, err := sayTo(t, h, run, "bob", "player-bob", gatePosted, stragglerLine)
	require.NoError(t, err)
	require.True(t, claim.GetPaused(), "precondition: his Guidance die stops the check to ask")
	react := reactRowOf(t, h, run, "bob", "player-bob")
	require.NotNil(t, react, "precondition: he is asked")

	away := walkAs(t, h, run, "bob", "player-bob", []*sessionpb.Position{pbAt(-1, 8), pbAt(-1, 9), pbAt(-1, 10), pbAt(-1, 11)})
	require.Equal(t, sessionpb.MovementStatus_MOVEMENT_STATUS_COMPLETED, away.GetStatus(), "bob walks away: %s", away.GetStopReason())
	require.True(t, holds(t, h, run, gatePosted, "bob", turnedAwayTopic), "the walk settled the case as rolled: turned away")
	require.True(t, holds(t, h, run, gateLoop, "bob", turnedAwayTopic), "by both guards")
	require.True(t, bobHoldsGuidance(t, h), "and his die is kept")
	require.Nil(t, reactRowOf(t, h, run, "bob", "player-bob"), "his question is gone")

	_, err = h.handler.React(auth.WithPlayerID(context.Background(), "player-bob"), &sessionpb.ReactRequest{
		Session: run, Member: "bob", DeclarationId: react.GetId(), Choice: sessionpb.ReactChoice_REACT_CHOICE_STRIKE,
	})
	require.Equal(t, codes.FailedPrecondition, status.Code(err), "a later answer finds no window: %v", err)
	require.True(t, bobHoldsGuidance(t, h), "and spends nothing")
}
