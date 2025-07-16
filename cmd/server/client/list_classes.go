package client

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/spf13/cobra"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
)

var (
	includeFeatures bool
	spellcastersOnly bool
)

var listClassesCmd = &cobra.Command{
	Use:   "list-classes",
	Short: "List all available classes",
	Long:  `List all available D&D 5e classes with their details including hit dice, proficiencies, and features.`,
	RunE:  runListClasses,
}

func init() {
	listClassesCmd.Flags().BoolVar(&includeFeatures, "include-features", true, "Include class features in the list")
	listClassesCmd.Flags().BoolVar(&spellcastersOnly, "spellcasters-only", false, "Only show spellcasting classes")
}

func runListClasses(cmd *cobra.Command, args []string) error {
	client, cleanup, err := createCharacterClient()
	if err != nil {
		return err
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	log.Printf("Requesting classes from %s...", serverAddr)

	req := &dnd5ev1alpha1.ListClassesRequest{
		IncludeFeatures:         includeFeatures,
		IncludeSpellcastersOnly: spellcastersOnly,
		PageSize:                100, // Get all classes
	}

	resp, err := client.ListClasses(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to list classes: %w", err)
	}

	fmt.Printf("Found %d classes:\n\n", resp.TotalSize)

	for _, class := range resp.Classes {
		fmt.Printf("⚔️  %s (ID: %s)\n", class.Name, class.Id)
		
		if class.Description != "" {
			fmt.Printf("   Description: %s\n", class.Description)
		}
		
		fmt.Printf("   Hit Die: %s\n", class.HitDie)
		
		if len(class.PrimaryAbilities) > 0 {
			fmt.Printf("   Primary Abilities: %s\n", strings.Join(class.PrimaryAbilities, ", "))
		}
		
		if len(class.SavingThrowProficiencies) > 0 {
			fmt.Printf("   Saving Throws: %s\n", strings.Join(class.SavingThrowProficiencies, ", "))
		}
		
		if len(class.ArmorProficiencies) > 0 {
			fmt.Printf("   Armor Proficiencies: %s\n", strings.Join(class.ArmorProficiencies, ", "))
		}
		
		if len(class.WeaponProficiencies) > 0 {
			fmt.Printf("   Weapon Proficiencies: %s\n", strings.Join(class.WeaponProficiencies, ", "))
		}
		
		if class.SkillChoicesCount > 0 && len(class.AvailableSkills) > 0 {
			fmt.Printf("   Skills: Choose %d from %s\n", class.SkillChoicesCount, strings.Join(class.AvailableSkills, ", "))
		}
		
		if class.Spellcasting != nil {
			fmt.Printf("   🔮 Spellcasting:\n")
			fmt.Printf("     - Ability: %s\n", class.Spellcasting.SpellcastingAbility)
			if class.Spellcasting.CantripsKnown > 0 {
				fmt.Printf("     - Cantrips Known: %d\n", class.Spellcasting.CantripsKnown)
			}
			if class.Spellcasting.SpellsKnown > 0 {
				fmt.Printf("     - Spells Known: %d\n", class.Spellcasting.SpellsKnown)
			}
			if class.Spellcasting.SpellSlotsLevel_1 > 0 {
				fmt.Printf("     - Level 1 Spell Slots: %d\n", class.Spellcasting.SpellSlotsLevel_1)
			}
			if class.Spellcasting.RitualCasting {
				fmt.Printf("     - Can cast rituals\n")
			}
			if class.Spellcasting.SpellcastingFocus != "" {
				fmt.Printf("     - Focus: %s\n", class.Spellcasting.SpellcastingFocus)
			}
		}
		
		if includeFeatures && len(class.Level_1Features) > 0 {
			fmt.Printf("   Level 1 Features:\n")
			for _, feature := range class.Level_1Features {
				fmt.Printf("     - %s", feature.Name)
				if feature.Description != "" {
					fmt.Printf(": %s", feature.Description)
				}
				fmt.Println()
			}
		}
		
		fmt.Println() // Empty line between classes
	}

	if resp.NextPageToken != "" {
		fmt.Printf("More results available. Next page token: %s\n", resp.NextPageToken)
	}

	return nil
}