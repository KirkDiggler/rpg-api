package session_test

// gate_exit_acceptance_test.go is leaving while addressed (rpg-project#942)
// on the server the api serves, for two players. Alice is hailed by the
// captain, misses her troupe claim so his address on her still stands, and
// leaves the run. Before encounter v0.201.1 every later verb in that run
// failed for everyone: the stored run named a member it no longer had.
// Now bob can still move, speak and leave, and both can start another run.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
)

// tonightLine is the stranger's line that teaches the feast; troupeLine the
// gate's troupe claim (the-mad-king.yaml).
const (
	tonightLine = "tonight"
	troupeLine  = "troupe"
)

// frontGateAddressee is who the front gate's stored address names, or "".
func frontGateAddressee(t *testing.T, h *acceptanceHarness, run string) string {
	t.Helper()
	world, err := sessionorch.NewEncounterRepository(h.redis, 0).GetEncounter(context.Background(), run)
	require.NoError(t, err)
	for _, in := range world.Interactions {
		if in.ID == frontGate && in.Address != nil {
			return string(in.Address.Member)
		}
	}
	return ""
}

// TestAcceptance_LeavingWhileAddressedLeavesARunTheOthersCanPlay: alice is
// hailed and leaves with the captain's address on her; bob moves, speaks
// and leaves; then both start a new run.
func TestAcceptance_LeavingWhileAddressedLeavesARunTheOthersCanPlay(t *testing.T) {
	const run = "gate-exit-run"
	alice := auth.WithPlayerID(context.Background(), "player-alice")
	bob := auth.WithPlayerID(context.Background(), "player-bob")
	dice := &gateDice{}
	dice.face.Store(20)
	h := newAcceptanceHarnessWithDice(t, dice)
	for _, c := range []struct{ id, player string }{{"alice", "player-alice"}, {"bob", "player-bob"}} {
		_, err := h.charRepo.Create(alice, characterrepo.CreateInput{
			Character: &entities.Character{Data: armedFighter(c.id, c.player)},
		})
		require.NoError(t, err)
	}
	dungeon, err := sessionworld.Compile(madKingRaw(t))
	require.NoError(t, err)
	// Alice beside the stranger outside; bob further down the road.
	out := h.launch(t, run, dungeon.Spec, seatAt("alice", 6, 15), seatAt("bob", 8, 16))
	require.Empty(t, out.Formed, "precondition: nobody fights")

	_, err = sayTo(t, h, run, "alice", "player-alice", "pedlar", tonightLine)
	require.NoError(t, err, "the stranger tells alice of the feast")
	hailed, err := h.handler.Move(alice, &sessionpb.MoveRequest{Session: run, Member: "alice", Path: toTheCaptain()})
	require.NoError(t, err)
	require.Equal(t, sessionpb.MovementStatus_MOVEMENT_STATUS_STOPPED, hailed.GetStatus(), "precondition: the captain hails her")
	dice.face.Store(2)
	_, err = sayTo(t, h, run, "alice", "player-alice", "captain", troupeLine)
	require.NoError(t, err, "alice makes the troupe claim, and misses")
	require.Equal(t, "alice", frontGateAddressee(t, h, run), "precondition: the captain's address on her still stands")

	_, err = h.handler.Exit(alice, &sessionpb.ExitRequest{Session: run, Member: "alice"})
	require.NoError(t, err, "alice leaves the run")
	require.Empty(t, frontGateAddressee(t, h, run), "her leaving ended the address")

	moved := walkAs(t, h, run, "bob", "player-bob", []*sessionpb.Position{pbAt(7, 16)})
	require.NotEmpty(t, moved.GetSteps(), "bob can still move: %s", moved.GetStopReason())
	dice.face.Store(20)
	_, err = sayTo(t, h, run, "bob", "player-bob", "pedlar", tonightLine)
	require.NoError(t, err, "bob can still speak")
	_, err = h.handler.Exit(bob, &sessionpb.ExitRequest{Session: run, Member: "bob"})
	require.NoError(t, err, "bob can still leave")

	// Any dungeon shows their seats are free; the small tomb keeps the
	// second launch off the castle's clock.
	again := h.launch(t, "gate-exit-run-again", buildThreeRoomTomb(t), seatAt("alice", 1, 1), seatAt("bob", 2, 1))
	require.NotEmpty(t, again.Members, "both start a new run")
}
