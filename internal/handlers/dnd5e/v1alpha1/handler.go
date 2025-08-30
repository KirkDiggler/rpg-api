// Package v1alpha1 handles the grpc service interface
package v1alpha1

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/clients/external"
	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
	"github.com/KirkDiggler/rpg-api/internal/errors"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/backgrounds"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character/choices"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/class"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/languages"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/race"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/races"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/skills"
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

// convertEquipmentSlotsToProto converts dnd5e equipment slots to proto format
func convertEquipmentSlotsToProto(equipmentSlots *dnd5e.EquipmentSlots) *dnd5ev1alpha1.EquipmentSlots {
	if equipmentSlots == nil {
		return &dnd5ev1alpha1.EquipmentSlots{}
	}

	protoSlots := &dnd5ev1alpha1.EquipmentSlots{}

	if equipmentSlots.MainHand != nil {
		protoSlots.MainHand = &dnd5ev1alpha1.InventoryItem{
			ItemId:     equipmentSlots.MainHand.ID,
			Quantity:   equipmentSlots.MainHand.Quantity,
			CustomName: equipmentSlots.MainHand.Name,
		}
	}

	if equipmentSlots.OffHand != nil {
		protoSlots.OffHand = &dnd5ev1alpha1.InventoryItem{
			ItemId:     equipmentSlots.OffHand.ID,
			Quantity:   equipmentSlots.OffHand.Quantity,
			CustomName: equipmentSlots.OffHand.Name,
		}
	}

	if equipmentSlots.Armor != nil {
		protoSlots.Armor = &dnd5ev1alpha1.InventoryItem{
			ItemId:     equipmentSlots.Armor.ID,
			Quantity:   equipmentSlots.Armor.Quantity,
			CustomName: equipmentSlots.Armor.Name,
		}
	}

	// TODO: Fix proto field names for Helm
	// if equipmentSlots.Helm != nil {
	// 	protoSlots.Helm = &dnd5ev1alpha1.InventoryItem{
	// 		ItemId:     equipmentSlots.Helm.ID,
	// 		Quantity:   equipmentSlots.Helm.Quantity,
	// 		CustomName: equipmentSlots.Helm.Name,
	// 	}
	// }

	if equipmentSlots.Gloves != nil {
		protoSlots.Gloves = &dnd5ev1alpha1.InventoryItem{
			ItemId:     equipmentSlots.Gloves.ID,
			Quantity:   equipmentSlots.Gloves.Quantity,
			CustomName: equipmentSlots.Gloves.Name,
		}
	}

	if equipmentSlots.Boots != nil {
		protoSlots.Boots = &dnd5ev1alpha1.InventoryItem{
			ItemId:     equipmentSlots.Boots.ID,
			Quantity:   equipmentSlots.Boots.Quantity,
			CustomName: equipmentSlots.Boots.Name,
		}
	}

	// TODO: Fix proto field names for Ring1 and Ring2
	// if equipmentSlots.Ring1 != nil {
	// 	protoSlots.Ring1 = &dnd5ev1alpha1.InventoryItem{
	// 		ItemId:     equipmentSlots.Ring1.ID,
	// 		Quantity:   equipmentSlots.Ring1.Quantity,
	// 		CustomName: equipmentSlots.Ring1.Name,
	// 	}
	// }

	// if equipmentSlots.Ring2 != nil {
	// 	protoSlots.Ring2 = &dnd5ev1alpha1.InventoryItem{
	// 		ItemId:     equipmentSlots.Ring2.ID,
	// 		Quantity:   equipmentSlots.Ring2.Quantity,
	// 		CustomName: equipmentSlots.Ring2.Name,
	// 	}
	// }

	if equipmentSlots.Cloak != nil {
		protoSlots.Cloak = &dnd5ev1alpha1.InventoryItem{
			ItemId:     equipmentSlots.Cloak.ID,
			Quantity:   equipmentSlots.Cloak.Quantity,
			CustomName: equipmentSlots.Cloak.Name,
		}
	}

	if equipmentSlots.Amulet != nil {
		protoSlots.Amulet = &dnd5ev1alpha1.InventoryItem{
			ItemId:     equipmentSlots.Amulet.ID,
			Quantity:   equipmentSlots.Amulet.Quantity,
			CustomName: equipmentSlots.Amulet.Name,
		}
	}

	return protoSlots
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

// convertDraftDataToProto converts toolkit DraftData to proto CharacterDraft
func convertDraftDataToProto(draft *toolkitchar.DraftData) *dnd5ev1alpha1.CharacterDraft {
	if draft == nil {
		return nil
	}

	protoDraft := &dnd5ev1alpha1.CharacterDraft{
		Id:       draft.ID,
		PlayerId: draft.PlayerID,
		Name:     draft.Name,
	}

	// Convert timestamps
	if !draft.CreatedAt.IsZero() {
		protoDraft.CreatedAt = draft.CreatedAt.Unix()
	}
	if !draft.UpdatedAt.IsZero() {
		protoDraft.UpdatedAt = draft.UpdatedAt.Unix()
	}

	// Convert progress - calculate completion based on progress flags
	progress := &dnd5ev1alpha1.CreationProgress{
		HasName:          draft.Name != "",
		HasRace:          draft.RaceChoice.RaceID != "",
		HasClass:         draft.ClassChoice.ClassID != "",
		HasBackground:    draft.BackgroundChoice != "",
		HasAbilityScores: hasAbilityScores(draft.AbilityScoreChoice),
		// TODO: Add skill and language tracking when we implement those
	}

	// Calculate completion percentage
	completedSteps := 0
	totalSteps := 5 // name, race, class, background, ability scores

	if progress.HasName {
		completedSteps++
	}
	if progress.HasRace {
		completedSteps++
	}
	if progress.HasClass {
		completedSteps++
	}
	if progress.HasBackground {
		completedSteps++
	}
	if progress.HasAbilityScores {
		completedSteps++
	}

	progress.CompletionPercentage = int32((completedSteps * 100) / totalSteps)
	protoDraft.Progress = progress

	// Convert choices
	protoDraft.Choices = convertToolkitChoicesToProto(draft.Choices)

	// Populate enum fields with the actual stored values
	if draft.RaceChoice.RaceID != "" {
		protoDraft.RaceId = convertToolkitRaceToProtoEnum(draft.RaceChoice.RaceID)
		if draft.RaceChoice.SubraceID != "" {
			protoDraft.SubraceId = convertToolkitSubraceToProtoEnum(draft.RaceChoice.SubraceID)
		}
	}

	if draft.ClassChoice.ClassID != "" {
		protoDraft.ClassId = convertToolkitClassToProtoEnum(draft.ClassChoice.ClassID)
		// Note: Subclass info should be included in the ClassInfo object returned by ListClasses
		// The CharacterDraft proto message doesn't have a direct Subclass field
		// The SubclassID is stored in draft.ClassChoice.SubclassID for later use
	}

	if draft.BackgroundChoice != "" {
		protoDraft.BackgroundId = convertToolkitBackgroundToProtoEnum(draft.BackgroundChoice)
	}

	// Convert ability scores if present
	if len(draft.AbilityScoreChoice) > 0 {
		protoDraft.AbilityScores = convertToolkitAbilityScoresToProto(draft.AbilityScoreChoice)
	}

	return protoDraft
}

// convertToolkitChoicesToProto converts toolkit ChoiceData to proto ChoiceData
func convertToolkitChoicesToProto(choices []toolkitchar.ChoiceData) []*dnd5ev1alpha1.ChoiceData {
	if len(choices) == 0 {
		return nil
	}

	protoChoices := make([]*dnd5ev1alpha1.ChoiceData, 0, len(choices))
	for _, choice := range choices {
		protoChoice := &dnd5ev1alpha1.ChoiceData{
			Category: convertToolkitCategoryToProto(choice.Category),
			Source:   convertToolkitSourceToProto(choice.Source),
			ChoiceId: choice.ChoiceID,
		}

		// Convert selection based on category
		switch choice.Category {
		case shared.ChoiceSkills:
			if len(choice.SkillSelection) > 0 {
				skills := make([]dnd5ev1alpha1.Skill, 0, len(choice.SkillSelection))
				for _, s := range choice.SkillSelection {
					skills = append(skills, convertSkillToProto(s))
				}
				protoChoice.Selection = &dnd5ev1alpha1.ChoiceData_Skills{
					Skills: &dnd5ev1alpha1.SkillList{
						Skills: skills,
					},
				}
			}
		case shared.ChoiceLanguages:
			if len(choice.LanguageSelection) > 0 {
				languages := make([]dnd5ev1alpha1.Language, 0, len(choice.LanguageSelection))
				for _, l := range choice.LanguageSelection {
					languages = append(languages, convertLanguageToProto(l))
				}
				protoChoice.Selection = &dnd5ev1alpha1.ChoiceData_Languages{
					Languages: &dnd5ev1alpha1.LanguageList{
						Languages: languages,
					},
				}
			}
		case shared.ChoiceAbilityScores:
			if choice.AbilityScoreSelection != nil && len(*choice.AbilityScoreSelection) > 0 {
				// Convert toolkit AbilityScores map to proto AbilityScores struct
				protoScores := &dnd5ev1alpha1.AbilityScores{}
				for ability, value := range *choice.AbilityScoreSelection {
					switch ability {
					case abilities.STR:
						protoScores.Strength = int32(value)
					case abilities.DEX:
						protoScores.Dexterity = int32(value)
					case abilities.CON:
						protoScores.Constitution = int32(value)
					case abilities.INT:
						protoScores.Intelligence = int32(value)
					case abilities.WIS:
						protoScores.Wisdom = int32(value)
					case abilities.CHA:
						protoScores.Charisma = int32(value)
					}
				}
				protoChoice.Selection = &dnd5ev1alpha1.ChoiceData_AbilityScores{
					AbilityScores: protoScores,
				}
			}
		case shared.ChoiceFightingStyle:
			if choice.FightingStyleSelection != nil && *choice.FightingStyleSelection != "" {
				protoChoice.Selection = &dnd5ev1alpha1.ChoiceData_FightingStyle{
					FightingStyle: string(*choice.FightingStyleSelection),
				}
			}
		case shared.ChoiceEquipment:
			if len(choice.EquipmentSelection) > 0 {
				protoChoice.Selection = &dnd5ev1alpha1.ChoiceData_Equipment{
					Equipment: &dnd5ev1alpha1.EquipmentList{
						Items: choice.EquipmentSelection,
					},
				}
			}
		case shared.ChoiceSpells:
			if len(choice.SpellSelection) > 0 {
				protoChoice.Selection = &dnd5ev1alpha1.ChoiceData_Spells{
					Spells: &dnd5ev1alpha1.SpellList{
						Spells: choice.SpellSelection,
					},
				}
			}
		case shared.ChoiceCantrips:
			if len(choice.CantripSelection) > 0 {
				protoChoice.Selection = &dnd5ev1alpha1.ChoiceData_Cantrips{
					Cantrips: &dnd5ev1alpha1.CantripList{
						Cantrips: choice.CantripSelection,
					},
				}
			}
		case shared.ChoiceExpertise:
			if len(choice.ExpertiseSelection) > 0 {
				// Expertise selections are skill names
				protoChoice.Selection = &dnd5ev1alpha1.ChoiceData_Expertise{
					Expertise: &dnd5ev1alpha1.ExpertiseList{
						Skills: choice.ExpertiseSelection,
					},
				}
			}
		case shared.ChoiceTraits:
			if len(choice.TraitSelection) > 0 {
				protoChoice.Selection = &dnd5ev1alpha1.ChoiceData_Traits{
					Traits: &dnd5ev1alpha1.TraitList{
						Traits: choice.TraitSelection,
					},
				}
			}
		case shared.ChoiceToolProficiency:
			if len(choice.ToolProficiencySelection) > 0 {
				protoChoice.Selection = &dnd5ev1alpha1.ChoiceData_ToolProficiencies{
					ToolProficiencies: &dnd5ev1alpha1.ToolProficiencyList{
						Tools: choice.ToolProficiencySelection,
					},
				}
			}
		default:
			// For other types, no selection data
		}

		protoChoices = append(protoChoices, protoChoice)
	}

	return protoChoices
}

// convertToolkitCategoryToProto converts toolkit ChoiceCategory to proto
func convertToolkitCategoryToProto(category shared.ChoiceCategory) dnd5ev1alpha1.ChoiceCategory {
	switch category {
	case shared.ChoiceEquipment:
		return dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT
	case shared.ChoiceSkills:
		return dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS
	// ChoiceTools doesn't exist in shared constants, map tool choices differently
	case shared.ChoiceLanguages:
		return dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_LANGUAGES
	case shared.ChoiceSpells:
		return dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SPELLS
	// ChoiceFeats doesn't exist in shared constants
	case shared.ChoiceAbilityScores:
		return dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_ABILITY_SCORES
	case shared.ChoiceName:
		return dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_NAME
	case shared.ChoiceFightingStyle:
		return dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_FIGHTING_STYLE
	case shared.ChoiceRace:
		return dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_RACE
	case shared.ChoiceClass:
		return dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_CLASS
	case shared.ChoiceBackground:
		return dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_BACKGROUND
	case shared.ChoiceCantrips:
		return dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_CANTRIPS
	case shared.ChoiceExpertise:
		return dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EXPERTISE
	case shared.ChoiceSubrace:
		return dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SUBRACE
	case shared.ChoiceTraits:
		return dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_TRAITS
	case shared.ChoiceToolProficiency:
		return dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_TOOLS
	default:
		return dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_UNSPECIFIED
	}
}

// convertToolkitSourceToProto converts toolkit ChoiceSource to proto
func convertToolkitSourceToProto(source shared.ChoiceSource) dnd5ev1alpha1.ChoiceSource {
	switch source {
	case shared.SourceRace:
		return dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_RACE
	case shared.SourceClass:
		return dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS
	case shared.SourceBackground:
		return dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_BACKGROUND
	case shared.SourcePlayer:
		return dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_PLAYER
	default:
		return dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_UNSPECIFIED
	}
}

// convertToolkitRaceToProtoEnum converts toolkit Race constant to proto Race enum
func convertToolkitRaceToProtoEnum(raceID races.Race) dnd5ev1alpha1.Race {
	switch raceID {
	case races.Dragonborn:
		return dnd5ev1alpha1.Race_RACE_DRAGONBORN
	case races.Dwarf:
		return dnd5ev1alpha1.Race_RACE_DWARF
	case races.Elf:
		return dnd5ev1alpha1.Race_RACE_ELF
	case races.Gnome:
		return dnd5ev1alpha1.Race_RACE_GNOME
	case races.HalfElf:
		return dnd5ev1alpha1.Race_RACE_HALF_ELF
	case races.Halfling:
		return dnd5ev1alpha1.Race_RACE_HALFLING
	case races.HalfOrc:
		return dnd5ev1alpha1.Race_RACE_HALF_ORC
	case races.Human:
		return dnd5ev1alpha1.Race_RACE_HUMAN
	case races.Tiefling:
		return dnd5ev1alpha1.Race_RACE_TIEFLING
	default:
		return dnd5ev1alpha1.Race_RACE_UNSPECIFIED
	}
}

// convertToolkitSubraceToProtoEnum converts toolkit Subrace constant to proto Subrace enum
func convertToolkitSubraceToProtoEnum(subraceID races.Race) dnd5ev1alpha1.Subrace {
	switch subraceID {
	case races.MountainDwarf:
		return dnd5ev1alpha1.Subrace_SUBRACE_MOUNTAIN_DWARF
	case races.HillDwarf:
		return dnd5ev1alpha1.Subrace_SUBRACE_HILL_DWARF
	case races.HighElf:
		return dnd5ev1alpha1.Subrace_SUBRACE_HIGH_ELF
	case races.WoodElf:
		return dnd5ev1alpha1.Subrace_SUBRACE_WOOD_ELF
	case races.DarkElf:
		return dnd5ev1alpha1.Subrace_SUBRACE_DARK_ELF
	case races.LightfootHalfling:
		return dnd5ev1alpha1.Subrace_SUBRACE_LIGHTFOOT_HALFLING
	case races.StoutHalfling:
		return dnd5ev1alpha1.Subrace_SUBRACE_STOUT_HALFLING
	case races.ForestGnome:
		return dnd5ev1alpha1.Subrace_SUBRACE_FOREST_GNOME
	case races.RockGnome:
		return dnd5ev1alpha1.Subrace_SUBRACE_ROCK_GNOME
	default:
		return dnd5ev1alpha1.Subrace_SUBRACE_UNSPECIFIED
	}
}

// convertToolkitClassToProtoEnum converts toolkit Class constant to proto Class enum
func convertToolkitClassToProtoEnum(classID classes.Class) dnd5ev1alpha1.Class {
	switch classID {
	case classes.Barbarian:
		return dnd5ev1alpha1.Class_CLASS_BARBARIAN
	case classes.Bard:
		return dnd5ev1alpha1.Class_CLASS_BARD
	case classes.Cleric:
		return dnd5ev1alpha1.Class_CLASS_CLERIC
	case classes.Druid:
		return dnd5ev1alpha1.Class_CLASS_DRUID
	case classes.Fighter:
		return dnd5ev1alpha1.Class_CLASS_FIGHTER
	case classes.Monk:
		return dnd5ev1alpha1.Class_CLASS_MONK
	case classes.Paladin:
		return dnd5ev1alpha1.Class_CLASS_PALADIN
	case classes.Ranger:
		return dnd5ev1alpha1.Class_CLASS_RANGER
	case classes.Rogue:
		return dnd5ev1alpha1.Class_CLASS_ROGUE
	case classes.Sorcerer:
		return dnd5ev1alpha1.Class_CLASS_SORCERER
	case classes.Warlock:
		return dnd5ev1alpha1.Class_CLASS_WARLOCK
	case classes.Wizard:
		return dnd5ev1alpha1.Class_CLASS_WIZARD
	default:
		return dnd5ev1alpha1.Class_CLASS_UNSPECIFIED
	}
}

// convertProtoClassToToolkit converts proto Class enum to toolkit class constant
func convertProtoClassToToolkit(class dnd5ev1alpha1.Class) classes.Class {
	switch class {
	case dnd5ev1alpha1.Class_CLASS_BARBARIAN:
		return classes.Barbarian
	case dnd5ev1alpha1.Class_CLASS_BARD:
		return classes.Bard
	case dnd5ev1alpha1.Class_CLASS_CLERIC:
		return classes.Cleric
	case dnd5ev1alpha1.Class_CLASS_DRUID:
		return classes.Druid
	case dnd5ev1alpha1.Class_CLASS_FIGHTER:
		return classes.Fighter
	case dnd5ev1alpha1.Class_CLASS_MONK:
		return classes.Monk
	case dnd5ev1alpha1.Class_CLASS_PALADIN:
		return classes.Paladin
	case dnd5ev1alpha1.Class_CLASS_RANGER:
		return classes.Ranger
	case dnd5ev1alpha1.Class_CLASS_ROGUE:
		return classes.Rogue
	case dnd5ev1alpha1.Class_CLASS_SORCERER:
		return classes.Sorcerer
	case dnd5ev1alpha1.Class_CLASS_WARLOCK:
		return classes.Warlock
	case dnd5ev1alpha1.Class_CLASS_WIZARD:
		return classes.Wizard
	default:
		return ""
	}
}

// convertProtoBackgroundToToolkit converts proto Background enum to toolkit background type
func convertProtoBackgroundToToolkit(background dnd5ev1alpha1.Background) backgrounds.Background {
	switch background {
	case dnd5ev1alpha1.Background_BACKGROUND_ACOLYTE:
		return backgrounds.Acolyte
	case dnd5ev1alpha1.Background_BACKGROUND_CHARLATAN:
		return backgrounds.Charlatan
	case dnd5ev1alpha1.Background_BACKGROUND_CRIMINAL:
		return backgrounds.Criminal
	case dnd5ev1alpha1.Background_BACKGROUND_ENTERTAINER:
		return backgrounds.Entertainer
	case dnd5ev1alpha1.Background_BACKGROUND_FOLK_HERO:
		return backgrounds.FolkHero
	case dnd5ev1alpha1.Background_BACKGROUND_GUILD_ARTISAN:
		return backgrounds.GuildArtisan
	case dnd5ev1alpha1.Background_BACKGROUND_HERMIT:
		return backgrounds.Hermit
	case dnd5ev1alpha1.Background_BACKGROUND_NOBLE:
		return backgrounds.Noble
	case dnd5ev1alpha1.Background_BACKGROUND_OUTLANDER:
		return backgrounds.Outlander
	case dnd5ev1alpha1.Background_BACKGROUND_SAGE:
		return backgrounds.Sage
	case dnd5ev1alpha1.Background_BACKGROUND_SAILOR:
		return backgrounds.Sailor
	case dnd5ev1alpha1.Background_BACKGROUND_SOLDIER:
		return backgrounds.Soldier
	case dnd5ev1alpha1.Background_BACKGROUND_URCHIN:
		return backgrounds.Urchin
	default:
		return ""
	}
}

// convertProtoSubclassToToolkit converts proto Subclass enum to toolkit subclass constant
func convertProtoSubclassToToolkit(subclass dnd5ev1alpha1.Subclass) classes.Subclass {
	switch subclass {
	// Cleric domains
	case dnd5ev1alpha1.Subclass_SUBCLASS_LIFE_DOMAIN:
		return classes.LifeDomain
	case dnd5ev1alpha1.Subclass_SUBCLASS_KNOWLEDGE_DOMAIN:
		return classes.KnowledgeDomain
	case dnd5ev1alpha1.Subclass_SUBCLASS_LIGHT_DOMAIN:
		return classes.LightDomain
	case dnd5ev1alpha1.Subclass_SUBCLASS_NATURE_DOMAIN:
		return classes.NatureDomain
	case dnd5ev1alpha1.Subclass_SUBCLASS_TEMPEST_DOMAIN:
		return classes.TempestDomain
	case dnd5ev1alpha1.Subclass_SUBCLASS_TRICKERY_DOMAIN:
		return classes.TrickeryDomain
	case dnd5ev1alpha1.Subclass_SUBCLASS_WAR_DOMAIN:
		return classes.WarDomain
	// Sorcerer origins
	case dnd5ev1alpha1.Subclass_SUBCLASS_DRACONIC_BLOODLINE:
		return classes.DraconicBloodline
	case dnd5ev1alpha1.Subclass_SUBCLASS_WILD_MAGIC:
		return classes.WildMagic
	// Warlock patrons
	case dnd5ev1alpha1.Subclass_SUBCLASS_ARCHFEY:
		return classes.Archfey
	case dnd5ev1alpha1.Subclass_SUBCLASS_FIEND:
		return classes.Fiend
	case dnd5ev1alpha1.Subclass_SUBCLASS_GREAT_OLD_ONE:
		return classes.GreatOldOne
	default:
		return ""
	}
}

// convertToolkitSubclassToProtoEnum converts toolkit Subclass constant to proto Subclass enum
func convertToolkitSubclassToProtoEnum(subclassID classes.Subclass) dnd5ev1alpha1.Subclass {
	switch subclassID {
	// Cleric domains
	case classes.LifeDomain:
		return dnd5ev1alpha1.Subclass_SUBCLASS_LIFE_DOMAIN
	case classes.KnowledgeDomain:
		return dnd5ev1alpha1.Subclass_SUBCLASS_KNOWLEDGE_DOMAIN
	case classes.LightDomain:
		return dnd5ev1alpha1.Subclass_SUBCLASS_LIGHT_DOMAIN
	case classes.NatureDomain:
		return dnd5ev1alpha1.Subclass_SUBCLASS_NATURE_DOMAIN
	case classes.TempestDomain:
		return dnd5ev1alpha1.Subclass_SUBCLASS_TEMPEST_DOMAIN
	case classes.TrickeryDomain:
		return dnd5ev1alpha1.Subclass_SUBCLASS_TRICKERY_DOMAIN
	case classes.WarDomain:
		return dnd5ev1alpha1.Subclass_SUBCLASS_WAR_DOMAIN
	// Sorcerer origins
	case classes.DraconicBloodline:
		return dnd5ev1alpha1.Subclass_SUBCLASS_DRACONIC_BLOODLINE
	case classes.WildMagic:
		return dnd5ev1alpha1.Subclass_SUBCLASS_WILD_MAGIC
	// Warlock patrons
	case classes.Archfey:
		return dnd5ev1alpha1.Subclass_SUBCLASS_ARCHFEY
	case classes.Fiend:
		return dnd5ev1alpha1.Subclass_SUBCLASS_FIEND
	case classes.GreatOldOne:
		return dnd5ev1alpha1.Subclass_SUBCLASS_GREAT_OLD_ONE
	default:
		return dnd5ev1alpha1.Subclass_SUBCLASS_UNSPECIFIED
	}
}

// convertToolkitBackgroundToProtoEnum converts toolkit Background constant to proto Background enum
func convertToolkitBackgroundToProtoEnum(backgroundID backgrounds.Background) dnd5ev1alpha1.Background {
	switch backgroundID {
	case backgrounds.Acolyte:
		return dnd5ev1alpha1.Background_BACKGROUND_ACOLYTE
	case backgrounds.Charlatan:
		return dnd5ev1alpha1.Background_BACKGROUND_CHARLATAN
	case backgrounds.Criminal:
		return dnd5ev1alpha1.Background_BACKGROUND_CRIMINAL
	case backgrounds.Entertainer:
		return dnd5ev1alpha1.Background_BACKGROUND_ENTERTAINER
	case backgrounds.FolkHero:
		return dnd5ev1alpha1.Background_BACKGROUND_FOLK_HERO
	case backgrounds.GuildArtisan:
		return dnd5ev1alpha1.Background_BACKGROUND_GUILD_ARTISAN
	case backgrounds.Hermit:
		return dnd5ev1alpha1.Background_BACKGROUND_HERMIT
	case backgrounds.Noble:
		return dnd5ev1alpha1.Background_BACKGROUND_NOBLE
	case backgrounds.Outlander:
		return dnd5ev1alpha1.Background_BACKGROUND_OUTLANDER
	case backgrounds.Sage:
		return dnd5ev1alpha1.Background_BACKGROUND_SAGE
	case backgrounds.Sailor:
		return dnd5ev1alpha1.Background_BACKGROUND_SAILOR
	case backgrounds.Soldier:
		return dnd5ev1alpha1.Background_BACKGROUND_SOLDIER
	case backgrounds.Urchin:
		return dnd5ev1alpha1.Background_BACKGROUND_URCHIN
	default:
		return dnd5ev1alpha1.Background_BACKGROUND_UNSPECIFIED
	}
}

// convertToolkitAbilityScoresToProto converts toolkit AbilityScores to proto AbilityScores
func convertToolkitAbilityScoresToProto(scores shared.AbilityScores) *dnd5ev1alpha1.AbilityScores {
	protoScores := &dnd5ev1alpha1.AbilityScores{}

	for ability, value := range scores {
		switch ability {
		case abilities.STR:
			protoScores.Strength = int32(value)
		case abilities.DEX:
			protoScores.Dexterity = int32(value)
		case abilities.CON:
			protoScores.Constitution = int32(value)
		case abilities.INT:
			protoScores.Intelligence = int32(value)
		case abilities.WIS:
			protoScores.Wisdom = int32(value)
		case abilities.CHA:
			protoScores.Charisma = int32(value)
		}
	}

	return protoScores
}

// hasAbilityScores checks if ability scores have been set
func hasAbilityScores(scores shared.AbilityScores) bool {
	// Check if all ability scores are set (map should have 6 entries with values > 0)
	if len(scores) != 6 {
		return false
	}

	// Check each ability score is greater than 0
	for _, score := range scores {
		if score <= 0 {
			return false
		}
	}

	return true
}

// convertProtoRaceToToolkit converts proto Race enum to toolkit Race constant
func convertProtoRaceToToolkit(race dnd5ev1alpha1.Race) races.Race {
	// Map proto enum to toolkit constants - direct mapping, no strings
	switch race {
	case dnd5ev1alpha1.Race_RACE_DRAGONBORN:
		return races.Dragonborn
	case dnd5ev1alpha1.Race_RACE_DWARF:
		return races.Dwarf
	case dnd5ev1alpha1.Race_RACE_ELF:
		return races.Elf
	case dnd5ev1alpha1.Race_RACE_GNOME:
		return races.Gnome
	case dnd5ev1alpha1.Race_RACE_HALF_ELF:
		return races.HalfElf
	case dnd5ev1alpha1.Race_RACE_HALFLING:
		return races.Halfling
	case dnd5ev1alpha1.Race_RACE_HALF_ORC:
		return races.HalfOrc
	case dnd5ev1alpha1.Race_RACE_HUMAN:
		return races.Human
	case dnd5ev1alpha1.Race_RACE_TIEFLING:
		return races.Tiefling
	default:
		return ""
	}
}

// convertProtoSubraceToToolkit converts proto Subrace enum to toolkit Subrace constant
func convertProtoSubraceToToolkit(subrace dnd5ev1alpha1.Subrace) races.Race {
	// Map proto enum to toolkit constants - direct mapping, no strings
	switch subrace {
	case dnd5ev1alpha1.Subrace_SUBRACE_HILL_DWARF:
		return races.HillDwarf
	case dnd5ev1alpha1.Subrace_SUBRACE_MOUNTAIN_DWARF:
		return races.MountainDwarf
	case dnd5ev1alpha1.Subrace_SUBRACE_HIGH_ELF:
		return races.HighElf
	case dnd5ev1alpha1.Subrace_SUBRACE_WOOD_ELF:
		return races.WoodElf
	case dnd5ev1alpha1.Subrace_SUBRACE_DARK_ELF:
		return races.DarkElf
	case dnd5ev1alpha1.Subrace_SUBRACE_FOREST_GNOME:
		return races.ForestGnome
	case dnd5ev1alpha1.Subrace_SUBRACE_ROCK_GNOME:
		return races.RockGnome
	case dnd5ev1alpha1.Subrace_SUBRACE_LIGHTFOOT_HALFLING:
		return races.LightfootHalfling
	case dnd5ev1alpha1.Subrace_SUBRACE_STOUT_HALFLING:
		return races.StoutHalfling
	default:
		return ""
	}
}

// convertProtoChoiceDataToToolkit converts a single proto ChoiceData to toolkit ChoiceData
func convertProtoChoiceDataToToolkit(pc *dnd5ev1alpha1.ChoiceData) toolkitchar.ChoiceData {
	if pc == nil {
		return toolkitchar.ChoiceData{}
	}

	choice := toolkitchar.ChoiceData{
		ChoiceID: pc.GetChoiceId(),
		Category: convertProtoCategoryToToolkit(pc.GetCategory()),
		Source:   convertProtoSourceToToolkit(pc.GetSource()),
	}

	// Convert selection based on oneof pattern
	switch selection := pc.GetSelection().(type) {
	case *dnd5ev1alpha1.ChoiceData_Name:
		// Name selection (not used for choices, but included for completeness)
		choice.NameSelection = &selection.Name
	case *dnd5ev1alpha1.ChoiceData_Skills:
		// Convert skill list to skill constants
		skills := make([]skills.Skill, 0, len(selection.Skills.GetSkills()))
		for _, skill := range selection.Skills.GetSkills() {
			skills = append(skills, convertProtoSkillToToolkit(skill))
		}
		choice.SkillSelection = skills
	case *dnd5ev1alpha1.ChoiceData_Languages:
		// Convert language list to language constants
		languages := make([]languages.Language, 0, len(selection.Languages.GetLanguages()))
		for _, lang := range selection.Languages.GetLanguages() {
			languages = append(languages, convertProtoLanguageToToolkit(lang))
		}
		choice.LanguageSelection = languages
	case *dnd5ev1alpha1.ChoiceData_AbilityScores:
		// Convert ability scores to toolkit format
		scores := make(shared.AbilityScores)
		if selection.AbilityScores.GetStrength() > 0 {
			scores[abilities.STR] = int(selection.AbilityScores.GetStrength())
		}
		if selection.AbilityScores.GetDexterity() > 0 {
			scores[abilities.DEX] = int(selection.AbilityScores.GetDexterity())
		}
		if selection.AbilityScores.GetConstitution() > 0 {
			scores[abilities.CON] = int(selection.AbilityScores.GetConstitution())
		}
		if selection.AbilityScores.GetIntelligence() > 0 {
			scores[abilities.INT] = int(selection.AbilityScores.GetIntelligence())
		}
		if selection.AbilityScores.GetWisdom() > 0 {
			scores[abilities.WIS] = int(selection.AbilityScores.GetWisdom())
		}
		if selection.AbilityScores.GetCharisma() > 0 {
			scores[abilities.CHA] = int(selection.AbilityScores.GetCharisma())
		}
		choice.AbilityScoreSelection = &scores
	case *dnd5ev1alpha1.ChoiceData_FightingStyle:
		// Convert fighting style selection
		choice.FightingStyleSelection = &selection.FightingStyle
	case *dnd5ev1alpha1.ChoiceData_Equipment:
		// Convert equipment list
		choice.EquipmentSelection = selection.Equipment.GetItems()
	case *dnd5ev1alpha1.ChoiceData_Spells:
		// Convert spell list
		choice.SpellSelection = selection.Spells.GetSpells()
	case *dnd5ev1alpha1.ChoiceData_Cantrips:
		// Convert cantrip list
		choice.CantripSelection = selection.Cantrips.GetCantrips()
	case *dnd5ev1alpha1.ChoiceData_Expertise:
		// Convert expertise list (skill names for double proficiency)
		choice.ExpertiseSelection = selection.Expertise.GetSkills()
	case *dnd5ev1alpha1.ChoiceData_Traits:
		// Convert trait list
		choice.TraitSelection = selection.Traits.GetTraits()
	case *dnd5ev1alpha1.ChoiceData_ToolProficiencies:
		// Convert tool proficiency list
		choice.ToolProficiencySelection = selection.ToolProficiencies.GetTools()
	case *dnd5ev1alpha1.ChoiceData_Race:
		// Handle race choice (not typically used in current choice system)
	case *dnd5ev1alpha1.ChoiceData_Class:
		// Handle class choice (not typically used in current choice system)
	case *dnd5ev1alpha1.ChoiceData_Background:
		// Handle background choice (not typically used in current choice system)
	default:
		// No selection data
	}

	return choice
}

// convertProtoChoiceDataListToToolkit converts a list of proto ChoiceData to toolkit ChoiceData
func convertProtoChoiceDataListToToolkit(protoChoices []*dnd5ev1alpha1.ChoiceData) []toolkitchar.ChoiceData {
	if len(protoChoices) == 0 {
		return nil
	}

	toolkitChoices := make([]toolkitchar.ChoiceData, 0, len(protoChoices))
	for _, pc := range protoChoices {
		toolkitChoices = append(toolkitChoices, convertProtoChoiceDataToToolkit(pc))
	}
	return toolkitChoices
}

// convertProtoAbilityToString converts proto Ability enum to string
func convertProtoAbilityToString(ability dnd5ev1alpha1.Ability) string {
	switch ability {
	case dnd5ev1alpha1.Ability_ABILITY_STRENGTH:
		return "strength"
	case dnd5ev1alpha1.Ability_ABILITY_DEXTERITY:
		return "dexterity"
	case dnd5ev1alpha1.Ability_ABILITY_CONSTITUTION:
		return "constitution"
	case dnd5ev1alpha1.Ability_ABILITY_INTELLIGENCE:
		return "intelligence"
	case dnd5ev1alpha1.Ability_ABILITY_WISDOM:
		return "wisdom"
	case dnd5ev1alpha1.Ability_ABILITY_CHARISMA:
		return "charisma"
	default:
		return ""
	}
}

// convertProtoCategoryToToolkit converts proto ChoiceCategory to toolkit string
func convertProtoCategoryToToolkit(category dnd5ev1alpha1.ChoiceCategory) shared.ChoiceCategory {
	switch category {
	case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_NAME:
		return shared.ChoiceName
	case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS:
		return shared.ChoiceSkills
	case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_LANGUAGES:
		return shared.ChoiceLanguages
	case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_ABILITY_SCORES:
		return shared.ChoiceAbilityScores
	case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_FIGHTING_STYLE:
		return shared.ChoiceFightingStyle
	case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT:
		return shared.ChoiceEquipment
	case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_RACE:
		return shared.ChoiceRace
	case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_CLASS:
		return shared.ChoiceClass
	case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_BACKGROUND:
		return shared.ChoiceBackground
	case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SPELLS:
		return shared.ChoiceSpells
	case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_CANTRIPS:
		return shared.ChoiceCantrips
	case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EXPERTISE:
		return shared.ChoiceExpertise
	case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SUBRACE:
		return shared.ChoiceSubrace
	case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_TRAITS:
		return shared.ChoiceTraits
	case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_TOOLS:
		return shared.ChoiceToolProficiency
	default:
		return ""
	}
}

// convertProtoSourceToToolkit converts proto ChoiceSource to toolkit string
func convertProtoSourceToToolkit(source dnd5ev1alpha1.ChoiceSource) shared.ChoiceSource {
	switch source {
	case dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_RACE:
		return shared.SourceRace
	case dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS:
		return shared.SourceClass
	case dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_BACKGROUND:
		return shared.SourceBackground
	default:
		// For other sources, default to player choice
		return shared.SourcePlayer
	}
}

// convertProtoSkillToToolkit converts proto Skill enum to toolkit Skill constant
func convertProtoSkillToToolkit(skill dnd5ev1alpha1.Skill) skills.Skill {
	switch skill {
	case dnd5ev1alpha1.Skill_SKILL_UNSPECIFIED:
		// Return empty string for unspecified skill
		return ""
	case dnd5ev1alpha1.Skill_SKILL_ACROBATICS:
		return skills.Acrobatics
	case dnd5ev1alpha1.Skill_SKILL_ANIMAL_HANDLING:
		return skills.AnimalHandling
	case dnd5ev1alpha1.Skill_SKILL_ARCANA:
		return skills.Arcana
	case dnd5ev1alpha1.Skill_SKILL_ATHLETICS:
		return skills.Athletics
	case dnd5ev1alpha1.Skill_SKILL_DECEPTION:
		return skills.Deception
	case dnd5ev1alpha1.Skill_SKILL_HISTORY:
		return skills.History
	case dnd5ev1alpha1.Skill_SKILL_INSIGHT:
		return skills.Insight
	case dnd5ev1alpha1.Skill_SKILL_INTIMIDATION:
		return skills.Intimidation
	case dnd5ev1alpha1.Skill_SKILL_INVESTIGATION:
		return skills.Investigation
	case dnd5ev1alpha1.Skill_SKILL_MEDICINE:
		return skills.Medicine
	case dnd5ev1alpha1.Skill_SKILL_NATURE:
		return skills.Nature
	case dnd5ev1alpha1.Skill_SKILL_PERCEPTION:
		return skills.Perception
	case dnd5ev1alpha1.Skill_SKILL_PERFORMANCE:
		return skills.Performance
	case dnd5ev1alpha1.Skill_SKILL_PERSUASION:
		return skills.Persuasion
	case dnd5ev1alpha1.Skill_SKILL_RELIGION:
		return skills.Religion
	case dnd5ev1alpha1.Skill_SKILL_SLEIGHT_OF_HAND:
		return skills.SleightOfHand
	case dnd5ev1alpha1.Skill_SKILL_STEALTH:
		return skills.Stealth
	case dnd5ev1alpha1.Skill_SKILL_SURVIVAL:
		return skills.Survival
	default:
		// Return empty string for unknown skills to avoid hiding bugs
		return ""
	}
}

// convertProtoLanguageToToolkit converts proto Language enum to toolkit Language constant
func convertProtoLanguageToToolkit(lang dnd5ev1alpha1.Language) languages.Language {
	switch lang {
	case dnd5ev1alpha1.Language_LANGUAGE_UNSPECIFIED:
		// Return empty string for unspecified language
		return ""
	case dnd5ev1alpha1.Language_LANGUAGE_COMMON:
		return languages.Common
	case dnd5ev1alpha1.Language_LANGUAGE_DWARVISH:
		return languages.Dwarvish
	case dnd5ev1alpha1.Language_LANGUAGE_ELVISH:
		return languages.Elvish
	case dnd5ev1alpha1.Language_LANGUAGE_GIANT:
		return languages.Giant
	case dnd5ev1alpha1.Language_LANGUAGE_GNOMISH:
		return languages.Gnomish
	case dnd5ev1alpha1.Language_LANGUAGE_GOBLIN:
		return languages.Goblin
	case dnd5ev1alpha1.Language_LANGUAGE_HALFLING:
		return languages.Halfling
	case dnd5ev1alpha1.Language_LANGUAGE_ORC:
		return languages.Orc
	case dnd5ev1alpha1.Language_LANGUAGE_DRACONIC:
		return languages.Draconic
	case dnd5ev1alpha1.Language_LANGUAGE_INFERNAL:
		return languages.Infernal
	default:
		// Return empty string for unknown languages to avoid hiding bugs
		return ""
	}
}

// convertRaceDataToProtoInfo converts toolkit race data to proto RaceInfo
func convertRaceDataToProtoInfo(raceData *race.Data, uiData *external.RaceUIData) *dnd5ev1alpha1.RaceInfo {
	if raceData == nil {
		return nil
	}

	info := &dnd5ev1alpha1.RaceInfo{
		Id:          string(raceData.ID),
		Name:        raceData.Name,
		Speed:       int32(raceData.Speed),
		Size:        convertSizeStringToProto(raceData.Size),
		Description: raceData.Description,
	}

	if uiData != nil {
		// Use UI data for additional descriptions
		info.AgeDescription = uiData.AgeDescription
		info.AlignmentDescription = uiData.AlignmentDescription
		info.SizeDescription = uiData.SizeDescription
	}

	// Convert ability bonuses
	if len(raceData.AbilityScoreIncreases) > 0 {
		info.AbilityBonuses = make(map[string]int32)
		for ability, bonus := range raceData.AbilityScoreIncreases {
			info.AbilityBonuses[string(ability)] = int32(bonus)
		}
	}

	// Convert traits
	info.Traits = make([]*dnd5ev1alpha1.RacialTrait, 0, len(raceData.Traits))
	for _, trait := range raceData.Traits {
		info.Traits = append(info.Traits, &dnd5ev1alpha1.RacialTrait{
			Name:        trait.Name,
			Description: trait.Description,
		})
	}

	// TODO: Convert subraces when we have the data structure

	// Convert proficiencies
	info.Proficiencies = make([]string, 0)
	for _, prof := range raceData.SkillProficiencies {
		info.Proficiencies = append(info.Proficiencies, string(prof))
	}
	info.Proficiencies = append(info.Proficiencies, raceData.WeaponProficiencies...)
	info.Proficiencies = append(info.Proficiencies, raceData.ToolProficiencies...)

	// Convert languages
	info.Languages = make([]dnd5ev1alpha1.Language, 0, len(raceData.Languages))
	for _, lang := range raceData.Languages {
		info.Languages = append(info.Languages, convertLanguageToProto(lang))
	}

	// Convert choices
	info.Choices = make([]*dnd5ev1alpha1.Choice, 0)

	// Add language choice if present
	if raceData.LanguageChoice != nil {
		info.Choices = append(info.Choices, convertRaceChoiceToProto(raceData.LanguageChoice))
	}

	// Add skill choice if present
	if raceData.SkillChoice != nil {
		info.Choices = append(info.Choices, convertRaceChoiceToProto(raceData.SkillChoice))
	}

	// Add tool choice if present
	if raceData.ToolChoice != nil {
		info.Choices = append(info.Choices, convertRaceChoiceToProto(raceData.ToolChoice))
	}

	return info
}

// convertSizeStringToProto converts toolkit size string to proto Size
func convertSizeStringToProto(size string) dnd5ev1alpha1.Size {
	switch size {
	case "Tiny":
		return dnd5ev1alpha1.Size_SIZE_TINY
	case "Small":
		return dnd5ev1alpha1.Size_SIZE_SMALL
	case "Medium":
		return dnd5ev1alpha1.Size_SIZE_MEDIUM
	case "Large":
		return dnd5ev1alpha1.Size_SIZE_LARGE
	case "Huge":
		return dnd5ev1alpha1.Size_SIZE_HUGE
	case "Gargantuan":
		return dnd5ev1alpha1.Size_SIZE_GARGANTUAN
	default:
		return dnd5ev1alpha1.Size_SIZE_MEDIUM
	}
}

// convertSubraceToProtoInfo converts toolkit subrace to proto SubraceInfo
func convertSubraceToProtoInfo(subrace interface{}) *dnd5ev1alpha1.SubraceInfo {
	// TODO: Implement when we have subrace data structure
	return nil
}

// convertSizeToProto converts toolkit Size to proto Size
func convertSizeToProto(size shared.Size) dnd5ev1alpha1.Size {
	switch size {
	case shared.SizeTiny:
		return dnd5ev1alpha1.Size_SIZE_TINY
	case shared.SizeSmall:
		return dnd5ev1alpha1.Size_SIZE_SMALL
	case shared.SizeMedium:
		return dnd5ev1alpha1.Size_SIZE_MEDIUM
	case shared.SizeLarge:
		return dnd5ev1alpha1.Size_SIZE_LARGE
	case shared.SizeHuge:
		return dnd5ev1alpha1.Size_SIZE_HUGE
	case shared.SizeGargantuan:
		return dnd5ev1alpha1.Size_SIZE_GARGANTUAN
	default:
		return dnd5ev1alpha1.Size_SIZE_MEDIUM
	}
}

// convertResourceTypeToProto converts toolkit resource type string to proto enum
func convertResourceTypeToProto(resourceType shared.ClassResourceType) dnd5ev1alpha1.ClassResourceType {
	switch resourceType {
	case shared.ClassResourceRage:
		return dnd5ev1alpha1.ClassResourceType_CLASS_RESOURCE_TYPE_RAGE
	case shared.ClassResourceBardicInspiration:
		return dnd5ev1alpha1.ClassResourceType_CLASS_RESOURCE_TYPE_BARDIC_INSPIRATION
	case shared.ClassResourceChannelDivinity:
		return dnd5ev1alpha1.ClassResourceType_CLASS_RESOURCE_TYPE_CHANNEL_DIVINITY
	case shared.ClassResourceWildShape:
		return dnd5ev1alpha1.ClassResourceType_CLASS_RESOURCE_TYPE_WILD_SHAPE
	case shared.ClassResourceSecondWind:
		return dnd5ev1alpha1.ClassResourceType_CLASS_RESOURCE_TYPE_SECOND_WIND
	case shared.ClassResourceActionSurge:
		return dnd5ev1alpha1.ClassResourceType_CLASS_RESOURCE_TYPE_ACTION_SURGE
	case shared.ClassResourceKiPoints:
		return dnd5ev1alpha1.ClassResourceType_CLASS_RESOURCE_TYPE_KI_POINTS
	case shared.ClassResourceDivineSense:
		return dnd5ev1alpha1.ClassResourceType_CLASS_RESOURCE_TYPE_DIVINE_SENSE
	case shared.ClassResourceLayOnHands:
		return dnd5ev1alpha1.ClassResourceType_CLASS_RESOURCE_TYPE_LAY_ON_HANDS
	case shared.ClassResourceSorceryPoints:
		return dnd5ev1alpha1.ClassResourceType_CLASS_RESOURCE_TYPE_SORCERY_POINTS
	case shared.ClassResourceArcaneRecovery:
		return dnd5ev1alpha1.ClassResourceType_CLASS_RESOURCE_TYPE_ARCANE_RECOVERY
	case shared.ClassResourceIndomitable:
		return dnd5ev1alpha1.ClassResourceType_CLASS_RESOURCE_TYPE_INDOMITABLE
	case shared.ClassResourceSuperiorityDice:
		return dnd5ev1alpha1.ClassResourceType_CLASS_RESOURCE_TYPE_SUPERIORITY_DICE
	default:
		return dnd5ev1alpha1.ClassResourceType_CLASS_RESOURCE_TYPE_UNSPECIFIED
	}
}

// convertRechargeTypeToProto converts toolkit recharge type to proto enum
func convertRechargeTypeToProto(rechargeOn shared.ResetType) dnd5ev1alpha1.RechargeType {
	switch rechargeOn {
	case shared.ResetTypeShortRest:
		return dnd5ev1alpha1.RechargeType_RECHARGE_TYPE_SHORT_REST
	case shared.ResetTypeLongRest:
		return dnd5ev1alpha1.RechargeType_RECHARGE_TYPE_LONG_REST
	case shared.ResetTypeDawn:
		return dnd5ev1alpha1.RechargeType_RECHARGE_TYPE_DAWN
	case shared.ResetTypeNone:
		return dnd5ev1alpha1.RechargeType_RECHARGE_TYPE_NONE
	default:
		return dnd5ev1alpha1.RechargeType_RECHARGE_TYPE_UNSPECIFIED
	}
}

// convertLanguageToProto converts toolkit Language to proto Language
func convertLanguageToProto(lang languages.Language) dnd5ev1alpha1.Language {
	// Map toolkit language constants to proto enums
	// This is a simplified mapping - you may need to expand based on available languages
	switch lang {
	case languages.Common:
		return dnd5ev1alpha1.Language_LANGUAGE_COMMON
	case languages.Dwarvish:
		return dnd5ev1alpha1.Language_LANGUAGE_DWARVISH
	case languages.Elvish:
		return dnd5ev1alpha1.Language_LANGUAGE_ELVISH
	case languages.Giant:
		return dnd5ev1alpha1.Language_LANGUAGE_GIANT
	case languages.Gnomish:
		return dnd5ev1alpha1.Language_LANGUAGE_GNOMISH
	case languages.Goblin:
		return dnd5ev1alpha1.Language_LANGUAGE_GOBLIN
	case languages.Halfling:
		return dnd5ev1alpha1.Language_LANGUAGE_HALFLING
	case languages.Orc:
		return dnd5ev1alpha1.Language_LANGUAGE_ORC
	case languages.Draconic:
		return dnd5ev1alpha1.Language_LANGUAGE_DRACONIC
	case languages.Infernal:
		return dnd5ev1alpha1.Language_LANGUAGE_INFERNAL
	default:
		return dnd5ev1alpha1.Language_LANGUAGE_COMMON
	}
}

// convertRequirementsToProtoChoices converts toolkit Requirements to proto Choice messages
func convertRequirementsToProtoChoices(reqs *choices.Requirements) []*dnd5ev1alpha1.Choice {
	if reqs == nil {
		return nil
	}

	var choices []*dnd5ev1alpha1.Choice

	// Convert skill requirements
	if reqs.Skills != nil && reqs.Skills.Count > 0 {
		skillChoice := &dnd5ev1alpha1.Choice{
			Id:          "class-skills",
			Description: reqs.Skills.Label,
			ChooseCount: int32(reqs.Skills.Count),
			ChoiceType:  dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS,
		}

		// If specific options are provided, use them
		if len(reqs.Skills.Options) > 0 {
			options := make([]*dnd5ev1alpha1.ChoiceOption, 0, len(reqs.Skills.Options))
			for _, skill := range reqs.Skills.Options {
				options = append(options, &dnd5ev1alpha1.ChoiceOption{
					OptionType: &dnd5ev1alpha1.ChoiceOption_Item{
						Item: &dnd5ev1alpha1.ItemReference{
							ItemId: string(skill),
							Name:   string(skill),
						},
					},
				})
			}
			skillChoice.OptionSet = &dnd5ev1alpha1.Choice_ExplicitOptions{
				ExplicitOptions: &dnd5ev1alpha1.ExplicitOptions{
					Options: options,
				},
			}
		} else {
			// Reference all skills category
			skillChoice.OptionSet = &dnd5ev1alpha1.Choice_CategoryReference{
				CategoryReference: &dnd5ev1alpha1.CategoryReference{
					CategoryId: "skills",
				},
			}
		}
		choices = append(choices, skillChoice)
	}

	// Convert cantrip requirements
	if reqs.Cantrips != nil && reqs.Cantrips.Count > 0 {
		cantripChoice := &dnd5ev1alpha1.Choice{
			Id:          "class-cantrips",
			Description: reqs.Cantrips.Label,
			ChooseCount: int32(reqs.Cantrips.Count),
			ChoiceType:  dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_CANTRIPS,
		}

		// For now, default to all cantrips (TODO: extract spell list from label if needed)
		cantripChoice.OptionSet = &dnd5ev1alpha1.Choice_CategoryReference{
			CategoryReference: &dnd5ev1alpha1.CategoryReference{
				CategoryId: "cantrips",
			},
		}
		choices = append(choices, cantripChoice)
	}

	// Convert spell requirements (1st level spells)
	if reqs.Spells != nil && reqs.Spells.Count > 0 {
		spellChoice := &dnd5ev1alpha1.Choice{
			Id:          "class-spells",
			Description: reqs.Spells.Label,
			ChooseCount: int32(reqs.Spells.Count),
			ChoiceType:  dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SPELLS,
		}

		// Default to level 1 spells (TODO: extract spell list from label if needed)
		spellChoice.OptionSet = &dnd5ev1alpha1.Choice_CategoryReference{
			CategoryReference: &dnd5ev1alpha1.CategoryReference{
				CategoryId: fmt.Sprintf("spells-%d", reqs.Spells.Level),
			},
		}
		choices = append(choices, spellChoice)
	}

	// Convert language requirements
	if reqs.Languages != nil && reqs.Languages.Count > 0 {
		langChoice := &dnd5ev1alpha1.Choice{
			Id:          "class-languages",
			Description: reqs.Languages.Label,
			ChooseCount: int32(reqs.Languages.Count),
			ChoiceType:  dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_LANGUAGES,
		}

		// Reference languages category
		langChoice.OptionSet = &dnd5ev1alpha1.Choice_CategoryReference{
			CategoryReference: &dnd5ev1alpha1.CategoryReference{
				CategoryId: "languages",
			},
		}
		choices = append(choices, langChoice)
	}

	// Convert tool requirements
	if reqs.Tools != nil && reqs.Tools.Count > 0 {
		toolChoice := &dnd5ev1alpha1.Choice{
			Id:          "class-tools",
			Description: reqs.Tools.Label,
			ChooseCount: int32(reqs.Tools.Count),
			ChoiceType:  dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_TOOLS,
		}

		// If specific options are provided
		if len(reqs.Tools.Options) > 0 {
			options := make([]*dnd5ev1alpha1.ChoiceOption, 0, len(reqs.Tools.Options))
			for _, tool := range reqs.Tools.Options {
				options = append(options, &dnd5ev1alpha1.ChoiceOption{
					OptionType: &dnd5ev1alpha1.ChoiceOption_Item{
						Item: &dnd5ev1alpha1.ItemReference{
							ItemId: string(tool),
							Name:   string(tool),
						},
					},
				})
			}
			toolChoice.OptionSet = &dnd5ev1alpha1.Choice_ExplicitOptions{
				ExplicitOptions: &dnd5ev1alpha1.ExplicitOptions{
					Options: options,
				},
			}
		} else {
			// Reference tools category
			toolChoice.OptionSet = &dnd5ev1alpha1.Choice_CategoryReference{
				CategoryReference: &dnd5ev1alpha1.CategoryReference{
					CategoryId: "tools",
				},
			}
		}
		choices = append(choices, toolChoice)
	}

	// Convert equipment requirements
	for i, eqReq := range reqs.Equipment {
		if eqReq == nil {
			continue
		}

		eqChoice := &dnd5ev1alpha1.Choice{
			Id:          fmt.Sprintf("class-equipment-%d", i+1),
			Description: eqReq.Label,
			ChooseCount: 1, // Equipment choices are typically "choose 1 option"
			ChoiceType:  dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
		}

		// Convert equipment options
		options := make([]*dnd5ev1alpha1.ChoiceOption, 0, len(eqReq.Options))
		for _, opt := range eqReq.Options {
			// Each option can be a single item or a bundle
			if len(opt.Items) == 1 && opt.Items[0].Quantity == 1 {
				// Single item
				options = append(options, &dnd5ev1alpha1.ChoiceOption{
					OptionType: &dnd5ev1alpha1.ChoiceOption_Item{
						Item: &dnd5ev1alpha1.ItemReference{
							ItemId: opt.Items[0].ID,
							Name:   opt.Items[0].ID, // TODO: Get proper name
						},
					},
				})
			} else if len(opt.Items) == 1 {
				// Single item with quantity
				options = append(options, &dnd5ev1alpha1.ChoiceOption{
					OptionType: &dnd5ev1alpha1.ChoiceOption_CountedItem{
						CountedItem: &dnd5ev1alpha1.CountedItemReference{
							ItemId:   opt.Items[0].ID,
							Name:     opt.Items[0].ID, // TODO: Get proper name
							Quantity: int32(opt.Items[0].Quantity),
						},
					},
				})
			} else {
				// Bundle of items
				bundle := &dnd5ev1alpha1.ItemBundle{
					Items: make([]*dnd5ev1alpha1.BundleItem, 0, len(opt.Items)),
				}
				for _, item := range opt.Items {
					bundle.Items = append(bundle.Items, &dnd5ev1alpha1.BundleItem{
						ItemType: &dnd5ev1alpha1.BundleItem_ConcreteItem{
							ConcreteItem: &dnd5ev1alpha1.CountedItemReference{
								ItemId:   item.ID,
								Name:     item.ID, // TODO: Get proper name
								Quantity: int32(item.Quantity),
							},
						},
					})
				}
				options = append(options, &dnd5ev1alpha1.ChoiceOption{
					OptionType: &dnd5ev1alpha1.ChoiceOption_Bundle{
						Bundle: bundle,
					},
				})
			}
		}

		eqChoice.OptionSet = &dnd5ev1alpha1.Choice_ExplicitOptions{
			ExplicitOptions: &dnd5ev1alpha1.ExplicitOptions{
				Options: options,
			},
		}
		choices = append(choices, eqChoice)
	}

	// Convert expertise requirements (for Rogue, etc.)
	if reqs.Expertise != nil && reqs.Expertise.Count > 0 {
		expertiseChoice := &dnd5ev1alpha1.Choice{
			Id:          "class-expertise",
			Description: reqs.Expertise.Label,
			ChooseCount: int32(reqs.Expertise.Count),
			ChoiceType:  dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EXPERTISE,
		}

		// Expertise is typically choosing from skills you already have proficiency in
		// or specific tools like thieves' tools
		// For now, reference a category that would be populated with valid options
		expertiseChoice.OptionSet = &dnd5ev1alpha1.Choice_CategoryReference{
			CategoryReference: &dnd5ev1alpha1.CategoryReference{
				CategoryId: "expertise-options", // UI would populate with skills the character has
			},
		}
		choices = append(choices, expertiseChoice)
	}

	return choices
}

// convertStartingClassToProtoInfo converts toolkit StartingClass to proto ClassInfo
func convertStartingClassToProtoInfo(sc *toolkitchar.StartingClass) *dnd5ev1alpha1.ClassInfo {
	if sc == nil {
		return nil
	}

	// TODO: Uncomment when proto v0.1.40 is available with Class field
	// classEnum := convertToolkitClassToProtoEnum(sc.ID)

	info := &dnd5ev1alpha1.ClassInfo{
		Id:          string(sc.ID),
		Name:        sc.ID.String(),      // Will be replaced with proper name from classes package
		Description: sc.ID.Description(), // Will be populated from classes package
		// Class:       classEnum,           // TODO: Add when proto v0.1.40 is available
	}

	// Extract data from grants if available
	if sc.Grants != nil {
		info.HitDie = fmt.Sprintf("1d%d", sc.Grants.HitDice)

		// Convert saving throw proficiencies
		info.SavingThrowProficiencies = make([]string, 0, len(sc.Grants.SavingThrows))
		for _, ability := range sc.Grants.SavingThrows {
			info.SavingThrowProficiencies = append(info.SavingThrowProficiencies, string(ability))
		}

		// Also use saving throws as primary abilities for now
		info.PrimaryAbilities = info.SavingThrowProficiencies

		// Convert weapon proficiencies
		info.WeaponProficiencies = make([]string, 0, len(sc.Grants.WeaponProficiencies))
		for _, prof := range sc.Grants.WeaponProficiencies {
			info.WeaponProficiencies = append(info.WeaponProficiencies, string(prof))
		}

		// Convert armor proficiencies
		info.ArmorProficiencies = make([]string, 0, len(sc.Grants.ArmorProficiencies))
		for _, prof := range sc.Grants.ArmorProficiencies {
			info.ArmorProficiencies = append(info.ArmorProficiencies, string(prof))
		}
	}

	// Extract skill choices from requirements
	if sc.Requirements != nil && sc.Requirements.Skills != nil {
		info.SkillChoicesCount = int32(sc.Requirements.Skills.Count)
		if sc.Requirements.Skills.Options != nil {
			info.AvailableSkills = make([]string, 0, len(sc.Requirements.Skills.Options))
			for _, skill := range sc.Requirements.Skills.Options {
				info.AvailableSkills = append(info.AvailableSkills, string(skill))
			}
		}
	}

	// Convert requirements to choices for the proto
	info.Choices = convertRequirementsToProtoChoices(sc.Requirements)

	// Convert subclasses if present
	if len(sc.Subclass) > 0 {
		info.SubclassType = string(sc.ID) // Base class type (e.g., "cleric")
		info.Subclasses = make([]*dnd5ev1alpha1.SubclassInfo, 0, len(sc.Subclass))
		for _, subclass := range sc.Subclass {
			info.Subclasses = append(info.Subclasses, convertSubclassToProtoInfo(subclass))
		}
	}

	// Set the group field to help with UI grouping
	// For classes with subclasses at level 1, ID is the subclass and Group is the base class
	// For other classes, ID and Group are the same
	if len(sc.Subclass) > 0 {
		// This is a base class with subclasses
		info.Group = convertToolkitClassToProtoEnum(sc.ID)
	} else {
		// This might be a regular class or it could be a flattened subclass entry
		// For now, we'll set group to the ID - this will be improved when SubclassInfo is available
		info.Group = convertToolkitClassToProtoEnum(sc.ID)
	}

	return info
}

// convertSubclassToProtoInfo converts a SubclassOption to proto SubclassInfo
func convertSubclassToProtoInfo(sc *toolkitchar.SubclassOption) *dnd5ev1alpha1.SubclassInfo {
	if sc == nil {
		return nil
	}

	// TODO: Uncomment when proto v0.1.40 is available with Subclass field
	// subclassEnum := convertToolkitSubclassToProtoEnum(sc.ID)
	
	info := &dnd5ev1alpha1.SubclassInfo{
		Id:          string(sc.ID), // TODO: Remove when proto v0.1.40 deprecates this
		Name:        string(sc.ID), // Will be replaced with proper name from classes package
		Description: "",            // Will be populated from classes package
		Level:       int32(sc.Level),
		// Subclass:    subclassEnum,  // TODO: Add when proto v0.1.40 is available
	}

	// Extract additional proficiencies this subclass grants
	if sc.Grants != nil {
		// Convert armor proficiencies
		info.ArmorProficiencies = make([]string, 0, len(sc.Grants.ArmorProficiencies))
		for _, prof := range sc.Grants.ArmorProficiencies {
			info.ArmorProficiencies = append(info.ArmorProficiencies, string(prof))
		}

		// Convert weapon proficiencies
		info.WeaponProficiencies = make([]string, 0, len(sc.Grants.WeaponProficiencies))
		for _, prof := range sc.Grants.WeaponProficiencies {
			info.WeaponProficiencies = append(info.WeaponProficiencies, string(prof))
		}

		// Convert tool proficiencies
		info.ToolProficiencies = make([]string, 0, len(sc.Grants.ToolProficiencies))
		for _, prof := range sc.Grants.ToolProficiencies {
			info.ToolProficiencies = append(info.ToolProficiencies, string(prof))
		}
	}

	// Convert additional requirements to choices
	info.AdditionalChoices = convertRequirementsToProtoChoices(sc.Requirements)

	return info
}

// convertClassDataToProtoInfo converts toolkit class data to proto ClassInfo
func convertClassDataToProtoInfo(classData *class.Data, uiData *external.ClassUIData) *dnd5ev1alpha1.ClassInfo {
	if classData == nil {
		return nil
	}

	info := &dnd5ev1alpha1.ClassInfo{
		Id:          string(classData.ID),
		Name:        classData.Name,
		Description: classData.Description,
		HitDie:      fmt.Sprintf("1d%d", classData.HitDice),
	}

	if uiData != nil {
		info.Description = uiData.Description
	}

	// Convert primary abilities - TODO: This field doesn't exist in toolkit, using saving throws for now
	info.PrimaryAbilities = make([]string, 0, len(classData.SavingThrows))
	for _, ability := range classData.SavingThrows {
		info.PrimaryAbilities = append(info.PrimaryAbilities, string(ability))
	}

	// Convert saving throw proficiencies
	info.SavingThrowProficiencies = make([]string, 0, len(classData.SavingThrows))
	for _, ability := range classData.SavingThrows {
		info.SavingThrowProficiencies = append(info.SavingThrowProficiencies, string(ability))
	}

	// Convert skill proficiencies
	info.SkillChoicesCount = int32(classData.SkillProficiencyCount)
	info.AvailableSkills = make([]string, 0, len(classData.SkillOptions))
	for _, skill := range classData.SkillOptions {
		info.AvailableSkills = append(info.AvailableSkills, string(skill))
	}

	// Convert weapon proficiencies
	info.WeaponProficiencies = make([]string, 0, len(classData.WeaponProficiencies))
	for _, prof := range classData.WeaponProficiencies {
		info.WeaponProficiencies = append(info.WeaponProficiencies, string(prof))
	}

	// Convert armor proficiencies
	info.ArmorProficiencies = make([]string, 0, len(classData.ArmorProficiencies))
	for _, prof := range classData.ArmorProficiencies {
		info.ArmorProficiencies = append(info.ArmorProficiencies, string(prof))
	}

	// Convert choices
	info.Choices = make([]*dnd5ev1alpha1.Choice, 0)

	// Add skill choice
	if classData.SkillProficiencyCount > 0 && len(classData.SkillOptions) > 0 {
		skillOptions := make([]*dnd5ev1alpha1.ChoiceOption, 0, len(classData.SkillOptions))
		for _, skill := range classData.SkillOptions {
			skillOptions = append(skillOptions, &dnd5ev1alpha1.ChoiceOption{
				OptionType: &dnd5ev1alpha1.ChoiceOption_Item{
					Item: &dnd5ev1alpha1.ItemReference{
						ItemId: fmt.Sprintf("skill_%s", skill),
						Name:   string(skill),
					},
				},
			})
		}

		info.Choices = append(info.Choices, &dnd5ev1alpha1.Choice{
			Id:          fmt.Sprintf("%s_skills", classData.ID),
			Description: fmt.Sprintf("Choose %d skills", classData.SkillProficiencyCount),
			ChooseCount: int32(classData.SkillProficiencyCount),
			ChoiceType:  dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS,
			OptionSet: &dnd5ev1alpha1.Choice_ExplicitOptions{
				ExplicitOptions: &dnd5ev1alpha1.ExplicitOptions{
					Options: skillOptions,
				},
			},
		})
	}

	// Add equipment choices
	equipmentChoices := convertEquipmentChoices(classData)
	info.Choices = append(info.Choices, equipmentChoices...)

	// Add feature choices (like fighting style)
	if features, ok := classData.Features[1]; ok {
		for _, feature := range features {
			if feature.Choice != nil {
				// Convert feature choice options
				featureOptions := make([]*dnd5ev1alpha1.ChoiceOption, 0, len(feature.Choice.From))
				for _, optionName := range feature.Choice.From {
					featureOptions = append(featureOptions, &dnd5ev1alpha1.ChoiceOption{
						OptionType: &dnd5ev1alpha1.ChoiceOption_Item{
							Item: &dnd5ev1alpha1.ItemReference{
								ItemId: fmt.Sprintf("feature_%s", strings.ToLower(strings.ReplaceAll(optionName, " ", "_"))),
								Name:   optionName,
							},
						},
					})
				}

				// Determine choice category based on feature type
				choiceCategory := dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_UNSPECIFIED
				if feature.Choice.Type == "fighting_style" {
					choiceCategory = dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_FIGHTING_STYLE
				}

				info.Choices = append(info.Choices, &dnd5ev1alpha1.Choice{
					Id:          fmt.Sprintf("%s_feature_%s", classData.ID, feature.Choice.ID),
					Description: feature.Choice.Description,
					ChooseCount: int32(feature.Choice.Choose),
					ChoiceType:  choiceCategory,
					OptionSet: &dnd5ev1alpha1.Choice_ExplicitOptions{
						ExplicitOptions: &dnd5ev1alpha1.ExplicitOptions{
							Options: featureOptions,
						},
					},
				})
			}
		}
	}

	return info
}

// convertAbilityToProto converts toolkit Ability to proto Ability
func convertAbilityToProto(ability abilities.Ability) dnd5ev1alpha1.Ability {
	switch ability {
	case abilities.STR:
		return dnd5ev1alpha1.Ability_ABILITY_STRENGTH
	case abilities.DEX:
		return dnd5ev1alpha1.Ability_ABILITY_DEXTERITY
	case abilities.CON:
		return dnd5ev1alpha1.Ability_ABILITY_CONSTITUTION
	case abilities.INT:
		return dnd5ev1alpha1.Ability_ABILITY_INTELLIGENCE
	case abilities.WIS:
		return dnd5ev1alpha1.Ability_ABILITY_WISDOM
	case abilities.CHA:
		return dnd5ev1alpha1.Ability_ABILITY_CHARISMA
	default:
		return dnd5ev1alpha1.Ability_ABILITY_STRENGTH
	}
}

// convertRaceChoiceToProto converts toolkit race.ChoiceData to proto Choice
func convertRaceChoiceToProto(choice *race.ChoiceData) *dnd5ev1alpha1.Choice {
	if choice == nil {
		return nil
	}

	protoChoice := &dnd5ev1alpha1.Choice{
		Id:          choice.ID,
		Description: choice.Description,
		ChooseCount: int32(choice.Choose),
	}

	// Convert choice type to category
	switch choice.Type {
	case "language":
		protoChoice.ChoiceType = dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_LANGUAGES
	case "skill":
		protoChoice.ChoiceType = dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS
	case "tool":
		protoChoice.ChoiceType = dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_TOOLS
	case "proficiency":
		// Default to skill proficiency, but could be weapon/armor/tool based on context
		protoChoice.ChoiceType = dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS
	default:
		protoChoice.ChoiceType = dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_UNSPECIFIED
	}

	// Build explicit options from the From field
	if len(choice.From) > 0 {
		options := make([]*dnd5ev1alpha1.ChoiceOption, 0, len(choice.From))
		for _, opt := range choice.From {
			options = append(options, &dnd5ev1alpha1.ChoiceOption{
				OptionType: &dnd5ev1alpha1.ChoiceOption_Item{
					Item: &dnd5ev1alpha1.ItemReference{
						ItemId: opt,
						Name:   formatOptionName(opt), // Convert key to display name
					},
				},
			})
		}
		protoChoice.OptionSet = &dnd5ev1alpha1.Choice_ExplicitOptions{
			ExplicitOptions: &dnd5ev1alpha1.ExplicitOptions{
				Options: options,
			},
		}
	}

	return protoChoice
}

// formatOptionName converts an option key to a display name
func formatOptionName(key string) string {
	// Convert snake_case or kebab-case to Title Case
	words := strings.FieldsFunc(key, func(r rune) bool {
		return r == '_' || r == '-'
	})
	for i, word := range words {
		words[i] = strings.Title(word)
	}
	return strings.Join(words, " ")
}

// convertSkillToProto converts toolkit Skill to proto Skill
func convertSkillToProto(skill skills.Skill) dnd5ev1alpha1.Skill {
	// This is a simplified mapping - you'll need to expand based on all skills
	switch skill {
	case skills.Acrobatics:
		return dnd5ev1alpha1.Skill_SKILL_ACROBATICS
	case skills.AnimalHandling:
		return dnd5ev1alpha1.Skill_SKILL_ANIMAL_HANDLING
	case skills.Arcana:
		return dnd5ev1alpha1.Skill_SKILL_ARCANA
	case skills.Athletics:
		return dnd5ev1alpha1.Skill_SKILL_ATHLETICS
	case skills.Deception:
		return dnd5ev1alpha1.Skill_SKILL_DECEPTION
	case skills.History:
		return dnd5ev1alpha1.Skill_SKILL_HISTORY
	case skills.Insight:
		return dnd5ev1alpha1.Skill_SKILL_INSIGHT
	case skills.Intimidation:
		return dnd5ev1alpha1.Skill_SKILL_INTIMIDATION
	case skills.Investigation:
		return dnd5ev1alpha1.Skill_SKILL_INVESTIGATION
	case skills.Medicine:
		return dnd5ev1alpha1.Skill_SKILL_MEDICINE
	case skills.Nature:
		return dnd5ev1alpha1.Skill_SKILL_NATURE
	case skills.Perception:
		return dnd5ev1alpha1.Skill_SKILL_PERCEPTION
	case skills.Performance:
		return dnd5ev1alpha1.Skill_SKILL_PERFORMANCE
	case skills.Persuasion:
		return dnd5ev1alpha1.Skill_SKILL_PERSUASION
	case skills.Religion:
		return dnd5ev1alpha1.Skill_SKILL_RELIGION
	case skills.SleightOfHand:
		return dnd5ev1alpha1.Skill_SKILL_SLEIGHT_OF_HAND
	case skills.Stealth:
		return dnd5ev1alpha1.Skill_SKILL_STEALTH
	case skills.Survival:
		return dnd5ev1alpha1.Skill_SKILL_SURVIVAL
	default:
		return dnd5ev1alpha1.Skill_SKILL_UNSPECIFIED
	}
}

// ConvertCharacterDataToProto converts toolkit character.Data to proto Character
func ConvertCharacterDataToProto(char *toolkitchar.Data) *dnd5ev1alpha1.Character {
	if char == nil {
		return nil
	}

	protoChar := &dnd5ev1alpha1.Character{
		Id:               char.ID,
		Name:             char.Name,
		Level:            int32(char.Level),
		ExperiencePoints: int32(char.Experience),

		// Race and class info
		Race:       convertToolkitRaceToProtoEnum(char.RaceID),
		Subrace:    convertToolkitSubraceToProtoEnum(char.SubraceID),
		Class:      convertToolkitClassToProtoEnum(char.ClassID),
		Background: convertToolkitBackgroundToProtoEnum(char.BackgroundID),

		// Hit points
		CurrentHitPoints: int32(char.HitPoints),

		// Ability scores
		AbilityScores: convertToolkitAbilityScoresToProto(char.AbilityScores),

		// Metadata
		Metadata: &dnd5ev1alpha1.CharacterMetadata{
			PlayerId:  char.PlayerID,
			CreatedAt: char.CreatedAt.Unix(),
			UpdatedAt: char.UpdatedAt.Unix(),
		},

		// Combat stats
		CombatStats: &dnd5ev1alpha1.CombatStats{
			HitPointMaximum: int32(char.MaxHitPoints),
			Speed:           int32(char.Speed),
			// TODO: Calculate other combat stats
		},
	}

	// Convert proficiencies
	protoChar.Proficiencies = &dnd5ev1alpha1.Proficiencies{
		Skills:       make([]dnd5ev1alpha1.Skill, 0),
		SavingThrows: make([]dnd5ev1alpha1.Ability, 0),
		Armor:        char.Proficiencies.Armor,
		Weapons:      char.Proficiencies.Weapons,
		Tools:        char.Proficiencies.Tools,
	}

	// Add skill proficiencies
	for skill, profLevel := range char.Skills {
		if profLevel > 0 {
			protoChar.Proficiencies.Skills = append(protoChar.Proficiencies.Skills, convertSkillToProto(skill))
		}
	}

	// Add saving throw proficiencies
	for ability, profLevel := range char.SavingThrows {
		if profLevel > 0 {
			protoChar.Proficiencies.SavingThrows = append(protoChar.Proficiencies.SavingThrows, convertAbilityToProto(ability))
		}
	}

	// Convert languages
	protoChar.Languages = make([]dnd5ev1alpha1.Language, 0, len(char.Languages))
	for _, lang := range char.Languages {
		// Convert string language to constant first
		protoChar.Languages = append(protoChar.Languages, convertLanguageToProto(languages.Language(lang)))
	}

	// Convert equipment to inventory
	protoChar.Inventory = make([]*dnd5ev1alpha1.InventoryItem, 0, len(char.Equipment))
	for _, equipmentID := range char.Equipment {
		protoChar.Inventory = append(protoChar.Inventory, &dnd5ev1alpha1.InventoryItem{
			ItemId:   equipmentID,
			Quantity: 1, // Default quantity for now
			// Equipment data would be populated if we had it
		})
	}

	// Extract fighting styles from choices
	fightingStyles := make([]string, 0)
	for _, choice := range char.Choices {
		if choice.Category == shared.ChoiceFightingStyle && choice.FightingStyleSelection != nil {
			fightingStyles = append(fightingStyles, *choice.FightingStyleSelection)
		}
	}
	protoChar.FightingStyles = fightingStyles

	// Convert class resources
	if char.ClassResources != nil {
		protoChar.ClassResources = make([]*dnd5ev1alpha1.ClassResource, 0, len(char.ClassResources))
		for resourceType, resource := range char.ClassResources {
			protoResource := &dnd5ev1alpha1.ClassResource{
				Type:     convertResourceTypeToProto(resourceType),
				Name:     resource.Name,
				Current:  int32(resource.Current),
				Maximum:  int32(resource.Max),
				Recharge: dnd5ev1alpha1.RechargeType_RECHARGE_TYPE_UNSPECIFIED, // TODO: Get from toolkit when available
			}
			protoChar.ClassResources = append(protoChar.ClassResources, protoResource)
		}
	}

	// Convert spell slots - toolkit uses map[int]SlotInfo
	if char.SpellSlots != nil && len(char.SpellSlots) > 0 {
		protoSlots := &dnd5ev1alpha1.SpellSlots{}

		// Map spell level to slot count
		for level, slotInfo := range char.SpellSlots {
			slotCount := int32(slotInfo.Max)
			switch level {
			case 1:
				protoSlots.Level_1 = slotCount
			case 2:
				protoSlots.Level_2 = slotCount
			case 3:
				protoSlots.Level_3 = slotCount
			case 4:
				protoSlots.Level_4 = slotCount
			case 5:
				protoSlots.Level_5 = slotCount
			case 6:
				protoSlots.Level_6 = slotCount
			case 7:
				protoSlots.Level_7 = slotCount
			case 8:
				protoSlots.Level_8 = slotCount
			case 9:
				protoSlots.Level_9 = slotCount
			}
		}

		protoChar.SpellSlots = protoSlots
	}

	// TODO: Convert features when available in toolkit
	// protoChar.Features = convertClassFeatures(char.Features)

	// TODO: Convert racial traits when available in toolkit
	// protoChar.RacialTraits = convertRacialTraits(char.RacialTraits)

	// TODO: Convert background feature when available in toolkit
	// protoChar.BackgroundFeature = convertBackgroundFeature(char.BackgroundFeature)

	// TODO(#168): Convert other fields as needed
	// - Conditions
	// - Effects
	// - DeathSaves

	return protoChar
}
