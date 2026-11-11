package session_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

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

	// gateCaptain is the captain, the third gate guard. Every gate guard
	// holds the gate (rpg-project#928), and he stands nearest the opening's
	// gatehouse side, so a stop or a refusal there names him: the stop names
	// the nearest holder its mover sees.
	gateCaptain = "captain"
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
		// The first step lands inside the gate's approach in her sight, but she
		// has no line that would move the gate interaction on for a stranger:
		// the guest claim is the captain's alone, the troupe's line wants the
		// feast, and the vouch and the straggler's case want someone admitted
		// (rpg-project#929). A hail with nothing to say is not raised, so the
		// walk goes on through her cell.
		moved, err := h.handler.Move(ctx, &sessionpb.MoveRequest{Session: "gate-loop-run", Member: "alice", Path: path})
		require.NoError(t, err)
		require.Equal(t, sessionpb.MovementStatus_MOVEMENT_STATUS_COMPLETED, moved.GetStatus(), moved.GetStopReason())
		require.NotEqual(t, gateLoop, speakRowOf(t, h, "gate-loop-run", "alice", "player-alice").GetAddress().GetGuardMemberId(),
			"she does not address her; the captain's standing address, from launch, is the only one")
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
// passage on the server (rpg-project#852 S1b, R5 and R7): posted gate-1 holds
// the whole opening of the gate door. A walk through the opening's OTHER cell
// stops at the opening by his name, open door or shut; the view says which
// door he holds; the atlas lists the gate's crossings marked as crossings;
// the door list carries the gate with its state; opening the held gate is
// refused by his name; and once he is cowed the gate opens and the walk
// crosses. The walk past the other gate guard, who holds no post, is
// TestAcceptance_TheGuardHoldsTheGate's.
func TestAcceptance_TheGuardHoldsTheGatesPassage(t *testing.T) {
	ctx := auth.WithPlayerID(context.Background(), "player-alice")
	h := newAcceptanceHarnessWithDice(t, standAsideDice{})
	const run = "gate-passage-run"
	out := launchTheMadKing(t, h, run, -1, 7)
	require.Empty(t, out.Formed, "precondition: the gate guards are neutral, nobody fights")
	guard := launched(t, out, gatePosted)
	require.True(t, samePosition(beneathTheGate(), &sessionpb.Position{X: guard.Position.X, Y: guard.Position.Y}),
		"precondition: he stands his post beneath the gate")
	nearest := launched(t, out, gateCaptain)

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
		// The opening's two crossings, each joining a gatehouse-side cell to
		// the courtyard cell past the gate. Pinned by name, not by count, so
		// a wider opening adds crossings without breaking this.
		crossings := []struct {
			name     string
			from, to *sessionpb.Position
		}{
			{"beneath the gate to past it", beneathTheGate(), pastTheGate()},
			{"beside the gate to past it", besideTheGate(), pastTheGate()},
		}
		for _, want := range crossings {
			var found *sessionpb.AtlasDoorway
			for _, d := range atlas.GetDoorways() {
				if d.GetConnection() != castleGateDoor {
					continue
				}
				if (samePosition(d.GetFrom(), want.from) && samePosition(d.GetTo(), want.to)) ||
					(samePosition(d.GetFrom(), want.to) && samePosition(d.GetTo(), want.from)) {
					found = d
				}
			}
			require.NotNilf(t, found, "the gate's crossing %s is a doorway of the map", want.name)
			require.Truef(t, found.GetCrossing(), "the gate's crossing %s is marked crossing", want.name)
		}
	})

	t.Run("the opening's other cell is barred by the nearest holder's name", func(t *testing.T) {
		view, err := h.handler.GetView(ctx, &sessionpb.GetViewRequest{Session: run, Member: "alice"})
		require.NoError(t, err)
		require.Equal(t, castleGateDoor, sightingOf(t, view, gatePosted).GetHoldsDoor(), "her sighting of him names the door he holds")

		moved, err := h.handler.Move(ctx, &sessionpb.MoveRequest{
			Session: run, Member: "alice", Path: []*sessionpb.Position{pastTheGate()},
		})
		require.NoError(t, err)
		require.Equal(t, sessionpb.MovementStatus_MOVEMENT_STATUS_STOPPED, moved.GetStatus(), "the held opening stops the walk")
		require.Empty(t, moved.GetSteps(), "no step was taken")
		require.Equal(t, nearest.Name+" bars the way", moved.GetStopReason(), "and the reason names the nearest holder, the captain")

		where, err := h.handler.GetWhere(ctx, &sessionpb.GetWhereRequest{Session: run, Member: "alice"})
		require.NoError(t, err)
		require.True(t, samePosition(besideTheGate(), where.GetPosition()), "she stands at the opening")
	})

	// R7 (lane #835): a held door refuses its verbs. Opening the gate from
	// beside him is refused by his name, and is the client's to show.
	t.Run("opening the held gate is refused by the nearest holder's name", func(t *testing.T) {
		_, err := h.handler.OpenDoor(ctx, &sessionpb.OpenDoorRequest{Session: run, Member: "alice", Door: castleGateDoor})
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		require.Equal(t, codes.FailedPrecondition, st.Code())
		require.Equal(t, nearest.Name+" bars the door", st.Message(), "the refusal names the nearest holder, the captain")
	})

	// The captain stands beside her (offset (0,8)). A stand-aside he hears
	// cows every gate guard, and a cowed guard holds nothing: the door opens
	// and the walk crosses.
	t.Run("once he is cowed, the gate opens and the walk crosses", func(t *testing.T) {
		_, err := speakAsOffered(ctx, t, h, run, "alice", "captain", "stand-aside")
		require.NoError(t, err)
		view, err := h.handler.GetView(ctx, &sessionpb.GetViewRequest{Session: run, Member: "alice"})
		require.NoError(t, err)
		require.Empty(t, sightingOf(t, view, gatePosted).GetHoldsDoor(), "a cowed guard holds no door")

		opened, err := h.handler.OpenDoor(ctx, &sessionpb.OpenDoorRequest{Session: run, Member: "alice", Door: castleGateDoor})
		require.NoError(t, err)
		require.Equal(t, sessionpb.DoorState_DOOR_STATE_OPEN, opened.GetDoor().GetState(), "the gate opens")

		moved, err := h.handler.Move(ctx, &sessionpb.MoveRequest{
			Session: run, Member: "alice", Path: []*sessionpb.Position{pastTheGate()},
		})
		require.NoError(t, err)
		require.Equal(t, sessionpb.MovementStatus_MOVEMENT_STATUS_COMPLETED, moved.GetStatus(), moved.GetStopReason())
		require.Len(t, moved.GetSteps(), 1, "she steps through the gate")
	})
}

// sightingOf is the view's sighting of subject, failing when it holds none.
func sightingOf(t *testing.T, view *sessionpb.GetViewResponse, subject string) *sessionpb.Sighting {
	t.Helper()
	for _, s := range view.GetSightings() {
		if s.GetSubject() == subject {
			return s
		}
	}
	require.FailNowf(t, "no sighting", "the view holds no sighting of %s", subject)

	return nil
}

// standAsideDice rolls every d20 at 20 and every other die at 1: the
// captain's stand-aside check passes and its weighted outcome takes the
// first row, the stand-aside that cows the gate (the session's castle tests
// roll the same way). A max-everything roller takes the last row, "GUARDS!".
type standAsideDice struct{}

func (standAsideDice) Roll(_ context.Context, size int) (int, error) {
	if size == 20 {
		return 20, nil
	}
	return 1, nil
}
