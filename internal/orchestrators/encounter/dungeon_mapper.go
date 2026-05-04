package encounter

import (
	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
)

// MapToGeneratorInputParams contains parameters for mapping to dungeon generator input
type MapToGeneratorInputParams struct {
	PartySize  int    // Number of players
	PartyLevel int    // Average party level (defaults to 1)
	ThemeID    string // Theme identifier (crypt, cave, bandit-lair)
	Difficulty string // Difficulty level (easy, medium, hard, deadly)
	Length     string // Dungeon length (short, medium, long)
	Seed       int64  // Optional seed for reproducibility
}

// MapToGeneratorInput converts orchestrator parameters to dungeon generator input
func MapToGeneratorInput(params *MapToGeneratorInputParams) *dungeon.GenerateInput {
	// Default party level
	partyLevel := params.PartyLevel
	if partyLevel <= 0 {
		partyLevel = 1
	}

	return &dungeon.GenerateInput{
		Theme:     mapTheme(params.ThemeID),
		Size:      dungeon.RoomSizeMedium,
		Length:    mapLength(params.Length),
		Layout:    dungeon.LayoutBranching,
		PartySize: params.PartySize,
		TargetCR:  mapDifficulty(params.Difficulty, partyLevel),
		Seed:      params.Seed,
	}
}

// mapTheme converts a theme ID string to a dungeon.Theme
func mapTheme(themeID string) dungeon.Theme {
	switch themeID {
	case "cave":
		return dungeon.ThemeCave
	case "bandit-lair":
		return dungeon.ThemeBanditLair
	case "crypt":
		return dungeon.ThemeCrypt
	case "debug-walls":
		return dungeon.ThemeDebugWalls
	default:
		return dungeon.ThemeCrypt // Default
	}
}

// mapDifficulty converts difficulty string and party level to target CR
// Based on D&D 5e encounter building:
// - Easy: CR = partyLevel - 1 (min 1)
// - Medium: CR = partyLevel
// - Hard: CR = partyLevel + 1
// - Deadly: CR = partyLevel + 2
func mapDifficulty(difficulty string, partyLevel int) int {
	switch difficulty {
	case "easy":
		cr := partyLevel - 1
		if cr < 1 {
			cr = 1
		}
		return cr
	case "medium":
		return partyLevel
	case "hard":
		return partyLevel + 1
	case "deadly":
		return partyLevel + 2
	default:
		// Default to easy (CR 1)
		return 1
	}
}

// mapLength converts length string to room count
func mapLength(length string) int {
	switch length {
	case "short":
		return 3
	case "medium":
		return 5
	case "long":
		return 8
	default:
		return 5 // Default to medium
	}
}
