package client

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/clients/dnd5e/api/v1alpha1"
)

var rollAbilityScoresCmd = &cobra.Command{
	Use:   "roll-ability-scores [draft-id]",
	Short: "Roll ability scores for character creation",
	Long: `Roll 6 sets of ability scores using 4d6 drop lowest for D&D character creation.
  
  Example: roll-ability-scores draft-abc123`,
	Args: cobra.ExactArgs(1),
	RunE: rollAbilityScores,
}

func rollAbilityScores(cmd *cobra.Command, args []string) error {
	draftID := args[0]

	client, cleanup, err := createCharacterClient()
	if err != nil {
		return err
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	fmt.Printf("Rolling ability scores for draft %s...\n", draftID)

	req := &dnd5ev1alpha1.RollAbilityScoresRequest{
		DraftId: draftID,
	}

	resp, err := client.RollAbilityScores(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to roll ability scores: %w", err)
	}

	fmt.Printf("\n🎲 Ability Score Rolls (4d6 drop lowest):\n")
	fmt.Printf("=========================================\n")

	abilities := []string{"Strength", "Dexterity", "Constitution", "Intelligence", "Wisdom", "Charisma"}

	for i, roll := range resp.Rolls {
		var abilityName string
		if i < len(abilities) {
			abilityName = abilities[i]
		} else {
			abilityName = fmt.Sprintf("Ability %d", i+1)
		}

		fmt.Printf("\n%s:\n", abilityName)
		fmt.Printf("  Roll ID: %s\n", roll.RollId)
		fmt.Printf("  All Dice: %v\n", roll.Dice)
		fmt.Printf("  Dropped: %d\n", roll.Dropped)
		fmt.Printf("  Final Score: %d\n", roll.Total)
		fmt.Printf("  Notation: %s\n", roll.Notation)
	}

	fmt.Printf("\nSession expires at: %d\n", resp.ExpiresAt)
	fmt.Printf("\n💡 These rolls are stored for 15 minutes so you can assign them to abilities!\n")
	fmt.Printf("💡 Use 'get-roll-session %s ability_scores' to see them again.\n", draftID)

	return nil
}
