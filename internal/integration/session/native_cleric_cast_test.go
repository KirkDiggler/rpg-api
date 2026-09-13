package session_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/resources"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/spells"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

func nativeClericCombatScene(t *testing.T) (*acceptanceHarness, context.Context, string) {
	t.Helper()
	h := newAcceptanceHarnessWith(t, failedSaveDice{}, sdk.Pass{})
	id := createNativeCleric(t, h)
	ctx := auth.WithPlayerID(context.Background(), "cleric-player")
	allyCtx := auth.WithPlayerID(context.Background(), "player-alice")
	_, err := h.charRepo.Create(ctx, characterrepo.CreateInput{Character: &entities.Character{Data: armedFighter("alice", "player-alice")}})
	require.NoError(t, err)
	_, err = h.manager.Manager.StartSession(ctx, &sdk.StartSessionInput{Session: castSessionID, Encounter: "room-encounter", World: buildOpenRoom(t, 12, 6)})
	require.NoError(t, err)
	_, err = h.handler.Join(ctx, &sessionpb.JoinRequest{Session: castSessionID, Member: id, Position: pbAt(2, 0)})
	require.NoError(t, err)
	_, err = h.handler.Join(allyCtx, &sessionpb.JoinRequest{Session: castSessionID, Member: "alice", Position: pbAt(3, 0)})
	require.NoError(t, err)
	worldOffers, err := h.handler.Afford(ctx, &sessionpb.AffordRequest{Session: castSessionID, Member: id})
	require.NoError(t, err)
	for _, offer := range worldOffers.GetDeclarations() {
		require.False(t, offer.GetVerb() == sessionpb.Verb_VERB_CAST && offer.GetAvailable(), "world-clock exploration does not imply a missing cast mapping")
	}
	_, err = h.manager.Manager.Spawn(ctx, &sdk.SpawnInput{Session: castSessionID, ID: "skel-1", Ref: refs.Monsters.Skeleton().String(), Position: at(4, 0)})
	require.NoError(t, err)
	turn, err := h.handler.Turn(ctx, &sessionpb.TurnRequest{Session: castSessionID, Member: id})
	require.NoError(t, err)
	if turn.GetActive() == "alice" {
		_, err = h.handler.EndTurn(allyCtx, &sessionpb.EndTurnRequest{Session: castSessionID, Member: "alice",
			DeclarationId: currentDeclarationID(allyCtx, t, h.handler, castSessionID, "alice", sessionpb.Verb_VERB_END_TURN)})
		require.NoError(t, err)
	}
	turn, err = h.handler.Turn(ctx, &sessionpb.TurnRequest{Session: castSessionID, Member: id})
	require.NoError(t, err)
	require.Equal(t, id, turn.GetActive())
	return h, ctx, id
}

func TestAcceptance_NativeClericWorldToCombatCasts(t *testing.T) {
	for _, spell := range []spells.Spell{spells.Bless, spells.CureWounds, spells.HealingWord} {
		t.Run(spell, func(t *testing.T) {
			h, ctx, id := nativeClericCombatScene(t)
			if spell != spells.Bless {
				ally, err := h.charRepo.Get(ctx, characterrepo.GetInput{ID: "alice"})
				require.NoError(t, err)
				ally.Character.Data.HitPoints = 1
				_, err = h.charRepo.Update(ctx, characterrepo.UpdateInput{Character: ally.Character})
				require.NoError(t, err)
			}
			row := castRowFor(ctx, t, h, id, spell)
			require.True(t, row.GetAvailable(), "%s", row.GetWhy())
			targets := []string{"alice"}
			if spell == spells.Bless {
				targets = []string{"alice", id, "skel-1"}
			}
			live := watchCast(ctx, t, h, id, func() {
				_, err := h.handler.Cast(ctx, &sessionpb.CastRequest{Session: castSessionID, Member: id, DeclarationId: row.GetId(), Targets: targets})
				require.NoError(t, err)
			})
			stored, err := h.charRepo.Get(ctx, characterrepo.GetInput{ID: id})
			require.NoError(t, err)
			require.Equal(t, 1, stored.Character.Data.Resources[resources.SpellSlotLevel1].Current)
			if spell == spells.HealingWord {
				require.Zero(t, stored.Character.Data.ActionEconomy.BonusActionsRemaining)
			} else {
				require.Zero(t, stored.Character.Data.ActionEconomy.ActionsRemaining)
			}
			var affected []string
			for _, event := range live {
				if condition := event.GetActivationResult().GetConditionApplied(); condition != nil {
					require.Equal(t, id, condition.GetSourceId())
					affected = append(affected, condition.GetTarget())
				}
				if heal := event.GetActivationResult().GetHealingApplied(); heal != nil {
					require.Positive(t, heal.GetAmount())
					require.Equal(t, row.GetSpell().GetRef(), heal.GetSourceRef())
					affected = append(affected, heal.GetTarget())
				}
			}
			require.Equal(t, targets, affected)
			require.NotEmpty(t, live)
			reopenClericHost(t, h, sdk.StaleTargetRefuse)
			story, err := h.handler.GetStory(ctx, &sessionpb.GetStoryRequest{Session: castSessionID, Member: id, FromSeq: live[0].GetSeq()})
			require.NoError(t, err)
			require.Len(t, story.GetEntries(), len(live))
			for i, event := range live {
				require.True(t, proto.Equal(event, story.Entries[i]))
			}
		})
	}
}
