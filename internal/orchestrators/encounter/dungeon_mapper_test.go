package encounter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
)

func TestMapToGeneratorInput_DefaultValues(t *testing.T) {
	// When no specific options are provided, should use sensible defaults
	input := &MapToGeneratorInputParams{
		PartySize: 4,
	}

	result := MapToGeneratorInput(input)

	require.NotNil(t, result)
	assert.Equal(t, 4, result.PartySize)
	assert.Equal(t, dungeon.ThemeCrypt, result.Theme) // Default theme
	assert.Equal(t, 5, result.Length)                 // Default: 5 rooms
	assert.Equal(t, 1, result.TargetCR)               // Default: CR 1
	assert.Equal(t, dungeon.RoomSizeMedium, result.Size)
	assert.Equal(t, dungeon.LayoutBranching, result.Layout)
}

func TestMapToGeneratorInput_ThemeCrypt(t *testing.T) {
	input := &MapToGeneratorInputParams{
		PartySize: 4,
		ThemeID:   "crypt",
	}

	result := MapToGeneratorInput(input)

	require.NotNil(t, result)
	// TODO: swap back to ThemeCrypt after wall UI testing
	assert.Equal(t, dungeon.ThemeDebugWalls, result.Theme)
}

func TestMapToGeneratorInput_ThemeCave(t *testing.T) {
	input := &MapToGeneratorInputParams{
		PartySize: 4,
		ThemeID:   "cave",
	}

	result := MapToGeneratorInput(input)

	require.NotNil(t, result)
	assert.Equal(t, dungeon.ThemeCave, result.Theme)
}

func TestMapToGeneratorInput_ThemeBanditLair(t *testing.T) {
	input := &MapToGeneratorInputParams{
		PartySize: 4,
		ThemeID:   "bandit-lair",
	}

	result := MapToGeneratorInput(input)

	require.NotNil(t, result)
	assert.Equal(t, dungeon.ThemeBanditLair, result.Theme)
}

func TestMapToGeneratorInput_DifficultyEasy(t *testing.T) {
	input := &MapToGeneratorInputParams{
		PartySize:  4,
		Difficulty: "easy",
	}

	result := MapToGeneratorInput(input)

	require.NotNil(t, result)
	// Easy difficulty: target CR = party average level - 1 (minimum 1)
	// For default party, use CR 1
	assert.Equal(t, 1, result.TargetCR)
}

func TestMapToGeneratorInput_DifficultyMedium(t *testing.T) {
	input := &MapToGeneratorInputParams{
		PartySize:  4,
		Difficulty: "medium",
	}

	result := MapToGeneratorInput(input)

	require.NotNil(t, result)
	// Medium difficulty: target CR = party level (default 1)
	assert.Equal(t, 1, result.TargetCR)
}

func TestMapToGeneratorInput_DifficultyHard(t *testing.T) {
	input := &MapToGeneratorInputParams{
		PartySize:  4,
		Difficulty: "hard",
	}

	result := MapToGeneratorInput(input)

	require.NotNil(t, result)
	// Hard difficulty: target CR = party level + 1 (default level 1 + 1 = 2)
	assert.Equal(t, 2, result.TargetCR)
}

func TestMapToGeneratorInput_DifficultyDeadly(t *testing.T) {
	input := &MapToGeneratorInputParams{
		PartySize:  4,
		Difficulty: "deadly",
	}

	result := MapToGeneratorInput(input)

	require.NotNil(t, result)
	// Deadly difficulty: target CR = party level + 2 (default level 1 + 2 = 3)
	assert.Equal(t, 3, result.TargetCR)
}

func TestMapToGeneratorInput_LengthShort(t *testing.T) {
	input := &MapToGeneratorInputParams{
		PartySize: 4,
		Length:    "short",
	}

	result := MapToGeneratorInput(input)

	require.NotNil(t, result)
	assert.Equal(t, 3, result.Length) // Short: 3 rooms
}

func TestMapToGeneratorInput_LengthMedium(t *testing.T) {
	input := &MapToGeneratorInputParams{
		PartySize: 4,
		Length:    "medium",
	}

	result := MapToGeneratorInput(input)

	require.NotNil(t, result)
	assert.Equal(t, 5, result.Length) // Medium: 5 rooms
}

func TestMapToGeneratorInput_LengthLong(t *testing.T) {
	input := &MapToGeneratorInputParams{
		PartySize: 4,
		Length:    "long",
	}

	result := MapToGeneratorInput(input)

	require.NotNil(t, result)
	assert.Equal(t, 8, result.Length) // Long: 8 rooms
}

func TestMapToGeneratorInput_CustomPartyLevel(t *testing.T) {
	input := &MapToGeneratorInputParams{
		PartySize:  4,
		PartyLevel: 5,
		Difficulty: "medium",
	}

	result := MapToGeneratorInput(input)

	require.NotNil(t, result)
	// Medium difficulty at level 5: target CR = 5
	assert.Equal(t, 5, result.TargetCR)
}

func TestMapToGeneratorInput_PreservesSeed(t *testing.T) {
	input := &MapToGeneratorInputParams{
		PartySize: 4,
		Seed:      12345,
	}

	result := MapToGeneratorInput(input)

	require.NotNil(t, result)
	assert.Equal(t, int64(12345), result.Seed)
}
