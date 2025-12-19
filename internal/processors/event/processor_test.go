package event

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/entities"
	"github.com/KirkDiggler/rpg-api/internal/publishers/encounter"
	publishermock "github.com/KirkDiggler/rpg-api/internal/publishers/encounter/mock"
	"github.com/KirkDiggler/rpg-api/internal/repositories/encounterlog"
	encounterlogmock "github.com/KirkDiggler/rpg-api/internal/repositories/encounterlog/mock"
)

type ProcessorTestSuite struct {
	suite.Suite
	ctrl          *gomock.Controller
	mockRepo      *encounterlogmock.MockRepository
	mockPublisher *publishermock.MockPublisher
	processor     Processor
	ctx           context.Context
}

func (s *ProcessorTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockRepo = encounterlogmock.NewMockRepository(s.ctrl)
	s.mockPublisher = publishermock.NewMockPublisher(s.ctrl)
	s.ctx = context.Background()

	var err error
	s.processor, err = New(&Config{
		EncounterLogRepo: s.mockRepo,
		Publisher:        s.mockPublisher,
	})
	s.Require().NoError(err)
}

func (s *ProcessorTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func TestProcessorSuite(t *testing.T) {
	suite.Run(t, new(ProcessorTestSuite))
}

// Helper to create test events
func (s *ProcessorTestSuite) createEvent(id, encounterID string) *entities.EncounterEvent {
	return &entities.EncounterEvent{
		ID:          id,
		EncounterID: encounterID,
		Type:        entities.EventTypeAttackResolved,
		Timestamp:   time.Now(),
	}
}

// New Tests

func (s *ProcessorTestSuite) TestNew_Success() {
	p, err := New(&Config{
		EncounterLogRepo: s.mockRepo,
		Publisher:        s.mockPublisher,
	})

	s.Require().NoError(err)
	s.Assert().NotNil(p)
}

func (s *ProcessorTestSuite) TestNew_NilConfig() {
	p, err := New(nil)

	s.Require().Error(err)
	s.Assert().Nil(p)
	s.Assert().Contains(err.Error(), "config is required")
}

func (s *ProcessorTestSuite) TestNew_NilRepository() {
	p, err := New(&Config{
		EncounterLogRepo: nil,
		Publisher:        s.mockPublisher,
	})

	s.Require().Error(err)
	s.Assert().Nil(p)
	s.Assert().Contains(err.Error(), "encounter log repository is required")
}

func (s *ProcessorTestSuite) TestNew_NilPublisher() {
	p, err := New(&Config{
		EncounterLogRepo: s.mockRepo,
		Publisher:        nil,
	})

	s.Require().Error(err)
	s.Assert().Nil(p)
	s.Assert().Contains(err.Error(), "publisher is required")
}

// Process Tests

func (s *ProcessorTestSuite) TestProcess_Success() {
	// Arrange
	event := s.createEvent("evt-1", "enc-1")
	input := &ProcessInput{
		EncounterID: "enc-1",
		Event:       event,
	}

	s.mockRepo.EXPECT().
		Append(s.ctx, &encounterlog.AppendInput{Event: event}).
		Return(&encounterlog.AppendOutput{EventID: "evt-1"}, nil)

	s.mockPublisher.EXPECT().
		Publish(s.ctx, &encounter.PublishInput{
			EncounterID: "enc-1",
			Event:       event,
		}).
		Return(&encounter.PublishOutput{}, nil)

	// Act
	output, err := s.processor.Process(s.ctx, input)

	// Assert
	s.Require().NoError(err)
	s.Assert().Equal("evt-1", output.EventID)
}

func (s *ProcessorTestSuite) TestProcess_NilInput() {
	// Act
	output, err := s.processor.Process(s.ctx, nil)

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "event is required")
}

func (s *ProcessorTestSuite) TestProcess_NilEvent() {
	// Arrange
	input := &ProcessInput{
		EncounterID: "enc-1",
		Event:       nil,
	}

	// Act
	output, err := s.processor.Process(s.ctx, input)

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "event is required")
}

func (s *ProcessorTestSuite) TestProcess_PersistFailure() {
	// Arrange
	event := s.createEvent("evt-1", "enc-1")
	input := &ProcessInput{
		EncounterID: "enc-1",
		Event:       event,
	}

	s.mockRepo.EXPECT().
		Append(s.ctx, &encounterlog.AppendInput{Event: event}).
		Return(nil, errors.New("database error"))

	// Publish should NOT be called when persist fails

	// Act
	output, err := s.processor.Process(s.ctx, input)

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "failed to persist event")
}

func (s *ProcessorTestSuite) TestProcess_PublishFailure_StillSucceeds() {
	// Arrange
	event := s.createEvent("evt-1", "enc-1")
	input := &ProcessInput{
		EncounterID: "enc-1",
		Event:       event,
	}

	s.mockRepo.EXPECT().
		Append(s.ctx, &encounterlog.AppendInput{Event: event}).
		Return(&encounterlog.AppendOutput{EventID: "evt-1"}, nil)

	s.mockPublisher.EXPECT().
		Publish(s.ctx, &encounter.PublishInput{
			EncounterID: "enc-1",
			Event:       event,
		}).
		Return(nil, errors.New("publish error"))

	// Act
	output, err := s.processor.Process(s.ctx, input)

	// Assert - Should still succeed because persistence is source of truth
	s.Require().NoError(err)
	s.Assert().Equal("evt-1", output.EventID)
}

func (s *ProcessorTestSuite) TestProcess_PersistsThenPublishes() {
	// This test verifies the order: persist first, then publish
	// Arrange
	event := s.createEvent("evt-1", "enc-1")
	input := &ProcessInput{
		EncounterID: "enc-1",
		Event:       event,
	}

	// gomock.InOrder ensures Append is called before Publish
	gomock.InOrder(
		s.mockRepo.EXPECT().
			Append(s.ctx, &encounterlog.AppendInput{Event: event}).
			Return(&encounterlog.AppendOutput{EventID: "evt-1"}, nil),
		s.mockPublisher.EXPECT().
			Publish(s.ctx, &encounter.PublishInput{
				EncounterID: "enc-1",
				Event:       event,
			}).
			Return(&encounter.PublishOutput{}, nil),
	)

	// Act
	output, err := s.processor.Process(s.ctx, input)

	// Assert
	s.Require().NoError(err)
	s.Assert().Equal("evt-1", output.EventID)
}
