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
	"github.com/KirkDiggler/rpg-api/internal/auth"
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
	testPlayerID    string
}

func TestHandlerSuite(t *testing.T) {
	suite.Run(t, new(HandlerTestSuite))
}

func (s *HandlerTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockService = charactermock.NewMockService(s.ctrl)
	s.ctx = context.Background()
	s.testCharacterID = "char-fighter-1"
	s.testPlayerID = "alice"

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
			PlayerID:         s.testPlayerID,
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
	ctx := auth.WithPlayerID(s.ctx, s.testPlayerID)
	charEntity := s.fighterCharacterEntity()

	// Fetched twice: once by verifyCallerOwnsCharacter's pre-write ownership
	// gate, once by recomputedCharacterData after the write. Same entity both
	// times in this fixture -- the mutation lives in the toolkit, not here.
	s.mockService.EXPECT().
		GetCharacter(ctx, &orchcharacter.GetCharacterInput{CharacterID: s.testCharacterID}).
		Return(&orchcharacter.GetCharacterOutput{Character: charEntity}, nil).
		Times(2)

	s.mockService.EXPECT().
		EquipItem(ctx, &orchcharacter.EquipItemInput{
			CharacterID: s.testCharacterID,
			ItemID:      "longsword",
			Slot:        character.SlotMainHand,
		}).
		Return(&orchcharacter.EquipItemOutput{}, nil)

	resp, err := s.handler.EquipItem(ctx, &characterpb.EquipItemRequest{
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

func (s *HandlerTestSuite) TestEquipItem_Unauthenticated_Errors() {
	// No auth.WithPlayerID on the context, and no mock expectations at all:
	// the ownership gate must refuse before GetCharacter or EquipItem are
	// ever called.
	_, err := s.handler.EquipItem(s.ctx, &characterpb.EquipItemRequest{
		CharacterId: s.testCharacterID,
		Item:        &encounterv2pb.Ref{Id: "longsword"},
		SlotKey:     "main_hand",
	})
	s.Require().Error(err)
	s.Assert().Equal(codes.Unauthenticated, status.Code(err))
}

// TestEquipItem_ForeignCharacter_NotFound pins rpg-api#814's ruling: a caller
// naming a character it does not control never reaches the write. No
// EquipItem mock expectation is set, so gomock fails the test if the guard
// lets the call through.
func (s *HandlerTestSuite) TestEquipItem_ForeignCharacter_NotFound() {
	ctx := auth.WithPlayerID(s.ctx, s.testPlayerID)
	charEntity := s.fighterCharacterEntity()
	charEntity.Data.PlayerID = "someone-else"

	s.mockService.EXPECT().
		GetCharacter(ctx, &orchcharacter.GetCharacterInput{CharacterID: s.testCharacterID}).
		Return(&orchcharacter.GetCharacterOutput{Character: charEntity}, nil)

	_, err := s.handler.EquipItem(ctx, &characterpb.EquipItemRequest{
		CharacterId: s.testCharacterID,
		Item:        &encounterv2pb.Ref{Id: "longsword"},
		SlotKey:     "main_hand",
	})
	s.Require().Error(err)
	s.Assert().Equal(codes.NotFound, status.Code(err))
}

func (s *HandlerTestSuite) TestEquipItem_OrchestratorError_PropagatesAsNotFound() {
	ctx := auth.WithPlayerID(s.ctx, s.testPlayerID)
	charEntity := s.fighterCharacterEntity()

	s.mockService.EXPECT().
		GetCharacter(ctx, &orchcharacter.GetCharacterInput{CharacterID: s.testCharacterID}).
		Return(&orchcharacter.GetCharacterOutput{Character: charEntity}, nil)

	s.mockService.EXPECT().
		EquipItem(ctx, gomock.Any()).
		Return(nil, apierr.NotFound("item not found in inventory"))

	_, err := s.handler.EquipItem(ctx, &characterpb.EquipItemRequest{
		CharacterId: s.testCharacterID,
		Item:        &encounterv2pb.Ref{Id: "not-owned"},
		SlotKey:     "main_hand",
	})
	s.Require().Error(err)
	s.Assert().Equal(codes.NotFound, status.Code(err))
}

func (s *HandlerTestSuite) TestEquipItem_OrchestratorGenericError_PropagatesAsInternal() {
	ctx := auth.WithPlayerID(s.ctx, s.testPlayerID)
	charEntity := s.fighterCharacterEntity()

	s.mockService.EXPECT().
		GetCharacter(ctx, &orchcharacter.GetCharacterInput{CharacterID: s.testCharacterID}).
		Return(&orchcharacter.GetCharacterOutput{Character: charEntity}, nil)

	s.mockService.EXPECT().
		EquipItem(ctx, gomock.Any()).
		Return(nil, errors.New("boom"))

	_, err := s.handler.EquipItem(ctx, &characterpb.EquipItemRequest{
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
	ctx := auth.WithPlayerID(s.ctx, s.testPlayerID)
	preEntity := s.fighterCharacterEntity() // still equipped, read by the ownership pre-check
	postEntity := s.fighterCharacterEntity()
	postEntity.Data.EquipmentSlots = character.EquipmentSlots{} // both slots now empty post-unequip

	// InOrder rather than Times(2): unlike EquipItem's success test, the two
	// reads here must return DIFFERENT states (before and after the write),
	// so which call gets which return value is load-bearing.
	gomock.InOrder(
		s.mockService.EXPECT().
			GetCharacter(ctx, &orchcharacter.GetCharacterInput{CharacterID: s.testCharacterID}).
			Return(&orchcharacter.GetCharacterOutput{Character: preEntity}, nil),
		s.mockService.EXPECT().
			GetCharacter(ctx, &orchcharacter.GetCharacterInput{CharacterID: s.testCharacterID}).
			Return(&orchcharacter.GetCharacterOutput{Character: postEntity}, nil),
	)

	s.mockService.EXPECT().
		UnequipItem(ctx, &orchcharacter.UnequipItemInput{
			CharacterID: s.testCharacterID,
			Slot:        character.SlotMainHand,
		}).
		Return(&orchcharacter.UnequipItemOutput{UnequippedItemID: "longsword"}, nil)

	resp, err := s.handler.UnequipItem(ctx, &characterpb.UnequipItemRequest{
		CharacterId: s.testCharacterID,
		SlotKey:     "main_hand",
	})
	s.Require().NoError(err)
	s.Assert().NotContains(resp.GetCharacter().GetEquipped(), "main_hand")
}

func (s *HandlerTestSuite) TestUnequipItem_Unauthenticated_Errors() {
	_, err := s.handler.UnequipItem(s.ctx, &characterpb.UnequipItemRequest{
		CharacterId: s.testCharacterID,
		SlotKey:     "main_hand",
	})
	s.Require().Error(err)
	s.Assert().Equal(codes.Unauthenticated, status.Code(err))
}

// TestUnequipItem_ForeignCharacter_NotFound is UnequipItem's half of
// rpg-api#814's ruling -- see TestEquipItem_ForeignCharacter_NotFound.
func (s *HandlerTestSuite) TestUnequipItem_ForeignCharacter_NotFound() {
	ctx := auth.WithPlayerID(s.ctx, s.testPlayerID)
	charEntity := s.fighterCharacterEntity()
	charEntity.Data.PlayerID = "someone-else"

	s.mockService.EXPECT().
		GetCharacter(ctx, &orchcharacter.GetCharacterInput{CharacterID: s.testCharacterID}).
		Return(&orchcharacter.GetCharacterOutput{Character: charEntity}, nil)

	_, err := s.handler.UnequipItem(ctx, &characterpb.UnequipItemRequest{
		CharacterId: s.testCharacterID,
		SlotKey:     "main_hand",
	})
	s.Require().Error(err)
	s.Assert().Equal(codes.NotFound, status.Code(err))
}

func (s *HandlerTestSuite) TestGetCharacterData_MissingCharacterID_InvalidArgument() {
	ctx := auth.WithPlayerID(s.ctx, s.testPlayerID)
	_, err := s.handler.GetCharacterData(ctx, &characterpb.GetCharacterDataRequest{})
	s.Require().Error(err)
	s.Assert().Equal(codes.InvalidArgument, status.Code(err))
}

func (s *HandlerTestSuite) TestGetCharacterData_Unauthenticated_Errors() {
	// No auth.WithPlayerID on the context, and no mock expectation: the
	// entitlement gate must refuse before the orchestrator is ever called.
	_, err := s.handler.GetCharacterData(s.ctx, &characterpb.GetCharacterDataRequest{
		CharacterId: s.testCharacterID,
	})
	s.Require().Error(err)
	s.Assert().Equal(codes.Unauthenticated, status.Code(err))
}

func (s *HandlerTestSuite) TestGetCharacterData_Success() {
	ctx := auth.WithPlayerID(s.ctx, s.testPlayerID)
	charEntity := s.fighterCharacterEntity()

	s.mockService.EXPECT().
		GetCharacter(ctx, &orchcharacter.GetCharacterInput{CharacterID: s.testCharacterID}).
		Return(&orchcharacter.GetCharacterOutput{Character: charEntity}, nil)

	resp, err := s.handler.GetCharacterData(ctx, &characterpb.GetCharacterDataRequest{
		CharacterId: s.testCharacterID,
	})
	s.Require().NoError(err)

	cd := resp.GetCharacter()
	s.Require().NotNil(cd)
	s.Require().NotNil(cd.GetArmorClassDetail(), "armor_class_detail is the only AC total on this response")
	s.Assert().NotZero(cd.GetArmorClassDetail().GetTotal(), "AC must be the real toolkit-computed total, not zero")
	s.Assert().Contains(cd.GetEquipped(), "main_hand")
	s.Assert().Equal("longsword", cd.GetEquipped()["main_hand"].GetId())
	s.Assert().Len(cd.GetInventory(), 2, "inventory includes equipped items, per the wire contract")
}

// TestGetCharacterData_ForeignCharacter_NotFound pins the deliberate choice
// of NOT_FOUND over PERMISSION_DENIED: this response hands back the
// character's full sheet, so the wrong-owner case must read identically to
// "no such character" on the wire -- a PERMISSION_DENIED would itself leak
// that a character exists at that id.
func (s *HandlerTestSuite) TestGetCharacterData_ForeignCharacter_NotFound() {
	ctx := auth.WithPlayerID(s.ctx, s.testPlayerID)
	charEntity := s.fighterCharacterEntity()
	charEntity.Data.PlayerID = "someone-else"

	s.mockService.EXPECT().
		GetCharacter(ctx, &orchcharacter.GetCharacterInput{CharacterID: s.testCharacterID}).
		Return(&orchcharacter.GetCharacterOutput{Character: charEntity}, nil)

	_, err := s.handler.GetCharacterData(ctx, &characterpb.GetCharacterDataRequest{
		CharacterId: s.testCharacterID,
	})
	s.Require().Error(err)
	s.Assert().Equal(codes.NotFound, status.Code(err))
}

func (s *HandlerTestSuite) TestGetCharacterData_OrchestratorError_PropagatesAsNotFound() {
	ctx := auth.WithPlayerID(s.ctx, s.testPlayerID)
	s.mockService.EXPECT().
		GetCharacter(ctx, gomock.Any()).
		Return(nil, apierr.NotFound("character not found"))

	_, err := s.handler.GetCharacterData(ctx, &characterpb.GetCharacterDataRequest{
		CharacterId: s.testCharacterID,
	})
	s.Require().Error(err)
	s.Assert().Equal(codes.NotFound, status.Code(err))
}

func (s *HandlerTestSuite) TestGetCharacterData_OrchestratorGenericError_PropagatesAsInternal() {
	ctx := auth.WithPlayerID(s.ctx, s.testPlayerID)
	s.mockService.EXPECT().
		GetCharacter(ctx, gomock.Any()).
		Return(nil, errors.New("boom"))

	_, err := s.handler.GetCharacterData(ctx, &characterpb.GetCharacterDataRequest{
		CharacterId: s.testCharacterID,
	})
	s.Require().Error(err)
	s.Assert().Equal(codes.Internal, status.Code(err))
}

// TestVerifyCallerOwnsCharacter_MissingAndForeign_IdenticalNotFoundText pins
// the fix for rpg-api#815's Copilot finding: a missing character and a
// foreign one must be BYTE-IDENTICAL on the wire, never distinguishable by
// message text -- the repository's own not-found wording never reaches the
// caller, only this handler's own canonical "character %q not found".
// Exercised through GetCharacterData since verifyCallerOwnsCharacter is the
// one gate all three RPCs share; EquipItem/UnequipItem go through the exact
// same function.
func (s *HandlerTestSuite) TestVerifyCallerOwnsCharacter_MissingAndForeign_IdenticalNotFoundText() {
	ctx := auth.WithPlayerID(s.ctx, s.testPlayerID)

	missingCtrl := gomock.NewController(s.T())
	missingSvc := charactermock.NewMockService(missingCtrl)
	missingSvc.EXPECT().
		GetCharacter(ctx, &orchcharacter.GetCharacterInput{CharacterID: s.testCharacterID}).
		Return(nil, apierr.NotFound("no character with that id in the repository")) // deliberately DIFFERENT wording
	missingHandler, err := New(&HandlerConfig{CharacterService: missingSvc})
	s.Require().NoError(err)
	_, missingErr := missingHandler.GetCharacterData(ctx, &characterpb.GetCharacterDataRequest{CharacterId: s.testCharacterID})

	foreignCtrl := gomock.NewController(s.T())
	foreignSvc := charactermock.NewMockService(foreignCtrl)
	foreignEntity := s.fighterCharacterEntity()
	foreignEntity.Data.PlayerID = "someone-else"
	foreignSvc.EXPECT().
		GetCharacter(ctx, &orchcharacter.GetCharacterInput{CharacterID: s.testCharacterID}).
		Return(&orchcharacter.GetCharacterOutput{Character: foreignEntity}, nil)
	foreignHandler, err := New(&HandlerConfig{CharacterService: foreignSvc})
	s.Require().NoError(err)
	_, foreignErr := foreignHandler.GetCharacterData(ctx, &characterpb.GetCharacterDataRequest{CharacterId: s.testCharacterID})

	s.Require().Error(missingErr)
	s.Require().Error(foreignErr)
	missingSt, ok := status.FromError(missingErr)
	s.Require().True(ok)
	foreignSt, ok := status.FromError(foreignErr)
	s.Require().True(ok)

	s.Assert().Equal(codes.NotFound, missingSt.Code())
	s.Assert().Equal(codes.NotFound, foreignSt.Code())
	s.Assert().Equal(missingSt.Message(), foreignSt.Message(),
		"a missing character and a foreign one must be indistinguishable by message text")
	s.Assert().NotContains(missingSt.Message(), "repository",
		"the repository's own wording must never reach the caller")
}
