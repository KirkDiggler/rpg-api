package session_test

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/proto"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	storypb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
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
			aim := &sessionpb.Position{X: 2.01, Y: 1}
			preview, err := h.handler.Afford(ctx, &sessionpb.AffordRequest{Session: castSessionID, Member: id, CastAim: &sessionpb.CastAim{Declaration: row.GetId(), Cell: aim}})
			s.Require().NoError(err)
			s.Require().NotNil(preview.GetCastAim())
			s.True(preview.GetCastAim().GetAvailable())
			s.Equal([]string{"skel-1"}, preview.GetCastAim().GetAffectedMembers())
			s.True(proto.Equal(aim, preview.GetCastAim().GetAim().GetCell()))
			untouched, err := h.charRepo.Get(ctx, characterrepo.GetInput{ID: id})
			s.Require().NoError(err)
			s.Equal(2, untouched.Character.Data.Resources[resources.SpellSlotLevel1].Current)
			live := watchCastActs(ctx, t, h, id, func() {
				_, castErr := h.handler.Cast(ctx, &sessionpb.CastRequest{Session: castSessionID, Member: id, DeclarationId: row.GetId(), Cell: aim})
				s.Require().NoError(castErr)
			})
			stored, err := h.charRepo.Get(ctx, characterrepo.GetInput{ID: id})
			s.Require().NoError(err)
			s.Zero(stored.Character.Data.ActionEconomy.ActionsRemaining)
			s.Equal(1, stored.Character.Data.Resources[resources.SpellSlotLevel1].Current)
			saves, damages := 0, 0
			for _, act := range live {
				for _, result := range act.GetCast().GetResults() {
					if result.GetSave() != nil {
						saves++
						s.Equal("skel-1", result.GetTarget().GetId())
						s.Equal(tc.roll == 20, result.GetOutcome() == storypb.CastOutcome_CAST_OUTCOME_SAVED)
					}
				}
				for _, result := range act.GetActivate().GetResults() {
					if result.GetKind() != "damage-applied" {
						continue
					}
					damages++
					s.Equal("skel-1", result.GetTarget().GetId())
					s.Equal(tc.damage, result.GetAmount())
					s.Equal(storypb.DamageType_DAMAGE_TYPE_FIRE, result.GetDamageType())
					s.Equal(refs.Spells.BurningHands().String(), result.GetRef())
					s.NotNil(result.GetRoll())
				}
				s.Nil(act.GetAttack(), "a save spell rolls no attack")
			}
			s.Equal(1, saves)
			s.Equal(1, damages)
			reopenClericHost(t, h, sdk.StaleTargetRefuse)
			requireLiveIsStory(ctx, t, h, castSessionID, id, live)
		})
	}
}
