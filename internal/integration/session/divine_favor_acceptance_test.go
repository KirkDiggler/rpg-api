package session_test

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/proto"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
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
			live := watchCast(ctx, t, h, id, func() {
				_, err := h.handler.Cast(ctx, &sessionpb.CastRequest{Session: castSessionID, Member: id, DeclarationId: row.GetId()})
				s.Require().NoError(err)
			})
			stored, err := h.charRepo.Get(ctx, characterrepo.GetInput{ID: id})
			s.Require().NoError(err)
			s.Equal(1, stored.Character.Data.ActionEconomy.ActionsRemaining)
			s.Zero(stored.Character.Data.ActionEconomy.BonusActionsRemaining)
			s.Equal(1, stored.Character.Data.Resources[resources.SpellSlotLevel1].Current)
			applied := 0
			for _, event := range live {
				if b := event.GetActivationResult().GetConditionApplied(); b != nil {
					applied++
					s.Equal(id, b.GetTarget())
					s.Equal(id, b.GetSourceId())
				}
				s.Nil(event.GetStruck())
				s.Nil(event.GetSaved())
			}
			s.Equal(1, applied)
			reopenClericHost(t, h, sdk.StaleTargetRefuse)
			story, err := h.handler.GetStory(ctx, &sessionpb.GetStoryRequest{Session: castSessionID, Member: id, FromSeq: live[0].GetSeq()})
			s.Require().NoError(err)
			s.Require().Len(story.GetEntries(), len(live))
			for i, event := range live {
				s.True(proto.Equal(event, story.Entries[i]))
			}
			useGuidingBoltDice(t, h, roll)
			attacked, err := h.handler.Attack(ctx, &sessionpb.AttackRequest{Session: castSessionID, Attacker: id, Target: "skel-1", DeclarationId: currentDeclarationID(ctx, t, h.handler, castSessionID, id, sessionpb.Verb_VERB_ATTACK)})
			s.Require().NoError(err)
			s.Equal(roll != 1, attacked.GetHit())
			if roll == 1 {
				s.Zero(attacked.GetDamage())
			} else {
				s.Positive(attacked.GetDamage())
				story, err := h.handler.GetStory(ctx, &sessionpb.GetStoryRequest{Session: castSessionID, Member: id, FromSeq: 1})
				s.Require().NoError(err)
				var extra *sessionpb.DamageComponent
				for _, event := range story.GetEntries() {
					for _, component := range event.GetStruck().GetDamageComponents() {
						if component.GetRoll().GetSource().GetRef() == "dnd5e:spells:divine-favor" {
							extra = component
						}
					}
				}
				s.Require().NotNil(extra, "weapon strike must expose the Divine Favor component in persisted Story")
				s.Equal(sessionpb.DamageType_DAMAGE_TYPE_RADIANT, extra.GetDamageType())
				s.Equal("Divine Favor", extra.GetRoll().GetSource().GetName())
				count := 1
				if roll == 20 {
					count = 2
				}
				s.Len(extra.GetRoll().GetDice().GetFinalRolls(), count)
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
