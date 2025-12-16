package encounter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
)

type GeneratorTestSuite struct {
	suite.Suite
	generator EncounterGenerator
}

func TestGeneratorTestSuite(t *testing.T) {
	suite.Run(t, new(GeneratorTestSuite))
}

func (s *GeneratorTestSuite) SetupTest() {
	s.generator = NewGenerator()
}

func (s *GeneratorTestSuite) TestGenerate_Success() {
	input := &GenerateInput{
		MonsterPool: []dungeon.MonsterRef{
			{ID: "skeleton", Role: dungeon.RoleMelee, CR: 0.25},
			{ID: "zombie", Role: dungeon.RoleMelee, CR: 0.25},
		},
		BossPool: []dungeon.MonsterRef{
			{ID: "skeleton-captain", Role: dungeon.RoleBoss, CR: 2},
		},
		CRBudget:   2.0,
		IsBossRoom: false,
		Seed:       12345,
	}

	output, err := s.generator.Generate(context.Background(), input)

	s.NoError(err)
	s.NotNil(output)
	s.NotNil(output.Encounter)
	s.Greater(len(output.Encounter.Monsters), 0)
	s.Greater(output.Encounter.TotalCR, 0.0)
	s.LessOrEqual(output.Encounter.TotalCR, input.CRBudget)
}

func (s *GeneratorTestSuite) TestGenerate_BossRoom() {
	input := &GenerateInput{
		MonsterPool: []dungeon.MonsterRef{
			{ID: "skeleton", Role: dungeon.RoleMelee, CR: 0.25},
		},
		BossPool: []dungeon.MonsterRef{
			{ID: "skeleton-captain", Role: dungeon.RoleBoss, CR: 2},
		},
		CRBudget:   5.0,
		IsBossRoom: true,
		Seed:       12345,
	}

	output, err := s.generator.Generate(context.Background(), input)

	s.NoError(err)
	s.NotNil(output)
	s.NotNil(output.Encounter)

	// Verify boss is present
	hasBoss := false
	for _, monster := range output.Encounter.Monsters {
		if monster.Role == dungeon.RoleBoss {
			hasBoss = true
			s.Equal("skeleton-captain", monster.MonsterID)
		}
	}
	s.True(hasBoss, "Boss room should contain a boss monster")
}

func (s *GeneratorTestSuite) TestGenerate_NilInput() {
	output, err := s.generator.Generate(context.Background(), nil)

	s.Error(err)
	s.Nil(output)
	s.Contains(err.Error(), "input is required")
}

func (s *GeneratorTestSuite) TestGenerate_ZeroBudget() {
	input := &GenerateInput{
		MonsterPool: []dungeon.MonsterRef{
			{ID: "skeleton", Role: dungeon.RoleMelee, CR: 0.25},
		},
		CRBudget: 0,
		Seed:     12345,
	}

	output, err := s.generator.Generate(context.Background(), input)

	s.Error(err)
	s.Nil(output)
	s.Contains(err.Error(), "CRBudget must be greater than 0")
}

func (s *GeneratorTestSuite) TestGenerate_NegativeBudget() {
	input := &GenerateInput{
		MonsterPool: []dungeon.MonsterRef{
			{ID: "skeleton", Role: dungeon.RoleMelee, CR: 0.25},
		},
		CRBudget: -1.0,
		Seed:     12345,
	}

	output, err := s.generator.Generate(context.Background(), input)

	s.Error(err)
	s.Nil(output)
	s.Contains(err.Error(), "CRBudget must be greater than 0")
}

func (s *GeneratorTestSuite) TestGenerate_EmptyMonsterPool() {
	input := &GenerateInput{
		MonsterPool: []dungeon.MonsterRef{},
		CRBudget:    2.0,
		Seed:        12345,
	}

	output, err := s.generator.Generate(context.Background(), input)

	s.Error(err)
	s.Nil(output)
	s.Contains(err.Error(), "MonsterPool cannot be empty")
}

func (s *GeneratorTestSuite) TestGenerate_BossRoomWithoutBossPool() {
	input := &GenerateInput{
		MonsterPool: []dungeon.MonsterRef{
			{ID: "skeleton", Role: dungeon.RoleMelee, CR: 0.25},
		},
		BossPool:   []dungeon.MonsterRef{},
		CRBudget:   5.0,
		IsBossRoom: true,
		Seed:       12345,
	}

	output, err := s.generator.Generate(context.Background(), input)

	s.Error(err)
	s.Nil(output)
	s.Contains(err.Error(), "BossPool cannot be empty for boss room")
}

func (s *GeneratorTestSuite) TestGenerate_Deterministic() {
	input := &GenerateInput{
		MonsterPool: []dungeon.MonsterRef{
			{ID: "skeleton", Role: dungeon.RoleMelee, CR: 0.25},
			{ID: "zombie", Role: dungeon.RoleMelee, CR: 0.25},
		},
		BossPool: []dungeon.MonsterRef{
			{ID: "skeleton-captain", Role: dungeon.RoleBoss, CR: 2},
		},
		CRBudget:   2.0,
		IsBossRoom: false,
		Seed:       12345,
	}

	// Generate twice with same seed
	output1, err1 := s.generator.Generate(context.Background(), input)
	output2, err2 := s.generator.Generate(context.Background(), input)

	s.NoError(err1)
	s.NoError(err2)
	s.Equal(len(output1.Encounter.Monsters), len(output2.Encounter.Monsters))
	s.Equal(output1.Encounter.TotalCR, output2.Encounter.TotalCR)

	// Verify same monsters in same order
	for i := range output1.Encounter.Monsters {
		s.Equal(output1.Encounter.Monsters[i].MonsterID, output2.Encounter.Monsters[i].MonsterID)
		s.Equal(output1.Encounter.Monsters[i].Role, output2.Encounter.Monsters[i].Role)
		s.Equal(output1.Encounter.Monsters[i].CR, output2.Encounter.Monsters[i].CR)
	}
}
