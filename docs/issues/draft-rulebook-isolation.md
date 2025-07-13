# Investigation: Implement Rulebook-Specific Entity Architecture

## Background

While implementing the character handler (#6), we discovered that our current entity structure won't scale to multiple RPG systems. Our entities are implicitly D&D 5e specific (with fields like `RaceID`, `ClassID`) but live in generic packages.

## Problem

Different RPG systems have fundamentally different concepts:
- **D&D 5e**: Race, Class, Level, Ability Scores (6)
- **Pathfinder**: Ancestry, Heritage, Class, Level, Ability Scores (6)
- **Vampire**: Clan, Generation, Disciplines, Attributes (9)
- **Call of Cthulhu**: Occupation, Skills, Characteristics (8)

Trying to force these into a single `Character` struct would lead to:
- Messy generic maps
- Loss of type safety
- Unclear domain boundaries
- Difficult testing

## Proposed Solution

Implement rulebook-specific entities with a hybrid repository approach:

### 1. Entity Structure
```
internal/entities/
├── core/               # Shared interfaces only
├── dnd5e/             # D&D 5e specific entities
└── pathfinder/        # Pathfinder specific entities
```

### 2. Repository Structure  
```
internal/repositories/
├── core/              # Shared infrastructure (Store interface, Redis client)
├── dnd5e/            # D&D 5e specific repositories
└── pathfinder/       # Pathfinder specific repositories
```

### 3. Benefits
- ✅ Type safety between systems
- ✅ Clear domain boundaries
- ✅ System-specific optimizations
- ✅ Follows our proto/handler pattern
- ✅ Shared infrastructure without shared business logic

## Tasks

- [ ] Review investigation document: `docs/investigations/rulebook-isolation-architecture.md`
- [ ] Make final architecture decision
- [ ] Refactor current entities from `internal/entities/` to `internal/entities/dnd5e/`
- [ ] Create `internal/entities/core/` interfaces
- [ ] Update all imports in handlers and services
- [ ] Update tests
- [ ] Document the pattern in CLAUDE.md

## Why This Is Priority 0

This MUST be done before implementing repositories (Priority 1) because:
1. Repository interfaces depend on entity types
2. Storage keys will be system-specific
3. Avoiding rework is better than refactoring later

## Success Criteria

- [ ] No generic `interface{}` or `map[string]interface{}` for system-specific data
- [ ] Clear package boundaries between systems
- [ ] Shared session management still works across systems
- [ ] Easy to add a new RPG system by following the pattern

## Labels
- `investigation`
- `architecture` 
- `breaking-change`
- `priority-0`

## References
- Investigation doc: `docs/investigations/rulebook-isolation-architecture.md`
- Current handler PR: #14
- Milestone: Consider adding to Milestone 1 as a blocker for repository work