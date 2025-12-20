package dungeon

import "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"

// Theme bundles all generation rules for a cohesive dungeon style.
// Themes control shape generation, feature placement, and monster selection.
type Theme struct {
	// ID is the unique identifier for this theme
	ID string
	// Name is the display name
	Name string
	// ShapeStyle determines the geometric style of rooms
	ShapeStyle ShapeStyle
	// Features contains rules for obstacle and terrain placement
	Features FeatureRules
	// MonsterPool contains regular monsters for encounters
	MonsterPool []MonsterRef
	// BossPool contains boss monsters for final rooms
	BossPool []MonsterRef
}

// FeatureRules defines probabilities and types for feature placement
type FeatureRules struct {
	// ObstacleChance is the probability (0-1) of placing obstacles
	ObstacleChance float64
	// ObstacleTypes are the allowed obstacle types for this theme
	ObstacleTypes []ObstacleType
	// TerrainChance is the probability (0-1) of placing terrain patches
	TerrainChance float64
	// TerrainTypes are the allowed terrain types for this theme
	TerrainTypes []TerrainType
}

// MonsterRef references a monster from the rulebook with role and CR metadata
type MonsterRef struct {
	// ID references the rulebook monster (will become toolkit constant)
	ID string
	// Role defines the tactical purpose in encounters
	Role MonsterRole
	// CR is the challenge rating
	CR float64
}

// ThemeCrypt represents an ancient crypt with undead monsters
var ThemeCrypt = Theme{
	ID:         "crypt",
	Name:       "Ancient Crypt",
	ShapeStyle: ShapeStyleStructured,
	Features: FeatureRules{
		ObstacleChance: 0.6,
		ObstacleTypes: []ObstacleType{
			ObstacleTypePillar,
			ObstacleTypeSarcophagus,
			ObstacleTypeAltar,
		},
		TerrainChance: 0.0,
		TerrainTypes:  []TerrainType{},
	},
	MonsterPool: []MonsterRef{
		{ID: refs.Monsters.Skeleton().ID, Role: RoleMelee, CR: 0.25},
		{ID: refs.Monsters.Zombie().ID, Role: RoleMelee, CR: 0.25},
		{ID: refs.Monsters.SkeletonArcher().ID, Role: RoleRanged, CR: 0.25},
	},
	BossPool: []MonsterRef{
		{ID: refs.Monsters.SkeletonCaptain().ID, Role: RoleBoss, CR: 2},
	},
}

// ThemeCave represents a natural cave with beast monsters
var ThemeCave = Theme{
	ID:         "cave",
	Name:       "Natural Cave",
	ShapeStyle: ShapeStyleOrganic,
	Features: FeatureRules{
		ObstacleChance: 0.4,
		ObstacleTypes: []ObstacleType{
			ObstacleTypeBoulder,
			ObstacleTypeStalagmite,
			ObstacleTypePool,
		},
		TerrainChance: 0.0,
		TerrainTypes:  []TerrainType{},
	},
	MonsterPool: []MonsterRef{
		{ID: refs.Monsters.GiantRat().ID, Role: RoleMelee, CR: 0.125},
		{ID: refs.Monsters.GiantSpider().ID, Role: RoleMelee, CR: 0.5},
	},
	BossPool: []MonsterRef{
		{ID: refs.Monsters.GiantWolfSpider().ID, Role: RoleBoss, CR: 1},
	},
}

// ThemeBanditLair represents a hideout occupied by humanoid bandits
var ThemeBanditLair = Theme{
	ID:         "bandit-lair",
	Name:       "Bandit Hideout",
	ShapeStyle: ShapeStyleMixed,
	Features: FeatureRules{
		ObstacleChance: 0.5,
		ObstacleTypes: []ObstacleType{
			ObstacleTypeCrate,
			ObstacleTypeBarrel,
		},
		TerrainChance: 0.0,
		TerrainTypes:  []TerrainType{},
	},
	MonsterPool: []MonsterRef{
		{ID: refs.Monsters.Bandit().ID, Role: RoleMelee, CR: 0.125},
		{ID: refs.Monsters.BanditArcher().ID, Role: RoleRanged, CR: 0.25},
	},
	BossPool: []MonsterRef{
		{ID: refs.Monsters.BanditCaptain().ID, Role: RoleBoss, CR: 2},
	},
}
