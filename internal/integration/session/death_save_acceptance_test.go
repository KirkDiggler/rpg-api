package session_test

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/combat"
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

func attackDeclaration(t *testing.T, s *deathSaveScene) *sessionpb.Declaration {
	t.Helper()
	afford, err := s.h.handler.Afford(s.ctx, &sessionpb.AffordRequest{Session: s.session, Member: s.actor})
	require.NoError(t, err)
	for _, declaration := range afford.GetDeclarations() {
		if declaration.GetVerb() == sessionpb.Verb_VERB_ATTACK {
			return declaration
		}
	}
	require.FailNow(t, "Attack declaration absent")
	return nil
}

func attackCandidatePresent(declaration *sessionpb.Declaration, member string) bool {
	for _, candidate := range declaration.GetCandidates() {
		if candidate.GetMember() == member {
			return true
		}
	}
	return false
}

func attackCandidateAvailable(declaration *sessionpb.Declaration, member string) bool {
	for _, candidate := range declaration.GetCandidates() {
		if candidate.GetMember() == member {
			return candidate.GetAvailable()
		}
	}
	return false
}

func participantByMember(t *testing.T, turn *sessionpb.TurnResponse, member string) *sessionpb.Participant {
	t.Helper()
	for _, participant := range turn.GetParticipants() {
		if participant.GetMember() == member {
			return participant
		}
	}
	require.FailNow(t, "participant absent", member)
	return nil
}

func assertPublicDeathSaveState(
	t *testing.T,
	participant *sessionpb.Participant,
	life sessionpb.LifeState,
	progress *sessionpb.DeathSaveProgress,
) {
	t.Helper()
	require.Equal(t, life, participant.GetLifeState())
	require.True(t, proto.Equal(progress, participant.GetDeathSaves()),
		"progress mismatch: want %v, got %v", progress, participant.GetDeathSaves())
}

type deathSaveWitness struct {
	member   string
	ctx      context.Context
	stream   *recordingStream
	baseline int
}

func subscribeDeathSaveWitness(t *testing.T, s *deathSaveScene, member string) *deathSaveWitness {
	t.Helper()
	ctx, cancel := context.WithCancel(auth.WithPlayerID(context.Background(), "player-"+member))
	stream := newRecordingStream(ctx)
	done := make(chan error, 1)
	go func() {
		done <- s.h.handler.StreamEvents(&sessionpb.StreamEventsRequest{Session: s.session, Member: member}, stream)
	}()
	waitForLive(t, s.h.manager.Broker, s.session, member, stream)
	witness := &deathSaveWitness{member: member, ctx: ctx, stream: stream, baseline: len(stream.snapshot())}
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(time.Second):
			t.Errorf("%s StreamEvents did not stop after cancellation", member)
		}
	})
	return witness
}

func (w *deathSaveWitness) deathSaveEvent(t *testing.T) *sessionpb.Event {
	t.Helper()
	events := waitForQuiescence(t, w.stream, 2*time.Second)[w.baseline:]
	return singleDeathSaveEvent(t, events)
}

func singleDeathSaveEvent(t *testing.T, events []*sessionpb.Event) *sessionpb.Event {
	t.Helper()
	var found []*sessionpb.Event
	for _, event := range events {
		if event.GetKind() == sessionpb.EventKind_EVENT_KIND_DEATH_SAVE_ROLLED {
			found = append(found, event)
		}
	}
	require.Len(t, found, 1, "expected exactly one Death Save event")
	return found[0]
}

func assertOrdinarySuccessResponse(t *testing.T, s *deathSaveScene, response *sessionpb.DeathSaveResponse) {
	t.Helper()
	require.Equal(t, int32(14), response.GetRoll())
	require.Equal(t, sessionpb.DeathSaveOutcome_DEATH_SAVE_OUTCOME_SUCCESS, response.GetOutcome())
	require.Equal(t, int32(1), response.GetSuccessesAdded())
	require.Zero(t, response.GetFailuresAdded())
	require.Equal(t, int32(2), response.GetSuccesses())
	require.Equal(t, int32(1), response.GetFailures())
	require.Equal(t, int32(1), response.GetSuccessesNeeded())
	require.Equal(t, int32(2), response.GetFailuresRemaining())
	require.False(t, response.GetStabilized())
	require.False(t, response.GetDead())
	require.False(t, response.GetRecovered())
	require.Zero(t, response.GetHpRestored())
	require.Equal(t, sessionpb.DeathSaveContinuation_DEATH_SAVE_CONTINUATION_END_TURN, response.GetContinuation())
	require.Equal(t, "presentation_1", response.GetPresentationId())
	require.Positive(t, response.GetSeq())
	require.NotEqual(t, strconv.FormatUint(response.GetSeq(), 10), response.GetPresentationId(),
		"opaque token remains separate from numeric authority")
	require.Equal(t, []string{"character:" + s.actor, "encounter:death-save-encounter", "session:" + s.session}, response.GetSaved().GetWritten())
	require.Empty(t, response.GetSaved().GetFailed())
	require.Equal(t, int32(6), response.GetDelivery().GetEvents(), "Death Save plus newly-observed Down are delivered to all three roster members")
	require.False(t, response.GetDelivery().GetFailed())
}

func assertDeathSaveEventMatchesResponse(
	t *testing.T,
	actor string,
	event *sessionpb.DeathSaveRolled,
	response *sessionpb.DeathSaveResponse,
) {
	t.Helper()
	require.NotNil(t, event)
	require.Equal(t, actor, event.GetActor())
	require.Equal(t, response.GetRoll(), event.GetRoll())
	require.Equal(t, response.GetOutcome(), event.GetOutcome())
	require.Equal(t, response.GetSuccessesAdded(), event.GetSuccessesAdded())
	require.Equal(t, response.GetFailuresAdded(), event.GetFailuresAdded())
	require.Equal(t, response.GetSuccesses(), event.GetSuccesses())
	require.Equal(t, response.GetFailures(), event.GetFailures())
	require.Equal(t, response.GetSuccessesNeeded(), event.GetSuccessesNeeded())
	require.Equal(t, response.GetFailuresRemaining(), event.GetFailuresRemaining())
	require.Equal(t, response.GetStabilized(), event.GetStabilized())
	require.Equal(t, response.GetDead(), event.GetDead())
	require.Equal(t, response.GetRecovered(), event.GetRecovered())
	require.Equal(t, response.GetHpRestored(), event.GetHpRestored())
	require.Equal(t, response.GetContinuation(), event.GetContinuation())
	require.Equal(t, response.GetPresentationId(), event.GetPresentationId())
}

type deathSaveStateSnapshot struct {
	HitPoints         int
	ProgressPresent   bool
	Progress          saves.DeathSaveState
	DeathSaveCapacity int
	LifeState         combat.LifeState
}

func deathSaveStateSnapshotOf(t *testing.T, s *deathSaveScene, member string) deathSaveStateSnapshot {
	t.Helper()
	data := s.stored(t, member)
	snapshot := deathSaveStateSnapshot{HitPoints: data.HitPoints}
	if data.DeathSaveState != nil {
		snapshot.ProgressPresent = true
		snapshot.Progress = *data.DeathSaveState
	}
	if data.ActionEconomy != nil {
		snapshot.DeathSaveCapacity = data.ActionEconomy.Granted[tkcharacter.GrantedDeathSaves]
	}
	loaded, err := tkcharacter.Load(context.Background(), data)
	require.NoError(t, err)
	snapshot.LifeState = loaded.LifeState()
	return snapshot
}

func TestAcceptance_DeathSave(t *testing.T) {
	t.Run("ordinary success persists, publishes, spends, and rejects replay", func(t *testing.T) {
		s := newDeathSaveScene(t)
		actorLive := subscribeDeathSaveWitness(t, s, s.actor)
		allyLive := subscribeDeathSaveWitness(t, s, s.ally)
		s.seed(t, s.actor, 0, &saves.DeathSaveState{Successes: 1, Failures: 1}, true)

		turn, err := s.h.handler.Turn(s.ctx, &sessionpb.TurnRequest{Session: s.session, Member: s.actor})
		require.NoError(t, err)
		require.Contains(t, turn.GetOrder(), s.actor, "Dying actor retains the actual initiative order entry")
		actor := participantByMember(t, turn, s.actor)
		assertPublicDeathSaveState(t, actor, sessionpb.LifeState_LIFE_STATE_DYING, &sessionpb.DeathSaveProgress{
			Successes: 1, Failures: 1, SuccessesNeeded: 2, FailuresRemaining: 2,
			Stabilized: false, Dead: false,
		})

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
		assertOrdinarySuccessResponse(t, s, resp)
		require.Equal(t, 1, s.dice.callCount())

		stored := s.stored(t, s.actor)
		require.Zero(t, stored.HitPoints)
		require.Equal(t, &saves.DeathSaveState{Successes: 2, Failures: 1}, stored.DeathSaveState)
		require.Zero(t, stored.ActionEconomy.Granted[tkcharacter.GrantedDeathSaves])
		afterTurn, turnErr := s.h.handler.Turn(s.ctx, &sessionpb.TurnRequest{Session: s.session, Member: s.actor})
		require.NoError(t, turnErr)
		require.Contains(t, afterTurn.GetOrder(), s.actor)
		assertPublicDeathSaveState(t, participantByMember(t, afterTurn, s.actor), sessionpb.LifeState_LIFE_STATE_DYING, &sessionpb.DeathSaveProgress{
			Successes: 2, Failures: 1, SuccessesNeeded: 1, FailuresRemaining: 2,
			Stabilized: false, Dead: false,
		})

		for _, witness := range []*deathSaveWitness{actorLive, allyLive} {
			liveEvent := witness.deathSaveEvent(t)
			require.Equal(t, witness.member, liveEvent.GetRecipient())
			require.Equal(t, s.session, liveEvent.GetSession())
			assertDeathSaveEventMatchesResponse(t, s.actor, liveEvent.GetDeathSaveRolled(), resp)

			story, storyErr := s.h.handler.GetStory(witness.ctx, &sessionpb.GetStoryRequest{Session: s.session, Member: witness.member})
			require.NoError(t, storyErr)
			storyEvent := singleDeathSaveEvent(t, story.GetEntries())
			require.Equal(t, witness.member, storyEvent.GetRecipient())
			assertDeathSaveEventMatchesResponse(t, s.actor, storyEvent.GetDeathSaveRolled(), resp)
		}
		require.Equal(t, actorLive.deathSaveEvent(t).GetSeq(), resp.GetSeq(), "response sequence is the actor recipient's delivered event sequence")

		beforeReplay := s.dice.callCount()
		beforeState := deathSaveStateSnapshotOf(t, s, s.actor)
		_, err = s.h.handler.DeathSave(s.ctx, &sessionpb.DeathSaveRequest{Session: s.session, Member: s.actor, DeclarationId: declaration.GetId()})
		requireGRPCCode(t, err, codes.FailedPrecondition)
		require.Equal(t, beforeReplay, s.dice.callCount())
		require.Equal(t, beforeState, deathSaveStateSnapshotOf(t, s, s.actor))
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
			before := deathSaveStateSnapshotOf(t, s, target)
			s.dice.reset(tc.rolls...)
			_, err = s.h.handler.Attack(s.ctx, &sessionpb.AttackRequest{Session: s.session, Attacker: s.actor, Target: target, DeclarationId: attack.GetId()})
			require.NoError(t, err)
			after := deathSaveStateSnapshotOf(t, s, target)
			if tc.weak {
				require.Equal(t, before, after, "zero applied damage preserves the complete target life/death-save state")
			} else {
				require.Equal(t, tc.wantFailures, after.Progress.Failures)
				require.Equal(t, tc.wantStabilized, after.Progress.Stabilized)
			}
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
		before := deathSaveStateSnapshotOf(t, s, s.ally)
		s.dice.reset(20, 8, 8)
		_, err = s.h.handler.Attack(s.ctx, &sessionpb.AttackRequest{Session: s.session, Attacker: s.actor, Target: s.ally, DeclarationId: attack.GetId()})
		requireGRPCCode(t, err, codes.FailedPrecondition)
		require.Zero(t, s.dice.callCount(), "stale target refuses before resolution or dice")
		require.Equal(t, before, deathSaveStateSnapshotOf(t, s, s.ally), "Dead target refusal preserves the complete target life/death-save state")
	})

	t.Run("defeated monster is excluded while another hostile keeps the fight active", func(t *testing.T) {
		s := newDeathSaveScene(t)
		_, err := s.h.manager.Manager.Spawn(context.Background(), &sdk.SpawnInput{
			Session: s.session, ID: "skel-2", Ref: refs.Monsters.Skeleton().String(), Position: at(20, 3),
		})
		require.NoError(t, err)
		attack := attackDeclaration(t, s)
		require.True(t, attackCandidateAvailable(attack, "skel-1"))
		require.True(t, attackCandidatePresent(attack, "skel-2"))

		s.dice.reset(20, 8, 8)
		_, err = s.h.handler.Attack(s.ctx, &sessionpb.AttackRequest{
			Session: s.session, Attacker: s.actor, Target: "skel-1", DeclarationId: attack.GetId(),
		})
		require.NoError(t, err)

		after := attackDeclaration(t, s)
		require.False(t, attackCandidatePresent(after, "skel-1"), "defeated monster is omitted from the provider candidate universe")
		require.True(t, attackCandidatePresent(after, "skel-2"), "live hostile proves the fight and Attack declaration remain")
		s.dice.reset(20, 8, 8)
		_, err = s.h.handler.Attack(s.ctx, &sessionpb.AttackRequest{
			Session: s.session, Attacker: s.actor, Target: "skel-1", DeclarationId: attack.GetId(),
		})
		requireGRPCCode(t, err, codes.FailedPrecondition)
		require.Zero(t, s.dice.callCount(), "defeated target is stale before a second resolution")
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
