package character

import (
	"context"
	"fmt"
	"strings"

	"github.com/KirkDiggler/rpg-api/internal/errors"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/dice"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	characterdraft "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
	"github.com/KirkDiggler/rpg-toolkit/events"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/backgrounds"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character/choices"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/races"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/spells"
)

// Config holds dependencies for the orchestrator
type Config struct {
	DraftRepo        characterdraft.Repository
	CharacterRepo    characterrepo.Repository
	DiceService      dice.Service
	IDGenerator      idgen.Generator
	DraftIDGenerator idgen.Generator
}

// Validate ensures all required dependencies are present
func (c *Config) Validate() error {
	if c.DraftRepo == nil {
		return errors.InvalidArgument("draft repository is required")
	}
	if c.CharacterRepo == nil {
		return errors.InvalidArgument("character repository is required")
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
	draftRepo     characterdraft.Repository
	characterRepo characterrepo.Repository
	diceService   dice.Service
	idGen         idgen.Generator
	draftIDGen    idgen.Generator
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
		draftRepo:     cfg.DraftRepo,
		characterRepo: cfg.CharacterRepo,
		diceService:   cfg.DiceService,
		idGen:         cfg.IDGenerator,
		draftIDGen:    cfg.DraftIDGenerator,
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
	if _, err := o.draftRepo.Create(ctx, characterdraft.CreateInput{
		Draft: draft.ToData(),
	}); err != nil {
		return nil, fmt.Errorf("failed to save draft: %w", err)
	}

	return &CreateDraftOutput{
		Draft: draft.ToData(),
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

	getOutput, err := o.draftRepo.Get(ctx, characterdraft.GetInput{
		ID: input.DraftID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get draft: %w", err)
	}

	return &GetDraftOutput{
		Draft:    getOutput.Draft,
		Progress: getOutput.Draft.Progress,
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

	if _, err := o.draftRepo.Delete(ctx, characterdraft.DeleteInput{
		ID: input.DraftID,
	}); err != nil {
		return nil, fmt.Errorf("failed to delete draft: %w", err)
	}

	return &DeleteDraftOutput{
		Success: true,
	}, nil
}

// GetRequirements returns the requirements for character creation choices
func (o *Orchestrator) GetRequirements(_ context.Context, input *GetRequirementsInput) (*GetRequirementsOutput, error) {
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
		// Use subclass-aware requirements if subclass is provided
		if input.Subclass != "" {
			requirements = choices.GetClassRequirementsWithSubclass(input.Class, level, input.Subclass)
		} else {
			requirements = choices.GetClassRequirementsAtLevel(input.Class, level)
		}
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
	getOutput, err := o.draftRepo.Get(ctx, characterdraft.GetInput{
		ID: input.DraftID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get draft: %w", err)
	}

	draft := character.LoadDraftFromData(getOutput.Draft)
	// Set name
	err = draft.SetName(&character.SetNameInput{Name: input.Name})
	if err != nil {
		return nil, fmt.Errorf("failed to set name: %w", err)
	}

	// Save updated draft
	updateOutput, err := o.draftRepo.Update(ctx, characterdraft.UpdateInput{
		Draft: draft.ToData(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to save draft: %w", err)
	}

	return &SetNameOutput{
		Draft:    updateOutput.Draft,
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
	getOutput, err := o.draftRepo.Get(ctx, characterdraft.GetInput{
		ID: input.DraftID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get draft: %w", err)
	}

	draft := character.LoadDraftFromData(getOutput.Draft)
	// Set race with choices
	err = draft.SetRace(input.Input)
	if err != nil {
		return nil, fmt.Errorf("failed to set race: %w", err)
	}

	// Save updated draft
	updateOutput, err := o.draftRepo.Update(ctx, characterdraft.UpdateInput{
		Draft: draft.ToData(),
	})
	if err != nil {
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
		Draft:      updateOutput.Draft,
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
	getOutput, err := o.draftRepo.Get(ctx, characterdraft.GetInput{
		ID: input.DraftID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get draft: %w", err)
	}

	draft := character.LoadDraftFromData(getOutput.Draft)
	// Set class with choices
	err = draft.SetClass(input.Input)
	if err != nil {
		return nil, fmt.Errorf("failed to set class: %w", err)
	}

	// Save updated draft
	updateOutput, err := o.draftRepo.Update(ctx, characterdraft.UpdateInput{
		Draft: draft.ToData(),
	})
	if err != nil {
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
		Draft:      updateOutput.Draft,
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
	getOutput, err := o.draftRepo.Get(ctx, characterdraft.GetInput{
		ID: input.DraftID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get draft: %w", err)
	}

	draft := character.LoadDraftFromData(getOutput.Draft)
	// Set background with choices
	err = draft.SetBackground(input.Input)
	if err != nil {
		return nil, fmt.Errorf("failed to set background: %w", err)
	}

	// Save updated draft
	updateOutput, err := o.draftRepo.Update(ctx, characterdraft.UpdateInput{
		Draft: draft.ToData(),
	})
	if err != nil {
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
		Draft:      updateOutput.Draft,
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
	getOutput, err := o.draftRepo.Get(ctx, characterdraft.GetInput{
		ID: input.DraftID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get draft: %w", err)
	}

	draft := character.LoadDraftFromData(getOutput.Draft)
	// Set ability scores
	err = draft.SetAbilityScores(input.Input)
	if err != nil {
		return nil, fmt.Errorf("failed to set ability scores: %w", err)
	}

	// Save updated draft
	updateOutput, err := o.draftRepo.Update(ctx, characterdraft.UpdateInput{
		Draft: draft.ToData(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to save draft: %w", err)
	}

	return &SetAbilityScoresOutput{
		Draft:    updateOutput.Draft,
		Progress: draft.Progress(),
	}, nil
}

// SetAbilityScoresFromRolls sets ability scores from dice roll assignments
func (o *Orchestrator) SetAbilityScoresFromRolls(ctx context.Context, input *SetAbilityScoresFromRollsInput) (*SetAbilityScoresFromRollsOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}
	if input.DraftID == "" {
		return nil, errors.InvalidArgument("draft ID is required")
	}
	if len(input.RollAssignments) == 0 {
		return nil, errors.InvalidArgument("roll assignments are required")
	}

	// Get draft first - we might need it for player ID lookup
	getOutput, err := o.draftRepo.Get(ctx, characterdraft.GetInput{
		ID: input.DraftID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get draft: %w", err)
	}

	// Get the dice session for this draft
	// First try with draft ID (correct way)
	sessionOutput, err := o.diceService.GetRollSession(ctx, &dice.GetRollSessionInput{
		EntityID: input.DraftID,
		Context:  dice.ContextAbilityScores,
	})
	if err != nil {
		// If not found with draft ID, try with player ID
		// (for backward compatibility with web app that might be using player ID)
		if errors.IsNotFound(err) {
			sessionOutput, err = o.diceService.GetRollSession(ctx, &dice.GetRollSessionInput{
				EntityID: getOutput.Draft.PlayerID,
				Context:  dice.ContextAbilityScores,
			})
			if err != nil {
				return nil, errors.Wrap(err, "failed to get dice roll session (tried both draft ID and player ID)")
			}
		} else {
			return nil, errors.Wrap(err, "failed to get dice roll session")
		}
	}

	// Build a map of roll IDs to their totals
	rollTotals := make(map[string]int)
	for _, roll := range sessionOutput.Session.Rolls {
		rollTotals[roll.RollID] = roll.Total
	}

	// Convert roll assignments to ability scores
	scores := make(shared.AbilityScores)
	for ability, rollID := range input.RollAssignments {
		if total, ok := rollTotals[rollID]; ok {
			scores[ability] = total
		} else {
			return nil, errors.InvalidArgument(fmt.Sprintf("roll ID %s not found in session", rollID))
		}
	}

	draft := character.LoadDraftFromData(getOutput.Draft)

	// Set ability scores with "rolled" method
	err = draft.SetAbilityScores(&character.SetAbilityScoresInput{
		Scores: scores,
		Method: "rolled",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to set ability scores: %w", err)
	}

	// Save updated draft
	updateOutput, err := o.draftRepo.Update(ctx, characterdraft.UpdateInput{
		Draft: draft.ToData(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to save draft: %w", err)
	}

	return &SetAbilityScoresFromRollsOutput{
		Draft:    updateOutput.Draft,
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
	getOutput, err := o.draftRepo.Get(ctx, characterdraft.GetInput{
		ID: input.DraftID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get draft: %w", err)
	}

	draft := character.LoadDraftFromData(getOutput.Draft)

	// Validate all choices
	validationErr := draft.ValidateChoices()

	// Convert validation error to structured result
	var validationResult *choices.ValidationResult
	if validationErr != nil {
		// Parse the error message to extract what's missing
		errMsg := validationErr.Error()
		validationResult = &choices.ValidationResult{
			Valid: false,
			Errors: []choices.ValidationError{
				{
					Category: "", // Unknown category
					Message:  errMsg,
				},
			},
		}

		// Try to identify the specific category from the error message
		if strings.Contains(errMsg, "fighting style") {
			validationResult.Errors[0].Category = shared.ChoiceFightingStyle
		} else if strings.Contains(errMsg, "skill") {
			validationResult.Errors[0].Category = shared.ChoiceSkills
		} else if strings.Contains(errMsg, "equipment") {
			validationResult.Errors[0].Category = shared.ChoiceEquipment
		}
	} else {
		validationResult = &choices.ValidationResult{
			Valid:  true,
			Errors: []choices.ValidationError{},
		}
	}

	return &ValidateDraftOutput{
		Valid:      validationErr == nil,
		Progress:   draft.Progress(),
		Validation: validationResult,
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
	getOutput, err := o.draftRepo.Get(ctx, characterdraft.GetInput{
		ID: input.DraftID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get draft: %w", err)
	}

	draft := character.LoadDraftFromData(getOutput.Draft)

	// Check if draft is complete
	progress := draft.Progress()
	if !progress.IsComplete() {
		// Build detailed error message about what's missing
		missing := []string{}
		if !progress.Has(character.ProgressName) {
			missing = append(missing, "name")
		}
		if !progress.Has(character.ProgressRace) {
			missing = append(missing, "race selection or race choices")
		}
		if !progress.Has(character.ProgressClass) {
			missing = append(missing, "class selection or class choices")
		}
		if !progress.Has(character.ProgressBackground) {
			missing = append(missing, "background selection or background choices")
		}
		if !progress.Has(character.ProgressAbilityScores) {
			missing = append(missing, "ability scores")
		}

		// For more detailed class validation, check what's missing
		if !progress.Has(character.ProgressClass) && draft.Class() != "" {
			// Class is set but choices are incomplete - validate to get details
			if validationErr := draft.ValidateChoices(); validationErr != nil {
				return nil, fmt.Errorf("draft is incomplete - missing: %v. Class validation: %w", missing, validationErr)
			}
		}

		return nil, errors.InvalidArgumentf("draft is incomplete - missing: %v (progress: %d%%)",
			missing, progress.PercentComplete())
	}

	// Validate all choices
	// TODO: Temporarily disabled due to toolkit bug #313 - ValidateChoices doesn't handle fighting styles
	// Re-enable once https://github.com/KirkDiggler/rpg-toolkit/issues/313 is fixed
	// if err := draft.ValidateChoices(); err != nil {
	// 	return nil, fmt.Errorf("draft validation failed: %w", err)
	// }

	// Convert to character with generated ID
	characterID := o.idGen.Generate()

	// Create EventBus for character features
	bus := events.NewEventBus()

	// ToCharacter now requires EventBus for feature subscription
	char, err := draft.ToCharacter(ctx, characterID, bus)
	if err != nil {
		return nil, fmt.Errorf("failed to convert draft to character: %w", err)
	}

	// Convert character to data for storage
	// ToData is now a method on Character
	charData := char.ToData()

	// Save character to character repository
	createOutput, err := o.characterRepo.Create(ctx, characterrepo.CreateInput{
		CharacterData: charData,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to save character: %w", err)
	}

	// Delete draft after successful finalization
	_, err = o.draftRepo.Delete(ctx, characterdraft.DeleteInput{
		ID: input.DraftID,
	})
	if err != nil {
		// Log error but don't fail the operation - character is already saved
		// TODO: Add proper error logging
		_ = err
	}

	// Load the saved character from the repository data
	// Create EventBus for character features
	finalBus := events.NewEventBus()

	// LoadFromData reconstructs Character with features subscribed to events
	finalChar, err := character.LoadFromData(ctx, createOutput.CharacterData, finalBus)
	if err != nil {
		return nil, fmt.Errorf("failed to load character from data: %w", err)
	}

	return &FinalizeDraftOutput{
		Character: finalChar,
	}, nil
}

// ListRaces returns all available races
func (o *Orchestrator) ListRaces(_ context.Context, _ *ListRacesInput) (*ListRacesOutput, error) {
	// Get races from toolkit - Data is now self-contained
	result := make([]*races.Data, 0, len(races.RaceData))
	for _, raceData := range races.RaceData {
		result = append(result, raceData)
	}

	return &ListRacesOutput{
		Races: result,
	}, nil
}

// ListClasses returns all available classes
func (o *Orchestrator) ListClasses(_ context.Context, _ *ListClassesInput) (*ListClassesOutput, error) {
	// Get classes from toolkit - Data is now self-contained
	result := make([]*classes.Data, 0, len(classes.ClassData))
	for _, classData := range classes.ClassData {
		result = append(result, classData)
	}

	return &ListClassesOutput{
		Classes: result,
	}, nil
}

// ListBackgrounds returns all available backgrounds
func (o *Orchestrator) ListBackgrounds(_ context.Context, _ *ListBackgroundsInput) (*ListBackgroundsOutput, error) {
	// Get backgrounds from toolkit - Data is now self-contained
	result := make([]*backgrounds.Data, 0, len(backgrounds.BackgroundData))
	for _, bgData := range backgrounds.BackgroundData {
		result = append(result, bgData)
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
		rolls[i] = AbilityScoreRoll{
			RollID:      roll.RollID,
			Total:       roll.Total,
			Dice:        roll.Dice,
			Dropped:     roll.Dropped,
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

// ListDrafts returns drafts for a player or session
func (o *Orchestrator) ListDrafts(ctx context.Context, input *ListDraftsInput) (*ListDraftsOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}
	if input.PlayerID == "" {
		return nil, errors.InvalidArgument("player ID is required")
	}

	// The repository uses a single-draft-per-player pattern
	// Try to get the player's draft
	getOutput, err := o.draftRepo.GetByPlayerID(ctx, characterdraft.GetByPlayerIDInput{
		PlayerID: input.PlayerID,
	})
	if err != nil {
		// If not found, return empty list
		if errors.IsNotFound(err) {
			return &ListDraftsOutput{
				Drafts:        []*character.DraftData{},
				NextPageToken: "",
			}, nil
		}
		return nil, fmt.Errorf("failed to get drafts: %w", err)
	}

	// Return the single draft as a list
	return &ListDraftsOutput{
		Drafts:        []*character.DraftData{getOutput.Draft},
		NextPageToken: "",
	}, nil
}

// GetCharacter retrieves a character by ID
func (o *Orchestrator) GetCharacter(ctx context.Context, input *GetCharacterInput) (*GetCharacterOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}
	if input.CharacterID == "" {
		return nil, errors.InvalidArgument("character ID is required")
	}

	// Get character from repository - equipment slots are part of character.Data
	result, err := o.characterRepo.Get(ctx, characterrepo.GetInput{
		ID: input.CharacterID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get character: %w", err)
	}

	return &GetCharacterOutput{
		Character: result.CharacterData,
	}, nil
}

// EquipItem equips an item to a specific slot
func (o *Orchestrator) EquipItem(ctx context.Context, input *EquipItemInput) (*EquipItemOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}
	if input.CharacterID == "" {
		return nil, errors.InvalidArgument("character ID is required")
	}
	if input.ItemID == "" {
		return nil, errors.InvalidArgument("item ID is required")
	}
	if input.Slot == "" {
		return nil, errors.InvalidArgument("slot is required")
	}

	// Get character data
	result, err := o.characterRepo.Get(ctx, characterrepo.GetInput{
		ID: input.CharacterID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get character: %w", err)
	}

	charData := result.CharacterData

	// Initialize equipment slots if nil
	if charData.EquipmentSlots == nil {
		charData.EquipmentSlots = make(character.EquipmentSlots)
	}

	// Get previous item ID for response
	previousItemID := charData.EquipmentSlots.Get(input.Slot)

	// Set the new item
	charData.EquipmentSlots.Set(input.Slot, input.ItemID)

	// Update character
	_, err = o.characterRepo.Update(ctx, characterrepo.UpdateInput{
		CharacterData: charData,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update character: %w", err)
	}

	return &EquipItemOutput{
		PreviousItemID: previousItemID,
	}, nil
}

// UnequipItem removes an item from a slot
func (o *Orchestrator) UnequipItem(ctx context.Context, input *UnequipItemInput) (*UnequipItemOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}
	if input.CharacterID == "" {
		return nil, errors.InvalidArgument("character ID is required")
	}
	if input.Slot == "" {
		return nil, errors.InvalidArgument("slot is required")
	}

	// Get character data
	result, err := o.characterRepo.Get(ctx, characterrepo.GetInput{
		ID: input.CharacterID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get character: %w", err)
	}

	charData := result.CharacterData

	// Get the item being unequipped for response
	unequippedItemID := charData.EquipmentSlots.Get(input.Slot)

	// Clear the slot
	charData.EquipmentSlots.Clear(input.Slot)

	// Update character
	_, err = o.characterRepo.Update(ctx, characterrepo.UpdateInput{
		CharacterData: charData,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update character: %w", err)
	}

	return &UnequipItemOutput{
		UnequippedItemID: unequippedItemID,
	}, nil
}

// ListCharacters returns characters for a player or session
func (o *Orchestrator) ListCharacters(ctx context.Context, input *ListCharactersInput) (*ListCharactersOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}

	// List by player ID (primary use case)
	if input.PlayerID != "" {
		result, err := o.characterRepo.ListByPlayerID(ctx, characterrepo.ListByPlayerIDInput{
			PlayerID: input.PlayerID,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list characters by player: %w", err)
		}

		return &ListCharactersOutput{
			Characters:    result.Characters,
			NextPageToken: "", // TODO: Add pagination support
			TotalSize:     len(result.Characters),
		}, nil
	}

	// List by session ID (secondary use case)
	if input.SessionID != "" {
		result, err := o.characterRepo.ListBySessionID(ctx, characterrepo.ListBySessionIDInput{
			SessionID: input.SessionID,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list characters by session: %w", err)
		}

		return &ListCharactersOutput{
			Characters:    result.Characters,
			NextPageToken: "", // TODO: Add pagination support
			TotalSize:     len(result.Characters),
		}, nil
	}

	// No filter provided - return error
	return nil, errors.InvalidArgument("either player_id or session_id must be provided")
}

// DeleteCharacter deletes a character permanently
func (o *Orchestrator) DeleteCharacter(ctx context.Context, input *DeleteCharacterInput) (*DeleteCharacterOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}
	if input.CharacterID == "" {
		return nil, errors.InvalidArgument("character ID is required")
	}

	// Delete from repository
	_, err := o.characterRepo.Delete(ctx, characterrepo.DeleteInput{
		ID: input.CharacterID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to delete character: %w", err)
	}

	return &DeleteCharacterOutput{}, nil
}

// ListEquipmentByType returns equipment filtered by type
func (o *Orchestrator) ListEquipmentByType(_ context.Context, input *ListEquipmentByTypeInput) (*ListEquipmentByTypeOutput, error) {
	// The handler will handle the conversion from proto enum to toolkit categories
	// For now, we'll let the handler do the work directly with the toolkit
	// This orchestrator method is a placeholder for future expansion

	// Return empty for now - the handler will use the toolkit directly
	return &ListEquipmentByTypeOutput{
		Equipment: []interface{}{},
	}, nil
}

// ListSpellsByLevel returns spells of a specific level with their info
func (o *Orchestrator) ListSpellsByLevel(_ context.Context, input *ListSpellsByLevelInput) (*ListSpellsByLevelOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}

	// Validate spell level
	if input.Level < 0 || input.Level > 9 {
		return nil, errors.InvalidArgument("spell level must be between 0 (cantrips) and 9")
	}

	// Get all spells for the requested level from the toolkit
	toolkitSpells := spells.GetSpellsByLevel(input.Level)

	// Convert to our SpellInfo format
	result := make([]SpellInfo, 0, len(toolkitSpells))
	for _, spellData := range toolkitSpells {
		info := SpellInfo{
			ID:          spellData.ID,
			Name:        spellData.Name,
			Description: spellData.Description,
			Level:       spellData.Level,
		}
		result = append(result, info)
	}

	// TODO: Implement class filtering if needed
	// TODO: Implement pagination if needed

	return &ListSpellsByLevelOutput{
		Spells: result,
		Total:  len(result),
	}, nil
}
