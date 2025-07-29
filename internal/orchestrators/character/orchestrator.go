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
	"time"

	"github.com/KirkDiggler/rpg-api/internal/clients/external"
	"github.com/KirkDiggler/rpg-api/internal/engine"
	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
	"github.com/KirkDiggler/rpg-api/internal/errors"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/dice"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	draftrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
	equipmentrepo "github.com/KirkDiggler/rpg-api/internal/repositories/equipment"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/class"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/constants"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/race"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
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

	// Equipment management
	GetInventory(ctx context.Context, input *GetInventoryInput) (*GetInventoryOutput, error)
	EquipItem(ctx context.Context, input *EquipItemInput) (*EquipItemOutput, error)
	UnequipItem(ctx context.Context, input *UnequipItemInput) (*UnequipItemOutput, error)
	AddToInventory(ctx context.Context, input *AddToInventoryInput) (*AddToInventoryOutput, error)
	RemoveFromInventory(ctx context.Context, input *RemoveFromInventoryInput) (*RemoveFromInventoryOutput, error)
}

// Config holds the dependencies for the character orchestrator
type Config struct {
	CharacterRepo      characterrepo.Repository
	CharacterDraftRepo draftrepo.Repository
	EquipmentRepo      equipmentrepo.Repository
	Engine             engine.Engine
	ExternalClient     external.Client
	DiceService        dice.Service
	IDGenerator        idgen.Generator
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
	if c.EquipmentRepo == nil {
		vb.RequiredField("EquipmentRepo")
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
	if c.IDGenerator == nil {
		vb.RequiredField("IDGenerator")
	}

	return vb.Build()
}

// Orchestrator implements the Service interface
type Orchestrator struct {
	characterRepo      characterrepo.Repository
	characterDraftRepo draftrepo.Repository
	equipmentRepo      equipmentrepo.Repository
	engine             engine.Engine
	externalClient     external.Client
	diceService        dice.Service
	idGenerator        idgen.Generator
}

// New creates a new character orchestrator
func New(cfg *Config) (*Orchestrator, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid config")
	}

	return &Orchestrator{
		characterRepo:      cfg.CharacterRepo,
		characterDraftRepo: cfg.CharacterDraftRepo,
		equipmentRepo:      cfg.EquipmentRepo,
		engine:             cfg.Engine,
		externalClient:     cfg.ExternalClient,
		diceService:        cfg.DiceService,
		idGenerator:        cfg.IDGenerator,
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

	// Generate draft ID using UUID generator
	draftID := o.idGenerator.Generate()

	// Create new builder
	builder, err := character.NewCharacterBuilder(draftID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create character builder")
	}

	// Get the initial draft data from builder
	draftData := builder.ToData()
	draftData.PlayerID = input.PlayerID

	// Apply initial data if provided
	if input.InitialData != nil {
		// Set name if provided
		if input.InitialData.Name != "" {
			if err := builder.SetName(input.InitialData.Name); err != nil {
				return nil, errors.Wrap(err, "failed to set name")
			}
		}

		// Set race if provided
		if input.InitialData.RaceChoice.RaceID != "" {
			if err := builder.SetRaceData(race.Data{ID: input.InitialData.RaceChoice.RaceID}, input.InitialData.RaceChoice.SubraceID); err != nil {
				return nil, errors.Wrap(err, "failed to set race")
			}
		}

		// Set class if provided
		if input.InitialData.ClassChoice != "" {
			if err := builder.SetClassData(class.Data{ID: input.InitialData.ClassChoice}); err != nil {
				return nil, errors.Wrap(err, "failed to set class")
			}
		}

		// Set ability scores if provided
		if len(input.InitialData.AbilityScoreChoice) > 0 {
			if err := builder.SetAbilityScores(input.InitialData.AbilityScoreChoice); err != nil {
				return nil, errors.Wrap(err, "failed to set ability scores")
			}
		}
	}

	// Get the updated draft data
	draftData = builder.ToData()
	draftData.PlayerID = input.PlayerID

	// Create draft using repository
	createOutput, err := o.characterDraftRepo.Create(ctx, draftrepo.CreateInput{
		Draft: &draftData,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create draft")
	}

	// Load the draft from the created data
	draft, err := character.LoadDraftFromData(*createOutput.Draft)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load draft")
	}

	return &CreateDraftOutput{
		Draft: draft,
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

	// Load the draft from the data
	draft, err := character.LoadDraftFromData(*getOutput.Draft)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load draft")
	}

	return &GetDraftOutput{
		Draft: draft,
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
				Drafts:        []*character.Draft{},
				NextPageToken: "",
			}, nil
		}
		return nil, errors.Wrap(err, "failed to get player draft")
	}

	// Load the draft from data
	draft, err := character.LoadDraftFromData(*draftOutput.Draft)
	if err != nil {
		// Log the error but offer to delete the corrupt draft
		slog.ErrorContext(ctx, "Failed to load draft data - draft may be corrupt",
			"error", err.Error(),
			"draft_id", draftOutput.Draft.ID,
			"player_id", input.PlayerID)

		// For now, we'll return an empty list and suggest deletion
		// In the future, we might want to auto-delete or return partial data
		return &ListDraftsOutput{
			Drafts:        []*character.Draft{},
			NextPageToken: "",
			// TODO: Add a field to indicate corrupt drafts that need cleanup
		}, nil
	}

	// Return single draft as a list
	// Note: rpg-toolkit Draft doesn't have hydrated info objects
	// The handler layer can fetch additional info if needed for UI
	return &ListDraftsOutput{
		Drafts:        []*character.Draft{draft},
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

	// Load into builder
	builder, err := character.LoadDraft(*getOutput.Draft)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load draft into builder")
	}

	// Update the name
	if err := builder.SetName(input.Name); err != nil {
		return nil, errors.Wrap(err, "failed to set name")
	}

	// Get updated draft data
	updatedData := builder.ToData()

	// Save the updated draft
	updateOutput, err := o.characterDraftRepo.Update(ctx, draftrepo.UpdateInput{
		Draft: &updatedData,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to update draft")
	}

	// Load the updated draft
	updatedDraft, err := character.LoadDraftFromData(*updateOutput.Draft)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load draft")
	}

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

	// Load into builder
	builder, err := character.LoadDraft(*getOutput.Draft)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load draft into builder")
	}

	// Get race data from external API
	raceData, err := o.externalClient.GetRaceData(ctx, input.RaceID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get race data")
	}

	// Convert to toolkit race data format
	// For now, we'll use the API race data to set basic info
	// TODO: Convert full race data when toolkit supports it
	toolkitRaceData := race.Data{
		ID:   raceData.ID,
		Name: raceData.Name,
		// Add other fields as needed
	}

	// Set race data in builder
	if err := builder.SetRaceData(toolkitRaceData, input.SubraceID); err != nil {
		return nil, errors.Wrap(err, "failed to set race data")
	}

	// Get updated draft data
	updatedData := builder.ToData()

	// Note: Race-specific choices (languages, variant human feat, etc.) should be handled
	// through the UpdateChoices method with the appropriate choice types

	// Save the updated draft
	updateOutput, err := o.characterDraftRepo.Update(ctx, draftrepo.UpdateInput{
		Draft: &updatedData,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to update draft")
	}

	// Load the updated draft
	updatedDraft, err := character.LoadDraftFromData(*updateOutput.Draft)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load draft")
	}

	// Collect warnings (for now, empty)
	var warnings []ValidationWarning

	return &UpdateRaceOutput{
		Draft:    updatedDraft,
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

	// Load into builder
	builder, err := character.LoadDraft(*getOutput.Draft)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load draft into builder")
	}

	// Get class data from external API
	classData, err := o.externalClient.GetClassData(ctx, input.ClassID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get class data")
	}

	// Convert to toolkit class data format
	// For now, we'll use the API class data to set basic info
	// TODO: Convert full class data when toolkit supports it
	toolkitClassData := class.Data{
		ID:   classData.ID,
		Name: classData.Name,
		// Add other fields as needed
	}

	// Set class data in builder
	if err := builder.SetClassData(toolkitClassData); err != nil {
		return nil, errors.Wrap(err, "failed to set class data")
	}

	// Get updated draft data
	updatedData := builder.ToData()

	// Note: Class-specific choices (skills, equipment, etc.) should be handled
	// through the UpdateChoices method with the appropriate choice types

	// Save the updated draft
	updateOutput, err := o.characterDraftRepo.Update(ctx, draftrepo.UpdateInput{
		Draft: &updatedData,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to update draft")
	}

	// Load the updated draft
	updatedDraft, err := character.LoadDraftFromData(*updateOutput.Draft)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load draft")
	}

	// Collect warnings (for now, empty)
	var warnings []ValidationWarning

	return &UpdateClassOutput{
		Draft:    updatedDraft,
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

	// Load into builder
	builder, err := character.LoadDraft(*getOutput.Draft)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load draft into builder")
	}

	// Get background data from external API
	bgData, err := o.externalClient.GetBackgroundData(ctx, input.BackgroundID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get background data")
	}

	// Convert to toolkit background data format
	toolkitBgData := shared.Background{
		ID:   bgData.ID,
		Name: bgData.Name,
		// Add other fields as needed
	}

	// Set background data in builder
	if err := builder.SetBackgroundData(toolkitBgData); err != nil {
		return nil, errors.Wrap(err, "failed to set background data")
	}

	// TODO(#128): Apply background choices to builder when toolkit supports it
	// Currently, the toolkit builder doesn't have methods for background-specific choices

	// Get updated draft data
	updatedData := builder.ToData()

	// Save the updated draft
	updateOutput, err := o.characterDraftRepo.Update(ctx, draftrepo.UpdateInput{
		Draft: &updatedData,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to update draft")
	}

	// Load the updated draft
	updatedDraft, err := character.LoadDraftFromData(*updateOutput.Draft)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load draft")
	}

	return &UpdateBackgroundOutput{
		Draft: updatedDraft,
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

	// Load into builder
	builder, err := character.LoadDraft(*getOutput.Draft)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load draft into builder")
	}

	// Use the ability scores directly - they're already in the right format
	scores := input.AbilityScores

	// Set ability scores in builder
	slog.InfoContext(ctx, "Setting ability scores",
		"draft_id", input.DraftID,
		"scores", scores)
	if err := builder.SetAbilityScores(scores); err != nil {
		return nil, errors.Wrap(err, "failed to set ability scores")
	}

	// Get updated draft data
	updatedData := builder.ToData()
	slog.InfoContext(ctx, "Draft data after setting ability scores",
		"draft_id", updatedData.ID)

	// Save the updated draft
	slog.InfoContext(ctx, "Saving updated draft to repository",
		"draft_id", updatedData.ID,
		"player_id", updatedData.PlayerID)
	updateOutput, err := o.characterDraftRepo.Update(ctx, draftrepo.UpdateInput{
		Draft: &updatedData,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to update draft")
	}
	slog.InfoContext(ctx, "Draft saved successfully",
		"draft_id", updateOutput.Draft.ID)

	// Load the updated draft
	updatedDraft, err := character.LoadDraftFromData(*updateOutput.Draft)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load draft")
	}

	// Collect warnings (for now, empty)
	var warnings []ValidationWarning

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

	// Load into builder
	builder, err := character.LoadDraft(*getOutput.Draft)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load draft into builder")
	}

	// Set skills in builder
	if err := builder.SelectSkills(input.SkillIDs); err != nil {
		return nil, errors.Wrap(err, "failed to select skills")
	}

	// Get updated draft data
	updatedData := builder.ToData()

	// Save the updated draft
	updateOutput, err := o.characterDraftRepo.Update(ctx, draftrepo.UpdateInput{
		Draft: &updatedData,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to update draft")
	}

	// Load the updated draft
	updatedDraft, err := character.LoadDraftFromData(*updateOutput.Draft)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load draft")
	}

	// Collect warnings (for now, empty)
	var warnings []ValidationWarning

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

	// Load into builder
	builder, err := character.LoadDraft(*getOutput.Draft)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load draft into builder")
	}

	// Validate using toolkit
	validationErrors := builder.Validate()

	// Convert validation errors
	errors := make([]ValidationError, 0, len(validationErrors))
	for _, ve := range validationErrors {
		errors = append(errors, ValidationError{
			Field:   ve.Field,
			Message: ve.Message,
			Type:    "VALIDATION_ERROR",
		})
	}

	// Check if can build (all required fields set)
	progress := builder.Progress()

	return &ValidateDraftOutput{
		IsComplete:   progress.CanBuild,
		IsValid:      len(errors) == 0,
		Errors:       errors,
		MissingSteps: []string{}, // TODO: Get from builder when supported
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

	// Load the draft data
	draftData := *getOutput.Draft

	// Extract IDs from choices
	var raceID, subraceID, classID, backgroundID string
	var abilityScores shared.AbilityScores

	// Extract choices from typed fields
	raceID = draftData.RaceChoice.RaceID
	subraceID = draftData.RaceChoice.SubraceID
	classID = draftData.ClassChoice
	backgroundID = draftData.BackgroundChoice
	
	// Use default background if not set
	if backgroundID == "" {
		backgroundID = dnd5e.BackgroundAcolyte
	}

	// Extract ability scores
	abilityScores = draftData.AbilityScoreChoice
	slog.InfoContext(ctx, "Extracting ability scores from draft",
		"ability_scores", fmt.Sprintf("%+v", abilityScores))
	
	
	// Validate ability scores are at least 3 (D&D 5e minimum)
	for ability, score := range abilityScores {
		if score < 3 {
			return nil, errors.InvalidArgumentf("invalid ability score for %s: %d (minimum is 3)", ability, score)
		}
	}

	// Fetch race data
	var toolkitRaceData *race.Data
	if raceID != "" {
		apiRaceData, err := o.externalClient.GetRaceData(ctx, raceID)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get race data for %s", raceID)
		}
		toolkitRaceData = &race.Data{
			ID:        apiRaceData.ID,
			Name:      apiRaceData.Name,
			Speed:     int(apiRaceData.Speed),
			Size:      apiRaceData.Size,
			Languages: make([]constants.Language, len(apiRaceData.Languages)),
			// Map ability score increases
			AbilityScoreIncreases: make(map[constants.Ability]int),
		}
		
		// Convert languages using typed constants
		for i, lang := range apiRaceData.Languages {
			toolkitRaceData.Languages[i] = constants.Language(lang)
		}

		// Convert ability bonuses from int32 to int
		for ability, bonus := range apiRaceData.AbilityBonuses {
			toolkitRaceData.AbilityScoreIncreases[constants.Ability(ability)] = int(bonus)
		}

		// Map proficiencies if available
		if len(apiRaceData.Proficiencies) > 0 {
			toolkitRaceData.WeaponProficiencies = apiRaceData.Proficiencies
			// TODO: Separate weapon/tool proficiencies when API provides distinction
		}
	}

	// Fetch class data
	var toolkitClassData *class.Data
	if classID != "" {
		apiClassData, err := o.externalClient.GetClassData(ctx, classID)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get class data for %s", classID)
		}
		toolkitClassData = &class.Data{
			ID:                    apiClassData.ID,
			Name:                  apiClassData.Name,
			HitDice:               int(apiClassData.HitDice),
			ArmorProficiencies:    apiClassData.ArmorProficiencies,
			WeaponProficiencies:   apiClassData.WeaponProficiencies,
			ToolProficiencies:     apiClassData.ToolProficiencies,
			SavingThrows:          make([]constants.Ability, len(apiClassData.SavingThrows)),
			SkillOptions:          make([]constants.Skill, len(apiClassData.AvailableSkills)),
			SkillProficiencyCount: int(apiClassData.SkillsCount),
		}
		
		// Convert saving throws to typed constants
		for i, st := range apiClassData.SavingThrows {
			toolkitClassData.SavingThrows[i] = constants.Ability(st)
		}
		
		// Convert skill options to typed constants
		for i, skill := range apiClassData.AvailableSkills {
			toolkitClassData.SkillOptions[i] = constants.Skill(skill)
		}
	}

	// Fetch background data if available
	var toolkitBackgroundData *shared.Background
	if backgroundID != "" {
		// Try to get background data from external client
		bgData, err := o.externalClient.GetBackgroundData(ctx, backgroundID)
		if err == nil && bgData != nil {
			toolkitBackgroundData = &shared.Background{
				ID:                 bgData.ID,
				Name:               bgData.Name,
				Description:        bgData.Description,
				SkillProficiencies: make([]constants.Skill, len(bgData.SkillProficiencies)),
				// TODO: Map languages and tool proficiencies when API provides them
			}
			
			// Convert skill proficiencies to typed constants
			for i, skill := range bgData.SkillProficiencies {
				toolkitBackgroundData.SkillProficiencies[i] = constants.Skill(skill)
			}
		} else {
			// Fallback to minimal data
			toolkitBackgroundData = &shared.Background{
				ID:   backgroundID,
				Name: backgroundID,
			}
		}
	}

	// Build choices map from typed fields for toolkit
	choices := make(map[string]any)
	
	// Add skill choices
	if len(draftData.SkillChoices) > 0 {
		choices["skills"] = draftData.SkillChoices
	}
	
	// Add language choices  
	if len(draftData.LanguageChoices) > 0 {
		choices["languages"] = draftData.LanguageChoices
	}
	
	// Add equipment choices
	if len(draftData.EquipmentChoices) > 0 {
		choices["equipment"] = draftData.EquipmentChoices
	}
	
	// Add fighting style if present
	if draftData.FightingStyleChoice != "" {
		choices["fighting_style"] = draftData.FightingStyleChoice
	}
	
	// Add spell/cantrip choices if present
	if len(draftData.SpellChoices) > 0 {
		choices["spells"] = draftData.SpellChoices
	}
	if len(draftData.CantripChoices) > 0 {
		choices["cantrips"] = draftData.CantripChoices
	}

	// Log the choices for debugging
	slog.InfoContext(ctx, "Using choices for character finalization",
		"draft_id", input.DraftID,
		"choices", choices)

	// Use CreationData to create the character
	creationData := character.CreationData{
		ID:             o.idGenerator.Generate(),
		PlayerID:       draftData.PlayerID,
		Name:           draftData.Name,
		RaceData:       toolkitRaceData,
		SubraceID:      subraceID,
		ClassData:      toolkitClassData,
		BackgroundData: toolkitBackgroundData,
		AbilityScores:  abilityScores,
		Choices:        choices,
	}

	// Create the character
	toolkitChar, err := character.NewFromCreationData(creationData)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to create character from creation data",
			"error", err.Error(),
			"raceID", raceID,
			"classID", classID,
			"backgroundID", backgroundID,
			"hasRaceData", toolkitRaceData != nil,
			"hasClassData", toolkitClassData != nil,
			"hasBackgroundData", toolkitBackgroundData != nil,
		)
		return nil, errors.Wrap(err, "failed to create character from draft")
	}

	// Get the character data from toolkit
	charData := toolkitChar.ToData()

	// Log what the toolkit produced
	slog.InfoContext(ctx, "Character data from toolkit",
		"character_id", charData.ID,
		"skills", charData.Skills,
		"languages", charData.Languages,
		"proficiencies", charData.Proficiencies,
		"ability_scores", fmt.Sprintf("%+v", charData.AbilityScores),
		"hp", charData.HitPoints,
		"max_hp", charData.MaxHitPoints,
		"choices_count", len(charData.Choices))

	// Note: The toolkit stores character choices but may not populate derived fields
	// like skills, languages, and proficiencies in charData. This is being verified.

	// Fetch the race, class and background info for stats calculation
	raceInfo, err := o.GetRaceDetails(ctx, &GetRaceDetailsInput{RaceID: raceID})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get race info for stats calculation")
	}

	classInfo, err := o.GetClassDetails(ctx, &GetClassDetailsInput{ClassID: classID})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get class info for stats calculation")
	}

	var backgroundInfo *dnd5e.BackgroundInfo
	if backgroundID != "" {
		bgDetails, err := o.GetBackgroundDetails(ctx, &GetBackgroundDetailsInput{BackgroundID: backgroundID})
		if err == nil && bgDetails != nil {
			backgroundInfo = bgDetails.Background
		}
	}

	// Convert to entity format for engine
	entityChar := &dnd5e.Character{
		ID:           charData.ID,
		Name:         charData.Name,
		Level:        int32(charData.Level),
		RaceID:       charData.RaceID,
		SubraceID:    charData.SubraceID,
		ClassID:      charData.ClassID,
		BackgroundID: charData.BackgroundID,
		AbilityScores: dnd5e.AbilityScores{
			Strength:     int32(charData.AbilityScores[constants.STR]),
			Dexterity:    int32(charData.AbilityScores[constants.DEX]),
			Constitution: int32(charData.AbilityScores[constants.CON]),
			Intelligence: int32(charData.AbilityScores[constants.INT]),
			Wisdom:       int32(charData.AbilityScores[constants.WIS]),
			Charisma:     int32(charData.AbilityScores[constants.CHA]),
		},
	}
	
	slog.InfoContext(ctx, "Entity character for stats calculation",
		"ability_scores", fmt.Sprintf("%+v", entityChar.AbilityScores))

	// Calculate character stats using the engine
	statsOutput, err := o.engine.CalculateCharacterStats(ctx, &engine.CalculateCharacterStatsInput{
		Character:  entityChar,
		Race:       raceInfo.Race,
		Subrace:    nil, // subraceInfo would need to be fetched separately if needed
		Class:      classInfo.Class,
		Background: backgroundInfo,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to calculate character stats")
	}

	// Update character data with calculated stats
	charData.HitPoints = int(statsOutput.MaxHP)
	charData.MaxHitPoints = int(statsOutput.MaxHP)

	// Fix timestamps if they're invalid
	if charData.CreatedAt.IsZero() {
		charData.CreatedAt = time.Now()
	}
	charData.UpdatedAt = time.Now()

	// Store the character data
	_, err = o.characterRepo.Create(ctx, characterrepo.CreateInput{
		CharacterData: &charData,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create character")
	}

	// Delete the draft
	draftDeleted := true
	deleteInput := draftrepo.DeleteInput{ID: input.DraftID}
	if _, err := o.characterDraftRepo.Delete(ctx, deleteInput); err != nil {
		// Log but don't fail - character was created successfully
		draftDeleted = false
		slog.Warn("Failed to delete draft after finalization",
			"draft_id", input.DraftID,
			"error", err)
	}

	// Return the toolkit character data directly
	return &FinalizeDraftOutput{
		CharacterData: &charData,
		DraftDeleted:  draftDeleted,
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

	// Load character from data
	// Note: This requires external data (race, class, background) which we don't have here
	// For now, we'll need to fetch that data
	charData := getOutput.CharacterData
	
	// Fetch required data for loading character
	// TODO: This is inefficient - consider caching or storing denormalized data
	raceData, err := o.externalClient.GetRaceData(ctx, charData.RaceID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get race data")
	}
	
	classData, err := o.externalClient.GetClassData(ctx, charData.ClassID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get class data")
	}
	
	backgroundData, err := o.externalClient.GetBackgroundData(ctx, charData.BackgroundID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get background data")
	}
	
	// Convert external data to toolkit format inline
	toolkitRaceData := &race.Data{
		ID:    raceData.ID,
		Name:  raceData.Name,
		Speed: int(raceData.Speed),
		Size:  raceData.Size,
		Languages: make([]constants.Language, len(raceData.Languages)),
		AbilityScoreIncreases: make(map[constants.Ability]int),
	}
	
	// Convert languages
	for i, lang := range raceData.Languages {
		toolkitRaceData.Languages[i] = constants.Language(lang)
	}
	
	// Convert ability bonuses
	for ability, bonus := range raceData.AbilityBonuses {
		toolkitRaceData.AbilityScoreIncreases[constants.Ability(ability)] = int(bonus)
	}
	
	// Map weapon proficiencies if available
	if len(raceData.Proficiencies) > 0 {
		toolkitRaceData.WeaponProficiencies = raceData.Proficiencies
	}
	
	toolkitClassData := &class.Data{
		ID:                    classData.ID,
		Name:                  classData.Name,
		HitDice:               int(classData.HitDice),
		ArmorProficiencies:    classData.ArmorProficiencies,
		WeaponProficiencies:   classData.WeaponProficiencies,
		ToolProficiencies:     classData.ToolProficiencies,
		SavingThrows:          make([]constants.Ability, len(classData.SavingThrows)),
		SkillOptions:          make([]constants.Skill, len(classData.AvailableSkills)),
		SkillProficiencyCount: int(classData.SkillsCount),
	}
	
	// Convert saving throws
	for i, st := range classData.SavingThrows {
		toolkitClassData.SavingThrows[i] = constants.Ability(st)
	}
	
	// Convert skill options
	for i, skill := range classData.AvailableSkills {
		toolkitClassData.SkillOptions[i] = constants.Skill(skill)
	}
	
	toolkitBackgroundData := &shared.Background{
		ID:                 backgroundData.ID,
		Name:               backgroundData.Name,
		Description:        backgroundData.Description,
		SkillProficiencies: make([]constants.Skill, len(backgroundData.SkillProficiencies)),
	}
	
	// Convert skill proficiencies
	for i, skill := range backgroundData.SkillProficiencies {
		toolkitBackgroundData.SkillProficiencies[i] = constants.Skill(skill)
	}
	
	// Load character with external data
	char, err := character.LoadCharacterFromData(*charData, toolkitRaceData, toolkitClassData, toolkitBackgroundData)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load character")
	}

	return &GetCharacterOutput{
		Character: char,
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
	var characterDataList []*character.Data
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
		characterDataList = listOutput.Characters
		slog.InfoContext(ctx, "successfully listed characters by player",
			"player_id", input.PlayerID,
			"count", len(characterDataList))
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
		characterDataList = listOutput.Characters
		slog.InfoContext(ctx, "successfully listed characters by session",
			"session_id", input.SessionID,
			"count", len(characterDataList))
	default:
		log.Printf("ListCharacters called without PlayerID or SessionID")
		return nil, errors.InvalidArgument("either PlayerID or SessionID must be provided")
	}

	// Convert character.Data list to toolkit Characters
	// Note: This requires fetching external data for each character, which is inefficient
	// TODO: Consider returning just the Data and letting the handler decide what to load
	characters := make([]*character.Character, 0, len(characterDataList))
	
	for _, charData := range characterDataList {
		// Skip characters we can't load (missing data, etc.)
		// In a real implementation, we might want to batch fetch the external data
		slog.WarnContext(ctx, "Skipping character load - need external data",
			"character_id", charData.ID,
			"reason", "ListCharacters needs optimization to avoid N+1 fetches")
		// For now, we'll return empty list to avoid performance issues
		// The handler can decide to fetch individual characters as needed
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

// Helper functions

// isValidChoiceCategory validates if a choice category is recognized
func isValidChoiceCategory(category shared.ChoiceCategory) bool {
	// Check predefined categories
	switch category {
	case shared.ChoiceName,
		shared.ChoiceRace,
		shared.ChoiceSubrace,
		shared.ChoiceClass,
		shared.ChoiceBackground,
		shared.ChoiceAbilityScores,
		shared.ChoiceSkills,
		shared.ChoiceLanguages,
		shared.ChoiceEquipment,
		shared.ChoiceSpells:
		return true
	}

	// Check dynamic categories with known prefixes
	categoryStr := string(category)
	if strings.HasPrefix(categoryStr, "race_") ||
		strings.HasPrefix(categoryStr, "class_") ||
		strings.HasPrefix(categoryStr, "background_") {
		return true
	}

	return false
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
	if race == nil {
		return nil
	}

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
	if class == nil {
		return nil
	}

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
			CantripsKnown:       class.Spellcasting.CantripsKnown,
			SpellsKnown:         class.Spellcasting.SpellsKnown,
			SpellSlotsLevel1:    class.Spellcasting.SpellSlotsLevel1,
		}
	}

	// Note: Proficiency choices are now handled through the rich Choices field
	// This conversion is kept for backward compatibility with deprecated fields

	return &dnd5e.ClassInfo{
		ID:                       class.ID,
		Name:                     class.Name,
		Description:              class.Description,
		HitDie:                   fmt.Sprintf("1d%d", class.HitDice),
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

// UpdateChoices updates the choices for a character draft
func (o *Orchestrator) UpdateChoices(
	ctx context.Context,
	input *UpdateChoicesInput,
) (*UpdateChoicesOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}

	vb := errors.NewValidationBuilder()
	errors.ValidateRequired("draftID", input.DraftID, vb)
	if err := vb.Build(); err != nil {
		return nil, err
	}

	slog.InfoContext(ctx, "Updating character choices",
		"draft_id", input.DraftID,
		"choice_type", input.ChoiceType,
		"selections", len(input.Selections))

	// Get the draft from repository
	getOutput, err := o.characterDraftRepo.Get(ctx, draftrepo.GetInput{ID: input.DraftID})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get draft")
	}

	// Load into builder
	builder, err := character.LoadDraft(*getOutput.Draft)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load draft into builder")
	}

	// Apply choice to the appropriate field based on choice type
	switch input.ChoiceType {
	case "skills":
		// Use the builder's SelectSkills method
		if err := builder.SelectSkills(input.Selections); err != nil {
			return nil, errors.Wrap(err, "failed to set skills")
		}
		
	case "languages":
		// Use the builder's SelectLanguages method
		if err := builder.SelectLanguages(input.Selections); err != nil {
			return nil, errors.Wrap(err, "failed to set languages")
		}
		
	case "fighting_style":
		if len(input.Selections) > 0 {
			// Use the builder's SelectFightingStyle method
			if err := builder.SelectFightingStyle(input.Selections[0]); err != nil {
				return nil, errors.Wrap(err, "failed to set fighting style")
			}
		}
		
	case "cantrips":
		// Use the builder's SelectCantrips method
		if err := builder.SelectCantrips(input.Selections); err != nil {
			return nil, errors.Wrap(err, "failed to set cantrips")
		}
		
	case "spells":
		// Use the builder's SelectSpells method
		if err := builder.SelectSpells(input.Selections); err != nil {
			return nil, errors.Wrap(err, "failed to set spells")
		}
		
	case "equipment":
		// Use the builder's SelectEquipment method
		if err := builder.SelectEquipment(input.Selections); err != nil {
			return nil, errors.Wrap(err, "failed to set equipment")
		}
		
	default:
		return nil, errors.InvalidArgumentf("unknown choice type: %s", input.ChoiceType)
	}
	
	// Get the updated draft data
	draftData := builder.ToData()

	// Save the updated draft to repository
	updateOutput, err := o.characterDraftRepo.Update(ctx, draftrepo.UpdateInput{
		Draft: &draftData,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to update draft in repository")
	}

	// Load the updated draft
	updatedDraft, err := character.LoadDraftFromData(*updateOutput.Draft)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load draft")
	}

	slog.InfoContext(ctx, "Successfully updated character choices",
		"draft_id", updatedDraft.ID,
		"choice_type", input.ChoiceType)

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
	if draft.ClassChoice == "" {
		return nil, errors.InvalidArgument("class must be selected before viewing choice options")
	}

	// Get available choice categories based on the draft's class
	categories, err := o.getAvailableChoiceCategories(ctx, draft.ClassChoice, input.ChoiceType)
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

// applyChoiceSelections applies validated selections to the draft

// getAvailableChoiceCategories returns the choice categories available for a draft
//
//nolint:unparam // error return kept for future extensibility when class/race details are used
func (o *Orchestrator) getAvailableChoiceCategories(
	ctx context.Context,
	classChoice string,
	filterType *dnd5e.ChoiceType,
) ([]*dnd5e.ChoiceCategory, error) {
	var categories []*dnd5e.ChoiceCategory

	// Helper function to check if choice type should be included
	shouldIncludeChoiceType := func(choiceType dnd5e.ChoiceType) bool {
		return filterType == nil || *filterType == choiceType
	}

	// Add class-specific choices based on class ID
	switch classChoice {
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

// hasPrerequisite checks if a draft meets a prerequisite

// hasConflictingChoice checks if a draft has a conflicting choice

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
			return nil, fmt.Errorf("failed to get race data for %s: %w", draft.RaceID, err)
		}
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

	// Fetch class info if class is set
	if draft.ClassID != "" {
		classData, err := o.externalClient.GetClassData(ctx, draft.ClassID)
		if err != nil {
			return nil, fmt.Errorf("failed to get class data for %s: %w", draft.ClassID, err)
		}
		hydratedDraft.Class = convertExternalClassToEntity(classData)
	}

	// Fetch background info if background is set
	if draft.BackgroundID != "" {
		backgroundData, err := o.externalClient.GetBackgroundData(ctx, draft.BackgroundID)
		if err != nil {
			return nil, fmt.Errorf("failed to get background data for %s: %w", draft.BackgroundID, err)
		}
		hydratedDraft.Background = convertExternalBackgroundToEntity(backgroundData)
	}

	return &hydratedDraft, nil
}

// convertDraftDataToCharacterDraft converts toolkit DraftData to API CharacterDraft
func (o *Orchestrator) convertDraftDataToCharacterDraft(ctx context.Context, data *character.DraftData) (*dnd5e.CharacterDraft, error) {
	// Add panic recovery to catch any conversion errors
	defer func() {
		if r := recover(); r != nil {
			slog.ErrorContext(ctx, "Panic in convertDraftDataToCharacterDraft",
				"panic", r,
				"draft_id", data.ID,
				"draft_data", fmt.Sprintf("%+v", data))
		}
	}()

	// Load the builder to get progress information
	builder, err := character.LoadDraft(*data)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load draft into builder")
	}

	// Get progress from builder
	progress := builder.Progress()

	// Create the CharacterDraft
	draft := &dnd5e.CharacterDraft{
		ID:       data.ID,
		PlayerID: data.PlayerID,
		Name:     data.Name,
		Progress: dnd5e.CreationProgress{
			CurrentStep:          progress.CurrentStep,
			CompletionPercentage: int32(progress.PercentComplete),
		},
		CreatedAt: data.CreatedAt.Unix(),
		UpdatedAt: data.UpdatedAt.Unix(),
	}

	// Extract data from explicit typed fields
	// Race
	draft.RaceID = data.RaceChoice.RaceID
	draft.SubraceID = data.RaceChoice.SubraceID

	// Class
	draft.ClassID = data.ClassChoice

	// Background
	draft.BackgroundID = data.BackgroundChoice

	// Ability Scores
	scores := data.AbilityScoreChoice
	draft.AbilityScores = &dnd5e.AbilityScores{
		Strength:     int32(scores[constants.STR]),
		Dexterity:    int32(scores[constants.DEX]),
		Constitution: int32(scores[constants.CON]),
		Intelligence: int32(scores[constants.INT]),
		Wisdom:       int32(scores[constants.WIS]),
		Charisma:     int32(scores[constants.CHA]),
	}


	// Extract choices from typed fields
	draft.ChoiceSelections = []dnd5e.ChoiceSelection{}
	
	// Skills
	if len(data.SkillChoices) > 0 {
		draft.ChoiceSelections = append(draft.ChoiceSelections, dnd5e.ChoiceSelection{
			ChoiceID:     "skill_proficiencies",
			Source:       dnd5e.ChoiceSourceClass,
			SelectedKeys: data.SkillChoices,
		})
	}
	
	// Languages
	if len(data.LanguageChoices) > 0 {
		draft.ChoiceSelections = append(draft.ChoiceSelections, dnd5e.ChoiceSelection{
			ChoiceID:     "language_choices",
			Source:       dnd5e.ChoiceSourceRace,
			SelectedKeys: data.LanguageChoices,
		})
	}
	
	// Fighting Style
	if data.FightingStyleChoice != "" {
		draft.ChoiceSelections = append(draft.ChoiceSelections, dnd5e.ChoiceSelection{
			ChoiceID:     "fighting_style",
			Source:       dnd5e.ChoiceSourceClass,
			SelectedKeys: []string{data.FightingStyleChoice},
		})
	}
	
	// Spells
	if len(data.SpellChoices) > 0 {
		draft.ChoiceSelections = append(draft.ChoiceSelections, dnd5e.ChoiceSelection{
			ChoiceID:     "spell_choices",
			Source:       dnd5e.ChoiceSourceClass,
			SelectedKeys: data.SpellChoices,
		})
	}
	
	// Cantrips
	if len(data.CantripChoices) > 0 {
		draft.ChoiceSelections = append(draft.ChoiceSelections, dnd5e.ChoiceSelection{
			ChoiceID:     "cantrip_choices",
			Source:       dnd5e.ChoiceSourceClass,
			SelectedKeys: data.CantripChoices,
		})
	}
	
	// Equipment
	if len(data.EquipmentChoices) > 0 {
		draft.ChoiceSelections = append(draft.ChoiceSelections, dnd5e.ChoiceSelection{
			ChoiceID:     "equipment_choices",
			Source:       dnd5e.ChoiceSourceClass,
			SelectedKeys: data.EquipmentChoices,
		})
	}

	// Hydrate with external data if we have race/class/background
	if draft.RaceID != "" || draft.ClassID != "" || draft.BackgroundID != "" {
		return o.hydrateDraft(ctx, draft)
	}

	return draft, nil
}

// GetInventory retrieves a character's equipment and inventory
func (o *Orchestrator) GetInventory(ctx context.Context, input *GetInventoryInput) (*GetInventoryOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}
	if input.CharacterID == "" {
		return nil, errors.InvalidArgument("character ID is required")
	}

	// First verify the character exists
	_, err := o.characterRepo.Get(ctx, characterrepo.GetInput{
		ID: input.CharacterID,
	})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, errors.NotFoundf("character with ID %s not found", input.CharacterID)
		}
		return nil, errors.Wrapf(err, "failed to get character")
	}

	// Get equipment data
	equipOutput, err := o.equipmentRepo.Get(ctx, equipmentrepo.GetInput{
		CharacterID: input.CharacterID,
	})
	if err != nil {
		// If equipment not found, return empty equipment
		if errors.IsNotFound(err) {
			return &GetInventoryOutput{
				EquipmentSlots:      &dnd5e.EquipmentSlots{},
				Inventory:           []dnd5e.InventoryItem{},
				Encumbrance:         &dnd5e.EncumbranceInfo{},
				AttunementSlotsUsed: 0,
				AttunementSlotsMax:  3, // D&D 5e default
			}, nil
		}
		return nil, errors.Wrapf(err, "failed to get equipment")
	}

	// Calculate attunement slots used
	attunementUsed := 0
	// Check equipped items
	if equipOutput.EquipmentSlots != nil {
		slots := []*dnd5e.InventoryItem{
			equipOutput.EquipmentSlots.MainHand,
			equipOutput.EquipmentSlots.OffHand,
			equipOutput.EquipmentSlots.Armor,
			equipOutput.EquipmentSlots.Helmet,
			equipOutput.EquipmentSlots.Boots,
			equipOutput.EquipmentSlots.Gloves,
			equipOutput.EquipmentSlots.Cloak,
			equipOutput.EquipmentSlots.Amulet,
			equipOutput.EquipmentSlots.Ring1,
			equipOutput.EquipmentSlots.Ring2,
			equipOutput.EquipmentSlots.Belt,
		}
		for _, item := range slots {
			if item != nil && item.IsAttuned {
				attunementUsed++
			}
		}
	}
	// Check inventory items
	for _, item := range equipOutput.Inventory {
		if item.IsAttuned {
			attunementUsed++
		}
	}

	return &GetInventoryOutput{
		EquipmentSlots:      equipOutput.EquipmentSlots,
		Inventory:           equipOutput.Inventory,
		Encumbrance:         equipOutput.Encumbrance,
		AttunementSlotsUsed: int32(attunementUsed),
		AttunementSlotsMax:  3, // D&D 5e default
	}, nil
}

// EquipItem equips an item from inventory to a specific slot
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

	// First verify the character exists
	_, err := o.characterRepo.Get(ctx, characterrepo.GetInput{
		ID: input.CharacterID,
	})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, errors.NotFoundf("character with ID %s not found", input.CharacterID)
		}
		return nil, errors.Wrapf(err, "failed to get character")
	}

	// Get current equipment
	equipOutput, err := o.equipmentRepo.Get(ctx, equipmentrepo.GetInput{
		CharacterID: input.CharacterID,
	})
	if err != nil && !errors.IsNotFound(err) {
		return nil, errors.Wrapf(err, "failed to get equipment")
	}

	// Initialize equipment if not found
	if errors.IsNotFound(err) {
		equipOutput = &equipmentrepo.GetOutput{
			CharacterID:    input.CharacterID,
			EquipmentSlots: &dnd5e.EquipmentSlots{},
			Inventory:      []dnd5e.InventoryItem{},
			Encumbrance:    &dnd5e.EncumbranceInfo{},
		}
	}

	// Find the item in inventory
	itemIndex := -1
	var itemToEquip dnd5e.InventoryItem
	for i, item := range equipOutput.Inventory {
		if item.ItemID == input.ItemID {
			itemIndex = i
			itemToEquip = item
			break
		}
	}

	if itemIndex == -1 {
		return nil, errors.NotFoundf("item %s not found in inventory", input.ItemID)
	}

	// Get equipment data for the item
	equipData, err := o.externalClient.GetEquipmentData(ctx, itemToEquip.ItemID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get equipment data")
	}

	// TODO: Add equipment validation using rpg-toolkit when available
	// For now, just do basic slot validation based on equipment type
	if !isValidSlotForEquipment(equipData, input.Slot) {
		return nil, errors.InvalidArgumentf("item cannot be equipped in slot %s", input.Slot)
	}

	// Initialize equipment slots if needed
	if equipOutput.EquipmentSlots == nil {
		equipOutput.EquipmentSlots = &dnd5e.EquipmentSlots{}
	}

	// Get current item in slot (if any) and move to inventory
	var currentItem *dnd5e.InventoryItem
	switch input.Slot {
	case dnd5e.EquipmentSlotMainHand:
		currentItem = equipOutput.EquipmentSlots.MainHand
	case dnd5e.EquipmentSlotOffHand:
		currentItem = equipOutput.EquipmentSlots.OffHand
	case dnd5e.EquipmentSlotArmor:
		currentItem = equipOutput.EquipmentSlots.Armor
	case dnd5e.EquipmentSlotHelmet:
		currentItem = equipOutput.EquipmentSlots.Helmet
	case dnd5e.EquipmentSlotBoots:
		currentItem = equipOutput.EquipmentSlots.Boots
	case dnd5e.EquipmentSlotGloves:
		currentItem = equipOutput.EquipmentSlots.Gloves
	case dnd5e.EquipmentSlotCloak:
		currentItem = equipOutput.EquipmentSlots.Cloak
	case dnd5e.EquipmentSlotAmulet:
		currentItem = equipOutput.EquipmentSlots.Amulet
	case dnd5e.EquipmentSlotRing1:
		currentItem = equipOutput.EquipmentSlots.Ring1
	case dnd5e.EquipmentSlotRing2:
		currentItem = equipOutput.EquipmentSlots.Ring2
	case dnd5e.EquipmentSlotBelt:
		currentItem = equipOutput.EquipmentSlots.Belt
	default:
		return nil, errors.InvalidArgumentf("invalid slot: %s", input.Slot)
	}

	// Remove item from inventory
	newInventory := make([]dnd5e.InventoryItem, 0, len(equipOutput.Inventory))
	for i, item := range equipOutput.Inventory {
		if i != itemIndex {
			newInventory = append(newInventory, item)
		}
	}

	// Add current slot item to inventory if exists
	if currentItem != nil {
		newInventory = append(newInventory, *currentItem)
	}

	// Convert equipment data to entity format
	itemToEquip.Equipment = convertExternalEquipmentToEntity(equipData)
	equippedItem := &itemToEquip
	switch input.Slot {
	case dnd5e.EquipmentSlotMainHand:
		equipOutput.EquipmentSlots.MainHand = equippedItem
	case dnd5e.EquipmentSlotOffHand:
		equipOutput.EquipmentSlots.OffHand = equippedItem
	case dnd5e.EquipmentSlotArmor:
		equipOutput.EquipmentSlots.Armor = equippedItem
	case dnd5e.EquipmentSlotHelmet:
		equipOutput.EquipmentSlots.Helmet = equippedItem
	case dnd5e.EquipmentSlotBoots:
		equipOutput.EquipmentSlots.Boots = equippedItem
	case dnd5e.EquipmentSlotGloves:
		equipOutput.EquipmentSlots.Gloves = equippedItem
	case dnd5e.EquipmentSlotCloak:
		equipOutput.EquipmentSlots.Cloak = equippedItem
	case dnd5e.EquipmentSlotAmulet:
		equipOutput.EquipmentSlots.Amulet = equippedItem
	case dnd5e.EquipmentSlotRing1:
		equipOutput.EquipmentSlots.Ring1 = equippedItem
	case dnd5e.EquipmentSlotRing2:
		equipOutput.EquipmentSlots.Ring2 = equippedItem
	case dnd5e.EquipmentSlotBelt:
		equipOutput.EquipmentSlots.Belt = equippedItem
	}

	// Update equipment in repository
	_, err = o.equipmentRepo.Update(ctx, equipmentrepo.UpdateInput{
		CharacterID:    input.CharacterID,
		EquipmentSlots: equipOutput.EquipmentSlots,
		Inventory:      newInventory,
		Encumbrance:    equipOutput.Encumbrance, // TODO: Recalculate encumbrance
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to update equipment")
	}

	return &EquipItemOutput{
		Success: true,
	}, nil
}

// UnequipItem unequips an item from a specific slot
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

	// First verify the character exists
	_, err := o.characterRepo.Get(ctx, characterrepo.GetInput{
		ID: input.CharacterID,
	})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, errors.NotFoundf("character with ID %s not found", input.CharacterID)
		}
		return nil, errors.Wrapf(err, "failed to get character")
	}

	// Get current equipment
	equipOutput, err := o.equipmentRepo.Get(ctx, equipmentrepo.GetInput{
		CharacterID: input.CharacterID,
	})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, errors.NotFoundf("no equipment found for character")
		}
		return nil, errors.Wrapf(err, "failed to get equipment")
	}

	if equipOutput.EquipmentSlots == nil {
		return nil, errors.NotFoundf("no item equipped in slot %s", input.Slot)
	}

	// Get item from slot
	var itemToUnequip *dnd5e.InventoryItem
	switch input.Slot {
	case dnd5e.EquipmentSlotMainHand:
		itemToUnequip = equipOutput.EquipmentSlots.MainHand
		equipOutput.EquipmentSlots.MainHand = nil
	case dnd5e.EquipmentSlotOffHand:
		itemToUnequip = equipOutput.EquipmentSlots.OffHand
		equipOutput.EquipmentSlots.OffHand = nil
	case dnd5e.EquipmentSlotArmor:
		itemToUnequip = equipOutput.EquipmentSlots.Armor
		equipOutput.EquipmentSlots.Armor = nil
	case dnd5e.EquipmentSlotHelmet:
		itemToUnequip = equipOutput.EquipmentSlots.Helmet
		equipOutput.EquipmentSlots.Helmet = nil
	case dnd5e.EquipmentSlotBoots:
		itemToUnequip = equipOutput.EquipmentSlots.Boots
		equipOutput.EquipmentSlots.Boots = nil
	case dnd5e.EquipmentSlotGloves:
		itemToUnequip = equipOutput.EquipmentSlots.Gloves
		equipOutput.EquipmentSlots.Gloves = nil
	case dnd5e.EquipmentSlotCloak:
		itemToUnequip = equipOutput.EquipmentSlots.Cloak
		equipOutput.EquipmentSlots.Cloak = nil
	case dnd5e.EquipmentSlotAmulet:
		itemToUnequip = equipOutput.EquipmentSlots.Amulet
		equipOutput.EquipmentSlots.Amulet = nil
	case dnd5e.EquipmentSlotRing1:
		itemToUnequip = equipOutput.EquipmentSlots.Ring1
		equipOutput.EquipmentSlots.Ring1 = nil
	case dnd5e.EquipmentSlotRing2:
		itemToUnequip = equipOutput.EquipmentSlots.Ring2
		equipOutput.EquipmentSlots.Ring2 = nil
	case dnd5e.EquipmentSlotBelt:
		itemToUnequip = equipOutput.EquipmentSlots.Belt
		equipOutput.EquipmentSlots.Belt = nil
	default:
		return nil, errors.InvalidArgumentf("invalid slot: %s", input.Slot)
	}

	if itemToUnequip == nil {
		return nil, errors.NotFoundf("no item equipped in slot %s", input.Slot)
	}

	// Add unequipped item to inventory
	newInventory := append(equipOutput.Inventory, *itemToUnequip)

	// Update equipment in repository
	_, err = o.equipmentRepo.Update(ctx, equipmentrepo.UpdateInput{
		CharacterID:    input.CharacterID,
		EquipmentSlots: equipOutput.EquipmentSlots,
		Inventory:      newInventory,
		Encumbrance:    equipOutput.Encumbrance, // TODO: Recalculate encumbrance
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to update equipment")
	}

	return &UnequipItemOutput{
		Success: true,
	}, nil
}

// AddToInventory adds an item to the character's inventory
func (o *Orchestrator) AddToInventory(ctx context.Context, input *AddToInventoryInput) (*AddToInventoryOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}
	if input.CharacterID == "" {
		return nil, errors.InvalidArgument("character ID is required")
	}
	if len(input.Items) == 0 {
		return nil, errors.InvalidArgument("at least one item is required")
	}

	// First verify the character exists
	_, err := o.characterRepo.Get(ctx, characterrepo.GetInput{
		ID: input.CharacterID,
	})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, errors.NotFoundf("character with ID %s not found", input.CharacterID)
		}
		return nil, errors.Wrapf(err, "failed to get character")
	}

	// Get current equipment
	equipOutput, err := o.equipmentRepo.Get(ctx, equipmentrepo.GetInput{
		CharacterID: input.CharacterID,
	})
	if err != nil && !errors.IsNotFound(err) {
		return nil, errors.Wrapf(err, "failed to get equipment")
	}

	// Initialize equipment if not found
	if errors.IsNotFound(err) {
		equipOutput = &equipmentrepo.GetOutput{
			CharacterID:    input.CharacterID,
			EquipmentSlots: &dnd5e.EquipmentSlots{},
			Inventory:      []dnd5e.InventoryItem{},
			Encumbrance:    &dnd5e.EncumbranceInfo{},
		}
	}

	// Copy existing inventory
	newInventory := make([]dnd5e.InventoryItem, len(equipOutput.Inventory))
	copy(newInventory, equipOutput.Inventory)

	// Process each item to add
	for _, addition := range input.Items {
		if addition.Item == nil {
			return nil, errors.InvalidArgument("item is required")
		}
		if addition.Item.ItemID == "" {
			return nil, errors.InvalidArgument("item ID is required")
		}
		if addition.Item.Quantity <= 0 {
			return nil, errors.InvalidArgument("quantity must be positive")
		}

		// Get equipment data for the item if not already present
		if addition.Item.Equipment == nil {
			equipData, err := o.externalClient.GetEquipmentData(ctx, addition.Item.ItemID)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to get equipment data for item %s", addition.Item.ItemID)
			}
			addition.Item.Equipment = convertExternalEquipmentToEntity(equipData)
		}

		// Check if item already exists in inventory
		found := false
		for i, item := range newInventory {
			if item.ItemID == addition.Item.ItemID {
				// Stack if stackable
				if addition.Item.Equipment != nil && addition.Item.Equipment.Stackable {
					newInventory[i].Quantity += addition.Item.Quantity
					found = true
					break
				}
			}
		}

		// Add new item if not found
		if !found {
			newInventory = append(newInventory, *addition.Item)
		}
	}

	// Update equipment in repository
	_, err = o.equipmentRepo.Update(ctx, equipmentrepo.UpdateInput{
		CharacterID:    input.CharacterID,
		EquipmentSlots: equipOutput.EquipmentSlots,
		Inventory:      newInventory,
		Encumbrance:    equipOutput.Encumbrance, // TODO: Recalculate encumbrance
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to update equipment")
	}

	return &AddToInventoryOutput{
		Success: true,
	}, nil
}

// RemoveFromInventory removes an item from the character's inventory
func (o *Orchestrator) RemoveFromInventory(
	ctx context.Context,
	input *RemoveFromInventoryInput,
) (*RemoveFromInventoryOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}
	if input.CharacterID == "" {
		return nil, errors.InvalidArgument("character ID is required")
	}
	if input.ItemID == "" {
		return nil, errors.InvalidArgument("item ID is required")
	}

	// First verify the character exists
	_, err := o.characterRepo.Get(ctx, characterrepo.GetInput{
		ID: input.CharacterID,
	})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, errors.NotFoundf("character with ID %s not found", input.CharacterID)
		}
		return nil, errors.Wrapf(err, "failed to get character")
	}

	// Get current equipment
	equipOutput, err := o.equipmentRepo.Get(ctx, equipmentrepo.GetInput{
		CharacterID: input.CharacterID,
	})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, errors.NotFoundf("no equipment found for character")
		}
		return nil, errors.Wrapf(err, "failed to get equipment")
	}

	// Find and remove item from inventory
	itemFound := false
	removedQuantity := int32(0)
	newInventory := make([]dnd5e.InventoryItem, 0, len(equipOutput.Inventory))

	for _, item := range equipOutput.Inventory {
		if item.ItemID == input.ItemID {
			itemFound = true
			if input.Quantity > 0 && input.Quantity < item.Quantity {
				// Remove partial quantity
				item.Quantity -= input.Quantity
				removedQuantity = input.Quantity
				newInventory = append(newInventory, item)
			} else {
				// Remove entire item
				removedQuantity = item.Quantity
				// Don't add to new inventory
			}
		} else {
			// Keep other items
			newInventory = append(newInventory, item)
		}
	}

	if !itemFound {
		return nil, errors.NotFoundf("item %s not found in inventory", input.ItemID)
	}

	// Update equipment in repository
	_, err = o.equipmentRepo.Update(ctx, equipmentrepo.UpdateInput{
		CharacterID:    input.CharacterID,
		EquipmentSlots: equipOutput.EquipmentSlots,
		Inventory:      newInventory,
		Encumbrance:    equipOutput.Encumbrance, // TODO: Recalculate encumbrance
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to update equipment")
	}

	return &RemoveFromInventoryOutput{
		Success:         true,
		QuantityRemoved: removedQuantity,
	}, nil
}

// isValidSlotForEquipment checks if an item can be equipped in the given slot
func isValidSlotForEquipment(equipData *external.EquipmentData, slot string) bool {
	switch equipData.EquipmentType {
	case "weapon":
		return slot == dnd5e.EquipmentSlotMainHand || slot == dnd5e.EquipmentSlotOffHand
	case "armor":
		if equipData.ArmorCategory == "shield" {
			return slot == dnd5e.EquipmentSlotOffHand
		}
		return slot == dnd5e.EquipmentSlotArmor
	case "gear":
		// TODO: More specific gear slot validation
		// For now, allow gear in various accessory slots
		switch slot {
		case dnd5e.EquipmentSlotHelmet,
			dnd5e.EquipmentSlotBoots,
			dnd5e.EquipmentSlotGloves,
			dnd5e.EquipmentSlotCloak,
			dnd5e.EquipmentSlotAmulet,
			dnd5e.EquipmentSlotRing1,
			dnd5e.EquipmentSlotRing2,
			dnd5e.EquipmentSlotBelt:
			return true
		}
	}
	return false
}

// convertExternalEquipmentToEntity converts external API equipment data to our entity format
func convertExternalEquipmentToEntity(equipData *external.EquipmentData) *dnd5e.EquipmentData {
	if equipData == nil {
		return nil
	}

	result := &dnd5e.EquipmentData{
		Name:       equipData.Name,
		Type:       equipData.EquipmentType,
		Weight:     int32(equipData.Weight * 10), // Convert to tenths of pounds
		Properties: equipData.Properties,
		Stackable:  isStackableEquipment(equipData),
	}

	// Convert weapon data
	if equipData.EquipmentType == "weapon" && equipData.Damage != nil {
		result.WeaponData = &dnd5e.WeaponData{
			WeaponCategory: strings.ToLower(equipData.WeaponCategory),
			Range:          strings.ToLower(equipData.WeaponRange),
			DamageDice:     equipData.Damage.DamageDice,
			DamageType:     equipData.Damage.DamageType,
			Properties:     equipData.Properties,
		}
	}

	// Convert armor data
	if equipData.EquipmentType == "armor" && equipData.ArmorClass != nil {
		result.ArmorData = &dnd5e.ArmorData{
			ArmorCategory:       strings.ToLower(equipData.ArmorCategory),
			BaseAC:              int32(equipData.ArmorClass.Base),
			HasDexLimit:         !equipData.ArmorClass.DexBonus, // If no dex bonus, there's a limit
			MaxDexBonus:         0,                              // Default to 0 if limited (heavy armor)
			StrMinimum:          int32(equipData.StrengthMinimum),
			StealthDisadvantage: equipData.StealthDisadvantage,
		}

		// Set max dex bonus based on armor category
		if equipData.ArmorClass.DexBonus {
			if strings.ToLower(equipData.ArmorCategory) == "medium" {
				result.ArmorData.MaxDexBonus = 2 // Medium armor cap
			} else {
				result.ArmorData.HasDexLimit = false // Light armor has no limit
			}
		}
	}

	// Convert gear data
	if equipData.EquipmentType == "gear" && equipData.Cost != nil {
		result.GearData = &dnd5e.GearData{
			CostInCopper: int32(equipData.Cost.Quantity * getCopperMultiplier(equipData.Cost.Unit)),
		}
	}

	return result
}

// isStackableEquipment determines if equipment should be stackable
func isStackableEquipment(equipData *external.EquipmentData) bool {
	// Weapons and armor are typically not stackable
	if equipData.EquipmentType == "weapon" || equipData.EquipmentType == "armor" {
		return false
	}

	// Some gear items are stackable (arrows, potions, etc.)
	// This is a simplified check - in reality, we'd need more detailed item data
	name := strings.ToLower(equipData.Name)
	return strings.Contains(name, "arrow") ||
		strings.Contains(name, "bolt") ||
		strings.Contains(name, "potion") ||
		strings.Contains(name, "torch") ||
		strings.Contains(name, "ration")
}

// getCopperMultiplier returns the multiplier to convert currency to copper pieces
func getCopperMultiplier(unit string) int {
	switch strings.ToLower(unit) {
	case "cp":
		return 1
	case "sp":
		return 10
	case "ep":
		return 50
	case "gp":
		return 100
	case "pp":
		return 1000
	default:
		return 1
	}
}

// formatChoicesForLog creates a log-friendly representation of choices
func formatChoicesForLog(choices map[shared.ChoiceCategory]any) map[string]any {
	formatted := make(map[string]any)
	for k, v := range choices {
		formatted[string(k)] = v
	}
	return formatted
}

// mapChoiceTypeToStandardCategory maps a choice type string to a standard shared.ChoiceCategory
func mapChoiceTypeToStandardCategory(choiceType string) shared.ChoiceCategory {
	// Handle exact matches first
	switch choiceType {
	case "skill":
		return shared.ChoiceSkills
	case "language":
		return shared.ChoiceLanguages
	case "equipment":
		return shared.ChoiceEquipment
	case "spell":
		return shared.ChoiceSpells
	case "cantrips":
		return shared.ChoiceCantrips
	case "fighting_style":
		return shared.ChoiceFightingStyle
	case "tool", "weapon_proficiency", "armor_proficiency":
		// Tools and proficiencies don't have a direct standard category in toolkit yet
		// For now, map to a generic category that can be processed later
		return shared.ChoiceCategory("proficiencies")
	}

	// Fallback to keyword-based matching
	lowerType := strings.ToLower(choiceType)
	switch {
	case strings.Contains(lowerType, "skill"):
		return shared.ChoiceSkills
	case strings.Contains(lowerType, "language"):
		return shared.ChoiceLanguages
	case strings.Contains(lowerType, "equipment") || strings.Contains(lowerType, "gear"):
		return shared.ChoiceEquipment
	case strings.Contains(lowerType, "spell") && !strings.Contains(lowerType, "cantrip"):
		return shared.ChoiceSpells
	case strings.Contains(lowerType, "cantrip"):
		return shared.ChoiceCantrips
	case strings.Contains(lowerType, "fighting") && strings.Contains(lowerType, "style"):
		return shared.ChoiceFightingStyle
	case strings.Contains(lowerType, "tool") || strings.Contains(lowerType, "proficienc"):
		return shared.ChoiceCategory("proficiencies")
	default:
		// If we can't map it, use the type as-is
		return shared.ChoiceCategory(choiceType)
	}
}

// getExistingChoicesForCategory extracts existing ChoiceData entries for a category
func getExistingChoicesForCategory(choices map[shared.ChoiceCategory]any, category shared.ChoiceCategory) []character.ChoiceData {
	if existing, ok := choices[category]; ok {
		if choiceDataList, ok := existing.([]character.ChoiceData); ok {
			return choiceDataList
		}
	}
	return []character.ChoiceData{}
}

// mapStandardCategoryToChoiceType maps a standard category back to API ChoiceType
func mapStandardCategoryToChoiceType(category string) dnd5e.ChoiceType {
	switch category {
	case "skills":
		return dnd5e.ChoiceTypeSkill
	case "languages":
		return dnd5e.ChoiceTypeLanguage
	case "equipment":
		return dnd5e.ChoiceTypeEquipment
	case "spells":
		return dnd5e.ChoiceTypeSpells
	case "cantrips":
		return dnd5e.ChoiceTypeCantrips
	case "fighting_style":
		return dnd5e.ChoiceTypeFightingStyle
	case "proficiencies":
		return dnd5e.ChoiceTypeTool // Default to tool for proficiencies
	default:
		return dnd5e.ChoiceType(category)
	}
}
