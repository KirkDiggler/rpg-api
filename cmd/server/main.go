// Package main is the entry point for the gRPC server
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/KirkDiggler/rpg-api/cmd/server/client"
)

var rootCmd = &cobra.Command{
	Use:   "rpg-api",
	Short: "RPG API gRPC Server",
	Long:  `RPG API provides a gRPC interface for managing D&D 5e characters, sessions, and encounters.`,
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(client.ClientCmd)
}
