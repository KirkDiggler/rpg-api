# Analysis: Damage Breakdown Implementation (Issue #241)

## Executive Summary

After analyzing the toolkit's combat implementation and rpg-api's integration, I have identified **two viable approaches** for exposing damage breakdown. The toolkit DOES track damage components through `DamageChainEvent`, but this data is NOT currently returned in `AttackResult`.

**Recommendation**: Extend toolkit's `AttackResult` to include a damage breakdown struct. This provides the cleanest separation of concerns and most maintainable solution.

## Current State Analysis

### Toolkit Combat Flow (rpg-toolkit/rulebooks/dnd5e/combat/attack.go)

The toolkit's `ResolveAttack` function follows this damage calculation flow:

1. **Roll damage dice** (lines 192-210)
   - Parses weapon damage notation (e.g., "2d6")
   - Rolls dice (doubled for critical hits)
   - Stores individual rolls in `result.DamageRolls []int`
   - Sums to `baseDamage`

2. **Calculate ability modifier** (line 219)
   - Uses `calculateAttackAbilityModifier()` (lines 301-320)
   - Returns STR or DEX modifier based on weapon properties
   - Finesse weapons use higher of STR/DEX
   - This is the ONLY place ability modifier is calculated

3. **Apply damage chain** (lines 219-222)
   - Calls `applyDamageChain()` which creates `DamageChainEvent`:
     ```go
     damageEvent := &DamageChainEvent{
         AttackerID:   input.Attacker.GetID(),
         TargetID:     input.Defender.GetID(),
         BaseDamage:   baseDamage,      // Sum of dice rolls
         DamageBonus:  abilityMod,       // STR or DEX modifier
         DamageType:   string(input.Weapon.DamageType),
         IsCritical:   isCritical,
         WeaponDamage: input.Weapon.Damage,
     }
     ```

4. **Execute damage chain** (lines 284-298)
   - Event flows through staged chain system
   - Features like Rage subscribe to `DamageChain` (via `onDamageChain`)
   - Rage adds to `DamageBonus`: `e.DamageBonus += r.DamageBonus`
   - Returns: `finalEvent.BaseDamage + finalEvent.DamageBonus`

5. **Store final values** (lines 224-225)
   - `result.DamageBonus = finalEvent.DamageBonus` (ability mod + all bonuses)
   - `result.TotalDamage = finalEvent.BaseDamage + finalEvent.DamageBonus`

### What We Can and Cannot Derive

**Currently Available in AttackResult:**
- `DamageRolls []int` - Individual dice rolls (e.g., [4, 6] for 2d6)
- `DamageBonus int` - COMBINED total of ability mod + rage + other bonuses
- `TotalDamage int` - Sum of base damage + bonus

**NOT Available in AttackResult:**
- Which ability (STR vs DEX) was used
- Breakdown of DamageBonus (how much is ability mod vs rage vs other bonuses)

**Available During Execution (but not returned):**
- `DamageChainEvent` contains all components during chain execution
- Ability modifier is calculated but not stored separately
- Rage subscribes to chain and adds its bonus directly to `DamageBonus`

### Why rpg-api Cannot Derive the Breakdown

The rpg-api orchestrator in `/home/kirk/personal/rpg-api/internal/orchestrators/encounter/orchestrator.go` receives only the final `AttackResult`. It CANNOT:

1. **Determine which ability was used**: The logic for STR vs DEX selection is encapsulated in toolkit's `calculateAttackAbilityModifier()`. The orchestrator would need to duplicate this logic (finesse weapon checks, ranged vs melee, etc.) which violates separation of concerns.

2. **Split DamageBonus into components**: The final `DamageBonus` is a combined total. There's no way to know:
   - How much came from the ability modifier
   - How much came from Rage
   - How much came from other features that might add bonuses in the future

3. **Recalculate without duplicating game rules**: Any attempt to derive this would require rpg-api to:
   - Re-implement weapon property checks (finesse, ranged, etc.)
   - Re-implement ability selection logic
   - Assume what features are active (Rage, etc.)
   - This violates the core principle: "rpg-api stores data, rpg-toolkit handles rules"

## Approach Comparison

### Approach 1: Extend Toolkit's AttackResult (RECOMMENDED)

**Implementation:**
```go
// In rpg-toolkit/rulebooks/dnd5e/combat/attack.go

// DamageBreakdown provides detailed component breakdown
type DamageBreakdown struct {
    BaseDamage      int    // Sum of dice rolls
    AbilityModifier int    // STR or DEX modifier applied
    AbilityUsed     string // "STR" or "DEX"
    RageBonus       int    // Bonus from Rage feature
    OtherBonuses    int    // Other feature bonuses
    TotalBonus      int    // Sum of all bonuses
    TotalDamage     int    // BaseDamage + TotalBonus
}

// AttackResult contains the complete outcome of an attack
type AttackResult struct {
    // ... existing fields ...
    DamageRolls []int
    DamageBonus int
    TotalDamage int
    DamageType  string

    // NEW: Detailed breakdown
    Breakdown *DamageBreakdown
}
```

**Changes Required:**

1. **Toolkit Changes** (rpg-toolkit):
   - Add `DamageBreakdown` struct to `combat/types.go`
   - Store ability used in `ResolveAttack` before chain execution
   - Track bonus sources during chain execution (requires chain context enhancement)
   - Populate `result.Breakdown` before returning

2. **API Changes** (rpg-api):
   - Update proto definition in rpg-api-protos
   - Map `toolkit.Breakdown` to proto in converter
   - No business logic changes needed

**Pros:**
- ✅ Toolkit remains source of truth for game rules
- ✅ Clean separation of concerns maintained
- ✅ Future features automatically tracked if they use damage chain
- ✅ rpg-api is just data orchestration (no rule duplication)
- ✅ Type-safe and testable

**Cons:**
- ❌ Requires toolkit changes (separate PR/release)
- ❌ rpg-api must wait for toolkit version bump
- ❌ Slightly more complex chain tracking needed in toolkit

### Approach 2: Derive in rpg-api (NOT RECOMMENDED)

**Implementation:**
```go
// In rpg-api orchestrator
func deriveBreakdown(char *character.Character, weapon *weapons.Weapon, result *combat.AttackResult) *DamageBreakdown {
    // Duplicate weapon logic from toolkit
    abilityMod := calculateAbilityModifier(char, weapon)

    // Assume rage bonus if character has rage active
    rageBonus := 0
    if hasRageCondition(char) {
        rageBonus = getRageBonusForLevel(char.Level)
    }

    otherBonuses := result.DamageBonus - abilityMod - rageBonus

    return &DamageBreakdown{
        BaseDamage: sumDiceRolls(result.DamageRolls),
        AbilityModifier: abilityMod,
        RageBonus: rageBonus,
        OtherBonuses: otherBonuses,
        // ...
    }
}
```

**Pros:**
- ✅ No toolkit changes needed
- ✅ Can implement immediately

**Cons:**
- ❌ Violates separation of concerns (rpg-api implements game rules)
- ❌ Duplicates weapon logic (finesse, ranged checks)
- ❌ Duplicates condition tracking (which features are active)
- ❌ Brittle - breaks when toolkit adds new damage sources
- ❌ Difficult to test (need to mock character state)
- ❌ Contradicts project philosophy: "rpg-api stores data, toolkit handles rules"

## Detailed Design: Approach 1 (Recommended)

### Phase 1: Extend DamageChainEvent

The `DamageChainEvent` needs to track bonus sources:

```go
// DamageChainEvent represents damage flowing through the modifier chain
type DamageChainEvent struct {
    AttackerID   string
    TargetID     string
    BaseDamage   int
    DamageBonus  int    // Total bonus (sum of all sources)
    DamageType   string
    IsCritical   bool
    WeaponDamage string

    // NEW: Track bonus sources
    BonusSources map[string]int // e.g., {"ability": 4, "rage": 2, "bless": 1}
    AbilityUsed  string         // "STR" or "DEX"
}
```

### Phase 2: Update Chain Subscribers

Features that modify damage should register their contribution:

```go
// In Rage condition's onDamageChain
modifyDamage := func(_ context.Context, e *DamageChainEvent) (*DamageChainEvent, error) {
    e.DamageBonus += r.DamageBonus

    // NEW: Track source
    if e.BonusSources == nil {
        e.BonusSources = make(map[string]int)
    }
    e.BonusSources["rage"] = r.DamageBonus

    return e, nil
}
```

### Phase 3: Populate AttackResult.Breakdown

In `ResolveAttack`, after chain execution:

```go
// Calculate ability used and modifier
abilityMod := calculateAttackAbilityModifier(input.Weapon, input.AttackerScores)
abilityUsed := determineAbilityUsed(input.Weapon) // "STR" or "DEX"

// Apply damage chain (existing code)
finalDamage, damageBonus, err := applyDamageChain(ctx, input, baseDamage, abilityMod, result.Critical)

// NEW: Build breakdown from chain event
result.Breakdown = &DamageBreakdown{
    BaseDamage:      baseDamage,
    AbilityModifier: finalEvent.BonusSources["ability"], // or extract from event
    AbilityUsed:     finalEvent.AbilityUsed,
    RageBonus:       finalEvent.BonusSources["rage"],
    OtherBonuses:    calculateOtherBonuses(finalEvent.BonusSources),
    TotalBonus:      damageBonus,
    TotalDamage:     finalDamage,
}
```

### Phase 4: Update Proto Definition

In `rpg-api-protos/dnd5e/api/v1alpha1/encounter.proto`:

```protobuf
// DamageBreakdown provides detailed damage calculation components
message DamageBreakdown {
  int32 base_damage = 1;       // Sum of weapon dice rolls
  int32 ability_modifier = 2;  // STR or DEX modifier
  string ability_used = 3;     // "STR" or "DEX"
  int32 rage_bonus = 4;        // Bonus from Rage feature (0 if not raging)
  int32 other_bonuses = 5;     // Other feature bonuses
  int32 total_bonus = 6;       // Sum of all bonuses
  int32 total_damage = 7;      // base_damage + total_bonus

  // Individual dice rolls for transparency
  repeated int32 dice_rolls = 8;
}

// AttackResult contains the outcome of an attack
message AttackResult {
  bool hit = 1;
  int32 attack_roll = 2;
  int32 attack_total = 3;
  int32 target_ac = 4;
  int32 damage = 5;              // Kept for backwards compatibility
  string damage_type = 6;
  bool critical = 7;

  // NEW: Detailed damage breakdown
  DamageBreakdown damage_breakdown = 8;
}
```

### Phase 5: Update rpg-api Converter

In `/home/kirk/personal/rpg-api/internal/handlers/dnd5e/v1alpha1/encounter/converters.go`:

```go
func convertAttackResultToProto(result *encounter.AttackResult) *dnd5ev1alpha1.AttackResult {
    if result == nil {
        return nil
    }

    proto := &dnd5ev1alpha1.AttackResult{
        Hit:         result.Hit,
        AttackRoll:  int32(result.AttackRoll),
        AttackTotal: int32(result.TotalAttack),
        TargetAc:    int32(result.TargetAC),
        Damage:      int32(result.TotalDamage),
        DamageType:  result.DamageType,
        Critical:    result.Critical,
    }

    // Add breakdown if available
    if result.Breakdown != nil {
        proto.DamageBreakdown = &dnd5ev1alpha1.DamageBreakdown{
            BaseDamage:      int32(result.Breakdown.BaseDamage),
            AbilityModifier: int32(result.Breakdown.AbilityModifier),
            AbilityUsed:     result.Breakdown.AbilityUsed,
            RageBonus:       int32(result.Breakdown.RageBonus),
            OtherBonuses:    int32(result.Breakdown.OtherBonuses),
            TotalBonus:      int32(result.Breakdown.TotalBonus),
            TotalDamage:     int32(result.Breakdown.TotalDamage),
            DiceRolls:       convertIntSliceToInt32(result.DamageRolls),
        }
    }

    return proto
}
```

## Implementation Strategy

### Step 1: Toolkit Enhancement (Separate PR)
1. Add `DamageBreakdown` struct to toolkit
2. Enhance `DamageChainEvent` to track bonus sources
3. Update Rage condition to register its contribution
4. Populate `AttackResult.Breakdown` in `ResolveAttack`
5. Add comprehensive tests for breakdown tracking

### Step 2: Proto Update (Separate PR)
1. Add `DamageBreakdown` message to encounter.proto
2. Add field to `AttackResult` message
3. Generate Go code via CI/CD

### Step 3: rpg-api Integration (This PR)
1. Bump toolkit dependency to version with breakdown
2. Bump proto dependency to version with breakdown message
3. Update orchestrator's `AttackResult` struct
4. Update converter to map breakdown to proto
5. Add tests verifying breakdown is populated correctly

## Testing Strategy

### Toolkit Tests
```go
func TestResolveAttack_Breakdown(t *testing.T) {
    // Test cases:
    // 1. Normal attack with STR modifier
    // 2. Finesse weapon choosing DEX
    // 3. Raging barbarian (STR + rage bonus)
    // 4. Critical hit (doubled dice, same bonuses)
    // 5. Multiple feature bonuses stacking
}
```

### rpg-api Tests
```go
func TestConvertAttackResultToProto_WithBreakdown(t *testing.T) {
    // Test converter handles:
    // 1. Nil breakdown (backwards compatibility)
    // 2. Complete breakdown with all fields
    // 3. Breakdown with zero rage bonus
}
```

## Migration Considerations

### Backwards Compatibility
- Keep existing `damage` field in proto for clients not using breakdown
- Make `damage_breakdown` optional field
- Old clients continue to work with total damage only
- New clients can opt-in to detailed breakdown

### Rollout Plan
1. Release toolkit with breakdown (no breaking changes)
2. Release proto with new optional field (no breaking changes)
3. Release rpg-api using new versions
4. Clients update at their own pace

## Alternative Considered: Minimal Proto-Only Change

**Concept**: Add breakdown to proto but populate it in rpg-api by re-calculating ability modifier.

**Why Rejected**:
- Still requires duplicating weapon logic (finesse, ranged checks)
- Cannot accurately track Rage or other feature bonuses
- Violates architectural principles
- Would need to change anyway when more features are added

## Recommendation

**Implement Approach 1**: Extend toolkit's `AttackResult` with `DamageBreakdown`.

**Rationale**:
1. Maintains clean separation: toolkit handles rules, rpg-api handles data
2. Future-proof: automatically supports new damage-modifying features
3. Type-safe and testable at all layers
4. Follows project philosophy from CLAUDE.md
5. Only place with complete information to build accurate breakdown

**Timeline Estimate**:
- Toolkit changes: 4-6 hours (design, implement, test)
- Proto changes: 1 hour (add message, regenerate)
- rpg-api integration: 2-3 hours (update converter, test)
- Total: ~8 hours across 3 PRs

**Dependencies**:
- Must complete toolkit PR first
- Proto PR after toolkit release
- rpg-api PR after both are released

## Files Referenced

### Toolkit (rpg-toolkit)
- `/home/kirk/personal/rpg-toolkit/rulebooks/dnd5e/combat/attack.go` - Combat resolution
- `/home/kirk/personal/rpg-toolkit/rulebooks/dnd5e/conditions/raging.go` - Rage damage bonus
- `/home/kirk/personal/rpg-toolkit/rulebooks/dnd5e/conditions/raging_test.go` - Rage tests

### API (rpg-api)
- `/home/kirk/personal/rpg-api/internal/orchestrators/encounter/orchestrator.go` - Attack orchestration
- `/home/kirk/personal/rpg-api/internal/orchestrators/encounter/service.go` - Service interface
- `/home/kirk/personal/rpg-api/internal/handlers/dnd5e/v1alpha1/encounter/converters.go` - Proto conversion

### Proto (rpg-api-protos)
- `/home/kirk/personal/rpg-api-protos/dnd5e/api/v1alpha1/encounter.proto` - Encounter service definition
