package client

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	apiv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/clients/api/v1alpha1"
)

var rollDiceCmd = &cobra.Command{
	Use:   "roll-dice [notation] [entity-id] [context]",
	Short: "Roll dice using dice notation",
	Long: `Roll dice and see individual results. Examples:
  
  roll-dice 4d6 char-123 ability-scores
  roll-dice 1d20 char-456 attack
  roll-dice 2d8 char-789 damage`,
	Args: cobra.ExactArgs(3),
	RunE: rollDice,
}

func rollDice(cmd *cobra.Command, args []string) error {
	notation := args[0]
	entityID := args[1]
	rollContext := args[2]

	client, cleanup, err := createDiceClient()
	if err != nil {
		return err
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	fmt.Printf("Rolling %s for entity %s (context: %s)...\n", notation, entityID, rollContext)

	req := &apiv1alpha1.RollDiceRequest{
		EntityId: entityID,
		Context:  rollContext,
		Notation: notation,
	}

	resp, err := client.RollDice(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to roll dice: %w", err)
	}

	fmt.Printf("\n🎲 Dice Roll Results:\n")
	fmt.Printf("===================\n")

	for i, roll := range resp.Rolls {
		fmt.Printf("\nRoll %d:\n", i+1)
		fmt.Printf("  Roll ID: %s\n", roll.RollId)
		fmt.Printf("  Notation: %s\n", roll.Notation)
		fmt.Printf("  Individual Dice: %v\n", roll.Dice)
		fmt.Printf("  Total: %d\n", roll.Total)
		if len(roll.Dropped) > 0 {
			fmt.Printf("  Dropped: %v\n", roll.Dropped)
		}
		if roll.Description != "" {
			fmt.Printf("  Description: %s\n", roll.Description)
		}
	}

	fmt.Printf("\nSession expires at: %d\n", resp.ExpiresAt)
	fmt.Printf("Total rolls in session: %d\n", len(resp.Rolls))

	return nil
}
