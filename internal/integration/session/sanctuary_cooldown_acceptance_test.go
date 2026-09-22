package session_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	characterpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/character"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	characterhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v2/character"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/conditions"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/resources"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/spells"
)

func TestAcceptance_SanctuaryRecipientCooldown(t *testing.T) {
	h, ctx, id := nativeClericCombatScene(t)
	row := castRowFor(ctx, t, h, id, spells.Sanctuary)
	live := watchCast(ctx, t, h, id, func() {
		_, err := h.handler.Cast(ctx, &sessionpb.CastRequest{Session: castSessionID, Member: id, DeclarationId: row.GetId(), Targets: []string{id}})
		require.NoError(t, err)
	})
	applied := map[string]bool{}
	for _, event := range live {
		if condition := event.GetActivationResult().GetConditionApplied(); condition != nil {
			require.Equal(t, id, condition.GetTarget())
			require.Equal(t, id, condition.GetSourceId())
			applied[condition.GetRef()] = true
		}
	}
	require.True(t, applied[refs.Conditions.Sanctuary().String()])
	require.True(t, applied[refs.Conditions.SanctuaryImmune().String()])
	reopenClericHost(t, h, sdk.StaleTargetRefuse)
	story, err := h.handler.GetStory(ctx, &sessionpb.GetStoryRequest{Session: castSessionID, Member: id, FromSeq: live[0].GetSeq()})
	require.NoError(t, err)
	require.Len(t, story.GetEntries(), len(live))
	for i, event := range live {
		require.True(t, proto.Equal(event, story.Entries[i]))
	}
	owner, err := characterhandler.New(&characterhandler.HandlerConfig{CharacterService: newAcceptanceCharacterService(t, h)})
	require.NoError(t, err)
	private, err := owner.GetCharacterData(ctx, &characterpb.GetCharacterDataRequest{CharacterId: id})
	require.NoError(t, err)
	found := false
	for _, condition := range private.GetCharacter().GetConditions() {
		if condition.GetRef().GetId() == refs.Conditions.SanctuaryImmune().ID {
			found = true
		}
	}
	require.True(t, found, "owner refresh must show the recipient cooldown")
	_, err = h.handler.EndTurn(ctx, &sessionpb.EndTurnRequest{Session: castSessionID, Member: id,
		DeclarationId: currentDeclarationID(ctx, t, h.handler, castSessionID, id, sessionpb.Verb_VERB_END_TURN)})
	require.NoError(t, err)
	allyCtx := auth.WithPlayerID(context.Background(), "player-alice")
	_, err = h.handler.EndTurn(allyCtx, &sessionpb.EndTurnRequest{Session: castSessionID, Member: "alice",
		DeclarationId: currentDeclarationID(allyCtx, t, h.handler, castSessionID, "alice", sessionpb.Verb_VERB_END_TURN)})
	require.NoError(t, err)
	reopenClericHost(t, h, sdk.StaleTargetRefuse)
	row = castRowFor(ctx, t, h, id, spells.Sanctuary)
	require.True(t, row.GetAvailable(), "the new turn can pay for Sanctuary on another target")
	var recipient *sessionpb.TargetCandidate
	for _, candidate := range row.GetCandidates() {
		if candidate.GetMember() == id {
			recipient = candidate
		}
	}
	require.NotNil(t, recipient)
	require.False(t, recipient.GetAvailable())
	require.Contains(t, recipient.GetWhy().GetText(), "cooldown")
	before, err := h.charRepo.Get(ctx, characterrepo.GetInput{ID: id})
	require.NoError(t, err)
	require.Equal(t, 1, before.Character.Data.Resources[resources.SpellSlotLevel1].Current)
	_, err = h.handler.Cast(ctx, &sessionpb.CastRequest{Session: castSessionID, Member: id, DeclarationId: row.GetId(), Targets: []string{id}})
	require.Error(t, err)
	after, err := h.charRepo.Get(ctx, characterrepo.GetInput{ID: id})
	require.NoError(t, err)
	require.Equal(t, before.Character.Data, after.Character.Data, "refusal must preserve payment and existing ward")
	found = false
	for _, blob := range after.Character.Data.Conditions {
		condition, loadErr := conditions.LoadJSON(blob)
		require.NoError(t, loadErr)
		if cooldown, ok := condition.(*conditions.SanctuaryImmuneCondition); ok {
			found = true
			require.Equal(t, conditions.SanctuaryImmuneTurnEnds-1, cooldown.TurnEndsLeft)
		}
	}
	require.True(t, found, "the recipient timer must persist across host reload")
	_, err = h.handler.Cast(ctx, &sessionpb.CastRequest{Session: castSessionID, Member: id, DeclarationId: row.GetId(), Targets: []string{"alice"}})
	require.NoError(t, err, "the refused recast must leave the bonus action and slot available")
}
