# Prototype-First Strategy Implementation

## Overview
Track the implementation of our Prototype-First Development Strategy for combat and other complex features.

## Strategic Documents Created
- [x] Journey 010: Prototype First - The Deliberate Evolution of Combat
- [x] ADR 010: Prototype-First Development Strategy  
- [x] Combat Evolution Roadmap
- [x] Combat Event Bus Design
- [x] Updated Combat System Improvements with strategy context

## Phase 1: Discovery & Experimentation (Current)

### Combat System Prototype
**Location**: `internal/toolkit/combat`
**Status**: Active Development

#### Completed
- [x] Basic attack resolution
- [x] Critical hit mechanics (proper 5e rules)
- [x] Dice parsing with error handling
- [x] Document prototype strategy

#### In Progress
- [ ] Weapon integration framework
- [ ] Entity type management
- [ ] Spell attack system

#### Planned
- [ ] Event bus prototype
- [ ] Saving throw system
- [ ] Condition/status tracking
- [ ] Area of effect mechanics
- [ ] Reaction system prototype

### What Makes a Good Prototype Candidate?

Features that should start in `internal/toolkit`:
1. **Complex domain logic** - Needs experimentation to find right abstractions
2. **Unclear requirements** - Requirements will emerge through usage
3. **High change velocity** - Will need many iterations
4. **Cross-cutting concerns** - Affects multiple parts of the system
5. **Novel patterns** - New architectural patterns being explored

Current candidates:
- [x] Combat system (active)
- [ ] Spell system (planned)
- [ ] Skill check system (planned)
- [ ] Equipment effects (planned)
- [ ] Status conditions (planned)

## Tracking Criteria

### When to Promote from Prototype to Production

A feature is ready for production when:
- [ ] Used in 10+ different scenarios
- [ ] No major refactors for 2 iterations
- [ ] Clear separation of generic vs specific identified
- [ ] Test patterns established
- [ ] Error handling patterns proven

### When to Extract from Production to Toolkit

A component is ready for toolkit when:
- [ ] Used in production for 3+ iterations
- [ ] API stable for 2+ iterations
- [ ] Would benefit other developers
- [ ] Clean interface designed
- [ ] Comprehensive tests written
- [ ] Documentation complete

## Combat System Specific Tracking

### Generic (Future rpg-toolkit)
Components that are universally useful:
- Attack resolution mechanics
- Damage calculation
- Critical hit system
- Advantage/disadvantage
- Dice rolling utilities
- Save DC calculations
- Generic status effects

### Specific (Stays in rpg-api)
Components tied to our implementation:
- Encounter orchestration
- Discord activity integration
- Redis persistence
- Session management
- Custom business rules
- WebSocket broadcasting
- Specific event handlers

## Event Bus Evolution

### Prototype Phase Goals
- [ ] Define core event types
- [ ] Implement basic pub/sub
- [ ] Test with combat events
- [ ] Identify patterns

### Questions to Answer
1. Sync vs async event processing?
2. Event ordering guarantees?
3. Error handling strategy?
4. Event persistence needs?
5. Performance requirements?

### Success Metrics
- Can handle 100+ events/second
- <10ms processing time per event
- No event loss under load
- Clean handler registration API

## Communication Plan

### Internal Team
- Document in journey/ when discovering patterns
- Update ADRs when making decisions
- Track in issues/ for implementation

### External Stakeholders
- Clear README sections about prototype code
- Document maturity level of each component
- Set expectations about stability

### Example README Addition
```markdown
## Component Maturity Levels

### 🧪 Prototype (internal/toolkit/)
- Experimental, expect breaking changes
- Not for external use
- Rapid iteration happening

### ✅ Production (internal/)
- Stable for our use case
- Well-tested
- Still internal only

### 📦 Library (rpg-toolkit)
- Stable public API
- Backward compatibility maintained
- Ready for community use
```

## Anti-Patterns to Avoid

### DON'T
- Rush to extract to toolkit
- Feel guilty about prototype code
- Create abstractions before understanding
- Optimize for reuse prematurely
- Skip the prototype phase for complex features

### DO
- Embrace messy discovery
- Document learnings
- Track what works/doesn't
- Validate through usage
- Promote based on evidence

## Next Actions

### Immediate (This Week)
1. [ ] Continue combat prototype development
2. [ ] Implement basic event bus
3. [ ] Document discoveries in journey/
4. [ ] Update team on strategy

### Short Term (This Month)
1. [ ] Complete Phase 1 combat milestones
2. [ ] Evaluate spell system for prototype
3. [ ] Gather feedback on combat patterns
4. [ ] Plan Phase 2 structure

### Medium Term (This Quarter)
1. [ ] Begin Phase 2 stabilization
2. [ ] Refactor proven patterns
3. [ ] Add comprehensive tests
4. [ ] Document public interfaces

### Long Term (This Year)
1. [ ] Extract generic components to toolkit
2. [ ] Publish toolkit v0.1.0
3. [ ] Gather community feedback
4. [ ] Iterate based on usage

## Success Indicators

### Phase 1 Success
- Rapid feature development
- Many experiments tried
- Clear patterns emerging
- Team understands domain better

### Phase 2 Success  
- Stable internal API
- Reduced refactoring
- Comprehensive tests
- Production usage successful

### Phase 3 Success
- Clean extraction achieved
- Community interest shown
- External adoption beginning
- Clear value demonstrated

## Conclusion

The Prototype-First Strategy is now our official approach for complex feature development. This issue tracks our progress in implementing this strategy, starting with the combat system as our pilot project. Success means better abstractions, cleaner code, and more value for the community - achieved through deliberate evolution rather than premature optimization.