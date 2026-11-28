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

// A CREATURE USES ACTION SURGE (rpg-project#1131, slice 5d-3): a site whose
// Lieutenant lets him use Action Surge from a count of one, and whose table's
// first row activates it when an enemy is in his reach, is launched from its
// YAML file and played over the wire. The first turn he has the fighter in
// reach he surges and swings twice; the fighter's client is sent the
// activation the way it is sent a player's own, with a result that says it
// gave one action. On his next turn the use is spent and he swings once.

func TestAcceptance_ALieutenantSurgesOnceAndSwingsTwiceAndTheWireCarriesIt(t *testing.T) {
	const run = "lieutenant-surges-run"
	// the pick lands on the first eligible row (the surge, while it is on his
	// table); every d20 rolls 1, so no blow lands and nobody is hurt.
	h := newAcceptanceHarnessWithDice(t, scriptedDice{d20: 1, d8: 8})
	ctx := auth.WithPlayerID(context.Background(), "player-alice")

	_, err := h.charRepo.Create(context.Background(), characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter("alice", "player-alice")},
	})
	require.NoError(t, err)

	raw, err := os.ReadFile(filepath.Join(dungeonstest.ContentDir(t), "..", "internal", "dungeons", "testdata", "action-surge", "site.yaml"))
	require.NoError(t, err)
	dungeon, err := sessionworld.Compile(raw)
	require.NoError(t, err)
	h.launchAtStart(t, run, dungeon.Spec, "alice")
	inCombat(t, h.charRepo, "alice", 1)

	turn, err := h.handler.Turn(ctx, &sessionpb.TurnRequest{Session: run, Member: "alice"})
	require.NoError(t, err)
	require.Equal(t, "alice", turn.GetActive(), "geometry gate: the fighter acts first")

	// His first turn: he walks up, surges, and swings twice.
	live := fighterEndsTurn(ctx, t, h, run)
	for _, a := range live {
		t.Logf("turn 1 act %s body %T", a.GetId(), a.GetBody())
	}
	activations := activationsOf(live)
	require.Len(t, activations, 1, "he surged once")
	act := activations[0]
	activate := act.GetActivate()
	require.Equal(t, storypb.Family_FAMILY_ACT, act.GetFamily())
	require.Equal(t, "lieutenant", activate.GetBy().GetId())
	require.Equal(t, "Action Surge", activate.GetAbility().GetName())
	require.Nil(t, activate.GetTarget())
	require.Len(t, activate.GetResults(), 1)
	granted := activate.GetResults()[0]
	require.Equal(t, "capacity-granted", granted.GetKind())
	require.Equal(t, "1 action", granted.GetDescription())
	t.Logf("wire: Act{family %s, activate{by %q, ability{ref %q, name %q}, results[{kind %q, description %q}]}}",
		act.GetFamily(), activate.GetBy().GetId(), activate.GetAbility().GetRef(), activate.GetAbility().GetName(),
		granted.GetKind(), granted.GetDescription())
	require.Len(t, strikesBy(live, "lieutenant"), 2, "the surge gave him a second action: two strikes")
	requireLiveIsStory(ctx, t, h, run, "alice", live)

	// His next turn: the one use is spent; no activation, one strike.
	next := fighterEndsTurn(ctx, t, h, run)
	require.Empty(t, activationsOf(next), "the use is spent; the row is not on his table")
	require.Len(t, strikesBy(next, "lieutenant"), 1, "one action: one strike")
}

// strikesBy is the attack acts a creature made.
func strikesBy(acts []*storypb.Act, id string) []*storypb.Act {
	var out []*storypb.Act
	for _, a := range acts {
		if a.GetAttack() != nil && a.GetAttack().GetBy().GetId() == id {
			out = append(out, a)
		}
	}
	return out
}
