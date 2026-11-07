package session_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/suite"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/conditions"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/resources"
)

type ComposedRagingAcceptanceSuite struct{ suite.Suite }

func TestComposedRagingAcceptanceSuite(t *testing.T) {
	suite.Run(t, new(ComposedRagingAcceptanceSuite))
}

func (s *ComposedRagingAcceptanceSuite) TestNativeActivationFirstBoundaryAndInformationReload() {
	h, ctx := inAFightWith(s.T(), ragingBarbarian("alice", "player-alice"))
	rageRef := refs.Conditions.Raging().String()
	offer := activationsFor(ctx, s.T(), h, "alice")[refs.Features.Rage().String()]
	s.Require().NotNil(offer)
	_, err := h.handler.Activate(ctx, &sessionpb.ActivateRequest{
		Session: "acceptance-run", Member: "alice", DeclarationId: offer.GetId(),
	})
	s.Require().NoError(err)
	readStored := func() conditions.ComposedData {
		sheet := storedSheetOf(s.T(), h.charRepo, "alice")
		s.Equal(1, sheet.Resources[resources.RageCharges].Current, "reload does not refill or spend again")
		for _, raw := range sheet.Conditions {
			var envelope struct {
				Ref string `json:"ref"`
			}
			s.Require().NoError(json.Unmarshal(raw, &envelope))
			if envelope.Ref != rageRef {
				continue
			}
			var saved conditions.ComposedData
			s.Require().NoError(json.Unmarshal(raw, &saved))
			s.Require().NotNil(saved.Runtime)
			s.Require().NotNil(saved.Origin)
			s.Equal("alice", saved.Origin.EntityID)
			s.Equal(*refs.Features.Rage(), saved.Origin.Ref)
			s.JSONEq(`{"damage_bonus":{"barbarian_level":true}}`, string(saved.Bindings))
			return saved
		}
		s.T().Fatal("native Raging was not persisted")
		return conditions.ComposedData{}
	}
	fresh := readStored()
	s.Equal("fresh", fresh.Runtime.State)
	s.Equal(float64(0), fresh.Runtime.Bag["turns_active"])

	// The real handler reloads the sheet and carries each domain's answer.
	panel, err := h.handler.Afford(ctx, &sessionpb.AffordRequest{Session: "acceptance-run", Member: "alice"})
	s.Require().NoError(err)
	var attack *sessionpb.Declaration
	for _, declaration := range panel.GetDeclarations() {
		if declaration.GetVerb() == sessionpb.Verb_VERB_ATTACK {
			attack = declaration
			break
		}
	}
	s.Require().NotNil(attack)
	ids := map[string]bool{}
	applies := 0
	for _, row := range attack.GetEffects() {
		if row.GetRef() != rageRef {
			continue
		}
		s.False(ids[row.GetId()], "each output has its own identity")
		ids[row.GetId()] = true
		s.NotEmpty(row.GetDescription())
		if row.GetState() == sessionpb.EffectState_EFFECT_STATE_APPLIES {
			applies++
			s.Equal("+2 damage", row.GetBenefit())
		}
	}
	s.Len(ids, 4, "damage, resistance, saving throw and ability check remain distinct")
	s.Equal(1, applies, "only outgoing damage applies to a Strength melee attack")

	_, err = h.handler.EndTurn(ctx, &sessionpb.EndTurnRequest{
		Session: "acceptance-run", Member: "alice",
		DeclarationId: currentDeclarationID(ctx, s.T(), h.handler, "acceptance-run", "alice", sessionpb.Verb_VERB_END_TURN),
	})
	s.Require().NoError(err)
	after := readStored()
	s.Equal("raging", after.Runtime.State)
	s.Equal(float64(1), after.Runtime.Bag["turns_active"], "the first boundary must cause a Redis save")
	_, err = h.handler.Afford(ctx, &sessionpb.AffordRequest{Session: "acceptance-run", Member: "alice"})
	s.Require().NoError(err)
	reloaded := readStored()
	s.Equal(after.Runtime, reloaded.Runtime, "an information read cannot renew grace or advance progress")
}
