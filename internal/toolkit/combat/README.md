# Combat System Prototype

## 🧪 Status: PROTOTYPE
This package is intentionally in the prototype phase following our Prototype-First Development Strategy (see ADR-010).

## What This Means

### This IS:
- A sandbox for rapid experimentation
- Where we discover the right combat abstractions
- Expected to change frequently
- Internal use only

### This IS NOT:
- Technical debt that needs immediate cleanup
- Ready for external consumption
- The final architecture
- Something to feel guilty about

## Development Philosophy

We follow a deliberate progression:
1. **Prototype Here** (Current) - Break things, learn fast
2. **Production in internal/** - Clean up proven patterns
3. **Library in rpg-toolkit** - Share generic components

## Current Components

### Core Systems
- `resolver.go` - Combat resolution engine (experimental)
- `types.go` - Data structures (will change)
- `weapon_integration.go` - Integration patterns (discovering)

### What's Working
- Basic attack resolution
- Critical hit mechanics (proper D&D 5e)
- Dice parsing with error handling

### What's Experimental
- Event bus patterns
- Entity type management
- Spell attack system
- Weapon data integration

## Future Evolution

### Will Move to rpg-toolkit (Generic)
- Combat resolution mechanics
- Damage calculation engine
- Attack/damage modifiers
- Dice utilities
- Generic save systems

### Will Stay in rpg-api (Specific)
- Encounter orchestration
- Discord integration
- Session management
- Business rules
- Database persistence

## Usage Guidelines

### For Developers

**DO:**
- Experiment freely
- Break backward compatibility if needed
- Try different approaches
- Document what you learn
- Refactor aggressively

**DON'T:**
- Worry about external consumers
- Optimize prematurely
- Feel bad about "messy" code
- Skip experimentation
- Rush to "clean up"

### For Reviewers

When reviewing code in this package:
1. Focus on learning and discovery, not perfection
2. Accept that interfaces will change
3. Value experimentation over stability
4. Look for patterns emerging
5. Document insights in journey docs

## Promotion Criteria

This package will move to the next phase when:
- [ ] Used in 10+ different combat scenarios
- [ ] API has stabilized (no major changes for 2 iterations)
- [ ] Generic vs specific parts clearly identified  
- [ ] Comprehensive test patterns established
- [ ] Team confident in the abstractions

## Current Experiments

### Event Bus (Active)
Testing event-driven combat for:
- Real-time updates
- Combat reactions
- Audit logging
- Decoupling concerns

### Entity Management (Planning)
Exploring how to:
- Track entity types
- Manage combat state
- Handle temporary effects
- Support multiple entity sources

## Questions We're Answering

1. What combat patterns are truly generic?
2. How should events flow through the system?
3. What's the right abstraction for weapons/spells?
4. How do we handle complex D&D mechanics elegantly?
5. What belongs in toolkit vs API?

## Contributing

Want to experiment? Great! This is the place for it:

1. Try your idea in this package
2. Document what you learn
3. Don't worry about breaking things
4. Share insights in journey docs
5. Help identify patterns

Remember: **Prototype code is not debt, it's investment in understanding.**

## References

- [ADR-010: Prototype-First Development Strategy](/docs/adr/010-prototype-first-development-strategy.md)
- [Journey 010: Combat Evolution](/docs/journey/010-prototype-first-combat-evolution.md)
- [Combat Evolution Roadmap](/docs/COMBAT_EVOLUTION_ROADMAP.md)
- [Combat System Improvements](/docs/issues/combat-system-improvements.md)