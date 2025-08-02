package character

import (
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

// PresentationChoice represents a choice prepared for API presentation
// This contains all the resolved data and proper structure for the handler to simply map to proto
type PresentationChoice struct {
	ID          string
	Description string
	Category    shared.ChoiceCategory
	ChooseCount int
	Options     []PresentationOption
}

// PresentationOption represents a choice option ready for presentation
type PresentationOption struct {
	Type   string // Use constants: OptionTypeSingleItem, OptionTypeCountedItem, OptionTypeBundle
	Item   *PresentationItem
	Bundle *PresentationBundle
}

// PresentationItem represents a single item
type PresentationItem struct {
	ItemID   string
	Name     string
	Quantity int
}

// PresentationBundle represents a bundle of items
type PresentationBundle struct {
	Items []PresentationBundleItem
}

// PresentationBundleItem can be a concrete item or a nested choice
type PresentationBundleItem struct {
	Type         string // Use constants: BundleItemTypeItem, BundleItemTypeChoice
	Item         *PresentationItem
	NestedChoice *PresentationChoice
}

// PresentationClass represents a class prepared for API presentation
type PresentationClass struct {
	ID                  string
	Name                string
	Description         string
	HitDie              string
	SavingThrows        []string
	WeaponProficiencies []string
	ArmorProficiencies  []string
	SkillChoicesCount   int
	AvailableSkills     []string
	Choices             []PresentationChoice
}
