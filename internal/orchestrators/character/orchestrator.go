package character

import (
	"context"
	"fmt"
	"strings"

	"github.com/KirkDiggler/rpg-api/internal/clients/external"
	"github.com/KirkDiggler/rpg-api/internal/errors"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/dice"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	"github.com/KirkDiggler/rpg-api/internal/repositories/character"
	draftrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/constants"
)

// Config holds dependencies for the orchestrator
type Config struct {
	CharacterRepo      character.Repository
	CharacterDraftRepo draftrepo.Repository
	ExternalClient     external.Client
	DiceService        dice.Service
	IDGenerator        idgen.Generator
}

// Validate ensures all required dependencies are present
func (c *Config) Validate() error {
	if c.CharacterRepo == nil {
		return errors.InvalidArgument("character repository is required")
	}
	if c.CharacterDraftRepo == nil {
		return errors.InvalidArgument("character draft repository is required")
	}
	if c.ExternalClient == nil {
		return errors.InvalidArgument("external client is required")
	}
	if c.DiceService == nil {
		return errors.InvalidArgument("dice service is required")
	}
	if c.IDGenerator == nil {
		return errors.InvalidArgument("ID generator is required")
	}
	return nil
}

// Orchestrator implements the character service
type Orchestrator struct {
	charRepo      character.Repository
	draftRepo     draftrepo.Repository
	externalClient external.Client
	diceService   dice.Service
	idGen         idgen.Generator
}

// New creates a new character orchestrator
func New(cfg *Config) (*Orchestrator, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &Orchestrator{
		charRepo:      cfg.CharacterRepo,
		draftRepo:     cfg.CharacterDraftRepo,
		externalClient: cfg.ExternalClient,
		diceService:   cfg.DiceService,
		idGen:         cfg.IDGenerator,
	}, nil
}

// All methods return unimplemented for now

func (o *Orchestrator) CreateDraft(ctx context.Context, input *CreateDraftInput) (*CreateDraftOutput, error) {
	// Validate input
	if input.PlayerID == "" {
		return nil, errors.InvalidArgument("player ID is required")
	}

	// Create new draft with minimal data
	draft := &toolkitchar.DraftData{
		ID:       o.idGen.Generate(),
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

	// Return the draft data directly
	// The repository returns toolkit DraftData which is what we want
	return &GetDraftOutput{
		Draft: getDraftOutput.Draft,
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
		RaceID:    constants.Race(input.RaceID),
		SubraceID: constants.Subrace(input.SubraceID),
	}

	// Update choices if provided
	if len(input.Choices) > 0 {
		// Filter out existing race choices and add new ones
		var nonRaceChoices []toolkitchar.ChoiceData
		for _, choice := range draft.Choices {
			if choice.Source != "race" {
				nonRaceChoices = append(nonRaceChoices, choice)
			}
		}
		draft.Choices = append(nonRaceChoices, input.Choices...)
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
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) UpdateBackground(ctx context.Context, input *UpdateBackgroundInput) (*UpdateBackgroundOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) UpdateAbilityScores(ctx context.Context, input *UpdateAbilityScoresInput) (*UpdateAbilityScoresOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) UpdateSkills(ctx context.Context, input *UpdateSkillsInput) (*UpdateSkillsOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) ValidateDraft(ctx context.Context, input *ValidateDraftInput) (*ValidateDraftOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) FinalizeDraft(ctx context.Context, input *FinalizeDraftInput) (*FinalizeDraftOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) GetCharacter(ctx context.Context, input *GetCharacterInput) (*GetCharacterOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) ListCharacters(ctx context.Context, input *ListCharactersInput) (*ListCharactersOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) DeleteCharacter(ctx context.Context, input *DeleteCharacterInput) (*DeleteCharacterOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) ListRaces(ctx context.Context, input *ListRacesInput) (*ListRacesOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) ListClasses(ctx context.Context, input *ListClassesInput) (*ListClassesOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) ListBackgrounds(ctx context.Context, input *ListBackgroundsInput) (*ListBackgroundsOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}


func (o *Orchestrator) UpdateChoices(ctx context.Context, input *UpdateChoicesInput) (*UpdateChoicesOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) ListChoiceOptions(ctx context.Context, input *ListChoiceOptionsInput) (*ListChoiceOptionsOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) GetRaceDetails(ctx context.Context, input *GetRaceDetailsInput) (*GetRaceDetailsOutput, error) {
	if input.RaceID == "" {
		return nil, errors.InvalidArgument("race ID is required")
	}

	// Get race data from external client
	raceDataOutput, err := o.externalClient.GetRaceData(ctx, input.RaceID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get race data for %s", input.RaceID)
	}

	return &GetRaceDetailsOutput{
		RaceData: raceDataOutput.RaceData,
		UIData:   raceDataOutput.UIData,
	}, nil
}

func (o *Orchestrator) GetClassDetails(ctx context.Context, input *GetClassDetailsInput) (*GetClassDetailsOutput, error) {
	if input.ClassID == "" {
		return nil, errors.InvalidArgument("class ID is required")
	}

	// Get class data from external client
	classDataOutput, err := o.externalClient.GetClassData(ctx, input.ClassID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get class data for %s", input.ClassID)
	}

	return &GetClassDetailsOutput{
		ClassData: classDataOutput.ClassData,
		UIData:    classDataOutput.UIData,
	}, nil
}

func (o *Orchestrator) GetBackgroundDetails(ctx context.Context, input *GetBackgroundDetailsInput) (*GetBackgroundDetailsOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) RollAbilityScores(ctx context.Context, input *RollAbilityScoresInput) (*RollAbilityScoresOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) GetDraftPreview(ctx context.Context, input *GetDraftPreviewInput) (*GetDraftPreviewOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) GetFeature(ctx context.Context, input *GetFeatureInput) (*GetFeatureOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) ListSpellsByLevel(ctx context.Context, input *ListSpellsByLevelInput) (*ListSpellsByLevelOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) ListEquipmentByType(ctx context.Context, input *ListEquipmentByTypeInput) (*ListEquipmentByTypeOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) GetCharacterInventory(ctx context.Context, input *GetCharacterInventoryInput) (*GetCharacterInventoryOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) EquipItem(ctx context.Context, input *EquipItemInput) (*EquipItemOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) UnequipItem(ctx context.Context, input *UnequipItemInput) (*UnequipItemOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) AddToInventory(ctx context.Context, input *AddToInventoryInput) (*AddToInventoryOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) RemoveFromInventory(ctx context.Context, input *RemoveFromInventoryInput) (*RemoveFromInventoryOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}