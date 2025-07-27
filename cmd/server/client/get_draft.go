package client

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
)

var (
	draftID string
)

var getDraftCmd = &cobra.Command{
	Use:   "get-draft",
	Short: "Get a character draft by ID",
	Long:  `Retrieve a character draft to view its current progress and details.`,
	RunE:  runGetDraft,
}

func init() {
	getDraftCmd.Flags().StringVar(&draftID, "draft-id", "", "Draft ID (required)")
	_ = getDraftCmd.MarkFlagRequired("draft-id") // nolint:errcheck // safe to ignore in init
}

func runGetDraft(_ *cobra.Command, _ []string) error {
	client, cleanup, err := createCharacterClient()
	if err != nil {
		return err
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req := &dnd5ev1alpha1.GetDraftRequest{
		DraftId: draftID,
	}

	resp, err := client.GetDraft(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to get draft: %w", err)
	}

	draft := resp.Draft
	fmt.Printf("📋 Character Draft Details\n\n")
	fmt.Printf("Draft ID: %s\n", draft.Id)
	fmt.Printf("Player ID: %s\n", draft.PlayerId)
	if draft.SessionId != "" {
		fmt.Printf("Session ID: %s\n", draft.SessionId)
	}
	if draft.Name != "" {
		fmt.Printf("Name: %s\n", draft.Name)
	}
	if draft.Race != nil {
		fmt.Printf("Race: %s\n", draft.Race.Name)
		if draft.Subrace != nil {
			fmt.Printf("Subrace: %s\n", draft.Subrace.Name)
		}
	}
	if draft.Class != nil {
		fmt.Printf("Class: %s\n", draft.Class.Name)
	}
	if draft.Background != nil {
		fmt.Printf("Background: %s\n", draft.Background.Name)
	}
	if draft.Alignment != dnd5ev1alpha1.Alignment_ALIGNMENT_UNSPECIFIED {
		fmt.Printf("Alignment: %s\n", draft.Alignment)
	}

	if draft.AbilityScores != nil {
		fmt.Printf("\nAbility Scores:\n")
		fmt.Printf("  - Strength: %d\n", draft.AbilityScores.Strength)
		fmt.Printf("  - Dexterity: %d\n", draft.AbilityScores.Dexterity)
		fmt.Printf("  - Constitution: %d\n", draft.AbilityScores.Constitution)
		fmt.Printf("  - Intelligence: %d\n", draft.AbilityScores.Intelligence)
		fmt.Printf("  - Wisdom: %d\n", draft.AbilityScores.Wisdom)
		fmt.Printf("  - Charisma: %d\n", draft.AbilityScores.Charisma)
	}

	if len(draft.Choices) > 0 {
		fmt.Printf("\nChoices:\n")
		for _, choice := range draft.Choices {
			fmt.Printf("  - %s (%s from %s): %v\n",
				choice.ChoiceId,
				choice.ChoiceType,
				choice.Source,
				choice.SelectedKeys)
		}
	}

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

	fmt.Printf("\nMetadata:\n")
	fmt.Printf("  - Created: %d\n", draft.Metadata.CreatedAt)
	fmt.Printf("  - Updated: %d\n", draft.Metadata.UpdatedAt)
	fmt.Printf("  - Expires: %d\n", draft.ExpiresAt)

	return nil
}
