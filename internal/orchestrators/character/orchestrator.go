package character

import (
	"context"
	"fmt"
	"time"

	"github.com/KirkDiggler/rpg-api/internal/clients/external"
	"github.com/KirkDiggler/rpg-api/internal/engine"
	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
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
	if c.CharacterRepo == nil {
		return fmt.Errorf("character repository is required")
	}
	if c.CharacterDraftRepo == nil {
		return fmt.Errorf("character draft repository is required")
	}
	if c.Engine == nil {
		return fmt.Errorf("engine is required")
	}
	if c.ExternalClient == nil {
		return fmt.Errorf("external client is required")
	}
	return nil
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
		return nil, fmt.Errorf("invalid config: %w", err)
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
func (o *Orchestrator) CreateDraft(ctx context.Context, input *character.CreateDraftInput) (*character.CreateDraftOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if input.PlayerID == "" {
		return nil, fmt.Errorf("player ID is required")
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
			draft.Progress.CurrentStep = dnd5e.CreationStepRace
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

		// Update completion percentage and current step
		draft.Progress = o.calculateProgress(draft)
	}

	// Save to repository
	err := o.characterDraftRepo.Create(ctx, draft)
	if err != nil {
		return nil, fmt.Errorf("failed to create draft: %w", err)
	}

	return &character.CreateDraftOutput{
		Draft: draft,
	}, nil
}

// GetDraft retrieves a character draft by ID
func (o *Orchestrator) GetDraft(ctx context.Context, input *character.GetDraftInput) (*character.GetDraftOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if input.DraftID == "" {
		return nil, fmt.Errorf("draft ID is required")
	}

	draft, err := o.characterDraftRepo.Get(ctx, input.DraftID)
	if err != nil {
		return nil, fmt.Errorf("failed to get draft: %w", err)
	}

	return &character.GetDraftOutput{
		Draft: draft,
	}, nil
}

// ListDrafts lists character drafts with pagination
func (o *Orchestrator) ListDrafts(ctx context.Context, input *character.ListDraftsInput) (*character.ListDraftsOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}

	// Build repository options
	opts := draftrepo.ListOptions{
		PageSize:  input.PageSize,
		PageToken: input.PageToken,
		PlayerID:  input.PlayerID,
		SessionID: input.SessionID,
	}

	// Default page size if not specified
	if opts.PageSize <= 0 {
		opts.PageSize = 20
	}

	result, err := o.characterDraftRepo.List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list drafts: %w", err)
	}

	return &character.ListDraftsOutput{
		Drafts:        result.Drafts,
		NextPageToken: result.NextPageToken,
	}, nil
}

// DeleteDraft deletes a character draft
func (o *Orchestrator) DeleteDraft(ctx context.Context, input *character.DeleteDraftInput) (*character.DeleteDraftOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if input.DraftID == "" {
		return nil, fmt.Errorf("draft ID is required")
	}

	// TODO: Consider checking if draft exists first for better error messages

	err := o.characterDraftRepo.Delete(ctx, input.DraftID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete draft: %w", err)
	}

	return &character.DeleteDraftOutput{
		Message: fmt.Sprintf("Draft %s deleted successfully", input.DraftID),
	}, nil
}

// Section update methods

// UpdateName updates the character's name
func (o *Orchestrator) UpdateName(ctx context.Context, input *character.UpdateNameInput) (*character.UpdateNameOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if input.DraftID == "" {
		return nil, fmt.Errorf("draft ID is required")
	}
	if input.Name == "" {
		return nil, fmt.Errorf("name is required")
	}

	// Get existing draft
	draft, err := o.characterDraftRepo.Get(ctx, input.DraftID)
	if err != nil {
		return nil, fmt.Errorf("failed to get draft: %w", err)
	}

	// Update name
	draft.Name = input.Name
	draft.Progress.SetStep(dnd5e.ProgressStepName, true)
	draft.UpdatedAt = time.Now().Unix()

	// Recalculate progress
	draft.Progress = o.calculateProgress(draft)

	// Save updated draft
	err = o.characterDraftRepo.Update(ctx, draft)
	if err != nil {
		return nil, fmt.Errorf("failed to update draft: %w", err)
	}

	// No validation warnings for name update
	return &character.UpdateNameOutput{
		Draft:    draft,
		Warnings: []character.ValidationWarning{},
	}, nil
}

// UpdateRace updates the character's race
func (o *Orchestrator) UpdateRace(ctx context.Context, input *character.UpdateRaceInput) (*character.UpdateRaceOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if input.DraftID == "" {
		return nil, fmt.Errorf("draft ID is required")
	}
	if input.RaceID == "" {
		return nil, fmt.Errorf("race ID is required")
	}

	// Get existing draft
	draft, err := o.characterDraftRepo.Get(ctx, input.DraftID)
	if err != nil {
		return nil, fmt.Errorf("failed to get draft: %w", err)
	}

	// Validate race choice with engine
	validateRaceInput := &engine.ValidateRaceChoiceInput{
		RaceID:    input.RaceID,
		SubraceID: input.SubraceID,
	}

	validateRaceOutput, err := o.engine.ValidateRaceChoice(ctx, validateRaceInput)
	if err != nil {
		return nil, fmt.Errorf("failed to validate race: %w", err)
	}

	if !validateRaceOutput.IsValid {
		// Convert engine errors to service errors
		warnings := make([]character.ValidationWarning, 0, len(validateRaceOutput.Errors))
		for _, err := range validateRaceOutput.Errors {
			warnings = append(warnings, character.ValidationWarning{
				Field:   err.Field,
				Message: err.Message,
				Type:    err.Code,
			})
		}
		return &character.UpdateRaceOutput{
			Draft:    draft,
			Warnings: warnings,
		}, nil
	}

	// Update race
	draft.RaceID = input.RaceID
	draft.SubraceID = input.SubraceID
	draft.Progress.SetStep(dnd5e.ProgressStepRace, true)
	draft.UpdatedAt = time.Now().Unix()

	// TODO: Apply racial ability modifiers when we have ability scores
	// This would involve modifying base scores with validateRaceOutput.AbilityMods

	// Recalculate progress
	draft.Progress = o.calculateProgress(draft)

	// Save updated draft
	err = o.characterDraftRepo.Update(ctx, draft)
	if err != nil {
		return nil, fmt.Errorf("failed to update draft: %w", err)
	}

	return &character.UpdateRaceOutput{
		Draft:    draft,
		Warnings: []character.ValidationWarning{},
	}, nil
}

// UpdateClass updates the character's class
func (o *Orchestrator) UpdateClass(ctx context.Context, input *character.UpdateClassInput) (*character.UpdateClassOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if input.DraftID == "" {
		return nil, fmt.Errorf("draft ID is required")
	}
	if input.ClassID == "" {
		return nil, fmt.Errorf("class ID is required")
	}

	// Get existing draft
	draft, err := o.characterDraftRepo.Get(ctx, input.DraftID)
	if err != nil {
		return nil, fmt.Errorf("failed to get draft: %w", err)
	}

	// Validate class choice with engine (requires ability scores if set)
	validateClassInput := &engine.ValidateClassChoiceInput{
		ClassID:       input.ClassID,
		AbilityScores: draft.AbilityScores,
	}

	validateClassOutput, err := o.engine.ValidateClassChoice(ctx, validateClassInput)
	if err != nil {
		return nil, fmt.Errorf("failed to validate class: %w", err)
	}

	warnings := make([]character.ValidationWarning, 0)

	// Convert validation errors to warnings
	if !validateClassOutput.IsValid {
		for _, err := range validateClassOutput.Errors {
			warnings = append(warnings, character.ValidationWarning{
				Field:   err.Field,
				Message: err.Message,
				Type:    err.Code,
			})
		}
	}

	// Also include any non-blocking warnings
	for _, warn := range validateClassOutput.Warnings {
		warnings = append(warnings, character.ValidationWarning{
			Field:   warn.Field,
			Message: warn.Message,
			Type:    warn.Code,
		})
	}

	// Update class even if there are warnings (user might fix ability scores later)
	draft.ClassID = input.ClassID
	draft.Progress.SetStep(dnd5e.ProgressStepClass, true)
	draft.UpdatedAt = time.Now().Unix()

	// Clear skills if class changed (they need to reselect based on new class)
	if len(draft.StartingSkillIDs) > 0 {
		draft.StartingSkillIDs = []string{}
		draft.Progress.SetStep(dnd5e.ProgressStepSkills, false)
	}

	// Recalculate progress
	draft.Progress = o.calculateProgress(draft)

	// Save updated draft
	err = o.characterDraftRepo.Update(ctx, draft)
	if err != nil {
		return nil, fmt.Errorf("failed to update draft: %w", err)
	}

	return &character.UpdateClassOutput{
		Draft:    draft,
		Warnings: warnings,
	}, nil
}

// UpdateBackground updates the character's background
func (o *Orchestrator) UpdateBackground(ctx context.Context, input *character.UpdateBackgroundInput) (*character.UpdateBackgroundOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if input.DraftID == "" {
		return nil, fmt.Errorf("draft ID is required")
	}
	if input.BackgroundID == "" {
		return nil, fmt.Errorf("background ID is required")
	}

	// Get existing draft
	draft, err := o.characterDraftRepo.Get(ctx, input.DraftID)
	if err != nil {
		return nil, fmt.Errorf("failed to get draft: %w", err)
	}

	// Validate background choice with engine
	validateBgInput := &engine.ValidateBackgroundChoiceInput{
		BackgroundID: input.BackgroundID,
	}

	validateBgOutput, err := o.engine.ValidateBackgroundChoice(ctx, validateBgInput)
	if err != nil {
		return nil, fmt.Errorf("failed to validate background: %w", err)
	}

	if !validateBgOutput.IsValid {
		// Convert engine errors to service warnings
		warnings := make([]character.ValidationWarning, 0, len(validateBgOutput.Errors))
		for _, err := range validateBgOutput.Errors {
			warnings = append(warnings, character.ValidationWarning{
				Field:   err.Field,
				Message: err.Message,
				Type:    err.Code,
			})
		}
		return &character.UpdateBackgroundOutput{
			Draft:    draft,
			Warnings: warnings,
		}, nil
	}

	// Update background
	draft.BackgroundID = input.BackgroundID
	draft.Progress.SetStep(dnd5e.ProgressStepBackground, true)
	draft.UpdatedAt = time.Now().Unix()

	// Clear skills if background changed (they get skills from background)
	if len(draft.StartingSkillIDs) > 0 {
		draft.StartingSkillIDs = []string{}
		draft.Progress.SetStep(dnd5e.ProgressStepSkills, false)
	}

	// Recalculate progress
	draft.Progress = o.calculateProgress(draft)

	// Save updated draft
	err = o.characterDraftRepo.Update(ctx, draft)
	if err != nil {
		return nil, fmt.Errorf("failed to update draft: %w", err)
	}

	return &character.UpdateBackgroundOutput{
		Draft:    draft,
		Warnings: []character.ValidationWarning{},
	}, nil
}

// UpdateAbilityScores updates the character's ability scores
func (o *Orchestrator) UpdateAbilityScores(ctx context.Context, input *character.UpdateAbilityScoresInput) (*character.UpdateAbilityScoresOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if input.DraftID == "" {
		return nil, fmt.Errorf("draft ID is required")
	}

	// Get existing draft
	draft, err := o.characterDraftRepo.Get(ctx, input.DraftID)
	if err != nil {
		return nil, fmt.Errorf("failed to get draft: %w", err)
	}

	// Validate ability scores with engine
	validateScoresInput := &engine.ValidateAbilityScoresInput{
		AbilityScores: &input.AbilityScores,
		Method:        "manual", // TODO: Support different methods (standard array, point buy)
	}

	validateScoresOutput, err := o.engine.ValidateAbilityScores(ctx, validateScoresInput)
	if err != nil {
		return nil, fmt.Errorf("failed to validate ability scores: %w", err)
	}

	warnings := make([]character.ValidationWarning, 0)

	// Convert validation errors to warnings
	if !validateScoresOutput.IsValid {
		for _, err := range validateScoresOutput.Errors {
			warnings = append(warnings, character.ValidationWarning{
				Field:   err.Field,
				Message: err.Message,
				Type:    err.Code,
			})
		}
	}

	// Include any non-blocking warnings
	for _, warn := range validateScoresOutput.Warnings {
		warnings = append(warnings, character.ValidationWarning{
			Field:   warn.Field,
			Message: warn.Message,
			Type:    warn.Code,
		})
	}

	// Update ability scores
	draft.AbilityScores = &input.AbilityScores
	draft.Progress.SetStep(dnd5e.ProgressStepAbilityScores, true)
	draft.UpdatedAt = time.Now().Unix()

	// If we have a class, revalidate it with new ability scores
	if draft.ClassID != "" {
		validateClassInput := &engine.ValidateClassChoiceInput{
			ClassID:       draft.ClassID,
			AbilityScores: draft.AbilityScores,
		}

		validateClassOutput, err := o.engine.ValidateClassChoice(ctx, validateClassInput)
		if err == nil && !validateClassOutput.IsValid {
			// Add class-related warnings
			for _, err := range validateClassOutput.Errors {
				warnings = append(warnings, character.ValidationWarning{
					Field:   "class_requirements",
					Message: err.Message,
					Type:    err.Code,
				})
			}
		}
	}

	// Recalculate progress
	draft.Progress = o.calculateProgress(draft)

	// Save updated draft
	err = o.characterDraftRepo.Update(ctx, draft)
	if err != nil {
		return nil, fmt.Errorf("failed to update draft: %w", err)
	}

	return &character.UpdateAbilityScoresOutput{
		Draft:    draft,
		Warnings: warnings,
	}, nil
}

// UpdateSkills updates the character's skills
func (o *Orchestrator) UpdateSkills(ctx context.Context, input *character.UpdateSkillsInput) (*character.UpdateSkillsOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if input.DraftID == "" {
		return nil, fmt.Errorf("draft ID is required")
	}

	// Get existing draft
	draft, err := o.characterDraftRepo.Get(ctx, input.DraftID)
	if err != nil {
		return nil, fmt.Errorf("failed to get draft: %w", err)
	}

	// Validate we have class and background before selecting skills
	if draft.ClassID == "" || draft.BackgroundID == "" {
		return &character.UpdateSkillsOutput{
			Draft: draft,
			Warnings: []character.ValidationWarning{
				{
					Field:   "prerequisites",
					Message: "Must select class and background before choosing skills",
					Type:    "MISSING_PREREQUISITES",
				},
			},
		}, nil
	}

	// Validate skill choices with engine
	validateSkillsInput := &engine.ValidateSkillChoicesInput{
		ClassID:          draft.ClassID,
		BackgroundID:     draft.BackgroundID,
		SelectedSkillIDs: input.SkillIDs,
	}

	validateSkillsOutput, err := o.engine.ValidateSkillChoices(ctx, validateSkillsInput)
	if err != nil {
		return nil, fmt.Errorf("failed to validate skills: %w", err)
	}

	warnings := make([]character.ValidationWarning, 0)

	if !validateSkillsOutput.IsValid {
		for _, err := range validateSkillsOutput.Errors {
			warnings = append(warnings, character.ValidationWarning{
				Field:   err.Field,
				Message: err.Message,
				Type:    err.Code,
			})
		}
		return &character.UpdateSkillsOutput{
			Draft:    draft,
			Warnings: warnings,
		}, nil
	}

	// Update skills
	draft.StartingSkillIDs = input.SkillIDs
	draft.Progress.SetStep(dnd5e.ProgressStepSkills, len(input.SkillIDs) > 0)
	draft.UpdatedAt = time.Now().Unix()

	// Recalculate progress
	draft.Progress = o.calculateProgress(draft)

	// Save updated draft
	err = o.characterDraftRepo.Update(ctx, draft)
	if err != nil {
		return nil, fmt.Errorf("failed to update draft: %w", err)
	}

	return &character.UpdateSkillsOutput{
		Draft:    draft,
		Warnings: warnings,
	}, nil
}

// Validation methods

// ValidateDraft validates a character draft
func (o *Orchestrator) ValidateDraft(ctx context.Context, input *character.ValidateDraftInput) (*character.ValidateDraftOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if input.DraftID == "" {
		return nil, fmt.Errorf("draft ID is required")
	}

	// Get existing draft
	draft, err := o.characterDraftRepo.Get(ctx, input.DraftID)
	if err != nil {
		return nil, fmt.Errorf("failed to get draft: %w", err)
	}

	// Use engine to validate the entire draft
	validateDraftInput := &engine.ValidateCharacterDraftInput{
		Draft: draft,
	}

	validateDraftOutput, err := o.engine.ValidateCharacterDraft(ctx, validateDraftInput)
	if err != nil {
		return nil, fmt.Errorf("failed to validate draft: %w", err)
	}

	// Convert engine validation results to service types
	errors := make([]character.ValidationError, 0, len(validateDraftOutput.Errors))
	for _, err := range validateDraftOutput.Errors {
		errors = append(errors, character.ValidationError{
			Field:   err.Field,
			Message: err.Message,
			Type:    err.Code,
		})
	}

	warnings := make([]character.ValidationWarning, 0, len(validateDraftOutput.Warnings))
	for _, warn := range validateDraftOutput.Warnings {
		warnings = append(warnings, character.ValidationWarning{
			Field:   warn.Field,
			Message: warn.Message,
			Type:    warn.Code,
		})
	}

	return &character.ValidateDraftOutput{
		IsComplete:   validateDraftOutput.IsComplete,
		IsValid:      validateDraftOutput.IsValid,
		Errors:       errors,
		Warnings:     warnings,
		MissingSteps: validateDraftOutput.MissingSteps,
	}, nil
}

// Finalization methods

// FinalizeDraft finalizes a draft into a complete character
func (o *Orchestrator) FinalizeDraft(ctx context.Context, input *character.FinalizeDraftInput) (*character.FinalizeDraftOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if input.DraftID == "" {
		return nil, fmt.Errorf("draft ID is required")
	}

	// Get existing draft
	draft, err := o.characterDraftRepo.Get(ctx, input.DraftID)
	if err != nil {
		return nil, fmt.Errorf("failed to get draft: %w", err)
	}

	// Validate draft is complete and valid
	validateDraftInput := &engine.ValidateCharacterDraftInput{
		Draft: draft,
	}

	validateDraftOutput, err := o.engine.ValidateCharacterDraft(ctx, validateDraftInput)
	if err != nil {
		return nil, fmt.Errorf("failed to validate draft: %w", err)
	}

	if !validateDraftOutput.IsComplete {
		return nil, fmt.Errorf("cannot finalize incomplete draft, missing steps: %v", validateDraftOutput.MissingSteps)
	}

	if !validateDraftOutput.IsValid {
		errMsgs := make([]string, 0, len(validateDraftOutput.Errors))
		for _, err := range validateDraftOutput.Errors {
			errMsgs = append(errMsgs, err.Message)
		}
		return nil, fmt.Errorf("cannot finalize invalid draft: %v", errMsgs)
	}

	// Calculate initial character stats
	calcStatsInput := &engine.CalculateCharacterStatsInput{
		Draft: draft,
	}

	calcStatsOutput, err := o.engine.CalculateCharacterStats(ctx, calcStatsInput)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate character stats: %w", err)
	}

	// Create finalized character
	finalChar := &dnd5e.Character{
		Name:             draft.Name,
		Level:            1, // All characters start at level 1
		ExperiencePoints: 0,
		RaceID:           draft.RaceID,
		SubraceID:        draft.SubraceID,
		ClassID:          draft.ClassID,
		BackgroundID:     draft.BackgroundID,
		Alignment:        draft.Alignment,
		AbilityScores:    *draft.AbilityScores,
		CurrentHP:        calcStatsOutput.MaxHP,
		TempHP:           0,
		SessionID:        draft.SessionID,
		PlayerID:         draft.PlayerID,
		CreatedAt:        time.Now().Unix(),
		UpdatedAt:        time.Now().Unix(),
	}

	// Save character to repository
	err = o.characterRepo.Create(ctx, finalChar)
	if err != nil {
		return nil, fmt.Errorf("failed to create character: %w", err)
	}

	// Delete the draft
	err = o.characterDraftRepo.Delete(ctx, draft.ID)
	if err != nil {
		// Log error but don't fail - character was created successfully
		// TODO: Add logging
	}

	return &character.FinalizeDraftOutput{
		Character:    finalChar,
		DraftDeleted: err == nil,
	}, nil
}

// Character operation methods

// GetCharacter retrieves a finalized character
func (o *Orchestrator) GetCharacter(ctx context.Context, input *character.GetCharacterInput) (*character.GetCharacterOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if input.CharacterID == "" {
		return nil, fmt.Errorf("character ID is required")
	}

	char, err := o.characterRepo.Get(ctx, input.CharacterID)
	if err != nil {
		return nil, fmt.Errorf("failed to get character: %w", err)
	}

	return &character.GetCharacterOutput{
		Character: char,
	}, nil
}

// ListCharacters lists finalized characters with pagination
func (o *Orchestrator) ListCharacters(ctx context.Context, input *character.ListCharactersInput) (*character.ListCharactersOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}

	// Build repository options
	opts := characterrepo.ListOptions{
		PageSize:  input.PageSize,
		PageToken: input.PageToken,
		PlayerID:  input.PlayerID,
		SessionID: input.SessionID,
	}

	// Default page size if not specified
	if opts.PageSize <= 0 {
		opts.PageSize = 20
	}

	result, err := o.characterRepo.List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list characters: %w", err)
	}

	return &character.ListCharactersOutput{
		Characters:    result.Characters,
		NextPageToken: result.NextPageToken,
		TotalSize:     result.TotalSize,
	}, nil
}

// DeleteCharacter deletes a finalized character
func (o *Orchestrator) DeleteCharacter(ctx context.Context, input *character.DeleteCharacterInput) (*character.DeleteCharacterOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if input.CharacterID == "" {
		return nil, fmt.Errorf("character ID is required")
	}

	err := o.characterRepo.Delete(ctx, input.CharacterID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete character: %w", err)
	}

	return &character.DeleteCharacterOutput{
		Message: fmt.Sprintf("Character %s deleted successfully", input.CharacterID),
	}, nil
}

// Helper methods

// calculateProgress determines completion percentage and next step
func (o *Orchestrator) calculateProgress(draft *dnd5e.CharacterDraft) dnd5e.CreationProgress {
	progress := draft.Progress

	// Count completed steps using bit manipulation
	completedSteps := 0
	totalSteps := 7 // name, race, class, background, ability scores, skills, languages

	// Count set bits in StepsCompleted
	steps := progress.StepsCompleted
	for steps > 0 {
		if steps&1 == 1 {
			completedSteps++
		}
		steps >>= 1
	}

	progress.CompletionPercentage = int32((completedSteps * 100) / totalSteps)

	// Determine next step by checking each flag in order
	if !progress.HasStep(dnd5e.ProgressStepName) {
		progress.CurrentStep = dnd5e.CreationStepName
	} else if !progress.HasStep(dnd5e.ProgressStepRace) {
		progress.CurrentStep = dnd5e.CreationStepRace
	} else if !progress.HasStep(dnd5e.ProgressStepClass) {
		progress.CurrentStep = dnd5e.CreationStepClass
	} else if !progress.HasStep(dnd5e.ProgressStepBackground) {
		progress.CurrentStep = dnd5e.CreationStepBackground
	} else if !progress.HasStep(dnd5e.ProgressStepAbilityScores) {
		progress.CurrentStep = dnd5e.CreationStepAbilityScores
	} else if !progress.HasStep(dnd5e.ProgressStepSkills) {
		progress.CurrentStep = dnd5e.CreationStepSkills
	} else if !progress.HasStep(dnd5e.ProgressStepLanguages) {
		progress.CurrentStep = dnd5e.CreationStepLanguages
	} else {
		progress.CurrentStep = dnd5e.CreationStepReview
	}

	return progress
}
