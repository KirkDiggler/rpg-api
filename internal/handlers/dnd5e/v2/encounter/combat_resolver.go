package encounter

import (
	"context"
	"fmt"

	tkdice "github.com/KirkDiggler/rpg-toolkit/dice"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
)

// CombatResolver is the rpg-api-side alias for the toolkit's
// CombatResolver interface. The handler wires an implementation via
// HandlerConfig.CombatResolver; without one, the toolkit returns
// ErrNoCombatResolver from TakeAction.
//
// Wave 2.11a ships StandInCombatResolver, which mimics the pre-2.11a
// behavior the toolkit's encounter SDK used to provide internally
// (d20+bonus vs AC; damage from the attacker's stored damage dice).
// This preserves end-to-end behavior while the resolver pattern
// settles in. A real implementation that goes through the dnd5e
// rulebook's combat.ResolveAttack chain (with character store lookup,
// effective AC, equipped weapons, ActionEconomy persistence) ships in
// a follow-up wave.
type CombatResolver = tkenc.CombatResolver

// StandInCombatResolver computes attacks using the d20+bonus vs AC
// formula the toolkit's stand-in used pre-2.11a, sourcing combat stats
// from the AttackInput snapshots (the encounter SDK fills these in from
// PlayerData / MonsterData at attack time, so the resolver never needs
// to look up encounter state).
//
// Behavior matches the toolkit's old internal stand-in:
//   - d20 roll (nat-1 always misses; nat-20 always crits)
//   - hit = (roll + AttackBonus) >= TargetAC, with nat-1 / nat-20 overrides
//   - crit doubles the damage dice
//   - damage notation defaults to "1d4" if unspecified
//
// Tests can supply a deterministic Roller; production uses dice.NewRoller().
type StandInCombatResolver struct {
	Roller tkdice.Roller
}

// NewStandInCombatResolver constructs a StandInCombatResolver. If roller
// is nil, a default dice.NewRoller() is used.
func NewStandInCombatResolver(roller tkdice.Roller) *StandInCombatResolver {
	if roller == nil {
		roller = tkdice.NewRoller()
	}
	return &StandInCombatResolver{Roller: roller}
}

// ResolveAttack computes a single attack outcome using the AttackInput's
// stat snapshots. The encounter SDK applies HP changes + publishes
// events from the returned AttackOutcome.
func (r *StandInCombatResolver) ResolveAttack(input tkenc.AttackInput) (*tkenc.AttackOutcome, error) {
	if r.Roller == nil {
		return nil, fmt.Errorf("stand-in combat resolver: nil roller")
	}

	roll, err := r.Roller.Roll(context.Background(), 20)
	if err != nil || roll <= 0 {
		// Fall back to d20 average (10) on roller error so a roller
		// hiccup doesn't abort a TakeAction call.
		roll = 10
	}

	out := &tkenc.AttackOutcome{
		AttackRoll:  roll,
		AttackBonus: input.AttackerAttackBonus,
		TargetAC:    input.TargetAC,
		DamageType:  input.AttackerDamageType,
	}

	switch roll {
	case 20:
		out.Hit = true
		out.Critical = true
	case 1:
		// Auto-miss; outcome is already zero.
		return out, nil
	default:
		out.Hit = (roll + input.AttackerAttackBonus) >= input.TargetAC
	}

	if !out.Hit {
		return out, nil
	}

	dice := input.AttackerDamageDice
	if dice == "" {
		dice = "1d4"
	}
	out.Damage = rollDamage(r.Roller, dice, out.Critical)
	return out, nil
}

// rollDamage parses dice notation, rolls it, and applies the crit
// doubling approximation matching the toolkit's pre-2.11a stand-in
// (total*2 - modifier on a crit, so only the dice portion doubles).
// Parse / roll failures fall back to 1 — the resolver keeps the verb
// non-blocking rather than aborting an attack on a malformed dice
// notation, matching the toolkit's old internal behavior.
func rollDamage(roller tkdice.Roller, notation string, critical bool) int {
	pool, err := tkdice.ParseNotation(notation)
	if err != nil {
		return 1
	}
	rolled := pool.RollContext(context.Background(), roller)
	if rolled.Error() != nil {
		return 1
	}
	dmg := rolled.Total()
	if critical {
		dmg += rolled.Total() - rolled.Modifier()
	}
	if dmg < 0 {
		dmg = 0
	}
	return dmg
}
