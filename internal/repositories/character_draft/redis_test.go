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
	"github.com/KirkDiggler/rpg-api/internal/testutils/builders"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
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

	repo, err := characterdraft.NewRedisRepository(cfg)
	s.Require().NoError(err)
	s.repo = repo

	s.ctx = context.Background()
}

func (s *RedisRepositoryTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

// Test Create method
func (s *RedisRepositoryTestSuite) TestCreate() {
	now := time.Now()
	generatedID := "draft_123"

	s.Run("successful create with no existing draft", func() {
		inputDraft := builders.NewToolkitDraftDataBuilder().
			WithPlayerID("player_456").
			WithName("Test Character").
			Build()

		// Setup expectations
		s.mockClock.EXPECT().Now().Return(now).Times(2)
		s.mockIDGen.EXPECT().Generate().Return(generatedID)

		// Expect pipeline operations
		s.mockClient.EXPECT().TxPipeline().Return(s.mockPipe)

		// Expect draft key set with TTL
		expectedDraft := *inputDraft
		expectedDraft.ID = generatedID
		expectedDraft.CreatedAt = now
		expectedDraft.UpdatedAt = now

		draftData, err := json.Marshal(&expectedDraft)
		s.Require().NoError(err)

		s.mockPipe.EXPECT().
			Set(s.ctx, testDraftKey, draftData, 24*time.Hour).
			Return(nil)

		// Expect player mapping
		s.mockPipe.EXPECT().
			Set(s.ctx, testDraftPlayerKey, generatedID, 24*time.Hour).
			Return(nil)

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
		s.Equal(now, output.Draft.CreatedAt)
		s.Equal(now, output.Draft.UpdatedAt)
	})

	s.Run("successful create replacing existing draft", func() {
		inputDraft := builders.NewToolkitDraftDataBuilder().
			WithPlayerID("player_456").
			WithName("New Character").
			Build()

		// Setup expectations
		s.mockClock.EXPECT().Now().Return(now).Times(2)
		s.mockIDGen.EXPECT().Generate().Return(generatedID)

		// Get existing draft ID from player mapping
		s.mockClient.EXPECT().
			Get(s.ctx, testDraftPlayerKey).
			Return(&redismocks.MockStringCmd{
				ValFunc: func() string { return "old_draft_123" },
				ErrFunc: func() error { return nil },
			})

		// Begin transaction
		s.mockClient.EXPECT().TxPipeline().Return(s.mockPipe)

		// Delete old draft
		s.mockPipe.EXPECT().
			Del(s.ctx, "draft:old_draft_123").
			Return(nil)

		// Create new draft
		expectedDraft := *inputDraft
		expectedDraft.ID = generatedID
		expectedDraft.CreatedAt = now
		expectedDraft.UpdatedAt = now

		draftData, err := json.Marshal(&expectedDraft)
		s.Require().NoError(err)

		s.mockPipe.EXPECT().
			Set(s.ctx, testDraftKey, draftData, 24*time.Hour).
			Return(nil)

		// Update player mapping
		s.mockPipe.EXPECT().
			Set(s.ctx, testDraftPlayerKey, generatedID, 24*time.Hour).
			Return(nil)

		// Execute pipeline
		s.mockPipe.EXPECT().
			Exec(s.ctx).
			Return([]redis.Cmder{}, nil)

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
	now := time.Now()

	s.Run("successful get", func() {
		draft := builders.NewToolkitDraftDataBuilder().
			WithID("draft_123").
			WithPlayerID("player_456").
			WithName("Test Character").
			WithTimestamps(now, now).
			Build()

		draftData, err := json.Marshal(draft)
		s.Require().NoError(err)

		s.mockClient.EXPECT().
			Get(s.ctx, testDraftKey).
			Return(&redismocks.MockStringCmd{
				ValFunc: func() string { return string(draftData) },
				ErrFunc: func() error { return nil },
			})

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
		s.mockClient.EXPECT().
			Get(s.ctx, testDraftKey).
			Return(&redismocks.MockStringCmd{
				ValFunc: func() string { return "" },
				ErrFunc: func() error { return redis.Nil },
			})

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
	now := time.Now()

	s.Run("successful get by player ID", func() {
		// Get draft ID from player mapping
		s.mockClient.EXPECT().
			Get(s.ctx, testDraftPlayerKey).
			Return(&redismocks.MockStringCmd{
				ValFunc: func() string { return "draft_123" },
				ErrFunc: func() error { return nil },
			})

		// Get actual draft
		draft := builders.NewToolkitDraftDataBuilder().
			WithID("draft_123").
			WithPlayerID("player_456").
			WithName("Test Character").
			WithTimestamps(now, now).
			Build()

		draftData, err := json.Marshal(draft)
		s.Require().NoError(err)

		s.mockClient.EXPECT().
			Get(s.ctx, testDraftKey).
			Return(&redismocks.MockStringCmd{
				ValFunc: func() string { return string(draftData) },
				ErrFunc: func() error { return nil },
			})

		// Execute
		output, err := s.repo.GetByPlayerID(s.ctx, characterdraft.GetByPlayerIDInput{PlayerID: "player_456"})

		// Assert
		s.NoError(err)
		s.NotNil(output)
		s.Equal("draft_123", output.Draft.ID)
		s.Equal("player_456", output.Draft.PlayerID)
	})

	s.Run("no draft for player", func() {
		s.mockClient.EXPECT().
			Get(s.ctx, testDraftPlayerKey).
			Return(&redismocks.MockStringCmd{
				ValFunc: func() string { return "" },
				ErrFunc: func() error { return redis.Nil },
			})

		// Execute
		output, err := s.repo.GetByPlayerID(s.ctx, characterdraft.GetByPlayerIDInput{PlayerID: "player_456"})

		// Assert
		s.Error(err)
		s.True(errors.IsNotFound(err))
		s.Nil(output)
	})

	s.Run("draft exists but is deleted - cleanup mapping", func() {
		// Get draft ID from player mapping
		s.mockClient.EXPECT().
			Get(s.ctx, testDraftPlayerKey).
			Return(&redismocks.MockStringCmd{
				ValFunc: func() string { return "draft_123" },
				ErrFunc: func() error { return nil },
			})

		// Draft doesn't exist
		s.mockClient.EXPECT().
			Get(s.ctx, testDraftKey).
			Return(&redismocks.MockStringCmd{
				ValFunc: func() string { return "" },
				ErrFunc: func() error { return redis.Nil },
			})

		// Cleanup mapping
		s.mockClient.EXPECT().
			Del(s.ctx, testDraftPlayerKey).
			Return(nil)

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
		inputDraft := builders.NewToolkitDraftDataBuilder().
			WithID("draft_123").
			WithPlayerID("player_456").
			WithName("Updated Character").
			Build()

		// Check if exists
		s.mockClient.EXPECT().
			Exists(s.ctx, testDraftKey).
			Return(&redismocks.MockIntCmd{
				ValFunc: func() int64 { return 1 },
				ErrFunc: func() error { return nil },
			})

		// Update timestamp
		s.mockClock.EXPECT().Now().Return(now)

		// Save updated draft
		expectedDraft := *inputDraft
		expectedDraft.UpdatedAt = now

		draftData, err := json.Marshal(&expectedDraft)
		s.Require().NoError(err)

		s.mockClient.EXPECT().
			Set(s.ctx, testDraftKey, draftData, 24*time.Hour).
			Return(nil)

		// Execute
		output, err := s.repo.Update(s.ctx, characterdraft.UpdateInput{Draft: inputDraft})

		// Assert
		s.NoError(err)
		s.NotNil(output)
		s.Equal("draft_123", output.Draft.ID)
		s.Equal(now, output.Draft.UpdatedAt)
	})

	s.Run("error when draft doesn't exist", func() {
		inputDraft := builders.NewToolkitDraftDataBuilder().
			WithID("draft_123").
			WithPlayerID("player_456").
			WithName("Updated Character").
			Build()

		// Check if exists - not found
		s.mockClient.EXPECT().
			Exists(s.ctx, testDraftKey).
			Return(&redismocks.MockIntCmd{
				ValFunc: func() int64 { return 0 },
				ErrFunc: func() error { return nil },
			})

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
	now := time.Now()

	s.Run("successful delete", func() {
		// Get draft to find player ID
		draft := builders.NewToolkitDraftDataBuilder().
			WithID("draft_123").
			WithPlayerID("player_456").
			WithTimestamps(now, now).
			Build()

		draftData, err := json.Marshal(draft)
		s.Require().NoError(err)

		s.mockClient.EXPECT().
			Get(s.ctx, testDraftKey).
			Return(&redismocks.MockStringCmd{
				ValFunc: func() string { return string(draftData) },
				ErrFunc: func() error { return nil },
			})

		// Setup pipeline for deletion
		s.mockClient.EXPECT().TxPipeline().Return(s.mockPipe)

		// Delete draft
		s.mockPipe.EXPECT().
			Del(s.ctx, testDraftKey).
			Return(nil)

		// Delete player mapping
		s.mockPipe.EXPECT().
			Del(s.ctx, testDraftPlayerKey).
			Return(nil)

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

	s.Run("error when draft doesn't exist", func() {
		s.mockClient.EXPECT().
			Get(s.ctx, testDraftKey).
			Return(&redismocks.MockStringCmd{
				ValFunc: func() string { return "" },
				ErrFunc: func() error { return redis.Nil },
			})

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