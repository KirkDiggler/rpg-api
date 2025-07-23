package characterdraft_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/redis/go-redis/v9"

	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
	"github.com/KirkDiggler/rpg-api/internal/errors"
	mockclock "github.com/KirkDiggler/rpg-api/internal/pkg/clock/mock"
	idgenmock "github.com/KirkDiggler/rpg-api/internal/pkg/idgen/mock"
	redismocks "github.com/KirkDiggler/rpg-api/internal/redis/mocks"
	characterdraft "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
)

const (
	testDraftPlayerKey = "draft:player:player_456"
	testDraftKey       = "draft:draft_123"
)

type RedisRepositoryTestSuite struct {
	suite.Suite
	ctrl       *gomock.Controller
	mockClock  *mockclock.MockClock
	mockIDGen  *idgenmock.MockGenerator
	mockClient *redismocks.MockClient
	mockPipe   *redismocks.MockPipeliner
	repo       characterdraft.Repository
	ctx        context.Context
}

func (s *RedisRepositoryTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockClient = redismocks.NewMockClient(s.ctrl)
	s.mockPipe = redismocks.NewMockPipeliner(s.ctrl)
	s.mockClock = mockclock.NewMockClock(s.ctrl)
	s.mockIDGen = idgenmock.NewMockGenerator(s.ctrl)

	// Create repository with proper config
	cfg := &characterdraft.Config{
		Client:      s.mockClient,
		Clock:       s.mockClock,
		IDGenerator: s.mockIDGen,
	}
	repo, err := characterdraft.NewRedis(cfg)
	s.Require().NoError(err)
	s.repo = repo

	s.ctx = context.Background()
}

func (s *RedisRepositoryTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *RedisRepositoryTestSuite) TestCreate() {
	now := time.Now().Add(1 * time.Hour) // Use future time to avoid expiration issues
	generatedID := "generated_draft_123"

	s.Run("successful create with no existing draft", func() {
		inputDraft := &dnd5e.CharacterDraftData{
			PlayerID:  "player_456",
			SessionID: "session_789",
			Name:      "Test Character",
			// No ID - repository will generate it
		}

		playerKey := "draft:player:player_456"
		draftKey := "draft:generated_draft_123"

		// Repository calls clock and idgen
		s.mockClock.EXPECT().Now().Return(now).AnyTimes()
		s.mockIDGen.EXPECT().Generate().Return(generatedID)

		// Check for existing draft (none found)
		s.mockClient.EXPECT().
			Get(s.ctx, playerKey).
			Return(redis.NewStringResult("", redis.Nil))

		// Setup pipeline
		s.mockClient.EXPECT().
			TxPipeline().
			Return(s.mockPipe)

		// Set draft with TTL and player mapping
		s.mockPipe.EXPECT().
			Set(s.ctx, draftKey, gomock.Any(), gomock.Any()).
			Return(redis.NewStatusResult("OK", nil))

		s.mockPipe.EXPECT().
			Set(s.ctx, playerKey, generatedID, time.Duration(0)).
			Return(redis.NewStatusResult("OK", nil))

		// Execute pipeline
		s.mockPipe.EXPECT().
			Exec(s.ctx).
			Return([]redis.Cmder{}, nil)

		// Execute
		output, err := s.repo.Create(s.ctx, characterdraft.CreateInput{Draft: inputDraft})

		// Assert
		s.NoError(err)
		s.NotNil(output)
		s.NotNil(output.Draft)
		s.Equal(generatedID, output.Draft.ID)
		s.Equal("player_456", output.Draft.PlayerID)
		s.Equal("Test Character", output.Draft.Name)
		s.Equal(now.Unix(), output.Draft.CreatedAt)
		s.Equal(now.Unix(), output.Draft.UpdatedAt)
		s.Equal(now.Add(24*time.Hour).Unix(), output.Draft.ExpiresAt)
	})

	s.Run("successful create replacing existing draft", func() {
		inputDraft := &dnd5e.CharacterDraftData{
			PlayerID:  "player_456",
			SessionID: "session_789",
			Name:      "New Character",
		}

		playerKey := "draft:player:player_456"
		oldDraftKey := "draft:old_draft_123"
		newDraftKey := "draft:generated_draft_123"

		// Repository calls clock and idgen
		s.mockClock.EXPECT().Now().Return(now).AnyTimes()
		s.mockIDGen.EXPECT().Generate().Return(generatedID)

		// Check for existing draft (found old one)
		s.mockClient.EXPECT().
			Get(s.ctx, playerKey).
			Return(redis.NewStringResult("old_draft_123", nil))

		// Setup pipeline
		s.mockClient.EXPECT().
			TxPipeline().
			Return(s.mockPipe)

		// Delete old draft
		s.mockPipe.EXPECT().
			Del(s.ctx, oldDraftKey).
			Return(redis.NewIntResult(1, nil))

		// Set new draft with TTL
		s.mockPipe.EXPECT().
			Set(s.ctx, newDraftKey, gomock.Any(), gomock.Any()).
			Return(redis.NewStatusResult("OK", nil))

		// Update player mapping
		s.mockPipe.EXPECT().
			Set(s.ctx, playerKey, generatedID, time.Duration(0)).
			Return(redis.NewStatusResult("OK", nil))

		// Execute pipeline
		s.mockPipe.EXPECT().
			Exec(s.ctx).
			Return([]redis.Cmder{}, nil)

		// Execute
		output, err := s.repo.Create(s.ctx, characterdraft.CreateInput{Draft: inputDraft})

		// Assert
		s.NoError(err)
		s.NotNil(output)
		s.NotNil(output.Draft)
		s.Equal(generatedID, output.Draft.ID)
		s.Equal("New Character", output.Draft.Name)
	})

	s.Run("error when draft is nil", func() {
		output, err := s.repo.Create(s.ctx, characterdraft.CreateInput{Draft: nil})

		s.Error(err)
		s.Nil(output)
		s.True(errors.IsInvalidArgument(err))
		s.Contains(err.Error(), "draft cannot be nil")
	})

	s.Run("error when player ID is empty", func() {
		draft := &dnd5e.CharacterDraftData{Name: "Test"}
		output, err := s.repo.Create(s.ctx, characterdraft.CreateInput{Draft: draft})

		s.Error(err)
		s.Nil(output)
		s.True(errors.IsInvalidArgument(err))
		s.Contains(err.Error(), "player ID cannot be empty")
	})

	s.Run("error when draft has already expired", func() {
		expiredDraft := &dnd5e.CharacterDraftData{
			ID:        "draft_123",
			PlayerID:  "player_456",
			ExpiresAt: time.Now().Add(-1 * time.Hour).Unix(), // Expired
		}

		output, err := s.repo.Create(s.ctx, characterdraft.CreateInput{Draft: expiredDraft})

		s.Error(err)
		s.Nil(output)
		s.True(errors.IsInvalidArgument(err))
		s.Contains(err.Error(), "draft has already expired")
	})
}

func (s *RedisRepositoryTestSuite) TestGet() {
	testDraft := &dnd5e.CharacterDraftData{
		ID:       "draft_123",
		PlayerID: "player_456",
		Name:     "Test Character",
	}

	s.Run("successful get", func() {
		draftKey := testDraftKey
		draftData, _ := json.Marshal(testDraft)

		s.mockClient.EXPECT().
			Get(s.ctx, draftKey).
			Return(redis.NewStringResult(string(draftData), nil))

		// Execute
		output, err := s.repo.Get(s.ctx, characterdraft.GetInput{ID: "draft_123"})

		// Assert
		s.NoError(err)
		s.NotNil(output)
		s.NotNil(output.Draft)
		s.Equal(testDraft.ID, output.Draft.ID)
		s.Equal(testDraft.PlayerID, output.Draft.PlayerID)
		s.Equal(testDraft.Name, output.Draft.Name)
	})

	s.Run("error when draft not found", func() {
		draftKey := testDraftKey

		s.mockClient.EXPECT().
			Get(s.ctx, draftKey).
			Return(redis.NewStringResult("", redis.Nil))

		// Execute
		output, err := s.repo.Get(s.ctx, characterdraft.GetInput{ID: "draft_123"})

		// Assert
		s.Error(err)
		s.Nil(output)
		s.True(errors.IsNotFound(err))
		s.Contains(err.Error(), "draft with ID draft_123 not found")
	})

	s.Run("error when ID is empty", func() {
		output, err := s.repo.Get(s.ctx, characterdraft.GetInput{ID: ""})

		s.Error(err)
		s.Nil(output)
		s.True(errors.IsInvalidArgument(err))
		s.Contains(err.Error(), "draft ID cannot be empty")
	})
}

func (s *RedisRepositoryTestSuite) TestGetByPlayerID() {
	testDraft := &dnd5e.CharacterDraftData{
		ID:       "draft_123",
		PlayerID: "player_456",
		Name:     "Test Character",
	}

	s.Run("successful get by player ID", func() {
		playerKey := testDraftPlayerKey
		draftKey := testDraftKey
		draftData, _ := json.Marshal(testDraft)

		// Get draft ID from player mapping
		s.mockClient.EXPECT().
			Get(s.ctx, playerKey).
			Return(redis.NewStringResult("draft_123", nil))

		// Get draft data
		s.mockClient.EXPECT().
			Get(s.ctx, draftKey).
			Return(redis.NewStringResult(string(draftData), nil))

		// Execute
		output, err := s.repo.GetByPlayerID(s.ctx, characterdraft.GetByPlayerIDInput{PlayerID: "player_456"})

		// Assert
		s.NoError(err)
		s.NotNil(output)
		s.NotNil(output.Draft)
		s.Equal(testDraft.ID, output.Draft.ID)
		s.Equal(testDraft.PlayerID, output.Draft.PlayerID)
	})

	s.Run("error when player has no draft", func() {
		playerKey := testDraftPlayerKey

		s.mockClient.EXPECT().
			Get(s.ctx, playerKey).
			Return(redis.NewStringResult("", redis.Nil))

		// Execute
		output, err := s.repo.GetByPlayerID(s.ctx, characterdraft.GetByPlayerIDInput{PlayerID: "player_456"})

		// Assert
		s.Error(err)
		s.Nil(output)
		s.True(errors.IsNotFound(err))
		s.Contains(err.Error(), "no draft found for player player_456")
	})

	s.Run("cleanup stale mapping when draft doesn't exist", func() {
		playerKey := testDraftPlayerKey
		draftKey := testDraftKey

		// Get draft ID from player mapping
		s.mockClient.EXPECT().
			Get(s.ctx, playerKey).
			Return(redis.NewStringResult("draft_123", nil))

		// Draft doesn't exist
		s.mockClient.EXPECT().
			Get(s.ctx, draftKey).
			Return(redis.NewStringResult("", redis.Nil))

		// Clean up stale mapping
		s.mockClient.EXPECT().
			Del(s.ctx, playerKey).
			Return(redis.NewIntResult(1, nil))

		// Execute
		output, err := s.repo.GetByPlayerID(s.ctx, characterdraft.GetByPlayerIDInput{PlayerID: "player_456"})

		// Assert
		s.Error(err)
		s.Nil(output)
		s.True(errors.IsNotFound(err))
	})

	s.Run("error when player ID is empty", func() {
		output, err := s.repo.GetByPlayerID(s.ctx, characterdraft.GetByPlayerIDInput{PlayerID: ""})

		s.Error(err)
		s.Nil(output)
		s.True(errors.IsInvalidArgument(err))
		s.Contains(err.Error(), "player ID cannot be empty")
	})
}

func (s *RedisRepositoryTestSuite) TestUpdate() {
	now := time.Now().Add(1 * time.Hour)

	s.Run("successful update", func() {
		inputDraft := &dnd5e.CharacterDraftData{
			ID:       "draft_123",
			PlayerID: "player_456",
			Name:     "Updated Character",
			// Repository will set UpdatedAt
		}

		draftKey := "draft:draft_123"

		// Repository calls clock for UpdatedAt
		s.mockClock.EXPECT().Now().Return(now)

		// Check existence
		s.mockClient.EXPECT().
			Exists(s.ctx, draftKey).
			Return(redis.NewIntResult(1, nil))

		// Update draft
		s.mockClient.EXPECT().
			Set(s.ctx, draftKey, gomock.Any(), gomock.Any()).
			Return(redis.NewStatusResult("OK", nil))

		// Execute
		output, err := s.repo.Update(s.ctx, characterdraft.UpdateInput{Draft: inputDraft})

		// Assert
		s.NoError(err)
		s.NotNil(output)
		s.NotNil(output.Draft)
		s.Equal("draft_123", output.Draft.ID)
		s.Equal("Updated Character", output.Draft.Name)
		s.Equal(now.Unix(), output.Draft.UpdatedAt)
	})

	s.Run("error when draft doesn't exist", func() {
		inputDraft := &dnd5e.CharacterDraftData{
			ID:       "draft_123",
			PlayerID: "player_456",
			Name:     "Updated Character",
		}
		draftKey := "draft:draft_123"

		// Check existence
		s.mockClient.EXPECT().
			Exists(s.ctx, draftKey).
			Return(redis.NewIntResult(0, nil))

		// Execute
		output, err := s.repo.Update(s.ctx, characterdraft.UpdateInput{Draft: inputDraft})

		// Assert
		s.Error(err)
		s.Nil(output)
		s.True(errors.IsNotFound(err))
		s.Contains(err.Error(), "draft with ID draft_123 not found")
	})

	s.Run("error when draft is nil", func() {
		output, err := s.repo.Update(s.ctx, characterdraft.UpdateInput{Draft: nil})

		s.Error(err)
		s.Nil(output)
		s.True(errors.IsInvalidArgument(err))
		s.Contains(err.Error(), "draft cannot be nil")
	})

	s.Run("error when draft ID is empty", func() {
		draft := &dnd5e.CharacterDraftData{PlayerID: "player_456"}
		output, err := s.repo.Update(s.ctx, characterdraft.UpdateInput{Draft: draft})

		s.Error(err)
		s.Nil(output)
		s.True(errors.IsInvalidArgument(err))
		s.Contains(err.Error(), "draft ID cannot be empty")
	})
}

func (s *RedisRepositoryTestSuite) TestDelete() {
	testDraft := &dnd5e.CharacterDraftData{
		ID:       "draft_123",
		PlayerID: "player_456",
		Name:     "Test Character",
	}

	s.Run("successful delete", func() {
		draftKey := testDraftKey
		playerKey := testDraftPlayerKey
		draftData, _ := json.Marshal(testDraft)

		// Get draft to find player ID
		s.mockClient.EXPECT().
			Get(s.ctx, draftKey).
			Return(redis.NewStringResult(string(draftData), nil))

		// Setup pipeline
		s.mockClient.EXPECT().
			TxPipeline().
			Return(s.mockPipe)

		// Delete draft
		s.mockPipe.EXPECT().
			Del(s.ctx, draftKey).
			Return(redis.NewIntResult(1, nil))

		// Delete player mapping
		s.mockPipe.EXPECT().
			Del(s.ctx, playerKey).
			Return(redis.NewIntResult(1, nil))

		// Execute pipeline
		s.mockPipe.EXPECT().
			Exec(s.ctx).
			Return([]redis.Cmder{}, nil)

		// Execute
		output, err := s.repo.Delete(s.ctx, characterdraft.DeleteInput{ID: "draft_123"})

		// Assert
		s.NoError(err)
		s.NotNil(output)
	})

	s.Run("error when draft not found", func() {
		draftKey := testDraftKey

		// Get draft returns not found
		s.mockClient.EXPECT().
			Get(s.ctx, draftKey).
			Return(redis.NewStringResult("", redis.Nil))

		// Execute
		output, err := s.repo.Delete(s.ctx, characterdraft.DeleteInput{ID: "draft_123"})

		// Assert
		s.Error(err)
		s.Nil(output)
		s.True(errors.IsNotFound(err))
	})

	s.Run("error when ID is empty", func() {
		output, err := s.repo.Delete(s.ctx, characterdraft.DeleteInput{ID: ""})

		s.Error(err)
		s.Nil(output)
		s.True(errors.IsInvalidArgument(err))
		s.Contains(err.Error(), "draft ID cannot be empty")
	})
}

func TestRedisRepositoryTestSuite(t *testing.T) {
	suite.Run(t, new(RedisRepositoryTestSuite))
}
