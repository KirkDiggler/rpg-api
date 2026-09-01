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
)

func TestRedisAppearanceRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := newRedisDraftRepository(t)

	_, err := repo.Create(ctx, characterdraft.CreateInput{Draft: &entities.CharacterDraft{
		Data: &tkcharacter.DraftData{ID: "draft-hair", PlayerID: "player-hair"},
	}})
	require.NoError(t, err)

	color := uint32(0x123456)
	roughness := float32(0.33)
	appearance := &entities.Appearance{Hair: &entities.HairCustomization{
		Scalp: &entities.StyleSelection{
			Kind:     entities.StyleSelectionKindStyle,
			StyleRef: "modular-fantasy-hero:hair:38",
		},
		FacialHair: &entities.StyleSelection{Kind: entities.StyleSelectionKindNone},
		ColorSRGB:  &color,
		Roughness:  &roughness,
	}}

	_, err = repo.Update(ctx, characterdraft.UpdateInput{Draft: &entities.CharacterDraft{
		Data:       &tkcharacter.DraftData{ID: "draft-hair", PlayerID: "player-hair"},
		Appearance: appearance,
	}})
	require.NoError(t, err)

	reloaded, err := repo.Get(ctx, characterdraft.GetInput{ID: "draft-hair"})
	require.NoError(t, err)
	require.Equal(t, appearance, reloaded.Draft.Appearance)
	requireDetachedAppearance(t, appearance, reloaded.Draft.Appearance)
}

func TestRedisAppearanceRoundTripPreservesPresentZeroOptionals(t *testing.T) {
	ctx := context.Background()
	repo := newRedisDraftRepository(t)
	zeroColor := uint32(0)
	zeroRoughness := float32(0)
	appearance := &entities.Appearance{Hair: &entities.HairCustomization{
		ColorSRGB: &zeroColor,
		Roughness: &zeroRoughness,
	}}

	_, err := repo.Create(ctx, characterdraft.CreateInput{Draft: &entities.CharacterDraft{
		Data:       &tkcharacter.DraftData{ID: "draft-zero-hair", PlayerID: "player-zero-hair"},
		Appearance: appearance,
	}})
	require.NoError(t, err)

	reloaded, err := repo.Get(ctx, characterdraft.GetInput{ID: "draft-zero-hair"})
	require.NoError(t, err)
	require.NotNil(t, reloaded.Draft.Appearance.Hair.ColorSRGB)
	require.Zero(t, *reloaded.Draft.Appearance.Hair.ColorSRGB)
	require.NotNil(t, reloaded.Draft.Appearance.Hair.Roughness)
	require.Zero(t, *reloaded.Draft.Appearance.Hair.Roughness)
	requireDetachedAppearance(t, appearance, reloaded.Draft.Appearance)
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

func requireDetachedAppearance(t *testing.T, input, reloaded *entities.Appearance) {
	t.Helper()
	require.NotSame(t, input, reloaded)
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
