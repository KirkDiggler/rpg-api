package external

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
	HitDice                  string
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

// ListSpellsInput represents input for listing spells
type ListSpellsInput struct {
	Level   *int32 // Optional filter by spell level (0-9)
	ClassID string // Optional filter by class ID
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

// Removed duplicate FeatureData - using the richer version below

// SpellcastingData represents spellcasting information
type SpellcastingData struct {
	SpellcastingAbility string
	RitualCasting       bool
	SpellcastingFocus   string
	CantripsKnown       int32
	SpellsKnown         int32
	SpellSlotsLevel1    int32
}

// EquipmentData represents equipment information from external source
type EquipmentData struct {
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
