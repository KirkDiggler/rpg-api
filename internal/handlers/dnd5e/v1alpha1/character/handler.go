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
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
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
		PlayerID:  req.GetPlayerId(),
		SessionID: req.GetSessionId(),
	}

	// If initial data provided, convert it
	if req.GetInitialData() != nil {
		input.InitialData = &toolkitchar.DraftData{
			Name: req.GetInitialData().GetName(),
			// TODO: Convert other fields as we implement them
		}
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
	// Validate request
	if req.GetDraftId() == "" {
		return nil, status.Error(codes.InvalidArgument, "draft_id is required")
	}

	// Call orchestrator
	output, err := h.characterService.GetDraft(ctx, &character.GetDraftInput{
		DraftID: req.GetDraftId(),
	})
	if err != nil {
		// Convert orchestrator errors to gRPC errors
		if errors.IsNotFound(err) {
			return nil, status.Error(codes.NotFound, "draft not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Convert toolkit DraftData to proto CharacterDraft
	protoDraft := convertDraftDataToProto(output.Draft)

	// Add validation to the draft if present
	if output.Validation != nil {
		protoDraft.Validation = convertToolkitValidationToProto(output.Validation)
	}

	return &dnd5ev1alpha1.GetDraftResponse{
		Draft: protoDraft,
	}, nil
}

// ListDrafts lists character drafts
func (h *Handler) ListDrafts(
	ctx context.Context,
	req *dnd5ev1alpha1.ListDraftsRequest,
) (*dnd5ev1alpha1.ListDraftsResponse, error) {
	// Call orchestrator
	output, err := h.characterService.ListDrafts(ctx, &character.ListDraftsInput{
		PlayerID:  req.PlayerId,
		SessionID: req.SessionId,
		PageSize:  req.PageSize,
		PageToken: req.PageToken,
	})
	if err != nil {
		if errors.IsInvalidArgument(err) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Convert drafts to proto
	protoDrafts := make([]*dnd5ev1alpha1.CharacterDraft, len(output.Drafts))
	for i, draft := range output.Drafts {
		protoDrafts[i] = convertDraftDataToProto(draft)
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
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

// UpdateName updates the name of a character draft
func (h *Handler) UpdateName(
	ctx context.Context,
	req *dnd5ev1alpha1.UpdateNameRequest,
) (*dnd5ev1alpha1.UpdateNameResponse, error) {
	// Call orchestrator
	output, err := h.characterService.UpdateName(ctx, &character.UpdateNameInput{
		DraftID: req.DraftId,
		Name:    req.Name,
	})
	if err != nil {
		if errors.IsInvalidArgument(err) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		if errors.IsNotFound(err) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Convert warnings
	protoWarnings := make([]*dnd5ev1alpha1.ValidationWarning, len(output.Warnings))
	for i, warning := range output.Warnings {
		protoWarnings[i] = &dnd5ev1alpha1.ValidationWarning{
			Field:   warning.Field,
			Message: warning.Message,
			Type:    warning.Type,
		}
	}

	return &dnd5ev1alpha1.UpdateNameResponse{
		Draft:    convertDraftDataToProto(output.Draft),
		Warnings: protoWarnings,
	}, nil
}

// UpdateRace updates the race of a character draft
func (h *Handler) UpdateRace(
	ctx context.Context,
	req *dnd5ev1alpha1.UpdateRaceRequest,
) (*dnd5ev1alpha1.UpdateRaceResponse, error) {
	// Convert proto Race enum to toolkit constant
	raceID := convertProtoRaceToToolkit(req.GetRace())
	subraceID := convertProtoSubraceToToolkit(req.GetSubrace())

	// Call orchestrator
	output, err := h.characterService.UpdateRace(ctx, &character.UpdateRaceInput{
		DraftID:   req.GetDraftId(),
		RaceID:    raceID,
		SubraceID: subraceID,
		Choices:   convertProtoChoiceDataListToToolkit(req.GetRaceChoices()),
	})
	if err != nil {
		if errors.IsInvalidArgument(err) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		if errors.IsNotFound(err) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Convert warnings
	protoWarnings := make([]*dnd5ev1alpha1.ValidationWarning, len(output.Warnings))
	for i, warning := range output.Warnings {
		protoWarnings[i] = &dnd5ev1alpha1.ValidationWarning{
			Field:   warning.Field,
			Message: warning.Message,
			Type:    warning.Type,
		}
	}

	return &dnd5ev1alpha1.UpdateRaceResponse{
		Draft:    convertDraftDataToProto(output.Draft),
		Warnings: protoWarnings,
	}, nil
}

// UpdateClass updates the class of a character draft
func (h *Handler) UpdateClass(
	ctx context.Context,
	req *dnd5ev1alpha1.UpdateClassRequest,
) (*dnd5ev1alpha1.UpdateClassResponse, error) {
	// Convert proto class to toolkit class constant
	classID := convertProtoClassToToolkit(req.Class)
	if classID == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid class")
	}

	// Convert proto subclass to toolkit subclass constant if provided
	var subclassID classes.Subclass
	if req.Subclass != dnd5ev1alpha1.Subclass_SUBCLASS_UNSPECIFIED {
		subclassID = convertProtoSubclassToToolkit(req.Subclass)
	}

	// Convert proto choices to toolkit choices
	var choices []toolkitchar.ChoiceData
	for _, protoChoice := range req.ClassChoices {
		choices = append(choices, convertProtoChoiceDataToToolkit(protoChoice))
	}

	// Call orchestrator
	output, err := h.characterService.UpdateClass(ctx, &character.UpdateClassInput{
		DraftID:    req.DraftId,
		ClassID:    classID,
		SubclassID: subclassID,
		Choices:    choices,
	})
	if err != nil {
		return nil, err
	}

	// Convert response
	protoDraft := convertDraftDataToProto(output.Draft)

	// Convert warnings
	warnings := make([]*dnd5ev1alpha1.ValidationWarning, 0, len(output.Warnings))
	for _, w := range output.Warnings {
		warnings = append(warnings, &dnd5ev1alpha1.ValidationWarning{
			Field:   w.Field,
			Message: w.Message,
		})
	}

	return &dnd5ev1alpha1.UpdateClassResponse{
		Draft:    protoDraft,
		Warnings: warnings,
	}, nil
}

// UpdateBackground updates the background of a character draft
func (h *Handler) UpdateBackground(
	ctx context.Context,
	req *dnd5ev1alpha1.UpdateBackgroundRequest,
) (*dnd5ev1alpha1.UpdateBackgroundResponse, error) {
	// Convert proto background to toolkit background type
	backgroundID := convertProtoBackgroundToToolkit(req.Background)
	if backgroundID == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid background")
	}

	// Convert proto choices to toolkit choices
	var choices []toolkitchar.ChoiceData
	for _, protoChoice := range req.BackgroundChoices {
		choices = append(choices, convertProtoChoiceDataToToolkit(protoChoice))
	}

	// Call orchestrator
	output, err := h.characterService.UpdateBackground(ctx, &character.UpdateBackgroundInput{
		DraftID:      req.DraftId,
		BackgroundID: backgroundID,
		Choices:      choices,
	})
	if err != nil {
		return nil, err
	}

	// Convert response
	protoDraft := convertDraftDataToProto(output.Draft)

	// Convert warnings - no conversion needed for now
	var warnings []*dnd5ev1alpha1.ValidationWarning

	return &dnd5ev1alpha1.UpdateBackgroundResponse{
		Draft:    protoDraft,
		Warnings: warnings,
	}, nil
}

// UpdateAbilityScores updates the ability scores of a character draft
func (h *Handler) UpdateAbilityScores(
	ctx context.Context,
	req *dnd5ev1alpha1.UpdateAbilityScoresRequest,
) (*dnd5ev1alpha1.UpdateAbilityScoresResponse, error) {
	// Validate request
	if req.DraftId == "" {
		return nil, status.Error(codes.InvalidArgument, "draft_id is required")
	}

	// Check which type of input we have
	switch scores := req.ScoresInput.(type) {
	case *dnd5ev1alpha1.UpdateAbilityScoresRequest_AbilityScores:
		// Manual ability score assignment
		// TODO: Implement manual score assignment
		return nil, status.Error(codes.Unimplemented, "manual ability score assignment not yet implemented")

	case *dnd5ev1alpha1.UpdateAbilityScoresRequest_RollAssignments:
		// Roll-based assignment
		assignments := scores.RollAssignments

		// Validate all roll IDs are provided
		if assignments.StrengthRollId == "" ||
			assignments.DexterityRollId == "" ||
			assignments.ConstitutionRollId == "" ||
			assignments.IntelligenceRollId == "" ||
			assignments.WisdomRollId == "" ||
			assignments.CharismaRollId == "" {
			return nil, status.Error(codes.InvalidArgument, "all ability score roll IDs must be provided")
		}

		// Call orchestrator to update ability scores with roll assignments
		output, err := h.characterService.UpdateAbilityScores(ctx, &character.UpdateAbilityScoresInput{
			DraftID: req.DraftId,
			RollAssignments: &character.RollAssignments{
				StrengthRollID:     assignments.StrengthRollId,
				DexterityRollID:    assignments.DexterityRollId,
				ConstitutionRollID: assignments.ConstitutionRollId,
				IntelligenceRollID: assignments.IntelligenceRollId,
				WisdomRollID:       assignments.WisdomRollId,
				CharismaRollID:     assignments.CharismaRollId,
			},
		})
		if err != nil {
			if errors.IsNotFound(err) {
				return nil, status.Error(codes.NotFound, err.Error())
			}
			if errors.IsInvalidArgument(err) {
				return nil, status.Error(codes.InvalidArgument, err.Error())
			}
			return nil, status.Error(codes.Internal, err.Error())
		}

		// Convert warnings
		protoWarnings := make([]*dnd5ev1alpha1.ValidationWarning, len(output.Warnings))
		for i, warning := range output.Warnings {
			protoWarnings[i] = &dnd5ev1alpha1.ValidationWarning{
				Field:   warning.Field,
				Message: warning.Message,
				Type:    warning.Type,
			}
		}

		return &dnd5ev1alpha1.UpdateAbilityScoresResponse{
			Draft:    convertDraftDataToProto(output.Draft),
			Warnings: protoWarnings,
		}, nil

	default:
		return nil, status.Error(codes.InvalidArgument, "scores_input must be provided")
	}
}

// UpdateSkills updates the skills of a character draft
func (h *Handler) UpdateSkills(
	ctx context.Context,
	req *dnd5ev1alpha1.UpdateSkillsRequest,
) (*dnd5ev1alpha1.UpdateSkillsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

// ValidateDraft validates a character draft
func (h *Handler) ValidateDraft(
	ctx context.Context,
	req *dnd5ev1alpha1.ValidateDraftRequest,
) (*dnd5ev1alpha1.ValidateDraftResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

// GetDraftPreview gets a preview of what the character would look like if finalized
func (h *Handler) GetDraftPreview(
	ctx context.Context,
	req *dnd5ev1alpha1.GetDraftPreviewRequest,
) (*dnd5ev1alpha1.GetDraftPreviewResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

// FinalizeDraft finalizes a character draft
func (h *Handler) FinalizeDraft(
	ctx context.Context,
	req *dnd5ev1alpha1.FinalizeDraftRequest,
) (*dnd5ev1alpha1.FinalizeDraftResponse, error) {
	// Validate request
	if req.GetDraftId() == "" {
		return nil, status.Error(codes.InvalidArgument, "draft_id is required")
	}

	// Call orchestrator to finalize the draft
	output, err := h.characterService.FinalizeDraft(ctx, &character.FinalizeDraftInput{
		DraftID: req.GetDraftId(),
	})
	if err != nil {
		// Convert errors to gRPC status
		if errors.IsInvalidArgument(err) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		if errors.IsNotFound(err) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Convert character to proto
	protoCharacter := ConvertCharacterDataToProto(output.Character)

	return &dnd5ev1alpha1.FinalizeDraftResponse{
		Character:    protoCharacter,
		DraftDeleted: output.DraftDeleted,
	}, nil
}

// GetCharacter retrieves a character
func (h *Handler) GetCharacter(
	ctx context.Context,
	req *dnd5ev1alpha1.GetCharacterRequest,
) (*dnd5ev1alpha1.GetCharacterResponse, error) {
	// Validate request
	if req.CharacterId == "" {
		return nil, status.Error(codes.InvalidArgument, "character_id is required")
	}

	// Call orchestrator to get the character
	output, err := h.characterService.GetCharacter(ctx, &character.GetCharacterInput{
		CharacterID: req.CharacterId,
	})
	if err != nil {
		// Convert errors to gRPC status
		if errors.IsNotFound(err) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		if errors.IsInvalidArgument(err) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Convert character to proto
	protoCharacter := ConvertCharacterDataToProto(output.Character)

	return &dnd5ev1alpha1.GetCharacterResponse{
		Character: protoCharacter,
	}, nil
}

// ListCharacters lists characters
func (h *Handler) ListCharacters(
	ctx context.Context,
	req *dnd5ev1alpha1.ListCharactersRequest,
) (*dnd5ev1alpha1.ListCharactersResponse, error) {
	// Call orchestrator to list characters
	output, err := h.characterService.ListCharacters(ctx, &character.ListCharactersInput{
		PlayerID: req.PlayerId,
	})
	if err != nil {
		if errors.IsInvalidArgument(err) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Convert characters to proto
	protoCharacters := make([]*dnd5ev1alpha1.Character, 0, len(output.Characters))
	for _, char := range output.Characters {
		protoCharacters = append(protoCharacters, ConvertCharacterDataToProto(char))
	}

	return &dnd5ev1alpha1.ListCharactersResponse{
		Characters: protoCharacters,
	}, nil
}

// DeleteCharacter deletes a character
func (h *Handler) DeleteCharacter(
	ctx context.Context,
	req *dnd5ev1alpha1.DeleteCharacterRequest,
) (*dnd5ev1alpha1.DeleteCharacterResponse, error) {
	// Validate request
	if req.CharacterId == "" {
		return nil, status.Error(codes.InvalidArgument, "character_id is required")
	}

	// Call orchestrator to delete the character
	output, err := h.characterService.DeleteCharacter(ctx, &character.DeleteCharacterInput{
		CharacterID: req.CharacterId,
	})
	if err != nil {
		// Convert errors to gRPC status
		if errors.IsNotFound(err) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		if errors.IsInvalidArgument(err) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &dnd5ev1alpha1.DeleteCharacterResponse{
		Message: output.Message,
	}, nil
}

// ListRaces lists available races
func (h *Handler) ListRaces(
	ctx context.Context,
	req *dnd5ev1alpha1.ListRacesRequest,
) (*dnd5ev1alpha1.ListRacesResponse, error) {
	// Call orchestrator
	output, err := h.characterService.ListRaces(ctx, &character.ListRacesInput{
		PageSize:  req.GetPageSize(),
		PageToken: req.GetPageToken(),
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Convert to proto RaceInfo
	protoRaces := make([]*dnd5ev1alpha1.RaceInfo, len(output.Races))
	for i, race := range output.Races {
		protoRaces[i] = convertRaceDataToProtoInfo(race.RaceData, race.UIData)
	}

	return &dnd5ev1alpha1.ListRacesResponse{
		Races:         protoRaces,
		NextPageToken: output.NextPageToken,
		TotalSize:     int32(output.TotalSize),
	}, nil
}

// ListClasses lists available classes
func (h *Handler) ListClasses(
	ctx context.Context,
	req *dnd5ev1alpha1.ListClassesRequest,
) (*dnd5ev1alpha1.ListClassesResponse, error) {
	// Call orchestrator
	output, err := h.characterService.ListClasses(ctx, &character.ListClassesInput{
		PageSize:  req.GetPageSize(),
		PageToken: req.GetPageToken(),
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Convert to proto ClassInfo
	protoClasses := make([]*dnd5ev1alpha1.ClassInfo, len(output.Classes))
	for i, class := range output.Classes {
		protoClasses[i] = convertStartingClassToProtoInfo(class)
	}

	return &dnd5ev1alpha1.ListClassesResponse{
		Classes:       protoClasses,
		NextPageToken: output.NextPageToken,
		TotalSize:     int32(output.TotalSize),
	}, nil
}

// ListBackgrounds lists available backgrounds
func (h *Handler) ListBackgrounds(
	ctx context.Context,
	req *dnd5ev1alpha1.ListBackgroundsRequest,
) (*dnd5ev1alpha1.ListBackgroundsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
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
	// Validate input
	if req.DraftId == "" {
		return nil, status.Error(codes.InvalidArgument, "draft_id is required")
	}

	// Call the character service to roll ability scores
	output, err := h.characterService.RollAbilityScores(ctx, &character.RollAbilityScoresInput{
		DraftID: req.DraftId,
	})
	if err != nil {
		// Check for specific error types
		if errors.IsNotFound(err) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		if errors.IsInvalidArgument(err) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Convert rolls to proto format
	protoRolls := make([]*dnd5ev1alpha1.AbilityScoreRoll, 0, len(output.Rolls))
	for _, roll := range output.Rolls {
		var dropped int32
		if len(roll.Dropped) > 0 {
			dropped = roll.Dropped[0] // Take the first dropped die
		}

		protoRoll := &dnd5ev1alpha1.AbilityScoreRoll{
			RollId:   roll.RollID,
			Dice:     roll.Dice,
			Total:    roll.Total,
			Dropped:  dropped,
			Notation: roll.Description,
		}
		protoRolls = append(protoRolls, protoRoll)
	}

	return &dnd5ev1alpha1.RollAbilityScoresResponse{
		Rolls:     protoRolls,
		ExpiresAt: output.ExpiresAt.Unix(),
	}, nil
}

// ListEquipmentByType lists equipment by type
func (h *Handler) ListEquipmentByType(
	ctx context.Context,
	req *dnd5ev1alpha1.ListEquipmentByTypeRequest,
) (*dnd5ev1alpha1.ListEquipmentByTypeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

// ListSpellsByLevel lists spells by level
func (h *Handler) ListSpellsByLevel(
	ctx context.Context,
	req *dnd5ev1alpha1.ListSpellsByLevelRequest,
) (*dnd5ev1alpha1.ListSpellsByLevelResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

// GetCharacterInventory gets character inventory
func (h *Handler) GetCharacterInventory(
	ctx context.Context,
	req *dnd5ev1alpha1.GetCharacterInventoryRequest,
) (*dnd5ev1alpha1.GetCharacterInventoryResponse, error) {
	if req == nil || req.CharacterId == "" {
		return nil, status.Error(codes.InvalidArgument, "character_id is required")
	}

	// Call orchestrator to get inventory
	result, err := h.characterService.GetCharacterInventory(ctx, &character.GetCharacterInventoryInput{
		CharacterID: req.CharacterId,
	})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, status.Error(codes.NotFound, "character not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Convert to proto response
	response := &dnd5ev1alpha1.GetCharacterInventoryResponse{
		EquipmentSlots: convertEquipmentSlotsToProto(result.EquipmentSlots),
		Inventory:      make([]*dnd5ev1alpha1.InventoryItem, 0),
		Encumbrance: &dnd5ev1alpha1.EncumbranceInfo{
			CurrentWeight:    0,
			CarryingCapacity: 150, // Default for STR 10
			MaxCapacity:      300,
			Level:            dnd5ev1alpha1.EncumbranceLevel_ENCUMBRANCE_LEVEL_UNENCUMBERED,
		},
		AttunementSlotsUsed: result.AttunementSlotsUsed,
		AttunementSlotsMax:  3, // D&D 5e standard
	}

	// Convert inventory items, separating equipped from unequipped
	for _, item := range result.Inventory {
		// If item is equipped, it should be in equipment slots and NOT in inventory
		if !item.Equipped {
			response.Inventory = append(response.Inventory, &dnd5ev1alpha1.InventoryItem{
				ItemId:     item.ID,
				Quantity:   item.Quantity,
				CustomName: item.Name,
			})
		}
	}

	return response, nil
}

// EquipItem equips an item
func (h *Handler) EquipItem(
	ctx context.Context,
	req *dnd5ev1alpha1.EquipItemRequest,
) (*dnd5ev1alpha1.EquipItemResponse, error) {
	if req == nil || req.CharacterId == "" {
		return nil, status.Error(codes.InvalidArgument, "character_id is required")
	}
	if req.ItemId == "" {
		return nil, status.Error(codes.InvalidArgument, "item_id is required")
	}

	// Call orchestrator to equip item
	result, err := h.characterService.EquipItem(ctx, &character.EquipItemInput{
		CharacterID: req.CharacterId,
		ItemID:      req.ItemId,
		Slot:        req.Slot.String(),
	})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		if errors.IsInvalidArgument(err) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	// For minimal implementation, just return success
	// TODO: Implement proper character to proto conversion with equipment
	response := &dnd5ev1alpha1.EquipItemResponse{
		// Character field left nil for now - frontend can refetch if needed
	}

	// Add previously equipped item if any
	if result.PreviouslyEquippedItem != nil {
		response.PreviouslyEquippedItem = &dnd5ev1alpha1.InventoryItem{
			ItemId:     result.PreviouslyEquippedItem.ID,
			Quantity:   result.PreviouslyEquippedItem.Quantity,
			IsAttuned:  result.PreviouslyEquippedItem.Equipped,
			CustomName: result.PreviouslyEquippedItem.Name,
		}
	}

	return response, nil
}

// UnequipItem unequips an item
func (h *Handler) UnequipItem(
	ctx context.Context,
	req *dnd5ev1alpha1.UnequipItemRequest,
) (*dnd5ev1alpha1.UnequipItemResponse, error) {
	if req == nil || req.CharacterId == "" {
		return nil, status.Error(codes.InvalidArgument, "character_id is required")
	}
	if req.Slot == dnd5ev1alpha1.EquipmentSlot_EQUIPMENT_SLOT_UNSPECIFIED {
		return nil, status.Error(codes.InvalidArgument, "slot is required")
	}

	// Call orchestrator to unequip item
	_, err := h.characterService.UnequipItem(ctx, &character.UnequipItemInput{
		CharacterID: req.CharacterId,
		Slot:        req.Slot.String(),
	})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, status.Error(codes.NotFound, "character not found")
		}
		if errors.IsInvalidArgument(err) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	// For minimal implementation, just return success
	// TODO: Implement proper character to proto conversion with equipment
	return &dnd5ev1alpha1.UnequipItemResponse{
		// Character field left nil for now - frontend can refetch if needed
	}, nil
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
