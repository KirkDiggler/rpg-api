package client

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/spf13/cobra"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/clients/dnd5e/api/v1alpha1"
)

var (
	spellLevel  int32
	classFilter string
)

var listSpellsCmd = &cobra.Command{
	Use:   "list-spells",
	Short: "List spells by level",
	Long: `List D&D 5e spells filtered by level (0-9, where 0 = cantrips).
Optionally filter by class such as:
- wizard
- sorcerer
- cleric
- bard
- warlock
- druid
- ranger
- paladin`,
	RunE: runListSpells,
}

func init() {
	listSpellsCmd.Flags().Int32Var(&spellLevel, "level", 0, "Spell level to filter by (0-9, where 0 = cantrips)")
	listSpellsCmd.Flags().StringVar(&classFilter, "class", "", "Class to filter by (optional)")
	listSpellsCmd.Flags().Int32Var(&pageSize, "page-size", 20, "Number of items per page")
}

func runListSpells(_ *cobra.Command, _ []string) error {
	client, cleanup, err := createCharacterClient()
	if err != nil {
		return err
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	levelDesc := "cantrips"
	if spellLevel > 0 {
		levelDesc = fmt.Sprintf("level %d", spellLevel)
	}

	if classFilter != "" {
		log.Printf("Requesting %s spells for %s from %s...", levelDesc, classFilter, serverAddr)
	} else {
		log.Printf("Requesting %s spells from %s...", levelDesc, serverAddr)
	}

	// Validate spell level
	if spellLevel < 0 || spellLevel > 9 {
		return fmt.Errorf("spell level must be between 0 and 9, got %d", spellLevel)
	}

	req := &dnd5ev1alpha1.ListSpellsByLevelRequest{
		Level:    spellLevel,
		PageSize: pageSize,
	}

	// Map class filter to enum if provided
	if classFilter != "" {
		var classEnum dnd5ev1alpha1.Class
		switch strings.ToLower(classFilter) {
		case "barbarian":
			classEnum = dnd5ev1alpha1.Class_CLASS_BARBARIAN
		case "bard":
			classEnum = dnd5ev1alpha1.Class_CLASS_BARD
		case "cleric":
			classEnum = dnd5ev1alpha1.Class_CLASS_CLERIC
		case "druid":
			classEnum = dnd5ev1alpha1.Class_CLASS_DRUID
		case "fighter":
			classEnum = dnd5ev1alpha1.Class_CLASS_FIGHTER
		case "monk":
			classEnum = dnd5ev1alpha1.Class_CLASS_MONK
		case "paladin":
			classEnum = dnd5ev1alpha1.Class_CLASS_PALADIN
		case "ranger":
			classEnum = dnd5ev1alpha1.Class_CLASS_RANGER
		case "rogue":
			classEnum = dnd5ev1alpha1.Class_CLASS_ROGUE
		case "sorcerer":
			classEnum = dnd5ev1alpha1.Class_CLASS_SORCERER
		case "warlock":
			classEnum = dnd5ev1alpha1.Class_CLASS_WARLOCK
		case "wizard":
			classEnum = dnd5ev1alpha1.Class_CLASS_WIZARD
		default:
			return fmt.Errorf("unknown class: %s", classFilter)
		}
		req.Class = classEnum
	}

	resp, err := client.ListSpellsByLevel(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to list spells: %w", err)
	}

	filterDesc := levelDesc
	if classFilter != "" {
		filterDesc = fmt.Sprintf("%s %s", classFilter, levelDesc)
	}

	fmt.Printf("Found %d %s spells:\n\n", resp.TotalSize, filterDesc)

	for _, spell := range resp.Spells {
		fmt.Printf("✨ %s (ID: %s)\n", spell.Name, spell.Id)

		fmt.Printf("   Level: %d", spell.Level)
		if spell.Level == 0 {
			fmt.Printf(" (cantrip)")
		}
		fmt.Println()

		if spell.School != "" {
			fmt.Printf("   School: %s\n", spell.School)
		}

		if spell.CastingTime != "" {
			fmt.Printf("   Casting Time: %s\n", spell.CastingTime)
		}

		if spell.Range != "" {
			fmt.Printf("   Range: %s\n", spell.Range)
		}

		if spell.Components != "" {
			fmt.Printf("   Components: %s\n", spell.Components)
		}

		if spell.Duration != "" {
			fmt.Printf("   Duration: %s\n", spell.Duration)
		}

		if len(spell.Classes) > 0 {
			fmt.Printf("   Classes: %s\n", strings.Join(spell.Classes, ", "))
		}

		if spell.Concentration {
			fmt.Printf("   🧠 Concentration: Yes\n")
		}

		if spell.Ritual {
			fmt.Printf("   📜 Ritual: Yes\n")
		}

		if spell.Damage != nil {
			fmt.Printf("   💥 Damage:\n")
			fmt.Printf("     - Type: %s\n", spell.Damage.DamageType)
			if len(spell.Damage.DamageAtSlotLevel) > 0 {
				fmt.Printf("     - Damage by slot level:\n")
				for _, damage := range spell.Damage.DamageAtSlotLevel {
					fmt.Printf("       Level %d: %s\n", damage.SlotLevel, damage.DamageDice)
				}
			}
		}

		if spell.AreaOfEffect != nil {
			fmt.Printf("   🎯 Area of Effect: %s (%d ft)\n", spell.AreaOfEffect.Type, spell.AreaOfEffect.Size)
		}

		if spell.Description != "" {
			fmt.Printf("   Description: %s\n", spell.Description)
		}

		fmt.Println() // Empty line between spells
	}

	if resp.NextPageToken != "" {
		fmt.Printf("More results available. Next page token: %s\n", resp.NextPageToken)
	}

	return nil
}
