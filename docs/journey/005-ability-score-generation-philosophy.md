# Journey 005: Ability Score Generation Philosophy

**Date**: July 14, 2025  
**Context**: Implementing ability score validation in the rpg-toolkit engine adapter (#35)

## The Question

While implementing D&D 5e ability score validation, we encountered a philosophical question: should we support the Standard Array method alongside Point Buy and Manual entry?

## The Tension

**Standard Array** (15, 14, 13, 12, 10, 8) represents a tension in D&D philosophy:

### Against Standard Array
- **Removes randomness**: Takes away the excitement of rolling dice - that moment when you roll an 18 (or a 3!) is pure D&D magic
- **Reduces character variety**: Everyone ends up with similar power levels and stat distributions
- **Lacks character stories**: Unusual stat distributions create unique character concepts and stories
- **Spirit of D&D**: Part of the game's charm is accepting what the dice give you and making it work

### For Standard Array  
- **Fairness in co-op**: Nobody feels like they got screwed by bad rolls while their friend got amazing stats
- **Reduces restart syndrome**: Players won't keep rerolling characters until they get "good enough" stats
- **Faster character creation**: Great for getting new players into the game quickly
- **Balanced encounters**: AI/DM can design challenges knowing everyone's baseline power level
- **Player choice**: Some groups prefer this approach for their table

## API vs Application Layer Decision

**Key insight**: This is an API-level decision vs. an application-level decision.

### API Layer (rpg-api)
- **Should support all valid D&D 5e options**: Standard Array, Point Buy, Manual
- **Provides complete rule validation**: Ensures any method chosen follows official rules
- **Stays true to the source material**: D&D 5e officially supports all three methods

### Application Layer (Discord bot, web app)
- **Makes UX decisions**: Which methods to expose to users
- **Can be selective**: Might only offer Point Buy and Manual to maintain "D&D spirit"
- **Could be configurable**: Different servers/campaigns could enable different methods

## Our Decision

1. **Implement all three methods in the API**: The engine validates Standard Array, Point Buy, and Manual
2. **Document the philosophical tension**: Preserve the context for future decisions
3. **Let applications choose**: Discord bot and web apps can decide which methods to expose
4. **Future flexibility**: Easy to enable/disable methods per server or campaign

## Technical Implementation

```go
// API supports all official D&D 5e methods
const (
    AbilityScoreMethodStandardArray AbilityScoreMethod = "standard_array"
    AbilityScoreMethodPointBuy      AbilityScoreMethod = "point_buy" 
    AbilityScoreMethodManual        AbilityScoreMethod = "manual"
)
```

## Future Considerations

### Possible Application-Level Configurations
- **Hardcore mode**: Only manual entry (simulating dice rolls)
- **Competitive mode**: Only Point Buy or Standard Array for fairness
- **Casual mode**: All methods available
- **Per-campaign settings**: DMs choose allowed methods

### Additional Methods We Could Add
- **Dice rolling with API**: 4d6 drop lowest, with full roll history
- **Variant rules**: Elite array (recommended in some sourcebooks)
- **Custom arrays**: DM-defined stat arrays

## Lessons Learned

1. **Separate concerns**: API completeness vs. UX philosophy are different layers
2. **Document tensions**: Philosophical decisions need context preservation
3. **Design for flexibility**: Supporting more options at the API level gives applications choice
4. **Stay true to source**: D&D 5e supports it, so our API should too

## Related Decisions

- **ADR-002**: Entity data models - keep data structures agnostic
- **Journey-001**: Architectural exploration - separation of concerns
- This decision reinforces our API-first, application-choice architecture

---

*This journey doc captures the philosophical tension between fairness and randomness in D&D character creation, and our decision to support all official methods at the API level while allowing applications to make UX choices.*