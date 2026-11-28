package session_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	storypb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
)

// A CREATURE RAGES (rpg-project#1131, slice 8f): a site whose Lieutenant lets
// him use Rage for 2 more damage, and whose table's first row activates it, is
// launched from its YAML file and played over the wire. On his first turn he
// rages: the fighter's client is sent the activation the way it is sent a
// player's own, with a result that the raging status landed on him. His
// longsword then hits for the 2 more, and on his next turn, with a use left, he
// is not asked to rage again while he rages. The fighter's slashing blow on him
// is halved.

const ragingRef = "dnd5e:conditions:raging"

// ragingComponent is the damage part a strike carries from the raging status.
func ragingComponent(t *testing.T, strike *storypb.Act) *storypb.DamageComponent {
	t.Helper()
	var found *storypb.DamageComponent
	for _, c := range strike.GetAttack().GetDamageComponents() {
		if c.GetSourceRef() == ragingRef {
			require.Nil(t, found, "one raging part per strike")
			found = c
		}
	}
	return found
}

func TestAcceptance_ALieutenantRagesOnceAndHitsHarderAndTheWireCarriesIt(t *testing.T) {
	const run = "lieutenant-rages-run"
	// the pick lands on the first eligible row (the rage, while it is on his
	// table); every d20 rolls 10, so a blow lands on either side; every d8 rolls
	// 7, so a longsword does 7 plus its wielder's Strength modifier.
	h := newAcceptanceHarnessWithDice(t, scriptedDice{d20: 10, d8: 7})
	ctx := auth.WithPlayerID(context.Background(), "player-alice")

	_, err := h.charRepo.Create(context.Background(), characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter("alice", "player-alice")},
	})
	require.NoError(t, err)

	raw, err := os.ReadFile(filepath.Join(dungeonstest.ContentDir(t), "..", "internal", "dungeons", "testdata", "rage", "site.yaml"))
	require.NoError(t, err)
	dungeon, err := sessionworld.Compile(raw)
	require.NoError(t, err)
	h.launchAtStart(t, run, dungeon.Spec, "alice")
	inCombat(t, h.charRepo, "alice", 1)

	turn, err := h.handler.Turn(ctx, &sessionpb.TurnRequest{Session: run, Member: "alice"})
	require.NoError(t, err)
	require.Equal(t, "alice", turn.GetActive(), "geometry gate: the fighter acts first")
	start := storedMonsterHitPoints(t, h, run, "lieutenant")

	// His first turn: he rages, and the raging shows on his blow.
	live := fighterEndsTurn(ctx, t, h, run)
	activations := activationsOf(live)
	require.Len(t, activations, 1, "he raged once")
	act := activations[0]
	activate := act.GetActivate()
	require.Equal(t, storypb.Family_FAMILY_ACT, act.GetFamily())
	require.Equal(t, "lieutenant", activate.GetBy().GetId())
	require.Equal(t, "Rage", activate.GetAbility().GetName())
	require.Equal(t, "dnd5e:features:rage", activate.GetAbility().GetRef())
	require.Nil(t, activate.GetTarget(), "Rage names no target")
	require.Len(t, activate.GetResults(), 1, "the status landing is the one result")
	landed := activate.GetResults()[0]
	require.Equal(t, "condition-applied", landed.GetKind())
	require.Equal(t, "lieutenant", landed.GetTarget().GetId())
	require.Equal(t, ragingRef, landed.GetRef())
	require.Equal(t, "Raging", landed.GetName())
	t.Logf("wire: Act{family %s, activate{by %q, ability{ref %q, name %q}, results[{kind %q, target %q, ref %q, name %q}]}}",
		act.GetFamily(), activate.GetBy().GetId(), activate.GetAbility().GetRef(), activate.GetAbility().GetName(),
		landed.GetKind(), landed.GetTarget().GetId(), landed.GetRef(), landed.GetName())

	strikes := strikesBy(live, "lieutenant")
	require.Len(t, strikes, 1, "he swings once")
	requireRagingStrike(t, strikes[0])
	requireLiveIsStory(ctx, t, h, run, "alice", live)

	// His next turn: one of his two uses is left, but he rages; no activation, and
	// the blow still carries the raging.
	next := fighterEndsTurn(ctx, t, h, run)
	require.Empty(t, activationsOf(next), "he rages; the row is not on his table")
	nextStrikes := strikesBy(next, "lieutenant")
	require.Len(t, nextStrikes, 1)
	requireRagingStrike(t, nextStrikes[0])

	// The fighter's longsword does 7 and her 3 slashing: 10. Raging, he takes half.
	require.Equal(t, start, storedMonsterHitPoints(t, h, run, "lieutenant"), "nothing has hurt him yet")
	hit, err := h.handler.Attack(ctx, &sessionpb.AttackRequest{
		Session: run, Attacker: "alice", Target: "lieutenant",
		DeclarationId: currentDeclarationID(ctx, t, h.handler, run, "alice", sessionpb.Verb_VERB_ATTACK),
	})
	require.NoError(t, err)
	require.True(t, hit.GetHit())
	require.Equal(t, int32(5), hit.GetDamage(), "10 slashing, halved while he rages")
	require.Equal(t, start-5, storedMonsterHitPoints(t, h, run, "lieutenant"))
	t.Logf("fighter's blow: 10 slashing, damage sent %d; his hit points %d to %d", hit.GetDamage(), start, start-5)

	// And once more: still raging, still not asked.
	require.Empty(t, activationsOf(fighterEndsTurn(ctx, t, h, run)), "still raging; not asked again")
}

// requireRagingStrike asks of a strike by the raging Lieutenant: it hit, and it
// carries one part from the raging status worth 2.
func requireRagingStrike(t *testing.T, strike *storypb.Act) {
	t.Helper()
	require.Equal(t, storypb.AttackOutcome_ATTACK_OUTCOME_HIT, strike.GetAttack().GetOutcome())
	part := ragingComponent(t, strike)
	require.NotNil(t, part, "the strike carries a part from the raging status")
	require.Equal(t, "condition", part.GetSource())
	require.Len(t, part.GetRoll().GetModifiers(), 1)
	require.Equal(t, int32(2), part.GetRoll().GetModifiers()[0].GetValue())
	require.Equal(t, int32(2), part.GetRoll().GetTotal())
	// the longsword's 7 and the strength modifier (none) and the 2.
	require.Equal(t, int32(9), strike.GetAttack().GetDamage())
}
