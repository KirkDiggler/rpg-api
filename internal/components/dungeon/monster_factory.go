// Package dungeon provides procedural dungeon generation with themed rooms and encounters.
package dungeon

import (
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster/monsters"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
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
	case refs.Monsters.Skeleton().ID, refs.Monsters.SkeletonArcher().ID:
		return monsters.NewSkeleton(id)
	case refs.Monsters.Zombie().ID:
		return monsters.NewZombie(id)
	case refs.Monsters.SkeletonCaptain().ID:
		// Use Ghoul as a stronger undead for boss (CR 1)
		return monsters.NewGhoul(id)
	case refs.Monsters.Ghoul().ID:
		return monsters.NewGhoul(id)

	// Beasts (Cave theme)
	case refs.Monsters.GiantRat().ID:
		return monsters.NewGiantRat(id)
	case refs.Monsters.GiantSpider().ID:
		// Use Wolf as similar CR beast
		return monsters.NewWolf(id)
	case refs.Monsters.GiantWolfSpider().ID:
		// Use Brown Bear for cave boss (CR 1)
		return monsters.NewBrownBear(id)
	case refs.Monsters.Wolf().ID:
		return monsters.NewWolf(id)
	case refs.Monsters.BrownBear().ID:
		return monsters.NewBrownBear(id)

	// Humanoids (Bandit Lair theme)
	case refs.Monsters.Bandit().ID:
		return monsters.NewBanditMelee(id)
	case refs.Monsters.BanditArcher().ID:
		return monsters.NewBanditRanged(id)
	case refs.Monsters.BanditCaptain().ID:
		// Use Thug for bandit boss (CR 1/2)
		return monsters.NewThug(id)
	case refs.Monsters.Thug().ID:
		return monsters.NewThug(id)

	// Fallback to goblin for unknown types
	default:
		return monster.NewGoblin(id)
	}
}
