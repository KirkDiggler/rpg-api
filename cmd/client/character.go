package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
)

var (
	playerID string
	class    string
	race     string
	format   string
	verbose  bool
)

var characterCmd = &cobra.Command{
	Use:   "character",
	Short: "Test character service",
}

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Create a draft with class/race and show validation warnings",
	Long: `Creates a character draft, sets the race and class, and displays any validation warnings.
This is useful for testing class validation logic during development.`,
	RunE: func(_ *cobra.Command, _ []string) error {
		// Create gRPC connection
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		conn, err := grpc.NewClient(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}
		defer func() {
			if err := conn.Close(); err != nil {
				log.Printf("Failed to close connection: %v", err)
			}
		}()

		// Create client
		client := dnd5ev1alpha1.NewCharacterServiceClient(conn)

		// 1. Create draft
		if verbose {
			fmt.Println("Creating draft...")
		}
		createResp, err := client.CreateDraft(ctx, &dnd5ev1alpha1.CreateDraftRequest{
			PlayerId: playerID,
		})
		if err != nil {
			return fmt.Errorf("failed to create draft: %w", err)
		}
		draftID := createResp.Draft.Id
		if verbose {
			fmt.Printf("Created draft: %s\n", draftID)
		}

		// 2. Set race if provided
		if race != "" {
			if verbose {
				fmt.Printf("Setting race to %s...\n", race)
			}
			raceEnum := parseRace(race)
			_, err = client.UpdateRace(ctx, &dnd5ev1alpha1.UpdateRaceRequest{
				DraftId: draftID,
				Race:    raceEnum,
			})
			if err != nil {
				return fmt.Errorf("failed to set race: %w", err)
			}
		}

		// 3. Set class and get validation warnings
		if class != "" {
			if verbose {
				fmt.Printf("Setting class to %s...\n", class)
			}
			classEnum := parseClass(class)
			updateResp, err := client.UpdateClass(ctx, &dnd5ev1alpha1.UpdateClassRequest{
				DraftId:      draftID,
				Class:        classEnum,
				ClassChoices: []*dnd5ev1alpha1.ChoiceData{}, // No choices, to trigger validation
			})
			if err != nil {
				return fmt.Errorf("failed to set class: %w", err)
			}

			// Display results based on format
			if format == "json" {
				output, err := json.MarshalIndent(map[string]interface{}{
					"draft_id": draftID,
					"class":    class,
					"race":     race,
					"warnings": updateResp.Warnings,
				}, "", "  ")
				if err != nil {
					return fmt.Errorf("failed to marshal response: %w", err)
				}
				fmt.Println(string(output))
			} else {
				// Human-readable format
				fmt.Printf("\n=== Character Draft Validation ===\n")
				fmt.Printf("Draft ID: %s\n", draftID)
				if race != "" {
					fmt.Printf("Race: %s\n", race)
				}
				fmt.Printf("Class: %s\n", class)
				
				if len(updateResp.Warnings) > 0 {
					fmt.Printf("\n⚠️  Validation Warnings (%d):\n", len(updateResp.Warnings))
					for _, warning := range updateResp.Warnings {
						fmt.Printf("  • [%s] %s\n", warning.Field, warning.Message)
					}
				} else {
					fmt.Printf("\n✅ No validation warnings\n")
				}
			}
		}

		return nil
	},
}

func parseClass(class string) dnd5ev1alpha1.Class {
	switch strings.ToLower(class) {
	case "fighter":
		return dnd5ev1alpha1.Class_CLASS_FIGHTER
	case "wizard":
		return dnd5ev1alpha1.Class_CLASS_WIZARD
	case "cleric":
		return dnd5ev1alpha1.Class_CLASS_CLERIC
	case "rogue":
		return dnd5ev1alpha1.Class_CLASS_ROGUE
	case "ranger":
		return dnd5ev1alpha1.Class_CLASS_RANGER
	case "paladin":
		return dnd5ev1alpha1.Class_CLASS_PALADIN
	case "barbarian":
		return dnd5ev1alpha1.Class_CLASS_BARBARIAN
	case "bard":
		return dnd5ev1alpha1.Class_CLASS_BARD
	case "druid":
		return dnd5ev1alpha1.Class_CLASS_DRUID
	case "monk":
		return dnd5ev1alpha1.Class_CLASS_MONK
	case "sorcerer":
		return dnd5ev1alpha1.Class_CLASS_SORCERER
	case "warlock":
		return dnd5ev1alpha1.Class_CLASS_WARLOCK
	default:
		return dnd5ev1alpha1.Class_CLASS_UNSPECIFIED
	}
}

func parseRace(race string) dnd5ev1alpha1.Race {
	switch strings.ToLower(race) {
	case "human":
		return dnd5ev1alpha1.Race_RACE_HUMAN
	case "elf":
		return dnd5ev1alpha1.Race_RACE_ELF
	case "dwarf":
		return dnd5ev1alpha1.Race_RACE_DWARF
	case "halfling":
		return dnd5ev1alpha1.Race_RACE_HALFLING
	case "dragonborn":
		return dnd5ev1alpha1.Race_RACE_DRAGONBORN
	case "gnome":
		return dnd5ev1alpha1.Race_RACE_GNOME
	case "half-elf", "halfelf":
		return dnd5ev1alpha1.Race_RACE_HALF_ELF
	case "half-orc", "halforc":
		return dnd5ev1alpha1.Race_RACE_HALF_ORC
	case "tiefling":
		return dnd5ev1alpha1.Race_RACE_TIEFLING
	default:
		return dnd5ev1alpha1.Race_RACE_UNSPECIFIED
	}
}

// Add a command to create and inspect a draft with all choices
var inspectDraftCmd = &cobra.Command{
	Use:   "inspect-draft",
	Short: "Create a draft and show all choices and validation",
	Long:  `Creates a character draft with specified race/class and displays all generated choices and validation details.`,
	RunE: func(_ *cobra.Command, _ []string) error {
		// Create gRPC connection
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		conn, err := grpc.NewClient(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}
		defer func() {
			if err := conn.Close(); err != nil {
				log.Printf("Failed to close connection: %v", err)
			}
		}()

		// Create client
		client := dnd5ev1alpha1.NewCharacterServiceClient(conn)

		// 1. Create draft
		createResp, err := client.CreateDraft(ctx, &dnd5ev1alpha1.CreateDraftRequest{
			PlayerId: playerID,
		})
		if err != nil {
			return fmt.Errorf("failed to create draft: %w", err)
		}
		draftID := createResp.Draft.Id

		// 2. Set race
		if race != "" {
			raceEnum := parseRace(race)
			_, err = client.UpdateRace(ctx, &dnd5ev1alpha1.UpdateRaceRequest{
				DraftId: draftID,
				Race:    raceEnum,
			})
			if err != nil {
				return fmt.Errorf("failed to set race: %w", err)
			}
		}

		// 3. Set class
		if class != "" {
			classEnum := parseClass(class)
			_, err = client.UpdateClass(ctx, &dnd5ev1alpha1.UpdateClassRequest{
				DraftId:      draftID,
				Class:        classEnum,
				ClassChoices: []*dnd5ev1alpha1.ChoiceData{},
			})
			if err != nil {
				return fmt.Errorf("failed to set class: %w", err)
			}
		}

		// 4. Get the full draft with choices and validation
		getDraftResp, err := client.GetDraft(ctx, &dnd5ev1alpha1.GetDraftRequest{
			DraftId: draftID,
		})
		if err != nil {
			return fmt.Errorf("failed to get draft: %w", err)
		}

		// Output based on format
		if format == "json" {
			output, err := json.MarshalIndent(getDraftResp, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal JSON: %w", err)
			}
			fmt.Println(string(output))
		} else {
			fmt.Printf("=== Draft: %s ===\n", draftID)
			fmt.Printf("Race: %s\n", getDraftResp.Draft.RaceId)
			fmt.Printf("Class: %s\n", getDraftResp.Draft.ClassId)
			
			if len(getDraftResp.Draft.Choices) > 0 {
				fmt.Printf("\n📋 Choices (%d):\n", len(getDraftResp.Draft.Choices))
				for _, choice := range getDraftResp.Draft.Choices {
					fmt.Printf("  • [%s] %s from %s\n", choice.Category, choice.ChoiceId, choice.Source)
				}
			}

			if getDraftResp.Draft.Validation != nil {
				if len(getDraftResp.Draft.Validation.Issues) > 0 {
					fmt.Printf("\n⚠️  Validation Issues (%d):\n", len(getDraftResp.Draft.Validation.Issues))
					for _, issue := range getDraftResp.Draft.Validation.Issues {
						fmt.Printf("  • [%s:%s] %s - %s\n", issue.Severity, issue.Field, issue.Message, issue.Source)
					}
				} else {
					fmt.Printf("\n✅ No validation issues\n")
				}
			}
		}

		return nil
	},
}

func init() {
	validateCmd.Flags().StringVar(&playerID, "player-id", "test-player", "Player ID for the draft")
	validateCmd.Flags().StringVar(&class, "class", "fighter", "Character class (fighter, wizard, etc.)")
	validateCmd.Flags().StringVar(&race, "race", "human", "Character race (human, elf, etc.)")
	validateCmd.Flags().StringVar(&format, "format", "text", "Output format (text or json)")
	validateCmd.Flags().BoolVar(&verbose, "verbose", false, "Show detailed progress")

	inspectDraftCmd.Flags().StringVar(&playerID, "player-id", "test-player", "Player ID for the draft")
	inspectDraftCmd.Flags().StringVar(&class, "class", "rogue", "Character class (fighter, wizard, rogue, etc.)")
	inspectDraftCmd.Flags().StringVar(&race, "race", "human", "Character race (human, elf, dragonborn, etc.)")
	inspectDraftCmd.Flags().StringVar(&format, "format", "text", "Output format (text or json)")

	characterCmd.AddCommand(validateCmd)
	characterCmd.AddCommand(inspectDraftCmd)
}