package character

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
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
