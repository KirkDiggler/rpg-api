package characterdraft_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/redis/go-redis/v9"

	"github.com/KirkDiggler/rpg-api/internal/errors"
	mockclock "github.com/KirkDiggler/rpg-api/internal/pkg/clock/mock"
	idgenmock "github.com/KirkDiggler/rpg-api/internal/pkg/idgen/mock"
	redismocks "github.com/KirkDiggler/rpg-api/internal/redis/mocks"
	characterdraft "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
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

	// Test data - reset in SetupSubTest
	testDraft *character.DraftData
	testTime  time.Time
}

func (s *RedisRepositoryTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockClient = redismocks.NewMockClient(s.ctrl)
	s.mockPipe = redismocks.NewMockPipeliner(s.ctrl)
	s.mockClock = mockclock.NewMockClock(s.ctrl)
	s.mockIDGen = idgenmock.NewMockGenerator(s.ctrl)

	// Create repository through the interface factory
	repo, err := characterdraft.NewRedis(&characterdraft.Config{
		Client:      s.mockClient,
		Clock:       s.mockClock,
		IDGenerator: s.mockIDGen,
	})
	s.Require().NoError(err)
	s.repo = repo

	s.ctx = context.Background()
}

// SetupSubTest runs before each s.Run() - reset test data to clean state
func (s *RedisRepositoryTestSuite) SetupSubTest() {
	s.testTime = time.Now()

	// Create fresh draft data that tests can modify
	s.testDraft = &character.DraftData{
		ID:            "draft_123",
		PlayerID:      "player_456",
		Name:          "Test Character",
		Choices:       make(map[shared.ChoiceCategory]any),
		ProgressFlags: 0,
		CreatedAt:     s.testTime,
		UpdatedAt:     s.testTime,
	}
}

func (s *RedisRepositoryTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

// Test Create method
func (s *RedisRepositoryTestSuite) TestCreate() {
	now := time.Now()
	generatedID := "draft_123"

	s.Run("successful create with no existing draft", func() {
		// Test wants ID to be generated, so clear it
		s.testDraft.ID = ""
		inputDraft := s.testDraft

		// Setup expectations
		s.mockClock.EXPECT().Now().Return(now) // Called once for timestamps
		s.mockIDGen.EXPECT().Generate().Return(generatedID)

		// Check for existing draft (none exists)
		getCmd := redis.NewStringResult("", redis.Nil)
		s.mockClient.EXPECT().
			Get(s.ctx, testDraftPlayerKey).
			Return(getCmd)

		// Expect pipeline operations
		s.mockClient.EXPECT().TxPipeline().Return(s.mockPipe)

		// Expect draft key set with TTL
		expectedDraft := *inputDraft
		expectedDraft.ID = generatedID
		expectedDraft.CreatedAt = now
		expectedDraft.UpdatedAt = now

		draftData, err := json.Marshal(&expectedDraft)
		s.Require().NoError(err)

		setCmd := redis.NewStatusCmd(s.ctx)
		s.mockPipe.EXPECT().
			Set(s.ctx, testDraftKey, draftData, 24*time.Hour).
			Return(setCmd)

		// Expect player mapping (no TTL for player mapping)
		playerSetCmd := redis.NewStatusCmd(s.ctx)
		s.mockPipe.EXPECT().
			Set(s.ctx, testDraftPlayerKey, generatedID, time.Duration(0)).
			Return(playerSetCmd)

		// Execute pipeline
		s.mockPipe.EXPECT().
			Exec(s.ctx).
			Return([]redis.Cmder{setCmd, playerSetCmd}, nil)

		// Execute
		output, err := s.repo.Create(s.ctx, characterdraft.CreateInput{Draft: inputDraft})

		// Assert
		s.NoError(err)
		s.NotNil(output)
		s.NotNil(output.Draft)
		s.Equal(generatedID, output.Draft.ID)
		s.Equal("player_456", output.Draft.PlayerID)
		s.Equal("Test Character", output.Draft.Name)
		s.Equal(now, output.Draft.CreatedAt)
		s.Equal(now, output.Draft.UpdatedAt)
	})

	s.Run("successful create replacing existing draft", func() {
		// Test wants ID to be generated, so clear it
		s.testDraft.ID = ""
		s.testDraft.Name = "New Character"
		s.testDraft.Choices[shared.ChoiceName] = "New Character"
		inputDraft := s.testDraft

		// Setup expectations
		s.mockClock.EXPECT().Now().Return(now) // Called once for timestamps
		s.mockIDGen.EXPECT().Generate().Return(generatedID)

		// Get existing draft ID from player mapping
		getCmd := redis.NewStringResult("old_draft_123", nil)
		s.mockClient.EXPECT().
			Get(s.ctx, testDraftPlayerKey).
			Return(getCmd)

		// Begin transaction
		s.mockClient.EXPECT().TxPipeline().Return(s.mockPipe)

		// Delete old draft
		delCmd := redis.NewIntCmd(s.ctx)
		s.mockPipe.EXPECT().
			Del(s.ctx, "draft:old_draft_123").
			Return(delCmd)

		// Create new draft
		expectedDraft := *inputDraft
		expectedDraft.ID = generatedID
		expectedDraft.CreatedAt = now
		expectedDraft.UpdatedAt = now

		draftData, err := json.Marshal(&expectedDraft)
		s.Require().NoError(err)

		setCmd := redis.NewStatusCmd(s.ctx)
		s.mockPipe.EXPECT().
			Set(s.ctx, testDraftKey, draftData, 24*time.Hour).
			Return(setCmd)

		// Update player mapping (no TTL for player mapping)
		playerSetCmd := redis.NewStatusCmd(s.ctx)
		s.mockPipe.EXPECT().
			Set(s.ctx, testDraftPlayerKey, generatedID, time.Duration(0)).
			Return(playerSetCmd)

		// Execute pipeline
		s.mockPipe.EXPECT().
			Exec(s.ctx).
			Return([]redis.Cmder{delCmd, setCmd, playerSetCmd}, nil)

		// Execute
		output, err := s.repo.Create(s.ctx, characterdraft.CreateInput{Draft: inputDraft})

		// Assert
		s.NoError(err)
		s.NotNil(output)
		s.Equal(generatedID, output.Draft.ID)
	})
}

// Test Get method
func (s *RedisRepositoryTestSuite) TestGet() {

	s.Run("successful get", func() {
		// Use the test draft data
		draft := s.testDraft

		draftData, err := json.Marshal(draft)
		s.Require().NoError(err)

		getCmd := redis.NewStringResult(string(draftData), nil)
		s.mockClient.EXPECT().
			Get(s.ctx, testDraftKey).
			Return(getCmd)

		// Execute
		output, err := s.repo.Get(s.ctx, characterdraft.GetInput{ID: "draft_123"})

		// Assert
		s.NoError(err)
		s.NotNil(output)
		s.Equal("draft_123", output.Draft.ID)
		s.Equal("player_456", output.Draft.PlayerID)
		s.Equal("Test Character", output.Draft.Name)
	})

	s.Run("draft not found", func() {
		getCmd := redis.NewStringResult("", redis.Nil)
		s.mockClient.EXPECT().
			Get(s.ctx, testDraftKey).
			Return(getCmd)

		// Execute
		output, err := s.repo.Get(s.ctx, characterdraft.GetInput{ID: "draft_123"})

		// Assert
		s.Error(err)
		s.True(errors.IsNotFound(err))
		s.Nil(output)
	})
}

// Test GetByPlayerID method
func (s *RedisRepositoryTestSuite) TestGetByPlayerID() {

	s.Run("successful get by player ID", func() {
		// Get draft ID from player mapping
		playerGetCmd := redis.NewStringResult("draft_123", nil)
		s.mockClient.EXPECT().
			Get(s.ctx, testDraftPlayerKey).
			Return(playerGetCmd)

		// Use the test draft data
		draft := s.testDraft

		draftData, err := json.Marshal(draft)
		s.Require().NoError(err)

		draftGetCmd := redis.NewStringResult(string(draftData), nil)
		s.mockClient.EXPECT().
			Get(s.ctx, testDraftKey).
			Return(draftGetCmd)

		// Execute
		output, err := s.repo.GetByPlayerID(s.ctx, characterdraft.GetByPlayerIDInput{PlayerID: "player_456"})

		// Assert
		s.NoError(err)
		s.NotNil(output)
		s.Equal("draft_123", output.Draft.ID)
		s.Equal("player_456", output.Draft.PlayerID)
	})

	s.Run("no draft for player", func() {
		getCmd := redis.NewStringResult("", redis.Nil)
		s.mockClient.EXPECT().
			Get(s.ctx, testDraftPlayerKey).
			Return(getCmd)

		// Execute
		output, err := s.repo.GetByPlayerID(s.ctx, characterdraft.GetByPlayerIDInput{PlayerID: "player_456"})

		// Assert
		s.Error(err)
		s.True(errors.IsNotFound(err))
		s.Nil(output)
	})

	s.Run("draft exists but is deleted - cleanup mapping", func() {
		// Get draft ID from player mapping
		playerGetCmd := redis.NewStringResult("draft_123", nil)
		s.mockClient.EXPECT().
			Get(s.ctx, testDraftPlayerKey).
			Return(playerGetCmd)

		// Draft doesn't exist
		draftGetCmd := redis.NewStringResult("", redis.Nil)
		s.mockClient.EXPECT().
			Get(s.ctx, testDraftKey).
			Return(draftGetCmd)

		// Cleanup mapping
		delCmd := redis.NewIntResult(1, nil)
		s.mockClient.EXPECT().
			Del(s.ctx, testDraftPlayerKey).
			Return(delCmd)

		// Execute
		output, err := s.repo.GetByPlayerID(s.ctx, characterdraft.GetByPlayerIDInput{PlayerID: "player_456"})

		// Assert
		s.Error(err)
		s.True(errors.IsNotFound(err))
		s.Nil(output)
	})
}

// Test Update method
func (s *RedisRepositoryTestSuite) TestUpdate() {
	now := time.Now()

	s.Run("successful update", func() {
		// Update test draft for this case
		s.testDraft.Name = "Updated Character"
		s.testDraft.Choices[shared.ChoiceName] = "Updated Character"
		inputDraft := s.testDraft

		// Check if exists
		existsCmd := redis.NewIntResult(1, nil)
		s.mockClient.EXPECT().
			Exists(s.ctx, testDraftKey).
			Return(existsCmd)

		// Update timestamp
		s.mockClock.EXPECT().Now().Return(now)

		// Save updated draft
		expectedDraft := *inputDraft
		expectedDraft.UpdatedAt = now

		draftData, err := json.Marshal(&expectedDraft)
		s.Require().NoError(err)

		setCmd := redis.NewStatusResult("OK", nil)
		s.mockClient.EXPECT().
			Set(s.ctx, testDraftKey, draftData, 24*time.Hour).
			Return(setCmd)

		// Execute
		output, err := s.repo.Update(s.ctx, characterdraft.UpdateInput{Draft: inputDraft})

		// Assert
		s.NoError(err)
		s.NotNil(output)
		s.Equal("draft_123", output.Draft.ID)
		s.Equal(now, output.Draft.UpdatedAt)
	})

	s.Run("error when draft doesn't exist", func() {
		// Update test draft for this case
		s.testDraft.Name = "Updated Character"
		s.testDraft.Choices[shared.ChoiceName] = "Updated Character"
		inputDraft := s.testDraft

		// Check if exists - not found
		existsCmd := redis.NewIntResult(0, nil)
		s.mockClient.EXPECT().
			Exists(s.ctx, testDraftKey).
			Return(existsCmd)

		// Execute
		output, err := s.repo.Update(s.ctx, characterdraft.UpdateInput{Draft: inputDraft})

		// Assert
		s.Error(err)
		s.True(errors.IsNotFound(err))
		s.Nil(output)
	})
}

// Test Delete method
func (s *RedisRepositoryTestSuite) TestDelete() {

	s.Run("successful delete", func() {
		// Use the test draft data
		draft := s.testDraft

		draftData, err := json.Marshal(draft)
		s.Require().NoError(err)

		getCmd := redis.NewStringResult(string(draftData), nil)
		s.mockClient.EXPECT().
			Get(s.ctx, testDraftKey).
			Return(getCmd)

		// Setup pipeline for deletion
		s.mockClient.EXPECT().TxPipeline().Return(s.mockPipe)

		// Delete draft
		delDraftCmd := redis.NewIntCmd(s.ctx)
		s.mockPipe.EXPECT().
			Del(s.ctx, testDraftKey).
			Return(delDraftCmd)

		// Delete player mapping
		delPlayerCmd := redis.NewIntCmd(s.ctx)
		s.mockPipe.EXPECT().
			Del(s.ctx, testDraftPlayerKey).
			Return(delPlayerCmd)

		// Execute pipeline
		s.mockPipe.EXPECT().
			Exec(s.ctx).
			Return([]redis.Cmder{delDraftCmd, delPlayerCmd}, nil)

		// Execute
		output, err := s.repo.Delete(s.ctx, characterdraft.DeleteInput{ID: "draft_123"})

		// Assert
		s.NoError(err)
		s.NotNil(output)
	})

	s.Run("error when draft doesn't exist", func() {
		getCmd := redis.NewStringResult("", redis.Nil)
		s.mockClient.EXPECT().
			Get(s.ctx, testDraftKey).
			Return(getCmd)

		// Execute
		output, err := s.repo.Delete(s.ctx, characterdraft.DeleteInput{ID: "draft_123"})

		// Assert
		s.Error(err)
		s.True(errors.IsNotFound(err))
		s.Nil(output)
	})
}

func TestRedisRepositoryTestSuite(t *testing.T) {
	suite.Run(t, new(RedisRepositoryTestSuite))
}
