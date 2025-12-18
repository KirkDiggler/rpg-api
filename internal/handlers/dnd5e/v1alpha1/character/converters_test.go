package character

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character/choices"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

type ConvertersTestSuite struct {
	suite.Suite
}

func TestConvertersTestSuite(t *testing.T) {
	suite.Run(t, new(ConvertersTestSuite))
}

func (s *ConvertersTestSuite) TestConvertClassDataToProto_Fighter() {
	// Get Fighter class data from toolkit
	fighterData := classes.ClassData[classes.Fighter]
	require.NotNil(s.T(), fighterData, "Fighter class data should exist")

	// Convert to proto
	result := convertClassDataToProto(fighterData)
	require.NotNil(s.T(), result, "Converted class info should not be nil")

	// Verify basic class info
	assert.Equal(s.T(), dnd5ev1alpha1.Class_CLASS_FIGHTER, result.ClassId)
	assert.Equal(s.T(), "Fighter", result.Name)
	assert.NotEmpty(s.T(), result.Description)
	assert.Equal(s.T(), int32(10), result.HitDie)
	assert.Equal(s.T(), dnd5ev1alpha1.Ability_ABILITY_STRENGTH, result.PrimaryAbility)

	// Verify automatic grants (proficiencies)
	assert.NotEmpty(s.T(), result.ArmorProficiencyCategories, "Fighter should have armor proficiencies")
	assert.Contains(s.T(), result.ArmorProficiencyCategories, dnd5ev1alpha1.ArmorProficiencyCategory_ARMOR_PROFICIENCY_CATEGORY_LIGHT)
	assert.Contains(s.T(), result.ArmorProficiencyCategories, dnd5ev1alpha1.ArmorProficiencyCategory_ARMOR_PROFICIENCY_CATEGORY_MEDIUM)
	assert.Contains(s.T(), result.ArmorProficiencyCategories, dnd5ev1alpha1.ArmorProficiencyCategory_ARMOR_PROFICIENCY_CATEGORY_HEAVY)
	assert.Contains(s.T(), result.ArmorProficiencyCategories, dnd5ev1alpha1.ArmorProficiencyCategory_ARMOR_PROFICIENCY_CATEGORY_SHIELDS)

	assert.NotEmpty(s.T(), result.WeaponProficiencyCategories, "Fighter should have weapon proficiencies")
	assert.Contains(s.T(), result.WeaponProficiencyCategories, dnd5ev1alpha1.WeaponProficiencyCategory_WEAPON_PROFICIENCY_CATEGORY_SIMPLE)
	assert.Contains(s.T(), result.WeaponProficiencyCategories, dnd5ev1alpha1.WeaponProficiencyCategory_WEAPON_PROFICIENCY_CATEGORY_MARTIAL)

	assert.NotEmpty(s.T(), result.SavingThrowProficiencies, "Fighter should have saving throw proficiencies")
	assert.Contains(s.T(), result.SavingThrowProficiencies, dnd5ev1alpha1.Ability_ABILITY_STRENGTH)
	assert.Contains(s.T(), result.SavingThrowProficiencies, dnd5ev1alpha1.Ability_ABILITY_CONSTITUTION)

	// Verify choices are populated
	assert.NotEmpty(s.T(), result.Choices, "Fighter should have choices")

	// Find and verify skill choice
	var skillChoice *dnd5ev1alpha1.Choice
	for _, choice := range result.Choices {
		if choice.ChoiceType == dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS {
			skillChoice = choice
			break
		}
	}
	require.NotNil(s.T(), skillChoice, "Fighter should have a skill choice")
	assert.Equal(s.T(), "fighter-skills", skillChoice.Id)
	assert.Equal(s.T(), "Choose 2 skills", skillChoice.Description)
	assert.Equal(s.T(), int32(2), skillChoice.ChooseCount)

	// Verify skill options
	skillOptions := skillChoice.GetSkillOptions()
	require.NotNil(s.T(), skillOptions, "Skill choice should have skill options")
	assert.NotEmpty(s.T(), skillOptions.Available, "Should have available skills to choose from")
	// Fighter can choose from specific skills like Acrobatics, Animal Handling, Athletics, etc.
	assert.Contains(s.T(), skillOptions.Available, dnd5ev1alpha1.Skill_SKILL_ACROBATICS)
	assert.Contains(s.T(), skillOptions.Available, dnd5ev1alpha1.Skill_SKILL_ATHLETICS)
	assert.Contains(s.T(), skillOptions.Available, dnd5ev1alpha1.Skill_SKILL_INTIMIDATION)

	// Find and verify fighting style choice
	var fightingStyleChoice *dnd5ev1alpha1.Choice
	for _, choice := range result.Choices {
		if choice.ChoiceType == dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_FIGHTING_STYLE {
			fightingStyleChoice = choice
			break
		}
	}
	require.NotNil(s.T(), fightingStyleChoice, "Fighter should have a fighting style choice")
	assert.Equal(s.T(), "fighter-fighting-style", fightingStyleChoice.Id)
	assert.Equal(s.T(), "Choose a fighting style", fightingStyleChoice.Description)
	assert.Equal(s.T(), int32(1), fightingStyleChoice.ChooseCount)

	// Verify fighting style options
	styleOptions := fightingStyleChoice.GetFightingStyleOptions()
	require.NotNil(s.T(), styleOptions, "Fighting style choice should have options")
	assert.NotEmpty(s.T(), styleOptions.Available, "Should have fighting styles to choose from")
	assert.Contains(s.T(), styleOptions.Available, dnd5ev1alpha1.FightingStyle_FIGHTING_STYLE_ARCHERY)
	assert.Contains(s.T(), styleOptions.Available, dnd5ev1alpha1.FightingStyle_FIGHTING_STYLE_DEFENSE)
	assert.Contains(s.T(), styleOptions.Available, dnd5ev1alpha1.FightingStyle_FIGHTING_STYLE_DUELING)

	// Find and verify equipment choices
	equipmentChoices := make([]*dnd5ev1alpha1.Choice, 0)
	for _, choice := range result.Choices {
		if choice.ChoiceType == dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT {
			equipmentChoices = append(equipmentChoices, choice)
		}
	}
	assert.NotEmpty(s.T(), equipmentChoices, "Fighter should have equipment choices")

	// Check one equipment choice in detail (armor choice)
	var armorChoice *dnd5ev1alpha1.Choice
	for _, choice := range equipmentChoices {
		if choice.Id == "fighter-armor" {
			armorChoice = choice
			break
		}
	}
	require.NotNil(s.T(), armorChoice, "Fighter should have armor choice")
	assert.Equal(s.T(), "Choose your armor", armorChoice.Description)
	assert.Equal(s.T(), int32(1), armorChoice.ChooseCount)

	// Verify equipment bundles
	equipOptions := armorChoice.GetEquipmentOptions()
	require.NotNil(s.T(), equipOptions, "Armor choice should have equipment options")
	assert.Len(s.T(), equipOptions.Bundles, 2, "Should have 2 armor options")

	// Check first bundle (chain mail)
	chainMailBundle := equipOptions.Bundles[0]
	assert.Equal(s.T(), "fighter-armor-a", chainMailBundle.Id)
	assert.Equal(s.T(), "Chain mail", chainMailBundle.Label)
	assert.Len(s.T(), chainMailBundle.Items, 1, "Chain mail bundle should have 1 item")
	assert.Equal(s.T(), "chain-mail", chainMailBundle.Items[0].SelectionId)
	assert.Equal(s.T(), int32(1), chainMailBundle.Items[0].Quantity)

	// Check second bundle (leather + longbow + arrows)
	leatherBundle := equipOptions.Bundles[1]
	assert.Equal(s.T(), "fighter-armor-b", leatherBundle.Id)
	assert.Equal(s.T(), "Leather armor, longbow, and 20 arrows", leatherBundle.Label)
	assert.Len(s.T(), leatherBundle.Items, 3, "Leather bundle should have 3 items")
}

func (s *ConvertersTestSuite) TestConvertClassDataToProto_Wizard() {
	// Get Wizard class data from toolkit
	wizardData := classes.ClassData[classes.Wizard]
	require.NotNil(s.T(), wizardData, "Wizard class data should exist")

	// Convert to proto
	result := convertClassDataToProto(wizardData)
	require.NotNil(s.T(), result, "Converted class info should not be nil")

	// Verify basic class info
	assert.Equal(s.T(), dnd5ev1alpha1.Class_CLASS_WIZARD, result.ClassId)
	assert.Equal(s.T(), "Wizard", result.Name)
	assert.Equal(s.T(), int32(6), result.HitDie)
	assert.Equal(s.T(), dnd5ev1alpha1.Ability_ABILITY_INTELLIGENCE, result.PrimaryAbility)

	// Wizards have no armor proficiencies
	assert.Empty(s.T(), result.ArmorProficiencyCategories, "Wizard should have no armor proficiencies")

	// Wizards have limited weapon proficiencies
	assert.NotEmpty(s.T(), result.SpecificWeaponProficiencies, "Wizard should have specific weapon proficiencies")

	// Verify choices
	assert.NotEmpty(s.T(), result.Choices, "Wizard should have choices")

	// Find skill choice
	var skillChoice *dnd5ev1alpha1.Choice
	for _, choice := range result.Choices {
		if choice.ChoiceType == dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS {
			skillChoice = choice
			break
		}
	}
	require.NotNil(s.T(), skillChoice, "Wizard should have a skill choice")
	assert.Equal(s.T(), "wizard-skills", skillChoice.Id)
	assert.Equal(s.T(), int32(2), skillChoice.ChooseCount)

	// Wizard has fewer skill options than Fighter
	skillOptions := skillChoice.GetSkillOptions()
	require.NotNil(s.T(), skillOptions, "Skill choice should have skill options")
	assert.NotEmpty(s.T(), skillOptions.Available, "Should have available skills")
	// Wizards can choose from INT-based skills
	assert.Contains(s.T(), skillOptions.Available, dnd5ev1alpha1.Skill_SKILL_ARCANA)
	assert.Contains(s.T(), skillOptions.Available, dnd5ev1alpha1.Skill_SKILL_HISTORY)
	assert.Contains(s.T(), skillOptions.Available, dnd5ev1alpha1.Skill_SKILL_INVESTIGATION)
}

func (s *ConvertersTestSuite) TestLoadAllClassChoices_NoRequirements() {
	// Test with a class that might not have requirements defined
	result := loadAllClassChoices(classes.Barbarian)
	// Should return empty slice, not nil
	assert.NotNil(s.T(), result, "Should return empty slice for class with no requirements")
}

func (s *ConvertersTestSuite) TestConvertChoiceToProto_PreservesOptionID() {
	// Create toolkit choice with OptionID (for equipment bundles)
	toolkitChoice := choices.ChoiceData{
		Category:           shared.ChoiceEquipment,
		Source:             shared.SourceClass,
		ChoiceID:           "barbarian-weapons-secondary",
		OptionID:           "barbarian-secondary-b",
		EquipmentSelection: []shared.SelectionID{"greatclub"},
	}

	// Convert to proto
	protoChoice := convertChoiceToProto(toolkitChoice)

	// Verify all fields are preserved
	require.NotNil(s.T(), protoChoice, "Proto choice should not be nil")
	assert.Equal(s.T(), dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT, protoChoice.Category)
	assert.Equal(s.T(), dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS, protoChoice.Source)
	assert.Equal(s.T(), "barbarian-weapons-secondary", protoChoice.ChoiceId)
	assert.Equal(s.T(), "barbarian-secondary-b", protoChoice.OptionId, "OptionID should be preserved")

	// Verify equipment selection
	equipment := protoChoice.GetEquipment()
	require.NotNil(s.T(), equipment, "Equipment selection should be set")
	require.Len(s.T(), equipment.Items, 1, "Should have 1 equipment item")
}

func (s *ConvertersTestSuite) TestConvertCharacterDataToProto_WithFeatures() {
	// Create test data with features
	testData := &toolkitchar.Data{
		ID:    "test-char",
		Name:  "Ragnar",
		Level: 5,
		Features: []json.RawMessage{
			json.RawMessage(`{
				"ref": {"value": "rage"},
				"id": "rage-1",
				"name": "Rage",
				"level": 5,
				"uses": 3,
				"max_uses": 3
			}`),
		},
	}

	// Convert to proto
	result := ConvertCharacterDataToProto(testData)

	// Verify features are converted
	require.NotNil(s.T(), result, "Result should not be nil")
	require.NotEmpty(s.T(), result.Features, "Features should be populated")
	assert.Len(s.T(), result.Features, 1, "Should have 1 feature")

	// Check feature details
	rageFeature := result.Features[0]
	assert.Equal(s.T(), "rage-1", rageFeature.Id)
	assert.Equal(s.T(), "Rage", rageFeature.Name)
	assert.Equal(s.T(), "class", rageFeature.Source)
}

func (s *ConvertersTestSuite) TestConvertCharacterDataToProto_EmptyFeatures() {
	// Create test data with no features
	testData := &toolkitchar.Data{
		ID:       "test-char",
		Name:     "Fighter",
		Level:    1,
		Features: nil,
	}

	// Convert to proto
	result := ConvertCharacterDataToProto(testData)

	// Verify no features
	require.NotNil(s.T(), result, "Result should not be nil")
	assert.Empty(s.T(), result.Features, "Features should be empty")
}

func (s *ConvertersTestSuite) TestConvertCharacterDataToProto_InvalidFeatureJSON() {
	// Create test data with invalid feature JSON
	testData := &toolkitchar.Data{
		ID:    "test-char",
		Name:  "Barbarian",
		Level: 1,
		Features: []json.RawMessage{
			json.RawMessage(`{invalid json`),
			json.RawMessage(`{
				"ref": {"value": "rage"},
				"id": "valid-rage",
				"name": "Rage",
				"level": 1
			}`),
		},
	}

	// Convert to proto - should skip invalid feature and include valid one
	result := ConvertCharacterDataToProto(testData)

	// Verify only valid feature is included
	require.NotNil(s.T(), result, "Result should not be nil")
	assert.Len(s.T(), result.Features, 1, "Should have 1 valid feature")
	assert.Equal(s.T(), "valid-rage", result.Features[0].Id)
}

func (s *ConvertersTestSuite) TestConvertCharacterDataToProto_FeatureWithToolkitFormat() {
	// Test actual toolkit format: ref is a string, id and name are top-level
	testData := &toolkitchar.Data{
		ID:    "test-char",
		Name:  "Barbarian",
		Level: 1,
		Features: []json.RawMessage{
			json.RawMessage(`{
				"ref": "dnd5e:features:rage",
				"id": "rage",
				"name": "Rage",
				"level": 1,
				"uses": 2,
				"max_uses": 2
			}`),
		},
	}

	result := ConvertCharacterDataToProto(testData)

	require.NotNil(s.T(), result, "Result should not be nil")
	require.Len(s.T(), result.Features, 1, "Should have 1 feature")
	assert.Equal(s.T(), "rage", result.Features[0].Id)
	assert.Equal(s.T(), "Rage", result.Features[0].Name)
}

func (s *ConvertersTestSuite) TestGetFeatureDisplayName() {
	// Test known feature names
	assert.Equal(s.T(), "Rage", getFeatureDisplayName("rage"))
	assert.Equal(s.T(), "Second Wind", getFeatureDisplayName("second_wind"))
	assert.Equal(s.T(), "Action Surge", getFeatureDisplayName("action_surge"))

	// Test unknown feature - should return the ID
	assert.Equal(s.T(), "unknown_feature", getFeatureDisplayName("unknown_feature"))
}

func (s *ConvertersTestSuite) TestConvertCharacterDataToProto_WithConditions() {
	// Create test data with conditions
	testData := &toolkitchar.Data{
		ID:    "test-char",
		Name:  "Ragnar",
		Level: 5,
		Conditions: []json.RawMessage{
			json.RawMessage(`{
				"id": "raging-1",
				"type": "raging",
				"name": "Raging",
				"source": "rage",
				"duration": 10
			}`),
			json.RawMessage(`{
				"id": "blessed-1",
				"name": "Blessed",
				"source": "bless_spell",
				"duration": 6
			}`),
		},
	}

	// Convert to proto
	result := ConvertCharacterDataToProto(testData)

	// Verify conditions are converted
	require.NotNil(s.T(), result, "Result should not be nil")
	require.NotEmpty(s.T(), result.ActiveConditions, "ActiveConditions should be populated")
	assert.Len(s.T(), result.ActiveConditions, 2, "Should have 2 conditions")

	// Check first condition details (Raging)
	ragingCondition := result.ActiveConditions[0]
	assert.Equal(s.T(), "Raging", ragingCondition.Name)
	assert.Equal(s.T(), "rage", ragingCondition.Source)
	assert.Equal(s.T(), int32(10), ragingCondition.Duration)

	// Check second condition details (Blessed)
	blessedCondition := result.ActiveConditions[1]
	assert.Equal(s.T(), "Blessed", blessedCondition.Name)
	assert.Equal(s.T(), "bless_spell", blessedCondition.Source)
	assert.Equal(s.T(), int32(6), blessedCondition.Duration)
}

func (s *ConvertersTestSuite) TestConvertCharacterDataToProto_EmptyConditions() {
	// Create test data with no conditions
	testData := &toolkitchar.Data{
		ID:         "test-char",
		Name:       "Fighter",
		Level:      1,
		Conditions: nil,
	}

	// Convert to proto
	result := ConvertCharacterDataToProto(testData)

	// Verify no conditions
	require.NotNil(s.T(), result, "Result should not be nil")
	assert.Empty(s.T(), result.ActiveConditions, "ActiveConditions should be empty")
}

func (s *ConvertersTestSuite) TestConvertCharacterDataToProto_InvalidConditionJSON() {
	// Create test data with invalid condition JSON
	testData := &toolkitchar.Data{
		ID:    "test-char",
		Name:  "Barbarian",
		Level: 1,
		Conditions: []json.RawMessage{
			json.RawMessage(`{invalid json`),
			json.RawMessage(`{
				"id": "valid-condition",
				"name": "Poisoned",
				"source": "trap",
				"duration": 3
			}`),
		},
	}

	// Convert to proto - should skip invalid condition and include valid one
	result := ConvertCharacterDataToProto(testData)

	// Verify only valid condition is included
	require.NotNil(s.T(), result, "Result should not be nil")
	assert.Len(s.T(), result.ActiveConditions, 1, "Should have 1 valid condition")
	assert.Equal(s.T(), "Poisoned", result.ActiveConditions[0].Name)
}

func (s *ConvertersTestSuite) TestConvertCharacterDataToProto_ConditionNameFallback() {
	// Create test data with condition missing name but having type
	testData := &toolkitchar.Data{
		ID:    "test-char",
		Name:  "Rogue",
		Level: 3,
		Conditions: []json.RawMessage{
			// Condition with type but no name - should fall back to type
			json.RawMessage(`{
				"id": "poisoned-1",
				"type": "poisoned",
				"source": "trap",
				"duration": 5
			}`),
			// Condition with only id - should fall back to id
			json.RawMessage(`{
				"id": "stunned",
				"source": "spell",
				"duration": 1
			}`),
		},
	}

	// Convert to proto
	result := ConvertCharacterDataToProto(testData)

	// Verify name fallback logic
	require.NotNil(s.T(), result, "Result should not be nil")
	require.Len(s.T(), result.ActiveConditions, 2, "Should have 2 conditions")

	// First condition should use type as name fallback
	assert.Equal(s.T(), "poisoned", result.ActiveConditions[0].Name, "Should use type as fallback name")

	// Second condition should use id as name fallback
	assert.Equal(s.T(), "stunned", result.ActiveConditions[1].Name, "Should use id as fallback name")
}

func (s *ConvertersTestSuite) TestConvertCharacterDataToProto_ConditionWithNoIdentifier() {
	// Create test data with condition having no name, type, or id
	testData := &toolkitchar.Data{
		ID:    "test-char",
		Name:  "Fighter",
		Level: 2,
		Conditions: []json.RawMessage{
			// Condition with no identifiable name - should be skipped
			json.RawMessage(`{
				"source": "unknown",
				"duration": 2
			}`),
			// Valid condition - should be included
			json.RawMessage(`{
				"id": "frightened",
				"name": "Frightened",
				"source": "fear_spell",
				"duration": 10
			}`),
		},
	}

	// Convert to proto
	result := ConvertCharacterDataToProto(testData)

	// Verify condition with no identifier is skipped
	require.NotNil(s.T(), result, "Result should not be nil")
	assert.Len(s.T(), result.ActiveConditions, 1, "Should skip condition with no identifier")
	assert.Equal(s.T(), "Frightened", result.ActiveConditions[0].Name)
}

func (s *ConvertersTestSuite) TestGetFeatureActionType() {
	testCases := []struct {
		name       string
		featureID  string
		wantAction dnd5ev1alpha1.ActionType
	}{
		// Barbarian
		{
			name:       "Rage is bonus action",
			featureID:  "rage",
			wantAction: dnd5ev1alpha1.ActionType_ACTION_TYPE_BONUS_ACTION,
		},
		// Fighter
		{
			name:       "Second Wind is bonus action",
			featureID:  "second_wind",
			wantAction: dnd5ev1alpha1.ActionType_ACTION_TYPE_BONUS_ACTION,
		},
		{
			name:       "Action Surge is free",
			featureID:  "action_surge",
			wantAction: dnd5ev1alpha1.ActionType_ACTION_TYPE_FREE,
		},
		// Monk
		{
			name:       "Flurry of Blows is bonus action",
			featureID:  "flurry_of_blows",
			wantAction: dnd5ev1alpha1.ActionType_ACTION_TYPE_BONUS_ACTION,
		},
		{
			name:       "Deflect Missiles is reaction",
			featureID:  "deflect_missiles",
			wantAction: dnd5ev1alpha1.ActionType_ACTION_TYPE_REACTION,
		},
		// Rogue
		{
			name:       "Sneak Attack is free",
			featureID:  "sneak_attack",
			wantAction: dnd5ev1alpha1.ActionType_ACTION_TYPE_FREE,
		},
		{
			name:       "Cunning Action is bonus action",
			featureID:  "cunning_action",
			wantAction: dnd5ev1alpha1.ActionType_ACTION_TYPE_BONUS_ACTION,
		},
		{
			name:       "Uncanny Dodge is reaction",
			featureID:  "uncanny_dodge",
			wantAction: dnd5ev1alpha1.ActionType_ACTION_TYPE_REACTION,
		},
		// Paladin
		{
			name:       "Divine Smite is free",
			featureID:  "divine_smite",
			wantAction: dnd5ev1alpha1.ActionType_ACTION_TYPE_FREE,
		},
		{
			name:       "Lay on Hands is action",
			featureID:  "lay_on_hands",
			wantAction: dnd5ev1alpha1.ActionType_ACTION_TYPE_ACTION,
		},
		// Unknown feature returns unspecified
		{
			name:       "Unknown feature returns unspecified",
			featureID:  "totally_made_up_feature",
			wantAction: dnd5ev1alpha1.ActionType_ACTION_TYPE_UNSPECIFIED,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			result := getFeatureActionType(tc.featureID)
			assert.Equal(s.T(), tc.wantAction, result)
		})
	}
}

func (s *ConvertersTestSuite) TestConvertCharacterDataToProto_FeaturesWithActionType() {
	testData := &toolkitchar.Data{
		ID:           "char-1",
		Name:         "Test Fighter",
		MaxHitPoints: 10,
		Level:        2,
		RaceID:       "human",
		ClassID:      classes.Fighter,
		Features: []json.RawMessage{
			json.RawMessage(`{"id": "second_wind", "name": "Second Wind"}`),
			json.RawMessage(`{"id": "action_surge", "name": "Action Surge"}`),
		},
	}

	result := ConvertCharacterDataToProto(testData)

	require.NotNil(s.T(), result)
	require.Len(s.T(), result.Features, 2, "Should have 2 features")

	// Verify action types are populated
	var secondWind, actionSurge *dnd5ev1alpha1.CharacterFeature
	for _, f := range result.Features {
		switch f.Id {
		case "second_wind":
			secondWind = f
		case "action_surge":
			actionSurge = f
		}
	}

	require.NotNil(s.T(), secondWind, "Should have Second Wind feature")
	assert.Equal(s.T(), dnd5ev1alpha1.ActionType_ACTION_TYPE_BONUS_ACTION, secondWind.ActionType)

	require.NotNil(s.T(), actionSurge, "Should have Action Surge feature")
	assert.Equal(s.T(), dnd5ev1alpha1.ActionType_ACTION_TYPE_FREE, actionSurge.ActionType)
}
