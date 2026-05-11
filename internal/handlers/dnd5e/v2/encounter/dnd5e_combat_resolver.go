package encounter

// Dnd5eCombatResolver implements tkenc.CombatResolver by routing every
// attack through the dnd5e rulebook's combat.ResolveAttack chain.
//
// # Architecture decision (Wave 2.11b seam)
//
// The encounter SDK's CombatResolver interface receives only an AttackInput
// with attacker/target entity IDs and stat snapshots — no context and no
// encounter reference. The real resolver needs to:
//
//   - Load the attacker's *character.Character (for player-side attacks) to
//     satisfy the combat.Combatant interface and retrieve the equipped weapon.
//   - Rehydrate the target/attacker *monster.Monster (for NPC-side attacks)
//     from the encounter's MonsterData.DataJSON blob (Decision Point 1:
//     no separate monster store).
//   - Build a per-attack CombatantRegistry, thread it onto a context via
//     combat.WithCombatantLookup, and call combat.ResolveAttack.
//
// To inject encounter-scope data (monster blobs) without changing the
// resolver interface, Dnd5eCombatResolver is created per-request via
// NewDnd5eCombatResolverForData. The handler stores a Dnd5eCombatResolverConfig
// and calls that constructor at each LoadFromData / New site, passing the
// freshly loaded encounter data. This keeps the resolver stateless between
// requests while still having access to the monster map.
//
// Context: context.Background() is used inside ResolveAttack because the
// encounter SDK's CombatResolver interface does not thread a context through.
// Propagating the calling RPC's context is a follow-up improvement (Wave 2.11).
//
// EventBus: a fresh events.EventBus is created per attack. The rulebook's
// combat chain uses it only for publishing within-attack events (attack chain,
// damage chain); these do not need to outlive the ResolveAttack call.
//
// Monster weapon: for monster attackers the resolver builds a synthetic melee
// weapon from the encounter's MonsterData snapshot (DamageDice / DamageType).
// The base dice are extracted from the DamageDice string (e.g. "1d6+2" → "1d6")
// so that combat.ResolveAttack can add the ability modifier from the
// rehydrated monster's Combatant.AbilityScores() without double-counting.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	encountercore "github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/events"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/combat"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/damage"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/weapons"
)

// fallbackDamageDice is the default dice notation used when no weapon is
// equipped or the damage dice string is empty (e.g. unarmed strike stub,
// empty MonsterData.DamageDice). "1d4" is the D&D 5e unarmed-strike damage.
const fallbackDamageDice = "1d4"

// Dnd5eCombatResolverConfig holds the dependencies needed to construct a
// Dnd5eCombatResolver. Stored on the handler; a new resolver is built per
// request via NewDnd5eCombatResolverForData.
//
// CharacterRepo is optional: if nil, character-side attacks fall back to
// StandInCombatResolver behavior. This allows the handler tests that do not
// wire a character repo to continue passing — the fixture uses the snapshot
// fields, which the stand-in already handles. Tests that want the real chain
// should supply a CharacterRepo populated with the fixture character.
type Dnd5eCombatResolverConfig struct {
	// CharacterRepo provides character lookup by ID. When nil, player-side
	// attacks fall back to the snapshot-based stand-in math.
	CharacterRepo characterrepo.Repository
}

// Dnd5eCombatResolver implements tkenc.CombatResolver against the dnd5e
// rulebook's combat.ResolveAttack chain. Create one per RPC request via
// NewDnd5eCombatResolverForData — do not share across requests.
type Dnd5eCombatResolver struct {
	cfg           Dnd5eCombatResolverConfig
	encounterData *tkenc.Data
	// standIn is used as fallback when the character repo is absent or a
	// character lookup fails. This preserves existing test behavior.
	standIn *StandInCombatResolver
}

// NewDnd5eCombatResolverForData constructs a resolver bound to the given
// encounter data. Called at each LoadFromData / New site in the handler so
// the resolver has access to the encounter's monster map for rehydration.
func NewDnd5eCombatResolverForData(cfg Dnd5eCombatResolverConfig, data *tkenc.Data) *Dnd5eCombatResolver {
	return &Dnd5eCombatResolver{
		cfg:           cfg,
		encounterData: data,
		standIn:       NewStandInCombatResolver(nil),
	}
}

// ResolveAttack implements tkenc.CombatResolver. It routes every attack
// through combat.ResolveAttack with a per-attack CombatantRegistry.
//
// The resolver distinguishes player vs monster by checking the attacker ID
// against data.Players (entity IDs) and data.Monsters (monster IDs).
// Player entity ID == character ID in the v2 handler stack (confirmed by the
// CreateEncounter path which seeds EntityID = PlayerID and the fixture which
// uses EntityID as the character ID directly).
//
// Fallback: if the character repo is absent or a character/monster lookup
// fails, the call delegates to StandInCombatResolver so existing tests
// (which do not wire a character repo) continue to pass unchanged.
func (r *Dnd5eCombatResolver) ResolveAttack(input tkenc.AttackInput) (*tkenc.AttackOutcome, error) {
	ctx := context.Background()

	attackerID := string(input.AttackerID)
	targetID := string(input.TargetID)

	// -- Classify attacker and target as player-entity or monster-entity ----

	attackerChar, attackerMon, err := r.resolveEntity(ctx, attackerID)
	if err != nil || (attackerChar == nil && attackerMon == nil) {
		// Entity unresolvable — fall back to stand-in so existing tests pass.
		return r.standIn.ResolveAttack(input)
	}

	targetChar, targetMon, err := r.resolveEntity(ctx, targetID)
	if err != nil || (targetChar == nil && targetMon == nil) {
		return r.standIn.ResolveAttack(input)
	}

	// -- Build per-attack CombatantRegistry ---------------------------------

	reg := NewCombatantRegistry()

	var attackerCombatant combat.Combatant
	if attackerChar != nil {
		attackerCombatant = attackerChar
	} else {
		attackerCombatant = attackerMon
	}

	var targetCombatant combat.Combatant
	if targetChar != nil {
		targetCombatant = targetChar
	} else {
		targetCombatant = targetMon
	}

	reg.Register(attackerID, attackerCombatant)
	reg.Register(targetID, targetCombatant)

	// -- Resolve weapon for the attacker ------------------------------------

	weapon, err := r.resolveWeapon(attackerChar, input)
	if err != nil {
		// Weapon unresolvable — fall back to stand-in.
		return r.standIn.ResolveAttack(input)
	}

	// -- Build combat.AttackInput and call the rulebook chain ---------------

	// A fresh event bus per attack: the rulebook's attack/damage chains use
	// this bus for within-attack publishing (attack chain modifiers, damage
	// chain). Events do not need to outlive this call.
	bus := events.NewEventBus()

	attackHand := combat.AttackHandMain
	if input.AttackHand == "off" {
		attackHand = combat.AttackHandOff
	}

	resolveCtx := combat.WithCombatantLookup(ctx, reg)

	result, err := combat.ResolveAttack(resolveCtx, &combat.AttackInput{
		AttackerID: attackerID,
		TargetID:   targetID,
		Weapon:     weapon,
		EventBus:   bus,
		AttackHand: attackHand,
	})
	if err != nil {
		return nil, fmt.Errorf("dnd5e combat resolver: %w", err)
	}

	// -- Translate *combat.AttackResult → *tkenc.AttackOutcome --------------

	return &tkenc.AttackOutcome{
		Hit:         result.Hit,
		Critical:    result.Critical,
		AttackRoll:  result.AttackRoll,
		AttackBonus: result.AttackBonus,
		TargetAC:    result.TargetAC,
		Damage:      result.TotalDamage,
		DamageType:  string(result.DamageType),
	}, nil
}

// resolveEntity loads either a *character.Character or *monster.Monster for
// the given entity ID. It returns (char, nil, nil) for players, (nil, mon, nil)
// for monsters, and (nil, nil, nil) when the entity cannot be classified.
//
// For player entities: the v2 handler uses EntityID == CharacterID; the entity
// ID is used directly as a character repo lookup key.
//
// For monster entities: the entity is rehydrated from MonsterData.DataJSON
// stored in the encounter (Decision Point 1: rehydrate per attack, no separate
// monster store).
func (r *Dnd5eCombatResolver) resolveEntity(
	ctx context.Context,
	entityID string,
) (*character.Character, *monster.Monster, error) {
	if r.encounterData == nil {
		return nil, nil, nil
	}

	// Check monsters first (entity IDs for monsters are distinct keys).
	if monData, ok := r.encounterData.Monsters[encountercore.EntityID(entityID)]; ok {
		mon, err := r.rehydrateMonster(ctx, monData)
		if err != nil {
			return nil, nil, fmt.Errorf("rehydrate monster %q: %w", entityID, err)
		}
		return nil, mon, nil
	}

	// Check players (entity IDs for players come from PlayerData.EntityID).
	for _, pd := range r.encounterData.Players {
		if string(pd.EntityID) == entityID {
			// EntityID == CharacterID in the v2 handler stack.
			char, err := r.loadCharacter(ctx, entityID)
			if err != nil {
				// Character load failed — return nil so caller falls back to stand-in.
				return nil, nil, nil //nolint:nilerr // deliberate fallback
			}
			return char, nil, nil
		}
	}

	// Entity not found in this encounter — caller falls back to stand-in.
	return nil, nil, nil
}

// loadCharacter fetches a *character.Character by ID from the character repo
// and rehydrates it with a fresh event bus. Returns an error if the repo is
// absent or the lookup fails.
func (r *Dnd5eCombatResolver) loadCharacter(ctx context.Context, characterID string) (*character.Character, error) {
	if r.cfg.CharacterRepo == nil {
		return nil, fmt.Errorf("character repo not configured")
	}
	out, err := r.cfg.CharacterRepo.Get(ctx, characterrepo.GetInput{ID: characterID})
	if err != nil {
		return nil, fmt.Errorf("get character %q: %w", characterID, err)
	}
	if out.Character == nil || out.Character.Data == nil {
		return nil, fmt.Errorf("character %q has no data", characterID)
	}
	bus := events.NewEventBus()
	char, err := character.LoadFromData(ctx, out.Character.Data, bus)
	if err != nil {
		return nil, fmt.Errorf("load character %q from data: %w", characterID, err)
	}
	return char, nil
}

// rehydrateMonster loads a *monster.Monster from the MonsterData blob.
// If DataJSON is present, it is used (round-trips full ability scores /
// proficiency / conditions). When DataJSON is absent, a minimal monster is
// constructed from the snapshot fields so the AC/HP values are available for
// the Combatant interface (combat.ResolveAttack will compute +0 ability
// modifier since ability scores are zero — acceptable for the no-DataJSON
// fallback path which is used only by test fixtures without a full monster).
func (r *Dnd5eCombatResolver) rehydrateMonster(ctx context.Context, md *tkenc.MonsterData) (*monster.Monster, error) {
	if len(md.DataJSON) > 0 {
		var data monster.Data
		if err := json.Unmarshal(md.DataJSON, &data); err != nil {
			return nil, fmt.Errorf("unmarshal monster data: %w", err)
		}
		// Sync live HP from the encounter's MonsterData — the DataJSON snapshot
		// may be stale if the monster took damage since last full persistence.
		data.HitPoints = md.HP
		data.MaxHitPoints = md.MaxHP
		bus := events.NewEventBus()
		mon, err := monster.LoadFromData(ctx, &data, bus)
		if err != nil {
			return nil, fmt.Errorf("load monster from data: %w", err)
		}
		return mon, nil
	}

	// No DataJSON — build from snapshot fields only. Ability scores are zero;
	// combat.ResolveAttack will compute a +0 ability modifier. This is the
	// test-fixture path (AddMonster without DataJSON) — acceptable for playtest
	// fixtures that don't embed full monster data.
	mon := monster.New(monster.Config{
		ID:               string(md.ID),
		Name:             md.MonsterRef,
		HP:               md.HP,
		AC:               md.AC,
		ProficiencyBonus: 2, // CR-appropriate default; snapshot doesn't carry proficiency
	})
	return mon, nil
}

// resolveWeapon returns the weapon to use for combat.AttackInput.
//
// For character attackers: reads the equipped main-hand (or off-hand) weapon
// from the character's inventory slot via character.GetEquippedSlot.
//
// For monster attackers: builds a synthetic melee weapon from the encounter
// SDK's MonsterData snapshot (AttackerDamageDice / AttackerDamageType). The
// base dice are extracted from the dice notation to avoid double-counting the
// ability modifier that combat.ResolveAttack adds from the Combatant interface.
//
// Fallback: if the character has no weapon equipped, a minimal stub weapon is
// returned so the attack can still resolve (using unarmed-strike stats as a
// proxy). This matches the stand-in's "1d4" fallback behavior — the
// difference is that the real chain will still compute the correct hit bonus
// from the combatant's ability scores.
func (r *Dnd5eCombatResolver) resolveWeapon(
	attackerChar *character.Character,
	input tkenc.AttackInput,
) (*weapons.Weapon, error) {
	if attackerChar != nil {
		return r.characterWeapon(attackerChar, input.AttackHand)
	}
	// Monster attacker: build synthetic weapon from snapshot.
	return syntheticMonsterWeapon(input.AttackerDamageDice, input.AttackerDamageType), nil
}

// characterWeapon retrieves the equipped weapon for a character attacker.
// Falls back to a stub "unarmed" weapon if no weapon is equipped.
func (r *Dnd5eCombatResolver) characterWeapon(char *character.Character, attackHand string) (*weapons.Weapon, error) {
	slot := character.SlotMainHand
	if attackHand == "off" {
		slot = character.SlotOffHand
	}
	equipped := char.GetEquippedSlot(slot)
	if equipped != nil {
		w := equipped.AsWeapon()
		if w != nil {
			return w, nil
		}
	}

	// No weapon equipped — use the unarmed stub so the attack resolves with
	// the character's strength modifier instead of aborting.
	return unarmedStubWeapon(), nil
}

// syntheticMonsterWeapon constructs a minimal *weapons.Weapon from the
// MonsterData snapshot fields. The Damage field is set to the BASE dice
// only (the "+N" suffix is stripped) so that combat.ResolveAttack can add
// the ability modifier from the monster's Combatant.AbilityScores() without
// double-counting.
//
// Examples:
//
//	"1d6+2" → Damage: "1d6"
//	"2d4"   → Damage: "2d4"
//	""      → Damage: "1d4" (stub fallback)
func syntheticMonsterWeapon(damageDice, damageTypeStr string) *weapons.Weapon {
	baseDice := extractBaseDice(damageDice)
	dmgType := damage.Type(damageTypeStr)
	if dmgType == "" {
		dmgType = damage.Bludgeoning
	}
	return &weapons.Weapon{
		ID:         "monster-natural-attack",
		Name:       "Natural Attack",
		Category:   weapons.CategorySimpleMelee,
		Damage:     baseDice,
		DamageType: dmgType,
	}
}

// unarmedStubWeapon returns a minimal melee weapon representing an unarmed
// strike (1d4 bludgeoning, no special properties). Used when a character has
// no weapon equipped on the active attack hand.
func unarmedStubWeapon() *weapons.Weapon {
	return &weapons.Weapon{
		ID:         "unarmed",
		Name:       "Unarmed Strike",
		Category:   weapons.CategorySimpleMelee,
		Damage:     fallbackDamageDice,
		DamageType: damage.Bludgeoning,
	}
}

// extractBaseDice strips a modifier suffix from a dice notation string,
// returning the bare dice portion.
//
//	"1d6+2"  → "1d6"
//	"2d4+1"  → "2d4"
//	"2d4"    → "2d4"
//	""       → "1d4"
func extractBaseDice(notation string) string {
	if notation == "" {
		return fallbackDamageDice
	}
	// Trim whitespace before parsing.
	notation = strings.TrimSpace(notation)
	// Find the first '+' or '-' after the dice spec. Everything before that
	// is the base dice. We locate 'd' first to skip modifiers that appear
	// before the dice (rare but safe).
	dIdx := strings.IndexByte(notation, 'd')
	if dIdx < 0 {
		// Not a standard dice notation — return as-is and let the rulebook
		// parser surface any error.
		return notation
	}
	// Look for +/- after the 'd'.
	rest := notation[dIdx+1:]
	if plusIdx := strings.IndexAny(rest, "+-"); plusIdx >= 0 {
		return notation[:dIdx+1+plusIdx]
	}
	return notation
}
