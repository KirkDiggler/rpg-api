package character

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	characterpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/character"
	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	orchcharacter "github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/orchestrators/character/mock"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

// HandlerTestSuite proves the v1alpha2 character handler is a thin proto<->
// orchestrator wrapper (rpg-api#680): validation, delegation to the shared
// characterService.EquipItem/UnequipItem, error propagation as gRPC status
// codes, and the recomputed CharacterData response — no business logic.
type HandlerTestSuite struct {
	suite.Suite
	ctrl        *gomock.Controller
	mockService *charactermock.MockService
	handler     *Handler
	ctx         context.Context

	testCharacterID string
}

func TestHandlerSuite(t *testing.T) {
	suite.Run(t, new(HandlerTestSuite))
}

func (s *HandlerTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockService = charactermock.NewMockService(s.ctrl)
	s.ctx = context.Background()
	s.testCharacterID = "char-fighter-1"

	var err error
	s.handler, err = New(&HandlerConfig{CharacterService: s.mockService})
	s.Require().NoError(err)
}

func (s *HandlerTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *HandlerTestSuite) TestNew_MissingConfig() {
	_, err := New(nil)
	s.Require().Error(err)

	_, err = New(&HandlerConfig{})
	s.Require().Error(err)
}

// fighterCharacterEntity mirrors the orchestrator suite's fixture: a
// longsword+shield fighter, enough to compute a real EquipmentView.
func (s *HandlerTestSuite) fighterCharacterEntity() *entities.Character {
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
			},
			EquipmentSlots: character.EquipmentSlots{
				character.SlotMainHand: "longsword",
				character.SlotOffHand:  "shield",
			},
		},
	}
}

func (s *HandlerTestSuite) TestEquipItem_ValidationErrors() {
	tests := []struct {
		name string
		req  *characterpb.EquipItemRequest
	}{
		{
			name: "missing character_id",
			req: &characterpb.EquipItemRequest{
				Item:    &encounterv2pb.Ref{Id: "longsword"},
				SlotKey: "main_hand",
			},
		},
		{
			name: "missing item",
			req: &characterpb.EquipItemRequest{
				CharacterId: s.testCharacterID,
				SlotKey:     "main_hand",
			},
		},
		{
			name: "missing slot_key",
			req: &characterpb.EquipItemRequest{
				CharacterId: s.testCharacterID,
				Item:        &encounterv2pb.Ref{Id: "longsword"},
			},
		},
	}

	for _, tc := range tests {
		s.Run(tc.name, func() {
			_, err := s.handler.EquipItem(s.ctx, tc.req)
			s.Require().Error(err)
			s.Assert().Equal(codes.InvalidArgument, status.Code(err))
		})
	}
}

func (s *HandlerTestSuite) TestEquipItem_Success() {
	charEntity := s.fighterCharacterEntity()

	s.mockService.EXPECT().
		EquipItem(s.ctx, &orchcharacter.EquipItemInput{
			CharacterID: s.testCharacterID,
			ItemID:      "longsword",
			Slot:        character.SlotMainHand,
		}).
		Return(&orchcharacter.EquipItemOutput{}, nil)

	s.mockService.EXPECT().
		GetCharacter(s.ctx, &orchcharacter.GetCharacterInput{CharacterID: s.testCharacterID}).
		Return(&orchcharacter.GetCharacterOutput{Character: charEntity}, nil)

	resp, err := s.handler.EquipItem(s.ctx, &characterpb.EquipItemRequest{
		CharacterId: s.testCharacterID,
		Item:        &encounterv2pb.Ref{Module: "dnd5e", Type: "item", Id: "longsword"},
		SlotKey:     "main_hand",
	})
	s.Require().NoError(err)
	s.Require().NotNil(resp.GetCharacter())

	cd := resp.GetCharacter()
	s.Require().NotNil(cd.GetArmorClassDetail(), "armor_class_detail is the only AC total on this response")
	s.Assert().NotZero(cd.GetArmorClassDetail().GetTotal(), "AC must be the real toolkit-computed total, not zero")
	s.Assert().Contains(cd.GetEquipped(), "main_hand")
	s.Assert().Equal("longsword", cd.GetEquipped()["main_hand"].GetId())
	s.Assert().Len(cd.GetInventory(), 2, "inventory includes equipped items, per the wire contract")
}

func (s *HandlerTestSuite) TestEquipItem_OrchestratorError_PropagatesAsNotFound() {
	s.mockService.EXPECT().
		EquipItem(s.ctx, gomock.Any()).
		Return(nil, apierr.NotFound("item not found in inventory"))

	_, err := s.handler.EquipItem(s.ctx, &characterpb.EquipItemRequest{
		CharacterId: s.testCharacterID,
		Item:        &encounterv2pb.Ref{Id: "not-owned"},
		SlotKey:     "main_hand",
	})
	s.Require().Error(err)
	s.Assert().Equal(codes.NotFound, status.Code(err))
}

func (s *HandlerTestSuite) TestEquipItem_OrchestratorGenericError_PropagatesAsInternal() {
	s.mockService.EXPECT().
		EquipItem(s.ctx, gomock.Any()).
		Return(nil, errors.New("boom"))

	_, err := s.handler.EquipItem(s.ctx, &characterpb.EquipItemRequest{
		CharacterId: s.testCharacterID,
		Item:        &encounterv2pb.Ref{Id: "longsword"},
		SlotKey:     "main_hand",
	})
	s.Require().Error(err)
	s.Assert().Equal(codes.Internal, status.Code(err))
}

func (s *HandlerTestSuite) TestUnequipItem_ValidationErrors() {
	tests := []struct {
		name string
		req  *characterpb.UnequipItemRequest
	}{
		{name: "missing character_id", req: &characterpb.UnequipItemRequest{SlotKey: "main_hand"}},
		{name: "missing slot_key", req: &characterpb.UnequipItemRequest{CharacterId: s.testCharacterID}},
	}

	for _, tc := range tests {
		s.Run(tc.name, func() {
			_, err := s.handler.UnequipItem(s.ctx, tc.req)
			s.Require().Error(err)
			s.Assert().Equal(codes.InvalidArgument, status.Code(err))
		})
	}
}

func (s *HandlerTestSuite) TestUnequipItem_Success() {
	charEntity := s.fighterCharacterEntity()
	charEntity.Data.EquipmentSlots = character.EquipmentSlots{} // both slots now empty post-unequip

	s.mockService.EXPECT().
		UnequipItem(s.ctx, &orchcharacter.UnequipItemInput{
			CharacterID: s.testCharacterID,
			Slot:        character.SlotMainHand,
		}).
		Return(&orchcharacter.UnequipItemOutput{UnequippedItemID: "longsword"}, nil)

	s.mockService.EXPECT().
		GetCharacter(s.ctx, &orchcharacter.GetCharacterInput{CharacterID: s.testCharacterID}).
		Return(&orchcharacter.GetCharacterOutput{Character: charEntity}, nil)

	resp, err := s.handler.UnequipItem(s.ctx, &characterpb.UnequipItemRequest{
		CharacterId: s.testCharacterID,
		SlotKey:     "main_hand",
	})
	s.Require().NoError(err)
	s.Assert().NotContains(resp.GetCharacter().GetEquipped(), "main_hand")
}
