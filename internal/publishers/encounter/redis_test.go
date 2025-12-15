package encounter

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	redisclient "github.com/KirkDiggler/rpg-api/internal/redis"
)

// mockIDGenerator is a simple mock for testing
type mockIDGenerator struct {
	ids   []string
	index int
}

func (m *mockIDGenerator) Generate() string {
	if m.index >= len(m.ids) {
		return fmt.Sprintf("id-%d", m.index)
	}
	id := m.ids[m.index]
	m.index++
	return id
}

type RedisPublisherTestSuite struct {
	suite.Suite
	miniredis *miniredis.Miniredis
	client    redisclient.Client
	publisher Publisher
	idgen     *mockIDGenerator
	ctx       context.Context
}

func (s *RedisPublisherTestSuite) SetupTest() {
	// Create miniredis instance
	mr := miniredis.RunT(s.T())
	s.miniredis = mr

	// Create Redis client
	s.client = redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	// Create mock ID generator
	s.idgen = &mockIDGenerator{
		ids: []string{"sub-1", "sub-2", "sub-3"},
	}

	// Create publisher
	publisher, err := NewRedis(&RedisConfig{
		Client: s.client,
		IDGen:  s.idgen,
	})
	s.Require().NoError(err)
	s.publisher = publisher

	s.ctx = context.Background()
}

func (s *RedisPublisherTestSuite) TearDownTest() {
	if s.client != nil {
		s.client.Close()
	}
	if s.miniredis != nil {
		s.miniredis.Close()
	}
}

func TestRedisPublisherSuite(t *testing.T) {
	suite.Run(t, new(RedisPublisherTestSuite))
}

// NewRedis Tests

func (s *RedisPublisherTestSuite) TestNewRedis_Success() {
	// Act
	pub, err := NewRedis(&RedisConfig{
		Client: s.client,
		IDGen:  s.idgen,
	})

	// Assert
	s.Require().NoError(err)
	s.Assert().NotNil(pub)
}

func (s *RedisPublisherTestSuite) TestNewRedis_NilConfig() {
	// Act
	pub, err := NewRedis(nil)

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(pub)
	s.Assert().Contains(err.Error(), "config cannot be nil")
}

func (s *RedisPublisherTestSuite) TestNewRedis_NilClient() {
	// Act
	pub, err := NewRedis(&RedisConfig{
		Client: nil,
		IDGen:  s.idgen,
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(pub)
	s.Assert().Contains(err.Error(), "client cannot be nil")
}

func (s *RedisPublisherTestSuite) TestNewRedis_UsesDefaultIDGen() {
	// Act
	pub, err := NewRedis(&RedisConfig{
		Client: s.client,
		IDGen:  nil, // No IDGen provided
	})

	// Assert
	s.Require().NoError(err)
	s.Assert().NotNil(pub)
}

// Publish Tests

func (s *RedisPublisherTestSuite) TestPublish_Success() {
	// Arrange
	event := &entities.EncounterEvent{
		ID:          "event-1",
		Type:        entities.EventTypePlayerJoined,
		EncounterID: "enc-1",
		Timestamp:   time.Now(),
		PlayerJoined: &entities.PlayerJoinedEvent{
			PlayerID:    "player-1",
			CharacterID: "char-1",
			PlayerName:  "Test Player",
		},
	}

	input := &PublishInput{
		EncounterID: "enc-1",
		Event:       event,
	}

	// Act
	output, err := s.publisher.Publish(s.ctx, input)

	// Assert
	s.Require().NoError(err)
	s.Assert().NotNil(output)
}

func (s *RedisPublisherTestSuite) TestPublish_NilInput() {
	// Act
	output, err := s.publisher.Publish(s.ctx, nil)

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "input cannot be nil")
}

func (s *RedisPublisherTestSuite) TestPublish_NilEvent() {
	// Arrange
	input := &PublishInput{
		EncounterID: "enc-1",
		Event:       nil,
	}

	// Act
	output, err := s.publisher.Publish(s.ctx, input)

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), errEventNil)
}

func (s *RedisPublisherTestSuite) TestPublish_EmptyEncounterID() {
	// Arrange
	event := &entities.EncounterEvent{
		ID:           "event-1",
		Type:         entities.EventTypePlayerJoined,
		EncounterID:  "enc-1",
		Timestamp:    time.Now(),
		PlayerJoined: &entities.PlayerJoinedEvent{},
	}

	input := &PublishInput{
		EncounterID: "",
		Event:       event,
	}

	// Act
	output, err := s.publisher.Publish(s.ctx, input)

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), errEncounterIDEmpty)
}

// Subscribe Tests

func (s *RedisPublisherTestSuite) TestSubscribe_Success() {
	// Arrange
	input := &SubscribeInput{
		EncounterID: "enc-1",
	}

	// Act
	output, err := s.publisher.Subscribe(s.ctx, input)

	// Assert
	s.Require().NoError(err)
	s.Assert().NotNil(output)
	s.Assert().Equal("sub-1", output.SubscriptionID)
	s.Assert().NotNil(output.Events)
	s.Assert().NotNil(output.Errors)
}

func (s *RedisPublisherTestSuite) TestSubscribe_NilInput() {
	// Act
	output, err := s.publisher.Subscribe(s.ctx, nil)

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "input cannot be nil")
}

func (s *RedisPublisherTestSuite) TestSubscribe_EmptyEncounterID() {
	// Arrange
	input := &SubscribeInput{
		EncounterID: "",
	}

	// Act
	output, err := s.publisher.Subscribe(s.ctx, input)

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), errEncounterIDEmpty)
}

// Unsubscribe Tests

func (s *RedisPublisherTestSuite) TestUnsubscribe_Success() {
	// Arrange - Subscribe first
	subOutput, err := s.publisher.Subscribe(s.ctx, &SubscribeInput{
		EncounterID: "enc-1",
	})
	s.Require().NoError(err)
	subscriptionID := subOutput.SubscriptionID

	// Give a moment for subscription to initialize
	time.Sleep(10 * time.Millisecond)

	// Act
	output, err := s.publisher.Unsubscribe(s.ctx, &UnsubscribeInput{
		SubscriptionID: subscriptionID,
	})

	// Assert
	s.Require().NoError(err)
	s.Assert().NotNil(output)

	// Verify channels are closed
	select {
	case _, ok := <-subOutput.Events:
		s.Assert().False(ok, "events channel should be closed")
	case <-time.After(100 * time.Millisecond):
		s.Fail("events channel was not closed")
	}
}

func (s *RedisPublisherTestSuite) TestUnsubscribe_NilInput() {
	// Act
	output, err := s.publisher.Unsubscribe(s.ctx, nil)

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "input cannot be nil")
}

func (s *RedisPublisherTestSuite) TestUnsubscribe_EmptySubscriptionID() {
	// Arrange
	input := &UnsubscribeInput{
		SubscriptionID: "",
	}

	// Act
	output, err := s.publisher.Unsubscribe(s.ctx, input)

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), errSubscriptionIDEmpty)
}

func (s *RedisPublisherTestSuite) TestUnsubscribe_NotFound() {
	// Arrange
	input := &UnsubscribeInput{
		SubscriptionID: "nonexistent",
	}

	// Act
	output, err := s.publisher.Unsubscribe(s.ctx, input)

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().True(apierr.IsNotFound(err))
}

// Integration Tests

func (s *RedisPublisherTestSuite) TestPublishSubscribe_Integration() {
	// Arrange - Subscribe first
	subOutput, err := s.publisher.Subscribe(s.ctx, &SubscribeInput{
		EncounterID: "enc-1",
	})
	s.Require().NoError(err)

	// Give subscription time to establish
	time.Sleep(50 * time.Millisecond)

	// Prepare event
	event := &entities.EncounterEvent{
		ID:          "event-1",
		Type:        entities.EventTypePlayerJoined,
		EncounterID: "enc-1",
		Timestamp:   time.Now(),
		PlayerJoined: &entities.PlayerJoinedEvent{
			PlayerID:    "player-1",
			CharacterID: "char-1",
			PlayerName:  "Test Player",
		},
	}

	// Act - Publish event
	_, err = s.publisher.Publish(s.ctx, &PublishInput{
		EncounterID: "enc-1",
		Event:       event,
	})
	s.Require().NoError(err)

	// Assert - Receive event
	select {
	case receivedEvent := <-subOutput.Events:
		s.Assert().Equal(event.ID, receivedEvent.ID)
		s.Assert().Equal(event.Type, receivedEvent.Type)
		s.Assert().Equal(event.EncounterID, receivedEvent.EncounterID)
	case subErr := <-subOutput.Errors:
		s.Failf("received error", "unexpected error: %v", subErr)
	case <-time.After(1 * time.Second):
		s.Fail("timeout waiting for event")
	}

	// Cleanup
	_, err = s.publisher.Unsubscribe(s.ctx, &UnsubscribeInput{
		SubscriptionID: subOutput.SubscriptionID,
	})
	s.Require().NoError(err)
}

func (s *RedisPublisherTestSuite) TestMultipleSubscribers_Integration() {
	// Arrange - Create two subscribers for the same encounter
	sub1Output, err := s.publisher.Subscribe(s.ctx, &SubscribeInput{
		EncounterID: "enc-1",
	})
	s.Require().NoError(err)

	sub2Output, err := s.publisher.Subscribe(s.ctx, &SubscribeInput{
		EncounterID: "enc-1",
	})
	s.Require().NoError(err)

	// Give subscriptions time to establish
	time.Sleep(50 * time.Millisecond)

	// Prepare event
	event := &entities.EncounterEvent{
		ID:            "event-1",
		Type:          entities.EventTypeCombatStarted,
		EncounterID:   "enc-1",
		Timestamp:     time.Now(),
		CombatStarted: &entities.CombatStartedEvent{},
	}

	// Act - Publish event
	_, err = s.publisher.Publish(s.ctx, &PublishInput{
		EncounterID: "enc-1",
		Event:       event,
	})
	s.Require().NoError(err)

	// Assert - Both subscribers receive the event
	select {
	case receivedEvent := <-sub1Output.Events:
		s.Assert().Equal(event.ID, receivedEvent.ID)
	case <-time.After(1 * time.Second):
		s.Fail("subscriber 1 timeout")
	}

	select {
	case receivedEvent := <-sub2Output.Events:
		s.Assert().Equal(event.ID, receivedEvent.ID)
	case <-time.After(1 * time.Second):
		s.Fail("subscriber 2 timeout")
	}

	// Cleanup
	_, err = s.publisher.Unsubscribe(s.ctx, &UnsubscribeInput{
		SubscriptionID: sub1Output.SubscriptionID,
	})
	s.Require().NoError(err)

	_, err = s.publisher.Unsubscribe(s.ctx, &UnsubscribeInput{
		SubscriptionID: sub2Output.SubscriptionID,
	})
	s.Require().NoError(err)
}

func (s *RedisPublisherTestSuite) TestDifferentEncounters_Integration() {
	// Arrange - Create subscribers for different encounters
	sub1Output, err := s.publisher.Subscribe(s.ctx, &SubscribeInput{
		EncounterID: "enc-1",
	})
	s.Require().NoError(err)

	sub2Output, err := s.publisher.Subscribe(s.ctx, &SubscribeInput{
		EncounterID: "enc-2",
	})
	s.Require().NoError(err)

	// Give subscriptions time to establish
	time.Sleep(50 * time.Millisecond)

	// Prepare events for different encounters
	event1 := &entities.EncounterEvent{
		ID:           "event-1",
		Type:         entities.EventTypePlayerJoined,
		EncounterID:  "enc-1",
		Timestamp:    time.Now(),
		PlayerJoined: &entities.PlayerJoinedEvent{PlayerID: "player-1"},
	}

	event2 := &entities.EncounterEvent{
		ID:           "event-2",
		Type:         entities.EventTypePlayerJoined,
		EncounterID:  "enc-2",
		Timestamp:    time.Now(),
		PlayerJoined: &entities.PlayerJoinedEvent{PlayerID: "player-2"},
	}

	// Act - Publish events to different encounters
	_, err = s.publisher.Publish(s.ctx, &PublishInput{
		EncounterID: "enc-1",
		Event:       event1,
	})
	s.Require().NoError(err)

	_, err = s.publisher.Publish(s.ctx, &PublishInput{
		EncounterID: "enc-2",
		Event:       event2,
	})
	s.Require().NoError(err)

	// Assert - Each subscriber only receives events for their encounter
	select {
	case receivedEvent := <-sub1Output.Events:
		s.Assert().Equal("event-1", receivedEvent.ID)
		s.Assert().Equal("enc-1", receivedEvent.EncounterID)
	case <-time.After(1 * time.Second):
		s.Fail("subscriber 1 timeout")
	}

	select {
	case receivedEvent := <-sub2Output.Events:
		s.Assert().Equal("event-2", receivedEvent.ID)
		s.Assert().Equal("enc-2", receivedEvent.EncounterID)
	case <-time.After(1 * time.Second):
		s.Fail("subscriber 2 timeout")
	}

	// Verify no cross-contamination
	select {
	case event := <-sub1Output.Events:
		s.Failf("unexpected event", "sub1 received event for enc-2: %v", event)
	case <-time.After(100 * time.Millisecond):
		// Expected - no event
	}

	// Cleanup
	_, err = s.publisher.Unsubscribe(s.ctx, &UnsubscribeInput{
		SubscriptionID: sub1Output.SubscriptionID,
	})
	s.Require().NoError(err)

	_, err = s.publisher.Unsubscribe(s.ctx, &UnsubscribeInput{
		SubscriptionID: sub2Output.SubscriptionID,
	})
	s.Require().NoError(err)
}
