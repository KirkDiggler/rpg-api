package character

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	charactermock "github.com/KirkDiggler/rpg-api/internal/orchestrators/character/mock"
)

type ListEquipmentTestSuite struct {
	suite.Suite
	ctrl             *gomock.Controller
	mockService      *charactermock.MockService
	handler          *Handler
	ctx              context.Context
}

func (s *ListEquipmentTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockService = charactermock.NewMockService(s.ctrl)
	s.handler = &Handler{
		characterService: s.mockService,
	}
	s.ctx = context.Background()
}

func (s *ListEquipmentTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *ListEquipmentTestSuite) TestListEquipmentByType_SimpleMeleeWeapons() {
	req := &dnd5ev1alpha1.ListEquipmentByTypeRequest{
		EquipmentType: dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_SIMPLE_MELEE_WEAPON,
	}

	resp, err := s.handler.ListEquipmentByType(s.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Greater(len(resp.Equipment), 0)

	// Check that we got simple melee weapons
	for _, equip := range resp.Equipment {
		s.NotEmpty(equip.Id)
		s.NotEmpty(equip.Name)
		s.Equal(dnd5ev1alpha1.EquipmentCategory_EQUIPMENT_CATEGORY_SIMPLE_WEAPON, equip.Category)

		// Verify weapon data exists
		weaponData := equip.GetWeaponData()
		s.NotNil(weaponData)
		s.Equal(dnd5ev1alpha1.WeaponCategory_WEAPON_CATEGORY_SIMPLE, weaponData.WeaponCategory)
		// Some melee weapons can be thrown so they have ranged capability
		s.Contains([]string{"melee", "ranged"}, weaponData.Range)
	}
}

func (s *ListEquipmentTestSuite) TestListEquipmentByType_MartialRangedWeapons() {
	req := &dnd5ev1alpha1.ListEquipmentByTypeRequest{
		EquipmentType: dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_MARTIAL_RANGED_WEAPON,
	}

	resp, err := s.handler.ListEquipmentByType(s.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Greater(len(resp.Equipment), 0)

	// Check that we got martial ranged weapons
	for _, equip := range resp.Equipment {
		s.NotEmpty(equip.Id)
		s.NotEmpty(equip.Name)
		s.Equal(dnd5ev1alpha1.EquipmentCategory_EQUIPMENT_CATEGORY_MARTIAL_WEAPON, equip.Category)

		// Verify weapon data exists
		weaponData := equip.GetWeaponData()
		s.NotNil(weaponData)
		s.Equal(dnd5ev1alpha1.WeaponCategory_WEAPON_CATEGORY_MARTIAL, weaponData.WeaponCategory)
		s.Equal("ranged", weaponData.Range)
		s.Greater(weaponData.NormalRange, int32(0))
	}
}

func (s *ListEquipmentTestSuite) TestListEquipmentByType_UnsupportedType() {
	req := &dnd5ev1alpha1.ListEquipmentByTypeRequest{
		EquipmentType: dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_LIGHT_ARMOR,
	}

	resp, err := s.handler.ListEquipmentByType(s.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(0, len(resp.Equipment))
	s.Equal(int32(0), resp.TotalSize)
}

func TestListEquipmentTestSuite(t *testing.T) {
	suite.Run(t, new(ListEquipmentTestSuite))
}