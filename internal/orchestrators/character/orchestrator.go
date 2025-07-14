// Package character implements the character orchestrator
package character

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/KirkDiggler/rpg-api/internal/clients/external"
	"github.com/KirkDiggler/rpg-api/internal/engine"
	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
	"github.com/KirkDiggler/rpg-api/internal/errors"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	draftrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
	"github.com/KirkDiggler/rpg-api/internal/services/character"
)

// Config holds the dependencies for the character orchestrator
type Config struct {
	CharacterRepo      characterrepo.Repository
	CharacterDraftRepo draftrepo.Repository
	Engine             engine.Engine
	ExternalClient     external.Client
}

// Validate ensures all required dependencies are provided
func (c *Config) Validate() error {
	vb := errors.NewValidationBuilder()

	if c.CharacterRepo == nil {
		vb.RequiredField("CharacterRepo")
	}
	if c.CharacterDraftRepo == nil {
		vb.RequiredField("CharacterDraftRepo")
	}
	if c.Engine == nil {
		vb.RequiredField("Engine")
	}
	if c.ExternalClient == nil {
		vb.RequiredField("ExternalClient")
	}

	return vb.Build()
}

// Orchestrator implements the character.Service interface
type Orchestrator struct {
	characterRepo      characterrepo.Repository
	characterDraftRepo draftrepo.Repository
	engine             engine.Engine
	externalClient     external.Client
}

// New creates a new character orchestrator
func New(cfg *Config) (*Orchestrator, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid config")
	}

	return &Orchestrator{
		characterRepo:      cfg.CharacterRepo,
		characterDraftRepo: cfg.CharacterDraftRepo,
		engine:             cfg.Engine,
		externalClient:     cfg.ExternalClient,
	}, nil
}

// Ensure Orchestrator implements the Service interface
var _ character.Service = (*Orchestrator)(nil)

// Draft lifecycle methods

// CreateDraft creates a new character draft
func (o *Orchestrator) CreateDraft(
	ctx context.Context,
	input *character.CreateDraftInput,
) (*character.CreateDraftOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}

	vb := errors.NewValidationBuilder()
	errors.ValidateRequired("playerID", input.PlayerID, vb)
	if err := vb.Build(); err != nil {
		return nil, err
	}

	// Create new draft with basic information
	draft := &dnd5e.CharacterDraft{
		PlayerID:  input.PlayerID,
		SessionID: input.SessionID,
		Progress: dnd5e.CreationProgress{
			StepsCompleted:       0, // No steps completed initially
			CompletionPercentage: 0,
			CurrentStep:          dnd5e.CreationStepName,
		},
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
	}

	// Apply initial data if provided
	if input.InitialData != nil {
		if input.InitialData.Name != "" {
			draft.Name = input.InitialData.Name
			draft.Progress.SetStep(dnd5e.ProgressStepName, true)
		}
		if input.InitialData.RaceID != "" {
			draft.RaceID = input.InitialData.RaceID
			draft.SubraceID = input.InitialData.SubraceID
			draft.Progress.SetStep(dnd5e.ProgressStepRace, true)
		}
		if input.InitialData.ClassID != "" {
			draft.ClassID = input.InitialData.ClassID
			draft.Progress.SetStep(dnd5e.ProgressStepClass, true)
		}
		if input.InitialData.BackgroundID != "" {
			draft.BackgroundID = input.InitialData.BackgroundID
			draft.Progress.SetStep(dnd5e.ProgressStepBackground, true)
		}
		if input.InitialData.AbilityScores != nil {
			draft.AbilityScores = input.InitialData.AbilityScores
			draft.Progress.SetStep(dnd5e.ProgressStepAbilityScores, true)
		}
		if len(input.InitialData.StartingSkillIDs) > 0 {
			draft.StartingSkillIDs = input.InitialData.StartingSkillIDs
			draft.Progress.SetStep(dnd5e.ProgressStepSkills, true)
		}
		if len(input.InitialData.AdditionalLanguages) > 0 {
			draft.AdditionalLanguages = input.InitialData.AdditionalLanguages
			draft.Progress.SetStep(dnd5e.ProgressStepLanguages, true)
		}
		draft.Alignment = input.InitialData.Alignment
		draft.DiscordChannelID = input.InitialData.DiscordChannelID
		draft.DiscordMessageID = input.InitialData.DiscordMessageID

		// Update completion percentage
		o.updateCompletionPercentage(draft)
	}

	// Generate a unique ID
	draft.ID = fmt.Sprintf("%s_%d", input.PlayerID, time.Now().UnixNano())

	// Set expiration (24 hours from now)
	draft.ExpiresAt = time.Now().Add(24 * time.Hour).Unix()

	// Validate the draft with the engine
	validateInput := &engine.ValidateCharacterDraftInput{
		Draft: draft,
	}
	validateOutput, err := o.engine.ValidateCharacterDraft(ctx, validateInput)
	if err != nil {
		return nil, errors.Wrap(err, "failed to validate draft")
	}
	if !validateOutput.IsValid {
		ve := errors.NewValidationError()
		for _, e := range validateOutput.Errors {
			ve.AddFieldError(e.Field, e.Message)
		}
		return nil, ve.ToError()
	}

	// Create the draft in the repository
	_, err = o.characterDraftRepo.Create(ctx, draftrepo.CreateInput{Draft: draft})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create draft")
	}

	return &character.CreateDraftOutput{
		Draft: draft,
	}, nil
}

// GetDraft retrieves a character draft by ID
func (o *Orchestrator) GetDraft(
	ctx context.Context,
	input *character.GetDraftInput,
) (*character.GetDraftOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}

	vb := errors.NewValidationBuilder()
	errors.ValidateRequired("draftID", input.DraftID, vb)
	if err := vb.Build(); err != nil {
		return nil, err
	}

	getOutput, err := o.characterDraftRepo.Get(ctx, draftrepo.GetInput{ID: input.DraftID})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get draft").
			WithMeta("draft_id", input.DraftID)
	}

	return &character.GetDraftOutput{
		Draft: getOutput.Draft,
	}, nil
}

// ListDrafts lists character drafts with optional filters
func (o *Orchestrator) ListDrafts(
	ctx context.Context,
	input *character.ListDraftsInput,
) (*character.ListDraftsOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}

	// With single-draft-per-player pattern, we only support listing by player
	if input.PlayerID == "" {
		return nil, errors.InvalidArgument("PlayerID is required")
	}

	// Get the single draft for this player
	draftOutput, err := o.characterDraftRepo.GetByPlayerID(ctx, draftrepo.GetByPlayerIDInput{
		PlayerID: input.PlayerID,
	})
	if err != nil {
		if errors.IsNotFound(err) {
			// No draft found - return empty list
			return &character.ListDraftsOutput{
				Drafts:        []*dnd5e.CharacterDraft{},
				NextPageToken: "",
			}, nil
		}
		return nil, errors.Wrap(err, "failed to get player draft")
	}

	// Return single draft as a list
	return &character.ListDraftsOutput{
		Drafts:        []*dnd5e.CharacterDraft{draftOutput.Draft},
		NextPageToken: "", // No pagination needed for single draft
	}, nil
}

// DeleteDraft deletes a character draft
func (o *Orchestrator) DeleteDraft(
	ctx context.Context,
	input *character.DeleteDraftInput,
) (*character.DeleteDraftOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}

	vb := errors.NewValidationBuilder()
	errors.ValidateRequired("draftID", input.DraftID, vb)
	if err := vb.Build(); err != nil {
		return nil, err
	}

	_, err := o.characterDraftRepo.Delete(ctx, draftrepo.DeleteInput{ID: input.DraftID})
	if err != nil {
		return nil, errors.Wrap(err, "failed to delete draft")
	}

	return &character.DeleteDraftOutput{
		Message: fmt.Sprintf("Draft %s deleted successfully", input.DraftID),
	}, nil
}

// Section-based update methods

// UpdateName updates the character's name
func (o *Orchestrator) UpdateName(
	ctx context.Context,
	input *character.UpdateNameInput,
) (*character.UpdateNameOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}

	vb := errors.NewValidationBuilder()
	errors.ValidateRequired("draftID", input.DraftID, vb)
	errors.ValidateRequired("name", input.Name, vb)
	if err := vb.Build(); err != nil {
		return nil, err
	}

	// Get the draft
	getOutput, err := o.characterDraftRepo.Get(ctx, draftrepo.GetInput{ID: input.DraftID})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get draft")
	}
	draft := getOutput.Draft

	// Update the name
	draft.Name = input.Name
	draft.UpdatedAt = time.Now().Unix()

	// Update progress
	draft.Progress.SetStep(dnd5e.ProgressStepName, true)
	o.updateCompletionPercentage(draft)

	// Save the updated draft
	_, err = o.characterDraftRepo.Update(ctx, draftrepo.UpdateInput{Draft: draft})
	if err != nil {
		return nil, errors.Wrap(err, "failed to update draft")
	}

	return &character.UpdateNameOutput{
		Draft: draft,
	}, nil
}

// UpdateRace updates the character's race
func (o *Orchestrator) UpdateRace(
	ctx context.Context,
	input *character.UpdateRaceInput,
) (*character.UpdateRaceOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}

	vb := errors.NewValidationBuilder()
	errors.ValidateRequired("draftID", input.DraftID, vb)
	errors.ValidateRequired("raceID", input.RaceID, vb)
	if err := vb.Build(); err != nil {
		return nil, err
	}

	// Get the draft
	getOutput, err := o.characterDraftRepo.Get(ctx, draftrepo.GetInput{ID: input.DraftID})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get draft")
	}
	draft := getOutput.Draft

	// Validate race choice with engine
	validateInput := &engine.ValidateRaceChoiceInput{
		RaceID:    input.RaceID,
		SubraceID: input.SubraceID,
	}
	validateOutput, err := o.engine.ValidateRaceChoice(ctx, validateInput)
	if err != nil {
		return nil, errors.Wrap(err, "failed to validate race choice")
	}

	// Convert validation errors to warnings
	var warnings []character.ValidationWarning
	if !validateOutput.IsValid {
		warnings = convertValidationErrorsToWarnings(validateOutput.Errors)
	}

	// Update the race
	draft.RaceID = input.RaceID
	draft.SubraceID = input.SubraceID
	draft.UpdatedAt = time.Now().Unix()

	// Reset dependent fields when race changes
	draft.AbilityScores = nil
	draft.StartingSkillIDs = nil
	draft.AdditionalLanguages = nil

	// Update progress
	draft.Progress.SetStep(dnd5e.ProgressStepRace, true)
	// Reset dependent steps
	draft.Progress.SetStep(dnd5e.ProgressStepAbilityScores, false)
	draft.Progress.SetStep(dnd5e.ProgressStepSkills, false)
	draft.Progress.SetStep(dnd5e.ProgressStepLanguages, false)
	o.updateCompletionPercentage(draft)

	// Save the updated draft
	_, err = o.characterDraftRepo.Update(ctx, draftrepo.UpdateInput{Draft: draft})
	if err != nil {
		return nil, errors.Wrap(err, "failed to update draft")
	}

	return &character.UpdateRaceOutput{
		Draft:    draft,
		Warnings: warnings,
	}, nil
}

// UpdateClass updates the character's class
func (o *Orchestrator) UpdateClass(
	ctx context.Context,
	input *character.UpdateClassInput,
) (*character.UpdateClassOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}

	vb := errors.NewValidationBuilder()
	errors.ValidateRequired("draftID", input.DraftID, vb)
	errors.ValidateRequired("classID", input.ClassID, vb)
	if err := vb.Build(); err != nil {
		return nil, err
	}

	// Get the draft
	getOutput, err := o.characterDraftRepo.Get(ctx, draftrepo.GetInput{ID: input.DraftID})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get draft")
	}
	draft := getOutput.Draft

	// Validate class choice with engine
	validateInput := &engine.ValidateClassChoiceInput{
		ClassID:       input.ClassID,
		AbilityScores: draft.AbilityScores,
	}
	validateOutput, err := o.engine.ValidateClassChoice(ctx, validateInput)
	if err != nil {
		return nil, errors.Wrap(err, "failed to validate class choice")
	}

	// Convert validation errors to warnings
	var warnings []character.ValidationWarning
	if !validateOutput.IsValid {
		warnings = convertValidationErrorsToWarnings(validateOutput.Errors)
	}
	warnings = append(warnings, convertValidationWarnings(validateOutput.Warnings)...)

	// Update the class
	draft.ClassID = input.ClassID
	draft.UpdatedAt = time.Now().Unix()

	// Reset dependent fields when class changes
	draft.StartingSkillIDs = nil

	// Update progress
	draft.Progress.SetStep(dnd5e.ProgressStepClass, true)
	// Reset dependent steps
	draft.Progress.SetStep(dnd5e.ProgressStepSkills, false)
	o.updateCompletionPercentage(draft)

	// Save the updated draft
	_, err = o.characterDraftRepo.Update(ctx, draftrepo.UpdateInput{Draft: draft})
	if err != nil {
		return nil, errors.Wrap(err, "failed to update draft")
	}

	return &character.UpdateClassOutput{
		Draft:    draft,
		Warnings: warnings,
	}, nil
}

// UpdateBackground updates the character's background
func (o *Orchestrator) UpdateBackground(
	ctx context.Context,
	input *character.UpdateBackgroundInput,
) (*character.UpdateBackgroundOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}

	vb := errors.NewValidationBuilder()
	errors.ValidateRequired("draftID", input.DraftID, vb)
	errors.ValidateRequired("backgroundID", input.BackgroundID, vb)
	if err := vb.Build(); err != nil {
		return nil, err
	}

	// Get the draft
	getOutput, err := o.characterDraftRepo.Get(ctx, draftrepo.GetInput{ID: input.DraftID})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get draft")
	}
	draft := getOutput.Draft

	// Validate background choice with engine
	validateInput := &engine.ValidateBackgroundChoiceInput{
		BackgroundID: input.BackgroundID,
	}
	validateOutput, err := o.engine.ValidateBackgroundChoice(ctx, validateInput)
	if err != nil {
		return nil, errors.Wrap(err, "failed to validate background choice")
	}
	if !validateOutput.IsValid {
		ve := errors.NewValidationError()
		ve.AddFieldError("background", "invalid background choice")
		return nil, ve.ToError()
	}

	// Update the background
	draft.BackgroundID = input.BackgroundID
	draft.UpdatedAt = time.Now().Unix()

	// Reset dependent fields when background changes
	draft.StartingSkillIDs = nil

	// Update progress
	draft.Progress.SetStep(dnd5e.ProgressStepBackground, true)
	// Reset dependent steps
	draft.Progress.SetStep(dnd5e.ProgressStepSkills, false)
	o.updateCompletionPercentage(draft)

	// Save the updated draft
	_, err = o.characterDraftRepo.Update(ctx, draftrepo.UpdateInput{Draft: draft})
	if err != nil {
		return nil, errors.Wrap(err, "failed to update draft")
	}

	return &character.UpdateBackgroundOutput{
		Draft: draft,
	}, nil
}

// UpdateAbilityScores updates the character's ability scores
func (o *Orchestrator) UpdateAbilityScores(
	ctx context.Context,
	input *character.UpdateAbilityScoresInput,
) (*character.UpdateAbilityScoresOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}

	vb := errors.NewValidationBuilder()
	errors.ValidateRequired("draftID", input.DraftID, vb)
	if err := vb.Build(); err != nil {
		return nil, err
	}

	// Get the draft
	getOutput, err := o.characterDraftRepo.Get(ctx, draftrepo.GetInput{ID: input.DraftID})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get draft")
	}
	draft := getOutput.Draft

	// Validate ability scores with engine
	validateInput := &engine.ValidateAbilityScoresInput{
		AbilityScores: &input.AbilityScores,
		Method:        "standard_array", // TODO: Make this configurable
	}
	validateOutput, err := o.engine.ValidateAbilityScores(ctx, validateInput)
	if err != nil {
		return nil, errors.Wrap(err, "failed to validate ability scores")
	}
	if !validateOutput.IsValid {
		ve := errors.NewValidationError()
		ve.AddFieldError("abilityScores", "invalid ability scores")
		return nil, ve.ToError()
	}

	// Collect warnings
	var warnings []character.ValidationWarning

	// Validate class requirements if class is selected
	if draft.ClassID != "" {
		classValidateInput := &engine.ValidateClassChoiceInput{
			ClassID:       draft.ClassID,
			AbilityScores: &input.AbilityScores,
		}
		classValidateOutput, err := o.engine.ValidateClassChoice(ctx, classValidateInput)
		if err != nil {
			return nil, errors.Wrap(err, "failed to validate class requirements")
		}
		if !classValidateOutput.IsValid {
			// Convert class requirement errors to warnings
			for _, e := range classValidateOutput.Errors {
				warnings = append(warnings, character.ValidationWarning{
					Field:   "class_requirements",
					Message: e.Message,
					Type:    e.Code,
				})
			}
		}
	}

	// Update the ability scores
	draft.AbilityScores = &input.AbilityScores
	draft.UpdatedAt = time.Now().Unix()

	// Update progress
	draft.Progress.SetStep(dnd5e.ProgressStepAbilityScores, true)
	o.updateCompletionPercentage(draft)

	// Save the updated draft
	_, err = o.characterDraftRepo.Update(ctx, draftrepo.UpdateInput{Draft: draft})
	if err != nil {
		return nil, errors.Wrap(err, "failed to update draft")
	}

	return &character.UpdateAbilityScoresOutput{
		Draft:    draft,
		Warnings: warnings,
	}, nil
}

// UpdateSkills updates the character's starting skills
func (o *Orchestrator) UpdateSkills(
	ctx context.Context,
	input *character.UpdateSkillsInput,
) (*character.UpdateSkillsOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}

	vb := errors.NewValidationBuilder()
	errors.ValidateRequired("draftID", input.DraftID, vb)
	if err := vb.Build(); err != nil {
		return nil, err
	}

	// Get the draft
	getOutput, err := o.characterDraftRepo.Get(ctx, draftrepo.GetInput{ID: input.DraftID})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get draft")
	}
	draft := getOutput.Draft

	// Collect warnings
	var warnings []character.ValidationWarning

	// Check prerequisites
	if draft.ClassID == "" || draft.BackgroundID == "" {
		warnings = append(warnings, character.ValidationWarning{
			Field:   "prerequisites",
			Message: "Class and background must be selected before choosing skills",
			Type:    "MISSING_PREREQUISITES",
		})
		// Still allow updating skills, but with warning
	} else {
		// Validate skill choices with engine
		validateInput := &engine.ValidateSkillChoicesInput{
			ClassID:          draft.ClassID,
			BackgroundID:     draft.BackgroundID,
			SelectedSkillIDs: input.SkillIDs,
		}
		validateOutput, err := o.engine.ValidateSkillChoices(ctx, validateInput)
		if err != nil {
			return nil, errors.Wrap(err, "failed to validate skill choices")
		}
		if !validateOutput.IsValid {
			// Convert validation errors to warnings
			for _, e := range validateOutput.Errors {
				warnings = append(warnings, character.ValidationWarning{
					Field:   e.Field,
					Message: e.Message,
					Type:    e.Code,
				})
			}
		}
	}

	// Update the skills
	draft.StartingSkillIDs = input.SkillIDs
	draft.UpdatedAt = time.Now().Unix()

	// Update progress
	draft.Progress.SetStep(dnd5e.ProgressStepSkills, true)
	o.updateCompletionPercentage(draft)

	// Save the updated draft
	_, err = o.characterDraftRepo.Update(ctx, draftrepo.UpdateInput{Draft: draft})
	if err != nil {
		return nil, errors.Wrap(err, "failed to update draft")
	}

	return &character.UpdateSkillsOutput{
		Draft:    draft,
		Warnings: warnings,
	}, nil
}

// Validation methods

// ValidateDraft validates a character draft
func (o *Orchestrator) ValidateDraft(
	ctx context.Context,
	input *character.ValidateDraftInput,
) (*character.ValidateDraftOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}

	vb := errors.NewValidationBuilder()
	errors.ValidateRequired("draftID", input.DraftID, vb)
	if err := vb.Build(); err != nil {
		return nil, err
	}

	// Get the draft
	getOutput, err := o.characterDraftRepo.Get(ctx, draftrepo.GetInput{ID: input.DraftID})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get draft")
	}
	draft := getOutput.Draft

	// Validate with engine
	validateInput := &engine.ValidateCharacterDraftInput{
		Draft: draft,
	}
	validateOutput, err := o.engine.ValidateCharacterDraft(ctx, validateInput)
	if err != nil {
		return nil, errors.Wrap(err, "failed to validate draft")
	}

	return &character.ValidateDraftOutput{
		IsComplete:   validateOutput.IsComplete,
		IsValid:      validateOutput.IsValid,
		Errors:       convertValidationErrors(validateOutput.Errors),
		Warnings:     convertValidationWarnings(validateOutput.Warnings),
		MissingSteps: validateOutput.MissingSteps,
	}, nil
}

// Character finalization

// FinalizeDraft finalizes a character draft into a full character
func (o *Orchestrator) FinalizeDraft(
	ctx context.Context,
	input *character.FinalizeDraftInput,
) (*character.FinalizeDraftOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}

	vb := errors.NewValidationBuilder()
	errors.ValidateRequired("draftID", input.DraftID, vb)
	if err := vb.Build(); err != nil {
		return nil, err
	}

	// Get the draft
	getOutput, err := o.characterDraftRepo.Get(ctx, draftrepo.GetInput{ID: input.DraftID})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get draft")
	}
	draft := getOutput.Draft

	// Validate the draft is complete
	validateInput := &engine.ValidateCharacterDraftInput{
		Draft: draft,
	}
	validateOutput, err := o.engine.ValidateCharacterDraft(ctx, validateInput)
	if err != nil {
		return nil, errors.Wrap(err, "failed to validate draft")
	}
	if !validateOutput.IsComplete {
		return nil, errors.InvalidArgument("cannot finalize incomplete draft: missing steps: " +
			strings.Join(validateOutput.MissingSteps, ", "))
	}
	if !validateOutput.IsValid {
		ve := errors.NewValidationError()
		for _, e := range validateOutput.Errors {
			ve.AddFieldError(e.Field, e.Message)
		}
		return nil, errors.Wrap(ve.ToError(), "cannot finalize invalid draft")
	}

	// Calculate final character stats
	calculateInput := &engine.CalculateCharacterStatsInput{
		Draft: draft,
	}
	calculateOutput, err := o.engine.CalculateCharacterStats(ctx, calculateInput)
	if err != nil {
		return nil, errors.Wrap(err, "failed to calculate character stats")
	}

	// Create the final character
	char := &dnd5e.Character{
		ID:               fmt.Sprintf("char_%s_%d", draft.PlayerID, time.Now().UnixNano()),
		Name:             draft.Name,
		Level:            1, // Starting level
		ExperiencePoints: 0,
		RaceID:           draft.RaceID,
		SubraceID:        draft.SubraceID,
		ClassID:          draft.ClassID,
		BackgroundID:     draft.BackgroundID,
		Alignment:        draft.Alignment,
		AbilityScores:    *draft.AbilityScores,
		CurrentHP:        calculateOutput.MaxHP,
		TempHP:           0,
		SessionID:        draft.SessionID,
		PlayerID:         draft.PlayerID,
		CreatedAt:        time.Now().Unix(),
		UpdatedAt:        time.Now().Unix(),
	}

	// Create the character in the repository
	_, err = o.characterRepo.Create(ctx, characterrepo.CreateInput{Character: char})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create character")
	}

	// Delete the draft
	_, err = o.characterDraftRepo.Delete(ctx, draftrepo.DeleteInput{ID: draft.ID})
	if err != nil {
		slog.Error("failed to delete draft", "draft_id", draft.ID, "error", err)
		// Continue - the character was created successfully
	}

	return &character.FinalizeDraftOutput{
		Character:    char,
		DraftDeleted: err == nil,
	}, nil
}

// Completed character operations

// GetCharacter retrieves a character by ID
func (o *Orchestrator) GetCharacter(
	ctx context.Context,
	input *character.GetCharacterInput,
) (*character.GetCharacterOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}

	vb := errors.NewValidationBuilder()
	errors.ValidateRequired("characterID", input.CharacterID, vb)
	if err := vb.Build(); err != nil {
		return nil, err
	}

	getOutput, err := o.characterRepo.Get(ctx, characterrepo.GetInput{ID: input.CharacterID})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get character")
	}

	return &character.GetCharacterOutput{
		Character: getOutput.Character,
	}, nil
}

// ListCharacters lists characters with optional filters
func (o *Orchestrator) ListCharacters(
	ctx context.Context,
	input *character.ListCharactersInput,
) (*character.ListCharactersOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}

	// Default page size
	if input.PageSize == 0 {
		input.PageSize = 20
	}

	// Use specific list methods based on filters
	var characters []*dnd5e.Character
	switch {
	case input.PlayerID != "":
		listOutput, err := o.characterRepo.ListByPlayerID(ctx, characterrepo.ListByPlayerIDInput{PlayerID: input.PlayerID})
		if err != nil {
			return nil, errors.Wrap(err, "failed to list characters")
		}
		characters = listOutput.Characters
	case input.SessionID != "":
		listOutput, err := o.characterRepo.ListBySessionID(ctx,
			characterrepo.ListBySessionIDInput{SessionID: input.SessionID})
		if err != nil {
			return nil, errors.Wrap(err, "failed to list characters")
		}
		characters = listOutput.Characters
	default:
		return nil, errors.InvalidArgument("either PlayerID or SessionID must be provided")
	}

	return &character.ListCharactersOutput{
		Characters:    characters,
		NextPageToken: "", // TODO: implement pagination if needed
	}, nil
}

// DeleteCharacter deletes a character
func (o *Orchestrator) DeleteCharacter(
	ctx context.Context,
	input *character.DeleteCharacterInput,
) (*character.DeleteCharacterOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}

	vb := errors.NewValidationBuilder()
	errors.ValidateRequired("characterID", input.CharacterID, vb)
	if err := vb.Build(); err != nil {
		return nil, err
	}

	_, err := o.characterRepo.Delete(ctx, characterrepo.DeleteInput{ID: input.CharacterID})
	if err != nil {
		return nil, errors.Wrap(err, "failed to delete character")
	}

	return &character.DeleteCharacterOutput{
		Message: fmt.Sprintf("Character %s deleted successfully", input.CharacterID),
	}, nil
}

// Helper methods

// updateCompletionPercentage updates the completion percentage based on completed steps
func (o *Orchestrator) updateCompletionPercentage(draft *dnd5e.CharacterDraft) {
	totalSteps := 7 // Name, Race, Class, Background, AbilityScores, Skills, Languages
	completedSteps := 0

	if draft.Progress.HasName() {
		completedSteps++
	}
	if draft.Progress.HasRace() {
		completedSteps++
	}
	if draft.Progress.HasClass() {
		completedSteps++
	}
	if draft.Progress.HasBackground() {
		completedSteps++
	}
	if draft.Progress.HasAbilityScores() {
		completedSteps++
	}
	if draft.Progress.HasSkills() {
		completedSteps++
	}
	if draft.Progress.HasLanguages() {
		completedSteps++
	}

	// Safe conversion - totalSteps is always 7 and completedSteps is 0-7
	// so max value is 700/7 = 100, which fits safely in int32
	draft.Progress.CompletionPercentage = int32((completedSteps * 100) / totalSteps) //nolint:gosec
}

// convertValidationErrors converts engine ValidationError to service ValidationError
func convertValidationErrors(errors []engine.ValidationError) []character.ValidationError {
	result := make([]character.ValidationError, len(errors))
	for i, e := range errors {
		result[i] = character.ValidationError{
			Field:   e.Field,
			Message: e.Message,
			Type:    e.Code,
		}
	}
	return result
}

// convertValidationWarnings converts engine ValidationWarning to service ValidationWarning
func convertValidationWarnings(warnings []engine.ValidationWarning) []character.ValidationWarning {
	result := make([]character.ValidationWarning, len(warnings))
	for i, w := range warnings {
		result[i] = character.ValidationWarning{
			Field:   w.Field,
			Message: w.Message,
			Type:    w.Code,
		}
	}
	return result
}

// convertValidationErrorsToWarnings converts engine ValidationError to service ValidationWarning
func convertValidationErrorsToWarnings(errors []engine.ValidationError) []character.ValidationWarning {
	result := make([]character.ValidationWarning, len(errors))
	for i, e := range errors {
		result[i] = character.ValidationWarning{
			Field:   e.Field,
			Message: e.Message,
			Type:    e.Code,
		}
	}
	return result
}
