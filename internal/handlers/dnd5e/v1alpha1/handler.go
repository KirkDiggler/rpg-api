// Package v1alpha1 handles the grpc service interface
package v1alpha1

import (
	"context"
	"log/slog"
	"strconv"
	"strings"

	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
	"github.com/KirkDiggler/rpg-api/internal/errors"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/clients/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
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
	if req.PlayerId == "" {
		return nil, errors.ToGRPCError(errors.InvalidArgument("player_id is required"))
	}

	input := &character.CreateDraftInput{
		PlayerID:  req.PlayerId,
		SessionID: req.SessionId,
	}

	// Convert initial data if provided
	if req.InitialData != nil {
		input.InitialData = convertProtoDraftDataToEntity(req.InitialData)
	}

	output, err := h.characterService.CreateDraft(ctx, input)
	if err != nil {
		return nil, errors.ToGRPCError(err)
	}

	return &dnd5ev1alpha1.CreateDraftResponse{
		Draft: convertEntityDraftToProto(output.Draft),
	}, nil
}

// GetDraft retrieves a character draft
func (h *Handler) GetDraft(
	ctx context.Context,
	req *dnd5ev1alpha1.GetDraftRequest,
) (*dnd5ev1alpha1.GetDraftResponse, error) {
	if req.DraftId == "" {
		return nil, errors.ToGRPCError(errors.InvalidArgument("draft_id is required"))
	}

	input := &character.GetDraftInput{
		DraftID: req.DraftId,
	}

	output, err := h.characterService.GetDraft(ctx, input)
	if err != nil {
		return nil, errors.ToGRPCError(err)
	}

	return &dnd5ev1alpha1.GetDraftResponse{
		Draft: convertEntityDraftToProto(output.Draft),
	}, nil
}

// ListDrafts lists character drafts
func (h *Handler) ListDrafts(
	ctx context.Context,
	req *dnd5ev1alpha1.ListDraftsRequest,
) (*dnd5ev1alpha1.ListDraftsResponse, error) {
	input := &character.ListDraftsInput{
		PlayerID:  req.PlayerId,
		SessionID: req.SessionId,
		PageSize:  req.PageSize,
		PageToken: req.PageToken,
	}

	// Default page size if not specified
	if input.PageSize == 0 {
		input.PageSize = 20
	}

	output, err := h.characterService.ListDrafts(ctx, input)
	if err != nil {
		return nil, errors.ToGRPCError(err)
	}

	// Convert drafts
	protoDrafts := make([]*dnd5ev1alpha1.CharacterDraft, len(output.Drafts))
	for i, draft := range output.Drafts {
		protoDrafts[i] = convertEntityDraftToProto(draft)
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
	if req.DraftId == "" {
		return nil, errors.ToGRPCError(errors.InvalidArgument("draft_id is required"))
	}

	input := &character.DeleteDraftInput{
		DraftID: req.DraftId,
	}

	output, err := h.characterService.DeleteDraft(ctx, input)
	if err != nil {
		return nil, errors.ToGRPCError(err)
	}

	return &dnd5ev1alpha1.DeleteDraftResponse{
		Message: output.Message,
	}, nil
}

// UpdateName updates the name of a character draft
func (h *Handler) UpdateName(
	ctx context.Context,
	req *dnd5ev1alpha1.UpdateNameRequest,
) (*dnd5ev1alpha1.UpdateNameResponse, error) {
	if req.DraftId == "" {
		return nil, errors.ToGRPCError(errors.InvalidArgument("draft_id is required"))
	}
	if req.Name == "" {
		return nil, errors.ToGRPCError(errors.InvalidArgument("name is required"))
	}

	input := &character.UpdateNameInput{
		DraftID: req.DraftId,
		Name:    req.Name,
	}

	output, err := h.characterService.UpdateName(ctx, input)
	if err != nil {
		return nil, errors.ToGRPCError(err)
	}

	return &dnd5ev1alpha1.UpdateNameResponse{
		Draft:    convertEntityDraftToProto(output.Draft),
		Warnings: convertWarningsToProto(output.Warnings),
	}, nil
}

// UpdateRace updates the race of a character draft
func (h *Handler) UpdateRace(
	ctx context.Context,
	req *dnd5ev1alpha1.UpdateRaceRequest,
) (*dnd5ev1alpha1.UpdateRaceResponse, error) {
	if req.DraftId == "" {
		return nil, errors.ToGRPCError(errors.InvalidArgument("draft_id is required"))
	}
	if req.Race == dnd5ev1alpha1.Race_RACE_UNSPECIFIED {
		return nil, errors.ToGRPCError(errors.InvalidArgument("race is required"))
	}

	// Convert race choices from proto
	choices := make([]dnd5e.ChoiceSelection, 0, len(req.RaceChoices))
	for _, protoChoice := range req.RaceChoices {
		if protoChoice != nil {
			choice := dnd5e.ChoiceSelection{
				ChoiceID:     protoChoice.ChoiceId,
				ChoiceType:   mapProtoChoiceTypeToConstant(protoChoice.ChoiceType),
				Source:       mapProtoChoiceSourceToConstant(protoChoice.Source),
				SelectedKeys: protoChoice.SelectedKeys,
			}

			// Convert ability score choices
			for _, abilityChoice := range protoChoice.AbilityScoreChoices {
				if abilityChoice != nil {
					choice.AbilityScoreChoices = append(choice.AbilityScoreChoices, dnd5e.AbilityScoreChoice{
						Ability: mapProtoAbilityToConstant(abilityChoice.Ability),
						Bonus:   abilityChoice.Bonus,
					})
				}
			}

			choices = append(choices, choice)
		}
	}

	input := &character.UpdateRaceInput{
		DraftID:   req.DraftId,
		RaceID:    mapProtoRaceToConstant(req.Race),
		SubraceID: mapProtoSubraceToConstant(req.Subrace),
		Choices:   choices,
	}

	output, err := h.characterService.UpdateRace(ctx, input)
	if err != nil {
		return nil, errors.ToGRPCError(err)
	}

	return &dnd5ev1alpha1.UpdateRaceResponse{
		Draft:    convertEntityDraftToProto(output.Draft),
		Warnings: convertWarningsToProto(output.Warnings),
	}, nil
}

// UpdateClass updates the class of a character draft
func (h *Handler) UpdateClass(
	ctx context.Context,
	req *dnd5ev1alpha1.UpdateClassRequest,
) (*dnd5ev1alpha1.UpdateClassResponse, error) {
	if req.DraftId == "" {
		return nil, errors.ToGRPCError(errors.InvalidArgument("draft_id is required"))
	}
	if req.Class == dnd5ev1alpha1.Class_CLASS_UNSPECIFIED {
		return nil, errors.ToGRPCError(errors.InvalidArgument("class is required"))
	}

	// Convert class choices from proto
	choices := convertProtoChoicesToEntity(req.ClassChoices)

	input := &character.UpdateClassInput{
		DraftID: req.DraftId,
		ClassID: mapProtoClassToConstant(req.Class),
		Choices: choices,
	}

	output, err := h.characterService.UpdateClass(ctx, input)
	if err != nil {
		return nil, errors.ToGRPCError(err)
	}

	return &dnd5ev1alpha1.UpdateClassResponse{
		Draft:    convertEntityDraftToProto(output.Draft),
		Warnings: convertWarningsToProto(output.Warnings),
	}, nil
}

// UpdateBackground updates the background of a character draft
func (h *Handler) UpdateBackground(
	ctx context.Context,
	req *dnd5ev1alpha1.UpdateBackgroundRequest,
) (*dnd5ev1alpha1.UpdateBackgroundResponse, error) {
	if req.DraftId == "" {
		return nil, errors.ToGRPCError(errors.InvalidArgument("draft_id is required"))
	}
	if req.Background == dnd5ev1alpha1.Background_BACKGROUND_UNSPECIFIED {
		return nil, errors.ToGRPCError(errors.InvalidArgument("background is required"))
	}

	// Convert background choices from proto
	choices := convertProtoChoicesToEntity(req.BackgroundChoices)

	input := &character.UpdateBackgroundInput{
		DraftID:      req.DraftId,
		BackgroundID: mapProtoBackgroundToConstant(req.Background),
		Choices:      choices,
	}

	output, err := h.characterService.UpdateBackground(ctx, input)
	if err != nil {
		return nil, errors.ToGRPCError(err)
	}

	return &dnd5ev1alpha1.UpdateBackgroundResponse{
		Draft:    convertEntityDraftToProto(output.Draft),
		Warnings: convertWarningsToProto(output.Warnings),
	}, nil
}

// UpdateAbilityScores updates the ability scores of a character draft
func (h *Handler) UpdateAbilityScores(
	ctx context.Context,
	req *dnd5ev1alpha1.UpdateAbilityScoresRequest,
) (*dnd5ev1alpha1.UpdateAbilityScoresResponse, error) {
	if req.DraftId == "" {
		return nil, errors.ToGRPCError(errors.InvalidArgument("draft_id is required"))
	}
	// Handle the oneof field for ability scores
	var abilityScores *dnd5e.AbilityScores
	switch scores := req.ScoresInput.(type) {
	case *dnd5ev1alpha1.UpdateAbilityScoresRequest_AbilityScores:
		if scores.AbilityScores != nil {
			abilityScores = &dnd5e.AbilityScores{
				Strength:     scores.AbilityScores.Strength,
				Dexterity:    scores.AbilityScores.Dexterity,
				Constitution: scores.AbilityScores.Constitution,
				Intelligence: scores.AbilityScores.Intelligence,
				Wisdom:       scores.AbilityScores.Wisdom,
				Charisma:     scores.AbilityScores.Charisma,
			}
		}
	case *dnd5ev1alpha1.UpdateAbilityScoresRequest_RollAssignments:
		// TODO: Handle roll assignments - this would require looking up rolls from dice service
		return nil, errors.ToGRPCError(errors.Unimplemented("roll assignments not yet implemented"))
	default:
		return nil, errors.ToGRPCError(errors.InvalidArgument("ability scores or roll assignments required"))
	}

	if abilityScores == nil {
		return nil, errors.ToGRPCError(errors.InvalidArgument("ability scores required"))
	}

	input := &character.UpdateAbilityScoresInput{
		DraftID:       req.DraftId,
		AbilityScores: *abilityScores,
	}

	output, err := h.characterService.UpdateAbilityScores(ctx, input)
	if err != nil {
		return nil, errors.ToGRPCError(err)
	}

	return &dnd5ev1alpha1.UpdateAbilityScoresResponse{
		Draft:    convertEntityDraftToProto(output.Draft),
		Warnings: convertWarningsToProto(output.Warnings),
	}, nil
}

// UpdateSkills updates the skills of a character draft
func (h *Handler) UpdateSkills(
	ctx context.Context,
	req *dnd5ev1alpha1.UpdateSkillsRequest,
) (*dnd5ev1alpha1.UpdateSkillsResponse, error) {
	if req.DraftId == "" {
		return nil, errors.ToGRPCError(errors.InvalidArgument("draft_id is required"))
	}

	// Convert skills from proto to constants
	var skillIDs []string
	for _, skill := range req.Skills {
		if skillID := mapProtoSkillToConstant(skill); skillID != "" {
			skillIDs = append(skillIDs, skillID)
		}
	}

	input := &character.UpdateSkillsInput{
		DraftID:  req.DraftId,
		SkillIDs: skillIDs,
	}

	output, err := h.characterService.UpdateSkills(ctx, input)
	if err != nil {
		return nil, errors.ToGRPCError(err)
	}

	return &dnd5ev1alpha1.UpdateSkillsResponse{
		Draft:    convertEntityDraftToProto(output.Draft),
		Warnings: convertWarningsToProto(output.Warnings),
	}, nil
}

// ValidateDraft validates a character draft
func (h *Handler) ValidateDraft(
	ctx context.Context,
	req *dnd5ev1alpha1.ValidateDraftRequest,
) (*dnd5ev1alpha1.ValidateDraftResponse, error) {
	if req.DraftId == "" {
		return nil, errors.ToGRPCError(errors.InvalidArgument("draft_id is required"))
	}

	input := &character.ValidateDraftInput{
		DraftID: req.DraftId,
	}

	output, err := h.characterService.ValidateDraft(ctx, input)
	if err != nil {
		return nil, errors.ToGRPCError(err)
	}

	return &dnd5ev1alpha1.ValidateDraftResponse{
		IsValid:  output.IsValid,
		Errors:   convertErrorsToProto(output.Errors),
		Warnings: convertWarningsToProto(output.Warnings),
	}, nil
}

// FinalizeDraft finalizes a character draft into a complete character
func (h *Handler) FinalizeDraft(
	ctx context.Context,
	req *dnd5ev1alpha1.FinalizeDraftRequest,
) (*dnd5ev1alpha1.FinalizeDraftResponse, error) {
	if req.DraftId == "" {
		return nil, errors.ToGRPCError(errors.InvalidArgument("draft_id is required"))
	}

	input := &character.FinalizeDraftInput{
		DraftID: req.DraftId,
	}

	output, err := h.characterService.FinalizeDraft(ctx, input)
	if err != nil {
		return nil, errors.ToGRPCError(err)
	}

	return &dnd5ev1alpha1.FinalizeDraftResponse{
		Character:    convertCharacterToProto(output.Character),
		DraftDeleted: output.DraftDeleted,
	}, nil
}

// GetCharacter retrieves a character
func (h *Handler) GetCharacter(
	ctx context.Context,
	req *dnd5ev1alpha1.GetCharacterRequest,
) (*dnd5ev1alpha1.GetCharacterResponse, error) {
	if req.CharacterId == "" {
		return nil, errors.ToGRPCError(errors.InvalidArgument("character_id is required"))
	}

	input := &character.GetCharacterInput{
		CharacterID: req.CharacterId,
	}

	output, err := h.characterService.GetCharacter(ctx, input)
	if err != nil {
		return nil, errors.ToGRPCError(err)
	}

	return &dnd5ev1alpha1.GetCharacterResponse{
		Character: convertCharacterToProto(output.Character),
	}, nil
}

// ListCharacters lists characters
func (h *Handler) ListCharacters(
	ctx context.Context,
	req *dnd5ev1alpha1.ListCharactersRequest,
) (*dnd5ev1alpha1.ListCharactersResponse, error) {
	slog.InfoContext(ctx, "ListCharacters request received",
		"player_id", req.PlayerId,
		"session_id", req.SessionId,
		"page_size", req.PageSize,
		"page_token", req.PageToken)

	input := &character.ListCharactersInput{
		PlayerID:  req.PlayerId,
		SessionID: req.SessionId,
		PageSize:  req.PageSize,
		PageToken: req.PageToken,
	}

	// Default page size if not specified
	if input.PageSize == 0 {
		input.PageSize = 20
	}

	if h.characterService == nil {
		slog.ErrorContext(ctx, "Character service is nil")
		return nil, errors.ToGRPCError(errors.Internal("character service not initialized"))
	}

	slog.InfoContext(ctx, "Calling character service ListCharacters",
		"input", input)

	output, err := h.characterService.ListCharacters(ctx, input)
	if err != nil {
		slog.ErrorContext(ctx, "ListCharacters failed",
			"error", err,
			"player_id", req.PlayerId,
			"session_id", req.SessionId)
		return nil, errors.ToGRPCError(err)
	}

	slog.InfoContext(ctx, "ListCharacters succeeded",
		"character_count", len(output.Characters))

	// Convert characters to proto
	protoCharacters := make([]*dnd5ev1alpha1.Character, len(output.Characters))
	for i, char := range output.Characters {
		protoCharacters[i] = convertCharacterToProto(char)
	}

	return &dnd5ev1alpha1.ListCharactersResponse{
		Characters:    protoCharacters,
		NextPageToken: output.NextPageToken,
		TotalSize:     output.TotalSize,
	}, nil
}

// DeleteCharacter deletes a character
func (h *Handler) DeleteCharacter(
	ctx context.Context,
	req *dnd5ev1alpha1.DeleteCharacterRequest,
) (*dnd5ev1alpha1.DeleteCharacterResponse, error) {
	if req.CharacterId == "" {
		return nil, errors.ToGRPCError(errors.InvalidArgument("character_id is required"))
	}

	input := &character.DeleteCharacterInput{
		CharacterID: req.CharacterId,
	}

	_, err := h.characterService.DeleteCharacter(ctx, input)
	if err != nil {
		return nil, errors.ToGRPCError(err)
	}

	return &dnd5ev1alpha1.DeleteCharacterResponse{
		Message: "Character deleted successfully",
	}, nil
}

// RollAbilityScores rolls ability scores for character creation
func (h *Handler) RollAbilityScores(
	ctx context.Context,
	req *dnd5ev1alpha1.RollAbilityScoresRequest,
) (*dnd5ev1alpha1.RollAbilityScoresResponse, error) {
	if req.DraftId == "" {
		return nil, errors.ToGRPCError(errors.InvalidArgument("draft_id is required"))
	}

	input := &character.RollAbilityScoresInput{
		DraftID: req.DraftId,
		Method:  "", // Method field doesn't exist in proto, use default
	}

	output, err := h.characterService.RollAbilityScores(ctx, input)
	if err != nil {
		return nil, errors.ToGRPCError(err)
	}

	// Convert rolls to proto format
	protoRolls := make([]*dnd5ev1alpha1.AbilityScoreRoll, len(output.Rolls))
	for i, roll := range output.Rolls {
		// Get the lowest dropped die value (proto expects single int32, not array)
		var droppedValue int32
		if len(roll.Dropped) > 0 {
			droppedValue = roll.Dropped[0] // First (and usually only) dropped die
		}
		protoRolls[i] = &dnd5ev1alpha1.AbilityScoreRoll{
			RollId:   roll.ID,
			Total:    roll.Value,
			Dice:     roll.Dice,
			Dropped:  droppedValue,
			Notation: roll.Notation,
		}
	}

	return &dnd5ev1alpha1.RollAbilityScoresResponse{
		Rolls:     protoRolls,
		ExpiresAt: output.ExpiresAt,
	}, nil
}

// Converter functions

// convertProtoDraftDataToEntity converts CharacterDraftData (input proto) to entity
func convertProtoDraftDataToEntity(proto *dnd5ev1alpha1.CharacterDraftData) *dnd5e.CharacterDraft {
	if proto == nil {
		return nil
	}

	draft := &dnd5e.CharacterDraft{
		ID:        proto.Id,
		PlayerID:  proto.PlayerId,
		SessionID: proto.SessionId,
		Name:      proto.Name,
	}

	// Convert race using our constants
	if proto.Race != dnd5ev1alpha1.Race_RACE_UNSPECIFIED {
		draft.RaceID = mapProtoRaceToConstant(proto.Race)
	}

	// Convert subrace
	if proto.Subrace != dnd5ev1alpha1.Subrace_SUBRACE_UNSPECIFIED {
		draft.SubraceID = mapProtoSubraceToConstant(proto.Subrace)
	}

	// Convert class
	if proto.Class != dnd5ev1alpha1.Class_CLASS_UNSPECIFIED {
		draft.ClassID = mapProtoClassToConstant(proto.Class)
	}

	// Convert background
	if proto.Background != dnd5ev1alpha1.Background_BACKGROUND_UNSPECIFIED {
		draft.BackgroundID = mapProtoBackgroundToConstant(proto.Background)
	}

	// Convert alignment
	if proto.Alignment != dnd5ev1alpha1.Alignment_ALIGNMENT_UNSPECIFIED {
		draft.Alignment = mapProtoAlignmentToConstant(proto.Alignment)
	}

	if proto.AbilityScores != nil {
		draft.AbilityScores = &dnd5e.AbilityScores{
			Strength:     proto.AbilityScores.Strength,
			Dexterity:    proto.AbilityScores.Dexterity,
			Constitution: proto.AbilityScores.Constitution,
			Intelligence: proto.AbilityScores.Intelligence,
			Wisdom:       proto.AbilityScores.Wisdom,
			Charisma:     proto.AbilityScores.Charisma,
		}
	}

	// Convert choices
	for _, protoChoice := range proto.Choices {
		if protoChoice != nil {
			choice := dnd5e.ChoiceSelection{
				ChoiceID:     protoChoice.ChoiceId,
				ChoiceType:   mapProtoChoiceTypeToConstant(protoChoice.ChoiceType),
				Source:       mapProtoChoiceSourceToConstant(protoChoice.Source),
				SelectedKeys: protoChoice.SelectedKeys,
			}

			// Convert ability score choices
			for _, abilityChoice := range protoChoice.AbilityScoreChoices {
				if abilityChoice != nil {
					choice.AbilityScoreChoices = append(choice.AbilityScoreChoices, dnd5e.AbilityScoreChoice{
						Ability: mapProtoAbilityToConstant(abilityChoice.Ability),
						Bonus:   abilityChoice.Bonus,
					})
				}
			}

			draft.ChoiceSelections = append(draft.ChoiceSelections, choice)
		}
	}

	if proto.Progress != nil {
		draft.Progress = dnd5e.CreationProgress{
			StepsCompleted:       0,
			CompletionPercentage: proto.Progress.CompletionPercentage,
			CurrentStep:          mapProtoCreationStepToConstant(proto.Progress.CurrentStep),
		}
		// Convert individual boolean flags to bitflags
		if proto.Progress.HasName {
			draft.Progress.SetStep(dnd5e.ProgressStepName, true)
		}
		if proto.Progress.HasRace {
			draft.Progress.SetStep(dnd5e.ProgressStepRace, true)
		}
		if proto.Progress.HasClass {
			draft.Progress.SetStep(dnd5e.ProgressStepClass, true)
		}
		if proto.Progress.HasBackground {
			draft.Progress.SetStep(dnd5e.ProgressStepBackground, true)
		}
		if proto.Progress.HasAbilityScores {
			draft.Progress.SetStep(dnd5e.ProgressStepAbilityScores, true)
		}
		if proto.Progress.HasSkills {
			draft.Progress.SetStep(dnd5e.ProgressStepSkills, true)
		}
		if proto.Progress.HasLanguages {
			draft.Progress.SetStep(dnd5e.ProgressStepLanguages, true)
		}
	}

	if proto.Metadata != nil {
		draft.CreatedAt = proto.Metadata.CreatedAt
		draft.UpdatedAt = proto.Metadata.UpdatedAt
		// Discord fields are not needed in the API service
	}

	draft.ExpiresAt = proto.ExpiresAt

	return draft
}

func convertEntityDraftToProto(entity *dnd5e.CharacterDraft) *dnd5ev1alpha1.CharacterDraft {
	if entity == nil {
		return nil
	}

	proto := &dnd5ev1alpha1.CharacterDraft{
		Id:        entity.ID,
		PlayerId:  entity.PlayerID,
		SessionId: entity.SessionID,
		Name:      entity.Name,
		Progress: &dnd5ev1alpha1.CreationProgress{
			HasName:              entity.Progress.HasName(),
			HasRace:              entity.Progress.HasRace(),
			HasClass:             entity.Progress.HasClass(),
			HasBackground:        entity.Progress.HasBackground(),
			HasAbilityScores:     entity.Progress.HasAbilityScores(),
			HasSkills:            entity.Progress.HasSkills(),
			HasLanguages:         entity.Progress.HasLanguages(),
			CompletionPercentage: entity.Progress.CompletionPercentage,
			CurrentStep:          mapConstantToProtoCreationStep(entity.Progress.CurrentStep),
		},
		ExpiresAt: entity.ExpiresAt,
		Metadata: &dnd5ev1alpha1.DraftMetadata{
			CreatedAt:        entity.CreatedAt,
			UpdatedAt:        entity.UpdatedAt,
			DiscordChannelId: "", // Discord fields are not used in the API service
			DiscordMessageId: "", // Discord fields are not used in the API service
		},
	}

	// Convert race info if populated by orchestrator
	if entity.Race != nil {
		proto.Race = convertRaceInfoToProto(entity.Race)
	}
	if entity.Subrace != nil {
		proto.Subrace = convertSubraceInfoToProto(entity.Subrace)
	}

	// Convert class info if populated by orchestrator
	if entity.Class != nil {
		proto.Class = convertClassInfoToProto(entity.Class)
	}

	// Convert background info if populated by orchestrator
	if entity.Background != nil {
		proto.Background = convertBackgroundInfoToProto(entity.Background)
	}

	// Convert alignment
	if entity.Alignment != "" {
		proto.Alignment = mapConstantToProtoAlignment(entity.Alignment)
	}

	// Convert ability scores
	if entity.AbilityScores != nil {
		proto.AbilityScores = &dnd5ev1alpha1.AbilityScores{
			Strength:     entity.AbilityScores.Strength,
			Dexterity:    entity.AbilityScores.Dexterity,
			Constitution: entity.AbilityScores.Constitution,
			Intelligence: entity.AbilityScores.Intelligence,
			Wisdom:       entity.AbilityScores.Wisdom,
			Charisma:     entity.AbilityScores.Charisma,
		}
	}

	// Note: StartingSkills and AdditionalLanguages are no longer in the response proto
	// All skills and languages are now tracked through the choices system

	// Convert choices
	for _, entityChoice := range entity.ChoiceSelections {
		protoChoice := &dnd5ev1alpha1.ChoiceSelection{
			ChoiceId:     entityChoice.ChoiceID,
			ChoiceType:   mapConstantToProtoChoiceType(entityChoice.ChoiceType),
			Source:       mapConstantToProtoChoiceSource(entityChoice.Source),
			SelectedKeys: entityChoice.SelectedKeys,
		}

		// Convert ability score choices
		for _, abilityChoice := range entityChoice.AbilityScoreChoices {
			protoChoice.AbilityScoreChoices = append(protoChoice.AbilityScoreChoices, &dnd5ev1alpha1.AbilityScoreChoice{
				Ability: mapConstantToProtoAbility(abilityChoice.Ability),
				Bonus:   abilityChoice.Bonus,
			})
		}

		proto.Choices = append(proto.Choices, protoChoice)
	}

	return proto
}

// Mapper functions - Proto to Constants

func mapProtoRaceToConstant(race dnd5ev1alpha1.Race) string {
	switch race {
	case dnd5ev1alpha1.Race_RACE_HUMAN:
		return dnd5e.RaceHuman
	case dnd5ev1alpha1.Race_RACE_DWARF:
		return dnd5e.RaceDwarf
	case dnd5ev1alpha1.Race_RACE_ELF:
		return dnd5e.RaceElf
	case dnd5ev1alpha1.Race_RACE_HALFLING:
		return dnd5e.RaceHalfling
	case dnd5ev1alpha1.Race_RACE_DRAGONBORN:
		return dnd5e.RaceDragonborn
	case dnd5ev1alpha1.Race_RACE_GNOME:
		return dnd5e.RaceGnome
	case dnd5ev1alpha1.Race_RACE_HALF_ELF:
		return dnd5e.RaceHalfElf
	case dnd5ev1alpha1.Race_RACE_HALF_ORC:
		return dnd5e.RaceHalfOrc
	case dnd5ev1alpha1.Race_RACE_TIEFLING:
		return dnd5e.RaceTiefling
	default:
		return ""
	}
}

func mapProtoSubraceToConstant(subrace dnd5ev1alpha1.Subrace) string {
	switch subrace {
	case dnd5ev1alpha1.Subrace_SUBRACE_HIGH_ELF:
		return dnd5e.SubraceHighElf
	case dnd5ev1alpha1.Subrace_SUBRACE_WOOD_ELF:
		return dnd5e.SubraceWoodElf
	case dnd5ev1alpha1.Subrace_SUBRACE_DARK_ELF:
		return dnd5e.SubraceDarkElf
	case dnd5ev1alpha1.Subrace_SUBRACE_HILL_DWARF:
		return dnd5e.SubraceHillDwarf
	case dnd5ev1alpha1.Subrace_SUBRACE_MOUNTAIN_DWARF:
		return dnd5e.SubraceMountainDwarf
	case dnd5ev1alpha1.Subrace_SUBRACE_LIGHTFOOT_HALFLING:
		return dnd5e.SubraceLightfootHalfling
	case dnd5ev1alpha1.Subrace_SUBRACE_STOUT_HALFLING:
		return dnd5e.SubraceStoutHalfling
	case dnd5ev1alpha1.Subrace_SUBRACE_FOREST_GNOME:
		return dnd5e.SubraceForestGnome
	case dnd5ev1alpha1.Subrace_SUBRACE_ROCK_GNOME:
		return dnd5e.SubraceRockGnome
	default:
		return ""
	}
}

func mapProtoClassToConstant(class dnd5ev1alpha1.Class) string {
	switch class {
	case dnd5ev1alpha1.Class_CLASS_BARBARIAN:
		return dnd5e.ClassBarbarian
	case dnd5ev1alpha1.Class_CLASS_BARD:
		return dnd5e.ClassBard
	case dnd5ev1alpha1.Class_CLASS_CLERIC:
		return dnd5e.ClassCleric
	case dnd5ev1alpha1.Class_CLASS_DRUID:
		return dnd5e.ClassDruid
	case dnd5ev1alpha1.Class_CLASS_FIGHTER:
		return dnd5e.ClassFighter
	case dnd5ev1alpha1.Class_CLASS_MONK:
		return dnd5e.ClassMonk
	case dnd5ev1alpha1.Class_CLASS_PALADIN:
		return dnd5e.ClassPaladin
	case dnd5ev1alpha1.Class_CLASS_RANGER:
		return dnd5e.ClassRanger
	case dnd5ev1alpha1.Class_CLASS_ROGUE:
		return dnd5e.ClassRogue
	case dnd5ev1alpha1.Class_CLASS_SORCERER:
		return dnd5e.ClassSorcerer
	case dnd5ev1alpha1.Class_CLASS_WARLOCK:
		return dnd5e.ClassWarlock
	case dnd5ev1alpha1.Class_CLASS_WIZARD:
		return dnd5e.ClassWizard
	default:
		return ""
	}
}

func mapProtoBackgroundToConstant(bg dnd5ev1alpha1.Background) string {
	switch bg {
	case dnd5ev1alpha1.Background_BACKGROUND_ACOLYTE:
		return dnd5e.BackgroundAcolyte
	case dnd5ev1alpha1.Background_BACKGROUND_CHARLATAN:
		return dnd5e.BackgroundCharlatan
	case dnd5ev1alpha1.Background_BACKGROUND_CRIMINAL:
		return dnd5e.BackgroundCriminal
	case dnd5ev1alpha1.Background_BACKGROUND_ENTERTAINER:
		return dnd5e.BackgroundEntertainer
	case dnd5ev1alpha1.Background_BACKGROUND_FOLK_HERO:
		return dnd5e.BackgroundFolkHero
	case dnd5ev1alpha1.Background_BACKGROUND_GUILD_ARTISAN:
		return dnd5e.BackgroundGuildArtisan
	case dnd5ev1alpha1.Background_BACKGROUND_HERMIT:
		return dnd5e.BackgroundHermit
	case dnd5ev1alpha1.Background_BACKGROUND_NOBLE:
		return dnd5e.BackgroundNoble
	case dnd5ev1alpha1.Background_BACKGROUND_OUTLANDER:
		return dnd5e.BackgroundOutlander
	case dnd5ev1alpha1.Background_BACKGROUND_SAGE:
		return dnd5e.BackgroundSage
	case dnd5ev1alpha1.Background_BACKGROUND_SAILOR:
		return dnd5e.BackgroundSailor
	case dnd5ev1alpha1.Background_BACKGROUND_SOLDIER:
		return dnd5e.BackgroundSoldier
	case dnd5ev1alpha1.Background_BACKGROUND_URCHIN:
		return dnd5e.BackgroundUrchin
	default:
		return ""
	}
}

func mapProtoAlignmentToConstant(alignment dnd5ev1alpha1.Alignment) string {
	switch alignment {
	case dnd5ev1alpha1.Alignment_ALIGNMENT_LAWFUL_GOOD:
		return dnd5e.AlignmentLawfulGood
	case dnd5ev1alpha1.Alignment_ALIGNMENT_NEUTRAL_GOOD:
		return dnd5e.AlignmentNeutralGood
	case dnd5ev1alpha1.Alignment_ALIGNMENT_CHAOTIC_GOOD:
		return dnd5e.AlignmentChaoticGood
	case dnd5ev1alpha1.Alignment_ALIGNMENT_LAWFUL_NEUTRAL:
		return dnd5e.AlignmentLawfulNeutral
	case dnd5ev1alpha1.Alignment_ALIGNMENT_TRUE_NEUTRAL:
		return dnd5e.AlignmentTrueNeutral
	case dnd5ev1alpha1.Alignment_ALIGNMENT_CHAOTIC_NEUTRAL:
		return dnd5e.AlignmentChaoticNeutral
	case dnd5ev1alpha1.Alignment_ALIGNMENT_LAWFUL_EVIL:
		return dnd5e.AlignmentLawfulEvil
	case dnd5ev1alpha1.Alignment_ALIGNMENT_NEUTRAL_EVIL:
		return dnd5e.AlignmentNeutralEvil
	case dnd5ev1alpha1.Alignment_ALIGNMENT_CHAOTIC_EVIL:
		return dnd5e.AlignmentChaoticEvil
	default:
		return ""
	}
}

func mapProtoSkillToConstant(skill dnd5ev1alpha1.Skill) string {
	switch skill {
	case dnd5ev1alpha1.Skill_SKILL_ACROBATICS:
		return dnd5e.SkillAcrobatics
	case dnd5ev1alpha1.Skill_SKILL_ANIMAL_HANDLING:
		return dnd5e.SkillAnimalHandling
	case dnd5ev1alpha1.Skill_SKILL_ARCANA:
		return dnd5e.SkillArcana
	case dnd5ev1alpha1.Skill_SKILL_ATHLETICS:
		return dnd5e.SkillAthletics
	case dnd5ev1alpha1.Skill_SKILL_DECEPTION:
		return dnd5e.SkillDeception
	case dnd5ev1alpha1.Skill_SKILL_HISTORY:
		return dnd5e.SkillHistory
	case dnd5ev1alpha1.Skill_SKILL_INSIGHT:
		return dnd5e.SkillInsight
	case dnd5ev1alpha1.Skill_SKILL_INTIMIDATION:
		return dnd5e.SkillIntimidation
	case dnd5ev1alpha1.Skill_SKILL_INVESTIGATION:
		return dnd5e.SkillInvestigation
	case dnd5ev1alpha1.Skill_SKILL_MEDICINE:
		return dnd5e.SkillMedicine
	case dnd5ev1alpha1.Skill_SKILL_NATURE:
		return dnd5e.SkillNature
	case dnd5ev1alpha1.Skill_SKILL_PERCEPTION:
		return dnd5e.SkillPerception
	case dnd5ev1alpha1.Skill_SKILL_PERFORMANCE:
		return dnd5e.SkillPerformance
	case dnd5ev1alpha1.Skill_SKILL_PERSUASION:
		return dnd5e.SkillPersuasion
	case dnd5ev1alpha1.Skill_SKILL_RELIGION:
		return dnd5e.SkillReligion
	case dnd5ev1alpha1.Skill_SKILL_SLEIGHT_OF_HAND:
		return dnd5e.SkillSleightOfHand
	case dnd5ev1alpha1.Skill_SKILL_STEALTH:
		return dnd5e.SkillStealth
	case dnd5ev1alpha1.Skill_SKILL_SURVIVAL:
		return dnd5e.SkillSurvival
	default:
		return ""
	}
}

func mapProtoCreationStepToConstant(step dnd5ev1alpha1.CreationStep) string {
	switch step {
	case dnd5ev1alpha1.CreationStep_CREATION_STEP_NAME:
		return dnd5e.CreationStepName
	case dnd5ev1alpha1.CreationStep_CREATION_STEP_RACE:
		return dnd5e.CreationStepRace
	case dnd5ev1alpha1.CreationStep_CREATION_STEP_CLASS:
		return dnd5e.CreationStepClass
	case dnd5ev1alpha1.CreationStep_CREATION_STEP_BACKGROUND:
		return dnd5e.CreationStepBackground
	case dnd5ev1alpha1.CreationStep_CREATION_STEP_ABILITY_SCORES:
		return dnd5e.CreationStepAbilityScores
	case dnd5ev1alpha1.CreationStep_CREATION_STEP_SKILLS:
		return dnd5e.CreationStepSkills
	case dnd5ev1alpha1.CreationStep_CREATION_STEP_LANGUAGES:
		return dnd5e.CreationStepLanguages
	case dnd5ev1alpha1.CreationStep_CREATION_STEP_REVIEW:
		return dnd5e.CreationStepReview
	default:
		return ""
	}
}

// Mapper functions - Constants to Proto

func mapConstantToProtoRace(constant string) dnd5ev1alpha1.Race {
	switch constant {
	case dnd5e.RaceHuman:
		return dnd5ev1alpha1.Race_RACE_HUMAN
	case dnd5e.RaceDwarf:
		return dnd5ev1alpha1.Race_RACE_DWARF
	case dnd5e.RaceElf:
		return dnd5ev1alpha1.Race_RACE_ELF
	case dnd5e.RaceHalfling:
		return dnd5ev1alpha1.Race_RACE_HALFLING
	case dnd5e.RaceDragonborn:
		return dnd5ev1alpha1.Race_RACE_DRAGONBORN
	case dnd5e.RaceGnome:
		return dnd5ev1alpha1.Race_RACE_GNOME
	case dnd5e.RaceHalfElf:
		return dnd5ev1alpha1.Race_RACE_HALF_ELF
	case dnd5e.RaceHalfOrc:
		return dnd5ev1alpha1.Race_RACE_HALF_ORC
	case dnd5e.RaceTiefling:
		return dnd5ev1alpha1.Race_RACE_TIEFLING
	default:
		return dnd5ev1alpha1.Race_RACE_UNSPECIFIED
	}
}

func mapConstantToProtoSubrace(constant string) dnd5ev1alpha1.Subrace {
	switch constant {
	case dnd5e.SubraceHighElf:
		return dnd5ev1alpha1.Subrace_SUBRACE_HIGH_ELF
	case dnd5e.SubraceWoodElf:
		return dnd5ev1alpha1.Subrace_SUBRACE_WOOD_ELF
	case dnd5e.SubraceDarkElf:
		return dnd5ev1alpha1.Subrace_SUBRACE_DARK_ELF
	case dnd5e.SubraceHillDwarf:
		return dnd5ev1alpha1.Subrace_SUBRACE_HILL_DWARF
	case dnd5e.SubraceMountainDwarf:
		return dnd5ev1alpha1.Subrace_SUBRACE_MOUNTAIN_DWARF
	case dnd5e.SubraceLightfootHalfling:
		return dnd5ev1alpha1.Subrace_SUBRACE_LIGHTFOOT_HALFLING
	case dnd5e.SubraceStoutHalfling:
		return dnd5ev1alpha1.Subrace_SUBRACE_STOUT_HALFLING
	case dnd5e.SubraceForestGnome:
		return dnd5ev1alpha1.Subrace_SUBRACE_FOREST_GNOME
	case dnd5e.SubraceRockGnome:
		return dnd5ev1alpha1.Subrace_SUBRACE_ROCK_GNOME
	default:
		return dnd5ev1alpha1.Subrace_SUBRACE_UNSPECIFIED
	}
}

func mapConstantToProtoClass(constant string) dnd5ev1alpha1.Class {
	switch constant {
	case dnd5e.ClassBarbarian:
		return dnd5ev1alpha1.Class_CLASS_BARBARIAN
	case dnd5e.ClassBard:
		return dnd5ev1alpha1.Class_CLASS_BARD
	case dnd5e.ClassCleric:
		return dnd5ev1alpha1.Class_CLASS_CLERIC
	case dnd5e.ClassDruid:
		return dnd5ev1alpha1.Class_CLASS_DRUID
	case dnd5e.ClassFighter:
		return dnd5ev1alpha1.Class_CLASS_FIGHTER
	case dnd5e.ClassMonk:
		return dnd5ev1alpha1.Class_CLASS_MONK
	case dnd5e.ClassPaladin:
		return dnd5ev1alpha1.Class_CLASS_PALADIN
	case dnd5e.ClassRanger:
		return dnd5ev1alpha1.Class_CLASS_RANGER
	case dnd5e.ClassRogue:
		return dnd5ev1alpha1.Class_CLASS_ROGUE
	case dnd5e.ClassSorcerer:
		return dnd5ev1alpha1.Class_CLASS_SORCERER
	case dnd5e.ClassWarlock:
		return dnd5ev1alpha1.Class_CLASS_WARLOCK
	case dnd5e.ClassWizard:
		return dnd5ev1alpha1.Class_CLASS_WIZARD
	default:
		return dnd5ev1alpha1.Class_CLASS_UNSPECIFIED
	}
}

func mapConstantToProtoBackground(constant string) dnd5ev1alpha1.Background {
	switch constant {
	case dnd5e.BackgroundAcolyte:
		return dnd5ev1alpha1.Background_BACKGROUND_ACOLYTE
	case dnd5e.BackgroundCharlatan:
		return dnd5ev1alpha1.Background_BACKGROUND_CHARLATAN
	case dnd5e.BackgroundCriminal:
		return dnd5ev1alpha1.Background_BACKGROUND_CRIMINAL
	case dnd5e.BackgroundEntertainer:
		return dnd5ev1alpha1.Background_BACKGROUND_ENTERTAINER
	case dnd5e.BackgroundFolkHero:
		return dnd5ev1alpha1.Background_BACKGROUND_FOLK_HERO
	case dnd5e.BackgroundGuildArtisan:
		return dnd5ev1alpha1.Background_BACKGROUND_GUILD_ARTISAN
	case dnd5e.BackgroundHermit:
		return dnd5ev1alpha1.Background_BACKGROUND_HERMIT
	case dnd5e.BackgroundNoble:
		return dnd5ev1alpha1.Background_BACKGROUND_NOBLE
	case dnd5e.BackgroundOutlander:
		return dnd5ev1alpha1.Background_BACKGROUND_OUTLANDER
	case dnd5e.BackgroundSage:
		return dnd5ev1alpha1.Background_BACKGROUND_SAGE
	case dnd5e.BackgroundSailor:
		return dnd5ev1alpha1.Background_BACKGROUND_SAILOR
	case dnd5e.BackgroundSoldier:
		return dnd5ev1alpha1.Background_BACKGROUND_SOLDIER
	case dnd5e.BackgroundUrchin:
		return dnd5ev1alpha1.Background_BACKGROUND_URCHIN
	default:
		return dnd5ev1alpha1.Background_BACKGROUND_UNSPECIFIED
	}
}

func mapConstantToProtoAlignment(constant string) dnd5ev1alpha1.Alignment {
	switch constant {
	case dnd5e.AlignmentLawfulGood:
		return dnd5ev1alpha1.Alignment_ALIGNMENT_LAWFUL_GOOD
	case dnd5e.AlignmentNeutralGood:
		return dnd5ev1alpha1.Alignment_ALIGNMENT_NEUTRAL_GOOD
	case dnd5e.AlignmentChaoticGood:
		return dnd5ev1alpha1.Alignment_ALIGNMENT_CHAOTIC_GOOD
	case dnd5e.AlignmentLawfulNeutral:
		return dnd5ev1alpha1.Alignment_ALIGNMENT_LAWFUL_NEUTRAL
	case dnd5e.AlignmentTrueNeutral:
		return dnd5ev1alpha1.Alignment_ALIGNMENT_TRUE_NEUTRAL
	case dnd5e.AlignmentChaoticNeutral:
		return dnd5ev1alpha1.Alignment_ALIGNMENT_CHAOTIC_NEUTRAL
	case dnd5e.AlignmentLawfulEvil:
		return dnd5ev1alpha1.Alignment_ALIGNMENT_LAWFUL_EVIL
	case dnd5e.AlignmentNeutralEvil:
		return dnd5ev1alpha1.Alignment_ALIGNMENT_NEUTRAL_EVIL
	case dnd5e.AlignmentChaoticEvil:
		return dnd5ev1alpha1.Alignment_ALIGNMENT_CHAOTIC_EVIL
	default:
		return dnd5ev1alpha1.Alignment_ALIGNMENT_UNSPECIFIED
	}
}

func mapConstantToProtoLanguage(constant string) dnd5ev1alpha1.Language {
	switch constant {
	case dnd5e.LanguageCommon:
		return dnd5ev1alpha1.Language_LANGUAGE_COMMON
	case dnd5e.LanguageDwarvish:
		return dnd5ev1alpha1.Language_LANGUAGE_DWARVISH
	case dnd5e.LanguageElvish:
		return dnd5ev1alpha1.Language_LANGUAGE_ELVISH
	case dnd5e.LanguageGiant:
		return dnd5ev1alpha1.Language_LANGUAGE_GIANT
	case dnd5e.LanguageGnomish:
		return dnd5ev1alpha1.Language_LANGUAGE_GNOMISH
	case dnd5e.LanguageGoblin:
		return dnd5ev1alpha1.Language_LANGUAGE_GOBLIN
	case dnd5e.LanguageHalfling:
		return dnd5ev1alpha1.Language_LANGUAGE_HALFLING
	case dnd5e.LanguageOrc:
		return dnd5ev1alpha1.Language_LANGUAGE_ORC
	case dnd5e.LanguageAbyssal:
		return dnd5ev1alpha1.Language_LANGUAGE_ABYSSAL
	case dnd5e.LanguageCelestial:
		return dnd5ev1alpha1.Language_LANGUAGE_CELESTIAL
	case dnd5e.LanguageDraconic:
		return dnd5ev1alpha1.Language_LANGUAGE_DRACONIC
	case dnd5e.LanguageDeepSpeech:
		return dnd5ev1alpha1.Language_LANGUAGE_DEEP_SPEECH
	case dnd5e.LanguageInfernal:
		return dnd5ev1alpha1.Language_LANGUAGE_INFERNAL
	case dnd5e.LanguagePrimordial:
		return dnd5ev1alpha1.Language_LANGUAGE_PRIMORDIAL
	case dnd5e.LanguageSylvan:
		return dnd5ev1alpha1.Language_LANGUAGE_SYLVAN
	case dnd5e.LanguageUndercommon:
		return dnd5ev1alpha1.Language_LANGUAGE_UNDERCOMMON
	default:
		return dnd5ev1alpha1.Language_LANGUAGE_UNSPECIFIED
	}
}

func mapConstantToProtoCreationStep(constant string) dnd5ev1alpha1.CreationStep {
	switch constant {
	case dnd5e.CreationStepName:
		return dnd5ev1alpha1.CreationStep_CREATION_STEP_NAME
	case dnd5e.CreationStepRace:
		return dnd5ev1alpha1.CreationStep_CREATION_STEP_RACE
	case dnd5e.CreationStepClass:
		return dnd5ev1alpha1.CreationStep_CREATION_STEP_CLASS
	case dnd5e.CreationStepBackground:
		return dnd5ev1alpha1.CreationStep_CREATION_STEP_BACKGROUND
	case dnd5e.CreationStepAbilityScores:
		return dnd5ev1alpha1.CreationStep_CREATION_STEP_ABILITY_SCORES
	case dnd5e.CreationStepSkills:
		return dnd5ev1alpha1.CreationStep_CREATION_STEP_SKILLS
	case dnd5e.CreationStepLanguages:
		return dnd5ev1alpha1.CreationStep_CREATION_STEP_LANGUAGES
	case dnd5e.CreationStepReview:
		return dnd5ev1alpha1.CreationStep_CREATION_STEP_REVIEW
	default:
		return dnd5ev1alpha1.CreationStep_CREATION_STEP_UNSPECIFIED
	}
}

// Helper converters

func convertWarningsToProto(warnings []character.ValidationWarning) []*dnd5ev1alpha1.ValidationWarning {
	protoWarnings := make([]*dnd5ev1alpha1.ValidationWarning, len(warnings))
	for i, w := range warnings {
		protoWarnings[i] = &dnd5ev1alpha1.ValidationWarning{
			Field:   w.Field,
			Message: w.Message,
			Type:    w.Type,
		}
	}
	return protoWarnings
}

func convertErrorsToProto(errors []character.ValidationError) []*dnd5ev1alpha1.ValidationError {
	protoErrors := make([]*dnd5ev1alpha1.ValidationError, len(errors))
	for i, e := range errors {
		protoErrors[i] = &dnd5ev1alpha1.ValidationError{
			Field:   e.Field,
			Message: e.Message,
			Code:    e.Type,
		}
	}
	return protoErrors
}

func convertCharacterToProto(char *dnd5e.Character) *dnd5ev1alpha1.Character {
	if char == nil {
		return nil
	}

	return &dnd5ev1alpha1.Character{
		Id:               char.ID,
		Name:             char.Name,
		Level:            char.Level,
		ExperiencePoints: char.ExperiencePoints,
		Race:             mapConstantToProtoRace(char.RaceID),
		Subrace:          mapConstantToProtoSubrace(char.SubraceID),
		Class:            mapConstantToProtoClass(char.ClassID),
		Background:       mapConstantToProtoBackground(char.BackgroundID),
		Alignment:        mapConstantToProtoAlignment(char.Alignment),
		AbilityScores: &dnd5ev1alpha1.AbilityScores{
			Strength:     char.AbilityScores.Strength,
			Dexterity:    char.AbilityScores.Dexterity,
			Constitution: char.AbilityScores.Constitution,
			Intelligence: char.AbilityScores.Intelligence,
			Wisdom:       char.AbilityScores.Wisdom,
			Charisma:     char.AbilityScores.Charisma,
		},
		CurrentHitPoints:   char.CurrentHP,
		TemporaryHitPoints: char.TempHP,
		SessionId:          char.SessionID,
		Metadata: &dnd5ev1alpha1.CharacterMetadata{
			CreatedAt: char.CreatedAt,
			UpdatedAt: char.UpdatedAt,
			PlayerId:  char.PlayerID,
		},
	}
}

// ListRaces returns a list of available races for character creation
func (h *Handler) ListRaces(
	ctx context.Context,
	req *dnd5ev1alpha1.ListRacesRequest,
) (*dnd5ev1alpha1.ListRacesResponse, error) {
	input := &character.ListRacesInput{
		PageSize:        req.PageSize,
		PageToken:       req.PageToken,
		IncludeSubraces: req.IncludeSubraces,
	}

	output, err := h.characterService.ListRaces(ctx, input)
	if err != nil {
		return nil, errors.ToGRPCError(err)
	}

	// Convert entity races to proto format
	protoRaces := make([]*dnd5ev1alpha1.RaceInfo, len(output.Races))
	for i, race := range output.Races {
		protoRaces[i] = convertEntityRaceToProto(race)
	}

	return &dnd5ev1alpha1.ListRacesResponse{
		Races:         protoRaces,
		NextPageToken: output.NextPageToken,
		TotalSize:     output.TotalSize,
	}, nil
}

// ListClasses returns a list of available classes for character creation
func (h *Handler) ListClasses(
	ctx context.Context,
	req *dnd5ev1alpha1.ListClassesRequest,
) (*dnd5ev1alpha1.ListClassesResponse, error) {
	input := &character.ListClassesInput{
		PageSize:                req.PageSize,
		PageToken:               req.PageToken,
		IncludeSpellcastersOnly: req.IncludeSpellcastersOnly,
		IncludeFeatures:         req.IncludeFeatures,
	}

	output, err := h.characterService.ListClasses(ctx, input)
	if err != nil {
		return nil, errors.ToGRPCError(err)
	}

	// Convert entity classes to proto format
	protoClasses := make([]*dnd5ev1alpha1.ClassInfo, len(output.Classes))
	for i, class := range output.Classes {
		protoClasses[i] = convertEntityClassToProto(class)
	}

	return &dnd5ev1alpha1.ListClassesResponse{
		Classes:       protoClasses,
		NextPageToken: output.NextPageToken,
		TotalSize:     output.TotalSize,
	}, nil
}

// ListBackgrounds returns a list of available backgrounds for character creation
func (h *Handler) ListBackgrounds(
	ctx context.Context,
	req *dnd5ev1alpha1.ListBackgroundsRequest,
) (*dnd5ev1alpha1.ListBackgroundsResponse, error) {
	input := &character.ListBackgroundsInput{
		PageSize:  req.PageSize,
		PageToken: req.PageToken,
	}

	output, err := h.characterService.ListBackgrounds(ctx, input)
	if err != nil {
		return nil, errors.ToGRPCError(err)
	}

	// Convert entity backgrounds to proto format
	protoBackgrounds := make([]*dnd5ev1alpha1.BackgroundInfo, len(output.Backgrounds))
	for i, background := range output.Backgrounds {
		protoBackgrounds[i] = convertEntityBackgroundToProto(background)
	}

	return &dnd5ev1alpha1.ListBackgroundsResponse{
		Backgrounds:   protoBackgrounds,
		NextPageToken: output.NextPageToken,
		TotalSize:     output.TotalSize,
	}, nil
}

// GetRaceDetails returns detailed information about a specific race
func (h *Handler) GetRaceDetails(
	ctx context.Context,
	req *dnd5ev1alpha1.GetRaceDetailsRequest,
) (*dnd5ev1alpha1.GetRaceDetailsResponse, error) {
	if req.RaceId == "" {
		return nil, errors.ToGRPCError(errors.InvalidArgument("race_id is required"))
	}

	input := &character.GetRaceDetailsInput{
		RaceID: req.RaceId,
	}

	output, err := h.characterService.GetRaceDetails(ctx, input)
	if err != nil {
		return nil, errors.ToGRPCError(err)
	}

	return &dnd5ev1alpha1.GetRaceDetailsResponse{
		Race: convertEntityRaceToProto(output.Race),
	}, nil
}

// GetClassDetails returns detailed information about a specific class
func (h *Handler) GetClassDetails(
	ctx context.Context,
	req *dnd5ev1alpha1.GetClassDetailsRequest,
) (*dnd5ev1alpha1.GetClassDetailsResponse, error) {
	if req.ClassId == "" {
		return nil, errors.ToGRPCError(errors.InvalidArgument("class_id is required"))
	}

	input := &character.GetClassDetailsInput{
		ClassID: req.ClassId,
	}

	output, err := h.characterService.GetClassDetails(ctx, input)
	if err != nil {
		return nil, errors.ToGRPCError(err)
	}

	return &dnd5ev1alpha1.GetClassDetailsResponse{
		Class: convertEntityClassToProto(output.Class),
	}, nil
}

// GetBackgroundDetails returns detailed information about a specific background
func (h *Handler) GetBackgroundDetails(
	ctx context.Context,
	req *dnd5ev1alpha1.GetBackgroundDetailsRequest,
) (*dnd5ev1alpha1.GetBackgroundDetailsResponse, error) {
	if req.BackgroundId == "" {
		return nil, errors.ToGRPCError(errors.InvalidArgument("background_id is required"))
	}

	input := &character.GetBackgroundDetailsInput{
		BackgroundID: req.BackgroundId,
	}

	output, err := h.characterService.GetBackgroundDetails(ctx, input)
	if err != nil {
		return nil, errors.ToGRPCError(err)
	}

	return &dnd5ev1alpha1.GetBackgroundDetailsResponse{
		Background: convertEntityBackgroundToProto(output.Background),
	}, nil
}

// Conversion functions from entity to proto format

// convertEntityRaceToProto converts entity race to proto format
func convertEntityRaceToProto(race *dnd5e.RaceInfo) *dnd5ev1alpha1.RaceInfo {
	if race == nil {
		return nil
	}

	// Convert traits
	protoTraits := make([]*dnd5ev1alpha1.RacialTrait, len(race.Traits))
	for i, trait := range race.Traits {
		protoTraits[i] = &dnd5ev1alpha1.RacialTrait{
			Name:        trait.Name,
			Description: trait.Description,
			IsChoice:    trait.IsChoice,
			Options:     trait.Options,
		}
	}

	// Convert subraces
	protoSubraces := make([]*dnd5ev1alpha1.SubraceInfo, len(race.Subraces))
	for i, subrace := range race.Subraces {
		subraceTraits := make([]*dnd5ev1alpha1.RacialTrait, len(subrace.Traits))
		for j, trait := range subrace.Traits {
			subraceTraits[j] = &dnd5ev1alpha1.RacialTrait{
				Name:        trait.Name,
				Description: trait.Description,
				IsChoice:    trait.IsChoice,
				Options:     trait.Options,
			}
		}

		// Convert subrace languages
		subraceLanguages := make([]dnd5ev1alpha1.Language, len(subrace.Languages))
		for k, lang := range subrace.Languages {
			subraceLanguages[k] = mapStringToProtoLanguage(lang)
		}

		protoSubraces[i] = &dnd5ev1alpha1.SubraceInfo{
			Id:             subrace.ID,
			Name:           subrace.Name,
			Description:    subrace.Description,
			AbilityBonuses: subrace.AbilityBonuses,
			Traits:         subraceTraits,
			Languages:      subraceLanguages,
			Proficiencies:  subrace.Proficiencies,
		}
	}

	// Convert languages
	protoLanguages := make([]dnd5ev1alpha1.Language, len(race.Languages))
	for i, lang := range race.Languages {
		protoLanguages[i] = mapStringToProtoLanguage(lang)
	}

	// Convert all choices
	choices := make([]*dnd5ev1alpha1.Choice, 0, len(race.Choices))
	for i := range race.Choices {
		choices = append(choices, convertChoiceToProto(&race.Choices[i]))
	}

	return &dnd5ev1alpha1.RaceInfo{
		Id:                   race.ID,
		Name:                 race.Name,
		Description:          race.Description,
		Speed:                race.Speed,
		Size:                 mapStringToProtoSize(race.Size),
		SizeDescription:      race.SizeDescription,
		AbilityBonuses:       race.AbilityBonuses,
		Traits:               protoTraits,
		Subraces:             protoSubraces,
		Proficiencies:        race.Proficiencies,
		Languages:            protoLanguages,
		AgeDescription:       race.AgeDescription,
		AlignmentDescription: race.AlignmentDescription,
		Choices:              choices,
	}
}

// convertEntityClassToProto converts entity class to proto format
func convertEntityClassToProto(class *dnd5e.ClassInfo) *dnd5ev1alpha1.ClassInfo {
	if class == nil {
		return nil
	}

	// Convert all choices
	choices := make([]*dnd5ev1alpha1.Choice, 0, len(class.Choices))
	for i := range class.Choices {
		choices = append(choices, convertChoiceToProto(&class.Choices[i]))
	}

	// Convert level 1 features
	protoLevel1Features := make([]*dnd5ev1alpha1.FeatureInfo, len(class.Level1Features))
	for i, feature := range class.Level1Features {
		protoLevel1Features[i] = convertFeatureInfoToProto(&feature)
	}

	// Convert spellcasting info
	var protoSpellcasting *dnd5ev1alpha1.SpellcastingInfo
	if class.Spellcasting != nil {
		protoSpellcasting = &dnd5ev1alpha1.SpellcastingInfo{
			SpellcastingAbility: class.Spellcasting.SpellcastingAbility,
			RitualCasting:       class.Spellcasting.RitualCasting,
			SpellcastingFocus:   class.Spellcasting.SpellcastingFocus,
			CantripsKnown:       class.Spellcasting.CantripsKnown,
			SpellsKnown:         class.Spellcasting.SpellsKnown,
			SpellSlotsLevel_1:   class.Spellcasting.SpellSlotsLevel1,
		}
	}

	return &dnd5ev1alpha1.ClassInfo{
		Id:                       class.ID,
		Name:                     class.Name,
		Description:              class.Description,
		HitDie:                   class.HitDie,
		PrimaryAbilities:         class.PrimaryAbilities,
		ArmorProficiencies:       class.ArmorProficiencies,
		WeaponProficiencies:      class.WeaponProficiencies,
		ToolProficiencies:        class.ToolProficiencies,
		SavingThrowProficiencies: class.SavingThrowProficiencies,
		SkillChoicesCount:        class.SkillChoicesCount,
		AvailableSkills:          class.AvailableSkills,
		StartingEquipment:        class.StartingEquipment,
		Level_1Features:          protoLevel1Features,
		Spellcasting:             protoSpellcasting,
		Choices:                  choices,
	}
}

// convertEntityBackgroundToProto converts entity background to proto format
func convertEntityBackgroundToProto(background *dnd5e.BackgroundInfo) *dnd5ev1alpha1.BackgroundInfo {
	if background == nil {
		return nil
	}

	// Convert languages
	protoLanguages := make([]dnd5ev1alpha1.Language, len(background.Languages))
	for i, lang := range background.Languages {
		protoLanguages[i] = mapStringToProtoLanguage(lang)
	}

	return &dnd5ev1alpha1.BackgroundInfo{
		Id:                  background.ID,
		Name:                background.Name,
		Description:         background.Description,
		SkillProficiencies:  background.SkillProficiencies,
		ToolProficiencies:   background.ToolProficiencies,
		Languages:           protoLanguages,
		AdditionalLanguages: background.AdditionalLanguages,
		StartingEquipment:   background.StartingEquipment,
		StartingGold:        background.StartingGold,
		FeatureName:         background.FeatureName,
		FeatureDescription:  background.FeatureDescription,
		PersonalityTraits:   background.PersonalityTraits,
		Ideals:              background.Ideals,
		Bonds:               background.Bonds,
		Flaws:               background.Flaws,
	}
}

// mapStringToProtoLanguage converts string to proto language enum
func mapStringToProtoLanguage(lang string) dnd5ev1alpha1.Language {
	// Convert to lowercase for case-insensitive matching
	switch strings.ToLower(lang) {
	case "common":
		return dnd5ev1alpha1.Language_LANGUAGE_COMMON
	case "dwarvish":
		return dnd5ev1alpha1.Language_LANGUAGE_DWARVISH
	case "elvish":
		return dnd5ev1alpha1.Language_LANGUAGE_ELVISH
	case "giant":
		return dnd5ev1alpha1.Language_LANGUAGE_GIANT
	case "gnomish":
		return dnd5ev1alpha1.Language_LANGUAGE_GNOMISH
	case "goblin":
		return dnd5ev1alpha1.Language_LANGUAGE_GOBLIN
	case "halfling":
		return dnd5ev1alpha1.Language_LANGUAGE_HALFLING
	case "orc":
		return dnd5ev1alpha1.Language_LANGUAGE_ORC
	case "abyssal":
		return dnd5ev1alpha1.Language_LANGUAGE_ABYSSAL
	case "celestial":
		return dnd5ev1alpha1.Language_LANGUAGE_CELESTIAL
	case "draconic":
		return dnd5ev1alpha1.Language_LANGUAGE_DRACONIC
	case "deep-speech":
		return dnd5ev1alpha1.Language_LANGUAGE_DEEP_SPEECH
	case "infernal":
		return dnd5ev1alpha1.Language_LANGUAGE_INFERNAL
	case "primordial":
		return dnd5ev1alpha1.Language_LANGUAGE_PRIMORDIAL
	case "sylvan":
		return dnd5ev1alpha1.Language_LANGUAGE_SYLVAN
	case "undercommon":
		return dnd5ev1alpha1.Language_LANGUAGE_UNDERCOMMON
	default:
		return dnd5ev1alpha1.Language_LANGUAGE_UNSPECIFIED
	}
}

// mapStringToProtoSize converts string to proto size enum
func mapStringToProtoSize(size string) dnd5ev1alpha1.Size {
	switch size {
	case "tiny":
		return dnd5ev1alpha1.Size_SIZE_TINY
	case "small":
		return dnd5ev1alpha1.Size_SIZE_SMALL
	case "medium":
		return dnd5ev1alpha1.Size_SIZE_MEDIUM
	case "large":
		return dnd5ev1alpha1.Size_SIZE_LARGE
	case "huge":
		return dnd5ev1alpha1.Size_SIZE_HUGE
	case "gargantuan":
		return dnd5ev1alpha1.Size_SIZE_GARGANTUAN
	default:
		return dnd5ev1alpha1.Size_SIZE_UNSPECIFIED
	}
}

// ListEquipmentByType returns equipment filtered by type
func (h *Handler) ListEquipmentByType(
	ctx context.Context,
	req *dnd5ev1alpha1.ListEquipmentByTypeRequest,
) (*dnd5ev1alpha1.ListEquipmentByTypeResponse, error) {
	if req.EquipmentType == dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_UNSPECIFIED {
		return nil, errors.ToGRPCError(errors.InvalidArgument("equipment_type is required"))
	}

	input := &character.ListEquipmentByTypeInput{
		EquipmentType: mapProtoEquipmentTypeToString(req.EquipmentType),
		PageSize:      req.PageSize,
		PageToken:     req.PageToken,
	}

	// Default page size if not specified
	if input.PageSize == 0 {
		input.PageSize = 20
	}

	output, err := h.characterService.ListEquipmentByType(ctx, input)
	if err != nil {
		return nil, errors.ToGRPCError(err)
	}

	// Convert equipment to proto format
	protoEquipment := make([]*dnd5ev1alpha1.Equipment, len(output.Equipment))
	for i, equipment := range output.Equipment {
		protoEquipment[i] = convertEquipmentToProto(equipment)
	}

	return &dnd5ev1alpha1.ListEquipmentByTypeResponse{
		Equipment:     protoEquipment,
		NextPageToken: output.NextPageToken,
		TotalSize:     output.TotalSize,
	}, nil
}

// ListSpellsByLevel returns spells filtered by level
func (h *Handler) ListSpellsByLevel(
	ctx context.Context,
	req *dnd5ev1alpha1.ListSpellsByLevelRequest,
) (*dnd5ev1alpha1.ListSpellsByLevelResponse, error) {
	// Validate spell level
	if req.Level < 0 || req.Level > 9 {
		return nil, errors.ToGRPCError(errors.InvalidArgument("level must be between 0 and 9"))
	}

	input := &character.ListSpellsByLevelInput{
		Level:     req.Level,
		PageSize:  req.PageSize,
		PageToken: req.PageToken,
	}

	// Convert class filter if provided
	if req.Class != dnd5ev1alpha1.Class_CLASS_UNSPECIFIED {
		input.ClassID = mapProtoClassToConstant(req.Class)
	}

	// Default page size if not specified
	if input.PageSize == 0 {
		input.PageSize = 20
	}

	output, err := h.characterService.ListSpellsByLevel(ctx, input)
	if err != nil {
		return nil, errors.ToGRPCError(err)
	}

	// Convert spells to proto format
	protoSpells := make([]*dnd5ev1alpha1.Spell, len(output.Spells))
	for i, spell := range output.Spells {
		protoSpells[i] = convertSpellToProto(spell)
	}

	return &dnd5ev1alpha1.ListSpellsByLevelResponse{
		Spells:        protoSpells,
		NextPageToken: output.NextPageToken,
		TotalSize:     output.TotalSize,
	}, nil
}

// Conversion functions for equipment and spells

// mapProtoEquipmentTypeToString converts proto equipment type to string
func mapProtoEquipmentTypeToString(equipmentType dnd5ev1alpha1.EquipmentType) string {
	switch equipmentType {
	case dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_SIMPLE_MELEE_WEAPON:
		return "simple-melee-weapons"
	case dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_SIMPLE_RANGED_WEAPON:
		return "simple-ranged-weapons"
	case dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_MARTIAL_MELEE_WEAPON:
		return "martial-melee-weapons"
	case dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_MARTIAL_RANGED_WEAPON:
		return "martial-ranged-weapons"
	case dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_LIGHT_ARMOR:
		return "light-armor"
	case dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_MEDIUM_ARMOR:
		return "medium-armor"
	case dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_HEAVY_ARMOR:
		return "heavy-armor"
	case dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_SHIELD:
		return "shields"
	case dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_ADVENTURING_GEAR:
		return "adventuring-gear"
	case dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_TOOLS:
		return "tools"
	case dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_ARTISAN_TOOLS:
		return "artisan-tools"
	case dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_GAMING_SET:
		return "gaming-sets"
	case dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_MUSICAL_INSTRUMENT:
		return "musical-instruments"
	case dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_VEHICLE:
		return "vehicles"
	default:
		return ""
	}
}

// convertEquipmentToProto converts entity equipment to proto format
func convertEquipmentToProto(equipment *dnd5e.EquipmentInfo) *dnd5ev1alpha1.Equipment {
	if equipment == nil {
		return nil
	}

	protoEquipment := &dnd5ev1alpha1.Equipment{
		Id:          equipment.ID,
		Name:        equipment.Name,
		Category:    equipment.Category,
		Description: equipment.Description,
	}

	// Convert cost - parse the string format
	if equipment.Cost != "" {
		// Parse cost string like "2 gp" into structured format
		parts := strings.Fields(equipment.Cost)
		if len(parts) == 2 {
			quantity, err := strconv.Atoi(parts[0])
			if err == nil {
				protoEquipment.Cost = &dnd5ev1alpha1.Cost{
					Quantity: int32(quantity), // nolint:gosec
					Unit:     parts[1],
				}
			} else {
				// Log error and default to safe values
				protoEquipment.Cost = &dnd5ev1alpha1.Cost{
					Quantity: 1,
					Unit:     "gp",
				}
			}
		} else {
			// Default to safe values if format is invalid
			protoEquipment.Cost = &dnd5ev1alpha1.Cost{
				Quantity: 1,
				Unit:     "gp",
			}
		}
	}

	// Convert weight - parse the string format
	if equipment.Weight != "" {
		// Parse weight string like "2 lbs" into structured format
		parts := strings.Fields(equipment.Weight)
		if len(parts) == 2 {
			quantity, err := strconv.Atoi(parts[0])
			if err == nil {
				protoEquipment.Weight = &dnd5ev1alpha1.Weight{
					Quantity: int32(quantity), // nolint:gosec
					Unit:     parts[1],
				}
			} else {
				// Log error and default to safe values
				protoEquipment.Weight = &dnd5ev1alpha1.Weight{
					Quantity: 1,
					Unit:     "lbs",
				}
			}
		} else {
			// Default to safe values if format is invalid
			protoEquipment.Weight = &dnd5ev1alpha1.Weight{
				Quantity: 1,
				Unit:     "lbs",
			}
		}
	}

	// TODO: Add equipment type-specific data (weapon, armor, gear)
	// This would require checking equipment.Type and converting accordingly

	return protoEquipment
}

// convertSpellToProto converts entity spell to proto format
func convertSpellToProto(spell *dnd5e.SpellInfo) *dnd5ev1alpha1.Spell {
	if spell == nil {
		return nil
	}

	protoSpell := &dnd5ev1alpha1.Spell{
		Id:          spell.ID,
		Name:        spell.Name,
		Level:       spell.Level,
		School:      spell.School,
		CastingTime: spell.CastingTime,
		Range:       spell.Range,
		Duration:    spell.Duration,
		Description: spell.Description,
		Components:  strings.Join(spell.Components, ", "),
	}

	// Convert classes
	protoSpell.Classes = make([]string, len(spell.Classes))
	copy(protoSpell.Classes, spell.Classes)

	// TODO: Add spell damage and area of effect conversion
	// This would require more detailed spell data from the dnd5e-api

	return protoSpell
}

// convertFeatureInfoToProto converts entity FeatureInfo to proto FeatureInfo
func convertFeatureInfoToProto(feature *dnd5e.FeatureInfo) *dnd5ev1alpha1.FeatureInfo {
	if feature == nil {
		return nil
	}

	protoFeature := &dnd5ev1alpha1.FeatureInfo{
		Id:          feature.ID,
		Name:        feature.Name,
		Description: feature.Description,
		Level:       feature.Level,
		ClassName:   feature.ClassName,
		HasChoices:  feature.HasChoices,
	}

	// Convert choices
	if len(feature.Choices) > 0 {
		protoChoices := make([]*dnd5ev1alpha1.Choice, len(feature.Choices))
		for i := range feature.Choices {
			protoChoices[i] = convertChoiceToProto(&feature.Choices[i])
		}
		protoFeature.Choices = protoChoices
	}

	// Convert spell selection info
	if feature.SpellSelection != nil {
		protoFeature.SpellSelection = &dnd5ev1alpha1.SpellSelectionInfo{
			SpellsToSelect:  feature.SpellSelection.SpellsToSelect,
			SpellLevels:     feature.SpellSelection.SpellLevels,
			SpellLists:      feature.SpellSelection.SpellLists,
			SelectionType:   convertSpellSelectionTypeToProto(feature.SpellSelection.SelectionType),
			RequiresReplace: feature.SpellSelection.RequiresReplace,
		}
	}

	return protoFeature
}

// convertSpellSelectionTypeToProto converts string to proto enum
func convertSpellSelectionTypeToProto(selectionType string) dnd5ev1alpha1.SpellSelectionType {
	switch selectionType {
	case "spellbook":
		return dnd5ev1alpha1.SpellSelectionType_SPELL_SELECTION_TYPE_SPELLBOOK
	case "known":
		return dnd5ev1alpha1.SpellSelectionType_SPELL_SELECTION_TYPE_KNOWN
	case "prepared":
		return dnd5ev1alpha1.SpellSelectionType_SPELL_SELECTION_TYPE_PREPARED
	default:
		return dnd5ev1alpha1.SpellSelectionType_SPELL_SELECTION_TYPE_UNSPECIFIED
	}
}

// Choice mapping functions

// mapProtoChoiceTypeToConstant converts proto ChoiceType to entity constant
func mapProtoChoiceTypeToConstant(protoType dnd5ev1alpha1.ChoiceType) dnd5e.ChoiceType {
	switch protoType {
	case dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_EQUIPMENT:
		return dnd5e.ChoiceTypeEquipment
	case dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_SKILL:
		return dnd5e.ChoiceTypeSkill
	case dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_TOOL:
		return dnd5e.ChoiceTypeTool
	case dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_LANGUAGE:
		return dnd5e.ChoiceTypeLanguage
	case dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_SPELL:
		return dnd5e.ChoiceTypeSpell
	case dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_FEAT:
		return dnd5e.ChoiceTypeFeat
	// Note: Fighting style, cantrips, and spells are handled as spell types
	// These will map to CHOICE_TYPE_SPELL for now
	default:
		return dnd5e.ChoiceTypeEquipment // Default
	}
}

// mapConstantToProtoChoiceType converts entity constant to proto ChoiceType
func mapConstantToProtoChoiceType(constant dnd5e.ChoiceType) dnd5ev1alpha1.ChoiceType {
	switch constant {
	case dnd5e.ChoiceTypeEquipment:
		return dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_EQUIPMENT
	case dnd5e.ChoiceTypeSkill:
		return dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_SKILL
	case dnd5e.ChoiceTypeTool:
		return dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_TOOL
	case dnd5e.ChoiceTypeLanguage:
		return dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_LANGUAGE
	case dnd5e.ChoiceTypeSpell:
		return dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_SPELL
	case dnd5e.ChoiceTypeFeat:
		return dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_FEAT
	case dnd5e.ChoiceTypeFightingStyle, dnd5e.ChoiceTypeCantrips, dnd5e.ChoiceTypeSpells:
		return dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_SPELL
	default:
		return dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_UNSPECIFIED
	}
}

// mapProtoChoiceSourceToConstant converts proto ChoiceSource to entity constant
func mapProtoChoiceSourceToConstant(protoSource dnd5ev1alpha1.ChoiceSource) dnd5e.ChoiceSource {
	switch protoSource {
	case dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_RACE:
		return dnd5e.ChoiceSourceRace
	case dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS:
		return dnd5e.ChoiceSourceClass
	case dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_BACKGROUND:
		return dnd5e.ChoiceSourceBackground
	case dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_SUBRACE:
		return dnd5e.ChoiceSourceSubrace
	// Note: CHOICE_SOURCE_FEATURE is not in the proto, mapping features to CLASS
	default:
		return dnd5e.ChoiceSourceRace // Default
	}
}

// mapConstantToProtoChoiceSource converts entity constant to proto ChoiceSource
func mapConstantToProtoChoiceSource(constant dnd5e.ChoiceSource) dnd5ev1alpha1.ChoiceSource {
	switch constant {
	case dnd5e.ChoiceSourceRace:
		return dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_RACE
	case dnd5e.ChoiceSourceClass:
		return dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS
	case dnd5e.ChoiceSourceBackground:
		return dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_BACKGROUND
	case dnd5e.ChoiceSourceSubrace:
		return dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_SUBRACE
	case dnd5e.ChoiceSourceFeature:
		return dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS
	default:
		return dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_UNSPECIFIED
	}
}

// mapProtoAbilityToConstant converts proto Ability to entity constant
func mapProtoAbilityToConstant(protoAbility dnd5ev1alpha1.Ability) string {
	switch protoAbility {
	case dnd5ev1alpha1.Ability_ABILITY_STRENGTH:
		return dnd5e.AbilityStrength
	case dnd5ev1alpha1.Ability_ABILITY_DEXTERITY:
		return dnd5e.AbilityDexterity
	case dnd5ev1alpha1.Ability_ABILITY_CONSTITUTION:
		return dnd5e.AbilityConstitution
	case dnd5ev1alpha1.Ability_ABILITY_INTELLIGENCE:
		return dnd5e.AbilityIntelligence
	case dnd5ev1alpha1.Ability_ABILITY_WISDOM:
		return dnd5e.AbilityWisdom
	case dnd5ev1alpha1.Ability_ABILITY_CHARISMA:
		return dnd5e.AbilityCharisma
	default:
		return dnd5e.AbilityStrength // Default
	}
}

// mapConstantToProtoAbility converts entity constant to proto Ability
func mapConstantToProtoAbility(constant string) dnd5ev1alpha1.Ability {
	switch constant {
	case dnd5e.AbilityStrength:
		return dnd5ev1alpha1.Ability_ABILITY_STRENGTH
	case dnd5e.AbilityDexterity:
		return dnd5ev1alpha1.Ability_ABILITY_DEXTERITY
	case dnd5e.AbilityConstitution:
		return dnd5ev1alpha1.Ability_ABILITY_CONSTITUTION
	case dnd5e.AbilityIntelligence:
		return dnd5ev1alpha1.Ability_ABILITY_INTELLIGENCE
	case dnd5e.AbilityWisdom:
		return dnd5ev1alpha1.Ability_ABILITY_WISDOM
	case dnd5e.AbilityCharisma:
		return dnd5ev1alpha1.Ability_ABILITY_CHARISMA
	default:
		return dnd5ev1alpha1.Ability_ABILITY_UNSPECIFIED
	}
}

// convertProtoChoicesToEntity converts proto choice selections to entity format
func convertProtoChoicesToEntity(protoChoices []*dnd5ev1alpha1.ChoiceSelection) []dnd5e.ChoiceSelection {
	choices := make([]dnd5e.ChoiceSelection, 0, len(protoChoices))
	for _, protoChoice := range protoChoices {
		if protoChoice != nil {
			choice := dnd5e.ChoiceSelection{
				ChoiceID:     protoChoice.ChoiceId,
				ChoiceType:   mapProtoChoiceTypeToConstant(protoChoice.ChoiceType),
				Source:       mapProtoChoiceSourceToConstant(protoChoice.Source),
				SelectedKeys: protoChoice.SelectedKeys,
			}

			// Convert ability score choices if present
			for _, protoASChoice := range protoChoice.AbilityScoreChoices {
				choice.AbilityScoreChoices = append(choice.AbilityScoreChoices, dnd5e.AbilityScoreChoice{
					Ability: mapProtoAbilityToConstant(protoASChoice.Ability),
					Bonus:   protoASChoice.Bonus,
				})
			}

			choices = append(choices, choice)
		}
	}
	return choices
}

// convertRaceInfoToProto converts entity RaceInfo to proto RaceInfo
func convertRaceInfoToProto(race *dnd5e.RaceInfo) *dnd5ev1alpha1.RaceInfo {
	if race == nil {
		return nil
	}

	protoRace := &dnd5ev1alpha1.RaceInfo{
		Id:                   race.ID,
		Name:                 race.Name,
		Description:          race.Description,
		Speed:                race.Speed,
		Size:                 mapSizeToProto(race.Size),
		SizeDescription:      race.SizeDescription,
		AgeDescription:       race.AgeDescription,
		AlignmentDescription: race.AlignmentDescription,
	}

	// Convert ability bonuses
	if len(race.AbilityBonuses) > 0 {
		protoRace.AbilityBonuses = make(map[string]int32)
		for k, v := range race.AbilityBonuses {
			protoRace.AbilityBonuses[k] = v
		}
	}

	// Convert traits
	if len(race.Traits) > 0 {
		protoRace.Traits = make([]*dnd5ev1alpha1.RacialTrait, len(race.Traits))
		for i, trait := range race.Traits {
			protoRace.Traits[i] = &dnd5ev1alpha1.RacialTrait{
				Name:        trait.Name,
				Description: trait.Description,
				IsChoice:    trait.IsChoice,
				Options:     trait.Options,
			}
		}
	}

	// Convert languages
	if len(race.Languages) > 0 {
		protoRace.Languages = make([]dnd5ev1alpha1.Language, len(race.Languages))
		for i, lang := range race.Languages {
			protoRace.Languages[i] = mapConstantToProtoLanguage(lang)
		}
	}

	// Convert proficiencies
	protoRace.Proficiencies = race.Proficiencies

	// Convert choices
	if len(race.Choices) > 0 {
		protoRace.Choices = make([]*dnd5ev1alpha1.Choice, len(race.Choices))
		for i := range race.Choices {
			protoRace.Choices[i] = convertChoiceToProto(&race.Choices[i])
		}
	}

	// Convert subraces
	if len(race.Subraces) > 0 {
		protoRace.Subraces = make([]*dnd5ev1alpha1.SubraceInfo, len(race.Subraces))
		for i := range race.Subraces {
			protoRace.Subraces[i] = convertSubraceInfoToProto(&race.Subraces[i])
		}
	}

	return protoRace
}

// convertSubraceInfoToProto converts entity SubraceInfo to proto SubraceInfo
func convertSubraceInfoToProto(subrace *dnd5e.SubraceInfo) *dnd5ev1alpha1.SubraceInfo {
	if subrace == nil {
		return nil
	}

	protoSubrace := &dnd5ev1alpha1.SubraceInfo{
		Id:          subrace.ID,
		Name:        subrace.Name,
		Description: subrace.Description,
	}

	// Convert ability bonuses
	if len(subrace.AbilityBonuses) > 0 {
		protoSubrace.AbilityBonuses = make(map[string]int32)
		for k, v := range subrace.AbilityBonuses {
			protoSubrace.AbilityBonuses[k] = v
		}
	}

	// Convert traits
	if len(subrace.Traits) > 0 {
		protoSubrace.Traits = make([]*dnd5ev1alpha1.RacialTrait, len(subrace.Traits))
		for i, trait := range subrace.Traits {
			protoSubrace.Traits[i] = &dnd5ev1alpha1.RacialTrait{
				Name:        trait.Name,
				Description: trait.Description,
				IsChoice:    trait.IsChoice,
				Options:     trait.Options,
			}
		}
	}

	// Convert languages
	if len(subrace.Languages) > 0 {
		protoSubrace.Languages = make([]dnd5ev1alpha1.Language, len(subrace.Languages))
		for i, lang := range subrace.Languages {
			protoSubrace.Languages[i] = mapConstantToProtoLanguage(lang)
		}
	}

	// Convert proficiencies
	protoSubrace.Proficiencies = subrace.Proficiencies

	return protoSubrace
}

// convertClassInfoToProto converts entity ClassInfo to proto ClassInfo
func convertClassInfoToProto(class *dnd5e.ClassInfo) *dnd5ev1alpha1.ClassInfo {
	if class == nil {
		return nil
	}

	protoClass := &dnd5ev1alpha1.ClassInfo{
		Id:                       class.ID,
		Name:                     class.Name,
		Description:              class.Description,
		HitDie:                   class.HitDie,
		PrimaryAbilities:         class.PrimaryAbilities,
		ArmorProficiencies:       class.ArmorProficiencies,
		WeaponProficiencies:      class.WeaponProficiencies,
		ToolProficiencies:        class.ToolProficiencies,
		SavingThrowProficiencies: class.SavingThrowProficiencies,
		SkillChoicesCount:        class.SkillChoicesCount,
		AvailableSkills:          class.AvailableSkills,
		StartingEquipment:        class.StartingEquipment,
	}

	// Convert level 1 features
	if len(class.Level1Features) > 0 {
		protoClass.Level_1Features = make([]*dnd5ev1alpha1.FeatureInfo, len(class.Level1Features))
		for i := range class.Level1Features {
			protoClass.Level_1Features[i] = convertFeatureInfoToProto(&class.Level1Features[i])
		}
	}

	// Convert spellcasting info
	if class.Spellcasting != nil {
		protoClass.Spellcasting = &dnd5ev1alpha1.SpellcastingInfo{
			SpellcastingAbility: class.Spellcasting.SpellcastingAbility,
			RitualCasting:       class.Spellcasting.RitualCasting,
			SpellcastingFocus:   class.Spellcasting.SpellcastingFocus,
			CantripsKnown:       class.Spellcasting.CantripsKnown,
			SpellsKnown:         class.Spellcasting.SpellsKnown,
			SpellSlotsLevel_1:   class.Spellcasting.SpellSlotsLevel1,
		}
	}

	// Convert choices
	if len(class.Choices) > 0 {
		protoClass.Choices = make([]*dnd5ev1alpha1.Choice, len(class.Choices))
		for i := range class.Choices {
			protoClass.Choices[i] = convertChoiceToProto(&class.Choices[i])
		}
	}

	return protoClass
}

// convertBackgroundInfoToProto converts entity BackgroundInfo to proto BackgroundInfo
func convertBackgroundInfoToProto(background *dnd5e.BackgroundInfo) *dnd5ev1alpha1.BackgroundInfo {
	if background == nil {
		return nil
	}

	protoBackground := &dnd5ev1alpha1.BackgroundInfo{
		Id:                  background.ID,
		Name:                background.Name,
		Description:         background.Description,
		SkillProficiencies:  background.SkillProficiencies,
		ToolProficiencies:   background.ToolProficiencies,
		AdditionalLanguages: background.AdditionalLanguages,
		StartingEquipment:   background.StartingEquipment,
		StartingGold:        background.StartingGold,
		FeatureName:         background.FeatureName,
		FeatureDescription:  background.FeatureDescription,
		PersonalityTraits:   background.PersonalityTraits,
		Ideals:              background.Ideals,
		Bonds:               background.Bonds,
		Flaws:               background.Flaws,
	}

	// Convert languages
	if len(background.Languages) > 0 {
		protoBackground.Languages = make([]dnd5ev1alpha1.Language, len(background.Languages))
		for i, lang := range background.Languages {
			protoBackground.Languages[i] = mapConstantToProtoLanguage(lang)
		}
	}

	return protoBackground
}

// mapSizeToProto converts entity size string to proto Size
func mapSizeToProto(size string) dnd5ev1alpha1.Size {
	switch size {
	case "tiny":
		return dnd5ev1alpha1.Size_SIZE_TINY
	case "small":
		return dnd5ev1alpha1.Size_SIZE_SMALL
	case "medium":
		return dnd5ev1alpha1.Size_SIZE_MEDIUM
	case "large":
		return dnd5ev1alpha1.Size_SIZE_LARGE
	case "huge":
		return dnd5ev1alpha1.Size_SIZE_HUGE
	case "gargantuan":
		return dnd5ev1alpha1.Size_SIZE_GARGANTUAN
	default:
		return dnd5ev1alpha1.Size_SIZE_UNSPECIFIED
	}
}
