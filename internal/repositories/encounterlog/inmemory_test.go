package encounterlog

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-api/internal/entities"
)

type InMemoryRepositoryTestSuite struct {
	suite.Suite
	repo *InMemoryRepository
	ctx  context.Context
}

func (s *InMemoryRepositoryTestSuite) SetupTest() {
	s.repo = NewInMemory()
	s.ctx = context.Background()
}

func TestInMemoryRepositorySuite(t *testing.T) {
	suite.Run(t, new(InMemoryRepositoryTestSuite))
}

// Helper to create test events
func (s *InMemoryRepositoryTestSuite) createEvent(id, encounterID string) *entities.EncounterEvent {
	return &entities.EncounterEvent{
		ID:          id,
		EncounterID: encounterID,
		Type:        entities.EventTypeAttackResolved,
		Timestamp:   time.Now(),
	}
}

// Append Tests

func (s *InMemoryRepositoryTestSuite) TestAppend_Success() {
	// Arrange
	event := s.createEvent("evt-1", "enc-1")
	input := &AppendInput{Event: event}

	// Act
	output, err := s.repo.Append(s.ctx, input)

	// Assert
	s.Require().NoError(err)
	s.Assert().Equal("evt-1", output.EventID)
}

func (s *InMemoryRepositoryTestSuite) TestAppend_NilInput() {
	// Act
	output, err := s.repo.Append(s.ctx, nil)

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "event is required")
}

func (s *InMemoryRepositoryTestSuite) TestAppend_NilEvent() {
	// Arrange
	input := &AppendInput{Event: nil}

	// Act
	output, err := s.repo.Append(s.ctx, input)

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "event is required")
}

func (s *InMemoryRepositoryTestSuite) TestAppend_EmptyEncounterID() {
	// Arrange
	event := &entities.EncounterEvent{
		ID:          "evt-1",
		EncounterID: "",
	}
	input := &AppendInput{Event: event}

	// Act
	output, err := s.repo.Append(s.ctx, input)

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "encounter ID is required")
}

func (s *InMemoryRepositoryTestSuite) TestAppend_EmptyEventID() {
	// Arrange
	event := &entities.EncounterEvent{
		ID:          "",
		EncounterID: "enc-1",
	}
	input := &AppendInput{Event: event}

	// Act
	output, err := s.repo.Append(s.ctx, input)

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "event ID is required")
}

func (s *InMemoryRepositoryTestSuite) TestAppend_MultipleEventsPreservesOrder() {
	// Arrange & Act - Append 3 events
	for i := 1; i <= 3; i++ {
		event := s.createEvent(fmt.Sprintf("evt-%d", i), "enc-1")
		_, err := s.repo.Append(s.ctx, &AppendInput{Event: event})
		s.Require().NoError(err)
	}

	// Get all events
	output, err := s.repo.GetByEncounter(s.ctx, &GetByEncounterInput{EncounterID: "enc-1"})

	// Assert - Order should be preserved
	s.Require().NoError(err)
	s.Require().Len(output.Events, 3)
	s.Assert().Equal("evt-1", output.Events[0].ID)
	s.Assert().Equal("evt-2", output.Events[1].ID)
	s.Assert().Equal("evt-3", output.Events[2].ID)
}

// GetByEncounter Tests

func (s *InMemoryRepositoryTestSuite) TestGetByEncounter_NilInput() {
	// Act
	output, err := s.repo.GetByEncounter(s.ctx, nil)

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "input is required")
}

func (s *InMemoryRepositoryTestSuite) TestGetByEncounter_EmptyEncounterID() {
	// Arrange
	input := &GetByEncounterInput{EncounterID: ""}

	// Act
	output, err := s.repo.GetByEncounter(s.ctx, input)

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "encounter ID is required")
}

func (s *InMemoryRepositoryTestSuite) TestGetByEncounter_NoEvents() {
	// Act
	output, err := s.repo.GetByEncounter(s.ctx, &GetByEncounterInput{EncounterID: "enc-empty"})

	// Assert
	s.Require().NoError(err)
	s.Assert().NotNil(output.Events)
	s.Assert().Len(output.Events, 0)
	s.Assert().False(output.HasMore)
	s.Assert().Empty(output.LastEventID)
}

func (s *InMemoryRepositoryTestSuite) TestGetByEncounter_ReturnsAllEvents() {
	// Arrange - Add 5 events
	for i := 1; i <= 5; i++ {
		event := s.createEvent(fmt.Sprintf("evt-%d", i), "enc-1")
		_, err := s.repo.Append(s.ctx, &AppendInput{Event: event})
		s.Require().NoError(err)
	}

	// Act
	output, err := s.repo.GetByEncounter(s.ctx, &GetByEncounterInput{EncounterID: "enc-1"})

	// Assert
	s.Require().NoError(err)
	s.Assert().Len(output.Events, 5)
	s.Assert().False(output.HasMore)
	s.Assert().Equal("evt-5", output.LastEventID)
}

func (s *InMemoryRepositoryTestSuite) TestGetByEncounter_WithLimit() {
	// Arrange - Add 5 events
	for i := 1; i <= 5; i++ {
		event := s.createEvent(fmt.Sprintf("evt-%d", i), "enc-1")
		_, err := s.repo.Append(s.ctx, &AppendInput{Event: event})
		s.Require().NoError(err)
	}

	// Act - Limit to 3
	output, err := s.repo.GetByEncounter(s.ctx, &GetByEncounterInput{
		EncounterID: "enc-1",
		Limit:       3,
	})

	// Assert
	s.Require().NoError(err)
	s.Assert().Len(output.Events, 3)
	s.Assert().True(output.HasMore)
	s.Assert().Equal("evt-3", output.LastEventID)
}

func (s *InMemoryRepositoryTestSuite) TestGetByEncounter_LimitGreaterThanEvents() {
	// Arrange - Add 3 events
	for i := 1; i <= 3; i++ {
		event := s.createEvent(fmt.Sprintf("evt-%d", i), "enc-1")
		_, err := s.repo.Append(s.ctx, &AppendInput{Event: event})
		s.Require().NoError(err)
	}

	// Act - Limit to 10 (more than available)
	output, err := s.repo.GetByEncounter(s.ctx, &GetByEncounterInput{
		EncounterID: "enc-1",
		Limit:       10,
	})

	// Assert
	s.Require().NoError(err)
	s.Assert().Len(output.Events, 3)
	s.Assert().False(output.HasMore)
}

func (s *InMemoryRepositoryTestSuite) TestGetByEncounter_WithUpToEventID() {
	// Arrange - Add 5 events
	for i := 1; i <= 5; i++ {
		event := s.createEvent(fmt.Sprintf("evt-%d", i), "enc-1")
		_, err := s.repo.Append(s.ctx, &AppendInput{Event: event})
		s.Require().NoError(err)
	}

	// Act - Get up to evt-3 (for late join scenario)
	output, err := s.repo.GetByEncounter(s.ctx, &GetByEncounterInput{
		EncounterID: "enc-1",
		UpToEventID: "evt-3",
	})

	// Assert - Should return events 1, 2, 3 (inclusive)
	s.Require().NoError(err)
	s.Assert().Len(output.Events, 3)
	s.Assert().Equal("evt-1", output.Events[0].ID)
	s.Assert().Equal("evt-2", output.Events[1].ID)
	s.Assert().Equal("evt-3", output.Events[2].ID)
	s.Assert().False(output.HasMore)
	s.Assert().Equal("evt-3", output.LastEventID)
}

func (s *InMemoryRepositoryTestSuite) TestGetByEncounter_UpToEventIDNotFound() {
	// Arrange - Add 3 events
	for i := 1; i <= 3; i++ {
		event := s.createEvent(fmt.Sprintf("evt-%d", i), "enc-1")
		_, err := s.repo.Append(s.ctx, &AppendInput{Event: event})
		s.Require().NoError(err)
	}

	// Act - Request up to a non-existent event
	output, err := s.repo.GetByEncounter(s.ctx, &GetByEncounterInput{
		EncounterID: "enc-1",
		UpToEventID: "evt-nonexistent",
	})

	// Assert - Should return all events (target not found, so no cutoff)
	s.Require().NoError(err)
	s.Assert().Len(output.Events, 3)
}

func (s *InMemoryRepositoryTestSuite) TestGetByEncounter_UpToEventIDWithLimit() {
	// Arrange - Add 10 events
	for i := 1; i <= 10; i++ {
		event := s.createEvent(fmt.Sprintf("evt-%02d", i), "enc-1")
		_, err := s.repo.Append(s.ctx, &AppendInput{Event: event})
		s.Require().NoError(err)
	}

	// Act - Get up to evt-08 with limit 3
	output, err := s.repo.GetByEncounter(s.ctx, &GetByEncounterInput{
		EncounterID: "enc-1",
		UpToEventID: "evt-08",
		Limit:       3,
	})

	// Assert - Limit should take precedence
	s.Require().NoError(err)
	s.Assert().Len(output.Events, 3)
	s.Assert().True(output.HasMore)
	s.Assert().Equal("evt-03", output.LastEventID)
}

func (s *InMemoryRepositoryTestSuite) TestGetByEncounter_IsolatesEncounters() {
	// Arrange - Add events to different encounters
	_, err := s.repo.Append(s.ctx, &AppendInput{Event: s.createEvent("evt-1", "enc-A")})
	s.Require().NoError(err)
	_, err = s.repo.Append(s.ctx, &AppendInput{Event: s.createEvent("evt-2", "enc-B")})
	s.Require().NoError(err)
	_, err = s.repo.Append(s.ctx, &AppendInput{Event: s.createEvent("evt-3", "enc-A")})
	s.Require().NoError(err)

	// Act
	outputA, err := s.repo.GetByEncounter(s.ctx, &GetByEncounterInput{EncounterID: "enc-A"})
	s.Require().NoError(err)
	outputB, err := s.repo.GetByEncounter(s.ctx, &GetByEncounterInput{EncounterID: "enc-B"})
	s.Require().NoError(err)

	// Assert
	s.Assert().Len(outputA.Events, 2)
	s.Assert().Equal("evt-1", outputA.Events[0].ID)
	s.Assert().Equal("evt-3", outputA.Events[1].ID)

	s.Assert().Len(outputB.Events, 1)
	s.Assert().Equal("evt-2", outputB.Events[0].ID)
}

// Concurrency Tests

func (s *InMemoryRepositoryTestSuite) TestConcurrentAccess() {
	// Test concurrent appends and reads
	var wg sync.WaitGroup
	numGoroutines := 10
	eventsPerGoroutine := 10

	// Concurrent appends
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for i := 0; i < eventsPerGoroutine; i++ {
				event := &entities.EncounterEvent{
					ID:          fmt.Sprintf("evt-%c-%d", 'A'+goroutineID, i),
					EncounterID: "enc-concurrent",
					Type:        entities.EventTypeMovementCompleted,
					Timestamp:   time.Now(),
				}
				_, err := s.repo.Append(s.ctx, &AppendInput{Event: event})
				s.Assert().NoError(err)
			}
		}(g)
	}

	// Concurrent reads while appending
	for g := 0; g < 5; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				_, err := s.repo.GetByEncounter(s.ctx, &GetByEncounterInput{EncounterID: "enc-concurrent"})
				s.Assert().NoError(err)
			}
		}()
	}

	wg.Wait()

	// Verify all events were appended
	output, err := s.repo.GetByEncounter(s.ctx, &GetByEncounterInput{EncounterID: "enc-concurrent"})
	s.Require().NoError(err)
	s.Assert().Equal(numGoroutines*eventsPerGoroutine, len(output.Events))
}

// Integration Tests

func (s *InMemoryRepositoryTestSuite) TestWorkflow_LateJoinScenario() {
	// Simulate: Combat starts, events happen, player joins late

	// 1. Combat starts, several events happen
	events := []struct {
		id      string
		evtType entities.EventType
	}{
		{"evt-001", entities.EventTypeCombatStarted},
		{"evt-002", entities.EventTypeMovementCompleted},
		{"evt-003", entities.EventTypeAttackResolved},
		{"evt-004", entities.EventTypeTurnEnded},
		{"evt-005", entities.EventTypeMovementCompleted},
		{"evt-006", entities.EventTypeAttackResolved},
	}

	for _, e := range events {
		event := &entities.EncounterEvent{
			ID:          e.id,
			EncounterID: "enc-latejoin",
			Type:        e.evtType,
			Timestamp:   time.Now(),
		}
		_, err := s.repo.Append(s.ctx, &AppendInput{Event: event})
		s.Require().NoError(err)
	}

	// 2. Player joins late - GetEncounterState returns lastEventID = "evt-004"
	// They need history up to that point
	historyOutput, err := s.repo.GetByEncounter(s.ctx, &GetByEncounterInput{
		EncounterID: "enc-latejoin",
		UpToEventID: "evt-004",
	})

	// Assert - Should get events 1-4
	s.Require().NoError(err)
	s.Assert().Len(historyOutput.Events, 4)
	s.Assert().Equal("evt-001", historyOutput.Events[0].ID)
	s.Assert().Equal("evt-004", historyOutput.Events[3].ID)

	// 3. Player can also get all events (for full log)
	allOutput, err := s.repo.GetByEncounter(s.ctx, &GetByEncounterInput{
		EncounterID: "enc-latejoin",
	})
	s.Require().NoError(err)
	s.Assert().Len(allOutput.Events, 6)
}
