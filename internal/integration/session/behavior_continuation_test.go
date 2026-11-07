package session_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	sessionhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

type behaviorFace struct{ sides, value int }
type behaviorScript struct {
	faces []behaviorFace
	calls int
}

func (d *behaviorScript) Roll(_ context.Context, sides int) (int, error) {
	if d.calls >= len(d.faces) {
		return 0, fmt.Errorf("unexpected d%d after script exhausted (%d rolls)", sides, d.calls)
	}
	face := d.faces[d.calls]
	if face.sides != sides {
		return 0, fmt.Errorf("roll %d: want d%d, got d%d", d.calls, face.sides, sides)
	}
	d.calls++
	return face.value, nil
}

func (s *BehaviorPatrolSuite) reloadBehaviorHost(h *acceptanceHarness, die sdk.Roller, label string) {
	orch, err := sessionorch.New(sessionorch.Config{Redis: h.redis, Characters: h.charRepo, TTL: time.Hour, Dice: die, PresentationIDs: idgen.NewSequential(label)})
	s.Require().NoError(err)
	handler, err := sessionhandler.New(&sessionhandler.HandlerConfig{Manager: orch.Manager, Characters: h.charRepo, Preferences: orch.Preferences})
	s.Require().NoError(err)
	h.manager, h.handler = orch, handler
	h.story = newStoryHandler(s.T(), orch, h.charRepo)
}

func (s *BehaviorPatrolSuite) storedBehavior(h *acceptanceHarness, run, id string) *encounter.MemberData {
	data, err := sessionorch.NewEncounterRepository(h.redis, time.Hour).GetEncounter(context.Background(), run)
	s.Require().NoError(err)
	for i := range data.Members {
		if string(data.Members[i].ID) == id {
			return &data.Members[i]
		}
	}
	s.Require().FailNow("actor is stored", id)
	return nil
}

func (s *BehaviorPatrolSuite) TestHeldBehaviorSurvivesRedisReloadAndSourceEdit() {
	ctx := auth.WithPlayerID(context.Background(), "player-alice")
	const run = "canonical-held-run"
	// The actual initiative rolls, then the sole behavior choice. A resume or
	// read that chooses again fails the script rather than receiving spare dice.
	die := &behaviorScript{faces: []behaviorFace{{20, 15}, {20, 15}, {100, 1}}}
	h := newAcceptanceHarnessWithDice(s.T(), die)
	_, err := h.charRepo.Create(ctx, characterrepo.CreateInput{Character: &entities.Character{Data: armedFighter("alice", "player-alice")}})
	s.Require().NoError(err)
	raw, err := os.ReadFile("testdata/behavior-held.yaml")
	s.Require().NoError(err)
	dungeon, err := sessionworld.Compile(raw)
	s.Require().NoError(err)
	launched := h.launchAtStart(s.T(), run, dungeon.Spec, "alice")
	s.Require().Len(launched.Formed, 1)
	s.Equal([]string{"alice", "guard"}, launched.Formed[0].Order)
	_, err = h.handler.EndTurn(ctx, &sessionpb.EndTurnRequest{Session: run, Member: "alice", DeclarationId: currentDeclarationID(ctx, s.T(), h.handler, run, "alice", sessionpb.Verb_VERB_END_TURN)})
	s.Require().NoError(err)
	data, err := sessionorch.NewEncounterRepository(h.redis, time.Hour).GetEncounter(ctx, run)
	s.Require().NoError(err)
	s.Require().NotNil(data.Pause, "the real opportunity reaction paused the selected walk")
	s.Require().NotNil(data.Pause.Turn)
	claim := data.Pause.Turn.Behavior
	s.Require().NotNil(claim)
	s.Equal("duty", claim.Instance)
	s.Require().NotNil(claim.Goal)
	s.Equal(6.0, claim.Goal.X)
	s.Equal(20, data.Pause.Turn.Budget.MovementFeet)
	before := s.storedBehavior(h, run, "guard")
	s.Equal("item/0/walk", before.Behaviors.Progress.Instances[0].Blob.State)
	s.Equal(spatial.Position{X: 2}, whereIs(s.T(), h, run, "guard"))
	// A new source compile has a different destination. It is not the saved
	// run's configuration and must never be consulted to resume existing work.
	changed := strings.Replace(string(raw), "items: [{q: 6, r: 0}", "items: [{q: 14, r: 0}", 1)
	s.Require().NotEqual(string(raw), changed)
	edited, err := sessionworld.Compile([]byte(changed))
	s.Require().NoError(err)
	s.NotEqual(dungeon.Spec.Monsters[0].Behaviors, edited.Spec.Monsters[0].Behaviors)
	s.reloadBehaviorHost(h, die, "held-reload")
	offer := reactRow(ctx, s.T(), h, run, "alice")
	s.Require().NotNil(offer)
	_, err = h.handler.React(ctx, &sessionpb.ReactRequest{Session: run, Member: "alice", DeclarationId: offer.GetId(), Choice: sessionpb.ReactChoice_REACT_CHOICE_HOLD})
	s.Require().NoError(err)
	after := s.storedBehavior(h, run, "guard")
	s.Equal(before.Behaviors.Definition, after.Behaviors.Definition)
	s.Equal("item/1/walk", after.Behaviors.Progress.Instances[0].Blob.State, "arrival advances exactly the selected attachment")
	s.Equal(spatial.Position{X: 6}, whereIs(s.T(), h, run, "guard"))
	data, err = sessionorch.NewEncounterRepository(h.redis, time.Hour).GetEncounter(ctx, run)
	s.Require().NoError(err)
	s.Nil(data.Pause)
	s.Equal(len(die.faces), die.calls, "resume, reaction lookup and reads do not select again")
}
