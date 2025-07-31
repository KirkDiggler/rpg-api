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
	// Convert proto Race enum to string ID
	raceID := convertProtoRaceToString(req.GetRace())
	subraceID := convertProtoSubraceToString(req.GetSubrace())

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
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

// UpdateBackground updates the background of a character draft
func (h *Handler) UpdateBackground(
	ctx context.Context,
	req *dnd5ev1alpha1.UpdateBackgroundRequest,
) (*dnd5ev1alpha1.UpdateBackgroundResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

// UpdateAbilityScores updates the ability scores of a character draft
func (h *Handler) UpdateAbilityScores(
	ctx context.Context,
	req *dnd5ev1alpha1.UpdateAbilityScoresRequest,
) (*dnd5ev1alpha1.UpdateAbilityScoresResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
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
	return nil, status.Error(codes.Unimplemented, "not implemented")
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

	// Convert choices - simplified for now
	// TODO: Implement full choice conversion when we handle updates
	protoDraft.Choices = make([]*dnd5ev1alpha1.ChoiceData, 0)

	// TODO: Convert race, class, background, and ability scores when we implement those updates

	return protoDraft
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

// convertProtoRaceToString converts proto Race enum to toolkit Race ID string
func convertProtoRaceToString(race dnd5ev1alpha1.Race) string {
	// Map proto enum to toolkit constants
	switch race {
	case dnd5ev1alpha1.Race_RACE_DRAGONBORN:
		return "RACE_DRAGONBORN"
	case dnd5ev1alpha1.Race_RACE_DWARF:
		return "RACE_DWARF"
	case dnd5ev1alpha1.Race_RACE_ELF:
		return "RACE_ELF"
	case dnd5ev1alpha1.Race_RACE_GNOME:
		return "RACE_GNOME"
	case dnd5ev1alpha1.Race_RACE_HALF_ELF:
		return "RACE_HALF_ELF"
	case dnd5ev1alpha1.Race_RACE_HALFLING:
		return "RACE_HALFLING"
	case dnd5ev1alpha1.Race_RACE_HALF_ORC:
		return "RACE_HALF_ORC"
	case dnd5ev1alpha1.Race_RACE_HUMAN:
		return "RACE_HUMAN"
	case dnd5ev1alpha1.Race_RACE_TIEFLING:
		return "RACE_TIEFLING"
	default:
		return ""
	}
}

// convertProtoSubraceToString converts proto Subrace enum to toolkit Subrace ID string
func convertProtoSubraceToString(subrace dnd5ev1alpha1.Subrace) string {
	// Map proto enum to toolkit constants
	switch subrace {
	case dnd5ev1alpha1.Subrace_SUBRACE_HILL_DWARF:
		return "SUBRACE_HILL_DWARF"
	case dnd5ev1alpha1.Subrace_SUBRACE_MOUNTAIN_DWARF:
		return "SUBRACE_MOUNTAIN_DWARF"
	case dnd5ev1alpha1.Subrace_SUBRACE_HIGH_ELF:
		return "SUBRACE_HIGH_ELF"
	case dnd5ev1alpha1.Subrace_SUBRACE_WOOD_ELF:
		return "SUBRACE_WOOD_ELF"
	case dnd5ev1alpha1.Subrace_SUBRACE_DARK_ELF:
		return "SUBRACE_DARK_ELF"
	case dnd5ev1alpha1.Subrace_SUBRACE_FOREST_GNOME:
		return "SUBRACE_FOREST_GNOME"
	case dnd5ev1alpha1.Subrace_SUBRACE_ROCK_GNOME:
		return "SUBRACE_ROCK_GNOME"
	case dnd5ev1alpha1.Subrace_SUBRACE_LIGHTFOOT_HALFLING:
		return "SUBRACE_LIGHTFOOT_HALFLING"
	case dnd5ev1alpha1.Subrace_SUBRACE_STOUT_HALFLING:
		return "SUBRACE_STOUT_HALFLING"
	default:
		return ""
	}
}

// convertProtoRaceChoicesToToolkit converts proto ChoiceSelection to toolkit ChoiceData
func convertProtoRaceChoicesToToolkit(protoChoices []*dnd5ev1alpha1.ChoiceSelection) []toolkitchar.ChoiceData {
	if len(protoChoices) == 0 {
		return nil
	}

	toolkitChoices := make([]toolkitchar.ChoiceData, 0, len(protoChoices))
	for _, pc := range protoChoices {
		choice := toolkitchar.ChoiceData{
			ChoiceID: pc.GetChoiceId(),
			Category: convertProtoCategoryToToolkit(pc.GetChoiceType()),
			Source:   convertProtoSourceToToolkit(pc.GetSource()),
		}

		// Convert based on choice type
		switch pc.GetChoiceType() {
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

		toolkitChoices = append(toolkitChoices, choice)
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

	// TODO: Convert starting equipment and choices

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
