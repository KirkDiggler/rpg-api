package session_test

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/suite"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	storypb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/resources"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/spells"
)

type DivineFavorSuite struct{ suite.Suite }

func TestDivineFavorSuite(t *testing.T) { suite.Run(t, new(DivineFavorSuite)) }

func (s *DivineFavorSuite) TestNativeWarGrantPaymentReloadDamageAndReplay() {
	for _, roll := range []int{1, 18, 20} {
		s.Run(strconv.Itoa(roll), func() {
			t := s.T()
			h, ctx, id := nativeClericCombatSceneAt(t, 2, 1, spells.DivineFavor)
			row := castRowFor(ctx, t, h, id, spells.DivineFavor)
			s.Require().True(row.GetAvailable(), row.GetWhy())
			live := watchCastActs(ctx, t, h, id, func() {
				_, err := h.handler.Cast(ctx, &sessionpb.CastRequest{Session: castSessionID, Member: id, DeclarationId: row.GetId()})
				s.Require().NoError(err)
			})
			stored, err := h.charRepo.Get(ctx, characterrepo.GetInput{ID: id})
			s.Require().NoError(err)
			s.Equal(1, stored.Character.Data.ActionEconomy.ActionsRemaining)
			s.Zero(stored.Character.Data.ActionEconomy.BonusActionsRemaining)
			s.Equal(1, stored.Character.Data.Resources[resources.SpellSlotLevel1].Current)
			applied := 0
			for _, act := range live {
				for _, result := range act.GetActivate().GetResults() {
					if result.GetKind() == "condition-applied" {
						applied++
						s.Equal(id, result.GetTarget().GetId())
						s.Equal(id, result.GetSourceId())
					}
				}
				s.Nil(act.GetAttack())
				for _, result := range act.GetCast().GetResults() {
					s.Nil(result.GetSave(), "Divine Favor asks no save")
				}
			}
			s.Equal(1, applied)
			reopenClericHost(t, h, sdk.StaleTargetRefuse)
			requireLiveIsStory(ctx, t, h, castSessionID, id, live)
			useGuidingBoltDice(t, h, roll)
			attacked, err := h.handler.Attack(ctx, &sessionpb.AttackRequest{Session: castSessionID, Attacker: id, Target: "skel-1", DeclarationId: currentDeclarationID(ctx, t, h.handler, castSessionID, id, sessionpb.Verb_VERB_ATTACK)})
			s.Require().NoError(err)
			s.Equal(roll != 1, attacked.GetHit())
			if roll == 1 {
				s.Zero(attacked.GetDamage())
			} else {
				s.Positive(attacked.GetDamage())
				// The attack act's damage part does not yet carry the Divine Favor
				// source ref or name (convert_act's damageRollToProto drops a traced
				// component's Roll.Source; reported on rpg-project#631), so the part
				// is found by the one radiant source in this scene.
				var extra *storypb.DamageComponent
				for _, act := range storyOf(ctx, t, h.story, castSessionID, id) {
					if act.GetAttack().GetOutcome() != storypb.AttackOutcome_ATTACK_OUTCOME_HIT {
						continue
					}
					for _, component := range act.GetAttack().GetDamageComponents() {
						if component.GetDamageType() == storypb.DamageType_DAMAGE_TYPE_RADIANT {
							s.Nil(extra, "one Divine Favor part per strike")
							extra = component
						}
					}
				}
				s.Require().NotNil(extra, "weapon strike must expose the Divine Favor component in persisted Story")
				s.Equal("spell", extra.GetSource())
				count := 1
				if roll == 20 {
					count = 2
				}
				s.Len(extra.GetRoll().GetDice(), count)
			}
		})
	}
}

func (s *DivineFavorSuite) TestReplacementRemovesFavorAndLeveledActionIsBlocked() {
	t := s.T()
	h, ctx, id := nativeClericCombatScene(t, spells.DivineFavor)
	stale := castRowFor(ctx, t, h, id, spells.Bane)
	row := castRowFor(ctx, t, h, id, spells.DivineFavor)
	_, err := h.handler.Cast(ctx, &sessionpb.CastRequest{Session: castSessionID, Member: id, DeclarationId: row.GetId()})
	s.Require().NoError(err)
	reopenClericHost(t, h, sdk.StaleTargetRefuse)
	blocked := castRowFor(ctx, t, h, id, spells.Bane)
	s.False(blocked.GetAvailable())
	before, err := h.charRepo.Get(ctx, characterrepo.GetInput{ID: id})
	s.Require().NoError(err)
	_, err = h.handler.Cast(ctx, &sessionpb.CastRequest{Session: castSessionID, Member: id, DeclarationId: stale.GetId(), Targets: []string{"skel-1"}})
	s.Require().Error(err)
	after, err := h.charRepo.Get(ctx, characterrepo.GetInput{ID: id})
	s.Require().NoError(err)
	s.Equal(before.Character.Data, after.Character.Data)
	guidance := castRowFor(ctx, t, h, id, spells.Guidance)
	s.Require().True(guidance.GetAvailable(), guidance.GetWhy())
	_, err = h.handler.Cast(ctx, &sessionpb.CastRequest{Session: castSessionID, Member: id, DeclarationId: guidance.GetId(), Targets: []string{id}})
	s.Require().NoError(err)
	after, err = h.charRepo.Get(ctx, characterrepo.GetInput{ID: id})
	s.Require().NoError(err)
	for _, condition := range after.Character.Data.Conditions {
		s.NotContains(string(condition), "divine_favor")
	}
}
