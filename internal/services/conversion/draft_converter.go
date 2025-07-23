// Package conversion provides centralized conversion logic between
// storage models (CharacterDraftData) and domain models (CharacterDraft).
package conversion

import (
	"context"
	"fmt"

	"github.com/KirkDiggler/rpg-api/internal/clients/external"
	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
)

// draftConverter is the concrete implementation of DraftConverter
type draftConverter struct {
	externalClient external.Client
}

// DraftConverterConfig holds the configuration for creating a draft converter
type DraftConverterConfig struct {
	ExternalClient external.Client
}

// Validate ensures the configuration is valid
func (c *DraftConverterConfig) Validate() error {
	if c == nil {
		return fmt.Errorf("config is required")
	}
	if c.ExternalClient == nil {
		return fmt.Errorf("external client is required")
	}
	return nil
}

// NewDraftConverter creates a new draft converter instance
func NewDraftConverter(cfg *DraftConverterConfig) (DraftConverter, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &draftConverter{
		externalClient: cfg.ExternalClient,
	}, nil
}

// ToCharacterDraft converts CharacterDraftData to CharacterDraft
func (c *draftConverter) ToCharacterDraft(data *dnd5e.CharacterDraftData) *dnd5e.CharacterDraft {
	if data == nil {
		return nil
	}

	return &dnd5e.CharacterDraft{
		ID:               data.ID,
		PlayerID:         data.PlayerID,
		SessionID:        data.SessionID,
		Name:             data.Name,
		RaceID:           data.RaceID,
		SubraceID:        data.SubraceID,
		ClassID:          data.ClassID,
		BackgroundID:     data.BackgroundID,
		AbilityScores:    data.AbilityScores,
		Alignment:        data.Alignment,
		ChoiceSelections: data.ChoiceSelections,
		Progress:         data.Progress,
		ExpiresAt:        data.ExpiresAt,
		CreatedAt:        data.CreatedAt,
		UpdatedAt:        data.UpdatedAt,
		// Info objects are left nil - use HydrateDraft to populate them
	}
}

// FromCharacterDraft converts CharacterDraft to CharacterDraftData
func (c *draftConverter) FromCharacterDraft(draft *dnd5e.CharacterDraft) *dnd5e.CharacterDraftData {
	if draft == nil {
		return nil
	}

	return &dnd5e.CharacterDraftData{
		ID:               draft.ID,
		PlayerID:         draft.PlayerID,
		SessionID:        draft.SessionID,
		Name:             draft.Name,
		RaceID:           draft.RaceID,
		SubraceID:        draft.SubraceID,
		ClassID:          draft.ClassID,
		BackgroundID:     draft.BackgroundID,
		AbilityScores:    draft.AbilityScores,
		Alignment:        draft.Alignment,
		ChoiceSelections: draft.ChoiceSelections,
		Progress:         draft.Progress,
		ExpiresAt:        draft.ExpiresAt,
		CreatedAt:        draft.CreatedAt,
		UpdatedAt:        draft.UpdatedAt,
	}
}

// HydrateDraft populates the info objects on a draft
func (c *draftConverter) HydrateDraft(ctx context.Context, draft *dnd5e.CharacterDraft) (*dnd5e.CharacterDraft, error) {
	if draft == nil {
		return nil, nil
	}

	// Create a copy to avoid modifying the original
	hydratedDraft := *draft

	// Clear any existing info objects
	hydratedDraft.Race = nil
	hydratedDraft.Subrace = nil
	hydratedDraft.Class = nil
	hydratedDraft.Background = nil

	// Fetch and populate race info
	if draft.RaceID != "" {
		raceData, err := c.externalClient.GetRaceData(ctx, draft.RaceID)
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
						Traits:         convertExternalTraitsToRacialTraits(subrace.Traits),
						Languages:      subrace.Languages,
						Proficiencies:  subrace.Proficiencies,
					}
					break
				}
			}
		}
	}

	// Fetch and populate class info
	if draft.ClassID != "" {
		classData, err := c.externalClient.GetClassData(ctx, draft.ClassID)
		if err != nil {
			return nil, fmt.Errorf("failed to get class data for %s: %w", draft.ClassID, err)
		}
		hydratedDraft.Class = convertExternalClassToEntity(classData)
	}

	// Fetch and populate background info
	if draft.BackgroundID != "" {
		backgroundData, err := c.externalClient.GetBackgroundData(ctx, draft.BackgroundID)
		if err != nil {
			return nil, fmt.Errorf("failed to get background data for %s: %w", draft.BackgroundID, err)
		}
		hydratedDraft.Background = convertExternalBackgroundToEntity(backgroundData)
	}

	return &hydratedDraft, nil
}

// Conversion helper functions
// These would be moved from the orchestrator

func convertExternalRaceToEntity(race *external.RaceData) *dnd5e.RaceInfo {
	if race == nil {
		return nil
	}

	return &dnd5e.RaceInfo{
		ID:                   race.ID,
		Name:                 race.Name,
		Description:          race.Description,
		Speed:                race.Speed,
		Size:                 race.Size,
		SizeDescription:      race.SizeDescription,
		AbilityBonuses:       race.AbilityBonuses,
		Traits:               convertExternalTraitsToRacialTraits(race.Traits),
		Subraces:             convertExternalSubracesToInfo(race.Subraces),
		Proficiencies:        race.Proficiencies,
		Languages:            race.Languages,
		AgeDescription:       race.AgeDescription,
		AlignmentDescription: race.AlignmentDescription,
		Choices:              race.Choices,
	}
}

func convertExternalClassToEntity(class *external.ClassData) *dnd5e.ClassInfo {
	if class == nil {
		return nil
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
		Level1Features:           convertExternalFeaturesToInfo(class.LevelOneFeatures),
		Choices:                  class.Choices,
	}
}

func convertExternalBackgroundToEntity(bg *external.BackgroundData) *dnd5e.BackgroundInfo {
	if bg == nil {
		return nil
	}

	return &dnd5e.BackgroundInfo{
		ID:                  bg.ID,
		Name:                bg.Name,
		Description:         bg.Description,
		SkillProficiencies:  bg.SkillProficiencies,
		Languages:           []string{}, // TODO(#115): Convert from language count
		AdditionalLanguages: bg.Languages,
		StartingEquipment:   bg.Equipment,
		FeatureName:         bg.Feature,
		FeatureDescription:  "", // TODO(#116): Not available in external data
	}
}

func convertExternalTraitsToRacialTraits(traits []external.TraitData) []dnd5e.RacialTrait {
	if traits == nil {
		return nil
	}

	result := make([]dnd5e.RacialTrait, len(traits))
	for i, trait := range traits {
		result[i] = dnd5e.RacialTrait{
			Name:        trait.Name,
			Description: trait.Description,
			IsChoice:    trait.IsChoice,
			Options:     trait.Options,
		}
	}
	return result
}

func convertExternalSubracesToInfo(subraces []external.SubraceData) []dnd5e.SubraceInfo {
	if subraces == nil {
		return nil
	}

	result := make([]dnd5e.SubraceInfo, len(subraces))
	for i, subrace := range subraces {
		result[i] = dnd5e.SubraceInfo{
			ID:             subrace.ID,
			Name:           subrace.Name,
			Description:    subrace.Description,
			AbilityBonuses: subrace.AbilityBonuses,
			Traits:         convertExternalTraitsToRacialTraits(subrace.Traits),
			Languages:      subrace.Languages,
			Proficiencies:  subrace.Proficiencies,
		}
	}
	return result
}

func convertExternalFeaturesToInfo(features []*external.FeatureData) []dnd5e.FeatureInfo {
	if features == nil {
		return nil
	}

	result := make([]dnd5e.FeatureInfo, len(features))
	for i, feature := range features {
		result[i] = dnd5e.FeatureInfo{
			ID:          feature.ID,
			Name:        feature.Name,
			Description: feature.Description,
			Level:       feature.Level,
			Choices:     convertChoiceDataToChoices(feature.Choices),
		}
	}
	return result
}

func convertChoiceDataToChoices(choices []*external.ChoiceData) []dnd5e.Choice {
	if choices == nil {
		return nil
	}

	result := make([]dnd5e.Choice, 0)
	for _, choice := range choices {
		if choice == nil {
			continue
		}
		// TODO(#117): Map external choice data to internal choice format
		// This would require more complex logic to determine choice type
	}
	return result
}
