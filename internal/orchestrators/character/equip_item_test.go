package character

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	dicemock "github.com/KirkDiggler/rpg-api/internal/orchestrators/dice/mock"
	idgenmock "github.com/KirkDiggler/rpg-api/internal/pkg/idgen/mock"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	draftmock "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft/mock"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

// EquipItemTestSuite proves the rules-correct equip/unequip path (rpg-api#680):
// the orchestrator must route through the toolkit's rules engine — occupancy,
// slot-compatibility, swap-on-occupied — rather than writing EquipmentSlots
// directly. Kept as its own suite (not folded into OrchestratorTestSuite) so
// the fixtures stay equipment-focused and don't drag in draft/class/race
// setup this slice doesn't need.
type EquipItemTestSuite struct {
	suite.Suite
	ctrl              *gomock.Controller
	mockCharacterRepo *charactermock.MockRepository
	orchestrator      *Orchestrator
	ctx               context.Context

	testCharacterID string
}

func TestEquipItemSuite(t *testing.T) {
	suite.Run(t, new(EquipItemTestSuite))
}

func (s *EquipItemTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockCharacterRepo = charactermock.NewMockRepository(s.ctrl)
	s.ctx = context.Background()
	s.testCharacterID = "char-fighter-1"

	var err error
	s.orchestrator, err = New(&Config{
		DraftRepo:        draftmock.NewMockRepository(s.ctrl),
		CharacterRepo:    s.mockCharacterRepo,
		DiceService:      dicemock.NewMockService(s.ctrl),
		IDGenerator:      idgenmock.NewMockGenerator(s.ctrl),
		DraftIDGenerator: idgenmock.NewMockGenerator(s.ctrl),
	})
	s.Require().NoError(err)
}

func (s *EquipItemTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

// fighterWithLongswordAndShield is unarmed (nothing equipped) but carries a
// longsword and a shield — enough to exercise a plain equip and a
// slot-swap without needing the full class/race/background setup the draft
// tests build.
func (s *EquipItemTestSuite) fighterWithLongswordAndShield() *entities.Character {
	return &entities.Character{
		Data: &character.Data{
			ID:               s.testCharacterID,
			Name:             "Test Fighter",
			Level:            1,
			ClassID:          "fighter",
			ProficiencyBonus: 2,
			HitPoints:        12,
			MaxHitPoints:     12,
			ArmorClass:       10,
			AbilityScores: shared.AbilityScores{
				abilities.STR: 16,
				abilities.DEX: 12,
				abilities.CON: 14,
				abilities.INT: 10,
				abilities.WIS: 10,
				abilities.CHA: 10,
			},
			Inventory: []character.InventoryItemData{
				{Type: "weapon", ID: "longsword", Quantity: 1},
				{Type: "armor", ID: "shield", Quantity: 1},
				{Type: "weapon", ID: "greatsword", Quantity: 1},
			},
			EquipmentSlots: character.EquipmentSlots{},
		},
	}
}

func (s *EquipItemTestSuite) TestEquipItem_Success_NoPreviousOccupant() {
	charEntity := s.fighterWithLongswordAndShield()

	s.mockCharacterRepo.EXPECT().
		Get(s.ctx, characterrepo.GetInput{ID: s.testCharacterID}).
		Return(&characterrepo.GetOutput{Character: charEntity}, nil)

	s.mockCharacterRepo.EXPECT().
		Update(s.ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, input characterrepo.UpdateInput) (*characterrepo.UpdateOutput, error) {
			s.Assert().Equal("longsword", input.Character.Data.EquipmentSlots.Get(character.SlotMainHand))
			return &characterrepo.UpdateOutput{Character: input.Character}, nil
		})

	out, err := s.orchestrator.EquipItem(s.ctx, &EquipItemInput{
		CharacterID: s.testCharacterID,
		ItemID:      "longsword",
		Slot:        character.SlotMainHand,
	})
	s.Require().NoError(err)
	s.Assert().Empty(out.PreviousItemID)
}

// TestEquipItem_TwoHanded_ClearsOffHand proves the toolkit's occupancy rule
// (rpg-toolkit#812) actually runs through this orchestrator method, not just
// in the toolkit's own tests: equipping a two-handed weapon into main_hand
// must clear off_hand as a side effect of the SAME EquipItem call.
func (s *EquipItemTestSuite) TestEquipItem_TwoHanded_ClearsOffHand() {
	charEntity := s.fighterWithLongswordAndShield()
	charEntity.Data.EquipmentSlots = character.EquipmentSlots{
		character.SlotOffHand: "shield",
	}

	s.mockCharacterRepo.EXPECT().
		Get(s.ctx, characterrepo.GetInput{ID: s.testCharacterID}).
		Return(&characterrepo.GetOutput{Character: charEntity}, nil)

	s.mockCharacterRepo.EXPECT().
		Update(s.ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, input characterrepo.UpdateInput) (*characterrepo.UpdateOutput, error) {
			s.Assert().Equal("greatsword", input.Character.Data.EquipmentSlots.Get(character.SlotMainHand))
			s.Assert().Empty(input.Character.Data.EquipmentSlots.Get(character.SlotOffHand),
				"equipping a two-handed weapon must clear off_hand")
			return &characterrepo.UpdateOutput{Character: input.Character}, nil
		})

	out, err := s.orchestrator.EquipItem(s.ctx, &EquipItemInput{
		CharacterID: s.testCharacterID,
		ItemID:      "greatsword",
		Slot:        character.SlotMainHand,
	})
	s.Require().NoError(err)
	s.Assert().Empty(out.PreviousItemID, "main_hand had nothing equipped before this call")
}

func (s *EquipItemTestSuite) TestEquipItem_Swap_ReturnsPreviousOccupant() {
	charEntity := s.fighterWithLongswordAndShield()
	charEntity.Data.Inventory = append(charEntity.Data.Inventory,
		character.InventoryItemData{Type: "weapon", ID: "handaxe", Quantity: 1})
	charEntity.Data.EquipmentSlots = character.EquipmentSlots{
		character.SlotMainHand: "handaxe",
	}

	s.mockCharacterRepo.EXPECT().
		Get(s.ctx, characterrepo.GetInput{ID: s.testCharacterID}).
		Return(&characterrepo.GetOutput{Character: charEntity}, nil)

	s.mockCharacterRepo.EXPECT().
		Update(s.ctx, gomock.Any()).
		Return(&characterrepo.UpdateOutput{}, nil)

	out, err := s.orchestrator.EquipItem(s.ctx, &EquipItemInput{
		CharacterID: s.testCharacterID,
		ItemID:      "longsword",
		Slot:        character.SlotMainHand,
	})
	s.Require().NoError(err)
	s.Assert().Equal("handaxe", out.PreviousItemID)
}

func (s *EquipItemTestSuite) TestEquipItem_ItemNotInInventory_ReturnsNotFound() {
	charEntity := s.fighterWithLongswordAndShield()

	s.mockCharacterRepo.EXPECT().
		Get(s.ctx, characterrepo.GetInput{ID: s.testCharacterID}).
		Return(&characterrepo.GetOutput{Character: charEntity}, nil)

	_, err := s.orchestrator.EquipItem(s.ctx, &EquipItemInput{
		CharacterID: s.testCharacterID,
		ItemID:      "not-owned-item",
		Slot:        character.SlotMainHand,
	})
	s.Require().Error(err)
	s.Assert().True(apierr.IsNotFound(err), "expected NotFound, got %v", err)
}

// TestEquipItem_IncompatibleSlot_ReturnsInvalidArgument proves the toolkit's
// new slot-compatibility validation (rpg-toolkit#812 — equipping into an
// incompatible slot used to silently succeed) surfaces as an
// InvalidArgument, not a silent no-op or an opaque Internal error.
func (s *EquipItemTestSuite) TestEquipItem_IncompatibleSlot_ReturnsInvalidArgument() {
	charEntity := s.fighterWithLongswordAndShield()

	s.mockCharacterRepo.EXPECT().
		Get(s.ctx, characterrepo.GetInput{ID: s.testCharacterID}).
		Return(&characterrepo.GetOutput{Character: charEntity}, nil)

	_, err := s.orchestrator.EquipItem(s.ctx, &EquipItemInput{
		CharacterID: s.testCharacterID,
		ItemID:      "shield",
		Slot:        character.SlotMainHand,
	})
	s.Require().Error(err)
	s.Assert().True(apierr.IsInvalidArgument(err), "expected InvalidArgument, got %v", err)
}

func (s *EquipItemTestSuite) TestUnequipItem_Success() {
	charEntity := s.fighterWithLongswordAndShield()
	charEntity.Data.EquipmentSlots = character.EquipmentSlots{
		character.SlotMainHand: "longsword",
	}

	s.mockCharacterRepo.EXPECT().
		Get(s.ctx, characterrepo.GetInput{ID: s.testCharacterID}).
		Return(&characterrepo.GetOutput{Character: charEntity}, nil)

	s.mockCharacterRepo.EXPECT().
		Update(s.ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, input characterrepo.UpdateInput) (*characterrepo.UpdateOutput, error) {
			s.Assert().Empty(input.Character.Data.EquipmentSlots.Get(character.SlotMainHand))
			return &characterrepo.UpdateOutput{Character: input.Character}, nil
		})

	out, err := s.orchestrator.UnequipItem(s.ctx, &UnequipItemInput{
		CharacterID: s.testCharacterID,
		Slot:        character.SlotMainHand,
	})
	s.Require().NoError(err)
	s.Assert().Equal("longsword", out.UnequippedItemID)
}
