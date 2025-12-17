package entities_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-api/internal/entities"
)

type DungeonTestSuite struct {
	suite.Suite
}

func TestDungeonSuite(t *testing.T) {
	suite.Run(t, new(DungeonTestSuite))
}

func (s *DungeonTestSuite) TestIsBossRoom_True() {
	dungeon := &entities.Dungeon{
		ID:         "dng-1",
		BossRoomID: "boss-room",
	}

	s.True(dungeon.IsBossRoom("boss-room"))
}

func (s *DungeonTestSuite) TestIsBossRoom_False() {
	dungeon := &entities.Dungeon{
		ID:         "dng-1",
		BossRoomID: "boss-room",
	}

	s.False(dungeon.IsBossRoom("other-room"))
}

func (s *DungeonTestSuite) TestIsBossRoom_EmptyBossRoomID() {
	dungeon := &entities.Dungeon{
		ID:         "dng-1",
		BossRoomID: "",
	}

	s.False(dungeon.IsBossRoom("any-room"))
}

func (s *DungeonTestSuite) TestMarkVictory() {
	dungeon := &entities.Dungeon{
		ID:    "dng-1",
		State: entities.DungeonStateActive,
	}

	before := time.Now()
	dungeon.MarkVictory()
	after := time.Now()

	s.Equal(entities.DungeonStateVictorious, dungeon.State)
	s.NotNil(dungeon.CompletedAt)
	s.True(dungeon.CompletedAt.After(before) || dungeon.CompletedAt.Equal(before))
	s.True(dungeon.CompletedAt.Before(after) || dungeon.CompletedAt.Equal(after))
}

func (s *DungeonTestSuite) TestMarkVictory_AlreadyCompleted() {
	// Should still update state even if already completed
	existingTime := time.Now().Add(-time.Hour)
	dungeon := &entities.Dungeon{
		ID:          "dng-1",
		State:       entities.DungeonStateFailed,
		CompletedAt: &existingTime,
	}

	dungeon.MarkVictory()

	s.Equal(entities.DungeonStateVictorious, dungeon.State)
	// CompletedAt should be updated to new time
	s.True(dungeon.CompletedAt.After(existingTime))
}

func (s *DungeonTestSuite) TestMarkFailed() {
	dungeon := &entities.Dungeon{
		ID:    "dng-1",
		State: entities.DungeonStateActive,
	}

	before := time.Now()
	dungeon.MarkFailed()
	after := time.Now()

	s.Equal(entities.DungeonStateFailed, dungeon.State)
	s.NotNil(dungeon.CompletedAt)
	s.True(dungeon.CompletedAt.After(before) || dungeon.CompletedAt.Equal(before))
	s.True(dungeon.CompletedAt.Before(after) || dungeon.CompletedAt.Equal(after))
}

func (s *DungeonTestSuite) TestMarkFailed_AlreadyCompleted() {
	existingTime := time.Now().Add(-time.Hour)
	dungeon := &entities.Dungeon{
		ID:          "dng-1",
		State:       entities.DungeonStateVictorious,
		CompletedAt: &existingTime,
	}

	dungeon.MarkFailed()

	s.Equal(entities.DungeonStateFailed, dungeon.State)
	s.True(dungeon.CompletedAt.After(existingTime))
}

// Test existing methods still work
func (s *DungeonTestSuite) TestIsRoomRevealed() {
	dungeon := &entities.Dungeon{
		RevealedRooms: map[string]bool{"room-1": true},
	}

	s.True(dungeon.IsRoomRevealed("room-1"))
	s.False(dungeon.IsRoomRevealed("room-2"))
}

func (s *DungeonTestSuite) TestIsRoomRevealed_NilMap() {
	dungeon := &entities.Dungeon{}

	s.False(dungeon.IsRoomRevealed("any-room"))
}
