// Package v1alpha1 handles the grpc service interface
package v1alpha1

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/errors"
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
	return nil, status.Error(codes.Unimplemented, "not implemented")
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
	// For now, return minimal response to get started
	protoDraft := &dnd5ev1alpha1.CharacterDraft{
		Id:       output.Draft.ID,
		PlayerId: output.Draft.PlayerID,
		Name:     output.Draft.Name,
		// TODO: Convert other fields as we implement them
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
	return nil, status.Error(codes.Unimplemented, "not implemented")
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
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

// UpdateRace updates the race of a character draft
func (h *Handler) UpdateRace(
	ctx context.Context,
	req *dnd5ev1alpha1.UpdateRaceRequest,
) (*dnd5ev1alpha1.UpdateRaceResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
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
	return nil, status.Error(codes.Unimplemented, "not implemented")
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
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

// ListClasses lists available classes
func (h *Handler) ListClasses(
	ctx context.Context,
	req *dnd5ev1alpha1.ListClassesRequest,
) (*dnd5ev1alpha1.ListClassesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
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