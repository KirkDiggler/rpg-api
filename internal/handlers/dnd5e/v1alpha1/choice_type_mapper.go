package v1alpha1

import (
	"strings"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
)

// Choice type constants to prevent magic strings
const (
	choiceTypeSkill             = "skill"
	choiceTypeTool              = "tool"
	choiceTypeLanguage          = "language"
	choiceTypeWeaponProficiency = "weapon_proficiency"
	choiceTypeArmorProficiency  = "armor_proficiency"
	choiceTypeSpell             = "spell"
	choiceTypeFeat              = "feat"
	choiceTypeEquipment         = "equipment"
)

// ChoiceTypeMapper provides centralized mapping between different choice type representations
type ChoiceTypeMapper struct{}

// NewChoiceTypeMapper creates a new choice type mapper
func NewChoiceTypeMapper() *ChoiceTypeMapper {
	return &ChoiceTypeMapper{}
}

// NormalizeExternalType converts various external choice type strings to a canonical form
func (m *ChoiceTypeMapper) NormalizeExternalType(externalType string) string {
	normalized := strings.ToLower(strings.TrimSpace(externalType))

	// Handle common aliases and normalize to canonical forms
	switch normalized {
	// Skill-related aliases
	case "skill", "skills", "proficiencies", "skill_proficiency", "skill-proficiency":
		return choiceTypeSkill

	// Tool-related aliases
	case "tool", "tools", "tool_proficiency", "tool-proficiency", "tool_proficiencies":
		return choiceTypeTool

	// Language-related aliases
	case "language", "languages", "language_choice", "language-choice":
		return choiceTypeLanguage

	// Weapon proficiency aliases
	case "weapon", "weapons", "weapon_proficiency", "weapon-proficiency", "weapon_proficiencies":
		return choiceTypeWeaponProficiency

	// Armor proficiency aliases
	case "armor", "armors", "armor_proficiency", "armor-proficiency", "armor_proficiencies":
		return choiceTypeArmorProficiency

	// Spell-related aliases
	case "spell", "spells", "spell_choice", "spell-choice":
		return choiceTypeSpell

	// Feat-related aliases
	case "feat", "feats", "feature", "features", "feat_choice", "feat-choice":
		return choiceTypeFeat

	// Equipment-related aliases (most common, so put last)
	case "equipment", "gear", "starting_equipment", "starting-equipment", "equipment_choice", "equipment-choice":
		return choiceTypeEquipment

	default:
		// Return as-is if no normalization needed, but default to equipment for unknown types
		if normalized == "" {
			return choiceTypeEquipment
		}
		return normalized
	}
}

// ExternalToEntity converts normalized external type to internal entity type
func (m *ChoiceTypeMapper) ExternalToEntity(externalType string) dnd5e.ChoiceType {
	normalized := m.NormalizeExternalType(externalType)

	switch normalized {
	case choiceTypeSkill:
		return dnd5e.ChoiceTypeSkill
	case choiceTypeTool:
		return dnd5e.ChoiceTypeTool
	case choiceTypeLanguage:
		return dnd5e.ChoiceTypeLanguage
	case choiceTypeWeaponProficiency:
		return dnd5e.ChoiceTypeWeaponProficiency
	case choiceTypeArmorProficiency:
		return dnd5e.ChoiceTypeArmorProficiency
	case choiceTypeSpell:
		return dnd5e.ChoiceTypeSpell
	case choiceTypeFeat:
		return dnd5e.ChoiceTypeFeat
	default:
		return dnd5e.ChoiceTypeEquipment
	}
}

// EntityToProto converts internal entity type to proto enum
func (m *ChoiceTypeMapper) EntityToProto(entityType dnd5e.ChoiceType) dnd5ev1alpha1.ChoiceType {
	switch entityType {
	case dnd5e.ChoiceTypeEquipment:
		return dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_EQUIPMENT
	case dnd5e.ChoiceTypeSkill:
		return dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_SKILL
	case dnd5e.ChoiceTypeTool:
		return dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_TOOL
	case dnd5e.ChoiceTypeLanguage:
		return dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_LANGUAGE
	case dnd5e.ChoiceTypeWeaponProficiency:
		return dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_WEAPON_PROFICIENCY
	case dnd5e.ChoiceTypeArmorProficiency:
		return dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_ARMOR_PROFICIENCY
	case dnd5e.ChoiceTypeSpell:
		return dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_SPELL
	case dnd5e.ChoiceTypeFeat:
		return dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_FEAT
	default:
		return dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_UNSPECIFIED
	}
}

// ExternalToProto converts external type directly to proto enum (convenience method)
func (m *ChoiceTypeMapper) ExternalToProto(externalType string) dnd5ev1alpha1.ChoiceType {
	entityType := m.ExternalToEntity(externalType)
	return m.EntityToProto(entityType)
}

// Package-level mapper instance for convenience
var defaultMapper = NewChoiceTypeMapper()

// NormalizeExternalChoiceType normalizes external choice type strings using the default mapper
func NormalizeExternalChoiceType(externalType string) string {
	return defaultMapper.NormalizeExternalType(externalType)
}

// ConvertExternalChoiceTypeToEntity converts external choice type to entity choice type using the default mapper
func ConvertExternalChoiceTypeToEntity(externalType string) dnd5e.ChoiceType {
	return defaultMapper.ExternalToEntity(externalType)
}

// ConvertEntityChoiceTypeToProto converts entity choice type to proto choice type using the default mapper
func ConvertEntityChoiceTypeToProto(entityType dnd5e.ChoiceType) dnd5ev1alpha1.ChoiceType {
	return defaultMapper.EntityToProto(entityType)
}

// ConvertExternalChoiceTypeToProto converts external choice type to proto choice type using the default mapper
func ConvertExternalChoiceTypeToProto(externalType string) dnd5ev1alpha1.ChoiceType {
	return defaultMapper.ExternalToProto(externalType)
}
