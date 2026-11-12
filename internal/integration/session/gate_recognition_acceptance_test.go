package session_test

import (
	"context"
	"testing"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	"github.com/stretchr/testify/require"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
)

// The gate guards' lines for a straggler (the-mad-king.yaml, rpg-project#929).
const (
	vouchLine     = "vouch"
	stragglerLine = "straggler"
)

// walkAs moves member along path, walking on through a hail as [walk] does,
// and returns the last answer.
func walkAs(t *testing.T, h *acceptanceHarness, run, member, player string, path []*sessionpb.Position) *sessionpb.MoveResponse {
	t.Helper()
	ctx := auth.WithPlayerID(context.Background(), player)
	moved, err := h.handler.Move(ctx, &sessionpb.MoveRequest{Session: run, Member: member, Path: path})
	require.NoError(t, err)
	if moved.GetStatus() == sessionpb.MovementStatus_MOVEMENT_STATUS_STOPPED && moved.GetStopReason() == "" &&
		len(moved.GetSteps()) > 0 && len(moved.GetSteps()) < len(path) {
		moved, err = h.handler.Move(ctx, &sessionpb.MoveRequest{Session: run, Member: member, Path: path[len(moved.GetSteps()):]})
		require.NoError(t, err)
	}

	return moved
}

// offers reports whether member's Speak row offers target a line, available.
func offers(t *testing.T, h *acceptanceHarness, run, member, player, target, option string) bool {
	t.Helper()
	for _, o := range speakRowOf(t, h, run, member, player).GetSpeakOptions() {
		if o.GetMemberId() == target && o.GetOptionId() == option && o.GetAvailable() {
			return true
		}
	}

	return false
}

// worldMember is a member of the stored world, read from the repository.
func worldMember(t *testing.T, h *acceptanceHarness, run, id string) encounter.MemberData {
	t.Helper()
	world, err := sessionorch.NewEncounterRepository(h.redis, 0).GetEncounter(context.Background(), run)
	require.NoError(t, err)
	for _, m := range world.Members {
		if string(m.ID) == id {
			return m
		}
	}
	require.FailNowf(t, "not in the run", "%s", id)

	return encounter.MemberData{}
}

// sayTo has member say a line to target as offered.
func sayTo(t *testing.T, h *acceptanceHarness, run, member, player, target, option string) (*sessionpb.SpeakResponse, error) {
	t.Helper()
	return speakAsOffered(auth.WithPlayerID(context.Background(), player), t, h, run, member, target, option)
}

// admittedWithBobOnTheRoad launches the castle for two, alice beside the
// captain and bob beside the stranger out of every gate guard's sight, and
// has alice win the guest claim.
func admittedWithBobOnTheRoad(t *testing.T, run string) *acceptanceHarness {
	t.Helper()
	return admittedWithBobOnTheRoadRolling(t, run, standAsideDice{})
}

// admittedWithBobOnTheRoadRolling is [admittedWithBobOnTheRoad] on the
// dice given; the guest claim must still win on them.
func admittedWithBobOnTheRoadRolling(t *testing.T, run string, dice sdk.Roller) *acceptanceHarness {
	t.Helper()
	ctx := auth.WithPlayerID(context.Background(), "player-alice")
	h := newAcceptanceHarnessWithDice(t, dice)
	for _, c := range []struct{ id, player string }{{"alice", "player-alice"}, {"bob", "player-bob"}} {
		_, err := h.charRepo.Create(ctx, characterrepo.CreateInput{
			Character: &entities.Character{Data: armedFighter(c.id, c.player)},
		})
		require.NoError(t, err)
	}
	dungeon, err := sessionworld.Compile(madKingRaw(t))
	require.NoError(t, err)
	out := h.launch(t, run, dungeon.Spec, seatAt("alice", -1, 9), seatAt("bob", 6, 15))
	require.Empty(t, out.Formed, "precondition: nobody fights")
	_, err = sayTo(t, h, run, "alice", "player-alice", "captain", guestLine)
	require.NoError(t, err, "every d20 a 20: the guest claim wins")

	return h
}

// bobUpToTheGate walks bob up the road to beside the gate, in gate-1's sight.
func bobUpToTheGate(t *testing.T, h *acceptanceHarness, run string) {
	t.Helper()
	up := walkAs(t, h, run, "bob", "player-bob", toTheCaptain())
	require.Equal(t, sessionpb.MovementStatus_MOVEMENT_STATUS_COMPLETED, up.GetStatus(), "precondition: bob walks up the road: %s", up.GetStopReason())
	toGate := walkAs(t, h, run, "bob", "player-bob", []*sessionpb.Position{pbAt(-1, 8), besideTheGate()})
	require.Equal(t, sessionpb.MovementStatus_MOVEMENT_STATUS_COMPLETED, toGate.GetStatus(), "precondition: bob reaches the gate: %s", toGate.GetStopReason())
}

// TestAcceptance_TheCaptainRecognizesWhomHeAdmitted is admission is
// recognition (rpg-project#928) with the captain going in and a straggler
// vouched for (rpg-project#929), on the server the api serves. Alice wins
// the guest claim at the captain while bob is down the road out of every
// gate guard's sight. The captain recognizes alice, and gate-1 and gate-2
// heard it; he then goes into the great hall, and the two gate guards keep
// the gate. Bob walks up after the fact and is barred by a hold; gate-1
// offers him the straggler's case, not the vouch. Alice's vouch from the
// courtyard, out of Speak reach, is refused; she comes back to the gate and
// vouches with bob in gate-1's sight, and bob walks through.
func TestAcceptance_TheCaptainRecognizesWhomHeAdmitted(t *testing.T) {
	const run = "gate-recognition-run"
	h := admittedWithBobOnTheRoad(t, run)
	require.True(t, offers(t, h, run, "alice", "player-alice", gatePosted, vouchLine), "gate-1 offers alice the vouch")
	require.False(t, offers(t, h, run, "alice", "player-alice", gatePosted, stragglerLine), "and not the straggler's case")

	// Alice walks through the gate, which the captain left open on his way in,
	// and on into the courtyard.
	walk(t, h, run, []*sessionpb.Position{pbAt(-1, 8), besideTheGate()}, "alice to the gate")
	crossed := walkAs(t, h, run, "alice", "player-alice", []*sessionpb.Position{pastTheGate()})
	require.Equal(t, sessionpb.MovementStatus_MOVEMENT_STATUS_COMPLETED, crossed.GetStatus(), "recognized, alice crosses: %s", crossed.GetStopReason())
	walkAs(t, h, run, "alice", "player-alice", []*sessionpb.Position{pbAt(0, 5), pbAt(0, 4), pbAt(0, 3)})
	captain := worldMember(t, h, run, "captain")
	require.Less(t, captain.Cell.Y, pastTheGate().GetY(), "the captain has gone through the gate, on his way to the hall")

	// Bob walks up after the fact and is barred.
	bobUpToTheGate(t, h, run)
	require.True(t, offers(t, h, run, "bob", "player-bob", gatePosted, stragglerLine), "gate-1 offers bob the straggler's case")
	require.False(t, offers(t, h, run, "bob", "player-bob", gatePosted, vouchLine), "and not the vouch")
	barred := walkAs(t, h, run, "bob", "player-bob", []*sessionpb.Position{pastTheGate()})
	require.Equal(t, sessionpb.MovementStatus_MOVEMENT_STATUS_STOPPED, barred.GetStatus(), "bob, recognized by nobody, is barred")
	require.Contains(t, barred.GetStopReason(), "bars the way", "by a hold, not a body")
	require.Empty(t, barred.GetSteps(), "and takes no step")

	// From the courtyard, out of Speak reach, alice's vouch is refused.
	_, err := h.handler.Speak(auth.WithPlayerID(context.Background(), "player-alice"), &sessionpb.SpeakRequest{
		Session: run, Member: "alice", Target: &sessionpb.SpeakRequest_MemberId{MemberId: gatePosted}, OptionId: vouchLine,
	})
	require.Error(t, err, "a vouch from out of reach is refused")

	// She comes back to the gate and vouches with bob in gate-1's sight.
	walkAs(t, h, run, "alice", "player-alice", []*sessionpb.Position{pbAt(0, 4), pbAt(0, 5), pastTheGate()})
	_, err = sayTo(t, h, run, "alice", "player-alice", gatePosted, vouchLine)
	require.NoError(t, err, "alice vouches for bob")
	walkAs(t, h, run, "alice", "player-alice", []*sessionpb.Position{pbAt(0, 5)})
	through := walkAs(t, h, run, "bob", "player-bob", []*sessionpb.Position{pastTheGate()})
	require.Equal(t, sessionpb.MovementStatus_MOVEMENT_STATUS_COMPLETED, through.GetStatus(), "vouched for, bob walks through: %s", through.GetStopReason())
}

// TestAcceptance_AStragglerMakesTheirOwnCase: alice is admitted and bob
// arrives late, as above; bob makes his own case to gate-1 and, every d20 a
// 20, wins it, and walks through. Nobody vouched for him.
func TestAcceptance_AStragglerMakesTheirOwnCase(t *testing.T) {
	const run = "gate-straggler-run"
	h := admittedWithBobOnTheRoad(t, run)
	walk(t, h, run, []*sessionpb.Position{pbAt(-1, 8), besideTheGate(), pastTheGate(), pbAt(0, 5)}, "alice into the courtyard")
	bobUpToTheGate(t, h, run)
	_, err := sayTo(t, h, run, "bob", "player-bob", gatePosted, stragglerLine)
	require.NoError(t, err, "bob makes his own case")
	through := walkAs(t, h, run, "bob", "player-bob", []*sessionpb.Position{pastTheGate()})
	require.Equal(t, sessionpb.MovementStatus_MOVEMENT_STATUS_COMPLETED, through.GetStatus(), "his case won, bob walks through: %s", through.GetStopReason())
}
