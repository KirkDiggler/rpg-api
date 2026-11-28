package session_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/status"

	coreResources "github.com/KirkDiggler/rpg-toolkit/core/resources"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/features"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/resources"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	storypb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	"github.com/KirkDiggler/rpg-api/internal/testsupport/levelfixture"
)

// THE FIGHTER'S OWN ACTION SURGE, OVER THE WIRE (rpg-project#1131, slice 6d).
//
// A level-2 fighter holds the feature in its saved form (who it is, and
// nothing else: no count) and her uses in her shared pool, resources.ActionSurge
// at 1 of 1. She is offered it as a free action, uses it through the session's
// activation, is sent the act, and strikes twice in the turn. No test here
// builds a feature carrying `uses` or `max_uses`: a fighter saved before the
// change is not played (operator ruling 2026-11-28).

const (
	surgeSession = "acceptance-run"
	surgeOffer   = "dnd5e:features:action_surge"
)

// surgeDice lets every d20 roll 10 and every d8 roll 1: the fight forms with
// alice acting first (as the Second Wind scene's tied initiative does), her
// blows hit the skeleton's armor 13 (10 + 5) and each does 4, so two blows
// leave it standing and the third attack is refused only for want of an action.
type surgeDice struct{}

func (surgeDice) Roll(_ context.Context, size int) (int, error) {
	switch size {
	case 20:
		return 10, nil
	case 8:
		return 1, nil
	}
	return size, nil
}

// levelTwoFighterWithSurge is armedFighter at level 2, holding Action Surge in
// the saved form and her pool full.
func levelTwoFighterWithSurge(t *testing.T) *tkcharacter.Data {
	t.Helper()
	fighter := armedFighter("alice", "player-alice")
	fighter.Level = 2
	fighter.Levels = levelfixture.Synthetic(classes.Fighter, 2)
	surge, err := json.Marshal(features.ActionSurgeData{
		Ref: refs.Features.ActionSurge(), ID: "action-surge", Name: "Action Surge",
		CharacterID: "alice",
	})
	require.NoError(t, err)
	fighter.Features = []json.RawMessage{surge}
	fighter.Resources = map[coreResources.ResourceKey]tkcharacter.RecoverableResourceData{
		resources.ActionSurge: {Current: 1, Maximum: 1, ResetType: coreResources.ResetShortRest},
	}
	return fighter
}

func surgeFight(t *testing.T) (*acceptanceHarness, context.Context) {
	t.Helper()
	h, ctx := inAFightWithDice(t, levelTwoFighterWithSurge(t), surgeDice{})
	turn, err := h.handler.Turn(ctx, &sessionpb.TurnRequest{Session: surgeSession, Member: "alice"})
	require.NoError(t, err)
	require.Equal(t, "alice", turn.GetActive(), "geometry gate: it is her turn")
	return h, ctx
}

func activateSurge(ctx context.Context, h *acceptanceHarness, id string) error {
	_, err := h.handler.Activate(ctx, &sessionpb.ActivateRequest{
		Session: surgeSession, Member: "alice", DeclarationId: id,
	})
	return err
}

// Test 1. She is offered it.
func TestAcceptance_TheFighterIsOfferedHerActionSurgeAsAFreeAction(t *testing.T) {
	h, ctx := surgeFight(t)

	offer, ok := activationsFor(ctx, t, h, "alice")[surgeOffer]
	require.True(t, ok, "a level-2 fighter holding Action Surge must be offered it")
	require.Equal(t, "Action Surge", offer.GetAbility().GetName())
	require.True(t, offer.GetAvailable(), "her pool has its one use")
	require.Nil(t, offer.GetWhy(), "available, so nothing ran out")
	require.NotEmpty(t, offer.GetId(), "it carries a selector")
	require.Equal(t, sessionpb.Slot_SLOT_NONE, offer.GetSlot(),
		"a free action draws none of her turn's action, bonus action or reaction")
	t.Logf("wire: offer{ref %q, name %q, slot %s, available %v}",
		offer.GetAbility().GetRef(), offer.GetAbility().GetName(), offer.GetSlot(), offer.GetAvailable())
}

// Test 2. She uses it.
func TestAcceptance_TheFighterUsesHerActionSurgeAndTheWireCarriesIt(t *testing.T) {
	h, ctx := surgeFight(t)
	offer := activationsFor(ctx, t, h, "alice")[surgeOffer]
	require.NotNil(t, offer)

	live := watchActs(ctx, t, h, surgeSession, "alice", func() {
		require.NoError(t, activateSurge(ctx, h, offer.GetId()))
	})

	require.Len(t, live, 1, "one act: her activation, nothing else")
	act := live[0]
	activate := act.GetActivate()
	require.NotNil(t, activate)
	require.Equal(t, storypb.Family_FAMILY_ACT, act.GetFamily())
	require.Equal(t, "alice", activate.GetBy().GetId())
	require.Equal(t, surgeOffer, activate.GetAbility().GetRef())
	require.Equal(t, "Action Surge", activate.GetAbility().GetName())
	require.Nil(t, activate.GetTarget(), "it lands on its user")
	require.Len(t, activate.GetResults(), 1)
	granted := activate.GetResults()[0]
	require.Equal(t, "capacity-granted", granted.GetKind())
	require.Equal(t, "1 action", granted.GetDescription())
	t.Logf("wire: Act{family %s, activate{by %q, ability{ref %q, name %q}, results[{kind %q, description %q}]}}",
		act.GetFamily(), activate.GetBy().GetId(), activate.GetAbility().GetRef(), activate.GetAbility().GetName(),
		granted.GetKind(), granted.GetDescription())
	requireLiveIsStory(ctx, t, h, surgeSession, "alice", live)

	stored := storedSheetOf(t, h.charRepo, "alice")
	require.Equal(t, tkcharacter.RecoverableResourceData{
		Current: 0, Maximum: 1, ResetType: coreResources.ResetShortRest,
	}, stored.Resources[resources.ActionSurge], "one use spent from her shared pool: 0 of 1")
	require.Equal(t, 2, stored.ActionEconomy.ActionsRemaining, "one more action")
}

// Test 3. Two strikes in the turn, and not a third.
func TestAcceptance_WithTheSecondActionTheFighterStrikesTwiceInATurn(t *testing.T) {
	h, ctx := surgeFight(t)
	offer := activationsFor(ctx, t, h, "alice")[surgeOffer]
	require.NotNil(t, offer)
	require.NoError(t, activateSurge(ctx, h, offer.GetId()))

	strike := func() error {
		_, err := h.handler.Attack(ctx, &sessionpb.AttackRequest{
			Session: surgeSession, Attacker: "alice", Target: "skel-1",
			DeclarationId: currentDeclarationID(ctx, t, h.handler, surgeSession, "alice", sessionpb.Verb_VERB_ATTACK),
		})
		return err
	}

	live := watchActs(ctx, t, h, surgeSession, "alice", func() {
		require.NoError(t, strike())
		require.Equal(t, 1, storedSheetOf(t, h.charRepo, "alice").ActionEconomy.ActionsRemaining,
			"one action left after the first blow")
		require.NoError(t, strike())
	})
	require.Len(t, strikesBy(live, "alice"), 2, "both strikes cross the wire")
	require.Zero(t, storedSheetOf(t, h.charRepo, "alice").ActionEconomy.ActionsRemaining, "both actions spent")
	requireLiveIsStory(ctx, t, h, surgeSession, "alice", live)

	// The third has no action to spend. Its row is still offered, dark, and
	// its selector is refused.
	var third *sessionpb.Declaration
	afford, err := h.handler.Afford(ctx, &sessionpb.AffordRequest{Session: surgeSession, Member: "alice"})
	require.NoError(t, err)
	for _, d := range afford.GetDeclarations() {
		if d.GetVerb() == sessionpb.Verb_VERB_ATTACK {
			third = d
		}
	}
	require.NotNil(t, third, "the attack row is still offered")
	require.False(t, third.GetAvailable(), "with no action left")
	_, err = h.handler.Attack(ctx, &sessionpb.AttackRequest{
		Session: surgeSession, Attacker: "alice", Target: "skel-1", DeclarationId: third.GetId(),
	})
	require.Error(t, err, "a third attack is refused")
	st, _ := status.FromError(err)
	t.Logf("wire: third attack refused with code %s, message %q", st.Code(), st.Message())
}

// Test 4. Once.
func TestAcceptance_WithTheUseSpentTheFightersActionSurgeIsRefused(t *testing.T) {
	h, ctx := surgeFight(t)
	offer := activationsFor(ctx, t, h, "alice")[surgeOffer]
	require.NotNil(t, offer)
	require.NoError(t, activateSurge(ctx, h, offer.GetId()))

	spent := activationsFor(ctx, t, h, "alice")[surgeOffer]
	require.NotNil(t, spent, "the row stays on her panel")
	require.False(t, spent.GetAvailable(), "its one use is spent")
	require.NotNil(t, spent.GetWhy())
	t.Logf("wire: spent offer{available %v, reason %s, currency %s, text %q}",
		spent.GetAvailable(), spent.GetWhy().GetReason(), spent.GetWhy().GetCurrency(), spent.GetWhy().GetText())

	before := storedSheetOf(t, h.charRepo, "alice")
	err := activateSurge(ctx, h, offer.GetId())
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok, "a refusal is a status error")
	t.Logf("wire: second activation refused with code %s, message %q", st.Code(), st.Message())

	after := storedSheetOf(t, h.charRepo, "alice")
	require.Equal(t, before.Resources, after.Resources, "nothing more is spent")
	require.Equal(t, before.ActionEconomy, after.ActionEconomy, "no action is given")
	require.Zero(t, after.Resources[resources.ActionSurge].Current)
}
