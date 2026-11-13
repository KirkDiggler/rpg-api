package session_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	storypb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
)

// The arrest's words and line (the-mad-king.yaml, rpg-project#925).
const (
	comeQuietlyLine = "come-quietly"
	theMarchWords   = "The guards march you to the cells."
	heldYouSentence = "The guards hold you; you cannot walk away."
)

// placeInTheStoredWorld stands member on cell in the run's stored world: a
// placement, not a walk, where how they got there is not what is asserted.
func placeInTheStoredWorld(t *testing.T, h *acceptanceHarness, run, member string, cell *sessionpb.Position) {
	t.Helper()
	repo := sessionorch.NewEncounterRepository(h.redis, 0)
	data, err := repo.GetEncounter(context.Background(), run)
	require.NoError(t, err)
	for i := range data.Members {
		if string(data.Members[i].ID) == member {
			data.Members[i].Cell = &tkencounter.PositionData{X: cell.GetX(), Y: cell.GetY()}
			require.NoError(t, repo.SaveEncounter(context.Background(), run, data))
			return
		}
	}
	require.FailNowf(t, "not in the record", "%s", member)
}

// frontGateState is the front gate conversation's state in the stored world.
func frontGateState(t *testing.T, h *acceptanceHarness, run string) string {
	t.Helper()
	data, err := sessionorch.NewEncounterRepository(h.redis, 0).GetEncounter(context.Background(), run)
	require.NoError(t, err)
	for _, in := range data.Interactions {
		if in.ID == frontGate {
			return in.Blob.State
		}
	}
	require.FailNow(t, "the record holds no front gate")
	return ""
}

// relocationsOf is every RelocateAct in acts moving mover, with its act.
func relocationsOf(acts []*storypb.Act, mover string) []*storypb.Act {
	var out []*storypb.Act
	for _, a := range acts {
		if r := a.GetRelocate(); r != nil && r.GetMover().GetId() == mover {
			out = append(out, a)
		}
	}
	return out
}

// requireCaughtUp requires every act in live to appear, unchanged, in the
// member's catch-up story.
func requireCaughtUp(t *testing.T, live, story []*storypb.Act, whose string) {
	t.Helper()
	byID := map[string]*storypb.Act{}
	for _, a := range story {
		byID[a.GetId()] = a
	}
	for _, a := range live {
		got, ok := byID[a.GetId()]
		require.True(t, ok, "%s: act %s (%s) is in the catch-up read", whose, a.GetId(), a.GetAct())
		require.True(t, proto.Equal(a, got), "%s: act %s reads the same caught up as live", whose, a.GetId())
	}
}

// TestAcceptance_TheArrestThroughTheDoors is arrest slice one and the jail
// (rpg-project#925) through the api's own doors, at the castle's default
// shared view. Alice, told of the feast and hailed at the gate, misses both
// claims (every d20 a 1): the gate arrests her, and her stream carries the
// ArrestAct. Held, her walk away is refused FAILED_PRECONDITION with the
// guards' sentence alone. Bob, never arrested, is stood at the gate. Alice
// says "We'll come quietly.": she is shown her own RelocateAct, both ends and
// the march's words; bob is shown it through her. The gate conversation is
// jailed, and a catch-up read returns what both streams were sent live.
func TestAcceptance_TheArrestThroughTheDoors(t *testing.T) {
	const run = "arrest-door-run"
	aliceCtx := auth.WithPlayerID(context.Background(), "player-alice")
	bobCtx := auth.WithPlayerID(context.Background(), "player-bob")
	dice := &gateDice{}
	dice.face.Store(1)
	h := newAcceptanceHarnessWithDice(t, dice)
	for _, c := range []struct{ id, player string }{{"alice", "player-alice"}, {"bob", "player-bob"}} {
		_, err := h.charRepo.Create(aliceCtx, characterrepo.CreateInput{
			Character: &entities.Character{Data: armedFighter(c.id, c.player)},
		})
		require.NoError(t, err)
	}
	dungeon, err := sessionworld.Compile(madKingRaw(t))
	require.NoError(t, err)
	out := h.launch(t, run, dungeon.Spec, seatAt("alice", 6, 15), seatAt("bob", 8, 16))
	require.Empty(t, out.Formed, "precondition: nobody fights")
	_, err = sayTo(t, h, run, "alice", "player-alice", "pedlar", pedlarOptionID)
	require.NoError(t, err, "the stranger tells of the feast")
	walk(t, h, run, toTheCaptain(), "alice walks up to the captain")

	// The second failed claim arrests.
	arrested := watchActs(aliceCtx, t, h, run, "alice", func() {
		for _, claim := range []string{troupeOptionID, guestLine} {
			_, sayErr := sayTo(t, h, run, "alice", "player-alice", gateCaptain, claim)
			require.NoError(t, sayErr, "alice's %s claim, which misses", claim)
		}
	})
	requireNothingRefused(t, arrested, "alice, arrested")
	var arrests []*storypb.ArrestAct
	for _, a := range arrested {
		if ar := a.GetArrest(); ar != nil {
			arrests = append(arrests, ar)
		}
	}
	require.Len(t, arrests, 1, "alice's stream carries the arrest")
	require.Equal(t, frontGate, arrests[0].GetInteraction())
	require.Equal(t, "region-cells", arrests[0].GetCell())
	require.Equal(t, []string{"alice"}, partyIDs(arrests[0].GetTaken()), "she is taken")
	require.NotNil(t, frontGateArrest(t, h, run), "precondition: the record holds the arrest")

	// Held, she cannot walk away.
	_, err = h.handler.Move(aliceCtx, &sessionpb.MoveRequest{Session: run, Member: "alice", Path: []*sessionpb.Position{pbAt(-1, 10)}})
	require.Equal(t, codes.FailedPrecondition, status.Code(err), "a held walk is refused: %v", err)
	require.Equal(t, heldYouSentence, status.Convert(err).Message(), "the guards' sentence alone")

	// Bob, never arrested, stands at the gate.
	placeInTheStoredWorld(t, h, run, "bob", pbAt(-2, 10))

	var aliceLive []*storypb.Act
	bobLive := watchActs(bobCtx, t, h, run, "bob", func() {
		aliceLive = watchActs(aliceCtx, t, h, run, "alice", func() {
			_, sayErr := sayTo(t, h, run, "alice", "player-alice", gateCaptain, comeQuietlyLine)
			require.NoError(t, sayErr, "alice comes quietly")
		})
	})
	requireNothingRefused(t, aliceLive, "alice, jailed")
	requireNothingRefused(t, bobLive, "bob, watching")

	mine := relocationsOf(aliceLive, "alice")
	require.Len(t, mine, 1, "alice is shown her move")
	r := mine[0].GetRelocate()
	require.Equal(t, storypb.RelocateEnd_RELOCATE_END_BOTH, r.GetEnd())
	require.NotNil(t, r.GetFrom(), "where from")
	require.NotNil(t, r.GetTo(), "and where to")
	require.Equal(t, theMarchWords, r.GetWords())
	require.Empty(t, mine[0].GetPerceived().GetVia(), "her own")

	seen := relocationsOf(bobLive, "alice")
	require.Len(t, seen, 1, "bob is shown alice's move once")
	require.Equal(t, storypb.RelocateEnd_RELOCATE_END_BOTH, seen[0].GetRelocate().GetEnd(), "her version, both ends")
	require.Equal(t, theMarchWords, seen[0].GetRelocate().GetWords())
	require.Equal(t, []string{"alice"}, viaIDs(seen[0]), "through alice, at shared view")

	require.Equal(t, "jailed", frontGateState(t, h, run), "the gate conversation is jailed")

	requireCaughtUp(t, aliceLive, storyOf(aliceCtx, t, h.story, run, "alice"), "alice")
	requireCaughtUp(t, bobLive, storyOf(bobCtx, t, h.story, run, "bob"), "bob")
}

// partyIDs is the ids of parties, in order.
func partyIDs(parties []*storypb.Party) []string {
	out := make([]string, 0, len(parties))
	for _, p := range parties {
		out = append(out, p.GetId())
	}
	return out
}
