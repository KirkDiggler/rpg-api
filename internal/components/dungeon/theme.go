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
	// Tables contains weighted selection tables for pattern-based generation
	Tables *ThemeTables
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
	Tables: &ThemeTables{
		WallPatterns: map[RoomType][]WeightedPattern{
			RoomTypeEntrance: {
				{Pattern: PatternSparse, Weight: 70},
				{Pattern: PatternPillarGrid, Weight: 30},
			},
			RoomTypeChamber: {
				{Pattern: PatternPillarGrid, Weight: 50},
				{Pattern: PatternCoverClusters, Weight: 30},
				{Pattern: PatternPerimeterCover, Weight: 20},
			},
			RoomTypeCorridor: {
				{Pattern: PatternChokepoints, Weight: 50},
				{Pattern: PatternPillarGrid, Weight: 50},
			},
			RoomTypeBoss: {
				{Pattern: PatternCentralFeature, Weight: 50},
				{Pattern: PatternPillarGrid, Weight: 30},
				{Pattern: PatternPerimeterCover, Weight: 20},
			},
			RoomTypeTreasure: {
				{Pattern: PatternPerimeterCover, Weight: 60},
				{Pattern: PatternPillarGrid, Weight: 40},
			},
			RoomTypeTrap: {
				{Pattern: PatternChokepoints, Weight: 60},
				{Pattern: PatternCoverClusters, Weight: 40},
			},
		},
		Obstacles: []WeightedObstacle{
			{Obstacle: ObstacleTypePillar, Weight: 40},
			{Obstacle: ObstacleTypeSarcophagus, Weight: 40},
			{Obstacle: ObstacleTypeAltar, Weight: 20},
		},
		Density: []WeightedDensity{
			{Density: DensityLow, Weight: 20},
			{Density: DensityMedium, Weight: 50},
			{Density: DensityHigh, Weight: 30},
		},
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
	Tables: &ThemeTables{
		WallPatterns: map[RoomType][]WeightedPattern{
			RoomTypeEntrance: {
				{Pattern: PatternSparse, Weight: 80},
				{Pattern: PatternPerimeterCover, Weight: 20},
			},
			RoomTypeChamber: {
				{Pattern: PatternCoverClusters, Weight: 50},
				{Pattern: PatternSparse, Weight: 30},
				{Pattern: PatternCentralFeature, Weight: 20},
			},
			RoomTypeCorridor: {
				{Pattern: PatternChokepoints, Weight: 60},
				{Pattern: PatternSparse, Weight: 40},
			},
			RoomTypeBoss: {
				{Pattern: PatternCentralFeature, Weight: 70},
				{Pattern: PatternCoverClusters, Weight: 30},
			},
			RoomTypeTreasure: {
				{Pattern: PatternPerimeterCover, Weight: 50},
				{Pattern: PatternCoverClusters, Weight: 50},
			},
			RoomTypeTrap: {
				{Pattern: PatternChokepoints, Weight: 50},
				{Pattern: PatternCoverClusters, Weight: 50},
			},
		},
		Obstacles: []WeightedObstacle{
			{Obstacle: ObstacleTypeBoulder, Weight: 50},
			{Obstacle: ObstacleTypeStalagmite, Weight: 30},
			{Obstacle: ObstacleTypePool, Weight: 20},
		},
		Density: []WeightedDensity{
			{Density: DensityLow, Weight: 30},
			{Density: DensityMedium, Weight: 50},
			{Density: DensityHigh, Weight: 20},
		},
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
	Tables: &ThemeTables{
		WallPatterns: map[RoomType][]WeightedPattern{
			RoomTypeEntrance: {
				{Pattern: PatternSparse, Weight: 60},
				{Pattern: PatternCoverClusters, Weight: 40},
			},
			RoomTypeChamber: {
				{Pattern: PatternCoverClusters, Weight: 60},
				{Pattern: PatternChokepoints, Weight: 20},
				{Pattern: PatternSparse, Weight: 20},
			},
			RoomTypeCorridor: {
				{Pattern: PatternChokepoints, Weight: 70},
				{Pattern: PatternCoverClusters, Weight: 30},
			},
			RoomTypeBoss: {
				{Pattern: PatternCentralFeature, Weight: 40},
				{Pattern: PatternCoverClusters, Weight: 40},
				{Pattern: PatternPerimeterCover, Weight: 20},
			},
			RoomTypeTreasure: {
				{Pattern: PatternPerimeterCover, Weight: 50},
				{Pattern: PatternChokepoints, Weight: 50},
			},
			RoomTypeTrap: {
				{Pattern: PatternChokepoints, Weight: 60},
				{Pattern: PatternCoverClusters, Weight: 40},
			},
		},
		Obstacles: []WeightedObstacle{
			{Obstacle: ObstacleTypeCrate, Weight: 50},
			{Obstacle: ObstacleTypeBarrel, Weight: 50},
		},
		Density: []WeightedDensity{
			{Density: DensityLow, Weight: 20},
			{Density: DensityMedium, Weight: 60},
			{Density: DensityHigh, Weight: 20},
		},
	},
	MonsterPool: []MonsterRef{
		{ID: refs.Monsters.Bandit().ID, Role: RoleMelee, CR: 0.125},
		{ID: refs.Monsters.BanditArcher().ID, Role: RoleRanged, CR: 0.25},
	},
	BossPool: []MonsterRef{
		{ID: refs.Monsters.BanditCaptain().ID, Role: RoleBoss, CR: 2},
	},
}

// ThemeDebugWalls is a test theme that forces wall generation for UI testing.
// Uses PillarGrid pattern with 100% weight for all room types.
var ThemeDebugWalls = Theme{
	ID:         "debug_walls",
	Name:       "Debug (Forced Walls)",
	ShapeStyle: ShapeStyleStructured,
	Features: FeatureRules{
		ObstacleChance: 0.0, // No obstacles, just walls
		ObstacleTypes:  []ObstacleType{},
		TerrainChance:  0.0,
		TerrainTypes:   []TerrainType{},
	},
	Tables: &ThemeTables{
		WallPatterns: map[RoomType][]WeightedPattern{
			RoomTypeEntrance: {
				{Pattern: PatternPillarGrid, Weight: 100},
			},
			RoomTypeChamber: {
				{Pattern: PatternPillarGrid, Weight: 100},
			},
			RoomTypeCorridor: {
				{Pattern: PatternChokepoints, Weight: 100},
			},
			RoomTypeBoss: {
				{Pattern: PatternCentralFeature, Weight: 100},
			},
			RoomTypeTreasure: {
				{Pattern: PatternPillarGrid, Weight: 100},
			},
			RoomTypeTrap: {
				{Pattern: PatternChokepoints, Weight: 100},
			},
		},
		Obstacles: []WeightedObstacle{},
		Density: []WeightedDensity{
			{Density: DensityHigh, Weight: 100},
		},
	},
	MonsterPool: []MonsterRef{
		{ID: refs.Monsters.Skeleton().ID, Role: RoleMelee, CR: 0.25},
	},
	BossPool: []MonsterRef{
		{ID: refs.Monsters.SkeletonCaptain().ID, Role: RoleBoss, CR: 2},
	},
}
