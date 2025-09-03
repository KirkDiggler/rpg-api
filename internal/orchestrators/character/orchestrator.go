package character

import (
	"context"
	"fmt"
	
	"github.com/KirkDiggler/rpg-api/internal/errors"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/dice"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	"github.com/KirkDiggler/rpg-api/internal/repositories/draft"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/backgrounds"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character/choices"
)

// Config holds dependencies for the orchestrator
type Config struct {
	DraftRepo        draft.Repository
	DiceService      dice.Service
	IDGenerator      idgen.Generator
	DraftIDGenerator idgen.Generator
}

// Validate ensures all required dependencies are present
func (c *Config) Validate() error {
	if c.DraftRepo == nil {
		return errors.InvalidArgument("draft repository is required")
	}
	if c.DiceService == nil {
		return errors.InvalidArgument("dice service is required")
	}
	if c.IDGenerator == nil {
		return errors.InvalidArgument("ID generator is required")
	}
	if c.DraftIDGenerator == nil {
		return errors.InvalidArgument("draft ID generator is required")
	}
	return nil
}

// Orchestrator implements the character service
type Orchestrator struct {
	draftRepo   draft.Repository
	diceService dice.Service
	idGen       idgen.Generator
	draftIDGen  idgen.Generator
}

// New creates a new character orchestrator
func New(cfg *Config) (*Orchestrator, error) {
	if cfg == nil {
		return nil, errors.InvalidArgument("config is required")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	
	return &Orchestrator{
		draftRepo:   cfg.DraftRepo,
		diceService: cfg.DiceService,
		idGen:       cfg.IDGenerator,
		draftIDGen:  cfg.DraftIDGenerator,
	}, nil
}

// CreateDraft creates a new character draft
func (o *Orchestrator) CreateDraft(ctx context.Context, input *CreateDraftInput) (*CreateDraftOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}
	if input.PlayerID == "" {
		return nil, errors.InvalidArgument("player ID is required")
	}
	
	// Create new draft with generated ID
	draftConfig := &character.DraftConfig{
		ID:       o.draftIDGen.Generate(),
		PlayerID: input.PlayerID,
	}
	
	draft, err := character.NewDraft(draftConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create draft: %w", err)
	}
	
	// Save to repository
	if err := o.draftRepo.Save(ctx, draft); err != nil {
		return nil, fmt.Errorf("failed to save draft: %w", err)
	}
	
	return &CreateDraftOutput{
		Draft: draft,
	}, nil
}

// GetDraft retrieves a draft by ID
func (o *Orchestrator) GetDraft(ctx context.Context, input *GetDraftInput) (*GetDraftOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}
	if input.DraftID == "" {
		return nil, errors.InvalidArgument("draft ID is required")
	}
	
	draft, err := o.draftRepo.Get(ctx, input.DraftID)
	if err != nil {
		return nil, fmt.Errorf("failed to get draft: %w", err)
	}
	
	return &GetDraftOutput{
		Draft:    draft,
		Progress: draft.Progress(),
	}, nil
}

// DeleteDraft deletes a draft
func (o *Orchestrator) DeleteDraft(ctx context.Context, input *DeleteDraftInput) (*DeleteDraftOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}
	if input.DraftID == "" {
		return nil, errors.InvalidArgument("draft ID is required")
	}
	
	if err := o.draftRepo.Delete(ctx, input.DraftID); err != nil {
		return nil, fmt.Errorf("failed to delete draft: %w", err)
	}
	
	return &DeleteDraftOutput{
		Success: true,
	}, nil
}

// GetRequirements returns the requirements for character creation choices
func (o *Orchestrator) GetRequirements(ctx context.Context, input *GetRequirementsInput) (*GetRequirementsOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}
	
	// Default to level 1 if not specified
	level := input.Level
	if level == 0 {
		level = 1
	}
	
	var requirements *choices.Requirements
	
	// Get class requirements if class is specified
	if input.Class != "" {
		requirements = choices.GetClassRequirementsAtLevel(input.Class, level)
	}
	
	// Get race requirements if race is specified
	if input.Race != "" {
		raceReqs := choices.GetRaceRequirements(input.Race)
		if requirements == nil {
			requirements = raceReqs
		} else {
			// Merge race requirements into class requirements
			if raceReqs.Skills != nil {
				requirements.Skills = raceReqs.Skills
			}
			if raceReqs.Languages != nil {
				requirements.Languages = raceReqs.Languages
			}
		}
	}
	
	// If no requirements found, return empty requirements
	if requirements == nil {
		requirements = &choices.Requirements{}
	}
	
	return &GetRequirementsOutput{
		Requirements: requirements,
	}, nil
}

// SetName sets the character name
func (o *Orchestrator) SetName(ctx context.Context, input *SetNameInput) (*SetNameOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}
	if input.DraftID == "" {
		return nil, errors.InvalidArgument("draft ID is required")
	}
	if input.Name == "" {
		return nil, errors.InvalidArgument("name is required")
	}
	
	// Get draft
	draft, err := o.draftRepo.Get(ctx, input.DraftID)
	if err != nil {
		return nil, fmt.Errorf("failed to get draft: %w", err)
	}
	
	// Set name
	if err := draft.SetName(&character.SetNameInput{Name: input.Name}); err != nil {
		return nil, fmt.Errorf("failed to set name: %w", err)
	}
	
	// Save updated draft
	if err := o.draftRepo.Save(ctx, draft); err != nil {
		return nil, fmt.Errorf("failed to save draft: %w", err)
	}
	
	return &SetNameOutput{
		Draft:    draft,
		Progress: draft.Progress(),
	}, nil
}

// SetRace sets the race with choices
func (o *Orchestrator) SetRace(ctx context.Context, input *SetRaceInput) (*SetRaceOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}
	if input.DraftID == "" {
		return nil, errors.InvalidArgument("draft ID is required")
	}
	if input.Input == nil {
		return nil, errors.InvalidArgument("race input is required")
	}
	
	// Get draft
	draft, err := o.draftRepo.Get(ctx, input.DraftID)
	if err != nil {
		return nil, fmt.Errorf("failed to get draft: %w", err)
	}
	
	// Set race with choices
	if err := draft.SetRace(input.Input); err != nil {
		return nil, fmt.Errorf("failed to set race: %w", err)
	}
	
	// Save updated draft
	if err := o.draftRepo.Save(ctx, draft); err != nil {
		return nil, fmt.Errorf("failed to save draft: %w", err)
	}
	
	// Validate race choices
	var validation *choices.ValidationResult
	if draft.IsRaceComplete() {
		// For now, skip validation since getRaceSubmissions is unexported
		// TODO: Add public method to Draft or handle differently
		validation = nil
	}
	
	return &SetRaceOutput{
		Draft:      draft,
		Progress:   draft.Progress(),
		Validation: validation,
	}, nil
}

// SetClass sets the class with choices
func (o *Orchestrator) SetClass(ctx context.Context, input *SetClassInput) (*SetClassOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}
	if input.DraftID == "" {
		return nil, errors.InvalidArgument("draft ID is required")
	}
	if input.Input == nil {
		return nil, errors.InvalidArgument("class input is required")
	}
	
	// Get draft
	draft, err := o.draftRepo.Get(ctx, input.DraftID)
	if err != nil {
		return nil, fmt.Errorf("failed to get draft: %w", err)
	}
	
	// Set class with choices
	if err := draft.SetClass(input.Input); err != nil {
		return nil, fmt.Errorf("failed to set class: %w", err)
	}
	
	// Save updated draft
	if err := o.draftRepo.Save(ctx, draft); err != nil {
		return nil, fmt.Errorf("failed to save draft: %w", err)
	}
	
	// Validate class choices
	var validation *choices.ValidationResult
	if draft.IsClassComplete() {
		// For now, skip validation since getClassSubmissions is unexported
		// TODO: Add public method to Draft or handle differently
		validation = nil
	}
	
	return &SetClassOutput{
		Draft:      draft,
		Progress:   draft.Progress(),
		Validation: validation,
	}, nil
}

// SetBackground sets the background with choices
func (o *Orchestrator) SetBackground(ctx context.Context, input *SetBackgroundInput) (*SetBackgroundOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}
	if input.DraftID == "" {
		return nil, errors.InvalidArgument("draft ID is required")
	}
	if input.Input == nil {
		return nil, errors.InvalidArgument("background input is required")
	}
	
	// Get draft
	draft, err := o.draftRepo.Get(ctx, input.DraftID)
	if err != nil {
		return nil, fmt.Errorf("failed to get draft: %w", err)
	}
	
	// Set background with choices
	if err := draft.SetBackground(input.Input); err != nil {
		return nil, fmt.Errorf("failed to set background: %w", err)
	}
	
	// Save updated draft
	if err := o.draftRepo.Save(ctx, draft); err != nil {
		return nil, fmt.Errorf("failed to save draft: %w", err)
	}
	
	// Validate background choices
	var validation *choices.ValidationResult
	if draft.IsBackgroundComplete() {
		// For now, skip validation since background methods are missing
		// TODO: Add background validation
		validation = nil
	}
	
	return &SetBackgroundOutput{
		Draft:      draft,
		Progress:   draft.Progress(),
		Validation: validation,
	}, nil
}

// SetAbilityScores sets ability scores
func (o *Orchestrator) SetAbilityScores(ctx context.Context, input *SetAbilityScoresInput) (*SetAbilityScoresOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}
	if input.DraftID == "" {
		return nil, errors.InvalidArgument("draft ID is required")
	}
	
	// Get draft
	draft, err := o.draftRepo.Get(ctx, input.DraftID)
	if err != nil {
		return nil, fmt.Errorf("failed to get draft: %w", err)
	}
	
	// Set ability scores
	if err := draft.SetAbilityScores(input.Input); err != nil {
		return nil, fmt.Errorf("failed to set ability scores: %w", err)
	}
	
	// Save updated draft
	if err := o.draftRepo.Save(ctx, draft); err != nil {
		return nil, fmt.Errorf("failed to save draft: %w", err)
	}
	
	return &SetAbilityScoresOutput{
		Draft:    draft,
		Progress: draft.Progress(),
	}, nil
}

// ValidateDraft validates a draft
func (o *Orchestrator) ValidateDraft(ctx context.Context, input *ValidateDraftInput) (*ValidateDraftOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}
	if input.DraftID == "" {
		return nil, errors.InvalidArgument("draft ID is required")
	}
	
	// Get draft
	draft, err := o.draftRepo.Get(ctx, input.DraftID)
	if err != nil {
		return nil, fmt.Errorf("failed to get draft: %w", err)
	}
	
	// Validate all choices
	validationErr := draft.ValidateChoices()
	
	// TODO: Convert validation errors to proper ValidationResult
	// For now, log validation errors for debugging
	if validationErr != nil {
		// This helps developers understand why validation failed
		fmt.Printf("Draft validation failed: %v\n", validationErr)
	}
	
	return &ValidateDraftOutput{
		Valid:      validationErr == nil,
		Progress:   draft.Progress(),
		Validation: nil, // TODO: Convert error to ValidationResult
	}, nil
}

// FinalizeDraft converts a draft to a character
func (o *Orchestrator) FinalizeDraft(ctx context.Context, input *FinalizeDraftInput) (*FinalizeDraftOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}
	if input.DraftID == "" {
		return nil, errors.InvalidArgument("draft ID is required")
	}
	
	// Get draft
	draft, err := o.draftRepo.Get(ctx, input.DraftID)
	if err != nil {
		return nil, fmt.Errorf("failed to get draft: %w", err)
	}
	
	// Check if draft is complete
	if !draft.Progress().IsComplete() {
		return nil, errors.InvalidArgument("draft is not complete")
	}
	
	// Validate all choices
	if err := draft.ValidateChoices(); err != nil {
		return nil, fmt.Errorf("draft validation failed: %w", err)
	}
	
	// Convert to character with generated ID
	characterID := o.idGen.Generate()
	char, err := draft.ToCharacter(characterID)
	if err != nil {
		return nil, fmt.Errorf("failed to convert draft to character: %w", err)
	}
	
	// TODO: Save character to character repository
	// For now, just return the character
	
	// Delete draft after successful finalization
	if err := o.draftRepo.Delete(ctx, input.DraftID); err != nil {
		// Log error but don't fail the operation
		// TODO: Add logging
	}
	
	return &FinalizeDraftOutput{
		Character: char,
	}, nil
}

// ListRaces returns all available races
func (o *Orchestrator) ListRaces(ctx context.Context, input *ListRacesInput) (*ListRacesOutput, error) {
	if input == nil {
		input = &ListRacesInput{}
	}
	
	// TODO: Implement ListRaces
	// Need to add similar structure to races as we did for backgrounds
	
	return &ListRacesOutput{
		Races: []RaceInfo{},
	}, nil
}

// ListClasses returns all available classes
func (o *Orchestrator) ListClasses(ctx context.Context, input *ListClassesInput) (*ListClassesOutput, error) {
	if input == nil {
		input = &ListClassesInput{}
	}
	
	// TODO: Implement ListClasses
	// Classes already have GetData() and methods, just need to wire up
	
	return &ListClassesOutput{
		Classes: []ClassInfo{},
	}, nil
}

// ListBackgrounds returns all available backgrounds
func (o *Orchestrator) ListBackgrounds(ctx context.Context, input *ListBackgroundsInput) (*ListBackgroundsOutput, error) {
	if input == nil {
		input = &ListBackgroundsInput{}
	}
	
	// Get backgrounds from toolkit
	result := make([]BackgroundInfo, 0, len(backgrounds.All))
	for id, bg := range backgrounds.All {
		bgData := backgrounds.GetData(bg)
		if bgData == nil {
			continue // Skip if no data available
		}
		
		// Convert skills to string IDs
		skillIDs := make([]string, len(bgData.Skills))
		for i, skill := range bgData.Skills {
			skillIDs[i] = string(skill)
		}
		
		result = append(result, BackgroundInfo{
			ID:          id,
			Name:        bg.Name(),
			Description: bg.Description(),
			Skills:      skillIDs,
		})
	}
	
	return &ListBackgroundsOutput{
		Backgrounds: result,
	}, nil
}

// RollAbilityScores rolls ability scores for character creation
func (o *Orchestrator) RollAbilityScores(ctx context.Context, input *RollAbilityScoresInput) (*RollAbilityScoresOutput, error) {
	if input == nil {
		input = &RollAbilityScoresInput{}
	}
	
	diceResult, err := o.diceService.RollAbilityScores(ctx, &dice.RollAbilityScoresInput{
		EntityID: input.DraftID, // Use draft ID as entity ID
		Method:   input.Method,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to roll ability scores: %w", err)
	}
	
	// Convert dice service result to our output format
	rolls := make([]AbilityScoreRoll, len(diceResult.Rolls))
	for i, roll := range diceResult.Rolls {
		// Convert int32 to int for our API
		dice := make([]int, len(roll.Dice))
		for j, d := range roll.Dice {
			dice[j] = int(d)
		}
		dropped := make([]int, len(roll.Dropped))
		for j, d := range roll.Dropped {
			dropped[j] = int(d)
		}
		
		rolls[i] = AbilityScoreRoll{
			RollID:      roll.RollID,
			Total:       int(roll.Total),
			Dice:        dice,
			Dropped:     dropped,
			Description: roll.Description,
		}
	}
	
	var sessionID string
	if diceResult.Session != nil {
		sessionID = fmt.Sprintf("%s:%s", diceResult.Session.EntityID, diceResult.Session.Context)
	}
	
	return &RollAbilityScoresOutput{
		Rolls:     rolls,
		SessionID: sessionID,
	}, nil
}