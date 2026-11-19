package session_test

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"google.golang.org/protobuf/encoding/protojson"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	recordpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	recordhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha2"
	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/sessionaccess"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
)

// TestASecondRolesStateReachesTheRecordAndNoStreamBreaks is the ordering
// proof for encounter v0.225.0 under session v0.197.0 (rpg-project#1001): a
// creature's role beside its body writes a `role_state` beat the session
// does not know yet. In the disturbance hall the alarm (alice's attack, a
// clang) moves west's `investigate` role from idle to active, and court's
// `duty` patrol walks its route. The run carries on without an error, a
// player's story and view stay as they were (no role reaches a player), and
// the record a watch reads carries every role_state whole, and every
// `sighted` beat names its observer.
func (s *BehaviorPatrolSuite) TestASecondRolesStateReachesTheRecordAndNoStreamBreaks() {
	ctx := auth.WithPlayerID(context.Background(), "player-alice")
	const run = "role-state-run"
	die := &behaviorScript{faces: []behaviorFace{
		{100, 1}, {100, 1}, // first world opportunity
		{20, 15}, {8, 1}, // the attack and its damage: the clang
		{100, 1}, {100, 1}, {100, 1}, {100, 1}, {100, 1}, {100, 1}, {100, 1}, {100, 1},
	}}
	h := newAcceptanceHarnessAnswering(s.T(), die, nil, false)
	_, err := h.charRepo.Create(ctx, characterrepo.CreateInput{Character: &entities.Character{Data: armedFighter("alice", "player-alice")}})
	s.Require().NoError(err)
	raw, err := os.ReadFile("testdata/behavior-disturbance.yaml")
	s.Require().NoError(err)
	dungeon, err := sessionworld.Compile(raw)
	s.Require().NoError(err)
	h.launchAtStart(s.T(), run, dungeon.Spec, "alice")

	forward := []*sessionpb.Position{{X: 1, Y: 2}, {X: 2, Y: 2}, {X: 3, Y: 2}, {X: 4, Y: 2}, {X: 5, Y: 2}, {X: 6, Y: 2}}
	backward := []*sessionpb.Position{{X: 5, Y: 2}, {X: 4, Y: 2}, {X: 3, Y: 2}, {X: 2, Y: 2}, {X: 1, Y: 2}, {X: 0, Y: 2}}
	moved, err := h.handler.Move(ctx, &sessionpb.MoveRequest{Session: run, Member: "alice", Path: forward})
	s.Require().NoError(err)
	s.awaitBehaviorMove(h, run, moved, forward[len(forward)-1])
	attack, err := h.handler.Attack(ctx, &sessionpb.AttackRequest{Session: run, Attacker: "alice", Target: "dummy",
		DeclarationId: currentDeclarationID(ctx, s.T(), h.handler, run, "alice", sessionpb.Verb_VERB_ATTACK)})
	s.Require().NoError(err)
	s.True(attack.GetHit())
	moved, err = h.handler.Move(ctx, &sessionpb.MoveRequest{Session: run, Member: "alice", Path: backward})
	s.Require().NoError(err, "play carries on after a role_state the session does not know")
	s.awaitBehaviorMove(h, run, moved, backward[len(backward)-1])

	drain, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s.Require().NoError(h.manager.Drain(drain))

	// A player: the story and the view read without error, and no role.
	acts := storyOf(ctx, s.T(), h.story, run, "alice")
	s.NotEmpty(acts)
	for _, act := range acts {
		s.NotContains(protojson.Format(act), encounter.BeatRoleState, "no role reaches a player")
	}
	view, err := h.handler.GetView(ctx, &sessionpb.GetViewRequest{Session: run, Member: "alice"})
	s.Require().NoError(err)
	s.NotContains(protojson.Format(view), encounter.BeatRoleState)

	// The record a watch reads: every role_state and sighted beat, whole.
	access, err := sessionaccess.New(h.charRepo, h.manager.Manager)
	s.Require().NoError(err)
	reader, err := recordhandler.New(&recordhandler.HandlerConfig{Manager: h.manager.Manager, Broker: h.manager.RecordBroker, Gate: access, Enabled: true})
	s.Require().NoError(err)
	page, err := reader.GetRecord(ctx, &recordpb.GetRecordRequest{Session: run, Member: "alice", Limit: 500})
	s.Require().NoError(err)
	s.False(page.GetMore())

	type roleState struct{ Member, Role, Recipe, From, To string }
	var roles []roleState
	observers := map[string]bool{}
	for _, e := range page.GetEvents() {
		switch e.GetBeat() {
		case encounter.BeatRoleState:
			var rs roleState
			s.Require().NoError(json.Unmarshal(e.GetFieldsJson(), &rs))
			s.Equal(rs.Member, e.GetFields().GetFields()["member"].GetStringValue(), "the struct form agrees with the bytes")
			roles = append(roles, rs)
		case encounter.BeatSighted:
			var seen struct{ Observer string }
			s.Require().NoError(json.Unmarshal(e.GetFieldsJson(), &seen))
			s.NotEmpty(seen.Observer, "every sighted beat names who saw")
			observers[seen.Observer] = true
		}
	}
	s.Contains(roles, roleState{Member: "west", Role: "investigate", Recipe: "example:behaviors:investigate", From: "idle", To: "active"},
		"the alarm moves west's second role, and the record carries it whole")
	s.Contains(roles, roleState{Member: "court", Role: "duty", Recipe: "example:behaviors:patrol", From: "item/0/walk", To: "item/1/walk"})
	s.True(observers["west"], "the guard's own sighting names it")
	s.True(observers["alice"])
}
