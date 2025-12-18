package encounter

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/initiative"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	"github.com/KirkDiggler/rpg-api/internal/entities"
	encounterpub "github.com/KirkDiggler/rpg-api/internal/publishers/encounter"
	publishermock "github.com/KirkDiggler/rpg-api/internal/publishers/encounter/mock"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	encounterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/encounters"
	encountermock "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/mock"
)

// EventPublishingTestSuite tests that events are correctly published
// when orchestrator methods are called
type EventPublishingTestSuite struct {
	suite.Suite
	ctrl          *gomock.Controller
	mockCharRepo  *charactermock.MockRepository
	mockEncRepo   *encountermock.MockRepository
	mockPublisher *publishermock.MockPublisher
	orchestrator  *Orchestrator
}

func (s *EventPublishingTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockCharRepo = charactermock.NewMockRepository(s.ctrl)
	s.mockEncRepo = encountermock.NewMockRepository(s.ctrl)
	s.mockPublisher = publishermock.NewMockPublisher(s.ctrl)

	var err error
	s.orchestrator, err = New(&Config{
		CharacterRepo: s.mockCharRepo,
		EncounterRepo: s.mockEncRepo,
		Publisher:     s.mockPublisher,
		EventIDGen:    &mockEventIDGen{prefix: "evt"},
	})
	s.Require().NoError(err)
}

func (s *EventPublishingTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func TestEventPublishingSuite(t *testing.T) {
	suite.Run(t, new(EventPublishingTestSuite))
}

// mockEventIDGen generates predictable event IDs for testing
type mockEventIDGen struct {
	prefix string
	count  int
}

func (m *mockEventIDGen) Generate() string {
	m.count++
	return m.prefix + "-" + string(rune('0'+m.count))
}

// ============================================================================
// Lobby Event Publishing Tests
// ============================================================================

func (s *EventPublishingTestSuite) TestJoinEncounter_PublishesPlayerJoinedEvent() {
	// Arrange
	joinCode := "ABC123"
	encounterID := "enc-123"
	playerID := "player-2"
	characterID := "char-2"

	// Mock GetByJoinCode
	s.mockEncRepo.EXPECT().
		GetByJoinCode(gomock.Any(), &encounterrepo.GetByJoinCodeInput{JoinCode: joinCode}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:       encounterID,
				State:    encounterrepo.StateWaiting,
				HostID:   "player-1",
				Players:  map[string]*encounterrepo.Player{},
				RoomData: &spatial.RoomData{},
			},
		}, nil)

	// Mock Update
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil)

	// Mock character lookup for event publishing (called first)
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: characterID}).
		Return(&characterrepo.GetOutput{
			CharacterData: &character.Data{
				ID:       characterID,
				Name:     "Test Character",
				PlayerID: playerID,
			},
		}, nil)

	// Mock character lookup for building party list (called second)
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: characterID}).
		Return(&characterrepo.GetOutput{
			CharacterData: &character.Data{
				ID:       characterID,
				Name:     "Test Character",
				PlayerID: playerID,
			},
		}, nil)

	// Expect PlayerJoined event to be published
	s.mockPublisher.EXPECT().
		Publish(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input *encounterpub.PublishInput) (*encounterpub.PublishOutput, error) {
			s.Assert().Equal(encounterID, input.EncounterID)
			s.Assert().Equal(entities.EventTypePlayerJoined, input.Event.Type)

			// Verify event data using typed field
			s.Require().NotNil(input.Event.PlayerJoined, "PlayerJoined should be set")
			s.Assert().Equal(playerID, input.Event.PlayerJoined.PlayerID)
			s.Assert().Equal(characterID, input.Event.PlayerJoined.CharacterID)
			// Verify character data is included
			s.Require().NotNil(input.Event.PlayerJoined.CharacterData, "CharacterData should be set")
			s.Assert().Equal("Test Character", input.Event.PlayerJoined.CharacterData.Name)

			return &encounterpub.PublishOutput{}, nil
		})

	// Act
	output, err := s.orchestrator.JoinEncounter(context.Background(), &JoinEncounterInput{
		JoinCode:     joinCode,
		PlayerID:     playerID,
		CharacterIDs: []string{characterID},
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
}

func (s *EventPublishingTestSuite) TestSetReady_PublishesPlayerReadyEvent() {
	// Arrange
	encounterID := "enc-123"
	playerID := "player-1"
	characterID := "char-1"

	// Mock Get
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: encounterID}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:     encounterID,
				State:  encounterrepo.StateWaiting,
				HostID: playerID,
				Players: map[string]*encounterrepo.Player{
					playerID: {
						PlayerID:    playerID,
						CharacterID: characterID,
						IsReady:     false,
					},
				},
			},
		}, nil)

	// Mock Update
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil)

	// Expect PlayerReady event to be published
	s.mockPublisher.EXPECT().
		Publish(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input *encounterpub.PublishInput) (*encounterpub.PublishOutput, error) {
			s.Assert().Equal(encounterID, input.EncounterID)
			s.Assert().Equal(entities.EventTypePlayerReady, input.Event.Type)

			// Verify event data using typed field
			s.Require().NotNil(input.Event.PlayerReady, "PlayerReady should be set")
			s.Assert().Equal(playerID, input.Event.PlayerReady.PlayerID)
			s.Assert().Equal(characterID, input.Event.PlayerReady.CharacterID)
			s.Assert().True(input.Event.PlayerReady.Ready)

			return &encounterpub.PublishOutput{}, nil
		})

	// Act
	output, err := s.orchestrator.SetReady(context.Background(), &SetReadyInput{
		EncounterID: encounterID,
		PlayerID:    playerID,
		IsReady:     true,
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().True(output.Success)
}

func (s *EventPublishingTestSuite) TestStartCombat_PublishesCombatStartedEvent() {
	// Arrange
	encounterID := "enc-123"
	playerID := "player-1"
	characterID := "char-1"

	roomData := &spatial.RoomData{
		ID:           encounterID + "-room",
		Width:        20,
		Height:       20,
		GridType:     spatial.GridTypeHex,
		CubeEntities: make(map[string]spatial.EntityCubePlacement),
	}

	// Mock Get
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: encounterID}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:       encounterID,
				State:    encounterrepo.StateWaiting,
				HostID:   playerID,
				RoomData: roomData,
				Players: map[string]*encounterrepo.Player{
					playerID: {
						PlayerID:    playerID,
						CharacterID: characterID,
						IsReady:     true,
						IsConnected: true,
					},
				},
			},
		}, nil)

	// Mock character lookup for initiative, party building, and monster attacks
	// May be called multiple times depending on combat flow
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: characterID}).
		Return(&characterrepo.GetOutput{
			CharacterData: &character.Data{
				ID:        characterID,
				PlayerID:  playerID,
				HitPoints: 10, // Set HP to non-zero to avoid triggering TPK check
				AbilityScores: shared.AbilityScores{
					abilities.DEX: 14, // +2 modifier
				},
			},
		}, nil).AnyTimes()

	// Mock Update (may be called multiple times if monster goes first)
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil).
		AnyTimes()

	// Expect CombatStarted event to be published
	s.mockPublisher.EXPECT().
		Publish(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input *encounterpub.PublishInput) (*encounterpub.PublishOutput, error) {
			s.Assert().Equal(encounterID, input.EncounterID)
			s.Assert().Equal(entities.EventTypeCombatStarted, input.Event.Type)

			// Verify event data using typed field
			s.Require().NotNil(input.Event.CombatStarted, "CombatStarted should be set")
			s.Assert().NotNil(input.Event.CombatStarted.CombatState)

			return &encounterpub.PublishOutput{}, nil
		})

	// Act
	output, err := s.orchestrator.StartCombat(context.Background(), &StartCombatInput{
		EncounterID: encounterID,
		PlayerID:    playerID,
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Require().NotNil(output.CombatState)
}

func (s *EventPublishingTestSuite) TestLeaveEncounter_PublishesPlayerLeftEvent() {
	// Arrange
	encounterID := "enc-123"
	playerID := "player-1"
	characterID := "char-1"

	// Mock Get
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: encounterID}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:     encounterID,
				State:  encounterrepo.StateWaiting,
				HostID: playerID,
				Players: map[string]*encounterrepo.Player{
					playerID: {
						PlayerID:    playerID,
						CharacterID: characterID,
					},
					"player-2": {
						PlayerID:    "player-2",
						CharacterID: "char-2",
					},
				},
			},
		}, nil)

	// Mock Update (host transfer + player removal)
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil)

	// Expect PlayerLeft event to be published
	s.mockPublisher.EXPECT().
		Publish(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input *encounterpub.PublishInput) (*encounterpub.PublishOutput, error) {
			s.Assert().Equal(encounterID, input.EncounterID)
			s.Assert().Equal(entities.EventTypePlayerLeft, input.Event.Type)

			// Verify event data using typed field
			s.Require().NotNil(input.Event.PlayerLeft, "PlayerLeft should be set")
			s.Assert().Equal(playerID, input.Event.PlayerLeft.PlayerID)
			s.Assert().Equal(characterID, input.Event.PlayerLeft.CharacterID)
			s.Assert().Equal("voluntary", input.Event.PlayerLeft.Reason)

			return &encounterpub.PublishOutput{}, nil
		})

	// Act
	output, err := s.orchestrator.LeaveEncounter(context.Background(), &LeaveEncounterInput{
		EncounterID: encounterID,
		PlayerID:    playerID,
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().True(output.Success)
	s.Assert().False(output.EncounterDeleted) // player-2 still in encounter
}

// ============================================================================
// Combat Event Publishing Tests
// ============================================================================

func (s *EventPublishingTestSuite) TestMoveCharacter_PublishesMovementCompletedEvent() {
	// Arrange
	encounterID := "enc-123"
	entityID := "char-1"
	// Use valid cube coordinates (x + y + z = 0)
	targetX := 5.0
	targetY := -10.0 // y = -x - z
	targetZ := 5.0

	roomData := &spatial.RoomData{
		ID:           encounterID + "-room",
		Width:        20,
		Height:       20,
		GridType:     spatial.GridTypeHex,
		CubeEntities: map[string]spatial.EntityCubePlacement{
			entityID: {
				EntityID:       entityID,
				EntityType:     "character",
				CubePosition:   spatial.CubeCoordinate{X: 3, Y: -6, Z: 3}, // 3 + -6 + 3 = 0
				Size:           1,
				BlocksMovement: true,
			},
		},
	}

	// Mock Get
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: encounterID}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:                encounterID,
				RoomData:          roomData,
				MovementRemaining: 30,
			},
		}, nil)

	// Mock Update
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil)

	// Expect MovementCompleted event to be published
	s.mockPublisher.EXPECT().
		Publish(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input *encounterpub.PublishInput) (*encounterpub.PublishOutput, error) {
			s.Assert().Equal(encounterID, input.EncounterID)
			s.Assert().Equal(entities.EventTypeMovementCompleted, input.Event.Type)

			// Verify event data using typed field
			s.Require().NotNil(input.Event.MovementCompleted, "MovementCompleted should be set")
			s.Assert().Equal(entityID, input.Event.MovementCompleted.EntityID)
			s.Assert().Equal("character", input.Event.MovementCompleted.EntityType)
			s.Assert().Equal("completed", input.Event.MovementCompleted.StopReason)

			return &encounterpub.PublishOutput{}, nil
		})

	// Act
	output, err := s.orchestrator.MoveCharacter(context.Background(), &MoveCharacterInput{
		EncounterID:    encounterID,
		EntityID:       entityID,
		TargetPosition: &Position{X: targetX, Y: targetY, Z: targetZ},
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().True(output.Success)
}

func (s *EventPublishingTestSuite) TestEndTurn_PublishesTurnEndedEvent() {
	// Arrange
	encounterID := "enc-123"

	initiativeData := &initiative.TrackerData{
		Order: []initiative.EntityData{
			{ID: "char-1", Type: "character"},
			{ID: "char-2", Type: "character"},
		},
		Current: 0,
		Round:   1,
	}

	initiativeRolls := []initiative.Roll{
		{Entity: initiative.NewParticipant("char-1", "character"), Roll: 18, Modifier: 2, Total: 20},
		{Entity: initiative.NewParticipant("char-2", "character"), Roll: 10, Modifier: 1, Total: 11},
	}

	// Mock Get
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: encounterID}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:              encounterID,
				InitiativeData:  initiativeData,
				InitiativeRolls: initiativeRolls,
			},
		}, nil)

	// Mock Update
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil)

	// Expect TurnEnded event to be published
	s.mockPublisher.EXPECT().
		Publish(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input *encounterpub.PublishInput) (*encounterpub.PublishOutput, error) {
			s.Assert().Equal(encounterID, input.EncounterID)
			s.Assert().Equal(entities.EventTypeTurnEnded, input.Event.Type)

			// Verify event data using typed field
			s.Require().NotNil(input.Event.TurnEnded, "TurnEnded should be set")
			s.Assert().Equal("char-1", input.Event.TurnEnded.PreviousEntityID)
			s.Assert().Equal("char-2", input.Event.TurnEnded.NextEntityID)
			s.Assert().Equal(1, input.Event.TurnEnded.Round)
			s.Assert().False(input.Event.TurnEnded.NewRound)

			return &encounterpub.PublishOutput{}, nil
		})

	// Act
	output, err := s.orchestrator.EndTurn(context.Background(), &EndTurnInput{
		EncounterID: encounterID,
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
}

func (s *EventPublishingTestSuite) TestActivateFeature_NoEventWhenFeatureNotFound() {
	// This tests the expected behavior: no event is published when feature isn't found
	// (The feature activation success case requires complex setup with serialized features)

	// Arrange
	encounterID := "enc-123"
	characterID := "char-1"
	featureID := "nonexistent-feature"

	// Create a character without the requested feature
	charData := &character.Data{
		ID:               characterID,
		Name:             "Test Character",
		PlayerID:         "player-1",
		Level:            1,
		RaceID:           "human",
		ClassID:          "barbarian",
		ProficiencyBonus: 2,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 16,
			abilities.DEX: 14,
			abilities.CON: 15,
			abilities.INT: 10,
			abilities.WIS: 12,
			abilities.CHA: 8,
		},
		// No features configured
	}

	// Mock encounter Get
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: encounterID}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID: encounterID,
			},
		}, nil)

	// Mock character Get
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: characterID}).
		Return(&characterrepo.GetOutput{
			CharacterData: charData,
		}, nil)

	// NO character Update expected - we return early when feature not found
	// NO Publish expected - no event when feature not found

	// Act
	output, err := s.orchestrator.ActivateFeature(context.Background(), &ActivateFeatureInput{
		EncounterID: encounterID,
		CharacterID: characterID,
		FeatureID:   featureID,
	})

	// Assert - should succeed but with Success=false
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().False(output.Success)
	s.Assert().Contains(output.Message, "not found")
}

// ============================================================================
// Edge Cases
// ============================================================================

func (s *EventPublishingTestSuite) TestPublishing_ContinuesOnPublishError() {
	// Arrange - publisher will return an error
	encounterID := "enc-123"
	playerID := "player-1"
	characterID := "char-1"

	// Mock Get
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: encounterID}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:     encounterID,
				State:  encounterrepo.StateWaiting,
				HostID: playerID,
				Players: map[string]*encounterrepo.Player{
					playerID: {
						PlayerID:    playerID,
						CharacterID: characterID,
						IsReady:     false,
					},
				},
			},
		}, nil)

	// Mock Update
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil)

	// Publisher returns error - but operation should still succeed
	s.mockPublisher.EXPECT().
		Publish(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("publish failed")) // Using any error for testing

	// Act - should NOT fail even though publish failed
	output, err := s.orchestrator.SetReady(context.Background(), &SetReadyInput{
		EncounterID: encounterID,
		PlayerID:    playerID,
		IsReady:     true,
	})

	// Assert - operation should succeed despite publish error
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().True(output.Success)
}

func (s *EventPublishingTestSuite) TestNoPublisher_DoesNotPanic() {
	// Arrange - orchestrator without publisher
	orchWithoutPublisher, err := New(&Config{
		CharacterRepo: s.mockCharRepo,
		EncounterRepo: s.mockEncRepo,
		// Publisher: nil - intentionally omitted
	})
	s.Require().NoError(err)

	encounterID := "enc-123"
	playerID := "player-1"
	characterID := "char-1"

	// Mock Get
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: encounterID}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:     encounterID,
				State:  encounterrepo.StateWaiting,
				HostID: playerID,
				Players: map[string]*encounterrepo.Player{
					playerID: {
						PlayerID:    playerID,
						CharacterID: characterID,
						IsReady:     false,
					},
				},
			},
		}, nil)

	// Mock Update
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil)

	// NO publisher expectations - it's nil

	// Act - should NOT panic
	output, err := orchWithoutPublisher.SetReady(context.Background(), &SetReadyInput{
		EncounterID: encounterID,
		PlayerID:    playerID,
		IsReady:     true,
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().True(output.Success)
}

func (s *EventPublishingTestSuite) TestLeaveEncounter_LastPlayer_PublishesBeforeDelete() {
	// Arrange - only one player in encounter
	encounterID := "enc-123"
	playerID := "player-1"
	characterID := "char-1"

	// Mock Get
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: encounterID}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:     encounterID,
				State:  encounterrepo.StateWaiting,
				HostID: playerID,
				Players: map[string]*encounterrepo.Player{
					playerID: {
						PlayerID:    playerID,
						CharacterID: characterID,
					},
				},
			},
		}, nil)

	// Expect event BEFORE delete
	publishCalled := false
	s.mockPublisher.EXPECT().
		Publish(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input *encounterpub.PublishInput) (*encounterpub.PublishOutput, error) {
			publishCalled = true
			s.Assert().Equal(entities.EventTypePlayerLeft, input.Event.Type)
			return &encounterpub.PublishOutput{}, nil
		})

	// Mock Delete (called after publish)
	s.mockEncRepo.EXPECT().
		Delete(gomock.Any(), &encounterrepo.DeleteInput{EncounterID: encounterID}).
		DoAndReturn(func(ctx context.Context, input *encounterrepo.DeleteInput) (*encounterrepo.DeleteOutput, error) {
			// Verify publish was called before delete
			s.Assert().True(publishCalled, "publish should be called before delete")
			return &encounterrepo.DeleteOutput{Success: true}, nil
		})

	// Act
	output, err := s.orchestrator.LeaveEncounter(context.Background(), &LeaveEncounterInput{
		EncounterID: encounterID,
		PlayerID:    playerID,
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().True(output.Success)
	s.Assert().True(output.EncounterDeleted)
}

func (s *EventPublishingTestSuite) TestEndTurn_NewRound_SetsNewRoundFlag() {
	// Arrange - last turn of round
	encounterID := "enc-123"

	initiativeData := &initiative.TrackerData{
		Order: []initiative.EntityData{
			{ID: "char-1", Type: "character"},
			{ID: "char-2", Type: "character"},
		},
		Current: 1, // Last entity in order
		Round:   1,
	}

	initiativeRolls := []initiative.Roll{
		{Entity: initiative.NewParticipant("char-1", "character"), Roll: 18, Modifier: 2, Total: 20},
		{Entity: initiative.NewParticipant("char-2", "character"), Roll: 10, Modifier: 1, Total: 11},
	}

	// Mock Get
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: encounterID}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:              encounterID,
				InitiativeData:  initiativeData,
				InitiativeRolls: initiativeRolls,
			},
		}, nil)

	// Mock Update
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil)

	// Expect TurnEnded with NewRound=true
	s.mockPublisher.EXPECT().
		Publish(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input *encounterpub.PublishInput) (*encounterpub.PublishOutput, error) {
			s.Assert().Equal(entities.EventTypeTurnEnded, input.Event.Type)

			// Verify event data using typed field
			s.Require().NotNil(input.Event.TurnEnded, "TurnEnded should be set")
			s.Assert().Equal("char-2", input.Event.TurnEnded.PreviousEntityID)
			s.Assert().Equal("char-1", input.Event.TurnEnded.NextEntityID) // Wraps to first
			s.Assert().Equal(2, input.Event.TurnEnded.Round)               // Incremented
			s.Assert().True(input.Event.TurnEnded.NewRound)

			return &encounterpub.PublishOutput{}, nil
		})

	// Act
	output, err := s.orchestrator.EndTurn(context.Background(), &EndTurnInput{
		EncounterID: encounterID,
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().True(output.TurnChange.NewRound)
}

func (s *EventPublishingTestSuite) TestEventTimestamp_IsRecent() {
	// Arrange
	encounterID := "enc-123"
	playerID := "player-1"
	characterID := "char-1"

	// Mock Get
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:     encounterID,
				State:  encounterrepo.StateWaiting,
				HostID: playerID,
				Players: map[string]*encounterrepo.Player{
					playerID: {
						PlayerID:    playerID,
						CharacterID: characterID,
					},
				},
			},
		}, nil)

	// Mock Update
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil)

	beforeTest := time.Now()

	// Capture timestamp
	s.mockPublisher.EXPECT().
		Publish(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input *encounterpub.PublishInput) (*encounterpub.PublishOutput, error) {
			// Verify timestamp is reasonable (within 1 second)
			s.Assert().WithinDuration(beforeTest, input.Event.Timestamp, time.Second)
			return &encounterpub.PublishOutput{}, nil
		})

	// Act
	_, err := s.orchestrator.SetReady(context.Background(), &SetReadyInput{
		EncounterID: encounterID,
		PlayerID:    playerID,
		IsReady:     true,
	})

	// Assert
	s.Require().NoError(err)
}

// ============================================================================
// Disconnect/Reconnect Event Publishing Tests
// ============================================================================

func (s *EventPublishingTestSuite) TestPlayerDisconnected_PublishesEvent() {
	// Arrange
	encounterID := "enc-123"
	playerID := "player-1"
	characterID := "char-1"

	// Mock Get
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: encounterID}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:     encounterID,
				State:  encounterrepo.StateWaiting,
				HostID: playerID,
				Players: map[string]*encounterrepo.Player{
					playerID: {
						PlayerID:    playerID,
						CharacterID: characterID,
						IsConnected: true,
					},
				},
			},
		}, nil)

	// Mock Update
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil)

	// Expect PlayerDisconnected event
	s.mockPublisher.EXPECT().
		Publish(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input *encounterpub.PublishInput) (*encounterpub.PublishOutput, error) {
			s.Assert().Equal(encounterID, input.EncounterID)
			s.Assert().Equal(entities.EventTypePlayerDisconnected, input.Event.Type)

			// Verify event data using typed field
			s.Require().NotNil(input.Event.PlayerDisconnected, "PlayerDisconnected should be set")
			s.Assert().Equal(playerID, input.Event.PlayerDisconnected.PlayerID)
			s.Assert().Equal(characterID, input.Event.PlayerDisconnected.CharacterID)
			s.Assert().Equal("timeout", input.Event.PlayerDisconnected.Reason)

			return &encounterpub.PublishOutput{}, nil
		})

	// Act
	output, err := s.orchestrator.PlayerDisconnected(context.Background(), &PlayerDisconnectedInput{
		EncounterID: encounterID,
		PlayerID:    playerID,
		Reason:      "timeout",
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().True(output.Success)
	s.Assert().False(output.EncounterPaused) // Not paused because not in combat
}

func (s *EventPublishingTestSuite) TestPlayerDisconnected_PausesCombatWhenAllDisconnected() {
	// Arrange - combat is active and this is the only connected player
	encounterID := "enc-123"
	playerID := "player-1"
	characterID := "char-1"

	// Mock Get
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: encounterID}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:     encounterID,
				State:  encounterrepo.StateActive,
				HostID: playerID,
				Players: map[string]*encounterrepo.Player{
					playerID: {
						PlayerID:    playerID,
						CharacterID: characterID,
						IsConnected: true,
					},
				},
			},
		}, nil)

	// Mock Update
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input *encounterrepo.UpdateInput) (*encounterrepo.UpdateOutput, error) {
			// Verify state is being set to paused
			s.Require().NotNil(input.State)
			s.Assert().Equal(encounterrepo.StatePaused, *input.State)
			return &encounterrepo.UpdateOutput{Success: true}, nil
		})

	// Expect PlayerDisconnected event first
	firstCall := s.mockPublisher.EXPECT().
		Publish(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input *encounterpub.PublishInput) (*encounterpub.PublishOutput, error) {
			s.Assert().Equal(entities.EventTypePlayerDisconnected, input.Event.Type)
			return &encounterpub.PublishOutput{}, nil
		})

	// Then CombatPaused event
	s.mockPublisher.EXPECT().
		Publish(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input *encounterpub.PublishInput) (*encounterpub.PublishOutput, error) {
			s.Assert().Equal(entities.EventTypeCombatPaused, input.Event.Type)

			// Verify event data using typed field
			s.Require().NotNil(input.Event.CombatPaused, "CombatPaused should be set")
			s.Assert().Equal(playerID, input.Event.CombatPaused.PausedBy)
			s.Assert().Equal("all players disconnected", input.Event.CombatPaused.Reason)

			return &encounterpub.PublishOutput{}, nil
		}).After(firstCall)

	// Act
	output, err := s.orchestrator.PlayerDisconnected(context.Background(), &PlayerDisconnectedInput{
		EncounterID: encounterID,
		PlayerID:    playerID,
		Reason:      "network_error",
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().True(output.Success)
	s.Assert().True(output.EncounterPaused)
	s.Assert().Equal("paused", output.State)
}

func (s *EventPublishingTestSuite) TestPlayerReconnected_PublishesEvent() {
	// Arrange
	encounterID := "enc-123"
	playerID := "player-1"
	characterID := "char-1"

	// Mock Get
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: encounterID}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:     encounterID,
				State:  encounterrepo.StateWaiting,
				HostID: playerID,
				Players: map[string]*encounterrepo.Player{
					playerID: {
						PlayerID:    playerID,
						CharacterID: characterID,
						IsConnected: false, // Currently disconnected
					},
				},
			},
		}, nil)

	// Mock Update
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil)

	// Expect PlayerReconnected event
	s.mockPublisher.EXPECT().
		Publish(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input *encounterpub.PublishInput) (*encounterpub.PublishOutput, error) {
			s.Assert().Equal(encounterID, input.EncounterID)
			s.Assert().Equal(entities.EventTypePlayerReconnected, input.Event.Type)

			// Verify event data using typed field
			s.Require().NotNil(input.Event.PlayerReconnected, "PlayerReconnected should be set")
			s.Assert().Equal(playerID, input.Event.PlayerReconnected.PlayerID)
			s.Assert().Equal(characterID, input.Event.PlayerReconnected.CharacterID)

			return &encounterpub.PublishOutput{}, nil
		})

	// Act
	output, err := s.orchestrator.PlayerReconnected(context.Background(), &PlayerReconnectedInput{
		EncounterID: encounterID,
		PlayerID:    playerID,
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().True(output.Success)
	s.Assert().False(output.EncounterResumed) // Not resumed because wasn't paused
}

func (s *EventPublishingTestSuite) TestPlayerReconnected_ResumesCombat() {
	// Arrange - encounter is paused, player reconnects
	encounterID := "enc-123"
	playerID := "player-1"
	characterID := "char-1"

	initiativeData := &initiative.TrackerData{
		Order: []initiative.EntityData{
			{ID: characterID, Type: "character"},
		},
		Current: 0,
		Round:   2,
	}

	initiativeRolls := []initiative.Roll{
		{Entity: initiative.NewParticipant(characterID, "character"), Roll: 15, Modifier: 2, Total: 17},
	}

	// Mock Get
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: encounterID}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:                encounterID,
				State:             encounterrepo.StatePaused,
				HostID:            playerID,
				InitiativeData:    initiativeData,
				InitiativeRolls:   initiativeRolls,
				MovementRemaining: 25,
				Players: map[string]*encounterrepo.Player{
					playerID: {
						PlayerID:    playerID,
						CharacterID: characterID,
						IsConnected: false, // Currently disconnected
					},
				},
			},
		}, nil)

	// Mock Update
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input *encounterrepo.UpdateInput) (*encounterrepo.UpdateOutput, error) {
			// Verify state is being set to active
			s.Require().NotNil(input.State)
			s.Assert().Equal(encounterrepo.StateActive, *input.State)
			return &encounterrepo.UpdateOutput{Success: true}, nil
		})

	// Expect PlayerReconnected event first
	firstCall := s.mockPublisher.EXPECT().
		Publish(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input *encounterpub.PublishInput) (*encounterpub.PublishOutput, error) {
			s.Assert().Equal(entities.EventTypePlayerReconnected, input.Event.Type)
			return &encounterpub.PublishOutput{}, nil
		})

	// Then CombatResumed event
	s.mockPublisher.EXPECT().
		Publish(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input *encounterpub.PublishInput) (*encounterpub.PublishOutput, error) {
			s.Assert().Equal(entities.EventTypeCombatResumed, input.Event.Type)

			// Verify event data using typed field
			s.Require().NotNil(input.Event.CombatResumed, "CombatResumed should be set")
			s.Assert().Equal(playerID, input.Event.CombatResumed.ResumedBy)

			return &encounterpub.PublishOutput{}, nil
		}).After(firstCall)

	// Act
	output, err := s.orchestrator.PlayerReconnected(context.Background(), &PlayerReconnectedInput{
		EncounterID: encounterID,
		PlayerID:    playerID,
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().True(output.Success)
	s.Assert().True(output.EncounterResumed)
	s.Assert().Equal("active", output.State)
	s.Require().NotNil(output.CombatState)
	s.Assert().Equal(2, output.CombatState.Round)
	s.Assert().Equal(int32(25), output.CombatState.MovementRemaining)
}

func (s *EventPublishingTestSuite) TestPlayerDisconnected_AlreadyDisconnected() {
	// Arrange
	encounterID := "enc-123"
	playerID := "player-1"
	characterID := "char-1"

	// Mock Get
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: encounterID}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:     encounterID,
				State:  encounterrepo.StateWaiting,
				HostID: playerID,
				Players: map[string]*encounterrepo.Player{
					playerID: {
						PlayerID:    playerID,
						CharacterID: characterID,
						IsConnected: false, // Already disconnected
					},
				},
			},
		}, nil)

	// NO Update or Publish expected

	// Act
	output, err := s.orchestrator.PlayerDisconnected(context.Background(), &PlayerDisconnectedInput{
		EncounterID: encounterID,
		PlayerID:    playerID,
		Reason:      "timeout",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Equal(ErrPlayerAlreadyDisconnected, err)
}

func (s *EventPublishingTestSuite) TestPlayerReconnected_AlreadyConnected() {
	// Arrange
	encounterID := "enc-123"
	playerID := "player-1"
	characterID := "char-1"

	// Mock Get
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: encounterID}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:     encounterID,
				State:  encounterrepo.StateWaiting,
				HostID: playerID,
				Players: map[string]*encounterrepo.Player{
					playerID: {
						PlayerID:    playerID,
						CharacterID: characterID,
						IsConnected: true, // Already connected
					},
				},
			},
		}, nil)

	// NO Update or Publish expected

	// Act
	output, err := s.orchestrator.PlayerReconnected(context.Background(), &PlayerReconnectedInput{
		EncounterID: encounterID,
		PlayerID:    playerID,
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Equal(ErrPlayerAlreadyConnected, err)
}

func (s *EventPublishingTestSuite) TestPlayerDisconnected_PlayerNotInEncounter() {
	// Arrange
	encounterID := "enc-123"
	playerID := "player-not-exists"

	// Mock Get - encounter has no such player
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: encounterID}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:      encounterID,
				State:   encounterrepo.StateWaiting,
				HostID:  "player-1",
				Players: map[string]*encounterrepo.Player{},
			},
		}, nil)

	// Act
	output, err := s.orchestrator.PlayerDisconnected(context.Background(), &PlayerDisconnectedInput{
		EncounterID: encounterID,
		PlayerID:    playerID,
		Reason:      "timeout",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Equal(ErrPlayerNotInEncounter, err)
}
