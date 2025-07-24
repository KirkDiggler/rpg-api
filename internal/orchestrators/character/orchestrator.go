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
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	draftrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/class"
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
}

// Config holds the dependencies for the character orchestrator
type Config struct {
	CharacterRepo      characterrepo.Repository
	CharacterDraftRepo draftrepo.Repository
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
		if input.InitialData.RaceID != "" {
			if err := builder.SetRaceData(race.Data{ID: input.InitialData.RaceID}, input.InitialData.SubraceID); err != nil {
				return nil, errors.Wrap(err, "failed to set race")
			}
		}

		// Set class if provided
		if input.InitialData.ClassID != "" {
			if err := builder.SetClassData(class.Data{ID: input.InitialData.ClassID}); err != nil {
				return nil, errors.Wrap(err, "failed to set class")
			}
		}

		// Set ability scores if provided
		if input.InitialData.AbilityScores != nil {
			// Convert to toolkit format
			abilityScores := shared.AbilityScores{
				Strength:     int(input.InitialData.AbilityScores.Strength),
				Dexterity:    int(input.InitialData.AbilityScores.Dexterity),
				Constitution: int(input.InitialData.AbilityScores.Constitution),
				Intelligence: int(input.InitialData.AbilityScores.Intelligence),
				Wisdom:       int(input.InitialData.AbilityScores.Wisdom),
				Charisma:     int(input.InitialData.AbilityScores.Charisma),
			}
			if err := builder.SetAbilityScores(abilityScores); err != nil {
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

	// Convert the created draft data to CharacterDraft for API response
	createdDraft, err := o.convertDraftDataToCharacterDraft(ctx, createOutput.Draft)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert draft data")
	}

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

	// Convert toolkit DraftData to API CharacterDraft
	draft, err := o.convertDraftDataToCharacterDraft(ctx, getOutput.Draft)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert draft data")
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
				Drafts:        []*dnd5e.CharacterDraft{},
				NextPageToken: "",
			}, nil
		}
		return nil, errors.Wrap(err, "failed to get player draft")
	}

	// Convert DraftData to CharacterDraft
	draft, err := o.convertDraftDataToCharacterDraft(ctx, draftOutput.Draft)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert draft data")
	}

	// Hydrate the draft with info objects
	hydratedDraft, err := o.hydrateDraft(ctx, draft)
	if err != nil {
		return nil, err
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

	// Convert result to CharacterDraft for return
	updatedDraft, err := o.convertDraftDataToCharacterDraft(ctx, updateOutput.Draft)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert draft data")
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

	// Handle race choices if provided
	if len(input.Choices) > 0 {
		slog.InfoContext(ctx, "Applying race choices",
			"draft_id", input.DraftID,
			"num_choices", len(input.Choices))

		// For now, store race choices in the draft's choice map
		// These will need to be properly integrated when toolkit supports race choices
		for _, choice := range input.Choices {
			choiceKey := shared.ChoiceCategory(fmt.Sprintf("race_%s", choice.ChoiceID))
			updatedData.Choices[choiceKey] = choice.SelectedKeys
			slog.InfoContext(ctx, "Stored race choice",
				"choice_id", choice.ChoiceID,
				"selected", choice.SelectedKeys)
		}
	}

	// Save the updated draft
	updateOutput, err := o.characterDraftRepo.Update(ctx, draftrepo.UpdateInput{
		Draft: &updatedData,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to update draft")
	}

	// Convert result to CharacterDraft and hydrate
	updatedDraft, err := o.convertDraftDataToCharacterDraft(ctx, updateOutput.Draft)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert draft data")
	}

	// Hydrate with race info
	hydratedDraft, err := o.hydrateDraft(ctx, updatedDraft)
	if err != nil {
		return nil, errors.Wrap(err, "failed to hydrate draft")
	}

	// Collect warnings (for now, empty)
	var warnings []ValidationWarning

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

	// Handle class choices if provided
	if len(input.Choices) > 0 {
		slog.InfoContext(ctx, "Applying class choices",
			"draft_id", input.DraftID,
			"num_choices", len(input.Choices))

		// For now, store class choices in the draft's choice map
		// These will need to be properly integrated when toolkit supports class choices
		for _, choice := range input.Choices {
			choiceKey := shared.ChoiceCategory(fmt.Sprintf("class_%s", choice.ChoiceID))
			updatedData.Choices[choiceKey] = choice.SelectedKeys
			slog.InfoContext(ctx, "Stored class choice",
				"choice_id", choice.ChoiceID,
				"selected", choice.SelectedKeys)
		}
	}

	// Save the updated draft
	updateOutput, err := o.characterDraftRepo.Update(ctx, draftrepo.UpdateInput{
		Draft: &updatedData,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to update draft")
	}

	// Convert result to CharacterDraft and hydrate
	updatedDraft, err := o.convertDraftDataToCharacterDraft(ctx, updateOutput.Draft)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert draft data")
	}

	// Hydrate with class info
	hydratedDraft, err := o.hydrateDraft(ctx, updatedDraft)
	if err != nil {
		return nil, errors.Wrap(err, "failed to hydrate draft")
	}

	// Collect warnings (for now, empty)
	var warnings []ValidationWarning

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

	// Convert result to CharacterDraft and hydrate
	updatedDraft, err := o.convertDraftDataToCharacterDraft(ctx, updateOutput.Draft)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert draft data")
	}

	// Hydrate with background info
	hydratedDraft, err := o.hydrateDraft(ctx, updatedDraft)
	if err != nil {
		return nil, errors.Wrap(err, "failed to hydrate draft")
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

	// Load into builder
	builder, err := character.LoadDraft(*getOutput.Draft)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load draft into builder")
	}

	// Convert to toolkit ability scores format
	scores := shared.AbilityScores{
		Strength:     int(input.AbilityScores.Strength),
		Dexterity:    int(input.AbilityScores.Dexterity),
		Constitution: int(input.AbilityScores.Constitution),
		Intelligence: int(input.AbilityScores.Intelligence),
		Wisdom:       int(input.AbilityScores.Wisdom),
		Charisma:     int(input.AbilityScores.Charisma),
	}

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
		"draft_id", updatedData.ID,
		"has_ability_scores", updatedData.Choices[shared.ChoiceAbilityScores] != nil)

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

	// Convert result to CharacterDraft
	updatedDraft, err := o.convertDraftDataToCharacterDraft(ctx, updateOutput.Draft)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert draft data")
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

	// Convert result to CharacterDraft
	updatedDraft, err := o.convertDraftDataToCharacterDraft(ctx, updateOutput.Draft)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert draft data")
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

	// Load into builder
	builder, err := character.LoadDraft(*getOutput.Draft)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load draft into builder")
	}

	// Build the final character
	toolkitChar, err := builder.Build()
	if err != nil {
		return nil, errors.Wrap(err, "failed to build character from draft")
	}

	// Get the character data from toolkit
	charData := toolkitChar.ToData()

	// Convert to API character format
	// TODO: This is a temporary mapping until we fully integrate toolkit
	char := &dnd5e.Character{
		Level:            int32(charData.Level),
		ExperiencePoints: 0, // Starting character
		Name:             charData.Name,
		RaceID:           charData.RaceID,
		SubraceID:        "", // TODO: Get from builder when supported
		ClassID:          charData.ClassID,
		BackgroundID:     charData.BackgroundID,
		Alignment:        "", // Not in toolkit yet
		AbilityScores: dnd5e.AbilityScores{
			Strength:     int32(charData.AbilityScores.Strength),
			Dexterity:    int32(charData.AbilityScores.Dexterity),
			Constitution: int32(charData.AbilityScores.Constitution),
			Intelligence: int32(charData.AbilityScores.Intelligence),
			Wisdom:       int32(charData.AbilityScores.Wisdom),
			Charisma:     int32(charData.AbilityScores.Charisma),
		},
		CurrentHP: int32(charData.HitPoints),
		TempHP:    0,
		SessionID: "", // Will be set from draft
		PlayerID:  getOutput.Draft.PlayerID,
	}

	// Convert draft data to get session ID
	draft, err := o.convertDraftDataToCharacterDraft(ctx, getOutput.Draft)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert draft data")
	}
	char.SessionID = draft.SessionID
	char.Alignment = draft.Alignment

	// Create the character in repository
	createCharOutput, err := o.characterRepo.Create(ctx, characterrepo.CreateInput{
		Character: char,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create character")
	}

	// Delete the draft
	deleteInput := draftrepo.DeleteInput{ID: input.DraftID}
	if _, err := o.characterDraftRepo.Delete(ctx, deleteInput); err != nil {
		// Log but don't fail - character was created successfully
		slog.Warn("Failed to delete draft after finalization",
			"draft_id", input.DraftID,
			"error", err)
	}

	return &FinalizeDraftOutput{
		Character: createCharOutput.Character,
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

	// Apply choices to the builder
	// The toolkit uses a map[shared.ChoiceCategory]any for choices
	draftData := builder.ToData()
	if draftData.Choices == nil {
		draftData.Choices = make(map[shared.ChoiceCategory]any)
	}

	// Convert and apply each selection
	for _, selection := range input.Selections {
		if selection == nil {
			continue
		}

		// Validate and convert the choice ID to a ChoiceCategory
		category := shared.ChoiceCategory(selection.ChoiceID)
		
		// Validate the category is recognized
		// The toolkit uses both predefined categories and dynamic ones with prefixes
		if !isValidChoiceCategory(category) {
			slog.WarnContext(ctx, "Skipping unrecognized choice category",
				"choice_id", selection.ChoiceID,
				"draft_id", draftData.ID)
			continue
		}
		
		// Store the selection based on its type
		// Different choice types may have different data structures
		switch {
		case len(selection.SelectedKeys) > 0:
			// Most choices use string arrays for selected options
			draftData.Choices[category] = selection.SelectedKeys
			
		case len(selection.AbilityScoreChoices) > 0:
			// Ability score choices have their own structure
			if category == shared.ChoiceAbilityScores {
				draftData.Choices[category] = selection.AbilityScoreChoices
			} else {
				slog.WarnContext(ctx, "AbilityScoreChoices provided for non-ability score category",
					"category", category)
			}
			
		default:
			slog.WarnContext(ctx, "No valid selection data provided",
				"choice_id", selection.ChoiceID)
		}
	}

	// Save the updated draft to repository
	updateOutput, err := o.characterDraftRepo.Update(ctx, draftrepo.UpdateInput{
		Draft: &draftData,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to update draft in repository")
	}

	// Convert the updated draft data back to CharacterDraft for API response
	updatedDraft, err := o.convertDraftDataToCharacterDraft(ctx, updateOutput.Draft)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert draft data")
	}

	slog.InfoContext(ctx, "Successfully updated character choices",
		"draft_id", updatedDraft.ID)

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

// applyChoiceSelections applies validated selections to the draft

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

	// Extract data from choices
	if choices := data.Choices; choices != nil {
		// Race
		if raceChoice, ok := choices[shared.ChoiceRace].(character.RaceChoice); ok {
			draft.RaceID = raceChoice.RaceID
			draft.SubraceID = raceChoice.SubraceID
		} else if raceMap, ok := choices[shared.ChoiceRace].(map[string]interface{}); ok {
			// Handle case where it's unmarshaled as a map
			if raceID, ok := raceMap["race_id"].(string); ok {
				draft.RaceID = raceID
			}
			if subraceID, ok := raceMap["subrace_id"].(string); ok {
				draft.SubraceID = subraceID
			}
		}

		// Class
		if classID, ok := choices[shared.ChoiceClass].(string); ok {
			draft.ClassID = classID
		}

		// Background
		if backgroundID, ok := choices[shared.ChoiceBackground].(string); ok {
			draft.BackgroundID = backgroundID
		}

		// Ability Scores
		if scoresData := choices[shared.ChoiceAbilityScores]; scoresData != nil {
			slog.InfoContext(ctx, "Extracting ability scores from choices",
				"type", fmt.Sprintf("%T", scoresData),
				"value", fmt.Sprintf("%+v", scoresData))

			if scores, ok := scoresData.(shared.AbilityScores); ok {
				draft.AbilityScores = &dnd5e.AbilityScores{
					Strength:     int32(scores.Strength),
					Dexterity:    int32(scores.Dexterity),
					Constitution: int32(scores.Constitution),
					Intelligence: int32(scores.Intelligence),
					Wisdom:       int32(scores.Wisdom),
					Charisma:     int32(scores.Charisma),
				}
			} else if scoresMap, ok := scoresData.(map[string]interface{}); ok {
				// Handle case where it's unmarshaled as a map
				draft.AbilityScores = &dnd5e.AbilityScores{}
				if str, ok := scoresMap["strength"].(float64); ok {
					draft.AbilityScores.Strength = int32(str)
				} else if str, ok := scoresMap["Strength"].(float64); ok {
					draft.AbilityScores.Strength = int32(str)
				}
				if dex, ok := scoresMap["dexterity"].(float64); ok {
					draft.AbilityScores.Dexterity = int32(dex)
				} else if dex, ok := scoresMap["Dexterity"].(float64); ok {
					draft.AbilityScores.Dexterity = int32(dex)
				}
				if con, ok := scoresMap["constitution"].(float64); ok {
					draft.AbilityScores.Constitution = int32(con)
				} else if con, ok := scoresMap["Constitution"].(float64); ok {
					draft.AbilityScores.Constitution = int32(con)
				}
				if intel, ok := scoresMap["intelligence"].(float64); ok {
					draft.AbilityScores.Intelligence = int32(intel)
				} else if intel, ok := scoresMap["Intelligence"].(float64); ok {
					draft.AbilityScores.Intelligence = int32(intel)
				}
				if wis, ok := scoresMap["wisdom"].(float64); ok {
					draft.AbilityScores.Wisdom = int32(wis)
				} else if wis, ok := scoresMap["Wisdom"].(float64); ok {
					draft.AbilityScores.Wisdom = int32(wis)
				}
				if cha, ok := scoresMap["charisma"].(float64); ok {
					draft.AbilityScores.Charisma = int32(cha)
				} else if cha, ok := scoresMap["Charisma"].(float64); ok {
					draft.AbilityScores.Charisma = int32(cha)
				}
			}
		}

		// Skills
		if skills, ok := choices[shared.ChoiceSkills].([]string); ok {
			// Convert to choice selections
			draft.ChoiceSelections = append(draft.ChoiceSelections, dnd5e.ChoiceSelection{
				ChoiceID:     "skill_proficiencies",
				Source:       dnd5e.ChoiceSourceClass,
				SelectedKeys: skills,
			})
		} else if skillsArr, ok := choices[shared.ChoiceSkills].([]interface{}); ok {
			// Handle case where it's unmarshaled as []interface{}
			var skills []string
			for _, s := range skillsArr {
				if skill, ok := s.(string); ok {
					skills = append(skills, skill)
				}
			}
			if len(skills) > 0 {
				draft.ChoiceSelections = append(draft.ChoiceSelections, dnd5e.ChoiceSelection{
					ChoiceID:     "skill_proficiencies",
					Source:       dnd5e.ChoiceSourceClass,
					SelectedKeys: skills,
				})
			}
		}

		// Extract race and class choices from the choices map
		for key, value := range choices {
			keyStr := string(key)

			// Extract race choices
			if strings.HasPrefix(keyStr, "race_") {
				choiceID := strings.TrimPrefix(keyStr, "race_")
				if selectedKeys, ok := value.([]string); ok {
					draft.ChoiceSelections = append(draft.ChoiceSelections, dnd5e.ChoiceSelection{
						ChoiceID:     choiceID,
						Source:       dnd5e.ChoiceSourceRace,
						SelectedKeys: selectedKeys,
					})
				} else if selectedArr, ok := value.([]interface{}); ok {
					// Handle case where it's unmarshaled as []interface{}
					var keys []string
					for _, k := range selectedArr {
						if key, ok := k.(string); ok {
							keys = append(keys, key)
						}
					}
					if len(keys) > 0 {
						draft.ChoiceSelections = append(draft.ChoiceSelections, dnd5e.ChoiceSelection{
							ChoiceID:     choiceID,
							Source:       dnd5e.ChoiceSourceRace,
							SelectedKeys: keys,
						})
					}
				}
			}

			// Extract class choices
			if strings.HasPrefix(keyStr, "class_") {
				choiceID := strings.TrimPrefix(keyStr, "class_")
				if selectedKeys, ok := value.([]string); ok {
					draft.ChoiceSelections = append(draft.ChoiceSelections, dnd5e.ChoiceSelection{
						ChoiceID:     choiceID,
						Source:       dnd5e.ChoiceSourceClass,
						SelectedKeys: selectedKeys,
					})
				} else if selectedArr, ok := value.([]interface{}); ok {
					// Handle case where it's unmarshaled as []interface{}
					var keys []string
					for _, k := range selectedArr {
						if key, ok := k.(string); ok {
							keys = append(keys, key)
						}
					}
					if len(keys) > 0 {
						draft.ChoiceSelections = append(draft.ChoiceSelections, dnd5e.ChoiceSelection{
							ChoiceID:     choiceID,
							Source:       dnd5e.ChoiceSourceClass,
							SelectedKeys: keys,
						})
					}
				}
			}
		}
	}

	// Hydrate with external data if we have race/class/background
	if draft.RaceID != "" || draft.ClassID != "" || draft.BackgroundID != "" {
		return o.hydrateDraft(ctx, draft)
	}

	return draft, nil
}
