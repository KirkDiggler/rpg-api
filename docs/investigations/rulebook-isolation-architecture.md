# Investigation: Rulebook Isolation Architecture

**Status**: 🔍 Under Investigation  
**Priority**: High  
**Impact**: Architecture-wide  
**Created**: 2025-01-13

## Problem Statement

Currently, our entities are implicitly D&D 5e specific but live in a generic `internal/entities/` package. As we prepare to support multiple RPG systems (Pathfinder, Vampire, Call of Cthulhu, etc.), we need to determine:

1. Should entities be isolated by rulebook?
2. How should repositories handle system-specific vs shared logic?
3. What's the right balance between code reuse and type safety?

## Current State

```
internal/entities/
├── character.go    # Has D&D 5e specific fields (RaceID, ClassID)
├── constants.go    # D&D 5e specific constants
```

This doesn't scale to multiple systems where:
- Pathfinder has Ancestry instead of Race
- Vampire has Clan and Generation
- Call of Cthulhu has Occupation and Sanity

## Proposed Architecture

### 1. Rulebook-Specific Entities

```
internal/entities/
├── core/               # Shared interfaces
│   ├── character.go    # Character interface
│   └── session.go      # Session interface (truly generic)
├── dnd5e/
│   ├── character.go    # D&D 5e specific
│   ├── constants.go    # D&D 5e enums
│   └── draft.go        
└── pathfinder/
    ├── character.go    # Pathfinder specific
    ├── constants.go    # Pathfinder enums
    └── draft.go
```

**Benefits:**
- Type safety between systems
- Clear domain boundaries
- No generic maps or interface{} abuse
- Follows our proto/handler pattern

### 2. Hybrid Repository Approach

```
internal/repositories/
├── core/                      # Shared infrastructure
│   ├── store.go              # Generic storage interface
│   ├── redis/                # Redis implementation
│   └── pagination/           # Shared patterns
├── dnd5e/
│   └── character/
│       ├── repository.go     # D&D-specific interface
│       └── redis.go          # Uses core.Store
└── pathfinder/
    └── character/
        ├── repository.go     # Pathfinder-specific interface
        └── redis.go          # Uses core.Store
```

**Benefits:**
- Repositories are type-safe per system
- Infrastructure code is reused
- System-specific queries possible
- Easy to add new storage backends

### 3. Service Layer Changes

Services would also be system-specific:
```
internal/services/
├── dnd5e/
│   └── character/
└── pathfinder/
    └── character/
```

## Key Decisions Needed

### 1. Entity Isolation Level
- **Option A**: Full isolation (recommended)
- **Option B**: Shared base with system-specific extensions
- **Option C**: Generic entity with metadata maps

### 2. Repository Pattern
- **Option A**: Fully isolated repositories
- **Option B**: Generic repositories with type parameters
- **Option C**: Hybrid approach (recommended)

### 3. Storage Key Strategy
```
# Option A: System prefix
character:dnd5e:{id}
character:pathfinder:{id}

# Option B: Separate databases/prefixes per system
dnd5e:character:{id}
pathfinder:character:{id}
```

### 4. Cross-System Operations
How do we handle:
- Listing all characters for a player across systems?
- Session management that spans systems?
- Shared resources (dice, chat, etc.)?

## Implementation Tasks

1. **Refactor current entities**
   - Move `internal/entities/*` → `internal/entities/dnd5e/*`
   - Create `internal/entities/core/` interfaces
   - Update all imports

2. **Create repository infrastructure**
   - Design `core.Store` interface
   - Implement Redis store with JSON serialization
   - Create pagination helpers

3. **Update service layer**
   - Move services to system-specific packages
   - Update handler imports

4. **Migration strategy**
   - This change should happen BEFORE we implement repositories
   - Current PR (#14) won't be affected much
   - Do this as first task in next session

## Success Criteria

- [ ] Clear separation between systems
- [ ] No type casting between systems
- [ ] Shared infrastructure without shared business logic
- [ ] Easy to add new RPG systems
- [ ] Maintains our Input/Output pattern
- [ ] Follows "explicit over implicit" principle

## Risks & Mitigations

**Risk**: Too much code duplication  
**Mitigation**: Share infrastructure, not business logic

**Risk**: Complex service wiring  
**Mitigation**: Clear package structure, dependency injection

**Risk**: Cross-system queries become hard  
**Mitigation**: Core interfaces for common operations

## Recommendation

Proceed with:
1. **Rulebook-specific entities** for type safety
2. **Hybrid repository approach** for balanced reuse
3. **System prefixes** in storage keys
4. **Core interfaces** for cross-system operations

This architecture will scale cleanly to multiple RPG systems while maintaining type safety and our established patterns.

## Next Steps

1. Create GitHub issue for this investigation
2. Get buy-in on the approach
3. Implement in next session BEFORE repositories
4. Update all documentation with decisions
