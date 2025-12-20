# Monster Type Enum Design

**Date:** 2025-12-19
**Status:** Draft
**Scope:** rpg-api-protos, rpg-toolkit, rpg-api

## Problem

The UI needs to know which texture to render for each monster. Currently:
- `MonsterData.monster_type` is a string (no type safety)
- `HexEntity.tsx` renders ALL monsters as goblins
- No way to differentiate skeleton vs zombie vs bandit visually

## Solution

Add a `MonsterType` proto enum and flow it through the system so the UI can select appropriate textures.

## Design

### 1. Proto Changes (rpg-api-protos)

**New enum in `enums.proto`:**

```protobuf
enum MonsterType {
  MONSTER_TYPE_UNSPECIFIED = 0;

  // Undead (Crypt theme) - 1-9
  MONSTER_TYPE_SKELETON = 1;
  MONSTER_TYPE_ZOMBIE = 2;
  MONSTER_TYPE_SKELETON_ARCHER = 3;
  MONSTER_TYPE_SKELETON_CAPTAIN = 4;
  MONSTER_TYPE_GHOUL = 5;

  // Beasts (Cave theme) - 10-19
  MONSTER_TYPE_GIANT_RAT = 10;
  MONSTER_TYPE_GIANT_SPIDER = 11;
  MONSTER_TYPE_GIANT_WOLF_SPIDER = 12;
  MONSTER_TYPE_WOLF = 13;
  MONSTER_TYPE_BROWN_BEAR = 14;

  // Humanoids (Bandit Lair theme) - 20-29
  MONSTER_TYPE_BANDIT = 20;
  MONSTER_TYPE_BANDIT_ARCHER = 21;
  MONSTER_TYPE_BANDIT_CAPTAIN = 22;
  MONSTER_TYPE_THUG = 23;

  // Fallback - 100+
  MONSTER_TYPE_GOBLIN = 100;
}
```

**Update `MonsterCombatState` in `encounter.proto`:**

```protobuf
message MonsterCombatState {
  string monster_id = 1;
  string monster_name = 2;
  int32 current_hit_points = 3;
  int32 max_hit_points = 4;
  MonsterType monster_type = 5;  // NEW
}
```

### 2. Toolkit Changes (rpg-toolkit)

**New refs package** (`rulebooks/dnd5e/monster/refs/`):

```go
package refs

// MonsterRef identifies a monster type (not an instance)
type MonsterRef struct {
    ID string // e.g., "skeleton" - the type identifier
}

type monsterRefs struct {
    // Undead
    Skeleton        MonsterRef
    Zombie          MonsterRef
    SkeletonArcher  MonsterRef
    SkeletonCaptain MonsterRef
    Ghoul           MonsterRef

    // Beasts
    GiantRat        MonsterRef
    GiantSpider     MonsterRef
    GiantWolfSpider MonsterRef
    Wolf            MonsterRef
    BrownBear       MonsterRef

    // Humanoids
    Bandit          MonsterRef
    BanditArcher    MonsterRef
    BanditCaptain   MonsterRef
    Thug            MonsterRef

    // Fallback
    Goblin          MonsterRef
}

var monsters = &monsterRefs{
    Skeleton:        MonsterRef{ID: "skeleton"},
    Zombie:          MonsterRef{ID: "zombie"},
    SkeletonArcher:  MonsterRef{ID: "skeleton-archer"},
    SkeletonCaptain: MonsterRef{ID: "skeleton-captain"},
    Ghoul:           MonsterRef{ID: "ghoul"},

    GiantRat:        MonsterRef{ID: "giant-rat"},
    GiantSpider:     MonsterRef{ID: "giant-spider"},
    GiantWolfSpider: MonsterRef{ID: "giant-wolf-spider"},
    Wolf:            MonsterRef{ID: "wolf"},
    BrownBear:       MonsterRef{ID: "brown-bear"},

    Bandit:          MonsterRef{ID: "bandit"},
    BanditArcher:    MonsterRef{ID: "bandit-archer"},
    BanditCaptain:   MonsterRef{ID: "bandit-captain"},
    Thug:            MonsterRef{ID: "thug"},

    Goblin:          MonsterRef{ID: "goblin"},
}

func Monsters() *monsterRefs {
    return monsters
}
```

**Update Monster struct** (`rulebooks/dnd5e/monster/monster.go`):

```go
type Monster struct {
    ID        string           // Unique instance ID
    SourceRef *refs.MonsterRef // The type this monster was created from
    Name      string
    // ... rest of fields
}
```

**Update monster constructors** to set SourceRef:

```go
func NewSkeleton(id string) *Monster {
    return &Monster{
        ID:        id,
        SourceRef: &refs.Monsters().Skeleton,
        Name:      "Skeleton",
        // ...
    }
}
```

### 3. API Changes (rpg-api)

**Mapping function** (`internal/components/dungeon/monster_type.go`):

```go
package dungeon

import (
    pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
    "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster/refs"
)

var monsterTypeMap = map[string]pb.MonsterType{
    refs.Monsters().Skeleton.ID:        pb.MonsterType_MONSTER_TYPE_SKELETON,
    refs.Monsters().Zombie.ID:          pb.MonsterType_MONSTER_TYPE_ZOMBIE,
    refs.Monsters().SkeletonArcher.ID:  pb.MonsterType_MONSTER_TYPE_SKELETON_ARCHER,
    refs.Monsters().SkeletonCaptain.ID: pb.MonsterType_MONSTER_TYPE_SKELETON_CAPTAIN,
    refs.Monsters().Ghoul.ID:           pb.MonsterType_MONSTER_TYPE_GHOUL,

    refs.Monsters().GiantRat.ID:        pb.MonsterType_MONSTER_TYPE_GIANT_RAT,
    refs.Monsters().GiantSpider.ID:     pb.MonsterType_MONSTER_TYPE_GIANT_SPIDER,
    refs.Monsters().GiantWolfSpider.ID: pb.MonsterType_MONSTER_TYPE_GIANT_WOLF_SPIDER,
    refs.Monsters().Wolf.ID:            pb.MonsterType_MONSTER_TYPE_WOLF,
    refs.Monsters().BrownBear.ID:       pb.MonsterType_MONSTER_TYPE_BROWN_BEAR,

    refs.Monsters().Bandit.ID:          pb.MonsterType_MONSTER_TYPE_BANDIT,
    refs.Monsters().BanditArcher.ID:    pb.MonsterType_MONSTER_TYPE_BANDIT_ARCHER,
    refs.Monsters().BanditCaptain.ID:   pb.MonsterType_MONSTER_TYPE_BANDIT_CAPTAIN,
    refs.Monsters().Thug.ID:            pb.MonsterType_MONSTER_TYPE_THUG,

    refs.Monsters().Goblin.ID:          pb.MonsterType_MONSTER_TYPE_GOBLIN,
}

// MonsterTypeFromRef returns the proto MonsterType for a monster's source ref
func MonsterTypeFromRef(ref *refs.MonsterRef) pb.MonsterType {
    if ref == nil {
        return pb.MonsterType_MONSTER_TYPE_GOBLIN
    }
    if mt, ok := monsterTypeMap[ref.ID]; ok {
        return mt
    }
    return pb.MonsterType_MONSTER_TYPE_GOBLIN
}
```

**Update theme.go** to use refs instead of magic strings:

```go
import "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster/refs"

var ThemeCrypt = Theme{
    ID:         "crypt",
    Name:       "Ancient Crypt",
    // ...
    MonsterPool: []MonsterRef{
        {ID: refs.Monsters().Skeleton.ID, Role: RoleMelee, CR: 0.25},
        {ID: refs.Monsters().Zombie.ID, Role: RoleMelee, CR: 0.25},
        {ID: refs.Monsters().SkeletonArcher.ID, Role: RoleRanged, CR: 0.25},
    },
    BossPool: []MonsterRef{
        {ID: refs.Monsters().SkeletonCaptain.ID, Role: RoleBoss, CR: 2},
    },
}
```

**Update encounter response building** to include monster type:

```go
// When building MonsterCombatState
combatState := &pb.MonsterCombatState{
    MonsterId:        monster.ID,
    MonsterName:      monster.Name,
    CurrentHitPoints: int32(monster.CurrentHP()),
    MaxHitPoints:     int32(monster.MaxHP()),
    MonsterType:      dungeon.MonsterTypeFromRef(monster.SourceRef),
}
```

## Data Flow

```
Theme (refs.Monsters().Skeleton.ID)
    ↓
MonsterFactory.CreateMonster(instanceID, refID)
    ↓
Monster instance with SourceRef → refs.Monsters().Skeleton
    ↓
Encounter builds MonsterCombatState
    ↓
MonsterTypeFromRef(monster.SourceRef) → MONSTER_TYPE_SKELETON
    ↓
UI receives MonsterType enum → selects skeleton texture
```

## Implementation Order

1. **rpg-api-protos**: Add MonsterType enum and update MonsterCombatState
2. **rpg-toolkit**: Add refs package and update Monster struct + constructors
3. **rpg-api**:
   - Update to new proto version
   - Update to new toolkit version
   - Add monster_type.go mapping
   - Update theme.go to use refs
   - Update encounter response building

## Testing

- Toolkit: Unit tests for refs, verify SourceRef set correctly
- API: Unit tests for MonsterTypeFromRef mapping
- Integration: Verify UI receives correct MonsterType values
