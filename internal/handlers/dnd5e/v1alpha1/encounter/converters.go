package encounter

import (
	apiv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/api/v1alpha1"
	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v1alpha1/character"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/encounter"
	"github.com/KirkDiggler/rpg-toolkit/core"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/combat"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

// stringToEntityType converts a string entity type to the proto enum
func stringToEntityType(s string) dnd5ev1alpha1.EntityType {
	switch s {
	case "character":
		return dnd5ev1alpha1.EntityType_ENTITY_TYPE_CHARACTER
	case "monster":
		return dnd5ev1alpha1.EntityType_ENTITY_TYPE_MONSTER
	case "obstacle":
		return dnd5ev1alpha1.EntityType_ENTITY_TYPE_OBSTACLE
	default:
		return dnd5ev1alpha1.EntityType_ENTITY_TYPE_UNSPECIFIED
	}
}

// intToEntitySize converts an int size to the proto enum
func intToEntitySize(size int) dnd5ev1alpha1.EntitySize {
	switch size {
	case 0:
		return dnd5ev1alpha1.EntitySize_ENTITY_SIZE_TINY
	case 1:
		return dnd5ev1alpha1.EntitySize_ENTITY_SIZE_SMALL
	case 2:
		return dnd5ev1alpha1.EntitySize_ENTITY_SIZE_MEDIUM
	case 3:
		return dnd5ev1alpha1.EntitySize_ENTITY_SIZE_LARGE
	case 4:
		return dnd5ev1alpha1.EntitySize_ENTITY_SIZE_HUGE
	case 5:
		return dnd5ev1alpha1.EntitySize_ENTITY_SIZE_GARGANTUAN
	default:
		// Default to MEDIUM for standard entities
		return dnd5ev1alpha1.EntitySize_ENTITY_SIZE_MEDIUM
	}
}

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
		DamageType:      string(result.DamageType), // damage.Type to proto string
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

	result := &dnd5ev1alpha1.DamageComponent{
		Source:            comp.Source,
		SourceRef:         convertCoreRefToProtoSourceRef(comp.SourceRef),
		OriginalDiceRolls: originalRolls,
		FinalDiceRolls:    finalRolls,
		Rerolls:           rerolls,
		FlatBonus:         int32(comp.FlatBonus),
		DamageType:        string(comp.DamageType), // damage.Type to proto string
		IsCritical:        comp.IsCritical,
	}

	// Set multiplier for monster trait components (vulnerability=2.0, resistance=0.5, immunity=0.0)
	// Check source type since immunity uses 0 which would otherwise be indistinguishable from "not set"
	if comp.Multiplier != 0 || comp.Source == "monster_trait" {
		mult := float32(comp.Multiplier)
		result.Multiplier = &mult
	}

	return result
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
// For hex grids, uses CubeEntities which already have cube coordinates (no conversion needed)
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

	// For hex grids, use CubeEntities (cube coordinates passed through directly)
	// For other grids, use Entities (offset coordinates)
	var entities map[string]*dnd5ev1alpha1.EntityPlacement

	if spatialRoom.GridType == spatial.GridTypeHex && len(spatialRoom.CubeEntities) > 0 {
		// Use CubeEntities for hex grids - coordinates are already cube, pass through directly
		entities = make(map[string]*dnd5ev1alpha1.EntityPlacement, len(spatialRoom.CubeEntities))
		for id, placement := range spatialRoom.CubeEntities {
			entities[id] = convertCubeEntityPlacementToProto(placement)
		}
	} else {
		// Use Entities for non-hex grids or legacy data
		hexOrientation := spatial.HexOrientationPointyTop
		if spatialRoom.HexFlatTop {
			hexOrientation = spatial.HexOrientationFlatTop
		}
		entities = make(map[string]*dnd5ev1alpha1.EntityPlacement, len(spatialRoom.Entities))
		for id, placement := range spatialRoom.Entities {
			entities[id] = convertEntityPlacementToProto(placement, spatialRoom.GridType, hexOrientation)
		}
	}

	// Convert HexFlatTop (toolkit) to hex_orientation (proto)
	// Toolkit: HexFlatTop=false means pointy-top, HexFlatTop=true means flat-top
	// Proto: hex_orientation=true means pointy-top, hex_orientation=false means flat-top
	var hexOrientationPtr *bool
	if spatialRoom.GridType == spatial.GridTypeHex {
		hexOrientationProto := !spatialRoom.HexFlatTop // Invert: pointy-top = true in proto
		hexOrientationPtr = &hexOrientationProto
	}

	return &dnd5ev1alpha1.Room{
		Id:             spatialRoom.ID,
		Type:           spatialRoom.Type,
		Width:          int32(spatialRoom.Width),
		Height:         int32(spatialRoom.Height),
		GridType:       gridType,
		HexOrientation: hexOrientationPtr,
		Entities:       entities,
	}
}

// convertCubeEntityPlacementToProto converts spatial.EntityCubePlacement to proto
// Cube coordinates are passed through directly without conversion
//
//nolint:gosec // G115: Game values are bounded, no overflow risk
func convertCubeEntityPlacementToProto(placement spatial.EntityCubePlacement) *dnd5ev1alpha1.EntityPlacement {
	return &dnd5ev1alpha1.EntityPlacement{
		EntityId:   placement.EntityID,
		EntityType: stringToEntityType(placement.EntityType),
		Position: &apiv1alpha1.Position{
			X: float64(placement.CubePosition.X),
			Y: float64(placement.CubePosition.Y),
			Z: float64(placement.CubePosition.Z),
		},
		Size:              intToEntitySize(placement.Size),
		BlocksMovement:    placement.BlocksMovement,
		BlocksLineOfSight: placement.BlocksLineOfSight,
	}
}

// convertEntityPlacementToProto converts spatial.EntityPlacement to proto
// For hex grids, positions are converted to cube coordinates (x, y, z)
//
//nolint:gosec // G115: Game values are bounded, no overflow risk
func convertEntityPlacementToProto(
	placement spatial.EntityPlacement,
	gridType string,
	hexOrientation spatial.HexOrientation,
) *dnd5ev1alpha1.EntityPlacement {
	position := convertPositionToProto(placement.Position, gridType, hexOrientation)

	return &dnd5ev1alpha1.EntityPlacement{
		EntityId:          placement.EntityID,
		EntityType:        stringToEntityType(placement.EntityType),
		Position:          position,
		Size:              intToEntitySize(placement.Size),
		BlocksMovement:    placement.BlocksMovement,
		BlocksLineOfSight: placement.BlocksLineOfSight,
	}
}

// convertPositionToProto converts a spatial.Position to proto Position
// For hex grids, converts offset coordinates to cube coordinates (x, y, z)
// For other grid types, uses offset coordinates (x, y, z=0)
func convertPositionToProto(
	pos spatial.Position,
	gridType string,
	hexOrientation spatial.HexOrientation,
) *apiv1alpha1.Position {
	if gridType == spatial.GridTypeHex {
		// Convert offset to cube coordinates for hex grids
		cube := spatial.OffsetCoordinateToCubeWithOrientation(pos, hexOrientation)
		return &apiv1alpha1.Position{
			X: float64(cube.X),
			Y: float64(cube.Y),
			Z: float64(cube.Z),
		}
	}

	// For square/gridless grids, use offset coordinates
	return &apiv1alpha1.Position{
		X: pos.X,
		Y: pos.Y,
		Z: 0,
	}
}

// convertProtoPositionToOffset converts a proto Position to spatial.Position (offset)
// For hex grids, converts cube coordinates (x, y, z) to offset coordinates
// For other grid types, uses the x, y values directly as offset coordinates
func convertProtoPositionToOffset(
	pos *apiv1alpha1.Position,
	gridType string,
	hexOrientation spatial.HexOrientation,
) spatial.Position {
	if pos == nil {
		return spatial.Position{X: 0, Y: 0}
	}

	if gridType == spatial.GridTypeHex {
		// Convert cube to offset coordinates for hex grids
		cube := spatial.CubeCoordinate{
			X: int(pos.X),
			Y: int(pos.Y),
			Z: int(pos.Z),
		}
		return cube.ToOffsetCoordinateWithOrientation(hexOrientation)
	}

	// For square/gridless grids, use x, y directly as offset
	return spatial.Position{X: pos.X, Y: pos.Y}
}

// extractGridInfo extracts grid type and hex orientation from room data
// Returns default values (hex, pointy-top) if room data is nil or invalid
func extractGridInfo(roomData interface{}) (string, spatial.HexOrientation) {
	if roomData == nil {
		return spatial.GridTypeHex, spatial.HexOrientationPointyTop
	}

	spatialRoom, ok := roomData.(*spatial.RoomData)
	if !ok {
		if spatialRoomVal, ok := roomData.(spatial.RoomData); ok {
			spatialRoom = &spatialRoomVal
		} else {
			return spatial.GridTypeHex, spatial.HexOrientationPointyTop
		}
	}

	hexOrientation := spatial.HexOrientationPointyTop
	if spatialRoom.HexFlatTop {
		hexOrientation = spatial.HexOrientationFlatTop
	}

	return spatialRoom.GridType, hexOrientation
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
// For hex grids, positions are converted to cube coordinates
//
//nolint:gosec // G115: Game values are bounded by D&D rules, no overflow risk
func convertCombatStateToProto(
	state *encounter.CombatState,
	gridType string,
	hexOrientation spatial.HexOrientation,
) *dnd5ev1alpha1.CombatState {
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

		// Convert position from service layer using cube coordinates for hex grids
		var position *apiv1alpha1.Position
		if activeEntry.Position != nil {
			position = convertPositionToProto(
				spatial.Position{X: activeEntry.Position.X, Y: activeEntry.Position.Y},
				gridType,
				hexOrientation,
			)
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

// convertTurnChangeToProto converts orchestrator's TurnChangeEvent to proto
//
//nolint:gosec // G115: Game values are bounded by D&D rules, no overflow risk
func convertTurnChangeToProto(event *encounter.TurnChangeEvent) *dnd5ev1alpha1.TurnChangeEvent {
	if event == nil {
		return nil
	}

	return &dnd5ev1alpha1.TurnChangeEvent{
		PreviousEntityId: event.PreviousEntityID,
		NextEntityId:     event.NextEntityID,
		Round:            int32(event.Round),
		NewRound:         event.NewRound,
	}
}

// convertCoreRefToProtoSourceRef converts a toolkit core.Ref to proto SourceRef.
// Uses pointer comparison with singleton refs for type-safe, IDE-discoverable conversions.
func convertCoreRefToProtoSourceRef(ref *core.Ref) *dnd5ev1alpha1.SourceRef {
	if ref == nil {
		return nil
	}

	switch ref.Type {
	case refs.TypeAbilities:
		return &dnd5ev1alpha1.SourceRef{
			Source: &dnd5ev1alpha1.SourceRef_Ability{
				Ability: abilityRefToProto(ref),
			},
		}
	case refs.TypeWeapons:
		return &dnd5ev1alpha1.SourceRef{
			Source: &dnd5ev1alpha1.SourceRef_Weapon{
				Weapon: weaponRefToProto(ref),
			},
		}
	case refs.TypeConditions:
		return &dnd5ev1alpha1.SourceRef{
			Source: &dnd5ev1alpha1.SourceRef_Condition{
				Condition: conditionRefToProto(ref),
			},
		}
	case refs.TypeFeatures:
		return &dnd5ev1alpha1.SourceRef{
			Source: &dnd5ev1alpha1.SourceRef_Feature{
				Feature: featureRefToProto(ref),
			},
		}
	// Note: refs.TypeFightingStyles was removed in toolkit PR #485
	// Fighting styles are now conditions (refs.TypeConditions) and handled above
	default:
		// Unknown type - return nil to indicate no mapping available
		return nil
	}
}

// abilityRefToProto converts toolkit ability ref to proto Ability enum using pointer comparison.
// Falls back to string comparison for unknown refs (e.g., homebrew content).
func abilityRefToProto(ref *core.Ref) dnd5ev1alpha1.Ability {
	// Fast path: pointer comparison with singleton refs
	switch ref {
	case refs.Abilities.Strength():
		return dnd5ev1alpha1.Ability_ABILITY_STRENGTH
	case refs.Abilities.Dexterity():
		return dnd5ev1alpha1.Ability_ABILITY_DEXTERITY
	case refs.Abilities.Constitution():
		return dnd5ev1alpha1.Ability_ABILITY_CONSTITUTION
	case refs.Abilities.Intelligence():
		return dnd5ev1alpha1.Ability_ABILITY_INTELLIGENCE
	case refs.Abilities.Wisdom():
		return dnd5ev1alpha1.Ability_ABILITY_WISDOM
	case refs.Abilities.Charisma():
		return dnd5ev1alpha1.Ability_ABILITY_CHARISMA
	}

	// Slow path: string comparison for unknown refs
	switch abilities.Ability(ref.ID) {
	case abilities.STR:
		return dnd5ev1alpha1.Ability_ABILITY_STRENGTH
	case abilities.DEX:
		return dnd5ev1alpha1.Ability_ABILITY_DEXTERITY
	case abilities.CON:
		return dnd5ev1alpha1.Ability_ABILITY_CONSTITUTION
	case abilities.INT:
		return dnd5ev1alpha1.Ability_ABILITY_INTELLIGENCE
	case abilities.WIS:
		return dnd5ev1alpha1.Ability_ABILITY_WISDOM
	case abilities.CHA:
		return dnd5ev1alpha1.Ability_ABILITY_CHARISMA
	default:
		return dnd5ev1alpha1.Ability_ABILITY_UNSPECIFIED
	}
}

// weaponRefToProto converts toolkit weapon ref to proto Weapon enum using pointer comparison.
// Falls back to string comparison for unknown refs (e.g., homebrew content).
//
//nolint:gocyclo // Exhaustive weapon mapping requires many cases
func weaponRefToProto(ref *core.Ref) dnd5ev1alpha1.Weapon {
	// Fast path: pointer comparison with singleton refs
	switch ref {
	// Simple Melee Weapons
	case refs.Weapons.Club():
		return dnd5ev1alpha1.Weapon_WEAPON_CLUB
	case refs.Weapons.Dagger():
		return dnd5ev1alpha1.Weapon_WEAPON_DAGGER
	case refs.Weapons.Greatclub():
		return dnd5ev1alpha1.Weapon_WEAPON_GREATCLUB
	case refs.Weapons.Handaxe():
		return dnd5ev1alpha1.Weapon_WEAPON_HANDAXE
	case refs.Weapons.Javelin():
		return dnd5ev1alpha1.Weapon_WEAPON_JAVELIN
	case refs.Weapons.LightHammer():
		return dnd5ev1alpha1.Weapon_WEAPON_LIGHT_HAMMER
	case refs.Weapons.Mace():
		return dnd5ev1alpha1.Weapon_WEAPON_MACE
	case refs.Weapons.Quarterstaff():
		return dnd5ev1alpha1.Weapon_WEAPON_QUARTERSTAFF
	case refs.Weapons.Sickle():
		return dnd5ev1alpha1.Weapon_WEAPON_SICKLE
	case refs.Weapons.Spear():
		return dnd5ev1alpha1.Weapon_WEAPON_SPEAR
	// Simple Ranged Weapons
	case refs.Weapons.LightCrossbow():
		return dnd5ev1alpha1.Weapon_WEAPON_LIGHT_CROSSBOW
	case refs.Weapons.Dart():
		return dnd5ev1alpha1.Weapon_WEAPON_DART
	case refs.Weapons.Shortbow():
		return dnd5ev1alpha1.Weapon_WEAPON_SHORTBOW
	case refs.Weapons.Sling():
		return dnd5ev1alpha1.Weapon_WEAPON_SLING
	// Martial Melee Weapons
	case refs.Weapons.Battleaxe():
		return dnd5ev1alpha1.Weapon_WEAPON_BATTLEAXE
	case refs.Weapons.Flail():
		return dnd5ev1alpha1.Weapon_WEAPON_FLAIL
	case refs.Weapons.Glaive():
		return dnd5ev1alpha1.Weapon_WEAPON_GLAIVE
	case refs.Weapons.Greataxe():
		return dnd5ev1alpha1.Weapon_WEAPON_GREATAXE
	case refs.Weapons.Greatsword():
		return dnd5ev1alpha1.Weapon_WEAPON_GREATSWORD
	case refs.Weapons.Halberd():
		return dnd5ev1alpha1.Weapon_WEAPON_HALBERD
	case refs.Weapons.Lance():
		return dnd5ev1alpha1.Weapon_WEAPON_LANCE
	case refs.Weapons.Longsword():
		return dnd5ev1alpha1.Weapon_WEAPON_LONGSWORD
	case refs.Weapons.Maul():
		return dnd5ev1alpha1.Weapon_WEAPON_MAUL
	case refs.Weapons.Morningstar():
		return dnd5ev1alpha1.Weapon_WEAPON_MORNINGSTAR
	case refs.Weapons.Pike():
		return dnd5ev1alpha1.Weapon_WEAPON_PIKE
	case refs.Weapons.Rapier():
		return dnd5ev1alpha1.Weapon_WEAPON_RAPIER
	case refs.Weapons.Scimitar():
		return dnd5ev1alpha1.Weapon_WEAPON_SCIMITAR
	case refs.Weapons.Shortsword():
		return dnd5ev1alpha1.Weapon_WEAPON_SHORTSWORD
	case refs.Weapons.Trident():
		return dnd5ev1alpha1.Weapon_WEAPON_TRIDENT
	case refs.Weapons.WarPick():
		return dnd5ev1alpha1.Weapon_WEAPON_WAR_PICK
	case refs.Weapons.Warhammer():
		return dnd5ev1alpha1.Weapon_WEAPON_WARHAMMER
	case refs.Weapons.Whip():
		return dnd5ev1alpha1.Weapon_WEAPON_WHIP
	// Martial Ranged Weapons
	case refs.Weapons.Blowgun():
		return dnd5ev1alpha1.Weapon_WEAPON_BLOWGUN
	case refs.Weapons.HandCrossbow():
		return dnd5ev1alpha1.Weapon_WEAPON_HAND_CROSSBOW
	case refs.Weapons.HeavyCrossbow():
		return dnd5ev1alpha1.Weapon_WEAPON_HEAVY_CROSSBOW
	case refs.Weapons.Longbow():
		return dnd5ev1alpha1.Weapon_WEAPON_LONGBOW
	case refs.Weapons.Net():
		return dnd5ev1alpha1.Weapon_WEAPON_NET
	}

	// Slow path: string comparison for unknown refs
	switch ref.ID {
	case "club":
		return dnd5ev1alpha1.Weapon_WEAPON_CLUB
	case "dagger":
		return dnd5ev1alpha1.Weapon_WEAPON_DAGGER
	case "greatclub":
		return dnd5ev1alpha1.Weapon_WEAPON_GREATCLUB
	case "handaxe":
		return dnd5ev1alpha1.Weapon_WEAPON_HANDAXE
	case "javelin":
		return dnd5ev1alpha1.Weapon_WEAPON_JAVELIN
	case "light-hammer":
		return dnd5ev1alpha1.Weapon_WEAPON_LIGHT_HAMMER
	case "mace":
		return dnd5ev1alpha1.Weapon_WEAPON_MACE
	case "quarterstaff":
		return dnd5ev1alpha1.Weapon_WEAPON_QUARTERSTAFF
	case "sickle":
		return dnd5ev1alpha1.Weapon_WEAPON_SICKLE
	case "spear":
		return dnd5ev1alpha1.Weapon_WEAPON_SPEAR
	case "light-crossbow":
		return dnd5ev1alpha1.Weapon_WEAPON_LIGHT_CROSSBOW
	case "dart":
		return dnd5ev1alpha1.Weapon_WEAPON_DART
	case "shortbow":
		return dnd5ev1alpha1.Weapon_WEAPON_SHORTBOW
	case "sling":
		return dnd5ev1alpha1.Weapon_WEAPON_SLING
	case "battleaxe":
		return dnd5ev1alpha1.Weapon_WEAPON_BATTLEAXE
	case "flail":
		return dnd5ev1alpha1.Weapon_WEAPON_FLAIL
	case "glaive":
		return dnd5ev1alpha1.Weapon_WEAPON_GLAIVE
	case "greataxe":
		return dnd5ev1alpha1.Weapon_WEAPON_GREATAXE
	case "greatsword":
		return dnd5ev1alpha1.Weapon_WEAPON_GREATSWORD
	case "halberd":
		return dnd5ev1alpha1.Weapon_WEAPON_HALBERD
	case "lance":
		return dnd5ev1alpha1.Weapon_WEAPON_LANCE
	case "longsword":
		return dnd5ev1alpha1.Weapon_WEAPON_LONGSWORD
	case "maul":
		return dnd5ev1alpha1.Weapon_WEAPON_MAUL
	case "morningstar":
		return dnd5ev1alpha1.Weapon_WEAPON_MORNINGSTAR
	case "pike":
		return dnd5ev1alpha1.Weapon_WEAPON_PIKE
	case "rapier":
		return dnd5ev1alpha1.Weapon_WEAPON_RAPIER
	case "scimitar":
		return dnd5ev1alpha1.Weapon_WEAPON_SCIMITAR
	case "shortsword":
		return dnd5ev1alpha1.Weapon_WEAPON_SHORTSWORD
	case "trident":
		return dnd5ev1alpha1.Weapon_WEAPON_TRIDENT
	case "war-pick":
		return dnd5ev1alpha1.Weapon_WEAPON_WAR_PICK
	case "warhammer":
		return dnd5ev1alpha1.Weapon_WEAPON_WARHAMMER
	case "whip":
		return dnd5ev1alpha1.Weapon_WEAPON_WHIP
	case "blowgun":
		return dnd5ev1alpha1.Weapon_WEAPON_BLOWGUN
	case "hand-crossbow":
		return dnd5ev1alpha1.Weapon_WEAPON_HAND_CROSSBOW
	case "heavy-crossbow":
		return dnd5ev1alpha1.Weapon_WEAPON_HEAVY_CROSSBOW
	case "longbow":
		return dnd5ev1alpha1.Weapon_WEAPON_LONGBOW
	case "net":
		return dnd5ev1alpha1.Weapon_WEAPON_NET
	default:
		return dnd5ev1alpha1.Weapon_WEAPON_UNSPECIFIED
	}
}

// conditionRefToProto converts toolkit condition ref to proto ConditionId enum using pointer comparison.
// Falls back to string comparison for unknown refs.
// Also handles fighting style conditions (PR #485 merged them into refs.Conditions).
func conditionRefToProto(ref *core.Ref) dnd5ev1alpha1.ConditionId {
	// Fast path: pointer comparison with singleton refs
	switch ref {
	case refs.Conditions.Raging():
		return dnd5ev1alpha1.ConditionId_CONDITION_ID_RAGING
	case refs.Conditions.BrutalCritical():
		return dnd5ev1alpha1.ConditionId_CONDITION_ID_BRUTAL_CRITICAL
	case refs.Conditions.SneakAttack():
		return dnd5ev1alpha1.ConditionId_CONDITION_ID_SNEAK_ATTACK
	case refs.Conditions.FightingStyleDueling():
		return dnd5ev1alpha1.ConditionId_CONDITION_ID_FIGHTING_STYLE_DUELING
	case refs.Conditions.FightingStyleTwoWeaponFighting():
		return dnd5ev1alpha1.ConditionId_CONDITION_ID_FIGHTING_STYLE_TWO_WEAPON_FIGHTING
	case refs.Conditions.FightingStyleGreatWeaponFighting():
		return dnd5ev1alpha1.ConditionId_CONDITION_ID_FIGHTING_STYLE_GREAT_WEAPON_FIGHTING
	case refs.Features.DivineSmite():
		return dnd5ev1alpha1.ConditionId_CONDITION_ID_DIVINE_SMITE
	}

	// Slow path: string comparison for unknown refs (e.g., homebrew)
	switch ref.ID {
	case refs.Conditions.Raging().ID:
		return dnd5ev1alpha1.ConditionId_CONDITION_ID_RAGING
	case refs.Conditions.BrutalCritical().ID:
		return dnd5ev1alpha1.ConditionId_CONDITION_ID_BRUTAL_CRITICAL
	case refs.Conditions.SneakAttack().ID:
		return dnd5ev1alpha1.ConditionId_CONDITION_ID_SNEAK_ATTACK
	case refs.Features.DivineSmite().ID:
		return dnd5ev1alpha1.ConditionId_CONDITION_ID_DIVINE_SMITE
	case refs.Conditions.FightingStyleDueling().ID:
		return dnd5ev1alpha1.ConditionId_CONDITION_ID_FIGHTING_STYLE_DUELING
	case refs.Conditions.FightingStyleTwoWeaponFighting().ID:
		return dnd5ev1alpha1.ConditionId_CONDITION_ID_FIGHTING_STYLE_TWO_WEAPON_FIGHTING
	case refs.Conditions.FightingStyleGreatWeaponFighting().ID:
		return dnd5ev1alpha1.ConditionId_CONDITION_ID_FIGHTING_STYLE_GREAT_WEAPON_FIGHTING
	default:
		return dnd5ev1alpha1.ConditionId_CONDITION_ID_UNSPECIFIED
	}
}

// featureRefToProto converts toolkit feature ref to proto FeatureId enum.
// Uses toolkit refs for type-safe comparison where available.
func featureRefToProto(ref *core.Ref) dnd5ev1alpha1.FeatureId {
	switch ref.ID {
	// Barbarian features
	case refs.Features.Rage().ID:
		return dnd5ev1alpha1.FeatureId_FEATURE_ID_RAGE
	// Fighter features
	case refs.Features.SecondWind().ID:
		return dnd5ev1alpha1.FeatureId_FEATURE_ID_SECOND_WIND
	case refs.Features.ActionSurge().ID:
		return dnd5ev1alpha1.FeatureId_FEATURE_ID_ACTION_SURGE
	// Rogue features
	case refs.Features.SneakAttack().ID:
		return dnd5ev1alpha1.FeatureId_FEATURE_ID_SNEAK_ATTACK
	// Paladin features
	case refs.Features.DivineSmite().ID:
		return dnd5ev1alpha1.FeatureId_FEATURE_ID_DIVINE_SMITE
	// Monk features
	case refs.Features.FlurryOfBlows().ID:
		return dnd5ev1alpha1.FeatureId_FEATURE_ID_FLURRY_OF_BLOWS
	case refs.Features.PatientDefense().ID:
		return dnd5ev1alpha1.FeatureId_FEATURE_ID_PATIENT_DEFENSE
	case refs.Features.StepOfTheWind().ID:
		return dnd5ev1alpha1.FeatureId_FEATURE_ID_STEP_OF_THE_WIND
	case refs.Features.DeflectMissiles().ID:
		return dnd5ev1alpha1.FeatureId_FEATURE_ID_DEFLECT_MISSILES
	// Features below don't have toolkit refs yet - magic strings until toolkit adds them
	case "breath_weapon":
		return dnd5ev1alpha1.FeatureId_FEATURE_ID_BREATH_WEAPON
	case "hellish_rebuke":
		return dnd5ev1alpha1.FeatureId_FEATURE_ID_HELLISH_REBUKE
	case "radiance_of_dawn":
		return dnd5ev1alpha1.FeatureId_FEATURE_ID_RADIANCE_OF_DAWN
	case "wrath_of_the_storm":
		return dnd5ev1alpha1.FeatureId_FEATURE_ID_WRATH_OF_THE_STORM
	case "destructive_wrath":
		return dnd5ev1alpha1.FeatureId_FEATURE_ID_DESTRUCTIVE_WRATH
	case "starry_form_archer":
		return dnd5ev1alpha1.FeatureId_FEATURE_ID_STARRY_FORM_ARCHER
	default:
		return dnd5ev1alpha1.FeatureId_FEATURE_ID_UNSPECIFIED
	}
}

// featureEnumToID converts a proto FeatureId enum to the toolkit feature ID string.
// This is the reverse of featureRefToProto.
func featureEnumToID(featureID dnd5ev1alpha1.FeatureId) string {
	switch featureID {
	// Barbarian features
	case dnd5ev1alpha1.FeatureId_FEATURE_ID_RAGE:
		return refs.Features.Rage().ID
	// Fighter features
	case dnd5ev1alpha1.FeatureId_FEATURE_ID_SECOND_WIND:
		return refs.Features.SecondWind().ID
	case dnd5ev1alpha1.FeatureId_FEATURE_ID_ACTION_SURGE:
		return refs.Features.ActionSurge().ID
	// Rogue features
	case dnd5ev1alpha1.FeatureId_FEATURE_ID_SNEAK_ATTACK:
		return refs.Features.SneakAttack().ID
	// Paladin features
	case dnd5ev1alpha1.FeatureId_FEATURE_ID_DIVINE_SMITE:
		return refs.Features.DivineSmite().ID
	// Monk features
	case dnd5ev1alpha1.FeatureId_FEATURE_ID_FLURRY_OF_BLOWS:
		return refs.Features.FlurryOfBlows().ID
	case dnd5ev1alpha1.FeatureId_FEATURE_ID_PATIENT_DEFENSE:
		return refs.Features.PatientDefense().ID
	case dnd5ev1alpha1.FeatureId_FEATURE_ID_STEP_OF_THE_WIND:
		return refs.Features.StepOfTheWind().ID
	case dnd5ev1alpha1.FeatureId_FEATURE_ID_DEFLECT_MISSILES:
		return refs.Features.DeflectMissiles().ID
	// Features without toolkit refs
	case dnd5ev1alpha1.FeatureId_FEATURE_ID_BREATH_WEAPON:
		return "breath_weapon"
	case dnd5ev1alpha1.FeatureId_FEATURE_ID_HELLISH_REBUKE:
		return "hellish_rebuke"
	case dnd5ev1alpha1.FeatureId_FEATURE_ID_RADIANCE_OF_DAWN:
		return "radiance_of_dawn"
	case dnd5ev1alpha1.FeatureId_FEATURE_ID_WRATH_OF_THE_STORM:
		return "wrath_of_the_storm"
	case dnd5ev1alpha1.FeatureId_FEATURE_ID_DESTRUCTIVE_WRATH:
		return "destructive_wrath"
	case dnd5ev1alpha1.FeatureId_FEATURE_ID_STARRY_FORM_ARCHER:
		return "starry_form_archer"
	default:
		return ""
	}
}

// fightingStyleRefToProto is deprecated - fighting styles are now conditions.
// Use conditionRefToProto instead. This function is kept for reference.
// Note: Removed in toolkit PR #485 - refs.FightingStyles no longer exists.
// Fighting styles are now at refs.Conditions.FightingStyle*().

// convertMonsterTurnResultToProto converts orchestrator's MonsterTurnResult to proto
// For hex grids, movement positions are converted to cube coordinates
func convertMonsterTurnResultToProto(
	result *encounter.MonsterTurnResult,
	gridType string,
	hexOrientation spatial.HexOrientation,
) *dnd5ev1alpha1.MonsterTurnResult {
	if result == nil {
		return nil
	}

	// Convert actions
	actions := make([]*dnd5ev1alpha1.MonsterExecutedAction, len(result.Actions))
	for i, action := range result.Actions {
		actions[i] = convertMonsterExecutedActionToProto(&action)
	}

	// Convert movement path using cube coordinates for hex grids
	movementPath := make([]*apiv1alpha1.Position, len(result.Movement))
	for i, pos := range result.Movement {
		// Convert encounter.Position to spatial.Position for cube conversion
		spatialPos := spatial.Position{X: pos.X, Y: pos.Y}
		movementPath[i] = convertPositionToProto(spatialPos, gridType, hexOrientation)
	}

	return &dnd5ev1alpha1.MonsterTurnResult{
		MonsterId:    result.MonsterID,
		MonsterName:  result.MonsterName,
		Actions:      actions,
		MovementPath: movementPath,
	}
}

// convertMonsterTurnsToProto converts a slice of MonsterTurnResult to proto
// For hex grids, movement positions are converted to cube coordinates
func convertMonsterTurnsToProto(
	results []*encounter.MonsterTurnResult,
	gridType string,
	hexOrientation spatial.HexOrientation,
) []*dnd5ev1alpha1.MonsterTurnResult {
	if results == nil {
		return nil
	}

	protoResults := make([]*dnd5ev1alpha1.MonsterTurnResult, len(results))
	for i, result := range results {
		protoResults[i] = convertMonsterTurnResultToProto(result, gridType, hexOrientation)
	}

	return protoResults
}

// convertMonsterExecutedActionToProto converts orchestrator's MonsterExecutedAction to proto
func convertMonsterExecutedActionToProto(action *encounter.MonsterExecutedAction) *dnd5ev1alpha1.MonsterExecutedAction {
	if action == nil {
		return nil
	}

	protoAction := &dnd5ev1alpha1.MonsterExecutedAction{
		ActionId:   action.ActionID,
		ActionType: convertMonsterActionTypeToProto(action.ActionType),
		TargetId:   action.TargetID,
		Success:    action.Success,
	}

	// Convert Details from typed struct (survives JSON serialization)
	if action.Details != nil {
		if action.Details.AttackResult != nil {
			protoAction.Details = &dnd5ev1alpha1.MonsterExecutedAction_AttackResult{
				AttackResult: convertAttackResultToProto(action.Details.AttackResult),
			}
		} else if action.Details.HealResult != nil {
			protoAction.Details = &dnd5ev1alpha1.MonsterExecutedAction_HealResult{
				HealResult: convertHealResultToProto(action.Details.HealResult),
			}
		}
	}

	return protoAction
}

// convertMonsterActionTypeToProto converts toolkit monster.ActionType to proto enum
func convertMonsterActionTypeToProto(actionType string) dnd5ev1alpha1.MonsterActionType {
	switch monster.ActionType(actionType) {
	case monster.TypeMeleeAttack:
		return dnd5ev1alpha1.MonsterActionType_MONSTER_ACTION_TYPE_MELEE_ATTACK
	case monster.TypeRangedAttack:
		return dnd5ev1alpha1.MonsterActionType_MONSTER_ACTION_TYPE_RANGED_ATTACK
	case monster.TypeSpell:
		return dnd5ev1alpha1.MonsterActionType_MONSTER_ACTION_TYPE_SPELL
	case monster.TypeHeal:
		return dnd5ev1alpha1.MonsterActionType_MONSTER_ACTION_TYPE_HEAL
	case monster.TypeMovement:
		return dnd5ev1alpha1.MonsterActionType_MONSTER_ACTION_TYPE_MOVEMENT
	case monster.TypeStealth:
		return dnd5ev1alpha1.MonsterActionType_MONSTER_ACTION_TYPE_STEALTH
	case monster.TypeDefend:
		return dnd5ev1alpha1.MonsterActionType_MONSTER_ACTION_TYPE_DEFEND
	default:
		return dnd5ev1alpha1.MonsterActionType_MONSTER_ACTION_TYPE_UNSPECIFIED
	}
}

// convertEncounterResultToProto converts an EncounterResult to proto
func convertEncounterResultToProto(result *encounter.EncounterResult) *dnd5ev1alpha1.EncounterResult {
	if result == nil {
		return nil
	}

	// Convert reason string to enum
	var protoReason dnd5ev1alpha1.EncounterEndReason
	switch result.Reason {
	case "victory":
		protoReason = dnd5ev1alpha1.EncounterEndReason_ENCOUNTER_END_REASON_VICTORY
	case "defeat":
		protoReason = dnd5ev1alpha1.EncounterEndReason_ENCOUNTER_END_REASON_DEFEAT
	default:
		protoReason = dnd5ev1alpha1.EncounterEndReason_ENCOUNTER_END_REASON_UNSPECIFIED
	}

	return &dnd5ev1alpha1.EncounterResult{
		Reason: protoReason,
	}
}

// convertPartyToProto converts a slice of PartyMember to proto
func convertPartyToProto(party []*encounter.PartyMember) []*dnd5ev1alpha1.PartyMember {
	if party == nil {
		return nil
	}

	protoParty := make([]*dnd5ev1alpha1.PartyMember, len(party))
	for i, member := range party {
		protoParty[i] = &dnd5ev1alpha1.PartyMember{
			PlayerId:    member.PlayerID,
			IsHost:      member.IsHost,
			IsReady:     member.IsReady,
			IsConnected: member.IsConnected,
		}
		// Convert character data if present
		if member.CharacterData != nil {
			if charData, ok := member.CharacterData.(*toolkitchar.Data); ok {
				protoParty[i].Character = characterhandler.ConvertCharacterDataToProto(charData)
			}
		}
	}
	return protoParty
}

// convertEncounterStateToProto converts state string to proto enum
func convertEncounterStateToProto(state string) dnd5ev1alpha1.EncounterState {
	switch state {
	case "waiting":
		return dnd5ev1alpha1.EncounterState_ENCOUNTER_STATE_WAITING
	case "active":
		return dnd5ev1alpha1.EncounterState_ENCOUNTER_STATE_ACTIVE
	case "paused":
		return dnd5ev1alpha1.EncounterState_ENCOUNTER_STATE_PAUSED
	case "completed":
		return dnd5ev1alpha1.EncounterState_ENCOUNTER_STATE_COMPLETED
	default:
		return dnd5ev1alpha1.EncounterState_ENCOUNTER_STATE_UNSPECIFIED
	}
}

// convertOpenDoorRoomToProto converts the orchestrator's RoomData to proto Room
//
//nolint:gosec // G115: Game values are bounded, no overflow risk
func convertOpenDoorRoomToProto(roomData *encounter.RoomData) *dnd5ev1alpha1.Room {
	if roomData == nil {
		return nil
	}

	// Default to hex grid with pointy-top orientation
	hexOrientation := true

	// Convert entities
	entities := make(map[string]*dnd5ev1alpha1.EntityPlacement, len(roomData.Entities))
	for id, placement := range roomData.Entities {
		if placement == nil {
			continue
		}
		var position *apiv1alpha1.Position
		if placement.Position != nil {
			position = &apiv1alpha1.Position{
				X: placement.Position.X,
				Y: placement.Position.Y,
				Z: placement.Position.Z,
			}
		}
		entities[id] = &dnd5ev1alpha1.EntityPlacement{
			EntityId:          placement.EntityID,
			EntityType:        stringToEntityType(placement.EntityType),
			Position:          position,
			Size:              intToEntitySize(placement.Size),
			BlocksMovement:    placement.BlocksMovement,
			BlocksLineOfSight: placement.BlocksLineOfSight,
		}
	}

	return &dnd5ev1alpha1.Room{
		Id:             roomData.ID,
		Type:           "dungeon",
		Width:          int32(roomData.Width),
		Height:         int32(roomData.Height),
		GridType:       apiv1alpha1.GridType_GRID_TYPE_HEX_POINTY,
		HexOrientation: &hexOrientation,
		Entities:       entities,
		Walls:          convertWallsToProto(roomData.Walls),
	}
}

// convertDoorInfoSliceToProto converts a slice of DoorInfo to proto
func convertDoorInfoSliceToProto(doors []encounter.DoorInfo) []*dnd5ev1alpha1.DoorInfo {
	if doors == nil {
		return nil
	}

	result := make([]*dnd5ev1alpha1.DoorInfo, len(doors))
	for i, door := range doors {
		var position *apiv1alpha1.Position
		if door.Position != nil {
			position = &apiv1alpha1.Position{
				X: door.Position.X,
				Y: door.Position.Y,
				Z: door.Position.Z,
			}
		}

		result[i] = &dnd5ev1alpha1.DoorInfo{
			ConnectionId:  door.ConnectionID,
			Position:      position,
			PhysicalHint:  door.Direction,
			IsOpen:        door.IsOpen,
			LeadsToRoomId: door.TargetRoomID,
		}
	}
	return result
}

// protoAttackHandToToolkit converts proto AttackHand enum to toolkit combat.AttackHand
func protoAttackHandToToolkit(hand dnd5ev1alpha1.AttackHand) combat.AttackHand {
	switch hand {
	case dnd5ev1alpha1.AttackHand_ATTACK_HAND_OFF:
		return combat.AttackHandOff
	case dnd5ev1alpha1.AttackHand_ATTACK_HAND_MAIN:
		return combat.AttackHandMain
	default:
		// UNSPECIFIED defaults to main hand behavior
		return combat.AttackHandMain
	}
}

// protoDungeonThemeToString converts proto DungeonTheme enum to theme ID string
func protoDungeonThemeToString(theme dnd5ev1alpha1.DungeonTheme) string {
	switch theme {
	case dnd5ev1alpha1.DungeonTheme_DUNGEON_THEME_CRYPT:
		return "crypt"
	case dnd5ev1alpha1.DungeonTheme_DUNGEON_THEME_CAVE:
		return "cave"
	case dnd5ev1alpha1.DungeonTheme_DUNGEON_THEME_RUINS:
		return "bandit-lair" // RUINS not yet implemented, use bandit-lair
	default:
		// UNSPECIFIED defaults to crypt
		return ""
	}
}

// protoDungeonDifficultyToString converts proto DungeonDifficulty enum to difficulty string
func protoDungeonDifficultyToString(difficulty dnd5ev1alpha1.DungeonDifficulty) string {
	switch difficulty {
	case dnd5ev1alpha1.DungeonDifficulty_DUNGEON_DIFFICULTY_EASY:
		return "easy"
	case dnd5ev1alpha1.DungeonDifficulty_DUNGEON_DIFFICULTY_MEDIUM:
		return "medium"
	case dnd5ev1alpha1.DungeonDifficulty_DUNGEON_DIFFICULTY_HARD:
		return "hard"
	default:
		// UNSPECIFIED defaults to easy
		return ""
	}
}

// protoDungeonLengthToString converts proto DungeonLength enum to length string
func protoDungeonLengthToString(length dnd5ev1alpha1.DungeonLength) string {
	switch length {
	case dnd5ev1alpha1.DungeonLength_DUNGEON_LENGTH_SHORT:
		return "short"
	case dnd5ev1alpha1.DungeonLength_DUNGEON_LENGTH_MEDIUM:
		return "medium"
	case dnd5ev1alpha1.DungeonLength_DUNGEON_LENGTH_LONG:
		return "long"
	default:
		// UNSPECIFIED defaults to short
		return ""
	}
}

// convertEntityDoorsToProto converts entities.DoorInfo to proto DoorInfo
func convertEntityDoorsToProto(doors []*entities.DoorInfo) []*dnd5ev1alpha1.DoorInfo {
	if doors == nil {
		return nil
	}
	result := make([]*dnd5ev1alpha1.DoorInfo, len(doors))
	for i, d := range doors {
		var pos *apiv1alpha1.Position
		if d.Position != nil {
			pos = &apiv1alpha1.Position{
				X: d.Position.X,
				Y: d.Position.Y,
				Z: d.Position.Z,
			}
		}
		result[i] = &dnd5ev1alpha1.DoorInfo{
			ConnectionId:  d.ConnectionID,
			LeadsToRoomId: d.TargetRoomID,
			PhysicalHint:  d.Direction,
			Position:      pos,
			IsOpen:        d.IsOpen,
		}
	}
	return result
}

// convertEntityMonsterTurnsToProto converts entity-based MonsterTurnCompletedEvent to proto
// Note: Proto MonsterTurnResult only has MonsterId, MonsterName, Actions, MovementPath
// Room and UpdatedCharacters are in the separate streaming event, not the embedded result
func convertEntityMonsterTurnsToProto(
	turns []*entities.MonsterTurnCompletedEvent,
	_ string, // gridType - unused but kept for interface consistency
	_ spatial.HexOrientation, // hexOrientation - unused but kept for interface consistency
) []*dnd5ev1alpha1.MonsterTurnResult {
	if turns == nil {
		return nil
	}
	result := make([]*dnd5ev1alpha1.MonsterTurnResult, len(turns))
	for i, t := range turns {
		// Convert actions
		var protoActions []*dnd5ev1alpha1.MonsterExecutedAction
		for _, a := range t.Actions {
			protoActions = append(protoActions, convertEntityMonsterActionToProto(&a))
		}

		// Convert movement positions
		var protoMovement []*apiv1alpha1.Position
		for _, pos := range t.Movement {
			protoMovement = append(protoMovement, &apiv1alpha1.Position{
				X: pos.X,
				Y: pos.Y,
				Z: pos.Z,
			})
		}

		result[i] = &dnd5ev1alpha1.MonsterTurnResult{
			MonsterId:    t.MonsterID,
			MonsterName:  t.MonsterName,
			Actions:      protoActions,
			MovementPath: protoMovement,
		}
	}
	return result
}

// convertEntityMonsterActionToProto converts entity MonsterExecutedAction to proto
func convertEntityMonsterActionToProto(action *entities.MonsterExecutedAction) *dnd5ev1alpha1.MonsterExecutedAction {
	if action == nil {
		return nil
	}
	protoAction := &dnd5ev1alpha1.MonsterExecutedAction{
		ActionId:   action.ActionID,
		ActionType: convertMonsterActionTypeToProto(action.ActionType),
		TargetId:   action.TargetID,
		Success:    action.Success,
	}

	// Convert Details from typed struct (survives JSON serialization)
	if action.Details != nil {
		if action.Details.AttackResult != nil {
			protoAction.Details = &dnd5ev1alpha1.MonsterExecutedAction_AttackResult{
				AttackResult: convertAttackResultToProto(action.Details.AttackResult),
			}
		} else if action.Details.HealResult != nil {
			protoAction.Details = &dnd5ev1alpha1.MonsterExecutedAction_HealResult{
				HealResult: convertHealResultToProto(action.Details.HealResult),
			}
		}
	}

	return protoAction
}

// convertHealResultToProto converts entities.HealResult to proto
//
//nolint:gosec // G115: Game values are bounded by D&D rules, no overflow risk
func convertHealResultToProto(result *entities.HealResult) *dnd5ev1alpha1.HealResult {
	if result == nil {
		return nil
	}
	return &dnd5ev1alpha1.HealResult{
		AmountHealed: int32(result.AmountHealed),
		NewHp:        int32(result.NewHP),
		MaxHp:        int32(result.MaxHP),
	}
}

// convertWallsToProto converts orchestrator WallInfo to proto Wall
func convertWallsToProto(walls []encounter.WallInfo) []*apiv1alpha1.Wall {
	if walls == nil {
		return nil
	}

	result := make([]*apiv1alpha1.Wall, len(walls))
	for i, w := range walls {
		var start, end *apiv1alpha1.Position
		if w.Start != nil {
			start = &apiv1alpha1.Position{
				X: w.Start.X,
				Y: w.Start.Y,
				Z: w.Start.Z,
			}
		}
		if w.End != nil {
			end = &apiv1alpha1.Position{
				X: w.End.X,
				Y: w.End.Y,
				Z: w.End.Z,
			}
		}

		result[i] = &apiv1alpha1.Wall{
			Start:             start,
			End:               end,
			Material:          w.Material,
			BlocksMovement:    w.BlocksMovement,
			BlocksLineOfSight: w.BlocksLineOfSight,
		}
	}

	return result
}

// convertDungeonWallsToProto converts dungeon.WallSegment slices to proto Walls
// Used for event data that comes directly from dungeon entities rather than orchestrator types
func convertDungeonWallsToProto(walls []dungeon.WallSegment) []*apiv1alpha1.Wall {
	if walls == nil {
		return nil
	}

	result := make([]*apiv1alpha1.Wall, len(walls))
	for i, w := range walls {
		start := &apiv1alpha1.Position{
			X: float64(w.Start.X),
			Y: float64(w.Start.Y),
			Z: float64(w.Start.Z),
		}
		end := &apiv1alpha1.Position{
			X: float64(w.End.X),
			Y: float64(w.End.Y),
			Z: float64(w.End.Z),
		}

		// Convert WallType to material string
		var material string
		switch w.Type {
		case dungeon.WallTypeIndestructible:
			material = "indestructible"
		case dungeon.WallTypeDestructible:
			material = "destructible"
		default:
			material = "unknown"
		}

		result[i] = &apiv1alpha1.Wall{
			Start:             start,
			End:               end,
			Material:          material,
			BlocksMovement:    w.BlocksMovement,
			BlocksLineOfSight: w.BlocksLineOfSight,
		}
	}

	return result
}

// convertActionEconomyToProto converts entities.ActionEconomyState to proto ActionEconomy
// Movement values come from CombatState since ActionEconomyState doesn't track movement
//
//nolint:gosec // G115: Game values are bounded by D&D rules, no overflow risk
func convertActionEconomyToProto(ae *entities.ActionEconomyState, cs *entities.CombatState) *dnd5ev1alpha1.ActionEconomy {
	if ae == nil {
		return nil
	}

	// Get movement values from CombatState
	var movementRemaining, movementMax int32
	if cs != nil {
		movementRemaining = cs.MovementRemaining
		movementMax = 30 // Default movement - dynamic values coming in Phase 4
	}

	return &dnd5ev1alpha1.ActionEconomy{
		MovementRemaining:       movementRemaining,
		MovementMax:             movementMax,
		AttacksRemaining:        int32(ae.AttacksRemaining),
		StandardActionAvailable: ae.ActionsRemaining > 0,
		BonusActionAvailable:    ae.BonusActionsRemaining > 0,
		ReactionAvailable:       ae.ReactionsRemaining > 0,
		OffHandAttacksRemaining: int32(ae.OffHandAttacksRemaining),
		FlurryStrikesRemaining:  int32(ae.FlurryStrikesRemaining),
		DisengageActive:         ae.DisengageActive,
		DodgeActive:             ae.DodgeActive,
	}
}

// convertMoveResultToProto converts encounter.MoveResult to proto MoveResult
//
//nolint:gosec // G115: Game values are bounded by D&D rules, no overflow risk
func convertMoveResultToProto(mr *encounter.MoveResult) *dnd5ev1alpha1.MoveResult {
	if mr == nil {
		return nil
	}

	var finalPosition *apiv1alpha1.Position
	if mr.FinalPosition != nil {
		finalPosition = &apiv1alpha1.Position{
			X: mr.FinalPosition.X,
			Y: mr.FinalPosition.Y,
			Z: mr.FinalPosition.Z,
		}
	}

	return &dnd5ev1alpha1.MoveResult{
		FinalPosition: finalPosition,
		MovementUsed:  int32(mr.MovementUsed),
		StopReason:    mr.StopReason,
	}
}

// convertGrantedActionToProto converts encounter.GrantedAction to proto GrantedAction
func convertGrantedActionToProto(ga *encounter.GrantedAction) *dnd5ev1alpha1.GrantedAction {
	if ga == nil {
		return nil
	}

	return &dnd5ev1alpha1.GrantedAction{
		Id:       ga.ID,
		Type:     ga.Type,
		Name:     ga.Name,
		Reason:   ga.Reason,
		WeaponId: ga.WeaponID,
	}
}
