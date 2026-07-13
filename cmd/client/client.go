// Package main provides a command-line client for testing RPG API services
package main

import (
	"log"
	"time"

	"github.com/spf13/cobra"
)

var (
	serverAddr string
	timeout    time.Duration
)

var rootCmd = &cobra.Command{
	Use:   "rpg-client",
	Short: "RPG API client for testing services",
}

func init() {
	rootCmd.PersistentFlags().StringVar(&serverAddr, "server", "localhost:50051", "gRPC server address")
	rootCmd.PersistentFlags().DurationVar(&timeout, "timeout", 10*time.Second, "request timeout")

	rootCmd.AddCommand(characterCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
