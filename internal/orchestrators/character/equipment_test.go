package character_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	externalmock "github.com/KirkDiggler/rpg-api/internal/clients/external/mock"
	"github.com/KirkDiggler/rpg-api/internal/errors"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	dicemock "github.com/KirkDiggler/rpg-api/internal/orchestrators/dice/mock"
	idgenmock "github.com/KirkDiggler/rpg-api/internal/pkg/idgen/mock"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	characterrepomock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	draftrepomock "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft/mock"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

type EquipmentTestSuite struct {
	suite.Suite
	ctrl            *gomock.Controller
	mockCharRepo    *characterrepomock.MockRepository
	mockDraftRepo   *draftrepomock.MockRepository
	mockExternal    *externalmock.MockClient
	mockDiceService *dicemock.MockService
	mockIDGenerator *idgenmock.MockGenerator
	mockDraftIDGen  *idgenmock.MockGenerator
	orchestrator    *character.Orchestrator
	ctx             context.Context

	// Test data
	testCharacterID string
	testItemID      string
	testCharData    *toolkitchar.Data
}

func (s *EquipmentTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockCharRepo = characterrepomock.NewMockRepository(s.ctrl)
	s.mockDraftRepo = draftrepomock.NewMockRepository(s.ctrl)
	s.mockExternal = externalmock.NewMockClient(s.ctrl)
	s.mockDiceService = dicemock.NewMockService(s.ctrl)
	s.mockIDGenerator = idgenmock.NewMockGenerator(s.ctrl)
	s.mockDraftIDGen = idgenmock.NewMockGenerator(s.ctrl)

	var err error
	s.orchestrator, err = character.New(&character.Config{
		CharacterRepo:      s.mockCharRepo,
		CharacterDraftRepo: s.mockDraftRepo,
		ExternalClient:     s.mockExternal,
		DiceService:        s.mockDiceService,
		IDGenerator:        s.mockIDGenerator,
		DraftIDGenerator:   s.mockDraftIDGen,
	})
	s.Require().NoError(err)
	s.ctx = context.Background()

	// Test data setup
	s.testCharacterID = "test-character-123"
	s.testItemID = "chain-mail"
	s.testCharData = &toolkitchar.Data{
		ID: s.testCharacterID,
		Equipment: []string{
			s.testItemID,    // Item available in inventory
			"leather-armor", // Another item
			"sword",         // Another item
		},
	}
}

func (s *EquipmentTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func TestEquipmentTestSuite(t *testing.T) {
	suite.Run(t, new(EquipmentTestSuite))
}

// Test EquipItem functionality
func (s *EquipmentTestSuite) TestEquipItem_Success() {
	// Setup mocks
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: s.testCharacterID}).
		Return(&characterrepo.GetOutput{CharacterData: s.testCharData}, nil)

	s.mockCharRepo.EXPECT().
		SetEquipmentSlot(gomock.Any(), characterrepo.SetEquipmentSlotInput{
			CharacterID: s.testCharacterID,
			Slot:        "armor",
			ItemID:      s.testItemID,
		}).
		Return(&characterrepo.SetEquipmentSlotOutput{PreviousItemID: ""}, nil)

	// Execute
	result, err := s.orchestrator.EquipItem(s.ctx, &character.EquipItemInput{
		CharacterID: s.testCharacterID,
		ItemID:      s.testItemID,
		Slot:        "EQUIPMENT_SLOT_ARMOR",
	})

	// Assert
	s.NoError(err)
	s.NotNil(result)
	s.True(result.Success)
	s.Equal(s.testCharData, result.Character)
	s.Nil(result.PreviouslyEquippedItem)
}

func (s *EquipmentTestSuite) TestEquipItem_WithPreviousItemInSlot() {
	previousItem := "leather-armor"

	// Setup mocks
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: s.testCharacterID}).
		Return(&characterrepo.GetOutput{CharacterData: s.testCharData}, nil)

	s.mockCharRepo.EXPECT().
		SetEquipmentSlot(gomock.Any(), characterrepo.SetEquipmentSlotInput{
			CharacterID: s.testCharacterID,
			Slot:        "armor",
			ItemID:      s.testItemID,
		}).
		Return(&characterrepo.SetEquipmentSlotOutput{PreviousItemID: previousItem}, nil)

	// Execute
	result, err := s.orchestrator.EquipItem(s.ctx, &character.EquipItemInput{
		CharacterID: s.testCharacterID,
		ItemID:      s.testItemID,
		Slot:        "EQUIPMENT_SLOT_ARMOR",
	})

	// Assert
	s.NoError(err)
	s.NotNil(result)
	s.True(result.Success)
	s.NotNil(result.PreviouslyEquippedItem)
	s.Equal(previousItem, result.PreviouslyEquippedItem.ID)
	s.False(result.PreviouslyEquippedItem.Equipped)
}

func (s *EquipmentTestSuite) TestEquipItem_ItemNotInInventory() {
	nonExistentItem := "dragon-scale-armor"

	// Setup mocks
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: s.testCharacterID}).
		Return(&characterrepo.GetOutput{CharacterData: s.testCharData}, nil)

	// Execute
	result, err := s.orchestrator.EquipItem(s.ctx, &character.EquipItemInput{
		CharacterID: s.testCharacterID,
		ItemID:      nonExistentItem,
		Slot:        "EQUIPMENT_SLOT_ARMOR",
	})

	// Assert
	s.Error(err)
	s.Nil(result)
	s.True(errors.IsNotFound(err))
	s.Contains(err.Error(), "not found in character inventory")
}

func (s *EquipmentTestSuite) TestEquipItem_InvalidInput() {
	tests := []struct {
		name  string
		input *character.EquipItemInput
	}{
		{
			name:  "nil input",
			input: nil,
		},
		{
			name: "empty character ID",
			input: &character.EquipItemInput{
				CharacterID: "",
				ItemID:      s.testItemID,
				Slot:        "EQUIPMENT_SLOT_ARMOR",
			},
		},
		{
			name: "empty item ID",
			input: &character.EquipItemInput{
				CharacterID: s.testCharacterID,
				ItemID:      "",
				Slot:        "EQUIPMENT_SLOT_ARMOR",
			},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			// Execute
			result, err := s.orchestrator.EquipItem(s.ctx, tt.input)

			// Assert
			s.Error(err)
			s.Nil(result)
			s.True(errors.IsInvalidArgument(err))
		})
	}
}

func (s *EquipmentTestSuite) TestEquipItem_CharacterNotFound() {
	// Setup mocks
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: s.testCharacterID}).
		Return(nil, errors.NotFound("character not found"))

	// Execute
	result, err := s.orchestrator.EquipItem(s.ctx, &character.EquipItemInput{
		CharacterID: s.testCharacterID,
		ItemID:      s.testItemID,
		Slot:        "EQUIPMENT_SLOT_ARMOR",
	})

	// Assert
	s.Error(err)
	s.Nil(result)
	s.True(errors.IsNotFound(err))
}

// Test GetCharacterInventory functionality
func (s *EquipmentTestSuite) TestGetCharacterInventory_Success() {
	// Test equipment slots with some items equipped
	equipmentSlots := &characterrepo.EquipmentSlots{
		MainHand: "sword",
		OffHand:  "",
		Armor:    s.testItemID, // chain-mail is equipped
		Helmet:   "",
		Gloves:   "",
		Boots:    "",
		Cloak:    "",
		Amulet:   "",
		Ring1:    "",
		Ring2:    "",
		Belt:     "",
		Shield:   "",
	}

	// Setup mocks
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: s.testCharacterID}).
		Return(&characterrepo.GetOutput{CharacterData: s.testCharData}, nil)

	s.mockCharRepo.EXPECT().
		GetEquipmentSlots(gomock.Any(), characterrepo.GetEquipmentSlotsInput{
			CharacterID: s.testCharacterID,
		}).
		Return(&characterrepo.GetEquipmentSlotsOutput{EquipmentSlots: equipmentSlots}, nil)

	// Execute
	result, err := s.orchestrator.GetCharacterInventory(s.ctx, &character.GetCharacterInventoryInput{
		CharacterID: s.testCharacterID,
	})

	// Assert
	s.NoError(err)
	s.NotNil(result)

	// Verify equipped items are in equipment slots
	s.NotNil(result.EquipmentSlots)
	s.Equal(s.testItemID, result.EquipmentSlots.Armor.ID) // chain-mail should be in armor slot
	s.Equal("sword", result.EquipmentSlots.MainHand.ID)   // sword should be in main hand

	// Verify inventory contains only unequipped items
	expectedInventoryItems := []string{"leather-armor"} // Only unequipped items should be in inventory
	s.Len(result.Inventory, len(expectedInventoryItems))

	for _, expectedItem := range expectedInventoryItems {
		found := false
		for _, inventoryItem := range result.Inventory {
			if inventoryItem.ID == expectedItem && !inventoryItem.Equipped {
				found = true
				break
			}
		}
		s.True(found, "Expected item %s to be in inventory and not equipped", expectedItem)
	}

	// Verify equipped items are NOT in inventory
	equippedItems := []string{s.testItemID, "sword"}
	for _, equippedItem := range equippedItems {
		for _, inventoryItem := range result.Inventory {
			s.NotEqual(equippedItem, inventoryItem.ID, "Equipped item %s should not be in inventory", equippedItem)
		}
	}
}

func (s *EquipmentTestSuite) TestGetCharacterInventory_NoEquippedItems() {
	// Test equipment slots with no items equipped
	emptyEquipmentSlots := &characterrepo.EquipmentSlots{}

	// Setup mocks
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: s.testCharacterID}).
		Return(&characterrepo.GetOutput{CharacterData: s.testCharData}, nil)

	s.mockCharRepo.EXPECT().
		GetEquipmentSlots(gomock.Any(), characterrepo.GetEquipmentSlotsInput{
			CharacterID: s.testCharacterID,
		}).
		Return(&characterrepo.GetEquipmentSlotsOutput{EquipmentSlots: emptyEquipmentSlots}, nil)

	// Execute
	result, err := s.orchestrator.GetCharacterInventory(s.ctx, &character.GetCharacterInventoryInput{
		CharacterID: s.testCharacterID,
	})

	// Assert
	s.NoError(err)
	s.NotNil(result)

	// All items should be in inventory since nothing is equipped
	s.Len(result.Inventory, len(s.testCharData.Equipment))

	// Verify all items are marked as unequipped
	for _, inventoryItem := range result.Inventory {
		s.False(inventoryItem.Equipped, "Item %s should not be equipped", inventoryItem.ID)
	}
}

func (s *EquipmentTestSuite) TestGetCharacterInventory_InvalidInput() {
	tests := []struct {
		name  string
		input *character.GetCharacterInventoryInput
	}{
		{
			name:  "nil input",
			input: nil,
		},
		{
			name: "empty character ID",
			input: &character.GetCharacterInventoryInput{
				CharacterID: "",
			},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			// Execute
			result, err := s.orchestrator.GetCharacterInventory(s.ctx, tt.input)

			// Assert
			s.Error(err)
			s.Nil(result)
			s.True(errors.IsInvalidArgument(err))
		})
	}
}

// Test UnequipItem functionality
func (s *EquipmentTestSuite) TestUnequipItem_Success() {
	// Setup mocks - item currently equipped in armor slot
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: s.testCharacterID}).
		Return(&characterrepo.GetOutput{CharacterData: s.testCharData}, nil)

	s.mockCharRepo.EXPECT().
		ClearEquipmentSlot(gomock.Any(), characterrepo.ClearEquipmentSlotInput{
			CharacterID: s.testCharacterID,
			Slot:        "armor",
		}).
		Return(&characterrepo.ClearEquipmentSlotOutput{ClearedItemID: s.testItemID}, nil)

	// Execute
	result, err := s.orchestrator.UnequipItem(s.ctx, &character.UnequipItemInput{
		CharacterID: s.testCharacterID,
		Slot:        "EQUIPMENT_SLOT_ARMOR",
	})

	// Assert
	s.NoError(err)
	s.NotNil(result)
	s.True(result.Success)
	// For now, just verify success since UnequippedItem field doesn't exist yet
	// TODO: Add UnequippedItem field to UnequipItemOutput after fixing types
}

func (s *EquipmentTestSuite) TestUnequipItem_EmptySlot() {
	// Setup mocks - slot is already empty
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: s.testCharacterID}).
		Return(&characterrepo.GetOutput{CharacterData: s.testCharData}, nil)

	s.mockCharRepo.EXPECT().
		ClearEquipmentSlot(gomock.Any(), characterrepo.ClearEquipmentSlotInput{
			CharacterID: s.testCharacterID,
			Slot:        "armor",
		}).
		Return(&characterrepo.ClearEquipmentSlotOutput{ClearedItemID: ""}, nil)

	// Execute
	result, err := s.orchestrator.UnequipItem(s.ctx, &character.UnequipItemInput{
		CharacterID: s.testCharacterID,
		Slot:        "EQUIPMENT_SLOT_ARMOR",
	})

	// Assert
	s.NoError(err)
	s.NotNil(result)
	s.True(result.Success)
	// For now, just verify success since UnequippedItem field doesn't exist yet
	// TODO: Add UnequippedItem field to UnequipItemOutput after fixing types
}

func (s *EquipmentTestSuite) TestUnequipItem_InvalidInput() {
	tests := []struct {
		name  string
		input *character.UnequipItemInput
	}{
		{
			name:  "nil input",
			input: nil,
		},
		{
			name: "empty character ID",
			input: &character.UnequipItemInput{
				CharacterID: "",
				Slot:        "EQUIPMENT_SLOT_ARMOR",
			},
		},
		{
			name: "empty slot",
			input: &character.UnequipItemInput{
				CharacterID: s.testCharacterID,
				Slot:        "",
			},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			// Execute
			result, err := s.orchestrator.UnequipItem(s.ctx, tt.input)

			// Assert
			s.Error(err)
			s.Nil(result)
			s.True(errors.IsInvalidArgument(err))
		})
	}
}

// Test equipment state persistence - integration scenario
func (s *EquipmentTestSuite) TestEquipmentStatePersistence() {
	// This test verifies that equipping and then getting inventory shows correct state
	equipmentSlots := &characterrepo.EquipmentSlots{
		Armor: s.testItemID, // Item will be equipped here after EquipItem call
	}

	// Step 1: Equip the item
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: s.testCharacterID}).
		Return(&characterrepo.GetOutput{CharacterData: s.testCharData}, nil)

	s.mockCharRepo.EXPECT().
		SetEquipmentSlot(gomock.Any(), characterrepo.SetEquipmentSlotInput{
			CharacterID: s.testCharacterID,
			Slot:        "armor",
			ItemID:      s.testItemID,
		}).
		Return(&characterrepo.SetEquipmentSlotOutput{PreviousItemID: ""}, nil)

	equipResult, err := s.orchestrator.EquipItem(s.ctx, &character.EquipItemInput{
		CharacterID: s.testCharacterID,
		ItemID:      s.testItemID,
		Slot:        "EQUIPMENT_SLOT_ARMOR",
	})
	s.NoError(err)
	s.True(equipResult.Success)

	// Step 2: Get inventory and verify the item is equipped and not in inventory
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: s.testCharacterID}).
		Return(&characterrepo.GetOutput{CharacterData: s.testCharData}, nil)

	s.mockCharRepo.EXPECT().
		GetEquipmentSlots(gomock.Any(), characterrepo.GetEquipmentSlotsInput{
			CharacterID: s.testCharacterID,
		}).
		Return(&characterrepo.GetEquipmentSlotsOutput{EquipmentSlots: equipmentSlots}, nil)

	inventoryResult, err := s.orchestrator.GetCharacterInventory(s.ctx, &character.GetCharacterInventoryInput{
		CharacterID: s.testCharacterID,
	})
	s.NoError(err)

	// Verify item is in equipment slot
	s.Equal(s.testItemID, inventoryResult.EquipmentSlots.Armor.ID)

	// Verify item is NOT in inventory
	for _, item := range inventoryResult.Inventory {
		s.NotEqual(s.testItemID, item.ID, "Equipped item should not be in inventory")
	}

	// Step 3: Unequip the item
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: s.testCharacterID}).
		Return(&characterrepo.GetOutput{CharacterData: s.testCharData}, nil)

	s.mockCharRepo.EXPECT().
		ClearEquipmentSlot(gomock.Any(), characterrepo.ClearEquipmentSlotInput{
			CharacterID: s.testCharacterID,
			Slot:        "armor",
		}).
		Return(&characterrepo.ClearEquipmentSlotOutput{ClearedItemID: s.testItemID}, nil)

	unequipResult, err := s.orchestrator.UnequipItem(s.ctx, &character.UnequipItemInput{
		CharacterID: s.testCharacterID,
		Slot:        "EQUIPMENT_SLOT_ARMOR",
	})
	s.NoError(err)
	s.True(unequipResult.Success)
	// For now, just verify success since UnequippedItem field doesn't exist yet
	// TODO: Add UnequippedItem field to UnequipItemOutput after fixing types

	// Step 4: Get inventory again and verify item is back in inventory
	emptyEquipmentSlots := &characterrepo.EquipmentSlots{} // No items equipped after unequipping

	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: s.testCharacterID}).
		Return(&characterrepo.GetOutput{CharacterData: s.testCharData}, nil)

	s.mockCharRepo.EXPECT().
		GetEquipmentSlots(gomock.Any(), characterrepo.GetEquipmentSlotsInput{
			CharacterID: s.testCharacterID,
		}).
		Return(&characterrepo.GetEquipmentSlotsOutput{EquipmentSlots: emptyEquipmentSlots}, nil)

	finalInventoryResult, err := s.orchestrator.GetCharacterInventory(s.ctx, &character.GetCharacterInventoryInput{
		CharacterID: s.testCharacterID,
	})
	s.NoError(err)

	// Verify armor slot is empty
	if finalInventoryResult.EquipmentSlots.Armor != nil {
		s.Empty(finalInventoryResult.EquipmentSlots.Armor.ID)
	}

	// Verify item is back in inventory and not equipped
	found := false
	for _, item := range finalInventoryResult.Inventory {
		if item.ID == s.testItemID {
			s.False(item.Equipped, "Item should not be equipped after unequipping")
			found = true
			break
		}
	}
	s.True(found, "Item should be back in inventory after unequipping")
}
