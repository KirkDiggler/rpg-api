# API Integration Test Plan - Class L1 Features

**Goal:** Verify toolkit features work correctly when invoked through the API layer.

**Principle:** We test the *integration*, not the rules. Rules are proven in toolkit.

---

## Test Structure

Each class gets its own test file mirroring toolkit pattern:
- `barbarian_test.go`
- `fighter_test.go`  
- `monk_test.go`
- `rogue_test.go`

---

## Barbarian L1 Tests

### Rage
- [ ] `TestRage_ActivateFeature_Success` - Activate via API, verify condition applied
- [ ] `TestRage_DamageBonus_AppearsInAttackResult` - Attack while raging, verify +2 damage in breakdown
- [ ] `TestRage_Resistance_AppliedWhenTakingDamage` - Monster attacks raging barbarian, verify half damage
- [ ] `TestRage_EndsOnTurnEnd_NoCombatActivity` - End turn without attacking, verify rage removed

### Unarmored Defense  
- [ ] `TestBarbarianUnarmoredDefense_ACCalculation` - Verify AC = 10 + DEX + CON when unarmored

---

## Fighter L1 Tests

### Second Wind
- [ ] `TestSecondWind_ActivateFeature_Success` - Activate via API, verify healing event
- [ ] `TestSecondWind_OncePerShortRest` - Second activation fails
- [ ] `TestSecondWind_ResetsAfterShortRest` - (Skip if rest not implemented)

### Fighting Styles (applied as conditions on character)
- [ ] `TestFightingStyle_Defense_ACBonus` - Character with Defense style has +1 AC with armor
- [ ] `TestFightingStyle_Dueling_DamageBonus` - Attack with one-handed, verify +2 in breakdown
- [ ] `TestFightingStyle_Archery_AttackBonus` - Ranged attack, verify +2 attack bonus
- [ ] `TestFightingStyle_GWF_RerollsLowDice` - Attack with 2H, verify rerolls in breakdown
- [ ] `TestFightingStyle_TWF_OffHandDamageBonus` - Off-hand attack has ability mod
- [ ] `TestFightingStyle_Protection_DisadvantageOnAllyAttack` - (Complex, may Skip)

---

## Monk L1 Tests

### Martial Arts
- [ ] `TestMartialArts_BonusStrikeGranted_AfterAttackAction` - Attack with monk weapon, verify bonus strike available
- [ ] `TestMartialArts_BonusStrike_Execution` - Execute the granted bonus strike
- [ ] `TestMartialArts_DexForMonkWeapons` - Can use DEX for monk weapons

### Flurry of Blows
- [ ] `TestFlurryOfBlows_ActivateFeature_Success` - Activate via API
- [ ] `TestFlurryOfBlows_GrantsTwoStrikes` - Verify 2 flurry strikes granted
- [ ] `TestFlurryOfBlows_ExecuteStrikes` - Execute both strikes

### Unarmored Defense
- [ ] `TestMonkUnarmoredDefense_ACCalculation` - Verify AC = 10 + DEX + WIS when unarmored

---

## Rogue L1 Tests

### Sneak Attack
- [ ] `TestSneakAttack_WithAdvantage_DamageAdded` - Attack with advantage, verify 1d6 in breakdown
- [ ] `TestSneakAttack_WithAllyAdjacent_DamageAdded` - Attack with ally nearby, verify 1d6
- [ ] `TestSneakAttack_OncePerTurn` - Second attack same turn, no sneak attack
- [ ] `TestSneakAttack_RequiresFinesseOrRanged` - STR attack doesn't trigger

---

## Gaps Identified (Issues to Create)

1. **Martial Arts Bonus Strike Granter** - Not called in ResolveAttack (TWF granter is called)
2. **Short Rest API** - May not exist, needed for Second Wind reset test
3. **Spatial Context in Tests** - Sneak Attack ally-adjacent needs room positioning
4. **Monster Attacks Character** - Need reverse attack flow for resistance tests

---

## Test Setup Pattern

```go
type ClassIntegrationSuite struct {
    suite.Suite
    orchestrator *encounter.Orchestrator
    charRepo     *mocks.MockCharacterRepository
    encRepo      *mocks.MockEncounterRepository
    // ... other mocks
}

func (s *ClassIntegrationSuite) createBarbarianWithRage() *entities.Character {
    // Create character data with Rage feature
}

func (s *ClassIntegrationSuite) createEncounterWithMonster() *entities.Encounter {
    // Create encounter with goblin target
}
```

---

## Priority Order

1. **Barbarian** - Rage is the most visible feature, good starting point
2. **Fighter** - Second Wind + Dueling are straightforward
3. **Rogue** - Sneak Attack needs spatial context
4. **Monk** - Martial Arts granter integration may need toolkit work first
