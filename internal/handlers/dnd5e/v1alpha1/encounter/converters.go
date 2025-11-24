package encounter

import (
	apiv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/api/v1alpha1"
	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/encounter"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

// convertAttackResultToProto converts orchestrator's AttackResult to proto
//
//nolint:gosec // G115: Game values are bounded by D&D rules, no overflow risk
func convertAttackResultToProto(result *encounter.AttackResult) *dnd5ev1alpha1.AttackResult {
	if result == nil {
		return nil
	}

	return &dnd5ev1alpha1.AttackResult{
		Hit:             result.Hit,
		AttackRoll:      int32(result.AttackRoll),
		AttackTotal:     int32(result.TotalAttack),
		TargetAc:        int32(result.TargetAC),
		Damage:          int32(result.TotalDamage),
		DamageType:      result.DamageType,
		Critical:        result.Critical,
		DamageBreakdown: convertDamageBreakdownToProto(result.Breakdown),
	}
}

// convertDamageBreakdownToProto converts orchestrator's DamageBreakdown to proto
//
//nolint:gosec // G115: Game values are bounded by D&D rules, no overflow risk
func convertDamageBreakdownToProto(breakdown *encounter.DamageBreakdown) *dnd5ev1alpha1.DamageBreakdown {
	if breakdown == nil {
		return nil
	}

	components := make([]*dnd5ev1alpha1.DamageComponent, len(breakdown.Components))
	for i, comp := range breakdown.Components {
		components[i] = convertDamageComponentToProto(&comp)
	}

	return &dnd5ev1alpha1.DamageBreakdown{
		Components:  components,
		AbilityUsed: breakdown.AbilityUsed,
		TotalDamage: int32(breakdown.TotalDamage),
	}
}

// convertDamageComponentToProto converts orchestrator's DamageComponent to proto
//
//nolint:gosec // G115: Game values are bounded by D&D rules, no overflow risk
func convertDamageComponentToProto(comp *encounter.DamageComponent) *dnd5ev1alpha1.DamageComponent {
	if comp == nil {
		return nil
	}

	// Convert original dice rolls to int32
	originalRolls := make([]int32, len(comp.OriginalDiceRolls))
	for i, roll := range comp.OriginalDiceRolls {
		originalRolls[i] = int32(roll)
	}

	// Convert final dice rolls to int32
	finalRolls := make([]int32, len(comp.FinalDiceRolls))
	for i, roll := range comp.FinalDiceRolls {
		finalRolls[i] = int32(roll)
	}

	// Convert reroll events
	rerolls := make([]*dnd5ev1alpha1.RerollEvent, len(comp.Rerolls))
	for i, r := range comp.Rerolls {
		rerolls[i] = convertRerollEventToProto(&r)
	}

	return &dnd5ev1alpha1.DamageComponent{
		Source:            comp.Source,
		OriginalDiceRolls: originalRolls,
		FinalDiceRolls:    finalRolls,
		Rerolls:           rerolls,
		FlatBonus:         int32(comp.FlatBonus),
		DamageType:        comp.DamageType,
		IsCritical:        comp.IsCritical,
	}
}

// convertRerollEventToProto converts orchestrator's RerollEvent to proto
//
//nolint:gosec // G115: Game values are bounded by D&D rules, no overflow risk
func convertRerollEventToProto(event *encounter.RerollEvent) *dnd5ev1alpha1.RerollEvent {
	if event == nil {
		return nil
	}

	return &dnd5ev1alpha1.RerollEvent{
		DieIndex: int32(event.DieIndex),
		Before:   int32(event.Before),
		After:    int32(event.After),
		Reason:   event.Reason,
	}
}

// convertRoomDataToProto converts spatial.RoomData to proto Room
//
//nolint:gosec // G115: Game values are bounded by room size limits, no overflow risk
func convertRoomDataToProto(roomData interface{}) *dnd5ev1alpha1.Room {
	if roomData == nil {
		return nil
	}

	// Type assert to spatial.RoomData
	spatialRoom, ok := roomData.(*spatial.RoomData)
	if !ok {
		// If it's not a pointer, try direct struct
		if spatialRoomVal, ok := roomData.(spatial.RoomData); ok {
			spatialRoom = &spatialRoomVal
		} else {
			return nil
		}
	}

	// Convert grid type string to proto enum
	gridType := convertGridTypeToProto(spatialRoom.GridType)

	// Convert entities map
	entities := make(map[string]*dnd5ev1alpha1.EntityPlacement, len(spatialRoom.Entities))
	for id, placement := range spatialRoom.Entities {
		entities[id] = convertEntityPlacementToProto(placement)
	}

	return &dnd5ev1alpha1.Room{
		Id:             spatialRoom.ID,
		Type:           spatialRoom.Type,
		Width:          int32(spatialRoom.Width),
		Height:         int32(spatialRoom.Height),
		GridType:       gridType,
		HexOrientation: spatialRoom.HexOrientation,
		Entities:       entities,
	}
}

// convertEntityPlacementToProto converts spatial.EntityPlacement to proto
//
//nolint:gosec // G115: Game values are bounded, no overflow risk
func convertEntityPlacementToProto(placement spatial.EntityPlacement) *dnd5ev1alpha1.EntityPlacement {
	return &dnd5ev1alpha1.EntityPlacement{
		EntityId:   placement.EntityID,
		EntityType: placement.EntityType,
		Position: &apiv1alpha1.Position{
			X: placement.Position.X,
			Y: placement.Position.Y,
		},
		Size:              int32(placement.Size),
		BlocksMovement:    placement.BlocksMovement,
		BlocksLineOfSight: placement.BlocksLineOfSight,
	}
}

// convertGridTypeToProto converts string grid type to proto enum
func convertGridTypeToProto(gridType string) apiv1alpha1.GridType {
	switch gridType {
	case spatial.GridTypeSquare:
		return apiv1alpha1.GridType_GRID_TYPE_SQUARE
	case spatial.GridTypeHex, "hex_pointy":
		return apiv1alpha1.GridType_GRID_TYPE_HEX_POINTY
	case "hex_flat":
		return apiv1alpha1.GridType_GRID_TYPE_HEX_FLAT
	case spatial.GridTypeGridless:
		return apiv1alpha1.GridType_GRID_TYPE_GRIDLESS
	default:
		return apiv1alpha1.GridType_GRID_TYPE_UNSPECIFIED
	}
}

// convertCombatStateToProto converts orchestrator's CombatState to proto
//
//nolint:gosec // G115: Game values are bounded by D&D rules, no overflow risk
func convertCombatStateToProto(state *encounter.CombatState) *dnd5ev1alpha1.CombatState {
	if state == nil {
		return nil
	}

	turnOrder := make([]*dnd5ev1alpha1.InitiativeEntry, len(state.TurnOrder))
	for i, entry := range state.TurnOrder {
		turnOrder[i] = &dnd5ev1alpha1.InitiativeEntry{
			EntityId:   entry.EntityID,
			EntityType: entry.EntityType,
			Initiative: int32(entry.InitiativeTotal),
			Modifier:   int32(entry.InitiativeModifier),
			HasActed:   false, // Default to false for new combat
		}
	}

	// Create current turn state if combat is active
	var currentTurn *dnd5ev1alpha1.TurnState
	if state.CombatStarted && !state.CombatEnded && len(state.TurnOrder) > 0 {
		activeEntry := state.TurnOrder[state.ActiveIndex]

		// Convert position from service layer
		var position *apiv1alpha1.Position
		if activeEntry.Position != nil {
			position = &apiv1alpha1.Position{
				X: activeEntry.Position.X,
				Y: activeEntry.Position.Y,
			}
		}

		movementUsed := int32(30) - state.MovementRemaining
		if movementUsed < 0 {
			movementUsed = 0
		}

		currentTurn = &dnd5ev1alpha1.TurnState{
			EntityId:          activeEntry.EntityID,
			MovementUsed:      movementUsed,
			MovementMax:       30, // Default movement - will be dynamic in Phase 4
			ActionUsed:        false,
			BonusActionUsed:   false,
			ReactionAvailable: true,
			Position:          position,
		}
	}

	return &dnd5ev1alpha1.CombatState{
		EncounterId:   state.EncounterID,
		Round:         int32(state.Round),
		TurnOrder:     turnOrder,
		ActiveIndex:   int32(state.ActiveIndex),
		CurrentTurn:   currentTurn,
		CombatStarted: state.CombatStarted,
		CombatEnded:   state.CombatEnded,
	}
}
