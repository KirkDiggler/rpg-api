// Package character implements the character orchestrator
package character

//go:generate mockgen -destination=mock/mock_service.go -package=charactermock github.com/KirkDiggler/rpg-api/internal/orchestrators/character Service

import (
	"context"
	"fmt"
	"log"
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
	UpdateChoices(ctx context.Context, input *UpdateChoicesInput) (*UpdateChoicesOutput, error)

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
	ListChoiceOptions(ctx context.Context, input *ListChoiceOptionsInput) (*ListChoiceOptionsOutput, error)

	// Equipment and spell filtering for character creation choices
	ListEquipmentByType(ctx context.Context, input *ListEquipmentByTypeInput) (*ListEquipmentByTypeOutput, error)
	ListSpellsByLevel(ctx context.Context, input *ListSpellsByLevelInput) (*ListSpellsByLevelOutput, error)

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

	// Create new draft data with basic information
	draftData := &dnd5e.CharacterDraftData{
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
			draftData.Name = input.InitialData.Name
			draftData.Progress.SetStep(dnd5e.ProgressStepName, true)
		}
		if input.InitialData.RaceID != "" {
			draftData.RaceID = input.InitialData.RaceID
			draftData.SubraceID = input.InitialData.SubraceID
			draftData.Progress.SetStep(dnd5e.ProgressStepRace, true)
		}
		if input.InitialData.ClassID != "" {
			draftData.ClassID = input.InitialData.ClassID
			draftData.Progress.SetStep(dnd5e.ProgressStepClass, true)
		}
		if input.InitialData.BackgroundID != "" {
			draftData.BackgroundID = input.InitialData.BackgroundID
			draftData.Progress.SetStep(dnd5e.ProgressStepBackground, true)
		}
		if input.InitialData.AbilityScores != nil {
			draftData.AbilityScores = input.InitialData.AbilityScores
			draftData.Progress.SetStep(dnd5e.ProgressStepAbilityScores, true)
		}
		// Skills and languages are now handled through ChoiceSelections
		// TODO(#46): Convert InitialData skills/languages to ChoiceSelections
		draftData.Alignment = input.InitialData.Alignment

		// Convert to CharacterDraft for validation and percentage update
		draft := draftData.ToCharacterDraft()
		o.updateCompletionPercentage(draft)
		// Copy updated progress back to draftData
		draftData.Progress = draft.Progress
	}

	// Validate the draft with the engine
	// Convert to CharacterDraft for validation
	draft := draftData.ToCharacterDraft()
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

	// Create the draft data in the repository
	createOutput, err := o.characterDraftRepo.Create(ctx, draftrepo.CreateInput{Draft: draftData})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create draft")
	}

	// Convert the created draft data to CharacterDraft
	createdDraft := createOutput.Draft.ToCharacterDraft()

	// No need to hydrate on create since we don't have race/class/background yet
	// Just return the created draft
	return &CreateDraftOutput{
		Draft: createdDraft,
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

	// Convert CharacterDraftData to CharacterDraft
	// Convert to CharacterDraft for manipulation
	draft := getOutput.Draft.ToCharacterDraft()

	// Hydrate the draft with info objects
	hydratedDraft, err := o.hydrateDraft(ctx, draft)
	if err != nil {
		// Log error but don't fail the get operation
		// TODO(#46): Add proper logging
		return &GetDraftOutput{
			Draft: draft,
		}, nil
	}

	return &GetDraftOutput{
		Draft: hydratedDraft,
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

	// Convert CharacterDraftData to CharacterDraft
	draft := draftOutput.Draft.ToCharacterDraft()

	// Hydrate the draft with info objects
	hydratedDraft, err := o.hydrateDraft(ctx, draft)
	if err != nil {
		// Log error but return the draft anyway
		// TODO(#46): Add proper logging
		return &ListDraftsOutput{
			Drafts:        []*dnd5e.CharacterDraft{draft},
			NextPageToken: "", // No pagination needed for single draft
		}, nil
	}

	// Return single draft as a list
	return &ListDraftsOutput{
		Drafts:        []*dnd5e.CharacterDraft{hydratedDraft},
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

	// Convert to CharacterDraft for manipulation
	// Convert to CharacterDraft for manipulation
	draft := getOutput.Draft.ToCharacterDraft()

	// Update the name
	draft.Name = input.Name
	// Repository will update timestamp

	// Update progress
	draft.Progress.SetStep(dnd5e.ProgressStepName, true)
	o.updateCompletionPercentage(draft)

	// Convert back to CharacterDraftData for repository update
	draftData := dnd5e.FromCharacterDraft(draft)

	// Save the updated draft
	updateOutput, err := o.characterDraftRepo.Update(ctx, draftrepo.UpdateInput{Draft: draftData})
	if err != nil {
		return nil, errors.Wrap(err, "failed to update draft")
	}

	// Convert result to CharacterDraft for return
	updatedDraft := updateOutput.Draft.ToCharacterDraft()

	return &UpdateNameOutput{
		Draft: updatedDraft,
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

	// Convert to CharacterDraft for manipulation
	// Convert to CharacterDraft for manipulation
	draft := getOutput.Draft.ToCharacterDraft()

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

	// Handle race choices
	if len(input.Choices) > 0 {
		// Remove any existing race/subrace choices
		var filteredChoices []dnd5e.ChoiceSelection
		for _, choice := range draft.ChoiceSelections {
			if choice.Source != dnd5e.ChoiceSourceRace && choice.Source != dnd5e.ChoiceSourceSubrace {
				filteredChoices = append(filteredChoices, choice)
			}
		}

		// Add new race choices
		for _, choice := range input.Choices {
			// Validate the choice matches the race source
			if choice.Source == dnd5e.ChoiceSourceRace || choice.Source == dnd5e.ChoiceSourceSubrace {
				filteredChoices = append(filteredChoices, choice)
			}
		}

		draft.ChoiceSelections = filteredChoices
	}

	// Update progress
	draft.Progress.SetStep(dnd5e.ProgressStepRace, true)
	// Reset dependent steps
	draft.Progress.SetStep(dnd5e.ProgressStepAbilityScores, false)
	draft.Progress.SetStep(dnd5e.ProgressStepSkills, false)
	draft.Progress.SetStep(dnd5e.ProgressStepLanguages, false)
	o.updateCompletionPercentage(draft)

	// Convert back to CharacterDraftData for repository update
	draftData := dnd5e.FromCharacterDraft(draft)

	// Save the updated draft
	updateOutput, err := o.characterDraftRepo.Update(ctx, draftrepo.UpdateInput{Draft: draftData})
	if err != nil {
		return nil, errors.Wrap(err, "failed to update draft")
	}

	// Convert result to CharacterDraft and hydrate with race info
	updatedDraft := updateOutput.Draft.ToCharacterDraft()
	hydratedDraft, err := o.hydrateDraft(ctx, updatedDraft)
	if err != nil {
		// Log error but don't fail the update
		// The draft was successfully saved, just couldn't populate the info
		return &UpdateRaceOutput{
			Draft:    updatedDraft,
			Warnings: warnings,
		}, nil
	}

	slog.Info("UpdateRace returning hydrated draft",
		"draft_id", hydratedDraft.ID,
		"race_id", hydratedDraft.RaceID,
		"race_info_nil", hydratedDraft.Race == nil,
		"race_name", func() string {
			if hydratedDraft.Race != nil {
				return hydratedDraft.Race.Name
			}
			return "nil"
		}(),
		"choice_count", len(hydratedDraft.ChoiceSelections),
		"choices", hydratedDraft.ChoiceSelections,
	)
	
	return &UpdateRaceOutput{
		Draft:    hydratedDraft,
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

	// Convert to CharacterDraft for manipulation
	// Convert to CharacterDraft for manipulation
	draft := getOutput.Draft.ToCharacterDraft()

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

	// Class changes don't reset any fields currently
	// Skills are handled through choices

	// Handle class choices
	if len(input.Choices) > 0 {
		// Remove any existing class choices
		var filteredChoices []dnd5e.ChoiceSelection
		for _, choice := range draft.ChoiceSelections {
			if choice.Source != dnd5e.ChoiceSourceClass {
				filteredChoices = append(filteredChoices, choice)
			}
		}

		// Add new class choices
		for _, choice := range input.Choices {
			// Validate the choice matches the class source
			if choice.Source == dnd5e.ChoiceSourceClass {
				filteredChoices = append(filteredChoices, choice)
			}
		}

		draft.ChoiceSelections = filteredChoices
	}

	// Update progress
	draft.Progress.SetStep(dnd5e.ProgressStepClass, true)
	// Reset dependent steps
	draft.Progress.SetStep(dnd5e.ProgressStepSkills, false)
	o.updateCompletionPercentage(draft)

	// Convert back to CharacterDraftData for repository update
	draftData := dnd5e.FromCharacterDraft(draft)

	// Save the updated draft
	updateOutput, err := o.characterDraftRepo.Update(ctx, draftrepo.UpdateInput{Draft: draftData})
	if err != nil {
		return nil, errors.Wrap(err, "failed to update draft")
	}

	// Convert result to CharacterDraft and hydrate with class info
	updatedDraft := updateOutput.Draft.ToCharacterDraft()
	hydratedDraft, err := o.hydrateDraft(ctx, updatedDraft)
	if err != nil {
		// Log error but don't fail the update
		// The draft was successfully saved, just couldn't populate the info
		return &UpdateClassOutput{
			Draft:    updatedDraft,
			Warnings: warnings,
		}, nil
	}

	return &UpdateClassOutput{
		Draft:    hydratedDraft,
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
	// Convert to CharacterDraft for manipulation
	draft := getOutput.Draft.ToCharacterDraft()

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

	// Background changes don't reset any fields currently
	// Skills are handled through choices

	// Handle background choices
	if len(input.Choices) > 0 {
		// Remove any existing background choices
		var filteredChoices []dnd5e.ChoiceSelection
		for _, choice := range draft.ChoiceSelections {
			if choice.Source != dnd5e.ChoiceSourceBackground {
				filteredChoices = append(filteredChoices, choice)
			}
		}

		// Add new background choices
		for _, choice := range input.Choices {
			// Validate the choice matches the background source
			if choice.Source == dnd5e.ChoiceSourceBackground {
				filteredChoices = append(filteredChoices, choice)
			}
		}

		draft.ChoiceSelections = filteredChoices
	}

	// Update progress
	draft.Progress.SetStep(dnd5e.ProgressStepBackground, true)
	// Reset dependent steps
	draft.Progress.SetStep(dnd5e.ProgressStepSkills, false)
	o.updateCompletionPercentage(draft)

	// Convert back to CharacterDraftData for repository update
	draftData := dnd5e.FromCharacterDraft(draft)

	// Save the updated draft
	updateOutput, err := o.characterDraftRepo.Update(ctx, draftrepo.UpdateInput{Draft: draftData})
	if err != nil {
		return nil, errors.Wrap(err, "failed to update draft")
	}

	// Convert result to CharacterDraft and hydrate with background info
	updatedDraft := updateOutput.Draft.ToCharacterDraft()
	hydratedDraft, err := o.hydrateDraft(ctx, updatedDraft)
	if err != nil {
		// Log error but don't fail the update
		// The draft was successfully saved, just couldn't populate the info
		return &UpdateBackgroundOutput{
			Draft: updatedDraft,
		}, nil
	}

	return &UpdateBackgroundOutput{
		Draft: hydratedDraft,
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
	// Convert to CharacterDraft for manipulation
	draft := getOutput.Draft.ToCharacterDraft()

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

	// Convert back to CharacterDraftData for repository update
	draftData := dnd5e.FromCharacterDraft(draft)

	// Save the updated draft
	updateOutput, err := o.characterDraftRepo.Update(ctx, draftrepo.UpdateInput{Draft: draftData})
	if err != nil {
		return nil, errors.Wrap(err, "failed to update draft")
	}

	// Convert result to CharacterDraft for return
	updatedDraft := updateOutput.Draft.ToCharacterDraft()

	return &UpdateAbilityScoresOutput{
		Draft:    updatedDraft,
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
	// Convert to CharacterDraft for manipulation
	draft := getOutput.Draft.ToCharacterDraft()

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

	// Update skills through choices
	// TODO(#46): Convert skill selection to use ChoiceSelections
	// For now, we'll need to store skills as choices
	// Repository will update timestamp

	// Update progress
	draft.Progress.SetStep(dnd5e.ProgressStepSkills, true)
	o.updateCompletionPercentage(draft)

	// Convert back to CharacterDraftData for repository update
	draftData := dnd5e.FromCharacterDraft(draft)

	// Save the updated draft
	updateOutput, err := o.characterDraftRepo.Update(ctx, draftrepo.UpdateInput{Draft: draftData})
	if err != nil {
		return nil, errors.Wrap(err, "failed to update draft")
	}

	// Convert result to CharacterDraft for return
	updatedDraft := updateOutput.Draft.ToCharacterDraft()

	return &UpdateSkillsOutput{
		Draft:    updatedDraft,
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
	// Convert to CharacterDraft for manipulation
	draft := getOutput.Draft.ToCharacterDraft()

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
	// Convert to CharacterDraft for manipulation
	draft := getOutput.Draft.ToCharacterDraft()

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

	log.Printf("ListCharacters called with PlayerID=%s, SessionID=%s, PageSize=%d, PageToken=%s",
		input.PlayerID, input.SessionID, input.PageSize, input.PageToken)

	// Default page size
	if input.PageSize == 0 {
		input.PageSize = 20
	}

	// Use specific list methods based on filters
	var characters []*dnd5e.Character
	switch {
	case input.PlayerID != "":
		slog.InfoContext(ctx, "listing characters by player",
			"player_id", input.PlayerID,
			"page_size", input.PageSize,
			"page_token", input.PageToken)
		listOutput, err := o.characterRepo.ListByPlayerID(ctx, characterrepo.ListByPlayerIDInput{PlayerID: input.PlayerID})
		if err != nil {
			slog.ErrorContext(ctx, "failed to list characters by player",
				"player_id", input.PlayerID,
				"error", err.Error())
			return nil, errors.Wrapf(err, "failed to list characters for player %s", input.PlayerID)
		}
		characters = listOutput.Characters
		slog.InfoContext(ctx, "successfully listed characters by player",
			"player_id", input.PlayerID,
			"count", len(characters))
	case input.SessionID != "":
		slog.InfoContext(ctx, "listing characters by session",
			"session_id", input.SessionID,
			"page_size", input.PageSize,
			"page_token", input.PageToken)
		listOutput, err := o.characterRepo.ListBySessionID(ctx,
			characterrepo.ListBySessionIDInput{SessionID: input.SessionID})
		if err != nil {
			slog.ErrorContext(ctx, "failed to list characters by session",
				"session_id", input.SessionID,
				"error", err.Error())
			return nil, errors.Wrapf(err, "failed to list characters for session %s", input.SessionID)
		}
		characters = listOutput.Characters
		slog.InfoContext(ctx, "successfully listed characters by session",
			"session_id", input.SessionID,
			"count", len(characters))
	default:
		log.Printf("ListCharacters called without PlayerID or SessionID")
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
	totalSteps := 8 // Name, Race, Class, Background, AbilityScores, Skills, Languages, Choices
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
	if draft.Progress.HasChoices() {
		completedSteps++
	}

	// Safe conversion - totalSteps is always 8 and completedSteps is 0-8
	// so max value is 800/8 = 100, which fits safely in int32
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

	// Note: Language and proficiency options are now handled through the rich Choices field
	// These conversions are kept for backward compatibility with deprecated fields

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
		LanguageOptions:      nil,          // Deprecated: handled through Choices
		ProficiencyOptions:   nil,          // Deprecated: handled through Choices
		Choices:              race.Choices, // Pass through rich choices from external client
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
	features := make([]dnd5e.FeatureInfo, len(class.LevelOneFeatures))
	for i, feature := range class.LevelOneFeatures {
		if feature != nil {
			// Convert external FeatureData to entity FeatureInfo
			convertedFeature := convertExternalFeatureToEntity(feature)
			features[i] = *convertedFeature
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
			SpellsKnown:      int32(class.Spellcasting.SpellsKnown),
			SpellSlotsLevel1: int32(class.Spellcasting.SpellSlotsLevel1),
		}
	}

	// Note: Proficiency choices are now handled through the rich Choices field
	// This conversion is kept for backward compatibility with deprecated fields

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
		ProficiencyChoices:       nil,           // Deprecated: handled through Choices
		Choices:                  class.Choices, // Pass through rich choices from external client
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
		AdditionalLanguages: 0,          // TODO(#82): Get additional language count from external data
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

// convertExternalFeatureToEntity converts external feature data to entity format
func convertExternalFeatureToEntity(feature *external.FeatureData) *dnd5e.FeatureInfo {
	if feature == nil {
		return nil
	}

	entityFeature := &dnd5e.FeatureInfo{
		ID:          feature.ID,
		Name:        feature.Name,
		Description: feature.Description,
		Level:       feature.Level,
		ClassName:   feature.ClassName,
		HasChoices:  feature.HasChoices,
	}

	// Note: Feature choices would need to be converted to the new Choice structure
	// For now, leaving this empty as features don't support the new choice system yet
	entityFeature.Choices = nil

	// Convert spell selection info
	if feature.SpellSelection != nil {
		entityFeature.SpellSelection = &dnd5e.SpellSelectionInfo{
			SpellsToSelect:  feature.SpellSelection.SpellsToSelect,
			SpellLevels:     feature.SpellSelection.SpellLevels,
			SpellLists:      feature.SpellSelection.SpellLists,
			SelectionType:   feature.SpellSelection.SelectionType,
			RequiresReplace: feature.SpellSelection.RequiresReplace,
		}
	}

	return entityFeature
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

// UpdateChoices updates the choices for a character draft
func (o *Orchestrator) UpdateChoices(
	ctx context.Context,
	input *UpdateChoicesInput,
) (*UpdateChoicesOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}

	if input.DraftID == "" {
		return nil, errors.InvalidArgument("draft ID is required")
	}

	slog.Info("Updating character choices", "draft_id", input.DraftID, "selections", len(input.Selections))

	// Get the existing draft
	getDraftInput := &GetDraftInput{DraftID: input.DraftID}
	getDraftOutput, err := o.GetDraft(ctx, getDraftInput)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get draft")
	}

	draft := getDraftOutput.Draft

	// Validate that the draft has the required information for choices
	if draft.ClassID == "" {
		return nil, errors.InvalidArgument("class must be selected before making choices")
	}

	// Initialize choices if not present
	if draft.ChoiceSelections == nil {
		draft.ChoiceSelections = []dnd5e.ChoiceSelection{}
	}

	// Validate and apply selections
	validationResult, err := o.validateChoiceSelections(ctx, draft, input.Selections)
	if err != nil {
		return nil, errors.Wrap(err, "failed to validate choice selections")
	}

	if !validationResult.IsValid {
		// Convert validation errors to a readable error
		errorMessages := make([]string, len(validationResult.Errors))
		for i, validationErr := range validationResult.Errors {
			errorMessages[i] = validationErr.Message
		}
		return nil, errors.InvalidArgumentf("invalid choice selections: %v", errorMessages)
	}

	// Apply the validated selections to the draft
	err = o.applyChoiceSelections(draft, input.Selections)
	if err != nil {
		return nil, errors.Wrap(err, "failed to apply choice selections")
	}

	// Update progress if all required choices are complete
	if o.areAllChoicesComplete(ctx, draft) {
		draft.Progress.SetStep(dnd5e.ProgressStepChoices, true)
		draft.Progress.CurrentStep = "finalize" // Move to final step
		slog.Info("All choices completed", "draft_id", draft.ID)
	}

	// Calculate completion percentage
	o.updateCompletionPercentage(draft)

	// Convert back to CharacterDraftData for repository update
	draftData := dnd5e.FromCharacterDraft(draft)

	// Save the updated draft
	updateOutput, err := o.characterDraftRepo.Update(ctx, draftrepo.UpdateInput{Draft: draftData})
	if err != nil {
		return nil, errors.Wrap(err, "failed to update draft")
	}

	// Convert result to CharacterDraft for return
	updatedDraft := updateOutput.Draft.ToCharacterDraft()

	slog.Info("Successfully updated character choices", "draft_id", updatedDraft.ID)

	return &UpdateChoicesOutput{
		Draft: updatedDraft,
	}, nil
}

// ListChoiceOptions retrieves available choice options for a character draft
func (o *Orchestrator) ListChoiceOptions(
	ctx context.Context,
	input *ListChoiceOptionsInput,
) (*ListChoiceOptionsOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}

	if input.DraftID == "" {
		return nil, errors.InvalidArgument("draft ID is required")
	}

	slog.Info("Listing choice options", "draft_id", input.DraftID, "choice_type", input.ChoiceType)

	// Get the draft to understand what choices are available
	getDraftInput := &GetDraftInput{DraftID: input.DraftID}
	getDraftOutput, err := o.GetDraft(ctx, getDraftInput)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get draft")
	}

	draft := getDraftOutput.Draft

	// Validate that the draft has required information
	if draft.ClassID == "" {
		return nil, errors.InvalidArgument("class must be selected before viewing choice options")
	}

	// Get available choice categories based on the draft's class
	categories, err := o.getAvailableChoiceCategories(ctx, draft, input.ChoiceType)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get available choice categories")
	}

	// TODO(#82): Implement pagination for choice options
	var totalSize int32
	if len(categories) > math.MaxInt32 {
		totalSize = math.MaxInt32
	} else {
		// nolint:gosec // Choice categories list size won't overflow int32
		totalSize = int32(len(categories))
	}

	return &ListChoiceOptionsOutput{
		Categories:    categories,
		NextPageToken: "", // TODO(#82): Implement pagination for choice options
		TotalSize:     totalSize,
	}, nil
}

// validateChoiceSelections validates that the selections are valid for the draft
func (o *Orchestrator) validateChoiceSelections(
	ctx context.Context,
	draft *dnd5e.CharacterDraft,
	selections []*dnd5e.ChoiceSelection,
) (*dnd5e.ChoiceValidationResult, error) {
	result := &dnd5e.ChoiceValidationResult{
		IsValid:  true,
		Errors:   []dnd5e.ChoiceValidationError{},
		Warnings: []dnd5e.ChoiceValidationWarning{},
	}

	// Get available choice categories to validate against
	availableCategories, err := o.getAvailableChoiceCategories(ctx, draft, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get available choice categories")
	}

	// Create a map for quick category lookup
	categoryMap := make(map[string]*dnd5e.ChoiceCategory)
	for _, category := range availableCategories {
		categoryMap[category.ID] = category
	}

	// Validate each selection
	for _, selection := range selections {
		category, exists := categoryMap[selection.ChoiceID]
		if !exists {
			result.IsValid = false
			result.Errors = append(result.Errors, dnd5e.ChoiceValidationError{
				CategoryID: selection.ChoiceID,
				Message:    fmt.Sprintf("unknown choice category: %s", selection.ChoiceID),
				Code:       "unknown_category",
			})
			continue
		}

		// Validate selection count with bounds checking to prevent DoS attacks
		optionCount := len(selection.SelectedKeys)

		// Protect against malicious input with a practical limit
		if optionCount > dnd5e.MaxChoiceOptionsLimit {
			result.IsValid = false
			result.Errors = append(result.Errors, dnd5e.ChoiceValidationError{
				CategoryID: selection.ChoiceID,
				Message:    fmt.Sprintf("too many options selected (maximum allowed: %d)", dnd5e.MaxChoiceOptionsLimit),
				Code:       "excessive_options",
			})
			continue
		}

		optionCount32 := int32(optionCount)

		if optionCount32 < category.MinChoices {
			result.IsValid = false
			result.Errors = append(result.Errors, dnd5e.ChoiceValidationError{
				CategoryID: selection.ChoiceID,
				Message:    fmt.Sprintf("must select at least %d options", category.MinChoices),
				Code:       "insufficient_choices",
			})
		}

		if optionCount32 > category.MaxChoices {
			result.IsValid = false
			result.Errors = append(result.Errors, dnd5e.ChoiceValidationError{
				CategoryID: selection.ChoiceID,
				Message:    fmt.Sprintf("cannot select more than %d options", category.MaxChoices),
				Code:       "too_many_choices",
			})
		}

		// Validate each option exists and is valid
		optionMap := make(map[string]*dnd5e.CategoryOption)
		for _, option := range category.Options {
			optionMap[option.ID] = option
		}

		for _, optionID := range selection.SelectedKeys {
			option, optionExists := optionMap[optionID]
			if !optionExists {
				result.IsValid = false
				result.Errors = append(result.Errors, dnd5e.ChoiceValidationError{
					CategoryID: selection.ChoiceID,
					OptionID:   optionID,
					Message:    fmt.Sprintf("unknown option: %s", optionID),
					Code:       "unknown_option",
				})
				continue
			}

			// Check prerequisites
			for _, prerequisite := range option.Prerequisites {
				if !o.hasPrerequisite(draft, prerequisite) {
					result.Warnings = append(result.Warnings, dnd5e.ChoiceValidationWarning{
						CategoryID: selection.ChoiceID,
						OptionID:   optionID,
						Message:    fmt.Sprintf("missing prerequisite: %s", prerequisite),
						Code:       "missing_prerequisite",
					})
				}
			}

			// Check conflicts
			for _, conflict := range option.Conflicts {
				if o.hasConflictingChoice(draft, conflict) {
					result.IsValid = false
					result.Errors = append(result.Errors, dnd5e.ChoiceValidationError{
						CategoryID: selection.ChoiceID,
						OptionID:   optionID,
						Message:    fmt.Sprintf("conflicts with existing choice: %s", conflict),
						Code:       "conflicting_choice",
					})
				}
			}
		}
	}

	return result, nil
}

// applyChoiceSelections applies validated selections to the draft
func (o *Orchestrator) applyChoiceSelections(draft *dnd5e.CharacterDraft, selections []*dnd5e.ChoiceSelection) error {
	// Build a map of existing selections for easy lookup
	selectionMap := make(map[string]*dnd5e.ChoiceSelection)
	for i := range draft.ChoiceSelections {
		selectionMap[draft.ChoiceSelections[i].ChoiceID] = &draft.ChoiceSelections[i]
	}

	// Update or add new selections
	for _, selection := range selections {
		// Validate the category ID is known
		switch selection.ChoiceID {
		case dnd5e.CategoryIDFighterFightingStyle,
			dnd5e.CategoryIDWizardCantrips,
			dnd5e.CategoryIDWizardSpells,
			dnd5e.CategoryIDClericCantrips,
			dnd5e.CategoryIDSorcererCantrips,
			dnd5e.CategoryIDSorcererSpells,
			dnd5e.CategoryIDAdditionalLanguages,
			dnd5e.CategoryIDToolProficiencies,
			dnd5e.CategoryIDEquipmentChoices:
			// Valid category
		default:
			return errors.InvalidArgumentf("unknown choice category: %s", selection.ChoiceID)
		}

		if existing, exists := selectionMap[selection.ChoiceID]; exists {
			// Update existing selection
			existing.SelectedKeys = selection.SelectedKeys
		} else {
			// Add new selection
			draft.ChoiceSelections = append(draft.ChoiceSelections, dnd5e.ChoiceSelection{
				ChoiceID:     selection.ChoiceID,
				SelectedKeys: selection.SelectedKeys,
			})
		}
	}
	return nil
}

// getAvailableChoiceCategories returns the choice categories available for a draft
//
//nolint:unparam // error return kept for future extensibility when class/race details are used
func (o *Orchestrator) getAvailableChoiceCategories(
	ctx context.Context,
	draft *dnd5e.CharacterDraft,
	filterType *dnd5e.ChoiceType,
) ([]*dnd5e.ChoiceCategory, error) {
	var categories []*dnd5e.ChoiceCategory

	// Helper function to check if choice type should be included
	shouldIncludeChoiceType := func(choiceType dnd5e.ChoiceType) bool {
		return filterType == nil || *filterType == choiceType
	}

	// Add class-specific choices based on class ID
	switch draft.ClassID {
	case dnd5e.ClassIDFighter:
		if shouldIncludeChoiceType(dnd5e.ChoiceTypeFightingStyle) {
			categories = append(categories, o.createFighterFightingStyleChoices())
		}
	case dnd5e.ClassIDWizard:
		if shouldIncludeChoiceType(dnd5e.ChoiceTypeCantrips) {
			categories = append(categories, o.createWizardCantripChoices(ctx))
		}
		if shouldIncludeChoiceType(dnd5e.ChoiceTypeSpells) {
			categories = append(categories, o.createWizardSpellChoices(ctx))
		}
	case dnd5e.ClassIDCleric:
		if shouldIncludeChoiceType(dnd5e.ChoiceTypeCantrips) {
			categories = append(categories, o.createClericCantripChoices(ctx))
		}
	case dnd5e.ClassIDSorcerer:
		if shouldIncludeChoiceType(dnd5e.ChoiceTypeCantrips) {
			categories = append(categories, o.createSorcererCantripChoices(ctx))
		}
		if shouldIncludeChoiceType(dnd5e.ChoiceTypeSpells) {
			categories = append(categories, o.createSorcererSpellChoices(ctx))
		}
	}

	// Add universal choices (like additional languages, tools)
	// TODO(#82): Add universal language choices based on race/background
	// if shouldIncludeChoiceType(dnd5e.ChoiceTypeLanguages) && draft.RaceID != "" {
	//     languageChoices := o.createLanguageChoices(ctx, draft)
	//     if languageChoices != nil {
	//         categories = append(categories, languageChoices)
	//     }
	// }

	// TODO(#82): Add equipment choices based on class starting equipment options
	// Future implementation:
	// if shouldIncludeChoiceType(dnd5e.ChoiceTypeEquipment) {
	//     classDetails, err := o.GetClassDetails(ctx, &GetClassDetailsInput{ClassID: draft.ClassID})
	//     if err == nil && classDetails.Class.EquipmentChoices != nil {
	//         equipmentChoices := o.createEquipmentChoices(ctx, classDetails.Class.EquipmentChoices)
	//         if equipmentChoices != nil {
	//             categories = append(categories, equipmentChoices)
	//         }
	//     }
	// }

	return categories, nil
}

// areAllChoicesComplete checks if all required choices have been made
func (o *Orchestrator) areAllChoicesComplete(_ context.Context, draft *dnd5e.CharacterDraft) bool {
	// Build a map of selections by choice ID
	selectionMap := make(map[string]*dnd5e.ChoiceSelection)
	for i := range draft.ChoiceSelections {
		selectionMap[draft.ChoiceSelections[i].ChoiceID] = &draft.ChoiceSelections[i]
	}

	// Check class-specific required choices
	switch draft.ClassID {
	case "fighter":
		// Fighter must choose 1 fighting style
		if selection, exists := selectionMap[dnd5e.CategoryIDFighterFightingStyle]; exists {
			return len(selection.SelectedKeys) == 1
		}
		return false
	case "wizard":
		// Wizard must choose 3 cantrips and 6 level 1 spells
		cantripSelection, hasCantrips := selectionMap[dnd5e.CategoryIDWizardCantrips]
		spellSelection, hasSpells := selectionMap[dnd5e.CategoryIDWizardSpells]
		return hasCantrips && hasSpells &&
			len(cantripSelection.SelectedKeys) == 3 &&
			len(spellSelection.SelectedKeys) == 6
	case "cleric":
		// Cleric must choose 3 cantrips
		if selection, exists := selectionMap[dnd5e.CategoryIDClericCantrips]; exists {
			return len(selection.SelectedKeys) == 3
		}
		return false
	case "sorcerer":
		// Sorcerer must choose 4 cantrips and 2 level 1 spells
		cantripSelection, hasCantrips := selectionMap[dnd5e.CategoryIDSorcererCantrips]
		spellSelection, hasSpells := selectionMap[dnd5e.CategoryIDSorcererSpells]
		return hasCantrips && hasSpells &&
			len(cantripSelection.SelectedKeys) == 4 &&
			len(spellSelection.SelectedKeys) == 2
	default:
		// Other classes may not have required choices
		return true
	}
}

// hasPrerequisite checks if a draft meets a prerequisite
func (o *Orchestrator) hasPrerequisite(draft *dnd5e.CharacterDraft, prerequisite string) bool {
	// For now, assume all prerequisites are met
	// TODO(#82): Implement actual prerequisite checking based on:
	// - Ability scores
	// - Race features
	// - Class features
	// - Previously made choices
	_ = draft
	_ = prerequisite
	return true
}

// hasConflictingChoice checks if a draft has a conflicting choice
func (o *Orchestrator) hasConflictingChoice(draft *dnd5e.CharacterDraft, conflict string) bool {
	// Check if the conflict ID exists in any of the current selections
	for _, selection := range draft.ChoiceSelections {
		for _, selectedID := range selection.SelectedKeys {
			if selectedID == conflict {
				return true
			}
		}
	}
	// TODO(#82): Check other choice types as they're added

	return false
}

// spellChoiceCategoryConfig holds configuration for creating spell choice categories
type spellChoiceCategoryConfig struct {
	classID     string
	level       int32
	id          string
	choiceType  dnd5e.ChoiceType
	name        string
	description string
	minChoices  int32
	maxChoices  int32
}

// createSpellChoiceCategory creates a spell choice category with the given configuration
func (o *Orchestrator) createSpellChoiceCategory(
	ctx context.Context, config spellChoiceCategoryConfig,
) *dnd5e.ChoiceCategory {
	// Get spells from the spell list
	spells, err := o.getSpellsByLevelAndClass(ctx, config.level, config.classID)
	if err != nil {
		slog.Error("Failed to get spells", "class", config.classID, "level", config.level, "error", err)
		// Return empty category on error
		return &dnd5e.ChoiceCategory{
			ID:          config.id,
			Type:        config.choiceType,
			Name:        config.name,
			Description: config.description,
			Required:    true,
			MinChoices:  config.minChoices,
			MaxChoices:  config.maxChoices,
			Options:     []*dnd5e.CategoryOption{},
		}
	}

	// Convert spells to choice options
	options := make([]*dnd5e.CategoryOption, len(spells))
	for i, spell := range spells {
		options[i] = &dnd5e.CategoryOption{
			ID:          spell.ID,
			Name:        spell.Name,
			Description: spell.Description,
			Level:       spell.Level,
			School:      spell.School,
			Source:      config.classID,
		}
	}

	return &dnd5e.ChoiceCategory{
		ID:          config.id,
		Type:        config.choiceType,
		Name:        config.name,
		Description: config.description,
		Required:    true,
		MinChoices:  config.minChoices,
		MaxChoices:  config.maxChoices,
		Options:     options,
	}
}

// createFighterFightingStyleChoices creates the fighting style choice category for fighters
func (o *Orchestrator) createFighterFightingStyleChoices() *dnd5e.ChoiceCategory {
	return &dnd5e.ChoiceCategory{
		ID:          dnd5e.CategoryIDFighterFightingStyle,
		Type:        dnd5e.ChoiceTypeFightingStyle,
		Name:        "Fighting Style",
		Description: "Choose a fighting style that represents your specialty in combat.",
		Required:    true,
		MinChoices:  1,
		MaxChoices:  1,
		Options: []*dnd5e.CategoryOption{
			{
				ID:          "archery",
				Name:        "Archery",
				Description: "You gain a +2 bonus to attack rolls you make with ranged weapons.",
				Source:      "fighter",
			},
			{
				ID:          "defense",
				Name:        "Defense",
				Description: "While you are wearing armor, you gain a +1 bonus to AC.",
				Source:      "fighter",
			},
			{
				ID:   "dueling",
				Name: "Dueling",
				Description: "When you are wielding a melee weapon in one hand and no other weapons, " +
					"you gain a +2 bonus to damage rolls with that weapon.",
				Source: "fighter",
			},
			{
				ID:   "great_weapon_fighting",
				Name: "Great Weapon Fighting",
				Description: "When you roll a 1 or 2 on a damage die for an attack you make with a melee weapon " +
					"that you are wielding with two hands, you can reroll the die and must use the new roll.",
				Source: "fighter",
			},
			{
				ID:   "protection",
				Name: "Protection",
				Description: "When a creature you can see attacks a target other than you that is within 5 feet of you, " +
					"you can use your reaction to impose disadvantage on the attack roll. You must be wielding a shield.",
				Prerequisites: []string{"shield_proficiency"},
				Source:        "fighter",
			},
			{
				ID:   "two_weapon_fighting",
				Name: "Two-Weapon Fighting",
				Description: "When you engage in two-weapon fighting, " +
					"you can add your ability modifier to the damage of the second attack.",
				Source: "fighter",
			},
		},
	}
}

// createWizardCantripChoices creates the cantrip choice category for wizards
func (o *Orchestrator) createWizardCantripChoices(ctx context.Context) *dnd5e.ChoiceCategory {
	return o.createSpellChoiceCategory(ctx, spellChoiceCategoryConfig{
		classID:     "wizard",
		level:       0,
		id:          dnd5e.CategoryIDWizardCantrips,
		choiceType:  dnd5e.ChoiceTypeCantrips,
		name:        "Wizard Cantrips",
		description: "Choose 3 cantrips from the wizard spell list.",
		minChoices:  3,
		maxChoices:  3,
	})
}

// createWizardSpellChoices creates the spell choice category for wizards
func (o *Orchestrator) createWizardSpellChoices(ctx context.Context) *dnd5e.ChoiceCategory {
	return o.createSpellChoiceCategory(ctx, spellChoiceCategoryConfig{
		classID:     "wizard",
		level:       1,
		id:          dnd5e.CategoryIDWizardSpells,
		choiceType:  dnd5e.ChoiceTypeSpells,
		name:        "1st Level Wizard Spells",
		description: "Choose 6 first-level spells from the wizard spell list.",
		minChoices:  6,
		maxChoices:  6,
	})
}

// createClericCantripChoices creates the cantrip choice category for clerics
func (o *Orchestrator) createClericCantripChoices(ctx context.Context) *dnd5e.ChoiceCategory {
	return o.createSpellChoiceCategory(ctx, spellChoiceCategoryConfig{
		classID:     "cleric",
		level:       0,
		id:          dnd5e.CategoryIDClericCantrips,
		choiceType:  dnd5e.ChoiceTypeCantrips,
		name:        "Cleric Cantrips",
		description: "Choose 3 cantrips from the cleric spell list.",
		minChoices:  3,
		maxChoices:  3,
	})
}

// createSorcererCantripChoices creates the cantrip choice category for sorcerers
func (o *Orchestrator) createSorcererCantripChoices(ctx context.Context) *dnd5e.ChoiceCategory {
	return o.createSpellChoiceCategory(ctx, spellChoiceCategoryConfig{
		classID:     "sorcerer",
		level:       0,
		id:          dnd5e.CategoryIDSorcererCantrips,
		choiceType:  dnd5e.ChoiceTypeCantrips,
		name:        "Sorcerer Cantrips",
		description: "Choose 4 cantrips from the sorcerer spell list.",
		minChoices:  4,
		maxChoices:  4,
	})
}

// createSorcererSpellChoices creates the spell choice category for sorcerers
func (o *Orchestrator) createSorcererSpellChoices(ctx context.Context) *dnd5e.ChoiceCategory {
	return o.createSpellChoiceCategory(ctx, spellChoiceCategoryConfig{
		classID:     "sorcerer",
		level:       1,
		id:          dnd5e.CategoryIDSorcererSpells,
		choiceType:  dnd5e.ChoiceTypeSpells,
		name:        "1st Level Sorcerer Spells",
		description: "Choose 2 first-level spells from the sorcerer spell list.",
		minChoices:  2,
		maxChoices:  2,
	})
}

// getSpellsByLevelAndClass gets spells filtered by level and class
func (o *Orchestrator) getSpellsByLevelAndClass(
	ctx context.Context, level int32, classID string,
) ([]*dnd5e.SpellInfo, error) {
	input := &ListSpellsInput{
		Level:    &level,
		ClassID:  classID,
		PageSize: dnd5e.DefaultSpellPageSize,
	}

	output, err := o.ListSpells(ctx, input)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list spells")
	}

	return output.Spells, nil
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

// ListEquipmentByType returns equipment filtered by type
func (o *Orchestrator) ListEquipmentByType(
	ctx context.Context,
	input *ListEquipmentByTypeInput,
) (*ListEquipmentByTypeOutput, error) {
	// Validate input
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}
	if input.EquipmentType == "" {
		return nil, errors.InvalidArgument("equipment type is required")
	}

	// Log the request for observability
	slog.InfoContext(ctx, "Listing equipment by type",
		slog.String("equipment_type", input.EquipmentType),
		slog.Int("page_size", int(input.PageSize)),
	)

	// Fetch equipment from external client
	equipmentData, err := o.externalClient.ListEquipmentByCategory(ctx, input.EquipmentType)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to list equipment by category",
			slog.String("equipment_type", input.EquipmentType),
			slog.String("error", err.Error()),
		)
		return nil, errors.Internal("failed to fetch equipment data")
	}

	// Convert external data to internal entities
	equipmentList := make([]*dnd5e.EquipmentInfo, 0, len(equipmentData))
	for _, equipment := range equipmentData {
		equipmentInfo := convertEquipmentDataToEntity(equipment)
		equipmentList = append(equipmentList, equipmentInfo)
	}

	// Apply pagination (simple in-memory pagination for now)
	pageSize := input.PageSize
	if pageSize == 0 {
		pageSize = 20 // Default page size
	}

	totalSize := int32(len(equipmentList)) // nolint:gosec
	startIndex := int32(0)
	nextPageToken := ""

	// Parse page token if provided
	if input.PageToken != "" {
		if parsed, err := strconv.ParseInt(input.PageToken, 10, 32); err == nil {
			startIndex = int32(parsed)
		}
	}

	// Calculate end index and next page token
	endIndex := startIndex + pageSize
	if endIndex > totalSize {
		endIndex = totalSize
	}
	if endIndex < totalSize {
		nextPageToken = strconv.FormatInt(int64(endIndex), 10)
	}

	// Get the page slice
	var pagedEquipment []*dnd5e.EquipmentInfo
	if startIndex < totalSize {
		pagedEquipment = equipmentList[startIndex:endIndex]
	}

	return &ListEquipmentByTypeOutput{
		Equipment:     pagedEquipment,
		NextPageToken: nextPageToken,
		TotalSize:     totalSize,
	}, nil
}

// ListSpellsByLevel returns spells filtered by level
func (o *Orchestrator) ListSpellsByLevel(
	ctx context.Context,
	input *ListSpellsByLevelInput,
) (*ListSpellsByLevelOutput, error) {
	// Validate input
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}
	if input.Level < 0 || input.Level > 9 {
		return nil, errors.InvalidArgument("spell level must be between 0 and 9")
	}

	// Log the request for observability
	slog.InfoContext(ctx, "Listing spells by level",
		slog.Int("level", int(input.Level)),
		slog.String("class_id", input.ClassID),
		slog.Int("page_size", int(input.PageSize)),
	)

	// Prepare external client input
	externalInput := &external.ListSpellsInput{
		Level: &input.Level,
	}

	// Convert internal class ID to external format if provided
	if input.ClassID != "" {
		externalInput.ClassID = convertClassIDToExternalFormat(input.ClassID)
	}

	// Fetch spells from external client
	spellData, err := o.externalClient.ListAvailableSpells(ctx, externalInput)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to list spells",
			slog.Int("level", int(input.Level)),
			slog.String("class_id", input.ClassID),
			slog.String("error", err.Error()),
		)
		return nil, errors.Internal("failed to fetch spell data")
	}

	// Convert external data to internal entities
	spellList := make([]*dnd5e.SpellInfo, 0, len(spellData))
	for _, spell := range spellData {
		spellInfo := convertSpellDataToEntity(spell)
		spellList = append(spellList, spellInfo)
	}

	// Apply pagination (simple in-memory pagination for now)
	pageSize := input.PageSize
	if pageSize == 0 {
		pageSize = 20 // Default page size
	}

	totalSize := int32(len(spellList)) // nolint:gosec
	startIndex := int32(0)
	nextPageToken := ""

	// Parse page token if provided
	if input.PageToken != "" {
		if parsed, err := strconv.ParseInt(input.PageToken, 10, 32); err == nil {
			startIndex = int32(parsed)
		}
	}

	// Calculate end index and next page token
	endIndex := startIndex + pageSize
	if endIndex > totalSize {
		endIndex = totalSize
	}
	if endIndex < totalSize {
		nextPageToken = strconv.FormatInt(int64(endIndex), 10)
	}

	// Get the page slice
	var pagedSpells []*dnd5e.SpellInfo
	if startIndex < totalSize {
		pagedSpells = spellList[startIndex:endIndex]
	}

	return &ListSpellsByLevelOutput{
		Spells:        pagedSpells,
		NextPageToken: nextPageToken,
		TotalSize:     totalSize,
	}, nil
}

// Conversion functions for external data to internal entities

// convertEquipmentDataToEntity converts external EquipmentData to internal EquipmentInfo
func convertEquipmentDataToEntity(equipment *external.EquipmentData) *dnd5e.EquipmentInfo {
	if equipment == nil {
		return nil
	}

	equipmentInfo := &dnd5e.EquipmentInfo{
		ID:          equipment.ID,
		Name:        equipment.Name,
		Type:        equipment.EquipmentType,
		Category:    equipment.Category,
		Description: equipment.Description,
		Properties:  equipment.Properties,
	}

	// Convert cost
	if equipment.Cost != nil {
		equipmentInfo.Cost = fmt.Sprintf("%d %s", equipment.Cost.Quantity, equipment.Cost.Unit)
	}

	// Convert weight
	if equipment.Weight > 0 {
		if equipment.Weight == float32(int(equipment.Weight)) {
			equipmentInfo.Weight = fmt.Sprintf("%d lbs", int(equipment.Weight))
		} else {
			equipmentInfo.Weight = fmt.Sprintf("%.1f lbs", equipment.Weight)
		}
	}

	return equipmentInfo
}

// convertSpellDataToEntity converts external SpellData to internal SpellInfo
func convertSpellDataToEntity(spell *external.SpellData) *dnd5e.SpellInfo {
	if spell == nil {
		return nil
	}

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
		Classes:     []string{}, // TODO: Add class filtering in external client
	}
}

// convertClassIDToExternalFormat converts internal class ID to external format
func convertClassIDToExternalFormat(classID string) string {
	// Convert from internal format (e.g., "CLASS_WIZARD") to external format (e.g., "wizard")
	// First, try to find a direct mapping
	switch classID {
	case dnd5e.ClassBarbarian:
		return "barbarian"
	case dnd5e.ClassBard:
		return "bard"
	case dnd5e.ClassCleric:
		return "cleric"
	case dnd5e.ClassDruid:
		return "druid"
	case dnd5e.ClassFighter:
		return "fighter"
	case dnd5e.ClassMonk:
		return "monk"
	case dnd5e.ClassPaladin:
		return "paladin"
	case dnd5e.ClassRanger:
		return "ranger"
	case dnd5e.ClassRogue:
		return "rogue"
	case dnd5e.ClassSorcerer:
		return "sorcerer"
	case dnd5e.ClassWarlock:
		return "warlock"
	case dnd5e.ClassWizard:
		return "wizard"
	default:
		// If no direct mapping, try to convert from CLASS_X format to lowercase
		if strings.HasPrefix(classID, "CLASS_") {
			return strings.ToLower(strings.TrimPrefix(classID, "CLASS_"))
		}
		return strings.ToLower(classID)
	}
}

// convertExternalTraitsToEntity converts external trait data to entity format
func convertExternalTraitsToEntity(traits []external.TraitData) []dnd5e.RacialTrait {
	if traits == nil {
		return nil
	}

	entityTraits := make([]dnd5e.RacialTrait, len(traits))
	for i, trait := range traits {
		entityTraits[i] = dnd5e.RacialTrait{
			Name:        trait.Name,
			Description: trait.Description,
			IsChoice:    trait.IsChoice,
			Options:     trait.Options,
		}
	}
	return entityTraits
}

// hydrateDraft populates the draft with full Info objects for race, class, and background
func (o *Orchestrator) hydrateDraft(ctx context.Context, draft *dnd5e.CharacterDraft) (*dnd5e.CharacterDraft, error) {
	// Create a copy of the draft to avoid modifying the original
	hydratedDraft := *draft
	
	// Ensure Info fields are nil to start (in case draft already had them)
	hydratedDraft.Race = nil
	hydratedDraft.Subrace = nil
	hydratedDraft.Class = nil
	hydratedDraft.Background = nil

	// Fetch race info if race is set
	if draft.RaceID != "" {
		raceData, err := o.externalClient.GetRaceData(ctx, draft.RaceID)
		if err != nil {
			// Log error but continue - we don't want to fail the whole hydration for one missing piece
			// TODO(#46): Add proper logging
		} else {
			hydratedDraft.Race = convertExternalRaceToEntity(raceData)

			// If subrace is set, find it in the race data
			if draft.SubraceID != "" && raceData != nil {
				for _, subrace := range raceData.Subraces {
					if subrace.ID == draft.SubraceID {
						hydratedDraft.Subrace = &dnd5e.SubraceInfo{
							ID:             subrace.ID,
							Name:           subrace.Name,
							Description:    subrace.Description,
							AbilityBonuses: subrace.AbilityBonuses,
							Traits:         convertExternalTraitsToEntity(subrace.Traits),
						}
						break
					}
				}
			}
		}
	}

	// Fetch class info if class is set
	if draft.ClassID != "" {
		classData, err := o.externalClient.GetClassData(ctx, draft.ClassID)
		if err != nil {
			// Log error but continue
			// TODO(#46): Add proper logging
		} else {
			hydratedDraft.Class = convertExternalClassToEntity(classData)
		}
	}

	// Fetch background info if background is set
	if draft.BackgroundID != "" {
		backgroundData, err := o.externalClient.GetBackgroundData(ctx, draft.BackgroundID)
		if err != nil {
			// Log error but continue
			// TODO(#46): Add proper logging
		} else {
			hydratedDraft.Background = convertExternalBackgroundToEntity(backgroundData)
		}
	}

	return &hydratedDraft, nil
}
