package v1alpha1

import "strings"

// Equipment category constants to prevent magic strings and ensure consistency
// These constants match the D&D 5e API equipment categories
const (
	// Weapon categories
	EquipmentCategorySimpleWeapons        = "simple-weapons"
	EquipmentCategoryMartialWeapons       = "martial-weapons"
	EquipmentCategorySimpleMeleeWeapons   = "simple-melee-weapons"
	EquipmentCategorySimpleRangedWeapons  = "simple-ranged-weapons"
	EquipmentCategoryMartialMeleeWeapons  = "martial-melee-weapons"
	EquipmentCategoryMartialRangedWeapons = "martial-ranged-weapons"

	// Armor categories
	EquipmentCategoryLightArmor  = "light-armor"
	EquipmentCategoryMediumArmor = "medium-armor"
	EquipmentCategoryHeavyArmor  = "heavy-armor"
	EquipmentCategoryShields     = "shields"

	// Tool categories
	EquipmentCategoryArtisanTools       = "artisan-tools"
	EquipmentCategoryMusicalInstruments = "musical-instruments"
	EquipmentCategoryGamingSets         = "gaming-sets"
	EquipmentCategoryToolProficiencies  = "tool-proficiencies"

	// General equipment
	EquipmentCategoryAdventuringGear = "adventuring-gear"
	EquipmentCategoryEquipment       = "equipment"
	EquipmentCategoryVehicles        = "vehicles"
)

// EquipmentCategoryMapper provides mapping from descriptions to category constants
type EquipmentCategoryMapper struct {
	descriptionToCategory map[string]string
}

// NewEquipmentCategoryMapper creates a new equipment category mapper with predefined mappings
func NewEquipmentCategoryMapper() *EquipmentCategoryMapper {
	mapper := &EquipmentCategoryMapper{
		descriptionToCategory: make(map[string]string),
	}

	// Initialize common description-to-category mappings
	mapper.addMappings()

	return mapper
}

// addMappings initializes the description-to-category mapping table
func (m *EquipmentCategoryMapper) addMappings() {
	// Weapon mappings
	weaponMappings := map[string]string{
		"martial weapon":        EquipmentCategoryMartialWeapons,
		"martial melee weapon":  EquipmentCategoryMartialMeleeWeapons,
		"martial ranged weapon": EquipmentCategoryMartialRangedWeapons,
		"simple weapon":         EquipmentCategorySimpleWeapons,
		"simple melee weapon":   EquipmentCategorySimpleMeleeWeapons,
		"simple ranged weapon":  EquipmentCategorySimpleRangedWeapons,
		"any martial weapon":    EquipmentCategoryMartialWeapons,
		"any simple weapon":     EquipmentCategorySimpleWeapons,
	}

	// Armor mappings
	armorMappings := map[string]string{
		"light armor":  EquipmentCategoryLightArmor,
		"medium armor": EquipmentCategoryMediumArmor,
		"heavy armor":  EquipmentCategoryHeavyArmor,
		"shield":       EquipmentCategoryShields,
		"a shield":     EquipmentCategoryShields,
	}

	// Tool mappings
	toolMappings := map[string]string{
		"artisan's tools":    EquipmentCategoryArtisanTools,
		"artisan tool":       EquipmentCategoryArtisanTools,
		"musical instrument": EquipmentCategoryMusicalInstruments,
		"gaming set":         EquipmentCategoryGamingSets,
		"tool proficiencies": EquipmentCategoryToolProficiencies,
	}

	// General equipment mappings
	generalMappings := map[string]string{
		"adventuring gear": EquipmentCategoryAdventuringGear,
		"equipment":        EquipmentCategoryEquipment,
		"gear":             EquipmentCategoryAdventuringGear,
		"vehicle":          EquipmentCategoryVehicles,
	}

	// Combine all mappings
	for desc, category := range weaponMappings {
		m.descriptionToCategory[desc] = category
	}
	for desc, category := range armorMappings {
		m.descriptionToCategory[desc] = category
	}
	for desc, category := range toolMappings {
		m.descriptionToCategory[desc] = category
	}
	for desc, category := range generalMappings {
		m.descriptionToCategory[desc] = category
	}
}

// GetCategoryFromDescription attempts to extract equipment category from a description
func (m *EquipmentCategoryMapper) GetCategoryFromDescription(description string) string {
	if description == "" {
		return EquipmentCategoryEquipment
	}

	desc := strings.ToLower(strings.TrimSpace(description))

	// Try exact matches first
	if category, exists := m.descriptionToCategory[desc]; exists {
		return category
	}

	// Try partial matches with contains logic
	for descPattern, category := range m.descriptionToCategory {
		if strings.Contains(desc, descPattern) {
			return category
		}
	}

	// Default to general equipment
	return EquipmentCategoryEquipment
}

// Package-level mapper instance
var defaultEquipmentMapper = NewEquipmentCategoryMapper()

// GetEquipmentCategoryFromDescription is a convenience function using the default mapper
func GetEquipmentCategoryFromDescription(description string) string {
	return defaultEquipmentMapper.GetCategoryFromDescription(description)
}
