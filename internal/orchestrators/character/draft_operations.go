package character

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/KirkDiggler/rpg-api/internal/errors"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/dice"
	draftrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character/choices"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

func (o *Orchestrator) CreateDraft(ctx context.Context, input *CreateDraftInput) (*CreateDraftOutput, error) {
	// Validate input
	if input.PlayerID == "" {
		return nil, errors.InvalidArgument("player ID is required")
	}

	// Create new draft with minimal data
	draft := &toolkitchar.DraftData{
		ID:       o.draftIDGen.Generate(),
		PlayerID: input.PlayerID,
	}

	// If initial data provided, merge it
	if input.InitialData != nil {
		if input.InitialData.Name != "" {
			draft.Name = input.InitialData.Name
		}
		// Add other fields as we implement them
	}

	// Save to repository
	createOutput, err := o.draftRepo.Create(ctx, draftrepo.CreateInput{
		Draft: draft,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create draft: %w", err)
	}

	return &CreateDraftOutput{
		Draft: createOutput.Draft,
	}, nil
}

func (o *Orchestrator) GetDraft(ctx context.Context, input *GetDraftInput) (*GetDraftOutput, error) {
	// Validate input
	if input.DraftID == "" {
		return nil, errors.InvalidArgument("draft ID is required")
	}

	// Get draft from repository
	getDraftOutput, err := o.draftRepo.Get(ctx, draftrepo.GetInput{
		ID: input.DraftID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get draft: %w", err)
	}

	// Validate the draft
	validation := o.validateDraft(ctx, getDraftOutput.Draft)

	// Return the draft data with validation
	return &GetDraftOutput{
		Draft:      getDraftOutput.Draft,
		Validation: validation,
	}, nil
}

func (o *Orchestrator) ListDrafts(ctx context.Context, input *ListDraftsInput) (*ListDraftsOutput, error) {
	// Validate input
	if input.PlayerID == "" {
		return nil, errors.InvalidArgument("player ID is required")
	}

	// Get the player's single draft
	getDraftOutput, err := o.draftRepo.GetByPlayerID(ctx, draftrepo.GetByPlayerIDInput{
		PlayerID: input.PlayerID,
	})
	if err != nil {
		if errors.IsNotFound(err) {
			// No draft found - return empty list
			return &ListDraftsOutput{
				Drafts:        []*toolkitchar.DraftData{},
				NextPageToken: "",
			}, nil
		}
		return nil, errors.Wrapf(err, "failed to get draft for player %s", input.PlayerID)
	}

	// Return the single draft as a list
	// Note: We ignore SessionID filter since we only have one draft per player
	return &ListDraftsOutput{
		Drafts:        []*toolkitchar.DraftData{getDraftOutput.Draft},
		NextPageToken: "", // No pagination needed for single draft
	}, nil
}

func (o *Orchestrator) DeleteDraft(ctx context.Context, input *DeleteDraftInput) (*DeleteDraftOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) UpdateBackground(ctx context.Context, input *UpdateBackgroundInput) (*UpdateBackgroundOutput, error) {
	// Validate input
	if input.DraftID == "" {
		return nil, errors.InvalidArgument("draft ID is required")
	}
	if input.BackgroundID == "" {
		return nil, errors.InvalidArgument("background ID is required")
	}

	// Get the existing draft
	getDraftOutput, err := o.draftRepo.Get(ctx, draftrepo.GetInput{
		ID: input.DraftID,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get draft %s", input.DraftID)
	}

	// Update the background choice
	draft := getDraftOutput.Draft
	draft.BackgroundChoice = input.BackgroundID

	// Always clear existing background choices when updating background
	var nonBackgroundChoices []toolkitchar.ChoiceData
	for _, choice := range draft.Choices {
		if choice.Source != shared.SourceBackground {
			nonBackgroundChoices = append(nonBackgroundChoices, choice)
		}
	}

	// Add new background choices if provided
	if len(input.Choices) > 0 {
		// Ensure all new choices have the background source set
		for i := range input.Choices {
			if input.Choices[i].Source == "" {
				input.Choices[i].Source = shared.SourceBackground
			}
		}
		draft.Choices = append(nonBackgroundChoices, input.Choices...)
	} else {
		// No choices provided, just keep non-background choices
		draft.Choices = nonBackgroundChoices
	}

	// Save the updated draft
	updateOutput, err := o.draftRepo.Update(ctx, draftrepo.UpdateInput{
		Draft: draft,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to update draft %s", input.DraftID)
	}

	// Return updated draft with any warnings
	return &UpdateBackgroundOutput{
		Draft:    updateOutput.Draft,
		Warnings: []ValidationWarning{}, // TODO: Add validation for background requirements
	}, nil
}

func (o *Orchestrator) UpdateAbilityScores(ctx context.Context, input *UpdateAbilityScoresInput) (*UpdateAbilityScoresOutput, error) {
	// Validate input
	if input.DraftID == "" {
		return nil, errors.InvalidArgument("draft ID is required")
	}

	// Must have either manual scores or roll assignments
	if input.AbilityScores == nil && input.RollAssignments == nil {
		return nil, errors.InvalidArgument("either ability scores or roll assignments must be provided")
	}

	// Get the existing draft
	getDraftOutput, err := o.draftRepo.Get(ctx, draftrepo.GetInput{
		ID: input.DraftID,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get draft %s", input.DraftID)
	}

	draft := getDraftOutput.Draft

	// Handle roll-based assignment
	if input.RollAssignments != nil {
		// Get the player ID from the draft
		playerID := draft.PlayerID

		slog.Info("Looking for dice session for ability score assignment",
			"draft_id", input.DraftID,
			"player_id", playerID,
			"context", "ability_scores")

		// Get the dice session for this player
		// The dice service uses "ability_scores" as the context for ability score rolls
		sessionOutput, err := o.diceService.GetRollSession(ctx, &dice.GetRollSessionInput{
			EntityID: playerID,
			Context:  "ability_scores",
		})
		if err != nil {
			slog.Error("Failed to get dice session",
				"draft_id", input.DraftID,
				"player_id", playerID,
				"context", "ability_scores",
				"error", err)
			return nil, errors.Wrapf(err, "failed to get dice session for player %s", playerID)
		}

		slog.Info("Found dice session",
			"draft_id", input.DraftID,
			"player_id", playerID,
			"rolls_count", len(sessionOutput.Session.Rolls))

		// Create a map of roll IDs to totals
		rollTotals := make(map[string]int32)
		for _, roll := range sessionOutput.Session.Rolls {
			rollTotals[roll.RollID] = roll.Total
		}

		// Validate all roll IDs exist and belong to this session
		rollIDs := []struct {
			ability string
			rollID  string
		}{
			{"strength", input.RollAssignments.StrengthRollID},
			{"dexterity", input.RollAssignments.DexterityRollID},
			{"constitution", input.RollAssignments.ConstitutionRollID},
			{"intelligence", input.RollAssignments.IntelligenceRollID},
			{"wisdom", input.RollAssignments.WisdomRollID},
			{"charisma", input.RollAssignments.CharismaRollID},
		}

		// Check all rolls exist
		for _, r := range rollIDs {
			if _, exists := rollTotals[r.rollID]; !exists {
				return nil, errors.InvalidArgumentf("roll ID %s for %s not found in session", r.rollID, r.ability)
			}
		}

		// Create ability scores from rolls
		abilityScores := shared.AbilityScores{
			abilities.STR: int(rollTotals[input.RollAssignments.StrengthRollID]),
			abilities.DEX: int(rollTotals[input.RollAssignments.DexterityRollID]),
			abilities.CON: int(rollTotals[input.RollAssignments.ConstitutionRollID]),
			abilities.INT: int(rollTotals[input.RollAssignments.IntelligenceRollID]),
			abilities.WIS: int(rollTotals[input.RollAssignments.WisdomRollID]),
			abilities.CHA: int(rollTotals[input.RollAssignments.CharismaRollID]),
		}

		// Update the draft with the ability scores
		draft.AbilityScoreChoice = abilityScores

		// Clear the dice session after using the rolls
		_, err = o.diceService.ClearRollSession(ctx, &dice.ClearRollSessionInput{
			EntityID: playerID,
			Context:  "ability_scores",
		})
		if err != nil {
			// Log warning but don't fail the operation
			slog.Warn("Failed to clear dice session after ability score assignment",
				"player_id", playerID,
				"context", "ability_scores",
				"error", err)
		}
	} else if input.AbilityScores != nil {
		// Manual assignment
		draft.AbilityScoreChoice = *input.AbilityScores
	}

	// Save the updated draft
	updateOutput, err := o.draftRepo.Update(ctx, draftrepo.UpdateInput{
		Draft: draft,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to update draft %s", input.DraftID)
	}

	// Return updated draft with any warnings
	return &UpdateAbilityScoresOutput{
		Draft:    updateOutput.Draft,
		Warnings: []ValidationWarning{}, // TODO: Add validation for ability score ranges
	}, nil
}

func (o *Orchestrator) UpdateSkills(ctx context.Context, input *UpdateSkillsInput) (*UpdateSkillsOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) UpdateName(ctx context.Context, input *UpdateNameInput) (*UpdateNameOutput, error) {
	// Validate input
	if input.DraftID == "" {
		return nil, errors.InvalidArgument("draft ID is required")
	}
	if strings.TrimSpace(input.Name) == "" {
		return nil, errors.InvalidArgument("name is required")
	}

	// Get the existing draft
	getDraftOutput, err := o.draftRepo.Get(ctx, draftrepo.GetInput{
		ID: input.DraftID,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get draft %s", input.DraftID)
	}

	// Update the name
	draft := getDraftOutput.Draft
	draft.Name = strings.TrimSpace(input.Name)

	// Save the updated draft
	updateOutput, err := o.draftRepo.Update(ctx, draftrepo.UpdateInput{
		Draft: draft,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to update draft %s", input.DraftID)
	}

	// Return updated draft with any warnings
	return &UpdateNameOutput{
		Draft:    updateOutput.Draft,
		Warnings: []ValidationWarning{}, // No warnings for name update
	}, nil
}

func (o *Orchestrator) UpdateRace(ctx context.Context, input *UpdateRaceInput) (*UpdateRaceOutput, error) {
	// Validate input
	if input.DraftID == "" {
		return nil, errors.InvalidArgument("draft ID is required")
	}
	if input.RaceID == "" {
		return nil, errors.InvalidArgument("race ID is required")
	}

	// Get the existing draft
	getDraftOutput, err := o.draftRepo.Get(ctx, draftrepo.GetInput{
		ID: input.DraftID,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get draft %s", input.DraftID)
	}

	// Update the race choice
	draft := getDraftOutput.Draft
	draft.RaceChoice = toolkitchar.RaceChoice{
		RaceID:    input.RaceID,
		SubraceID: input.SubraceID,
	}

	// Always clear existing race choices when updating race
	var nonRaceChoices []toolkitchar.ChoiceData
	for _, choice := range draft.Choices {
		if choice.Source != shared.SourceRace {
			nonRaceChoices = append(nonRaceChoices, choice)
		}
	}

	// Add new race choices if provided
	if len(input.Choices) > 0 {
		// Ensure all new choices have the race source set
		for i := range input.Choices {
			if input.Choices[i].Source == "" {
				input.Choices[i].Source = shared.SourceRace
			}
		}
		draft.Choices = append(nonRaceChoices, input.Choices...)
	} else {
		// No choices provided, just keep non-race choices
		draft.Choices = nonRaceChoices
	}

	// Save the updated draft
	updateOutput, err := o.draftRepo.Update(ctx, draftrepo.UpdateInput{
		Draft: draft,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to update draft %s", input.DraftID)
	}

	// Return updated draft with any warnings
	return &UpdateRaceOutput{
		Draft:    updateOutput.Draft,
		Warnings: []ValidationWarning{}, // TODO: Add validation for race/subrace compatibility
	}, nil
}

func (o *Orchestrator) UpdateClass(ctx context.Context, input *UpdateClassInput) (*UpdateClassOutput, error) {
	// Validate input
	if input.DraftID == "" {
		return nil, errors.InvalidArgument("draft ID is required")
	}
	if input.ClassID == "" {
		return nil, errors.InvalidArgument("class ID is required")
	}

	// Get the existing draft
	getDraftOutput, err := o.draftRepo.Get(ctx, draftrepo.GetInput{
		ID: input.DraftID,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get draft %s", input.DraftID)
	}

	// Update the class choice
	draft := getDraftOutput.Draft
	draft.ClassChoice = toolkitchar.ClassChoice{
		ClassID:    input.ClassID,
		SubclassID: input.SubclassID,
	}

	// Always clear existing class choices when updating class
	var nonClassChoices []toolkitchar.ChoiceData
	for _, choice := range draft.Choices {
		if choice.Source != shared.SourceClass {
			nonClassChoices = append(nonClassChoices, choice)
		}
	}

	// Get class requirements from the new choices system and create choice templates
	var classRequirements *choices.Requirements
	if input.SubclassID != "" {
		// If a subclass is selected, get the combined requirements (base + subclass)
		classRequirements = choices.GetSubclassRequirements(input.SubclassID)
	} else {
		// Otherwise just get the base class requirements
		classRequirements = choices.GetClassRequirements(input.ClassID)
	}
	classChoices := convertRequirementsToChoiceData(string(input.ClassID), classRequirements)

	// If user provided choices, merge them with the templates
	if len(input.Choices) > 0 {
		// Create a map to track which choices have been provided
		providedChoices := make(map[string]toolkitchar.ChoiceData)
		for _, choice := range input.Choices {
			if choice.Source == "" {
				choice.Source = shared.SourceClass
			}
			// Key by category + choice ID for uniqueness
			key := string(choice.Category) + ":" + choice.ChoiceID
			providedChoices[key] = choice
		}

		// Merge templates with user choices
		mergedClassChoices := []toolkitchar.ChoiceData{}
		for _, templateChoice := range classChoices {
			key := string(templateChoice.Category) + ":" + templateChoice.ChoiceID
			if userChoice, exists := providedChoices[key]; exists {
				// Use the user-provided choice (has actual selections)
				mergedClassChoices = append(mergedClassChoices, userChoice)
				delete(providedChoices, key)
			} else {
				// Use the template choice
				mergedClassChoices = append(mergedClassChoices, templateChoice)
			}
		}

		// Add any remaining user choices that weren't in templates
		for _, choice := range providedChoices {
			mergedClassChoices = append(mergedClassChoices, choice)
		}

		draft.Choices = append(nonClassChoices, mergedClassChoices...)
	} else {
		// No choices provided, just use templates
		draft.Choices = append(nonClassChoices, classChoices...)
	}

	// Save the updated draft
	updateOutput, err := o.draftRepo.Update(ctx, draftrepo.UpdateInput{
		Draft: draft,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to update draft %s", input.DraftID)
	}

	// Validate class requirements
	warnings, validationErr := o.validateClassRequirements(ctx, updateOutput.Draft)
	if validationErr != nil {
		// Still save the draft, but return validation errors as warnings
		warnings = append(warnings, ValidationWarning{
			Field:   "class_validation",
			Message: validationErr.Error(),
		})
	}

	// Return updated draft with any warnings
	return &UpdateClassOutput{
		Draft:    updateOutput.Draft,
		Warnings: warnings,
	}, nil
}

func (o *Orchestrator) RollAbilityScores(ctx context.Context, input *RollAbilityScoresInput) (*RollAbilityScoresOutput, error) {
	// Validate input
	if input.DraftID == "" {
		return nil, errors.InvalidArgument("draft ID is required")
	}

	// Default to standard method if not specified
	method := input.Method
	if method == "" {
		method = dice.MethodStandard
	}

	// Get the draft to ensure it exists and get player ID
	getDraftOutput, err := o.draftRepo.Get(ctx, draftrepo.GetInput{
		ID: input.DraftID,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get draft %s", input.DraftID)
	}

	// Use player ID as entity ID (this must match what UpdateAbilityScores expects)
	playerID := getDraftOutput.Draft.PlayerID

	slog.Info("Rolling ability scores",
		"draft_id", input.DraftID,
		"player_id", playerID,
		"method", method)

	// Roll ability scores using dice service
	rollOutput, err := o.diceService.RollAbilityScores(ctx, &dice.RollAbilityScoresInput{
		EntityID: playerID,
		Method:   method,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to roll ability scores")
	}

	slog.Info("Ability scores rolled successfully",
		"draft_id", input.DraftID,
		"player_id", playerID,
		"session_entity_id", rollOutput.Session.EntityID,
		"session_context", rollOutput.Session.Context,
		"rolls_count", len(rollOutput.Rolls))

	// Convert dice rolls to our format
	rolls := make([]*AbilityScoreRoll, 0, len(rollOutput.Rolls))
	for _, roll := range rollOutput.Rolls {
		rolls = append(rolls, &AbilityScoreRoll{
			RollID:      roll.RollID,
			Total:       roll.Total,
			Description: roll.Description,
			Dice:        roll.Dice,
			Dropped:     roll.Dropped,
		})
	}

	return &RollAbilityScoresOutput{
		Rolls:     rolls,
		SessionID: playerID, // The session is identified by playerID + context
		ExpiresAt: rollOutput.Session.ExpiresAt,
	}, nil
}

func (o *Orchestrator) GetDraftPreview(ctx context.Context, input *GetDraftPreviewInput) (*GetDraftPreviewOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}
