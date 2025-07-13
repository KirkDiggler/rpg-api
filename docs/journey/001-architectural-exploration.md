# Journey: Architectural Exploration

## The Beginning

This project emerged from a late-night realization while working on [dnd-bot-discord issue #316](https://github.com/KirkDiggler/dnd-bot-discord/issues/316). We had accumulated significant architectural debt - business logic scattered across packages, tight coupling between domain and ruleset-specific code, and two different state tracking systems coexisting uncomfortably.

The question arose: What if we separated the Discord bot (renderer) from the game logic entirely? What if we could play D&D through Discord, web, mobile, or even CLI - all using the same backend?

## The Evolution

### From dnd-bot to rpg-api

Our journey took an important turn when we realized the name itself was constraining our thinking:
- Started as `dnd-bot` - implying Discord and D&D specific
- Evolved to `rpg-api` - a pure API gateway for ANY tabletop RPG

This opened our architecture to support multiple rulesets and interfaces.

## Critical Insights

### Insight 1: Data vs Rules Separation

The breakthrough came when discussing domain models. Initially, we were building "rich" domain models with methods like:

```go
func (c *Character) ProficiencyBonus() int {
    return 2 + (c.Level-1)/4
}
```

Then the realization: **Proficiency bonus is a rulebook concern!** 

This led to our fundamental principle:
- **rpg-api stores data** (character stats, session state)
- **rpg-toolkit handles rules** (calculations, mechanics, dice)

### Insight 2: Battle-Tested Patterns

Instead of over-engineering, we leveraged proven patterns from production gRPC services:
- Simple, flat package structure
- Handlers organized by proto version
- Orchestrators organized by business flow
- Repositories with Input/Output types to prevent interface churn

### Insight 3: Explicit Over Implicit

Every function, at every layer, uses explicit Input/Output types. This prevents:
- Function signature changes breaking mocks
- Unclear parameter meanings
- Difficult-to-extend interfaces

## Dragons Encountered

### Dragon 1: Event-Driven Everything?
**Initial thought**: Make the entire system event-driven like rpg-toolkit.

**Reality check**: The API layer doesn't need internal events. Over-engineering alert!

**Resolution**: Only the game engine (rpg-toolkit) is event-driven. The API layer uses traditional service patterns.

### Dragon 2: Rich Domain Models
**Initial approach**: Domain models with business logic methods.

**Problem**: Where does game logic end and data begin?

**Resolution**: Simple data models in rpg-api, ALL game logic in rpg-toolkit.

### Dragon 3: Storage Lock-in
**Concern**: Don't force PostgreSQL or any specific database.

**Resolution**: Repository pattern with pluggable adapters. Start with Redis, let users choose.

## Lessons from the Past

From dnd-bot-discord's architectural debt:
1. **Scatter business logic → pain**: When character creation logic lived in 5 different packages, every change was a nightmare
2. **Tight coupling → inflexibility**: Mixing D&D 5e rules with domain logic made other rulesets impossible
3. **Multiple state systems → bugs**: Legacy bit flags + new FlowState = confusion and errors

## Open Questions

1. **Session Persistence**: How much state do we persist vs recompute?
2. **Conflict Resolution**: When Discord and web clients conflict, who wins?
3. **Performance**: Can we handle 100 concurrent games? 1000?
4. **Versioning**: How do we evolve APIs without breaking clients? (Answer: proto versioning!)

## Next Dragons to Slay

1. **Dragon: Hex Grid State Management**
   - Efficient storage and streaming of large maps
   - Fog of war calculations
   - Line of sight algorithms

2. **Dragon: Multi-ruleset Support**
   - How does rpg-api stay agnostic?
   - Where's the boundary between common and ruleset-specific?

3. **Dragon: Real-time Synchronization**
   - gRPC streaming for live updates
   - Conflict resolution across multiple clients
   - Performance at scale

## Reflections

The journey from dnd-bot to rpg-api represents more than a name change - it's a fundamental shift in thinking. By separating data from rules, we've created an architecture that can:
- Support any tabletop RPG system
- Serve any client interface
- Scale with user needs
- Evolve without breaking

The beauty is in the simplicity: rpg-api is just a data-powered API that orchestrates between clients and rpg-toolkit. No more, no less.

---

*Last updated: Foundation established with clear separation of concerns*