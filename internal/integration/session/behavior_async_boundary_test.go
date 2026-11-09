package session_test

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/stretchr/testify/require"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	sessionhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// The gate controls scheduling only, then hands the unchanged continuation to
// the production runner. Compiler, composer, world, lock, persistence and
// delivery are real. No test driver chooses a monster's move.
func (s *BehaviorPatrolSuite) TestCanonicalWorldAfterEarlyAnswerKeepsItsGuardAndRunsOnce() {
	t := s.T()
	ctx := auth.WithPlayerID(context.Background(), "player-alice")
	die := &behaviorScript{faces: []behaviorFace{{100, 1}, {100, 1}}}
	h := newAcceptanceHarnessAnswering(t, die, nil, false)
	gate := &heldContinuations{inner: sessionorch.NewInProcessContinuationRunner()}
	orch, err := sessionorch.New(sessionorch.Config{Redis: h.redis, Characters: h.charRepo, TTL: time.Hour, Dice: die, Continuations: gate, PresentationIDs: idgen.NewSequential("async-behavior")})
	require.NoError(t, err)
	t.Cleanup(func() {
		drain, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		require.NoError(t, orch.Drain(drain))
	})
	h.manager = orch
	h.handler, err = sessionhandler.New(&sessionhandler.HandlerConfig{Manager: orch.Manager, Characters: h.charRepo, Preferences: orch.Preferences})
	require.NoError(t, err)
	_, err = h.charRepo.Create(ctx, characterrepo.CreateInput{Character: &entities.Character{Data: armedFighter("alice", "player-alice")}})
	require.NoError(t, err)
	raw, err := os.ReadFile("../../orchestrators/authoring/testdata/behavior-patrol-consumer.yaml")
	require.NoError(t, err)
	dungeon, err := sessionworld.Compile(raw)
	require.NoError(t, err)
	const run = "canonical-delayed-world"
	h.launchAtStart(t, run, dungeon.Spec, "alice")
	repository := sessionorch.NewEncounterRepository(h.redis, time.Hour)
	initial, err := repository.GetEncounter(ctx, run)
	require.NoError(t, err)
	request, cancelRequest := context.WithCancel(ctx)
	answer, err := h.handler.Move(request, &sessionpb.MoveRequest{Session: run, Member: "alice", Path: []*sessionpb.Position{{X: 1, Y: 2}, {X: 2, Y: 2}, {X: 3, Y: 2}, {X: 4, Y: 2}, {X: 5, Y: 2}, {X: 6, Y: 2}}})
	require.NoError(t, err)
	cancelRequest()
	require.True(t, answer.GetContinues())
	require.Len(t, answer.GetSteps(), 5)
	require.NotEmpty(t, answer.GetThrough())
	require.Empty(t, answer.GetSaved().GetWritten())
	require.Zero(t, die.calls, "no NPC selection happened before the held world work")
	stillStored, err := repository.GetEncounter(ctx, run)
	require.NoError(t, err)
	require.Equal(t, initial, stillStored, "an early answer is not a partial save")
	blocked, cancelBlocked := context.WithTimeout(context.Background(), 20*time.Millisecond)
	_, err = orch.Manager.Where(blocked, &sdk.WhereInput{Session: run, Member: "alice"})
	cancelBlocked()
	require.ErrorIs(t, err, context.DeadlineExceeded, "a following verb waits for this continuation's guard")
	dungeon.Spec.Monsters[0].Behaviors.Instances[1].Instance.Recipe.Ref.ID = "edited-after-answer"
	gate.release(t)
	drain, cancelDrain := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelDrain()
	require.NoError(t, orch.Drain(drain))
	require.Equal(t, 2, die.calls)
	where, err := h.handler.GetWhere(ctx, &sessionpb.GetWhereRequest{Session: run, Member: "alice"})
	require.NoError(t, err)
	require.Equal(t, 6.0, where.GetPosition().GetX())
	after, err := repository.GetEncounter(ctx, run)
	require.NoError(t, err)
	found := false
	for i, m := range after.Members {
		if m.ID == "west" {
			found = true
			require.Equal(t, initial.Members[i].Behaviors.Definition, m.Behaviors.Definition)
			require.Equal(t, "item/1/walk", m.Behaviors.Progress.Instances[1].Blob.State)
		}
	}
	require.True(t, found, "the continued world retains its independently progressing guard")
	record, err := orch.Manager.Record(ctx, &sdk.RecordReadInput{Session: run, AfterID: answer.GetThrough()})
	require.NoError(t, err)
	playerSteps := 0
	for _, event := range record.Events {
		if event.Beat == "moved" {
			var body struct {
				Member string `json:"member"`
			}
			require.NoError(t, json.Unmarshal(event.Fields, &body))
			if body.Member == "alice" {
				playerSteps++
			}
		}
	}
	require.Equal(t, 1, playerSteps, "only the unpaid-at-answer sixth player step follows Through")
	_, err = gate.held.Continue(context.Background())
	require.ErrorIs(t, err, sdk.ErrContinuationSpent)
	require.Equal(t, 2, die.calls, "a repeated callback cannot select either patrol again")
}
