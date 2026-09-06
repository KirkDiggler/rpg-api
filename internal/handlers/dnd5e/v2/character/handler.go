// Package character implements dnd5e.api.v1alpha2.character.CharacterService — the
// out-of-encounter character sheet surface (rpg-api#680): EquipItem/UnequipItem
// for sheet editing between encounters, and GetCharacterData to read the same
// view without changing it (rpg-project#249 §2, equipment row — the
// combat-turn session route mounts the equipment screen but the session stack
// carries no encounter snapshot to seed it from). The in-encounter equivalent
// of a write, when it exists, rides the encounter stream instead (see
// CharacterData's doc comment in dnd5e/api/v1alpha2/encounter/types.proto).
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

	characterpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/character"
	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	orchcharacter "github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
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
//
// Authenticated owner only: see verifyCallerOwnsCharacter.
func (h *Handler) EquipItem(
	ctx context.Context,
	req *characterpb.EquipItemRequest,
) (*characterpb.EquipItemResponse, error) {
	if req.GetCharacterId() == "" {
		return nil, apierr.ToGRPCError(apierr.InvalidArgument("character_id is required"))
	}
	if req.GetItem().GetId() == "" {
		return nil, apierr.ToGRPCError(apierr.InvalidArgument("item.id is required"))
	}
	if req.GetSlotKey() == "" {
		return nil, apierr.ToGRPCError(apierr.InvalidArgument("slot_key is required"))
	}

	if _, err := h.verifyCallerOwnsCharacter(ctx, req.GetCharacterId()); err != nil {
		return nil, err
	}

	out, err := h.characterService.EquipItem(ctx, &orchcharacter.EquipItemInput{
		CharacterID: req.GetCharacterId(),
		ItemID:      req.GetItem().GetId(),
		Slot:        tkcharacter.InventorySlot(req.GetSlotKey()),
	})
	if err != nil {
		return nil, characterRPCError(err)
	}
	if out == nil || out.Character == nil || out.Character.Data == nil || out.View == nil {
		return nil, characterRPCError(errors.New("equip item returned incomplete character post-state"))
	}

	cd, err := BuildCharacterData(out.View)
	if err != nil {
		return nil, characterRPCError(err)
	}
	return &characterpb.EquipItemResponse{Character: cd}, nil
}

// UnequipItem clears a slot, returning its occupant to inventory.
//
// Authenticated owner only: see verifyCallerOwnsCharacter.
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

	if _, err := h.verifyCallerOwnsCharacter(ctx, req.GetCharacterId()); err != nil {
		return nil, err
	}

	out, err := h.characterService.UnequipItem(ctx, &orchcharacter.UnequipItemInput{
		CharacterID: req.GetCharacterId(),
		Slot:        tkcharacter.InventorySlot(req.GetSlotKey()),
	})
	if err != nil {
		return nil, characterRPCError(err)
	}
	if out == nil || out.Character == nil || out.Character.Data == nil || out.View == nil {
		return nil, characterRPCError(errors.New("unequip item returned incomplete character post-state"))
	}

	cd, err := BuildCharacterData(out.View)
	if err != nil {
		return nil, characterRPCError(err)
	}
	return &characterpb.UnequipItemResponse{Character: cd}, nil
}

// GetCharacterData reads one character's current view without changing it —
// the read EquipItem/UnequipItem never needed, because they always compose a
// response after their own write. The equipment screen mounts on the session
// route to seed itself from this: the session stack carries no encounter
// snapshot the way the old route did (rpg-project#249 §2, equipment row).
//
// Ownership is gated by verifyCallerOwnsCharacter -- see its doc for why a
// wrong owner and a missing character must read identically on the wire.
func (h *Handler) GetCharacterData(
	ctx context.Context,
	req *characterpb.GetCharacterDataRequest,
) (*characterpb.GetCharacterDataResponse, error) {
	if req.GetCharacterId() == "" {
		return nil, apierr.ToGRPCError(apierr.InvalidArgument("character_id is required"))
	}

	data, err := h.verifyCallerOwnsCharacter(ctx, req.GetCharacterId())
	if err != nil {
		return nil, err
	}

	cd, err := h.characterDataFromEntity(ctx, data)
	if err != nil {
		return nil, err
	}
	return &characterpb.GetCharacterDataResponse{Character: cd}, nil
}

// verifyCallerOwnsCharacter is the ONE ownership gate this service checks
// through -- EquipItem, UnequipItem and GetCharacterData all call it before
// touching a character, per Kirk's ruling on rpg-api#814: "any player facing
// routes should extract the character id from the headers and verify the
// auth'd player owns the character." Returns the character's toolkit Data on
// success so a caller that also needs it (GetCharacterData) does not fetch
// twice.
//
// NOT_FOUND, NEVER PERMISSION_DENIED. Every one of these three RPCs either
// hands back the character's full sheet or mutates it; either way,
// PERMISSION_DENIED would itself confirm that SOME character exists at that
// id, which is exactly what "found but not yours" must not leak. A caller
// naming a character it does not control gets the same NOT_FOUND a caller
// naming one that was never created would -- the same shape the session
// handler's callerActingAs keeps for the same reason
// (internal/handlers/dnd5e/session/v1alpha1/handler.go).
func (h *Handler) verifyCallerOwnsCharacter(ctx context.Context, characterID string) (*tkcharacter.Data, error) {
	playerID := auth.GetPlayerID(ctx)
	if playerID == "" {
		return nil, apierr.ToGRPCError(apierr.Unauthenticated("player not authenticated"))
	}

	out, err := h.characterService.GetCharacter(ctx, &orchcharacter.GetCharacterInput{
		CharacterID: characterID,
	})
	if err != nil {
		if apierr.IsNotFound(err) {
			return nil, apierr.ToGRPCError(notFoundCharacter(characterID))
		}
		return nil, apierr.ToGRPCError(err)
	}
	if out == nil || out.Character == nil || out.Character.Data == nil || out.Character.Data.PlayerID != playerID {
		return nil, apierr.ToGRPCError(notFoundCharacter(characterID))
	}

	return out.Character.Data, nil
}

// notFoundCharacter is the ONE NOT_FOUND this gate ever returns, for BOTH
// refusal reasons -- a missing character and a foreign one. Copilot caught
// the leak this closes (rpg-api#815): the repository's own not-found error
// carries its own wording, and a caller comparing that text against this
// handler's own "character %q not found" for the foreign-owner case could
// tell the two refusals apart -- which is exactly the distinction NOT_FOUND
// exists here to erase (see verifyCallerOwnsCharacter's own doc). One
// canonical message, never the repository's.
func notFoundCharacter(characterID string) *apierr.Error {
	return apierr.NotFoundf("character %q not found", characterID)
}

func characterRPCError(err error) error {
	var coded *apierr.Error
	if errors.As(err, &coded) && coded.Code != apierr.CodeInternal {
		return apierr.ToGRPCError(err)
	}
	return apierr.ToGRPCError(apierr.WrapWithCode(err, apierr.CodeInternal, orchcharacter.CharacterDataUnavailableMessage))
}

// characterDataFromEntity runs only after verifyCallerOwnsCharacter has
// authenticated the owner. ProjectView is strict: malformed private state is
// an INTERNAL response rather than a forgiving partial sheet.
func (h *Handler) characterDataFromEntity(
	ctx context.Context,
	data *tkcharacter.Data,
) (*encounterv2pb.CharacterData, error) {
	projected, err := orchcharacter.ProjectView(ctx, &orchcharacter.ProjectViewInput{Data: data})
	if err != nil {
		return nil, characterRPCError(err)
	}
	if projected == nil || projected.View == nil {
		return nil, characterRPCError(errors.New("project character returned no view"))
	}

	cd, err := BuildCharacterData(projected.View)
	if err != nil {
		return nil, characterRPCError(err)
	}
	return cd, nil
}
