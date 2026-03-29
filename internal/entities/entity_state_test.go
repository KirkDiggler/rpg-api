package entities_test

import (
	"encoding/json"
	"testing"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/races"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/saves"
	"github.com/stretchr/testify/suite"
)

type EntityStateTestSuite struct {
	suite.Suite
}

func TestEntityStateSuite(t *testing.T) {
	suite.Run(t, new(EntityStateTestSuite))
}

// --- ToEntityStateProto: character ---

func (s *EntityStateTestSuite) TestToEntityStateProto_Character_AllFields() {
	conditionJSON := json.RawMessage(`{"ref":{"id":"raging","module":"dnd5e","type":"conditions"},"name":"Raging"}`)

	charData := &toolkitchar.Data{
		ID:           "char-1",
		Name:         "Thorin",
		Level:        5,
		RaceID:       races.Dwarf,
		ClassID:      classes.Fighter,
		HitPoints:    38,
		MaxHitPoints: 44,
		ArmorClass:   18,
		Conditions:   []json.RawMessage{conditionJSON},
		EquipmentSlots: toolkitchar.EquipmentSlots{
			toolkitchar.SlotMainHand: "longsword",
			toolkitchar.SlotArmor:    "chain_mail",
		},
		DeathSaveState: &saves.DeathSaveState{
			Successes:  2,
			Failures:   1,
			Stabilized: false,
			Dead:       false,
		},
	}

	appearance := &entities.Appearance{
		SkinTone:       "#D5A88C",
		PrimaryColor:   "#8B0000",
		SecondaryColor: "#FFD700",
		EyeColor:       "#4A2511",
	}

	esd := &entities.EntityStateData{
		EntityID:    "char-1",
		EntityType:  entities.EntityTypeCharacter,
		RoomID:      "room-42",
		Position:    &entities.Position{X: 3, Y: -1, Z: -2},
		Size:        2, // medium
		Appearance:  appearance,
		ToolkitData: charData,
	}

	result := entities.ToEntityStateProto(&entities.ToEntityStateProtoInput{
		EntityStateData: esd,
	})

	// Common fields
	s.Equal("char-1", result.EntityId)
	s.Equal(dnd5ev1alpha1.EntityType_ENTITY_TYPE_CHARACTER, result.EntityType)
	s.Equal("room-42", result.RoomId)
	s.Equal(float64(3), result.Position.X)
	s.Equal(float64(-1), result.Position.Y)
	s.Equal(float64(-2), result.Position.Z)
	s.Equal(dnd5ev1alpha1.EntitySize_ENTITY_SIZE_MEDIUM, result.Size)

	// HP
	s.Equal(int32(38), result.CurrentHitPoints)
	s.Equal(int32(44), result.MaxHitPoints)

	// Death saves
	s.Require().NotNil(result.DeathSaves)
	s.Equal(int32(2), result.DeathSaves.SuccessCount)
	s.Equal(int32(1), result.DeathSaves.FailureCount)
	s.False(result.DeathSaves.Stabilized)
	s.False(result.DeathSaves.Dead)

	// Conditions
	s.Require().Len(result.ActiveConditions, 1)
	s.Equal("Raging", result.ActiveConditions[0].Name)
	s.NotEmpty(result.ActiveConditions[0].ConditionData)

	// Character details
	details := result.GetCharacterDetails()
	s.Require().NotNil(details)
	s.Equal("Thorin", details.Name)
	s.Equal(dnd5ev1alpha1.Race_RACE_DWARF, details.Race)
	s.Equal(dnd5ev1alpha1.Class_CLASS_FIGHTER, details.CharacterClass)
	s.Equal(int32(5), details.Level)
	s.Equal(int32(18), details.ArmorClass)

	// Appearance
	s.Require().NotNil(details.Appearance)
	s.Equal("#D5A88C", details.Appearance.SkinTone)
	s.Equal("#8B0000", details.Appearance.PrimaryColor)
	s.Equal("#FFD700", details.Appearance.SecondaryColor)
	s.Equal("#4A2511", details.Appearance.EyeColor)

	// Equipment slots
	s.Require().NotNil(details.EquipmentSlots)
	s.Require().NotNil(details.EquipmentSlots.MainHand)
	s.Equal("longsword", details.EquipmentSlots.MainHand.ItemId)
	s.Require().NotNil(details.EquipmentSlots.Armor)
	s.Equal("chain_mail", details.EquipmentSlots.Armor.ItemId)

	// No monster/obstacle details
	s.Nil(result.GetMonsterDetails())
	s.Nil(result.GetObstacleDetails())
}

func (s *EntityStateTestSuite) TestToEntityStateProto_Character_NoDeathSaves() {
	charData := &toolkitchar.Data{
		ID:           "char-2",
		Name:         "Legolas",
		Level:        3,
		RaceID:       races.Elf,
		ClassID:      classes.Ranger,
		HitPoints:    22,
		MaxHitPoints: 22,
		ArmorClass:   14,
	}

	esd := &entities.EntityStateData{
		EntityID:    "char-2",
		EntityType:  entities.EntityTypeCharacter,
		RoomID:      "room-1",
		Position:    &entities.Position{X: 0, Y: 0, Z: 0},
		Size:        2,
		ToolkitData: charData,
	}

	result := entities.ToEntityStateProto(&entities.ToEntityStateProtoInput{
		EntityStateData: esd,
	})

	s.Nil(result.DeathSaves)
	s.Empty(result.ActiveConditions)

	details := result.GetCharacterDetails()
	s.Require().NotNil(details)
	s.Equal("Legolas", details.Name)
	s.Equal(dnd5ev1alpha1.Race_RACE_ELF, details.Race)
	s.Equal(dnd5ev1alpha1.Class_CLASS_RANGER, details.CharacterClass)
	s.Nil(details.Appearance) // No appearance set
}

// --- ToEntityStateProto: monster ---

func (s *EntityStateTestSuite) TestToEntityStateProto_Monster_AllFields() {
	conditionJSON := json.RawMessage(`{"ref":{"id":"poisoned","module":"dnd5e","type":"conditions"}}`)

	monsterData := &monster.Data{
		ID:           "mob-1",
		Name:         "Skeleton Warrior",
		Ref:          refs.Monsters.Skeleton(),
		HitPoints:    8,
		MaxHitPoints: 13,
		ArmorClass:   13,
		Conditions:   []json.RawMessage{conditionJSON},
	}

	esd := &entities.EntityStateData{
		EntityID:    "mob-1",
		EntityType:  entities.EntityTypeMonster,
		RoomID:      "room-42",
		Position:    &entities.Position{X: 5, Y: -3, Z: -2},
		Size:        2,
		ToolkitData: monsterData,
	}

	result := entities.ToEntityStateProto(&entities.ToEntityStateProtoInput{
		EntityStateData: esd,
	})

	// Common fields
	s.Equal("mob-1", result.EntityId)
	s.Equal(dnd5ev1alpha1.EntityType_ENTITY_TYPE_MONSTER, result.EntityType)
	s.Equal("room-42", result.RoomId)
	s.Equal(int32(8), result.CurrentHitPoints)
	s.Equal(int32(13), result.MaxHitPoints)

	// Conditions
	s.Require().Len(result.ActiveConditions, 1)
	s.Equal("poisoned", result.ActiveConditions[0].Name)

	// Monster details
	details := result.GetMonsterDetails()
	s.Require().NotNil(details)
	s.Equal("Skeleton Warrior", details.Name)
	s.Equal(dnd5ev1alpha1.MonsterType_MONSTER_TYPE_SKELETON, details.MonsterType)
	s.Equal(int32(13), details.ArmorClass)

	// No character/obstacle details
	s.Nil(result.GetCharacterDetails())
	s.Nil(result.GetObstacleDetails())
}

func (s *EntityStateTestSuite) TestToEntityStateProto_Monster_NilRef() {
	monsterData := &monster.Data{
		ID:           "mob-2",
		Name:         "Unknown Beast",
		Ref:          nil,
		HitPoints:    20,
		MaxHitPoints: 20,
		ArmorClass:   10,
	}

	esd := &entities.EntityStateData{
		EntityID:    "mob-2",
		EntityType:  entities.EntityTypeMonster,
		RoomID:      "room-1",
		Position:    &entities.Position{},
		Size:        2,
		ToolkitData: monsterData,
	}

	result := entities.ToEntityStateProto(&entities.ToEntityStateProtoInput{
		EntityStateData: esd,
	})

	details := result.GetMonsterDetails()
	s.Require().NotNil(details)
	// MonsterTypeFromRef falls back to GOBLIN for nil refs
	s.Equal(dnd5ev1alpha1.MonsterType_MONSTER_TYPE_GOBLIN, details.MonsterType)
}

// --- ToEntityStateProto: obstacle ---

func (s *EntityStateTestSuite) TestToEntityStateProto_Obstacle() {
	esd := &entities.EntityStateData{
		EntityID:   "obs-1",
		EntityType: entities.EntityTypeObstacle,
		RoomID:     "room-42",
		Position:   &entities.Position{X: 1, Y: 0, Z: -1},
		Size:       2,
	}

	result := entities.ToEntityStateProto(&entities.ToEntityStateProtoInput{
		EntityStateData: esd,
	})

	s.Equal("obs-1", result.EntityId)
	s.Equal(dnd5ev1alpha1.EntityType_ENTITY_TYPE_OBSTACLE, result.EntityType)
	s.True(result.BlocksMovement)
	s.True(result.BlocksLineOfSight)

	obstacleDetails := result.GetObstacleDetails()
	s.Require().NotNil(obstacleDetails)

	s.Nil(result.GetCharacterDetails())
	s.Nil(result.GetMonsterDetails())
}

// --- ToEntityStateProto: edge cases ---

func (s *EntityStateTestSuite) TestToEntityStateProto_NilInput() {
	result := entities.ToEntityStateProto(nil)
	s.NotNil(result)
	s.Equal("", result.EntityId)
}

func (s *EntityStateTestSuite) TestToEntityStateProto_NilEntityStateData() {
	result := entities.ToEntityStateProto(&entities.ToEntityStateProtoInput{
		EntityStateData: nil,
	})
	s.NotNil(result)
	s.Equal("", result.EntityId)
}

func (s *EntityStateTestSuite) TestToEntityStateProto_NilToolkitData() {
	esd := &entities.EntityStateData{
		EntityID:    "ent-1",
		EntityType:  entities.EntityTypeCharacter,
		RoomID:      "room-1",
		Position:    &entities.Position{X: 1, Y: 1, Z: -2},
		Size:        2,
		ToolkitData: nil,
	}

	result := entities.ToEntityStateProto(&entities.ToEntityStateProtoInput{
		EntityStateData: esd,
	})

	s.NotNil(result)
	s.Equal("ent-1", result.EntityId)
	s.Equal(dnd5ev1alpha1.EntityType_ENTITY_TYPE_CHARACTER, result.EntityType)
	s.Equal(int32(0), result.CurrentHitPoints)
	s.Nil(result.GetCharacterDetails())
}

func (s *EntityStateTestSuite) TestToEntityStateProto_NilPosition() {
	esd := &entities.EntityStateData{
		EntityID:    "ent-2",
		EntityType:  entities.EntityTypeMonster,
		RoomID:      "room-1",
		Position:    nil,
		Size:        2,
		ToolkitData: &monster.Data{ID: "mob-3", Name: "Goblin", HitPoints: 7, MaxHitPoints: 7},
	}

	result := entities.ToEntityStateProto(&entities.ToEntityStateProtoInput{
		EntityStateData: esd,
	})

	// Should return a zero-origin position, never nil
	s.Require().NotNil(result.Position)
	s.Equal(float64(0), result.Position.X)
	s.Equal(float64(0), result.Position.Y)
	s.Equal(float64(0), result.Position.Z)
}

// --- Condition JSON parsing ---

func (s *EntityStateTestSuite) TestConditionsFromJSON_WithRef() {
	condJSON := json.RawMessage(`{"ref":{"id":"stunned","module":"dnd5e","type":"conditions"},"name":"Stunned"}`)

	esd := &entities.EntityStateData{
		EntityID:   "char-c",
		EntityType: entities.EntityTypeCharacter,
		RoomID:     "room-1",
		Position:   &entities.Position{},
		Size:       2,
		ToolkitData: &toolkitchar.Data{
			ID:         "char-c",
			HitPoints:  10,
			Conditions: []json.RawMessage{condJSON},
		},
	}

	result := entities.ToEntityStateProto(&entities.ToEntityStateProtoInput{
		EntityStateData: esd,
	})

	s.Require().Len(result.ActiveConditions, 1)
	s.Equal("Stunned", result.ActiveConditions[0].Name)
}

func (s *EntityStateTestSuite) TestConditionsFromJSON_RefOnly_NoName() {
	// When name is empty, falls back to ref.ID
	condJSON := json.RawMessage(`{"ref":{"id":"frightened","module":"dnd5e","type":"conditions"}}`)

	esd := &entities.EntityStateData{
		EntityID:   "char-d",
		EntityType: entities.EntityTypeCharacter,
		RoomID:     "room-1",
		Position:   &entities.Position{},
		Size:       2,
		ToolkitData: &toolkitchar.Data{
			ID:         "char-d",
			HitPoints:  10,
			Conditions: []json.RawMessage{condJSON},
		},
	}

	result := entities.ToEntityStateProto(&entities.ToEntityStateProtoInput{
		EntityStateData: esd,
	})

	s.Require().Len(result.ActiveConditions, 1)
	s.Equal("frightened", result.ActiveConditions[0].Name)
}

func (s *EntityStateTestSuite) TestConditionsFromJSON_InvalidJSON() {
	condJSON := json.RawMessage(`{invalid json}`)

	esd := &entities.EntityStateData{
		EntityID:   "char-e",
		EntityType: entities.EntityTypeCharacter,
		RoomID:     "room-1",
		Position:   &entities.Position{},
		Size:       2,
		ToolkitData: &toolkitchar.Data{
			ID:         "char-e",
			HitPoints:  10,
			Conditions: []json.RawMessage{condJSON},
		},
	}

	result := entities.ToEntityStateProto(&entities.ToEntityStateProtoInput{
		EntityStateData: esd,
	})

	// Invalid JSON conditions are skipped
	s.Empty(result.ActiveConditions)
}

// --- ToEntityStateProtos: batch ---

func (s *EntityStateTestSuite) TestToEntityStateProtos_BatchConversion() {
	entitiesMap := map[string]*entities.EntityStateData{
		"char-1": {
			EntityID:   "char-1",
			EntityType: entities.EntityTypeCharacter,
			RoomID:     "room-1",
			Position:   &entities.Position{X: 1, Y: 0, Z: -1},
			Size:       2,
			ToolkitData: &toolkitchar.Data{
				ID: "char-1", Name: "Hero", HitPoints: 20, MaxHitPoints: 20,
				RaceID: races.Human, ClassID: classes.Fighter,
			},
		},
		"mob-1": {
			EntityID:   "mob-1",
			EntityType: entities.EntityTypeMonster,
			RoomID:     "room-1",
			Position:   &entities.Position{X: 3, Y: -2, Z: -1},
			Size:       2,
			ToolkitData: &monster.Data{
				ID: "mob-1", Name: "Skeleton", HitPoints: 10, MaxHitPoints: 13,
				Ref: refs.Monsters.Skeleton(),
			},
		},
	}

	result := entities.ToEntityStateProtos(&entities.ToEntityStateProtosInput{
		EntityIDs: []string{"char-1", "mob-1"},
		Entities:  entitiesMap,
	})

	s.Require().Len(result, 2)
	s.Equal("char-1", result[0].EntityId)
	s.Equal("mob-1", result[1].EntityId)
}

func (s *EntityStateTestSuite) TestToEntityStateProtos_SkipsMissing() {
	entitiesMap := map[string]*entities.EntityStateData{
		"char-1": {
			EntityID:   "char-1",
			EntityType: entities.EntityTypeCharacter,
			RoomID:     "room-1",
			Position:   &entities.Position{},
			Size:       2,
			ToolkitData: &toolkitchar.Data{
				ID: "char-1", Name: "Hero", HitPoints: 20, MaxHitPoints: 20,
			},
		},
	}

	result := entities.ToEntityStateProtos(&entities.ToEntityStateProtosInput{
		EntityIDs: []string{"char-1", "missing-id"},
		Entities:  entitiesMap,
	})

	s.Require().Len(result, 1)
	s.Equal("char-1", result[0].EntityId)
}

func (s *EntityStateTestSuite) TestToEntityStateProtos_NilInput() {
	result := entities.ToEntityStateProtos(nil)
	s.NotNil(result)
	s.Empty(result)
}

func (s *EntityStateTestSuite) TestToEntityStateProtos_EmptyInput() {
	result := entities.ToEntityStateProtos(&entities.ToEntityStateProtosInput{
		EntityIDs: []string{},
		Entities:  map[string]*entities.EntityStateData{},
	})
	s.NotNil(result)
	s.Empty(result)
}

// --- Entity size mapping ---

func (s *EntityStateTestSuite) TestIntToEntitySize() {
	testCases := []struct {
		size     int
		expected dnd5ev1alpha1.EntitySize
	}{
		{0, dnd5ev1alpha1.EntitySize_ENTITY_SIZE_TINY},
		{1, dnd5ev1alpha1.EntitySize_ENTITY_SIZE_SMALL},
		{2, dnd5ev1alpha1.EntitySize_ENTITY_SIZE_MEDIUM},
		{3, dnd5ev1alpha1.EntitySize_ENTITY_SIZE_LARGE},
		{4, dnd5ev1alpha1.EntitySize_ENTITY_SIZE_HUGE},
		{5, dnd5ev1alpha1.EntitySize_ENTITY_SIZE_GARGANTUAN},
		{99, dnd5ev1alpha1.EntitySize_ENTITY_SIZE_MEDIUM}, // unknown defaults to medium
	}

	for _, tc := range testCases {
		esd := &entities.EntityStateData{
			EntityID:    "ent-size",
			EntityType:  entities.EntityTypeMonster,
			RoomID:      "room-1",
			Position:    &entities.Position{},
			Size:        tc.size,
			ToolkitData: &monster.Data{ID: "m", Name: "M", HitPoints: 1, MaxHitPoints: 1},
		}
		result := entities.ToEntityStateProto(&entities.ToEntityStateProtoInput{
			EntityStateData: esd,
		})
		s.Equal(tc.expected, result.Size, "size %d should map to %v", tc.size, tc.expected)
	}
}

// --- Race and class enum mapping ---

func (s *EntityStateTestSuite) TestRaceMapping() {
	testCases := []struct {
		race     races.Race
		expected dnd5ev1alpha1.Race
	}{
		{races.Human, dnd5ev1alpha1.Race_RACE_HUMAN},
		{races.Elf, dnd5ev1alpha1.Race_RACE_ELF},
		{races.Dwarf, dnd5ev1alpha1.Race_RACE_DWARF},
		{races.Halfling, dnd5ev1alpha1.Race_RACE_HALFLING},
		{races.Dragonborn, dnd5ev1alpha1.Race_RACE_DRAGONBORN},
		{races.Gnome, dnd5ev1alpha1.Race_RACE_GNOME},
		{races.HalfElf, dnd5ev1alpha1.Race_RACE_HALF_ELF},
		{races.HalfOrc, dnd5ev1alpha1.Race_RACE_HALF_ORC},
		{races.Tiefling, dnd5ev1alpha1.Race_RACE_TIEFLING},
		{"unknown_race", dnd5ev1alpha1.Race_RACE_UNSPECIFIED},
	}

	for _, tc := range testCases {
		esd := &entities.EntityStateData{
			EntityID:   "char-race",
			EntityType: entities.EntityTypeCharacter,
			RoomID:     "room-1",
			Position:   &entities.Position{},
			Size:       2,
			ToolkitData: &toolkitchar.Data{
				ID: "c", Name: "C", HitPoints: 10, MaxHitPoints: 10,
				RaceID: tc.race,
			},
		}
		result := entities.ToEntityStateProto(&entities.ToEntityStateProtoInput{EntityStateData: esd})
		details := result.GetCharacterDetails()
		s.Require().NotNil(details)
		s.Equal(tc.expected, details.Race, "race %q should map to %v", tc.race, tc.expected)
	}
}

func (s *EntityStateTestSuite) TestClassMapping() {
	testCases := []struct {
		class    classes.Class
		expected dnd5ev1alpha1.Class
	}{
		{classes.Barbarian, dnd5ev1alpha1.Class_CLASS_BARBARIAN},
		{classes.Bard, dnd5ev1alpha1.Class_CLASS_BARD},
		{classes.Cleric, dnd5ev1alpha1.Class_CLASS_CLERIC},
		{classes.Druid, dnd5ev1alpha1.Class_CLASS_DRUID},
		{classes.Fighter, dnd5ev1alpha1.Class_CLASS_FIGHTER},
		{classes.Monk, dnd5ev1alpha1.Class_CLASS_MONK},
		{classes.Paladin, dnd5ev1alpha1.Class_CLASS_PALADIN},
		{classes.Ranger, dnd5ev1alpha1.Class_CLASS_RANGER},
		{classes.Rogue, dnd5ev1alpha1.Class_CLASS_ROGUE},
		{classes.Sorcerer, dnd5ev1alpha1.Class_CLASS_SORCERER},
		{classes.Warlock, dnd5ev1alpha1.Class_CLASS_WARLOCK},
		{classes.Wizard, dnd5ev1alpha1.Class_CLASS_WIZARD},
		{"unknown_class", dnd5ev1alpha1.Class_CLASS_UNSPECIFIED},
	}

	for _, tc := range testCases {
		esd := &entities.EntityStateData{
			EntityID:   "char-class",
			EntityType: entities.EntityTypeCharacter,
			RoomID:     "room-1",
			Position:   &entities.Position{},
			Size:       2,
			ToolkitData: &toolkitchar.Data{
				ID: "c", Name: "C", HitPoints: 10, MaxHitPoints: 10,
				ClassID: tc.class,
			},
		}
		result := entities.ToEntityStateProto(&entities.ToEntityStateProtoInput{EntityStateData: esd})
		details := result.GetCharacterDetails()
		s.Require().NotNil(details)
		s.Equal(tc.expected, details.CharacterClass, "class %q should map to %v", tc.class, tc.expected)
	}
}

