package character

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	characterpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/character"
	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	orchcharacter "github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/orchestrators/character/mock"
	coreResources "github.com/KirkDiggler/rpg-toolkit/core/resources"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/combat"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/conditions"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/currency"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/features"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/races"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/resources"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/weapons"
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

// fighterCharacterEntity is a persisted level-3 Fighter carrying equipment,
// feature-private resources, a condition, and legacy/magic rows that the
// no-magic StatusView must exclude.
func (s *HandlerTestSuite) fighterCharacterEntity() *entities.Character {
	return &entities.Character{
		Data: &character.Data{
			ID:               s.testCharacterID,
			PlayerID:         s.testPlayerID,
			Name:             "Test Fighter",
			Level:            3,
			RaceID:           races.Human,
			ClassID:          classes.Fighter,
			ProficiencyBonus: 2,
			HitPoints:        20,
			MaxHitPoints:     30,
			ArmorClass:       10,
			Wallet:           currency.FromGold(15),
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
			Features: []json.RawMessage{
				s.mustJSON(features.SecondWindData{
					Ref: refs.Features.SecondWind(), ID: "second-wind", Name: "Second Wind",
					Level: 3, CharacterID: s.testCharacterID, Uses: 1, MaxUses: 1,
				}),
				s.mustJSON(features.ActionSurgeData{
					Ref: refs.Features.ActionSurge(), ID: "action-surge", Name: "Action Surge",
					CharacterID: s.testCharacterID, Uses: 1, MaxUses: 1,
				}),
			},
			Conditions: []json.RawMessage{
				s.mustJSON(conditions.FightingStyleDefenseData{
					Ref: refs.Conditions.FightingStyleDefense(), MemberID: s.testCharacterID,
				}),
			},
			Resources: map[coreResources.ResourceKey]character.RecoverableResourceData{
				resources.HitDice: {Current: 2, Maximum: 3, ResetType: coreResources.ResetLongRest},
			},
			SpellSlots: map[int]character.SpellSlotData{1: {Max: 2}},
			ClassResources: map[shared.ClassResourceType]character.ResourceData{
				shared.ClassResourceType(99): {Name: "legacy-resource", Current: 1, Max: 1},
			},
		},
	}
}

func (s *HandlerTestSuite) mustJSON(value any) json.RawMessage {
	raw, err := json.Marshal(value)
	s.Require().NoError(err)
	return raw
}

func (s *HandlerTestSuite) project(data *character.Data) *orchcharacter.View {
	out, err := orchcharacter.ProjectView(s.ctx, &orchcharacter.ProjectViewInput{Data: data})
	s.Require().NoError(err)
	s.Require().NotNil(out)
	return out.View
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

	// Ownership is fetched once before the write. The orchestrator returns the
	// already-composed post-view, so no post-write reload can fail.
	s.mockService.EXPECT().
		GetCharacter(ctx, &orchcharacter.GetCharacterInput{CharacterID: s.testCharacterID}).
		Return(&orchcharacter.GetCharacterOutput{Character: charEntity}, nil)

	postView := s.project(charEntity.Data)
	s.mockService.EXPECT().
		EquipItem(ctx, &orchcharacter.EquipItemInput{
			CharacterID: s.testCharacterID,
			ItemID:      "longsword",
			Slot:        character.SlotMainHand,
		}).
		Return(&orchcharacter.EquipItemOutput{Character: charEntity, View: postView}, nil)

	resp, err := s.handler.EquipItem(ctx, &characterpb.EquipItemRequest{
		CharacterId: s.testCharacterID,
		Item:        &encounterv2pb.Ref{Module: "dnd5e", Type: "item", Id: "longsword"},
		SlotKey:     "main_hand",
	})
	s.Require().NoError(err)
	s.Require().NotNil(resp.GetCharacter())

	cd := resp.GetCharacter()
	s.assertOwnerIdentity(cd, classes.Fighter, races.Human)
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
	preEntity := s.fighterCharacterEntity()
	postEntity := s.fighterCharacterEntity()
	postEntity.Data.EquipmentSlots = character.EquipmentSlots{}

	s.mockService.EXPECT().
		GetCharacter(ctx, &orchcharacter.GetCharacterInput{CharacterID: s.testCharacterID}).
		Return(&orchcharacter.GetCharacterOutput{Character: preEntity}, nil)

	s.mockService.EXPECT().
		UnequipItem(ctx, &orchcharacter.UnequipItemInput{
			CharacterID: s.testCharacterID,
			Slot:        character.SlotMainHand,
		}).
		Return(&orchcharacter.UnequipItemOutput{
			UnequippedItemID: "longsword",
			Character:        postEntity,
			View:             s.project(postEntity.Data),
		}, nil)

	resp, err := s.handler.UnequipItem(ctx, &characterpb.UnequipItemRequest{
		CharacterId: s.testCharacterID,
		SlotKey:     "main_hand",
	})
	s.Require().NoError(err)
	s.assertOwnerIdentity(resp.GetCharacter(), classes.Fighter, races.Human)
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
	s.assertOwnerIdentity(cd, classes.Fighter, races.Human)
	s.Require().NotNil(cd.GetArmorClassDetail(), "armor_class_detail is the only AC total on this response")
	s.Assert().NotZero(cd.GetArmorClassDetail().GetTotal(), "AC must be the real toolkit-computed total, not zero")
	s.Assert().Contains(cd.GetEquipped(), "main_hand")
	s.Assert().Equal("longsword", cd.GetEquipped()["main_hand"].GetId())
	s.Assert().Len(cd.GetInventory(), 2, "inventory includes equipped items, per the wire contract")
	s.Assert().Equal(int32(3), cd.GetLevel())
	s.Require().NotNil(cd.GetHitPoints())
	s.Assert().Equal(int32(20), cd.GetHitPoints().GetCurrent())
	s.Assert().Equal(int32(30), cd.GetHitPoints().GetMax())
	s.Assert().Zero(cd.GetHitPoints().GetTemp())
	s.Assert().Equal(int32(30), cd.GetBaseSpeedFeet())

	// Wallet visibility (rpg-toolkit#1533): the owner's persistent purse
	// reaches this owner-private response, gated by the same
	// verifyCallerOwnsCharacter check every field here already runs through.
	s.Require().NotNil(cd.GetWallet())
	s.Assert().Equal(int32(1500), cd.GetWallet().GetCopper(), "15 gp")

	s.Require().Len(cd.GetFeatures(), 2)
	s.Assert().Equal("dnd5e", cd.GetFeatures()[0].GetRef().GetModule())
	s.Assert().Equal("features", cd.GetFeatures()[0].GetRef().GetType())
	s.Assert().Equal("action_surge", cd.GetFeatures()[0].GetRef().GetId())
	s.Require().NotNil(cd.GetFeatures()[0].ResourceKey)
	s.Assert().Equal(string(resources.ActionSurge), cd.GetFeatures()[0].GetResourceKey())

	s.Require().Len(cd.GetConditions(), 1)
	s.Assert().Equal("dnd5e", cd.GetConditions()[0].GetRef().GetModule())
	s.Assert().Equal("conditions", cd.GetConditions()[0].GetRef().GetType())
	s.Assert().Equal("fighting_style_defense", cd.GetConditions()[0].GetRef().GetId())
	s.Assert().Nil(cd.GetConditions()[0].SourceMember)

	s.Require().Len(cd.GetResources(), 3)
	for _, resource := range cd.GetResources() {
		s.Assert().NotEqual("spell_slots", resource.GetKey())
		s.Assert().NotEqual("legacy-resource", resource.GetKey())
	}
}

func (s *HandlerTestSuite) TestBuildCharacterData_MapsQuantityAndAuthoritativeEquipmentSlots() {
	cd := BuildCharacterData(&orchcharacter.View{
		Equipment: &character.EquipmentView{
			Items: []character.EquippedItemView{{
				ItemID:   weapons.Handaxe,
				Name:     "Handaxe",
				Kind:     "weapon",
				SlotKeys: []string{string(character.SlotMainHand), string(character.SlotOffHand)},
				StatLine: "1d6 slashing",
				Quantity: 2,
			}},
			Equipped: character.EquipmentSlots{
				character.SlotMainHand: weapons.Handaxe,
				character.SlotOffHand:  weapons.Handaxe,
			},
		},
	})

	s.Require().Len(cd.GetInventory(), 1)
	s.Equal(int32(2), cd.GetInventory()[0].GetQuantity())
	s.Equal(weapons.Handaxe, cd.GetEquipped()[string(character.SlotMainHand)].GetId())
	s.Equal(weapons.Handaxe, cd.GetEquipped()[string(character.SlotOffHand)].GetId())
}

func (s *HandlerTestSuite) TestBuildCharacterData_FourBuildStatusMapping() {
	source := "party-member-2"
	tests := []struct {
		name       string
		classID    classes.Class
		raceID     races.Race
		status     *character.StatusView
		featureRef string
		resource   string
	}{
		{
			name:    "fighter",
			classID: classes.Fighter,
			raceID:  races.Human,
			status: &character.StatusView{
				Features: []character.FeatureView{{
					Ref: *refs.Features.SecondWind(), Name: "Second Wind",
					ResourceKey: resourceKey(resources.SecondWind),
				}},
				Conditions: []character.ConditionView{{
					Ref: *refs.Conditions.FightingStyleDefense(), Name: "Defense",
				}},
				Resources: []character.ResourceView{{Key: resources.SecondWind, Name: "Second Wind", Current: 1, Maximum: 1}},
			},
			featureRef: refs.Features.SecondWind().String(),
			resource:   string(resources.SecondWind),
		},
		{
			name:    "barbarian",
			classID: classes.Barbarian,
			raceID:  races.HalfOrc,
			status: &character.StatusView{
				Features:   []character.FeatureView{{Ref: *refs.Features.Rage(), Name: "Rage", ResourceKey: resourceKey(resources.RageCharges)}},
				Conditions: []character.ConditionView{{Ref: *refs.Conditions.Raging(), Name: "Raging", SourceMember: &source}},
				Resources:  []character.ResourceView{{Key: resources.RageCharges, Name: "Rage", Current: 2, Maximum: 3}},
			},
			featureRef: refs.Features.Rage().String(),
			resource:   string(resources.RageCharges),
		},
		{
			name:    "monk",
			classID: classes.Monk,
			raceID:  races.Human,
			status: &character.StatusView{
				Features:   []character.FeatureView{{Ref: *refs.Features.FlurryOfBlows(), Name: "Flurry of Blows", ResourceKey: resourceKey(resources.Ki)}},
				Conditions: []character.ConditionView{{Ref: *refs.Conditions.MartialArts(), Name: "Martial Arts"}},
				Resources:  []character.ResourceView{{Key: resources.Ki, Name: "Ki", Current: 2, Maximum: 3}},
			},
			featureRef: refs.Features.FlurryOfBlows().String(),
			resource:   string(resources.Ki),
		},
		{
			name:    "rogue",
			classID: classes.Rogue,
			raceID:  races.Halfling,
			status: &character.StatusView{
				Features:   []character.FeatureView{{Ref: *refs.Features.SneakAttack(), Name: "Sneak Attack"}},
				Conditions: []character.ConditionView{{Ref: *refs.Features.SneakAttack(), Name: "Sneak Attack"}},
				Resources:  []character.ResourceView{{Key: resources.HitDice, Name: "Hit Dice", Current: 2, Maximum: 3}},
			},
			featureRef: refs.Features.SneakAttack().String(),
			resource:   string(resources.HitDice),
		},
	}

	for _, tc := range tests {
		s.Run(tc.name, func() {
			cd := BuildCharacterData(&orchcharacter.View{
				Identity: orchcharacter.IdentityView{PlayerID: s.testPlayerID, ClassID: tc.classID, RaceID: tc.raceID},
				Status:   tc.status,
			})
			s.assertOwnerIdentity(cd, tc.classID, tc.raceID)
			s.Require().Len(cd.GetFeatures(), 1)
			s.Equal(tc.featureRef, protoRefString(cd.GetFeatures()[0].GetRef()))
			s.Require().Len(cd.GetConditions(), 1)
			s.Require().Len(cd.GetResources(), 1)
			s.Equal(tc.resource, cd.GetResources()[0].GetKey())
			if tc.name == "barbarian" {
				s.Require().NotNil(cd.GetConditions()[0].SourceMember)
				s.Equal(source, cd.GetConditions()[0].GetSourceMember())
			}
		})
	}
}

func (s *HandlerTestSuite) TestStrictProjectionFailuresAreSanitized() {
	const secret = "PRIVATE_CHARACTER_JSON_MARKER"
	ctx := auth.WithPlayerID(s.ctx, s.testPlayerID)

	s.Run("get", func() {
		entity := s.fighterCharacterEntity()
		entity.Data.Features = []json.RawMessage{json.RawMessage(`{"ref":"` + secret)}
		s.mockService.EXPECT().
			GetCharacter(ctx, &orchcharacter.GetCharacterInput{CharacterID: s.testCharacterID}).
			Return(&orchcharacter.GetCharacterOutput{Character: entity}, nil)

		_, err := s.handler.GetCharacterData(ctx, &characterpb.GetCharacterDataRequest{CharacterId: s.testCharacterID})
		s.assertSanitizedCharacterDataError(err, secret)
	})

	s.Run("equip", func() {
		entity := s.fighterCharacterEntity()
		s.mockService.EXPECT().
			GetCharacter(ctx, &orchcharacter.GetCharacterInput{CharacterID: s.testCharacterID}).
			Return(&orchcharacter.GetCharacterOutput{Character: entity}, nil)
		s.mockService.EXPECT().
			EquipItem(ctx, gomock.Any()).
			Return(nil, errors.New("strict projection failed: "+secret))

		_, err := s.handler.EquipItem(ctx, &characterpb.EquipItemRequest{
			CharacterId: s.testCharacterID,
			Item:        &encounterv2pb.Ref{Id: "longsword"},
			SlotKey:     "main_hand",
		})
		s.assertSanitizedCharacterDataError(err, secret)
	})

	s.Run("unequip", func() {
		entity := s.fighterCharacterEntity()
		s.mockService.EXPECT().
			GetCharacter(ctx, &orchcharacter.GetCharacterInput{CharacterID: s.testCharacterID}).
			Return(&orchcharacter.GetCharacterOutput{Character: entity}, nil)
		s.mockService.EXPECT().
			UnequipItem(ctx, gomock.Any()).
			Return(nil, errors.New("strict projection failed: "+secret))

		_, err := s.handler.UnequipItem(ctx, &characterpb.UnequipItemRequest{
			CharacterId: s.testCharacterID,
			SlotKey:     "main_hand",
		})
		s.assertSanitizedCharacterDataError(err, secret)
	})
}

func (s *HandlerTestSuite) assertSanitizedCharacterDataError(err error, secret string) {
	s.Require().Error(err)
	s.Equal(codes.Internal, status.Code(err))
	s.Equal("character data unavailable", status.Convert(err).Message())
	s.NotContains(status.Convert(err).Message(), secret)
}

func (s *HandlerTestSuite) TestGetCharacterData_OwnerMalformedCharacterIsInternal() {
	ctx := auth.WithPlayerID(s.ctx, s.testPlayerID)
	charEntity := s.fighterCharacterEntity()
	charEntity.Data.Inventory = append(charEntity.Data.Inventory,
		character.InventoryItemData{Type: "item", ID: "vorpal-spork", Quantity: 1})

	s.mockService.EXPECT().
		GetCharacter(ctx, &orchcharacter.GetCharacterInput{CharacterID: s.testCharacterID}).
		Return(&orchcharacter.GetCharacterOutput{Character: charEntity}, nil)

	_, err := s.handler.GetCharacterData(ctx, &characterpb.GetCharacterDataRequest{CharacterId: s.testCharacterID})
	s.Require().Error(err)
	s.Equal(codes.Internal, status.Code(err))
}

func (s *HandlerTestSuite) TestGetCharacterData_ForeignMalformedCharacterStillNotFound() {
	ctx := auth.WithPlayerID(s.ctx, s.testPlayerID)
	charEntity := s.fighterCharacterEntity()
	charEntity.Data.PlayerID = "someone-else"
	charEntity.Data.Inventory = append(charEntity.Data.Inventory,
		character.InventoryItemData{Type: "item", ID: "vorpal-spork", Quantity: 1})

	s.mockService.EXPECT().
		GetCharacter(ctx, &orchcharacter.GetCharacterInput{CharacterID: s.testCharacterID}).
		Return(&orchcharacter.GetCharacterOutput{Character: charEntity}, nil)

	_, err := s.handler.GetCharacterData(ctx, &characterpb.GetCharacterDataRequest{CharacterId: s.testCharacterID})
	s.Require().Error(err)
	s.Equal(codes.NotFound, status.Code(err), "ownership must be gated before private projection")
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

func (s *HandlerTestSuite) TestBuildCharacterData_MapsExplicitDeadLifeStateAndProviderProgress() {
	cd := BuildCharacterData(&orchcharacter.View{Status: &character.StatusView{
		LifeState: combat.LifeStateDead,
		DeathSaves: &character.DeathSaveProgress{
			Successes: 1, Failures: 3, SuccessesNeeded: 2, FailuresRemaining: 0,
			Dead: true,
		},
	}})

	s.Require().NotNil(cd)
	s.Equal(sessionpb.LifeState_LIFE_STATE_DEAD, cd.GetLifeState())
	s.Require().NotNil(cd.GetDeathSaves(), "owner projection retains Dead progress from the provider view")
	s.Equal(int32(1), cd.GetDeathSaves().GetSuccesses())
	s.Equal(int32(3), cd.GetDeathSaves().GetFailures())
	s.Equal(int32(2), cd.GetDeathSaves().GetSuccessesNeeded())
	s.Zero(cd.GetDeathSaves().GetFailuresRemaining())
	s.True(cd.GetDeathSaves().GetDead())
}

func (s *HandlerTestSuite) assertOwnerIdentity(
	data *encounterv2pb.CharacterData,
	classID classes.Class,
	raceID races.Race,
) {
	s.Require().NotNil(data)
	s.Equal(s.testPlayerID, data.GetPlayerId())
	s.Equal("dnd5e:class:"+classID, protoRefString(data.GetClassRef()))
	s.Equal("dnd5e:race:"+string(raceID), protoRefString(data.GetRaceRef()))
}

func resourceKey(key coreResources.ResourceKey) *coreResources.ResourceKey {
	return &key
}

func protoRefString(ref *encounterv2pb.Ref) string {
	if ref == nil {
		return ""
	}
	return ref.GetModule() + ":" + ref.GetType() + ":" + ref.GetId()
}
