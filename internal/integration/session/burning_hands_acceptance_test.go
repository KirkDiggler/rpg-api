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

type BurningHandsSuite struct{ suite.Suite }

func TestBurningHandsSuite(t *testing.T) { suite.Run(t, new(BurningHandsSuite)) }

func (s *BurningHandsSuite) TestNativeLightGrantPaidAreaSaveAndReplay() {
	for _, tc := range []struct {
		name   string
		roll   int
		damage int32
	}{{"failed-save", 1, 3}, {"successful-save", 20, 1}} {
		s.Run(tc.name, func() {
			t := s.T()
			h, ctx, id := nativeClericCombatSceneAt(t, 2, 1, spells.BurningHands)
			useGuidingBoltDice(t, h, tc.roll)
			row := castRowFor(ctx, t, h, id, spells.BurningHands)
			s.Require().True(row.GetAvailable(), row.GetWhy())
			s.Equal(sessionpb.TargetKind_TARGET_KIND_CELL, row.GetTargetKind())
			s.Equal(sessionpb.FootprintShape_FOOTPRINT_SHAPE_TRIANGLE, row.GetFootprint().GetShape())
			s.Equal(sessionpb.FootprintOrigin_FOOTPRINT_ORIGIN_CASTER, row.GetFootprint().GetOrigin())
			s.Equal(int32(15), row.GetFootprint().GetSizeFeet())
			live := watchCast(ctx, t, h, id, func() {
				_, err := h.handler.Cast(ctx, &sessionpb.CastRequest{Session: castSessionID, Member: id, DeclarationId: row.GetId(), Cell: pbAt(2, 1)})
				s.Require().NoError(err)
			})
			stored, err := h.charRepo.Get(ctx, characterrepo.GetInput{ID: id})
			s.Require().NoError(err)
			s.Zero(stored.Character.Data.ActionEconomy.ActionsRemaining)
			s.Equal(1, stored.Character.Data.Resources[resources.SpellSlotLevel1].Current)
			saves, damages := 0, 0
			for _, event := range live {
				if save := event.GetSaved(); save != nil {
					saves++
					s.Equal(tc.roll == 20, save.GetSucceeded())
				}
				if damage := event.GetActivationResult().GetDamageApplied(); damage != nil {
					damages++
					s.Equal("skel-1", damage.GetTarget())
					s.Equal(tc.damage, damage.GetAmount())
					s.Equal(sessionpb.DamageType_DAMAGE_TYPE_FIRE, damage.GetDamageType())
					s.Equal(refs.Spells.BurningHands().String(), damage.GetSourceRef())
					s.NotNil(damage.GetCalculation())
				}
				s.Nil(event.GetStruck())
				s.Nil(event.GetMissed())
			}
			s.Equal(1, saves)
			s.Equal(1, damages)
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
