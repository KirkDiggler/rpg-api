// Package main provides a command-line client for testing RPG API services
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
)

var (
	serverAddr string
	timeout    time.Duration
)

var rootCmd = &cobra.Command{
	Use:   "rpg-client",
	Short: "RPG API client for testing services",
}

var encounterCmd = &cobra.Command{
	Use:   "encounter",
	Short: "Test encounter service",
}

var dungeonStartCmd = &cobra.Command{
	Use:   "start [character_ids...]",
	Short: "Start a dungeon encounter",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
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
		client := dnd5ev1alpha1.NewEncounterServiceClient(conn)

		// Call DungeonStart
		req := &dnd5ev1alpha1.DungeonStartRequest{
			CharacterIds: args,
		}

		resp, err := client.DungeonStart(ctx, req)
		if err != nil {
			return fmt.Errorf("failed to start dungeon: %w", err)
		}

		// Pretty print the response
		output, err := json.MarshalIndent(resp, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal response: %w", err)
		}

		fmt.Println(string(output))
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&serverAddr, "server", "localhost:50051", "gRPC server address")
	rootCmd.PersistentFlags().DurationVar(&timeout, "timeout", 10*time.Second, "request timeout")

	encounterCmd.AddCommand(dungeonStartCmd)
	rootCmd.AddCommand(encounterCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
