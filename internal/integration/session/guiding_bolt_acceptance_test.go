package session_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/KirkDiggler/rpg-toolkit/core"

	"github.com/KirkDiggler/rpg-api/internal/auth"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/resources"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/spells"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
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
			h, ctx, id := nativeClericCombatScene(t)
			useGuidingBoltDice(t, h, roll)
			row := castRowFor(ctx, t, h, id, spells.GuidingBolt)
			require.True(t, row.GetAvailable(), "%s", row.GetWhy())
			live := watchCast(ctx, t, h, id, func() {
				_, castErr := h.handler.Cast(ctx, &sessionpb.CastRequest{Session: castSessionID, Member: id, DeclarationId: row.GetId(), Targets: []string{"skel-1"}})
				require.NoError(t, castErr)
			})
			stored, err := h.charRepo.Get(ctx, characterrepo.GetInput{ID: id})
			require.NoError(t, err)
			require.Zero(t, stored.Character.Data.ActionEconomy.ActionsRemaining)
			require.Equal(t, 1, stored.Character.Data.Resources[resources.SpellSlotLevel1].Current)
			attacks, lights := 0, 0
			for _, event := range live {
				if b := event.GetStruck(); b != nil {
					require.True(t, hit)
					attacks++
					require.Equal(t, refs.Spells.GuidingBolt().String(), b.GetAttack().GetRef())
					require.Equal(t, int32(4), b.GetDamage())
					require.NotNil(t, b.GetCalculation())
					require.NotEmpty(t, b.GetPresentationId())
				}
				if b := event.GetMissed(); b != nil {
					require.False(t, hit)
					attacks++
					require.Equal(t, refs.Spells.GuidingBolt().String(), b.GetAttack().GetRef())
					require.NotNil(t, b.GetCalculation())
					require.NotEmpty(t, b.GetPresentationId())
				}
				if b := event.GetActivationResult().GetConditionApplied(); b != nil {
					lights++
					require.True(t, hit)
					require.Equal(t, "skel-1", b.GetTarget())
					require.Equal(t, refs.Conditions.GuidingBolt().String(), b.GetRef())
					require.Equal(t, id, b.GetSourceId())
				}
				require.Nil(t, event.GetSaved(), "Guiding Bolt makes an attack, not a saving throw")
				require.Nil(t, event.GetActivationResult().GetDamageApplied(), "the attack is the sole damage account")
			}
			require.Equal(t, 1, attacks)
			if hit {
				require.Equal(t, 1, lights)
			} else {
				require.Zero(t, lights)
			}
			require.NotEmpty(t, live)
			reopenClericHost(t, h, sdk.StaleTargetRefuse)
			story, err := h.handler.GetStory(ctx, &sessionpb.GetStoryRequest{Session: castSessionID, Member: id, FromSeq: live[0].GetSeq()})
			require.NoError(t, err)
			require.Len(t, story.GetEntries(), len(live))
			for i, event := range live {
				require.True(t, proto.Equal(event, story.Entries[i]))
			}
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
	h.handler, err = sessionhandler.New(&sessionhandler.HandlerConfig{Manager: orch.Manager, Broker: orch.Broker, Characters: h.charRepo})
	require.NoError(t, err)
}

func TestAcceptance_GuidingBoltExpiresAfterNextCasterTurn(t *testing.T) {
	h, ctx, id := nativeClericCombatScene(t)
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
