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
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/races"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
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

	// Check second bundle (leather + longbow + arrows). This legacy bundle
	// predates category options, so keep its concrete IDs, quantities, details,
	// type hints, and order pinned while the two paths share a converter.
	leatherBundle := equipOptions.Bundles[1]
	assert.Equal(s.T(), "fighter-armor-b", leatherBundle.Id)
	assert.Equal(s.T(), "Leather armor, longbow, and 20 arrows", leatherBundle.Label)
	requireEquipmentItemsEqual(s.T(), leatherBundle.Items, []equipmentItemExpectation{
		{id: "leather", quantity: 1, detailName: "Leather Armor", armor: dnd5ev1alpha1.Armor_ARMOR_LEATHER},
		{id: "longbow", quantity: 1, detailName: "Longbow", weapon: dnd5ev1alpha1.Weapon_WEAPON_LONGBOW},
		{id: "arrows-20", quantity: 1, detailName: "Arrows (20)"},
	})

	leather := leatherBundle.Items[0]
	leatherData := leather.GetEquipmentDetail().GetArmorData()
	require.NotNil(s.T(), leatherData)
	assert.Equal(s.T(), dnd5ev1alpha1.ArmorCategory_ARMOR_CATEGORY_LIGHT, leatherData.GetArmorCategory())
	assert.Equal(s.T(), int32(11), leatherData.GetBaseAc())
	assert.True(s.T(), leatherData.GetDexBonus())

	arrows := leatherBundle.Items[2]
	assert.Nil(s.T(), arrows.GetTypeHint(), "legacy ammunition has no type hint; preserve that wire contract")
	assert.Equal(s.T(), dnd5ev1alpha1.EquipmentCategory_EQUIPMENT_CATEGORY_ADVENTURING_GEAR,
		arrows.GetEquipmentDetail().GetCategory())
	assert.Nil(s.T(), arrows.GetEquipmentDetail().GetEquipmentData())
}

func (s *ConvertersTestSuite) TestCreateEquipmentChoice_PopulatesEquipmentDetail_Armor() {
	fighterData := classes.ClassData[classes.Fighter]
	require.NotNil(s.T(), fighterData, "Fighter class data should exist")

	result := convertClassDataToProto(fighterData)
	require.NotNil(s.T(), result)

	var armorChoice *dnd5ev1alpha1.Choice
	for _, choice := range result.Choices {
		if choice.Id == "fighter-armor" {
			armorChoice = choice
			break
		}
	}
	require.NotNil(s.T(), armorChoice, "Fighter should have armor choice")

	chainMailBundle := armorChoice.GetEquipmentOptions().Bundles[0]
	require.Len(s.T(), chainMailBundle.Items, 1)
	chainMail := chainMailBundle.Items[0]
	require.Equal(s.T(), "chain-mail", chainMail.SelectionId)

	// This is the field the fix populates. Without the fix, EquipmentDetail
	// is nil and the web's EquipmentCard falls back to a name-only render.
	require.NotNil(s.T(), chainMail.EquipmentDetail, "chain mail should carry resolved equipment detail")
	assert.Equal(s.T(), "Chain Mail", chainMail.EquipmentDetail.Name)

	armorData := chainMail.EquipmentDetail.GetArmorData()
	require.NotNil(s.T(), armorData, "chain mail detail should carry armor data")
	assert.Equal(s.T(), dnd5ev1alpha1.ArmorCategory_ARMOR_CATEGORY_HEAVY, armorData.ArmorCategory)
	assert.Equal(s.T(), int32(16), armorData.BaseAc)
	assert.False(s.T(), armorData.DexBonus, "chain mail grants no dex bonus (max dex 0)")
	assert.Equal(s.T(), int32(13), armorData.StrMinimum)
	assert.True(s.T(), armorData.StealthDisadvantage)
}

func (s *ConvertersTestSuite) TestCreateEquipmentChoice_PopulatesEquipmentDetail_Weapon() {
	fighterData := classes.ClassData[classes.Fighter]
	require.NotNil(s.T(), fighterData, "Fighter class data should exist")

	result := convertClassDataToProto(fighterData)
	require.NotNil(s.T(), result)

	var armorChoice *dnd5ev1alpha1.Choice
	for _, choice := range result.Choices {
		if choice.Id == "fighter-armor" {
			armorChoice = choice
			break
		}
	}
	require.NotNil(s.T(), armorChoice, "Fighter should have armor choice")

	leatherBundle := armorChoice.GetEquipmentOptions().Bundles[1]
	require.Len(s.T(), leatherBundle.Items, 3)
	longbow := leatherBundle.Items[1]
	require.Equal(s.T(), "longbow", longbow.SelectionId)

	// This is the field the fix populates. Without the fix, EquipmentDetail
	// is nil and the web's EquipmentCard falls back to a name-only render.
	require.NotNil(s.T(), longbow.EquipmentDetail, "longbow should carry resolved equipment detail")
	assert.Equal(s.T(), "Longbow", longbow.EquipmentDetail.Name)

	weaponData := longbow.EquipmentDetail.GetWeaponData()
	require.NotNil(s.T(), weaponData, "longbow detail should carry weapon data")
	assert.Equal(s.T(), dnd5ev1alpha1.WeaponCategory_WEAPON_CATEGORY_MARTIAL, weaponData.WeaponCategory)
	assert.Equal(s.T(), "1d8", weaponData.DamageDice)
	assert.Equal(s.T(), dnd5ev1alpha1.DamageType_DAMAGE_TYPE_PIERCING, weaponData.DamageType)
	assert.Equal(s.T(), "ranged", weaponData.Range)
	assert.Equal(s.T(), int32(150), weaponData.NormalRange)
	assert.Equal(s.T(), int32(600), weaponData.LongRange)
	assert.Contains(s.T(), weaponData.Properties, dnd5ev1alpha1.WeaponProperty_WEAPON_PROPERTY_AMMUNITION)
	assert.Contains(s.T(), weaponData.Properties, dnd5ev1alpha1.WeaponProperty_WEAPON_PROPERTY_HEAVY)
	assert.Contains(s.T(), weaponData.Properties, dnd5ev1alpha1.WeaponProperty_WEAPON_PROPERTY_TWO_HANDED)
}

func (s *ConvertersTestSuite) TestCreateEquipmentChoice_MapsCategoryChoiceOptionsOneToOne() {
	fighter := convertClassDataToProto(classes.ClassData[classes.Fighter])
	fighterMartial := findEquipmentCategoryChoice(s.T(), fighter, "fighter-weapons-primary", "fighter-weapon-a")
	requireCategoryOptionsMatchToolkit(
		s.T(),
		fighterMartial.GetOptions(),
		classes.Fighter,
		"fighter-weapons-primary",
		"fighter-weapon-a",
		[]string{
			"greatsword", "longsword", "rapier", "shortsword", "battleaxe", "flail", "glaive", "greataxe",
			"halberd", "lance", "maul", "morningstar", "pike", "scimitar", "trident", "war-pick",
			"warhammer", "whip", "heavy-crossbow", "longbow", "blowgun", "hand-crossbow", "net",
		},
	)

	monk := convertClassDataToProto(classes.ClassData[classes.Monk])
	monkSimple := findEquipmentCategoryChoice(s.T(), monk, "monk-weapons-primary", "monk-weapon-b")
	requireCategoryOptionsMatchToolkit(
		s.T(),
		monkSimple.GetOptions(),
		classes.Monk,
		"monk-weapons-primary",
		"monk-weapon-b",
		[]string{
			"club", "dagger", "handaxe", "javelin", "greatclub", "light-hammer", "mace", "quarterstaff",
			"sickle", "spear", "light-crossbow", "shortbow", "dart", "sling",
		},
	)

	monkOptionIDs := equipmentItemSelectionIDs(monkSimple.GetOptions())
	assert.NotContains(s.T(), monkOptionIDs, "shortsword",
		"shortsword is the toolkit's separate fixed Monk alternative")
	assert.NotContains(s.T(), monkOptionIDs, "unarmed-strike",
		"API must not invent the toolkit-excluded special weapon")
}

func findEquipmentCategoryChoice(t *testing.T, classInfo *dnd5ev1alpha1.ClassInfo, choiceID, bundleID string) *dnd5ev1alpha1.EquipmentCategoryChoice {
	t.Helper()
	for _, choice := range classInfo.GetChoices() {
		if choice.GetId() != choiceID {
			continue
		}
		for _, bundle := range choice.GetEquipmentOptions().GetBundles() {
			if bundle.GetId() == bundleID {
				require.Len(t, bundle.GetCategoryChoices(), 1)
				return bundle.GetCategoryChoices()[0]
			}
		}
	}
	require.Failf(t, "missing category choice", "choice %q bundle %q", choiceID, bundleID)
	return nil
}

func equipmentItemSelectionIDs(items []*dnd5ev1alpha1.EquipmentItem) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.GetSelectionId())
	}
	return ids
}

type equipmentItemExpectation struct {
	id         string
	quantity   int32
	detailName string
	weapon     dnd5ev1alpha1.Weapon
	armor      dnd5ev1alpha1.Armor
}

func requireEquipmentItemsEqual(t *testing.T, actual []*dnd5ev1alpha1.EquipmentItem, expected []equipmentItemExpectation) {
	t.Helper()
	require.Len(t, actual, len(expected))
	for index, expectation := range expected {
		item := actual[index]
		require.NotNil(t, item)
		assert.Equal(t, expectation.id, item.GetSelectionId(), "item %d selection ID", index)
		assert.Equal(t, expectation.quantity, item.GetQuantity(), "item %d quantity", index)
		require.NotNil(t, item.GetEquipmentDetail(), "item %d detail", index)
		assert.Equal(t, expectation.detailName, item.GetEquipmentDetail().GetName(), "item %d detail name", index)
		switch {
		case expectation.weapon != dnd5ev1alpha1.Weapon_WEAPON_UNSPECIFIED:
			assert.Equal(t, expectation.weapon, item.GetWeapon(), "item %d weapon type hint", index)
		case expectation.armor != dnd5ev1alpha1.Armor_ARMOR_UNSPECIFIED:
			assert.Equal(t, expectation.armor, item.GetArmor(), "item %d armor type hint", index)
		}
	}
}

func requireCategoryOptionsMatchToolkit(
	t *testing.T,
	actual []*dnd5ev1alpha1.EquipmentItem,
	classID classes.Class,
	choiceID string,
	bundleID string,
	expectedIDs []string,
) {
	t.Helper()
	expected := toolkitCategoryOptions(t, classID, choiceID, bundleID)
	require.Len(t, actual, len(expected), "wire options must have the same cardinality as toolkit options")
	require.Equal(t, expectedIDs, equipmentItemSelectionIDs(actual), "wire options must retain the published registry order")

	for index, source := range expected {
		item := actual[index]
		require.NotNil(t, item)
		assert.Equal(t, string(source.ID), item.GetSelectionId(), "option %d selection ID", index)
		assert.Equal(t, int32(source.Quantity), item.GetQuantity(), "option %d quantity", index)
		require.NotNil(t, item.GetEquipmentDetail(), "option %d detail", index)
		require.NotNil(t, source.Detail, "toolkit option %d detail", index)
		assert.Equal(t, source.Detail.Name, item.GetEquipmentDetail().GetName(), "option %d detail name", index)
		assert.Equal(t, int32(source.Detail.Weight), item.GetEquipmentDetail().GetWeight().GetQuantity(), "option %d detail weight", index)
		require.NotNil(t, source.Detail.Weapon, "option %d must be a weapon", index)
		weaponData := item.GetEquipmentDetail().GetWeaponData()
		require.NotNil(t, weaponData, "option %d weapon detail", index)
		assert.Equal(t, source.Detail.Weapon.Damage, weaponData.GetDamageDice(), "option %d damage dice", index)
	}
}

func toolkitCategoryOptions(t *testing.T, classID classes.Class, choiceID, bundleID string) []choices.EquipmentItem {
	t.Helper()
	for _, requirement := range choices.GetClassRequirements(classID).Equipment {
		if string(requirement.ID) != choiceID {
			continue
		}
		for _, bundle := range requirement.Options {
			if bundle.ID == bundleID {
				require.Len(t, bundle.CategoryChoices, 1)
				return bundle.CategoryChoices[0].Options
			}
		}
	}
	require.Failf(t, "missing toolkit category choice", "class %q choice %q bundle %q", classID, choiceID, bundleID)
	return nil
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
	// Create test data with features - using toolkit's ref string format
	testData := &toolkitchar.Data{
		ID:    "test-char",
		Name:  "Ragnar",
		Level: 5,
		Features: []json.RawMessage{
			json.RawMessage(`{
				"ref": "dnd5e:features:rage",
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

	// Check feature details - Id is FeatureId enum (RAGE extracted from ref "dnd5e:features:rage")
	rageFeature := result.Features[0]
	assert.Equal(s.T(), dnd5ev1alpha1.FeatureId_FEATURE_ID_RAGE, rageFeature.Id)
	assert.Equal(s.T(), "Rage", rageFeature.Name)
	assert.Equal(s.T(), "class", rageFeature.Source)
	// Verify raw JSON is passed through
	assert.NotEmpty(s.T(), rageFeature.FeatureData, "FeatureData should contain raw JSON")
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
				"ref": "dnd5e:features:rage",
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
	// RAGE extracted from ref "dnd5e:features:rage"
	assert.Equal(s.T(), dnd5ev1alpha1.FeatureId_FEATURE_ID_RAGE, result.Features[0].Id)
	assert.Equal(s.T(), "Rage", result.Features[0].Name)
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
	// Id is now FeatureId enum - rage maps to FEATURE_ID_RAGE
	assert.Equal(s.T(), dnd5ev1alpha1.FeatureId_FEATURE_ID_RAGE, result.Features[0].Id)
	assert.Equal(s.T(), "Rage", result.Features[0].Name)
	// Verify raw JSON is passed through for UI
	assert.NotEmpty(s.T(), result.Features[0].FeatureData)
}

func (s *ConvertersTestSuite) TestFeatureIDToDisplayName() {
	// Test known feature names (using refs)
	assert.Equal(s.T(), "Rage", featureIDToDisplayName(refs.Features.Rage().ID))
	assert.Equal(s.T(), "Second Wind", featureIDToDisplayName(refs.Features.SecondWind().ID))
	assert.Equal(s.T(), "Action Surge", featureIDToDisplayName(refs.Features.ActionSurge().ID))

	// Test unknown feature - should convert snake_case to Title Case
	assert.Equal(s.T(), "Unknown Feature", featureIDToDisplayName("unknown_feature"))
}

func (s *ConvertersTestSuite) TestConvertCharacterDataToProto_WithConditions() {
	// Create test data with conditions - using toolkit's ref string format
	testData := &toolkitchar.Data{
		ID:    "test-char",
		Name:  "Ragnar",
		Level: 5,
		Conditions: []json.RawMessage{
			json.RawMessage(`{
				"ref": "dnd5e:conditions:raging",
				"source": "dnd5e:features:rage",
				"duration": 10
			}`),
			json.RawMessage(`{
				"ref": "dnd5e:conditions:blessed",
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
	assert.Equal(s.T(), dnd5ev1alpha1.ConditionId_CONDITION_ID_RAGING, ragingCondition.Id)
	assert.Equal(s.T(), "Raging", ragingCondition.Name)
	assert.Equal(s.T(), "dnd5e:features:rage", ragingCondition.Source)
	assert.Equal(s.T(), int32(10), ragingCondition.Duration)
	assert.NotEmpty(s.T(), ragingCondition.ConditionData, "ConditionData should contain raw JSON")

	// Check second condition details (Blessed - no enum, should be UNSPECIFIED)
	blessedCondition := result.ActiveConditions[1]
	assert.Equal(s.T(), dnd5ev1alpha1.ConditionId_CONDITION_ID_UNSPECIFIED, blessedCondition.Id)
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
				"ref": "dnd5e:conditions:poisoned",
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
	// Create test data with conditions - name is derived from ref ID
	testData := &toolkitchar.Data{
		ID:    "test-char",
		Name:  "Rogue",
		Level: 3,
		Conditions: []json.RawMessage{
			// Condition with known ref - name should be derived
			json.RawMessage(`{
				"ref": "dnd5e:conditions:raging",
				"source": "rage",
				"duration": 5
			}`),
			// Condition with unknown ref - should use toTitleCase fallback
			json.RawMessage(`{
				"ref": "dnd5e:conditions:custom_effect",
				"source": "spell",
				"duration": 1
			}`),
		},
	}

	// Convert to proto
	result := ConvertCharacterDataToProto(testData)

	// Verify name derivation from ref
	require.NotNil(s.T(), result, "Result should not be nil")
	require.Len(s.T(), result.ActiveConditions, 2, "Should have 2 conditions")

	// First condition should use known display name
	assert.Equal(s.T(), "Raging", result.ActiveConditions[0].Name, "Should derive display name from ref")

	// Second condition should use title case fallback
	assert.Equal(s.T(), "Custom Effect", result.ActiveConditions[1].Name, "Should use toTitleCase fallback")
}

func (s *ConvertersTestSuite) TestConvertCharacterDataToProto_ConditionWithNoRef() {
	// Create test data with condition missing ref - should be skipped
	testData := &toolkitchar.Data{
		ID:    "test-char",
		Name:  "Fighter",
		Level: 2,
		Conditions: []json.RawMessage{
			// Condition with no ref - should be skipped
			json.RawMessage(`{
				"source": "unknown",
				"duration": 2
			}`),
			// Valid condition with ref - should be included
			json.RawMessage(`{
				"ref": "dnd5e:conditions:frightened",
				"source": "fear_spell",
				"duration": 10
			}`),
		},
	}

	// Convert to proto
	result := ConvertCharacterDataToProto(testData)

	// Verify condition with no ref is skipped
	require.NotNil(s.T(), result, "Result should not be nil")
	assert.Len(s.T(), result.ActiveConditions, 1, "Should skip condition with no ref")
	assert.Equal(s.T(), "Frightened", result.ActiveConditions[0].Name)
}

func (s *ConvertersTestSuite) TestGetFeatureActionType() {
	// Test features implemented in the toolkit - action types come from toolkit, not hardcoded
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
			name:       "Patient Defense is bonus action",
			featureID:  "patient_defense",
			wantAction: dnd5ev1alpha1.ActionType_ACTION_TYPE_BONUS_ACTION,
		},
		{
			name:       "Step of the Wind is bonus action",
			featureID:  "step_of_the_wind",
			wantAction: dnd5ev1alpha1.ActionType_ACTION_TYPE_BONUS_ACTION,
		},
		{
			name:       "Deflect Missiles is reaction",
			featureID:  "deflect_missiles",
			wantAction: dnd5ev1alpha1.ActionType_ACTION_TYPE_REACTION,
		},
		// Features not yet in toolkit return unspecified
		{
			name:       "Unknown feature returns unspecified",
			featureID:  "totally_made_up_feature",
			wantAction: dnd5ev1alpha1.ActionType_ACTION_TYPE_UNSPECIFIED,
		},
		{
			name:       "Feature not in toolkit returns unspecified",
			featureID:  "sneak_attack",
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
			json.RawMessage(`{"ref": "dnd5e:features:second_wind", "id": "second_wind", "name": "Second Wind"}`),
			json.RawMessage(`{"ref": "dnd5e:features:action_surge", "id": "action_surge", "name": "Action Surge"}`),
		},
	}

	result := ConvertCharacterDataToProto(testData)

	require.NotNil(s.T(), result)
	require.Len(s.T(), result.Features, 2, "Should have 2 features")

	// Verify action types are populated
	// Note: Since second_wind and action_surge don't have FeatureId enum values,
	// they both return FEATURE_ID_UNSPECIFIED. We identify them by name instead.
	var secondWind, actionSurge *dnd5ev1alpha1.CharacterFeature
	for _, f := range result.Features {
		switch f.Name {
		case "Second Wind":
			secondWind = f
		case "Action Surge":
			actionSurge = f
		}
	}

	require.NotNil(s.T(), secondWind, "Should have Second Wind feature")
	assert.Equal(s.T(), dnd5ev1alpha1.ActionType_ACTION_TYPE_BONUS_ACTION, secondWind.ActionType)

	require.NotNil(s.T(), actionSurge, "Should have Action Surge feature")
	assert.Equal(s.T(), dnd5ev1alpha1.ActionType_ACTION_TYPE_FREE, actionSurge.ActionType)
}

func (s *ConvertersTestSuite) TestConvertProtoWeaponToToolkit() {
	testCases := []struct {
		name     string
		weapon   dnd5ev1alpha1.Weapon
		expected shared.SelectionID
	}{
		// Simple Melee
		{"Club", dnd5ev1alpha1.Weapon_WEAPON_CLUB, "club"},
		{"Dagger", dnd5ev1alpha1.Weapon_WEAPON_DAGGER, "dagger"},
		{"Quarterstaff", dnd5ev1alpha1.Weapon_WEAPON_QUARTERSTAFF, "quarterstaff"},
		// Simple Ranged
		{"Shortbow", dnd5ev1alpha1.Weapon_WEAPON_SHORTBOW, "shortbow"},
		{"Light Crossbow", dnd5ev1alpha1.Weapon_WEAPON_LIGHT_CROSSBOW, "light-crossbow"},
		// Martial Melee
		{"Longsword", dnd5ev1alpha1.Weapon_WEAPON_LONGSWORD, "longsword"},
		{"Greataxe", dnd5ev1alpha1.Weapon_WEAPON_GREATAXE, "greataxe"},
		{"Rapier", dnd5ev1alpha1.Weapon_WEAPON_RAPIER, "rapier"},
		// Martial Ranged
		{"Longbow", dnd5ev1alpha1.Weapon_WEAPON_LONGBOW, "longbow"},
		{"Heavy Crossbow", dnd5ev1alpha1.Weapon_WEAPON_HEAVY_CROSSBOW, "heavy-crossbow"},
		// Ammunition
		{"Arrows (20)", dnd5ev1alpha1.Weapon_WEAPON_ARROWS_20, "arrows-20"},
		{"Bolts (20)", dnd5ev1alpha1.Weapon_WEAPON_BOLTS_20, "bolts-20"},
		// Category placeholders
		{"Any Simple", dnd5ev1alpha1.Weapon_WEAPON_ANY_SIMPLE, "simple-weapon"},
		{"Any Martial", dnd5ev1alpha1.Weapon_WEAPON_ANY_MARTIAL, "martial-weapon"},
		{"Any Weapon", dnd5ev1alpha1.Weapon_WEAPON_ANY, "any-weapon"},
		// Unspecified
		{"Unspecified", dnd5ev1alpha1.Weapon_WEAPON_UNSPECIFIED, ""},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			result := convertProtoWeaponToToolkit(tc.weapon)
			assert.Equal(s.T(), tc.expected, result)
		})
	}
}

func (s *ConvertersTestSuite) TestConvertProtoArmorToToolkit() {
	testCases := []struct {
		name     string
		armor    dnd5ev1alpha1.Armor
		expected shared.SelectionID
	}{
		// Light Armor
		{"Leather", dnd5ev1alpha1.Armor_ARMOR_LEATHER, "leather"},
		{"Studded Leather", dnd5ev1alpha1.Armor_ARMOR_STUDDED_LEATHER, "studded-leather"},
		// Medium Armor
		{"Chain Shirt", dnd5ev1alpha1.Armor_ARMOR_CHAIN_SHIRT, "chain-shirt"},
		{"Breastplate", dnd5ev1alpha1.Armor_ARMOR_BREASTPLATE, "breastplate"},
		{"Half Plate", dnd5ev1alpha1.Armor_ARMOR_HALF_PLATE, "half-plate"},
		// Heavy Armor
		{"Chain Mail", dnd5ev1alpha1.Armor_ARMOR_CHAIN_MAIL, "chain-mail"},
		{"Plate", dnd5ev1alpha1.Armor_ARMOR_PLATE, "plate"},
		// Shield
		{"Shield", dnd5ev1alpha1.Armor_ARMOR_SHIELD, "shield"},
		// Unspecified
		{"Unspecified", dnd5ev1alpha1.Armor_ARMOR_UNSPECIFIED, ""},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			result := convertProtoArmorToToolkit(tc.armor)
			assert.Equal(s.T(), tc.expected, result)
		})
	}
}

func (s *ConvertersTestSuite) TestConvertRaceDataToProto_Human_NoTraits() {
	humanData := races.RaceData[races.Human]
	require.NotNil(s.T(), humanData, "Human race data should exist")
	require.Empty(s.T(), humanData.Traits, "fixture assumption: Human has no toolkit traits")

	result := convertRaceDataToProto(humanData)
	require.NotNil(s.T(), result)

	assert.Empty(s.T(), result.GetTraits(), "Human should not fabricate traits it doesn't have")
}

func (s *ConvertersTestSuite) TestConvertRaceDataToProto_Elf_PopulatesTraits() {
	elfData := races.RaceData[races.Elf]
	require.NotNil(s.T(), elfData, "Elf race data should exist")
	require.NotEmpty(s.T(), elfData.Traits, "fixture assumption: Elf has toolkit traits")

	result := convertRaceDataToProto(elfData)
	require.NotNil(s.T(), result)

	require.Len(s.T(), result.GetTraits(), len(elfData.Traits))

	names := make([]string, 0, len(result.GetTraits()))
	for _, trait := range result.GetTraits() {
		names = append(names, trait.GetName())
		// Honest mapping: toolkit races.Trait carries no choice data today.
		assert.False(s.T(), trait.GetIsChoice(), "races.Trait has no choice data - is_choice must not be invented")
		assert.Empty(s.T(), trait.GetOptions(), "races.Trait has no choice data - options must not be invented")
	}
	assert.Contains(s.T(), names, "Darkvision")
	assert.Contains(s.T(), names, "Fey Ancestry")
	assert.Contains(s.T(), names, "Keen Senses")
	assert.Contains(s.T(), names, "Trance")

	// Description should travel too, not just the name.
	for _, trait := range result.GetTraits() {
		if trait.GetName() == "Fey Ancestry" {
			assert.Contains(s.T(), trait.GetDescription(), "charmed")
		}
	}
}

func (s *ConvertersTestSuite) TestConvertRaceDataToProto_Dwarf_PopulatesTraits() {
	dwarfData := races.RaceData[races.Dwarf]
	require.NotNil(s.T(), dwarfData, "Dwarf race data should exist")
	require.NotEmpty(s.T(), dwarfData.Traits, "fixture assumption: Dwarf has toolkit traits")

	result := convertRaceDataToProto(dwarfData)
	require.NotNil(s.T(), result)

	require.Len(s.T(), result.GetTraits(), len(dwarfData.Traits))

	names := make([]string, 0, len(result.GetTraits()))
	for _, trait := range result.GetTraits() {
		names = append(names, trait.GetName())
	}
	assert.Contains(s.T(), names, "Darkvision")
	assert.Contains(s.T(), names, "Dwarven Resilience")
	assert.Contains(s.T(), names, "Stonecunning")
}

func (s *ConvertersTestSuite) TestConvertRaceTraitsToProto_Discriminates() {
	// Directly exercise the helper: nil/empty input must yield nil, not a
	// spurious empty-but-non-nil slice masquerading as "populated".
	assert.Nil(s.T(), convertRaceTraitsToProto(nil))
	assert.Nil(s.T(), convertRaceTraitsToProto([]races.Trait{}))

	result := convertRaceTraitsToProto([]races.Trait{
		{ID: "test-trait", Name: "Test Trait", Description: "A test trait description"},
	})
	require.Len(s.T(), result, 1)
	assert.Equal(s.T(), "Test Trait", result[0].GetName())
	assert.Equal(s.T(), "A test trait description", result[0].GetDescription())
	assert.False(s.T(), result[0].GetIsChoice())
	assert.Empty(s.T(), result[0].GetOptions())
}
