package character

import (
	"fmt"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/class"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

// convertClassDataToPresentation converts raw class data to presentation format
// This includes resolving equipment choices and creating proper nested structures
func (o *Orchestrator) convertClassDataToPresentation(classData *class.Data) *PresentationClass {
	if classData == nil {
		return nil
	}

	pc := &PresentationClass{
		ID:          string(classData.ID),
		Name:        classData.Name,
		Description: classData.Description,
		HitDie:      fmt.Sprintf("1d%d", classData.HitDice),
		Choices:     make([]PresentationChoice, 0),
	}

	// Convert saving throws
	pc.SavingThrows = make([]string, 0, len(classData.SavingThrows))
	for _, st := range classData.SavingThrows {
		pc.SavingThrows = append(pc.SavingThrows, string(st))
	}

	// Convert weapon proficiencies
	pc.WeaponProficiencies = make([]string, 0, len(classData.WeaponProficiencies))
	for _, wp := range classData.WeaponProficiencies {
		pc.WeaponProficiencies = append(pc.WeaponProficiencies, string(wp))
	}

	// Convert armor proficiencies
	pc.ArmorProficiencies = make([]string, 0, len(classData.ArmorProficiencies))
	for _, ap := range classData.ArmorProficiencies {
		pc.ArmorProficiencies = append(pc.ArmorProficiencies, string(ap))
	}

	// Set skill choice metadata
	pc.SkillChoicesCount = classData.SkillProficiencyCount
	pc.AvailableSkills = make([]string, 0, len(classData.SkillOptions))
	for _, skill := range classData.SkillOptions {
		pc.AvailableSkills = append(pc.AvailableSkills, string(skill))
	}

	// Add skill choice if present
	if classData.SkillProficiencyCount > 0 && len(classData.SkillOptions) > 0 {
		skillChoice := PresentationChoice{
			ID:          fmt.Sprintf("%s_skills", classData.ID),
			Description: fmt.Sprintf("Choose %d skills", classData.SkillProficiencyCount),
			Category:    shared.ChoiceSkills,
			ChooseCount: classData.SkillProficiencyCount,
			Options:     make([]PresentationOption, 0, len(classData.SkillOptions)),
		}

		for _, skill := range classData.SkillOptions {
			skillChoice.Options = append(skillChoice.Options, PresentationOption{
				Type: OptionTypeSingleItem,
				Item: &PresentationItem{
					ItemID: fmt.Sprintf("%s%s", ChoiceIDPrefixSkill, skill),
					Name:   string(skill),
				},
			})
		}

		pc.Choices = append(pc.Choices, skillChoice)
	}

	// Convert equipment choices
	for i, equipChoice := range classData.EquipmentChoices {
		pc.Choices = append(pc.Choices, o.convertEquipmentChoice(string(classData.ID), i+1, &equipChoice))
	}

	// Add level 1 features (like fighting style)
	if features, ok := classData.Features[1]; ok {
		for _, feature := range features {
			// Only add features that have choices
			if feature.Choice != nil {
				// Use appropriate category based on feature type
				category := shared.ChoiceCategory("feature") // generic feature choice
				if feature.Choice.Type == "fighting_style" {
					category = shared.ChoiceFightingStyle
				}

				featureChoice := PresentationChoice{
					ID:          fmt.Sprintf("%s_feature_%s", classData.ID, feature.Choice.ID),
					Description: feature.Choice.Description,
					Category:    category,
					ChooseCount: feature.Choice.Choose,
					Options:     make([]PresentationOption, 0, len(feature.Choice.From)),
				}

				// Convert feature options
				for _, optionName := range feature.Choice.From {
					// Feature choices are typically single item selections
					featureChoice.Options = append(featureChoice.Options, PresentationOption{
						Type: OptionTypeSingleItem,
						Item: &PresentationItem{
							ItemID: fmt.Sprintf("%s%s", ChoiceIDPrefixFeature, optionName),
							Name:   optionName,
						},
					})
				}

				pc.Choices = append(pc.Choices, featureChoice)
			}
		}
	}

	return pc
}

// convertEquipmentChoice converts a single equipment choice to presentation format
func (o *Orchestrator) convertEquipmentChoice(classID string, index int, choice *class.EquipmentChoiceData) PresentationChoice {
	pc := PresentationChoice{
		ID:          fmt.Sprintf("%s%s%d", classID, ChoiceIDPrefixEquipment, index),
		Description: choice.Description,
		Category:    shared.ChoiceEquipment,
		ChooseCount: choice.Choose,
		Options:     make([]PresentationOption, 0, len(choice.Options)),
	}

	for _, opt := range choice.Options {
		pc.Options = append(pc.Options, o.convertEquipmentOption(opt))
	}

	return pc
}

// convertEquipmentOption converts an equipment option to presentation format
func (o *Orchestrator) convertEquipmentOption(opt class.EquipmentOption) PresentationOption {
	// Single item option
	if len(opt.Items) == 1 && opt.Items[0].ConcreteItem != nil {
		item := opt.Items[0].ConcreteItem
		if item.Quantity == 1 {
			return PresentationOption{
				Type: OptionTypeSingleItem,
				Item: &PresentationItem{
					ItemID: item.ItemID,
					Name:   item.ItemID, // TODO: Resolve actual item name
				},
			}
		}
		return PresentationOption{
			Type: OptionTypeCountedItem,
			Item: &PresentationItem{
				ItemID:   item.ItemID,
				Name:     item.ItemID, // TODO: Resolve actual item name
				Quantity: item.Quantity,
			},
		}
	}

	// Bundle option
	bundle := &PresentationBundle{
		Items: make([]PresentationBundleItem, 0, len(opt.Items)),
	}

	for _, bundleItem := range opt.Items {
		if bundleItem.ConcreteItem != nil {
			bundle.Items = append(bundle.Items, PresentationBundleItem{
				Type: BundleItemTypeItem,
				Item: &PresentationItem{
					ItemID:   bundleItem.ConcreteItem.ItemID,
					Name:     bundleItem.ConcreteItem.ItemID, // TODO: Resolve actual item name
					Quantity: bundleItem.ConcreteItem.Quantity,
				},
			})
		} else if bundleItem.NestedChoice != nil {
			bundle.Items = append(bundle.Items, PresentationBundleItem{
				Type:         BundleItemTypeChoice,
				NestedChoice: o.convertNestedChoice(bundleItem.NestedChoice),
			})
		}
	}

	return PresentationOption{
		Type:   OptionTypeBundle,
		Bundle: bundle,
	}
}

// convertNestedChoice converts a nested equipment choice to presentation format
func (o *Orchestrator) convertNestedChoice(choice *class.EquipmentChoiceData) *PresentationChoice {
	pc := &PresentationChoice{
		ID:          choice.ID,
		Description: choice.Description,
		Category:    shared.ChoiceEquipment,
		ChooseCount: choice.Choose,
		Options:     make([]PresentationOption, 0, len(choice.Options)),
	}

	for _, opt := range choice.Options {
		// Nested choices should only have single items
		if len(opt.Items) > 0 && opt.Items[0].ConcreteItem != nil {
			item := opt.Items[0].ConcreteItem
			pc.Options = append(pc.Options, PresentationOption{
				Type: OptionTypeSingleItem,
				Item: &PresentationItem{
					ItemID: item.ItemID,
					Name:   item.ItemID, // TODO: Resolve actual item name
				},
			})
		}
	}

	return pc
}
