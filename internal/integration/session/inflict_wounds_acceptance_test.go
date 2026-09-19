package session_test

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/proto"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
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
			live := watchCast(ctx, t, h, id, func() {
				_, err := h.handler.Cast(ctx, &sessionpb.CastRequest{Session: castSessionID, Member: id, DeclarationId: row.GetId(), Targets: []string{"skel-1"}})
				s.Require().NoError(err)
			})
			stored, err := h.charRepo.Get(ctx, characterrepo.GetInput{ID: id})
			s.Require().NoError(err)
			s.Zero(stored.Character.Data.ActionEconomy.ActionsRemaining)
			s.Equal(1, stored.Character.Data.Resources[resources.SpellSlotLevel1].Current)
			attacks := 0
			for _, event := range live {
				if b := event.GetStruck(); b != nil {
					attacks++
					s.Positive(tc.damage)
					s.Equal(tc.damage, b.GetDamage())
					s.Equal(refs.Spells.InflictWounds().String(), b.GetAttack().GetRef())
					s.NotNil(b.GetCalculation())
					s.NotEmpty(b.GetPresentationId())
				}
				if b := event.GetMissed(); b != nil {
					attacks++
					s.Zero(tc.damage)
					s.Equal(refs.Spells.InflictWounds().String(), b.GetAttack().GetRef())
					s.NotNil(b.GetCalculation())
					s.NotEmpty(b.GetPresentationId())
				}
				s.Nil(event.GetSaved())
				s.Nil(event.GetActivationResult().GetConditionApplied())
				s.Nil(event.GetActivationResult().GetDamageApplied())
			}
			s.Equal(1, attacks)
			reopenClericHost(t, h, sdk.StaleTargetRefuse)
			story, err := h.handler.GetStory(ctx, &sessionpb.GetStoryRequest{Session: castSessionID, Member: id, FromSeq: live[0].GetSeq()})
			s.Require().NoError(err)
			s.Require().Len(story.GetEntries(), len(live))
			for i, event := range live {
				s.True(proto.Equal(event, story.Entries[i]))
			}
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
