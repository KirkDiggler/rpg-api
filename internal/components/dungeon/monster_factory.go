// Package dungeon provides procedural dungeon generation with themed rooms and encounters.
package dungeon

import (
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster/monsters"
)

// MonsterFactory creates monsters based on their ID from theme monster pools
type MonsterFactory struct{}

// NewMonsterFactory creates a new monster factory
func NewMonsterFactory() *MonsterFactory {
	return &MonsterFactory{}
}

// CreateMonster creates a monster instance based on the monsterID from the theme
// Falls back to Goblin for unknown monster types
func (f *MonsterFactory) CreateMonster(id, monsterID string) *monster.Monster {
	switch monsterID {
	// Undead (Crypt theme)
	case "skeleton", "skeleton-archer":
		return monsters.NewSkeleton(id)
	case "zombie":
		return monsters.NewZombie(id)
	case "skeleton-captain":
		// Use Ghoul as a stronger undead for boss (CR 1)
		return monsters.NewGhoul(id)
	case "ghoul":
		return monsters.NewGhoul(id)

	// Beasts (Cave theme)
	case "giant-rat":
		return monsters.NewGiantRat(id)
	case "giant-spider":
		// Use Wolf as similar CR beast
		return monsters.NewWolf(id)
	case "giant-wolf-spider":
		// Use Brown Bear for cave boss (CR 1)
		return monsters.NewBrownBear(id)
	case "wolf":
		return monsters.NewWolf(id)
	case "brown-bear":
		return monsters.NewBrownBear(id)

	// Humanoids (Bandit Lair theme)
	case "bandit":
		return monsters.NewBanditMelee(id)
	case "bandit-archer":
		return monsters.NewBanditRanged(id)
	case "bandit-captain":
		// Use Thug for bandit boss (CR 1/2)
		return monsters.NewThug(id)
	case "thug":
		return monsters.NewThug(id)

	// Fallback to goblin for unknown types
	default:
		return monster.NewGoblin(id)
	}
}
