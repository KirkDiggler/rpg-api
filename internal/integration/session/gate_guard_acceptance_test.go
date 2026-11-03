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
