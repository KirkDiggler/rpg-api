package session_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	storypb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
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

// theShout is hail-the-pedlar's words, verbatim from the castle.
const theShout = "Oi! Pedlar! What have you got to sell?"

// TestAcceptance_AShoutIsHeardUnseen is the one speech message on the
// stream (rpg-project#934): alice shouts hail-the-pedlar beside the stranger
// down the road; bob, up by the gatehouse, one room over and out of her
// sight, receives a Speech with the words, no speaker and no spoken-to, one
// room off, reached by hearing. The stranger's spoken answer reaches him as
// a voice with no words: talk does not carry its words out of its room.
//
// WHY NOT THE COURTYARD: inside the gate is two rooms from the road, and a
// shout carries its words one room (R9), so a player there hears nothing of
// it at all.
//
// AT EACH THEIR OWN (rpg-project#935): this is what bob's own ears give him.
// At shared view he is also shown alice's version through her, speaker and
// all (TestAcceptance_TheSharingDial).
func TestAcceptance_AShoutIsHeardUnseen(t *testing.T) {
	aliceCtx := auth.WithPlayerID(context.Background(), "player-alice")
	bobCtx := auth.WithPlayerID(context.Background(), "player-bob")
	h := newAcceptanceHarnessWithDice(t, standAsideDice{})
	const run = "gate-shout-heard-run"
	for _, c := range []struct{ id, player string }{{"alice", "player-alice"}, {"bob", "player-bob"}} {
		_, err := h.charRepo.Create(aliceCtx, characterrepo.CreateInput{
			Character: &entities.Character{Data: armedFighter(c.id, c.player)},
		})
		require.NoError(t, err)
	}
	dungeon, err := sessionworld.Compile(eachTheirOwn(t, madKingRaw(t)))
	require.NoError(t, err)
	out := h.launch(t, run, dungeon.Spec, seatAt("alice", 6, 15), seatAt("bob", -1, 9))
	require.Empty(t, out.Formed, "precondition: nobody fights")

	acts := watchActs(bobCtx, t, h, run, "bob", func() {
		_, err = speakAsOffered(aliceCtx, t, h, run, "alice", "pedlar", hailThePedlar)
		require.NoError(t, err, "alice shouts")
	})
	var heard, voices []*storypb.Speech
	for _, a := range acts {
		if sp := a.GetSpeech(); sp != nil && sp.Words == nil {
			voices = append(voices, sp)
		}
		if sp := a.GetSpeech(); sp != nil && sp.GetWords() == theShout {
			require.Equal(t, storypb.Family_FAMILY_ACT, a.GetFamily(), "a speech rides the act family")
			require.Equal(t, []storypb.Sense{storypb.Sense_SENSE_HEARING}, a.GetPerceived().GetSenses(), "he heard it and saw nobody")
			heard = append(heard, sp)
		}
	}
	require.Len(t, heard, 1, "bob hears the shout once")
	sp := heard[0]
	require.Nil(t, sp.GetSpeaker(), "he did not see who shouted")
	require.Nil(t, sp.GetSpokenTo(), "nor whom it was shouted at")
	require.Equal(t, storypb.Loudness_LOUDNESS_SHOUT, sp.GetLoudness())
	require.NotNil(t, sp.Hops, "an unseen speaker is placed by rooms")
	require.Equal(t, int32(1), sp.GetHops(), "one room over")
	require.NotNil(t, sp.GetPlace(), "and by where the voice was")

	require.Len(t, voices, 1, "the stranger's answer reaches him as a voice")
	require.Nil(t, voices[0].GetSpeaker())
	require.Equal(t, storypb.Loudness_LOUDNESS_SPEAK, voices[0].GetLoudness(), "spoken, so its words stay in its room")
	requireNoRetiredSpeech(t, acts, "bob, who heard it unseen")

	// The speaker's own story of the exchange: her line is a Speech of her
	// own words, and nothing rides the retired shapes.
	mine := storyOf(aliceCtx, t, h.story, run, "alice")
	requireNoRetiredSpeech(t, mine, "alice, who shouted")
	require.Contains(t, linesOf(mine, "alice"), theShout, "her own line is a Speech")
}

// requireNoRetiredSpeech requires a stream for a spoken exchange to carry
// none of the retired speech shapes (rpg-project#934): no `speak` act, and no
// heard Sound of a voice -- an unseen voice arrives only as a Speech act.
func requireNoRetiredSpeech(t *testing.T, acts []*storypb.Act, whose string) {
	t.Helper()
	for _, a := range acts {
		require.Nil(t, a.GetSpeak(), "%s: no retired speak act (%s)", whose, a.GetId()) //nolint:staticcheck // asserting the retired body is never sent
		if h := a.GetPerception().GetHeard(); h != nil {
			require.NotEqual(t, "voice", h.GetKind(), "%s: a voice arrives as a Speech, never a Sound (%s)", whose, a.GetId())
		}
	}
}
