//go:build integration
// +build integration

package external_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/KirkDiggler/rpg-api/internal/clients/external"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/constants"
)

func TestGetRaceData_Integration(t *testing.T) {
	// Skip if not running integration tests
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	client, err := external.New(&external.Config{})
	require.NoError(t, err)

	ctx := context.Background()

	testCases := []struct {
		name     string
		raceID   string
		wantName string
	}{
		{
			name:     "dragonborn",
			raceID:   string(constants.RaceDragonborn),
			wantName: "Dragonborn",
		},
		{
			name:     "half-elf",
			raceID:   string(constants.RaceHalfElf),
			wantName: "Half-Elf",
		},
		{
			name:     "human",
			raceID:   string(constants.RaceHuman),
			wantName: "Human",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			output, err := client.GetRaceData(ctx, tc.raceID)
			require.NoError(t, err)
			require.NotNil(t, output)
			require.NotNil(t, output.RaceData)
			require.NotNil(t, output.UIData)

			// Verify the ID is preserved in our format
			assert.Equal(t, constants.Race(tc.raceID), output.RaceData.ID)
			// Verify we got the right race
			assert.Equal(t, tc.wantName, output.RaceData.Name)
			// Verify we have some data
			assert.NotEmpty(t, output.RaceData.AbilityScoreIncreases)
			// Verify UI data is present
			assert.NotNil(t, output.UIData)
			assert.NotEmpty(t, output.UIData.SizeDescription)
		})
	}
}

func TestGetClassData_Integration(t *testing.T) {
	// Skip if not running integration tests
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	client, err := external.New(&external.Config{})
	require.NoError(t, err)

	ctx := context.Background()

	testCases := []struct {
		name        string
		classID     string
		wantName    string
		wantHitDice int
	}{
		{
			name:        "wizard",
			classID:     string(constants.ClassWizard),
			wantName:    "Wizard",
			wantHitDice: 6,
		},
		{
			name:        "fighter",
			classID:     string(constants.ClassFighter),
			wantName:    "Fighter",
			wantHitDice: 10,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			output, err := client.GetClassData(ctx, tc.classID)
			require.NoError(t, err)
			require.NotNil(t, output)
			require.NotNil(t, output.ClassData)
			require.NotNil(t, output.UIData)

			// Verify the ID is preserved in our format
			assert.Equal(t, constants.Class(tc.classID), output.ClassData.ID)
			// Verify we got the right class
			assert.Equal(t, tc.wantName, output.ClassData.Name)
			assert.Equal(t, tc.wantHitDice, output.ClassData.HitDice)
			// Verify we have some data
			assert.NotEmpty(t, output.ClassData.SavingThrows)
			// Verify UI data is present
			assert.NotNil(t, output.UIData)
		})
	}
}
