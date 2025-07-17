// Package character implements the character orchestrator
package character

//go:generate mockgen -destination=mock/mock_service.go -package=charactermock github.com/KirkDiggler/rpg-api/internal/orchestrators/character Service

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"

	"github.com/KirkDiggler/rpg-api/internal/clients/external"
	"github.com/KirkDiggler/rpg-api/internal/engine"
	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
	"github.com/KirkDiggler/rpg-api/internal/errors"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/dice"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	draftrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
)

// Service defines the interface for character operations
type Service interface {
	// Draft lifecycle
	CreateDraft(ctx context.Context, input *CreateDraftInput) (*CreateDraftOutput, error)
	GetDraft(ctx context.Context, input *GetDraftInput) (*GetDraftOutput, error)
	ListDrafts(ctx context.Context, input *ListDraftsInput) (*ListDraftsOutput, error)
	DeleteDraft(ctx context.Context, input *DeleteDraftInput) (*DeleteDraftOutput, error)

	// Section-based updates
	UpdateName(ctx context.Context, input *UpdateNameInput) (*UpdateNameOutput, error)
	UpdateRace(ctx context.Context, input *UpdateRaceInput) (*UpdateRaceOutput, error)
	UpdateClass(ctx context.Context, input *UpdateClassInput) (*UpdateClassOutput, error)
	UpdateBackground(ctx context.Context, input *UpdateBackgroundInput) (*UpdateBackgroundOutput, error)
	UpdateAbilityScores(ctx context.Context, input *UpdateAbilityScoresInput) (*UpdateAbilityScoresOutput, error)
	UpdateSkills(ctx context.Context, input *UpdateSkillsInput) (*UpdateSkillsOutput, error)

	// Validation
	ValidateDraft(ctx context.Context, input *ValidateDraftInput) (*ValidateDraftOutput, error)

	// Character finalization
	FinalizeDraft(ctx context.Context, input *FinalizeDraftInput) (*FinalizeDraftOutput, error)

	// Completed character operations
	GetCharacter(ctx context.Context, input *GetCharacterInput) (*GetCharacterOutput, error)
	ListCharacters(ctx context.Context, input *ListCharactersInput) (*ListCharactersOutput, error)
	DeleteCharacter(ctx context.Context, input *DeleteCharacterInput) (*DeleteCharacterOutput, error)

	// Data loading for character creation UI
	ListRaces(ctx context.Context, input *ListRacesInput) (*ListRacesOutput, error)
	ListClasses(ctx context.Context, input *ListClassesInput) (*ListClassesOutput, error)
	ListBackgrounds(ctx context.Context, input *ListBackgroundsInput) (*ListBackgroundsOutput, error)
	ListSpells(ctx context.Context, input *ListSpellsInput) (*ListSpellsOutput, error)
	GetRaceDetails(ctx context.Context, input *GetRaceDetailsInput) (*GetRaceDetailsOutput, error)
	GetClassDetails(ctx context.Context, input *GetClassDetailsInput) (*GetClassDetailsOutput, error)
	GetBackgroundDetails(ctx context.Context, input *GetBackgroundDetailsInput) (*GetBackgroundDetailsOutput, error)

	// Dice rolling for ability scores
	RollAbilityScores(ctx context.Context, input *RollAbilityScoresInput) (*RollAbilityScoresOutput, error)
}

// Config holds the dependencies for the character orchestrator
type Config struct {
	CharacterRepo      characterrepo.Repository
	CharacterDraftRepo draftrepo.Repository
	Engine             engine.Engine
	ExternalClient     external.Client
	DiceService        dice.Service
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
	if c.DiceService == nil {
		vb.RequiredField("DiceService")
	}

	return vb.Build()
}

// Orchestrator implements the Service interface
type Orchestrator struct {
	characterRepo      characterrepo.Repository
	characterDraftRepo draftrepo.Repository
	engine             engine.Engine
	externalClient     external.Client
	diceService        dice.Service
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
		diceService:        cfg.DiceService,
	}, nil
}

// Ensure Orchestrator implements the Service interface
var _ Service = (*Orchestrator)(nil)

// Draft lifecycle methods

// CreateDraft creates a new character draft
func (o *Orchestrator) CreateDraft(
	ctx context.Context,
	input *CreateDraftInput,
) (*CreateDraftOutput, error) {
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
		// Repository will set ID, timestamps, and expiration
	}

	// Apply initial data if provided
	// Note: We intentionally ignore any ID, timestamps, or repository-managed fields from InitialData
	// The repository will handle ID generation and find/replace existing drafts for this player
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
	createOutput, err := o.characterDraftRepo.Create(ctx, draftrepo.CreateInput{Draft: draft})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create draft")
	}

	return &CreateDraftOutput{
		Draft: createOutput.Draft,
	}, nil
}

// GetDraft retrieves a character draft by ID
func (o *Orchestrator) GetDraft(
	ctx context.Context,
	input *GetDraftInput,
) (*GetDraftOutput, error) {
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

	return &GetDraftOutput{
		Draft: getOutput.Draft,
	}, nil
}

// ListDrafts lists character drafts with optional filters
func (o *Orchestrator) ListDrafts(
	ctx context.Context,
	input *ListDraftsInput,
) (*ListDraftsOutput, error) {
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
			return &ListDraftsOutput{
				Drafts:        []*dnd5e.CharacterDraft{},
				NextPageToken: "",
			}, nil
		}
		return nil, errors.Wrap(err, "failed to get player draft")
	}

	// Return single draft as a list
	return &ListDraftsOutput{
		Drafts:        []*dnd5e.CharacterDraft{draftOutput.Draft},
		NextPageToken: "", // No pagination needed for single draft
	}, nil
}

// DeleteDraft deletes a character draft
func (o *Orchestrator) DeleteDraft(
	ctx context.Context,
	input *DeleteDraftInput,
) (*DeleteDraftOutput, error) {
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

	return &DeleteDraftOutput{
		Message: fmt.Sprintf("Draft %s deleted successfully", input.DraftID),
	}, nil
}

// Section-based update methods

// UpdateName updates the character's name
func (o *Orchestrator) UpdateName(
	ctx context.Context,
	input *UpdateNameInput,
) (*UpdateNameOutput, error) {
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
	// Repository will update timestamp

	// Update progress
	draft.Progress.SetStep(dnd5e.ProgressStepName, true)
	o.updateCompletionPercentage(draft)

	// Save the updated draft
	updateOutput, err := o.characterDraftRepo.Update(ctx, draftrepo.UpdateInput{Draft: draft})
	if err != nil {
		return nil, errors.Wrap(err, "failed to update draft")
	}

	return &UpdateNameOutput{
		Draft: updateOutput.Draft,
	}, nil
}

// UpdateRace updates the character's race
func (o *Orchestrator) UpdateRace(
	ctx context.Context,
	input *UpdateRaceInput,
) (*UpdateRaceOutput, error) {
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
	var warnings []ValidationWarning
	if !validateOutput.IsValid {
		warnings = convertValidationErrorsToWarnings(validateOutput.Errors)
	}

	// Update the race
	draft.RaceID = input.RaceID
	draft.SubraceID = input.SubraceID
	// Repository will update timestamp

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
	updateOutput, err := o.characterDraftRepo.Update(ctx, draftrepo.UpdateInput{Draft: draft})
	if err != nil {
		return nil, errors.Wrap(err, "failed to update draft")
	}

	return &UpdateRaceOutput{
		Draft:    updateOutput.Draft,
		Warnings: warnings,
	}, nil
}

// UpdateClass updates the character's class
func (o *Orchestrator) UpdateClass(
	ctx context.Context,
	input *UpdateClassInput,
) (*UpdateClassOutput, error) {
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
	var warnings []ValidationWarning
	if !validateOutput.IsValid {
		warnings = convertValidationErrorsToWarnings(validateOutput.Errors)
	}
	warnings = append(warnings, convertValidationWarnings(validateOutput.Warnings)...)

	// Update the class
	draft.ClassID = input.ClassID
	// Repository will update timestamp

	// Reset dependent fields when class changes
	draft.StartingSkillIDs = nil

	// Update progress
	draft.Progress.SetStep(dnd5e.ProgressStepClass, true)
	// Reset dependent steps
	draft.Progress.SetStep(dnd5e.ProgressStepSkills, false)
	o.updateCompletionPercentage(draft)

	// Save the updated draft
	updateOutput, err := o.characterDraftRepo.Update(ctx, draftrepo.UpdateInput{Draft: draft})
	if err != nil {
		return nil, errors.Wrap(err, "failed to update draft")
	}

	return &UpdateClassOutput{
		Draft:    updateOutput.Draft,
		Warnings: warnings,
	}, nil
}

// UpdateBackground updates the character's background
func (o *Orchestrator) UpdateBackground(
	ctx context.Context,
	input *UpdateBackgroundInput,
) (*UpdateBackgroundOutput, error) {
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
	// Repository will update timestamp

	// Reset dependent fields when background changes
	draft.StartingSkillIDs = nil

	// Update progress
	draft.Progress.SetStep(dnd5e.ProgressStepBackground, true)
	// Reset dependent steps
	draft.Progress.SetStep(dnd5e.ProgressStepSkills, false)
	o.updateCompletionPercentage(draft)

	// Save the updated draft
	updateOutput, err := o.characterDraftRepo.Update(ctx, draftrepo.UpdateInput{Draft: draft})
	if err != nil {
		return nil, errors.Wrap(err, "failed to update draft")
	}

	return &UpdateBackgroundOutput{
		Draft: updateOutput.Draft,
	}, nil
}

// UpdateAbilityScores updates the character's ability scores
func (o *Orchestrator) UpdateAbilityScores(
	ctx context.Context,
	input *UpdateAbilityScoresInput,
) (*UpdateAbilityScoresOutput, error) {
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
		Method:        "standard_array", // TODO(#82): Make ability score method configurable
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
	var warnings []ValidationWarning

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
				warnings = append(warnings, ValidationWarning{
					Field:   "class_requirements",
					Message: e.Message,
					Type:    e.Code,
				})
			}
		}
	}

	// Update the ability scores
	draft.AbilityScores = &input.AbilityScores
	// Repository will update timestamp

	// Update progress
	draft.Progress.SetStep(dnd5e.ProgressStepAbilityScores, true)
	o.updateCompletionPercentage(draft)

	// Save the updated draft
	updateOutput, err := o.characterDraftRepo.Update(ctx, draftrepo.UpdateInput{Draft: draft})
	if err != nil {
		return nil, errors.Wrap(err, "failed to update draft")
	}

	return &UpdateAbilityScoresOutput{
		Draft:    updateOutput.Draft,
		Warnings: warnings,
	}, nil
}

// UpdateSkills updates the character's starting skills
func (o *Orchestrator) UpdateSkills(
	ctx context.Context,
	input *UpdateSkillsInput,
) (*UpdateSkillsOutput, error) {
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
	var warnings []ValidationWarning

	// Check prerequisites
	if draft.ClassID == "" || draft.BackgroundID == "" {
		warnings = append(warnings, ValidationWarning{
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
				warnings = append(warnings, ValidationWarning{
					Field:   e.Field,
					Message: e.Message,
					Type:    e.Code,
				})
			}
		}
	}

	// Update the skills
	draft.StartingSkillIDs = input.SkillIDs
	// Repository will update timestamp

	// Update progress
	draft.Progress.SetStep(dnd5e.ProgressStepSkills, true)
	o.updateCompletionPercentage(draft)

	// Save the updated draft
	updateOutput, err := o.characterDraftRepo.Update(ctx, draftrepo.UpdateInput{Draft: draft})
	if err != nil {
		return nil, errors.Wrap(err, "failed to update draft")
	}

	return &UpdateSkillsOutput{
		Draft:    updateOutput.Draft,
		Warnings: warnings,
	}, nil
}

// Validation methods

// ValidateDraft validates a character draft
func (o *Orchestrator) ValidateDraft(
	ctx context.Context,
	input *ValidateDraftInput,
) (*ValidateDraftOutput, error) {
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

	return &ValidateDraftOutput{
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
	input *FinalizeDraftInput,
) (*FinalizeDraftOutput, error) {
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
		// Repository will set ID, CreatedAt, and UpdatedAt
	}

	// Create the character in the repository
	createOutput, err := o.characterRepo.Create(ctx, characterrepo.CreateInput{Character: char})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create character")
	}

	// Delete the draft
	_, err = o.characterDraftRepo.Delete(ctx, draftrepo.DeleteInput{ID: draft.ID})
	if err != nil {
		slog.Error("failed to delete draft", "draft_id", draft.ID, "error", err)
		// Continue - the character was created successfully
	}

	return &FinalizeDraftOutput{
		Character:    createOutput.Character,
		DraftDeleted: err == nil,
	}, nil
}

// Completed character operations

// GetCharacter retrieves a character by ID
func (o *Orchestrator) GetCharacter(
	ctx context.Context,
	input *GetCharacterInput,
) (*GetCharacterOutput, error) {
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

	return &GetCharacterOutput{
		Character: getOutput.Character,
	}, nil
}

// ListCharacters lists characters with optional filters
func (o *Orchestrator) ListCharacters(
	ctx context.Context,
	input *ListCharactersInput,
) (*ListCharactersOutput, error) {
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

	return &ListCharactersOutput{
		Characters:    characters,
		NextPageToken: "", // TODO(#82): Implement pagination for ListCharacters
	}, nil
}

// DeleteCharacter deletes a character
func (o *Orchestrator) DeleteCharacter(
	ctx context.Context,
	input *DeleteCharacterInput,
) (*DeleteCharacterOutput, error) {
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

	return &DeleteCharacterOutput{
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
func convertValidationErrors(errors []engine.ValidationError) []ValidationError {
	result := make([]ValidationError, len(errors))
	for i, e := range errors {
		result[i] = ValidationError{
			Field:   e.Field,
			Message: e.Message,
			Type:    e.Code,
		}
	}
	return result
}

// convertValidationWarnings converts engine ValidationWarning to service ValidationWarning
func convertValidationWarnings(warnings []engine.ValidationWarning) []ValidationWarning {
	result := make([]ValidationWarning, len(warnings))
	for i, w := range warnings {
		result[i] = ValidationWarning{
			Field:   w.Field,
			Message: w.Message,
			Type:    w.Code,
		}
	}
	return result
}

// Data loading methods for character creation UI

// ListRaces retrieves available races for character creation
func (o *Orchestrator) ListRaces(
	ctx context.Context,
	input *ListRacesInput,
) (*ListRacesOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}

	slog.Info("Fetching races from external API",
		"includeSubraces", input.IncludeSubraces,
		"pageSize", input.PageSize)

	// TODO(#82): Implement pagination with PageSize and PageToken for ListRaces
	// For now, return all races
	races, err := o.externalClient.ListAvailableRaces(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list races from external client")
	}

	slog.Info("Successfully fetched races", "count", len(races))

	// Convert external race data to entity format
	entityRaces := make([]*dnd5e.RaceInfo, len(races))
	for i, race := range races {
		entityRaces[i] = convertExternalRaceToEntity(race)
	}

	var totalSize int32
	if len(entityRaces) > 2147483647 { // Max int32 value
		totalSize = 2147483647
	} else {
		// nolint:gosec // List size won't overflow int32
		totalSize = int32(len(entityRaces))
	}

	return &ListRacesOutput{
		Races:         entityRaces,
		NextPageToken: "", // TODO(#82): Implement pagination for ListRaces
		TotalSize:     totalSize,
	}, nil
}

// ListClasses retrieves available classes for character creation
func (o *Orchestrator) ListClasses(
	ctx context.Context,
	input *ListClassesInput,
) (*ListClassesOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}

	// TODO(#82): Implement pagination and filtering for ListBackgrounds
	classes, err := o.externalClient.ListAvailableClasses(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list classes from external client")
	}

	// Convert external class data to entity format
	entityClasses := make([]*dnd5e.ClassInfo, len(classes))
	for i, class := range classes {
		entityClasses[i] = convertExternalClassToEntity(class)
	}

	var totalSize int32
	if len(entityClasses) > 2147483647 { // Max int32 value
		totalSize = 2147483647
	} else {
		// nolint:gosec // List size won't overflow int32
		totalSize = int32(len(entityClasses))
	}

	return &ListClassesOutput{
		Classes:       entityClasses,
		NextPageToken: "", // TODO(#82): Implement pagination for ListClasses
		TotalSize:     totalSize,
	}, nil
}

// ListBackgrounds retrieves available backgrounds for character creation
func (o *Orchestrator) ListBackgrounds(
	ctx context.Context,
	input *ListBackgroundsInput,
) (*ListBackgroundsOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}

	// TODO(#82): Implement pagination for GetAvailableChoices
	backgrounds, err := o.externalClient.ListAvailableBackgrounds(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list backgrounds from external client")
	}

	// Convert external background data to entity format
	entityBackgrounds := make([]*dnd5e.BackgroundInfo, len(backgrounds))
	for i, background := range backgrounds {
		entityBackgrounds[i] = convertExternalBackgroundToEntity(background)
	}

	var totalSize int32
	if len(entityBackgrounds) > 2147483647 { // Max int32 value
		totalSize = 2147483647
	} else {
		// nolint:gosec // List size won't overflow int32
		totalSize = int32(len(entityBackgrounds))
	}

	return &ListBackgroundsOutput{
		Backgrounds:   entityBackgrounds,
		NextPageToken: "", // TODO(#82): Implement pagination for ListBackgrounds
		TotalSize:     totalSize,
	}, nil
}

// ListSpells retrieves available spells for character creation
func (o *Orchestrator) ListSpells(
	ctx context.Context,
	input *ListSpellsInput,
) (*ListSpellsOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}

	slog.Info("Fetching spells from external API",
		"level", input.Level,
		"school", input.School,
		"classID", input.ClassID,
		"searchTerm", input.SearchTerm,
		"pageSize", input.PageSize)

	// Convert orchestrator input to external client input
	externalInput := &external.ListSpellsInput{
		Level:   input.Level,
		ClassID: input.ClassID,
	}

	// TODO(#82): Implement pagination with PageSize and PageToken
	spells, err := o.externalClient.ListAvailableSpells(ctx, externalInput)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list spells from external client")
	}

	slog.Info("Successfully fetched spells", "count", len(spells))

	// Convert external spell data to entity format and apply additional filters
	entitySpells := make([]*dnd5e.SpellInfo, 0, len(spells))
	for _, spell := range spells {
		entitySpell := convertExternalSpellToEntity(spell)

		// Apply additional filters that the external client doesn't support
		if input.School != "" && entitySpell.School != input.School {
			continue
		}
		if input.SearchTerm != "" &&
			!strings.Contains(strings.ToLower(entitySpell.Name), strings.ToLower(input.SearchTerm)) &&
			!strings.Contains(strings.ToLower(entitySpell.Description), strings.ToLower(input.SearchTerm)) {
			continue
		}

		entitySpells = append(entitySpells, entitySpell)
	}

	// Apply client-side pagination
	startIndex := 0
	if input.PageToken != "" {
		var err error
		startIndex, err = strconv.Atoi(input.PageToken)
		if err != nil || startIndex < 0 || startIndex >= len(entitySpells) {
			return nil, errors.InvalidArgument("invalid PageToken")
		}
	}

	endIndex := startIndex + int(input.PageSize)
	if endIndex > len(entitySpells) {
		endIndex = len(entitySpells)
	}

	nextPageToken := ""
	if endIndex < len(entitySpells) {
		nextPageToken = strconv.Itoa(endIndex)
	}

	// Apply pagination to results
	paginatedSpells := entitySpells[startIndex:endIndex]

	var totalSize int32
	if len(entitySpells) > math.MaxInt32 {
		totalSize = math.MaxInt32
	} else {
		// nolint:gosec // List size won't overflow int32
		totalSize = int32(len(entitySpells))
	}

	return &ListSpellsOutput{
		Spells:        paginatedSpells,
		NextPageToken: nextPageToken,
		TotalSize:     totalSize,
	}, nil
}

// GetRaceDetails retrieves detailed information about a specific race
func (o *Orchestrator) GetRaceDetails(
	ctx context.Context,
	input *GetRaceDetailsInput,
) (*GetRaceDetailsOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}

	vb := errors.NewValidationBuilder()
	errors.ValidateRequired("raceID", input.RaceID, vb)
	if err := vb.Build(); err != nil {
		return nil, err
	}

	race, err := o.externalClient.GetRaceData(ctx, input.RaceID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get race details from external client")
	}

	entityRace := convertExternalRaceToEntity(race)

	return &GetRaceDetailsOutput{
		Race: entityRace,
	}, nil
}

// GetClassDetails retrieves detailed information about a specific class
func (o *Orchestrator) GetClassDetails(
	ctx context.Context,
	input *GetClassDetailsInput,
) (*GetClassDetailsOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}

	vb := errors.NewValidationBuilder()
	errors.ValidateRequired("classID", input.ClassID, vb)
	if err := vb.Build(); err != nil {
		return nil, err
	}

	class, err := o.externalClient.GetClassData(ctx, input.ClassID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get class details from external client")
	}

	entityClass := convertExternalClassToEntity(class)

	return &GetClassDetailsOutput{
		Class: entityClass,
	}, nil
}

// GetBackgroundDetails retrieves detailed information about a specific background
func (o *Orchestrator) GetBackgroundDetails(
	ctx context.Context,
	input *GetBackgroundDetailsInput,
) (*GetBackgroundDetailsOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}

	vb := errors.NewValidationBuilder()
	errors.ValidateRequired("backgroundID", input.BackgroundID, vb)
	if err := vb.Build(); err != nil {
		return nil, err
	}

	background, err := o.externalClient.GetBackgroundData(ctx, input.BackgroundID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get background details from external client")
	}

	entityBackground := convertExternalBackgroundToEntity(background)

	return &GetBackgroundDetailsOutput{
		Background: entityBackground,
	}, nil
}

// Conversion helpers for external data to entity format

// convertExternalRaceToEntity converts external race data to entity format
func convertExternalRaceToEntity(race *external.RaceData) *dnd5e.RaceInfo {
	traits := make([]dnd5e.RacialTrait, len(race.Traits))
	for i, trait := range race.Traits {
		traits[i] = dnd5e.RacialTrait{
			Name:        trait.Name,
			Description: trait.Description,
			IsChoice:    trait.IsChoice,
			Options:     trait.Options,
		}
	}

	subraces := make([]dnd5e.SubraceInfo, len(race.Subraces))
	for i, subrace := range race.Subraces {
		subraceTraits := make([]dnd5e.RacialTrait, len(subrace.Traits))
		for j, trait := range subrace.Traits {
			subraceTraits[j] = dnd5e.RacialTrait{
				Name:        trait.Name,
				Description: trait.Description,
				IsChoice:    trait.IsChoice,
				Options:     trait.Options,
			}
		}

		subraces[i] = dnd5e.SubraceInfo{
			ID:             subrace.ID,
			Name:           subrace.Name,
			Description:    subrace.Description,
			AbilityBonuses: subrace.AbilityBonuses,
			Traits:         subraceTraits,
			Languages:      subrace.Languages,
			Proficiencies:  subrace.Proficiencies,
		}
	}

	// Convert language options
	var languageOptions *dnd5e.Choice
	if race.LanguageOptions != nil {
		languageOptions = &dnd5e.Choice{
			Type: race.LanguageOptions.Type,
			// nolint:gosec // safe conversion
			Choose:  int32(race.LanguageOptions.Choose),
			Options: race.LanguageOptions.Options,
			From:    race.LanguageOptions.From,
		}
	}

	// Convert proficiency options
	proficiencyOptions := make([]dnd5e.Choice, len(race.ProficiencyOptions))
	for i, opt := range race.ProficiencyOptions {
		if opt != nil {
			proficiencyOptions[i] = dnd5e.Choice{
				Type: opt.Type,
				// nolint:gosec // safe conversion
				Choose:  int32(opt.Choose),
				Options: opt.Options,
				From:    opt.From,
			}
		}
	}

	return &dnd5e.RaceInfo{
		ID:                   race.ID,
		Name:                 race.Name,
		Description:          race.Description,
		Speed:                race.Speed,
		Size:                 race.Size,
		SizeDescription:      race.SizeDescription,
		AbilityBonuses:       race.AbilityBonuses,
		Traits:               traits,
		Subraces:             subraces,
		Proficiencies:        race.Proficiencies,
		Languages:            race.Languages,
		AgeDescription:       race.AgeDescription,
		AlignmentDescription: race.AlignmentDescription,
		LanguageOptions:      languageOptions,
		ProficiencyOptions:   proficiencyOptions,
	}
}

// convertExternalClassToEntity converts external class data to entity format
func convertExternalClassToEntity(class *external.ClassData) *dnd5e.ClassInfo {
	// Convert equipment choices
	equipmentChoices := make([]dnd5e.EquipmentChoice, len(class.StartingEquipmentOptions))
	for i, choice := range class.StartingEquipmentOptions {
		if choice != nil {
			equipmentChoices[i] = dnd5e.EquipmentChoice{
				Description: choice.Description,
				Options:     choice.Options,
				// nolint:gosec // safe conversion
				ChooseCount: int32(choice.ChooseCount),
			}
		}
	}

	// Convert level 1 features
	features := make([]dnd5e.ClassFeature, len(class.LevelOneFeatures))
	for i, feature := range class.LevelOneFeatures {
		if feature != nil {
			features[i] = dnd5e.ClassFeature{
				Name:        feature.Name,
				Description: feature.Description,
				// nolint:gosec // safe conversion
				Level:      int32(feature.Level),
				HasChoices: feature.HasChoices,
				Choices:    feature.Choices,
			}
		}
	}

	// Convert spellcasting info
	var spellcasting *dnd5e.SpellcastingInfo
	if class.Spellcasting != nil {
		spellcasting = &dnd5e.SpellcastingInfo{
			SpellcastingAbility: class.Spellcasting.SpellcastingAbility,
			RitualCasting:       class.Spellcasting.RitualCasting,
			SpellcastingFocus:   class.Spellcasting.SpellcastingFocus,
			// nolint:gosec // safe conversion
			CantripsKnown:    int32(class.Spellcasting.CantripsKnown),
			SpellsKnown:      int32(class.Spellcasting.SpellsKnown),      // nolint:gosec
			SpellSlotsLevel1: int32(class.Spellcasting.SpellSlotsLevel1), // nolint:gosec
		}
	}

	// Convert proficiency choices
	proficiencyChoices := make([]dnd5e.Choice, len(class.ProficiencyChoices))
	for i, choice := range class.ProficiencyChoices {
		if choice != nil {
			proficiencyChoices[i] = dnd5e.Choice{
				Type: choice.Type,
				// nolint:gosec // safe conversion
				Choose:  int32(choice.Choose),
				Options: choice.Options,
				From:    choice.From,
			}
		}
	}

	return &dnd5e.ClassInfo{
		ID:                       class.ID,
		Name:                     class.Name,
		Description:              class.Description,
		HitDie:                   class.HitDice,
		PrimaryAbilities:         class.PrimaryAbilities,
		ArmorProficiencies:       class.ArmorProficiencies,
		WeaponProficiencies:      class.WeaponProficiencies,
		ToolProficiencies:        class.ToolProficiencies,
		SavingThrowProficiencies: class.SavingThrows,
		SkillChoicesCount:        class.SkillsCount,
		AvailableSkills:          class.AvailableSkills,
		StartingEquipment:        class.StartingEquipment,
		EquipmentChoices:         equipmentChoices,
		Level1Features:           features,
		Spellcasting:             spellcasting,
		ProficiencyChoices:       proficiencyChoices,
	}
}

// convertExternalBackgroundToEntity converts external background data to entity format
func convertExternalBackgroundToEntity(background *external.BackgroundData) *dnd5e.BackgroundInfo {
	return &dnd5e.BackgroundInfo{
		ID:                  background.ID,
		Name:                background.Name,
		Description:         background.Description,
		SkillProficiencies:  background.SkillProficiencies,
		ToolProficiencies:   []string{}, // TODO(#82): Get tool proficiencies from external data
		Languages:           []string{}, // TODO(#82): Get languages from external data
		AdditionalLanguages: background.Languages,
		StartingEquipment:   background.Equipment,
		StartingGold:        0, // TODO(#82): Get starting gold from external data
		FeatureName:         background.Feature,
		FeatureDescription:  background.Feature, // TODO(#82): Get detailed feature description
		PersonalityTraits:   []string{},         // TODO(#82): Get personality traits from external data
		Ideals:              []string{},         // TODO(#82): Get ideals from external data
		Bonds:               []string{},         // TODO(#82): Get bonds from external data
		Flaws:               []string{},         // TODO(#82): Get flaws from external data
	}
}

// convertExternalSpellToEntity converts external spell data to entity format
func convertExternalSpellToEntity(spell *external.SpellData) *dnd5e.SpellInfo {
	// Parse classes from spell description if available
	classes := parseClassesFromSpellDescription(spell.Description)

	return &dnd5e.SpellInfo{
		ID:          spell.ID,
		Name:        spell.Name,
		Level:       spell.Level,
		School:      spell.School,
		CastingTime: spell.CastingTime,
		Range:       spell.Range,
		Components:  spell.Components,
		Duration:    spell.Duration,
		Description: spell.Description,
		Classes:     classes,
	}
}

// parseClassesFromSpellDescription extracts class names from spell description
func parseClassesFromSpellDescription(description string) []string {
	// Look for "Classes: <class1>, <class2>..." pattern in description
	classesPattern := "Classes: "
	classesIndex := strings.Index(description, classesPattern)
	if classesIndex == -1 {
		return []string{}
	}

	// Extract the classes substring
	start := classesIndex + len(classesPattern)
	end := strings.Index(description[start:], ".")
	if end == -1 {
		// Classes might be at the end without a period
		end = len(description) - start
	}

	classesStr := strings.TrimSpace(description[start : start+end])
	if classesStr == "" {
		return []string{}
	}

	// Split by comma and clean up
	parts := strings.Split(classesStr, ",")
	classes := make([]string, 0, len(parts))
	for _, part := range parts {
		class := strings.TrimSpace(part)
		if class != "" {
			classes = append(classes, class)
		}
	}

	return classes
}

// convertValidationErrorsToWarnings converts engine ValidationError to service ValidationWarning
func convertValidationErrorsToWarnings(errors []engine.ValidationError) []ValidationWarning {
	result := make([]ValidationWarning, len(errors))
	for i, e := range errors {
		result[i] = ValidationWarning{
			Field:   e.Field,
			Message: e.Message,
			Type:    e.Code,
		}
	}
	return result
}

// RollAbilityScores rolls ability scores for character creation using the dice service
func (o *Orchestrator) RollAbilityScores(
	ctx context.Context,
	input *RollAbilityScoresInput,
) (*RollAbilityScoresOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}

	vb := errors.NewValidationBuilder()
	errors.ValidateRequired("draftID", input.DraftID, vb)
	if err := vb.Build(); err != nil {
		return nil, err
	}

	// Get the draft to validate it exists
	_, err := o.characterDraftRepo.Get(ctx, draftrepo.GetInput{ID: input.DraftID})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get draft")
	}

	// Use default method if not specified
	method := input.Method
	if method == "" {
		method = dice.MethodStandard
	}

	// Roll ability scores using dice service
	diceInput := &dice.RollAbilityScoresInput{
		EntityID: input.DraftID,
		Method:   method,
	}
	diceOutput, err := o.diceService.RollAbilityScores(ctx, diceInput)
	if err != nil {
		return nil, errors.Wrap(err, "failed to roll ability scores")
	}

	// Convert dice rolls to character service format
	rolls := make([]*AbilityScoreRoll, len(diceOutput.Rolls))
	for i, roll := range diceOutput.Rolls {
		rolls[i] = &AbilityScoreRoll{
			ID:          roll.RollID,
			Value:       roll.Total,
			Description: roll.Description,
			RolledAt:    diceOutput.Session.CreatedAt.Unix(),
			Dice:        roll.Dice,
			Dropped:     roll.Dropped,
			Notation:    roll.Notation,
		}
	}

	return &RollAbilityScoresOutput{
		Rolls:     rolls,
		SessionID: diceOutput.Session.EntityID + ":" + diceOutput.Session.Context,
		ExpiresAt: diceOutput.Session.ExpiresAt.Unix(),
	}, nil
}
