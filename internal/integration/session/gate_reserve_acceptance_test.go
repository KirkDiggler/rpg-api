package session_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/conditions"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
)

// The front gate's lines this test speaks (the-mad-king.yaml): `guest` is a
// checked claim that reads the gate's conversation, `greet` reads none.
const (
	guestLine = "guest"
	greetLine = "greet"
)

// bob stands beside the hail cell, within Speak reach of the captain and in
// his sight -- the session's castle reserve tests stand the cleric there.
const (
	bobCol = -2
	bobRow = 10
)

// giveGuidance stores a Guidance die on member's sheet, so a check it makes
// pauses to ask whether the die is spent. Given AFTER the launch, whose long
// rest would clear it -- the session's castle reserve tests do the same.
func giveGuidance(t *testing.T, h *acceptanceHarness, member string) {
	t.Helper()
	ctx := context.Background()
	stored, err := h.charRepo.Get(ctx, characterrepo.GetInput{ID: member})
	require.NoError(t, err)
	condition, err := conditions.NewGuidedCondition(conditions.NewGuidedConditionInput{
		MemberID: member, SourceID: "cleric-1", SourceRef: refs.Spells.Guidance(),
	})
	require.NoError(t, err)
	raw, err := condition.ToJSON()
	require.NoError(t, err)
	stored.Character.Data.Conditions = append(stored.Character.Data.Conditions, raw)
	_, err = h.charRepo.Update(ctx, characterrepo.UpdateInput{Character: stored.Character})
	require.NoError(t, err)
}

// captainLine is one line said to the captain on member's Speak row.
func captainLine(t *testing.T, h *acceptanceHarness, run, member, player, option string) *sessionpb.SpeakOption {
	t.Helper()
	for _, o := range speakRowOf(t, h, run, member, player).GetSpeakOptions() {
		if o.GetMemberId() == "captain" && o.GetOptionId() == option {
			return o
		}
	}
	require.FailNowf(t, "line not listed", "%s has no %q for the captain", member, option)

	return nil
}

// reactRowOf is member's REACT row, nil when none is listed.
func reactRowOf(t *testing.T, h *acceptanceHarness, run, member, player string) *sessionpb.Declaration {
	t.Helper()
	ctx := auth.WithPlayerID(context.Background(), player)
	aff, err := h.handler.Afford(ctx, &sessionpb.AffordRequest{Session: run, Member: member})
	require.NoError(t, err)
	for _, d := range aff.GetDeclarations() {
		if d.GetVerb() == sessionpb.Verb_VERB_REACT {
			return d
		}
	}

	return nil
}

// sayToCaptain sends one line to the captain as offered.
func sayToCaptain(h *acceptanceHarness, run, member, player string, o *sessionpb.SpeakOption) (*sessionpb.SpeakResponse, error) {
	ctx := auth.WithPlayerID(context.Background(), player)
	return h.handler.Speak(ctx, &sessionpb.SpeakRequest{
		Session: run, Member: member, Target: &sessionpb.SpeakRequest_MemberId{MemberId: "captain"},
		OptionId: o.GetOptionId(), Revision: o.GetRevision(),
	})
}

// speakAsOffered says option to target the way a client does since gate
// slice 4 (rpg-project#924): it reads member's Speak row and echoes the
// revision the line was offered at. A line that reads no conversation is
// offered at revision zero, so this is the plain Speak for those.
func speakAsOffered(
	ctx context.Context, t *testing.T, h *acceptanceHarness, run, member, target, option string,
) (*sessionpb.SpeakResponse, error) {
	t.Helper()
	aff, err := h.handler.Afford(ctx, &sessionpb.AffordRequest{Session: run, Member: member})
	require.NoError(t, err)
	for _, d := range aff.GetDeclarations() {
		for _, o := range d.GetSpeakOptions() {
			if o.GetMemberId() == target && o.GetOptionId() == option {
				return h.handler.Speak(ctx, &sessionpb.SpeakRequest{
					Session: run, Member: member, Target: &sessionpb.SpeakRequest_MemberId{MemberId: target},
					OptionId: option, Revision: o.GetRevision(),
				})
			}
		}
	}
	require.FailNowf(t, "line not listed", "%s has no %q for %s", member, option, target)

	return nil, nil
}

// TestAcceptance_AWaitingClaimHoldsTheGate is gate slice 4 (reserve before
// dice, rpg-project#924) on the server the api serves. Alice, holding a
// Guidance die, is hailed at the front gate and claims to be a guest: the
// check pauses on her die. While it waits, bob beside her sees the gate's
// claim pending on alice and the greeting open; sending the pending claim is
// FAILED_PRECONDITION; replying from the menu he read before her claim is
// ABORTED; the greeting is spoken; and alice's React settles the claim once.
func TestAcceptance_AWaitingClaimHoldsTheGate(t *testing.T) {
	aliceCtx := auth.WithPlayerID(context.Background(), "player-alice")
	h := newAcceptanceHarnessWithDice(t, standAsideDice{})
	const run = "gate-reserve-run"

	for _, c := range []struct{ id, player string }{{"alice", "player-alice"}, {"bob", "player-bob"}} {
		_, err := h.charRepo.Create(aliceCtx, characterrepo.CreateInput{
			Character: &entities.Character{Data: armedFighter(c.id, c.player)},
		})
		require.NoError(t, err)
	}
	dungeon, err := sessionworld.Compile(madKingRaw(t))
	require.NoError(t, err)
	// Alice a few cells down the road, beyond every carrier's approach, so
	// the walk to the hail is short: the launch and the walk are this test's
	// fixture cost, the reserve is what it is about.
	out := h.launch(t, run, dungeon.Spec, seatAt("alice", 1, 14), seatAt("bob", bobCol, bobRow))
	require.Empty(t, out.Formed, "precondition: nobody fights")
	giveGuidance(t, h, "alice")

	walk(t, h, run, toTheCaptain()[6:], "alice walks up to the gate")
	require.NotNil(t, speakRowOf(t, h, run, "alice", "player-alice").GetAddress(),
		"precondition: the captain hailed her")

	before := captainLine(t, h, run, "bob", "player-bob", guestLine)
	require.True(t, before.GetAvailable(), "precondition: bob may claim to be a guest")
	require.Equal(t, frontGate, before.GetInteractionId())
	require.NotZero(t, before.GetRevision())

	claim, err := sayToCaptain(h, run, "alice", "player-alice", captainLine(t, h, run, "alice", "player-alice", guestLine))
	require.NoError(t, err)
	require.True(t, claim.GetPaused(), "precondition: her Guidance die stops the check to ask")

	// The hold, as bob reads it.
	guest := captainLine(t, h, run, "bob", "player-bob", guestLine)
	require.True(t, guest.GetPending(), "the guest claim waits on alice's roll")
	require.False(t, guest.GetAvailable())
	require.Equal(t, "alice", guest.GetPendingOnMemberId())
	require.Equal(t, frontGate, guest.GetInteractionId())
	require.NotEqual(t, before.GetRevision(), guest.GetRevision(), "the reservation moved the gate's revision")
	greet := captainLine(t, h, run, "bob", "player-bob", greetLine)
	require.True(t, greet.GetAvailable(), "a line that reads no conversation stays open")
	require.False(t, greet.GetPending())
	require.Nil(t, reactRowOf(t, h, run, "bob", "player-bob"), "the question is alice's alone")

	_, err = sayToCaptain(h, run, "bob", "player-bob", guest)
	require.Equal(t, codes.FailedPrecondition, status.Code(err), "a pending line waits on a roll: %v", err)

	_, err = sayToCaptain(h, run, "bob", "player-bob", before)
	require.Equal(t, codes.Aborted, status.Code(err), "a reply from the older menu is stale: %v", err)

	_, err = sayToCaptain(h, run, "bob", "player-bob", greet)
	require.NoError(t, err, "the table is not frozen: the greeting is said")

	// Alice settles once. Her REACT row sits beside a full panel.
	react := reactRowOf(t, h, run, "alice", "player-alice")
	require.NotNil(t, react, "alice is asked about her die")
	_, err = h.handler.React(aliceCtx, &sessionpb.ReactRequest{
		Session: run, Member: "alice", DeclarationId: react.GetId(),
		Choice: sessionpb.ReactChoice_REACT_CHOICE_STRIKE,
	})
	require.NoError(t, err, "her answer settles the claim")
	_, err = h.handler.React(aliceCtx, &sessionpb.ReactRequest{
		Session: run, Member: "alice", DeclarationId: react.GetId(),
		Choice: sessionpb.ReactChoice_REACT_CHOICE_STRIKE,
	})
	require.Equal(t, codes.FailedPrecondition, status.Code(err), "a second answer finds no question: %v", err)
	for _, o := range speakRowOf(t, h, run, "bob", "player-bob").GetSpeakOptions() {
		require.False(t, o.GetPending(), "the hold is gone once the claim settles: %s", o.GetOptionId())
	}
}
