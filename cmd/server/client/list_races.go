package client

import (
	"context"
	"fmt"
	"log"

	"github.com/spf13/cobra"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/clients/dnd5e/api/v1alpha1"
)

var (
	includeSubraces bool
)

var listRacesCmd = &cobra.Command{
	Use:   "list-races",
	Short: "List all available races",
	Long:  `List all available D&D 5e races with their details including ability bonuses and traits.`,
	RunE:  runListRaces,
}

func init() {
	listRacesCmd.Flags().BoolVar(&includeSubraces, "include-subraces", true, "Include subraces in the list")
}

func runListRaces(_ *cobra.Command, _ []string) error {
	client, cleanup, err := createCharacterClient()
	if err != nil {
		return err
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	log.Printf("Requesting races from %s...", serverAddr)

	req := &dnd5ev1alpha1.ListRacesRequest{
		IncludeSubraces: includeSubraces,
		PageSize:        100, // Get all races
	}

	resp, err := client.ListRaces(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to list races: %w", err)
	}

	fmt.Printf("Found %d races:\n\n", resp.TotalSize)

	for _, race := range resp.Races {
		fmt.Printf("🎭 %s (ID: %s)\n", race.Name, race.Id)

		if race.Description != "" {
			fmt.Printf("   Description: %s\n", race.Description)
		}

		fmt.Printf("   Speed: %d ft\n", race.Speed)
		fmt.Printf("   Size: %s\n", race.Size)

		if len(race.AbilityBonuses) > 0 {
			fmt.Printf("   Ability Bonuses:\n")
			for ability, bonus := range race.AbilityBonuses {
				if bonus > 0 {
					fmt.Printf("     - %s: +%d\n", ability, bonus)
				} else if bonus < 0 {
					fmt.Printf("     - %s: %d\n", ability, bonus)
				}
			}
		}

		if len(race.Traits) > 0 {
			fmt.Printf("   Traits:\n")
			for _, trait := range race.Traits {
				fmt.Printf("     - %s\n", trait.Name)
			}
		}

		if len(race.Subraces) > 0 {
			fmt.Printf("   Subraces:\n")
			for _, subrace := range race.Subraces {
				fmt.Printf("     - %s", subrace.Name)
				if len(subrace.AbilityBonuses) > 0 {
					fmt.Printf(" (")
					first := true
					for ability, bonus := range subrace.AbilityBonuses {
						if !first {
							fmt.Printf(", ")
						}
						first = false
						if bonus > 0 {
							fmt.Printf("%s +%d", ability, bonus)
						} else {
							fmt.Printf("%s %d", ability, bonus)
						}
					}
					fmt.Printf(")")
				}
				fmt.Println()
			}
		}

		fmt.Println() // Empty line between races
	}

	if resp.NextPageToken != "" {
		fmt.Printf("More results available. Next page token: %s\n", resp.NextPageToken)
	}

	return nil
}
