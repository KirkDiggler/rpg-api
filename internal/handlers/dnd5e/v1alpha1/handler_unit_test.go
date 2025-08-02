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

func TestUpdateRace_PreservesID(t *testing.T) {
	// GIVEN
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCharService := charactermock.NewMockService(ctrl)

	handler, err := v1alpha1.NewHandler(&v1alpha1.HandlerConfig{
		CharacterService: mockCharService,
	})
	require.NoError(t, err)

	draftID := "draft-123"
	playerID := "player-456"
	characterName := "Test Character"

	// Mock the orchestrator to return a draft with ID
	mockDraft := &toolkitchar.DraftData{
		ID:        draftID,
		PlayerID:  playerID,
		Name:      characterName,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		RaceChoice: toolkitchar.RaceChoice{
			RaceID:    constants.RaceElf,
			SubraceID: "SUBRACE_HIGH_ELF",
		},
	}

	mockCharService.EXPECT().
		UpdateRace(ctx, &character.UpdateRaceInput{
			DraftID:   draftID,
			RaceID:    "RACE_ELF",
			SubraceID: "SUBRACE_HIGH_ELF",
			Choices:   nil,
		}).
		Return(&character.UpdateRaceOutput{
			Draft:    mockDraft,
			Warnings: []character.ValidationWarning{},
		}, nil)

	// WHEN
	req := &dnd5ev1alpha1.UpdateRaceRequest{
		DraftId: draftID,
		Race:    dnd5ev1alpha1.Race_RACE_ELF,
		Subrace: dnd5ev1alpha1.Subrace_SUBRACE_HIGH_ELF,
	}

	resp, err := handler.UpdateRace(ctx, req)

	// THEN
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Draft)

	// The critical assertion - ID should be preserved
	assert.Equal(t, draftID, resp.Draft.Id, "Draft ID should be preserved after update")
	assert.Equal(t, playerID, resp.Draft.PlayerId)
	assert.Equal(t, characterName, resp.Draft.Name)
	assert.Equal(t, dnd5ev1alpha1.Race_RACE_ELF, resp.Draft.RaceId)
	assert.Equal(t, dnd5ev1alpha1.Subrace_SUBRACE_HIGH_ELF, resp.Draft.SubraceId)
}
