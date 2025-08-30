package character

import (
	"context"
	"log/slog"
	"strings"

	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
	"github.com/KirkDiggler/rpg-api/internal/entities/equipment"
	"github.com/KirkDiggler/rpg-api/internal/errors"
	"github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

func (o *Orchestrator) GetCharacterInventory(ctx context.Context, input *GetCharacterInventoryInput) (*GetCharacterInventoryOutput, error) {
	if input == nil || input.CharacterID == "" {
		return nil, errors.InvalidArgument("character ID is required")
	}

	// Get character data
	charResp, err := o.charRepo.Get(ctx, character.GetInput{
		ID: input.CharacterID,
	})
	if err != nil {
		return nil, err
	}

	// Get equipment slots
	slotsResp, err := o.charRepo.GetEquipmentSlots(ctx, character.GetEquipmentSlotsInput{
		CharacterID: input.CharacterID,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get equipment slots")
	}

	slog.DebugContext(ctx, "Retrieved equipment slots",
		"character_id", input.CharacterID,
		"armor", slotsResp.EquipmentSlots.Armor,
		"main_hand", slotsResp.EquipmentSlots.MainHand)

	// Convert repository equipment slots to dnd5e format
	equipmentSlots := convertToProtoDnd5eEquipmentSlots(slotsResp.EquipmentSlots)

	// Track which items are equipped
	equippedItems := make(map[string]bool)
	if slotsResp.EquipmentSlots.MainHand != "" {
		equippedItems[slotsResp.EquipmentSlots.MainHand] = true
	}
	if slotsResp.EquipmentSlots.OffHand != "" {
		equippedItems[slotsResp.EquipmentSlots.OffHand] = true
	}
	if slotsResp.EquipmentSlots.Armor != "" {
		equippedItems[slotsResp.EquipmentSlots.Armor] = true
	}
	if slotsResp.EquipmentSlots.Shield != "" {
		equippedItems[slotsResp.EquipmentSlots.Shield] = true
	}
	if slotsResp.EquipmentSlots.Ring1 != "" {
		equippedItems[slotsResp.EquipmentSlots.Ring1] = true
	}
	if slotsResp.EquipmentSlots.Ring2 != "" {
		equippedItems[slotsResp.EquipmentSlots.Ring2] = true
	}
	if slotsResp.EquipmentSlots.Amulet != "" {
		equippedItems[slotsResp.EquipmentSlots.Amulet] = true
	}
	if slotsResp.EquipmentSlots.Boots != "" {
		equippedItems[slotsResp.EquipmentSlots.Boots] = true
	}
	if slotsResp.EquipmentSlots.Gloves != "" {
		equippedItems[slotsResp.EquipmentSlots.Gloves] = true
	}
	if slotsResp.EquipmentSlots.Helmet != "" {
		equippedItems[slotsResp.EquipmentSlots.Helmet] = true
	}
	if slotsResp.EquipmentSlots.Belt != "" {
		equippedItems[slotsResp.EquipmentSlots.Belt] = true
	}
	if slotsResp.EquipmentSlots.Cloak != "" {
		equippedItems[slotsResp.EquipmentSlots.Cloak] = true
	}

	// Convert equipment list to inventory items, but only include unequipped items
	// Equipped items should not appear in inventory - they're shown in equipment slots
	inventory := []dnd5e.InventoryItem{}
	for _, itemName := range charResp.CharacterData.Equipment {
		isEquipped := equippedItems[itemName]
		slog.DebugContext(ctx, "Processing inventory item",
			"item_name", itemName,
			"is_equipped", isEquipped)

		// Only add to inventory if the item is NOT equipped
		if !isEquipped {
			inventory = append(inventory, dnd5e.InventoryItem{
				ID:       itemName,
				Name:     itemName,
				Quantity: 1,
				Equipped: false, // All inventory items are unequipped by definition
			})
		}
	}

	return &GetCharacterInventoryOutput{
		EquipmentSlots: equipmentSlots,
		Inventory:      inventory,
		Encumbrance:    &dnd5e.EncumbranceInfo{},
	}, nil
}

func (o *Orchestrator) EquipItem(ctx context.Context, input *EquipItemInput) (*EquipItemOutput, error) {
	// Validate input
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}
	if input.CharacterID == "" {
		return nil, errors.InvalidArgument("character ID is required")
	}
	if input.ItemID == "" {
		return nil, errors.InvalidArgument("item ID is required")
	}
	if input.Slot == "" {
		return nil, errors.InvalidArgument("equipment slot is required")
	}

	// Convert and validate proto slot name to domain type
	slot, valid := equipment.EquipmentSlotFromProtoString(input.Slot)
	if !valid {
		return nil, errors.InvalidArgumentf("invalid equipment slot: %s", input.Slot)
	}

	// Get character data
	charResp, err := o.charRepo.Get(ctx, character.GetInput{
		ID: input.CharacterID,
	})
	if err != nil {
		return nil, err
	}

	// Check if item exists in character's equipment list
	hasItem := false
	for _, item := range charResp.CharacterData.Equipment {
		if item == input.ItemID {
			hasItem = true
			break
		}
	}

	if !hasItem {
		return nil, errors.NotFoundf("item %s not found in character inventory", input.ItemID)
	}

	// Additional validation could be added here:
	// - Check if item can be equipped to this slot (requires equipment database)
	// - Check if character has prerequisites for this item
	// - Check if item conflicts with other equipped items

	// Set the equipment slot - this will return any previously equipped item
	setSlotResult, err := o.charRepo.SetEquipmentSlot(ctx, character.SetEquipmentSlotInput{
		CharacterID: input.CharacterID,
		Slot:        slot.String(),
		ItemID:      input.ItemID,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to set equipment slot")
	}

	// Prepare output
	output := &EquipItemOutput{
		Success:   true,
		Character: charResp.CharacterData,
	}

	// If there was a previously equipped item, return it
	if setSlotResult.PreviousItemID != "" {
		output.PreviouslyEquippedItem = &dnd5e.InventoryItem{
			ID:       setSlotResult.PreviousItemID,
			Name:     setSlotResult.PreviousItemID, // Using ID as name for now
			Quantity: 1,
			Equipped: false,
		}
	}

	return output, nil
}

func (o *Orchestrator) ListEquipmentByType(ctx context.Context, input *ListEquipmentByTypeInput) (*ListEquipmentByTypeOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

// convertToProtoDnd5eEquipmentSlots converts repository equipment slots to dnd5e format
func convertToProtoDnd5eEquipmentSlots(repoSlots *character.EquipmentSlots) *dnd5e.EquipmentSlots {
	equipmentSlots := &dnd5e.EquipmentSlots{}

	if repoSlots.MainHand != "" {
		equipmentSlots.MainHand = &dnd5e.InventoryItem{
			ID:       repoSlots.MainHand,
			Name:     repoSlots.MainHand,
			Quantity: 1,
			Equipped: true,
		}
	}
	if repoSlots.OffHand != "" {
		equipmentSlots.OffHand = &dnd5e.InventoryItem{
			ID:       repoSlots.OffHand,
			Name:     repoSlots.OffHand,
			Quantity: 1,
			Equipped: true,
		}
	}
	if repoSlots.Armor != "" {
		equipmentSlots.Armor = &dnd5e.InventoryItem{
			ID:       repoSlots.Armor,
			Name:     repoSlots.Armor,
			Quantity: 1,
			Equipped: true,
		}
	}
	if repoSlots.Helmet != "" {
		equipmentSlots.Helm = &dnd5e.InventoryItem{
			ID:       repoSlots.Helmet,
			Name:     repoSlots.Helmet,
			Quantity: 1,
			Equipped: true,
		}
	}
	if repoSlots.Gloves != "" {
		equipmentSlots.Gloves = &dnd5e.InventoryItem{
			ID:       repoSlots.Gloves,
			Name:     repoSlots.Gloves,
			Quantity: 1,
			Equipped: true,
		}
	}
	if repoSlots.Boots != "" {
		equipmentSlots.Boots = &dnd5e.InventoryItem{
			ID:       repoSlots.Boots,
			Name:     repoSlots.Boots,
			Quantity: 1,
			Equipped: true,
		}
	}
	if repoSlots.Ring1 != "" {
		equipmentSlots.Ring1 = &dnd5e.InventoryItem{
			ID:       repoSlots.Ring1,
			Name:     repoSlots.Ring1,
			Quantity: 1,
			Equipped: true,
		}
	}
	if repoSlots.Ring2 != "" {
		equipmentSlots.Ring2 = &dnd5e.InventoryItem{
			ID:       repoSlots.Ring2,
			Name:     repoSlots.Ring2,
			Quantity: 1,
			Equipped: true,
		}
	}
	if repoSlots.Cloak != "" {
		equipmentSlots.Cloak = &dnd5e.InventoryItem{
			ID:       repoSlots.Cloak,
			Name:     repoSlots.Cloak,
			Quantity: 1,
			Equipped: true,
		}
	}
	if repoSlots.Amulet != "" {
		equipmentSlots.Amulet = &dnd5e.InventoryItem{
			ID:       repoSlots.Amulet,
			Name:     repoSlots.Amulet,
			Quantity: 1,
			Equipped: true,
		}
	}

	return equipmentSlots
}

func (o *Orchestrator) UnequipItem(ctx context.Context, input *UnequipItemInput) (*UnequipItemOutput, error) {
	// Validate input
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}
	if input.CharacterID == "" {
		return nil, errors.InvalidArgument("character ID is required")
	}
	if input.Slot == "" {
		return nil, errors.InvalidArgument("equipment slot is required")
	}

	// Convert and validate proto slot name to domain type
	slot, valid := equipment.EquipmentSlotFromProtoString(input.Slot)
	if !valid {
		return nil, errors.InvalidArgumentf("invalid equipment slot: %s", input.Slot)
	}

	// Get character data
	charResp, err := o.charRepo.Get(ctx, character.GetInput{
		ID: input.CharacterID,
	})
	if err != nil {
		return nil, err
	}

	// Clear the equipment slot
	_, err = o.charRepo.ClearEquipmentSlot(ctx, character.ClearEquipmentSlotInput{
		CharacterID: input.CharacterID,
		Slot:        slot.String(),
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to clear equipment slot")
	}

	return &UnequipItemOutput{
		Success:   true,
		Character: charResp.CharacterData,
	}, nil
}

// unpackBundleItem extracts the actual item ID from a bundle reference
// Bundle references come from the Discord bot in format "bundle_X:Y:item_id"
// where X is the bundle number, Y is the index, and item_id is the actual item
// For example: "bundle_1:0:greatclub" -> "greatclub"
func unpackBundleItem(bundleRef string) string {
	// Check if this is a bundle reference
	if strings.HasPrefix(bundleRef, "bundle_") {
		// Split by colon to get parts
		parts := strings.Split(bundleRef, ":")
		if len(parts) == 3 {
			// Return the actual item ID (last part)
			return parts[2]
		}
	}
	// Not a bundle reference, return as-is
	return bundleRef
}

func (o *Orchestrator) AddToInventory(ctx context.Context, input *AddToInventoryInput) (*AddToInventoryOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) RemoveFromInventory(ctx context.Context, input *RemoveFromInventoryInput) (*RemoveFromInventoryOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}
