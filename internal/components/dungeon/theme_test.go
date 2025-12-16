package dungeon

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type ThemeTestSuite struct {
	suite.Suite
}

func TestThemeTestSuite(t *testing.T) {
	suite.Run(t, new(ThemeTestSuite))
}

func (s *ThemeTestSuite) TestThemeCrypt() {
	theme := ThemeCrypt

	// Verify theme metadata
	s.Equal("crypt", theme.ID)
	s.Equal("Ancient Crypt", theme.Name)
	s.Equal(ShapeStyleStructured, theme.ShapeStyle)

	// Verify feature rules
	s.Equal(0.6, theme.Features.ObstacleChance)
	s.Equal(0.0, theme.Features.TerrainChance)

	// Verify obstacle types match crypt theme
	s.Len(theme.Features.ObstacleTypes, 3)
	s.Contains(theme.Features.ObstacleTypes, ObstacleTypePillar)
	s.Contains(theme.Features.ObstacleTypes, ObstacleTypeSarcophagus)
	s.Contains(theme.Features.ObstacleTypes, ObstacleTypeAltar)

	// Verify terrain types (should be empty for crypt)
	s.Empty(theme.Features.TerrainTypes)

	// Verify monster pool
	s.Len(theme.MonsterPool, 3)

	// Check skeleton
	skeleton := findMonster(theme.MonsterPool, "skeleton")
	s.NotNil(skeleton)
	s.Equal(RoleMelee, skeleton.Role)
	s.Equal(0.25, skeleton.CR)

	// Check zombie
	zombie := findMonster(theme.MonsterPool, "zombie")
	s.NotNil(zombie)
	s.Equal(RoleMelee, zombie.Role)
	s.Equal(0.25, zombie.CR)

	// Check skeleton archer
	archer := findMonster(theme.MonsterPool, "skeleton-archer")
	s.NotNil(archer)
	s.Equal(RoleRanged, archer.Role)
	s.Equal(0.25, archer.CR)

	// Verify boss pool
	s.Len(theme.BossPool, 1)
	boss := theme.BossPool[0]
	s.Equal("skeleton-captain", boss.ID)
	s.Equal(RoleBoss, boss.Role)
	s.Equal(2.0, boss.CR)
}

func (s *ThemeTestSuite) TestThemeCave() {
	theme := ThemeCave

	// Verify theme metadata
	s.Equal("cave", theme.ID)
	s.Equal("Natural Cave", theme.Name)
	s.Equal(ShapeStyleOrganic, theme.ShapeStyle)

	// Verify feature rules
	s.Equal(0.4, theme.Features.ObstacleChance)
	s.Equal(0.0, theme.Features.TerrainChance)

	// Verify obstacle types match cave theme
	s.Len(theme.Features.ObstacleTypes, 3)
	s.Contains(theme.Features.ObstacleTypes, ObstacleTypeBoulder)
	s.Contains(theme.Features.ObstacleTypes, ObstacleTypeStalagmite)
	s.Contains(theme.Features.ObstacleTypes, ObstacleTypePool)

	// Verify terrain types (should be empty for cave)
	s.Empty(theme.Features.TerrainTypes)

	// Verify monster pool
	s.Len(theme.MonsterPool, 2)

	// Check giant rat
	rat := findMonster(theme.MonsterPool, "giant-rat")
	s.NotNil(rat)
	s.Equal(RoleMelee, rat.Role)
	s.Equal(0.125, rat.CR)

	// Check giant spider
	spider := findMonster(theme.MonsterPool, "giant-spider")
	s.NotNil(spider)
	s.Equal(RoleMelee, spider.Role)
	s.Equal(0.5, spider.CR)

	// Verify boss pool
	s.Len(theme.BossPool, 1)
	boss := theme.BossPool[0]
	s.Equal("giant-wolf-spider", boss.ID)
	s.Equal(RoleBoss, boss.Role)
	s.Equal(1.0, boss.CR)
}

func (s *ThemeTestSuite) TestThemeBanditLair() {
	theme := ThemeBanditLair

	// Verify theme metadata
	s.Equal("bandit-lair", theme.ID)
	s.Equal("Bandit Hideout", theme.Name)
	s.Equal(ShapeStyleMixed, theme.ShapeStyle)

	// Verify feature rules
	s.Equal(0.5, theme.Features.ObstacleChance)
	s.Equal(0.0, theme.Features.TerrainChance)

	// Verify obstacle types match bandit lair theme
	s.Len(theme.Features.ObstacleTypes, 2)
	s.Contains(theme.Features.ObstacleTypes, ObstacleTypeCrate)
	s.Contains(theme.Features.ObstacleTypes, ObstacleTypeBarrel)

	// Verify terrain types (should be empty for bandit lair)
	s.Empty(theme.Features.TerrainTypes)

	// Verify monster pool
	s.Len(theme.MonsterPool, 2)

	// Check bandit
	bandit := findMonster(theme.MonsterPool, "bandit")
	s.NotNil(bandit)
	s.Equal(RoleMelee, bandit.Role)
	s.Equal(0.125, bandit.CR)

	// Check bandit archer
	archer := findMonster(theme.MonsterPool, "bandit-archer")
	s.NotNil(archer)
	s.Equal(RoleRanged, archer.Role)
	s.Equal(0.25, archer.CR)

	// Verify boss pool
	s.Len(theme.BossPool, 1)
	boss := theme.BossPool[0]
	s.Equal("bandit-captain", boss.ID)
	s.Equal(RoleBoss, boss.Role)
	s.Equal(2.0, boss.CR)
}

func (s *ThemeTestSuite) TestThemeShapeStyles() {
	// Verify each theme has the correct shape style
	s.Equal(ShapeStyleStructured, ThemeCrypt.ShapeStyle, "Crypt should use structured shapes")
	s.Equal(ShapeStyleOrganic, ThemeCave.ShapeStyle, "Cave should use organic shapes")
	s.Equal(ShapeStyleMixed, ThemeBanditLair.ShapeStyle, "Bandit lair should use mixed shapes")
}

func (s *ThemeTestSuite) TestThemeUniqueIDs() {
	// Verify each theme has a unique ID
	ids := map[string]bool{
		ThemeCrypt.ID:      true,
		ThemeCave.ID:       true,
		ThemeBanditLair.ID: true,
	}
	s.Len(ids, 3, "All theme IDs should be unique")
}

func (s *ThemeTestSuite) TestAllThemesHaveBosses() {
	// Verify all themes have at least one boss
	themes := []Theme{ThemeCrypt, ThemeCave, ThemeBanditLair}

	for _, theme := range themes {
		s.NotEmpty(theme.BossPool, "Theme %s should have at least one boss", theme.ID)

		// Verify all bosses have RoleBoss
		for _, boss := range theme.BossPool {
			s.Equal(RoleBoss, boss.Role, "Boss %s in theme %s should have RoleBoss", boss.ID, theme.ID)
		}
	}
}

func (s *ThemeTestSuite) TestAllThemesHaveMonsters() {
	// Verify all themes have monsters
	themes := []Theme{ThemeCrypt, ThemeCave, ThemeBanditLair}

	for _, theme := range themes {
		s.NotEmpty(theme.MonsterPool, "Theme %s should have at least one monster", theme.ID)

		// Verify no regular monsters have RoleBoss
		for _, monster := range theme.MonsterPool {
			s.NotEqual(RoleBoss, monster.Role, "Regular monster %s in theme %s should not have RoleBoss", monster.ID, theme.ID)
		}
	}
}

func (s *ThemeTestSuite) TestAllThemesHaveObstacles() {
	// Verify all themes have obstacle configuration
	themes := []Theme{ThemeCrypt, ThemeCave, ThemeBanditLair}

	for _, theme := range themes {
		s.Greater(theme.Features.ObstacleChance, 0.0, "Theme %s should have positive obstacle chance", theme.ID)
		s.NotEmpty(theme.Features.ObstacleTypes, "Theme %s should have at least one obstacle type", theme.ID)
	}
}

func (s *ThemeTestSuite) TestFeatureRulesValidation() {
	// Verify feature rules have valid probability ranges
	themes := []Theme{ThemeCrypt, ThemeCave, ThemeBanditLair}

	for _, theme := range themes {
		s.GreaterOrEqual(theme.Features.ObstacleChance, 0.0, "Theme %s obstacle chance should be >= 0", theme.ID)
		s.LessOrEqual(theme.Features.ObstacleChance, 1.0, "Theme %s obstacle chance should be <= 1", theme.ID)

		s.GreaterOrEqual(theme.Features.TerrainChance, 0.0, "Theme %s terrain chance should be >= 0", theme.ID)
		s.LessOrEqual(theme.Features.TerrainChance, 1.0, "Theme %s terrain chance should be <= 1", theme.ID)
	}
}

func (s *ThemeTestSuite) TestMonsterCRValues() {
	// Verify all monsters have positive CR values
	themes := []Theme{ThemeCrypt, ThemeCave, ThemeBanditLair}

	for _, theme := range themes {
		// Check monster pool
		for _, monster := range theme.MonsterPool {
			s.Greater(monster.CR, 0.0, "Monster %s in theme %s should have positive CR", monster.ID, theme.ID)
		}

		// Check boss pool
		for _, boss := range theme.BossPool {
			s.Greater(boss.CR, 0.0, "Boss %s in theme %s should have positive CR", boss.ID, theme.ID)
		}
	}
}

func (s *ThemeTestSuite) TestBossesHaveHigherCR() {
	// Verify bosses have higher CR than regular monsters
	themes := []Theme{ThemeCrypt, ThemeCave, ThemeBanditLair}

	for _, theme := range themes {
		// Find max CR in monster pool
		var maxMonsterCR float64
		for _, monster := range theme.MonsterPool {
			if monster.CR > maxMonsterCR {
				maxMonsterCR = monster.CR
			}
		}

		// Verify all bosses have CR higher than any regular monster
		for _, boss := range theme.BossPool {
			s.Greater(boss.CR, maxMonsterCR, "Boss %s in theme %s should have higher CR than regular monsters", boss.ID, theme.ID)
		}
	}
}

// Helper function to find a monster by ID
func findMonster(pool []MonsterRef, id string) *MonsterRef {
	for _, m := range pool {
		if m.ID == id {
			return &m
		}
	}
	return nil
}

// TestMonsterRefStruct verifies the MonsterRef structure
func TestMonsterRefStruct(t *testing.T) {
	monster := MonsterRef{
		ID:   "test-monster",
		Role: RoleMelee,
		CR:   1.5,
	}

	assert.Equal(t, "test-monster", monster.ID)
	assert.Equal(t, RoleMelee, monster.Role)
	assert.Equal(t, 1.5, monster.CR)
}

// TestFeatureRulesStruct verifies the FeatureRules structure
func TestFeatureRulesStruct(t *testing.T) {
	rules := FeatureRules{
		ObstacleChance: 0.7,
		ObstacleTypes:  []ObstacleType{ObstacleTypePillar, ObstacleTypeCrate},
		TerrainChance:  0.3,
		TerrainTypes:   []TerrainType{TerrainTypeDifficult},
	}

	assert.Equal(t, 0.7, rules.ObstacleChance)
	assert.Len(t, rules.ObstacleTypes, 2)
	assert.Equal(t, 0.3, rules.TerrainChance)
	assert.Len(t, rules.TerrainTypes, 1)
}

// TestThemeStruct verifies the Theme structure
func TestThemeStruct(t *testing.T) {
	theme := Theme{
		ID:         "test-theme",
		Name:       "Test Theme",
		ShapeStyle: ShapeStyleStructured,
		Features: FeatureRules{
			ObstacleChance: 0.5,
			ObstacleTypes:  []ObstacleType{ObstacleTypePillar},
		},
		MonsterPool: []MonsterRef{
			{ID: "monster1", Role: RoleMelee, CR: 0.5},
		},
		BossPool: []MonsterRef{
			{ID: "boss1", Role: RoleBoss, CR: 2.0},
		},
	}

	assert.Equal(t, "test-theme", theme.ID)
	assert.Equal(t, "Test Theme", theme.Name)
	assert.Equal(t, ShapeStyleStructured, theme.ShapeStyle)
	assert.Len(t, theme.MonsterPool, 1)
	assert.Len(t, theme.BossPool, 1)
}
