package client

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/clients/dnd5e/api/v1alpha1"
)

var (
	updateRaceDraftID string
	raceName          string
	subraceName       string
)

var updateRaceCmd = &cobra.Command{
	Use:   "update-race",
	Short: "Update character race in a draft",
	Long: `Update the character's race (and optionally subrace) in an existing draft.

Available races:
  - RACE_HUMAN
  - RACE_DWARF (subraces: SUBRACE_HILL_DWARF, SUBRACE_MOUNTAIN_DWARF)
  - RACE_ELF (subraces: SUBRACE_HIGH_ELF, SUBRACE_WOOD_ELF, SUBRACE_DARK_ELF)
  - RACE_HALFLING (subraces: SUBRACE_LIGHTFOOT_HALFLING, SUBRACE_STOUT_HALFLING)
  - RACE_DRAGONBORN
  - RACE_GNOME (subraces: SUBRACE_FOREST_GNOME, SUBRACE_ROCK_GNOME)
  - RACE_HALF_ELF
  - RACE_HALF_ORC
  - RACE_TIEFLING`,
	RunE: runUpdateRace,
}

func init() {
	updateRaceCmd.Flags().StringVar(&updateRaceDraftID, "draft-id", "", "Draft ID (required)")
	updateRaceCmd.Flags().StringVar(&raceName, "race", "", "Race name (required, e.g., RACE_HUMAN)")
	updateRaceCmd.Flags().StringVar(&subraceName, "subrace", "", "Subrace name (optional, e.g., SUBRACE_HIGH_ELF)")
	updateRaceCmd.MarkFlagRequired("draft-id")
	updateRaceCmd.MarkFlagRequired("race")
}

func runUpdateRace(cmd *cobra.Command, args []string) error {
	client, cleanup, err := createCharacterClient()
	if err != nil {
		return err
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Parse race enum
	race, err := parseRaceEnum(raceName)
	if err != nil {
		return err
	}

	// Parse subrace enum if provided
	var subrace dnd5ev1alpha1.Subrace
	if subraceName != "" {
		subrace, err = parseSubraceEnum(subraceName)
		if err != nil {
			return err
		}
	}

	req := &dnd5ev1alpha1.UpdateRaceRequest{
		DraftId: updateRaceDraftID,
		Race:    race,
		Subrace: subrace,
	}

	resp, err := client.UpdateRace(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to update race: %w", err)
	}

	draft := resp.Draft
	fmt.Printf("✅ Character race updated successfully!\n\n")
	fmt.Printf("Draft ID: %s\n", draft.Id)
	fmt.Printf("Race: %s\n", draft.Race)
	if draft.Subrace != dnd5ev1alpha1.Subrace_SUBRACE_UNSPECIFIED {
		fmt.Printf("Subrace: %s\n", draft.Subrace)
	}
	fmt.Printf("Completion: %d%%\n", draft.Progress.CompletionPercentage)

	if len(resp.Warnings) > 0 {
		fmt.Printf("\n⚠️  Warnings:\n")
		for _, warning := range resp.Warnings {
			fmt.Printf("  - %s: %s\n", warning.Field, warning.Message)
		}
	}

	fmt.Printf("\n💡 Note: Changing race resets ability scores, skills, and languages.\n")

	return nil
}

func parseRaceEnum(name string) (dnd5ev1alpha1.Race, error) {
	// Convert to uppercase and ensure it has the RACE_ prefix
	name = strings.ToUpper(name)
	if !strings.HasPrefix(name, "RACE_") {
		name = "RACE_" + name
	}

	switch name {
	case "RACE_HUMAN":
		return dnd5ev1alpha1.Race_RACE_HUMAN, nil
	case "RACE_DWARF":
		return dnd5ev1alpha1.Race_RACE_DWARF, nil
	case "RACE_ELF":
		return dnd5ev1alpha1.Race_RACE_ELF, nil
	case "RACE_HALFLING":
		return dnd5ev1alpha1.Race_RACE_HALFLING, nil
	case "RACE_DRAGONBORN":
		return dnd5ev1alpha1.Race_RACE_DRAGONBORN, nil
	case "RACE_GNOME":
		return dnd5ev1alpha1.Race_RACE_GNOME, nil
	case "RACE_HALF_ELF":
		return dnd5ev1alpha1.Race_RACE_HALF_ELF, nil
	case "RACE_HALF_ORC":
		return dnd5ev1alpha1.Race_RACE_HALF_ORC, nil
	case "RACE_TIEFLING":
		return dnd5ev1alpha1.Race_RACE_TIEFLING, nil
	default:
		return dnd5ev1alpha1.Race_RACE_UNSPECIFIED, fmt.Errorf("unknown race: %s", name)
	}
}

func parseSubraceEnum(name string) (dnd5ev1alpha1.Subrace, error) {
	// Convert to uppercase and ensure it has the SUBRACE_ prefix
	name = strings.ToUpper(name)
	if !strings.HasPrefix(name, "SUBRACE_") {
		name = "SUBRACE_" + name
	}

	switch name {
	case "SUBRACE_HIGH_ELF":
		return dnd5ev1alpha1.Subrace_SUBRACE_HIGH_ELF, nil
	case "SUBRACE_WOOD_ELF":
		return dnd5ev1alpha1.Subrace_SUBRACE_WOOD_ELF, nil
	case "SUBRACE_DARK_ELF":
		return dnd5ev1alpha1.Subrace_SUBRACE_DARK_ELF, nil
	case "SUBRACE_HILL_DWARF":
		return dnd5ev1alpha1.Subrace_SUBRACE_HILL_DWARF, nil
	case "SUBRACE_MOUNTAIN_DWARF":
		return dnd5ev1alpha1.Subrace_SUBRACE_MOUNTAIN_DWARF, nil
	case "SUBRACE_LIGHTFOOT_HALFLING":
		return dnd5ev1alpha1.Subrace_SUBRACE_LIGHTFOOT_HALFLING, nil
	case "SUBRACE_STOUT_HALFLING":
		return dnd5ev1alpha1.Subrace_SUBRACE_STOUT_HALFLING, nil
	case "SUBRACE_FOREST_GNOME":
		return dnd5ev1alpha1.Subrace_SUBRACE_FOREST_GNOME, nil
	case "SUBRACE_ROCK_GNOME":
		return dnd5ev1alpha1.Subrace_SUBRACE_ROCK_GNOME, nil
	default:
		return dnd5ev1alpha1.Subrace_SUBRACE_UNSPECIFIED, fmt.Errorf("unknown subrace: %s", name)
	}
}
