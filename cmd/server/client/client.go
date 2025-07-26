// Package client provides test commands for the RPG API gRPC service
package client

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
)

var (
	// Connection flags
	serverAddr string
	timeout    time.Duration
)

// ClientCmd is the root command for all client test commands
var ClientCmd = &cobra.Command{
	Use:   "client",
	Short: "Test client commands for the RPG API",
	Long:  `Client commands allow you to test the RPG API by making real gRPC requests.`,
}

func init() {
	// Add persistent flags for all client commands
	ClientCmd.PersistentFlags().StringVar(&serverAddr, "server", "localhost:50051", "gRPC server address")
	ClientCmd.PersistentFlags().DurationVar(&timeout, "timeout", 30*time.Second, "Request timeout")

	// Add subcommands
	ClientCmd.AddCommand(listRacesCmd)
	ClientCmd.AddCommand(listClassesCmd)
	ClientCmd.AddCommand(listBackgroundsCmd)
	ClientCmd.AddCommand(getRaceCmd)
	ClientCmd.AddCommand(getClassCmd)

	// Equipment and spell commands
	ClientCmd.AddCommand(listEquipmentCmd)
	ClientCmd.AddCommand(listSpellsCmd)

	// Draft commands
	ClientCmd.AddCommand(createDraftCmd)
	ClientCmd.AddCommand(getDraftCmd)
	ClientCmd.AddCommand(updateNameCmd)
	ClientCmd.AddCommand(updateRaceCmd)
}

// createConnection creates a gRPC connection to the server
func createConnection() (*grpc.ClientConn, error) {
	conn, err := grpc.NewClient(serverAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to server: %w", err)
	}

	return conn, nil
}

// createCharacterClient creates a character service client
func createCharacterClient() (dnd5ev1alpha1.CharacterServiceClient, func(), error) {
	conn, err := createConnection()
	if err != nil {
		return nil, nil, err
	}

	cleanup := func() {
		_ = conn.Close() // nolint:errcheck // safe to ignore in cleanup
	}

	client := dnd5ev1alpha1.NewCharacterServiceClient(conn)
	return client, cleanup, nil
}
