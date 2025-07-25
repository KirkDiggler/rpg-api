# Internal Packages

This directory contains the private application code for rpg-api. Code here is not intended to be imported by other projects.

## Package Structure

### entities/
Simple domain models - just data structures, no business logic. All game mechanics and calculations belong in rpg-toolkit.

**⚠️ IMPORTANT**: Entities are anemic by design! No methods like `CalculateAC()` or `TakeDamage()`. See [entities README](./entities/README.md) and [ADR-002](../docs/adr/002-entity-data-models.md).

### handlers/
gRPC handlers organized by proto version (e.g., `sessionv1alpha1/`). Handlers convert between proto and entity types and delegate to orchestrators.

### services/
Service interfaces and contracts organized by domain (e.g., `character/`). Define Input/Output types and business logic contracts for orchestrators to implement.

### orchestrators/
Business logic implementations organized by domain flow (e.g., `character/`). Orchestrators coordinate between repositories, rpg-toolkit, and external services.

### repositories/
Storage layer with repository interfaces and implementations. Uses plural naming (e.g., `sessions/`, `characters/`). Always uses Input/Output types for stability.

### engine/
Integration layer for rpg-toolkit. Adapts the game engine to our specific needs and provides a clean interface for orchestrators.

## Design Principles

1. **Clear Boundaries**: Each package has a single, well-defined responsibility
2. **Interface Dependencies**: Packages depend on interfaces, not concrete types
3. **No Circular Dependencies**: Strict layering is enforced
4. **Input/Output Types**: Every function uses structured types, not primitives
5. **Testability**: All packages designed for easy testing with mocks
