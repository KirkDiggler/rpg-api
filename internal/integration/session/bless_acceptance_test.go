package session_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/KirkDiggler/rpg-toolkit/core"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"

	"github.com/KirkDiggler/rpg-api/internal/testsupport/levelfixture"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/resources"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/spells"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	storypb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
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
	levelfixture.Reclass(sheet, classes.Cleric)
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
	h.handler, err = sessionhandler.New(&sessionhandler.HandlerConfig{Manager: orch.Manager, Characters: h.charRepo})
	require.NoError(t, err)
	h.story = newStoryHandler(t, orch, h.charRepo)
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
			world, err := repo.GetEncounter(ctx, castSessionID)
			require.NoError(t, err)
			holding, found := world.Perception.Intel.Holdings[core.EntityID("member|"+id)]["member|skel-1"]
			require.True(t, found, "seed an existing sight holding, not a fabricated map entry")
			holding.CurrentVia = nil
			holding.Payload, err = encounter.EncodeSightTestimony(encounter.SightTestimony{State: encounter.LocationKnown, Position: at(3, 2)})
			require.NoError(t, err)
			world.Perception.Intel.Holdings[core.EntityID("member|"+id)]["member|skel-1"] = holding
			require.NoError(t, repo.SaveEncounter(ctx, castSessionID, world))

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
			live := watchCastActs(ctx, t, h, id, func() {
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
			for _, act := range live {
				for _, result := range act.GetCast().GetResults() {
					if result.GetOutcome() == storypb.CastOutcome_CAST_OUTCOME_MISS {
						require.Equal(t, id, act.GetCast().GetBy().GetId())
						require.Equal(t, "dnd5e:spells:bless", act.GetCast().GetSpell().GetRef())
						targets = append(targets, result.GetTarget().GetId())
					}
				}
				for _, result := range act.GetActivate().GetResults() {
					if result.GetKind() == "condition-applied" {
						require.Equal(t, id, result.GetSourceId())
						targets = append(targets, result.GetTarget().GetId())
					}
				}
			}
			require.Equal(t, []string{"skel-1", "alice", id}, targets)
			reopenClericHost(t, h, policy)
			requireLiveIsStory(ctx, t, h, castSessionID, id, live)
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
			live := watchCastActs(ctx, t, h, "bella", func() {
				_, err = h.handler.Cast(ctx, &sessionpb.CastRequest{Session: castSessionID, Member: "bella", DeclarationId: row.GetId(), Targets: []string{"alice"}})
				require.NoError(t, err)
			})
			var healed *storypb.ActivationResult
			for _, act := range live {
				for _, result := range act.GetActivate().GetResults() {
					if result.GetKind() == "healing-applied" {
						healed = result
					}
				}
			}
			require.NotNil(t, healed)
			require.Equal(t, "alice", healed.GetTarget().GetId())
			require.Equal(t, row.GetSpell().GetRef(), healed.GetRef())
			require.Equal(t, int32(1), healed.GetBefore())
			require.Positive(t, healed.GetAmount())
			require.Equal(t, healed.GetBefore()+healed.GetAmount(), healed.GetAfter())
			require.NotNil(t, healed.GetRoll())
		})
	}
}
