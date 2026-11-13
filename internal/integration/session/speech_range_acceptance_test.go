package session_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	storypb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
)

// admittedTopic is what a gate guard holds about a member he lets through
// (the-mad-king.yaml, rpg-project#928).
const admittedTopic = "admitted"

// TestAcceptance_AVouchFromJustInsideTheGateCarries is the operator's case
// on rpg-project#953 at each their own, the friend WALKED: alice is admitted,
// walks into the yard, and comes back to one step inside the open gate; bob,
// the straggler, comes up beside it. Her ordinary vouch to gate-1 now carries
// a short range through the open doorway, so gate-2 in the gatehouse catches
// it and recognizes bob too, and bob's walk through the gate succeeds.
func TestAcceptance_AVouchFromJustInsideTheGateCarries(t *testing.T) {
	const run = "vouch-inside-run"
	dice := &gateDice{}
	dice.face.Store(20)
	h := admittedWithBobOnTheRoadFrom(t, run, dice, eachTheirOwn(t, madKingRaw(t)))
	walk(t, h, run, []*sessionpb.Position{pbAt(-1, 8), besideTheGate(), pastTheGate(), pbAt(0, 5)}, "alice into the yard")
	bobUpToTheGate(t, h, run)
	back := walkAs(t, h, run, "alice", "player-alice", []*sessionpb.Position{pastTheGate()})
	require.Equal(t, sessionpb.MovementStatus_MOVEMENT_STATUS_COMPLETED, back.GetStatus(), "alice comes back to just inside the gate: %s", back.GetStopReason())
	require.False(t, holds(t, h, run, gateLoop, "bob", admittedTopic), "precondition: gate-2 does not know bob")

	_, err := sayTo(t, h, run, "alice", "player-alice", gatePosted, vouchLine)
	require.NoError(t, err, "alice vouches for bob from just inside the gate")
	require.True(t, holds(t, h, run, gatePosted, "bob", admittedTopic), "gate-1 recognizes bob")
	require.True(t, holds(t, h, run, gateLoop, "bob", admittedTopic), "gate-2, in the gatehouse, caught the vouch and recognizes him too")

	aside := walkAs(t, h, run, "alice", "player-alice", []*sessionpb.Position{pbAt(0, 5)})
	require.Equal(t, sessionpb.MovementStatus_MOVEMENT_STATUS_COMPLETED, aside.GetStatus(), "alice steps aside: %s", aside.GetStopReason())
	through := walkAs(t, h, run, "bob", "player-bob", []*sessionpb.Position{pastTheGate()})
	require.Equal(t, sessionpb.MovementStatus_MOVEMENT_STATUS_COMPLETED, through.GetStatus(), "vouched for, bob walks through: %s", through.GetStopReason())
}

// requireNothingRefused requires no act in acts to have gone out refused or
// undecoded: every beat the stream was sent, this build can spell.
func requireNothingRefused(t *testing.T, acts []*storypb.Act, whose string) {
	t.Helper()
	for _, a := range acts {
		require.False(t, a.GetRefused(), "%s: act %s (%s) went out refused: %s", whose, a.GetId(), a.GetAct(), a.GetRefusal())
	}
}

// TestAcceptance_TwoFailedClaimsArrestWithNoActOnTheWire is arrest slice one
// (encounter v0.207.0) through the api's doors: alice, told of the feast by
// the stranger and hailed at the gate,
// misses the troupe claim and then the guest claim, every d20 a 1, and the
// gate arrests. The `arrested` beat is the record's: neither her stream nor
// bob's carries a refused or undecoded act for it, live or on catch-up, and
// the run still loads and saves after.
func TestAcceptance_TwoFailedClaimsArrestWithNoActOnTheWire(t *testing.T) {
	const run = "arrest-run"
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
	// Beside the stranger, who tells of the feast (the troupe claim needs it,
	// and a missed line still teaches it); bob further down the road.
	out := h.launch(t, run, dungeon.Spec, seatAt("alice", 6, 15), seatAt("bob", 8, 16))
	require.Empty(t, out.Formed, "precondition: nobody fights")
	_, err = sayTo(t, h, run, "alice", "player-alice", "pedlar", pedlarOptionID)
	require.NoError(t, err, "the stranger tells of the feast")
	walk(t, h, run, toTheCaptain(), "alice walks up to the captain")

	var aliceLive []*storypb.Act
	bobLive := watchActs(bobCtx, t, h, run, "bob", func() {
		aliceLive = watchActs(aliceCtx, t, h, run, "alice", func() {
			for _, claim := range []string{troupeOptionID, guestLine} {
				_, sayErr := sayTo(t, h, run, "alice", "player-alice", gateCaptain, claim)
				require.NoError(t, sayErr, "alice's %s claim, which misses", claim)
			}
		})
	})
	requireNothingRefused(t, aliceLive, "alice, live")
	requireNothingRefused(t, bobLive, "bob, live")
	requireNothingRefused(t, storyOf(aliceCtx, t, h.story, run, "alice"), "alice, on catch-up")
	requireNothingRefused(t, storyOf(bobCtx, t, h.story, run, "bob"), "bob, on catch-up")
	arrest := frontGateArrest(t, h, run)
	require.NotNil(t, arrest, "precondition: the gate arrested")
	taken := make([]string, 0, len(arrest.Taken))
	for _, m := range arrest.Taken {
		taken = append(taken, string(m.ID))
	}
	require.Contains(t, taken, "alice", "precondition: alice is taken")

	// The run loads and saves after the arrest.
	_, err = h.handler.Afford(bobCtx, &sessionpb.AffordRequest{Session: run, Member: "bob"})
	require.NoError(t, err, "the run loads")
	moved, err := h.handler.Move(bobCtx, &sessionpb.MoveRequest{Session: run, Member: "bob", Path: []*sessionpb.Position{pbAt(7, 16)}})
	require.NoError(t, err, "and saves")
	require.NotEmpty(t, moved.GetSteps())
}

// TestAcceptance_StatusReadsThePartySharing is rpg-project#935's indicator
// read: GetStatus carries the run's sharing dial -- OWN for a castle marked
// own, VIEW for one marked view and for the unmarked castle (the default).
func TestAcceptance_StatusReadsThePartySharing(t *testing.T) {
	ctx := auth.WithPlayerID(context.Background(), "player-alice")
	for name, c := range map[string]struct {
		source func(t *testing.T) []byte
		want   sessionpb.PartySharing
	}{
		"own":      {func(t *testing.T) []byte { return eachTheirOwn(t, madKingRaw(t)) }, sessionpb.PartySharing_PARTY_SHARING_OWN},
		"view":     {func(t *testing.T) []byte { return markedView(t, madKingRaw(t)) }, sessionpb.PartySharing_PARTY_SHARING_VIEW},
		"unmarked": {madKingRaw, sessionpb.PartySharing_PARTY_SHARING_VIEW},
	} {
		t.Run(name, func(t *testing.T) {
			h := newAcceptanceHarness(t)
			_, err := h.charRepo.Create(ctx, characterrepo.CreateInput{
				Character: &entities.Character{Data: armedFighter("alice", "player-alice")},
			})
			require.NoError(t, err)
			dungeon, err := sessionworld.Compile(c.source(t))
			require.NoError(t, err)
			h.launch(t, "status-run", dungeon.Spec, seatAt("alice", 6, 15))
			resp, err := h.handler.GetStatus(ctx, &sessionpb.GetStatusRequest{Session: "status-run"})
			require.NoError(t, err)
			require.Equal(t, c.want, resp.GetPartySharing())
		})
	}
}

// markedView is source marked `party: { sharing: view }`.
func markedView(t *testing.T, source []byte) []byte {
	t.Helper()
	require.Equal(t, 1, bytes.Count(source, []byte("\nroom:")), "precondition: one root room key")
	return bytes.Replace(source, []byte("\nroom:"), []byte("\nparty:\n  sharing: view\nroom:"), 1)
}

// frontGateArrest is the front gate's arrest record as the stored world
// holds it, nil when the gate has arrested nobody.
func frontGateArrest(t *testing.T, h *acceptanceHarness, run string) *tkencounter.ArrestRecord {
	t.Helper()
	world, err := sessionorch.NewEncounterRepository(h.redis, 0).GetEncounter(context.Background(), run)
	require.NoError(t, err)
	for _, in := range world.Interactions {
		if in.ID == frontGate {
			return in.Arrest
		}
	}
	require.FailNow(t, "the record holds no front gate")
	return nil
}
