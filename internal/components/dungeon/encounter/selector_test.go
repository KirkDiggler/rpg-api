package encounter

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
)

type SelectorTestSuite struct {
	suite.Suite
	selector MonsterSelector
}

func TestSelectorTestSuite(t *testing.T) {
	suite.Run(t, new(SelectorTestSuite))
}

func (s *SelectorTestSuite) SetupTest() {
	s.selector = NewMonsterSelector()
}

func (s *SelectorTestSuite) TestSelectMonsters_LowCR() {
	pool := []dungeon.MonsterRef{
		{ID: "skeleton", Role: dungeon.RoleMelee, CR: 0.25},
		{ID: "zombie", Role: dungeon.RoleMelee, CR: 0.25},
		{ID: "skeleton-archer", Role: dungeon.RoleRanged, CR: 0.25},
	}

	input := &SelectInput{
		Pool:   pool,
		Budget: 2.0,
		IsBoss: false,
		Seed:   12345,
	}

	output := s.selector.SelectMonsters(input)

	s.NotNil(output)
	s.Greater(len(output.Monsters), 0)
	s.LessOrEqual(output.TotalCR, input.Budget)

	// Low CR should have mostly melee (70%), some ranged (30%)
	meleeCount := 0
	rangedCount := 0
	for _, monster := range output.Monsters {
		switch monster.Role {
		case dungeon.RoleMelee:
			meleeCount++
		case dungeon.RoleRanged:
			rangedCount++
		}
	}

	s.Greater(meleeCount, 0, "Low CR should have melee monsters")
}

func (s *SelectorTestSuite) TestSelectMonsters_MediumCR() {
	pool := []dungeon.MonsterRef{
		{ID: "skeleton", Role: dungeon.RoleMelee, CR: 0.25},
		{ID: "skeleton-archer", Role: dungeon.RoleRanged, CR: 0.25},
		{ID: "necromancer", Role: dungeon.RoleSupport, CR: 0.5},
	}

	input := &SelectInput{
		Pool:   pool,
		Budget: 4.0,
		IsBoss: false,
		Seed:   12345,
	}

	output := s.selector.SelectMonsters(input)

	s.NotNil(output)
	s.Greater(len(output.Monsters), 0)
	s.LessOrEqual(output.TotalCR, input.Budget)

	// Medium CR (2-5) should have melee (50%), ranged (30%), support (20%)
	meleeCount := 0
	rangedCount := 0
	supportCount := 0
	for _, monster := range output.Monsters {
		switch monster.Role {
		case dungeon.RoleMelee:
			meleeCount++
		case dungeon.RoleRanged:
			rangedCount++
		case dungeon.RoleSupport:
			supportCount++
		}
	}

	s.Greater(meleeCount, 0, "Medium CR should have melee monsters")
}

func (s *SelectorTestSuite) TestSelectMonsters_HighCR() {
	pool := []dungeon.MonsterRef{
		{ID: "skeleton-warrior", Role: dungeon.RoleMelee, CR: 1.0},
		{ID: "skeleton-archer", Role: dungeon.RoleRanged, CR: 1.0},
		{ID: "necromancer", Role: dungeon.RoleSupport, CR: 1.0},
	}

	input := &SelectInput{
		Pool:   pool,
		Budget: 6.0,
		IsBoss: false,
		Seed:   12345,
	}

	output := s.selector.SelectMonsters(input)

	s.NotNil(output)
	s.Greater(len(output.Monsters), 0)
	s.LessOrEqual(output.TotalCR, input.Budget)

	// High CR (>5) should have melee (40%), ranged (30%), support (30%)
	meleeCount := 0
	rangedCount := 0
	supportCount := 0
	for _, monster := range output.Monsters {
		switch monster.Role {
		case dungeon.RoleMelee:
			meleeCount++
		case dungeon.RoleRanged:
			rangedCount++
		case dungeon.RoleSupport:
			supportCount++
		}
	}

	s.Greater(meleeCount, 0, "High CR should have melee monsters")
}

func (s *SelectorTestSuite) TestSelectMonsters_BossEncounter() {
	pool := []dungeon.MonsterRef{
		{ID: "skeleton", Role: dungeon.RoleMelee, CR: 0.25},
		{ID: "zombie", Role: dungeon.RoleMelee, CR: 0.25},
	}

	bossPool := []dungeon.MonsterRef{
		{ID: "skeleton-captain", Role: dungeon.RoleBoss, CR: 2.0},
	}

	input := &SelectInput{
		Pool:     pool,
		BossPool: bossPool,
		Budget:   5.0,
		IsBoss:   true,
		Seed:     12345,
	}

	output := s.selector.SelectMonsters(input)

	s.NotNil(output)
	s.Greater(len(output.Monsters), 0)
	s.LessOrEqual(output.TotalCR, input.Budget)

	// Verify boss is present
	hasBoss := false
	for _, monster := range output.Monsters {
		if monster.Role == dungeon.RoleBoss {
			hasBoss = true
			s.Equal("skeleton-captain", monster.MonsterID)
			s.Equal(2.0, monster.CR)
		}
	}
	s.True(hasBoss, "Boss encounter should contain boss monster")

	// Verify minions are present
	minionCount := 0
	for _, monster := range output.Monsters {
		if monster.Role == dungeon.RoleMelee {
			minionCount++
		}
	}
	s.Greater(minionCount, 0, "Boss encounter should have minions")
}

func (s *SelectorTestSuite) TestSelectMonsters_NilInput() {
	output := s.selector.SelectMonsters(nil)

	s.NotNil(output)
	s.Empty(output.Monsters)
	s.Equal(0.0, output.TotalCR)
}

func (s *SelectorTestSuite) TestSelectMonsters_EmptyPool() {
	input := &SelectInput{
		Pool:   []dungeon.MonsterRef{},
		Budget: 2.0,
		IsBoss: false,
		Seed:   12345,
	}

	output := s.selector.SelectMonsters(input)

	s.NotNil(output)
	s.Empty(output.Monsters)
	s.Equal(0.0, output.TotalCR)
}

func (s *SelectorTestSuite) TestSelectMonsters_BossWithoutBossPool() {
	pool := []dungeon.MonsterRef{
		{ID: "skeleton", Role: dungeon.RoleMelee, CR: 0.25},
	}

	input := &SelectInput{
		Pool:     pool,
		BossPool: []dungeon.MonsterRef{},
		Budget:   5.0,
		IsBoss:   true,
		Seed:     12345,
	}

	output := s.selector.SelectMonsters(input)

	s.NotNil(output)
	s.Empty(output.Monsters)
	s.Equal(0.0, output.TotalCR)
}

func (s *SelectorTestSuite) TestSelectMonsters_Deterministic() {
	pool := []dungeon.MonsterRef{
		{ID: "skeleton", Role: dungeon.RoleMelee, CR: 0.25},
		{ID: "zombie", Role: dungeon.RoleMelee, CR: 0.25},
		{ID: "skeleton-archer", Role: dungeon.RoleRanged, CR: 0.25},
	}

	input := &SelectInput{
		Pool:   pool,
		Budget: 2.0,
		IsBoss: false,
		Seed:   12345,
	}

	// Select twice with same seed
	output1 := s.selector.SelectMonsters(input)
	output2 := s.selector.SelectMonsters(input)

	s.Equal(len(output1.Monsters), len(output2.Monsters))
	s.Equal(output1.TotalCR, output2.TotalCR)

	// Verify same monsters in same order
	for i := range output1.Monsters {
		s.Equal(output1.Monsters[i].MonsterID, output2.Monsters[i].MonsterID)
		s.Equal(output1.Monsters[i].Role, output2.Monsters[i].Role)
		s.Equal(output1.Monsters[i].CR, output2.Monsters[i].CR)
	}
}

func (s *SelectorTestSuite) TestSelectMonsters_DifferentSeeds() {
	pool := []dungeon.MonsterRef{
		{ID: "skeleton", Role: dungeon.RoleMelee, CR: 0.25},
		{ID: "zombie", Role: dungeon.RoleMelee, CR: 0.25},
		{ID: "skeleton-archer", Role: dungeon.RoleRanged, CR: 0.25},
	}

	input1 := &SelectInput{
		Pool:   pool,
		Budget: 2.0,
		IsBoss: false,
		Seed:   12345,
	}

	input2 := &SelectInput{
		Pool:   pool,
		Budget: 2.0,
		IsBoss: false,
		Seed:   67890,
	}

	output1 := s.selector.SelectMonsters(input1)
	output2 := s.selector.SelectMonsters(input2)

	// Different seeds should potentially produce different results
	// Note: might be same by chance, but we can at least test both work
	s.NotNil(output1)
	s.NotNil(output2)
	s.Greater(len(output1.Monsters), 0)
	s.Greater(len(output2.Monsters), 0)
}

func (s *SelectorTestSuite) TestSelectMonsters_BudgetRespected() {
	pool := []dungeon.MonsterRef{
		{ID: "goblin", Role: dungeon.RoleMelee, CR: 0.25},
		{ID: "hobgoblin", Role: dungeon.RoleMelee, CR: 0.5},
	}

	input := &SelectInput{
		Pool:   pool,
		Budget: 1.0,
		IsBoss: false,
		Seed:   12345,
	}

	output := s.selector.SelectMonsters(input)

	s.NotNil(output)
	s.LessOrEqual(output.TotalCR, input.Budget)
	s.Greater(output.TotalCR, 0.0)
}

func (s *SelectorTestSuite) TestSelectMonsters_MonsterIDsUnique() {
	pool := []dungeon.MonsterRef{
		{ID: "skeleton", Role: dungeon.RoleMelee, CR: 0.25},
	}

	input := &SelectInput{
		Pool:   pool,
		Budget: 2.0,
		IsBoss: false,
		Seed:   12345,
	}

	output := s.selector.SelectMonsters(input)

	s.NotNil(output)
	s.Greater(len(output.Monsters), 0)

	// Verify all placement IDs are unique
	idMap := make(map[string]bool)
	for _, monster := range output.Monsters {
		s.NotEmpty(monster.ID)
		s.False(idMap[monster.ID], "Duplicate monster placement ID: %s", monster.ID)
		idMap[monster.ID] = true
	}
}

func (s *SelectorTestSuite) TestFilterByRole() {
	pool := []dungeon.MonsterRef{
		{ID: "skeleton", Role: dungeon.RoleMelee, CR: 0.25},
		{ID: "zombie", Role: dungeon.RoleMelee, CR: 0.25},
		{ID: "skeleton-archer", Role: dungeon.RoleRanged, CR: 0.25},
		{ID: "necromancer", Role: dungeon.RoleSupport, CR: 0.5},
	}

	melee := filterByRole(pool, dungeon.RoleMelee)
	ranged := filterByRole(pool, dungeon.RoleRanged)
	support := filterByRole(pool, dungeon.RoleSupport)
	boss := filterByRole(pool, dungeon.RoleBoss)

	s.Equal(2, len(melee))
	s.Equal(1, len(ranged))
	s.Equal(1, len(support))
	s.Equal(0, len(boss))
}

func (s *SelectorTestSuite) TestMonsterRoleString() {
	s.Equal("melee", monsterRoleString(dungeon.RoleMelee))
	s.Equal("ranged", monsterRoleString(dungeon.RoleRanged))
	s.Equal("support", monsterRoleString(dungeon.RoleSupport))
	s.Equal("boss", monsterRoleString(dungeon.RoleBoss))
	s.Equal("unknown", monsterRoleString(dungeon.MonsterRole(999)))
}

func (s *SelectorTestSuite) TestGenerateMonsterID() {
	id1 := generateMonsterID("melee", 0)
	id2 := generateMonsterID("melee", 1)
	id3 := generateMonsterID("ranged", 0)

	s.Equal("melee-0", id1)
	s.Equal("melee-1", id2)
	s.Equal("ranged-0", id3)
}
