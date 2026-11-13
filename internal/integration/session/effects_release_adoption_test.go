package session_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	storypb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/spells"
)

type EffectsReleaseAdoptionSuite struct{ suite.Suite }

func TestEffectsReleaseAdoptionSuite(t *testing.T) { suite.Run(t, new(EffectsReleaseAdoptionSuite)) }

// This is an adoption/transport proof with a saved-sheet fixture, not a new
// character-acquisition claim. Cast, turn progression, consumption, persistence
// and both public streams use the actual API and toolkit paths.
func (s *EffectsReleaseAdoptionSuite) TestLastMemberEndReasonSurvivesLiveAndColdStory() {
	t := s.T()
	h := newAcceptanceHarnessWith(t, failedSaveDice{}, sdk.Pass{})
	bardCtx := auth.WithPlayerID(context.Background(), "player-bella")
	allyCtx := auth.WithPlayerID(context.Background(), "player-alice")
	_, err := h.charRepo.Create(bardCtx, characterrepo.CreateInput{Character: &entities.Character{Data: castingBard("bella", "player-bella", spells.TrueStrike)}})
	s.Require().NoError(err)
	_, err = h.charRepo.Create(allyCtx, characterrepo.CreateInput{Character: &entities.Character{Data: armedFighter("alice", "player-alice")}})
	s.Require().NoError(err)
	h.launch(t, castSessionID, withMonsters(buildOpenRoom(t, 12, 6), monsterAt("skel-1", refs.Monsters.Skeleton().String(), 2, 1)), seatAt("bella", 2, 0), seatAt("alice", 3, 0))
	turn, err := h.handler.Turn(allyCtx, &sessionpb.TurnRequest{Session: castSessionID, Member: "alice"})
	s.Require().NoError(err)
	s.Equal("alice", turn.GetActive())
	endTurn := func(ctx context.Context, member string) {
		_, endErr := h.handler.EndTurn(ctx, &sessionpb.EndTurnRequest{Session: castSessionID, Member: member, DeclarationId: currentDeclarationID(ctx, t, h.handler, castSessionID, member, sessionpb.Verb_VERB_END_TURN)})
		s.Require().NoError(endErr)
	}
	endTurn(allyCtx, "alice")
	row := castRowFor(bardCtx, t, h, "bella", spells.TrueStrike)
	_, err = h.handler.Cast(bardCtx, &sessionpb.CastRequest{Session: castSessionID, Member: "bella", DeclarationId: row.GetId(), Target: "skel-1"})
	s.Require().NoError(err)
	s.Contains(conditionRefsOf(t, h.charRepo, "bella"), refs.Conditions.TrueStrike().String())
	reopenClericHost(t, h, sdk.StaleTargetRefuse)
	endTurn(bardCtx, "bella")
	endTurn(allyCtx, "alice")
	var casterLive []*storypb.Act
	observerLive := watchCastActs(allyCtx, t, h, "alice", func() {
		casterLive = watchCastActs(bardCtx, t, h, "bella", func() {
			_, attackErr := h.handler.Attack(bardCtx, &sessionpb.AttackRequest{Session: castSessionID, Attacker: "bella", Target: "skel-1", DeclarationId: currentDeclarationID(bardCtx, t, h.handler, castSessionID, "bella", sessionpb.Verb_VERB_ATTACK)})
			s.Require().NoError(attackErr)
		})
	})
	for _, acts := range [][]*storypb.Act{casterLive, observerLive} {
		ended := 0
		for _, act := range acts {
			if end := act.GetState().GetConcentration(); end != nil {
				ended++
				s.Equal("bella", act.GetState().GetParty().GetId())
				s.Equal(refs.Spells.TrueStrike().String(), end.GetSpell().GetRef())
				s.Equal("members_empty", end.GetReason(), "the API must preserve the provider's current end cause, not a legacy spell-specific value")
			}
		}
		s.Equal(1, ended, "last-member consumption ends exactly one maintainer on both streams")
	}
	s.NotContains(conditionRefsOf(t, h.charRepo, "bella"), refs.Conditions.TrueStrike().String())
	s.NotContains(conditionRefsOf(t, h.charRepo, "bella"), refs.Conditions.LinkedEffects().String())
	reopenClericHost(t, h, sdk.StaleTargetRefuse)
	requireLiveIsStory(bardCtx, t, h, castSessionID, "bella", casterLive)
	requireLiveIsStory(allyCtx, t, h, castSessionID, "alice", observerLive)
}
