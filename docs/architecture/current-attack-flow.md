# Current Attack Flow Architecture

This document describes how attacks currently work in the system as of January 2026.

## Overview

```
API Request → Orchestrator → combat.ResolveAttack() → Result
                                    │
                                    ├── AttackChain (before roll)
                                    │      └── Listeners modify: advantage, bonus, crit threshold
                                    │
                                    ├── Roll d20
                                    │
                                    ├── Hit determination
                                    │
                                    └── DamageChain (if hit)
                                           └── Listeners modify: damage components, multipliers
```

## Step-by-Step Flow

### 1. API Receives Attack Request

**File:** `internal/handlers/dnd5e/v1alpha1/encounter/handler.go`

```go
func (h *Handler) Attack(ctx context.Context, req *dnd5ev1alpha1.AttackRequest) (*dnd5ev1alpha1.AttackResponse, error) {
    input := &encounter.ResolveAttackInput{
        EncounterID: req.GetEncounterId(),
        AttackerID:  req.GetAttackerId(),
        TargetID:    req.GetTargetId(),
        AttackHand:  combat.AttackHand(req.GetAttackHand()),
    }
    output, err := h.encounterService.ResolveAttack(ctx, input)
    // ... convert and return
}
```

### 2. Orchestrator Sets Up Context

**File:** `internal/orchestrators/encounter/orchestrator.go:249-335`

The orchestrator:
1. Creates EventBus
2. Loads character from repo
3. Calls `character.LoadFromData(ctx, data, bus)` - **this subscribes features to events**
4. Loads encounter state
5. Checks action economy (has action available?)
6. Loads monster and its conditions/traits
7. Gets equipped weapon
8. Builds GameContext with character equipment
9. Creates CombatantRegistry and adds attacker + defender
10. Calls `combat.ResolveAttack()`

### 3. combat.ResolveAttack - Attack Chain

**File:** `rpg-toolkit/rulebooks/dnd5e/combat/attack.go:141-199`

```go
func ResolveAttack(ctx context.Context, input *AttackInput) (*AttackResult, error) {
    // Step 1: Calculate base attack bonus
    abilityMod := calculateAttackAbilityModifier(input.Weapon, input.AttackerScores)
    baseBonus := abilityMod + input.ProficiencyBonus

    // Step 2: Build attack chain event with base values
    attackEvent := dnd5eEvents.AttackChainEvent{
        AttackerID:          input.Attacker.GetID(),
        TargetID:            input.Defender.GetID(),
        WeaponRef:           weaponToRef(input.Weapon),
        IsMelee:             !input.Weapon.IsRanged(),
        AdvantageSources:    nil,              // Listeners add these
        DisadvantageSources: nil,              // Listeners add these
        AttackBonus:         baseBonus,        // Listeners can modify
        TargetAC:            input.DefenderAC, // For reference
        CriticalThreshold:   20,               // Listeners can lower (Champion)
        ReactionsConsumed:   nil,              // Listeners record reactions used
    }

    // Step 3: Publish through chain - listeners modify the event
    attackChain := events.NewStagedChain[dnd5eEvents.AttackChainEvent](ModifierStages)
    attacks := dnd5eEvents.AttackChain.On(input.EventBus)
    modifiedAttackChain, err := attacks.PublishWithChain(ctx, attackEvent, attackChain)

    // Step 4: Execute chain to get final event with all modifications
    finalAttackEvent, err := modifiedAttackChain.Execute(ctx, attackEvent)
    // ...
}
```

### 4. What Listeners Can Modify (Attack Chain)

**AttackChainEvent fields:**

| Field | Description | Example Modifier |
|-------|-------------|------------------|
| `AdvantageSources` | Sources granting advantage | Reckless Attack, Pack Tactics |
| `DisadvantageSources` | Sources imposing disadvantage | Prone target at range |
| `AttackBonus` | Flat bonus to attack roll | Bless (+1d4), Magic weapon |
| `CriticalThreshold` | Roll >= this is crit | Champion Fighter (19-20) |
| `ReactionsConsumed` | Track reactions used | Protection fighting style |

### 5. Roll and Hit Determination

**File:** `rpg-toolkit/rulebooks/dnd5e/combat/attack.go:201-277`

```go
// Determine advantage/disadvantage (D&D 5e: any of each = they cancel)
hasAdvantage := len(finalAttackEvent.AdvantageSources) > 0
hasDisadvantage := len(finalAttackEvent.DisadvantageSources) > 0

// Roll based on advantage state
switch {
case hasAdvantage && hasDisadvantage:
    attackRoll = roller.Roll(ctx, 20)  // Normal roll
case hasAdvantage:
    rolls := roller.RollN(ctx, 2, 20)
    attackRoll = max(rolls[0], rolls[1])
case hasDisadvantage:
    rolls := roller.RollN(ctx, 2, 20)
    attackRoll = min(rolls[0], rolls[1])
default:
    attackRoll = roller.Roll(ctx, 20)
}

// Determine hit (natural 20 always hits, natural 1 always misses)
result.Critical = attackRoll >= finalAttackEvent.CriticalThreshold
switch {
case attackRoll == 1:
    result.Hit = false
case attackRoll == 20:
    result.Hit = true
default:
    result.Hit = (attackRoll + finalAttackEvent.AttackBonus) >= input.DefenderAC
}
```

### 6. Damage Chain (If Hit)

**File:** `rpg-toolkit/rulebooks/dnd5e/combat/attack.go:279-368`

```go
if result.Hit {
    // Roll damage dice (doubled for crits)
    damageRolls := rollDamageDice(ctx, damagePool, roller, critMultiplier)

    // Build damage components
    weaponComponent := dnd5eEvents.DamageComponent{
        Source:            dnd5eEvents.DamageSourceWeapon,
        SourceRef:         weaponToRef(input.Weapon),
        OriginalDiceRolls: damageRolls,
        FinalDiceRolls:    damageRolls,
        DamageType:        input.Weapon.DamageType,
        IsCritical:        result.Critical,
    }

    abilityComponent := dnd5eEvents.DamageComponent{
        Source:     dnd5eEvents.DamageSourceAbility,
        FlatBonus:  abilityMod,
        DamageType: input.Weapon.DamageType,
    }

    // Publish through damage chain
    resolveOutput, err := ResolveDamage(ctx, &ResolveDamageInput{
        Components:      []dnd5eEvents.DamageComponent{weaponComponent, abilityComponent},
        IsCritical:      result.Critical,
        IsOffHandAttack: isOffHandAttack,
        AbilityModifier: abilityMod,
        EventBus:        input.EventBus,
        // ... more fields
    })

    result.TotalDamage = resolveOutput.TotalDamage
}
```

### 7. What Listeners Can Modify (Damage Chain)

**DamageChainEvent fields:**

| Field | Description | Example Modifier |
|-------|-------------|------------------|
| `Components` | Damage sources (can add/modify) | Rage (+2), Sneak Attack (+dice) |
| `Components[].FinalDiceRolls` | Can reroll dice | Great Weapon Fighting |
| `Components[].Multiplier` | Resistance/vulnerability | Fire vulnerability (2.0) |
| `IsOffHandAttack` | Flag for off-hand | Removes ability mod from damage |

### 8. Post-Attack Processing

Back in the orchestrator:

1. Get monster HP after damage (applied via DamageReceivedEvent)
2. Check for two-weapon fighting grant (if main hand attack)
3. Consume action from action economy
4. Persist encounter state
5. Publish AttackResolvedEvent
6. Check for dungeon victory (if monster died)
7. Return result

## Key Design Points

### Chain Pattern
- Chains run BEFORE the roll (attack chain) or AFTER dice are rolled (damage chain)
- Listeners modify a shared event object
- Execute returns the final modified event

### Event Bus Per Request
- Fresh EventBus created for each attack
- `character.LoadFromData()` subscribes features to the bus
- Features listen and modify chain events
- Bus is discarded after request

### gamectx for Lookups
- CharacterRegistry: weapons, ability scores, action economy
- CombatantRegistry: attacker/defender entities
- Allows listeners to query game state without bloated events

## Current Issues

1. **DefenderAC passed separately** - Defender is already in input, AC should come from it
2. **Off-hand strike action publishes event nobody handles** - OffHandStrike.Activate() publishes OffHandStrikeRequestedEvent but API ignores it and calls ResolveAttack directly
3. **Actions not unified** - Main attack is direct API call, off-hand strike is an "action" that doesn't actually do the attack
4. **gamectx not fully utilized** - ADR-0026 intended for combatants to be loaded into gamectx, mutated during combat, then saved. Currently API manages saving separately.

---

## Intended Design (ADR-0026 - Not Yet Implemented)

The intended flow per ADR-0026:

```go
func (o *Orchestrator) ResolveAttack(ctx context.Context, input *ResolveAttackInput) (*ResolveAttackOutput, error) {
    // 1. Load combatants into gamectx
    ctx = gamectx.WithCharacters(ctx, characters)
    ctx = gamectx.WithMonsters(ctx, monsters)

    // 2. Apply conditions/features (they subscribe to events)
    for _, char := range characters {
        char.ApplyFeatures(ctx, bus)
    }

    // 3. Call toolkit - it looks up everything from gamectx
    result, err := combat.ResolveAttack(ctx, bus, &combat.AttackInput{
        AttackerID: input.AttackerID,
        TargetID:   input.TargetID,
        // NO defender AC passed - toolkit gets it from gamectx
        // NO ability scores passed - toolkit gets them from gamectx
    })

    // 4. Combatants were mutated in gamectx, save dirty ones
    for _, combatant := range gamectx.GetAllCombatants(ctx) {
        if combatant.IsDirty() {
            o.repo.Save(ctx, combatant.ToData())
            combatant.MarkClean()
        }
    }

    return result, nil
}
```

### Key Differences from Current

| Aspect | Current | Intended (ADR-0026) |
|--------|---------|---------------------|
| Combatant loading | API loads, passes to toolkit | API loads into gamectx, toolkit queries |
| Defender AC | Passed as `input.DefenderAC` | Toolkit gets from `gamectx.GetCombatant(targetID).AC()` |
| Ability scores | Passed as `input.AttackerScores` | Toolkit gets from gamectx |
| HP mutation | API reads `goblin.HP()` after combat | Combatant mutated in gamectx, API saves dirty |
| Saving | API manually persists after each operation | API iterates gamectx, saves all dirty combatants |

### What gamectx Should Provide

```go
type GameContext struct {
    combatants CombatantRegistry  // Characters + Monsters
}

type CombatantRegistry interface {
    Get(id string) Combatant
    GetAll() []Combatant
}

type Combatant interface {
    GetID() string
    AC() int
    HP() int
    ApplyDamage(input *ApplyDamageInput) *ApplyDamageResult
    GetAbilityScores() AbilityScores
    IsDirty() bool
    MarkClean()
    ToData() interface{}
}
```

### Why This Matters for Actions

With this design, an Action's Activate() can:
1. Look up attacker from gamectx: `gamectx.GetCombatant(ctx, action.OwnerID)`
2. Look up target from gamectx: `gamectx.GetCombatant(ctx, input.TargetID)`
3. Call `combat.ResolveAttack()` with just IDs
4. Toolkit handles everything via gamectx lookups

The action IS the attack - no event indirection needed.
