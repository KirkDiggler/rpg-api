package session_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	storypb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
)

// sharing_acceptance_test.go is the party's sharing dial on the server the
// api serves (rpg-project#935): the shipped castle says nothing, so it plays
// at shared view, and each player is sent what a party friend perceived,
// marked via that friend; at each their own, nothing comes through a friend.

// inTheGreatHall is a cell in the great hall, behind closed doors, where no
// shout from the road reaches (the session's castle sharing tests stand a
// player there).
func inTheGreatHall() (col, row int) { return -14, -16 }

// sharingRun is the castle launched with alice beside the stranger on the
// road and bob in the great hall; eachOwn marks it `party: { sharing: own }`.
func sharingRun(t *testing.T, eachOwn bool) *acceptanceHarness {
	t.Helper()
	ctx := auth.WithPlayerID(context.Background(), "player-alice")
	h := newAcceptanceHarnessWithDice(t, standAsideDice{})
	for _, c := range []struct{ id, player string }{{"alice", "player-alice"}, {"bob", "player-bob"}} {
		_, err := h.charRepo.Create(ctx, characterrepo.CreateInput{
			Character: &entities.Character{Data: armedFighter(c.id, c.player)},
		})
		require.NoError(t, err)
	}
	source := madKingRaw(t)
	if eachOwn {
		source = eachTheirOwn(t, source)
	}
	dungeon, err := sessionworld.Compile(source)
	require.NoError(t, err)
	col, row := inTheGreatHall()
	out := h.launch(t, sharingRunID, dungeon.Spec, seatAt("alice", 6, 15), seatAt("bob", col, row))
	require.Empty(t, out.Formed, "precondition: nobody fights")
	return h
}

const sharingRunID = "sharing-run"

// eachTheirOwn is source marked `party: { sharing: own }`: the stop at which
// a player is shown only what their own character perceives. A scene about
// one player's own perception plays here, so a friend's version does not
// stand in for it (rpg-project#935).
func eachTheirOwn(t *testing.T, source []byte) []byte {
	t.Helper()
	require.Equal(t, 1, bytes.Count(source, []byte("\nroom:")), "precondition: one root room key")
	return bytes.Replace(source, []byte("\nroom:"), []byte("\nparty:\n  sharing: own\nroom:"), 1)
}

// ownActs is the acts in acts that reached the reader by their own senses:
// none came through a friend (rpg-project#935).
func ownActs(acts []*storypb.Act) []*storypb.Act {
	var out []*storypb.Act
	for _, a := range acts {
		if len(a.GetPerceived().GetVia()) == 0 {
			out = append(out, a)
		}
	}
	return out
}

// viaIDs is the ids of an act's via, in order.
func viaIDs(a *storypb.Act) []string {
	out := make([]string, 0, len(a.GetPerceived().GetVia()))
	for _, p := range a.GetPerceived().GetVia() {
		out = append(out, p.GetId())
	}
	return out
}

// affords is alice's and bob's Afford, in that order.
func affords(t *testing.T, h *acceptanceHarness) [2]*sessionpb.AffordResponse {
	t.Helper()
	var out [2]*sessionpb.AffordResponse
	for i, m := range []struct{ id, player string }{{"alice", "player-alice"}, {"bob", "player-bob"}} {
		resp, err := h.handler.Afford(auth.WithPlayerID(context.Background(), m.player),
			&sessionpb.AffordRequest{Session: sharingRunID, Member: m.id})
		require.NoError(t, err)
		out[i] = resp
	}
	return out
}

// playTheDial shouts hail-the-pedlar from alice on the road, watching bob's
// stream, then walks bob one cell in the great hall, watching alice's. It
// returns what bob was sent of the shout, what alice was sent of bob's
// step, and both players' Afford after.
func playTheDial(t *testing.T, h *acceptanceHarness) (shout, step []*storypb.Act, after [2]*sessionpb.AffordResponse) {
	t.Helper()
	aliceCtx := auth.WithPlayerID(context.Background(), "player-alice")
	bobCtx := auth.WithPlayerID(context.Background(), "player-bob")
	shout = watchActs(bobCtx, t, h, sharingRunID, "bob", func() {
		_, err := speakAsOffered(aliceCtx, t, h, sharingRunID, "alice", "pedlar", hailThePedlar)
		require.NoError(t, err, "alice shouts")
	})
	col, row := inTheGreatHall()
	step = watchActs(aliceCtx, t, h, sharingRunID, "alice", func() {
		_, err := h.handler.Move(bobCtx, &sessionpb.MoveRequest{
			Session: sharingRunID, Member: "bob", Path: []*sessionpb.Position{pbAt(col+1, row)},
		})
		require.NoError(t, err, "bob steps in the great hall")
	})
	return shout, step, affords(t, h)
}

// TestAcceptance_TheSharingDial plays the same castle at shared view (the
// default: the castle says nothing) and at each their own.
//
//   - (a) At shared view bob, in the great hall, perceives nothing of
//     alice's shout on the road, and is sent a Speech with the words, the
//     speaker, no hops, via alice.
//   - (b) At each their own he is sent nothing of it, and nothing via anyone.
//   - (c) At shared view alice is sent bob's sightings as he steps, the
//     observer bob and via bob; at each their own nothing via bob.
//   - (d) Each player's Afford is the same at both stops.
func TestAcceptance_TheSharingDial(t *testing.T) {
	viewShout, viewStep, viewAfford := playTheDial(t, sharingRun(t, false))

	var spoken []*storypb.Act
	for _, a := range viewShout {
		if sp := a.GetSpeech(); sp != nil && sp.GetWords() == theShout {
			spoken = append(spoken, a)
		}
	}
	require.Len(t, spoken, 1, "(a) bob is sent the shout once")
	sp := spoken[0].GetSpeech()
	require.Equal(t, storypb.Family_FAMILY_ACT, spoken[0].GetFamily())
	require.Equal(t, "alice", sp.GetSpeaker().GetId(), "(a) who said it, as alice saw herself say it")
	require.Equal(t, "pedlar", sp.GetSpokenTo().GetId(), "(a) and to whom")
	require.Nil(t, sp.Hops, "(a) a speaker named is no distance")
	require.NotNil(t, sp.GetPlace(), "(a) the place it was said")
	require.Equal(t, storypb.Loudness_LOUDNESS_SHOUT, sp.GetLoudness())
	require.Nil(t, sp.GetRoll(), "(a) hail-the-pedlar is a plain line: no roll")
	require.Equal(t, []string{"alice"}, viaIDs(spoken[0]), "(a) via alice")

	var sighted int
	for _, a := range viewStep {
		p := a.GetPerception()
		if p == nil || p.GetSense() != storypb.Sense_SENSE_SIGHT || len(viaIDs(a)) == 0 {
			continue
		}
		require.Equal(t, []string{"bob"}, viaIDs(a), "(c) a sighting through a friend is bob's")
		require.Equal(t, "bob", p.GetObserver().GetId(), "(c) and names bob as the one who saw")
		sighted++
	}
	require.Positive(t, sighted, "(c) alice is sent bob's sightings via bob")

	ownShout, ownStep, ownAfford := playTheDial(t, sharingRun(t, true))
	for _, a := range ownShout {
		require.NotEqual(t, theShout, a.GetSpeech().GetWords(), "(b) bob is sent nothing of the shout")
		require.Empty(t, viaIDs(a), "(b) nothing comes through a friend")
	}
	for _, a := range ownStep {
		require.Empty(t, viaIDs(a), "(b) nothing of bob's step reaches alice through him")
	}

	for i, who := range []string{"alice", "bob"} {
		require.True(t, proto.Equal(viewAfford[i], ownAfford[i]), "(d) %s's Afford is the same at both stops", who)
	}
}
