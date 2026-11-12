package session_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/KirkDiggler/rpg-api/internal/auth"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
)

// hailThePedlar is the stranger's shouted line (the-mad-king.yaml,
// rpg-project#933): `loudness: shout`, the noise gate-1 answers.
const hailThePedlar = "hail-the-pedlar"

// storedCellOf is member's cell as the run's stored world holds it: gate-1
// stands in the gatehouse, out of alice's sight from the road, so the record
// and not her view is what says whether he left his post.
func storedCellOf(t *testing.T, h *acceptanceHarness, run, member string) [2]float64 {
	t.Helper()
	ctx := context.Background()
	session, err := sessionorch.NewSessionRepository(h.redis, 0).GetSession(ctx, run)
	require.NoError(t, err)
	data, err := sessionorch.NewEncounterRepository(h.redis, 0).GetEncounter(ctx, session.Encounter)
	require.NoError(t, err)
	for _, m := range data.Members {
		if string(m.ID) == member {
			require.NotNil(t, m.Cell, "%s stands somewhere", member)
			return [2]float64{m.Cell.X, m.Cell.Y}
		}
	}
	require.FailNowf(t, "not in the record", "%s", member)

	return [2]float64{}
}

// TestAcceptance_GateOneAnswersAShoutNotTalk is the operator's ruling on
// rpg-project#930 through the api's own Speak door (rpg-project#933): alice,
// beside the stranger outside the gate, asks him about the bustle — an
// ordinary spoken line gate-1 hears from the gatehouse — and he stays on his
// post. She hails the stranger with the castle's shouted line, and once a
// round has passed he has left it.
func TestAcceptance_GateOneAnswersAShoutNotTalk(t *testing.T) {
	ctx := auth.WithPlayerID(context.Background(), "player-alice")
	h := newAcceptanceHarnessWithDice(t, standAsideDice{})
	const run = "gate-shout-run"
	// Beside the stranger outside, where he sees her.
	out := launchTheMadKing(t, h, run, 6, 15)
	require.Empty(t, out.Formed, "precondition: nobody fights")
	posted := storedCellOf(t, h, run, gatePosted)

	_, err := speakAsOffered(ctx, t, h, run, "alice", "pedlar", pedlarOptionID)
	require.NoError(t, err, "a spoken line, which pays the world a round")
	require.Equal(t, posted, storedCellOf(t, h, run, gatePosted), "talk outside leaves him on his post")

	_, err = speakAsOffered(ctx, t, h, run, "alice", "pedlar", hailThePedlar)
	require.NoError(t, err, "the shout")
	_, err = speakAsOffered(ctx, t, h, run, "alice", "pedlar", pedlarOptionID)
	require.NoError(t, err, "and a round passes")
	require.NotEqual(t, posted, storedCellOf(t, h, run, gatePosted), "the shout drew him off his post")
}
