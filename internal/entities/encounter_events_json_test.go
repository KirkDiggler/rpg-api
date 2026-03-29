// Package entities defines the core data structures for the RPG API
package entities

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/suite"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
)

// EncounterEventsJSONSuite tests the round-trip JSON serialization of encounter
// event types that contain proto message fields.
type EncounterEventsJSONSuite struct {
	suite.Suite
}

func TestEncounterEventsJSONSuite(t *testing.T) {
	suite.Run(t, new(EncounterEventsJSONSuite))
}

// makeEncounterStateData builds a minimal non-nil EncounterStateData for testing.
func makeEncounterStateData() *dnd5ev1alpha1.EncounterStateData {
	return &dnd5ev1alpha1.EncounterStateData{
		Entities: map[string]*dnd5ev1alpha1.EntityState{
			"char-1": {
				EntityId:         "char-1",
				EntityType:       dnd5ev1alpha1.EntityType_ENTITY_TYPE_CHARACTER,
				CurrentHitPoints: 20,
				MaxHitPoints:     30,
			},
		},
	}
}

// makeEntityState builds a minimal non-nil EntityState for testing.
func makeEntityState(id string) *dnd5ev1alpha1.EntityState {
	return &dnd5ev1alpha1.EntityState{
		EntityId:         id,
		EntityType:       dnd5ev1alpha1.EntityType_ENTITY_TYPE_CHARACTER,
		CurrentHitPoints: 10,
		MaxHitPoints:     20,
	}
}

// makeCombatState builds a minimal non-nil CombatState for testing.
func makeCombatState() *dnd5ev1alpha1.CombatState {
	return &dnd5ev1alpha1.CombatState{
		Round:       2,
		ActiveIndex: 1,
	}
}

// roundTrip marshals v to JSON and back into a new value of the same type.
func roundTrip[T any](t *testing.T, v T) T {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var out T
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	return out
}

func (s *EncounterEventsJSONSuite) TestPlayerReconnectedEvent_RoundTrip() {
	orig := &PlayerReconnectedEvent{
		PlayerID:           "p1",
		CharacterID:        "c1",
		EncounterStateData: makeEncounterStateData(),
	}
	got := roundTrip(s.T(), orig)
	s.Require().NotNil(got)
	s.Equal(orig.PlayerID, got.PlayerID)
	s.Equal(orig.CharacterID, got.CharacterID)
	s.Require().NotNil(got.EncounterStateData)
	s.Require().Contains(got.EncounterStateData.GetEntities(), "char-1")
	s.Equal(int32(20), got.EncounterStateData.GetEntities()["char-1"].GetCurrentHitPoints())
}

func (s *EncounterEventsJSONSuite) TestPlayerReconnectedEvent_NilProto_RoundTrip() {
	orig := &PlayerReconnectedEvent{PlayerID: "p1", CharacterID: "c1"}
	got := roundTrip(s.T(), orig)
	s.Require().NotNil(got)
	s.Equal("p1", got.PlayerID)
	s.Nil(got.EncounterStateData)
}

func (s *EncounterEventsJSONSuite) TestCombatStartedEvent_RoundTrip() {
	orig := CombatStartedEvent{
		DungeonID:          "dungeon-1",
		EncounterStateData: makeEncounterStateData(),
	}
	got := roundTrip(s.T(), orig)
	s.Equal("dungeon-1", got.DungeonID)
	s.Require().NotNil(got.EncounterStateData)
	s.Require().Contains(got.EncounterStateData.GetEntities(), "char-1")
}

func (s *EncounterEventsJSONSuite) TestCombatStartedEvent_NilProto_RoundTrip() {
	orig := CombatStartedEvent{DungeonID: "dungeon-1"}
	got := roundTrip(s.T(), orig)
	s.Equal("dungeon-1", got.DungeonID)
	s.Nil(got.EncounterStateData)
}

func (s *EncounterEventsJSONSuite) TestCombatEndedEvent_RoundTrip() {
	orig := CombatEndedEvent{
		EncounterResult:    &EncounterResult{Reason: "victory"},
		EncounterStateData: makeEncounterStateData(),
	}
	got := roundTrip(s.T(), orig)
	s.Require().NotNil(got.EncounterResult)
	s.Equal("victory", got.EncounterResult.Reason)
	s.Require().NotNil(got.EncounterStateData)
	s.Require().Contains(got.EncounterStateData.GetEntities(), "char-1")
}

func (s *EncounterEventsJSONSuite) TestCombatResumedEvent_RoundTrip() {
	orig := CombatResumedEvent{
		ResumedBy:          "player-1",
		EncounterStateData: makeEncounterStateData(),
	}
	got := roundTrip(s.T(), orig)
	s.Equal("player-1", got.ResumedBy)
	s.Require().NotNil(got.EncounterStateData)
}

func (s *EncounterEventsJSONSuite) TestMovementCompletedEvent_RoundTrip() {
	orig := MovementCompletedEvent{
		EntityID:         "char-1",
		EntityType:       "character",
		UpdatedEntity:    makeEntityState("char-1"),
		CombatStateProto: makeCombatState(),
	}
	got := roundTrip(s.T(), orig)
	s.Equal("char-1", got.EntityID)
	s.Equal("character", got.EntityType)
	s.Require().NotNil(got.UpdatedEntity)
	s.Equal("char-1", got.UpdatedEntity.GetEntityId())
	s.Equal(int32(10), got.UpdatedEntity.GetCurrentHitPoints())
	s.Require().NotNil(got.CombatStateProto)
	s.Equal(int32(2), got.CombatStateProto.GetRound())
}

func (s *EncounterEventsJSONSuite) TestMovementCompletedEvent_NilProtos_RoundTrip() {
	orig := MovementCompletedEvent{EntityID: "char-1", EntityType: "character"}
	got := roundTrip(s.T(), orig)
	s.Equal("char-1", got.EntityID)
	s.Nil(got.UpdatedEntity)
	s.Nil(got.CombatStateProto)
}

func (s *EncounterEventsJSONSuite) TestAttackResolvedEvent_RoundTrip() {
	orig := AttackResolvedEvent{
		AttackerID:    "char-1",
		TargetID:      "monster-1",
		AttackerState: makeEntityState("char-1"),
		TargetState:   makeEntityState("monster-1"),
	}
	got := roundTrip(s.T(), orig)
	s.Equal("char-1", got.AttackerID)
	s.Equal("monster-1", got.TargetID)
	s.Require().NotNil(got.AttackerState)
	s.Equal("char-1", got.AttackerState.GetEntityId())
	s.Require().NotNil(got.TargetState)
	s.Equal("monster-1", got.TargetState.GetEntityId())
}

func (s *EncounterEventsJSONSuite) TestTurnEndedEvent_RoundTrip() {
	orig := TurnEndedEvent{
		PreviousEntityID: "char-1",
		NextEntityID:     "monster-1",
		Round:            3,
		UpdatedEntities: []*dnd5ev1alpha1.EntityState{
			makeEntityState("char-1"),
			makeEntityState("monster-1"),
		},
		CombatStateProto: makeCombatState(),
	}
	got := roundTrip(s.T(), orig)
	s.Equal("char-1", got.PreviousEntityID)
	s.Equal("monster-1", got.NextEntityID)
	s.Equal(3, got.Round)
	s.Require().Len(got.UpdatedEntities, 2)
	s.Equal("char-1", got.UpdatedEntities[0].GetEntityId())
	s.Equal("monster-1", got.UpdatedEntities[1].GetEntityId())
	s.Require().NotNil(got.CombatStateProto)
	s.Equal(int32(2), got.CombatStateProto.GetRound())
}

func (s *EncounterEventsJSONSuite) TestTurnEndedEvent_EmptySlice_RoundTrip() {
	orig := TurnEndedEvent{PreviousEntityID: "char-1", Round: 1}
	got := roundTrip(s.T(), orig)
	s.Equal("char-1", got.PreviousEntityID)
	s.Nil(got.UpdatedEntities)
	s.Nil(got.CombatStateProto)
}

func (s *EncounterEventsJSONSuite) TestMonsterTurnCompletedEvent_RoundTrip() {
	orig := MonsterTurnCompletedEvent{
		MonsterID:   "monster-1",
		MonsterName: "Goblin",
		UpdatedEntities: []*dnd5ev1alpha1.EntityState{
			makeEntityState("monster-1"),
		},
		CombatStateProto: makeCombatState(),
	}
	got := roundTrip(s.T(), orig)
	s.Equal("monster-1", got.MonsterID)
	s.Equal("Goblin", got.MonsterName)
	s.Require().Len(got.UpdatedEntities, 1)
	s.Equal("monster-1", got.UpdatedEntities[0].GetEntityId())
	s.Require().NotNil(got.CombatStateProto)
	s.Equal(int32(2), got.CombatStateProto.GetRound())
}

func (s *EncounterEventsJSONSuite) TestDungeonVictoryEvent_RoundTrip() {
	orig := DungeonVictoryEvent{
		DungeonID:          "dungeon-1",
		BossID:             "boss-1",
		BossName:           "Dragon",
		EncounterStateData: makeEncounterStateData(),
	}
	got := roundTrip(s.T(), orig)
	s.Equal("dungeon-1", got.DungeonID)
	s.Equal("Dragon", got.BossName)
	s.Require().NotNil(got.EncounterStateData)
}

func (s *EncounterEventsJSONSuite) TestDungeonFailureEvent_RoundTrip() {
	orig := DungeonFailureEvent{
		DungeonID:          "dungeon-1",
		Reason:             "tpk",
		EncounterStateData: makeEncounterStateData(),
	}
	got := roundTrip(s.T(), orig)
	s.Equal("tpk", got.Reason)
	s.Require().NotNil(got.EncounterStateData)
}

func (s *EncounterEventsJSONSuite) TestRoomRevealedEvent_RoundTrip() {
	orig := RoomRevealedEvent{
		DungeonID:          "dungeon-1",
		ConnectionID:       "door-1",
		EncounterStateData: makeEncounterStateData(),
	}
	got := roundTrip(s.T(), orig)
	s.Equal("door-1", got.ConnectionID)
	s.Require().NotNil(got.EncounterStateData)
}

// TestEncounterEvent_FullEnvelope verifies the EncounterEvent wrapper
// correctly serializes a CombatStartedEvent with proto fields through the
// full json.Marshal/Unmarshal cycle (simulating the Redis pub/sub path).
func (s *EncounterEventsJSONSuite) TestEncounterEvent_FullEnvelope_CombatStarted() {
	orig := &EncounterEvent{
		ID:          "evt-1",
		Type:        EventTypeCombatStarted,
		EncounterID: "enc-1",
		CombatStarted: &CombatStartedEvent{
			DungeonID:          "dungeon-1",
			EncounterStateData: makeEncounterStateData(),
		},
	}

	data, err := json.Marshal(orig)
	s.Require().NoError(err)

	var got EncounterEvent
	s.Require().NoError(json.Unmarshal(data, &got))

	s.Equal("evt-1", got.ID)
	s.Require().NotNil(got.CombatStarted)
	s.Equal("dungeon-1", got.CombatStarted.DungeonID)
	s.Require().NotNil(got.CombatStarted.EncounterStateData,
		"EncounterStateData must survive JSON round-trip through Redis pub/sub")
	s.Require().Contains(got.CombatStarted.EncounterStateData.GetEntities(), "char-1")
}
