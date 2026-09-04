package characterdraft_test

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/KirkDiggler/rpg-api/internal/entities"
	"github.com/KirkDiggler/rpg-api/internal/pkg/clock"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	characterdraft "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/customization"
)

func TestRedisAppearanceRoundTripStoresToolkitDraftShape(t *testing.T) {
	ctx := context.Background()
	repo := newRedisDraftRepository(t)

	color := uint32(0x123456)
	roughness := float32(0.33)
	primary := uint32(0)
	secondary := uint32(0xFFFFFF)
	appearance := &customization.Appearance{
		Hair: &customization.HairCustomization{
			Scalp:      &customization.StyleSelection{Kind: customization.StyleSelectionStyle, StyleRef: "unknown:hair:38"},
			FacialHair: &customization.StyleSelection{Kind: customization.StyleSelectionNone},
			ColorSRGB:  &color,
			Roughness:  &roughness,
		},
		Outfit: &customization.OutfitCustomization{
			PrimaryColorSRGB:   &primary,
			SecondaryColorSRGB: &secondary,
		},
	}

	_, err := repo.Create(ctx, characterdraft.CreateInput{Draft: &entities.CharacterDraft{
		Data: &tkcharacter.DraftData{ID: "draft-appearance", PlayerID: "player-appearance", Appearance: appearance},
	}})
	require.NoError(t, err)

	reloaded, err := repo.Get(ctx, characterdraft.GetInput{ID: "draft-appearance"})
	require.NoError(t, err)
	require.Equal(t, appearance, reloaded.Draft.Data.Appearance)
	requireDetachedAppearance(t, appearance, reloaded.Draft.Data.Appearance)
}

func TestRedisAppearanceRoundTripPreservesPresentZeroOptionals(t *testing.T) {
	ctx := context.Background()
	repo := newRedisDraftRepository(t)
	zeroColor := uint32(0)
	zeroRoughness := float32(0)
	appearance := &customization.Appearance{Hair: &customization.HairCustomization{
		ColorSRGB: &zeroColor,
		Roughness: &zeroRoughness,
	}}

	_, err := repo.Create(ctx, characterdraft.CreateInput{Draft: &entities.CharacterDraft{
		Data: &tkcharacter.DraftData{ID: "draft-zero-appearance", PlayerID: "player-zero-appearance", Appearance: appearance},
	}})
	require.NoError(t, err)

	reloaded, err := repo.Get(ctx, characterdraft.GetInput{ID: "draft-zero-appearance"})
	require.NoError(t, err)
	require.NotNil(t, reloaded.Draft.Data.Appearance.Hair.ColorSRGB)
	require.Zero(t, *reloaded.Draft.Data.Appearance.Hair.ColorSRGB)
	require.NotNil(t, reloaded.Draft.Data.Appearance.Hair.Roughness)
	require.Zero(t, *reloaded.Draft.Data.Appearance.Hair.Roughness)
	requireDetachedAppearance(t, appearance, reloaded.Draft.Data.Appearance)
}

func newRedisDraftRepository(t *testing.T) characterdraft.Repository {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	repo, err := characterdraft.NewRedis(&characterdraft.Config{
		Client:      client,
		Clock:       clock.New(),
		IDGenerator: idgen.NewPrefixed("test-draft-"),
	})
	require.NoError(t, err)
	return repo
}

func requireDetachedAppearance(t *testing.T, input, reloaded *customization.Appearance) {
	t.Helper()
	require.NotSame(t, input, reloaded)
	if input.Hair != nil {
		require.NotSame(t, input.Hair, reloaded.Hair)
		if input.Hair.Scalp != nil {
			require.NotSame(t, input.Hair.Scalp, reloaded.Hair.Scalp)
		}
		if input.Hair.FacialHair != nil {
			require.NotSame(t, input.Hair.FacialHair, reloaded.Hair.FacialHair)
		}
		if input.Hair.ColorSRGB != nil {
			require.NotSame(t, input.Hair.ColorSRGB, reloaded.Hair.ColorSRGB)
		}
		if input.Hair.Roughness != nil {
			require.NotSame(t, input.Hair.Roughness, reloaded.Hair.Roughness)
		}
	}
	if input.Outfit != nil {
		require.NotSame(t, input.Outfit, reloaded.Outfit)
		if input.Outfit.PrimaryColorSRGB != nil {
			require.NotSame(t, input.Outfit.PrimaryColorSRGB, reloaded.Outfit.PrimaryColorSRGB)
		}
		if input.Outfit.SecondaryColorSRGB != nil {
			require.NotSame(t, input.Outfit.SecondaryColorSRGB, reloaded.Outfit.SecondaryColorSRGB)
		}
	}
}
