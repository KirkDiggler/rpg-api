package character

// Option types for presentation
const (
	OptionTypeSingleItem  = "single_item"
	OptionTypeCountedItem = "counted_item"
	OptionTypeBundle      = "bundle"
)

// Bundle item types
const (
	BundleItemTypeItem   = "item"
	BundleItemTypeChoice = "choice"
)

// Equipment categories
const (
	CategoryMartialWeapons     = "martial-weapons"
	CategorySimpleWeapons      = "simple-weapons"
	CategoryArtisansTools      = "artisans-tools"
	CategoryMusicalInstruments = "musical-instruments"
	CategoryGamingSets         = "gaming-sets"
)

// Choice ID prefixes
const (
	ChoiceIDPrefixSkill     = "skill_"
	ChoiceIDPrefixEquipment = "_equipment_"
	ChoiceIDPrefixFeature   = "feature_"
)

// Skill proficiency prefix
const SkillProficiencyPrefix = "Skill: "
