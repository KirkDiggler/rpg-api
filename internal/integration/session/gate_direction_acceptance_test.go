package session_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
)

// The R6 words the castle's gate post holds by (rpg-project#857).
const (
	holdsEntering  = "entering"
	troupeOptionID = "troupe"
	pedlarOptionID = "tonight"
)

// requireOnTheGatePost requires alice's sighting of gate-1 at his cell
// beneath the gate: he has not left his post.
func requireOnTheGatePost(t *testing.T, view *sessionpb.GetViewResponse, msg string) {
	t.Helper()
	seen := sightingOf(t, view, gatePosted).GetSeen()
	require.NotNil(t, seen, msg)
	require.True(t, samePosition(beneathTheGate(), seen.GetPosition()), msg)
}

// cellsOf is the positions as plain coordinate pairs, so an exact set
// compare reads values rather than proto pointers.
func cellsOf(ps ...*sessionpb.Position) [][2]float64 {
	out := make([][2]float64, 0, len(ps))
	for _, p := range ps {
		out = append(out, [2]float64{p.GetX(), p.GetY()})
	}
	return out
}

// stepCellsOf is the cells a walk's steps stood on, in order.
func stepCellsOf(steps []*sessionpb.Step) [][2]float64 {
	out := make([][2]float64, 0, len(steps))
	for _, s := range steps {
		out = append(out, [2]float64{s.GetPosition().GetX(), s.GetPosition().GetY()})
	}
	return out
}

// toTheCaptain is the walk from beside the stranger to beside the captain,
// round the gatehouse: the session's own route for it, in offset cells.
func toTheCaptain() []*sessionpb.Position {
	return []*sessionpb.Position{
		pbAt(5, 15), pbAt(4, 15), pbAt(3, 15), pbAt(2, 15), pbAt(1, 15), pbAt(1, 14),
		pbAt(0, 13), pbAt(0, 12), pbAt(-1, 11), pbAt(-1, 10), pbAt(-1, 9),
	}
}

// walk moves alice along path and requires every step taken. A hail
// (rpg-project#923) stops a walk where it lands and drops the rest; when one
// does, her Speak row must carry the address, and she walks on with the rest
// of the path, which the standing address does not stop again.
func walk(t *testing.T, h *acceptanceHarness, run string, path []*sessionpb.Position, what string) {
	t.Helper()
	ctx := auth.WithPlayerID(context.Background(), "player-alice")
	moved, err := h.handler.Move(ctx, &sessionpb.MoveRequest{Session: run, Member: "alice", Path: path})
	require.NoError(t, err, what)
	if moved.GetStatus() == sessionpb.MovementStatus_MOVEMENT_STATUS_STOPPED && moved.GetStopReason() == "" &&
		len(moved.GetSteps()) > 0 && len(moved.GetSteps()) < len(path) {
		require.NotNil(t, speakRowOf(t, h, run, "alice", "player-alice").GetAddress(),
			"%s: a walk stopped with no reason was a hail", what)
		moved, err = h.handler.Move(ctx, &sessionpb.MoveRequest{
			Session: run, Member: "alice", Path: path[len(moved.GetSteps()):],
		})
		require.NoError(t, err, what)
	}
	require.Equal(t, sessionpb.MovementStatus_MOVEMENT_STATUS_COMPLETED, moved.GetStatus(), "%s: %s", what, moved.GetStopReason())
}

// TestAcceptance_TheGateGuardChecksArrivals is S1c of the guard holds the
// gate on the server (rpg-project#857, R6): gate-1's post holds the gate
// `against: entering` with `pass: troupe-admitted`. The view tells a viewer
// in sight of the gate which door he holds, the direction, and the post's
// side cells. An arrival is barred at the opening and at the door; a party
// inside walks out past him unchallenged and is barred on the way back in;
// and the troupe the captain let through is told no hold and crosses in
// while he stays posted.
func TestAcceptance_TheGateGuardChecksArrivals(t *testing.T) {
	ctx := auth.WithPlayerID(context.Background(), "player-alice")

	t.Run("the view tells the hold, its direction and the post's side; the arrival is barred", func(t *testing.T) {
		h := newAcceptanceHarnessWithDice(t, standAsideDice{})
		const run = "gate-direction-seen-run"
		out := launchTheMadKing(t, h, run, -1, 7)
		require.Empty(t, out.Formed, "precondition: the gate guards are neutral, nobody fights")
		guard := launched(t, out, gatePosted)

		view, err := h.handler.GetView(ctx, &sessionpb.GetViewRequest{Session: run, Member: "alice"})
		require.NoError(t, err)
		seen := sightingOf(t, view, gatePosted)
		require.Equal(t, castleGateDoor, seen.GetHoldsDoor(), "he holds the gate door")
		require.Equal(t, holdsEntering, seen.GetHoldsAgainst(), "against arrivals")
		require.ElementsMatch(t, cellsOf(besideTheGate(), beneathTheGate()), cellsOf(seen.GetHoldsSide()...),
			"the post's side is exactly the opening's two gatehouse cells")

		moved, err := h.handler.Move(ctx, &sessionpb.MoveRequest{
			Session: run, Member: "alice", Path: []*sessionpb.Position{pastTheGate()},
		})
		require.NoError(t, err)
		require.Equal(t, sessionpb.MovementStatus_MOVEMENT_STATUS_STOPPED, moved.GetStatus(), "the walk in is an arrival")
		require.Equal(t, guard.Name+" bars the way", moved.GetStopReason())

		_, err = h.handler.OpenDoor(ctx, &sessionpb.OpenDoorRequest{Session: run, Member: "alice", Door: castleGateDoor})
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		require.Equal(t, codes.FailedPrecondition, st.Code())
		require.Equal(t, guard.Name+" bars the door", st.Message(), "the door verb from outside is refused by his name")
	})

	t.Run("a party inside walks out past him unchallenged and is barred on the way back in", func(t *testing.T) {
		h := newAcceptanceHarnessWithDice(t, standAsideDice{})
		const run = "gate-direction-out-run"
		// Seated in the courtyard past the shut gate, pastTheGate's offset cell.
		out := launchTheMadKing(t, h, run, 0, 6)
		require.Empty(t, out.Formed, "precondition: nobody fights")
		guard := launched(t, out, gatePosted)

		// The post holds against entering, so her hand on the gate from inside
		// is not refused.
		opened, err := h.handler.OpenDoor(ctx, &sessionpb.OpenDoorRequest{Session: run, Member: "alice", Door: castleGateDoor})
		require.NoError(t, err, "the gate opens from inside")
		require.Equal(t, sessionpb.DoorState_DOOR_STATE_OPEN, opened.GetDoor().GetState())

		moved, err := h.handler.Move(ctx, &sessionpb.MoveRequest{
			Session: run, Member: "alice", Path: []*sessionpb.Position{besideTheGate()},
		})
		require.NoError(t, err)
		require.Equal(t, sessionpb.MovementStatus_MOVEMENT_STATUS_COMPLETED, moved.GetStatus(), "out unchallenged: %s", moved.GetStopReason())
		require.Equal(t, cellsOf(besideTheGate()), stepCellsOf(moved.GetSteps()), "she steps out through the gate")

		view, err := h.handler.GetView(ctx, &sessionpb.GetViewRequest{Session: run, Member: "alice"})
		require.NoError(t, err)
		requireOnTheGatePost(t, view, "the guard stays at his post")
		require.Equal(t, castleGateDoor, sightingOf(t, view, gatePosted).GetHoldsDoor(), "and still holds the gate against her")

		back, err := h.handler.Move(ctx, &sessionpb.MoveRequest{
			Session: run, Member: "alice", Path: []*sessionpb.Position{pastTheGate()},
		})
		require.NoError(t, err)
		require.Equal(t, sessionpb.MovementStatus_MOVEMENT_STATUS_STOPPED, back.GetStatus(), "the way back in is an arrival")
		require.Equal(t, guard.Name+" bars the way", back.GetStopReason())
	})

	t.Run("the troupe is told no hold and crosses in while he stays posted", func(t *testing.T) {
		// Every d20 at 20: the stranger's DC 5 and the captain's Performance
		// DC 13 both succeed.
		h := newAcceptanceHarnessWithDice(t, standAsideDice{})
		const run = "gate-direction-troupe-run"
		// Beside the stranger outside, where he sees her.
		out := launchTheMadKing(t, h, run, 6, 15)
		require.Empty(t, out.Formed, "precondition: nobody fights")

		_, err := h.handler.Speak(ctx, &sessionpb.SpeakRequest{
			Session: run, Member: "alice", Target: &sessionpb.SpeakRequest_MemberId{MemberId: "pedlar"}, OptionId: pedlarOptionID,
		})
		require.NoError(t, err, "the stranger tells of the feast")
		walk(t, h, run, toTheCaptain(), "to the captain")
		_, err = speakAsOffered(ctx, t, h, run, "alice", "captain", troupeOptionID)
		require.NoError(t, err, "the captain lets the troupe through")
		walk(t, h, run, []*sessionpb.Position{pbAt(-1, 8), besideTheGate()}, "to the gate")

		view, err := h.handler.GetView(ctx, &sessionpb.GetViewRequest{Session: run, Member: "alice"})
		require.NoError(t, err)
		requireOnTheGatePost(t, view, "the troupe's word leaves him on his post")
		seen := sightingOf(t, view, gatePosted)
		require.Empty(t, seen.GetHoldsDoor(), "she knows the pass, so he holds nothing against her")
		require.Empty(t, seen.GetHoldsAgainst(), "and no direction is told")
		require.Empty(t, seen.GetHoldsSide(), "and no side")

		opened, err := h.handler.OpenDoor(ctx, &sessionpb.OpenDoorRequest{Session: run, Member: "alice", Door: castleGateDoor})
		require.NoError(t, err, "the gate opens for the troupe")
		require.Equal(t, sessionpb.DoorState_DOOR_STATE_OPEN, opened.GetDoor().GetState())

		moved, err := h.handler.Move(ctx, &sessionpb.MoveRequest{
			Session: run, Member: "alice", Path: []*sessionpb.Position{pastTheGate()},
		})
		require.NoError(t, err)
		require.Equal(t, sessionpb.MovementStatus_MOVEMENT_STATUS_COMPLETED, moved.GetStatus(), "the troupe crosses: %s", moved.GetStopReason())

		view, err = h.handler.GetView(ctx, &sessionpb.GetViewRequest{Session: run, Member: "alice"})
		require.NoError(t, err)
		requireOnTheGatePost(t, view, "past the guard, who stays posted")
	})
}
