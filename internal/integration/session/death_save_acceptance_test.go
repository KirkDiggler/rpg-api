package session_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/saves"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

func TestAcceptance_AllStandingAssessmentProjectsEveryMember(t *testing.T) {
	assessment, err := (allStanding{}).Assess([]tkencounter.MemberID{"alice", "bob"})
	require.NoError(t, err)
	require.False(t, assessment.PartyDefeated)
	require.Equal(t, []tkencounter.MemberParticipation{
		{Member: "alice", Contact: true, Conscious: true, Turn: tkencounter.TurnParticipationWait},
		{Member: "bob", Contact: true, Conscious: true, Turn: tkencounter.TurnParticipationWait},
	}, assessment.Members)
}

type scriptedDeathSaveDice struct {
	mu     sync.Mutex
	values []int
	calls  int
}

func (d *scriptedDeathSaveDice) Roll(_ context.Context, size int) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls++
	if len(d.values) == 0 {
		return size, nil
	}
	value := d.values[0]
	d.values = d.values[1:]
	return value, nil
}

func (d *scriptedDeathSaveDice) reset(values ...int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.values = append([]int(nil), values...)
	d.calls = 0
}

func (d *scriptedDeathSaveDice) callCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.calls
}

type deathSaveScene struct {
	h       *acceptanceHarness
	dice    *scriptedDeathSaveDice
	ctx     context.Context
	session string
	actor   string
	ally    string
}

func newDeathSaveScene(t *testing.T) *deathSaveScene {
	t.Helper()
	dice := &scriptedDeathSaveDice{}
	h := newAcceptanceHarnessWithDice(t, dice)
	const sessionID = "death-save-run"

	for _, row := range []struct{ id, player string }{{"alice", "player-alice"}, {"bob", "player-bob"}} {
		_, err := h.charRepo.Create(context.Background(), characterrepo.CreateInput{
			Character: &entities.Character{Data: armedFighter(row.id, row.player)},
		})
		require.NoError(t, err)
	}
	_, err := h.manager.Manager.StartSession(context.Background(), &sdk.StartSessionInput{
		Session: sessionID, Encounter: "death-save-encounter", World: buildThreeRoomTomb(t),
	})
	require.NoError(t, err)
	_, err = h.manager.Manager.Spawn(context.Background(), &sdk.SpawnInput{
		Session: sessionID, ID: "skel-1", Ref: refs.Monsters.Skeleton().String(), Position: at(19, 3),
	})
	require.NoError(t, err)
	for _, row := range []struct {
		id, player string
		col        int
	}{{"alice", "player-alice", 18}, {"bob", "player-bob", 17}} {
		_, err = h.handler.Join(auth.WithPlayerID(context.Background(), row.player), &sessionpb.JoinRequest{
			Session: sessionID, Member: row.id, Position: pbAt(row.col, 3),
		})
		require.NoError(t, err)
	}

	turn, err := h.manager.Manager.Turn(context.Background(), &sdk.TurnInput{Session: sessionID, Member: "alice"})
	require.NoError(t, err)
	require.Equal(t, sdk.ClockTurn, turn.Clock)
	require.Contains(t, []string{"alice", "bob"}, turn.Active, "monster turns are synchronously driven")
	actor, ally := turn.Active, "alice"
	if actor == ally {
		ally = "bob"
	}
	player := "player-" + actor
	return &deathSaveScene{h: h, dice: dice, ctx: auth.WithPlayerID(context.Background(), player), session: sessionID, actor: actor, ally: ally}
}

func (s *deathSaveScene) seed(t *testing.T, member string, hp int, state *saves.DeathSaveState, grant bool) {
	t.Helper()
	got, err := s.h.charRepo.Get(context.Background(), characterrepo.GetInput{ID: member})
	require.NoError(t, err)
	got.Character.Data.HitPoints = hp
	got.Character.Data.DeathSaveState = state
	if got.Character.Data.ActionEconomy == nil {
		got.Character.Data.ActionEconomy = &tkcharacter.ActionEconomyData{}
	}
	if got.Character.Data.ActionEconomy.Granted == nil {
		got.Character.Data.ActionEconomy.Granted = map[tkcharacter.GrantedActionKey]int{}
	}
	if grant {
		got.Character.Data.ActionEconomy.Granted[tkcharacter.GrantedDeathSaves] = 1
	} else {
		delete(got.Character.Data.ActionEconomy.Granted, tkcharacter.GrantedDeathSaves)
	}
	_, err = s.h.charRepo.Update(context.Background(), characterrepo.UpdateInput{Character: got.Character})
	require.NoError(t, err)
}

func (s *deathSaveScene) stored(t *testing.T, member string) *tkcharacter.Data {
	t.Helper()
	got, err := s.h.charRepo.Get(context.Background(), characterrepo.GetInput{ID: member})
	require.NoError(t, err)
	return got.Character.Data
}

func deathSaveDeclaration(t *testing.T, s *deathSaveScene) *sessionpb.Declaration {
	t.Helper()
	afford, err := s.h.handler.Afford(s.ctx, &sessionpb.AffordRequest{Session: s.session, Member: s.actor})
	require.NoError(t, err)
	for _, declaration := range afford.GetDeclarations() {
		if declaration.GetVerb() == sessionpb.Verb_VERB_DEATH_SAVE {
			return declaration
		}
	}
	require.FailNow(t, "Death Save declaration absent")
	return nil
}

func TestAcceptance_DeathSave(t *testing.T) {
	t.Run("ordinary success persists, publishes, spends, and rejects replay", func(t *testing.T) {
		s := newDeathSaveScene(t)
		s.seed(t, s.actor, 0, &saves.DeathSaveState{Successes: 1, Failures: 1}, true)

		turn, err := s.h.handler.Turn(s.ctx, &sessionpb.TurnRequest{Session: s.session, Member: s.actor})
		require.NoError(t, err)
		var actor *sessionpb.Participant
		for _, participant := range turn.GetParticipants() {
			if participant.GetMember() == s.actor {
				actor = participant
			}
		}
		require.NotNil(t, actor)
		require.Equal(t, sessionpb.LifeState_LIFE_STATE_DYING, actor.GetLifeState())
		require.Equal(t, int32(1), actor.GetDeathSaves().GetSuccesses())
		require.Equal(t, int32(1), actor.GetDeathSaves().GetFailures())

		declaration := deathSaveDeclaration(t, s)
		require.NotEmpty(t, declaration.GetId())
		require.Equal(t, sessionpb.Slot_SLOT_NONE, declaration.GetSlot())
		require.Equal(t, sessionpb.TargetKind_TARGET_KIND_NONE, declaration.GetTargetKind())
		require.Equal(t, "Death Saving Throw", declaration.GetDeathSave().GetName())
		s.dice.reset(14)
		resp, err := s.h.handler.DeathSave(s.ctx, &sessionpb.DeathSaveRequest{
			Session: s.session, Member: s.actor, DeclarationId: declaration.GetId(),
		})
		require.NoError(t, err)
		require.Equal(t, int32(14), resp.GetRoll())
		require.Equal(t, sessionpb.DeathSaveOutcome_DEATH_SAVE_OUTCOME_SUCCESS, resp.GetOutcome())
		require.Equal(t, sessionpb.DeathSaveContinuation_DEATH_SAVE_CONTINUATION_END_TURN, resp.GetContinuation())
		require.Equal(t, "presentation_1", resp.GetPresentationId())
		require.NotEqual(t, "1", resp.GetPresentationId(), "opaque token is not the numeric event sequence")
		require.Equal(t, 1, s.dice.callCount())

		stored := s.stored(t, s.actor)
		require.Equal(t, 2, stored.DeathSaveState.Successes)
		require.Zero(t, stored.ActionEconomy.Granted[tkcharacter.GrantedDeathSaves])
		for _, witness := range []struct{ member, player string }{{s.actor, "player-" + s.actor}, {s.ally, "player-" + s.ally}} {
			story, storyErr := s.h.handler.GetStory(auth.WithPlayerID(context.Background(), witness.player), &sessionpb.GetStoryRequest{Session: s.session, Member: witness.member})
			require.NoError(t, storyErr)
			var rolled *sessionpb.DeathSaveRolled
			for _, event := range story.GetEntries() {
				if event.GetKind() == sessionpb.EventKind_EVENT_KIND_DEATH_SAVE_ROLLED {
					rolled = event.GetDeathSaveRolled()
				}
			}
			require.NotNil(t, rolled)
			require.Equal(t, resp.GetPresentationId(), rolled.GetPresentationId())
			require.Equal(t, resp.GetRoll(), rolled.GetRoll())
		}
		beforeReplay := s.dice.callCount()
		_, err = s.h.handler.DeathSave(s.ctx, &sessionpb.DeathSaveRequest{Session: s.session, Member: s.actor, DeclarationId: declaration.GetId()})
		requireGRPCCode(t, err, codes.FailedPrecondition)
		require.Equal(t, beforeReplay, s.dice.callCount())
	})

	for _, tc := range []struct {
		name         string
		roll         int
		state        *saves.DeathSaveState
		outcome      sessionpb.DeathSaveOutcome
		life         sessionpb.LifeState
		continuation sessionpb.DeathSaveContinuation
	}{
		{"natural 20", 20, &saves.DeathSaveState{Successes: 1, Failures: 1}, sessionpb.DeathSaveOutcome_DEATH_SAVE_OUTCOME_RECOVERED, sessionpb.LifeState_LIFE_STATE_CONSCIOUS, sessionpb.DeathSaveContinuation_DEATH_SAVE_CONTINUATION_KEEP_TURN},
		{"third success", 14, &saves.DeathSaveState{Successes: 2}, sessionpb.DeathSaveOutcome_DEATH_SAVE_OUTCOME_STABILIZED, sessionpb.LifeState_LIFE_STATE_STABILIZED, sessionpb.DeathSaveContinuation_DEATH_SAVE_CONTINUATION_END_TURN},
		{"third failure", 5, &saves.DeathSaveState{Failures: 2}, sessionpb.DeathSaveOutcome_DEATH_SAVE_OUTCOME_DEAD, sessionpb.LifeState_LIFE_STATE_DEAD, sessionpb.DeathSaveContinuation_DEATH_SAVE_CONTINUATION_ALREADY_ADVANCED},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newDeathSaveScene(t)
			s.seed(t, s.actor, 0, tc.state, true)
			declaration := deathSaveDeclaration(t, s)
			s.dice.reset(tc.roll)
			resp, err := s.h.handler.DeathSave(s.ctx, &sessionpb.DeathSaveRequest{Session: s.session, Member: s.actor, DeclarationId: declaration.GetId()})
			require.NoError(t, err)
			require.Equal(t, tc.outcome, resp.GetOutcome())
			require.Equal(t, tc.continuation, resp.GetContinuation())
			stored := s.stored(t, s.actor)
			if tc.life == sessionpb.LifeState_LIFE_STATE_CONSCIOUS {
				require.Equal(t, 1, stored.HitPoints)
				require.Equal(t, &saves.DeathSaveState{}, stored.DeathSaveState)
			} else {
				require.Equal(t, tc.life == sessionpb.LifeState_LIFE_STATE_DEAD, stored.DeathSaveState.Dead)
				require.Equal(t, tc.life == sessionpb.LifeState_LIFE_STATE_STABILIZED, stored.DeathSaveState.Stabilized)
			}
		})
	}
}

func TestAcceptance_DeathSaveAttackTransitionsAndTargetability(t *testing.T) {
	tests := []struct {
		name           string
		state          *saves.DeathSaveState
		rolls          []int
		weak           bool
		wantFailures   int
		wantStabilized bool
	}{
		{"normal damage adds one failure", &saves.DeathSaveState{}, []int{19, 4}, false, 1, false},
		{"critical damage unstabilizes and adds two", &saves.DeathSaveState{Successes: 3, Stabilized: true}, []int{20, 4, 4}, false, 2, false},
		{"zero applied damage adds no failure", &saves.DeathSaveState{Successes: 3, Stabilized: true}, []int{19, 1}, true, 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newDeathSaveScene(t)
			target := s.ally
			s.seed(t, target, 0, tc.state, false)
			if tc.weak {
				data := s.stored(t, s.actor)
				data.AbilityScores[abilities.STR] = 1
				got, err := s.h.charRepo.Get(context.Background(), characterrepo.GetInput{ID: s.actor})
				require.NoError(t, err)
				got.Character.Data = data
				_, err = s.h.charRepo.Update(context.Background(), characterrepo.UpdateInput{Character: got.Character})
				require.NoError(t, err)
			}
			afford, err := s.h.handler.Afford(s.ctx, &sessionpb.AffordRequest{Session: s.session, Member: s.actor})
			require.NoError(t, err)
			var attack *sessionpb.Declaration
			for _, declaration := range afford.GetDeclarations() {
				if declaration.GetVerb() == sessionpb.Verb_VERB_ATTACK {
					attack = declaration
					break
				}
			}
			require.NotNil(t, attack)
			var targetListed bool
			for _, candidate := range attack.GetCandidates() {
				if candidate.GetMember() == target {
					targetListed = true
				}
			}
			require.True(t, targetListed, "Dying and Stabilized remain provider-authored targets")
			s.dice.reset(tc.rolls...)
			_, err = s.h.handler.Attack(s.ctx, &sessionpb.AttackRequest{Session: s.session, Attacker: s.actor, Target: target, DeclarationId: attack.GetId()})
			require.NoError(t, err)
			stored := s.stored(t, target)
			require.Equal(t, tc.wantFailures, stored.DeathSaveState.Failures)
			require.Equal(t, tc.wantStabilized, stored.DeathSaveState.Stabilized)
		})
	}

	t.Run("dead target is stale before resolution", func(t *testing.T) {
		s := newDeathSaveScene(t)
		s.seed(t, s.ally, 0, &saves.DeathSaveState{Failures: 3, Dead: true}, false)
		afford, err := s.h.handler.Afford(s.ctx, &sessionpb.AffordRequest{Session: s.session, Member: s.actor})
		require.NoError(t, err)
		var attack *sessionpb.Declaration
		for _, declaration := range afford.GetDeclarations() {
			if declaration.GetVerb() == sessionpb.Verb_VERB_ATTACK {
				attack = declaration
				break
			}
		}
		require.NotNil(t, attack)
		for _, candidate := range attack.GetCandidates() {
			require.NotEqual(t, s.ally, candidate.GetMember())
		}
		s.dice.reset(20, 8, 8)
		_, err = s.h.handler.Attack(s.ctx, &sessionpb.AttackRequest{Session: s.session, Attacker: s.actor, Target: s.ally, DeclarationId: attack.GetId()})
		requireGRPCCode(t, err, codes.FailedPrecondition)
		require.Zero(t, s.dice.callCount(), "stale target refuses before resolution or dice")
		require.Equal(t, 3, s.stored(t, s.ally).DeathSaveState.Failures)
	})
}

func TestAcceptance_DeathSaveNoConsciousPartyDefeat(t *testing.T) {
	s := newDeathSaveScene(t)
	s.seed(t, s.actor, 0, &saves.DeathSaveState{}, true)
	s.seed(t, s.ally, 0, &saves.DeathSaveState{}, false)
	endID := currentDeclarationID(s.ctx, t, s.h.handler, s.session, s.actor, sessionpb.Verb_VERB_END_TURN)
	_, err := s.h.handler.EndTurn(s.ctx, &sessionpb.EndTurnRequest{
		Session: s.session, Member: s.actor, DeclarationId: endID,
	})
	require.NoError(t, err)
	statusResp, err := s.h.handler.GetStatus(s.ctx, &sessionpb.GetStatusRequest{Session: s.session})
	require.NoError(t, err)
	require.False(t, statusResp.GetOpen())
	require.Equal(t, "party_defeated", statusResp.GetOutcome().GetEnding())
}
