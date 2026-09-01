package character

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/orchestrators/character/mock"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/races"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

type HandlerTestSuite struct {
	suite.Suite
	ctrl        *gomock.Controller
	mockService *charactermock.MockService
	handler     *Handler
	ctx         context.Context
}

func (s *HandlerTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockService = charactermock.NewMockService(s.ctrl)
	s.ctx = context.Background()

	config := &HandlerConfig{
		CharacterService: s.mockService,
	}

	var err error
	s.handler, err = NewHandler(config)
	s.Require().NoError(err)
}

func (s *HandlerTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *HandlerTestSuite) TestUpdateRace_MapsDwarfToolChoice() {
	const draftID = "draft-dwarf"
	req := &dnd5ev1alpha1.UpdateRaceRequest{
		DraftId: draftID,
		Race:    dnd5ev1alpha1.Race_RACE_DWARF,
		RaceChoices: []*dnd5ev1alpha1.ChoiceData{
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_TOOLS,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_RACE,
				ChoiceId: "dwarf-tools",
				Selection: &dnd5ev1alpha1.ChoiceData_Tools{
					Tools: &dnd5ev1alpha1.ToolSelection{
						Tools: []dnd5ev1alpha1.Tool{dnd5ev1alpha1.Tool_TOOL_SMITH_TOOLS},
					},
				},
			},
		},
	}

	s.mockService.EXPECT().
		SetRace(s.ctx, &character.SetRaceInput{
			DraftID: draftID,
			Input: &toolkitchar.SetRaceInput{
				RaceID: races.Dwarf,
				Choices: toolkitchar.RaceChoices{
					Tools: []shared.SelectionID{refs.Tools.SmithTools().ID},
				},
			},
		}).
		Return(&character.SetRaceOutput{
			Draft: &toolkitchar.DraftData{ID: draftID, Race: races.Dwarf},
		}, nil)

	resp, err := s.handler.UpdateRace(s.ctx, req)

	s.Require().NoError(err)
	s.Require().NotNil(resp.GetDraft())
	s.Equal(draftID, resp.GetDraft().GetId())
	s.Equal(dnd5ev1alpha1.Race_RACE_DWARF, resp.GetDraft().GetRace())
}

func (s *HandlerTestSuite) TestDeleteCharacter_Success() {
	characterID := "char-123"
	req := &dnd5ev1alpha1.DeleteCharacterRequest{
		CharacterId: characterID,
	}

	// Mock the service call
	s.mockService.EXPECT().
		DeleteCharacter(s.ctx, &character.DeleteCharacterInput{
			CharacterID: characterID,
		}).
		Return(&character.DeleteCharacterOutput{}, nil)

	// Call the handler
	resp, err := s.handler.DeleteCharacter(s.ctx, req)

	// Assert success
	s.NoError(err)
	s.NotNil(resp)
}

func (s *HandlerTestSuite) TestDeleteCharacter_InvalidRequest() {
	testCases := []struct {
		name string
		req  *dnd5ev1alpha1.DeleteCharacterRequest
		code codes.Code
		msg  string
	}{
		{
			name: "nil request",
			req:  nil,
			code: codes.InvalidArgument,
			msg:  "request is required",
		},
		{
			name: "empty character ID",
			req:  &dnd5ev1alpha1.DeleteCharacterRequest{CharacterId: ""},
			code: codes.InvalidArgument,
			msg:  "character_id is required",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			resp, err := s.handler.DeleteCharacter(s.ctx, tc.req)

			s.Error(err)
			s.Nil(resp)

			st, ok := status.FromError(err)
			s.True(ok)
			s.Equal(tc.code, st.Code())
			s.Equal(tc.msg, st.Message())
		})
	}
}

func (s *HandlerTestSuite) TestDeleteCharacter_NotFound() {
	characterID := "char-404"
	req := &dnd5ev1alpha1.DeleteCharacterRequest{
		CharacterId: characterID,
	}

	// Mock the service to return not found
	s.mockService.EXPECT().
		DeleteCharacter(s.ctx, &character.DeleteCharacterInput{
			CharacterID: characterID,
		}).
		Return(nil, apierr.NotFound("character not found"))

	// Call the handler
	resp, err := s.handler.DeleteCharacter(s.ctx, req)

	// Assert not found error
	s.Error(err)
	s.Nil(resp)

	st, ok := status.FromError(err)
	s.True(ok)
	s.Equal(codes.NotFound, st.Code())
	s.Equal("character not found", st.Message())
}

func (s *HandlerTestSuite) TestDeleteCharacter_InvalidArgument() {
	characterID := "char-123"
	req := &dnd5ev1alpha1.DeleteCharacterRequest{
		CharacterId: characterID,
	}

	// Mock the service to return invalid argument
	s.mockService.EXPECT().
		DeleteCharacter(s.ctx, &character.DeleteCharacterInput{
			CharacterID: characterID,
		}).
		Return(nil, apierr.InvalidArgument("invalid character ID format"))

	// Call the handler
	resp, err := s.handler.DeleteCharacter(s.ctx, req)

	// Assert invalid argument error
	s.Error(err)
	s.Nil(resp)

	st, ok := status.FromError(err)
	s.True(ok)
	s.Equal(codes.InvalidArgument, st.Code())
	s.Contains(st.Message(), "invalid character ID format")
}

func (s *HandlerTestSuite) TestDeleteCharacter_InternalError() {
	characterID := "char-123"
	req := &dnd5ev1alpha1.DeleteCharacterRequest{
		CharacterId: characterID,
	}

	// Mock the service to return internal error
	s.mockService.EXPECT().
		DeleteCharacter(s.ctx, &character.DeleteCharacterInput{
			CharacterID: characterID,
		}).
		Return(nil, apierr.Internal("database error"))

	// Call the handler
	resp, err := s.handler.DeleteCharacter(s.ctx, req)

	// Assert internal error
	s.Error(err)
	s.Nil(resp)

	st, ok := status.FromError(err)
	s.True(ok)
	s.Equal(codes.Internal, st.Code())
	s.Equal("failed to delete character", st.Message())
}

func (s *HandlerTestSuite) TestGetCharacter_IncludesEquipmentSlotsAndHairAppearance() {
	characterID := "char-123"
	req := &dnd5ev1alpha1.GetCharacterRequest{CharacterId: characterID}
	appearance := handlerHairAppearance()
	charData := &toolkitchar.Data{
		ID:    characterID,
		Name:  "Test Fighter",
		Level: 1,
		EquipmentSlots: toolkitchar.EquipmentSlots{
			toolkitchar.SlotMainHand: "item-longsword",
			toolkitchar.SlotArmor:    "item-chainmail",
			toolkitchar.SlotOffHand:  "item-shield",
		},
	}

	s.mockService.EXPECT().
		GetCharacter(s.ctx, &character.GetCharacterInput{CharacterID: characterID}).
		Return(&character.GetCharacterOutput{Character: &entities.Character{
			Data: charData, Appearance: appearance,
		}}, nil)

	resp, err := s.handler.GetCharacter(s.ctx, req)

	s.NoError(err)
	s.Require().NotNil(resp)
	s.Require().NotNil(resp.Character)
	s.Require().NotNil(resp.Character.EquipmentSlots, "EquipmentSlots must be present in GetCharacter response")
	s.Require().NotNil(resp.Character.EquipmentSlots.MainHand)
	s.Equal("item-longsword", resp.Character.EquipmentSlots.MainHand.ItemId)
	s.Require().NotNil(resp.Character.EquipmentSlots.Armor)
	s.Equal("item-chainmail", resp.Character.EquipmentSlots.Armor.ItemId)
	s.Require().NotNil(resp.Character.EquipmentSlots.OffHand)
	s.Equal("item-shield", resp.Character.EquipmentSlots.OffHand.ItemId)
	s.assertHairAppearance(resp.Character.GetAppearance())
}

func (s *HandlerTestSuite) TestListCharacters_IncludesHairAppearance() {
	ctx := auth.WithPlayerID(s.ctx, "player-1")
	appearance := handlerHairAppearance()
	s.mockService.EXPECT().ListCharacters(ctx, &character.ListCharactersInput{
		PlayerID: "player-1",
	}).Return(&character.ListCharactersOutput{
		Characters: []*entities.Character{{
			Data:       &toolkitchar.Data{ID: "char-123", Name: "Test Fighter"},
			Appearance: appearance,
		}},
		TotalSize: 1,
	}, nil)

	resp, err := s.handler.ListCharacters(ctx, &dnd5ev1alpha1.ListCharactersRequest{})
	s.Require().NoError(err)
	s.Require().Len(resp.GetCharacters(), 1)
	s.assertHairAppearance(resp.GetCharacters()[0].GetAppearance())
}

func handlerHairAppearance() *entities.Appearance {
	color := uint32(0x123456)
	roughness := float32(0.33)
	return &entities.Appearance{Hair: &entities.HairCustomization{
		Scalp: &entities.StyleSelection{
			Kind:     entities.StyleSelectionKindStyle,
			StyleRef: "modular-fantasy-hero:hair:38",
		},
		FacialHair: &entities.StyleSelection{Kind: entities.StyleSelectionKindNone},
		ColorSRGB:  &color,
		Roughness:  &roughness,
	}}
}

func (s *HandlerTestSuite) assertHairAppearance(appearance *dnd5ev1alpha1.Appearance) {
	s.Require().NotNil(appearance)
	hair := appearance.GetHair()
	s.Require().NotNil(hair)
	s.Equal("modular-fantasy-hero:hair:38", hair.GetScalp().GetStyleRef())
	s.NotNil(hair.GetFacialHair().GetNone())
	s.Require().NotNil(hair.ColorSrgb)
	s.Equal(uint32(0x123456), hair.GetColorSrgb())
	s.Require().NotNil(hair.Roughness)
	s.InDelta(0.33, hair.GetRoughness(), 0.000001)
}

func (s *HandlerTestSuite) TestGetCharacter_InvalidRequest() {
	testCases := []struct {
		name string
		req  *dnd5ev1alpha1.GetCharacterRequest
		code codes.Code
		msg  string
	}{
		{
			name: "nil request",
			req:  nil,
			code: codes.InvalidArgument,
			msg:  "request is required",
		},
		{
			name: "empty character ID",
			req:  &dnd5ev1alpha1.GetCharacterRequest{CharacterId: ""},
			code: codes.InvalidArgument,
			msg:  "character_id is required",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			resp, err := s.handler.GetCharacter(s.ctx, tc.req)

			s.Error(err)
			s.Nil(resp)

			st, ok := status.FromError(err)
			s.True(ok)
			s.Equal(tc.code, st.Code())
			s.Equal(tc.msg, st.Message())
		})
	}
}

func (s *HandlerTestSuite) TestGetCharacter_ServiceError() {
	characterID := "char-404"
	req := &dnd5ev1alpha1.GetCharacterRequest{
		CharacterId: characterID,
	}

	s.mockService.EXPECT().
		GetCharacter(s.ctx, &character.GetCharacterInput{
			CharacterID: characterID,
		}).
		Return(nil, apierr.NotFound("character not found"))

	resp, err := s.handler.GetCharacter(s.ctx, req)

	s.Error(err)
	s.Nil(resp)

	st, ok := status.FromError(err)
	s.True(ok)
	s.Equal(codes.NotFound, st.Code())
	s.Equal("character not found", st.Message())
}

func (s *HandlerTestSuite) TestEquipItem_ReturnsPersistedPostStateWithoutRefetch() {
	postState := &entities.Character{
		Data: &toolkitchar.Data{
			ID:       "char-equip",
			PlayerID: "player-1",
			Name:     "Fighter",
			ClassID:  "fighter",
			RaceID:   "human",
			EquipmentSlots: toolkitchar.EquipmentSlots{
				toolkitchar.SlotMainHand: "longsword",
			},
		},
		Appearance: &entities.Appearance{
			Hair: &entities.HairCustomization{
				Scalp: &entities.StyleSelection{
					Kind:     entities.StyleSelectionKindStyle,
					StyleRef: "modular-fantasy-hero:hair:38",
				},
			},
		},
	}

	s.mockService.EXPECT().
		EquipItem(s.ctx, &character.EquipItemInput{
			CharacterID: "char-equip",
			ItemID:      "longsword",
			Slot:        toolkitchar.SlotMainHand,
		}).
		Return(&character.EquipItemOutput{
			PreviousItemID: "handaxe",
			Character:      postState,
		}, nil)
	s.mockService.EXPECT().
		GetCharacter(s.ctx, &character.GetCharacterInput{CharacterID: "char-equip"}).
		Return(nil, errors.New("post-write Get would fail")).
		MaxTimes(0)

	resp, err := s.handler.EquipItem(s.ctx, &dnd5ev1alpha1.EquipItemRequest{
		CharacterId: "char-equip",
		ItemId:      "longsword",
		Slot:        dnd5ev1alpha1.EquipmentSlot_EQUIPMENT_SLOT_MAIN_HAND,
	})
	s.Require().NoError(err)
	s.Equal("char-equip", resp.GetCharacter().GetId())
	s.Equal("longsword", resp.GetCharacter().GetEquipmentSlots().GetMainHand().GetItemId())
	s.Equal("modular-fantasy-hero:hair:38", resp.GetCharacter().GetAppearance().GetHair().GetScalp().GetStyleRef())
	s.Equal("handaxe", resp.GetPreviouslyEquippedItem().GetItemId())
}

func (s *HandlerTestSuite) TestUnequipItem_ReturnsPersistedPostStateWithoutRefetch() {
	postState := &entities.Character{
		Data: &toolkitchar.Data{
			ID:             "char-unequip",
			PlayerID:       "player-1",
			Name:           "Fighter",
			ClassID:        "fighter",
			RaceID:         "human",
			EquipmentSlots: toolkitchar.EquipmentSlots{},
		},
	}

	s.mockService.EXPECT().
		UnequipItem(s.ctx, &character.UnequipItemInput{
			CharacterID: "char-unequip",
			Slot:        toolkitchar.SlotMainHand,
		}).
		Return(&character.UnequipItemOutput{
			UnequippedItemID: "longsword",
			Character:        postState,
		}, nil)
	s.mockService.EXPECT().
		GetCharacter(s.ctx, &character.GetCharacterInput{CharacterID: "char-unequip"}).
		Return(nil, errors.New("post-write Get would fail")).
		MaxTimes(0)

	resp, err := s.handler.UnequipItem(s.ctx, &dnd5ev1alpha1.UnequipItemRequest{
		CharacterId: "char-unequip",
		Slot:        dnd5ev1alpha1.EquipmentSlot_EQUIPMENT_SLOT_MAIN_HAND,
	})
	s.Require().NoError(err)
	s.Equal("char-unequip", resp.GetCharacter().GetId())
	s.NotNil(resp.GetCharacter().GetEquipmentSlots())
	s.Nil(resp.GetCharacter().GetEquipmentSlots().GetMainHand())
}

func (s *HandlerTestSuite) TestEquipmentProjectionErrorsAreSanitized() {
	const secret = "PRIVATE_CHARACTER_JSON_MARKER"

	s.mockService.EXPECT().
		EquipItem(s.ctx, gomock.Any()).
		Return(nil, errors.New("strict character projection failed: "+secret))
	_, equipErr := s.handler.EquipItem(s.ctx, &dnd5ev1alpha1.EquipItemRequest{
		CharacterId: "char-secret",
		ItemId:      "longsword",
		Slot:        dnd5ev1alpha1.EquipmentSlot_EQUIPMENT_SLOT_MAIN_HAND,
	})
	s.Require().Error(equipErr)
	s.Equal(codes.Internal, status.Code(equipErr))
	s.Equal("character data unavailable", status.Convert(equipErr).Message())
	s.NotContains(status.Convert(equipErr).Message(), secret)

	s.mockService.EXPECT().
		UnequipItem(s.ctx, gomock.Any()).
		Return(nil, errors.New("strict character projection failed: "+secret))
	_, unequipErr := s.handler.UnequipItem(s.ctx, &dnd5ev1alpha1.UnequipItemRequest{
		CharacterId: "char-secret",
		Slot:        dnd5ev1alpha1.EquipmentSlot_EQUIPMENT_SLOT_MAIN_HAND,
	})
	s.Require().Error(unequipErr)
	s.Equal(codes.Internal, status.Code(unequipErr))
	s.Equal("character data unavailable", status.Convert(unequipErr).Message())
	s.NotContains(status.Convert(unequipErr).Message(), secret)
}

func TestHandlerSuite(t *testing.T) {
	suite.Run(t, new(HandlerTestSuite))
}
