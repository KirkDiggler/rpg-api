package encounter

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	mock_dice "github.com/KirkDiggler/rpg-toolkit/dice/mock"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/initiative"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	dungeontoolkit "github.com/KirkDiggler/rpg-api/internal/components/dungeon/toolkit"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	eventprocessor "github.com/KirkDiggler/rpg-api/internal/processors/event"
	eventmock "github.com/KirkDiggler/rpg-api/internal/processors/event/mock"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	dungeonrepo "github.com/KirkDiggler/rpg-api/internal/repositories/dungeons"
	dungeonmock "github.com/KirkDiggler/rpg-api/internal/repositories/dungeons/mock"
	encounterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/encounters"
	encountermock "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/mock"
)

// EventPublishingTestSuite tests that events are correctly published
// when orchestrator methods are called
type EventPublishingTestSuite struct {
	suite.Suite
	ctrl               *gomock.Controller
	mockCharRepo       *charactermock.MockRepository
	mockEncRepo        *encountermock.MockRepository
	mockDungeonRepo    *dungeonmock.MockRepository
	mockEventProcessor *eventmock.MockProcessor
	mockRoller         *mock_dice.MockRoller
	orchestrator       *Orchestrator
}

func (s *EventPublishingTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockCharRepo = charactermock.NewMockRepository(s.ctrl)
	s.mockEncRepo = encountermock.NewMockRepository(s.ctrl)
	s.mockDungeonRepo = dungeonmock.NewMockRepository(s.ctrl)
	s.mockEventProcessor = eventmock.NewMockProcessor(s.ctrl)
	s.mockRoller = mock_dice.NewMockRoller(s.ctrl)

	// Default: high rolls ensure players go first in initiative (deterministic tests)
	s.mockRoller.EXPECT().Roll(gomock.Any(), gomock.Any()).Return(20, nil).AnyTimes()
	// RollN is used by dungeon generator for monster HP (hit dice)
	s.mockRoller.EXPECT().RollN(gomock.Any(), gomock.Any(), gomock.Any()).Return([]int{4}, nil).AnyTimes()

	var err error
	s.orchestrator, err = New(&Config{
		CharacterRepo:  s.mockCharRepo,
		EncounterRepo:  s.mockEncRepo,
		DungeonRepo:    s.mockDungeonRepo,
		DungeonGen:     dungeontoolkit.CreateGenerator(&dungeontoolkit.ToolkitConfig{}),
		EventProcessor: s.mockEventProcessor,
		Roller:         s.mockRoller,
	})
	s.Require().NoError(err)
}

func (s *EventPublishingTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func TestEventPublishingSuite(t *testing.T) {
	suite.Run(t, new(EventPublishingTestSuite))
}

// lastEventIDUpdateMatcher matches Update calls that only set LastEventID
type lastEventIDUpdateMatcher struct {
	encounterID string
}

func (m lastEventIDUpdateMatcher) Matches(x interface{}) bool {
	input, ok := x.(*encounterrepo.UpdateInput)
	if !ok {
		return false
	}
	// Must have the right encounter ID
	if input.EncounterID != m.encounterID {
		return false
	}
	// Must have LastEventID set
	if input.LastEventID == nil {
		return false
	}
	// Should not have other fields set (this is a LastEventID-only update)
	if input.InitiativeData != nil || input.RoomData != nil ||
		input.MovementRemaining != nil || input.ActionEconomy != nil ||
		input.Monsters != nil || input.BossMonsterIDs != nil ||
		input.CharacterHP != nil || input.State != nil ||
		input.HostID != nil || input.Players != nil {
		return false
	}
	return true
}

func (m lastEventIDUpdateMatcher) String() string {
	return "is LastEventID update for " + m.encounterID
}

// isLastEventIDUpdate returns a matcher for LastEventID-only updates
func isLastEventIDUpdate(encounterID string) gomock.Matcher {
	return lastEventIDUpdateMatcher{encounterID: encounterID}
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

	// Mock Update (may be called multiple times: once for main operation, once for LastEventID)
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil)

	// Mock character lookup for event publishing (called first)
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: characterID}).
		Return(&characterrepo.GetOutput{
			Character: &entities.Character{Data: &character.Data{
				ID:       characterID,
				Name:     "Test Character",
				PlayerID: playerID,
			}},
		}, nil)

	// Mock character lookup for building party list (called second)
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: characterID}).
		Return(&characterrepo.GetOutput{
			Character: &entities.Character{Data: &character.Data{
				ID:       characterID,
				Name:     "Test Character",
				PlayerID: playerID,
			}},
		}, nil)

	// Expect PlayerJoined event to be processed
	s.mockEventProcessor.EXPECT().
		Process(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input *eventprocessor.ProcessInput) (*eventprocessor.ProcessOutput, error) {
			s.Assert().Equal(encounterID, input.EncounterID)
			s.Assert().Equal(entities.EventTypePlayerJoined, input.Event.Type)

			// Verify event data using typed field
			s.Require().NotNil(input.Event.PlayerJoined, "PlayerJoined should be set")
			s.Assert().Equal(playerID, input.Event.PlayerJoined.PlayerID)
			s.Assert().Equal(characterID, input.Event.PlayerJoined.CharacterID)
			// Verify character data is included
			s.Require().NotNil(input.Event.PlayerJoined.CharacterData, "CharacterData should be set")
			s.Assert().Equal("Test Character", input.Event.PlayerJoined.CharacterData.Name)

			return &eventprocessor.ProcessOutput{EventID: "evt-1"}, nil
		})

	// Expect LastEventID update after publish
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), isLastEventIDUpdate(encounterID)).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil)

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

	// Expect PlayerReady event to be processed
	s.mockEventProcessor.EXPECT().
		Process(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input *eventprocessor.ProcessInput) (*eventprocessor.ProcessOutput, error) {
			s.Assert().Equal(encounterID, input.EncounterID)
			s.Assert().Equal(entities.EventTypePlayerReady, input.Event.Type)

			// Verify event data using typed field
			s.Require().NotNil(input.Event.PlayerReady, "PlayerReady should be set")
			s.Assert().Equal(playerID, input.Event.PlayerReady.PlayerID)
			s.Assert().Equal(characterID, input.Event.PlayerReady.CharacterID)
			s.Assert().True(input.Event.PlayerReady.Ready)

			return &eventprocessor.ProcessOutput{EventID: "evt-1"}, nil
		})

	// Expect LastEventID update after publish
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), isLastEventIDUpdate(encounterID)).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil)

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
			Character: &entities.Character{Data: &character.Data{
				ID:        characterID,
				PlayerID:  playerID,
				HitPoints: 10, // Set HP to non-zero to avoid triggering TPK check
				AbilityScores: shared.AbilityScores{
					abilities.DEX: 14, // +2 modifier
				},
			}},
		}, nil).AnyTimes()

	// Mock character update (long rest saves rested character before combat)
	s.mockCharRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&characterrepo.UpdateOutput{}, nil).
		AnyTimes()

	// Mock dungeon repo save (dungeon is generated when combat starts)
	s.mockDungeonRepo.EXPECT().
		Save(gomock.Any(), gomock.Any()).
		Return(&dungeonrepo.SaveOutput{Success: true}, nil)

	// Mock dungeon repo get (for wall collision in monster turns)
	s.mockDungeonRepo.EXPECT().
		GetByEncounterID(gomock.Any(), &dungeonrepo.GetByEncounterIDInput{EncounterID: encounterID}).
		Return(&dungeonrepo.GetOutput{Dungeon: nil}, nil).
		AnyTimes()

	// Mock Update (may be called multiple times if monster goes first)
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil).
		AnyTimes()

	// Expect CombatStarted event to be processed (may be followed by monster turn events)
	var sawCombatStarted atomic.Bool
	s.mockEventProcessor.EXPECT().
		Process(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *eventprocessor.ProcessInput) (*eventprocessor.ProcessOutput, error) {
			if input.Event.Type == entities.EventTypeCombatStarted {
				sawCombatStarted.Store(true)
			}
			return &eventprocessor.ProcessOutput{EventID: "evt-1"}, nil
		}).AnyTimes()

	// Act
	output, err := s.orchestrator.StartCombat(context.Background(), &StartCombatInput{
		EncounterID: encounterID,
		PlayerID:    playerID,
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Require().NotNil(output.CombatState)
	s.Assert().True(sawCombatStarted.Load(), "CombatStarted event should have been published")
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

	// Expect PlayerLeft event to be processed
	s.mockEventProcessor.EXPECT().
		Process(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input *eventprocessor.ProcessInput) (*eventprocessor.ProcessOutput, error) {
			s.Assert().Equal(encounterID, input.EncounterID)
			s.Assert().Equal(entities.EventTypePlayerLeft, input.Event.Type)

			// Verify event data using typed field
			s.Require().NotNil(input.Event.PlayerLeft, "PlayerLeft should be set")
			s.Assert().Equal(playerID, input.Event.PlayerLeft.PlayerID)
			s.Assert().Equal(characterID, input.Event.PlayerLeft.CharacterID)
			s.Assert().Equal("voluntary", input.Event.PlayerLeft.Reason)

			return &eventprocessor.ProcessOutput{EventID: "evt-1"}, nil
		})

	// Expect LastEventID update after publish
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), isLastEventIDUpdate(encounterID)).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil)

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
		ID:       encounterID + "-room",
		Width:    20,
		Height:   20,
		GridType: spatial.GridTypeHex,
		CubeEntities: map[string]spatial.EntityCubePlacement{
			entityID: {
				EntityID:       entityID,
				EntityType:     "character",
				CubePosition:   spatial.CubeCoordinate{X: 3, Y: -6, Z: 3}, //nolint:gocritic // valid cube coords
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

	// Mock dungeon repo for walls loading
	s.mockDungeonRepo.EXPECT().
		GetByEncounterID(gomock.Any(), gomock.Any()).
		Return(&dungeonrepo.GetOutput{
			Dungeon: &entities.Dungeon{ID: "test-dungeon"},
		}, nil)

	// Mock Update
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil)

	// Expect MovementCompleted event to be processed
	s.mockEventProcessor.EXPECT().
		Process(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input *eventprocessor.ProcessInput) (*eventprocessor.ProcessOutput, error) {
			s.Assert().Equal(encounterID, input.EncounterID)
			s.Assert().Equal(entities.EventTypeMovementCompleted, input.Event.Type)

			// Verify event data using typed field
			s.Require().NotNil(input.Event.MovementCompleted, "MovementCompleted should be set")
			s.Assert().Equal(entityID, input.Event.MovementCompleted.EntityID)
			s.Assert().Equal("character", input.Event.MovementCompleted.EntityType)
			s.Assert().Equal("completed", input.Event.MovementCompleted.StopReason)

			return &eventprocessor.ProcessOutput{EventID: "evt-1"}, nil
		})

	// Expect LastEventID update after publish
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), isLastEventIDUpdate(encounterID)).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil)

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
	characterID := "char-1"

	initiativeData := &initiative.TrackerData{
		Order: []initiative.EntityData{
			{ID: characterID, Type: "character"},
			{ID: "char-2", Type: "character"},
		},
		Current: 0,
		Round:   1,
	}

	initiativeRolls := []initiative.Roll{
		{Entity: initiative.NewParticipant(characterID, "character"), Roll: 18, Modifier: 2, Total: 20},
		{Entity: initiative.NewParticipant("char-2", "character"), Roll: 10, Modifier: 1, Total: 11},
	}

	// Create character data for turn-end event publishing
	charData := &character.Data{
		ID:               characterID,
		Name:             "Test Character",
		PlayerID:         "player-1",
		Level:            1,
		RaceID:           "human",
		ClassID:          "fighter",
		ProficiencyBonus: 2,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 16,
			abilities.DEX: 14,
			abilities.CON: 15,
			abilities.INT: 10,
			abilities.WIS: 12,
			abilities.CHA: 8,
		},
		HitPoints:    12,
		MaxHitPoints: 12,
	}

	// Mock Get encounter
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: encounterID}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:              encounterID,
				InitiativeData:  initiativeData,
				InitiativeRolls: initiativeRolls,
			},
		}, nil)

	// Mock Get character for turn-end event publishing
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: characterID}).
		Return(&characterrepo.GetOutput{Character: &entities.Character{Data: charData}}, nil)

	// Mock Update character after turn-end event processing
	s.mockCharRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&characterrepo.UpdateOutput{}, nil)

	// Mock dungeon lookup for walls (optional - just return empty)
	s.mockDungeonRepo.EXPECT().
		GetByEncounterID(gomock.Any(), &dungeonrepo.GetByEncounterIDInput{EncounterID: encounterID}).
		Return(&dungeonrepo.GetOutput{}, nil)

	// Mock Update encounter
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil)

	// Expect TurnEnded event to be processed
	s.mockEventProcessor.EXPECT().
		Process(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input *eventprocessor.ProcessInput) (*eventprocessor.ProcessOutput, error) {
			s.Assert().Equal(encounterID, input.EncounterID)
			s.Assert().Equal(entities.EventTypeTurnEnded, input.Event.Type)

			// Verify event data using typed field
			s.Require().NotNil(input.Event.TurnEnded, "TurnEnded should be set")
			s.Assert().Equal(characterID, input.Event.TurnEnded.PreviousEntityID)
			s.Assert().Equal("char-2", input.Event.TurnEnded.NextEntityID)
			s.Assert().Equal(1, input.Event.TurnEnded.Round)
			s.Assert().False(input.Event.TurnEnded.NewRound)

			return &eventprocessor.ProcessOutput{EventID: "evt-1"}, nil
		})

	// Expect LastEventID update after publish
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), isLastEventIDUpdate(encounterID)).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil)

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
			Character: &entities.Character{Data: charData},
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

	// EventProcessor returns error - but operation should still succeed
	s.mockEventProcessor.EXPECT().
		Process(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("process failed")) // Using any error for testing

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

func (s *EventPublishingTestSuite) TestNoEventProcessor_DoesNotPanic() {
	// Arrange - orchestrator without event processor
	orchWithoutEventProcessor, err := New(&Config{
		CharacterRepo: s.mockCharRepo,
		EncounterRepo: s.mockEncRepo,
		DungeonRepo:   s.mockDungeonRepo,
		DungeonGen:    dungeontoolkit.CreateGenerator(&dungeontoolkit.ToolkitConfig{}),
		// EventProcessor: nil - intentionally omitted
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

	// NO event processor expectations - it's nil

	// Act - should NOT panic
	output, err := orchWithoutEventProcessor.SetReady(context.Background(), &SetReadyInput{
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
	processCalled := false
	s.mockEventProcessor.EXPECT().
		Process(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input *eventprocessor.ProcessInput) (*eventprocessor.ProcessOutput, error) {
			processCalled = true
			s.Assert().Equal(entities.EventTypePlayerLeft, input.Event.Type)
			return &eventprocessor.ProcessOutput{EventID: "evt-1"}, nil
		})

	// Expect LastEventID update after publish
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), isLastEventIDUpdate(encounterID)).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil)

	// Mock Delete (called after process)
	s.mockEncRepo.EXPECT().
		Delete(gomock.Any(), &encounterrepo.DeleteInput{EncounterID: encounterID}).
		DoAndReturn(func(ctx context.Context, input *encounterrepo.DeleteInput) (*encounterrepo.DeleteOutput, error) {
			// Verify process was called before delete
			s.Assert().True(processCalled, "process should be called before delete")
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
	characterID := "char-2" // This is the active entity (Current: 1)

	initiativeData := &initiative.TrackerData{
		Order: []initiative.EntityData{
			{ID: "char-1", Type: "character"},
			{ID: characterID, Type: "character"},
		},
		Current: 1, // Last entity in order
		Round:   1,
	}

	initiativeRolls := []initiative.Roll{
		{Entity: initiative.NewParticipant("char-1", "character"), Roll: 18, Modifier: 2, Total: 20},
		{Entity: initiative.NewParticipant(characterID, "character"), Roll: 10, Modifier: 1, Total: 11},
	}

	// Create character data for turn-end event publishing
	charData := &character.Data{
		ID:               characterID,
		Name:             "Test Character 2",
		PlayerID:         "player-2",
		Level:            1,
		RaceID:           "human",
		ClassID:          "fighter",
		ProficiencyBonus: 2,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 16,
			abilities.DEX: 14,
			abilities.CON: 15,
			abilities.INT: 10,
			abilities.WIS: 12,
			abilities.CHA: 8,
		},
		HitPoints:    12,
		MaxHitPoints: 12,
	}

	// Mock Get encounter
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: encounterID}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:              encounterID,
				InitiativeData:  initiativeData,
				InitiativeRolls: initiativeRolls,
			},
		}, nil)

	// Mock Get character for turn-end event publishing
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: characterID}).
		Return(&characterrepo.GetOutput{Character: &entities.Character{Data: charData}}, nil)

	// Mock Update character after turn-end event processing
	s.mockCharRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&characterrepo.UpdateOutput{}, nil)

	// Mock dungeon lookup for walls (optional - just return empty)
	s.mockDungeonRepo.EXPECT().
		GetByEncounterID(gomock.Any(), &dungeonrepo.GetByEncounterIDInput{EncounterID: encounterID}).
		Return(&dungeonrepo.GetOutput{}, nil)

	// Mock Update encounter
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil)

	// Expect TurnEnded with NewRound=true
	s.mockEventProcessor.EXPECT().
		Process(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input *eventprocessor.ProcessInput) (*eventprocessor.ProcessOutput, error) {
			s.Assert().Equal(entities.EventTypeTurnEnded, input.Event.Type)

			// Verify event data using typed field
			s.Require().NotNil(input.Event.TurnEnded, "TurnEnded should be set")
			s.Assert().Equal("char-2", input.Event.TurnEnded.PreviousEntityID)
			s.Assert().Equal("char-1", input.Event.TurnEnded.NextEntityID) // Wraps to first
			s.Assert().Equal(2, input.Event.TurnEnded.Round)               // Incremented
			s.Assert().True(input.Event.TurnEnded.NewRound)

			return &eventprocessor.ProcessOutput{EventID: "evt-1"}, nil
		})

	// Expect LastEventID update after publish
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), isLastEventIDUpdate(encounterID)).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil)

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
	s.mockEventProcessor.EXPECT().
		Process(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input *eventprocessor.ProcessInput) (*eventprocessor.ProcessOutput, error) {
			// Verify timestamp is reasonable (within 1 second)
			s.Assert().WithinDuration(beforeTest, input.Event.Timestamp, time.Second)
			return &eventprocessor.ProcessOutput{EventID: "evt-1"}, nil
		})

	// Expect LastEventID update after publish
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), isLastEventIDUpdate(encounterID)).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil)

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
	s.mockEventProcessor.EXPECT().
		Process(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input *eventprocessor.ProcessInput) (*eventprocessor.ProcessOutput, error) {
			s.Assert().Equal(encounterID, input.EncounterID)
			s.Assert().Equal(entities.EventTypePlayerDisconnected, input.Event.Type)

			// Verify event data using typed field
			s.Require().NotNil(input.Event.PlayerDisconnected, "PlayerDisconnected should be set")
			s.Assert().Equal(playerID, input.Event.PlayerDisconnected.PlayerID)
			s.Assert().Equal(characterID, input.Event.PlayerDisconnected.CharacterID)
			s.Assert().Equal("timeout", input.Event.PlayerDisconnected.Reason)

			return &eventprocessor.ProcessOutput{EventID: "evt-1"}, nil
		})

	// Expect LastEventID update after publish
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), isLastEventIDUpdate(encounterID)).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil)

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
	firstCall := s.mockEventProcessor.EXPECT().
		Process(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input *eventprocessor.ProcessInput) (*eventprocessor.ProcessOutput, error) {
			s.Assert().Equal(entities.EventTypePlayerDisconnected, input.Event.Type)
			return &eventprocessor.ProcessOutput{EventID: "evt-1"}, nil
		})

	// Expect LastEventID update after first process
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), isLastEventIDUpdate(encounterID)).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil)

	// Then CombatPaused event
	s.mockEventProcessor.EXPECT().
		Process(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input *eventprocessor.ProcessInput) (*eventprocessor.ProcessOutput, error) {
			s.Assert().Equal(entities.EventTypeCombatPaused, input.Event.Type)

			// Verify event data using typed field
			s.Require().NotNil(input.Event.CombatPaused, "CombatPaused should be set")
			s.Assert().Equal(playerID, input.Event.CombatPaused.PausedBy)
			s.Assert().Equal("all players disconnected", input.Event.CombatPaused.Reason)

			return &eventprocessor.ProcessOutput{EventID: "evt-2"}, nil
		}).After(firstCall)

	// Expect LastEventID update after second publish
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), isLastEventIDUpdate(encounterID)).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil)

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
	s.mockEventProcessor.EXPECT().
		Process(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input *eventprocessor.ProcessInput) (*eventprocessor.ProcessOutput, error) {
			s.Assert().Equal(encounterID, input.EncounterID)
			s.Assert().Equal(entities.EventTypePlayerReconnected, input.Event.Type)

			// Verify event data using typed field
			s.Require().NotNil(input.Event.PlayerReconnected, "PlayerReconnected should be set")
			s.Assert().Equal(playerID, input.Event.PlayerReconnected.PlayerID)
			s.Assert().Equal(characterID, input.Event.PlayerReconnected.CharacterID)

			return &eventprocessor.ProcessOutput{EventID: "evt-1"}, nil
		})

	// Expect LastEventID update after publish
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), isLastEventIDUpdate(encounterID)).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil)

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
	firstCall := s.mockEventProcessor.EXPECT().
		Process(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input *eventprocessor.ProcessInput) (*eventprocessor.ProcessOutput, error) {
			s.Assert().Equal(entities.EventTypePlayerReconnected, input.Event.Type)
			return &eventprocessor.ProcessOutput{EventID: "evt-1"}, nil
		})

	// Expect LastEventID update after first process
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), isLastEventIDUpdate(encounterID)).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil)

	// Then CombatResumed event
	s.mockEventProcessor.EXPECT().
		Process(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input *eventprocessor.ProcessInput) (*eventprocessor.ProcessOutput, error) {
			s.Assert().Equal(entities.EventTypeCombatResumed, input.Event.Type)

			// Verify event data using typed field
			s.Require().NotNil(input.Event.CombatResumed, "CombatResumed should be set")
			s.Assert().Equal(playerID, input.Event.CombatResumed.ResumedBy)

			return &eventprocessor.ProcessOutput{EventID: "evt-2"}, nil
		}).After(firstCall)

	// Expect LastEventID update after second publish
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), isLastEventIDUpdate(encounterID)).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil)

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
