package session_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

// A MOVE ANSWERS BEFORE THE WORLD RUNS (rpg-project#879 R2), through the
// production wiring: a free-roam walk longer than one pace answers with the
// steps before the one that pays a round, and the rest -- that step, the
// steps after, the save and the delivery -- runs on the orchestrator's
// continuation runner. Once the runner drains, the store holds the whole
// walk: a fresh orchestrator over the same Redis finds alice at the route's
// end, and the record carries every step the answer did not.
func TestAcceptance_AMoveAnswersBeforeTheWorldRuns(t *testing.T) {
	h := newAcceptanceHarnessAnswering(t, testDice{}, nil, false)
	ctx := auth.WithPlayerID(context.Background(), "player-alice")
	const run = "continuation-run"

	_, err := h.charRepo.Create(context.Background(), characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter("alice", "player-alice")},
	})
	require.NoError(t, err)

	// The tomb with nobody in it: nothing forms a fight, so the whole route
	// is free roam on the world clock and a 30 ft walker pays a round every
	// six cells.
	h.launch(t, run, buildThreeRoomTomb(t), seatAt("alice", 1, 1))

	route := tombRoute()
	answer, err := h.handler.Move(ctx, &sessionpb.MoveRequest{Session: run, Member: "alice", Path: route})
	require.NoError(t, err)

	require.True(t, answer.GetContinues(), "a route longer than a pace answers before the round is paid")
	require.Equal(t, sessionpb.MovementStatus_MOVEMENT_STATUS_CONTINUING, answer.GetStatus())
	answered := len(answer.GetSteps())
	require.Positive(t, answered, "the steps before the paying one are in the answer")
	require.Less(t, answered, len(route), "the rest of the walk is not")
	require.NotEmpty(t, answer.GetThrough(), "the answer names where the stream continues")
	require.Empty(t, answer.GetSaved().GetWritten(), "nothing is saved before the rest has run")
	require.Nil(t, answer.GetFormed())

	drain, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, h.manager.Drain(drain), "the continuation finishes")

	// A fresh orchestrator reads only what was stored.
	fresh, err := sessionorch.New(sessionorch.Config{
		Redis: h.redis, Characters: h.charRepo, Dice: testDice{}, AnswerWhole: true,
	})
	require.NoError(t, err)
	end := route[len(route)-1]
	where, err := fresh.Manager.Where(context.Background(), &sdk.WhereInput{Session: run, Member: "alice"})
	require.NoError(t, err)
	require.Equal(t, spatial.Position{X: float64(end.GetX()), Y: float64(end.GetY())}, where.Position,
		"the stored walk reached the route's end")

	rest, err := fresh.Manager.Record(context.Background(), &sdk.RecordReadInput{Session: run, AfterID: answer.GetThrough()})
	require.NoError(t, err)
	moved := 0
	for _, e := range rest.Events {
		if e.Beat == "moved" {
			moved++
		}
	}
	require.Equal(t, len(route)-answered, moved, "every step after the answer's is on the record after Through")
}
