package session_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/KirkDiggler/rpg-toolkit/core"

	"github.com/KirkDiggler/rpg-api/internal/auth"

	"github.com/stretchr/testify/require"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/resources"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/spells"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	storypb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	sessionhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

type guidingBoltDice struct{ attack int }

func (d guidingBoltDice) Roll(_ context.Context, size int) (int, error) {
	if size == 20 {
		return d.attack, nil
	}
	return 1, nil
}

func TestAcceptance_GuidingBoltNativeCastAndReplay(t *testing.T) {
	for _, hit := range []bool{false, true} {
		name := "miss"
		roll := 1
		if hit {
			name = "hit"
			roll = 18
		}
		t.Run(name, func(t *testing.T) {
			h, ctx, id := nativeClericCombatScene(t, spells.GuidingBolt)
			useGuidingBoltDice(t, h, roll)
			row := castRowFor(ctx, t, h, id, spells.GuidingBolt)
			require.True(t, row.GetAvailable(), "%s", row.GetWhy())
			live := watchCastActs(ctx, t, h, id, func() {
				_, castErr := h.handler.Cast(ctx, &sessionpb.CastRequest{Session: castSessionID, Member: id, DeclarationId: row.GetId(), Targets: []string{"skel-1"}})
				require.NoError(t, castErr)
			})
			stored, err := h.charRepo.Get(ctx, characterrepo.GetInput{ID: id})
			require.NoError(t, err)
			require.Zero(t, stored.Character.Data.ActionEconomy.ActionsRemaining)
			require.Equal(t, 1, stored.Character.Data.Resources[resources.SpellSlotLevel1].Current)
			attacks, lights := 0, 0
			for _, act := range live {
				if b := act.GetAttack(); b != nil {
					attacks++
					require.Equal(t, refs.Spells.GuidingBolt().String(), b.GetAttack().GetRef())
					require.Equal(t, "skel-1", b.GetTo().GetId())
					require.NotNil(t, b.GetRoll())
					require.NotEmpty(t, b.GetPresentationId())
					if hit {
						require.Equal(t, storypb.AttackOutcome_ATTACK_OUTCOME_HIT, b.GetOutcome())
						require.Equal(t, int32(4), b.GetDamage())
					} else {
						require.Equal(t, storypb.AttackOutcome_ATTACK_OUTCOME_MISS, b.GetOutcome())
						require.Zero(t, b.GetDamage())
					}
				}
				for _, result := range act.GetActivate().GetResults() {
					require.NotEqual(t, "damage-applied", result.GetKind(), "the attack is the sole damage account")
					if result.GetKind() == "condition-applied" {
						lights++
						require.True(t, hit)
						require.Equal(t, "skel-1", result.GetTarget().GetId())
						require.Equal(t, refs.Conditions.GuidingBolt().String(), result.GetRef())
						require.Equal(t, id, result.GetSourceId())
					}
				}
				for _, result := range act.GetCast().GetResults() {
					require.Nil(t, result.GetSave(), "Guiding Bolt makes an attack, not a saving throw")
				}
			}
			require.Equal(t, 1, attacks)
			if hit {
				require.Equal(t, 1, lights)
			} else {
				require.Zero(t, lights)
			}
			reopenClericHost(t, h, sdk.StaleTargetRefuse)
			requireLiveIsStory(ctx, t, h, castSessionID, id, live)
			if hit {
				require.True(t, hasGuidingBoltLight(t, h))
				_, err = h.handler.EndTurn(ctx, &sessionpb.EndTurnRequest{Session: castSessionID, Member: id,
					DeclarationId: currentDeclarationID(ctx, t, h.handler, castSessionID, id, sessionpb.Verb_VERB_END_TURN)})
				require.NoError(t, err)
				allyCtx := auth.WithPlayerID(context.Background(), "player-alice")
				attacked, attackErr := h.handler.Attack(allyCtx, &sessionpb.AttackRequest{Session: castSessionID, Attacker: "alice", Target: "skel-1",
					DeclarationId: currentDeclarationID(allyCtx, t, h.handler, castSessionID, "alice", sessionpb.Verb_VERB_ATTACK)})
				require.NoError(t, attackErr)
				require.NotEmpty(t, attacked.GetCalculation().GetComponents())
				granted := attacked.GetCalculation().GetComponents()[0].GetDice().GetKeep().GetGranted()
				require.NotEmpty(t, granted, "the ally's attack uses the persisted light")
				require.Equal(t, refs.Conditions.GuidingBolt().String(), granted[0].GetRef())
				require.False(t, hasGuidingBoltLight(t, h), "even a missed attack consumes the light")
			}
		})
	}
}

func hasGuidingBoltLight(t *testing.T, h *acceptanceHarness) bool {
	t.Helper()
	stored, err := sessionorch.NewSessionRepository(h.redis, 0).GetSession(context.Background(), castSessionID)
	require.NoError(t, err)
	for _, npc := range stored.NPCs {
		if npc.ID != "skel-1" {
			continue
		}
		for _, raw := range npc.Conditions {
			var condition struct{ Ref core.Ref }
			require.NoError(t, json.Unmarshal(raw, &condition))
			if condition.Ref.String() == refs.Conditions.GuidingBolt().String() {
				return true
			}
		}
	}
	return false
}

func useGuidingBoltDice(t *testing.T, h *acceptanceHarness, roll int) {
	t.Helper()
	orch, err := sessionorch.New(sessionorch.Config{Redis: h.redis, Characters: h.charRepo, Dice: guidingBoltDice{attack: roll}, TurnDriver: sdk.Pass{}})
	require.NoError(t, err)
	h.manager = orch
	h.handler, err = sessionhandler.New(&sessionhandler.HandlerConfig{Manager: orch.Manager, Characters: h.charRepo})
	require.NoError(t, err)
	h.story = newStoryHandler(t, orch, h.charRepo)
}

func TestAcceptance_GuidingBoltExpiresAfterNextCasterTurn(t *testing.T) {
	h, ctx, id := nativeClericCombatScene(t, spells.GuidingBolt)
	useGuidingBoltDice(t, h, 18)
	row := castRowFor(ctx, t, h, id, spells.GuidingBolt)
	_, err := h.handler.Cast(ctx, &sessionpb.CastRequest{Session: castSessionID, Member: id, DeclarationId: row.GetId(), Targets: []string{"skel-1"}})
	require.NoError(t, err)
	require.True(t, hasGuidingBoltLight(t, h))
	end := func(ctx context.Context, member string) {
		_, endErr := h.handler.EndTurn(ctx, &sessionpb.EndTurnRequest{Session: castSessionID, Member: member,
			DeclarationId: currentDeclarationID(ctx, t, h.handler, castSessionID, member, sessionpb.Verb_VERB_END_TURN)})
		require.NoError(t, endErr)
	}
	end(ctx, id)
	require.True(t, hasGuidingBoltLight(t, h), "casting turn does not expire the light")
	reopenClericHost(t, h, sdk.StaleTargetRefuse)
	end(auth.WithPlayerID(context.Background(), "player-alice"), "alice")
	require.True(t, hasGuidingBoltLight(t, h), "the target and ally do not own the expiry clock")
	end(ctx, id)
	require.False(t, hasGuidingBoltLight(t, h), "the caster's next turn end expires persisted light")
}
