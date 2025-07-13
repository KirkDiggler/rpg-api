# Journey: Character Proto Design

## The Challenge

How do we design character protos that are:
- Specific enough for D&D 5e Discord bot builders
- Flexible enough for future rulesets
- Easy to consume and implement

## Key Decisions

### 1. Explicit Over Generic

We started with the question: should the API be generic or ruleset-specific?

**Generic approach considered:**
```protobuf
message Character {
  map<string, string> attributes;
  map<string, int32> stats;
}
```

**Decision**: Be explicit and ruleset-specific
```protobuf
message DnD5eCharacterInput {
  Race race = 1;
  Class class = 2;
  Background background = 3;
  AbilityScores ability_scores = 4;
}
```

**Why**: From the consumer's perspective, "I want to build a D&D 5e Discord bot" - they need clear, typed fields, not generic maps.

### 2. Package Structure

**Google style guide says**: Package should match directory structure

**Initial attempt**: `package api.v1alpha1.dnd5e`

**Buf said**: "No, if you're in `api/proto/v1alpha1/dnd5e/`, use `package v1alpha1.dnd5e`"

**Lesson**: Tools enforce conventions - follow them.

### 3. Service Naming

**Initial**: `CharacterAPI`

**Buf linter**: "Should be suffixed with 'Service'"

**Final**: `CharacterService`

**Pattern**: Google's conventions prefer "Service" suffix for service definitions.

### 4. Enums for Everything

Instead of string fields for race/class/etc., we use enums:
- Type safety at compile time
- Clear valid values for consumers
- IDE autocomplete support
- Prevents typos

### 5. Rich Response Objects

The API returns calculated values:
```protobuf
message Character {
  // What they input
  Race race = 5;
  Class class = 7;
  AbilityScores ability_scores = 10;
  
  // What we calculate
  AbilityModifiers ability_modifiers = 11;
  CombatStats combat_stats = 12;
  int32 proficiency_bonus = 13;
}
```

This saves clients from implementing D&D math.

## Dragons Encountered

### Dragon: Import Paths

**Problem**: Proto imports failing with "file does not exist"

**Cause**: Buf uses module-relative imports, not filesystem paths

**Solution**: 
- Use `import "v1alpha1/dnd5e/enums.proto"` not `import "api/proto/v1alpha1/dnd5e/enums.proto"`
- Add googleapis as a buf dependency

### Dragon: Generated File Location

**Problem**: Generated files appeared at project root

**Solution**: Update `buf.gen.yaml` to specify output directory:
```yaml
plugins:
  - plugin: buf.build/protocolbuffers/go
    out: api/proto  # Not just "."
```

## Tools & Setup

### Buf Configuration

We use Buf for proto management because:
- Enforces Google's style guide
- Detects breaking changes
- Manages dependencies cleanly
- Better than raw protoc

Key files:
- `buf.work.yaml` - Workspace configuration
- `api/proto/buf.yaml` - Module configuration with deps
- `buf.gen.yaml` - Code generation settings

### Makefile Integration

Added buf commands:
- `make buf-lint` - Check proto style
- `make buf-generate` - Generate Go code  
- `make buf-breaking` - Detect breaking changes

## Reflections

Starting explicit and ruleset-specific was the right call. We can always extract common patterns later, but right now our consumers need a clear D&D 5e API.

The tooling (Buf) pushed us toward better practices - embracing its conventions rather than fighting them made everything smoother.

Next up: implementing the orchestrator that makes these protos come alive.

---

*Last updated: Proto definitions complete and generating cleanly*
