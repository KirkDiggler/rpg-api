package character

import (
	"context"
	"fmt"

	"github.com/KirkDiggler/rpg-api/internal/clients/external"
	"github.com/KirkDiggler/rpg-api/internal/errors"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/dice"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	"github.com/KirkDiggler/rpg-api/internal/repositories/character"
	characterdraft "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
)

// Config holds dependencies for the orchestrator
type Config struct {
	CharacterRepo      character.Repository
	CharacterDraftRepo characterdraft.Repository
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
	draftRepo     characterdraft.Repository
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
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) GetDraft(ctx context.Context, input *GetDraftInput) (*GetDraftOutput, error) {
	// Validate input
	if input.DraftID == "" {
		return nil, errors.InvalidArgument("draft ID is required")
	}

	// Get draft from repository
	getDraftOutput, err := o.draftRepo.Get(ctx, characterdraft.GetInput{
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
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) DeleteDraft(ctx context.Context, input *DeleteDraftInput) (*DeleteDraftOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) UpdateName(ctx context.Context, input *UpdateNameInput) (*UpdateNameOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) UpdateRace(ctx context.Context, input *UpdateRaceInput) (*UpdateRaceOutput, error) {
	return nil, errors.Unimplemented("not implemented")
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
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) GetClassDetails(ctx context.Context, input *GetClassDetailsInput) (*GetClassDetailsOutput, error) {
	return nil, errors.Unimplemented("not implemented")
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