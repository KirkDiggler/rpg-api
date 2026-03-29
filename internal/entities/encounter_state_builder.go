// Package entities defines the core data structures for the RPG API
package entities

import (
	"errors"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
)

// BuildEncounterStateDataInput is the input for BuildEncounterStateData.
type BuildEncounterStateDataInput struct {
	EncounterID     string
	DungeonID       string
	Entities        map[string]*EntityStateData
	CurrentRoomID   string
	RevealedRoomIDs []string
	Combat          *CombatState
	DungeonState    DungeonState
	RoomsCleared    int
	Doors           map[string]*DoorInfo
}

// BuildEncounterStateDataOutput is the output for BuildEncounterStateData.
type BuildEncounterStateDataOutput struct {
	EncounterStateData *dnd5ev1alpha1.EncounterStateData
}

// BuildEncounterStateData builds the full proto EncounterStateData message from the API's
// encounter data. This is what snapshot events (CombatStarted, RoomRevealed, etc.) will use.
//
//nolint:gosec // G115: Game values are bounded by D&D rules, no overflow risk
func BuildEncounterStateData(input *BuildEncounterStateDataInput) (*BuildEncounterStateDataOutput, error) {
	if input == nil {
		return nil, errors.New("input is required")
	}

	state := &dnd5ev1alpha1.EncounterStateData{
		EncounterId:     input.EncounterID,
		DungeonId:       input.DungeonID,
		CurrentRoomId:   input.CurrentRoomID,
		RevealedRoomIds: input.RevealedRoomIDs,
		DungeonState:    dungeonStateToProto(input.DungeonState),
		RoomsCleared:    int32(input.RoomsCleared),
	}

	// Convert entities
	state.Entities = buildEntityMap(input.Entities)

	// Convert combat state
	state.Combat = combatStateToProto(input.Combat)

	// Convert doors
	state.Doors = buildDoorMap(input.Doors)

	// TODO: Convert rooms to RoomLayout map. Room layout conversion depends on spatial types
	// that live in the handler layer. For now the rooms map is empty -- entities are the
	// critical path. Room layouts are static spatial data sent via RoomRevealed events.

	return &BuildEncounterStateDataOutput{
		EncounterStateData: state,
	}, nil
}

// buildEntityMap converts all EntityStateData entries to proto EntityState messages.
func buildEntityMap(entities map[string]*EntityStateData) map[string]*dnd5ev1alpha1.EntityState {
	if len(entities) == 0 {
		return nil
	}

	result := make(map[string]*dnd5ev1alpha1.EntityState, len(entities))
	for id, esd := range entities {
		if esd == nil {
			continue
		}
		result[id] = ToEntityStateProto(&ToEntityStateProtoInput{
			EntityStateData: esd,
		})
	}

	return result
}

// combatStateToProto converts an entities.CombatState to the proto CombatState message.
// This is a simplified version of the handler layer's convertCombatStateToProto that does
// not depend on spatial grid conversion (positions are already in cube coordinates).
//
//nolint:gosec // G115: Game values are bounded by D&D rules, no overflow risk
func combatStateToProto(cs *CombatState) *dnd5ev1alpha1.CombatState {
	if cs == nil {
		return nil
	}

	turnOrder := make([]*dnd5ev1alpha1.InitiativeEntry, len(cs.TurnOrder))
	for i, entry := range cs.TurnOrder {
		turnOrder[i] = &dnd5ev1alpha1.InitiativeEntry{
			EntityId:   entry.EntityID,
			EntityType: entry.EntityType,
			Initiative: int32(entry.InitiativeTotal),
			Modifier:   int32(entry.InitiativeModifier),
		}
	}

	return &dnd5ev1alpha1.CombatState{
		EncounterId:   cs.EncounterID,
		Round:         int32(cs.Round),
		TurnOrder:     turnOrder,
		ActiveIndex:   int32(cs.ActiveIndex),
		CombatStarted: cs.CombatStarted,
		CombatEnded:   cs.CombatEnded,
	}
}

// dungeonStateToProto converts an entities.DungeonState to the proto DungeonState enum.
func dungeonStateToProto(ds DungeonState) dnd5ev1alpha1.DungeonState {
	switch ds {
	case DungeonStateActive:
		return dnd5ev1alpha1.DungeonState_DUNGEON_STATE_ACTIVE
	case DungeonStateVictorious:
		return dnd5ev1alpha1.DungeonState_DUNGEON_STATE_VICTORIOUS
	case DungeonStateFailed:
		return dnd5ev1alpha1.DungeonState_DUNGEON_STATE_FAILED
	case DungeonStateAbandoned:
		return dnd5ev1alpha1.DungeonState_DUNGEON_STATE_ABANDONED
	default:
		return dnd5ev1alpha1.DungeonState_DUNGEON_STATE_UNSPECIFIED
	}
}

// buildDoorMap converts DoorInfo entries to proto DoorInfo messages.
func buildDoorMap(doors map[string]*DoorInfo) map[string]*dnd5ev1alpha1.DoorInfo {
	if len(doors) == 0 {
		return nil
	}

	result := make(map[string]*dnd5ev1alpha1.DoorInfo, len(doors))
	for id, door := range doors {
		if door == nil {
			continue
		}
		result[id] = doorInfoToProto(door)
	}

	return result
}

// doorInfoToProto converts an entities.DoorInfo to the proto DoorInfo message.
func doorInfoToProto(door *DoorInfo) *dnd5ev1alpha1.DoorInfo {
	protoDoor := &dnd5ev1alpha1.DoorInfo{
		ConnectionId: door.ConnectionID,
		Position:     positionToProto(door.Position),
		PhysicalHint: door.Direction,
		IsOpen:       door.IsOpen,
	}

	// Only reveal the target room ID if the door is open
	if door.IsOpen {
		protoDoor.LeadsToRoomId = door.TargetRoomID
	}

	return protoDoor
}
