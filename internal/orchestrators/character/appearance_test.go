package character

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterdraft "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
	draftmock "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft/mock"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/customization"
)

func TestSetAppearance_MutatesToolkitDraftOnceAndReturnsStoredDraft(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := draftmock.NewMockRepository(ctrl)
	orch := newAppearanceTestOrchestrator(t, repo)

	stored := &entities.CharacterDraft{Data: &character.DraftData{
		ID:       "draft-1",
		PlayerID: "player-1",
		Name:     "Before",
	}}
	appearance := &customization.Appearance{
		Outfit: &customization.OutfitCustomization{PrimaryColorSRGB: uint32Ptr(0x102030)},
	}

	repo.EXPECT().Get(context.Background(), characterdraft.GetInput{ID: "draft-1"}).Return(&characterdraft.GetOutput{
		Draft: stored,
	}, nil)
	repo.EXPECT().Update(context.Background(), gomock.Any()).DoAndReturn(
		func(_ context.Context, input characterdraft.UpdateInput) (*characterdraft.UpdateOutput, error) {
			require.Equal(t, uint32(0x102030), *input.Draft.Data.Appearance.Outfit.PrimaryColorSRGB)
			require.Equal(t, "Before", input.Draft.Data.Name)
			return &characterdraft.UpdateOutput{Draft: &entities.CharacterDraft{Data: &character.DraftData{
				ID:         "draft-1",
				PlayerID:   "player-1",
				Name:       "Stored",
				Appearance: input.Draft.Data.Appearance,
			}}}, nil
		},
	)

	out, err := orch.SetAppearance(context.Background(), &SetAppearanceInput{
		DraftID:    "draft-1",
		Appearance: appearance,
	})
	require.NoError(t, err)
	require.Equal(t, "Stored", out.Draft.Name)
	require.Equal(t, uint32(0x102030), *out.Draft.Appearance.Outfit.PrimaryColorSRGB)
}

func TestSetAppearance_RefusesToolkitErrorBeforeRepositoryUpdate(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := draftmock.NewMockRepository(ctrl)
	orch := newAppearanceTestOrchestrator(t, repo)

	repo.EXPECT().Get(context.Background(), characterdraft.GetInput{ID: "draft-1"}).Return(&characterdraft.GetOutput{
		Draft: &entities.CharacterDraft{Data: &character.DraftData{ID: "draft-1", PlayerID: "player-1"}},
	}, nil)

	_, err := orch.SetAppearance(context.Background(), &SetAppearanceInput{
		DraftID: "draft-1",
		Appearance: &customization.Appearance{Hair: &customization.HairCustomization{
			Scalp: &customization.StyleSelection{},
		}},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "selection is required")
}

func newAppearanceTestOrchestrator(t *testing.T, repo characterdraft.Repository) *Orchestrator {
	t.Helper()
	return &Orchestrator{draftRepo: repo}
}

func uint32Ptr(value uint32) *uint32 {
	return &value
}
