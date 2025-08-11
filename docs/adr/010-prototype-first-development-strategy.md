# ADR 010: Prototype-First Development Strategy

## Status
Accepted

## Context

We have been developing combat functionality in `rpg-api/internal/toolkit/combat` with the assumption that this is temporary technical debt that should be moved to `rpg-toolkit` as soon as possible. This assumption is causing unnecessary pressure and potentially premature abstraction.

The reality is that complex domain logic like combat systems need space to evolve and mature before being abstracted into reusable libraries. We need a deliberate strategy for how features progress from experimentation to production to shared libraries.

## Decision

We adopt a **Prototype-First Development Strategy** with clear progression phases:

### 1. Prototype Phase (internal/toolkit)
- New complex features start in `rpg-api/internal/toolkit/`
- This is a sandbox for experimentation
- Can break backward compatibility freely
- No external consumers
- Focus on discovering the right abstractions

### 2. Production Phase (internal/)
- Once patterns stabilize, refactor within rpg-api
- Clean up the prototype into production code
- Still internal to rpg-api
- Can still evolve rapidly

### 3. Library Phase (rpg-toolkit)
- Only after proven in production
- Extract truly generic, reusable components
- Maintain backward compatibility
- Serve the broader community

## Consequences

### Positive
- **Faster iteration**: No need to worry about external consumers during development
- **Better abstractions**: Discover the right patterns through real usage
- **Reduced risk**: Proven code gets promoted, not theoretical designs
- **Clear boundaries**: Explicit criteria for what belongs where
- **Flexibility**: Can refactor aggressively in prototype phase

### Negative
- **Apparent duplication**: Similar code might exist in multiple places temporarily
- **Migration effort**: Eventually need to migrate from prototype to production/library
- **Documentation overhead**: Need to track what's prototype vs production

### Neutral
- **Perception management**: Need to communicate that prototype code is intentional, not debt
- **Tracking required**: Must track feature maturity and promotion readiness

## Implementation Guidelines

### What Belongs in internal/toolkit (Prototype)
- Experimental features
- Complex domain logic being discovered
- Rapid iteration needed
- Unclear abstractions
- Breaking changes expected

### What Belongs in rpg-toolkit (Library)
- Proven, stable patterns
- Generic, reusable components
- Clear abstractions
- Valuable to multiple consumers
- Backward compatibility maintained

### Promotion Criteria
Before promoting from prototype to library:
1. Used in production for at least 3 iterations
2. API has stabilized (no breaking changes for 2 iterations)
3. Generic vs specific parts clearly identified
4. Comprehensive test coverage
5. Documentation complete
6. External interest expressed

## Examples

### Combat System (Current)
- **Status**: Prototype in `internal/toolkit/combat`
- **Why**: Still discovering the right abstractions
- **Future**: Will extract generic combat resolution to toolkit, keep encounter orchestration in API

### Dice Rolling (Completed)
- **Status**: Promoted to rpg-toolkit
- **Why**: Clear, stable, universally useful
- **History**: Started in API, proven useful, extracted

### Event Bus (Planned)
- **Status**: Prototype design phase
- **Why**: Patterns still emerging
- **Future**: Generic event bus to toolkit, specific events stay in API

## References

- Journey 010: Prototype First - The Deliberate Evolution of Combat
- Martin Fowler on "Refactoring": Change code structure without changing behavior
- Kent Beck on "Make it work, make it right, make it fast"

## Decision
We embrace prototype-first development as a deliberate strategy, not technical debt. Features earn their way from prototype to production to library through proven value and stability.