package v1alpha1

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api/gen/go/github.com/KirkDiggler/rpg-api/api/proto/dnd5e/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	"github.com/KirkDiggler/rpg-api/internal/services/character"
)

// HandlerConfig holds dependencies for the handler
type HandlerConfig struct {
	CharacterService character.Service
}

// Validate ensures all required dependencies are present
func (c *HandlerConfig) Validate() error {
	if c.CharacterService == nil {
		return errors.New("character service is required")
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
func (h *Handler) CreateDraft(ctx context.Context, req *dnd5ev1alpha1.CreateDraftRequest) (*dnd5ev1alpha1.CreateDraftResponse, error) {
	if req.PlayerId == "" {
		return nil, status.Error(codes.InvalidArgument, "player_id is required")
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
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &dnd5ev1alpha1.CreateDraftResponse{
		Draft: convertEntityDraftToProto(output.Draft),
	}, nil
}

// GetDraft retrieves a character draft
func (h *Handler) GetDraft(ctx context.Context, req *dnd5ev1alpha1.GetDraftRequest) (*dnd5ev1alpha1.GetDraftResponse, error) {
	if req.DraftId == "" {
		return nil, status.Error(codes.InvalidArgument, "draft_id is required")
	}

	input := &character.GetDraftInput{
		DraftID: req.DraftId,
	}

	output, err := h.characterService.GetDraft(ctx, input)
	if err != nil {
		// TODO: Handle specific errors (NotFound -> codes.NotFound)
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &dnd5ev1alpha1.GetDraftResponse{
		Draft: convertEntityDraftToProto(output.Draft),
	}, nil
}

// ListDrafts lists character drafts
func (h *Handler) ListDrafts(ctx context.Context, req *dnd5ev1alpha1.ListDraftsRequest) (*dnd5ev1alpha1.ListDraftsResponse, error) {
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
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Convert drafts
	var protoDrafts []*dnd5ev1alpha1.CharacterDraft
	for _, draft := range output.Drafts {
		protoDrafts = append(protoDrafts, convertEntityDraftToProto(draft))
	}

	return &dnd5ev1alpha1.ListDraftsResponse{
		Drafts:        protoDrafts,
		NextPageToken: output.NextPageToken,
	}, nil
}

// DeleteDraft deletes a character draft
func (h *Handler) DeleteDraft(ctx context.Context, req *dnd5ev1alpha1.DeleteDraftRequest) (*dnd5ev1alpha1.DeleteDraftResponse, error) {
	if req.DraftId == "" {
		return nil, status.Error(codes.InvalidArgument, "draft_id is required")
	}

	input := &character.DeleteDraftInput{
		DraftID: req.DraftId,
	}

	output, err := h.characterService.DeleteDraft(ctx, input)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &dnd5ev1alpha1.DeleteDraftResponse{
		Message: output.Message,
	}, nil
}

// UpdateName updates the name of a character draft
func (h *Handler) UpdateName(ctx context.Context, req *dnd5ev1alpha1.UpdateNameRequest) (*dnd5ev1alpha1.UpdateNameResponse, error) {
	if req.DraftId == "" {
		return nil, status.Error(codes.InvalidArgument, "draft_id is required")
	}
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	input := &character.UpdateNameInput{
		DraftID: req.DraftId,
		Name:    req.Name,
	}

	output, err := h.characterService.UpdateName(ctx, input)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &dnd5ev1alpha1.UpdateNameResponse{
		Draft:    convertEntityDraftToProto(output.Draft),
		Warnings: convertWarningsToProto(output.Warnings),
	}, nil
}

// UpdateRace updates the race of a character draft
func (h *Handler) UpdateRace(ctx context.Context, req *dnd5ev1alpha1.UpdateRaceRequest) (*dnd5ev1alpha1.UpdateRaceResponse, error) {
	if req.DraftId == "" {
		return nil, status.Error(codes.InvalidArgument, "draft_id is required")
	}
	if req.Race == dnd5ev1alpha1.Race_RACE_UNSPECIFIED {
		return nil, status.Error(codes.InvalidArgument, "race is required")
	}

	input := &character.UpdateRaceInput{
		DraftID:   req.DraftId,
		RaceID:    mapProtoRaceToConstant(req.Race),
		SubraceID: mapProtoSubraceToConstant(req.Subrace),
	}

	output, err := h.characterService.UpdateRace(ctx, input)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &dnd5ev1alpha1.UpdateRaceResponse{
		Draft:    convertEntityDraftToProto(output.Draft),
		Warnings: convertWarningsToProto(output.Warnings),
	}, nil
}

// UpdateClass updates the class of a character draft
func (h *Handler) UpdateClass(ctx context.Context, req *dnd5ev1alpha1.UpdateClassRequest) (*dnd5ev1alpha1.UpdateClassResponse, error) {
	if req.DraftId == "" {
		return nil, status.Error(codes.InvalidArgument, "draft_id is required")
	}
	if req.Class == dnd5ev1alpha1.Class_CLASS_UNSPECIFIED {
		return nil, status.Error(codes.InvalidArgument, "class is required")
	}

	input := &character.UpdateClassInput{
		DraftID: req.DraftId,
		ClassID: mapProtoClassToConstant(req.Class),
	}

	output, err := h.characterService.UpdateClass(ctx, input)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &dnd5ev1alpha1.UpdateClassResponse{
		Draft:    convertEntityDraftToProto(output.Draft),
		Warnings: convertWarningsToProto(output.Warnings),
	}, nil
}

// UpdateBackground updates the background of a character draft
func (h *Handler) UpdateBackground(ctx context.Context, req *dnd5ev1alpha1.UpdateBackgroundRequest) (*dnd5ev1alpha1.UpdateBackgroundResponse, error) {
	if req.DraftId == "" {
		return nil, status.Error(codes.InvalidArgument, "draft_id is required")
	}
	if req.Background == dnd5ev1alpha1.Background_BACKGROUND_UNSPECIFIED {
		return nil, status.Error(codes.InvalidArgument, "background is required")
	}

	input := &character.UpdateBackgroundInput{
		DraftID:      req.DraftId,
		BackgroundID: mapProtoBackgroundToConstant(req.Background),
	}

	output, err := h.characterService.UpdateBackground(ctx, input)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &dnd5ev1alpha1.UpdateBackgroundResponse{
		Draft:    convertEntityDraftToProto(output.Draft),
		Warnings: convertWarningsToProto(output.Warnings),
	}, nil
}

// UpdateAbilityScores updates the ability scores of a character draft
func (h *Handler) UpdateAbilityScores(ctx context.Context, req *dnd5ev1alpha1.UpdateAbilityScoresRequest) (*dnd5ev1alpha1.UpdateAbilityScoresResponse, error) {
	if req.DraftId == "" {
		return nil, status.Error(codes.InvalidArgument, "draft_id is required")
	}
	if req.AbilityScores == nil {
		return nil, status.Error(codes.InvalidArgument, "ability_scores is required")
	}

	input := &character.UpdateAbilityScoresInput{
		DraftID: req.DraftId,
		AbilityScores: entities.AbilityScores{
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
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &dnd5ev1alpha1.UpdateAbilityScoresResponse{
		Draft:    convertEntityDraftToProto(output.Draft),
		Warnings: convertWarningsToProto(output.Warnings),
	}, nil
}

// UpdateSkills updates the skills of a character draft
func (h *Handler) UpdateSkills(ctx context.Context, req *dnd5ev1alpha1.UpdateSkillsRequest) (*dnd5ev1alpha1.UpdateSkillsResponse, error) {
	if req.DraftId == "" {
		return nil, status.Error(codes.InvalidArgument, "draft_id is required")
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
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &dnd5ev1alpha1.UpdateSkillsResponse{
		Draft:    convertEntityDraftToProto(output.Draft),
		Warnings: convertWarningsToProto(output.Warnings),
	}, nil
}

// ValidateDraft validates a character draft
func (h *Handler) ValidateDraft(ctx context.Context, req *dnd5ev1alpha1.ValidateDraftRequest) (*dnd5ev1alpha1.ValidateDraftResponse, error) {
	if req.DraftId == "" {
		return nil, status.Error(codes.InvalidArgument, "draft_id is required")
	}

	input := &character.ValidateDraftInput{
		DraftID: req.DraftId,
	}

	output, err := h.characterService.ValidateDraft(ctx, input)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &dnd5ev1alpha1.ValidateDraftResponse{
		IsValid:  output.IsValid,
		Errors:   convertErrorsToProto(output.Errors),
		Warnings: convertWarningsToProto(output.Warnings),
	}, nil
}

// FinalizeDraft finalizes a character draft into a complete character
func (h *Handler) FinalizeDraft(ctx context.Context, req *dnd5ev1alpha1.FinalizeDraftRequest) (*dnd5ev1alpha1.FinalizeDraftResponse, error) {
	if req.DraftId == "" {
		return nil, status.Error(codes.InvalidArgument, "draft_id is required")
	}

	input := &character.FinalizeDraftInput{
		DraftID: req.DraftId,
	}

	output, err := h.characterService.FinalizeDraft(ctx, input)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &dnd5ev1alpha1.FinalizeDraftResponse{
		Character:    convertCharacterToProto(output.Character),
		DraftDeleted: output.DraftDeleted,
	}, nil
}

// GetCharacter retrieves a character
func (h *Handler) GetCharacter(ctx context.Context, req *dnd5ev1alpha1.GetCharacterRequest) (*dnd5ev1alpha1.GetCharacterResponse, error) {
	if req.CharacterId == "" {
		return nil, status.Error(codes.InvalidArgument, "character_id is required")
	}

	input := &character.GetCharacterInput{
		CharacterID: req.CharacterId,
	}

	output, err := h.characterService.GetCharacter(ctx, input)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &dnd5ev1alpha1.GetCharacterResponse{
		Character: convertCharacterToProto(output.Character),
	}, nil
}

// ListCharacters lists characters
func (h *Handler) ListCharacters(ctx context.Context, req *dnd5ev1alpha1.ListCharactersRequest) (*dnd5ev1alpha1.ListCharactersResponse, error) {
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
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Convert characters to proto
	var protoCharacters []*dnd5ev1alpha1.Character
	for _, char := range output.Characters {
		protoCharacters = append(protoCharacters, convertCharacterToProto(char))
	}

	return &dnd5ev1alpha1.ListCharactersResponse{
		Characters:    protoCharacters,
		NextPageToken: output.NextPageToken,
		TotalSize:     output.TotalSize,
	}, nil
}

// DeleteCharacter deletes a character
func (h *Handler) DeleteCharacter(ctx context.Context, req *dnd5ev1alpha1.DeleteCharacterRequest) (*dnd5ev1alpha1.DeleteCharacterResponse, error) {
	if req.CharacterId == "" {
		return nil, status.Error(codes.InvalidArgument, "character_id is required")
	}

	input := &character.DeleteCharacterInput{
		CharacterID: req.CharacterId,
	}

	_, err := h.characterService.DeleteCharacter(ctx, input)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &dnd5ev1alpha1.DeleteCharacterResponse{
		Message: "Character deleted successfully",
	}, nil
}

// Converter functions

func convertProtoDraftToEntity(proto *dnd5ev1alpha1.CharacterDraft) *entities.CharacterDraft {
	if proto == nil {
		return nil
	}

	draft := &entities.CharacterDraft{
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
		draft.AbilityScores = &entities.AbilityScores{
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
		draft.Progress = entities.CreationProgress{
			HasName:              proto.Progress.HasName,
			HasRace:              proto.Progress.HasRace,
			HasClass:             proto.Progress.HasClass,
			HasBackground:        proto.Progress.HasBackground,
			HasAbilityScores:     proto.Progress.HasAbilityScores,
			HasSkills:            proto.Progress.HasSkills,
			HasLanguages:         proto.Progress.HasLanguages,
			CompletionPercentage: proto.Progress.CompletionPercentage,
			CurrentStep:          mapProtoCreationStepToConstant(proto.Progress.CurrentStep),
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

func convertEntityDraftToProto(entity *entities.CharacterDraft) *dnd5ev1alpha1.CharacterDraft {
	if entity == nil {
		return nil
	}

	proto := &dnd5ev1alpha1.CharacterDraft{
		Id:        entity.ID,
		PlayerId:  entity.PlayerID,
		SessionId: entity.SessionID,
		Name:      entity.Name,
		Progress: &dnd5ev1alpha1.CreationProgress{
			HasName:              entity.Progress.HasName,
			HasRace:              entity.Progress.HasRace,
			HasClass:             entity.Progress.HasClass,
			HasBackground:        entity.Progress.HasBackground,
			HasAbilityScores:     entity.Progress.HasAbilityScores,
			HasSkills:            entity.Progress.HasSkills,
			HasLanguages:         entity.Progress.HasLanguages,
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
		return entities.RaceHuman
	case dnd5ev1alpha1.Race_RACE_DWARF:
		return entities.RaceDwarf
	case dnd5ev1alpha1.Race_RACE_ELF:
		return entities.RaceElf
	case dnd5ev1alpha1.Race_RACE_HALFLING:
		return entities.RaceHalfling
	case dnd5ev1alpha1.Race_RACE_DRAGONBORN:
		return entities.RaceDragonborn
	case dnd5ev1alpha1.Race_RACE_GNOME:
		return entities.RaceGnome
	case dnd5ev1alpha1.Race_RACE_HALF_ELF:
		return entities.RaceHalfElf
	case dnd5ev1alpha1.Race_RACE_HALF_ORC:
		return entities.RaceHalfOrc
	case dnd5ev1alpha1.Race_RACE_TIEFLING:
		return entities.RaceTiefling
	default:
		return ""
	}
}

func mapProtoSubraceToConstant(subrace dnd5ev1alpha1.Subrace) string {
	switch subrace {
	case dnd5ev1alpha1.Subrace_SUBRACE_HIGH_ELF:
		return entities.SubraceHighElf
	case dnd5ev1alpha1.Subrace_SUBRACE_WOOD_ELF:
		return entities.SubraceWoodElf
	case dnd5ev1alpha1.Subrace_SUBRACE_DARK_ELF:
		return entities.SubraceDarkElf
	case dnd5ev1alpha1.Subrace_SUBRACE_HILL_DWARF:
		return entities.SubraceHillDwarf
	case dnd5ev1alpha1.Subrace_SUBRACE_MOUNTAIN_DWARF:
		return entities.SubraceMountainDwarf
	case dnd5ev1alpha1.Subrace_SUBRACE_LIGHTFOOT_HALFLING:
		return entities.SubraceLightfootHalfling
	case dnd5ev1alpha1.Subrace_SUBRACE_STOUT_HALFLING:
		return entities.SubraceStoutHalfling
	case dnd5ev1alpha1.Subrace_SUBRACE_FOREST_GNOME:
		return entities.SubraceForestGnome
	case dnd5ev1alpha1.Subrace_SUBRACE_ROCK_GNOME:
		return entities.SubraceRockGnome
	default:
		return ""
	}
}

func mapProtoClassToConstant(class dnd5ev1alpha1.Class) string {
	switch class {
	case dnd5ev1alpha1.Class_CLASS_BARBARIAN:
		return entities.ClassBarbarian
	case dnd5ev1alpha1.Class_CLASS_BARD:
		return entities.ClassBard
	case dnd5ev1alpha1.Class_CLASS_CLERIC:
		return entities.ClassCleric
	case dnd5ev1alpha1.Class_CLASS_DRUID:
		return entities.ClassDruid
	case dnd5ev1alpha1.Class_CLASS_FIGHTER:
		return entities.ClassFighter
	case dnd5ev1alpha1.Class_CLASS_MONK:
		return entities.ClassMonk
	case dnd5ev1alpha1.Class_CLASS_PALADIN:
		return entities.ClassPaladin
	case dnd5ev1alpha1.Class_CLASS_RANGER:
		return entities.ClassRanger
	case dnd5ev1alpha1.Class_CLASS_ROGUE:
		return entities.ClassRogue
	case dnd5ev1alpha1.Class_CLASS_SORCERER:
		return entities.ClassSorcerer
	case dnd5ev1alpha1.Class_CLASS_WARLOCK:
		return entities.ClassWarlock
	case dnd5ev1alpha1.Class_CLASS_WIZARD:
		return entities.ClassWizard
	default:
		return ""
	}
}

func mapProtoBackgroundToConstant(bg dnd5ev1alpha1.Background) string {
	switch bg {
	case dnd5ev1alpha1.Background_BACKGROUND_ACOLYTE:
		return entities.BackgroundAcolyte
	case dnd5ev1alpha1.Background_BACKGROUND_CHARLATAN:
		return entities.BackgroundCharlatan
	case dnd5ev1alpha1.Background_BACKGROUND_CRIMINAL:
		return entities.BackgroundCriminal
	case dnd5ev1alpha1.Background_BACKGROUND_ENTERTAINER:
		return entities.BackgroundEntertainer
	case dnd5ev1alpha1.Background_BACKGROUND_FOLK_HERO:
		return entities.BackgroundFolkHero
	case dnd5ev1alpha1.Background_BACKGROUND_GUILD_ARTISAN:
		return entities.BackgroundGuildArtisan
	case dnd5ev1alpha1.Background_BACKGROUND_HERMIT:
		return entities.BackgroundHermit
	case dnd5ev1alpha1.Background_BACKGROUND_NOBLE:
		return entities.BackgroundNoble
	case dnd5ev1alpha1.Background_BACKGROUND_OUTLANDER:
		return entities.BackgroundOutlander
	case dnd5ev1alpha1.Background_BACKGROUND_SAGE:
		return entities.BackgroundSage
	case dnd5ev1alpha1.Background_BACKGROUND_SAILOR:
		return entities.BackgroundSailor
	case dnd5ev1alpha1.Background_BACKGROUND_SOLDIER:
		return entities.BackgroundSoldier
	case dnd5ev1alpha1.Background_BACKGROUND_URCHIN:
		return entities.BackgroundUrchin
	default:
		return ""
	}
}

func mapProtoAlignmentToConstant(alignment dnd5ev1alpha1.Alignment) string {
	switch alignment {
	case dnd5ev1alpha1.Alignment_ALIGNMENT_LAWFUL_GOOD:
		return entities.AlignmentLawfulGood
	case dnd5ev1alpha1.Alignment_ALIGNMENT_NEUTRAL_GOOD:
		return entities.AlignmentNeutralGood
	case dnd5ev1alpha1.Alignment_ALIGNMENT_CHAOTIC_GOOD:
		return entities.AlignmentChaoticGood
	case dnd5ev1alpha1.Alignment_ALIGNMENT_LAWFUL_NEUTRAL:
		return entities.AlignmentLawfulNeutral
	case dnd5ev1alpha1.Alignment_ALIGNMENT_TRUE_NEUTRAL:
		return entities.AlignmentTrueNeutral
	case dnd5ev1alpha1.Alignment_ALIGNMENT_CHAOTIC_NEUTRAL:
		return entities.AlignmentChaoticNeutral
	case dnd5ev1alpha1.Alignment_ALIGNMENT_LAWFUL_EVIL:
		return entities.AlignmentLawfulEvil
	case dnd5ev1alpha1.Alignment_ALIGNMENT_NEUTRAL_EVIL:
		return entities.AlignmentNeutralEvil
	case dnd5ev1alpha1.Alignment_ALIGNMENT_CHAOTIC_EVIL:
		return entities.AlignmentChaoticEvil
	default:
		return ""
	}
}

func mapProtoSkillToConstant(skill dnd5ev1alpha1.Skill) string {
	switch skill {
	case dnd5ev1alpha1.Skill_SKILL_ACROBATICS:
		return entities.SkillAcrobatics
	case dnd5ev1alpha1.Skill_SKILL_ANIMAL_HANDLING:
		return entities.SkillAnimalHandling
	case dnd5ev1alpha1.Skill_SKILL_ARCANA:
		return entities.SkillArcana
	case dnd5ev1alpha1.Skill_SKILL_ATHLETICS:
		return entities.SkillAthletics
	case dnd5ev1alpha1.Skill_SKILL_DECEPTION:
		return entities.SkillDeception
	case dnd5ev1alpha1.Skill_SKILL_HISTORY:
		return entities.SkillHistory
	case dnd5ev1alpha1.Skill_SKILL_INSIGHT:
		return entities.SkillInsight
	case dnd5ev1alpha1.Skill_SKILL_INTIMIDATION:
		return entities.SkillIntimidation
	case dnd5ev1alpha1.Skill_SKILL_INVESTIGATION:
		return entities.SkillInvestigation
	case dnd5ev1alpha1.Skill_SKILL_MEDICINE:
		return entities.SkillMedicine
	case dnd5ev1alpha1.Skill_SKILL_NATURE:
		return entities.SkillNature
	case dnd5ev1alpha1.Skill_SKILL_PERCEPTION:
		return entities.SkillPerception
	case dnd5ev1alpha1.Skill_SKILL_PERFORMANCE:
		return entities.SkillPerformance
	case dnd5ev1alpha1.Skill_SKILL_PERSUASION:
		return entities.SkillPersuasion
	case dnd5ev1alpha1.Skill_SKILL_RELIGION:
		return entities.SkillReligion
	case dnd5ev1alpha1.Skill_SKILL_SLEIGHT_OF_HAND:
		return entities.SkillSleightOfHand
	case dnd5ev1alpha1.Skill_SKILL_STEALTH:
		return entities.SkillStealth
	case dnd5ev1alpha1.Skill_SKILL_SURVIVAL:
		return entities.SkillSurvival
	default:
		return ""
	}
}

func mapProtoLanguageToConstant(lang dnd5ev1alpha1.Language) string {
	switch lang {
	case dnd5ev1alpha1.Language_LANGUAGE_COMMON:
		return entities.LanguageCommon
	case dnd5ev1alpha1.Language_LANGUAGE_DWARVISH:
		return entities.LanguageDwarvish
	case dnd5ev1alpha1.Language_LANGUAGE_ELVISH:
		return entities.LanguageElvish
	case dnd5ev1alpha1.Language_LANGUAGE_GIANT:
		return entities.LanguageGiant
	case dnd5ev1alpha1.Language_LANGUAGE_GNOMISH:
		return entities.LanguageGnomish
	case dnd5ev1alpha1.Language_LANGUAGE_GOBLIN:
		return entities.LanguageGoblin
	case dnd5ev1alpha1.Language_LANGUAGE_HALFLING:
		return entities.LanguageHalfling
	case dnd5ev1alpha1.Language_LANGUAGE_ORC:
		return entities.LanguageOrc
	case dnd5ev1alpha1.Language_LANGUAGE_ABYSSAL:
		return entities.LanguageAbyssal
	case dnd5ev1alpha1.Language_LANGUAGE_CELESTIAL:
		return entities.LanguageCelestial
	case dnd5ev1alpha1.Language_LANGUAGE_DRACONIC:
		return entities.LanguageDraconic
	case dnd5ev1alpha1.Language_LANGUAGE_DEEP_SPEECH:
		return entities.LanguageDeepSpeech
	case dnd5ev1alpha1.Language_LANGUAGE_INFERNAL:
		return entities.LanguageInfernal
	case dnd5ev1alpha1.Language_LANGUAGE_PRIMORDIAL:
		return entities.LanguagePrimordial
	case dnd5ev1alpha1.Language_LANGUAGE_SYLVAN:
		return entities.LanguageSylvan
	case dnd5ev1alpha1.Language_LANGUAGE_UNDERCOMMON:
		return entities.LanguageUndercommon
	default:
		return ""
	}
}

func mapProtoCreationStepToConstant(step dnd5ev1alpha1.CreationStep) string {
	switch step {
	case dnd5ev1alpha1.CreationStep_CREATION_STEP_NAME:
		return entities.CreationStepName
	case dnd5ev1alpha1.CreationStep_CREATION_STEP_RACE:
		return entities.CreationStepRace
	case dnd5ev1alpha1.CreationStep_CREATION_STEP_CLASS:
		return entities.CreationStepClass
	case dnd5ev1alpha1.CreationStep_CREATION_STEP_BACKGROUND:
		return entities.CreationStepBackground
	case dnd5ev1alpha1.CreationStep_CREATION_STEP_ABILITY_SCORES:
		return entities.CreationStepAbilityScores
	case dnd5ev1alpha1.CreationStep_CREATION_STEP_SKILLS:
		return entities.CreationStepSkills
	case dnd5ev1alpha1.CreationStep_CREATION_STEP_LANGUAGES:
		return entities.CreationStepLanguages
	case dnd5ev1alpha1.CreationStep_CREATION_STEP_REVIEW:
		return entities.CreationStepReview
	default:
		return ""
	}
}

// Mapper functions - Constants to Proto

func mapConstantToProtoRace(constant string) dnd5ev1alpha1.Race {
	switch constant {
	case entities.RaceHuman:
		return dnd5ev1alpha1.Race_RACE_HUMAN
	case entities.RaceDwarf:
		return dnd5ev1alpha1.Race_RACE_DWARF
	case entities.RaceElf:
		return dnd5ev1alpha1.Race_RACE_ELF
	case entities.RaceHalfling:
		return dnd5ev1alpha1.Race_RACE_HALFLING
	case entities.RaceDragonborn:
		return dnd5ev1alpha1.Race_RACE_DRAGONBORN
	case entities.RaceGnome:
		return dnd5ev1alpha1.Race_RACE_GNOME
	case entities.RaceHalfElf:
		return dnd5ev1alpha1.Race_RACE_HALF_ELF
	case entities.RaceHalfOrc:
		return dnd5ev1alpha1.Race_RACE_HALF_ORC
	case entities.RaceTiefling:
		return dnd5ev1alpha1.Race_RACE_TIEFLING
	default:
		return dnd5ev1alpha1.Race_RACE_UNSPECIFIED
	}
}

func mapConstantToProtoSubrace(constant string) dnd5ev1alpha1.Subrace {
	switch constant {
	case entities.SubraceHighElf:
		return dnd5ev1alpha1.Subrace_SUBRACE_HIGH_ELF
	case entities.SubraceWoodElf:
		return dnd5ev1alpha1.Subrace_SUBRACE_WOOD_ELF
	case entities.SubraceDarkElf:
		return dnd5ev1alpha1.Subrace_SUBRACE_DARK_ELF
	case entities.SubraceHillDwarf:
		return dnd5ev1alpha1.Subrace_SUBRACE_HILL_DWARF
	case entities.SubraceMountainDwarf:
		return dnd5ev1alpha1.Subrace_SUBRACE_MOUNTAIN_DWARF
	case entities.SubraceLightfootHalfling:
		return dnd5ev1alpha1.Subrace_SUBRACE_LIGHTFOOT_HALFLING
	case entities.SubraceStoutHalfling:
		return dnd5ev1alpha1.Subrace_SUBRACE_STOUT_HALFLING
	case entities.SubraceForestGnome:
		return dnd5ev1alpha1.Subrace_SUBRACE_FOREST_GNOME
	case entities.SubraceRockGnome:
		return dnd5ev1alpha1.Subrace_SUBRACE_ROCK_GNOME
	default:
		return dnd5ev1alpha1.Subrace_SUBRACE_UNSPECIFIED
	}
}

func mapConstantToProtoClass(constant string) dnd5ev1alpha1.Class {
	switch constant {
	case entities.ClassBarbarian:
		return dnd5ev1alpha1.Class_CLASS_BARBARIAN
	case entities.ClassBard:
		return dnd5ev1alpha1.Class_CLASS_BARD
	case entities.ClassCleric:
		return dnd5ev1alpha1.Class_CLASS_CLERIC
	case entities.ClassDruid:
		return dnd5ev1alpha1.Class_CLASS_DRUID
	case entities.ClassFighter:
		return dnd5ev1alpha1.Class_CLASS_FIGHTER
	case entities.ClassMonk:
		return dnd5ev1alpha1.Class_CLASS_MONK
	case entities.ClassPaladin:
		return dnd5ev1alpha1.Class_CLASS_PALADIN
	case entities.ClassRanger:
		return dnd5ev1alpha1.Class_CLASS_RANGER
	case entities.ClassRogue:
		return dnd5ev1alpha1.Class_CLASS_ROGUE
	case entities.ClassSorcerer:
		return dnd5ev1alpha1.Class_CLASS_SORCERER
	case entities.ClassWarlock:
		return dnd5ev1alpha1.Class_CLASS_WARLOCK
	case entities.ClassWizard:
		return dnd5ev1alpha1.Class_CLASS_WIZARD
	default:
		return dnd5ev1alpha1.Class_CLASS_UNSPECIFIED
	}
}

func mapConstantToProtoBackground(constant string) dnd5ev1alpha1.Background {
	switch constant {
	case entities.BackgroundAcolyte:
		return dnd5ev1alpha1.Background_BACKGROUND_ACOLYTE
	case entities.BackgroundCharlatan:
		return dnd5ev1alpha1.Background_BACKGROUND_CHARLATAN
	case entities.BackgroundCriminal:
		return dnd5ev1alpha1.Background_BACKGROUND_CRIMINAL
	case entities.BackgroundEntertainer:
		return dnd5ev1alpha1.Background_BACKGROUND_ENTERTAINER
	case entities.BackgroundFolkHero:
		return dnd5ev1alpha1.Background_BACKGROUND_FOLK_HERO
	case entities.BackgroundGuildArtisan:
		return dnd5ev1alpha1.Background_BACKGROUND_GUILD_ARTISAN
	case entities.BackgroundHermit:
		return dnd5ev1alpha1.Background_BACKGROUND_HERMIT
	case entities.BackgroundNoble:
		return dnd5ev1alpha1.Background_BACKGROUND_NOBLE
	case entities.BackgroundOutlander:
		return dnd5ev1alpha1.Background_BACKGROUND_OUTLANDER
	case entities.BackgroundSage:
		return dnd5ev1alpha1.Background_BACKGROUND_SAGE
	case entities.BackgroundSailor:
		return dnd5ev1alpha1.Background_BACKGROUND_SAILOR
	case entities.BackgroundSoldier:
		return dnd5ev1alpha1.Background_BACKGROUND_SOLDIER
	case entities.BackgroundUrchin:
		return dnd5ev1alpha1.Background_BACKGROUND_URCHIN
	default:
		return dnd5ev1alpha1.Background_BACKGROUND_UNSPECIFIED
	}
}

func mapConstantToProtoAlignment(constant string) dnd5ev1alpha1.Alignment {
	switch constant {
	case entities.AlignmentLawfulGood:
		return dnd5ev1alpha1.Alignment_ALIGNMENT_LAWFUL_GOOD
	case entities.AlignmentNeutralGood:
		return dnd5ev1alpha1.Alignment_ALIGNMENT_NEUTRAL_GOOD
	case entities.AlignmentChaoticGood:
		return dnd5ev1alpha1.Alignment_ALIGNMENT_CHAOTIC_GOOD
	case entities.AlignmentLawfulNeutral:
		return dnd5ev1alpha1.Alignment_ALIGNMENT_LAWFUL_NEUTRAL
	case entities.AlignmentTrueNeutral:
		return dnd5ev1alpha1.Alignment_ALIGNMENT_TRUE_NEUTRAL
	case entities.AlignmentChaoticNeutral:
		return dnd5ev1alpha1.Alignment_ALIGNMENT_CHAOTIC_NEUTRAL
	case entities.AlignmentLawfulEvil:
		return dnd5ev1alpha1.Alignment_ALIGNMENT_LAWFUL_EVIL
	case entities.AlignmentNeutralEvil:
		return dnd5ev1alpha1.Alignment_ALIGNMENT_NEUTRAL_EVIL
	case entities.AlignmentChaoticEvil:
		return dnd5ev1alpha1.Alignment_ALIGNMENT_CHAOTIC_EVIL
	default:
		return dnd5ev1alpha1.Alignment_ALIGNMENT_UNSPECIFIED
	}
}

func mapConstantToProtoSkill(constant string) dnd5ev1alpha1.Skill {
	switch constant {
	case entities.SkillAcrobatics:
		return dnd5ev1alpha1.Skill_SKILL_ACROBATICS
	case entities.SkillAnimalHandling:
		return dnd5ev1alpha1.Skill_SKILL_ANIMAL_HANDLING
	case entities.SkillArcana:
		return dnd5ev1alpha1.Skill_SKILL_ARCANA
	case entities.SkillAthletics:
		return dnd5ev1alpha1.Skill_SKILL_ATHLETICS
	case entities.SkillDeception:
		return dnd5ev1alpha1.Skill_SKILL_DECEPTION
	case entities.SkillHistory:
		return dnd5ev1alpha1.Skill_SKILL_HISTORY
	case entities.SkillInsight:
		return dnd5ev1alpha1.Skill_SKILL_INSIGHT
	case entities.SkillIntimidation:
		return dnd5ev1alpha1.Skill_SKILL_INTIMIDATION
	case entities.SkillInvestigation:
		return dnd5ev1alpha1.Skill_SKILL_INVESTIGATION
	case entities.SkillMedicine:
		return dnd5ev1alpha1.Skill_SKILL_MEDICINE
	case entities.SkillNature:
		return dnd5ev1alpha1.Skill_SKILL_NATURE
	case entities.SkillPerception:
		return dnd5ev1alpha1.Skill_SKILL_PERCEPTION
	case entities.SkillPerformance:
		return dnd5ev1alpha1.Skill_SKILL_PERFORMANCE
	case entities.SkillPersuasion:
		return dnd5ev1alpha1.Skill_SKILL_PERSUASION
	case entities.SkillReligion:
		return dnd5ev1alpha1.Skill_SKILL_RELIGION
	case entities.SkillSleightOfHand:
		return dnd5ev1alpha1.Skill_SKILL_SLEIGHT_OF_HAND
	case entities.SkillStealth:
		return dnd5ev1alpha1.Skill_SKILL_STEALTH
	case entities.SkillSurvival:
		return dnd5ev1alpha1.Skill_SKILL_SURVIVAL
	default:
		return dnd5ev1alpha1.Skill_SKILL_UNSPECIFIED
	}
}

func mapConstantToProtoLanguage(constant string) dnd5ev1alpha1.Language {
	switch constant {
	case entities.LanguageCommon:
		return dnd5ev1alpha1.Language_LANGUAGE_COMMON
	case entities.LanguageDwarvish:
		return dnd5ev1alpha1.Language_LANGUAGE_DWARVISH
	case entities.LanguageElvish:
		return dnd5ev1alpha1.Language_LANGUAGE_ELVISH
	case entities.LanguageGiant:
		return dnd5ev1alpha1.Language_LANGUAGE_GIANT
	case entities.LanguageGnomish:
		return dnd5ev1alpha1.Language_LANGUAGE_GNOMISH
	case entities.LanguageGoblin:
		return dnd5ev1alpha1.Language_LANGUAGE_GOBLIN
	case entities.LanguageHalfling:
		return dnd5ev1alpha1.Language_LANGUAGE_HALFLING
	case entities.LanguageOrc:
		return dnd5ev1alpha1.Language_LANGUAGE_ORC
	case entities.LanguageAbyssal:
		return dnd5ev1alpha1.Language_LANGUAGE_ABYSSAL
	case entities.LanguageCelestial:
		return dnd5ev1alpha1.Language_LANGUAGE_CELESTIAL
	case entities.LanguageDraconic:
		return dnd5ev1alpha1.Language_LANGUAGE_DRACONIC
	case entities.LanguageDeepSpeech:
		return dnd5ev1alpha1.Language_LANGUAGE_DEEP_SPEECH
	case entities.LanguageInfernal:
		return dnd5ev1alpha1.Language_LANGUAGE_INFERNAL
	case entities.LanguagePrimordial:
		return dnd5ev1alpha1.Language_LANGUAGE_PRIMORDIAL
	case entities.LanguageSylvan:
		return dnd5ev1alpha1.Language_LANGUAGE_SYLVAN
	case entities.LanguageUndercommon:
		return dnd5ev1alpha1.Language_LANGUAGE_UNDERCOMMON
	default:
		return dnd5ev1alpha1.Language_LANGUAGE_UNSPECIFIED
	}
}

func mapConstantToProtoCreationStep(constant string) dnd5ev1alpha1.CreationStep {
	switch constant {
	case entities.CreationStepName:
		return dnd5ev1alpha1.CreationStep_CREATION_STEP_NAME
	case entities.CreationStepRace:
		return dnd5ev1alpha1.CreationStep_CREATION_STEP_RACE
	case entities.CreationStepClass:
		return dnd5ev1alpha1.CreationStep_CREATION_STEP_CLASS
	case entities.CreationStepBackground:
		return dnd5ev1alpha1.CreationStep_CREATION_STEP_BACKGROUND
	case entities.CreationStepAbilityScores:
		return dnd5ev1alpha1.CreationStep_CREATION_STEP_ABILITY_SCORES
	case entities.CreationStepSkills:
		return dnd5ev1alpha1.CreationStep_CREATION_STEP_SKILLS
	case entities.CreationStepLanguages:
		return dnd5ev1alpha1.CreationStep_CREATION_STEP_LANGUAGES
	case entities.CreationStepReview:
		return dnd5ev1alpha1.CreationStep_CREATION_STEP_REVIEW
	default:
		return dnd5ev1alpha1.CreationStep_CREATION_STEP_UNSPECIFIED
	}
}

// Helper converters

func convertWarningsToProto(warnings []character.ValidationWarning) []*dnd5ev1alpha1.ValidationWarning {
	var protoWarnings []*dnd5ev1alpha1.ValidationWarning
	for _, w := range warnings {
		protoWarnings = append(protoWarnings, &dnd5ev1alpha1.ValidationWarning{
			Field:   w.Field,
			Message: w.Message,
			Type:    w.Type,
		})
	}
	return protoWarnings
}

func convertErrorsToProto(errors []character.ValidationError) []*dnd5ev1alpha1.ValidationError {
	var protoErrors []*dnd5ev1alpha1.ValidationError
	for _, e := range errors {
		protoErrors = append(protoErrors, &dnd5ev1alpha1.ValidationError{
			Field:   e.Field,
			Message: e.Message,
			Code:    e.Type,
		})
	}
	return protoErrors
}

func convertCharacterToProto(char *entities.Character) *dnd5ev1alpha1.Character {
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
