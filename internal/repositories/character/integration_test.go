//go:build integration
// +build integration

package character_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"

	redisclient "github.com/KirkDiggler/rpg-api/internal/redis"
	"github.com/KirkDiggler/rpg-api/internal/repositories/character"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/constants"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

type IntegrationTestSuite struct {
	suite.Suite
	miniRedis *miniredis.Miniredis
	client    redisclient.Client
	repo      character.Repository
	ctx       context.Context
}

func (s *IntegrationTestSuite) SetupTest() {
	// Create miniredis server
	mr, err := miniredis.Run()
	s.Require().NoError(err)
	s.miniRedis = mr

	// Create redis client
	redisClient := redis.NewClient(&redis.Options{
		Addr: s.miniRedis.Addr(),
	})
	s.client = redisClient

	// Create repository
	repo, err := character.NewRedis(&character.RedisConfig{
		Client: s.client,
	})
	s.Require().NoError(err)
	s.repo = repo

	s.ctx = context.Background()
}

func (s *IntegrationTestSuite) TearDownTest() {
	s.miniRedis.Close()
}

func (s *IntegrationTestSuite) TestFullCharacterLifecycle() {
	// Create test character data
	charData := s.createTestCharacterData("char_001", "player_001", "Aragorn")

	// Test Create
	createOut, err := s.repo.Create(s.ctx, character.CreateInput{
		CharacterData: charData,
	})
	s.Require().NoError(err)
	s.NotNil(createOut)
	s.Equal(charData, createOut.CharacterData)

	// Verify in Redis
	key := "character:char_001"
	s.True(s.miniRedis.Exists(key))

	// Test Get
	getOut, err := s.repo.Get(s.ctx, character.GetInput{
		ID: "char_001",
	})
	s.Require().NoError(err)
	s.NotNil(getOut)
	s.Equal("Aragorn", getOut.CharacterData.Name)
	s.Equal(16, getOut.CharacterData.AbilityScores[constants.STR])

	// Test Update - level up
	charData.Level = 2
	charData.Experience = 300
	charData.HitPoints = 20
	charData.MaxHitPoints = 20

	updateOut, err := s.repo.Update(s.ctx, character.UpdateInput{
		CharacterData: charData,
	})
	s.Require().NoError(err)
	s.NotNil(updateOut)

	// Verify update
	getOut2, err := s.repo.Get(s.ctx, character.GetInput{
		ID: "char_001",
	})
	s.Require().NoError(err)
	s.Equal(2, getOut2.CharacterData.Level)
	s.Equal(300, getOut2.CharacterData.Experience)
	s.Equal(20, getOut2.CharacterData.HitPoints)

	// Test Delete
	deleteOut, err := s.repo.Delete(s.ctx, character.DeleteInput{
		ID: "char_001",
	})
	s.Require().NoError(err)
	s.NotNil(deleteOut)

	// Verify deletion
	s.False(s.miniRedis.Exists(key))

	// Get should fail now
	_, err = s.repo.Get(s.ctx, character.GetInput{
		ID: "char_001",
	})
	s.Error(err)
	s.Contains(err.Error(), "not found")
}

func (s *IntegrationTestSuite) TestListByPlayerID() {
	// Create multiple characters for same player
	player := "player_001"

	char1 := s.createTestCharacterData("char_001", player, "Aragorn")
	char2 := s.createTestCharacterData("char_002", player, "Legolas")
	char3 := s.createTestCharacterData("char_003", "player_002", "Gimli") // Different player

	// Create all characters
	_, err := s.repo.Create(s.ctx, character.CreateInput{CharacterData: char1})
	s.Require().NoError(err)
	_, err = s.repo.Create(s.ctx, character.CreateInput{CharacterData: char2})
	s.Require().NoError(err)
	_, err = s.repo.Create(s.ctx, character.CreateInput{CharacterData: char3})
	s.Require().NoError(err)

	// List by player
	listOut, err := s.repo.ListByPlayerID(s.ctx, character.ListByPlayerIDInput{
		PlayerID: player,
	})
	s.Require().NoError(err)
	s.Len(listOut.Characters, 2)

	// Verify we got the right characters
	names := make(map[string]bool)
	for _, char := range listOut.Characters {
		names[char.Name] = true
		s.Equal(player, char.PlayerID)
	}
	s.True(names["Aragorn"])
	s.True(names["Legolas"])
	s.False(names["Gimli"])
}

func (s *IntegrationTestSuite) TestIndexCleanup() {
	// Create character
	charData := s.createTestCharacterData("char_001", "player_001", "Aragorn")
	_, err := s.repo.Create(s.ctx, character.CreateInput{
		CharacterData: charData,
	})
	s.Require().NoError(err)

	// Manually delete character data but leave index
	key := "character:char_001"
	s.miniRedis.Del(key)

	// List should handle missing character gracefully
	listOut, err := s.repo.ListByPlayerID(s.ctx, character.ListByPlayerIDInput{
		PlayerID: "player_001",
	})
	s.Require().NoError(err)
	s.Len(listOut.Characters, 0) // Should be empty after cleanup

	// Index should be cleaned up
	indexKey := "character:player:player_001"
	members, _ := s.miniRedis.SMembers(indexKey)
	s.Len(members, 0)
}

func (s *IntegrationTestSuite) TestConcurrentAccess() {
	// Test concurrent creates don't conflict
	charData := s.createTestCharacterData("char_001", "player_001", "Aragorn")

	// First create should succeed
	_, err := s.repo.Create(s.ctx, character.CreateInput{
		CharacterData: charData,
	})
	s.Require().NoError(err)

	// Second create should fail with already exists
	_, err = s.repo.Create(s.ctx, character.CreateInput{
		CharacterData: charData,
	})
	s.Error(err)
	s.Contains(err.Error(), "already exists")
}

// Helper to create test character data
func (s *IntegrationTestSuite) createTestCharacterData(id, playerID, name string) *toolkitchar.Data {
	// Use fixed time for consistent tests
	testTime := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	return &toolkitchar.Data{
		ID:           id,
		PlayerID:     playerID,
		Name:         name,
		Level:        1,
		Experience:   0,
		RaceID:       "human",
		ClassID:      "ranger",
		BackgroundID: "outlander",
		AbilityScores: shared.AbilityScores{
			constants.STR: 16,
			constants.DEX: 14,
			constants.CON: 13,
			constants.INT: 12,
			constants.WIS: 15,
			constants.CHA: 10,
		},
		HitPoints:    11,
		MaxHitPoints: 11,
		Skills: map[string]int{
			"survival": 1,
			"nature":   1,
		},
		SavingThrows: map[string]int{
			"strength":  1,
			"dexterity": 1,
		},
		Languages: []string{"Common", "Elvish"},
		Proficiencies: shared.Proficiencies{
			Armor:   []string{"light", "medium", "shields"},
			Weapons: []string{"simple", "martial"},
			Tools:   []string{"herbalism kit"},
		},
		CreatedAt: testTime,
		UpdatedAt: testTime,
	}
}

func TestIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(IntegrationTestSuite))
}
