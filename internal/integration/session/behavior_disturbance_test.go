package session_test

import (
	"context"
	"os"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

func (s *BehaviorPatrolSuite) TestActualClangInterruptsOnlyTheQualifiedRouteAndReloadResumesIt() {
	ctx := auth.WithPlayerID(context.Background(), "player-alice")
	const run = "canonical-disturbance-run"
	die := &behaviorScript{faces: []behaviorFace{
		{100, 1}, {100, 1}, // first world opportunity
		{20, 15}, {8, 1}, // actual attack and damage, not a fabricated heard event
		{100, 1}, {100, 1}, {100, 1}, {100, 1}, {100, 1}, {100, 1}, {100, 1}, {100, 1},
	}}
	h := newAcceptanceHarnessAnswering(s.T(), die, nil, false)
	_, err := h.charRepo.Create(ctx, characterrepo.CreateInput{Character: &entities.Character{Data: armedFighter("alice", "player-alice")}})
	s.Require().NoError(err)
	raw, err := os.ReadFile("testdata/behavior-disturbance.yaml")
	s.Require().NoError(err)
	dungeon, err := sessionworld.Compile(raw)
	s.Require().NoError(err)
	launched := h.launchAtStart(s.T(), run, dungeon.Spec, "alice")
	s.Require().Empty(launched.Formed)
	forward := []*sessionpb.Position{{X: 1, Y: 2}, {X: 2, Y: 2}, {X: 3, Y: 2}, {X: 4, Y: 2}, {X: 5, Y: 2}, {X: 6, Y: 2}}
	backward := []*sessionpb.Position{{X: 5, Y: 2}, {X: 4, Y: 2}, {X: 3, Y: 2}, {X: 2, Y: 2}, {X: 1, Y: 2}, {X: 0, Y: 2}}
	moved, err := h.handler.Move(ctx, &sessionpb.MoveRequest{Session: run, Member: "alice", Path: forward})
	s.Require().NoError(err)
	s.awaitBehaviorMove(h, run, moved, forward[len(forward)-1])
	s.Equal(spatial.Position{X: 8}, whereIs(s.T(), h, run, "west"))
	before := s.storedBehavior(h, run, "west")
	pending := before.Behaviors.Progress.Instances[1]
	s.Equal("item/0/walk", pending.Blob.State, "the long first leg has not arrived")
	s.reloadBehaviorHost(h, die, "disturbance-attack")
	attack, err := h.handler.Attack(ctx, &sessionpb.AttackRequest{Session: run, Attacker: "alice", Target: "dummy", DeclarationId: currentDeclarationID(ctx, s.T(), h.handler, run, "alice", sessionpb.Verb_VERB_ATTACK)})
	s.Require().NoError(err)
	s.True(attack.GetHit())
	s.GreaterOrEqual(attack.GetDamage(), int32(1))
	for round, path := range [][]*sessionpb.Position{nil, backward, forward, backward} {
		s.reloadBehaviorHost(h, die, "disturbance-read")
		if path != nil {
			moved, err = h.handler.Move(ctx, &sessionpb.MoveRequest{Session: run, Member: "alice", Path: path})
			s.Require().NoError(err)
			s.awaitBehaviorMove(h, run, moved, path[len(path)-1])
		}
		west := s.storedBehavior(h, run, "west")
		court := s.storedBehavior(h, run, "court")
		s.Equal(before.Behaviors.Definition, west.Behaviors.Definition)
		s.Equal("idle", court.Behaviors.Progress.Instances[0].Blob.State, "far guard's own scope does not qualify")
		switch round {
		case 0, 1:
			s.Equal(pending, west.Behaviors.Progress.Instances[1], "investigation leaves the original duty continuation untouched")
			s.Equal(spatial.Position{X: 2, Y: 2}, whereIs(s.T(), h, run, "west"))
		case 2:
			s.Equal(pending, west.Behaviors.Progress.Instances[1])
			s.Equal(8.0, west.Cell.X, "resumes toward pending fourteen, not nearer old waypoint two")
		case 3:
			s.Equal("item/1/walk", west.Behaviors.Progress.Instances[1].Blob.State)
			s.Equal(spatial.Position{X: 14}, whereIs(s.T(), h, run, "west"))
			s.Equal(spatial.Position{X: 16}, whereIs(s.T(), h, run, "court"))
		}
	}
	s.Equal(len(die.faces), die.calls, "ten exact d100 choices plus the real attack's two dice")
}
