package session_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/proto"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	characterpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/character"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	characterhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v2/character"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/resources"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/spells"
)

type ShieldOfFaithSuite struct{ suite.Suite }

func TestShieldOfFaithSuite(t *testing.T) { suite.Run(t, new(ShieldOfFaithSuite)) }

func (s *ShieldOfFaithSuite) TestNativeProtectionPrivateSheetReplayAndReplacement() {
	for _, self := range []bool{true, false} {
		name := "ally"
		if self {
			name = "self"
		}
		s.Run(name, func() {
			t := s.T()
			h, ctx, id := nativeClericCombatScene(t, spells.ShieldOfFaith)
			target, owner := "alice", "player-alice"
			if self {
				target, owner = id, "cleric-player"
			}
			sheet, err := characterhandler.New(&characterhandler.HandlerConfig{CharacterService: newAcceptanceCharacterService(t, h)})
			s.Require().NoError(err)
			ac := func() int32 {
				view, readErr := sheet.GetCharacterData(auth.WithPlayerID(context.Background(), owner), &characterpb.GetCharacterDataRequest{CharacterId: target})
				s.Require().NoError(readErr)
				return view.GetCharacter().GetArmorClassDetail().GetTotal()
			}
			before := ac()
			row := castRowFor(ctx, t, h, id, spells.ShieldOfFaith)
			s.Require().True(row.GetAvailable(), row.GetWhy())
			live := watchCast(ctx, t, h, id, func() {
				_, castErr := h.handler.Cast(ctx, &sessionpb.CastRequest{Session: castSessionID, Member: id, DeclarationId: row.GetId(), Targets: []string{target}})
				s.Require().NoError(castErr)
			})
			stored, err := h.charRepo.Get(ctx, characterrepo.GetInput{ID: id})
			s.Require().NoError(err)
			s.Equal(1, stored.Character.Data.ActionEconomy.ActionsRemaining)
			s.Zero(stored.Character.Data.ActionEconomy.BonusActionsRemaining)
			s.Equal(1, stored.Character.Data.Resources[resources.SpellSlotLevel1].Current)
			s.Equal(before+2, ac())
			_, err = sheet.GetCharacterData(auth.WithPlayerID(context.Background(), "outsider"), &characterpb.GetCharacterDataRequest{CharacterId: target})
			s.Error(err, "the AC refresh must not disclose a private sheet to another player")
			applied := 0
			for _, event := range live {
				if b := event.GetActivationResult().GetConditionApplied(); b != nil {
					applied++
					s.Equal(target, b.GetTarget())
					s.Equal(id, b.GetSourceId())
				}
				s.Nil(event.GetStruck())
				s.Nil(event.GetSaved())
				s.Nil(event.GetActivationResult().GetHealingApplied())
			}
			s.Equal(1, applied)
			reopenClericHost(t, h, sdk.StaleTargetRefuse)
			s.Equal(before+2, ac())
			story, err := h.handler.GetStory(ctx, &sessionpb.GetStoryRequest{Session: castSessionID, Member: id, FromSeq: live[0].GetSeq()})
			s.Require().NoError(err)
			s.Require().Len(story.GetEntries(), len(live))
			for i, event := range live {
				s.True(proto.Equal(event, story.Entries[i]))
			}
			// Guidance is an action cantrip: legal after a bonus-action spell, and
			// replacing concentration must remove the target's AC bonus immediately.
			guidance := castRowFor(ctx, t, h, id, spells.Guidance)
			s.Require().True(guidance.GetAvailable(), guidance.GetWhy())
			_, err = h.handler.Cast(ctx, &sessionpb.CastRequest{Session: castSessionID, Member: id, DeclarationId: guidance.GetId(), Targets: []string{id}})
			s.Require().NoError(err)
			s.Equal(before, ac())
			reopenClericHost(t, h, sdk.StaleTargetRefuse)
			s.Equal(before, ac())
		})
	}
}

func (s *ShieldOfFaithSuite) TestLeveledActionSpellBlockedInBothOrdersWithoutPayment() {
	for _, first := range []spells.Spell{spells.ShieldOfFaith, spells.Bless} {
		s.Run(first, func() {
			t := s.T()
			h, ctx, id := nativeClericCombatScene(t, spells.ShieldOfFaith)
			second := spells.Bless
			if first == spells.Bless {
				second = spells.ShieldOfFaith
			}
			stale := castRowFor(ctx, t, h, id, second)
			row := castRowFor(ctx, t, h, id, first)
			_, err := h.handler.Cast(ctx, &sessionpb.CastRequest{Session: castSessionID, Member: id, DeclarationId: row.GetId(), Targets: []string{id}})
			s.Require().NoError(err)
			reopenClericHost(t, h, sdk.StaleTargetRefuse)
			blocked := castRowFor(ctx, t, h, id, second)
			s.False(blocked.GetAvailable())
			before, err := h.charRepo.Get(ctx, characterrepo.GetInput{ID: id})
			s.Require().NoError(err)
			_, err = h.handler.Cast(ctx, &sessionpb.CastRequest{Session: castSessionID, Member: id, DeclarationId: stale.GetId(), Targets: []string{id}})
			s.Require().Error(err)
			after, err := h.charRepo.Get(ctx, characterrepo.GetInput{ID: id})
			s.Require().NoError(err)
			s.Equal(before.Character.Data, after.Character.Data)
		})
	}
}

func (s *ShieldOfFaithSuite) TestProtectionTurnsBoundaryHitIntoMissAfterReload() {
	for _, protected := range []bool{false, true} {
		name := "unprotected"
		if protected {
			name = "protected"
		}
		s.Run(name, func() {
			t := s.T()
			h, ctx, id := nativeClericCombatScene(t, spells.ShieldOfFaith)
			sheet, err := characterhandler.New(&characterhandler.HandlerConfig{CharacterService: newAcceptanceCharacterService(t, h)})
			s.Require().NoError(err)
			view, err := sheet.GetCharacterData(ctx, &characterpb.GetCharacterDataRequest{CharacterId: id})
			s.Require().NoError(err)
			base := view.GetCharacter().GetArmorClassDetail().GetTotal()
			if protected {
				row := castRowFor(ctx, t, h, id, spells.ShieldOfFaith)
				_, err = h.handler.Cast(ctx, &sessionpb.CastRequest{Session: castSessionID, Member: id, DeclarationId: row.GetId(), Targets: []string{id}})
				s.Require().NoError(err)
			}
			_, err = h.handler.EndTurn(ctx, &sessionpb.EndTurnRequest{Session: castSessionID, Member: id,
				DeclarationId: currentDeclarationID(ctx, t, h.handler, castSessionID, id, sessionpb.Verb_VERB_END_TURN)})
			s.Require().NoError(err)
			// This fixture's fighter has STR +3 and proficiency +2.
			useGuidingBoltDice(t, h, int(base)-5)
			allyCtx := auth.WithPlayerID(context.Background(), "player-alice")
			attacked, err := h.handler.Attack(allyCtx, &sessionpb.AttackRequest{Session: castSessionID, Attacker: "alice", Target: id,
				DeclarationId: currentDeclarationID(allyCtx, t, h.handler, castSessionID, "alice", sessionpb.Verb_VERB_ATTACK)})
			s.Require().NoError(err)
			s.Equal(!protected, attacked.GetHit())
			if protected {
				s.Zero(attacked.GetDamage())
			}
		})
	}
}

func (s *ShieldOfFaithSuite) TestOrdinaryAttackRemainsAvailableAfterBonusSpell() {
	t := s.T()
	h, ctx, id := nativeClericCombatSceneAt(t, 2, 1, spells.ShieldOfFaith)
	row := castRowFor(ctx, t, h, id, spells.ShieldOfFaith)
	_, err := h.handler.Cast(ctx, &sessionpb.CastRequest{Session: castSessionID, Member: id, DeclarationId: row.GetId(), Targets: []string{id}})
	s.Require().NoError(err)
	_, err = h.handler.Attack(ctx, &sessionpb.AttackRequest{Session: castSessionID, Attacker: id, Target: "skel-1",
		DeclarationId: currentDeclarationID(ctx, t, h.handler, castSessionID, id, sessionpb.Verb_VERB_ATTACK)})
	s.Require().NoError(err)
	stored, err := h.charRepo.Get(ctx, characterrepo.GetInput{ID: id})
	s.Require().NoError(err)
	s.Equal(1, stored.Character.Data.Resources[resources.SpellSlotLevel1].Current)
}

func (s *ShieldOfFaithSuite) TestMonsterProtectionChangesAttackOutcome() {
	t := s.T()
	h, ctx, id := nativeClericCombatScene(t, spells.ShieldOfFaith)
	row := castRowFor(ctx, t, h, id, spells.ShieldOfFaith)
	_, err := h.handler.Cast(ctx, &sessionpb.CastRequest{Session: castSessionID, Member: id, DeclarationId: row.GetId(), Targets: []string{"skel-1"}})
	s.Require().NoError(err)
	_, err = h.handler.EndTurn(ctx, &sessionpb.EndTurnRequest{Session: castSessionID, Member: id,
		DeclarationId: currentDeclarationID(ctx, t, h.handler, castSessionID, id, sessionpb.Verb_VERB_END_TURN)})
	s.Require().NoError(err)
	useGuidingBoltDice(t, h, 8) // 8 + 5 hits skeleton AC 13, but misses protected AC 15.
	allyCtx := auth.WithPlayerID(context.Background(), "player-alice")
	attacked, err := h.handler.Attack(allyCtx, &sessionpb.AttackRequest{Session: castSessionID, Attacker: "alice", Target: "skel-1",
		DeclarationId: currentDeclarationID(allyCtx, t, h.handler, castSessionID, "alice", sessionpb.Verb_VERB_ATTACK)})
	s.Require().NoError(err)
	s.False(attacked.GetHit())
	s.Zero(attacked.GetDamage())
}
