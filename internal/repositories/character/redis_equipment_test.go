package character_test

import (
	"context"
	"encoding/json"
	"maps"
	"testing"

	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	coreResources "github.com/KirkDiggler/rpg-toolkit/core/resources"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/customization"
)

func TestPatchEquipment_ConcurrentCombatStateSurvives(t *testing.T) {
	ctx := context.Background()
	repo := newRedisCharacterRepository(t)
	original := repositoryCharacter("char-concurrent")
	_, err := repo.Create(ctx, characterrepo.CreateInput{Character: original})
	require.NoError(t, err)

	before, err := repo.Get(ctx, characterrepo.GetInput{ID: original.Data.ID})
	require.NoError(t, err)
	expectedSlots := maps.Clone(before.Character.Data.EquipmentSlots)

	// This update lands after the equipment caller's initial read, as a combat
	// writer can. The equipment patch must not write the caller's stale full
	// snapshot over these fields.
	concurrent := before.Character
	concurrent.Data.HitPoints = 4
	concurrent.Data.Resources = map[coreResources.ResourceKey]tkcharacter.RecoverableResourceData{
		"rage": {Current: 1, Maximum: 3},
	}
	concurrent.Data.Conditions = []json.RawMessage{json.RawMessage(`{"ref":{"module":"dnd5e","type":"conditions","id":"raging"}}`)}
	concurrent.Data.ActionEconomy = &tkcharacter.ActionEconomyData{
		TurnNumber: 2, ActionsRemaining: 0, ReactionsRemaining: 1, MovementRemaining: 15,
	}
	color := uint32(0x123456)
	roughness := float32(0.33)
	concurrent.Data.Appearance = &customization.Appearance{Hair: &customization.HairCustomization{
		Scalp:      &customization.StyleSelection{Kind: customization.StyleSelectionStyle, StyleRef: "modular-fantasy-hero:hair:38"},
		FacialHair: &customization.StyleSelection{Kind: customization.StyleSelectionNone},
		ColorSRGB:  &color,
		Roughness:  &roughness,
	}}
	_, err = repo.Update(ctx, characterrepo.UpdateInput{Character: concurrent})
	require.NoError(t, err)

	first, err := repo.PatchEquipment(ctx, characterrepo.PatchEquipmentInput{
		CharacterID:            original.Data.ID,
		ExpectedVersion:        before.Version,
		ExpectedEquipmentSlots: expectedSlots,
		EquipmentSlots: tkcharacter.EquipmentSlots{
			tkcharacter.SlotMainHand: "longsword",
		},
		ArmorClass: 16,
	})
	require.NoError(t, err)
	require.False(t, first.Applied, "an unrelated concurrent revision must be returned for strict reprojection before writing")
	require.Equal(t, 4, first.Character.Data.HitPoints)

	patched, err := repo.PatchEquipment(ctx, characterrepo.PatchEquipmentInput{
		CharacterID:            original.Data.ID,
		ExpectedVersion:        first.Version,
		ExpectedEquipmentSlots: maps.Clone(first.Character.Data.EquipmentSlots),
		EquipmentSlots: tkcharacter.EquipmentSlots{
			tkcharacter.SlotMainHand: "longsword",
		},
		ArmorClass: 16,
	})
	require.NoError(t, err)
	require.True(t, patched.Applied)

	stored, err := repo.Get(ctx, characterrepo.GetInput{ID: original.Data.ID})
	require.NoError(t, err)
	require.Equal(t, 4, stored.Character.Data.HitPoints)
	require.Equal(t, concurrent.Data.Resources, stored.Character.Data.Resources)
	require.Equal(t, concurrent.Data.Conditions, stored.Character.Data.Conditions)
	require.Equal(t, concurrent.Data.ActionEconomy, stored.Character.Data.ActionEconomy)
	require.Equal(t, concurrent.Data.Appearance, stored.Character.Data.Appearance)
	require.Equal(t, "longsword", stored.Character.Data.EquipmentSlots.Get(tkcharacter.SlotMainHand))
	require.Equal(t, 16, stored.Character.Data.ArmorClass)
}

func TestPatchEquipment_StaleExpectedEquipmentRefusesAndPreservesNewerData(t *testing.T) {
	ctx := context.Background()
	repo := newRedisCharacterRepository(t)
	original := repositoryCharacter("char-stale-equipment")
	_, err := repo.Create(ctx, characterrepo.CreateInput{Character: original})
	require.NoError(t, err)

	stale, err := repo.Get(ctx, characterrepo.GetInput{ID: original.Data.ID})
	require.NoError(t, err)
	expectedSlots := maps.Clone(stale.Character.Data.EquipmentSlots)

	newer := stale.Character
	newer.Data.EquipmentSlots = tkcharacter.EquipmentSlots{tkcharacter.SlotOffHand: "shield"}
	newer.Data.ArmorClass = 12
	newer.Data.HitPoints = 5
	_, err = repo.Update(ctx, characterrepo.UpdateInput{Character: newer})
	require.NoError(t, err)

	out, err := repo.PatchEquipment(ctx, characterrepo.PatchEquipmentInput{
		CharacterID:            original.Data.ID,
		ExpectedVersion:        stale.Version,
		ExpectedEquipmentSlots: expectedSlots,
		EquipmentSlots:         tkcharacter.EquipmentSlots{tkcharacter.SlotMainHand: "longsword"},
		ArmorClass:             16,
	})
	require.Error(t, err)
	require.Nil(t, out)
	require.True(t, apierr.IsAborted(err), "stale equipment must be an ABORTED conflict, got %v", err)

	stored, err := repo.Get(ctx, characterrepo.GetInput{ID: original.Data.ID})
	require.NoError(t, err)
	require.Equal(t, newer.Data.EquipmentSlots, stored.Character.Data.EquipmentSlots)
	require.Equal(t, 12, stored.Character.Data.ArmorClass)
	require.Equal(t, 5, stored.Character.Data.HitPoints)
}

func newRedisCharacterRepository(t *testing.T) characterrepo.Repository {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	repo, err := characterrepo.NewRedis(&characterrepo.RedisConfig{Client: client})
	require.NoError(t, err)
	return repo
}

func repositoryCharacter(id string) *entities.Character {
	return &entities.Character{Data: &tkcharacter.Data{
		ID:             id,
		PlayerID:       "player-1",
		Name:           "Fighter",
		Level:          3,
		ClassID:        "fighter",
		RaceID:         "human",
		HitPoints:      12,
		MaxHitPoints:   12,
		ArmorClass:     10,
		EquipmentSlots: tkcharacter.EquipmentSlots{},
	}}
}
