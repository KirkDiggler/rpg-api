package session_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	storypb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	"github.com/KirkDiggler/rpg-api/internal/auth"
)

// gate-1's two lines in the castle (the-mad-king.yaml, rpg-project#842): his
// question at the first bump, and the shorter one while he is challenged.
const (
	theGuardsQuestion = "Whoa there, buddy. Can I help you?"

	// theCaptainsBar is the captain's answer to a bump at the gate he holds
	// (rpg-project#928).
	theCaptainsBar  = "Not one step further without my say."
	theGuardsRepeat = "I said, can I help you?"
)

// barredActs is every bump in acts.
func barredActs(acts []*storypb.Act) []*storypb.BarredAct {
	var out []*storypb.BarredAct
	for _, a := range acts {
		if b := a.GetBarred(); b != nil {
			out = append(out, b)
		}
	}
	return out
}

// spokenBy is who's Speech act, when the act is one the reader saw them say
// and caught the words of (rpg-project#934): the words of a creature's line
// ride the Speech act, never the table answer.
func spokenBy(a *storypb.Act, who string) *storypb.Speech {
	if sp := a.GetSpeech(); sp != nil && sp.GetSpeaker().GetId() == who && sp.Words != nil {
		return sp
	}
	return nil
}

// linesOf is every line who says in acts, in order.
func linesOf(acts []*storypb.Act, who string) []string {
	var out []string
	for _, a := range acts {
		if sp := spokenBy(a, who); sp != nil {
			out = append(out, sp.GetWords())
		}
	}
	return out
}

// requireAnsweredUnderTheBump requires the one bump in acts and who's one line
// to carry the same `act` correlation: the guard's answer reopens the bump's
// act, which is what the web keys the line to the bump by (rpg-project#842).
func requireAnsweredUnderTheBump(t *testing.T, acts []*storypb.Act, who string) {
	t.Helper()
	var bump, line string
	for _, a := range acts {
		if a.GetBarred() != nil {
			require.Empty(t, bump, "one bump")
			bump = a.GetAct()
		}
		if spokenBy(a, who) != nil {
			require.Empty(t, line, "one line")
			line = a.GetAct()
		}
	}
	require.NotEmpty(t, bump, "the bump carries an act correlation")
	require.Equal(t, bump, line, "the guard's line rides the bump's act")
}

// TestAcceptance_TheGateGuardAnswersTheBump is S2 of the guard holds the gate
// on the server (rpg-project#842, R2 and R7): a bump is a beat the guard
// hears. Alice walks into posted gate-1: the Move answers stopped, and her
// act stream carries the BarredAct naming the cell she was refused into and
// his question. A second bump gets the shorter line. A hand on the gate he
// holds is refused FAILED_PRECONDITION by his name while the stream carries
// the bump naming the door and no cell. A wall bump writes no BarredAct and
// no line.
func TestAcceptance_TheGateGuardAnswersTheBump(t *testing.T) {
	ctx := auth.WithPlayerID(context.Background(), "player-alice")

	t.Run("walked into, he asks what she wants; bumped again, he says it shorter", func(t *testing.T) {
		h := newAcceptanceHarnessWithDice(t, standAsideDice{})
		const run = "gate-bump-run"
		out := launchTheMadKing(t, h, run, -1, 7)
		require.Empty(t, out.Formed, "precondition: the gate guards are neutral, nobody fights")
		guard := launched(t, out, gatePosted)

		var moved *sessionpb.MoveResponse
		acts := watchActs(ctx, t, h, run, "alice", func() {
			var err error
			moved, err = h.handler.Move(ctx, &sessionpb.MoveRequest{
				Session: run, Member: "alice", Path: []*sessionpb.Position{beneathTheGate(), pastTheGate()},
			})
			require.NoError(t, err, "a bump is an answer, never an error")
		})
		require.Equal(t, sessionpb.MovementStatus_MOVEMENT_STATUS_STOPPED, moved.GetStatus(), "the walk stops at him")
		require.Equal(t, guard.Name+" stands in the way", moved.GetStopReason())
		require.Empty(t, moved.GetSteps(), "she took no step")
		require.Nil(t, moved.GetFormed(), "a bump is not a fight")

		bumps := barredActs(acts)
		require.Len(t, bumps, 1, "the stream carries the bump once")
		require.Equal(t, "alice", bumps[0].GetMover().GetId())
		require.Equal(t, gatePosted, bumps[0].GetBarrier().GetId())
		cell := bumps[0].GetCell()
		require.NotNil(t, cell, "a step bump names its cell")
		require.Equal(t, [2]float64{beneathTheGate().GetX(), beneathTheGate().GetY()}, [2]float64{cell.GetX(), cell.GetY()},
			"the cell she was refused into, his own")
		require.Empty(t, bumps[0].GetDoor(), "a step names no door")
		require.Equal(t, []string{theGuardsQuestion}, linesOf(acts, gatePosted), "he asks what she wants")
		requireAnsweredUnderTheBump(t, acts, gatePosted)

		var again *sessionpb.MoveResponse
		acts = watchActs(ctx, t, h, run, "alice", func() {
			var err error
			again, err = h.handler.Move(ctx, &sessionpb.MoveRequest{
				Session: run, Member: "alice", Path: []*sessionpb.Position{beneathTheGate(), pastTheGate()},
			})
			require.NoError(t, err)
		})
		require.Equal(t, sessionpb.MovementStatus_MOVEMENT_STATUS_STOPPED, again.GetStatus(), "challenged, he still holds")
		require.Len(t, barredActs(acts), 1, "the second bump is barred")
		require.Equal(t, []string{theGuardsRepeat}, linesOf(acts, gatePosted), "and gets the shorter line")
		requireAnsweredUnderTheBump(t, acts, gatePosted)
	})

	t.Run("a walk whose goal is his cell is the bump, not a bad position", func(t *testing.T) {
		h := newAcceptanceHarnessWithDice(t, standAsideDice{})
		const run = "gate-bump-goal-run"
		out := launchTheMadKing(t, h, run, -1, 7)
		require.Empty(t, out.Formed, "precondition: nobody fights")
		guard := launched(t, out, gatePosted)

		var moved *sessionpb.MoveResponse
		acts := watchActs(ctx, t, h, run, "alice", func() {
			var err error
			// The click lands on him: the path ends on his cell.
			moved, err = h.handler.Move(ctx, &sessionpb.MoveRequest{
				Session: run, Member: "alice", Path: []*sessionpb.Position{beneathTheGate()},
			})
			require.NoError(t, err, "a goal on his cell is answered, never refused as a bad position")
		})
		require.Equal(t, sessionpb.MovementStatus_MOVEMENT_STATUS_STOPPED, moved.GetStatus())
		require.Equal(t, guard.Name+" stands in the way", moved.GetStopReason())
		require.Empty(t, moved.GetSteps(), "she took no step")

		bumps := barredActs(acts)
		require.Len(t, bumps, 1, "the stream carries the bump")
		cell := bumps[0].GetCell()
		require.NotNil(t, cell)
		require.Equal(t, [2]float64{beneathTheGate().GetX(), beneathTheGate().GetY()}, [2]float64{cell.GetX(), cell.GetY()},
			"the refused cell is his own")
		require.Equal(t, gatePosted, bumps[0].GetBarrier().GetId())
		require.Equal(t, []string{theGuardsQuestion}, linesOf(acts, gatePosted), "he asks what she wants")
		requireAnsweredUnderTheBump(t, acts, gatePosted)
	})

	t.Run("a hand on the held gate is refused by the nearest holder's name and heard", func(t *testing.T) {
		h := newAcceptanceHarnessWithDice(t, standAsideDice{})
		const run = "gate-bump-door-run"
		out := launchTheMadKing(t, h, run, -1, 7)
		require.Empty(t, out.Formed, "precondition: nobody fights")
		// Every gate guard holds the gate (rpg-project#928); the captain stands
		// nearest the opening's gatehouse side, so the hand is his to answer.
		nearest := launched(t, out, gateCaptain)

		var openErr error
		acts := watchActs(ctx, t, h, run, "alice", func() {
			_, openErr = h.handler.OpenDoor(ctx, &sessionpb.OpenDoorRequest{Session: run, Member: "alice", Door: castleGateDoor})
		})
		st, ok := status.FromError(openErr)
		require.True(t, ok)
		require.Equal(t, codes.FailedPrecondition, st.Code(), "a refused door verb never crosses as a success")
		require.Equal(t, nearest.Name+" bars the door", st.Message())

		bumps := barredActs(acts)
		require.Len(t, bumps, 1, "the stream carries the bump")
		require.Equal(t, castleGateDoor, bumps[0].GetDoor(), "naming the door")
		require.Nil(t, bumps[0].GetCell(), "and no cell")
		require.Equal(t, gateCaptain, bumps[0].GetBarrier().GetId())
		require.Equal(t, []string{theCaptainsBar}, linesOf(acts, gateCaptain), "he answers the hand on his gate")
		requireAnsweredUnderTheBump(t, acts, gateCaptain)

		doors, err := h.handler.GetDoors(ctx, &sessionpb.GetDoorsRequest{Session: run, Member: "alice"})
		require.NoError(t, err)
		found := false
		for _, d := range doors.GetDoors() {
			if d.GetDoor() == castleGateDoor {
				found = true
				require.NotEqual(t, sessionpb.DoorState_DOOR_STATE_OPEN, d.GetState(), "the gate stays shut")
			}
		}
		require.True(t, found, "the gate is in the door list")
	})

	t.Run("a wall bump writes no bump and no line", func(t *testing.T) {
		h := newAcceptanceHarnessWithDice(t, standAsideDice{})
		const run = "gate-bump-wall-run"
		out := launchTheMadKing(t, h, run, -1, 7)
		require.Empty(t, out.Formed, "precondition: nobody fights")
		guard := launched(t, out, gatePosted)

		var moved *sessionpb.MoveResponse
		acts := watchActs(ctx, t, h, run, "alice", func() {
			var err error
			// North of her is the gate's wall, then the courtyard beyond it.
			moved, err = h.handler.Move(ctx, &sessionpb.MoveRequest{
				Session: run, Member: "alice", Path: []*sessionpb.Position{pbAt(-1, 6), pbAt(-2, 5)},
			})
			require.NoError(t, err)
		})
		require.Equal(t, sessionpb.MovementStatus_MOVEMENT_STATUS_STOPPED, moved.GetStatus(), "the wall stops the walk")
		require.NotContains(t, moved.GetStopReason(), guard.Name, "and its reason names no one")
		require.Empty(t, barredActs(acts), "a wall is no body: nothing is barred")
		require.Empty(t, linesOf(acts, gatePosted), "and he says nothing")
	})
}
