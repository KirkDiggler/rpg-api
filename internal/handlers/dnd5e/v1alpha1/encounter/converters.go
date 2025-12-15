package encounter

import (
	apiv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/api/v1alpha1"
	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	characterhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v1alpha1/character"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/encounter"
	"github.com/KirkDiggler/rpg-toolkit/core"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
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

	return &dnd5ev1alpha1.DamageComponent{
		Source:            comp.Source,
		SourceRef:         convertCoreRefToProtoSourceRef(comp.SourceRef),
		OriginalDiceRolls: originalRolls,
		FinalDiceRolls:    finalRolls,
		Rerolls:           rerolls,
		FlatBonus:         int32(comp.FlatBonus),
		DamageType:        string(comp.DamageType), // damage.Type to proto string
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

	// Determine hex orientation for cube coordinate conversion
	hexOrientation := spatial.HexOrientationPointyTop // Default
	if spatialRoom.HexFlatTop {
		hexOrientation = spatial.HexOrientationFlatTop
	}

	// Convert entities map with cube coordinates for hex grids
	entities := make(map[string]*dnd5ev1alpha1.EntityPlacement, len(spatialRoom.Entities))
	for id, placement := range spatialRoom.Entities {
		entities[id] = convertEntityPlacementToProto(placement, spatialRoom.GridType, hexOrientation)
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
		EntityType:        placement.EntityType,
		Position:          position,
		Size:              int32(placement.Size),
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
	case refs.TypeFightingStyles:
		// Fighting styles use ConditionId in the proto (they're condition-based modifiers)
		return &dnd5ev1alpha1.SourceRef{
			Source: &dnd5ev1alpha1.SourceRef_Condition{
				Condition: fightingStyleRefToProto(ref),
			},
		}
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
func conditionRefToProto(ref *core.Ref) dnd5ev1alpha1.ConditionId {
	// Fast path: pointer comparison with singleton refs
	switch ref {
	case refs.Conditions.Raging():
		return dnd5ev1alpha1.ConditionId_CONDITION_ID_RAGING
	case refs.Conditions.BrutalCritical():
		return dnd5ev1alpha1.ConditionId_CONDITION_ID_BRUTAL_CRITICAL
	}

	// Slow path: string comparison for unknown refs
	switch ref.ID {
	case "raging":
		return dnd5ev1alpha1.ConditionId_CONDITION_ID_RAGING
	case "brutal_critical":
		return dnd5ev1alpha1.ConditionId_CONDITION_ID_BRUTAL_CRITICAL
	case "sneak_attack":
		return dnd5ev1alpha1.ConditionId_CONDITION_ID_SNEAK_ATTACK
	case "divine_smite":
		return dnd5ev1alpha1.ConditionId_CONDITION_ID_DIVINE_SMITE
	default:
		return dnd5ev1alpha1.ConditionId_CONDITION_ID_UNSPECIFIED
	}
}

// featureRefToProto converts toolkit feature ref to proto FeatureId enum using pointer comparison.
// Falls back to string comparison for unknown refs.
// Note: Not all toolkit features have proto enum values yet (e.g., Rage, SecondWind).
func featureRefToProto(ref *core.Ref) dnd5ev1alpha1.FeatureId {
	// Fast path: pointer comparison with singleton refs
	// (Currently no singleton features have corresponding proto enums)

	// Slow path: string comparison for unknown refs
	switch ref.ID {
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
	case "deflect_missiles":
		return dnd5ev1alpha1.FeatureId_FEATURE_ID_DEFLECT_MISSILES
	case "starry_form_archer":
		return dnd5ev1alpha1.FeatureId_FEATURE_ID_STARRY_FORM_ARCHER
	default:
		return dnd5ev1alpha1.FeatureId_FEATURE_ID_UNSPECIFIED
	}
}

// fightingStyleRefToProto converts toolkit fighting style ref to proto ConditionId enum using pointer comparison.
// Falls back to string comparison for unknown refs.
func fightingStyleRefToProto(ref *core.Ref) dnd5ev1alpha1.ConditionId {
	// Fast path: pointer comparison with singleton refs
	switch ref {
	case refs.FightingStyles.Dueling():
		return dnd5ev1alpha1.ConditionId_CONDITION_ID_FIGHTING_STYLE_DUELING
	case refs.FightingStyles.TwoWeaponFighting():
		return dnd5ev1alpha1.ConditionId_CONDITION_ID_FIGHTING_STYLE_TWO_WEAPON_FIGHTING
	}

	// Slow path: string comparison for unknown refs
	switch ref.ID {
	case "dueling":
		return dnd5ev1alpha1.ConditionId_CONDITION_ID_FIGHTING_STYLE_DUELING
	case "two_weapon_fighting":
		return dnd5ev1alpha1.ConditionId_CONDITION_ID_FIGHTING_STYLE_TWO_WEAPON_FIGHTING
	default:
		return dnd5ev1alpha1.ConditionId_CONDITION_ID_UNSPECIFIED
	}
}

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

	// Convert Details based on action type and success
	// For attack actions, Details should be an AttackResult
	if action.Success && action.Details != nil {
		switch action.ActionType {
		case string(monster.TypeMeleeAttack), string(monster.TypeRangedAttack):
			// Try to convert Details to AttackResult
			if attackResult, ok := action.Details.(*encounter.AttackResult); ok {
				protoAction.Details = &dnd5ev1alpha1.MonsterExecutedAction_AttackResult{
					AttackResult: convertAttackResultToProto(attackResult),
				}
			}
		case string(monster.TypeHeal):
			// Future: Add HealResult conversion when needed
			// For now, leave Details nil
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
