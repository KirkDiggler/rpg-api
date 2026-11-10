package session_test

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
)

// attackRow is the member's Attack declaration from a fresh Afford.
func attackRow(ctx context.Context, t *testing.T, h *acceptanceHarness, run, member string) *sessionpb.Declaration {
	t.Helper()
	afforded, err := h.handler.Afford(ctx, &sessionpb.AffordRequest{Session: run, Member: member})
	require.NoError(t, err)
	for _, d := range afforded.GetDeclarations() {
		if d.GetVerb() == sessionpb.Verb_VERB_ATTACK {
			return d
		}
	}
	require.FailNow(t, "no Attack row", "%s is offered no Attack", member)

	return nil
}

// attackCandidate is target's candidate on an Attack row.
func attackCandidate(t *testing.T, row *sessionpb.Declaration, target string) *sessionpb.TargetCandidate {
	t.Helper()
	for _, c := range row.GetCandidates() {
		if c.GetMember() == target {
			return c
		}
	}
	require.FailNow(t, "not a candidate", "%s is not an Attack candidate", target)

	return nil
}

// hexLine is a shortest walk across open floor from one axial cell to
// another, excluding the start: the cube-lerp line a client's own walk on an
// open floor draws. Each step is adjacent to the last, and its length is the
// hex distance.
func hexLine(from, to *sessionpb.Position) []*sessionpb.Position {
	dq, dr := to.GetX()-from.GetX(), to.GetY()-from.GetY()
	n := int((math.Abs(dq) + math.Abs(dr) + math.Abs(dq+dr)) / 2)
	out := make([]*sessionpb.Position, 0, n)
	for i := 1; i <= n; i++ {
		f := float64(i) / float64(n)
		// A tiny nudge keeps the rounding off exact hex edges.
		q := from.GetX() + dq*f + 1e-6
		r := from.GetY() + dr*f + 1e-6
		s := -q - r
		rq, rr, rs := math.Round(q), math.Round(r), math.Round(s)
		switch dQ, dR, dS := math.Abs(rq-q), math.Abs(rr-r), math.Abs(rs-s); {
		case dQ > dR && dQ > dS:
			rq = -rr - rs
		case dR > dS:
			rr = -rq - rs
		}
		out = append(out, &sessionpb.Position{X: rq, Y: rr})
	}

	return out
}

// TestAcceptance_AnOutOfReachGuardNamesHisApproach is click a hostile to
// attack on the api (rpg-project#872 D4): in free roam on the shipped
// castle, alice's Attack candidate for the posted gate guard, out of reach,
// names the cell this turn's walk ends on in reach of him. A Move along a
// shortest walk to that cell is accepted whole, and from it the Attack on
// him is available, the approach cleared, and the swing lands as an
// ordinary attack.
func TestAcceptance_AnOutOfReachGuardNamesHisApproach(t *testing.T) {
	ctx := auth.WithPlayerID(context.Background(), "player-alice")
	h := newAcceptanceHarnessWithDice(t, standAsideDice{})
	const run = "click-attack-approach-run"
	// On the gatehouse floor, four steps south of gate-1's reach.
	out := launchTheMadKing(t, h, run, 2, 12)
	require.Empty(t, out.Formed, "precondition: free roam, nobody fights")
	start := launched(t, out, "alice").Position

	row := attackRow(ctx, t, h, run, "alice")
	guard := attackCandidate(t, row, gatePosted)
	require.False(t, guard.GetAvailable(), "precondition: he starts out of reach")
	require.Equal(t, sessionpb.ShortfallReason_SHORTFALL_REASON_TARGET_OUT_OF_REACH, guard.GetWhy().GetReason())
	approach := guard.GetApproach()
	require.NotNil(t, approach, "free roam has no budget: a walk reaches him, and the candidate names where it stops")

	path := hexLine(&sessionpb.Position{X: start.X, Y: start.Y}, approach)
	require.Greater(t, len(path), 1, "precondition: the approach is a walk, not a step")
	// The walk crosses the gate's approach in sight of its carriers, so a hail
	// stops it once and she walks on (rpg-project#923); walk requires the rest
	// accepted whole.
	walk(t, h, run, path, "the Move to the offered cell is not refused")
	where, err := h.handler.GetWhere(ctx, &sessionpb.GetWhereRequest{Session: run, Member: "alice"})
	require.NoError(t, err)
	require.True(t, samePosition(approach, where.GetPosition()), "the walk ends on the offered cell")

	row = attackRow(ctx, t, h, run, "alice")
	guard = attackCandidate(t, row, gatePosted)
	require.True(t, guard.GetAvailable(), "from the offered cell he is in reach")
	require.Nil(t, guard.GetApproach(), "an in-reach candidate needs no walk")
	require.True(t, row.GetAvailable())

	attacked, err := h.handler.Attack(ctx, &sessionpb.AttackRequest{
		Session: run, Attacker: "alice", Target: gatePosted, DeclarationId: row.GetId(),
	})
	require.NoError(t, err, "the swing lands as an ordinary attack")
	require.NotNil(t, attacked.GetCalculation())
}
