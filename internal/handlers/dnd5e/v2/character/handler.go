// Package character implements dnd5e.api.v1alpha2.character.CharacterService — the
// out-of-encounter character sheet surface (rpg-api#680): EquipItem/UnequipItem
// for sheet editing between encounters. The in-encounter equivalent, when it
// exists, rides the encounter stream instead (see CharacterData's doc comment
// in dnd5e/api/v1alpha2/encounter/types.proto).
//
// Equip/UnequipItem call the SAME orchestrator method the v1alpha1
// character handler calls (internal/orchestrators/character's EquipItem/
// UnequipItem) — occupancy and slot-compatibility are enforced exactly once,
// in the toolkit, for both API surfaces. This handler is proto conversion
// only: no business logic, no rules, no display-field composition beyond
// Ref/enum translation.
package character

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	characterpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/character"
	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	"github.com/KirkDiggler/rpg-api/internal/apierr"
	encounterhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v2/encounter"
	orchcharacter "github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	"github.com/KirkDiggler/rpg-toolkit/events"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

// HandlerConfig configures a v1alpha2 character Handler.
type HandlerConfig struct {
	CharacterService orchcharacter.Service
}

// Validate ensures required dependencies are present.
func (c *HandlerConfig) Validate() error {
	if c.CharacterService == nil {
		return errors.New("HandlerConfig.CharacterService is required")
	}
	return nil
}

// Handler implements dnd5e.api.v1alpha2.character.CharacterServiceServer.
type Handler struct {
	characterpb.UnimplementedCharacterServiceServer
	characterService orchcharacter.Service
}

// New constructs a Handler. Returns error on missing required deps.
func New(cfg *HandlerConfig) (*Handler, error) {
	if cfg == nil {
		return nil, errors.New("HandlerConfig is required")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Handler{characterService: cfg.CharacterService}, nil
}

// EquipItem equips an inventory item into a slot, out of combat. The
// requested item is a Ref (not a bare id) per the wire contract — module/type
// are ignored here (item.id is the toolkit item id rpg-api already keys
// inventory by); validating module/type would require rpg-api to know what
// makes a Ref "a valid item ref", which is exactly the kind of rules
// knowledge this handler doesn't have.
func (h *Handler) EquipItem(
	ctx context.Context,
	req *characterpb.EquipItemRequest,
) (*characterpb.EquipItemResponse, error) {
	if req.GetCharacterId() == "" {
		return nil, apierr.ToGRPCError(apierr.InvalidArgument("character_id is required"))
	}
	if req.GetItem().GetId() == "" {
		return nil, apierr.ToGRPCError(apierr.InvalidArgument("item is required"))
	}
	if req.GetSlotKey() == "" {
		return nil, apierr.ToGRPCError(apierr.InvalidArgument("slot_key is required"))
	}

	_, err := h.characterService.EquipItem(ctx, &orchcharacter.EquipItemInput{
		CharacterID: req.GetCharacterId(),
		ItemID:      req.GetItem().GetId(),
		Slot:        tkcharacter.InventorySlot(req.GetSlotKey()),
	})
	if err != nil {
		return nil, apierr.ToGRPCError(err)
	}

	cd, err := h.recomputedCharacterData(ctx, req.GetCharacterId())
	if err != nil {
		return nil, err
	}
	return &characterpb.EquipItemResponse{Character: cd}, nil
}

// UnequipItem clears a slot, returning its occupant to inventory.
func (h *Handler) UnequipItem(
	ctx context.Context,
	req *characterpb.UnequipItemRequest,
) (*characterpb.UnequipItemResponse, error) {
	if req.GetCharacterId() == "" {
		return nil, apierr.ToGRPCError(apierr.InvalidArgument("character_id is required"))
	}
	if req.GetSlotKey() == "" {
		return nil, apierr.ToGRPCError(apierr.InvalidArgument("slot_key is required"))
	}

	_, err := h.characterService.UnequipItem(ctx, &orchcharacter.UnequipItemInput{
		CharacterID: req.GetCharacterId(),
		Slot:        tkcharacter.InventorySlot(req.GetSlotKey()),
	})
	if err != nil {
		return nil, apierr.ToGRPCError(err)
	}

	cd, err := h.recomputedCharacterData(ctx, req.GetCharacterId())
	if err != nil {
		return nil, err
	}
	return &characterpb.UnequipItemResponse{Character: cd}, nil
}

// recomputedCharacterData re-fetches the character after an equip/unequip
// and composes the post-change CharacterData via
// encounterhandler.BuildEquipmentCharacterData — the SAME composition the
// encounter snapshot path uses (v2/encounter/character_data.go), so the
// sheet this RPC returns and the HUD an active encounter shows never drift
// from two independent compositions (rpg-api#680).
//
// armor_class_detail.total is the ONLY AC total on this response: unlike
// the encounter snapshot, there is no surrounding Entity.armor_class to
// keep in sync (see CharacterData.armor_class_detail's doc comment in
// types.proto).
func (h *Handler) recomputedCharacterData(
	ctx context.Context,
	characterID string,
) (*encounterv2pb.CharacterData, error) {
	out, err := h.characterService.GetCharacter(ctx, &orchcharacter.GetCharacterInput{
		CharacterID: characterID,
	})
	if err != nil {
		return nil, apierr.ToGRPCError(err)
	}

	char, err := tkcharacter.LoadFromData(ctx, out.Character.Data, events.NewEventBus())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "load character: %v", err)
	}

	return encounterhandler.BuildEquipmentCharacterData(char.EquipmentView(ctx)), nil
}
