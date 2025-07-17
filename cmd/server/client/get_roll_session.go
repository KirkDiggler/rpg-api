package client

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	apiv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/clients/api/v1alpha1"
)

// getRollSessionCmd is currently unused but kept for future implementation
// var getRollSessionCmd = &cobra.Command{
// 	Use:   "get-roll-session [entity-id] [context]",
// 	Short: "Get existing dice roll session",
// 	Long: `Retrieve all dice rolls for a specific entity and context. Examples:
//
//   get-roll-session char-123 ability-scores
//   get-roll-session char-456 combat`,
// 	Args: cobra.ExactArgs(2),
// 	RunE: getRollSession,
// }

// getRollSession is currently unused but kept for future implementation
func getRollSession(_ *cobra.Command, args []string) error { // nolint:unused
	entityID := args[0]
	rollContext := args[1]

	client, cleanup, err := createDiceClient()
	if err != nil {
		return err
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	fmt.Printf("Getting roll session for entity %s (context: %s)...\n", entityID, rollContext)

	req := &apiv1alpha1.GetRollSessionRequest{
		EntityId: entityID,
		Context:  rollContext,
	}

	resp, err := client.GetRollSession(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to get roll session: %w", err)
	}

	fmt.Printf("\n📜 Roll Session:\n")
	fmt.Printf("================\n")

	createdAt := time.Unix(resp.CreatedAt, 0)
	expiresAt := time.Unix(resp.ExpiresAt, 0)

	fmt.Printf("Created: %s\n", createdAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Expires: %s\n", expiresAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Total Rolls: %d\n", len(resp.Rolls))

	for i, roll := range resp.Rolls {
		fmt.Printf("\n🎲 Roll %d:\n", i+1)
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

	return nil
}

// createDiceClient creates a dice service client
func createDiceClient() (apiv1alpha1.DiceServiceClient, func(), error) { // nolint:unused
	conn, err := createConnection()
	if err != nil {
		return nil, nil, err
	}

	cleanup := func() {
		_ = conn.Close() // nolint:errcheck // safe to ignore in cleanup
	}

	client := apiv1alpha1.NewDiceServiceClient(conn)
	return client, cleanup, nil
}
