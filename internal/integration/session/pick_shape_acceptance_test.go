package session_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	storypb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
)

// standAsideLine is the captain's stand-aside as the shipped castle writes
// it: the line this scene whispers.
var standAsideLine = []byte("          - id: stand-aside\n            say: Stand aside or bleed.\n")

// captainsPicks is every table answer the captain gives under a speak act
// in acts: his pick of how a line went.
func captainsPicks(acts []*storypb.Act) []*storypb.TableAnswer {
	var out []*storypb.TableAnswer
	for _, a := range acts {
		if ans := a.GetTableAnswer(); ans != nil && ans.GetWho().GetId() == gateCaptain && a.GetVerb() == "speak" {
			out = append(out, ans)
		}
	}
	return out
}

// whisperedStandAside launches this scene's copy of the castle, with the
// captain's stand-aside WHISPERED, alice walked up beside the captain and bob
// watching from beside the hail cell: in the captain's sight and out of
// whisper reach. Every d20 shows the dice's face.
func whisperedStandAside(t *testing.T, run string) (*acceptanceHarness, *gateDice) {
	t.Helper()
	ctx := auth.WithPlayerID(context.Background(), "player-alice")
	dice := &gateDice{}
	dice.face.Store(20)
	h := newAcceptanceHarnessWithDice(t, dice)
	for _, c := range []struct{ id, player string }{{"alice", "player-alice"}, {"bob", "player-bob"}} {
		_, err := h.charRepo.Create(ctx, characterrepo.CreateInput{
			Character: &entities.Character{Data: armedFighter(c.id, c.player)},
		})
		require.NoError(t, err)
	}
	source := madKingRaw(t)
	require.Equal(t, 1, bytes.Count(source, standAsideLine), "precondition: the captain's stand-aside, once")
	whispered := bytes.Replace(source, standAsideLine,
		append(append([]byte(nil), standAsideLine...), []byte("            loudness: whisper\n")...), 1)
	dungeon, err := sessionworld.Compile(whispered)
	require.NoError(t, err)
	out := h.launch(t, run, dungeon.Spec, seatAt("alice", 1, 14), seatAt("bob", bobCol, bobRow))
	require.Empty(t, out.Formed, "precondition: nobody fights")
	walk(t, h, run, toTheCaptain()[6:], "alice walks up to the captain")

	return h, dice
}

// standAsideWatched has alice say the whispered stand-aside, every d20 at
// face, and returns the captain's picks each of alice and bob is sent.
func standAsideWatched(t *testing.T, h *acceptanceHarness, dice *gateDice, run string, face int32) (alice, bob []*storypb.TableAnswer) {
	t.Helper()
	aliceCtx := auth.WithPlayerID(context.Background(), "player-alice")
	bobCtx := auth.WithPlayerID(context.Background(), "player-bob")
	dice.face.Store(face)
	var aliceActs []*storypb.Act
	bobActs := watchActs(bobCtx, t, h, run, "bob", func() {
		aliceActs = watchActs(aliceCtx, t, h, run, "alice", func() {
			_, err := speakAsOffered(aliceCtx, t, h, run, "alice", gateCaptain, "stand-aside")
			require.NoError(t, err, "alice tells the captain to stand aside")
		})
	})
	var watched []*storypb.Speech
	for _, a := range bobActs {
		if sp := a.GetSpeech(); sp != nil && sp.GetSpeaker().GetId() == "alice" {
			watched = append(watched, sp)
		}
	}
	require.Len(t, watched, 1, "precondition: bob saw alice say it")
	require.Nil(t, watched[0].Words, "precondition: and caught none of it")

	return captainsPicks(aliceActs), captainsPicks(bobActs)
}

// TestAcceptance_AWatcherIsSentOnlyWhoAnswered is rpg-project#948 on the
// server the api serves: bob, who sees the captain and caught none of
// alice's whispered stand-aside, receives the captain's pick as exactly who
// answered -- no roll, no candidates, no word -- the same whether the line
// missed or landed; alice, who said it, receives its die and candidates.
// One run: the miss first, which leaves the line on her menu, then the win.
func TestAcceptance_AWatcherIsSentOnlyWhoAnswered(t *testing.T) {
	const run = "pick-shape-run"
	h, dice := whisperedStandAside(t, run)
	var watched [2]*storypb.TableAnswer
	for i, face := range []int32{2, 18} {
		alice, bob := standAsideWatched(t, h, dice, run, face)
		require.Len(t, bob, 1, "bob sees the captain answer")
		require.True(t, proto.Equal(&storypb.TableAnswer{Who: bob[0].GetWho()}, bob[0]),
			"who answered, and nothing of the die: %v", bob[0])
		require.Equal(t, gateCaptain, bob[0].GetWho().GetId())
		require.Nil(t, bob[0].GetRoll(), "no zero roll standing in for no roll")
		watched[i] = bob[0]

		require.Len(t, alice, 1)
		require.NotNil(t, alice[0].GetRoll(), "alice, who said it, is sent the die")
		require.NotEmpty(t, alice[0].GetCandidates(), "and every candidate")
	}
	require.True(t, proto.Equal(watched[0], watched[1]), "the same pick, missed or landed")
}
