package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
)

var encounterCmd = &cobra.Command{
	Use:   "encounter",
	Short: "Encounter client commands",
}

var dungeonStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start a dungeon encounter",
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, err := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}
		defer conn.Close()

		client := dnd5ev1alpha1.NewEncounterServiceClient(conn)

		// Start with test characters
		req := &dnd5ev1alpha1.DungeonStartRequest{
			CharacterIds: []string{"char-1", "char-2", "char-3"},
		}

		resp, err := client.DungeonStart(context.Background(), req)
		if err != nil {
			return fmt.Errorf("failed to start dungeon: %w", err)
		}

		// Pretty print the response
		data, _ := json.MarshalIndent(map[string]interface{}{
			"encounterId": resp.GetEncounterId(),
			"room": map[string]interface{}{
				"width":    resp.GetRoom().GetWidth(),
				"height":   resp.GetRoom().GetHeight(),
				"entities": len(resp.GetRoom().GetEntities()),
			},
			"combatState": map[string]interface{}{
				"round":       resp.GetCombatState().GetRound(),
				"currentTurn": resp.GetCombatState().GetCurrentTurn().GetEntityId(),
				"turnOrder":   len(resp.GetCombatState().GetTurnOrder()),
			},
		}, "", "  ")
		fmt.Println(string(data))

		// Save encounter ID for next commands
		fmt.Printf("\n# To end turn, run:\n")
		fmt.Printf("./bin/rpg-api encounter end-turn %s\n", resp.GetEncounterId())

		return nil
	},
}

var endTurnCmd = &cobra.Command{
	Use:   "end-turn [encounter-id]",
	Short: "End the current turn",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, err := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}
		defer conn.Close()

		client := dnd5ev1alpha1.NewEncounterServiceClient(conn)

		req := &dnd5ev1alpha1.EndTurnRequest{
			EncounterId: args[0],
		}

		resp, err := client.EndTurn(context.Background(), req)
		if err != nil {
			return fmt.Errorf("failed to end turn: %w", err)
		}

		// Show turn progression
		fmt.Printf("Turn ended successfully!\n")
		fmt.Printf("Round: %d\n", resp.GetCombatState().GetRound())
		fmt.Printf("Current Turn: %s\n", resp.GetCombatState().GetCurrentTurn().GetEntityId())
		fmt.Printf("\nTurn Order:\n")
		for i, entry := range resp.GetCombatState().GetTurnOrder() {
			active := ""
			if i == int(resp.GetCombatState().GetActiveIndex()) {
				active = " <- ACTIVE"
			}
			fmt.Printf("  %d. %s (Initiative: %d)%s\n",
				i+1,
				entry.GetEntityId(),
				entry.GetInitiative(),
				active,
			)
		}

		return nil
	},
}

var testFlowCmd = &cobra.Command{
	Use:   "test-flow",
	Short: "Test the full encounter flow",
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, err := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}
		defer conn.Close()

		client := dnd5ev1alpha1.NewEncounterServiceClient(conn)

		// Start encounter
		fmt.Println("=== Starting Encounter ===")
		startReq := &dnd5ev1alpha1.DungeonStartRequest{
			CharacterIds: []string{"char-1", "char-2", "char-3"},
		}

		startResp, err := client.DungeonStart(context.Background(), startReq)
		if err != nil {
			return fmt.Errorf("failed to start dungeon: %w", err)
		}

		encounterId := startResp.GetEncounterId()
		fmt.Printf("Encounter ID: %s\n", encounterId)
		fmt.Printf("Initial Turn: %s\n", startResp.GetCombatState().GetCurrentTurn().GetEntityId())

		// Test ending turns multiple times
		fmt.Println("\n=== Testing Turn Cycling ===")
		for i := 0; i < 5; i++ {
			endReq := &dnd5ev1alpha1.EndTurnRequest{
				EncounterId: encounterId,
			}

			endResp, err := client.EndTurn(context.Background(), endReq)
			if err != nil {
				return fmt.Errorf("failed to end turn %d: %w", i+1, err)
			}

			fmt.Printf("Turn %d ended -> New Turn: %s (Round %d)\n",
				i+1,
				endResp.GetCombatState().GetCurrentTurn().GetEntityId(),
				endResp.GetCombatState().GetRound(),
			)
		}

		fmt.Println("\n✓ Turn cycling test complete!")
		return nil
	},
}

func init() {
	encounterCmd.AddCommand(dungeonStartCmd)
	encounterCmd.AddCommand(endTurnCmd)
	encounterCmd.AddCommand(testFlowCmd)
	rootCmd.AddCommand(encounterCmd)
}
