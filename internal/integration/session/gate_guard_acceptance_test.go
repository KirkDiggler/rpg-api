package session_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// The shipped castle's key and the two gate guards it places.
const (
	madKingKey = "the-mad-king"
	gatePosted = "gate-1"
	gateLoop   = "gate-2"
)

// launchTheMadKing compiles the shipped castle the way the lobby does and
// launches it with alice seated on an offset cell.
func launchTheMadKing(t *testing.T, h *acceptanceHarness, run string, col, row int) *sdk.LaunchOutput {
	t.Helper()
	ctx := auth.WithPlayerID(context.Background(), "player-alice")
	_, err := h.charRepo.Create(ctx, characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter("alice", "player-alice")},
	})
	require.NoError(t, err)

	raw, err := os.ReadFile(filepath.Join(dungeonstest.ContentDir(t), madKingKey+".yaml")) //nolint:gosec // the shipped content directory
	require.NoError(t, err)
	dungeon, err := sessionworld.Compile(raw)
	require.NoError(t, err)

	return h.launch(t, run, dungeon.Spec, seatAt("alice", col, row))
}

// launched is the member a launch placed under an id.
func launched(t *testing.T, out *sdk.LaunchOutput, id string) sdk.Member {
	t.Helper()
	for _, m := range out.Members {
		if m.ID == id {
			return m
		}
	}
	require.FailNowf(t, "not placed", "%s", id)

	return sdk.Member{}
}

// TestAcceptance_TheGuardHoldsTheGate is the guard holds the gate on the
// server (rpg-project#836 S1, B1 on the castle the api serves): the posted
// gate guard stands the gate's one crossing, the cell beneath the gate in
// front of the shut door, and a walk past him stops one cell short with a
// reason that names him. The other gate guard is not posted, and a walk
// crosses her cell as before.
func TestAcceptance_TheGuardHoldsTheGate(t *testing.T) {
	ctx := auth.WithPlayerID(context.Background(), "player-alice")

	t.Run("a walk past the posted guard stops one cell short, by his name", func(t *testing.T) {
		h := newAcceptanceHarness(t)
		// Beside the crossing, along the gate wall, the posted guard east of her.
		out := launchTheMadKing(t, h, "gate-held-run", -1, 7)
		require.Empty(t, out.Formed, "precondition: the gate guards are neutral, nobody fights")
		guard := launched(t, out, gatePosted)
		require.Equal(t, at(0, 7), guard.Position, "precondition: he is posted beneath the gate")
		require.NotEmpty(t, guard.Name, "precondition: he has a name to be named by")

		// Through his cell toward the courtyard beyond the shut gate door.
		moved, err := h.handler.Move(ctx, &sessionpb.MoveRequest{
			Session: "gate-held-run", Member: "alice", Path: []*sessionpb.Position{pbAt(0, 7), pbAt(0, 6)},
		})
		require.NoError(t, err)
		require.Equal(t, sessionpb.MovementStatus_MOVEMENT_STATUS_STOPPED, moved.GetStatus(), "the walk stops")
		require.Empty(t, moved.GetSteps(), "one cell short of him: no step was taken")
		require.Equal(t, guard.Name+" stands in the way", moved.GetStopReason(), "and the reason names the guard")

		where, err := h.handler.GetWhere(ctx, &sessionpb.GetWhereRequest{Session: "gate-held-run", Member: "alice"})
		require.NoError(t, err)
		require.Equal(t, pbAt(-1, 7).GetX(), where.GetPosition().GetX())
		require.Equal(t, pbAt(-1, 7).GetY(), where.GetPosition().GetY())
	})

	t.Run("a walk past the other gate guard crosses her cell as before", func(t *testing.T) {
		h := newAcceptanceHarness(t)
		// South of her on the gatehouse floor.
		out := launchTheMadKing(t, h, "gate-loop-run", 2, 11)
		require.Empty(t, out.Formed, "precondition: nobody fights")
		her := launched(t, out, gateLoop).Position
		require.Equal(t, at(2, 10), her, "precondition: she stands where the file places her")

		beyond := at(1, 9)
		path := []*sessionpb.Position{{X: her.X, Y: her.Y}, {X: beyond.X, Y: beyond.Y}}
		moved, err := h.handler.Move(ctx, &sessionpb.MoveRequest{Session: "gate-loop-run", Member: "alice", Path: path})
		require.NoError(t, err)
		require.Equal(t, sessionpb.MovementStatus_MOVEMENT_STATUS_COMPLETED, moved.GetStatus(), moved.GetStopReason())
		require.Len(t, moved.GetSteps(), len(path), "through her cell and beyond it")
	})
}

// The castle gate's footprint door and the three cells of its opening, in the
// offset coordinates the launch seats by: beneath the gate is gate-1's post,
// beside it the opening's other gatehouse-side cell, past it the courtyard.
const castleGateDoor = "the-mad-king/gate-wall-door"

func beneathTheGate() *sessionpb.Position { return pbAt(0, 7) }
func besideTheGate() *sessionpb.Position  { return pbAt(-1, 7) }
func pastTheGate() *sessionpb.Position    { return pbAt(0, 6) }

func samePosition(a, b *sessionpb.Position) bool {
	return a.GetX() == b.GetX() && a.GetY() == b.GetY()
}

// TestAcceptance_TheGuardHoldsTheGatesPassage is the guard holds the gate's
// passage on the server (rpg-project#852 S1b, R5): posted gate-1 holds the
// whole opening of the gate door, so with the door open a walk through the
// opening's OTHER cell stops at the opening by his name; the view says which
// door he holds; the atlas lists the gate's crossings marked as crossings;
// the door list carries the gate with its state; and the other gate guard,
// who holds no post, is walked past as before.
func TestAcceptance_TheGuardHoldsTheGatesPassage(t *testing.T) {
	ctx := auth.WithPlayerID(context.Background(), "player-alice")
	h := newAcceptanceHarness(t)
	const run = "gate-passage-run"
	out := launchTheMadKing(t, h, run, -1, 7)
	require.Empty(t, out.Formed, "precondition: the gate guards are neutral, nobody fights")
	guard := launched(t, out, gatePosted)
	require.True(t, samePosition(beneathTheGate(), &sessionpb.Position{X: guard.Position.X, Y: guard.Position.Y}),
		"precondition: he stands his post beneath the gate")

	t.Run("the door list carries the gate with its state", func(t *testing.T) {
		doors, err := h.handler.GetDoors(ctx, &sessionpb.GetDoorsRequest{Session: run, Member: "alice"})
		require.NoError(t, err)
		var gate *sessionpb.DoorInfo
		for _, d := range doors.GetDoors() {
			if d.GetDoor() == castleGateDoor {
				gate = d
			}
		}
		require.NotNil(t, gate, "the castle's footprint gate door is in the door list")
		require.NotEqual(t, sessionpb.DoorState_DOOR_STATE_UNSPECIFIED, gate.GetState(), "with its state")
	})

	t.Run("the atlas lists the gate's two crossings, marked", func(t *testing.T) {
		atlas, err := h.handler.GetAtlas(ctx, &sessionpb.GetAtlasRequest{Session: run, Member: "alice"})
		require.NoError(t, err)
		// Every door the castle draws is a footprint door, so the edge-door
		// half (crossing false) is pinned by the converter tests.
		var gate []*sessionpb.AtlasDoorway
		for _, d := range atlas.GetDoorways() {
			if d.GetConnection() == castleGateDoor {
				gate = append(gate, d)
			}
		}
		require.Len(t, gate, 2, "the gate's opening has two crossings")
		for _, d := range gate {
			require.True(t, d.GetCrossing(), "a footprint door's crossing %v-%v is marked", d.GetFrom(), d.GetTo())
			require.True(t, samePosition(d.GetFrom(), pastTheGate()) || samePosition(d.GetTo(), pastTheGate()),
				"every crossing of the gate reaches the courtyard cell past it")
		}
	})

	t.Run("with the door open, the opening's other cell is barred by his name", func(t *testing.T) {
		opened, err := h.handler.OpenDoor(ctx, &sessionpb.OpenDoorRequest{Session: run, Member: "alice", Door: castleGateDoor})
		require.NoError(t, err)
		require.Equal(t, sessionpb.DoorState_DOOR_STATE_OPEN, opened.GetDoor().GetState(), "precondition: the gate stands open")

		view, err := h.handler.GetView(ctx, &sessionpb.GetViewRequest{Session: run, Member: "alice"})
		require.NoError(t, err)
		var seen *sessionpb.Sighting
		for _, s := range view.GetSightings() {
			if s.GetSubject() == gatePosted {
				seen = s
			}
		}
		require.NotNil(t, seen, "precondition: alice sees the posted guard")
		require.Equal(t, castleGateDoor, seen.GetHoldsDoor(), "her sighting of him names the door he holds")

		moved, err := h.handler.Move(ctx, &sessionpb.MoveRequest{
			Session: run, Member: "alice", Path: []*sessionpb.Position{pastTheGate()},
		})
		require.NoError(t, err)
		require.Equal(t, sessionpb.MovementStatus_MOVEMENT_STATUS_STOPPED, moved.GetStatus(), "the held opening stops the walk")
		require.Empty(t, moved.GetSteps(), "no step was taken")
		require.Equal(t, guard.Name+" bars the way", moved.GetStopReason(), "and the reason names the guard")

		where, err := h.handler.GetWhere(ctx, &sessionpb.GetWhereRequest{Session: run, Member: "alice"})
		require.NoError(t, err)
		require.True(t, samePosition(besideTheGate(), where.GetPosition()), "she stands at the opening")
	})
}
