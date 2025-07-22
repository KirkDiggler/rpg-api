// Package dnd5e implements the D&D 5e entities
package dnd5e

// Character represents a finalized D&D 5e character
// NOTE: This is a data-only struct. All calculations (AC, proficiency bonus, etc.)
// are done by the engine (rpg-toolkit), not here. See internal/entities/README.md
type Character struct {
	ID               string
	Name             string
	Level            int32
	ExperiencePoints int32
	RaceID           string
	SubraceID        string
	ClassID          string
	BackgroundID     string
	Alignment        string
	AbilityScores    AbilityScores
	CurrentHP        int32
	TempHP           int32
	SessionID        string
	PlayerID         string
	CreatedAt        int64
	UpdatedAt        int64
}

// CharacterDraft represents a character in creation with hydrated info
// This is the full entity returned by the orchestrator with all info objects populated
type CharacterDraft struct {
	ID               string
	PlayerID         string
	SessionID        string
	Name             string
	RaceID           string
	SubraceID        string
	ClassID          string
	BackgroundID     string
	AbilityScores    *AbilityScores
	Alignment        string
	ChoiceSelections []ChoiceSelection // All player choices for this character
	Progress         CreationProgress
	ExpiresAt        int64
	CreatedAt        int64
	UpdatedAt        int64

	// Populated by orchestrator when returning draft data
	Race       *RaceInfo       `json:"-"` // Full race info when loaded
	Subrace    *SubraceInfo    `json:"-"` // Full subrace info when loaded
	Class      *ClassInfo      `json:"-"` // Full class info when loaded
	Background *BackgroundInfo `json:"-"` // Full background info when loaded
}

// AbilityScores holds the six core ability scores
type AbilityScores struct {
	Strength     int32
	Dexterity    int32
	Constitution int32
	Intelligence int32
	Wisdom       int32
	Charisma     int32
}

// CreationProgress tracks completion of character creation steps using bitflags
type CreationProgress struct {
	StepsCompleted       uint8 // Bitflags for completed steps
	CompletionPercentage int32
	CurrentStep          string
}

// Progress step bitflags
const (
	ProgressStepName          uint8 = 1 << iota // 1
	ProgressStepRace                            // 2
	ProgressStepClass                           // 4
	ProgressStepBackground                      // 8
	ProgressStepAbilityScores                   // 16
	ProgressStepSkills                          // 32
	ProgressStepLanguages                       // 64
	ProgressStepChoices                         // 128 (fighting styles, cantrips, spells)
)

// HasStep checks if a specific step is completed
func (p CreationProgress) HasStep(step uint8) bool {
	return p.StepsCompleted&step != 0
}

// SetStep marks a step as completed
func (p *CreationProgress) SetStep(step uint8, completed bool) {
	if completed {
		p.StepsCompleted |= step
	} else {
		p.StepsCompleted &^= step
	}
}

// Convenience methods for backward compatibility

// HasName checks if the name step is completed
func (p CreationProgress) HasName() bool { return p.HasStep(ProgressStepName) }

// HasRace checks if the race step is completed
func (p CreationProgress) HasRace() bool { return p.HasStep(ProgressStepRace) }

// HasClass checks if the class step is completed
func (p CreationProgress) HasClass() bool { return p.HasStep(ProgressStepClass) }

// HasBackground checks if the background step is completed
func (p CreationProgress) HasBackground() bool { return p.HasStep(ProgressStepBackground) }

// HasAbilityScores checks if the ability scores step is completed
func (p CreationProgress) HasAbilityScores() bool { return p.HasStep(ProgressStepAbilityScores) }

// HasSkills checks if the skills step is completed
func (p CreationProgress) HasSkills() bool { return p.HasStep(ProgressStepSkills) }

// HasChoices checks if the choices step is completed
func (p CreationProgress) HasChoices() bool { return p.HasStep(ProgressStepChoices) }

// HasLanguages checks if the languages step is completed
func (p CreationProgress) HasLanguages() bool { return p.HasStep(ProgressStepLanguages) }

// Data loading entities for character creation UI

// RaceInfo contains detailed information about a D&D 5e race
type RaceInfo struct {
	ID                   string
	Name                 string
	Description          string
	Speed                int32
	Size                 string
	SizeDescription      string
	AbilityBonuses       map[string]int32
	Traits               []RacialTrait
	Subraces             []SubraceInfo
	Proficiencies        []string
	Languages            []string
	AgeDescription       string
	AlignmentDescription string
	LanguageOptions      *Choice  // Deprecated: will be moved to Choices
	ProficiencyOptions   []Choice // Deprecated: will be moved to Choices
	Choices              []Choice // All choices unified here
}

// SubraceInfo contains information about a D&D 5e subrace
type SubraceInfo struct {
	ID             string
	Name           string
	Description    string
	AbilityBonuses map[string]int32
	Traits         []RacialTrait
	Languages      []string
	Proficiencies  []string
}

// RacialTrait contains information about a racial trait
type RacialTrait struct {
	Name        string
	Description string
	IsChoice    bool
	Options     []string
}

// Choice represents a choice for proficiencies, languages, equipment, etc
type Choice struct {
	ID          string
	Description string
	Type        ChoiceType
	ChooseCount int32
	OptionSet   ChoiceOptionSet
}

// ChoiceType represents the type of choice
type ChoiceType string

const (
	// ChoiceTypeEquipment represents equipment choices
	ChoiceTypeEquipment ChoiceType = "equipment"
	// ChoiceTypeSkill represents skill proficiency choices
	ChoiceTypeSkill             ChoiceType = "skill"
	ChoiceTypeTool              ChoiceType = "tool"
	ChoiceTypeLanguage          ChoiceType = "language"
	ChoiceTypeWeaponProficiency ChoiceType = "weapon_proficiency"
	ChoiceTypeArmorProficiency  ChoiceType = "armor_proficiency"
	ChoiceTypeSpell             ChoiceType = "spell"
	ChoiceTypeFeat              ChoiceType = "feat"
	ChoiceTypeFightingStyle     ChoiceType = "fighting_style"
	ChoiceTypeCantrips          ChoiceType = "cantrips"
	ChoiceTypeSpells            ChoiceType = "spells"
)

// ChoiceSource represents where a choice came from
type ChoiceSource string

const (
	// ChoiceSourceRace represents choices from race
	ChoiceSourceRace ChoiceSource = "race"
	// ChoiceSourceClass represents choices from class
	ChoiceSourceClass ChoiceSource = "class"
	// ChoiceSourceBackground represents choices from background
	ChoiceSourceBackground ChoiceSource = "background"
	// ChoiceSourceSubrace represents choices from subrace
	ChoiceSourceSubrace ChoiceSource = "subrace"
	// ChoiceSourceFeature represents choices from class features
	ChoiceSourceFeature ChoiceSource = "feature"
)

// AbilityScoreChoice represents an ability score bonus selection
type AbilityScoreChoice struct {
	Ability string // Ability constant (AbilityStrength, etc.)
	Bonus   int32  // Bonus amount
}

// Choice category ID constants to prevent magic strings
const (
	// CategoryIDFighterFightingStyle is the category ID for fighter fighting styles
	CategoryIDFighterFightingStyle = "fighter_fighting_style"
	// CategoryIDWizardCantrips is the category ID for wizard cantrips
	CategoryIDWizardCantrips = "wizard_cantrips"
	// CategoryIDWizardSpells is the category ID for wizard spells
	CategoryIDWizardSpells = "wizard_spells"
	// CategoryIDClericCantrips is the category ID for cleric cantrips
	CategoryIDClericCantrips = "cleric_cantrips"
	// CategoryIDSorcererCantrips is the category ID for sorcerer cantrips
	CategoryIDSorcererCantrips = "sorcerer_cantrips"
	// CategoryIDSorcererSpells is the category ID for sorcerer spells
	CategoryIDSorcererSpells = "sorcerer_spells"
	// CategoryIDAdditionalLanguages is the category ID for additional languages
	CategoryIDAdditionalLanguages = "additional_languages"
	// CategoryIDToolProficiencies is the category ID for tool proficiencies
	CategoryIDToolProficiencies = "tool_proficiencies"
	// CategoryIDEquipmentChoices is the category ID for equipment choices
	CategoryIDEquipmentChoices = "equipment_choices"
)

// Validation constants
const (
	// MaxChoiceOptionsLimit is the maximum number of options that can be selected to prevent DoS attacks
	MaxChoiceOptionsLimit = 1000
	// DefaultSpellPageSize is the default page size when fetching spells for choices
	DefaultSpellPageSize = 100
)

// Class ID constants to prevent magic strings
const (
	// ClassIDFighter is the class ID for fighter
	ClassIDFighter = "fighter"
	// ClassIDWizard is the class ID for wizard
	ClassIDWizard = "wizard"
	// ClassIDCleric is the class ID for cleric
	ClassIDCleric = "cleric"
	// ClassIDSorcerer is the class ID for sorcerer
	ClassIDSorcerer = "sorcerer"
)

// ChoiceOptionSet represents the options for a choice
type ChoiceOptionSet interface {
	isChoiceOptionSet()
}

// ExplicitOptions represents a specific list of options to choose from
type ExplicitOptions struct {
	Options []ChoiceOption
}

func (ExplicitOptions) isChoiceOptionSet() {}

// CategoryReference represents a reference to a category of items
type CategoryReference struct {
	CategoryID string
	ExcludeIDs []string
}

func (CategoryReference) isChoiceOptionSet() {}

// ChoiceOption represents a single option within a choice
type ChoiceOption interface {
	isChoiceOption()
}

// ItemReference represents a reference to a single item
type ItemReference struct {
	ItemID string
	Name   string
}

func (ItemReference) isChoiceOption() {}

// CountedItemReference represents an item with a quantity
type CountedItemReference struct {
	ItemID   string
	Name     string
	Quantity int32
}

func (CountedItemReference) isChoiceOption() {}

// ItemBundle represents multiple items as one option
type ItemBundle struct {
	Items []BundleItem
}

func (ItemBundle) isChoiceOption() {}

// BundleItem represents a single item in a bundle, which can be concrete or a choice
type BundleItem struct {
	ItemType BundleItemType
}

// BundleItemType is the interface for bundle item types
type BundleItemType interface {
	isBundleItemType()
}

// BundleItemConcreteItem represents a concrete item in a bundle
type BundleItemConcreteItem struct {
	ConcreteItem *CountedItemReference
}

func (BundleItemConcreteItem) isBundleItemType() {}

// BundleItemChoiceItem represents a choice within a bundle
type BundleItemChoiceItem struct {
	ChoiceItem *NestedChoice
}

func (BundleItemChoiceItem) isBundleItemType() {}

// NestedChoice represents a choice that contains another choice
type NestedChoice struct {
	Choice *Choice
}

func (NestedChoice) isChoiceOption() {}

// ChoiceSelection represents a selection made by the player for a specific choice
// ChoiceSelection tracks a choice made during character creation
type ChoiceSelection struct {
	ChoiceID            string               // ID from Choice in RaceInfo/ClassInfo
	ChoiceType          ChoiceType           // What kind of choice this is
	Source              ChoiceSource         // Where this choice came from
	SelectedKeys        []string             // What was selected
	AbilityScoreChoices []AbilityScoreChoice // For ability score choices
}

// ChoiceCategory represents a grouping of related choices
type ChoiceCategory struct {
	ID          string
	Type        ChoiceType
	Name        string
	Description string
	Required    bool
	MinChoices  int32
	MaxChoices  int32
	Options     []*CategoryOption // Options for this category
}

// CategoryOption represents a single option within a choice category
type CategoryOption struct {
	ID            string
	Name          string
	Description   string
	Prerequisites []string // IDs of other choices that must be selected first
	Conflicts     []string // IDs of other choices that cannot be selected together
	Level         int32    // For spells/cantrips, the level requirement
	School        string   // For spells, the school of magic
	Source        string   // Where this choice comes from (class, race, background, etc.)
}

// ChoiceValidationResult represents the result of validating choice selections
type ChoiceValidationResult struct {
	IsValid  bool
	Errors   []ChoiceValidationError
	Warnings []ChoiceValidationWarning
}

// ChoiceValidationError represents a validation error for a choice selection
type ChoiceValidationError struct {
	ChoiceID   string
	CategoryID string // Category ID for the choice
	OptionID   string // Option ID for the specific option
	Message    string
	Type       string
	Code       string // Error code
}

// ChoiceValidationWarning represents a validation warning for a choice selection
type ChoiceValidationWarning struct {
	ChoiceID   string
	CategoryID string // Category ID for the choice
	OptionID   string // Option ID for the specific option
	Message    string
	Type       string
	Code       string // Warning code
}

// ClassInfo contains detailed information about a D&D 5e class
type ClassInfo struct {
	ID                       string
	Name                     string
	Description              string
	HitDie                   string
	PrimaryAbilities         []string
	ArmorProficiencies       []string
	WeaponProficiencies      []string
	ToolProficiencies        []string
	SavingThrowProficiencies []string
	SkillChoicesCount        int32
	AvailableSkills          []string
	StartingEquipment        []string
	EquipmentChoices         []EquipmentChoice // Deprecated: will be moved to Choices
	Level1Features           []FeatureInfo
	Spellcasting             *SpellcastingInfo
	ProficiencyChoices       []Choice // Deprecated: will be moved to Choices
	Choices                  []Choice // All choices unified here
}

// EquipmentChoice represents a choice in starting equipment
type EquipmentChoice struct {
	Description string
	Options     []string
	ChooseCount int32
}

// ClassFeature represents a class feature (deprecated - use FeatureInfo instead)
type ClassFeature struct {
	Name        string
	Description string
	Level       int32
	HasChoices  bool
	Choices     []string
}

// FeatureInfo represents detailed information about a class feature, racial trait, or other feature
type FeatureInfo struct {
	ID             string
	Name           string
	Description    string
	Level          int32
	ClassName      string
	HasChoices     bool
	Choices        []Choice
	SpellSelection *SpellSelectionInfo
}

// SpellSelectionInfo contains programmatic spell selection requirements
type SpellSelectionInfo struct {
	SpellsToSelect  int32
	SpellLevels     []int32
	SpellLists      []string
	SelectionType   string
	RequiresReplace bool
}

// SpellcastingInfo contains spellcasting information for a class
type SpellcastingInfo struct {
	SpellcastingAbility string
	RitualCasting       bool
	SpellcastingFocus   string
	CantripsKnown       int32
	SpellsKnown         int32
	SpellSlotsLevel1    int32
}

// BackgroundInfo contains detailed information about a D&D 5e background
type BackgroundInfo struct {
	ID                  string
	Name                string
	Description         string
	SkillProficiencies  []string
	ToolProficiencies   []string
	Languages           []string
	AdditionalLanguages int32
	StartingEquipment   []string
	StartingGold        int32
	FeatureName         string
	FeatureDescription  string
	PersonalityTraits   []string
	Ideals              []string
	Bonds               []string
	Flaws               []string
}

// SpellInfo contains information about a D&D 5e spell
type SpellInfo struct {
	ID          string
	Name        string
	Level       int32
	School      string
	CastingTime string
	Range       string
	Components  []string
	Duration    string
	Description string
	Classes     []string
}

// EquipmentInfo contains information about D&D 5e equipment
type EquipmentInfo struct {
	ID          string
	Name        string
	Type        string // "weapon", "armor", "gear", etc.
	Category    string // "simple-weapon", "martial-weapon", "light-armor", etc.
	Cost        string // "2 gp", "50 gp", etc.
	Weight      string // "1 lb", "2 lbs", etc.
	Description string
	Properties  []string // For weapons: "light", "finesse", etc.
}

// TODO(#46): Separate CharacterDraft into data and presentation models. Add ToData() method to convert for repository storage
