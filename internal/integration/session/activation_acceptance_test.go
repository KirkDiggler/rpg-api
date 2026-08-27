package session_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

// activationOffer finds one ability's current offer in Afford's answer.
//
// By REF rather than by verb, unlike currentDeclarationID: Activate is the
// first verb that compiles more than one declaration, so "the first row for
// this verb" stopped being a question with an answer.
func activationOffer(
	t *testing.T, h *acceptanceHarness, ctx context.Context, member, ref string,
) *sessionpb.Declaration {
	t.Helper()
	out, err := h.handler.Afford(ctx, &sessionpb.AffordRequest{
		Session: "acceptance-run", Member: member,
	})
	require.NoError(t, err)
	for _, d := range out.GetDeclarations() {
		if d.GetVerb() == sessionpb.Verb_VERB_ACTIVATE && d.GetAbility().GetRef() == ref {
			return d
		}
	}
	t.Fatalf("Afford offered no activation for %q", ref)
	return nil
}

func storedConditionRefs(t *testing.T, repo characterrepo.Repository, id string) []string {
	t.Helper()
	got, err := repo.Get(context.Background(), characterrepo.GetInput{ID: id})
	require.NoError(t, err)
	out := make([]string, 0)
	for _, raw := range got.Character.Data.Conditions {
		var envelope struct {
			Ref string `json:"ref"`
		}
		require.NoError(t, json.Unmarshal(raw, &envelope))
		out = append(out, envelope.Ref)
	}
	return out
}

// A PLAYER ACTIVATES SOMETHING, THROUGH THE REAL SERVICE, AND IT IS THERE
// AFTERWARDS.
//
// This is rpg-project#300's proof at this layer, and it is a different claim
// from the toolkit's own: the sheet goes through the ACTUAL repository —
// serialized into Redis and read back out — rather than a fake that hands the
// same struct back. A condition that did not survive its own JSON round trip
// would pass every test in the SDK and fail here.
//
// Dodge rather than Rage because every character carries Dodge and the seam is
// what is being proved, not the ability. It is also the design's own build
// order: the purest case first — self-targeting, no resource, no input beyond
// the actor.
func TestAcceptance_ActivatingDodgeReachesRedisAndSpendsTheAction(t *testing.T) {
	h := newAcceptanceHarness(t)
	ctx := auth.WithPlayerID(context.Background(), "player-alice")

	_, err := h.charRepo.Create(context.Background(), characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter("alice", "player-alice")},
	})
	require.NoError(t, err)

	world := buildThreeRoomTomb(t)
	_, err = h.manager.Manager.StartSession(context.Background(), &sdk.StartSessionInput{
		Session: "acceptance-run", Encounter: "tomb-encounter", World: world,
	})
	require.NoError(t, err)
	_, err = h.manager.Manager.Spawn(context.Background(), &sdk.SpawnInput{
		Session: "acceptance-run", ID: "skel-1", Ref: refs.Monsters.Skeleton().String(),
		Position: at(19, 3),
	})
	require.NoError(t, err)

	_, err = h.handler.Join(ctx, &sessionpb.JoinRequest{
		Session: "acceptance-run", Member: "alice", Position: pbAt(1, 1),
	})
	require.NoError(t, err)

	// Walking into sight of the skeleton starts the fight, which is what puts
	// alice on a turn clock — there are no activations on the world clock.
	_, err = h.handler.Move(ctx, &sessionpb.MoveRequest{
		Session: "acceptance-run", Member: "alice", Path: tombRoute(),
	})
	require.NoError(t, err)

	// -- the offer, rendered exactly as a panel would read it --
	dodge := activationOffer(t, h, ctx, "alice", "dnd5e:combat_abilities:dodge")
	require.True(t, dodge.GetAvailable())
	require.NotEmpty(t, dodge.GetId(), "a compiled activation carries a selector")
	require.Equal(t, "Dodge", dodge.GetAbility().GetName(),
		"the button's label comes from the server, not a client-side table")
	require.Equal(t, sessionpb.Slot_SLOT_ACTION, dodge.GetSlot())
	require.Equal(t, sessionpb.TargetKind_TARGET_KIND_NONE, dodge.GetTargetKind())

	// -- the verb, through the gRPC handler --
	resp, err := h.handler.Activate(ctx, &sessionpb.ActivateRequest{
		Session: "acceptance-run", Member: "alice", DeclarationId: dodge.GetId(),
	})
	require.NoError(t, err)
	require.Contains(t, resp.GetSaved().GetWritten(), "character:alice",
		"the ack names what it persisted, which is the whole reason it is not empty")

	// -- and it is in Redis, not in a double --
	require.Contains(t, storedConditionRefs(t, h.charRepo, "alice"),
		"dnd5e:conditions:dodging")

	// -- and the action is gone, so the panel goes dark with a reason a
	// player can read --
	after := activationOffer(t, h, ctx, "alice", "dnd5e:combat_abilities:dodge")
	require.False(t, after.GetAvailable())
	require.Equal(t, sessionpb.ShortfallReason_SHORTFALL_REASON_NO_BUDGET,
		after.GetWhy().GetReason())
	require.Equal(t, sessionpb.Currency_CURRENCY_ACTION, after.GetWhy().GetCurrency())

	// -- the spent selector is refused rather than replayed --
	_, err = h.handler.Activate(ctx, &sessionpb.ActivateRequest{
		Session: "acceptance-run", Member: "alice", DeclarationId: dodge.GetId(),
	})
	require.Error(t, err, "a selector spent once must not work twice")
}
