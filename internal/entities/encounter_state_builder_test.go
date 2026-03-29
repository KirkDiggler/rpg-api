package entities_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	apiv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/api/v1alpha1"
	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/races"
)

type EncounterStateBuilderTestSuite struct {
	suite.Suite
}

func TestEncounterStateBuilder(t *testing.T) {
	suite.Run(t, new(EncounterStateBuilderTestSuite))
}

func (s *EncounterStateBuilderTestSuite) TestBuildEncounterStateData_NilInput() {
	_, err := entities.BuildEncounterStateData(nil)
	s.Error(err)
}

func (s *EncounterStateBuilderTestSuite) TestBuildEncounterStateData_MinimalInput() {
	output, err := entities.BuildEncounterStateData(&entities.BuildEncounterStateDataInput{
		EncounterID: "enc-1",
		DungeonID:   "dun-1",
	})

	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Require().NotNil(output.EncounterStateData)

	state := output.EncounterStateData
	s.Equal("enc-1", state.EncounterId)
	s.Equal("dun-1", state.DungeonId)
	s.Empty(state.Entities)
	s.Empty(state.Rooms)
	s.Empty(state.CurrentRoomId)
	s.Empty(state.RevealedRoomIds)
	s.Nil(state.Combat)
	s.Equal(dnd5ev1alpha1.DungeonState_DUNGEON_STATE_UNSPECIFIED, state.DungeonState)
	s.Equal(int32(0), state.RoomsCleared)
	s.Empty(state.Doors)
}

func (s *EncounterStateBuilderTestSuite) TestBuildEncounterStateData_FullInput() {
	charData := &toolkitchar.Data{
		ID:           "char-1",
		Name:         "Thorin",
		Level:        5,
		RaceID:       races.Dwarf,
		ClassID:      classes.Fighter,
		HitPoints:    38,
		MaxHitPoints: 44,
		ArmorClass:   18,
	}

	monsterData := &monster.Data{
		ID:           "mon-1",
		Name:         "Goblin",
		HitPoints:    7,
		MaxHitPoints: 7,
		ArmorClass:   15,
	}

	entitiesMap := map[string]*entities.EntityStateData{
		"char-1": {
			EntityID:    "char-1",
			EntityType:  entities.EntityTypeCharacter,
			RoomID:      "room-1",
			Position:    &entities.Position{X: 1, Y: -1, Z: 0},
			Size:        2,
			ToolkitData: charData,
		},
		"mon-1": {
			EntityID:    "mon-1",
			EntityType:  entities.EntityTypeMonster,
			RoomID:      "room-1",
			Position:    &entities.Position{X: 3, Y: -2, Z: -1},
			Size:        1,
			ToolkitData: monsterData,
		},
	}

	combat := &entities.CombatState{
		EncounterID:   "enc-1",
		Round:         2,
		ActiveIndex:   1,
		CombatStarted: true,
		CombatEnded:   false,
		TurnOrder: []entities.InitiativeEntry{
			{
				EntityID:        "char-1",
				EntityType:      "character",
				InitiativeTotal: 18,
			},
			{
				EntityID:        "mon-1",
				EntityType:      "monster",
				InitiativeTotal: 12,
			},
		},
		MovementRemaining: 25,
		ActionEconomy:     entities.NewActionEconomyState(),
	}

	doors := map[string]*entities.DoorInfo{
		"conn-1": {
			ConnectionID: "conn-1",
			TargetRoomID: "room-2",
			Direction:    "north door",
			Position:     &entities.Position{X: 0, Y: 0, Z: 0},
			IsOpen:       false,
		},
		"conn-2": {
			ConnectionID: "conn-2",
			TargetRoomID: "room-3",
			Direction:    "east passage",
			Position:     &entities.Position{X: 5, Y: -3, Z: -2},
			IsOpen:       true,
		},
	}

	output, err := entities.BuildEncounterStateData(&entities.BuildEncounterStateDataInput{
		EncounterID:     "enc-1",
		DungeonID:       "dun-1",
		Entities:        entitiesMap,
		CurrentRoomID:   "room-1",
		RevealedRoomIDs: []string{"room-1"},
		Combat:          combat,
		DungeonState:    entities.DungeonStateActive,
		RoomsCleared:    1,
		Doors:           doors,
	})

	s.Require().NoError(err)
	s.Require().NotNil(output)
	state := output.EncounterStateData

	// Metadata
	s.Equal("enc-1", state.EncounterId)
	s.Equal("dun-1", state.DungeonId)
	s.Equal("room-1", state.CurrentRoomId)
	s.Equal([]string{"room-1"}, state.RevealedRoomIds)
	s.Equal(dnd5ev1alpha1.DungeonState_DUNGEON_STATE_ACTIVE, state.DungeonState)
	s.Equal(int32(1), state.RoomsCleared)

	// Entities
	s.Require().Len(state.Entities, 2)
	charProto := state.Entities["char-1"]
	s.Require().NotNil(charProto)
	s.Equal("char-1", charProto.EntityId)
	s.Equal(dnd5ev1alpha1.EntityType_ENTITY_TYPE_CHARACTER, charProto.EntityType)
	s.Equal(int32(38), charProto.CurrentHitPoints)

	monProto := state.Entities["mon-1"]
	s.Require().NotNil(monProto)
	s.Equal("mon-1", monProto.EntityId)
	s.Equal(dnd5ev1alpha1.EntityType_ENTITY_TYPE_MONSTER, monProto.EntityType)
	s.Equal(int32(7), monProto.CurrentHitPoints)

	// Combat
	s.Require().NotNil(state.Combat)
	s.Equal("enc-1", state.Combat.EncounterId)
	s.Equal(int32(2), state.Combat.Round)
	s.Equal(int32(1), state.Combat.ActiveIndex)
	s.True(state.Combat.CombatStarted)
	s.False(state.Combat.CombatEnded)
	s.Require().Len(state.Combat.TurnOrder, 2)
	s.Equal("char-1", state.Combat.TurnOrder[0].EntityId)
	s.Equal(int32(18), state.Combat.TurnOrder[0].Initiative)
	s.Equal("mon-1", state.Combat.TurnOrder[1].EntityId)
	s.Equal(int32(12), state.Combat.TurnOrder[1].Initiative)

	// Doors
	s.Require().Len(state.Doors, 2)
	door1 := state.Doors["conn-1"]
	s.Require().NotNil(door1)
	s.Equal("conn-1", door1.ConnectionId)
	s.False(door1.IsOpen)
	s.Equal("", door1.LeadsToRoomId) // not open, so no target revealed

	door2 := state.Doors["conn-2"]
	s.Require().NotNil(door2)
	s.Equal("conn-2", door2.ConnectionId)
	s.True(door2.IsOpen)
	s.Equal("room-3", door2.LeadsToRoomId)
}

func (s *EncounterStateBuilderTestSuite) TestBuildEncounterStateData_DungeonStateConversion() {
	tests := []struct {
		name     string
		state    entities.DungeonState
		expected dnd5ev1alpha1.DungeonState
	}{
		{"active", entities.DungeonStateActive, dnd5ev1alpha1.DungeonState_DUNGEON_STATE_ACTIVE},
		{"victorious", entities.DungeonStateVictorious, dnd5ev1alpha1.DungeonState_DUNGEON_STATE_VICTORIOUS},
		{"failed", entities.DungeonStateFailed, dnd5ev1alpha1.DungeonState_DUNGEON_STATE_FAILED},
		{"abandoned", entities.DungeonStateAbandoned, dnd5ev1alpha1.DungeonState_DUNGEON_STATE_ABANDONED},
		{"unspecified", entities.DungeonStateUnspecified, dnd5ev1alpha1.DungeonState_DUNGEON_STATE_UNSPECIFIED},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			output, err := entities.BuildEncounterStateData(&entities.BuildEncounterStateDataInput{
				EncounterID:  "enc-1",
				DungeonID:    "dun-1",
				DungeonState: tt.state,
			})

			s.Require().NoError(err)
			s.Equal(tt.expected, output.EncounterStateData.DungeonState)
		})
	}
}

func (s *EncounterStateBuilderTestSuite) TestBuildEncounterStateData_NilCombat() {
	output, err := entities.BuildEncounterStateData(&entities.BuildEncounterStateDataInput{
		EncounterID: "enc-1",
		DungeonID:   "dun-1",
		Combat:      nil,
	})

	s.Require().NoError(err)
	s.Nil(output.EncounterStateData.Combat)
}

func (s *EncounterStateBuilderTestSuite) TestBuildEncounterStateData_EmptyEntities() {
	output, err := entities.BuildEncounterStateData(&entities.BuildEncounterStateDataInput{
		EncounterID: "enc-1",
		DungeonID:   "dun-1",
		Entities:    map[string]*entities.EntityStateData{},
	})

	s.Require().NoError(err)
	s.Empty(output.EncounterStateData.Entities)
}

func (s *EncounterStateBuilderTestSuite) TestBuildEncounterStateData_NilEntitySkipped() {
	entitiesMap := map[string]*entities.EntityStateData{
		"char-1": nil,
		"char-2": {
			EntityID:    "char-2",
			EntityType:  entities.EntityTypeCharacter,
			ToolkitData: &toolkitchar.Data{Name: "Elara", HitPoints: 20, MaxHitPoints: 20},
		},
	}

	output, err := entities.BuildEncounterStateData(&entities.BuildEncounterStateDataInput{
		EncounterID: "enc-1",
		DungeonID:   "dun-1",
		Entities:    entitiesMap,
	})

	s.Require().NoError(err)
	// nil entity should be skipped
	s.Len(output.EncounterStateData.Entities, 1)
	s.NotNil(output.EncounterStateData.Entities["char-2"])
}

func (s *EncounterStateBuilderTestSuite) TestBuildEncounterStateData_DoorPositionConversion() {
	doors := map[string]*entities.DoorInfo{
		"conn-1": {
			ConnectionID: "conn-1",
			TargetRoomID: "room-2",
			Direction:    "north",
			Position:     &entities.Position{X: 2, Y: -1, Z: -1},
			IsOpen:       false,
		},
	}

	output, err := entities.BuildEncounterStateData(&entities.BuildEncounterStateDataInput{
		EncounterID: "enc-1",
		DungeonID:   "dun-1",
		Doors:       doors,
	})

	s.Require().NoError(err)
	door := output.EncounterStateData.Doors["conn-1"]
	s.Require().NotNil(door)
	s.Require().NotNil(door.Position)
	s.Equal(float64(2), door.Position.X)
	s.Equal(float64(-1), door.Position.Y)
	s.Equal(float64(-1), door.Position.Z)
}

func (s *EncounterStateBuilderTestSuite) TestBuildEncounterStateData_DoorNilPosition() {
	doors := map[string]*entities.DoorInfo{
		"conn-1": {
			ConnectionID: "conn-1",
			TargetRoomID: "room-2",
			Direction:    "north",
			Position:     nil,
			IsOpen:       true,
		},
	}

	output, err := entities.BuildEncounterStateData(&entities.BuildEncounterStateDataInput{
		EncounterID: "enc-1",
		DungeonID:   "dun-1",
		Doors:       doors,
	})

	s.Require().NoError(err)
	door := output.EncounterStateData.Doors["conn-1"]
	s.Require().NotNil(door)
	// Position should be zero-value when nil
	s.Equal(&apiv1alpha1.Position{}, door.Position)
	s.Equal("room-2", door.LeadsToRoomId)
}

func (s *EncounterStateBuilderTestSuite) TestBuildEncounterStateData_RoomsMapEmptyByDesign() {
	// Rooms map is intentionally empty for now (spatial data handled separately)
	output, err := entities.BuildEncounterStateData(&entities.BuildEncounterStateDataInput{
		EncounterID: "enc-1",
		DungeonID:   "dun-1",
	})

	s.Require().NoError(err)
	s.Empty(output.EncounterStateData.Rooms, "rooms map should be empty until room layout conversion is implemented")
}
