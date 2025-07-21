package client

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/clients/dnd5e/api/v1alpha1"
)

var (
	jsonOutput bool
)

var getClassCmd = &cobra.Command{
	Use:   "get-class [class-id]",
	Short: "Get details for a specific class",
	Long:  `Get detailed information about a specific D&D 5e class by its ID.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runGetClass,
}

func init() {
	getClassCmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
}

func runGetClass(_ *cobra.Command, args []string) error {
	classID := args[0]

	client, cleanup, err := createCharacterClient()
	if err != nil {
		return err
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	log.Printf("Requesting class '%s' from %s...", classID, serverAddr)

	req := &dnd5ev1alpha1.GetClassDetailsRequest{
		ClassId: classID,
	}

	resp, err := client.GetClassDetails(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to get class details: %w", err)
	}

	if jsonOutput {
		// Output as JSON
		marshaler := protojson.MarshalOptions{
			Indent:          "  ",
			EmitUnpopulated: false,
		}
		jsonBytes, err := marshaler.Marshal(resp)
		if err != nil {
			return fmt.Errorf("failed to marshal response to JSON: %w", err)
		}
		fmt.Println(string(jsonBytes))
		return nil
	}

	class := resp.Class
	printClassHeader(class)
	printClassBasicInfo(class)
	printClassProficiencies(class)
	printClassSkills(class)
	printClassEquipment(class)
	printClassFeatures(class)
	printClassSpellcasting(class)

	return nil
}

func printClassHeader(class *dnd5ev1alpha1.ClassInfo) {
	fmt.Printf("⚔️  %s (ID: %s)\n", class.Name, class.Id)

	if class.Description != "" {
		fmt.Printf("\nDescription:\n%s\n", class.Description)
	}
}

func printClassBasicInfo(class *dnd5ev1alpha1.ClassInfo) {
	fmt.Printf("\nBasic Info:\n")
	fmt.Printf("  Hit Die: %s\n", class.HitDie)

	if len(class.PrimaryAbilities) > 0 {
		fmt.Printf("  Primary Abilities: %s\n", strings.Join(class.PrimaryAbilities, ", "))
	}
}

func printClassProficiencies(class *dnd5ev1alpha1.ClassInfo) {
	fmt.Printf("\nProficiencies:\n")
	if len(class.ArmorProficiencies) > 0 {
		fmt.Printf("  Armor: %s\n", strings.Join(class.ArmorProficiencies, ", "))
	}
	if len(class.WeaponProficiencies) > 0 {
		fmt.Printf("  Weapons: %s\n", strings.Join(class.WeaponProficiencies, ", "))
	}
	if len(class.ToolProficiencies) > 0 {
		fmt.Printf("  Tools: %s\n", strings.Join(class.ToolProficiencies, ", "))
	}
	if len(class.SavingThrowProficiencies) > 0 {
		fmt.Printf("  Saving Throws: %s\n", strings.Join(class.SavingThrowProficiencies, ", "))
	}
}

func printClassSkills(class *dnd5ev1alpha1.ClassInfo) {
	if class.SkillChoicesCount > 0 && len(class.AvailableSkills) > 0 {
		fmt.Printf("\nSkills: Choose %d from:\n", class.SkillChoicesCount)
		for _, skill := range class.AvailableSkills {
			fmt.Printf("  - %s\n", skill)
		}
	}
}

func printClassEquipment(class *dnd5ev1alpha1.ClassInfo) {
	if len(class.StartingEquipment) > 0 {
		fmt.Printf("\nStarting Equipment:\n")
		for _, item := range class.StartingEquipment {
			fmt.Printf("  - %s\n", item)
		}
	}

	// Equipment choices are now part of the general choices array
	hasEquipmentChoices := false
	for _, choice := range class.Choices {
		if choice.ChoiceType == dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_EQUIPMENT {
			if !hasEquipmentChoices {
				fmt.Printf("\nEquipment Choices:\n")
				hasEquipmentChoices = true
			}
			fmt.Printf("  • %s\n", choice.Description)
			// Print options based on the option set type
			switch optSet := choice.OptionSet.(type) {
			case *dnd5ev1alpha1.Choice_ExplicitOptions:
				for _, opt := range optSet.ExplicitOptions.Options {
					printChoiceOption(opt, "    ")
				}
			case *dnd5ev1alpha1.Choice_CategoryReference:
				fmt.Printf("    - Choose from: %s\n", optSet.CategoryReference.CategoryId)
			}
		}
	}
}

func printClassFeatures(class *dnd5ev1alpha1.ClassInfo) {
	if len(class.Level_1Features) > 0 {
		fmt.Printf("\nLevel 1 Features:\n")
		for _, feature := range class.Level_1Features {
			fmt.Printf("  • %s", feature.Name)
			if feature.Level > 0 && feature.Level != 1 {
				fmt.Printf(" (Level %d)", feature.Level)
			}
			fmt.Println()
			if feature.Description != "" {
				// Indent description
				lines := strings.Split(feature.Description, "\n")
				for _, line := range lines {
					fmt.Printf("    %s\n", line)
				}
			}
			if feature.HasChoices && len(feature.Choices) > 0 {
				fmt.Printf("    Choices:\n")
				for _, choice := range feature.Choices {
					fmt.Printf("      %s (Choose %d)\n", choice.Description, choice.ChooseCount)
					// Print options based on the option set type
					switch optSet := choice.OptionSet.(type) {
					case *dnd5ev1alpha1.Choice_ExplicitOptions:
						for _, opt := range optSet.ExplicitOptions.Options {
							printChoiceOption(opt, "        ")
						}
					case *dnd5ev1alpha1.Choice_CategoryReference:
						fmt.Printf("        Choose from: %s\n", optSet.CategoryReference.CategoryId)
					}
				}
			}
			if feature.SpellSelection != nil {
				fmt.Printf("    Spell Selection:\n")
				fmt.Printf("      Spells to select: %d\n", feature.SpellSelection.SpellsToSelect)
				fmt.Printf("      Spell levels: %v\n", feature.SpellSelection.SpellLevels)
				fmt.Printf("      Spell lists: %v\n", feature.SpellSelection.SpellLists)
				fmt.Printf("      Selection type: %s\n", feature.SpellSelection.SelectionType)
				fmt.Printf("      Requires replace: %v\n", feature.SpellSelection.RequiresReplace)
			}
		}
	}
}

func printClassSpellcasting(class *dnd5ev1alpha1.ClassInfo) {
	if class.Spellcasting != nil {
		fmt.Printf("\n🔮 Spellcasting:\n")
		fmt.Printf("  Spellcasting Ability: %s\n", class.Spellcasting.SpellcastingAbility)
		if class.Spellcasting.SpellcastingFocus != "" {
			fmt.Printf("  Spellcasting Focus: %s\n", class.Spellcasting.SpellcastingFocus)
		}
		if class.Spellcasting.RitualCasting {
			fmt.Printf("  Ritual Casting: Yes\n")
		}
		if class.Spellcasting.CantripsKnown > 0 {
			fmt.Printf("  Cantrips Known at 1st Level: %d\n", class.Spellcasting.CantripsKnown)
		}
		if class.Spellcasting.SpellsKnown > 0 {
			fmt.Printf("  Spells Known at 1st Level: %d\n", class.Spellcasting.SpellsKnown)
		}
		if class.Spellcasting.SpellSlotsLevel_1 > 0 {
			fmt.Printf("  1st Level Spell Slots: %d\n", class.Spellcasting.SpellSlotsLevel_1)
		}
	}
}

// printChoiceOption prints a choice option with proper formatting
// isCategoryID checks if an item ID is a category reference
func isCategoryID(itemID string) bool {
	categoryIDs := []string{
		"martial-weapons",
		"simple-weapons",
		"shields",
		"light-armor",
		"medium-armor",
		"heavy-armor",
		"artisan-tools",
		"gaming-sets",
		"musical-instruments",
		"packs",
		"holy-symbols",
	}

	for _, cat := range categoryIDs {
		if itemID == cat {
			return true
		}
	}
	return false
}

func printChoiceOption(opt *dnd5ev1alpha1.ChoiceOption, indent string) {
	switch optType := opt.OptionType.(type) {
	case *dnd5ev1alpha1.ChoiceOption_Item:
		fmt.Printf("%s- %s\n", indent, optType.Item.Name)
	case *dnd5ev1alpha1.ChoiceOption_CountedItem:
		fmt.Printf("%s- %d %s\n", indent, optType.CountedItem.Quantity, optType.CountedItem.Name)
	case *dnd5ev1alpha1.ChoiceOption_Bundle:
		fmt.Printf("%s- Bundle:\n", indent)
		for _, bundleItem := range optType.Bundle.Items {
			switch itemType := bundleItem.ItemType.(type) {
			case *dnd5ev1alpha1.BundleItem_ConcreteItem:
				item := itemType.ConcreteItem
				fmt.Printf("%s  - %d %s\n", indent, item.Quantity, item.Name)
			case *dnd5ev1alpha1.BundleItem_ChoiceItem:
				nestedChoice := itemType.ChoiceItem.Choice
				fmt.Printf("%s  - Choose %d from: %s\n", indent, nestedChoice.ChooseCount, nestedChoice.Description)
			}
		}
	case *dnd5ev1alpha1.ChoiceOption_NestedChoice:
		nestedChoice := optType.NestedChoice.Choice
		fmt.Printf("%s- %s (Choose %d)\n", indent, nestedChoice.Description, nestedChoice.ChooseCount)
		// Show what the nested choice references
		switch optSet := nestedChoice.OptionSet.(type) {
		case *dnd5ev1alpha1.Choice_CategoryReference:
			fmt.Printf("%s  Choose from: %s\n", indent, optSet.CategoryReference.CategoryId)
		case *dnd5ev1alpha1.Choice_ExplicitOptions:
			// Could show the explicit options if needed
		}
	}
}
