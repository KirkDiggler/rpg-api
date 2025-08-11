# Combat System Evolution Roadmap

## Overview
This roadmap tracks the deliberate evolution of our combat system from prototype to production to library, following our Prototype-First Development Strategy (ADR-010).

## Current Status: PROTOTYPE PHASE
Location: `rpg-api/internal/toolkit/combat`
Freedom Level: HIGH - Can break anything, experiment freely

## Phase 1: Discovery & Experimentation (CURRENT)
**Timeline**: Now - Q1 2025
**Location**: `internal/toolkit/combat`
**Goal**: Discover the right abstractions through real usage

### Milestones
- [x] Basic attack resolution
- [x] Critical hit mechanics  
- [x] Dice parsing system
- [ ] Weapon integration framework
- [ ] Entity type management
- [ ] Spell attack system
- [ ] Event bus integration prototype
- [ ] Save system prototype
- [ ] Condition tracking prototype

### Success Criteria
- Can run 10+ different combat scenarios
- Identified patterns that work vs don't work
- Clear understanding of generic vs specific needs

## Phase 2: Stabilization & Validation
**Timeline**: Q1 2025 - Q2 2025
**Location**: Refactor within `rpg-api/internal`
**Goal**: Clean up successful patterns, validate with real encounters

### Planned Structure
```
rpg-api/internal/
├── toolkit/combat/        # Continuing prototype work
└── combat/                # Production-ready code
    ├── orchestrator.go    # Our business logic
    ├── encounter.go       # Encounter management
    ├── events.go         # Event handling
    └── resolver.go       # Using proven patterns
```

### Milestones
- [ ] Refactor proven patterns to internal/combat
- [ ] Integrate with encounter system fully
- [ ] Add comprehensive test coverage
- [ ] Document combat flow and APIs
- [ ] Run 50+ real combat encounters
- [ ] Gather feedback from usage

### Success Criteria
- No breaking API changes for 2 iterations
- 80%+ test coverage
- Used in production successfully
- Clear separation of concerns identified

## Phase 3: Extraction & Promotion
**Timeline**: Q2 2025 - Q3 2025
**Location**: Extract to `rpg-toolkit`
**Goal**: Share proven, generic patterns with the community

### What Gets Promoted
**TO rpg-toolkit** (Generic, Reusable):
- Combat resolution engine
- Attack roll mechanics
- Damage calculation system
- Critical hit system
- Advantage/disadvantage
- Generic save system
- Dice parsing utilities
- Combat event interfaces

**STAYS in rpg-api** (Business-Specific):
- Encounter orchestration
- Discord activity integration
- Session management
- Custom business rules
- Specific event handlers
- Database persistence logic

### Planned Structure
```
rpg-toolkit/
└── systems/
    └── dnd5e/
        ├── combat/
        │   ├── resolution/    # Core combat engine
        │   ├── attacks/       # Attack mechanics
        │   ├── damage/        # Damage calculation
        │   ├── saves/         # Saving throws
        │   └── conditions/    # Status effects
        └── events/
            └── combat.go      # Event interfaces

rpg-api/internal/
└── combat/
    ├── orchestrator.go       # Uses toolkit
    ├── encounter.go          # Our logic
    └── adapters.go          # Toolkit integration
```

### Milestones
- [ ] Design clean toolkit interfaces
- [ ] Extract generic components
- [ ] Create toolkit documentation
- [ ] Publish v0.1.0 of combat module
- [ ] Migrate rpg-api to use toolkit
- [ ] Gather community feedback

### Success Criteria
- Clean separation achieved
- Other developers show interest
- API is intuitive and well-documented
- Maintains backward compatibility

## Phase 4: Community Evolution
**Timeline**: Q3 2025+
**Location**: Both `rpg-toolkit` and `rpg-api`
**Goal**: Evolve both independently based on needs

### Ongoing Work
- Toolkit serves broader community
- API maintains specific business needs
- Regular sync on shared patterns
- Version management and compatibility

## Key Principles Throughout

### DO
- Experiment freely in prototype phase
- Break things to learn
- Document discoveries
- Track what's generic vs specific
- Validate through real usage
- Promote only proven patterns

### DON'T
- Rush to "clean up" working prototypes
- Feel guilty about prototype code
- Abstract before understanding
- Move to toolkit prematurely
- Optimize for reuse before proving value

## Event Bus Integration Plan

### Prototype (Current)
```go
// internal/toolkit/combat/events.go
// Quick iteration, discover patterns
type CombatEvent interface {
    GetType() string
    GetTimestamp() time.Time
}
```

### Production (Phase 2)
```go
// internal/combat/events.go
// Refined patterns, production use
type EventBus struct {
    handlers map[string][]EventHandler
}
```

### Library (Phase 3)
```go
// rpg-toolkit/events/combat.go
// Generic, reusable by anyone
type CombatEventBus interface {
    Publish(ctx context.Context, event CombatEvent) error
    Subscribe(eventType string, handler EventHandler) error
}
```

## Tracking Metrics

### Phase 1 Metrics (Discovery)
- Number of refactors: UNLIMITED
- Breaking changes: EXPECTED
- Test coverage: OPTIONAL
- Documentation: NOTES ONLY

### Phase 2 Metrics (Validation)
- Number of refactors: 2-3 expected
- Breaking changes: Should decrease
- Test coverage: 60%+ target
- Documentation: Internal complete

### Phase 3 Metrics (Promotion)
- Number of refactors: 0-1 minor
- Breaking changes: NONE
- Test coverage: 80%+ required
- Documentation: Public API complete

## Risk Mitigation

### Risk: Premature Abstraction
**Mitigation**: Stay in prototype phase until patterns are proven

### Risk: Over-engineering
**Mitigation**: Start simple, add complexity only when needed

### Risk: Breaking Changes After Promotion
**Mitigation**: Extensive validation before Phase 3

### Risk: Divergent Evolution
**Mitigation**: Regular sync between toolkit and API teams

## Success Indicators

### Short Term (Phase 1-2)
- Rapid feature development
- Quick iteration cycles
- Learning documented
- Patterns emerging

### Medium Term (Phase 3)
- Clean extraction achieved
- Community adoption starting
- API stability achieved
- Clear boundaries established

### Long Term (Phase 4+)
- Toolkit used by multiple projects
- API evolution independent
- Community contributions
- Established best practices

## Conclusion

This roadmap represents our commitment to **Deliberate Design Evolution**. We're not creating technical debt - we're investing in understanding the problem space before committing to abstractions. The prototype phase is where innovation happens. The promotion phase is where standardization happens. Both are necessary, and timing matters.