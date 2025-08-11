# Journey 010: Prototype First - The Deliberate Evolution of Combat

Date: 2025-08-11

## The Epiphany

We've been treating `internal/toolkit/combat` as technical debt that needs to be "fixed" by moving it to rpg-toolkit. This is wrong. It's not debt - it's a deliberate architectural strategy: **Prototype First, Then Promote**.

## The Strategy

### 1. Prototype in internal/toolkit
- **Purpose**: A sandbox for rapid experimentation
- **Location**: `rpg-api/internal/toolkit/combat`
- **Freedom**: Can break, change, refactor without affecting external consumers
- **Goal**: Discover the right abstractions through real usage

### 2. Optimize for Flexibility
- Keep it local while discovering both domain model AND data contracts
- No premature abstraction
- No forced separation before understanding the problem space
- Rapid iteration without version management overhead

### 3. Then Optimize for Modularity
- Once patterns solidify through real combat scenarios
- Extract truly generic mechanics to rpg-toolkit
- Keep business logic in rpg-api
- Only promote what provides value for ANY developer

### 4. Value for All
- What goes in toolkit must be valuable beyond our specific needs
- Generic combat resolution: YES
- Our specific encounter management: NO
- D&D 5e rules engine: YES
- Our Discord-specific adaptations: NO

## What This Means Practically

### Current State (Correct)
```
rpg-api/internal/toolkit/combat/
├── resolver.go         # Our prototype combat logic
├── types.go           # Our experimental data structures
└── weapon_integration.go  # Our integration patterns
```

### Future State (After Maturity)
```
rpg-toolkit/
└── systems/
    └── dnd5e/
        └── combat/
            ├── resolution/   # Generic combat resolution
            ├── damage/       # Damage calculation engine
            └── modifiers/    # Attack/damage modifiers

rpg-api/internal/
└── combat/
    ├── orchestrator.go   # Our business logic
    ├── encounter.go      # Our encounter management
    └── events.go         # Our event handling
```

## The Progression Roadmap

### Phase 1: Discovery (Current)
- Experiment freely in internal/toolkit
- Break things, learn, iterate
- No external consumers to worry about
- Focus on finding the right abstractions

### Phase 2: Validation
- Use the prototype in real encounters
- Discover edge cases and missing features
- Refine the data model through usage
- Identify what's generic vs specific

### Phase 3: Extraction
- Identify truly generic patterns
- Create clean interfaces
- Extract to rpg-toolkit with proper versioning
- Keep custom logic in rpg-api

### Phase 4: Evolution
- Both can evolve independently
- Toolkit serves broader community
- API maintains its specific needs
- Clean architectural boundaries

## Event Bus Integration

The event bus patterns we discovered fit perfectly with this strategy:

### In Prototype (Now)
```go
// internal/toolkit/combat/events.go
type CombatEvent interface {
    GetType() string
    GetTimestamp() time.Time
}

// Quick iteration, discover patterns
```

### In Toolkit (Future)
```go
// rpg-toolkit/events/combat.go
type CombatEventBus interface {
    Publish(ctx context.Context, event CombatEvent) error
    Subscribe(eventType string, handler EventHandler) error
}

// Generic, reusable by anyone
```

## Key Insights

1. **Technical debt vs Strategic flexibility**: What looks like debt might be deliberate flexibility
2. **Premature abstraction is evil**: Don't extract until you understand the domain
3. **Prototype space is sacred**: Need a place to break things without consequences
4. **Generic vs Specific**: Clear criteria for what belongs where
5. **Evolution not Revolution**: Gradual promotion as understanding deepens

## What NOT to Do

- Don't rush to "clean up" by moving to toolkit
- Don't feel guilty about prototype code
- Don't abstract before understanding
- Don't confuse motion with progress
- Don't optimize for reuse before proving value

## Success Criteria

We'll know we're ready to promote when:
1. Combat has been used in 10+ different encounter types
2. The API has stabilized for 3+ iterations
3. We can clearly separate generic from specific
4. Other developers express interest in the patterns
5. We have comprehensive test coverage

## The Philosophy

This is about **Deliberate Design Evolution**:
- Start messy, end clean
- Discover through doing
- Promote when proven
- Keep what's custom, share what's generic

We're not being lazy or creating debt. We're being smart about when to invest in abstractions. The prototype space in internal/toolkit is where innovation happens. The promotion to rpg-toolkit is where standardization happens.

Both are necessary. Both are valuable. The timing matters.

## Next Steps

1. Continue developing in internal/toolkit without guilt
2. Track what patterns emerge through usage
3. Document which parts feel generic vs specific
4. Create criteria for promotion decisions
5. Plan extraction only after validation

This isn't about doing it right the first time. It's about having a safe space to discover what "right" even means.