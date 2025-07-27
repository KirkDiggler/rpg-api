package client

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/spf13/cobra"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
)

var listBackgroundsCmd = &cobra.Command{
	Use:   "list-backgrounds",
	Short: "List all available backgrounds",
	Long:  `List all available D&D 5e backgrounds with their details including skills, languages, and features.`,
	RunE:  runListBackgrounds,
}

func runListBackgrounds(_ *cobra.Command, _ []string) error {
	client, cleanup, err := createCharacterClient()
	if err != nil {
		return err
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	log.Printf("Requesting backgrounds from %s...", serverAddr)

	req := &dnd5ev1alpha1.ListBackgroundsRequest{
		PageSize: 100, // Get all backgrounds
	}

	resp, err := client.ListBackgrounds(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to list backgrounds: %w", err)
	}

	fmt.Printf("Found %d backgrounds:\n\n", resp.TotalSize)

	for _, bg := range resp.Backgrounds {
		fmt.Printf("📜 %s (ID: %s)\n", bg.Name, bg.Id)

		if bg.Description != "" {
			fmt.Printf("   Description: %s\n", bg.Description)
		}

		if len(bg.SkillProficiencies) > 0 {
			fmt.Printf("   Skill Proficiencies: %s\n", strings.Join(bg.SkillProficiencies, ", "))
		}

		if len(bg.ToolProficiencies) > 0 {
			fmt.Printf("   Tool Proficiencies: %s\n", strings.Join(bg.ToolProficiencies, ", "))
		}

		if len(bg.Languages) > 0 {
			languages := make([]string, len(bg.Languages))
			for i, lang := range bg.Languages {
				// Convert enum to string - just use the enum name for now
				languages[i] = lang.String()
			}
			fmt.Printf("   Languages: %s\n", strings.Join(languages, ", "))
		} else if bg.AdditionalLanguages > 0 {
			fmt.Printf("   Languages: Choose %d of your choice\n", bg.AdditionalLanguages)
		}

		if bg.StartingGold > 0 {
			fmt.Printf("   Starting Gold: %d gp\n", bg.StartingGold)
		}

		if len(bg.StartingEquipment) > 0 {
			fmt.Printf("   Starting Equipment:\n")
			for _, item := range bg.StartingEquipment {
				fmt.Printf("     - %s\n", item)
			}
		}

		if bg.FeatureName != "" {
			fmt.Printf("   Feature: %s\n", bg.FeatureName)
			if bg.FeatureDescription != "" {
				fmt.Printf("     %s\n", bg.FeatureDescription)
			}
		}

		fmt.Println() // Empty line between backgrounds
	}

	if resp.NextPageToken != "" {
		fmt.Printf("More results available. Next page token: %s\n", resp.NextPageToken)
	}

	return nil
}
