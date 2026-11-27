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

// A CREATURE USES WHAT ITS STAT BLOCK LETS IT (rpg-project#1131, slice 3d): a
// site whose stat block `lets` its Lieutenant use Second Wind and whose table
// has the row `activate: second wind` is launched from its YAML file; the
// fighter ends her turn, the Lieutenant takes his, and the fighter's client is
// sent his activation the way it is sent a player's own: one ActivateAct, BY
// him, no target, one healing result naming him.
// tablePickSides is the die a table's weighed pick rolls (the answer's own
// roll says `of 200`).
const tablePickSides = 200

// firstRowDice makes a table's weighed pick land on its first eligible row (the
// pick is a d200) and every other roll the maximum face, as testDice does.
type firstRowDice struct{}

func (firstRowDice) Roll(_ context.Context, size int) (int, error) {
	if size >= tablePickSides {
		return 1, nil
	}
	return size, nil
}

func TestAcceptance_ALieutenantUsesSecondWindAndTheWireCarriesIt(t *testing.T) {
	const run = "lieutenant-activation-run"
	h := newAcceptanceHarnessWithDice(t, firstRowDice{})
	ctx := auth.WithPlayerID(context.Background(), "player-alice")

	_, err := h.charRepo.Create(context.Background(), characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter("alice", "player-alice")},
	})
	require.NoError(t, err)

	raw, err := os.ReadFile(filepath.Join(dungeonstest.ContentDir(t), "..", "internal", "dungeons", "testdata", "activate-row", "site.yaml"))
	require.NoError(t, err)
	dungeon, err := sessionworld.Compile(raw)
	require.NoError(t, err)
	h.launchAtStart(t, run, dungeon.Spec, "alice")
	inCombat(t, h.charRepo, "alice", 1)

	turn, err := h.handler.Turn(ctx, &sessionpb.TurnRequest{Session: run, Member: "alice"})
	require.NoError(t, err)
	t.Logf("order %v active %q", turn.GetOrder(), turn.GetActive())
	require.Equal(t, "alice", turn.GetActive(), "geometry gate: the fighter acts first")

	live := watchActs(ctx, t, h, run, "alice", func() {
		_, endErr := h.handler.EndTurn(ctx, &sessionpb.EndTurnRequest{
			Session: run, Member: "alice",
			DeclarationId: currentDeclarationID(ctx, t, h.handler, run, "alice", sessionpb.Verb_VERB_END_TURN),
		})
		require.NoError(t, endErr)
	})

	var activations []*storypb.Act
	for _, a := range live {
		t.Logf("act %s family %s body %T", a.GetId(), a.GetFamily(), a.GetBody())
		if a.GetActivate() != nil {
			activations = append(activations, a)
		}
	}
	require.Len(t, activations, 1, "the Lieutenant activated once; his one use is spent")
	act := activations[0]
	activate := act.GetActivate()
	require.Equal(t, storypb.Family_FAMILY_ACT, act.GetFamily())
	require.Equal(t, "lieutenant", activate.GetBy().GetId())
	require.Equal(t, "Second Wind", activate.GetAbility().GetName())
	require.Nil(t, activate.GetTarget(), "Second Wind names no target")
	require.Len(t, activate.GetResults(), 1, "healing is the one result")
	healing := activate.GetResults()[0]
	require.Equal(t, "healing-applied", healing.GetKind())
	require.Equal(t, "lieutenant", healing.GetTarget().GetId())
	require.Equal(t, "dnd5e:features:second_wind", activate.GetAbility().GetRef())
	require.Equal(t, "dnd5e:features:second_wind", healing.GetRef())
	// the d10 rolls its maximum and the level adds 1; he stands at full hit
	// points, so the heal is requested and none of it lands.
	require.Equal(t, int32(11), healing.GetRequested())
	require.Equal(t, healing.GetBefore(), healing.GetAfter())
	require.Zero(t, healing.GetAmount())
	t.Logf("wire: Act{family %s, activate{by %q, ability{ref %q, name %q}, target %v, results[{kind %q, target %q, amount %d, requested %d, before %d, after %d, ref %q, name %q}]}}",
		act.GetFamily(), activate.GetBy().GetId(), activate.GetAbility().GetRef(), activate.GetAbility().GetName(), activate.GetTarget(),
		healing.GetKind(), healing.GetTarget().GetId(), healing.GetAmount(), healing.GetRequested(), healing.GetBefore(), healing.GetAfter(), healing.GetRef(), healing.GetName())
	requireLiveIsStory(ctx, t, h, run, "alice", live)
}
