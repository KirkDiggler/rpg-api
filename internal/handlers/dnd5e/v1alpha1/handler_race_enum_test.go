package v1alpha1_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/orchestrators/character/mock"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/constants"
)

func TestUpdateRace_DragonbornRaceEnum(t *testing.T) {
	// This test specifically checks that RACE_DRAGONBORN (enum value 1)
	// is properly converted and returned in the response

	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCharService := charactermock.NewMockService(ctrl)

	handler, err := v1alpha1.NewHandler(&v1alpha1.HandlerConfig{
		CharacterService: mockCharService,
	})
	require.NoError(t, err)

	draftID := "draft_8d873e24-ec52-492c-b235-4c028aca7e0b"
	playerID := "test-player"

	// Mock the orchestrator to return a draft with Dragonborn race
	mockDraft := &toolkitchar.DraftData{
		ID:        draftID,
		PlayerID:  playerID,
		Name:      "",
		CreatedAt: time.Unix(1754072213, 0),
		UpdatedAt: time.Unix(1754072221, 0),
		RaceChoice: toolkitchar.RaceChoice{
			RaceID:    constants.RaceDragonborn, // Use the actual constant "dragonborn"
			SubraceID: "",
		},
		Choices: []toolkitchar.ChoiceData{
			{
				Category:          "languages",
				Source:            "race",
				ChoiceID:          "language_choice",
				LanguageSelection: []constants.Language{"goblin"},
			},
		},
	}

	mockCharService.EXPECT().
		UpdateRace(ctx, gomock.Any()).
		Return(&character.UpdateRaceOutput{
			Draft:    mockDraft,
			Warnings: []character.ValidationWarning{},
		}, nil)

	// WHEN - Update race to Dragonborn (enum value 1)
	req := &dnd5ev1alpha1.UpdateRaceRequest{
		DraftId: draftID,
		Race:    dnd5ev1alpha1.Race_RACE_DRAGONBORN, // This is enum value 1
		RaceChoices: []*dnd5ev1alpha1.ChoiceSelection{
			{
				ChoiceId:     "language_choice",
				ChoiceType:   dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_UNSPECIFIED,
				Source:       dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_RACE,
				SelectedKeys: []string{"goblin"},
			},
		},
	}

	resp, err := handler.UpdateRace(ctx, req)

	// THEN
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Draft)

	// The critical assertion - raceId should be 1 (RACE_DRAGONBORN), not 0
	assert.Equal(t, dnd5ev1alpha1.Race_RACE_DRAGONBORN, resp.Draft.RaceId,
		"Draft should have RACE_DRAGONBORN (1) not RACE_UNSPECIFIED (0)")

	// Also verify the draft has the expected progress
	assert.True(t, resp.Draft.Progress.HasRace, "Progress should show race is set")
	assert.Equal(t, int32(20), resp.Draft.Progress.CompletionPercentage)
}

func TestRaceConversion_AllRaces(t *testing.T) {
	// Test that all race conversions work properly
	testCases := []struct {
		name          string
		toolkitRaceID constants.Race
		expectedEnum  dnd5ev1alpha1.Race
	}{
		{"Dragonborn", constants.RaceDragonborn, dnd5ev1alpha1.Race_RACE_DRAGONBORN},
		{"Dwarf", constants.RaceDwarf, dnd5ev1alpha1.Race_RACE_DWARF},
		{"Elf", constants.RaceElf, dnd5ev1alpha1.Race_RACE_ELF},
		{"Gnome", constants.RaceGnome, dnd5ev1alpha1.Race_RACE_GNOME},
		{"Half-Elf", constants.RaceHalfElf, dnd5ev1alpha1.Race_RACE_HALF_ELF},
		{"Halfling", constants.RaceHalfling, dnd5ev1alpha1.Race_RACE_HALFLING},
		{"Half-Orc", constants.RaceHalfOrc, dnd5ev1alpha1.Race_RACE_HALF_ORC},
		{"Human", constants.RaceHuman, dnd5ev1alpha1.Race_RACE_HUMAN},
		{"Tiefling", constants.RaceTiefling, dnd5ev1alpha1.Race_RACE_TIEFLING},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockCharService := charactermock.NewMockService(ctrl)
			handler, err := v1alpha1.NewHandler(&v1alpha1.HandlerConfig{
				CharacterService: mockCharService,
			})
			require.NoError(t, err)

			// Mock draft with the specific race
			mockDraft := &toolkitchar.DraftData{
				ID:       "test-draft",
				PlayerID: "test-player",
				RaceChoice: toolkitchar.RaceChoice{
					RaceID: tc.toolkitRaceID,
				},
			}

			mockCharService.EXPECT().
				UpdateRace(ctx, gomock.Any()).
				Return(&character.UpdateRaceOutput{
					Draft: mockDraft,
				}, nil)

			req := &dnd5ev1alpha1.UpdateRaceRequest{
				DraftId: "test-draft",
				Race:    tc.expectedEnum,
			}

			resp, err := handler.UpdateRace(ctx, req)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedEnum, resp.Draft.RaceId,
				"Race %s should convert to enum value %d", tc.name, tc.expectedEnum)
		})
	}
}
