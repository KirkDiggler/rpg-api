package session_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
)

// theWayIn is the captain's line for whom he recognizes as admitted
// (the-mad-king.yaml, rpg-project#928).
const theWayIn = "the-way-in"

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

// offersCaptain reports whether member's Speak row offers the captain a line.
func offersCaptain(t *testing.T, h *acceptanceHarness, run, member, player, option string) bool {
	t.Helper()
	for _, o := range speakRowOf(t, h, run, member, player).GetSpeakOptions() {
		if o.GetMemberId() == "captain" && o.GetOptionId() == option && o.GetAvailable() {
			return true
		}
	}

	return false
}

// TestAcceptance_TheCaptainRecognizesWhomHeAdmitted is admission is
// recognition on the server the api serves (rpg-project#928). Alice wins the
// guest claim at the captain while bob is down the road by the stranger, out
// of every gate guard's sight. The captain recognizes alice, and gate-1 and
// gate-2 heard it; nobody recognizes bob, who was not there. So alice's Speak
// row offers the captain's line for whom he recognizes and bob's, once he
// walks up, does not; alice walks through the gate; bob is barred at it, by
// a hold. Bob barred is the gate working: he was never admitted.
func TestAcceptance_TheCaptainRecognizesWhomHeAdmitted(t *testing.T) {
	aliceCtx := auth.WithPlayerID(context.Background(), "player-alice")
	h := newAcceptanceHarnessWithDice(t, standAsideDice{})
	const run = "gate-recognition-run"

	for _, c := range []struct{ id, player string }{{"alice", "player-alice"}, {"bob", "player-bob"}} {
		_, err := h.charRepo.Create(aliceCtx, characterrepo.CreateInput{
			Character: &entities.Character{Data: armedFighter(c.id, c.player)},
		})
		require.NoError(t, err)
	}
	dungeon, err := sessionworld.Compile(madKingRaw(t))
	require.NoError(t, err)
	// Alice seated beside the captain, where the walk up the road ends; bob
	// beside the stranger, where the troupe test starts its walk.
	out := h.launch(t, run, dungeon.Spec, seatAt("alice", -1, 9), seatAt("bob", 6, 15))
	require.Empty(t, out.Formed, "precondition: nobody fights")

	require.False(t, offersCaptain(t, h, run, "alice", "player-alice", theWayIn), "precondition: before the claim, no such line")
	_, err = speakAsOffered(aliceCtx, t, h, run, "alice", "captain", guestLine)
	require.NoError(t, err, "every d20 a 20: the guest claim wins")
	require.True(t, offersCaptain(t, h, run, "alice", "player-alice", theWayIn), "the captain offers alice his line for whom he recognizes")

	// Alice walks through the gate.
	walk(t, h, run, []*sessionpb.Position{pbAt(-1, 8), besideTheGate()}, "alice to the gate")
	opened, err := h.handler.OpenDoor(aliceCtx, &sessionpb.OpenDoorRequest{Session: run, Member: "alice", Door: castleGateDoor})
	require.NoError(t, err, "recognized, alice works the gate door")
	require.Equal(t, sessionpb.DoorState_DOOR_STATE_OPEN, opened.GetDoor().GetState())
	crossed := walkAs(t, h, run, "alice", "player-alice", []*sessionpb.Position{pastTheGate()})
	require.Equal(t, sessionpb.MovementStatus_MOVEMENT_STATUS_COMPLETED, crossed.GetStatus(), "recognized, alice crosses: %s", crossed.GetStopReason())
	walkAs(t, h, run, "alice", "player-alice", []*sessionpb.Position{pbAt(0, 5)})

	// Bob walks up after the fact.
	up := walkAs(t, h, run, "bob", "player-bob", toTheCaptain())
	require.Equal(t, sessionpb.MovementStatus_MOVEMENT_STATUS_COMPLETED, up.GetStatus(), "precondition: bob reaches the captain: %s", up.GetStopReason())
	require.False(t, offersCaptain(t, h, run, "bob", "player-bob", theWayIn), "the captain has no such line for bob, who was not there")
	toGate := walkAs(t, h, run, "bob", "player-bob", []*sessionpb.Position{pbAt(-1, 8), besideTheGate()})
	require.Equal(t, sessionpb.MovementStatus_MOVEMENT_STATUS_COMPLETED, toGate.GetStatus(), "precondition: bob reaches the gate: %s", toGate.GetStopReason())
	barred := walkAs(t, h, run, "bob", "player-bob", []*sessionpb.Position{pastTheGate()})
	require.Equal(t, sessionpb.MovementStatus_MOVEMENT_STATUS_STOPPED, barred.GetStatus(), "bob, recognized by nobody, is barred")
	require.Contains(t, barred.GetStopReason(), "bars the way", "by a hold, not a body")
	require.Empty(t, barred.GetSteps(), "and takes no step")
}
