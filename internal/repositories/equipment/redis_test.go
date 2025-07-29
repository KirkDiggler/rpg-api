package equipment_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
	"github.com/KirkDiggler/rpg-api/internal/errors"
	redismocks "github.com/KirkDiggler/rpg-api/internal/redis/mocks"
	"github.com/KirkDiggler/rpg-api/internal/repositories/equipment"
)

const (
	testCharID       = "char_test123"
	testEquipmentKey = "equipment:character:char_test123"
)

type RedisEquipmentTestSuite struct {
	suite.Suite
	ctrl       *gomock.Controller
	mockClient *redismocks.MockClient
	repo       equipment.Repository
	ctx        context.Context
}

func (s *RedisEquipmentTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockClient = redismocks.NewMockClient(s.ctrl)
	s.ctx = context.Background()

	repo, err := equipment.NewRedis(&equipment.RedisConfig{
		Client: s.mockClient,
	})
	s.Require().NoError(err)
	s.repo = repo
}

func (s *RedisEquipmentTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *RedisEquipmentTestSuite) TestNewRedis() {
	testCases := []struct {
		name    string
		config  *equipment.RedisConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "success with valid config",
			config: &equipment.RedisConfig{
				Client: s.mockClient,
			},
			wantErr: false,
		},
		{
			name:    "error with nil config",
			config:  nil,
			wantErr: true,
			errMsg:  "config cannot be nil",
		},
		{
			name: "error with nil client",
			config: &equipment.RedisConfig{
				Client: nil,
			},
			wantErr: true,
			errMsg:  "client cannot be nil",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			repo, err := equipment.NewRedis(tc.config)

			if tc.wantErr {
				s.Error(err)
				s.Contains(err.Error(), tc.errMsg)
				s.Nil(repo)
			} else {
				s.NoError(err)
				s.NotNil(repo)
			}
		})
	}
}

func (s *RedisEquipmentTestSuite) TestGet() {
	testCases := []struct {
		name      string
		input     equipment.GetInput
		setupMock func()
		wantErr   bool
		errMsg    string
		validate  func(output *equipment.GetOutput)
	}{
		{
			name: "success retrieving equipment",
			input: equipment.GetInput{
				CharacterID: testCharID,
			},
			setupMock: func() {
				equipData := s.createTestEquipmentData()
				data, _ := json.Marshal(equipData)

				s.mockClient.EXPECT().
					Get(s.ctx, testEquipmentKey).
					Return(redis.NewStringResult(string(data), nil))
			},
			wantErr: false,
			validate: func(output *equipment.GetOutput) {
				s.Equal(testCharID, output.CharacterID)
				s.NotNil(output.EquipmentSlots)
				s.NotNil(output.Inventory)
				s.NotNil(output.Encumbrance)
				s.Len(output.Inventory, 3)
			},
		},
		{
			name: "error when character ID is empty",
			input: equipment.GetInput{
				CharacterID: "",
			},
			setupMock: func() {},
			wantErr:   true,
			errMsg:    "character ID cannot be empty",
		},
		{
			name: "error when equipment not found",
			input: equipment.GetInput{
				CharacterID: "char_notfound",
			},
			setupMock: func() {
				key := "equipment:character:char_notfound"
				s.mockClient.EXPECT().
					Get(s.ctx, key).
					Return(redis.NewStringResult("", redis.Nil))
			},
			wantErr: true,
			errMsg:  "not found",
		},
		{
			name: "error on Redis failure",
			input: equipment.GetInput{
				CharacterID: testCharID,
			},
			setupMock: func() {
				s.mockClient.EXPECT().
					Get(s.ctx, testEquipmentKey).
					Return(redis.NewStringResult("", errors.Internal("redis error")))
			},
			wantErr: true,
			errMsg:  "failed to get equipment",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			tc.setupMock()

			output, err := s.repo.Get(s.ctx, tc.input)

			if tc.wantErr {
				s.Error(err)
				s.Contains(err.Error(), tc.errMsg)
				s.Nil(output)
			} else {
				s.NoError(err)
				s.NotNil(output)
				tc.validate(output)
			}
		})
	}
}

func (s *RedisEquipmentTestSuite) TestUpdate() {
	testCases := []struct {
		name      string
		input     equipment.UpdateInput
		setupMock func()
		wantErr   bool
		errMsg    string
	}{
		{
			name: "success updating equipment",
			input: equipment.UpdateInput{
				CharacterID:    testCharID,
				EquipmentSlots: s.createTestEquipmentSlots(),
				Inventory:      s.createTestInventory(),
				Encumbrance:    s.createTestEncumbrance(),
			},
			setupMock: func() {
				cmd := redis.NewStatusCmd(s.ctx)
				cmd.SetErr(nil)
				s.mockClient.EXPECT().
					Set(s.ctx, testEquipmentKey, gomock.Any(), gomock.Any()).
					Return(cmd)
			},
			wantErr: false,
		},
		{
			name: "success creating new equipment",
			input: equipment.UpdateInput{
				CharacterID:    testCharID,
				EquipmentSlots: &dnd5e.EquipmentSlots{},
				Inventory:      []dnd5e.InventoryItem{},
				Encumbrance:    &dnd5e.EncumbranceInfo{},
			},
			setupMock: func() {
				cmd := redis.NewStatusCmd(s.ctx)
				cmd.SetErr(nil)
				s.mockClient.EXPECT().
					Set(s.ctx, testEquipmentKey, gomock.Any(), gomock.Any()).
					Return(cmd)
			},
			wantErr: false,
		},
		{
			name: "error when character ID is empty",
			input: equipment.UpdateInput{
				CharacterID: "",
			},
			setupMock: func() {},
			wantErr:   true,
			errMsg:    "character ID cannot be empty",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			tc.setupMock()

			output, err := s.repo.Update(s.ctx, tc.input)

			if tc.wantErr {
				s.Error(err)
				s.Contains(err.Error(), tc.errMsg)
				s.Nil(output)
			} else {
				s.NoError(err)
				s.NotNil(output)
				s.Equal(tc.input.CharacterID, output.CharacterID)
				s.Equal(tc.input.EquipmentSlots, output.EquipmentSlots)
				s.Equal(tc.input.Inventory, output.Inventory)
				s.Equal(tc.input.Encumbrance, output.Encumbrance)
			}
		})
	}
}

func (s *RedisEquipmentTestSuite) TestDelete() {
	testCases := []struct {
		name      string
		input     equipment.DeleteInput
		setupMock func()
		wantErr   bool
		errMsg    string
	}{
		{
			name: "success deleting equipment",
			input: equipment.DeleteInput{
				CharacterID: testCharID,
			},
			setupMock: func() {
				// Check existence
				s.mockClient.EXPECT().
					Exists(s.ctx, testEquipmentKey).
					Return(redis.NewIntResult(1, nil))

				// Delete
				cmd := redis.NewIntCmd(s.ctx)
				cmd.SetErr(nil)
				s.mockClient.EXPECT().
					Del(s.ctx, testEquipmentKey).
					Return(cmd)
			},
			wantErr: false,
		},
		{
			name: "error when character ID is empty",
			input: equipment.DeleteInput{
				CharacterID: "",
			},
			setupMock: func() {},
			wantErr:   true,
			errMsg:    "character ID cannot be empty",
		},
		{
			name: "error when equipment not found",
			input: equipment.DeleteInput{
				CharacterID: "char_notfound",
			},
			setupMock: func() {
				key := "equipment:character:char_notfound"
				s.mockClient.EXPECT().
					Exists(s.ctx, key).
					Return(redis.NewIntResult(0, nil))
			},
			wantErr: true,
			errMsg:  "not found",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			tc.setupMock()

			output, err := s.repo.Delete(s.ctx, tc.input)

			if tc.wantErr {
				s.Error(err)
				s.Contains(err.Error(), tc.errMsg)
				s.Nil(output)
			} else {
				s.NoError(err)
				s.NotNil(output)
			}
		})
	}
}

// Helper methods

func (s *RedisEquipmentTestSuite) createTestEquipmentData() map[string]interface{} {
	return map[string]interface{}{
		"character_id":    testCharID,
		"equipment_slots": s.createTestEquipmentSlots(),
		"inventory":       s.createTestInventory(),
		"encumbrance":     s.createTestEncumbrance(),
	}
}

func (s *RedisEquipmentTestSuite) createTestEquipmentSlots() *dnd5e.EquipmentSlots {
	return &dnd5e.EquipmentSlots{
		MainHand: &dnd5e.InventoryItem{
			ItemID:   "longsword",
			Quantity: 1,
		},
		OffHand: &dnd5e.InventoryItem{
			ItemID:   "shield",
			Quantity: 1,
		},
		Armor: &dnd5e.InventoryItem{
			ItemID:   "chainmail",
			Quantity: 1,
		},
	}
}

func (s *RedisEquipmentTestSuite) createTestInventory() []dnd5e.InventoryItem {
	return []dnd5e.InventoryItem{
		{
			ItemID:   "potion_healing",
			Quantity: 3,
		},
		{
			ItemID:   "rope_hempen",
			Quantity: 1,
		},
		{
			ItemID:   "torch",
			Quantity: 10,
		},
	}
}

func (s *RedisEquipmentTestSuite) createTestEncumbrance() *dnd5e.EncumbranceInfo {
	return &dnd5e.EncumbranceInfo{
		CurrentWeight:    750,  // 75 lbs
		CarryingCapacity: 2400, // 240 lbs (STR 16 * 15)
		MaxCapacity:      4800, // 480 lbs (STR 16 * 30)
		Level:            dnd5e.EncumbranceLevelUnencumbered,
	}
}

func TestRedisEquipmentTestSuite(t *testing.T) {
	suite.Run(t, new(RedisEquipmentTestSuite))
}
