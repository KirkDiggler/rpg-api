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
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/resources"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/spells"
)

type WardingFlareSuite struct{ suite.Suite }

func TestWardingFlareSuite(t *testing.T) { suite.Run(t, new(WardingFlareSuite)) }
func (s *WardingFlareSuite) TestNativeLightReactionOwnerSheetAndReplay() {
	for _, spend := range []bool{true, false} {
		s.Run(map[bool]string{true: "use", false: "decline"}[spend], func() {
			t := s.T()
			h, ctx, id := nativeClericCombatSceneAt(t, 8, 2, spells.FaerieFire)
			stored, err := h.charRepo.Get(ctx, characterrepo.GetInput{ID: id})
			s.Require().NoError(err)
			initial := stored.Character.Data.Resources[resources.WardingFlare].Current
			s.Positive(initial)
			sheetHandler, err := characterhandler.New(&characterhandler.HandlerConfig{CharacterService: newAcceptanceCharacterService(t, h)})
			s.Require().NoError(err)
			assertSheet := func(want int) {
				view, e := sheetHandler.GetCharacterData(ctx, &characterpb.GetCharacterDataRequest{CharacterId: id})
				s.Require().NoError(e)
				found := false
				for _, r := range view.GetCharacter().GetResources() {
					if r.GetKey() == string(resources.WardingFlare) {
						found = true
						s.Equal("Warding Flare", r.GetName())
						s.Equal(int32(want), r.GetCurrent())
					}
				}
				s.True(found)
			}
			assertSheet(initial)
			otherCtx := auth.WithPlayerID(context.Background(), "player-alice")
			_, err = sheetHandler.GetCharacterData(otherCtx, &characterpb.GetCharacterDataRequest{CharacterId: id})
			s.Error(err, "private resources stay owner-only")
			_, err = h.handler.EndTurn(ctx, &sessionpb.EndTurnRequest{Session: castSessionID, Member: id, DeclarationId: currentDeclarationID(ctx, t, h.handler, castSessionID, id, sessionpb.Verb_VERB_END_TURN)})
			s.Require().NoError(err)
			useGuidingBoltDice(t, h, 2)
			attacked, err := h.handler.Attack(otherCtx, &sessionpb.AttackRequest{Session: castSessionID, Attacker: "alice", Target: id, DeclarationId: currentDeclarationID(otherCtx, t, h.handler, castSessionID, "alice", sessionpb.Verb_VERB_ATTACK)})
			s.Require().NoError(err)
			s.True(attacked.GetPaused())
			s.Nil(attacked.GetCalculation())
			reopenClericHost(t, h, sdk.StaleTargetRefuse)
			useGuidingBoltDice(t, h, 2)
			row := reactRow(ctx, t, h, castSessionID, id)
			s.Require().NotNil(row)
			s.Equal(refs.Features.WardingFlare().String(), row.GetReaction().GetRef())
			s.Require().Len(row.GetOptions(), 1)
			in := &sessionpb.ReactRequest{Session: castSessionID, Member: id, DeclarationId: row.GetId(), Choice: sessionpb.ReactChoice_REACT_CHOICE_HOLD}
			if spend {
				in.Choice = sessionpb.ReactChoice_REACT_CHOICE_STRIKE
				in.Option = "use"
			}
			_, err = h.handler.React(otherCtx, in)
			s.Error(err, "another player cannot answer this reaction")
			live := watchCast(ctx, t, h, id, func() { _, e := h.handler.React(ctx, in); s.Require().NoError(e) })
			want := initial
			if spend {
				want--
			}
			assertSheet(want)
			stored, err = h.charRepo.Get(ctx, characterrepo.GetInput{ID: id})
			s.Require().NoError(err)
			s.Equal(2, stored.Character.Data.Resources[resources.SpellSlotLevel1].Current)
			swings := 0
			for _, e := range live {
				if e.GetMissed() != nil {
					swings++
					imposed := e.GetMissed().GetCalculation().GetComponents()[0].GetDice().GetKeep().GetImposed()
					if spend {
						s.Require().Len(imposed, 1)
						s.Equal(refs.Features.WardingFlare().String(), imposed[0].GetRef())
					} else {
						s.Empty(imposed)
					}
				}
			}
			s.Equal(1, swings)
			_, err = h.handler.React(ctx, in)
			s.Error(err)
			assertSheet(want)
			reopenClericHost(t, h, sdk.StaleTargetRefuse)
			s.Require().NotEmpty(live)
			story, err := h.handler.GetStory(ctx, &sessionpb.GetStoryRequest{Session: castSessionID, Member: id, FromSeq: live[0].GetSeq()})
			s.Require().NoError(err)
			s.Require().Len(story.GetEntries(), len(live))
			for i, e := range live {
				s.True(proto.Equal(e, story.Entries[i]))
			}
		})
	}
}
