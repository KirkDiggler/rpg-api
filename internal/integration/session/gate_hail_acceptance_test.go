package session_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
)

// The castle's gate interaction and the captain's hail (the-mad-king.yaml,
// rpg-project#923): `front-gate` carries `approach: { within: 2 }`, and the
// captain's machine picks this line `on: approached`.
const (
	frontGate       = "front-gate"
	theCaptainsHail = "You there. Nobody passes this gate without my say. Speak."
)

// madKingRaw is the shipped castle's bytes.
func madKingRaw(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dungeonstest.ContentDir(t), madKingKey+".yaml")) //nolint:gosec // the shipped content directory
	require.NoError(t, err)

	return raw
}

// speakRowOf is the Speak declaration on member's menu.
func speakRowOf(t *testing.T, h *acceptanceHarness, run, member, player string) *sessionpb.Declaration {
	t.Helper()
	ctx := auth.WithPlayerID(context.Background(), player)
	aff, err := h.handler.Afford(ctx, &sessionpb.AffordRequest{Session: run, Member: member})
	require.NoError(t, err)
	for _, d := range aff.GetDeclarations() {
		if d.GetVerb() == sessionpb.Verb_VERB_SPEAK {
			return d
		}
	}
	require.FailNowf(t, "no Speak row", "%s", member)

	return nil
}

// TestAcceptance_TheGuardHailsTheApproach is Gate S3 on the server the api
// serves (rpg-project#923): alice walks the road toward the gate; the first
// carrier of `front-gate` who sees her come within two cells — the captain,
// on this road — hails her. The walk stops where the hail lands, his line is
// in her act stream, and her Speak row carries the address (guard and
// interaction) beside the claims. Bob, seated away from the gate, is not
// addressed: his Speak row carries none.
func TestAcceptance_TheGuardHailsTheApproach(t *testing.T) {
	aliceCtx := auth.WithPlayerID(context.Background(), "player-alice")
	h := newAcceptanceHarnessWithDice(t, standAsideDice{})
	const run = "gate-hail-run"

	for _, c := range []struct{ id, player string }{{"alice", "player-alice"}, {"bob", "player-bob"}} {
		_, err := h.charRepo.Create(aliceCtx, characterrepo.CreateInput{
			Character: &entities.Character{Data: armedFighter(c.id, c.player)},
		})
		require.NoError(t, err)
	}
	dungeon, err := sessionworld.Compile(madKingRaw(t))
	require.NoError(t, err)
	// Alice beside the stranger outside; bob further down the road, beyond
	// every carrier's approach.
	out := h.launch(t, run, dungeon.Spec, seatAt("alice", 6, 15), seatAt("bob", 8, 16))
	require.Empty(t, out.Formed, "precondition: nobody fights")

	require.Nil(t, speakRowOf(t, h, run, "alice", "player-alice").GetAddress(),
		"precondition: nobody addresses her before she walks")

	path := toTheCaptain()
	var moved *sessionpb.MoveResponse
	acts := watchActs(aliceCtx, t, h, run, "alice", func() {
		moved, err = h.handler.Move(aliceCtx, &sessionpb.MoveRequest{Session: run, Member: "alice", Path: path})
		require.NoError(t, err, "a hail is an answer, never an error")
	})
	require.Equal(t, sessionpb.MovementStatus_MOVEMENT_STATUS_STOPPED, moved.GetStatus(), "the hail stops her")
	require.NotEmpty(t, moved.GetSteps(), "she walked until the hail landed")
	require.Less(t, len(moved.GetSteps()), len(path), "and the rest of the walk was dropped")
	require.Equal(t, []string{theCaptainsHail}, linesOf(acts, "captain"), "the captain hails her")
	var hailedTo []string
	for _, a := range acts {
		if ans := a.GetTableAnswer(); ans != nil && ans.GetWho().GetId() == "captain" && ans.GetSay() != "" {
			hailedTo = append(hailedTo, ans.GetTo().GetId())
		}
	}
	require.Equal(t, []string{"alice"}, hailedTo, "his line names who it is spoken to")

	speak := speakRowOf(t, h, run, "alice", "player-alice")
	require.True(t, speak.GetAvailable())
	require.NotEmpty(t, speak.GetSpeakOptions(), "the panel's claims ride the same row")
	require.NotNil(t, speak.GetAddress(), "her Speak row carries the address")
	require.Equal(t, "captain", speak.GetAddress().GetGuardMemberId())
	require.Equal(t, frontGate, speak.GetAddress().GetInteractionId())

	require.Nil(t, speakRowOf(t, h, run, "bob", "player-bob").GetAddress(),
		"nobody else's Speak row carries it")
}
