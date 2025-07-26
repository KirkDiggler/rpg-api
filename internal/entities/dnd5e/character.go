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
	MaxHP            int32
	TempHP           int32
	SessionID        string
	PlayerID         string
	CreatedAt        int64
	UpdatedAt        int64

	// Equipment and inventory
	EquipmentSlots *EquipmentSlots  // Equipped items by slot
	Inventory      []InventoryItem  // Unequipped items
	Encumbrance    *EncumbranceInfo // Weight and carrying capacity
}

// GetEquippedItem returns the item in the specified equipment slot
func (c *Character) GetEquippedItem(slot string) *InventoryItem {
	if c.EquipmentSlots == nil {
		return nil
	}

	switch slot {
	case EquipmentSlotMainHand:
		return c.EquipmentSlots.MainHand
	case EquipmentSlotOffHand:
		return c.EquipmentSlots.OffHand
	case EquipmentSlotArmor:
		return c.EquipmentSlots.Armor
	case EquipmentSlotHelmet:
		return c.EquipmentSlots.Helmet
	case EquipmentSlotBoots:
		return c.EquipmentSlots.Boots
	case EquipmentSlotGloves:
		return c.EquipmentSlots.Gloves
	case EquipmentSlotCloak:
		return c.EquipmentSlots.Cloak
	case EquipmentSlotAmulet:
		return c.EquipmentSlots.Amulet
	case EquipmentSlotRing1:
		return c.EquipmentSlots.Ring1
	case EquipmentSlotRing2:
		return c.EquipmentSlots.Ring2
	case EquipmentSlotBelt:
		return c.EquipmentSlots.Belt
	default:
		return nil
	}
}

// SetEquippedItem sets the item in the specified equipment slot
func (c *Character) SetEquippedItem(slot string, item *InventoryItem) {
	if c.EquipmentSlots == nil {
		c.EquipmentSlots = &EquipmentSlots{}
	}

	switch slot {
	case EquipmentSlotMainHand:
		c.EquipmentSlots.MainHand = item
	case EquipmentSlotOffHand:
		c.EquipmentSlots.OffHand = item
	case EquipmentSlotArmor:
		c.EquipmentSlots.Armor = item
	case EquipmentSlotHelmet:
		c.EquipmentSlots.Helmet = item
	case EquipmentSlotBoots:
		c.EquipmentSlots.Boots = item
	case EquipmentSlotGloves:
		c.EquipmentSlots.Gloves = item
	case EquipmentSlotCloak:
		c.EquipmentSlots.Cloak = item
	case EquipmentSlotAmulet:
		c.EquipmentSlots.Amulet = item
	case EquipmentSlotRing1:
		c.EquipmentSlots.Ring1 = item
	case EquipmentSlotRing2:
		c.EquipmentSlots.Ring2 = item
	case EquipmentSlotBelt:
		c.EquipmentSlots.Belt = item
	}
}

// FindInventoryItem finds an item in the character's inventory by item ID
// Returns a copy of the item and whether it was found
func (c *Character) FindInventoryItem(itemID string) (InventoryItem, bool) {
	for i := range c.Inventory {
		if c.Inventory[i].ItemID == itemID {
			return c.Inventory[i], true
		}
	}
	return InventoryItem{}, false
}

// FindInventoryItemIndex finds an item in the character's inventory by item ID
// Returns the index and whether it was found
func (c *Character) FindInventoryItemIndex(itemID string) (int, bool) {
	for i := range c.Inventory {
		if c.Inventory[i].ItemID == itemID {
			return i, true
		}
	}
	return -1, false
}

// RemoveInventoryItem removes an item from inventory by item ID
// Returns the removed item and whether it was found
func (c *Character) RemoveInventoryItem(itemID string) (*InventoryItem, bool) {
	for i := range c.Inventory {
		if c.Inventory[i].ItemID == itemID {
			item := c.Inventory[i]
			// Remove from slice
			c.Inventory = append(c.Inventory[:i], c.Inventory[i+1:]...)
			return &item, true
		}
	}
	return nil, false
}

// AddInventoryItem adds an item to the character's inventory
// If the item is stackable and already exists, it increases the quantity
func (c *Character) AddInventoryItem(item InventoryItem) {
	// Check if item already exists and is stackable
	for i := range c.Inventory {
		if c.Inventory[i].ItemID == item.ItemID &&
			c.Inventory[i].Equipment != nil &&
			c.Inventory[i].Equipment.Stackable {
			c.Inventory[i].Quantity += item.Quantity
			return
		}
	}
	// Add as new item
	c.Inventory = append(c.Inventory, item)
}

// CountAttunedItems returns the number of items currently attuned
func (c *Character) CountAttunedItems() int {
	count := 0

	// Check equipped items
	if c.EquipmentSlots != nil {
		slots := []*InventoryItem{
			c.EquipmentSlots.MainHand,
			c.EquipmentSlots.OffHand,
			c.EquipmentSlots.Armor,
			c.EquipmentSlots.Helmet,
			c.EquipmentSlots.Boots,
			c.EquipmentSlots.Gloves,
			c.EquipmentSlots.Cloak,
			c.EquipmentSlots.Amulet,
			c.EquipmentSlots.Ring1,
			c.EquipmentSlots.Ring2,
			c.EquipmentSlots.Belt,
		}

		for _, item := range slots {
			if item != nil && item.IsAttuned {
				count++
			}
		}
	}

	// Check inventory items (in case some attuned items are unequipped)
	for _, item := range c.Inventory {
		if item.IsAttuned {
			count++
		}
	}

	return count
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
	// These fields are excluded from JSON serialization (json:"-") because:
	// 1. They contain redundant data already represented by the ID fields
	// 2. They're only populated for API responses, not storage
	// 3. Including them would significantly increase payload size
	// 4. The handler layer converts these to proper proto messages for responses
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
	ChoiceTypeSkill ChoiceType = "skill"
	// ChoiceTypeTool represents tool proficiency choices
	ChoiceTypeTool ChoiceType = "tool"
	// ChoiceTypeLanguage represents language choices
	ChoiceTypeLanguage ChoiceType = "language"
	// ChoiceTypeWeaponProficiency represents weapon proficiency choices
	ChoiceTypeWeaponProficiency ChoiceType = "weapon_proficiency"
	// ChoiceTypeArmorProficiency represents armor proficiency choices
	ChoiceTypeArmorProficiency ChoiceType = "armor_proficiency"
	// ChoiceTypeSpell represents spell choices
	ChoiceTypeSpell ChoiceType = "spell"
	// ChoiceTypeFeat represents feat/feature choices
	ChoiceTypeFeat ChoiceType = "feat"
	// ChoiceTypeFightingStyle represents fighting style choices
	ChoiceTypeFightingStyle ChoiceType = "fighting_style"
	// ChoiceTypeCantrips represents cantrip spell choices
	ChoiceTypeCantrips ChoiceType = "cantrips"
	// ChoiceTypeSpells represents spell list choices
	ChoiceTypeSpells ChoiceType = "spells"
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

// EquipmentSlots represents the equipment slots for a character
type EquipmentSlots struct {
	// Combat equipment
	MainHand *InventoryItem
	OffHand  *InventoryItem

	// Armor slots
	Armor  *InventoryItem
	Helmet *InventoryItem
	Boots  *InventoryItem
	Gloves *InventoryItem
	Cloak  *InventoryItem

	// Accessory slots
	Amulet *InventoryItem
	Ring1  *InventoryItem
	Ring2  *InventoryItem
	Belt   *InventoryItem
}

// InventoryItem represents an item in inventory (equipped or not)
type InventoryItem struct {
	ItemID     string         // Reference to equipment ID
	Quantity   int32          // For stackable items
	IsAttuned  bool           // For magic items requiring attunement
	CustomName string         // Optional custom name (e.g., "My Lucky Sword")
	Equipment  *EquipmentData // Denormalized equipment data for quick access
}

// EquipmentData contains the essential equipment information
// This is a simplified version of the full Equipment proto for storage
type EquipmentData struct {
	ID         string
	Name       string
	Type       string // "weapon", "armor", "gear", etc.
	Category   string // "simple-weapon", "martial-weapon", "light-armor", etc.
	Weight     int32  // Weight in tenths of pounds (for 0.1 lb precision)
	Properties []string
	Stackable  bool // Whether this item can stack (e.g., arrows, potions)

	// Type-specific data
	WeaponData *WeaponData
	ArmorData  *ArmorData
	GearData   *GearData
}

// WeaponData contains weapon-specific information
type WeaponData struct {
	WeaponCategory string   // "simple", "martial"
	DamageDice     string   // "1d6", "1d8", etc.
	DamageType     string   // "slashing", "piercing", etc.
	Properties     []string // "light", "finesse", etc.
	Range          string   // "melee", "ranged"
	NormalRange    int32    // Range in feet for ranged weapons
	LongRange      int32    // Long range in feet for ranged weapons
}

// ArmorData contains armor-specific information
type ArmorData struct {
	ArmorCategory       string // "light", "medium", "heavy", "shield"
	BaseAC              int32
	DexBonus            bool
	HasDexLimit         bool  // Indicates if MaxDexBonus is applicable
	MaxDexBonus         int32 // Maximum Dexterity bonus to AC
	StrMinimum          int32
	StealthDisadvantage bool
}

// GearData contains general gear information
type GearData struct {
	GearCategory string // "adventuring-gear", "tools", etc.
	Properties   []string
}

// EncumbranceInfo tracks weight and carrying capacity
type EncumbranceInfo struct {
	CurrentWeight    int32            // Total weight carried (in tenths of pounds)
	CarryingCapacity int32            // Max weight before encumbered (in tenths of pounds)
	MaxCapacity      int32            // Max weight before immobilized (in tenths of pounds)
	Level            EncumbranceLevel // Current encumbrance level
}

// EncumbranceLevel represents different levels of encumbrance
type EncumbranceLevel string

const (
	// EncumbranceLevelUnencumbered means under carrying capacity
	EncumbranceLevelUnencumbered EncumbranceLevel = "unencumbered"
	// EncumbranceLevelEncumbered means speed reduced by 10 feet
	EncumbranceLevelEncumbered EncumbranceLevel = "encumbered"
	// EncumbranceLevelHeavilyEncumbered means speed reduced by 20 feet, disadvantage on ability checks
	EncumbranceLevelHeavilyEncumbered EncumbranceLevel = "heavily_encumbered"
	// EncumbranceLevelImmobilized means cannot move
	EncumbranceLevelImmobilized EncumbranceLevel = "immobilized"
)

// Equipment slot type constants
const (
	// EquipmentSlotMainHand is the main hand slot
	EquipmentSlotMainHand = "main_hand"
	// EquipmentSlotOffHand is the off hand slot
	EquipmentSlotOffHand = "off_hand"
	// EquipmentSlotArmor is the armor slot
	EquipmentSlotArmor = "armor"
	// EquipmentSlotHelmet is the helmet slot
	EquipmentSlotHelmet = "helmet"
	// EquipmentSlotBoots is the boots slot
	EquipmentSlotBoots = "boots"
	// EquipmentSlotGloves is the gloves slot
	EquipmentSlotGloves = "gloves"
	// EquipmentSlotCloak is the cloak slot
	EquipmentSlotCloak = "cloak"
	// EquipmentSlotAmulet is the amulet slot
	EquipmentSlotAmulet = "amulet"
	// EquipmentSlotRing1 is the first ring slot
	EquipmentSlotRing1 = "ring_1"
	// EquipmentSlotRing2 is the second ring slot
	EquipmentSlotRing2 = "ring_2"
	// EquipmentSlotBelt is the belt slot
	EquipmentSlotBelt = "belt"
)

// TODO(#46): Separate CharacterDraft into data and presentation models.
// Add ToData() method to convert for repository storage
