// Package entities defines the core data structures for the RPG API
package entities

import (
	"encoding/json"

	apiv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/api/v1alpha1"
	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
	"github.com/KirkDiggler/rpg-toolkit/core"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/races"
)

// Entity type constants
const (
	EntityTypeCharacter = "character"
	EntityTypeMonster   = "monster"
	EntityTypeObstacle  = "obstacle"
)

// EntityStateData stores the unified state of an entity (character, monster, or obstacle)
// in the API layer. It wraps the toolkit's data and adds spatial/room context.
type EntityStateData struct {
	EntityID   string
	EntityType string      // EntityTypeCharacter, EntityTypeMonster, EntityTypeObstacle
	RoomID     string
	Position   *Position
	Size       int
	Appearance *Appearance // Character appearance (nil for non-characters)

	// ToolkitData holds character.Data or monster.Data depending on EntityType.
	// For obstacles, this may be nil.
	ToolkitData any
}

// ToEntityStateProtoInput is the input for ToEntityStateProto.
type ToEntityStateProtoInput struct {
	EntityStateData *EntityStateData
}

// ToEntityStateProto converts an EntityStateData to the proto EntityState message.
// This is the SINGLE conversion point between toolkit data and proto types.
// Returns an empty EntityState (with entity_id set) if EntityStateData has nil ToolkitData.
//
//nolint:gosec // G115: Game values are bounded by D&D rules, no overflow risk
func ToEntityStateProto(input *ToEntityStateProtoInput) *dnd5ev1alpha1.EntityState {
	if input == nil || input.EntityStateData == nil {
		return &dnd5ev1alpha1.EntityState{}
	}

	data := input.EntityStateData

	result := &dnd5ev1alpha1.EntityState{
		EntityId:   data.EntityID,
		EntityType: stringToEntityType(data.EntityType),
		RoomId:     data.RoomID,
		Position:   positionToProto(data.Position),
		Size:       intToEntitySize(data.Size),
	}

	// Type-switch on ToolkitData to populate HP, conditions, death saves, and details
	switch td := data.ToolkitData.(type) {
	case *toolkitchar.Data:
		if td != nil {
			populateFromCharacterData(result, td, data.Appearance)
		}
	case *monster.Data:
		if td != nil {
			populateFromMonsterData(result, td)
		}
	}

	// Obstacles: blocks_movement and blocks_line_of_sight default to true
	if data.EntityType == EntityTypeObstacle {
		result.BlocksMovement = true
		result.BlocksLineOfSight = true
		result.Details = &dnd5ev1alpha1.EntityState_ObstacleDetails{
			ObstacleDetails: &dnd5ev1alpha1.ObstacleDetails{},
		}
	}

	return result
}

// ToEntityStateProtosInput is the input for ToEntityStateProtos.
type ToEntityStateProtosInput struct {
	EntityIDs []string
	Entities  map[string]*EntityStateData
}

// ToEntityStateProtos converts a batch of EntityStateData entries to proto EntityState messages.
// Only entities present in both EntityIDs and the Entities map are converted.
// Returns an empty slice if no matching entities are found.
func ToEntityStateProtos(input *ToEntityStateProtosInput) []*dnd5ev1alpha1.EntityState {
	if input == nil {
		return []*dnd5ev1alpha1.EntityState{}
	}

	result := make([]*dnd5ev1alpha1.EntityState, 0, len(input.EntityIDs))
	for _, id := range input.EntityIDs {
		esd, ok := input.Entities[id]
		if !ok || esd == nil {
			continue
		}
		result = append(result, ToEntityStateProto(&ToEntityStateProtoInput{
			EntityStateData: esd,
		}))
	}

	return result
}

// --- internal helpers ---

// populateFromCharacterData fills the EntityState with character-specific fields.
//
//nolint:gosec // G115: Game values are bounded by D&D rules, no overflow risk
func populateFromCharacterData(
	state *dnd5ev1alpha1.EntityState,
	charData *toolkitchar.Data,
	appearance *Appearance,
) {
	state.CurrentHitPoints = int32(charData.HitPoints)
	state.MaxHitPoints = int32(charData.MaxHitPoints)
	state.ActiveConditions = conditionsFromJSON(charData.Conditions)

	if charData.DeathSaveState != nil {
		state.DeathSaves = &dnd5ev1alpha1.DeathSaveProgress{
			SuccessCount: int32(charData.DeathSaveState.Successes),
			FailureCount: int32(charData.DeathSaveState.Failures),
			Stabilized:   charData.DeathSaveState.Stabilized,
			Dead:         charData.DeathSaveState.Dead,
		}
	}

	details := &dnd5ev1alpha1.CharacterDetails{
		Name:           charData.Name,
		Race:           raceToProtoEnum(charData.RaceID),
		CharacterClass: classToProtoEnum(charData.ClassID),
		Level:          int32(charData.Level),
		ArmorClass:     int32(charData.ArmorClass),
		EquipmentSlots: equipmentSlotsToProto(charData.EquipmentSlots),
	}

	if appearance != nil {
		details.Appearance = &dnd5ev1alpha1.Appearance{
			SkinTone:       appearance.SkinTone,
			PrimaryColor:   appearance.PrimaryColor,
			SecondaryColor: appearance.SecondaryColor,
			EyeColor:       appearance.EyeColor,
		}
	}

	state.Details = &dnd5ev1alpha1.EntityState_CharacterDetails{
		CharacterDetails: details,
	}
}

// populateFromMonsterData fills the EntityState with monster-specific fields.
//
//nolint:gosec // G115: Game values are bounded by D&D rules, no overflow risk
func populateFromMonsterData(state *dnd5ev1alpha1.EntityState, monsterData *monster.Data) {
	state.CurrentHitPoints = int32(monsterData.HitPoints)
	state.MaxHitPoints = int32(monsterData.MaxHitPoints)
	state.ActiveConditions = conditionsFromJSON(monsterData.Conditions)

	state.Details = &dnd5ev1alpha1.EntityState_MonsterDetails{
		MonsterDetails: &dnd5ev1alpha1.MonsterDetails{
			Name:        monsterData.Name,
			MonsterType: dungeon.MonsterTypeFromRef(monsterData.Ref),
			ArmorClass:  int32(monsterData.ArmorClass),
		},
	}
}

// conditionsFromJSON converts toolkit's []json.RawMessage conditions to proto Condition messages.
// Each condition JSON is expected to have a "ref" field (core.Ref) with name/id.
func conditionsFromJSON(rawConditions []json.RawMessage) []*dnd5ev1alpha1.Condition {
	if len(rawConditions) == 0 {
		return nil
	}

	conditions := make([]*dnd5ev1alpha1.Condition, 0, len(rawConditions))
	for _, raw := range rawConditions {
		condition := conditionFromJSON(raw)
		if condition != nil {
			conditions = append(conditions, condition)
		}
	}

	return conditions
}

// conditionPeek is a minimal struct for extracting condition info from JSON.
type conditionPeek struct {
	Ref  *core.Ref `json:"ref"`
	Name string    `json:"name"`
}

// conditionFromJSON extracts basic condition info from a JSON blob.
// Returns nil if the JSON cannot be parsed.
func conditionFromJSON(raw json.RawMessage) *dnd5ev1alpha1.Condition {
	var peek conditionPeek
	if err := json.Unmarshal(raw, &peek); err != nil {
		return nil
	}

	name := peek.Name
	if name == "" && peek.Ref != nil {
		name = peek.Ref.ID
	}

	return &dnd5ev1alpha1.Condition{
		Name:          name,
		ConditionData: raw,
	}
}

// positionToProto converts an entities.Position to a proto Position.
func positionToProto(pos *Position) *apiv1alpha1.Position {
	if pos == nil {
		return &apiv1alpha1.Position{}
	}
	return &apiv1alpha1.Position{
		X: pos.X,
		Y: pos.Y,
		Z: pos.Z,
	}
}

// stringToEntityType converts a string entity type to the proto enum.
func stringToEntityType(s string) dnd5ev1alpha1.EntityType {
	switch s {
	case EntityTypeCharacter:
		return dnd5ev1alpha1.EntityType_ENTITY_TYPE_CHARACTER
	case EntityTypeMonster:
		return dnd5ev1alpha1.EntityType_ENTITY_TYPE_MONSTER
	case EntityTypeObstacle:
		return dnd5ev1alpha1.EntityType_ENTITY_TYPE_OBSTACLE
	default:
		return dnd5ev1alpha1.EntityType_ENTITY_TYPE_UNSPECIFIED
	}
}

// intToEntitySize converts an int size to the proto EntitySize enum.
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
		return dnd5ev1alpha1.EntitySize_ENTITY_SIZE_MEDIUM
	}
}

// raceToProtoEnum converts toolkit races.Race to proto Race enum.
func raceToProtoEnum(race races.Race) dnd5ev1alpha1.Race {
	switch race {
	case races.Human:
		return dnd5ev1alpha1.Race_RACE_HUMAN
	case races.Elf:
		return dnd5ev1alpha1.Race_RACE_ELF
	case races.Dwarf:
		return dnd5ev1alpha1.Race_RACE_DWARF
	case races.Halfling:
		return dnd5ev1alpha1.Race_RACE_HALFLING
	case races.Dragonborn:
		return dnd5ev1alpha1.Race_RACE_DRAGONBORN
	case races.Gnome:
		return dnd5ev1alpha1.Race_RACE_GNOME
	case races.HalfElf:
		return dnd5ev1alpha1.Race_RACE_HALF_ELF
	case races.HalfOrc:
		return dnd5ev1alpha1.Race_RACE_HALF_ORC
	case races.Tiefling:
		return dnd5ev1alpha1.Race_RACE_TIEFLING
	default:
		return dnd5ev1alpha1.Race_RACE_UNSPECIFIED
	}
}

// classToProtoEnum converts toolkit classes.Class to proto Class enum.
func classToProtoEnum(class classes.Class) dnd5ev1alpha1.Class {
	switch class {
	case classes.Barbarian:
		return dnd5ev1alpha1.Class_CLASS_BARBARIAN
	case classes.Bard:
		return dnd5ev1alpha1.Class_CLASS_BARD
	case classes.Cleric:
		return dnd5ev1alpha1.Class_CLASS_CLERIC
	case classes.Druid:
		return dnd5ev1alpha1.Class_CLASS_DRUID
	case classes.Fighter:
		return dnd5ev1alpha1.Class_CLASS_FIGHTER
	case classes.Monk:
		return dnd5ev1alpha1.Class_CLASS_MONK
	case classes.Paladin:
		return dnd5ev1alpha1.Class_CLASS_PALADIN
	case classes.Ranger:
		return dnd5ev1alpha1.Class_CLASS_RANGER
	case classes.Rogue:
		return dnd5ev1alpha1.Class_CLASS_ROGUE
	case classes.Sorcerer:
		return dnd5ev1alpha1.Class_CLASS_SORCERER
	case classes.Warlock:
		return dnd5ev1alpha1.Class_CLASS_WARLOCK
	case classes.Wizard:
		return dnd5ev1alpha1.Class_CLASS_WIZARD
	default:
		return dnd5ev1alpha1.Class_CLASS_UNSPECIFIED
	}
}

// equipmentSlotsToProto converts toolkit EquipmentSlots to proto EquipmentSlots.
func equipmentSlotsToProto(slots toolkitchar.EquipmentSlots) *dnd5ev1alpha1.EquipmentSlots {
	if len(slots) == 0 {
		return nil
	}

	makeItem := func(itemID string) *dnd5ev1alpha1.InventoryItem {
		if itemID == "" {
			return nil
		}
		return &dnd5ev1alpha1.InventoryItem{
			ItemId:   itemID,
			Quantity: 1,
		}
	}

	return &dnd5ev1alpha1.EquipmentSlots{
		MainHand: makeItem(slots.Get(toolkitchar.SlotMainHand)),
		OffHand:  makeItem(slots.Get(toolkitchar.SlotOffHand)),
		Armor:    makeItem(slots.Get(toolkitchar.SlotArmor)),
		Helmet:   makeItem(slots.Get(toolkitchar.SlotHelm)),
		Boots:    makeItem(slots.Get(toolkitchar.SlotBoots)),
		Cloak:    makeItem(slots.Get(toolkitchar.SlotCloak)),
		Amulet:   makeItem(slots.Get(toolkitchar.SlotAmulet)),
		Ring_1:   makeItem(slots.Get(toolkitchar.SlotRingLeft)),
		Ring_2:   makeItem(slots.Get(toolkitchar.SlotRingRight)),
		Belt:     makeItem(slots.Get(toolkitchar.SlotBelt)),
	}
}
