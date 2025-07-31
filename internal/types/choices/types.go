// Package choices provides types for representing complex character creation choices
// in the API layer. These types handle the presentation complexity of D&D 5e choices
// while the toolkit focuses on the simple game mechanics.
package choices

// Choice represents a choice definition for character creation
// This is what choices are available, not what was chosen
type Choice struct {
	ID          string          `json:"id"`
	Description string          `json:"description"`
	Type        ChoiceType      `json:"type"`
	ChooseCount int32           `json:"choose_count"`
	OptionSet   ChoiceOptionSet `json:"option_set"`
}

// ChoiceType represents the type of choice being made
type ChoiceType string

const (
	ChoiceTypeEquipment         ChoiceType = "equipment"
	ChoiceTypeSkill             ChoiceType = "skill"
	ChoiceTypeTool              ChoiceType = "tool"
	ChoiceTypeLanguage          ChoiceType = "language"
	ChoiceTypeWeaponProficiency ChoiceType = "weapon_proficiency"
	ChoiceTypeArmorProficiency  ChoiceType = "armor_proficiency"
	ChoiceTypeSpell             ChoiceType = "spell"
	ChoiceTypeFeat              ChoiceType = "feat"
	ChoiceTypeAbilityScore      ChoiceType = "ability_score"
	ChoiceTypeFightingStyle     ChoiceType = "fighting_style"
)

// ChoiceOptionSet represents the set of options for a choice
type ChoiceOptionSet interface {
	isChoiceOptionSet()
}

// ExplicitOptions contains a list of specific options to choose from
type ExplicitOptions struct {
	Options []ChoiceOption `json:"options"`
}

func (ExplicitOptions) isChoiceOptionSet() {}

// CategoryReference references a category that needs to be expanded
// This is an API concern - the toolkit doesn't care about categories
type CategoryReference struct {
	CategoryID string   `json:"category_id"` // e.g., "martial-weapons", "artisan-tools"
	ExcludeIDs []string `json:"exclude_ids,omitempty"`
}

func (CategoryReference) isChoiceOptionSet() {}

// ChoiceOption represents a single option within a choice
type ChoiceOption interface {
	isChoiceOption()
}

// ItemReference represents a single item option
type ItemReference struct {
	ItemID string `json:"item_id"`
	Name   string `json:"name"`
}

func (ItemReference) isChoiceOption() {}

// CountedItemReference represents an item with quantity
// This is purely for display - "20 arrows" vs "arrow x20"
type CountedItemReference struct {
	ItemID   string `json:"item_id"`
	Name     string `json:"name"`
	Quantity int32  `json:"quantity"`
}

func (CountedItemReference) isChoiceOption() {}

// ItemBundle represents multiple items as one option
// This is a presentation choice - "sword and shield" is one option
type ItemBundle struct {
	Items []BundleItem `json:"items"`
}

func (ItemBundle) isChoiceOption() {}

// NestedChoice represents a choice within a choice
// This allows complex UI flows like "choose a martial weapon" within a bundle
type NestedChoice struct {
	Choice *Choice `json:"choice"`
}

func (NestedChoice) isChoiceOption() {}

// BundleItem represents an item in a bundle
type BundleItem struct {
	ItemType BundleItemType `json:"item_type"`
}

// BundleItemType represents the type of item in a bundle
type BundleItemType interface {
	isBundleItemType()
}

// BundleItemConcreteItem represents a concrete item in a bundle
type BundleItemConcreteItem struct {
	ConcreteItem *CountedItemReference `json:"concrete_item"`
}

func (BundleItemConcreteItem) isBundleItemType() {}

// BundleItemChoiceItem represents a choice item in a bundle
type BundleItemChoiceItem struct {
	ChoiceItem *NestedChoice `json:"choice_item"`
}

func (BundleItemChoiceItem) isBundleItemType() {}
