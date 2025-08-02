package external

import (
	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
)

// Re-export entity types for backward compatibility
type (
	RaceData            = dnd5e.RaceData
	SubraceData         = dnd5e.SubraceData
	ClassData           = dnd5e.ClassData
	BackgroundData      = dnd5e.BackgroundData
	SpellData           = dnd5e.SpellData
	TraitData           = dnd5e.TraitData
	ChoiceData          = dnd5e.ChoiceData
	EquipmentChoiceData = dnd5e.EquipmentChoiceData
	SpellcastingData    = dnd5e.SpellcastingData
	EquipmentData       = dnd5e.EquipmentAPIData
	FeatureData         = dnd5e.FeatureData
	SpellSelectionData  = dnd5e.SpellSelectionData
	CostData            = dnd5e.CostData
	DamageData          = dnd5e.DamageData
	ArmorClassData      = dnd5e.ArmorClassData
)

// ListSpellsInput represents input for listing spells
type ListSpellsInput struct {
	Level   *int32 // Optional filter by spell level (0-9)
	ClassID string // Optional filter by class ID
}
