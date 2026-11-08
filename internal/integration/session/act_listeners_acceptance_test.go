package session_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	storypb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

// walkBy is the first walk act by mover in acts, or nil.
func walkBy(acts []*storypb.Act, mover string) *storypb.Act {
	for _, a := range acts {
		if a.GetWalk().GetMover().GetId() == mover {
			return a
		}
	}
	return nil
}

// TestAcceptance_OnlyTheListenerIsFoldedLiveAndTheOtherCatchesUp is
// rpg-project#879 S7 on the api: the act broker names who listens, so a
// commit folds live only for them. alice holds a real StreamEvents call and
// bob holds none. alice's Move still reaches her live, act for act as her
// GetStory reads it; bob, folded for nobody, reads the same walk on catch-up.
func TestAcceptance_OnlyTheListenerIsFoldedLiveAndTheOtherCatchesUp(t *testing.T) {
	h := newAcceptanceHarness(t)
	ctxAlice := auth.WithPlayerID(context.Background(), "player-alice")
	ctxBob := auth.WithPlayerID(context.Background(), "player-bob")
	const run = "act-listeners-run"

	for _, seat := range []struct{ id, player string }{{"alice", "player-alice"}, {"bob", "player-bob"}} {
		_, err := h.charRepo.Create(context.Background(), characterrepo.CreateInput{
			Character: &entities.Character{Data: armedFighter(seat.id, seat.player)},
		})
		require.NoError(t, err)
	}
	out := h.launch(t, run, buildOpenRoom(t, 12, 6), seatAt("alice", 1, 1), seatAt("bob", 4, 1))
	require.Empty(t, out.Formed, "precondition: free roam, no monsters")

	listening, err := h.manager.ActBroker.Listening(context.Background(), run)
	require.NoError(t, err)
	require.Empty(t, listening, "precondition: nobody streams yet")

	var moved *sessionpb.MoveResponse
	live := watchActs(ctxAlice, t, h, run, "alice", func() {
		listening, err := h.manager.ActBroker.Listening(context.Background(), run)
		require.NoError(t, err)
		require.Equal(t, []string{"alice"}, listening, "alice's stream is the only listener")

		moved, err = h.handler.Move(ctxAlice, &sessionpb.MoveRequest{
			Session: run, Member: "alice", Path: []*sessionpb.Position{pbAt(2, 1)},
		})
		require.NoError(t, err)
	})
	require.Equal(t, sessionpb.MovementStatus_MOVEMENT_STATUS_COMPLETED, moved.GetStatus(), moved.GetStopReason())

	require.NotNil(t, walkBy(live, "alice"), "the subscribed player receives her own walk live")
	requireLiveIsStory(ctxAlice, t, h, run, "alice", live)

	require.NotNil(t, walkBy(storyOf(ctxBob, t, h.story, run, "bob"), "alice"),
		"bob, never listening, reads alice's walk on catch-up")
}
