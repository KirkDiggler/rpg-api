package character_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	redismocks "github.com/KirkDiggler/rpg-api/internal/redis/mocks"
	"github.com/KirkDiggler/rpg-api/internal/repositories/character"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

type RedisToolkitTestSuite struct {
	suite.Suite
	ctrl       *gomock.Controller
	mockClient *redismocks.MockClient
	mockPipe   *redismocks.MockPipeliner
	repo       character.Repository
	ctx        context.Context
}

func (s *RedisToolkitTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockClient = redismocks.NewMockClient(s.ctrl)
	s.mockPipe = redismocks.NewMockPipeliner(s.ctrl)
	s.ctx = context.Background()

	repo, err := character.NewRedis(&character.RedisConfig{
		Client: s.mockClient,
	})
	s.Require().NoError(err)
	s.repo = repo
}

func (s *RedisToolkitTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *RedisToolkitTestSuite) TestCreate() {
	testCases := []struct {
		name      string
		input     character.CreateInput
		setupMock func()
		wantErr   bool
		errMsg    string
	}{
		{
			name: "success with full character data",
			input: character.CreateInput{
				CharacterData: s.createTestCharacterData(),
			},
			setupMock: func() {
				charKey := "character:char_test123"
				playerKey := "character:player:player_123"

				// Check existence
				s.mockClient.EXPECT().
					Exists(s.ctx, charKey).
					Return(redis.NewIntResult(0, nil))

				// Transaction
				s.mockClient.EXPECT().TxPipeline().Return(s.mockPipe)
				s.mockPipe.EXPECT().
					Set(s.ctx, charKey, gomock.Any(), time.Duration(0)).
					Return(redis.NewStatusResult("", nil))
				s.mockPipe.EXPECT().
					SAdd(s.ctx, playerKey, "char_test123").
					Return(redis.NewIntResult(1, nil))
				s.mockPipe.EXPECT().
					Exec(s.ctx).
					Return([]redis.Cmder{}, nil)
			},
			wantErr: false,
		},
		{
			name: "error when character data is nil",
			input: character.CreateInput{
				CharacterData: nil,
			},
			setupMock: func() {},
			wantErr:   true,
			errMsg:    "character cannot be nil",
		},
		{
			name: "error when character ID is empty",
			input: character.CreateInput{
				CharacterData: &toolkitchar.Data{},
			},
			setupMock: func() {},
			wantErr:   true,
			errMsg:    "character ID cannot be empty",
		},
		{
			name: "error when character already exists",
			input: character.CreateInput{
				CharacterData: s.createTestCharacterData(),
			},
			setupMock: func() {
				charKey := "character:char_test123"
				s.mockClient.EXPECT().
					Exists(s.ctx, charKey).
					Return(redis.NewIntResult(1, nil))
			},
			wantErr: true,
			errMsg:  "already exists",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			tc.setupMock()

			output, err := s.repo.Create(s.ctx, tc.input)

			if tc.wantErr {
				s.Error(err)
				s.Contains(err.Error(), tc.errMsg)
				s.Nil(output)
			} else {
				s.NoError(err)
				s.NotNil(output)
				s.Equal(tc.input.CharacterData, output.CharacterData)
			}
		})
	}
}

func (s *RedisToolkitTestSuite) TestGet() {
	testCases := []struct {
		name      string
		input     character.GetInput
		setupMock func()
		wantErr   bool
		errMsg    string
	}{
		{
			name: "success retrieving character",
			input: character.GetInput{
				ID: "char_test123",
			},
			setupMock: func() {
				charKey := "character:char_test123"
				charData := s.createTestCharacterData()
				data, _ := json.Marshal(charData)

				s.mockClient.EXPECT().
					Get(s.ctx, charKey).
					Return(redis.NewStringResult(string(data), nil))
			},
			wantErr: false,
		},
		{
			name: "error when ID is empty",
			input: character.GetInput{
				ID: "",
			},
			setupMock: func() {},
			wantErr:   true,
			errMsg:    "character ID cannot be empty",
		},
		{
			name: "error when character not found",
			input: character.GetInput{
				ID: "char_notfound",
			},
			setupMock: func() {
				charKey := "character:char_notfound"
				s.mockClient.EXPECT().
					Get(s.ctx, charKey).
					Return(redis.NewStringResult("", redis.Nil))
			},
			wantErr: true,
			errMsg:  "not found",
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
				s.NotNil(output.CharacterData)
				s.Equal("char_test123", output.CharacterData.ID)
				s.Equal("Test Character", output.CharacterData.Name)
			}
		})
	}
}

func (s *RedisToolkitTestSuite) TestUpdate() {
	testCases := []struct {
		name      string
		input     character.UpdateInput
		setupMock func()
		wantErr   bool
		errMsg    string
	}{
		{
			name: "success updating character",
			input: character.UpdateInput{
				CharacterData: s.createTestCharacterData(),
			},
			setupMock: func() {
				charKey := "character:char_test123"
				charData := s.createTestCharacterData()
				data, _ := json.Marshal(charData)

				// Get existing character
				s.mockClient.EXPECT().
					Get(s.ctx, charKey).
					Return(redis.NewStringResult(string(data), nil))

				// Update transaction
				s.mockClient.EXPECT().TxPipeline().Return(s.mockPipe)
				s.mockPipe.EXPECT().
					Set(s.ctx, charKey, gomock.Any(), time.Duration(0)).
					Return(redis.NewStatusResult("", nil))
				s.mockPipe.EXPECT().
					Exec(s.ctx).
					Return([]redis.Cmder{}, nil)
			},
			wantErr: false,
		},
		{
			name: "success updating with player ID change",
			input: character.UpdateInput{
				CharacterData: func() *toolkitchar.Data {
					data := s.createTestCharacterData()
					data.PlayerID = "player_new"
					return data
				}(),
			},
			setupMock: func() {
				charKey := "character:char_test123"
				oldPlayerKey := "character:player:player_123"
				newPlayerKey := "character:player:player_new"

				// Get existing with old player ID
				existingData := s.createTestCharacterData()
				existingJSON, _ := json.Marshal(existingData)
				s.mockClient.EXPECT().
					Get(s.ctx, charKey).
					Return(redis.NewStringResult(string(existingJSON), nil))

				// Update with new player ID
				s.mockClient.EXPECT().TxPipeline().Return(s.mockPipe)
				s.mockPipe.EXPECT().
					Set(s.ctx, charKey, gomock.Any(), time.Duration(0)).
					Return(redis.NewStatusResult("", nil))
				s.mockPipe.EXPECT().
					SRem(s.ctx, oldPlayerKey, "char_test123").
					Return(redis.NewIntResult(1, nil))
				s.mockPipe.EXPECT().
					SAdd(s.ctx, newPlayerKey, "char_test123").
					Return(redis.NewIntResult(1, nil))
				s.mockPipe.EXPECT().
					Exec(s.ctx).
					Return([]redis.Cmder{}, nil)
			},
			wantErr: false,
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
				s.Equal(tc.input.CharacterData, output.CharacterData)
			}
		})
	}
}

func (s *RedisToolkitTestSuite) TestDelete() {
	testCases := []struct {
		name      string
		input     character.DeleteInput
		setupMock func()
		wantErr   bool
		errMsg    string
	}{
		{
			name: "success deleting character",
			input: character.DeleteInput{
				ID: "char_test123",
			},
			setupMock: func() {
				charKey := "character:char_test123"
				playerKey := "character:player:player_123"
				charData := s.createTestCharacterData()
				data, _ := json.Marshal(charData)

				// Get character to find indexes
				s.mockClient.EXPECT().
					Get(s.ctx, charKey).
					Return(redis.NewStringResult(string(data), nil))

				// Delete transaction
				s.mockClient.EXPECT().TxPipeline().Return(s.mockPipe)
				s.mockPipe.EXPECT().
					Del(s.ctx, charKey).
					Return(redis.NewIntResult(1, nil))
				s.mockPipe.EXPECT().
					SRem(s.ctx, playerKey, "char_test123").
					Return(redis.NewIntResult(1, nil))
				s.mockPipe.EXPECT().
					Exec(s.ctx).
					Return([]redis.Cmder{}, nil)
			},
			wantErr: false,
		},
		{
			name: "error when ID is empty",
			input: character.DeleteInput{
				ID: "",
			},
			setupMock: func() {},
			wantErr:   true,
			errMsg:    "character ID cannot be empty",
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

func (s *RedisToolkitTestSuite) TestListByPlayerID() {
	testCases := []struct {
		name      string
		input     character.ListByPlayerIDInput
		setupMock func()
		wantErr   bool
		errMsg    string
		wantCount int
	}{
		{
			name: "success listing characters by player",
			input: character.ListByPlayerIDInput{
				PlayerID: "player_123",
			},
			setupMock: func() {
				playerKey := "character:player:player_123"
				char1Key := "character:char_test123"
				char2Key := "character:char_test456"

				// Get character IDs from index
				s.mockClient.EXPECT().
					SMembers(s.ctx, playerKey).
					Return(redis.NewStringSliceResult([]string{"char_test123", "char_test456"}, nil))

				// Get first character
				char1 := s.createTestCharacterData()
				char1.ID = "char_test123"
				data1, _ := json.Marshal(char1)
				s.mockClient.EXPECT().
					Get(s.ctx, char1Key).
					Return(redis.NewStringResult(string(data1), nil))

				// Get second character
				char2 := s.createTestCharacterData()
				char2.ID = "char_test456"
				char2.Name = "Test Character 2"
				data2, _ := json.Marshal(char2)
				s.mockClient.EXPECT().
					Get(s.ctx, char2Key).
					Return(redis.NewStringResult(string(data2), nil))
			},
			wantErr:   false,
			wantCount: 2,
		},
		{
			name: "success with missing character cleaned up",
			input: character.ListByPlayerIDInput{
				PlayerID: "player_123",
			},
			setupMock: func() {
				playerKey := "character:player:player_123"
				char1Key := "character:char_test123"
				char2Key := "character:char_missing"

				// Get character IDs from index
				s.mockClient.EXPECT().
					SMembers(s.ctx, playerKey).
					Return(redis.NewStringSliceResult([]string{"char_test123", "char_missing"}, nil))

				// Get first character - exists
				char1 := s.createTestCharacterData()
				data1, _ := json.Marshal(char1)
				s.mockClient.EXPECT().
					Get(s.ctx, char1Key).
					Return(redis.NewStringResult(string(data1), nil))

				// Get second character - missing
				s.mockClient.EXPECT().
					Get(s.ctx, char2Key).
					Return(redis.NewStringResult("", redis.Nil))

				// Clean up missing character from index
				s.mockClient.EXPECT().
					SRem(s.ctx, playerKey, "char_missing").
					Return(redis.NewIntResult(1, nil))
			},
			wantErr:   false,
			wantCount: 1,
		},
		{
			name: "error when player ID is empty",
			input: character.ListByPlayerIDInput{
				PlayerID: "",
			},
			setupMock: func() {},
			wantErr:   true,
			errMsg:    "player ID cannot be empty",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			tc.setupMock()

			output, err := s.repo.ListByPlayerID(s.ctx, tc.input)

			if tc.wantErr {
				s.Error(err)
				s.Contains(err.Error(), tc.errMsg)
				s.Nil(output)
			} else {
				s.NoError(err)
				s.NotNil(output)
				s.Len(output.Characters, tc.wantCount)
			}
		})
	}
}

// Helper method to create test character data
func (s *RedisToolkitTestSuite) createTestCharacterData() *toolkitchar.Data {
	now := time.Now()
	return &toolkitchar.Data{
		ID:           "char_test123",
		PlayerID:     "player_123",
		Name:         "Test Character",
		Level:        1,
		Experience:   0,
		RaceID:       "human",
		ClassID:      "fighter",
		BackgroundID: "soldier",
		AbilityScores: shared.AbilityScores{
			Strength:     16,
			Dexterity:    14,
			Constitution: 15,
			Intelligence: 10,
			Wisdom:       12,
			Charisma:     8,
		},
		HitPoints:    12,
		MaxHitPoints: 12,
		Skills: map[string]int{
			"athletics":    1,
			"intimidation": 1,
		},
		SavingThrows: map[string]int{
			"strength":     1,
			"constitution": 1,
		},
		Languages: []string{"Common", "Orc"},
		Proficiencies: shared.Proficiencies{
			Armor:   []string{"light", "medium", "heavy", "shields"},
			Weapons: []string{"simple", "martial"},
			Tools:   []string{},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestRedisToolkitTestSuite(t *testing.T) {
	suite.Run(t, new(RedisToolkitTestSuite))
}

