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
)

// The gate's refusal (the-mad-king.yaml, rpg-project#1032): a line with no
// check that arrests, so an arrest can be played on purpose.
const (
	notTellingLine  = "not-telling"
	theRefusalReply = "Then you’ll tell it to the cells."
)

// atTheCaptain launches the castle for alice alone, seated beside the
// captain (as the recognition tests seat her), every d20 at face.
func atTheCaptain(t *testing.T, run string, face int32) (*acceptanceHarness, *gateDice) {
	t.Helper()
	dice := &gateDice{}
	dice.face.Store(face)
	h := newAcceptanceHarnessWithDice(t, dice)
	_, err := h.charRepo.Create(auth.WithPlayerID(context.Background(), "player-alice"), characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter("alice", "player-alice")},
	})
	require.NoError(t, err)
	dungeon, err := sessionworld.Compile(madKingRaw(t))
	require.NoError(t, err)
	out := h.launch(t, run, dungeon.Spec, seatAt("alice", -1, 9))
	require.Empty(t, out.Formed, "precondition: nobody fights")

	return h, dice
}

// gateSpeakers is every carrier of the-gate.
var gateSpeakers = []string{gatePosted, gateLoop, gateCaptain}

// TestAcceptance_RefusingToAnswerArrests: at a fresh gate, and after one
// missed claim, saying "I’m not telling you anything." to any gate speaker
// arrests alice as a second missed claim does: the gate is arrested, she is
// taken and held, the speaker answers with the refusal's reply, and
// `come-quietly` and `never-alive` are offered.
func TestAcceptance_RefusingToAnswerArrests(t *testing.T) {
	for _, speaker := range gateSpeakers {
		for _, warned := range []bool{false, true} {
			name := speaker + "/fresh"
			if warned {
				name = speaker + "/warned"
			}
			t.Run(name, func(t *testing.T) {
				run := "gate-refuse-" + speaker
				if warned {
					run += "-warned"
				}
				h, _ := atTheCaptain(t, run, 1)
				ctx := auth.WithPlayerID(context.Background(), "player-alice")
				if warned {
					_, err := sayTo(t, h, run, "alice", "player-alice", gateCaptain, guestLine)
					require.NoError(t, err, "alice's guest claim, which misses")
					require.Equal(t, "warned-guest", frontGateState(t, h, run), "precondition: one warning")
				} else {
					require.Equal(t, "fresh", frontGateState(t, h, run), "precondition: a fresh gate")
				}
				require.Nil(t, frontGateArrest(t, h, run), "precondition: nobody is arrested")
				require.False(t, offers(t, h, run, "alice", "player-alice", speaker, comeQuietlyLine), "precondition: no going quietly yet")

				acts := watchActs(ctx, t, h, run, "alice", func() {
					_, err := sayTo(t, h, run, "alice", "player-alice", speaker, notTellingLine)
					require.NoError(t, err, "alice refuses to answer %s", speaker)
				})
				requireNothingRefused(t, acts, "alice")
				require.Contains(t, linesOf(acts, speaker), theRefusalReply, "%s answers the refusal", speaker)

				require.Equal(t, "arrested", frontGateState(t, h, run), "the gate arrests")
				arrest := frontGateArrest(t, h, run)
				require.NotNil(t, arrest, "the record holds the arrest")
				taken := make([]string, 0, len(arrest.Taken))
				for _, m := range arrest.Taken {
					taken = append(taken, string(m.ID))
				}
				require.Equal(t, []string{"alice"}, taken, "she is taken")

				_, err := h.handler.Move(ctx, &sessionpb.MoveRequest{Session: run, Member: "alice", Path: []*sessionpb.Position{pbAt(-1, 10)}})
				require.Equal(t, codes.FailedPrecondition, status.Code(err), "a held walk is refused: %v", err)
				require.Equal(t, heldYouSentence, status.Convert(err).Message(), "the guards' sentence alone")

				require.True(t, offers(t, h, run, "alice", "player-alice", speaker, comeQuietlyLine), "%s offers going quietly", speaker)
				require.True(t, offers(t, h, run, "alice", "player-alice", speaker, neverAliveLine), "%s offers the refusal by force", speaker)
				for _, s := range gateSpeakers {
					require.False(t, offers(t, h, run, "alice", "player-alice", s, notTellingLine), "arrested: %s no longer offers the refusal", s)
				}
			})
		}
	}
}

// TestAcceptance_TheRefusalIsOfferedOnlyBeforeTheGateDecides: every gate
// speaker offers the refusal at a fresh gate, and none does once the guest
// claim has admitted the party.
func TestAcceptance_TheRefusalIsOfferedOnlyBeforeTheGateDecides(t *testing.T) {
	const run = "gate-refuse-admitted"
	h, _ := atTheCaptain(t, run, 20)
	for _, s := range gateSpeakers {
		require.True(t, offers(t, h, run, "alice", "player-alice", s, notTellingLine), "fresh: %s offers the refusal", s)
	}
	_, err := sayTo(t, h, run, "alice", "player-alice", gateCaptain, guestLine)
	require.NoError(t, err, "every d20 a 20: the guest claim wins")
	require.Equal(t, "admitted", frontGateState(t, h, run), "precondition: admitted")
	for _, s := range gateSpeakers {
		require.False(t, offers(t, h, run, "alice", "player-alice", s, notTellingLine), "admitted: %s no longer offers the refusal", s)
	}
}
