package session_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/encoding/protojson"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	sessionhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
)

type BehaviorPatrolSuite struct{ suite.Suite }

func TestBehaviorPatrolSuite(t *testing.T) { suite.Run(t, new(BehaviorPatrolSuite)) }

type patrolDice struct{ choices int }

func (d *patrolDice) Roll(_ context.Context, sides int) (int, error) {
	if sides != 100 || d.choices >= 4 {
		return 0, fmt.Errorf("unexpected d%d after %d choices", sides, d.choices)
	}
	d.choices++
	return 1, nil
}

func (s *BehaviorPatrolSuite) TestBehaviorPatrolAcceptance() {
	ctx := auth.WithPlayerID(context.Background(), "player-alice")
	die := &patrolDice{}
	h := newAcceptanceHarnessWithDice(s.T(), die)
	_, err := h.charRepo.Create(ctx, characterrepo.CreateInput{Character: &entities.Character{Data: armedFighter("alice", "player-alice")}})
	s.Require().NoError(err)
	raw, err := os.ReadFile(filepath.Join(dungeonstest.ContentDir(s.T()), "..", "internal", "orchestrators", "authoring", "testdata", "behavior-patrol-consumer.yaml"))
	s.Require().NoError(err)
	dungeon, err := sessionworld.Compile(raw)
	s.Require().NoError(err)
	const run = "canonical-behavior-run"
	h.launchAtStart(s.T(), run, dungeon.Spec, "alice")
	repository := sessionorch.NewEncounterRepository(h.redis, time.Hour)
	before, err := repository.GetEncounter(ctx, run)
	s.Require().NoError(err)
	configurations := map[encounter.MemberID]string{}
	for _, m := range before.Members {
		if m.Behaviors != nil {
			configurations[m.ID] = m.Behaviors.Definition
		}
	}
	s.Require().Contains(configurations, encounter.MemberID("west"))
	s.Require().Contains(configurations, encounter.MemberID("court"))
	s.Zero(die.choices)
	// Editing producer configuration after launch must not change the saved run.
	dungeon.Spec.Monsters[0].Behaviors.Instances[1].Instance.Recipe.Ref.ID = "edited-after-launch"
	for round, path := range [][]*sessionpb.Position{
		{{X: 1, Y: 2}, {X: 2, Y: 2}, {X: 3, Y: 2}, {X: 4, Y: 2}, {X: 5, Y: 2}, {X: 6, Y: 2}},
		{{X: 5, Y: 2}, {X: 4, Y: 2}, {X: 3, Y: 2}, {X: 2, Y: 2}, {X: 1, Y: 2}, {X: 0, Y: 2}},
	} {
		// Recreate the real manager/handler over the same Redis between verbs.
		orch, makeErr := sessionorch.New(sessionorch.Config{Redis: h.redis, Characters: h.charRepo, TTL: time.Hour, Dice: die, PresentationIDs: idgen.NewSequential(fmt.Sprintf("reload-%d", round))})
		s.Require().NoError(makeErr)
		handler, makeErr := sessionhandler.New(&sessionhandler.HandlerConfig{Manager: orch.Manager, Characters: h.charRepo, Preferences: orch.Preferences})
		s.Require().NoError(makeErr)
		moved, moveErr := handler.Move(ctx, &sessionpb.MoveRequest{Session: run, Member: "alice", Path: path})
		s.Require().NoError(moveErr)
		s.Equal(sessionpb.MovementStatus_MOVEMENT_STATUS_COMPLETED, moved.GetStatus())
		s.Equal(2*(round+1), die.choices)
		stored, readErr := repository.GetEncounter(ctx, run)
		s.Require().NoError(readErr)
		for _, m := range stored.Members {
			if want, ok := configurations[m.ID]; ok {
				s.Require().NotNil(m.Behaviors)
				s.Equal(want, m.Behaviors.Definition)
			}
		}
		view, readErr := handler.GetView(ctx, &sessionpb.GetViewRequest{Session: run, Member: "alice"})
		s.Require().NoError(readErr)
		for _, private := range []string{"behaviorDefinitions", "ComposerData", "item/0/walk"} {
			s.NotContains(protojson.Format(view), private)
		}
		s.Equal(2*(round+1), die.choices, "a player read never selects an NPC action")
	}
	stored, err := repository.GetEncounter(ctx, run)
	s.Require().NoError(err)
	found := false
	for _, m := range stored.Members {
		if m.ID == "west" {
			found = true
			s.Require().NotNil(m.Cell)
			s.Equal(2.0, m.Cell.X)
			s.Equal(0.0, m.Cell.Y)
		}
	}
	s.True(found, "the independently progressing west guard remains seated")
}
