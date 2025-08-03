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
	"github.com/KirkDiggler/rpg-api/internal/errors"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/class"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/constants"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/race"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
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
		Choices:   convertProtoRaceChoicesToToolkit(req.GetRaceChoices()),
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

	// Convert proto choices to toolkit choices
	var choices []toolkitchar.ChoiceData
	for _, protoChoice := range req.ClassChoices {
		choices = append(choices, convertProtoChoiceSelectionToToolkit(protoChoice))
	}

	// Call orchestrator
	output, err := h.characterService.UpdateClass(ctx, &character.UpdateClassInput{
		DraftID: req.DraftId,
		ClassID: classID,
		Choices: choices,
	})
	if err != nil {
		return nil, err
	}

	// Convert response
	protoDraft := convertDraftDataToProto(output.Draft)

	// Convert warnings - no conversion needed for now
	var warnings []*dnd5ev1alpha1.ValidationWarning

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
	// Convert proto background to toolkit background ID
	backgroundID := convertProtoBackgroundToToolkitID(req.Background)
	if backgroundID == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid background")
	}

	// Convert proto choices to toolkit choices
	var choices []toolkitchar.ChoiceData
	for _, protoChoice := range req.BackgroundChoices {
		choices = append(choices, convertProtoChoiceSelectionToToolkit(protoChoice))
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
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

// GetCharacter retrieves a character
func (h *Handler) GetCharacter(
	ctx context.Context,
	req *dnd5ev1alpha1.GetCharacterRequest,
) (*dnd5ev1alpha1.GetCharacterResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

// ListCharacters lists characters
func (h *Handler) ListCharacters(
	ctx context.Context,
	req *dnd5ev1alpha1.ListCharactersRequest,
) (*dnd5ev1alpha1.ListCharactersResponse, error) {
	return &dnd5ev1alpha1.ListCharactersResponse{}, nil
}

// DeleteCharacter deletes a character
func (h *Handler) DeleteCharacter(
	ctx context.Context,
	req *dnd5ev1alpha1.DeleteCharacterRequest,
) (*dnd5ev1alpha1.DeleteCharacterResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
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
		protoClasses[i] = convertClassDataToProtoInfo(class.ClassData, class.UIData)
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
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

// EquipItem equips an item
func (h *Handler) EquipItem(
	ctx context.Context,
	req *dnd5ev1alpha1.EquipItemRequest,
) (*dnd5ev1alpha1.EquipItemResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

// UnequipItem unequips an item
func (h *Handler) UnequipItem(
	ctx context.Context,
	req *dnd5ev1alpha1.UnequipItemRequest,
) (*dnd5ev1alpha1.UnequipItemResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
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
					case constants.STR:
						protoScores.Strength = int32(value)
					case constants.DEX:
						protoScores.Dexterity = int32(value)
					case constants.CON:
						protoScores.Constitution = int32(value)
					case constants.INT:
						protoScores.Intelligence = int32(value)
					case constants.WIS:
						protoScores.Wisdom = int32(value)
					case constants.CHA:
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
func convertToolkitRaceToProtoEnum(raceID constants.Race) dnd5ev1alpha1.Race {
	switch raceID {
	case constants.RaceDragonborn:
		return dnd5ev1alpha1.Race_RACE_DRAGONBORN
	case constants.RaceDwarf:
		return dnd5ev1alpha1.Race_RACE_DWARF
	case constants.RaceElf:
		return dnd5ev1alpha1.Race_RACE_ELF
	case constants.RaceGnome:
		return dnd5ev1alpha1.Race_RACE_GNOME
	case constants.RaceHalfElf:
		return dnd5ev1alpha1.Race_RACE_HALF_ELF
	case constants.RaceHalfling:
		return dnd5ev1alpha1.Race_RACE_HALFLING
	case constants.RaceHalfOrc:
		return dnd5ev1alpha1.Race_RACE_HALF_ORC
	case constants.RaceHuman:
		return dnd5ev1alpha1.Race_RACE_HUMAN
	case constants.RaceTiefling:
		return dnd5ev1alpha1.Race_RACE_TIEFLING
	default:
		return dnd5ev1alpha1.Race_RACE_UNSPECIFIED
	}
}

// convertToolkitSubraceToProtoEnum converts toolkit Subrace constant to proto Subrace enum
func convertToolkitSubraceToProtoEnum(subraceID constants.Subrace) dnd5ev1alpha1.Subrace {
	switch subraceID {
	case constants.SubraceMountainDwarf:
		return dnd5ev1alpha1.Subrace_SUBRACE_MOUNTAIN_DWARF
	case constants.SubraceHillDwarf:
		return dnd5ev1alpha1.Subrace_SUBRACE_HILL_DWARF
	case constants.SubraceHighElf:
		return dnd5ev1alpha1.Subrace_SUBRACE_HIGH_ELF
	case constants.SubraceWoodElf:
		return dnd5ev1alpha1.Subrace_SUBRACE_WOOD_ELF
	case constants.SubraceDarkElf:
		return dnd5ev1alpha1.Subrace_SUBRACE_DARK_ELF
	case constants.SubraceLightfootHalfling:
		return dnd5ev1alpha1.Subrace_SUBRACE_LIGHTFOOT_HALFLING
	case constants.SubraceStoutHalfling:
		return dnd5ev1alpha1.Subrace_SUBRACE_STOUT_HALFLING
	case constants.SubraceForestGnome:
		return dnd5ev1alpha1.Subrace_SUBRACE_FOREST_GNOME
	case constants.SubraceRockGnome:
		return dnd5ev1alpha1.Subrace_SUBRACE_ROCK_GNOME
	default:
		return dnd5ev1alpha1.Subrace_SUBRACE_UNSPECIFIED
	}
}

// convertToolkitClassToProtoEnum converts toolkit Class constant to proto Class enum
func convertToolkitClassToProtoEnum(classID constants.Class) dnd5ev1alpha1.Class {
	switch classID {
	case constants.ClassBarbarian:
		return dnd5ev1alpha1.Class_CLASS_BARBARIAN
	case constants.ClassBard:
		return dnd5ev1alpha1.Class_CLASS_BARD
	case constants.ClassCleric:
		return dnd5ev1alpha1.Class_CLASS_CLERIC
	case constants.ClassDruid:
		return dnd5ev1alpha1.Class_CLASS_DRUID
	case constants.ClassFighter:
		return dnd5ev1alpha1.Class_CLASS_FIGHTER
	case constants.ClassMonk:
		return dnd5ev1alpha1.Class_CLASS_MONK
	case constants.ClassPaladin:
		return dnd5ev1alpha1.Class_CLASS_PALADIN
	case constants.ClassRanger:
		return dnd5ev1alpha1.Class_CLASS_RANGER
	case constants.ClassRogue:
		return dnd5ev1alpha1.Class_CLASS_ROGUE
	case constants.ClassSorcerer:
		return dnd5ev1alpha1.Class_CLASS_SORCERER
	case constants.ClassWarlock:
		return dnd5ev1alpha1.Class_CLASS_WARLOCK
	case constants.ClassWizard:
		return dnd5ev1alpha1.Class_CLASS_WIZARD
	default:
		return dnd5ev1alpha1.Class_CLASS_UNSPECIFIED
	}
}

// convertProtoClassToToolkit converts proto Class enum to toolkit class constant
func convertProtoClassToToolkit(class dnd5ev1alpha1.Class) constants.Class {
	switch class {
	case dnd5ev1alpha1.Class_CLASS_BARBARIAN:
		return constants.ClassBarbarian
	case dnd5ev1alpha1.Class_CLASS_BARD:
		return constants.ClassBard
	case dnd5ev1alpha1.Class_CLASS_CLERIC:
		return constants.ClassCleric
	case dnd5ev1alpha1.Class_CLASS_DRUID:
		return constants.ClassDruid
	case dnd5ev1alpha1.Class_CLASS_FIGHTER:
		return constants.ClassFighter
	case dnd5ev1alpha1.Class_CLASS_MONK:
		return constants.ClassMonk
	case dnd5ev1alpha1.Class_CLASS_PALADIN:
		return constants.ClassPaladin
	case dnd5ev1alpha1.Class_CLASS_RANGER:
		return constants.ClassRanger
	case dnd5ev1alpha1.Class_CLASS_ROGUE:
		return constants.ClassRogue
	case dnd5ev1alpha1.Class_CLASS_SORCERER:
		return constants.ClassSorcerer
	case dnd5ev1alpha1.Class_CLASS_WARLOCK:
		return constants.ClassWarlock
	case dnd5ev1alpha1.Class_CLASS_WIZARD:
		return constants.ClassWizard
	default:
		return ""
	}
}

// convertProtoBackgroundToToolkitID converts proto Background enum to toolkit background ID string
func convertProtoBackgroundToToolkitID(background dnd5ev1alpha1.Background) string {
	switch background {
	case dnd5ev1alpha1.Background_BACKGROUND_ACOLYTE:
		return string(constants.BackgroundAcolyte)
	case dnd5ev1alpha1.Background_BACKGROUND_CHARLATAN:
		return string(constants.BackgroundCharlatan)
	case dnd5ev1alpha1.Background_BACKGROUND_CRIMINAL:
		return string(constants.BackgroundCriminal)
	case dnd5ev1alpha1.Background_BACKGROUND_ENTERTAINER:
		return string(constants.BackgroundEntertainer)
	case dnd5ev1alpha1.Background_BACKGROUND_FOLK_HERO:
		return string(constants.BackgroundFolkHero)
	case dnd5ev1alpha1.Background_BACKGROUND_GUILD_ARTISAN:
		return string(constants.BackgroundGuildArtisan)
	case dnd5ev1alpha1.Background_BACKGROUND_HERMIT:
		return string(constants.BackgroundHermit)
	case dnd5ev1alpha1.Background_BACKGROUND_NOBLE:
		return string(constants.BackgroundNoble)
	case dnd5ev1alpha1.Background_BACKGROUND_OUTLANDER:
		return string(constants.BackgroundOutlander)
	case dnd5ev1alpha1.Background_BACKGROUND_SAGE:
		return string(constants.BackgroundSage)
	case dnd5ev1alpha1.Background_BACKGROUND_SAILOR:
		return string(constants.BackgroundSailor)
	case dnd5ev1alpha1.Background_BACKGROUND_SOLDIER:
		return string(constants.BackgroundSoldier)
	case dnd5ev1alpha1.Background_BACKGROUND_URCHIN:
		return string(constants.BackgroundUrchin)
	default:
		return ""
	}
}

// convertToolkitBackgroundToProtoEnum converts toolkit Background constant to proto Background enum
func convertToolkitBackgroundToProtoEnum(backgroundID constants.Background) dnd5ev1alpha1.Background {
	switch backgroundID {
	case constants.BackgroundAcolyte:
		return dnd5ev1alpha1.Background_BACKGROUND_ACOLYTE
	case constants.BackgroundCharlatan:
		return dnd5ev1alpha1.Background_BACKGROUND_CHARLATAN
	case constants.BackgroundCriminal:
		return dnd5ev1alpha1.Background_BACKGROUND_CRIMINAL
	case constants.BackgroundEntertainer:
		return dnd5ev1alpha1.Background_BACKGROUND_ENTERTAINER
	case constants.BackgroundFolkHero:
		return dnd5ev1alpha1.Background_BACKGROUND_FOLK_HERO
	case constants.BackgroundGuildArtisan:
		return dnd5ev1alpha1.Background_BACKGROUND_GUILD_ARTISAN
	case constants.BackgroundHermit:
		return dnd5ev1alpha1.Background_BACKGROUND_HERMIT
	case constants.BackgroundNoble:
		return dnd5ev1alpha1.Background_BACKGROUND_NOBLE
	case constants.BackgroundOutlander:
		return dnd5ev1alpha1.Background_BACKGROUND_OUTLANDER
	case constants.BackgroundSage:
		return dnd5ev1alpha1.Background_BACKGROUND_SAGE
	case constants.BackgroundSailor:
		return dnd5ev1alpha1.Background_BACKGROUND_SAILOR
	case constants.BackgroundSoldier:
		return dnd5ev1alpha1.Background_BACKGROUND_SOLDIER
	case constants.BackgroundUrchin:
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
		case constants.STR:
			protoScores.Strength = int32(value)
		case constants.DEX:
			protoScores.Dexterity = int32(value)
		case constants.CON:
			protoScores.Constitution = int32(value)
		case constants.INT:
			protoScores.Intelligence = int32(value)
		case constants.WIS:
			protoScores.Wisdom = int32(value)
		case constants.CHA:
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
func convertProtoRaceToToolkit(race dnd5ev1alpha1.Race) constants.Race {
	// Map proto enum to toolkit constants - direct mapping, no strings
	switch race {
	case dnd5ev1alpha1.Race_RACE_DRAGONBORN:
		return constants.RaceDragonborn
	case dnd5ev1alpha1.Race_RACE_DWARF:
		return constants.RaceDwarf
	case dnd5ev1alpha1.Race_RACE_ELF:
		return constants.RaceElf
	case dnd5ev1alpha1.Race_RACE_GNOME:
		return constants.RaceGnome
	case dnd5ev1alpha1.Race_RACE_HALF_ELF:
		return constants.RaceHalfElf
	case dnd5ev1alpha1.Race_RACE_HALFLING:
		return constants.RaceHalfling
	case dnd5ev1alpha1.Race_RACE_HALF_ORC:
		return constants.RaceHalfOrc
	case dnd5ev1alpha1.Race_RACE_HUMAN:
		return constants.RaceHuman
	case dnd5ev1alpha1.Race_RACE_TIEFLING:
		return constants.RaceTiefling
	default:
		return ""
	}
}

// convertProtoSubraceToToolkit converts proto Subrace enum to toolkit Subrace constant
func convertProtoSubraceToToolkit(subrace dnd5ev1alpha1.Subrace) constants.Subrace {
	// Map proto enum to toolkit constants - direct mapping, no strings
	switch subrace {
	case dnd5ev1alpha1.Subrace_SUBRACE_HILL_DWARF:
		return constants.SubraceHillDwarf
	case dnd5ev1alpha1.Subrace_SUBRACE_MOUNTAIN_DWARF:
		return constants.SubraceMountainDwarf
	case dnd5ev1alpha1.Subrace_SUBRACE_HIGH_ELF:
		return constants.SubraceHighElf
	case dnd5ev1alpha1.Subrace_SUBRACE_WOOD_ELF:
		return constants.SubraceWoodElf
	case dnd5ev1alpha1.Subrace_SUBRACE_DARK_ELF:
		return constants.SubraceDarkElf
	case dnd5ev1alpha1.Subrace_SUBRACE_FOREST_GNOME:
		return constants.SubraceForestGnome
	case dnd5ev1alpha1.Subrace_SUBRACE_ROCK_GNOME:
		return constants.SubraceRockGnome
	case dnd5ev1alpha1.Subrace_SUBRACE_LIGHTFOOT_HALFLING:
		return constants.SubraceLightfootHalfling
	case dnd5ev1alpha1.Subrace_SUBRACE_STOUT_HALFLING:
		return constants.SubraceStoutHalfling
	default:
		return ""
	}
}

// convertProtoChoiceSelectionToToolkit converts a single proto ChoiceSelection to toolkit ChoiceData
func convertProtoChoiceSelectionToToolkit(pc *dnd5ev1alpha1.ChoiceSelection) toolkitchar.ChoiceData {
	if pc == nil {
		return toolkitchar.ChoiceData{}
	}

	// Try to infer choice type from choice ID if not specified
	choiceType := pc.GetChoiceType()
	if choiceType == dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_UNSPECIFIED {
		// Infer from choice ID
		switch pc.GetChoiceId() {
		case "language_choice":
			choiceType = dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_LANGUAGES
		case "skill_choice":
			choiceType = dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS
		case "tool_choice":
			choiceType = dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_TOOLS
		}
	}

	choice := toolkitchar.ChoiceData{
		ChoiceID: pc.GetChoiceId(),
		Category: convertProtoCategoryToToolkit(choiceType),
		Source:   convertProtoSourceToToolkit(pc.GetSource()),
	}

	// Convert based on choice type
	switch choiceType {
	case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS:
		// Convert selected keys to skill constants
		skills := make([]constants.Skill, 0, len(pc.GetSelectedKeys()))
		for _, sk := range pc.GetSelectedKeys() {
			skills = append(skills, constants.Skill(sk))
		}
		choice.SkillSelection = skills
	case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_LANGUAGES:
		// Convert selected keys to language constants
		languages := make([]constants.Language, 0, len(pc.GetSelectedKeys()))
		for _, lk := range pc.GetSelectedKeys() {
			languages = append(languages, constants.Language(lk))
		}
		choice.LanguageSelection = languages
	case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_ABILITY_SCORES:
		// Handle ability score choices if present
		if len(pc.GetAbilityScoreChoices()) > 0 {
			scores := make(shared.AbilityScores)
			for _, asc := range pc.GetAbilityScoreChoices() {
				// Convert proto ability to toolkit ability
				ability := convertProtoAbilityToString(asc.GetAbility())
				scores[constants.Ability(ability)] = int(asc.GetBonus())
			}
			choice.AbilityScoreSelection = &scores
		}
	default:
		// For other types, store selected keys as equipment
		choice.EquipmentSelection = pc.GetSelectedKeys()
	}

	return choice
}

// convertProtoRaceChoicesToToolkit converts proto ChoiceSelection to toolkit ChoiceData
func convertProtoRaceChoicesToToolkit(protoChoices []*dnd5ev1alpha1.ChoiceSelection) []toolkitchar.ChoiceData {
	if len(protoChoices) == 0 {
		return nil
	}

	toolkitChoices := make([]toolkitchar.ChoiceData, 0, len(protoChoices))
	for _, pc := range protoChoices {
		toolkitChoices = append(toolkitChoices, convertProtoChoiceSelectionToToolkit(pc))
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
	for _, prof := range raceData.WeaponProficiencies {
		info.Proficiencies = append(info.Proficiencies, prof)
	}
	for _, prof := range raceData.ToolProficiencies {
		info.Proficiencies = append(info.Proficiencies, prof)
	}

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
func convertSizeToProto(size constants.Size) dnd5ev1alpha1.Size {
	switch size {
	case constants.SizeTiny:
		return dnd5ev1alpha1.Size_SIZE_TINY
	case constants.SizeSmall:
		return dnd5ev1alpha1.Size_SIZE_SMALL
	case constants.SizeMedium:
		return dnd5ev1alpha1.Size_SIZE_MEDIUM
	case constants.SizeLarge:
		return dnd5ev1alpha1.Size_SIZE_LARGE
	case constants.SizeHuge:
		return dnd5ev1alpha1.Size_SIZE_HUGE
	case constants.SizeGargantuan:
		return dnd5ev1alpha1.Size_SIZE_GARGANTUAN
	default:
		return dnd5ev1alpha1.Size_SIZE_MEDIUM
	}
}

// convertLanguageToProto converts toolkit Language to proto Language
func convertLanguageToProto(lang constants.Language) dnd5ev1alpha1.Language {
	// Map toolkit language constants to proto enums
	// This is a simplified mapping - you may need to expand based on available languages
	switch lang {
	case constants.LanguageCommon:
		return dnd5ev1alpha1.Language_LANGUAGE_COMMON
	case constants.LanguageDwarvish:
		return dnd5ev1alpha1.Language_LANGUAGE_DWARVISH
	case constants.LanguageElvish:
		return dnd5ev1alpha1.Language_LANGUAGE_ELVISH
	case constants.LanguageGiant:
		return dnd5ev1alpha1.Language_LANGUAGE_GIANT
	case constants.LanguageGnomish:
		return dnd5ev1alpha1.Language_LANGUAGE_GNOMISH
	case constants.LanguageGoblin:
		return dnd5ev1alpha1.Language_LANGUAGE_GOBLIN
	case constants.LanguageHalfling:
		return dnd5ev1alpha1.Language_LANGUAGE_HALFLING
	case constants.LanguageOrc:
		return dnd5ev1alpha1.Language_LANGUAGE_ORC
	case constants.LanguageDraconic:
		return dnd5ev1alpha1.Language_LANGUAGE_DRACONIC
	case constants.LanguageInfernal:
		return dnd5ev1alpha1.Language_LANGUAGE_INFERNAL
	default:
		return dnd5ev1alpha1.Language_LANGUAGE_COMMON
	}
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
func convertAbilityToProto(ability constants.Ability) dnd5ev1alpha1.Ability {
	switch ability {
	case constants.STR:
		return dnd5ev1alpha1.Ability_ABILITY_STRENGTH
	case constants.DEX:
		return dnd5ev1alpha1.Ability_ABILITY_DEXTERITY
	case constants.CON:
		return dnd5ev1alpha1.Ability_ABILITY_CONSTITUTION
	case constants.INT:
		return dnd5ev1alpha1.Ability_ABILITY_INTELLIGENCE
	case constants.WIS:
		return dnd5ev1alpha1.Ability_ABILITY_WISDOM
	case constants.CHA:
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
func convertSkillToProto(skill constants.Skill) dnd5ev1alpha1.Skill {
	// This is a simplified mapping - you'll need to expand based on all skills
	switch skill {
	case constants.SkillAcrobatics:
		return dnd5ev1alpha1.Skill_SKILL_ACROBATICS
	case constants.SkillAnimalHandling:
		return dnd5ev1alpha1.Skill_SKILL_ANIMAL_HANDLING
	case constants.SkillArcana:
		return dnd5ev1alpha1.Skill_SKILL_ARCANA
	case constants.SkillAthletics:
		return dnd5ev1alpha1.Skill_SKILL_ATHLETICS
	case constants.SkillDeception:
		return dnd5ev1alpha1.Skill_SKILL_DECEPTION
	case constants.SkillHistory:
		return dnd5ev1alpha1.Skill_SKILL_HISTORY
	case constants.SkillInsight:
		return dnd5ev1alpha1.Skill_SKILL_INSIGHT
	case constants.SkillIntimidation:
		return dnd5ev1alpha1.Skill_SKILL_INTIMIDATION
	case constants.SkillInvestigation:
		return dnd5ev1alpha1.Skill_SKILL_INVESTIGATION
	case constants.SkillMedicine:
		return dnd5ev1alpha1.Skill_SKILL_MEDICINE
	case constants.SkillNature:
		return dnd5ev1alpha1.Skill_SKILL_NATURE
	case constants.SkillPerception:
		return dnd5ev1alpha1.Skill_SKILL_PERCEPTION
	case constants.SkillPerformance:
		return dnd5ev1alpha1.Skill_SKILL_PERFORMANCE
	case constants.SkillPersuasion:
		return dnd5ev1alpha1.Skill_SKILL_PERSUASION
	case constants.SkillReligion:
		return dnd5ev1alpha1.Skill_SKILL_RELIGION
	case constants.SkillSleightOfHand:
		return dnd5ev1alpha1.Skill_SKILL_SLEIGHT_OF_HAND
	case constants.SkillStealth:
		return dnd5ev1alpha1.Skill_SKILL_STEALTH
	case constants.SkillSurvival:
		return dnd5ev1alpha1.Skill_SKILL_SURVIVAL
	default:
		return dnd5ev1alpha1.Skill_SKILL_ATHLETICS
	}
}
