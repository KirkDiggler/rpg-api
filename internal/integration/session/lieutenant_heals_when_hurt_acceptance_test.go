package session_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	storypb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
)

// A CREATURE ACTS ON ITS OWN HIT POINTS (rpg-project#1131, slice 4d): a site
// whose Lieutenant's table asks `hit_points: { at_most: 30% }` before it
// activates Second Wind is launched from its YAML file and played over the
// wire. Unhurt (27 of 27) he does not use it; hurt to 8 or fewer he does,
// once, and the fighter's client is sent it the way it is sent a player's own.

// scriptedDice picks a table's first eligible row (the d200), lets every d20
// roll `d20` and every d8 roll `d8`, and gives every other die its maximum.
type scriptedDice struct{ d20, d8 int }

func (d scriptedDice) Roll(_ context.Context, size int) (int, error) {
	switch {
	case size >= tablePickSides:
		return 1, nil
	case size == 20:
		return d.d20, nil
	case size == 8:
		return d.d8, nil
	}
	return size, nil
}

// hitPointsRun launches the fixture with the fighter and returns the harness,
// the fighter's context and the run id's story.
func hitPointsRun(t *testing.T, run string, dice scriptedDice) (*acceptanceHarness, context.Context) {
	t.Helper()
	h := newAcceptanceHarnessWithDice(t, dice)
	ctx := auth.WithPlayerID(context.Background(), "player-alice")

	_, err := h.charRepo.Create(context.Background(), characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter("alice", "player-alice")},
	})
	require.NoError(t, err)

	raw, err := os.ReadFile(filepath.Join(dungeonstest.ContentDir(t), "..", "internal", "dungeons", "testdata", "hit-points-when", "site.yaml"))
	require.NoError(t, err)
	dungeon, err := sessionworld.Compile(raw)
	require.NoError(t, err)
	h.launchAtStart(t, run, dungeon.Spec, "alice")
	inCombat(t, h.charRepo, "alice", 1)

	turn, err := h.handler.Turn(ctx, &sessionpb.TurnRequest{Session: run, Member: "alice"})
	require.NoError(t, err)
	require.Equal(t, "alice", turn.GetActive(), "geometry gate: the fighter acts first")
	return h, ctx
}

// fighterEndsTurn ends the fighter's turn and returns the acts her client is
// sent while the Lieutenant takes his.
func fighterEndsTurn(ctx context.Context, t *testing.T, h *acceptanceHarness, run string) []*storypb.Act {
	t.Helper()
	return watchActs(ctx, t, h, run, "alice", func() {
		_, err := h.handler.EndTurn(ctx, &sessionpb.EndTurnRequest{
			Session: run, Member: "alice",
			DeclarationId: currentDeclarationID(ctx, t, h.handler, run, "alice", sessionpb.Verb_VERB_END_TURN),
		})
		require.NoError(t, err)
	})
}

func activationsBy(acts []*storypb.Act, by string) []*storypb.Act {
	var out []*storypb.Act
	for _, a := range acts {
		if a.GetActivate() != nil && a.GetActivate().GetBy().GetId() == by {
			out = append(out, a)
		}
	}
	return out
}

func TestAcceptance_AnUnhurtLieutenantDoesNotUseSecondWind(t *testing.T) {
	const run = "lieutenant-unhurt-run"
	// every d20 rolls 1: nothing lands, so nobody is hurt in three rounds.
	h, ctx := hitPointsRun(t, run, scriptedDice{d20: 1, d8: 8})

	acted := false
	for round := 1; round <= 3; round++ {
		live := fighterEndsTurn(ctx, t, h, run)
		for _, a := range live {
			t.Logf("round %d act %s body %T", round, a.GetId(), a.GetBody())
			if a.GetAttack() != nil || a.GetWalk() != nil {
				acted = true
			}
		}
		require.Emptyf(t, activationsOf(live), "round %d: unhurt (27 of 27), the fighter's client is sent no activate act", round)
	}
	require.True(t, acted, "he took his turns: at least one strike or step reached her")
	require.Equal(t, 27, storedMonsterHitPoints(t, h, run, "lieutenant"), "he stood at his maximum throughout")
}

func activationsOf(acts []*storypb.Act) []*storypb.Act {
	var out []*storypb.Act
	for _, a := range acts {
		if a.GetActivate() != nil {
			out = append(out, a)
		}
	}
	return out
}

// storedMonsterHitPoints reads a monster's stored sheet; it only reads.
func storedMonsterHitPoints(t *testing.T, h *acceptanceHarness, run, id string) int {
	t.Helper()
	stored, err := sessionorch.NewSessionRepository(h.redis, time.Hour).GetSession(context.Background(), run)
	require.NoError(t, err)
	for _, npc := range stored.NPCs {
		if npc.ID == id {
			return npc.HitPoints
		}
	}
	require.FailNowf(t, "no such monster", "no stored sheet for %q", id)
	return 0
}

func TestAcceptance_AHurtLieutenantUsesSecondWindOnceAndTheWireCarriesIt(t *testing.T) {
	const run = "lieutenant-hurt-run"
	// every d20 rolls 10 (a blow lands on either side); every d8 rolls 7, so the
	// fighter's longsword takes 10 off him (7 and her 3) and his 27 stand at 17
	// and then 7: at or below 30% of his maximum (8) and above 5.
	h, ctx := hitPointsRun(t, run, scriptedDice{d20: 10, d8: 7})

	attack := func() {
		_, err := h.handler.Attack(ctx, &sessionpb.AttackRequest{
			Session: run, Attacker: "alice", Target: "lieutenant",
			DeclarationId: currentDeclarationID(ctx, t, h.handler, run, "alice", sessionpb.Verb_VERB_ATTACK),
		})
		require.NoError(t, err)
	}

	// Round 1: she waits; he walks up to her. Rounds 2 and 3: she strikes, he answers.
	require.Empty(t, activationsOf(fighterEndsTurn(ctx, t, h, run)), "unhurt, he does not use Second Wind")
	attack()
	require.Equal(t, 17, storedMonsterHitPoints(t, h, run, "lieutenant"))
	require.Empty(t, activationsOf(fighterEndsTurn(ctx, t, h, run)), "at 17 of 27 he is not hurt enough")
	attack()
	require.Equal(t, 7, storedMonsterHitPoints(t, h, run, "lieutenant"), "7 of 27: at most 30%% (8), above 5")

	live := fighterEndsTurn(ctx, t, h, run)
	activations := activationsOf(live)
	require.Len(t, activations, 1, "hurt, he used Second Wind once")
	act := activations[0]
	activate := act.GetActivate()
	require.Equal(t, storypb.Family_FAMILY_ACT, act.GetFamily())
	require.Equal(t, "lieutenant", activate.GetBy().GetId())
	require.Equal(t, "Second Wind", activate.GetAbility().GetName())
	require.Nil(t, activate.GetTarget())
	require.Len(t, activate.GetResults(), 1)
	healing := activate.GetResults()[0]
	require.Equal(t, "healing-applied", healing.GetKind())
	require.Equal(t, "lieutenant", healing.GetTarget().GetId())
	// the d10 rolls its maximum and the level adds 1: 11 of the 11 requested lands.
	require.Equal(t, int32(11), healing.GetRequested())
	require.Equal(t, int32(7), healing.GetBefore())
	require.Equal(t, int32(18), healing.GetAfter())
	require.Equal(t, int32(11), healing.GetAmount())
	t.Logf("wire: Act{family %s, activate{by %q, ability{ref %q, name %q}, results[{kind %q, target %q, amount %d, requested %d, before %d, after %d}]}}",
		act.GetFamily(), activate.GetBy().GetId(), activate.GetAbility().GetRef(), activate.GetAbility().GetName(),
		healing.GetKind(), healing.GetTarget().GetId(), healing.GetAmount(), healing.GetRequested(), healing.GetBefore(), healing.GetAfter())
	requireLiveIsStory(ctx, t, h, run, "alice", live)

	// His one use is spent: on the turn after, however hurt he is, no activation.
	require.Empty(t, activationsOf(fighterEndsTurn(ctx, t, h, run)), "the use is spent; the row is not on his table")
}
