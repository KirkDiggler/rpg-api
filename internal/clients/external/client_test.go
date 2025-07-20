package external

import (
	"context"
	"errors"
	"testing"

	"github.com/fadedpez/dnd5e-api/clients/dnd5e"
	"github.com/fadedpez/dnd5e-api/entities"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// mockDND5eClient is a mock implementation of the dnd5e.Interface for testing
type mockDND5eClient struct {
	mock.Mock
}

func (m *mockDND5eClient) ListRaces() ([]*entities.ReferenceItem, error) {
	args := m.Called()
	return args.Get(0).([]*entities.ReferenceItem), args.Error(1)
}

func (m *mockDND5eClient) GetRace(key string) (*entities.Race, error) {
	args := m.Called(key)
	return args.Get(0).(*entities.Race), args.Error(1)
}

func (m *mockDND5eClient) ListEquipment() ([]*entities.ReferenceItem, error) {
	args := m.Called()
	return args.Get(0).([]*entities.ReferenceItem), args.Error(1)
}

func (m *mockDND5eClient) GetEquipment(key string) (dnd5e.EquipmentInterface, error) {
	args := m.Called(key)
	return args.Get(0).(dnd5e.EquipmentInterface), args.Error(1)
}

func (m *mockDND5eClient) GetEquipmentCategory(key string) (*entities.EquipmentCategory, error) {
	args := m.Called(key)
	return args.Get(0).(*entities.EquipmentCategory), args.Error(1)
}

func (m *mockDND5eClient) ListClasses() ([]*entities.ReferenceItem, error) {
	args := m.Called()
	return args.Get(0).([]*entities.ReferenceItem), args.Error(1)
}

func (m *mockDND5eClient) GetClass(key string) (*entities.Class, error) {
	args := m.Called(key)
	return args.Get(0).(*entities.Class), args.Error(1)
}

func (m *mockDND5eClient) ListSpells(input *dnd5e.ListSpellsInput) ([]*entities.ReferenceItem, error) {
	args := m.Called(input)
	return args.Get(0).([]*entities.ReferenceItem), args.Error(1)
}

func (m *mockDND5eClient) GetSpell(key string) (*entities.Spell, error) {
	args := m.Called(key)
	return args.Get(0).(*entities.Spell), args.Error(1)
}

func (m *mockDND5eClient) ListFeatures() ([]*entities.ReferenceItem, error) {
	args := m.Called()
	return args.Get(0).([]*entities.ReferenceItem), args.Error(1)
}

func (m *mockDND5eClient) GetFeature(key string) (*entities.Feature, error) {
	args := m.Called(key)
	return args.Get(0).(*entities.Feature), args.Error(1)
}

func (m *mockDND5eClient) ListSkills() ([]*entities.ReferenceItem, error) {
	args := m.Called()
	return args.Get(0).([]*entities.ReferenceItem), args.Error(1)
}

func (m *mockDND5eClient) GetSkill(key string) (*entities.Skill, error) {
	args := m.Called(key)
	return args.Get(0).(*entities.Skill), args.Error(1)
}

func (m *mockDND5eClient) ListMonsters() ([]*entities.ReferenceItem, error) {
	args := m.Called()
	return args.Get(0).([]*entities.ReferenceItem), args.Error(1)
}

func (m *mockDND5eClient) ListMonstersWithFilter(input *dnd5e.ListMonstersInput) ([]*entities.ReferenceItem, error) {
	args := m.Called(input)
	return args.Get(0).([]*entities.ReferenceItem), args.Error(1)
}

func (m *mockDND5eClient) GetMonster(key string) (*entities.Monster, error) {
	args := m.Called(key)
	return args.Get(0).(*entities.Monster), args.Error(1)
}

func (m *mockDND5eClient) GetClassLevel(key string, level int) (*entities.Level, error) {
	args := m.Called(key, level)
	return args.Get(0).(*entities.Level), args.Error(1)
}

func (m *mockDND5eClient) GetProficiency(key string) (*entities.Proficiency, error) {
	args := m.Called(key)
	return args.Get(0).(*entities.Proficiency), args.Error(1)
}

func (m *mockDND5eClient) ListDamageTypes() ([]*entities.ReferenceItem, error) {
	args := m.Called()
	return args.Get(0).([]*entities.ReferenceItem), args.Error(1)
}

func (m *mockDND5eClient) GetDamageType(key string) (*entities.DamageType, error) {
	args := m.Called(key)
	return args.Get(0).(*entities.DamageType), args.Error(1)
}

func (m *mockDND5eClient) ListBackgrounds() ([]*entities.ReferenceItem, error) {
	args := m.Called()
	return args.Get(0).([]*entities.ReferenceItem), args.Error(1)
}

func (m *mockDND5eClient) GetBackground(key string) (*entities.Background, error) {
	args := m.Called(key)
	return args.Get(0).(*entities.Background), args.Error(1)
}

func TestListAvailableEquipment(t *testing.T) {
	t.Run("successful equipment listing", func(t *testing.T) {
		mockClient := new(mockDND5eClient)
		client := &client{dnd5eClient: mockClient}

		// Mock reference items
		refs := []*entities.ReferenceItem{
			{Key: "longsword", Name: "Longsword"},
			{Key: "shield", Name: "Shield"},
		}

		// Mock equipment details
		longsword := &entities.Weapon{
			Key:            "longsword",
			Name:           "Longsword",
			WeaponCategory: "Martial",
			WeaponRange:    "Melee",
			Weight:         3.0,
			Cost:           &entities.Cost{Quantity: 15, Unit: "gp"},
			Damage:         &entities.Damage{DamageDice: "1d8", DamageType: &entities.ReferenceItem{Name: "Slashing"}},
		}
		shield := &entities.Equipment{
			Key:    "shield",
			Name:   "Shield",
			Weight: 6.0,
			Cost:   &entities.Cost{Quantity: 10, Unit: "gp"},
		}

		mockClient.On("ListEquipment").Return(refs, nil)
		mockClient.On("GetEquipment", "longsword").Return(longsword, nil)
		mockClient.On("GetEquipment", "shield").Return(shield, nil)

		result, err := client.ListAvailableEquipment(context.Background())

		assert.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Equal(t, "longsword", result[0].ID)
		assert.Equal(t, "Longsword", result[0].Name)
		assert.Equal(t, "weapon", result[0].EquipmentType)
		assert.Equal(t, "Martial", result[0].WeaponCategory)
		assert.Equal(t, "shield", result[1].ID)
		assert.Equal(t, "Shield", result[1].Name)
		assert.Equal(t, "equipment", result[1].EquipmentType)

		mockClient.AssertExpectations(t)
	})

	t.Run("equipment listing API error", func(t *testing.T) {
		mockClient := new(mockDND5eClient)
		client := &client{dnd5eClient: mockClient}

		mockClient.On("ListEquipment").Return(([]*entities.ReferenceItem)(nil), errors.New("API error"))

		result, err := client.ListAvailableEquipment(context.Background())

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to list equipment from D&D 5e API")

		mockClient.AssertExpectations(t)
	})
}

func TestListEquipmentByCategory(t *testing.T) {
	t.Run("successful category listing", func(t *testing.T) {
		mockClient := new(mockDND5eClient)
		client := &client{dnd5eClient: mockClient}

		// Mock equipment category
		category := &entities.EquipmentCategory{
			Index: "martial-weapons",
			Name:  "Martial Weapons",
			Equipment: []*entities.ReferenceItem{
				{Key: "longsword", Name: "Longsword"},
				{Key: "battleaxe", Name: "Battleaxe"},
			},
		}

		// Mock equipment details
		longsword := &entities.Weapon{
			Key:            "longsword",
			Name:           "Longsword",
			WeaponCategory: "Martial",
			WeaponRange:    "Melee",
		}
		battleaxe := &entities.Weapon{
			Key:            "battleaxe",
			Name:           "Battleaxe",
			WeaponCategory: "Martial",
			WeaponRange:    "Melee",
		}

		mockClient.On("GetEquipmentCategory", "martial-weapons").Return(category, nil)
		mockClient.On("GetEquipment", "longsword").Return(longsword, nil)
		mockClient.On("GetEquipment", "battleaxe").Return(battleaxe, nil)

		result, err := client.ListEquipmentByCategory(context.Background(), "martial-weapons")

		assert.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Equal(t, "longsword", result[0].ID)
		assert.Equal(t, "Longsword", result[0].Name)
		assert.Equal(t, "battleaxe", result[1].ID)
		assert.Equal(t, "Battleaxe", result[1].Name)

		mockClient.AssertExpectations(t)
	})

	t.Run("category not found", func(t *testing.T) {
		mockClient := new(mockDND5eClient)
		client := &client{dnd5eClient: mockClient}

		mockClient.On("GetEquipmentCategory", "invalid-category").Return(
			(*entities.EquipmentCategory)(nil), errors.New("category not found"))

		result, err := client.ListEquipmentByCategory(context.Background(), "invalid-category")

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to get equipment category")

		mockClient.AssertExpectations(t)
	})
}

func TestGetEquipmentData(t *testing.T) {
	t.Run("successful equipment retrieval", func(t *testing.T) {
		mockClient := new(mockDND5eClient)
		client := &client{dnd5eClient: mockClient}

		weapon := &entities.Weapon{
			Key:            "longsword",
			Name:           "Longsword",
			WeaponCategory: "Martial",
			WeaponRange:    "Melee",
			Weight:         3.0,
			Cost:           &entities.Cost{Quantity: 15, Unit: "gp"},
			Damage:         &entities.Damage{DamageDice: "1d8", DamageType: &entities.ReferenceItem{Name: "Slashing"}},
			Properties:     []*entities.ReferenceItem{{Name: "Versatile"}},
		}

		mockClient.On("GetEquipment", "longsword").Return(weapon, nil)

		result, err := client.GetEquipmentData(context.Background(), "longsword")

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "longsword", result.ID)
		assert.Equal(t, "Longsword", result.Name)
		assert.Equal(t, "weapon", result.EquipmentType)
		assert.Equal(t, "Martial", result.WeaponCategory)
		assert.Equal(t, "Melee", result.WeaponRange)
		assert.Equal(t, float32(3.0), result.Weight)
		assert.Equal(t, 15, result.Cost.Quantity)
		assert.Equal(t, "gp", result.Cost.Unit)
		assert.Equal(t, "1d8", result.Damage.DamageDice)
		assert.Equal(t, "Slashing", result.Damage.DamageType)
		assert.Equal(t, []string{"Versatile"}, result.Properties)

		mockClient.AssertExpectations(t)
	})

	t.Run("equipment not found", func(t *testing.T) {
		mockClient := new(mockDND5eClient)
		client := &client{dnd5eClient: mockClient}

		mockClient.On("GetEquipment", "invalid-equipment").Return(
			(*entities.Equipment)(nil), errors.New("equipment not found"))

		result, err := client.GetEquipmentData(context.Background(), "invalid-equipment")

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to get equipment")

		mockClient.AssertExpectations(t)
	})
}

func TestConvertEquipmentToEquipmentData(t *testing.T) {
	t.Run("convert weapon equipment", func(t *testing.T) {
		weapon := &entities.Weapon{
			Key:               "longsword",
			Name:              "Longsword",
			WeaponCategory:    "Martial",
			WeaponRange:       "Melee",
			Weight:            3.0,
			Cost:              &entities.Cost{Quantity: 15, Unit: "gp"},
			Damage:            &entities.Damage{DamageDice: "1d8", DamageType: &entities.ReferenceItem{Name: "Slashing"}},
			Properties:        []*entities.ReferenceItem{{Name: "Versatile"}},
			EquipmentCategory: &entities.ReferenceItem{Key: "martial-weapons"},
		}

		result := convertEquipmentToEquipmentData(weapon)

		assert.NotNil(t, result)
		assert.Equal(t, "longsword", result.ID)
		assert.Equal(t, "Longsword", result.Name)
		assert.Equal(t, "weapon", result.EquipmentType)
		assert.Equal(t, "martial-weapons", result.Category)
		assert.Equal(t, "Martial", result.WeaponCategory)
		assert.Equal(t, "Melee", result.WeaponRange)
		assert.Equal(t, float32(3.0), result.Weight)
		assert.Equal(t, 15, result.Cost.Quantity)
		assert.Equal(t, "gp", result.Cost.Unit)
		assert.Equal(t, "1d8", result.Damage.DamageDice)
		assert.Equal(t, "Slashing", result.Damage.DamageType)
		assert.Equal(t, []string{"Versatile"}, result.Properties)
	})

	t.Run("convert armor equipment", func(t *testing.T) {
		armor := &entities.Armor{
			Key:                 "leather-armor",
			Name:                "Leather Armor",
			ArmorCategory:       "Light",
			Weight:              10.0,
			Cost:                &entities.Cost{Quantity: 10, Unit: "gp"},
			ArmorClass:          &entities.ArmorClass{Base: 11, DexBonus: true},
			StrMinimum:          0,
			StealthDisadvantage: false,
			EquipmentCategory:   &entities.ReferenceItem{Key: "light-armor"},
		}

		result := convertEquipmentToEquipmentData(armor)

		assert.NotNil(t, result)
		assert.Equal(t, "leather-armor", result.ID)
		assert.Equal(t, "Leather Armor", result.Name)
		assert.Equal(t, "armor", result.EquipmentType)
		assert.Equal(t, "light-armor", result.Category)
		assert.Equal(t, "Light", result.ArmorCategory)
		assert.Equal(t, float32(10.0), result.Weight)
		assert.Equal(t, 10, result.Cost.Quantity)
		assert.Equal(t, "gp", result.Cost.Unit)
		assert.Equal(t, 11, result.ArmorClass.Base)
		assert.Equal(t, true, result.ArmorClass.DexBonus)
		assert.Equal(t, 0, result.StrengthMinimum)
		assert.Equal(t, false, result.StealthDisadvantage)
	})

	t.Run("convert generic equipment", func(t *testing.T) {
		equipment := &entities.Equipment{
			Key:               "rope",
			Name:              "Rope (50 feet)",
			Weight:            10.0,
			Cost:              &entities.Cost{Quantity: 2, Unit: "gp"},
			EquipmentCategory: &entities.ReferenceItem{Key: "adventuring-gear"},
		}

		result := convertEquipmentToEquipmentData(equipment)

		assert.NotNil(t, result)
		assert.Equal(t, "rope", result.ID)
		assert.Equal(t, "Rope (50 feet)", result.Name)
		assert.Equal(t, "equipment", result.EquipmentType)
		assert.Equal(t, "adventuring-gear", result.Category)
		assert.Equal(t, float32(10.0), result.Weight)
		assert.Equal(t, 2, result.Cost.Quantity)
		assert.Equal(t, "gp", result.Cost.Unit)
	})

	t.Run("convert nil equipment", func(t *testing.T) {
		result := convertEquipmentToEquipmentData(nil)
		assert.Nil(t, result)
	})
}
