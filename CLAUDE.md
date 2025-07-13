# Claude AI Development Guidelines

## Current Focus
Building rpg-api with outside-in development for AI-driven D&D campaigns.
- Mission: Procedural campaigns tailored to party skills (see docs/mission.md)
- Architecture: Fine-grained services + SDK wrapper (see ADR-002)

## Core Rules

**rpg-api stores data. rpg-toolkit handles rules.**

### Proto Patterns
- Package: `dnd5e.api.v1alpha1` (domain.api.version)
- Service naming: `CharacterService` (not API suffix)
- Each RPC has unique Request/Response types

### Code Patterns
**Always Input/Output types** - Every function at every layer:
```go
// ✅ GOOD
func CreateSession(ctx context.Context, input *CreateSessionInput) (*CreateSessionOutput, error)
```

**Outside-in development**:
1. Handler with mocked service
2. Service with mocked repository  
3. Repository implementation

### Testing
- Uber's gomock (not mockery)
- Testify suites with SetupTest/SetupSubTest
- Real Redis when safe (miniredis)

### Project Structure
```
/internal/
  /handlers/characterv1alpha1/    # Proto version naming
  /services/character/            # Business logic
  /repositories/characters/       # Storage interface
  /entities/                      # Simple structs (no logic)
```

## What We're NOT Doing
- Human DM features (yet)
- PostgreSQL preference
- Generic abstractions
- Business logic in entities

## Workflow
```bash
git checkout -b feat/thing
make pre-commit  # Before every commit
make fix-eof     # Fix newlines
```

## Recent Decisions
- Character creation uses draft pattern
- Services are fine-grained per domain
- Type-safe SDK with per-service versioning