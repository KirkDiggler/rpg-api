package session_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	coreResources "github.com/KirkDiggler/rpg-toolkit/core/resources"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/resources"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/testsupport/levelfixture"
)

const (
	rageOffer       = "dnd5e:features:rage"
	secondWindOffer = "dnd5e:features:second_wind"
	ragingCondition = "dnd5e:conditions:raging"
)

// A barbarian who is already raging asks for Rage again: the one gate refuses
// it, her charges and her bonus action stay where the first Rage left them,
// and she holds exactly one raging status.
func TestAcceptance_ASecondRageWhileRagingIsRefusedAndSpendsNothing(t *testing.T) {
	h, ctx := inAFightWith(t, ragingBarbarian("alice", "player-alice"))

	rage := activationsFor(ctx, t, h, "alice")[rageOffer]
	_, err := h.handler.Activate(ctx, &sessionpb.ActivateRequest{
		Session: "acceptance-run", Member: "alice", DeclarationId: rage.GetId(),
	})
	require.NoError(t, err)

	before := storedSheetOf(t, h.charRepo, "alice")
	require.Equal(t, 1, before.Resources[resources.RageCharges].Current)

	again := activationsFor(ctx, t, h, "alice")[rageOffer]
	_, err = h.handler.Activate(ctx, &sessionpb.ActivateRequest{
		Session: "acceptance-run", Member: "alice", DeclarationId: again.GetId(),
	})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok, "a refusal must be a status error")
	require.Equal(t, codes.Aborted, st.Code())
	require.Equal(t, "activate: declaration is stale", st.Message())

	after := storedSheetOf(t, h.charRepo, "alice")
	require.Equal(t, 1, after.Resources[resources.RageCharges].Current, "no charge is spent by the refusal")
	require.Equal(t, 0, after.ActionEconomy.BonusActionsRemaining)
	require.Equal(t, 1, after.ActionEconomy.ActionsRemaining)

	raging := 0
	for _, ref := range conditionRefsOf(t, h.charRepo, "alice") {
		if ref == ragingCondition {
			raging++
		}
	}
	require.Equal(t, 1, raging, "she holds exactly one raging status")
}

// An ability that lands on its user, sent with a target named, is a malformed
// request: INVALID_ARGUMENT, and nothing is spent.
func TestAcceptance_ASelfAbilityNamedWithATargetIsMalformedAndSpendsNothing(t *testing.T) {
	fighter := armedFighter("alice", "player-alice")
	fighter.Level = 1
	fighter.Levels = levelfixture.Synthetic(classes.Fighter, 1)
	fighter.Features = []json.RawMessage{json.RawMessage(
		`{"ref":{"module":"dnd5e","type":"features","id":"second_wind"},` +
			`"id":"second-wind-1","name":"Second Wind","level":1,"character_id":"alice"}`)}
	fighter.Resources = map[coreResources.ResourceKey]tkcharacter.RecoverableResourceData{
		resources.SecondWind: {Current: 1, Maximum: 1, ResetType: coreResources.ResetShortRest},
	}

	cases := []struct {
		name  string
		sheet *tkcharacter.Data
		offer string
		key   coreResources.ResourceKey
		want  int
	}{
		{"rage", ragingBarbarian("alice", "player-alice"), rageOffer, resources.RageCharges, 2},
		{"second wind", fighter, secondWindOffer, resources.SecondWind, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, ctx := inAFightWith(t, tc.sheet)
			offer := activationsFor(ctx, t, h, "alice")[tc.offer]
			require.NotNil(t, offer)
			before := storedSheetOf(t, h.charRepo, "alice").ActionEconomy

			_, err := h.handler.Activate(ctx, &sessionpb.ActivateRequest{
				Session: "acceptance-run", Member: "alice", DeclarationId: offer.GetId(), Target: "skel-1",
			})
			require.Error(t, err)
			st, ok := status.FromError(err)
			require.True(t, ok, "a refusal must be a status error")
			require.Equal(t, codes.InvalidArgument, st.Code())
			require.Contains(t, st.Message(), "names nobody else")

			after := storedSheetOf(t, h.charRepo, "alice")
			require.Equal(t, tc.want, after.Resources[tc.key].Current, "nothing is charged")
			require.Equal(t, before, after.ActionEconomy, "no action or bonus action is spent")
			require.Empty(t, ragingStatusesOf(t, h), "no status landed")
		})
	}
}

func ragingStatusesOf(t *testing.T, h *acceptanceHarness) []string {
	t.Helper()
	var out []string
	for _, r := range conditionRefsOf(t, h.charRepo, "alice") {
		if r == ragingCondition {
			out = append(out, r)
		}
	}
	return out
}
