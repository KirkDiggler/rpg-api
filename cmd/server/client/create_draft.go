package client

import (
	"context"
	"fmt"
	"log"

	"github.com/spf13/cobra"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/clients/dnd5e/api/v1alpha1"
)

var (
	playerID  string
	sessionID string
)

var createDraftCmd = &cobra.Command{
	Use:   "create-draft",
	Short: "Create a new character draft",
	Long:  `Create a new character draft for starting the character creation process.`,
	RunE:  runCreateDraft,
}

func init() {
	createDraftCmd.Flags().StringVar(&playerID, "player-id", "", "Player ID (required)")
	createDraftCmd.Flags().StringVar(&sessionID, "session-id", "", "Session ID (optional)")
	_ = createDraftCmd.MarkFlagRequired("player-id") // nolint:errcheck // safe to ignore in init
}

func runCreateDraft(_ *cobra.Command, _ []string) error {
	client, cleanup, err := createCharacterClient()
	if err != nil {
		return err
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	log.Printf("Creating character draft for player %s...", playerID)

	req := &dnd5ev1alpha1.CreateDraftRequest{
		PlayerId:  playerID,
		SessionId: sessionID,
	}

	resp, err := client.CreateDraft(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to create draft: %w", err)
	}

	draft := resp.Draft
	fmt.Printf("✅ Character draft created successfully!\n\n")
	fmt.Printf("Draft ID: %s\n", draft.Id)
	fmt.Printf("Player ID: %s\n", draft.PlayerId)
	if draft.SessionId != "" {
		fmt.Printf("Session ID: %s\n", draft.SessionId)
	}
	fmt.Printf("Expires At: %d\n", draft.ExpiresAt)
	fmt.Printf("\nCreation Progress:\n")
	fmt.Printf("  - Name: %v\n", draft.Progress.HasName)
	fmt.Printf("  - Race: %v\n", draft.Progress.HasRace)
	fmt.Printf("  - Class: %v\n", draft.Progress.HasClass)
	fmt.Printf("  - Background: %v\n", draft.Progress.HasBackground)
	fmt.Printf("  - Ability Scores: %v\n", draft.Progress.HasAbilityScores)
	fmt.Printf("  - Skills: %v\n", draft.Progress.HasSkills)
	fmt.Printf("  - Languages: %v\n", draft.Progress.HasLanguages)
	fmt.Printf("  - Completion: %d%%\n", draft.Progress.CompletionPercentage)
	fmt.Printf("  - Current Step: %s\n", draft.Progress.CurrentStep)

	fmt.Printf("\n💡 Next steps:\n")
	fmt.Printf("1. Set character name: rpg-api client update-name --draft-id %s --name \"Your Character Name\"\n",
		draft.Id)
	fmt.Printf("2. Choose race: rpg-api client update-race --draft-id %s --race RACE_HUMAN\n", draft.Id)
	fmt.Printf("3. Choose class: rpg-api client update-class --draft-id %s --class CLASS_FIGHTER\n", draft.Id)
	fmt.Printf("4. Continue with background, abilities, skills...\n")
	fmt.Printf("5. Finalize: rpg-api client finalize-draft --draft-id %s\n", draft.Id)

	return nil
}
