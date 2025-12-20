package dungeon

import (
	"github.com/KirkDiggler/rpg-toolkit/core"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"

	pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
)

// monsterTypeMap maps monster ref IDs to proto MonsterType enum values
var monsterTypeMap = map[string]pb.MonsterType{
	// Undead (Crypt theme)
	refs.Monsters.Skeleton().ID:        pb.MonsterType_MONSTER_TYPE_SKELETON,
	refs.Monsters.Zombie().ID:          pb.MonsterType_MONSTER_TYPE_ZOMBIE,
	refs.Monsters.SkeletonArcher().ID:  pb.MonsterType_MONSTER_TYPE_SKELETON_ARCHER,
	refs.Monsters.SkeletonCaptain().ID: pb.MonsterType_MONSTER_TYPE_SKELETON_CAPTAIN,
	refs.Monsters.Ghoul().ID:           pb.MonsterType_MONSTER_TYPE_GHOUL,

	// Beasts (Cave theme)
	refs.Monsters.GiantRat().ID:        pb.MonsterType_MONSTER_TYPE_GIANT_RAT,
	refs.Monsters.GiantSpider().ID:     pb.MonsterType_MONSTER_TYPE_GIANT_SPIDER,
	refs.Monsters.GiantWolfSpider().ID: pb.MonsterType_MONSTER_TYPE_GIANT_WOLF_SPIDER,
	refs.Monsters.Wolf().ID:            pb.MonsterType_MONSTER_TYPE_WOLF,
	refs.Monsters.BrownBear().ID:       pb.MonsterType_MONSTER_TYPE_BROWN_BEAR,

	// Humanoids (Bandit Lair theme)
	refs.Monsters.Bandit().ID:        pb.MonsterType_MONSTER_TYPE_BANDIT,
	refs.Monsters.BanditArcher().ID:  pb.MonsterType_MONSTER_TYPE_BANDIT_ARCHER,
	refs.Monsters.BanditCaptain().ID: pb.MonsterType_MONSTER_TYPE_BANDIT_CAPTAIN,
	refs.Monsters.Thug().ID:          pb.MonsterType_MONSTER_TYPE_THUG,
	refs.Monsters.Goblin().ID:        pb.MonsterType_MONSTER_TYPE_GOBLIN,
}

// MonsterTypeFromRef returns the proto MonsterType for a monster's ref.
// Falls back to GOBLIN for unknown or nil refs.
func MonsterTypeFromRef(ref *core.Ref) pb.MonsterType {
	if ref == nil {
		return pb.MonsterType_MONSTER_TYPE_GOBLIN
	}

	if mt, ok := monsterTypeMap[ref.ID]; ok {
		return mt
	}

	return pb.MonsterType_MONSTER_TYPE_GOBLIN
}

// MonsterTypeFromID returns the proto MonsterType for a monster ID string.
// Falls back to GOBLIN for unknown IDs.
func MonsterTypeFromID(id string) pb.MonsterType {
	if mt, ok := monsterTypeMap[id]; ok {
		return mt
	}

	return pb.MonsterType_MONSTER_TYPE_GOBLIN
}
