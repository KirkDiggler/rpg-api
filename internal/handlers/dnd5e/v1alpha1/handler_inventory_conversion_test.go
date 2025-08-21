package v1alpha1_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v1alpha1"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/races"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

func TestConvertCharacterDataToProto_PopulatesInventory(t *testing.T) {
	// Given a character with equipment
	charData := &toolkitchar.Data{
		ID:       "test-char-1",
		PlayerID: "player-1",
		Name:     "Test Character",
		Level:    1,
		RaceID:   races.Human,
		ClassID:  classes.Fighter,
		Equipment: []string{
			"longsword",
			"chain-mail",
			"shield",
			"adventurers-pack",
			"handaxe",
			"handaxe", // Second handaxe
		},
		AbilityScores: shared.AbilityScores{
			abilities.STR: 15,
			abilities.DEX: 14,
			abilities.CON: 13,
			abilities.INT: 12,
			abilities.WIS: 11,
			abilities.CHA: 10,
		},
		MaxHitPoints: 12,
		HitPoints:    12,
		Speed:        30,
		Size:         "Medium",
		Languages:    []string{"Common"},
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	// When converting to proto
	protoChar := v1alpha1.ConvertCharacterDataToProto(charData)

	// Then inventory should be populated
	assert.NotNil(t, protoChar)
	assert.NotNil(t, protoChar.Inventory)
	assert.Len(t, protoChar.Inventory, 6, "Should have 6 inventory items")

	// Verify each item
	expectedItems := []string{
		"longsword",
		"chain-mail",
		"shield",
		"adventurers-pack",
		"handaxe",
		"handaxe",
	}

	for i, expectedID := range expectedItems {
		assert.Equal(t, expectedID, protoChar.Inventory[i].ItemId,
			"Item %d should have correct ID", i)
		assert.Equal(t, int32(1), protoChar.Inventory[i].Quantity,
			"Item %d should have quantity 1", i)
	}
}

func TestConvertCharacterDataToProto_EmptyInventory(t *testing.T) {
	// Given a character with no equipment
	charData := &toolkitchar.Data{
		ID:        "test-char-2",
		PlayerID:  "player-2",
		Name:      "No Equipment Character",
		Level:     1,
		RaceID:    races.Elf,
		ClassID:   classes.Wizard,
		Equipment: []string{}, // Empty equipment
		AbilityScores: shared.AbilityScores{
			abilities.STR: 8,
			abilities.DEX: 14,
			abilities.CON: 12,
			abilities.INT: 16,
			abilities.WIS: 13,
			abilities.CHA: 10,
		},
		MaxHitPoints: 6,
		HitPoints:    6,
		Speed:        30,
		Size:         "Medium",
		Languages:    []string{"Common", "Elvish"},
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	// When converting to proto
	protoChar := v1alpha1.ConvertCharacterDataToProto(charData)

	// Then inventory should be empty but not nil
	assert.NotNil(t, protoChar)
	assert.NotNil(t, protoChar.Inventory)
	assert.Len(t, protoChar.Inventory, 0, "Should have empty inventory")
}

func TestConvertCharacterDataToProto_NilCharacter(t *testing.T) {
	// When converting nil character
	protoChar := v1alpha1.ConvertCharacterDataToProto(nil)

	// Then should return nil
	assert.Nil(t, protoChar, "Should return nil for nil input")
}
