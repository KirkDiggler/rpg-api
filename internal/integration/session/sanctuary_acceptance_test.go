package session_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	storypb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/resources"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/spells"
)

func TestAcceptance_Sanctuary(t *testing.T) {
	for _, cast := range []bool{false, true} {
		name := "attack"
		if cast {
			name = "hostile_cast"
		}
		t.Run(name, func(t *testing.T) {
			h, ctx, id := nativeClericCombatScene(t)
			allyCtx := auth.WithPlayerID(context.Background(), "player-alice")
			target := id
			if cast {
				// Move adjacent to the enemy, then ward it through the ordinary Cast door.
				_, err := h.handler.Move(ctx, &sessionpb.MoveRequest{Session: castSessionID, Member: id,
					DeclarationId: currentDeclarationID(ctx, t, h.handler, castSessionID, id, sessionpb.Verb_VERB_MOVE),
					Path:          []*sessionpb.Position{pbAt(2, 1), pbAt(3, 1), pbAt(4, 1)}})
				require.NoError(t, err)
				target = "skel-1"
			}
			row := castRowFor(ctx, t, h, id, spells.Sanctuary)
			require.True(t, row.GetAvailable(), "%s", row.GetWhy())
			_, err := h.handler.Cast(ctx, &sessionpb.CastRequest{Session: castSessionID, Member: id, DeclarationId: row.GetId(), Targets: []string{target}})
			require.NoError(t, err)
			stored, err := h.charRepo.Get(ctx, characterrepo.GetInput{ID: id})
			require.NoError(t, err)
			require.Equal(t, 1, stored.Character.Data.Resources[resources.SpellSlotLevel1].Current)
			require.Zero(t, stored.Character.Data.ActionEconomy.BonusActionsRemaining)
			_, err = h.handler.EndTurn(ctx, &sessionpb.EndTurnRequest{Session: castSessionID, Member: id,
				DeclarationId: currentDeclarationID(ctx, t, h.handler, castSessionID, id, sessionpb.Verb_VERB_END_TURN)})
			require.NoError(t, err)
			actorCtx, actor := allyCtx, "alice"
			if cast {
				_, err = h.handler.EndTurn(allyCtx, &sessionpb.EndTurnRequest{Session: castSessionID, Member: "alice",
					DeclarationId: currentDeclarationID(allyCtx, t, h.handler, castSessionID, "alice", sessionpb.Verb_VERB_END_TURN)})
				require.NoError(t, err)
				actorCtx, actor = ctx, id
			}
			live := watchCastActs(actorCtx, t, h, actor, func() {
				if cast {
					offer := castRowFor(ctx, t, h, id, spells.SacredFlame)
					out, castErr := h.handler.Cast(ctx, &sessionpb.CastRequest{Session: castSessionID, Member: id, DeclarationId: offer.GetId(), Targets: []string{"skel-1"}})
					require.NoError(t, castErr)
					require.Equal(t, []string{"skel-1"}, out.GetWardedTargets())
				} else {
					out, attackErr := h.handler.Attack(allyCtx, &sessionpb.AttackRequest{Session: castSessionID, Attacker: "alice", Target: id,
						DeclarationId: currentDeclarationID(allyCtx, t, h.handler, castSessionID, "alice", sessionpb.Verb_VERB_ATTACK)})
					require.NoError(t, attackErr)
					require.True(t, out.GetWarded())
					require.Equal(t, id, out.GetWardedBy())
					require.Zero(t, out.GetRoll())
					require.Zero(t, out.GetDamage())
				}
			})
			warded := 0
			for _, act := range live {
				if b := act.GetAttack(); b != nil && b.GetOutcome() == storypb.AttackOutcome_ATTACK_OUTCOME_WARDED {
					warded++
					require.Equal(t, id, b.GetWard().GetSource().GetId())
					require.Equal(t, "alice", b.GetBy().GetId())
					require.Equal(t, id, b.GetTo().GetId())
					require.NotNil(t, b.GetWard().GetRoll())
				}
				for _, result := range act.GetCast().GetResults() {
					if result.GetOutcome() == storypb.CastOutcome_CAST_OUTCOME_WARDED {
						warded++
						require.Equal(t, id, result.GetSource().GetId())
						require.Equal(t, "skel-1", result.GetTarget().GetId())
						require.NotNil(t, result.GetSave())
					}
				}
			}
			require.Equal(t, 1, warded)
			paid, err := h.charRepo.Get(actorCtx, characterrepo.GetInput{ID: actor})
			require.NoError(t, err)
			require.Zero(t, paid.Character.Data.ActionEconomy.ActionsRemaining, "a warded attempt still pays its action")
			if cast {
				require.Equal(t, 1, paid.Character.Data.Resources[resources.SpellSlotLevel1].Current, "the warded cantrip spends no extra spell slot")
			} else {
				protected, getErr := h.charRepo.Get(ctx, characterrepo.GetInput{ID: id})
				require.NoError(t, getErr)
				require.Equal(t, protected.Character.Data.MaxHitPoints, protected.Character.Data.HitPoints)
			}
			reopenClericHost(t, h, sdk.StaleTargetRefuse)
			requireLiveIsStory(actorCtx, t, h, castSessionID, actor, live)
		})
	}
}
