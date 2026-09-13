package session_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/KirkDiggler/rpg-toolkit/core"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/resources"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/spells"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	sessionhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

// Reuse the established turn/geometry fixture, replacing its caster's stored
// sheet with a Cleric. Character acquisition is exercised separately.
func clericCastScene(t *testing.T) (*acceptanceHarness, context.Context) {
	t.Helper()
	h, ctx, _ := castScene(t)
	stored, err := h.charRepo.Get(ctx, characterrepo.GetInput{ID: "bella"})
	require.NoError(t, err)
	sheet := stored.Character.Data
	sheet.ClassID = classes.Cleric
	sheet.Features = nil
	sheet.AbilityScores[abilities.WIS] = 16
	sheet.KnownSpells = []string{"dnd5e:spells:bless", "dnd5e:spells:cure-wounds", "dnd5e:spells:healing-word"}
	sheet.Resources[resources.SpellSlotLevel1] = character.RecoverableResourceData{Current: 2, Maximum: 2}
	_, err = h.charRepo.Update(ctx, characterrepo.UpdateInput{Character: stored.Character})
	require.NoError(t, err)
	return h, ctx
}

func reopenClericHost(t *testing.T, h *acceptanceHarness, policy sdk.StaleTargetPolicy) {
	t.Helper()
	orch, err := sessionorch.New(sessionorch.Config{
		Redis: h.redis, Characters: h.charRepo, Dice: failedSaveDice{}, TurnDriver: sdk.Pass{}, StaleTargetPolicy: policy,
	})
	require.NoError(t, err)
	h.manager = orch
	h.handler, err = sessionhandler.New(&sessionhandler.HandlerConfig{Manager: orch.Manager, Broker: orch.Broker, Characters: h.charRepo})
	require.NoError(t, err)
}

func TestAcceptance_BlessHostPolicyAndPersistedMixedOutcomes(t *testing.T) {
	for _, policy := range []sdk.StaleTargetPolicy{"", sdk.StaleTargetRefuse, sdk.StaleTargetAttempt} {
		t.Run("policy="+string(policy), func(t *testing.T) {
			h, ctx, id := nativeClericCombatScene(t)
			reopenClericHost(t, h, policy)
			row := castRowFor(ctx, t, h, id, spells.Bless)
			require.True(t, row.GetAvailable(), "host construction must explicitly enable known-creature offers")
			require.Equal(t, int32(1), row.GetMinTargets())
			require.Equal(t, int32(3), row.GetMaxTargets())

			// Store stale remembered testimony, not the target's current position.
			repo := sessionorch.NewEncounterRepository(h.redis, 0)
			world, err := repo.GetEncounter(ctx, "room-encounter")
			require.NoError(t, err)
			holding := world.Perception.Intel.Holdings[core.EntityID(id)]["skel-1"]
			holding.CurrentVia = nil
			holding.Payload, err = encounter.EncodeSightTestimony(encounter.SightTestimony{State: encounter.LocationKnown, Position: at(3, 2)})
			require.NoError(t, err)
			world.Perception.Intel.Holdings[core.EntityID(id)]["skel-1"] = holding
			require.NoError(t, repo.SaveEncounter(ctx, "room-encounter", world))

			row = castRowFor(ctx, t, h, id, spells.Bless)
			var candidate *sessionpb.TargetCandidate
			for _, c := range row.GetCandidates() {
				if c.GetMember() == "skel-1" {
					candidate = c
				}
			}
			require.NotNil(t, candidate)
			require.Equal(t, policy == sdk.StaleTargetAttempt, candidate.GetAvailable())
			if policy != sdk.StaleTargetAttempt {
				require.NotEmpty(t, candidate.GetWhy().GetText())
			}
			live := watchCast(ctx, t, h, id, func() {
				_, castErr := h.handler.Cast(ctx, &sessionpb.CastRequest{
					Session: castSessionID, Member: id, DeclarationId: row.GetId(), Targets: []string{"skel-1", "alice", id},
				})
				if policy == sdk.StaleTargetAttempt {
					require.NoError(t, castErr)
				} else {
					require.Error(t, castErr)
				}
			})
			stored, err := h.charRepo.Get(ctx, characterrepo.GetInput{ID: id})
			require.NoError(t, err)
			if policy != sdk.StaleTargetAttempt {
				require.Equal(t, 2, stored.Character.Data.Resources[resources.SpellSlotLevel1].Current)
				return
			}
			require.Equal(t, 1, stored.Character.Data.Resources[resources.SpellSlotLevel1].Current)
			var targets []string
			for _, event := range live {
				if miss := event.GetCastMissed(); miss != nil {
					require.Equal(t, id, miss.GetActor())
					require.Equal(t, "dnd5e:spells:bless", miss.GetSpell().GetRef())
					targets = append(targets, miss.GetTarget())
				}
				if applied := event.GetActivationResult().GetConditionApplied(); applied != nil {
					require.Equal(t, id, applied.GetSourceId())
					targets = append(targets, applied.GetTarget())
				}
			}
			require.Equal(t, []string{"skel-1", "alice", id}, targets)
			require.NotEmpty(t, live)
			reopenClericHost(t, h, policy)
			story, err := h.handler.GetStory(ctx, &sessionpb.GetStoryRequest{Session: castSessionID, Member: id, FromSeq: live[0].GetSeq()})
			require.NoError(t, err)
			require.Len(t, story.GetEntries(), len(live))
			for i, event := range live {
				require.True(t, proto.Equal(event, story.Entries[i]), "live/story mismatch at %d", i)
			}
		})
	}
}

func TestAcceptance_ClericHealingResults(t *testing.T) {
	for _, spell := range []spells.Spell{spells.CureWounds, spells.HealingWord} {
		t.Run(spell, func(t *testing.T) {
			h, ctx := clericCastScene(t)
			patient, err := h.charRepo.Get(ctx, characterrepo.GetInput{ID: "alice"})
			require.NoError(t, err)
			patient.Character.Data.HitPoints = 1
			_, err = h.charRepo.Update(ctx, characterrepo.UpdateInput{Character: patient.Character})
			require.NoError(t, err)
			row := castRowFor(ctx, t, h, "bella", spell)
			require.True(t, row.GetAvailable())
			live := watchCast(ctx, t, h, "bella", func() {
				_, err = h.handler.Cast(ctx, &sessionpb.CastRequest{Session: castSessionID, Member: "bella", DeclarationId: row.GetId(), Targets: []string{"alice"}})
				require.NoError(t, err)
			})
			var healed *sessionpb.HealingApplied
			for _, event := range live {
				if body := event.GetActivationResult().GetHealingApplied(); body != nil {
					healed = body
				}
			}
			require.NotNil(t, healed)
			require.Equal(t, "alice", healed.GetTarget())
			require.Equal(t, row.GetSpell().GetRef(), healed.GetSourceRef())
			require.Equal(t, int32(1), healed.GetHpBefore())
			require.Positive(t, healed.GetAmount())
			require.Equal(t, healed.GetHpBefore()+healed.GetAmount(), healed.GetHpAfter())
			require.NotNil(t, healed.GetCalculation())
		})
	}
}
