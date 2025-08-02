package dnd5e

import (
	"github.com/KirkDiggler/rpg-api/internal/types/choices"
)

// RaceData represents race information from external source
type RaceData struct {
	ID                   string
	Name                 string
	Description          string
	Size                 string
	SizeDescription      string
	Speed                int32
	AbilityBonuses       map[string]int32
	Traits               []TraitData
	Subraces             []SubraceData
	Languages            []string
	LanguageOptions      *ChoiceData
	Proficiencies        []string
	ProficiencyOptions   []*ChoiceData
	AgeDescription       string
	AlignmentDescription string
	Choices              []choices.Choice // Rich choice structures parsed from proficiency choices
}

// SubraceData represents subrace information
type SubraceData struct {
	ID             string
	Name           string
	Description    string
	AbilityBonuses map[string]int32
	Traits         []TraitData
	Languages      []string
	Proficiencies  []string
}

// ClassData represents class information from external source
type ClassData struct {
	ID                       string
	Name                     string
	Description              string
	HitDice                  int32 // Hit die size (e.g., 10 for d10)
	HitPointsAt1st           int32 // HP at level 1 (same as HitDice for D&D 5e)
	PrimaryAbilities         []string
	SavingThrows             []string
	SkillsCount              int32
	AvailableSkills          []string
	StartingEquipment        []string
	StartingEquipmentOptions []*EquipmentChoiceData
	ArmorProficiencies       []string
	WeaponProficiencies      []string
	ToolProficiencies        []string
	ProficiencyChoices       []*ChoiceData
	LevelOneFeatures         []*FeatureData
	Spellcasting             *SpellcastingData
	Choices                  []choices.Choice // Rich choice structures parsed from equipment and proficiency choices
}

// BackgroundData represents background information from external source
type BackgroundData struct {
	ID                 string
	Name               string
	Description        string
	SkillProficiencies []string
	Languages          int32
	Equipment          []string
	Feature            string
}

// SpellData represents spell information from external source
type SpellData struct {
	ID          string
	Name        string
	Level       int32
	School      string
	CastingTime string
	Range       string
	Components  []string
	Duration    string
	Description string
}

// TraitData represents a racial trait
type TraitData struct {
	Name        string
	Description string
	IsChoice    bool
	Options     []string
}

// ChoiceData represents a choice for proficiencies, languages, etc
type ChoiceData struct {
	Type    string   // e.g., "language", "skill", "tool_proficiency"
	Choose  int      // How many to choose
	Options []string // Available options
	From    string   // Optional filter/category
}

// EquipmentChoiceData represents a choice for starting equipment
type EquipmentChoiceData struct {
	Description string
	Options     []string
	ChooseCount int
}

// SpellcastingData represents spellcasting information
type SpellcastingData struct {
	SpellcastingAbility string
	RitualCasting       bool
	SpellcastingFocus   string
	CantripsKnown       int32
	SpellsKnown         int32
	SpellSlotsLevel1    int32
}

// EquipmentAPIData represents equipment information from external source
type EquipmentAPIData struct {
	ID            string
	Name          string
	Description   string
	EquipmentType string // "weapon", "armor", "gear", etc.
	Category      string // "simple-weapons", "martial-weapons", etc.
	Cost          *CostData
	Weight        float32
	// Weapon-specific fields
	WeaponCategory string // "Simple", "Martial"
	WeaponRange    string // "Melee", "Ranged"
	Damage         *DamageData
	Properties     []string
	// Armor-specific fields
	ArmorCategory       string // "Light", "Medium", "Heavy"
	ArmorClass          *ArmorClassData
	StrengthMinimum     int
	StealthDisadvantage bool
}

// FeatureData represents feature information from external source
type FeatureData struct {
	ID             string
	Name           string
	Description    string
	Level          int32
	ClassName      string
	HasChoices     bool
	Choices        []*ChoiceData
	SpellSelection *SpellSelectionData
}

// SpellSelectionData represents programmatic spell selection requirements
type SpellSelectionData struct {
	SpellsToSelect  int32    // Number of spells to select
	SpellLevels     []int32  // Allowed spell levels (0 for cantrips)
	SpellLists      []string // Allowed spell lists (e.g., "wizard", "cleric")
	SelectionType   string   // "spellbook", "known", "prepared"
	RequiresReplace bool     // Whether spells can be replaced on level up
}

// CostData represents equipment cost
type CostData struct {
	Quantity int
	Unit     string
}

// DamageData represents weapon damage
type DamageData struct {
	DamageDice string
	DamageType string
}

// ArmorClassData represents armor class information
type ArmorClassData struct {
	Base     int
	DexBonus bool
}

// InventoryItem represents an item in inventory
type InventoryItem struct {
	ID          string
	Name        string
	Description string
	Quantity    int32
	Weight      float32
	Cost        *CostData
	Type        string
	Equipped    bool
	EquipSlot   string
}

// EquipmentSlots represents equipped items
type EquipmentSlots struct {
	MainHand *InventoryItem
	OffHand  *InventoryItem
	Armor    *InventoryItem
	Helm     *InventoryItem
	Gloves   *InventoryItem
	Boots    *InventoryItem
	Ring1    *InventoryItem
	Ring2    *InventoryItem
	Cloak    *InventoryItem
	Amulet   *InventoryItem
}

// EncumbranceInfo represents character encumbrance
type EncumbranceInfo struct {
	Current           float32
	Light             float32
	Medium            float32
	Heavy             float32
	Encumbered        bool
	HeavilyEncumbered bool
}
