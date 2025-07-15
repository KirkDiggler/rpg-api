package character_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/redis/go-redis/v9"

	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
	"github.com/KirkDiggler/rpg-api/internal/errors"
	redismocks "github.com/KirkDiggler/rpg-api/internal/redis/mocks"
	character "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

const (
	testCharID     = "char_123"
	testPlayerID   = "player_456"
	testSessionID  = "session_789"
	testCharKey    = "character:char_123"
	testPlayerKey  = "character:player:player_456"
	testSessionKey = "character:session:session_789"
)

type RedisRepositoryTestSuite struct {
	suite.Suite
	ctrl       *gomock.Controller
	mockClient *redismocks.MockClient
	mockPipe   *redismocks.MockPipeliner
	repo       character.Repository
	ctx        context.Context
}

func (s *RedisRepositoryTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockClient = redismocks.NewMockClient(s.ctrl)
	s.mockPipe = redismocks.NewMockPipeliner(s.ctrl)
	repo, err := character.NewRedis(&character.RedisConfig{
		Client: s.mockClient,
	})
	s.Require().NoError(err)
	s.repo = repo
	s.ctx = context.Background()
}

func (s *RedisRepositoryTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *RedisRepositoryTestSuite) TestCreate() {
	testCharacter := &dnd5e.Character{
		ID:        testCharID,
		PlayerID:  testPlayerID,
		SessionID: testSessionID,
		Name:      "Test Hero",
		Level:     1,
		ClassID:   "fighter",
		RaceID:    "human",
	}

	s.Run("successful create", func() {
		charKey := testCharKey
		playerKey := testPlayerKey
		sessionKey := testSessionKey

		// Check existence
		s.mockClient.EXPECT().
			Exists(s.ctx, charKey).
			Return(redis.NewIntResult(0, nil))

		// Setup pipeline
		s.mockClient.EXPECT().
			TxPipeline().
			Return(s.mockPipe)

		// Marshal character data
		charData, _ := json.Marshal(testCharacter)

		// Set character
		s.mockPipe.EXPECT().
			Set(s.ctx, charKey, charData, gomock.Any()).
			Return(redis.NewStatusResult("OK", nil))

		// Add to player index
		s.mockPipe.EXPECT().
			SAdd(s.ctx, playerKey, testCharID).
			Return(redis.NewIntResult(1, nil))

		// Add to session index
		s.mockPipe.EXPECT().
			SAdd(s.ctx, sessionKey, testCharID).
			Return(redis.NewIntResult(1, nil))

		// Execute pipeline
		s.mockPipe.EXPECT().
			Exec(s.ctx).
			Return([]redis.Cmder{}, nil)

		// Execute
		output, err := s.repo.Create(s.ctx, character.CreateInput{Character: testCharacter})

		// Assert
		s.NoError(err)
		s.NotNil(output)
	})

	s.Run("error when character already exists", func() {
		charKey := testCharKey

		// Check existence - character exists
		s.mockClient.EXPECT().
			Exists(s.ctx, charKey).
			Return(redis.NewIntResult(1, nil))

		// Execute
		output, err := s.repo.Create(s.ctx, character.CreateInput{Character: testCharacter})

		// Assert
		s.Error(err)
		s.Nil(output)
		s.True(errors.IsAlreadyExists(err))
		s.Contains(err.Error(), "character with ID char_123 already exists")
	})

	s.Run("error when character is nil", func() {
		output, err := s.repo.Create(s.ctx, character.CreateInput{Character: nil})

		s.Error(err)
		s.Nil(output)
		s.True(errors.IsInvalidArgument(err))
		s.Contains(err.Error(), "character cannot be nil")
	})

	s.Run("error when character ID is empty", func() {
		char := &dnd5e.Character{PlayerID: testPlayerID}
		output, err := s.repo.Create(s.ctx, character.CreateInput{Character: char})

		s.Error(err)
		s.Nil(output)
		s.True(errors.IsInvalidArgument(err))
		s.Contains(err.Error(), "character ID cannot be empty")
	})

	s.Run("successful create without player or session", func() {
		charWithoutIndexes := &dnd5e.Character{
			ID:   testCharID,
			Name: "Solo Hero",
		}
		charKey := testCharKey

		// Check existence
		s.mockClient.EXPECT().
			Exists(s.ctx, charKey).
			Return(redis.NewIntResult(0, nil))

		// Setup pipeline
		s.mockClient.EXPECT().
			TxPipeline().
			Return(s.mockPipe)

		// Marshal character data
		charData, _ := json.Marshal(charWithoutIndexes)

		// Set character (no index operations)
		s.mockPipe.EXPECT().
			Set(s.ctx, charKey, charData, gomock.Any()).
			Return(redis.NewStatusResult("OK", nil))

		// Execute pipeline
		s.mockPipe.EXPECT().
			Exec(s.ctx).
			Return([]redis.Cmder{}, nil)

		// Execute
		output, err := s.repo.Create(s.ctx, character.CreateInput{Character: charWithoutIndexes})

		// Assert
		s.NoError(err)
		s.NotNil(output)
	})
}

func (s *RedisRepositoryTestSuite) TestGet() {
	testCharacter := &dnd5e.Character{
		ID:       testCharID,
		PlayerID: testPlayerID,
		Name:     "Test Hero",
		Level:    5,
	}

	s.Run("successful get", func() {
		charKey := testCharKey
		charData, _ := json.Marshal(testCharacter)

		s.mockClient.EXPECT().
			Get(s.ctx, charKey).
			Return(redis.NewStringResult(string(charData), nil))

		// Execute
		output, err := s.repo.Get(s.ctx, character.GetInput{ID: testCharID})

		// Assert
		s.NoError(err)
		s.NotNil(output)
		s.NotNil(output.Character)
		s.Equal(testCharacter.ID, output.Character.ID)
		s.Equal(testCharacter.PlayerID, output.Character.PlayerID)
		s.Equal(testCharacter.Name, output.Character.Name)
		s.Equal(testCharacter.Level, output.Character.Level)
	})

	s.Run("error when character not found", func() {
		charKey := testCharKey

		s.mockClient.EXPECT().
			Get(s.ctx, charKey).
			Return(redis.NewStringResult("", redis.Nil))

		// Execute
		output, err := s.repo.Get(s.ctx, character.GetInput{ID: testCharID})

		// Assert
		s.Error(err)
		s.Nil(output)
		s.True(errors.IsNotFound(err))
		s.Contains(err.Error(), "character with ID char_123 not found")
	})

	s.Run("error when ID is empty", func() {
		output, err := s.repo.Get(s.ctx, character.GetInput{ID: ""})

		s.Error(err)
		s.Nil(output)
		s.True(errors.IsInvalidArgument(err))
		s.Contains(err.Error(), "character ID cannot be empty")
	})
}

func (s *RedisRepositoryTestSuite) TestUpdate() {
	existingCharacter := &dnd5e.Character{
		ID:        testCharID,
		PlayerID:  testPlayerID,
		SessionID: testSessionID,
		Name:      "Test Hero",
		Level:     5,
	}

	updatedCharacter := &dnd5e.Character{
		ID:        testCharID,
		PlayerID:  "player_999",  // Changed
		SessionID: "session_999", // Changed
		Name:      "Updated Hero",
		Level:     10,
	}

	s.Run("successful update with index changes", func() {
		charKey := testCharKey
		oldPlayerKey := testPlayerKey
		newPlayerKey := "character:player:player_999"
		oldSessionKey := testSessionKey
		newSessionKey := "character:session:session_999"

		existingData, _ := json.Marshal(existingCharacter)
		updatedData, _ := json.Marshal(updatedCharacter)

		// Get existing character
		s.mockClient.EXPECT().
			Get(s.ctx, charKey).
			Return(redis.NewStringResult(string(existingData), nil))

		// Setup pipeline
		s.mockClient.EXPECT().
			TxPipeline().
			Return(s.mockPipe)

		// Update character
		s.mockPipe.EXPECT().
			Set(s.ctx, charKey, updatedData, gomock.Any()).
			Return(redis.NewStatusResult("OK", nil))

		// Remove from old player index
		s.mockPipe.EXPECT().
			SRem(s.ctx, oldPlayerKey, testCharID).
			Return(redis.NewIntResult(1, nil))

		// Add to new player index
		s.mockPipe.EXPECT().
			SAdd(s.ctx, newPlayerKey, testCharID).
			Return(redis.NewIntResult(1, nil))

		// Remove from old session index
		s.mockPipe.EXPECT().
			SRem(s.ctx, oldSessionKey, testCharID).
			Return(redis.NewIntResult(1, nil))

		// Add to new session index
		s.mockPipe.EXPECT().
			SAdd(s.ctx, newSessionKey, testCharID).
			Return(redis.NewIntResult(1, nil))

		// Execute pipeline
		s.mockPipe.EXPECT().
			Exec(s.ctx).
			Return([]redis.Cmder{}, nil)

		// Execute
		output, err := s.repo.Update(s.ctx, character.UpdateInput{Character: updatedCharacter})

		// Assert
		s.NoError(err)
		s.NotNil(output)
	})

	s.Run("successful update without index changes", func() {
		sameIndexCharacter := &dnd5e.Character{
			ID:        testCharID,
			PlayerID:  testPlayerID,  // Same
			SessionID: testSessionID, // Same
			Name:      "Updated Hero",
			Level:     10,
		}

		charKey := testCharKey
		existingData, _ := json.Marshal(existingCharacter)
		updatedData, _ := json.Marshal(sameIndexCharacter)

		// Get existing character
		s.mockClient.EXPECT().
			Get(s.ctx, charKey).
			Return(redis.NewStringResult(string(existingData), nil))

		// Setup pipeline
		s.mockClient.EXPECT().
			TxPipeline().
			Return(s.mockPipe)

		// Update character (no index operations)
		s.mockPipe.EXPECT().
			Set(s.ctx, charKey, updatedData, gomock.Any()).
			Return(redis.NewStatusResult("OK", nil))

		// Execute pipeline
		s.mockPipe.EXPECT().
			Exec(s.ctx).
			Return([]redis.Cmder{}, nil)

		// Execute
		output, err := s.repo.Update(s.ctx, character.UpdateInput{Character: sameIndexCharacter})

		// Assert
		s.NoError(err)
		s.NotNil(output)
	})

	s.Run("error when character doesn't exist", func() {
		charKey := testCharKey

		// Get returns not found
		s.mockClient.EXPECT().
			Get(s.ctx, charKey).
			Return(redis.NewStringResult("", redis.Nil))

		// Execute
		output, err := s.repo.Update(s.ctx, character.UpdateInput{Character: updatedCharacter})

		// Assert
		s.Error(err)
		s.Nil(output)
		s.True(errors.IsNotFound(err))
	})

	s.Run("error when character is nil", func() {
		output, err := s.repo.Update(s.ctx, character.UpdateInput{Character: nil})

		s.Error(err)
		s.Nil(output)
		s.True(errors.IsInvalidArgument(err))
		s.Contains(err.Error(), "character cannot be nil")
	})
}

func (s *RedisRepositoryTestSuite) TestDelete() {
	testCharacter := &dnd5e.Character{
		ID:        testCharID,
		PlayerID:  testPlayerID,
		SessionID: testSessionID,
		Name:      "Test Hero",
	}

	s.Run("successful delete", func() {
		charKey := testCharKey
		playerKey := testPlayerKey
		sessionKey := testSessionKey
		charData, _ := json.Marshal(testCharacter)

		// Get character to find indexes
		s.mockClient.EXPECT().
			Get(s.ctx, charKey).
			Return(redis.NewStringResult(string(charData), nil))

		// Setup pipeline
		s.mockClient.EXPECT().
			TxPipeline().
			Return(s.mockPipe)

		// Delete character
		s.mockPipe.EXPECT().
			Del(s.ctx, charKey).
			Return(redis.NewIntResult(1, nil))

		// Remove from player index
		s.mockPipe.EXPECT().
			SRem(s.ctx, playerKey, testCharID).
			Return(redis.NewIntResult(1, nil))

		// Remove from session index
		s.mockPipe.EXPECT().
			SRem(s.ctx, sessionKey, testCharID).
			Return(redis.NewIntResult(1, nil))

		// Execute pipeline
		s.mockPipe.EXPECT().
			Exec(s.ctx).
			Return([]redis.Cmder{}, nil)

		// Execute
		output, err := s.repo.Delete(s.ctx, character.DeleteInput{ID: testCharID})

		// Assert
		s.NoError(err)
		s.NotNil(output)
	})

	s.Run("error when character not found", func() {
		charKey := testCharKey

		// Get returns not found
		s.mockClient.EXPECT().
			Get(s.ctx, charKey).
			Return(redis.NewStringResult("", redis.Nil))

		// Execute
		output, err := s.repo.Delete(s.ctx, character.DeleteInput{ID: testCharID})

		// Assert
		s.Error(err)
		s.Nil(output)
		s.True(errors.IsNotFound(err))
	})

	s.Run("error when ID is empty", func() {
		output, err := s.repo.Delete(s.ctx, character.DeleteInput{ID: ""})

		s.Error(err)
		s.Nil(output)
		s.True(errors.IsInvalidArgument(err))
		s.Contains(err.Error(), "character ID cannot be empty")
	})
}

// Helper function to create test characters
func (s *RedisRepositoryTestSuite) createTestCharactersForPlayer() (*dnd5e.Character, *dnd5e.Character) {
	char1 := &dnd5e.Character{
		ID:       "char_1",
		PlayerID: testPlayerID,
		Name:     "Hero 1",
		Level:    5,
	}
	char2 := &dnd5e.Character{
		ID:       "char_2",
		PlayerID: testPlayerID,
		Name:     "Hero 2",
		Level:    3,
	}
	return char1, char2
}

// Helper function to create test characters for session
func (s *RedisRepositoryTestSuite) createTestCharactersForSession() (*dnd5e.Character, *dnd5e.Character) {
	char1 := &dnd5e.Character{
		ID:        "char_1",
		SessionID: testSessionID,
		Name:      "Hero 1",
	}
	char2 := &dnd5e.Character{
		ID:        "char_2",
		SessionID: testSessionID,
		Name:      "Hero 2",
	}
	return char1, char2
}

// Helper function to test successful list operations
func (s *RedisRepositoryTestSuite) testSuccessfulList(
	indexKey string,
	char1, char2 *dnd5e.Character,
	listFunc func() ([]*dnd5e.Character, error),
) {
	charData1, _ := json.Marshal(char1)
	charData2, _ := json.Marshal(char2)

	// Get character IDs from index
	s.mockClient.EXPECT().
		SMembers(s.ctx, indexKey).
		Return(redis.NewStringSliceResult([]string{"char_1", "char_2"}, nil))

	// Get first character
	s.mockClient.EXPECT().
		Get(s.ctx, "character:char_1").
		Return(redis.NewStringResult(string(charData1), nil))

	// Get second character
	s.mockClient.EXPECT().
		Get(s.ctx, "character:char_2").
		Return(redis.NewStringResult(string(charData2), nil))

	// Execute
	characters, err := listFunc()

	// Assert
	s.NoError(err)
	s.NotNil(characters)
	s.Len(characters, 2)
	s.Equal("char_1", characters[0].ID)
	s.Equal("char_2", characters[1].ID)
}

func (s *RedisRepositoryTestSuite) TestListByPlayerID() {
	char1, char2 := s.createTestCharactersForPlayer()

	s.Run("successful list", func() {
		s.testSuccessfulList(testPlayerKey, char1, char2, func() ([]*dnd5e.Character, error) {
			output, err := s.repo.ListByPlayerID(s.ctx, character.ListByPlayerIDInput{PlayerID: testPlayerID})
			if err != nil {
				return nil, err
			}
			return output.Characters, nil
		})
	})

	s.Run("successful list with stale index cleanup", func() {
		playerKey := testPlayerKey
		charData1, _ := json.Marshal(char1)

		// Get character IDs from index (includes stale entry)
		s.mockClient.EXPECT().
			SMembers(s.ctx, playerKey).
			Return(redis.NewStringSliceResult([]string{"char_1", "char_stale"}, nil))

		// Get first character
		s.mockClient.EXPECT().
			Get(s.ctx, "character:char_1").
			Return(redis.NewStringResult(string(charData1), nil))

		// Get stale character - not found
		s.mockClient.EXPECT().
			Get(s.ctx, "character:char_stale").
			Return(redis.NewStringResult("", redis.Nil))

		// Clean up stale index
		s.mockClient.EXPECT().
			SRem(s.ctx, playerKey, "char_stale").
			Return(redis.NewIntResult(1, nil))

		// Execute
		output, err := s.repo.ListByPlayerID(s.ctx, character.ListByPlayerIDInput{PlayerID: testPlayerID})

		// Assert
		s.NoError(err)
		s.NotNil(output)
		s.Len(output.Characters, 1)
		s.Equal("char_1", output.Characters[0].ID)
	})

	s.Run("empty list when player has no characters", func() {
		playerKey := testPlayerKey

		// Get character IDs from index - empty
		s.mockClient.EXPECT().
			SMembers(s.ctx, playerKey).
			Return(redis.NewStringSliceResult([]string{}, nil))

		// Execute
		output, err := s.repo.ListByPlayerID(s.ctx, character.ListByPlayerIDInput{PlayerID: testPlayerID})

		// Assert
		s.NoError(err)
		s.NotNil(output)
		s.Empty(output.Characters)
	})

	s.Run("error when player ID is empty", func() {
		output, err := s.repo.ListByPlayerID(s.ctx, character.ListByPlayerIDInput{PlayerID: ""})

		s.Error(err)
		s.Nil(output)
		s.True(errors.IsInvalidArgument(err))
		s.Contains(err.Error(), "player ID cannot be empty")
	})
}

func (s *RedisRepositoryTestSuite) TestListBySessionID() {
	char1, char2 := s.createTestCharactersForSession()

	s.Run("successful list", func() {
		s.testSuccessfulList(testSessionKey, char1, char2, func() ([]*dnd5e.Character, error) {
			output, err := s.repo.ListBySessionID(s.ctx, character.ListBySessionIDInput{SessionID: testSessionID})
			if err != nil {
				return nil, err
			}
			return output.Characters, nil
		})
	})

	s.Run("error when session ID is empty", func() {
		output, err := s.repo.ListBySessionID(s.ctx, character.ListBySessionIDInput{SessionID: ""})

		s.Error(err)
		s.Nil(output)
		s.True(errors.IsInvalidArgument(err))
		s.Contains(err.Error(), "session ID cannot be empty")
	})
}

func TestRedisRepositoryTestSuite(t *testing.T) {
	suite.Run(t, new(RedisRepositoryTestSuite))
}
