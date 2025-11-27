// Package character contains the character orchestrator tests
package character

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListSpellsByLevel_Integration(t *testing.T) {
	// Create a minimal orchestrator for integration testing
	orchestrator := &Orchestrator{}

	testCases := []struct {
		name         string
		level        int
		expectSpells bool
		expectError  bool
	}{
		{
			name:         "cantrips (level 0)",
			level:        0,
			expectSpells: true,
			expectError:  false,
		},
		{
			name:         "level 1 spells",
			level:        1,
			expectSpells: true,
			expectError:  false,
		},
		{
			name:         "level 3 spells",
			level:        3,
			expectSpells: true,
			expectError:  false,
		},
		{
			name:         "invalid level (-1)",
			level:        -1,
			expectSpells: false,
			expectError:  true,
		},
		{
			name:         "invalid level (10)",
			level:        10,
			expectSpells: false,
			expectError:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			input := &ListSpellsByLevelInput{
				Level:    tc.level,
				PageSize: 100, // Get all spells
			}

			result, err := orchestrator.ListSpellsByLevel(context.Background(), input)

			if tc.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)

				if tc.expectSpells {
					assert.Greater(t, len(result.Spells), 0, "Expected some spells for level %d", tc.level)
					assert.Equal(t, len(result.Spells), result.Total)

					// Verify all spells are the correct level
					for i, spell := range result.Spells {
						assert.Equal(t, tc.level, spell.Level, "Spell %d (%s) has wrong level", i, spell.Name)
						assert.NotEmpty(t, spell.ID, "Spell %d should have ID", i)
						assert.NotEmpty(t, spell.Name, "Spell %d should have name", i)
						assert.NotEmpty(t, spell.Description, "Spell %d should have description", i)
					}
				}
			}
		})
	}
}

func TestListSpellsByLevel_SpecificSpells(t *testing.T) {
	orchestrator := &Orchestrator{}

	// Test that we get expected spells
	result, err := orchestrator.ListSpellsByLevel(context.Background(), &ListSpellsByLevelInput{
		Level:    0,
		PageSize: 100,
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Greater(t, len(result.Spells), 0)

	// Check for some known cantrips
	foundFireBolt := false
	foundMageHand := false

	for _, spell := range result.Spells {
		switch spell.ID {
		case "fire-bolt":
			foundFireBolt = true
			assert.Equal(t, "Fire Bolt", spell.Name)
			assert.Equal(t, 0, spell.Level)
		case "mage-hand":
			foundMageHand = true
			assert.Equal(t, "Mage Hand", spell.Name)
			assert.Equal(t, 0, spell.Level)
		}
	}

	assert.True(t, foundFireBolt, "Expected to find Fire Bolt cantrip")
	assert.True(t, foundMageHand, "Expected to find Mage Hand cantrip")
}
