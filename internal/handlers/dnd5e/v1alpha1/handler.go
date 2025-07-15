// Package v1alpha1 handles the grpc service interface
package v1alpha1

import (
	"context"

	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
	"github.com/KirkDiggler/rpg-api/internal/errors"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/services/character"
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
		input.InitialData = convertProtoDraftToEntity(req.InitialData)
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

	input := &character.UpdateRaceInput{
		DraftID:   req.DraftId,
		RaceID:    mapProtoRaceToConstant(req.Race),
		SubraceID: mapProtoSubraceToConstant(req.Subrace),
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

	input := &character.UpdateClassInput{
		DraftID: req.DraftId,
		ClassID: mapProtoClassToConstant(req.Class),
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

	input := &character.UpdateBackgroundInput{
		DraftID:      req.DraftId,
		BackgroundID: mapProtoBackgroundToConstant(req.Background),
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
	if req.AbilityScores == nil {
		return nil, errors.ToGRPCError(errors.InvalidArgument("ability_scores is required"))
	}

	input := &character.UpdateAbilityScoresInput{
		DraftID: req.DraftId,
		AbilityScores: dnd5e.AbilityScores{
			Strength:     req.AbilityScores.Strength,
			Dexterity:    req.AbilityScores.Dexterity,
			Constitution: req.AbilityScores.Constitution,
			Intelligence: req.AbilityScores.Intelligence,
			Wisdom:       req.AbilityScores.Wisdom,
			Charisma:     req.AbilityScores.Charisma,
		},
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

	output, err := h.characterService.ListCharacters(ctx, input)
	if err != nil {
		return nil, errors.ToGRPCError(err)
	}

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

// Converter functions

func convertProtoDraftToEntity(proto *dnd5ev1alpha1.CharacterDraft) *dnd5e.CharacterDraft {
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

	// Convert skills
	for _, skill := range proto.StartingSkills {
		if skillID := mapProtoSkillToConstant(skill); skillID != "" {
			draft.StartingSkillIDs = append(draft.StartingSkillIDs, skillID)
		}
	}

	// Convert languages
	for _, lang := range proto.AdditionalLanguages {
		if langID := mapProtoLanguageToConstant(lang); langID != "" {
			draft.AdditionalLanguages = append(draft.AdditionalLanguages, langID)
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
		draft.DiscordChannelID = proto.Metadata.DiscordChannelId
		draft.DiscordMessageID = proto.Metadata.DiscordMessageId
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
			DiscordChannelId: entity.DiscordChannelID,
			DiscordMessageId: entity.DiscordMessageID,
		},
	}

	// Convert race
	if entity.RaceID != "" {
		proto.Race = mapConstantToProtoRace(entity.RaceID)
	}

	// Convert subrace
	if entity.SubraceID != "" {
		proto.Subrace = mapConstantToProtoSubrace(entity.SubraceID)
	}

	// Convert class
	if entity.ClassID != "" {
		proto.Class = mapConstantToProtoClass(entity.ClassID)
	}

	// Convert background
	if entity.BackgroundID != "" {
		proto.Background = mapConstantToProtoBackground(entity.BackgroundID)
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

	// Convert skills
	for _, skillID := range entity.StartingSkillIDs {
		if skill := mapConstantToProtoSkill(skillID); skill != dnd5ev1alpha1.Skill_SKILL_UNSPECIFIED {
			proto.StartingSkills = append(proto.StartingSkills, skill)
		}
	}

	// Convert languages
	for _, langID := range entity.AdditionalLanguages {
		if lang := mapConstantToProtoLanguage(langID); lang != dnd5ev1alpha1.Language_LANGUAGE_UNSPECIFIED {
			proto.AdditionalLanguages = append(proto.AdditionalLanguages, lang)
		}
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

func mapProtoLanguageToConstant(lang dnd5ev1alpha1.Language) string {
	switch lang {
	case dnd5ev1alpha1.Language_LANGUAGE_COMMON:
		return dnd5e.LanguageCommon
	case dnd5ev1alpha1.Language_LANGUAGE_DWARVISH:
		return dnd5e.LanguageDwarvish
	case dnd5ev1alpha1.Language_LANGUAGE_ELVISH:
		return dnd5e.LanguageElvish
	case dnd5ev1alpha1.Language_LANGUAGE_GIANT:
		return dnd5e.LanguageGiant
	case dnd5ev1alpha1.Language_LANGUAGE_GNOMISH:
		return dnd5e.LanguageGnomish
	case dnd5ev1alpha1.Language_LANGUAGE_GOBLIN:
		return dnd5e.LanguageGoblin
	case dnd5ev1alpha1.Language_LANGUAGE_HALFLING:
		return dnd5e.LanguageHalfling
	case dnd5ev1alpha1.Language_LANGUAGE_ORC:
		return dnd5e.LanguageOrc
	case dnd5ev1alpha1.Language_LANGUAGE_ABYSSAL:
		return dnd5e.LanguageAbyssal
	case dnd5ev1alpha1.Language_LANGUAGE_CELESTIAL:
		return dnd5e.LanguageCelestial
	case dnd5ev1alpha1.Language_LANGUAGE_DRACONIC:
		return dnd5e.LanguageDraconic
	case dnd5ev1alpha1.Language_LANGUAGE_DEEP_SPEECH:
		return dnd5e.LanguageDeepSpeech
	case dnd5ev1alpha1.Language_LANGUAGE_INFERNAL:
		return dnd5e.LanguageInfernal
	case dnd5ev1alpha1.Language_LANGUAGE_PRIMORDIAL:
		return dnd5e.LanguagePrimordial
	case dnd5ev1alpha1.Language_LANGUAGE_SYLVAN:
		return dnd5e.LanguageSylvan
	case dnd5ev1alpha1.Language_LANGUAGE_UNDERCOMMON:
		return dnd5e.LanguageUndercommon
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

func mapConstantToProtoSkill(constant string) dnd5ev1alpha1.Skill {
	switch constant {
	case dnd5e.SkillAcrobatics:
		return dnd5ev1alpha1.Skill_SKILL_ACROBATICS
	case dnd5e.SkillAnimalHandling:
		return dnd5ev1alpha1.Skill_SKILL_ANIMAL_HANDLING
	case dnd5e.SkillArcana:
		return dnd5ev1alpha1.Skill_SKILL_ARCANA
	case dnd5e.SkillAthletics:
		return dnd5ev1alpha1.Skill_SKILL_ATHLETICS
	case dnd5e.SkillDeception:
		return dnd5ev1alpha1.Skill_SKILL_DECEPTION
	case dnd5e.SkillHistory:
		return dnd5ev1alpha1.Skill_SKILL_HISTORY
	case dnd5e.SkillInsight:
		return dnd5ev1alpha1.Skill_SKILL_INSIGHT
	case dnd5e.SkillIntimidation:
		return dnd5ev1alpha1.Skill_SKILL_INTIMIDATION
	case dnd5e.SkillInvestigation:
		return dnd5ev1alpha1.Skill_SKILL_INVESTIGATION
	case dnd5e.SkillMedicine:
		return dnd5ev1alpha1.Skill_SKILL_MEDICINE
	case dnd5e.SkillNature:
		return dnd5ev1alpha1.Skill_SKILL_NATURE
	case dnd5e.SkillPerception:
		return dnd5ev1alpha1.Skill_SKILL_PERCEPTION
	case dnd5e.SkillPerformance:
		return dnd5ev1alpha1.Skill_SKILL_PERFORMANCE
	case dnd5e.SkillPersuasion:
		return dnd5ev1alpha1.Skill_SKILL_PERSUASION
	case dnd5e.SkillReligion:
		return dnd5ev1alpha1.Skill_SKILL_RELIGION
	case dnd5e.SkillSleightOfHand:
		return dnd5ev1alpha1.Skill_SKILL_SLEIGHT_OF_HAND
	case dnd5e.SkillStealth:
		return dnd5ev1alpha1.Skill_SKILL_STEALTH
	case dnd5e.SkillSurvival:
		return dnd5ev1alpha1.Skill_SKILL_SURVIVAL
	default:
		return dnd5ev1alpha1.Skill_SKILL_UNSPECIFIED
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
