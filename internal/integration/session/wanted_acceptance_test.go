package session_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	storypb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
)

// The fight line and its answer (the-mad-king.yaml, rpg-project#925).
const (
	neverAliveLine = "never-alive"
	theHardWay     = "Then we do this the hard way. To arms!"
)

// hailedAtTheGate is the castle launched with alice told of the feast and
// walked up to the captain, who hails her, and bob down the road; every d20
// a 1, so every claim misses.
func hailedAtTheGate(t *testing.T, run string) *acceptanceHarness {
	t.Helper()
	ctx := auth.WithPlayerID(context.Background(), "player-alice")
	dice := &gateDice{}
	dice.face.Store(1)
	h := newAcceptanceHarnessWithDice(t, dice)
	for _, c := range []struct{ id, player string }{{"alice", "player-alice"}, {"bob", "player-bob"}} {
		_, err := h.charRepo.Create(ctx, characterrepo.CreateInput{
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
	return h
}

// claim has alice make claim to the captain, which misses.
func claim(t *testing.T, h *acceptanceHarness, run, option string) {
	t.Helper()
	_, err := sayTo(t, h, run, "alice", "player-alice", gateCaptain, option)
	require.NoError(t, err, "alice's %s claim, which misses", option)
}

// cube is a cube cell as the wire carries it.
func cube(x, y float64) *sessionpb.Position { return &sessionpb.Position{X: x, Y: y} }

// stepInto stands member one step from to in the stored world and walks
// that step, so the step is a verb the world refreshes on: a placement for
// the getting there, a walk for the arriving.
func stepInto(t *testing.T, h *acceptanceHarness, run, member, player string, from, to *sessionpb.Position) *sessionpb.MoveResponse {
	t.Helper()
	placeInTheStoredWorld(t, h, run, member, from)
	moved, err := h.handler.Move(auth.WithPlayerID(context.Background(), player),
		&sessionpb.MoveRequest{Session: run, Member: member, Path: []*sessionpb.Position{to}})
	require.NoError(t, err, "%s steps to %v", member, to)
	return moved
}

// requireNoWanted requires no act in acts to say anything of `wanted`: the
// guards' memory of whom they mean to arrest is theirs alone.
func requireNoWanted(t *testing.T, acts []*storypb.Act, whose string) {
	t.Helper()
	for _, a := range acts {
		raw, err := protojson.Marshal(a)
		require.NoError(t, err)
		require.NotContains(t, strings.ToLower(string(raw)), "wanted", "%s: act %s", whose, a.GetId())
	}
}

// arrestsIn is every ArrestAct in acts.
func arrestsIn(acts []*storypb.Act) []*storypb.ArrestAct {
	var out []*storypb.ArrestAct
	for _, a := range acts {
		if ar := a.GetArrest(); ar != nil {
			out = append(out, ar)
		}
	}
	return out
}

// TestAcceptance_TheFightLineAtTheArrest (rpg-project#925): alice, arrested
// at the gate, says "You will never take me alive." to the captain. Her
// stream carries his shouted answer and the garrison turning on the party;
// the fight forms, and her walk on its Move offer is no longer refused with
// the guards' custody sentence.
func TestAcceptance_TheFightLineAtTheArrest(t *testing.T) {
	const run = "fight-line-run"
	aliceCtx := auth.WithPlayerID(context.Background(), "player-alice")
	h := hailedAtTheGate(t, run)
	claim(t, h, run, troupeOptionID)
	claim(t, h, run, guestLine)
	require.NotNil(t, frontGateArrest(t, h, run), "precondition: the gate arrested")

	acts := watchActs(aliceCtx, t, h, run, "alice", func() {
		_, err := sayTo(t, h, run, "alice", "player-alice", gateCaptain, neverAliveLine)
		require.NoError(t, err, "alice refuses the arrest")
	})
	requireNothingRefused(t, acts, "alice")
	requireNoWanted(t, acts, "alice")
	var answers []*storypb.Speech
	turned := false
	for _, a := range acts {
		if sp := a.GetSpeech(); sp != nil && sp.GetWords() == theHardWay {
			answers = append(answers, sp)
		}
		if st := a.GetState().GetStance(); st != nil && st.GetStance() == "hostile" {
			turned = true
		}
	}
	require.NotEmpty(t, answers, "the guard's answer reaches her")
	require.Equal(t, storypb.Loudness_LOUDNESS_SHOUT, answers[0].GetLoudness(), "shouted")
	require.True(t, turned, "the garrison turns on the party")

	// The fight has formed, so a walk names the turn clock's Move offer.
	aff, err := h.handler.Afford(aliceCtx, &sessionpb.AffordRequest{Session: run, Member: "alice"})
	require.NoError(t, err)
	moveID := ""
	for _, d := range aff.GetDeclarations() {
		if d.GetVerb() == sessionpb.Verb_VERB_MOVE {
			moveID = d.GetId()
		}
	}
	require.NotEmpty(t, moveID, "the fight offers alice a Move")
	_, err = h.handler.Move(aliceCtx, &sessionpb.MoveRequest{
		Session: run, Member: "alice", DeclarationId: moveID, Path: []*sessionpb.Position{pbAt(-1, 10)},
	})
	require.NoError(t, err, "the hold is broken: she walks")
}

// TestAcceptance_TheWantedAreSeizedOnSight (rpg-project#925): bob stands at
// the gate while alice's first claim misses, then leaves down the road out
// of every guard's sight before her second. The gate arrests alice; bob,
// seen and gone, is wanted -- and nothing on any stream says so. Alice goes
// quietly and the gate is jailed. Bob comes back within two cells of the
// captain and is seized: his stream carries an ArrestAct naming him, his
// walk away is refused with the guards' sentence, and "We'll come quietly."
// takes him to the cells. The gate stays jailed.
func TestAcceptance_TheWantedAreSeizedOnSight(t *testing.T) {
	const run = "seize-run"
	bobCtx := auth.WithPlayerID(context.Background(), "player-bob")
	h := hailedAtTheGate(t, run)
	claim(t, h, run, troupeOptionID)

	// Bob at the gate, beside alice, in the guards' sight.
	stepInto(t, h, run, "bob", "player-bob", pbAt(-2, 11), pbAt(-2, 10))
	// And gone down the road, out of every guard's sight.
	stepInto(t, h, run, "bob", "player-bob", pbAt(7, 16), pbAt(6, 16))

	claim(t, h, run, guestLine)
	arrest := frontGateArrest(t, h, run)
	require.NotNil(t, arrest, "precondition: the gate arrested")
	taken := make([]string, 0, len(arrest.Taken))
	for _, m := range arrest.Taken {
		taken = append(taken, string(m.ID))
	}
	wanted := make([]string, 0, len(arrest.Wanted))
	for _, m := range arrest.Wanted {
		wanted = append(wanted, string(m.ID))
	}
	require.Equal(t, []string{"alice"}, taken, "precondition: alice is taken")
	require.Equal(t, []string{"bob"}, wanted, "precondition: bob, seen and gone, is wanted")

	_, err := sayTo(t, h, run, "alice", "player-alice", gateCaptain, comeQuietlyLine)
	require.NoError(t, err, "alice comes quietly")
	require.Equal(t, "jailed", frontGateState(t, h, run))

	// Bob comes back up to the captain.
	captain := worldMember(t, h, run, gateCaptain).Cell
	bobLive := watchActs(bobCtx, t, h, run, "bob", func() {
		stepInto(t, h, run, "bob", "player-bob", cube(captain.X, captain.Y+3), cube(captain.X, captain.Y+2))
	})
	seized := arrestsIn(bobLive)
	require.Len(t, seized, 1, "bob's stream carries his arrest")
	require.Equal(t, []string{"bob"}, partyIDs(seized[0].GetTaken()), "naming him")

	_, err = h.handler.Move(bobCtx, &sessionpb.MoveRequest{Session: run, Member: "bob", Path: []*sessionpb.Position{cube(captain.X, captain.Y+3)}})
	require.Equal(t, codes.FailedPrecondition, status.Code(err), "a held walk is refused: %v", err)
	require.Equal(t, heldYouSentence, status.Convert(err).Message())

	jailed := watchActs(bobCtx, t, h, run, "bob", func() {
		_, sayErr := sayTo(t, h, run, "bob", "player-bob", gateCaptain, comeQuietlyLine)
		require.NoError(t, sayErr, "bob comes quietly")
	})
	mine := relocationsOf(jailed, "bob")
	require.Len(t, mine, 1, "bob is shown his move")
	require.Equal(t, theMarchWords, mine[0].GetRelocate().GetWords())
	require.Equal(t, "jailed", frontGateState(t, h, run), "the gate is still jailed")

	for _, who := range []struct{ id, player string }{{"alice", "player-alice"}, {"bob", "player-bob"}} {
		ctx := auth.WithPlayerID(context.Background(), who.player)
		requireNoWanted(t, storyOf(ctx, t, h.story, run, who.id), who.id)
	}
	requireNoWanted(t, bobLive, "bob, live")
	requireNoWanted(t, jailed, "bob, live")
}
