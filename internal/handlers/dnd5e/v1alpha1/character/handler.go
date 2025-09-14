// Package v1alpha1 handles the grpc service interface
package character

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/errors"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character/choices"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
	// "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/spells" // TODO: Re-enable when spell conversion is needed
)

// HandlerConfig holds dependencies for the handler
type HandlerConfig struct {
	CharacterService character.Service
}

// Validate ensures all required dependencies are present
func (c *HandlerConfig) Validate() error {
	if c.CharacterService == nil {
		return errors.InvalidArgument("character service is required")
	}
	return nil
}

// Handler implements the D&D 5e gRPC service
type Handler struct {
	dnd5ev1alpha1.UnimplementedCharacterServiceServer
	characterService character.Service
}

// NewHandler creates a new handler with the given configuration
func NewHandler(cfg *HandlerConfig) (*Handler, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &Handler{
		characterService: cfg.CharacterService,
	}, nil
}

// CreateDraft creates a new character draft
func (h *Handler) CreateDraft(
	ctx context.Context,
	req *dnd5ev1alpha1.CreateDraftRequest,
) (*dnd5ev1alpha1.CreateDraftResponse, error) {
	// Validate request
	if req.GetPlayerId() == "" {
		return nil, status.Error(codes.InvalidArgument, "player_id is required")
	}

	// Create input for orchestrator
	input := &character.CreateDraftInput{
		PlayerID:  req.PlayerId,
		SessionID: req.SessionId,
	}

	// Call orchestrator
	output, err := h.characterService.CreateDraft(ctx, input)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Convert toolkit DraftData to proto CharacterDraft
	protoDraft := convertDraftDataToProto(output.Draft)

	return &dnd5ev1alpha1.CreateDraftResponse{
		Draft: protoDraft,
	}, nil
}

// GetDraft retrieves a character draft
func (h *Handler) GetDraft(
	ctx context.Context,
	req *dnd5ev1alpha1.GetDraftRequest,
) (*dnd5ev1alpha1.GetDraftResponse, error) {
	return nil, errors.Unimplemented("GetDraft not implemented")
}

// ListDrafts lists character drafts
func (h *Handler) ListDrafts(
	ctx context.Context,
	req *dnd5ev1alpha1.ListDraftsRequest,
) (*dnd5ev1alpha1.ListDraftsResponse, error) {
	// Build input for orchestrator
	input := &character.ListDraftsInput{
		PlayerID:  req.GetPlayerId(),
		SessionID: req.GetSessionId(),
		PageSize:  int(req.GetPageSize()),
		PageToken: req.GetPageToken(),
	}

	// Call orchestrator
	output, err := h.characterService.ListDrafts(ctx, input)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Convert toolkit DraftData to proto CharacterDraft
	protoDrafts := make([]*dnd5ev1alpha1.CharacterDraft, 0, len(output.Drafts))
	for _, draftData := range output.Drafts {
		protoDrafts = append(protoDrafts, convertDraftDataToProto(draftData))
	}

	return &dnd5ev1alpha1.ListDraftsResponse{
		Drafts:        protoDrafts,
		NextPageToken: output.NextPageToken,
	}, nil
}

// DeleteDraft deletes a character draft
func (h *Handler) DeleteDraft(
	ctx context.Context,
	req *dnd5ev1alpha1.DeleteDraftRequest,
) (*dnd5ev1alpha1.DeleteDraftResponse, error) {
	return nil, errors.Unimplemented("DeleteDraft not implemented")
}

// UpdateName updates the name of a character draft
func (h *Handler) UpdateName(
	ctx context.Context,
	req *dnd5ev1alpha1.UpdateNameRequest,
) (*dnd5ev1alpha1.UpdateNameResponse, error) {
	if req == nil {
		return nil, errors.InvalidArgument("request is required")
	}
	if req.DraftId == "" {
		return nil, errors.InvalidArgument("draft_id is required")
	}
	if req.Name == "" {
		return nil, errors.InvalidArgument("name is required")
	}

	// Call orchestrator
	result, err := h.characterService.SetName(ctx, &character.SetNameInput{
		DraftID: req.DraftId,
		Name:    req.Name,
	})
	if err != nil {
		return nil, err
	}

	// Convert result to proto
	return &dnd5ev1alpha1.UpdateNameResponse{
		Draft: convertDraftDataToProto(result.Draft),
	}, nil
}

// UpdateRace updates the race of a character draft
func (h *Handler) UpdateRace(
	ctx context.Context,
	req *dnd5ev1alpha1.UpdateRaceRequest,
) (*dnd5ev1alpha1.UpdateRaceResponse, error) {
	if req == nil {
		return nil, errors.InvalidArgument("request is required")
	}
	if req.DraftId == "" {
		return nil, errors.InvalidArgument("draft_id is required")
	}

	// Convert proto choices to toolkit format
	var raceChoices toolkitchar.RaceChoices
	if len(req.RaceChoices) > 0 {
		for _, choice := range req.RaceChoices {
			switch choice.Category {
			case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_LANGUAGES:
				if langs := choice.GetLanguages(); langs != nil {
					for _, lang := range langs.Languages {
						raceChoices.Languages = append(raceChoices.Languages, convertProtoLanguageToToolkit(lang))
					}
				}
			case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS:
				if skills := choice.GetSkills(); skills != nil {
					for _, skill := range skills.Skills {
						raceChoices.Skills = append(raceChoices.Skills, convertProtoSkillToToolkit(skill))
					}
				}
			case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SPELLS:
				// Spells now include both spells and cantrips
				if spellSelection := choice.GetSpells(); spellSelection != nil {
					for _, spell := range spellSelection.Spells {
						// TODO: Convert spell enum to spells.Spell when proper mapping is available
						_ = spell
					}
				}
			}
		}
	}

	// Call orchestrator
	result, err := h.characterService.SetRace(ctx, &character.SetRaceInput{
		DraftID: req.DraftId,
		Input: &toolkitchar.SetRaceInput{
			RaceID:    convertProtoRaceToToolkit(req.Race),
			SubraceID: convertProtoSubraceToToolkit(req.Subrace),
			Choices:   raceChoices,
		},
	})
	if err != nil {
		return nil, err
	}

	// Convert result to proto
	return &dnd5ev1alpha1.UpdateRaceResponse{
		Draft: convertDraftDataToProto(result.Draft),
	}, nil
}

// UpdateClass updates the class of a character draft
func (h *Handler) UpdateClass(
	ctx context.Context,
	req *dnd5ev1alpha1.UpdateClassRequest,
) (*dnd5ev1alpha1.UpdateClassResponse, error) {
	if req == nil {
		return nil, errors.InvalidArgument("request is required")
	}
	if req.DraftId == "" {
		return nil, errors.InvalidArgument("draft_id is required")
	}

	// Convert proto choices to toolkit format
	var classChoices toolkitchar.ClassChoices
	if len(req.ClassChoices) > 0 {
		for _, choice := range req.ClassChoices {
			switch choice.Category {
			case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS:
				if skills := choice.GetSkills(); skills != nil {
					for _, skill := range skills.Skills {
						classChoices.Skills = append(classChoices.Skills, convertProtoSkillToToolkit(skill))
					}
				}
			case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_FIGHTING_STYLE:
				if fs := choice.GetFightingStyle(); fs != nil && fs.Style != dnd5ev1alpha1.FightingStyle_FIGHTING_STYLE_UNSPECIFIED {
					classChoices.FightingStyle = convertProtoFightingStyleToToolkit(fs.Style)
				}
			// Note: Cantrips are now part of SPELLS category
			case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SPELLS:
				if spellList := choice.GetSpells(); spellList != nil {
					for range spellList.Spells {
						// TODO: Convert spell enum to spells.Spell when mapping is available
						// For now, skip spells
					}
				}
			case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT:
				if equipment := choice.GetEquipment(); equipment != nil {
					if classChoices.Equipment == nil {
						classChoices.Equipment = make(map[choices.ChoiceID]shared.SelectionID)
					}
					// Handle equipment selection mapping
					if choice.ChoiceId != "" && len(equipment.Items) > 0 {
						// Extract the item ID from the first EquipmentSelection
						firstItem := equipment.Items[0]
						var itemID string

						// Extract the ID based on the equipment type
						switch eq := firstItem.Equipment.(type) {
						case *dnd5ev1alpha1.EquipmentSelectionItem_Weapon:
							// Convert weapon enum back to string ID
							itemID = string(eq.Weapon) // Use enum string value as ID
						case *dnd5ev1alpha1.EquipmentSelectionItem_Armor:
							itemID = string(eq.Armor) // Use enum string value as ID
						case *dnd5ev1alpha1.EquipmentSelectionItem_Tool:
							itemID = string(eq.Tool) // Use enum string value as ID
						case *dnd5ev1alpha1.EquipmentSelectionItem_OtherEquipmentId:
							itemID = eq.OtherEquipmentId
						}

						if itemID != "" {
							classChoices.Equipment[choices.ChoiceID(choice.ChoiceId)] = shared.SelectionID(itemID)
						}
					}
				}
			}
		}
	}

	// Call orchestrator
	result, err := h.characterService.SetClass(ctx, &character.SetClassInput{
		DraftID: req.DraftId,
		Input: &toolkitchar.SetClassInput{
			ClassID:    convertProtoClassToToolkit(req.Class),
			SubclassID: convertProtoSubclassToToolkit(req.Subclass),
			Choices:    classChoices,
		},
	})
	if err != nil {
		return nil, err
	}

	// Convert result to proto
	return &dnd5ev1alpha1.UpdateClassResponse{
		Draft: convertDraftDataToProto(result.Draft),
	}, nil
}

// UpdateBackground updates the background of a character draft
func (h *Handler) UpdateBackground(
	ctx context.Context,
	req *dnd5ev1alpha1.UpdateBackgroundRequest,
) (*dnd5ev1alpha1.UpdateBackgroundResponse, error) {
	if req == nil {
		return nil, errors.InvalidArgument("request is required")
	}
	if req.DraftId == "" {
		return nil, errors.InvalidArgument("draft_id is required")
	}

	// Convert proto choices to toolkit format
	var bgChoices toolkitchar.BackgroundChoices
	if len(req.BackgroundChoices) > 0 {
		for _, choice := range req.BackgroundChoices {
			if choice.Category == dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_LANGUAGES {
				if langs := choice.GetLanguages(); langs != nil {
					for _, lang := range langs.Languages {
						bgChoices.Languages = append(bgChoices.Languages, convertProtoLanguageToToolkit(lang))
					}
				}
			}
		}
	}

	// Call orchestrator
	result, err := h.characterService.SetBackground(ctx, &character.SetBackgroundInput{
		DraftID: req.DraftId,
		Input: &toolkitchar.SetBackgroundInput{
			BackgroundID: convertProtoBackgroundToToolkit(req.Background),
			Choices:      bgChoices,
		},
	})
	if err != nil {
		return nil, err
	}

	// Convert result to proto
	return &dnd5ev1alpha1.UpdateBackgroundResponse{
		Draft: convertDraftDataToProto(result.Draft),
	}, nil
}

// UpdateAbilityScores updates the ability scores of a character draft
func (h *Handler) UpdateAbilityScores(
	ctx context.Context,
	req *dnd5ev1alpha1.UpdateAbilityScoresRequest,
) (*dnd5ev1alpha1.UpdateAbilityScoresResponse, error) {
	if req == nil {
		return nil, errors.InvalidArgument("request is required")
	}
	if req.DraftId == "" {
		return nil, errors.InvalidArgument("draft_id is required")
	}
	// Handle the oneof scores_input field
	var scores shared.AbilityScores
	method := "standard"

	switch scoresInput := req.GetScoresInput().(type) {
	case *dnd5ev1alpha1.UpdateAbilityScoresRequest_AbilityScores:
		if scoresInput.AbilityScores == nil {
			return nil, errors.InvalidArgument("ability_scores is required")
		}
		scores = convertProtoAbilityScoresToToolkit(scoresInput.AbilityScores)
		method = "manual"
	case *dnd5ev1alpha1.UpdateAbilityScoresRequest_RollAssignments:
		// TODO: Handle roll assignments when implemented
		return nil, errors.InvalidArgument("roll assignments not yet implemented")
	default:
		return nil, errors.InvalidArgument("scores_input is required")
	}

	// Call orchestrator
	result, err := h.characterService.SetAbilityScores(ctx, &character.SetAbilityScoresInput{
		DraftID: req.DraftId,
		Input: &toolkitchar.SetAbilityScoresInput{
			Scores: scores,
			Method: method,
		},
	})
	if err != nil {
		return nil, err
	}

	// Convert result to proto
	return &dnd5ev1alpha1.UpdateAbilityScoresResponse{
		Draft: convertDraftDataToProto(result.Draft),
	}, nil
}

// UpdateSkills updates the skills of a character draft
func (h *Handler) UpdateSkills(
	ctx context.Context,
	req *dnd5ev1alpha1.UpdateSkillsRequest,
) (*dnd5ev1alpha1.UpdateSkillsResponse, error) {
	return nil, errors.Unimplemented("UpdateSkills not implemented")
}

// ValidateDraft validates a character draft
func (h *Handler) ValidateDraft(
	ctx context.Context,
	req *dnd5ev1alpha1.ValidateDraftRequest,
) (*dnd5ev1alpha1.ValidateDraftResponse, error) {
	return nil, errors.Unimplemented("ValidateDraft not implemented")
}

// GetDraftPreview gets a preview of what the character would look like if finalized
func (h *Handler) GetDraftPreview(
	ctx context.Context,
	req *dnd5ev1alpha1.GetDraftPreviewRequest,
) (*dnd5ev1alpha1.GetDraftPreviewResponse, error) {
	return nil, errors.Unimplemented("GetDraftPreview not implemented")
}

// FinalizeDraft finalizes a character draft
func (h *Handler) FinalizeDraft(
	ctx context.Context,
	req *dnd5ev1alpha1.FinalizeDraftRequest,
) (*dnd5ev1alpha1.FinalizeDraftResponse, error) {
	return nil, errors.Unimplemented("FinalizeDraft not implemented")
}

// GetCharacter retrieves a character
func (h *Handler) GetCharacter(
	ctx context.Context,
	req *dnd5ev1alpha1.GetCharacterRequest,
) (*dnd5ev1alpha1.GetCharacterResponse, error) {
	return nil, errors.Unimplemented("GetCharacter not implemented")
}

// ListCharacters lists characters
func (h *Handler) ListCharacters(
	ctx context.Context,
	req *dnd5ev1alpha1.ListCharactersRequest,
) (*dnd5ev1alpha1.ListCharactersResponse, error) {
	// Build input for orchestrator
	input := &character.ListCharactersInput{
		PlayerID:  req.GetPlayerId(),
		SessionID: req.GetSessionId(),
		PageSize:  int(req.GetPageSize()),
		PageToken: req.GetPageToken(),
	}

	// Call orchestrator
	output, err := h.characterService.ListCharacters(ctx, input)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Convert toolkit character.Data to proto Character
	protoCharacters := make([]*dnd5ev1alpha1.Character, 0, len(output.Characters))
	for _, charData := range output.Characters {
		protoCharacters = append(protoCharacters, convertCharacterDataToProto(charData))
	}

	return &dnd5ev1alpha1.ListCharactersResponse{
		Characters:    protoCharacters,
		NextPageToken: output.NextPageToken,
		TotalSize:     int32(output.TotalSize),
	}, nil
}

// DeleteCharacter deletes a character
func (h *Handler) DeleteCharacter(
	ctx context.Context,
	req *dnd5ev1alpha1.DeleteCharacterRequest,
) (*dnd5ev1alpha1.DeleteCharacterResponse, error) {
	return nil, errors.Unimplemented("DeleteCharacter not implemented")
}

// ListRaces lists available races
func (h *Handler) ListRaces(
	ctx context.Context,
	req *dnd5ev1alpha1.ListRacesRequest,
) (*dnd5ev1alpha1.ListRacesResponse, error) {
	// Call orchestrator
	result, err := h.characterService.ListRaces(ctx, &character.ListRacesInput{})
	if err != nil {
		return nil, err
	}

	// Convert toolkit Data to proto
	races := make([]*dnd5ev1alpha1.RaceInfo, 0, len(result.Races))
	for _, raceData := range result.Races {
		races = append(races, convertRaceDataToProto(raceData))
	}

	return &dnd5ev1alpha1.ListRacesResponse{
		Races:     races,
		TotalSize: int32(len(races)),
	}, nil
}

// ListClasses lists available classes
func (h *Handler) ListClasses(
	ctx context.Context,
	req *dnd5ev1alpha1.ListClassesRequest,
) (*dnd5ev1alpha1.ListClassesResponse, error) {
	// Call orchestrator
	result, err := h.characterService.ListClasses(ctx, &character.ListClassesInput{})
	if err != nil {
		return nil, err
	}

	// Convert toolkit Data to proto
	classes := make([]*dnd5ev1alpha1.ClassInfo, 0, len(result.Classes))
	for _, classData := range result.Classes {
		classes = append(classes, convertClassDataToProto(classData))
	}

	return &dnd5ev1alpha1.ListClassesResponse{
		Classes:   classes,
		TotalSize: int32(len(classes)),
	}, nil
}

// ListBackgrounds lists available backgrounds
func (h *Handler) ListBackgrounds(
	ctx context.Context,
	req *dnd5ev1alpha1.ListBackgroundsRequest,
) (*dnd5ev1alpha1.ListBackgroundsResponse, error) {
	// Call orchestrator
	result, err := h.characterService.ListBackgrounds(ctx, &character.ListBackgroundsInput{})
	if err != nil {
		return nil, err
	}

	// Convert toolkit Data to proto
	backgrounds := make([]*dnd5ev1alpha1.BackgroundInfo, 0, len(result.Backgrounds))
	for _, bgData := range result.Backgrounds {
		backgrounds = append(backgrounds, convertBackgroundDataToProto(bgData))
	}

	return &dnd5ev1alpha1.ListBackgroundsResponse{
		Backgrounds: backgrounds,
		TotalSize:   int32(len(backgrounds)),
	}, nil
}

// GetRaceDetails returns detailed information about a specific race
func (h *Handler) GetRaceDetails(
	ctx context.Context,
	req *dnd5ev1alpha1.GetRaceDetailsRequest,
) (*dnd5ev1alpha1.GetRaceDetailsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

// GetClassDetails returns detailed information about a specific class
func (h *Handler) GetClassDetails(
	ctx context.Context,
	req *dnd5ev1alpha1.GetClassDetailsRequest,
) (*dnd5ev1alpha1.GetClassDetailsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

// GetBackgroundDetails returns detailed information about a specific background
func (h *Handler) GetBackgroundDetails(
	ctx context.Context,
	req *dnd5ev1alpha1.GetBackgroundDetailsRequest,
) (*dnd5ev1alpha1.GetBackgroundDetailsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

// GetFeature returns detailed information about a specific feature
func (h *Handler) GetFeature(
	ctx context.Context,
	req *dnd5ev1alpha1.GetFeatureRequest,
) (*dnd5ev1alpha1.GetFeatureResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

// RollAbilityScores rolls ability scores for character creation
func (h *Handler) RollAbilityScores(
	ctx context.Context,
	req *dnd5ev1alpha1.RollAbilityScoresRequest,
) (*dnd5ev1alpha1.RollAbilityScoresResponse, error) {
	return nil, errors.Unimplemented("RollAbilityScores not implemented")
}

// ListEquipmentByType lists equipment by type
func (h *Handler) ListEquipmentByType(
	ctx context.Context,
	req *dnd5ev1alpha1.ListEquipmentByTypeRequest,
) (*dnd5ev1alpha1.ListEquipmentByTypeResponse, error) {
	return nil, errors.Unimplemented("ListEquipmentByType not implemented")
}

// ListSpellsByLevel lists spells by level
func (h *Handler) ListSpellsByLevel(
	ctx context.Context,
	req *dnd5ev1alpha1.ListSpellsByLevelRequest,
) (*dnd5ev1alpha1.ListSpellsByLevelResponse, error) {
	return nil, errors.Unimplemented("ListSpellsByLevel not implemented")
}

// GetCharacterInventory gets character inventory
func (h *Handler) GetCharacterInventory(
	ctx context.Context,
	req *dnd5ev1alpha1.GetCharacterInventoryRequest,
) (*dnd5ev1alpha1.GetCharacterInventoryResponse, error) {
	return nil, errors.Unimplemented("GetCharacterInventory not implemented")
}

// EquipItem equips an item
func (h *Handler) EquipItem(
	ctx context.Context,
	req *dnd5ev1alpha1.EquipItemRequest,
) (*dnd5ev1alpha1.EquipItemResponse, error) {
	return nil, errors.Unimplemented("EquipItem not implemented")
}

// UnequipItem unequips an item
func (h *Handler) UnequipItem(
	ctx context.Context,
	req *dnd5ev1alpha1.UnequipItemRequest,
) (*dnd5ev1alpha1.UnequipItemResponse, error) {
	return nil, errors.Unimplemented("UnequipItem not implemented")
}

// AddToInventory adds items to inventory
func (h *Handler) AddToInventory(
	ctx context.Context,
	req *dnd5ev1alpha1.AddToInventoryRequest,
) (*dnd5ev1alpha1.AddToInventoryResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

// RemoveFromInventory removes items from inventory
func (h *Handler) RemoveFromInventory(
	ctx context.Context,
	req *dnd5ev1alpha1.RemoveFromInventoryRequest,
) (*dnd5ev1alpha1.RemoveFromInventoryResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}
