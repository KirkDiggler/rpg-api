package client

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/clients/dnd5e/api/v1alpha1"
)

var (
	updateNameDraftID string
	characterName     string
)

var updateNameCmd = &cobra.Command{
	Use:   "update-name",
	Short: "Update character name in a draft",
	Long:  `Update the character's name in an existing draft.`,
	RunE:  runUpdateName,
}

func init() {
	updateNameCmd.Flags().StringVar(&updateNameDraftID, "draft-id", "", "Draft ID (required)")
	updateNameCmd.Flags().StringVar(&characterName, "name", "", "Character name (required)")
	_ = updateNameCmd.MarkFlagRequired("draft-id") // nolint:errcheck // safe to ignore in init
	_ = updateNameCmd.MarkFlagRequired("name")     // nolint:errcheck // safe to ignore in init
}

func runUpdateName(_ *cobra.Command, _ []string) error {
	client, cleanup, err := createCharacterClient()
	if err != nil {
		return err
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req := &dnd5ev1alpha1.UpdateNameRequest{
		DraftId: updateNameDraftID,
		Name:    characterName,
	}

	resp, err := client.UpdateName(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to update name: %w", err)
	}

	draft := resp.Draft
	fmt.Printf("✅ Character name updated successfully!\n\n")
	fmt.Printf("Draft ID: %s\n", draft.Id)
	fmt.Printf("Name: %s\n", draft.Name)
	fmt.Printf("Completion: %d%%\n", draft.Progress.CompletionPercentage)

	if len(resp.Warnings) > 0 {
		fmt.Printf("\n⚠️  Warnings:\n")
		for _, warning := range resp.Warnings {
			fmt.Printf("  - %s: %s\n", warning.Field, warning.Message)
		}
	}

	return nil
}
