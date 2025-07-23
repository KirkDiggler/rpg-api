package character

import (
	"context"
	"fmt"
	"strings"

	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
)

// CompiledCharacterData represents the results of compiling choices into character data
type CompiledCharacterData struct {
	// Proficiencies compiled from all sources
	SkillProficiencies  []string
	ArmorProficiencies  []string
	WeaponProficiencies []string
	ToolProficiencies   []string
	SavingThrows        []string

	// Languages compiled from all sources
	Languages []string

	// Equipment compiled from choices
	Equipment []EquipmentItem

	// Ability score improvements from choices
	AbilityScoreImprovements map[string]int32
}

// EquipmentItem represents a piece of equipment with quantity
type EquipmentItem struct {
	ItemID   string
	Name     string
	Quantity int32
	Type     string // weapon, armor, gear, etc
}

// compileChoices takes a draft with choices and compiles them into character data
func (o *Orchestrator) compileChoices(
	ctx context.Context,
	draft *dnd5e.CharacterDraft,
	hydratedDraft *dnd5e.CharacterDraft,
) (*CompiledCharacterData, error) {
	result := &CompiledCharacterData{
		SkillProficiencies:       []string{},
		ArmorProficiencies:       []string{},
		WeaponProficiencies:      []string{},
		ToolProficiencies:        []string{},
		SavingThrows:             []string{},
		Languages:                []string{},
		Equipment:                []EquipmentItem{},
		AbilityScoreImprovements: make(map[string]int32),
	}

	// First, apply automatic grants from race/class/background
	o.applyAutomaticGrants(result, hydratedDraft)

	// Then, process player choices
	for _, selection := range draft.ChoiceSelections {
		if err := o.processChoiceSelection(ctx, result, selection, hydratedDraft); err != nil {
			return nil, fmt.Errorf("failed to process choice %s: %w", selection.ChoiceID, err)
		}
	}

	// Deduplicate lists
	result.SkillProficiencies = deduplicateStrings(result.SkillProficiencies)
	result.ArmorProficiencies = deduplicateStrings(result.ArmorProficiencies)
	result.WeaponProficiencies = deduplicateStrings(result.WeaponProficiencies)
	result.ToolProficiencies = deduplicateStrings(result.ToolProficiencies)
	result.SavingThrows = deduplicateStrings(result.SavingThrows)
	result.Languages = deduplicateStrings(result.Languages)

	return result, nil
}

// applyAutomaticGrants applies proficiencies and languages that are automatically granted
func (o *Orchestrator) applyAutomaticGrants(
	result *CompiledCharacterData,
	hydratedDraft *dnd5e.CharacterDraft,
) {
	// Apply race grants
	if hydratedDraft.Race != nil {
		result.Languages = append(result.Languages, hydratedDraft.Race.Languages...)
		result.WeaponProficiencies = append(result.WeaponProficiencies, hydratedDraft.Race.Proficiencies...)

		// Apply ability score bonuses from race
		for ability, bonus := range hydratedDraft.Race.AbilityBonuses {
			result.AbilityScoreImprovements[ability] += bonus
		}
	}

	// Apply subrace grants
	if hydratedDraft.Subrace != nil {
		result.Languages = append(result.Languages, hydratedDraft.Subrace.Languages...)
		result.WeaponProficiencies = append(result.WeaponProficiencies, hydratedDraft.Subrace.Proficiencies...)

		// Apply ability score bonuses from subrace
		for ability, bonus := range hydratedDraft.Subrace.AbilityBonuses {
			result.AbilityScoreImprovements[ability] += bonus
		}
	}

	// Apply class grants
	if hydratedDraft.Class != nil {
		result.ArmorProficiencies = append(result.ArmorProficiencies, hydratedDraft.Class.ArmorProficiencies...)
		result.WeaponProficiencies = append(result.WeaponProficiencies, hydratedDraft.Class.WeaponProficiencies...)
		result.ToolProficiencies = append(result.ToolProficiencies, hydratedDraft.Class.ToolProficiencies...)
		result.SavingThrows = append(result.SavingThrows, hydratedDraft.Class.SavingThrowProficiencies...)

		// Class starting equipment is handled through choices, not automatic grants
	}

	// Apply background grants
	if hydratedDraft.Background != nil {
		result.SkillProficiencies = append(result.SkillProficiencies, hydratedDraft.Background.SkillProficiencies...)
		result.ToolProficiencies = append(result.ToolProficiencies, hydratedDraft.Background.ToolProficiencies...)
		result.Languages = append(result.Languages, hydratedDraft.Background.Languages...)

		// Background starting equipment
		for _, item := range hydratedDraft.Background.StartingEquipment {
			result.Equipment = append(result.Equipment, EquipmentItem{
				ItemID:   item,
				Name:     item, // Will be resolved later
				Quantity: 1,
				Type:     "gear",
			})
		}
	}
}

// processChoiceSelection processes a single choice selection
func (o *Orchestrator) processChoiceSelection(
	ctx context.Context,
	result *CompiledCharacterData,
	selection dnd5e.ChoiceSelection,
	hydratedDraft *dnd5e.CharacterDraft,
) error {
	switch selection.ChoiceType {
	case dnd5e.ChoiceTypeSkill:
		result.SkillProficiencies = append(result.SkillProficiencies, selection.SelectedKeys...)

	case dnd5e.ChoiceTypeLanguage:
		result.Languages = append(result.Languages, selection.SelectedKeys...)

	case dnd5e.ChoiceTypeTool:
		result.ToolProficiencies = append(result.ToolProficiencies, selection.SelectedKeys...)

	case dnd5e.ChoiceTypeWeaponProficiency:
		result.WeaponProficiencies = append(result.WeaponProficiencies, selection.SelectedKeys...)

	case dnd5e.ChoiceTypeArmorProficiency:
		result.ArmorProficiencies = append(result.ArmorProficiencies, selection.SelectedKeys...)

	case dnd5e.ChoiceTypeEquipment:
		// Equipment choices need to be resolved from the choice definitions
		if err := o.processEquipmentChoice(ctx, result, selection, hydratedDraft); err != nil {
			return fmt.Errorf("failed to process equipment choice: %w", err)
		}

	case dnd5e.ChoiceTypeSpell, dnd5e.ChoiceTypeCantrips, dnd5e.ChoiceTypeSpells:
		// Spells are handled separately in the character's spell list
		// For now, we skip these as they're not part of basic proficiencies

	case dnd5e.ChoiceTypeFeat, dnd5e.ChoiceTypeFightingStyle:
		// These are features that need special handling
		// For now, we skip these as they require more complex implementation

	default:
		// Handle ability score choices
		for _, asChoice := range selection.AbilityScoreChoices {
			result.AbilityScoreImprovements[asChoice.Ability] += asChoice.Bonus
		}
	}

	return nil
}

// processEquipmentChoice processes equipment choices into actual items
func (o *Orchestrator) processEquipmentChoice(
	ctx context.Context,
	result *CompiledCharacterData,
	selection dnd5e.ChoiceSelection,
	hydratedDraft *dnd5e.CharacterDraft,
) error {
	// Find the choice definition from the hydrated data
	var choice *dnd5e.Choice

	// Check class choices
	if hydratedDraft.Class != nil {
		for i := range hydratedDraft.Class.Choices {
			if hydratedDraft.Class.Choices[i].ID == selection.ChoiceID {
				choice = &hydratedDraft.Class.Choices[i]
				break
			}
		}
	}

	// Check background choices if not found
	// (Background equipment choices would be in the Choices field if it existed)

	if choice == nil {
		// If we can't find the choice definition, just add the selected items by ID
		for _, itemID := range selection.SelectedKeys {
			result.Equipment = append(result.Equipment, EquipmentItem{
				ItemID:   itemID,
				Name:     itemID, // Will need to be resolved
				Quantity: 1,
				Type:     "gear",
			})
		}
		return nil
	}

	// Process the choice based on its option set
	switch optionSet := choice.OptionSet.(type) {
	case *dnd5e.ExplicitOptions:
		// Process each selected option
		for _, selectedKey := range selection.SelectedKeys {
			if err := o.processSelectedEquipmentOption(ctx, result, selectedKey, optionSet.Options); err != nil {
				return err
			}
		}

	case *dnd5e.CategoryReference:
		// For category references, we just add the selected items
		for _, itemID := range selection.SelectedKeys {
			result.Equipment = append(result.Equipment, EquipmentItem{
				ItemID:   itemID,
				Name:     itemID,
				Quantity: 1,
				Type:     "gear",
			})
		}
	}

	return nil
}

// processSelectedEquipmentOption processes a selected equipment option
func (o *Orchestrator) processSelectedEquipmentOption(
	ctx context.Context,
	result *CompiledCharacterData,
	selectedKey string,
	options []dnd5e.ChoiceOption,
) error {
	// Find the selected option
	for _, option := range options {
		switch opt := option.(type) {
		case *dnd5e.ItemReference:
			if opt.ItemID == selectedKey {
				result.Equipment = append(result.Equipment, EquipmentItem{
					ItemID:   opt.ItemID,
					Name:     opt.Name,
					Quantity: 1,
					Type:     "gear",
				})
				return nil
			}

		case *dnd5e.CountedItemReference:
			if opt.ItemID == selectedKey {
				result.Equipment = append(result.Equipment, EquipmentItem{
					ItemID:   opt.ItemID,
					Name:     opt.Name,
					Quantity: opt.Quantity,
					Type:     "gear",
				})
				return nil
			}

		case *dnd5e.ItemBundle:
			// For bundles, if the selected key contains "bundle",
			// we assume this bundle was selected
			// In a real implementation, bundles would have proper IDs
			if strings.Contains(selectedKey, "bundle") {
				for _, bundleItem := range opt.Items {
					if err := o.processBundleItem(ctx, result, bundleItem); err != nil {
						return err
					}
				}
				return nil
			}

		case *dnd5e.NestedChoice:
			// Nested choices would need special handling
			// For now, we skip these
		}
	}

	// If we didn't find the option, just add it as a basic item
	result.Equipment = append(result.Equipment, EquipmentItem{
		ItemID:   selectedKey,
		Name:     selectedKey,
		Quantity: 1,
		Type:     "gear",
	})

	return nil
}

// processBundleItem processes a single item from a bundle
func (o *Orchestrator) processBundleItem(
	_ context.Context,
	result *CompiledCharacterData,
	bundleItem dnd5e.BundleItem,
) error {
	switch itemType := bundleItem.ItemType.(type) {
	case *dnd5e.BundleItemConcreteItem:
		if itemType.ConcreteItem != nil {
			result.Equipment = append(result.Equipment, EquipmentItem{
				ItemID:   itemType.ConcreteItem.ItemID,
				Name:     itemType.ConcreteItem.Name,
				Quantity: itemType.ConcreteItem.Quantity,
				Type:     "gear",
			})
		}

	case *dnd5e.BundleItemChoiceItem:
		// Nested choices in bundles would need special handling
		// For now, we skip these
	}

	return nil
}

// deduplicateStrings removes duplicate strings from a slice
func deduplicateStrings(input []string) []string {
	seen := make(map[string]bool)
	result := []string{}

	for _, str := range input {
		if !seen[str] {
			seen[str] = true
			result = append(result, str)
		}
	}

	return result
}
