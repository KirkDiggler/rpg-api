package v1alpha1_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/clients/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
	handler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v1alpha1"
)

type CharacterConversionTestSuite struct {
	suite.Suite
}

func TestCharacterConversionSuite(t *testing.T) {
	suite.Run(t, new(CharacterConversionTestSuite))
}

func (s *CharacterConversionTestSuite) TestConvertCharacterToProto_Complete() {
	// Test converting a complete character with all fields populated
	character := &dnd5e.Character{
		ID:               "char-123",
		Name:             "Thorin Oakenshield",
		Level:            1,
		ExperiencePoints: 0,
		RaceID:           dnd5e.RaceDwarf,
		SubraceID:        dnd5e.SubraceMountainDwarf,
		ClassID:          dnd5e.ClassFighter,
		BackgroundID:     dnd5e.BackgroundSoldier,
		Alignment:        dnd5e.AlignmentLawfulGood,
		AbilityScores: dnd5e.AbilityScores{
			Strength:     18,
			Dexterity:    13,
			Constitution: 16,
			Intelligence: 10,
			Wisdom:       12,
			Charisma:     8,
		},
		CurrentHP: 13,
		TempHP:    0,
		SessionID: "session-123",
		PlayerID:  "player-123",
		CreatedAt: 1234567890,
		UpdatedAt: 1234567900,
		// New fields
		SkillProficiencies: []string{
			dnd5e.SkillAthletics,
			dnd5e.SkillIntimidation,
			dnd5e.SkillSurvival,
		},
		ArmorProficiencies: []string{
			"light armor",
			"medium armor",
			"heavy armor",
			"shields",
		},
		WeaponProficiencies: []string{
			"simple weapons",
			"martial weapons",
		},
		ToolProficiencies: []string{
			"smith's tools",
			"gaming set",
		},
		SavingThrows: []string{
			"strength",
			"constitution",
		},
		Languages: []string{
			dnd5e.LanguageCommon,
			dnd5e.LanguageDwarvish,
			dnd5e.LanguageOrc,
		},
		Equipment: []dnd5e.CharacterEquipment{
			{
				ItemID:   "chain-mail",
				Name:     "Chain Mail",
				Quantity: 1,
				Type:     "armor",
				Equipped: true,
			},
			{
				ItemID:   "shield",
				Name:     "Shield",
				Quantity: 1,
				Type:     "armor",
				Equipped: true,
			},
			{
				ItemID:   "longsword",
				Name:     "Longsword",
				Quantity: 1,
				Type:     "weapon",
				Equipped: true,
			},
			{
				ItemID:   "handaxe",
				Name:     "Handaxe",
				Quantity: 2,
				Type:     "weapon",
				Equipped: false,
			},
		},
	}

	// Use the handler's test helper function
	protoChar := handler.TestConvertCharacterToProto(character)

	// Verify basic fields
	s.Equal("char-123", protoChar.Id)
	s.Equal("Thorin Oakenshield", protoChar.Name)
	s.Equal(int32(1), protoChar.Level)
	s.Equal(int32(0), protoChar.ExperiencePoints)
	s.Equal(dnd5ev1alpha1.Race_RACE_DWARF, protoChar.Race)
	s.Equal(dnd5ev1alpha1.Subrace_SUBRACE_MOUNTAIN_DWARF, protoChar.Subrace)
	s.Equal(dnd5ev1alpha1.Class_CLASS_FIGHTER, protoChar.Class)
	s.Equal(dnd5ev1alpha1.Background_BACKGROUND_SOLDIER, protoChar.Background)
	s.Equal(dnd5ev1alpha1.Alignment_ALIGNMENT_LAWFUL_GOOD, protoChar.Alignment)

	// Verify ability scores
	s.Equal(int32(18), protoChar.AbilityScores.Strength)
	s.Equal(int32(13), protoChar.AbilityScores.Dexterity)
	s.Equal(int32(16), protoChar.AbilityScores.Constitution)
	s.Equal(int32(10), protoChar.AbilityScores.Intelligence)
	s.Equal(int32(12), protoChar.AbilityScores.Wisdom)
	s.Equal(int32(8), protoChar.AbilityScores.Charisma)

	// Verify ability modifiers
	s.Equal(int32(4), protoChar.AbilityModifiers.Strength)     // (18-10)/2 = 4
	s.Equal(int32(1), protoChar.AbilityModifiers.Dexterity)    // (13-10)/2 = 1
	s.Equal(int32(3), protoChar.AbilityModifiers.Constitution) // (16-10)/2 = 3
	s.Equal(int32(0), protoChar.AbilityModifiers.Intelligence) // (10-10)/2 = 0
	s.Equal(int32(1), protoChar.AbilityModifiers.Wisdom)       // (12-10)/2 = 1
	s.Equal(int32(-1), protoChar.AbilityModifiers.Charisma)    // (8-10)/2 = -1

	// Verify combat stats
	s.Equal(int32(13), protoChar.CombatStats.HitPointMaximum)
	s.Equal(int32(10), protoChar.CombatStats.ArmorClass) // TODO: Should be calculated
	s.Equal(int32(1), protoChar.CombatStats.Initiative)  // DEX modifier
	s.Equal(int32(30), protoChar.CombatStats.Speed)      // Default
	s.Equal(int32(2), protoChar.CombatStats.ProficiencyBonus)
	s.Equal("1d8", protoChar.CombatStats.HitDice) // TODO: Should come from class

	// Verify proficiencies
	s.Len(protoChar.Proficiencies.Skills, 3)
	s.Contains(protoChar.Proficiencies.Skills, dnd5ev1alpha1.Skill_SKILL_ATHLETICS)
	s.Contains(protoChar.Proficiencies.Skills, dnd5ev1alpha1.Skill_SKILL_INTIMIDATION)
	s.Contains(protoChar.Proficiencies.Skills, dnd5ev1alpha1.Skill_SKILL_SURVIVAL)

	s.Len(protoChar.Proficiencies.SavingThrows, 2)
	s.Contains(protoChar.Proficiencies.SavingThrows, dnd5ev1alpha1.Ability_ABILITY_STRENGTH)
	s.Contains(protoChar.Proficiencies.SavingThrows, dnd5ev1alpha1.Ability_ABILITY_CONSTITUTION)

	s.Equal([]string{"light armor", "medium armor", "heavy armor", "shields"}, protoChar.Proficiencies.Armor)
	s.Equal([]string{"simple weapons", "martial weapons"}, protoChar.Proficiencies.Weapons)
	s.Equal([]string{"smith's tools", "gaming set"}, protoChar.Proficiencies.Tools)

	// Verify languages
	s.Len(protoChar.Languages, 3)
	s.Contains(protoChar.Languages, dnd5ev1alpha1.Language_LANGUAGE_COMMON)
	s.Contains(protoChar.Languages, dnd5ev1alpha1.Language_LANGUAGE_DWARVISH)
	s.Contains(protoChar.Languages, dnd5ev1alpha1.Language_LANGUAGE_ORC)

	// Verify HP
	s.Equal(int32(13), protoChar.CurrentHitPoints)
	s.Equal(int32(0), protoChar.TemporaryHitPoints)

	// Verify metadata
	s.Equal(int64(1234567890), protoChar.Metadata.CreatedAt)
	s.Equal(int64(1234567900), protoChar.Metadata.UpdatedAt)
	s.Equal("player-123", protoChar.Metadata.PlayerId)
	s.Equal("session-123", protoChar.SessionId)
}

func (s *CharacterConversionTestSuite) TestConvertCharacterToProto_Minimal() {
	// Test converting a character with minimal fields
	character := &dnd5e.Character{
		ID:           "char-456",
		Name:         "Simple Hero",
		Level:        1,
		RaceID:       dnd5e.RaceHuman,
		ClassID:      dnd5e.ClassFighter,
		BackgroundID: dnd5e.BackgroundFolkHero,
		AbilityScores: dnd5e.AbilityScores{
			Strength:     10,
			Dexterity:    10,
			Constitution: 10,
			Intelligence: 10,
			Wisdom:       10,
			Charisma:     10,
		},
		CurrentHP: 10,
		PlayerID:  "player-456",
		// Empty proficiencies and equipment
		SkillProficiencies:  []string{},
		ArmorProficiencies:  []string{},
		WeaponProficiencies: []string{},
		ToolProficiencies:   []string{},
		SavingThrows:        []string{},
		Languages:           []string{},
		Equipment:           []dnd5e.CharacterEquipment{},
	}

	protoChar := handler.TestConvertCharacterToProto(character)

	// Verify basic conversion
	s.Equal("char-456", protoChar.Id)
	s.Equal("Simple Hero", protoChar.Name)

	// Verify all ability modifiers are 0
	s.Equal(int32(0), protoChar.AbilityModifiers.Strength)
	s.Equal(int32(0), protoChar.AbilityModifiers.Dexterity)
	s.Equal(int32(0), protoChar.AbilityModifiers.Constitution)
	s.Equal(int32(0), protoChar.AbilityModifiers.Intelligence)
	s.Equal(int32(0), protoChar.AbilityModifiers.Wisdom)
	s.Equal(int32(0), protoChar.AbilityModifiers.Charisma)

	// Verify empty collections
	s.Empty(protoChar.Proficiencies.Skills)
	s.Empty(protoChar.Proficiencies.SavingThrows)
	s.Empty(protoChar.Proficiencies.Armor)
	s.Empty(protoChar.Proficiencies.Weapons)
	s.Empty(protoChar.Proficiencies.Tools)
	s.Empty(protoChar.Languages)
}

func (s *CharacterConversionTestSuite) TestConvertCharacterToProto_NilCharacter() {
	// Test nil character returns nil
	protoChar := handler.TestConvertCharacterToProto(nil)
	s.Nil(protoChar)
}

func (s *CharacterConversionTestSuite) TestConvertCharacterToProto_InvalidSkills() {
	// Test character with invalid skill IDs that don't map to proto enums
	character := &dnd5e.Character{
		ID:           "char-789",
		Name:         "Test Character",
		Level:        1,
		RaceID:       dnd5e.RaceHuman,
		ClassID:      dnd5e.ClassFighter,
		BackgroundID: dnd5e.BackgroundSoldier,
		AbilityScores: dnd5e.AbilityScores{
			Strength:     15,
			Dexterity:    14,
			Constitution: 13,
			Intelligence: 12,
			Wisdom:       11,
			Charisma:     10,
		},
		CurrentHP: 11,
		PlayerID:  "player-789",
		SkillProficiencies: []string{
			dnd5e.SkillAthletics,
			"invalid-skill", // Invalid skill
			dnd5e.SkillPerception,
			"", // Empty string
		},
		SavingThrows: []string{
			"strength",
			"invalid-ability", // Invalid ability
			"wisdom",
		},
		Languages: []string{
			dnd5e.LanguageCommon,
			"invalid-language", // Invalid language
			dnd5e.LanguageElvish,
		},
	}

	protoChar := handler.TestConvertCharacterToProto(character)

	// Verify only valid skills are included
	s.Len(protoChar.Proficiencies.Skills, 2)
	s.Contains(protoChar.Proficiencies.Skills, dnd5ev1alpha1.Skill_SKILL_ATHLETICS)
	s.Contains(protoChar.Proficiencies.Skills, dnd5ev1alpha1.Skill_SKILL_PERCEPTION)

	// Verify only valid saving throws are included
	s.Len(protoChar.Proficiencies.SavingThrows, 2)
	s.Contains(protoChar.Proficiencies.SavingThrows, dnd5ev1alpha1.Ability_ABILITY_STRENGTH)
	s.Contains(protoChar.Proficiencies.SavingThrows, dnd5ev1alpha1.Ability_ABILITY_WISDOM)

	// Verify only valid languages are included
	s.Len(protoChar.Languages, 2)
	s.Contains(protoChar.Languages, dnd5ev1alpha1.Language_LANGUAGE_COMMON)
	s.Contains(protoChar.Languages, dnd5ev1alpha1.Language_LANGUAGE_ELVISH)
}

func (s *CharacterConversionTestSuite) TestCalculateAbilityModifier() {
	testCases := []struct {
		score    int32
		expected int32
		name     string
	}{
		{1, -5, "score 1"},
		{2, -4, "score 2"},
		{3, -4, "score 3"},
		{4, -3, "score 4"},
		{5, -3, "score 5"},
		{6, -2, "score 6"},
		{7, -2, "score 7"},
		{8, -1, "score 8"},
		{9, -1, "score 9"},
		{10, 0, "score 10"},
		{11, 0, "score 11"},
		{12, 1, "score 12"},
		{13, 1, "score 13"},
		{14, 2, "score 14"},
		{15, 2, "score 15"},
		{16, 3, "score 16"},
		{17, 3, "score 17"},
		{18, 4, "score 18"},
		{19, 4, "score 19"},
		{20, 5, "score 20"},
		{21, 5, "score 21"},
		{22, 6, "score 22"},
		{30, 10, "score 30"},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			// Create a character with the test score for strength
			character := &dnd5e.Character{
				ID:    "test",
				Name:  "Test",
				Level: 1,
				AbilityScores: dnd5e.AbilityScores{
					Strength:     tc.score,
					Dexterity:    10,
					Constitution: 10,
					Intelligence: 10,
					Wisdom:       10,
					Charisma:     10,
				},
			}

			protoChar := handler.TestConvertCharacterToProto(character)
			s.Equal(tc.expected, protoChar.AbilityModifiers.Strength,
				"Ability score %d should have modifier %d", tc.score, tc.expected)
		})
	}
}

func (s *CharacterConversionTestSuite) TestConvertCharacterToProto_DefaultValues() {
	// Test that default/unspecified values are handled correctly
	character := &dnd5e.Character{
		ID:    "char-default",
		Name:  "Default Hero",
		Level: 1,
		// Unspecified race/class/background should map to UNSPECIFIED
		AbilityScores: dnd5e.AbilityScores{
			Strength:     15,
			Dexterity:    14,
			Constitution: 13,
			Intelligence: 12,
			Wisdom:       11,
			Charisma:     10,
		},
		CurrentHP: 10,
		PlayerID:  "player-default",
	}

	protoChar := handler.TestConvertCharacterToProto(character)

	// Verify unspecified enums
	s.Equal(dnd5ev1alpha1.Race_RACE_UNSPECIFIED, protoChar.Race)
	s.Equal(dnd5ev1alpha1.Subrace_SUBRACE_UNSPECIFIED, protoChar.Subrace)
	s.Equal(dnd5ev1alpha1.Class_CLASS_UNSPECIFIED, protoChar.Class)
	s.Equal(dnd5ev1alpha1.Background_BACKGROUND_UNSPECIFIED, protoChar.Background)
	s.Equal(dnd5ev1alpha1.Alignment_ALIGNMENT_UNSPECIFIED, protoChar.Alignment)

	// Verify default combat stats
	s.Equal(int32(10), protoChar.CombatStats.ArmorClass)      // Base AC
	s.Equal(int32(30), protoChar.CombatStats.Speed)           // Default speed
	s.Equal(int32(2), protoChar.CombatStats.ProficiencyBonus) // Level 1 bonus
	s.Equal("1d8", protoChar.CombatStats.HitDice)             // Default hit dice
}

func (s *CharacterConversionTestSuite) TestConvertCharacterToProto_MixedCaseSavingThrows() {
	// Test that saving throws are normalized from various formats
	character := &dnd5e.Character{
		ID:           "char-mixed",
		Name:         "Mixed Case Hero",
		Level:        1,
		RaceID:       dnd5e.RaceHuman,
		ClassID:      dnd5e.ClassFighter,
		BackgroundID: dnd5e.BackgroundSoldier,
		AbilityScores: dnd5e.AbilityScores{
			Strength:     16,
			Dexterity:    14,
			Constitution: 15,
			Intelligence: 10,
			Wisdom:       12,
			Charisma:     8,
		},
		CurrentHP: 12,
		PlayerID:  "player-mixed",
		SavingThrows: []string{
			"STRENGTH",        // Uppercase
			"constitution",    // Lowercase
			"DexTeRiTy",       // Mixed case
			"WISDOM",          // Uppercase
			"unknown-ability", // Invalid
		},
	}

	protoChar := handler.TestConvertCharacterToProto(character)

	// Verify saving throws are normalized correctly
	s.Len(protoChar.Proficiencies.SavingThrows, 4) // Invalid one is filtered out
	s.Contains(protoChar.Proficiencies.SavingThrows, dnd5ev1alpha1.Ability_ABILITY_STRENGTH)
	s.Contains(protoChar.Proficiencies.SavingThrows, dnd5ev1alpha1.Ability_ABILITY_CONSTITUTION)
	s.Contains(protoChar.Proficiencies.SavingThrows, dnd5ev1alpha1.Ability_ABILITY_DEXTERITY)
	s.Contains(protoChar.Proficiencies.SavingThrows, dnd5ev1alpha1.Ability_ABILITY_WISDOM)
}
