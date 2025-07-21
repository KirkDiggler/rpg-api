package client

import (
	"context"
	"fmt"
	"log"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/clients/dnd5e/api/v1alpha1"
)

var (
	raceJsonOutput bool
)

var getRaceCmd = &cobra.Command{
	Use:   "get-race [race-id]",
	Short: "Get details for a specific race",
	Long:  `Get detailed information about a specific D&D 5e race by its ID.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runGetRace,
}

func init() {
	getRaceCmd.Flags().BoolVar(&raceJsonOutput, "json", false, "Output as JSON")
}

func runGetRace(_ *cobra.Command, args []string) error {
	raceID := args[0]

	client, cleanup, err := createCharacterClient()
	if err != nil {
		return err
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	log.Printf("Requesting race '%s' from %s...", raceID, serverAddr)

	req := &dnd5ev1alpha1.GetRaceDetailsRequest{
		RaceId: raceID,
	}

	resp, err := client.GetRaceDetails(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to get race details: %w", err)
	}

	if raceJsonOutput {
		// Import the protojson package at the top of the file
		// "google.golang.org/protobuf/encoding/protojson"
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

	race := resp.Race
	fmt.Printf("🎭 %s (ID: %s)\n", race.Name, race.Id)

	if race.Description != "" {
		fmt.Printf("\nDescription:\n%s\n", race.Description)
	}

	fmt.Printf("\nBasic Info:\n")
	fmt.Printf("  Speed: %d ft\n", race.Speed)
	fmt.Printf("  Size: %s\n", race.Size)

	if race.AgeDescription != "" {
		fmt.Printf("\nAge: %s\n", race.AgeDescription)
	}

	if race.AlignmentDescription != "" {
		fmt.Printf("\nAlignment: %s\n", race.AlignmentDescription)
	}

	if len(race.AbilityBonuses) > 0 {
		fmt.Printf("\nAbility Score Increases:\n")
		for ability, bonus := range race.AbilityBonuses {
			if bonus > 0 {
				fmt.Printf("  %s: +%d\n", ability, bonus)
			} else if bonus < 0 {
				fmt.Printf("  %s: %d\n", ability, bonus)
			}
		}
	}

	if len(race.Proficiencies) > 0 {
		fmt.Printf("\nProficiencies:\n")
		for _, prof := range race.Proficiencies {
			fmt.Printf("  - %s\n", prof)
		}
	}

	if len(race.Languages) > 0 {
		fmt.Printf("\nLanguages:\n")
		for _, lang := range race.Languages {
			fmt.Printf("  - %s\n", lang)
		}
	}

	if len(race.Traits) > 0 {
		fmt.Printf("\nRacial Traits:\n")
		for _, trait := range race.Traits {
			fmt.Printf("  • %s", trait.Name)
			if trait.Description != "" {
				fmt.Printf(": %s", trait.Description)
			}
			fmt.Println()
		}
	}

	if len(race.Subraces) > 0 {
		fmt.Printf("\nSubraces:\n")
		for _, subrace := range race.Subraces {
			fmt.Printf("  🎭 %s (ID: %s)\n", subrace.Name, subrace.Id)
			if subrace.Description != "" {
				fmt.Printf("     %s\n", subrace.Description)
			}
			if len(subrace.AbilityBonuses) > 0 {
				fmt.Printf("     Ability Bonuses: ")
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
				fmt.Println()
			}
			if len(subrace.Traits) > 0 {
				fmt.Printf("     Traits:\n")
				for _, trait := range subrace.Traits {
					fmt.Printf("       - %s", trait.Name)
					if trait.Description != "" {
						fmt.Printf(": %s", trait.Description)
					}
					fmt.Println()
				}
			}
		}
	}

	return nil
}
