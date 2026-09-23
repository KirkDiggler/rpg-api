package session_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/KirkDiggler/rpg-api/internal/auth"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	"github.com/KirkDiggler/rpg-toolkit/core"

	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/proto"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/resources"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/spells"
)

type FaerieFireSuite struct{ suite.Suite }

func TestFaerieFireSuite(t *testing.T) { suite.Run(t, new(FaerieFireSuite)) }

func (s *FaerieFireSuite) TestNativeLightGrantPaidAreaSaveAndReplay() {
	for _, tc := range []struct {
		name    string
		roll    int
		applied int
	}{{"failed-save", 1, 1}, {"successful-save", 20, 0}} {
		s.Run(tc.name, func() {
			t := s.T()
			h, ctx, id := nativeClericCombatSceneAt(t, 8, 2, spells.FaerieFire)
			useGuidingBoltDice(t, h, tc.roll)
			row := castRowFor(ctx, t, h, id, spells.FaerieFire)
			s.Require().True(row.GetAvailable(), row.GetWhy())
			s.Equal(sessionpb.TargetKind_TARGET_KIND_CELL, row.GetTargetKind())
			s.Equal(sessionpb.FootprintShape_FOOTPRINT_SHAPE_BOX, row.GetFootprint().GetShape())
			s.Equal(sessionpb.FootprintOrigin_FOOTPRINT_ORIGIN_POINT, row.GetFootprint().GetOrigin())
			s.Equal(int32(20), row.GetFootprint().GetSizeFeet())
			aim := &sessionpb.Position{X: 8, Y: 2}
			preview, err := h.handler.Afford(ctx, &sessionpb.AffordRequest{Session: castSessionID, Member: id, CastAim: &sessionpb.CastAim{Declaration: row.GetId(), Cell: aim}})
			s.Require().NoError(err)
			s.Require().NotNil(preview.GetCastAim())
			s.True(preview.GetCastAim().GetAvailable())
			s.Equal([]string{"skel-1"}, preview.GetCastAim().GetAffectedMembers())
			s.True(proto.Equal(aim, preview.GetCastAim().GetAim().GetCell()))
			untouched, err := h.charRepo.Get(ctx, characterrepo.GetInput{ID: id})
			s.Require().NoError(err)
			s.Equal(2, untouched.Character.Data.Resources[resources.SpellSlotLevel1].Current)
			live := watchCast(ctx, t, h, id, func() {
				_, castErr := h.handler.Cast(ctx, &sessionpb.CastRequest{Session: castSessionID, Member: id, DeclarationId: row.GetId(), Cell: aim})
				s.Require().NoError(castErr)
			})
			stored, err := h.charRepo.Get(ctx, characterrepo.GetInput{ID: id})
			s.Require().NoError(err)
			s.Zero(stored.Character.Data.ActionEconomy.ActionsRemaining)
			s.Equal(1, stored.Character.Data.Resources[resources.SpellSlotLevel1].Current)
			saves, applied := 0, 0
			for _, event := range live {
				if save := event.GetSaved(); save != nil {
					saves++
					s.Equal(tc.roll == 20, save.GetSucceeded())
				}
				s.Nil(event.GetActivationResult().GetDamageApplied())
				if condition := event.GetActivationResult().GetConditionApplied(); condition != nil && condition.GetRef() == refs.Conditions.FaerieFire().String() {
					applied++
					s.Equal("skel-1", condition.GetTarget())
					s.Equal(id, condition.GetSourceId())
				}

				s.Nil(event.GetStruck())
				s.Nil(event.GetMissed())
			}
			s.Equal(1, saves)
			s.Equal(tc.applied, applied)
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

func (s *FaerieFireSuite) TestReloadedConditionBenefitsRepeatedAttacksAndExpiresWithConcentration() {
	t := s.T()
	h, ctx, id := nativeClericCombatSceneAt(t, 2, 1, spells.FaerieFire)
	useGuidingBoltDice(t, h, 1)
	row := castRowFor(ctx, t, h, id, spells.FaerieFire)
	_, err := h.handler.Cast(ctx, &sessionpb.CastRequest{Session: castSessionID, Member: id, DeclarationId: row.GetId(), Cell: &sessionpb.Position{X: 2, Y: 1}})
	s.Require().NoError(err)
	held := func() bool {
		stored, err := sessionorch.NewSessionRepository(h.redis, 0).GetSession(ctx, castSessionID)
		s.Require().NoError(err)
		for _, npc := range stored.NPCs {
			if npc.ID != "skel-1" {
				continue
			}
			for _, raw := range npc.Conditions {
				var condition struct{ Ref core.Ref }
				s.Require().NoError(json.Unmarshal(raw, &condition))
				if condition.Ref.String() == refs.Conditions.FaerieFire().String() {
					return true
				}
			}
		}
		return false
	}
	s.True(held())
	reopenClericHost(t, h, sdk.StaleTargetRefuse)
	useGuidingBoltDice(t, h, 1)
	allyCtx := auth.WithPlayerID(context.Background(), "player-alice")
	end := func(ctx context.Context, member string) {
		_, err := h.handler.EndTurn(ctx, &sessionpb.EndTurnRequest{Session: castSessionID, Member: member, DeclarationId: currentDeclarationID(ctx, t, h.handler, castSessionID, member, sessionpb.Verb_VERB_END_TURN)})
		s.Require().NoError(err)
	}
	for round := 0; round < 10; round++ {
		end(ctx, id)
		s.True(held(), "condition remains for its full duration")
		if round < 2 {
			attacked, err := h.handler.Attack(allyCtx, &sessionpb.AttackRequest{Session: castSessionID, Attacker: "alice", Target: "skel-1", DeclarationId: currentDeclarationID(allyCtx, t, h.handler, castSessionID, "alice", sessionpb.Verb_VERB_ATTACK)})
			s.Require().NoError(err)
			s.Require().NotEmpty(attacked.GetCalculation().GetComponents())
			granted := attacked.GetCalculation().GetComponents()[0].GetDice().GetKeep().GetGranted()
			s.Require().NotEmpty(granted)
			s.Equal(refs.Conditions.FaerieFire().String(), granted[0].GetRef())
			s.True(held(), "attacks never consume the condition")
		}
		end(allyCtx, "alice")
	}
	end(ctx, id)
	s.False(held(), "concentration expiry removes the persisted child")
}
