package encounter

// Dnd5eCombatResolver implements tkenc.CombatResolver by routing every
// attack through the dnd5e rulebook's combat.ResolveAttack chain.
//
// # Architecture (rpg-toolkit#689 — held-entity seam)
//
// The encounter SDK's LoadFromData cascade hydrates every combatant ONCE and
// subscribes its conditions to the encounter bus (the #684 double-subscribe
// cure). The resolver receives the already-hydrated attacker/defender on
// AttackInput.Attacker / AttackInput.Defender (combat.Combatant) and MUST NOT
// re-load them — re-loading on the same bus is exactly the double-subscribe
// class #689 removed.
//
// The resolver is therefore pure translation: take the held entities + the
// encounter-scoped bus, build the cheap per-attack CombatantRegistry + gamectx
// (room, character registry, reaction readiness), call combat.ResolveAttack /
// the phased verbs, and translate *combat.AttackResult → *tkenc.AttackOutcome.
// No character store reads, no condition re-application, no cross-RPC write-back
// (the SDK's ToData cascade persists held-entity state, e.g. SneakAttack
// UsedThisTurn).
//
// Stand-in fallback: the attacker may be nil when its seat carried no
// rehydratable DataJSON (e.g. handler tests that don't wire a character store).
// In that case the resolver delegates to StandInCombatResolver, which uses the
// AttackInput stat snapshots. When only the DEFENDER is nil, the resolver builds
// a minimal snapshot stub for it (AC only) so the held attacker's real chain
// still runs.
//
// EventBus: input.EventBus is the encounter-scoped bus the held entities'
// conditions are subscribed to. The resolver fires the chain on this bus so
// once-per-turn semantics (SneakAttack) hold across attacks within the turn.
//
// Monster weapon: for monster attackers the resolver maps the monster's first
// melee action ref to the real toolkit weapon definition so combat.ResolveAttack
// picks the right ability modifier — the same single model used for characters.
//
// Character weapon (rpg-toolkit#712): for character attackers the resolver
// delegates the strike-ActionRef-to-weapon mapping to the toolkit-owned
// Character.WeaponForActionRef instead of switching on the equipped slot /
// AttackHand itself. This is what makes unarmed_strike/flurry_strike resolve
// unarmed even when a weapon is equipped, and off_hand_strike resolve with a
// working combat.TwoWeaponContext instead of hard-failing.

import (
	"context"
	"fmt"

	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	tkdice "github.com/KirkDiggler/rpg-toolkit/dice"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/events"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/combat"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/damage"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/gamectx"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/weapons"
)

// fallbackDamageDice is the default dice notation used when no weapon is
// equipped (e.g. unarmed strike stub). "1d4" is the D&D 5e unarmed-strike damage.
const fallbackDamageDice = "1d4"

// attackHandOffLabel is the wire-side string representation of
// combat.AttackHandOff. Wire callers (translators) send the lowercase
// label; the resolver maps it to the toolkit's typed value once.
const attackHandOffLabel = "off"

// Dnd5eCombatResolverConfig holds the dependencies needed to construct a
// Dnd5eCombatResolver.
//
// #689: the resolver no longer reads the character store on the attack path
// (the SDK hands it the held entities). CharacterRepo is retained for the
// movement-registry threatener lookup (see Dnd5eMovementResolver) and is
// otherwise unused by the combat resolver.
//
// Roller is optional: if nil, the rulebook combat chain uses its own default
// roller (crypto-random). Tests can supply a deterministic roller to control
// attack-roll and weapon-damage dice for HP-delta assertions.
type Dnd5eCombatResolverConfig struct {
	// CharacterRepo provides character lookup by ID. No longer used on the
	// combat attack path (#689 hands the held entity); retained for the
	// movement resolver's threatener registry.
	CharacterRepo characterrepo.Repository

	// Roller is the dice roller for attack and weapon damage rolls. When nil,
	// the rulebook combat chain uses its own default roller.
	Roller tkdice.Roller
}

// Dnd5eCombatResolver implements tkenc.CombatResolver against the dnd5e
// rulebook's combat.ResolveAttack chain. Create one per RPC request via
// NewDnd5eCombatResolverForData — do not share across requests.
type Dnd5eCombatResolver struct {
	cfg           Dnd5eCombatResolverConfig
	encounterData *tkenc.Data
	// standIn is used as fallback when the held attacker is absent (no DataJSON).
	standIn *StandInCombatResolver
	// phasedPrep caches the phase-1 heldAttackPrep so the same-RPC
	// ApplyAttackOutcome reuses the already-held entities (no re-load). Set in
	// ResolveAttackHit, consumed once in ApplyAttackOutcome. nil on the
	// cross-RPC resume path (fresh resolver). See dnd5e_combat_resolver_phased.go.
	phasedPrep *heldAttackPrep
}

// NewDnd5eCombatResolverForData constructs a resolver bound to the given
// encounter data. The encounter data is used only for spatial Room +
// reaction-readiness gamectx wiring and the monster weapon-ref lookup; the
// held entities (not the data) supply the combatants.
func NewDnd5eCombatResolverForData(cfg Dnd5eCombatResolverConfig, data *tkenc.Data) *Dnd5eCombatResolver {
	return &Dnd5eCombatResolver{
		cfg:           cfg,
		encounterData: data,
		standIn:       NewStandInCombatResolver(cfg.Roller),
	}
}

// ResolveAttack implements tkenc.CombatResolver. It routes the attack through
// combat.ResolveAttack using the SDK-held attacker/defender (#689) — no re-load.
//
// Fallback: if the held attacker is nil (seat carried no DataJSON), delegates
// to StandInCombatResolver so fixtures without a character store still resolve.
func (r *Dnd5eCombatResolver) ResolveAttack(input tkenc.AttackInput) (*tkenc.AttackOutcome, error) {
	prep, ok, err := r.prepareHeldAttack(input)
	if err != nil {
		return nil, err
	}
	if !ok {
		// Held attacker unavailable — fall back to stand-in stat math.
		return r.standIn.ResolveAttack(input)
	}

	//nolint:staticcheck // SA1019: legacy single-phase wrapper retained for the CombatResolver interface
	result, err := combat.ResolveAttack(prep.resolveCtx, &combat.AttackInput{
		AttackerID: prep.attackerID,
		TargetID:   prep.targetID,
		Weapon:     prep.weapon,
		EventBus:   prep.bus,
		AttackHand: prep.attackHand,
		Roller:     r.cfg.Roller, // nil = rulebook default (crypto-random)
	})
	if err != nil {
		return nil, fmt.Errorf("dnd5e combat resolver: %w", err)
	}

	return &tkenc.AttackOutcome{
		Hit:                 result.Hit,
		Critical:            result.Critical,
		AttackRoll:          result.AttackRoll,
		AttackBonus:         result.AttackBonus,
		TargetAC:            result.TargetAC,
		Damage:              result.TotalDamage,
		DamageType:          string(result.DamageType),
		HasAdvantage:        result.HasAdvantage,
		HasDisadvantage:     result.HasDisadvantage,
		AdvantageSources:    result.AdvantageSources,
		DisadvantageSources: result.DisadvantageSources,
	}, nil
}

// buildEncounterCharacterRegistryFromResolved constructs a
// gamectx.BasicCharacterRegistry seeded from the held attacker and target
// characters. Monsters and additional players are registered with empty weapon
// data so ally-lookup queries return non-nil.
//
// Returns a non-nil registry even when both chars are nil.
func (r *Dnd5eCombatResolver) buildEncounterCharacterRegistryFromResolved(
	attackerChar *character.Character,
	targetChar *character.Character,
) *gamectx.BasicCharacterRegistry {
	reg := gamectx.NewBasicCharacterRegistry()

	if attackerChar != nil {
		reg.Add(attackerChar.GetID(), characterToGamectxWeapons(attackerChar))
		reg.AddAbilityScores(attackerChar.GetID(), characterToGamectxAbilityScores(attackerChar))
	}
	if targetChar != nil {
		reg.Add(targetChar.GetID(), characterToGamectxWeapons(targetChar))
		reg.AddAbilityScores(targetChar.GetID(), characterToGamectxAbilityScores(targetChar))
	}

	if r.encounterData == nil {
		return reg
	}

	for _, pd := range r.encounterData.Players {
		entityID := string(pd.EntityID)
		if reg.GetCharacterWeapons(entityID) != nil {
			continue
		}
		reg.Add(entityID, gamectx.NewCharacterWeapons(nil))
	}
	for _, md := range r.encounterData.Monsters {
		id := string(md.ID)
		if reg.GetCharacterWeapons(id) != nil {
			continue
		}
		reg.Add(id, gamectx.NewCharacterWeapons(nil))
	}

	return reg
}

// characterToGamectxAbilityScores converts a character's ability scores to the
// gamectx.AbilityScores type so conditions like MartialArts can read DEX/STR
// scores via registry.GetCharacterAbilityScores during the damage chain.
func characterToGamectxAbilityScores(char *character.Character) *gamectx.AbilityScores {
	scores := char.AbilityScores()
	return &gamectx.AbilityScores{
		Strength:     scores[abilities.STR],
		Dexterity:    scores[abilities.DEX],
		Constitution: scores[abilities.CON],
		Intelligence: scores[abilities.INT],
		Wisdom:       scores[abilities.WIS],
		Charisma:     scores[abilities.CHA],
	}
}

// characterToGamectxWeapons extracts the equipped weapon slots from a held
// *character.Character and converts them to a gamectx.CharacterWeapons value.
func characterToGamectxWeapons(char *character.Character) *gamectx.CharacterWeapons {
	var equipped []*gamectx.EquippedWeapon
	mainHand := char.GetEquippedSlot(character.SlotMainHand)
	if mainHand != nil {
		if w := mainHand.AsWeapon(); w != nil {
			equipped = append(equipped, &gamectx.EquippedWeapon{
				ID:      w.ID,
				Name:    w.Name,
				Slot:    gamectx.SlotMainHand,
				IsMelee: w.Category == weapons.CategorySimpleMelee || w.Category == weapons.CategoryMartialMelee,
			})
		}
	}
	offHand := char.GetEquippedSlot(character.SlotOffHand)
	if offHand != nil {
		if w := offHand.AsWeapon(); w != nil {
			equipped = append(equipped, &gamectx.EquippedWeapon{
				ID:      w.ID,
				Name:    w.Name,
				Slot:    gamectx.SlotOffHand,
				IsMelee: w.Category == weapons.CategorySimpleMelee || w.Category == weapons.CategoryMartialMelee,
			})
		}
	}
	return gamectx.NewCharacterWeapons(equipped)
}

// buildReactionReadinessMap converts the encounter Data's ReactionReadiness
// map (keyed by encountercore.EntityID) to a gamectx.ReactionReadinessMap
// (keyed by string). Returns an empty map (not nil) when data is nil.
func buildReactionReadinessMap(data *tkenc.Data) gamectx.ReactionReadinessMap {
	if data == nil {
		return gamectx.ReactionReadinessMap{}
	}
	result := make(gamectx.ReactionReadinessMap, len(data.ReactionReadiness))
	for entityID, refMap := range data.ReactionReadiness {
		inner := make(map[string]bool, len(refMap))
		for ref, ready := range refMap {
			inner[ref] = ready
		}
		result[string(entityID)] = inner
	}
	return result
}

// asCharacter / asMonster narrow a held combat.Combatant to its concrete dnd5e
// type. Held entities arrive as combat.Combatant (the interface both satisfy);
// the resolver needs the concrete type for richer surface (equipped weapon,
// ability scores). Returns nil when the combatant is nil or a different type.
func asCharacter(c combat.Combatant) *character.Character {
	ch, _ := c.(*character.Character)
	return ch
}

func asMonster(c combat.Combatant) *monster.Monster {
	m, _ := c.(*monster.Monster)
	return m
}

// weaponResolution is the resolver's unified per-attack weapon pick: the
// weapon to swing, the attack hand, and (for character off-hand strikes) the
// TwoWeaponContext the rulebook's off-hand validation requires.
type weaponResolution struct {
	weapon     *weapons.Weapon
	attackHand combat.AttackHand
	twoWeapon  combat.TwoWeaponContext
}

// resolveWeapon returns the weapon (+ hand + optional two-weapon context) to
// use for combat.AttackInput.
//
// For character attackers: delegates to the toolkit-owned
// Character.WeaponForActionRef (rpg-toolkit#712) — "which weapon does this
// strike ActionRef swing" is rules knowledge the toolkit owns, not something
// the resolver switches on. This replaces the old equipped-slot/AttackHand
// lookup: unarmed_strike/flurry_strike now always resolve unarmed even when a
// weapon is equipped, and off_hand_strike installs a working TwoWeaponContext
// instead of hard-failing.
//
// For monster attackers: maps the monster's first melee action ref to the real
// toolkit weapon definition so combat.ResolveAttack picks the correct ability
// modifier — the same single model used for characters. Monsters have no
// ActionRef-driven hand selection, so AttackHand comes from the wire label.
//
// Fallback: if the character has no weapon equipped, a minimal unarmed stub is
// returned so the attack can still resolve.
func (r *Dnd5eCombatResolver) resolveWeapon(
	attackerChar *character.Character,
	attackerMon *monster.Monster,
	input tkenc.AttackInput,
) (*weaponResolution, error) {
	if attackerChar != nil {
		return r.characterWeapon(attackerChar, input)
	}
	if attackerMon != nil {
		return &weaponResolution{
			weapon:     r.monsterWeapon(attackerMon, input.AttackerDamageType),
			attackHand: attackHandFromWire(input.AttackHand),
		}, nil
	}
	return &weaponResolution{weapon: unarmedStubWeapon(), attackHand: attackHandFromWire(input.AttackHand)}, nil
}

// characterWeapon delegates the strike-ActionRef-to-weapon mapping to the
// toolkit (rpg-toolkit#712, Character.WeaponForActionRef) instead of picking
// the weapon by equipped slot / AttackHand itself.
func (r *Dnd5eCombatResolver) characterWeapon(char *character.Character, input tkenc.AttackInput) (*weaponResolution, error) {
	sel, err := char.WeaponForActionRef(&input.ActionRef)
	if err != nil {
		return nil, fmt.Errorf("weapon for action ref: %w", err)
	}
	return &weaponResolution{
		weapon:     sel.Weapon,
		attackHand: sel.AttackHand,
		twoWeapon:  sel.TwoWeapon,
	}, nil
}

// attackHandFromWire maps the wire-side AttackHand label ("off" vs
// main/empty) to the toolkit's typed combat.AttackHand. Used for monster and
// no-attacker paths, which have no ActionRef-driven weapon/hand selection.
func attackHandFromWire(wire string) combat.AttackHand {
	if wire == attackHandOffLabel {
		return combat.AttackHandOff
	}
	return combat.AttackHandMain
}

// monsterWeapon resolves the real weapon for a held monster attacker by reading
// the monster's actions and mapping the first known melee action ref to the
// corresponding toolkit weapon definition. Single-model path: monsters use the
// same dice + ability-modifier resolution as characters.
//
// When no known-ref action is found, falls back to the unarmed stub so the
// attack resolves with the monster's ability modifier via the normal path.
// Always returns a valid weapon (never nil).
func (r *Dnd5eCombatResolver) monsterWeapon(mon *monster.Monster, damageTypeStr string) *weapons.Weapon {
	dmgType := damage.Type(damageTypeStr)
	if dmgType == "" {
		dmgType = damage.Bludgeoning
	}

	for _, actionData := range mon.ToData().Actions {
		weaponID, known := monsterActionRefToWeaponID(actionData.Ref.ID)
		if !known {
			continue
		}
		w, err := weapons.GetByID(weaponID)
		if err != nil {
			continue
		}
		if w.DamageType == "" {
			w.DamageType = dmgType
		}
		return &w
	}

	return unarmedStubWeapon()
}

// monsterActionRefToWeaponID maps a monster action ref ID to its toolkit
// WeaponID. Returns the weapon ID and true when the ref is known.
//
// Only refs with direct toolkit weapon equivalents are listed. Generic "melee"
// and "ranged" actions are excluded until MeleeConfig separates the dice
// notation from the flat bonus so the ability-modifier model is clean.
func monsterActionRefToWeaponID(refID string) (weapons.WeaponID, bool) {
	switch refID {
	case refs.MonsterActions.Scimitar().ID:
		return weapons.Scimitar, true
	default:
		return "", false
	}
}

// snapshotDefenderStub builds a minimal *monster.Monster (no conditions, no
// actions) to stand in as the defender's combat.Combatant when the SDK did not
// hand a held defender (the defender seat carried no DataJSON). It supplies the
// AC the attack chain reads via the CombatantLookup; it subscribes nothing to
// any bus, so it cannot recur the #684 double-subscribe. This preserves the
// pre-#689 behavior where a stat-only monster still let the attacker's real
// chain (and its conditions) run, rather than degrading the whole attack to the
// pure-snapshot stand-in.
func snapshotDefenderStub(id string, ac int) combat.Combatant {
	return monster.New(monster.Config{
		ID:               id,
		AC:               ac,
		ProficiencyBonus: 2,
	})
}

// unarmedStubWeapon returns a minimal melee weapon representing an unarmed
// strike (1d4 bludgeoning, no special properties). Used when an attacker has
// no weapon equipped on the active attack hand, or a monster action ref has no
// direct toolkit weapon mapping.
func unarmedStubWeapon() *weapons.Weapon {
	return &weapons.Weapon{
		ID:         "unarmed",
		Name:       "Unarmed Strike",
		Category:   weapons.CategorySimpleMelee,
		Damage:     fallbackDamageDice,
		DamageType: damage.Bludgeoning,
	}
}

// heldAttackPrep is the shared per-attack context built from the SDK-held
// entities, consumed by ResolveAttack and the phased verbs.
type heldAttackPrep struct {
	attackerID string
	targetID   string
	weapon     *weapons.Weapon
	resolveCtx context.Context
	bus        events.EventBus
	attackHand combat.AttackHand
}

// prepareHeldAttack builds the per-attack context from the held entities on the
// AttackInput (#689). Returns ok=false (with nil prep, nil err) when the held
// ATTACKER is absent, signaling the caller to fall back to the stand-in.
//
// No re-load, no condition re-application: the held entities were hydrated once
// by the SDK's LoadFromData cascade and their conditions are already subscribed
// to input.EventBus.
func (r *Dnd5eCombatResolver) prepareHeldAttack(input tkenc.AttackInput) (*heldAttackPrep, bool, error) {
	// The ATTACKER must be held — its conditions (SneakAttack, Rage damage,
	// MartialArts) drive the rulebook attack chain, and they are already
	// subscribed to the encounter bus by the SDK's hydration cascade. Without a
	// held attacker we cannot run the real chain → fall back to the stat-snapshot
	// stand-in (signaled by ok=false).
	//
	// The DEFENDER may be a held entity OR, when its seat carried no DataJSON
	// (e.g. a stat-only monster fixture), a minimal snapshot stub. The defender's
	// role in the chain is its AC + being present in the CombatantLookup; a stub
	// built from the AttackInput snapshot satisfies that without re-loading any
	// conditions onto the bus (a stub has none — no #684 risk). This keeps the
	// attacker's chain (and its conditions) running even against a stat-only
	// defender, which the all-or-nothing stand-in fallback would have skipped.
	if input.Attacker == nil {
		return nil, false, nil
	}

	bus := input.EventBus
	if bus == nil {
		bus = events.NewEventBus()
	}

	attackerID := string(input.AttackerID)
	targetID := string(input.TargetID)

	attackerChar := asCharacter(input.Attacker)
	targetChar := asCharacter(input.Defender)

	defender := input.Defender
	if defender == nil {
		defender = snapshotDefenderStub(targetID, input.TargetAC)
	}

	// Per-attack CombatantLookup: attacker + target only (the rulebook chain's
	// Combatant interface needs just the two participants).
	reg := NewCombatantRegistry()
	reg.Register(attackerID, input.Attacker)
	reg.Register(targetID, defender)

	charReg := r.buildEncounterCharacterRegistryFromResolved(attackerChar, targetChar)

	weaponSel, err := r.resolveWeapon(attackerChar, asMonster(input.Attacker), input)
	if err != nil {
		return nil, false, fmt.Errorf("resolve weapon: %w", err)
	}

	ctx := context.Background()
	resolveCtx := combat.WithCombatantLookup(ctx, reg)
	gameCtx := gamectx.NewGameContext(gamectx.GameContextConfig{CharacterRegistry: charReg})
	resolveCtx = gamectx.WithGameContext(resolveCtx, gameCtx)
	if weaponSel.twoWeapon != nil {
		// off_hand_strike: install the toolkit-built TwoWeaponContext so the
		// rulebook's off-hand validation (light-weapon check + bonus-action
		// gate) doesn't hard-fail (rpg-toolkit#712).
		resolveCtx = combat.WithTwoWeaponContext(resolveCtx, weaponSel.twoWeapon)
	}
	if r.encounterData != nil {
		resolveCtx = gamectx.WithRoom(resolveCtx, newEncounterHexRoom(r.encounterData))
		resolveCtx = gamectx.WithReactionReadiness(resolveCtx, buildReactionReadinessMap(r.encounterData))
	}

	return &heldAttackPrep{
		attackerID: attackerID,
		targetID:   targetID,
		weapon:     weaponSel.weapon,
		resolveCtx: resolveCtx,
		bus:        bus,
		attackHand: weaponSel.attackHand,
	}, true, nil
}
