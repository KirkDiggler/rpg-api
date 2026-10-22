package session_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	storypb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/resources"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/spells"
)

type InflictWoundsSuite struct{ suite.Suite }

func TestInflictWoundsSuite(t *testing.T) { suite.Run(t, new(InflictWoundsSuite)) }

func (s *InflictWoundsSuite) TestNativePaidAttackAndReplay() {
	for _, tc := range []struct {
		name   string
		roll   int
		damage int32
	}{{"miss", 1, 0}, {"hit", 18, 3}, {"critical", 20, 6}} {
		s.Run(tc.name, func() {
			t := s.T()
			h, ctx, id := nativeClericCombatSceneAt(t, 2, 1, spells.InflictWounds)
			useGuidingBoltDice(t, h, tc.roll)
			row := castRowFor(ctx, t, h, id, spells.InflictWounds)
			s.Require().True(row.GetAvailable(), row.GetWhy())
			live := watchCastActs(ctx, t, h, id, func() {
				_, err := h.handler.Cast(ctx, &sessionpb.CastRequest{Session: castSessionID, Member: id, DeclarationId: row.GetId(), Targets: []string{"skel-1"}})
				s.Require().NoError(err)
			})
			stored, err := h.charRepo.Get(ctx, characterrepo.GetInput{ID: id})
			s.Require().NoError(err)
			s.Zero(stored.Character.Data.ActionEconomy.ActionsRemaining)
			s.Equal(1, stored.Character.Data.Resources[resources.SpellSlotLevel1].Current)
			attacks := 0
			for _, act := range live {
				if b := act.GetAttack(); b != nil {
					attacks++
					if tc.damage > 0 {
						s.Equal(storypb.AttackOutcome_ATTACK_OUTCOME_HIT, b.GetOutcome())
					} else {
						s.Equal(storypb.AttackOutcome_ATTACK_OUTCOME_MISS, b.GetOutcome())
					}
					s.Equal(tc.roll == 20, b.GetCritical())
					s.Equal(tc.damage, b.GetDamage())
					s.Equal("skel-1", b.GetTo().GetId())
					s.Equal(refs.Spells.InflictWounds().String(), b.GetAttack().GetRef())
					s.NotNil(b.GetRoll())
					s.NotEmpty(b.GetPresentationId())
				}
				for _, result := range act.GetCast().GetResults() {
					s.Nil(result.GetSave(), "Inflict Wounds makes an attack, not a saving throw")
				}
				for _, result := range act.GetActivate().GetResults() {
					s.NotEqual("condition-applied", result.GetKind())
					s.NotEqual("damage-applied", result.GetKind(), "the attack is the sole damage account")
				}
			}
			s.Equal(1, attacks)
			reopenClericHost(t, h, sdk.StaleTargetRefuse)
			requireLiveIsStory(ctx, t, h, castSessionID, id, live)
		})
	}
}

func (s *InflictWoundsSuite) TestOutOfTouchRangeDoesNotPay() {
	t := s.T()
	h, ctx, id := nativeClericCombatScene(t, spells.InflictWounds)
	row := castRowFor(ctx, t, h, id, spells.InflictWounds)
	before, err := h.charRepo.Get(ctx, characterrepo.GetInput{ID: id})
	s.Require().NoError(err)
	_, err = h.handler.Cast(ctx, &sessionpb.CastRequest{Session: castSessionID, Member: id, DeclarationId: row.GetId(), Targets: []string{"skel-1"}})
	s.Require().Error(err)
	after, err := h.charRepo.Get(ctx, characterrepo.GetInput{ID: id})
	s.Require().NoError(err)
	s.Equal(before.Character.Data, after.Character.Data)
}
