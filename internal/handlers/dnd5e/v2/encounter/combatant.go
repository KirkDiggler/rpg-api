package encounter

import (
	"fmt"

	"github.com/KirkDiggler/rpg-toolkit/rpgerr"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/combat"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
)

// Interface satisfaction assertions.
// Both toolkit types already implement combat.Combatant directly — no adapter
// struct is needed on the rpg-api side. These compile-time checks document
// the fact and guard against future toolkit breakage.
var (
	_ combat.Combatant = (*character.Character)(nil)
	_ combat.Combatant = (*monster.Monster)(nil)
)

// CombatantRegistry is a simple per-attack CombatantLookup backed by a
// string-keyed map. It satisfies the combat.CombatantLookup interface and is
// injected onto the resolve context via combat.WithCombatantLookup before
// calling combat.ResolveAttack.
//
// Zero value: a zero-value CombatantRegistry is usable — Register
// lazy-initialises the internal map on first call. NewCombatantRegistry is
// provided as a convenience but is not required.
//
// Scope: per-attack. Build one registry for the attacker + target of a single
// attack, thread it onto context, call ResolveAttack, then discard it. The
// rulebook only looks up two IDs per attack (attacker and target), so
// registering the full encounter is unnecessary overhead.
//
// Concurrency: not safe for concurrent use. CombatantRegistry is intended for
// single-attack, single-goroutine use; do not share across goroutines.
type CombatantRegistry struct {
	entries map[string]combat.Combatant
}

// NewCombatantRegistry returns an empty CombatantRegistry ready for
// registration.
func NewCombatantRegistry() *CombatantRegistry {
	return &CombatantRegistry{
		entries: make(map[string]combat.Combatant),
	}
}

// Register adds a Combatant under the given id. Overwrites any existing entry
// for that id, which is intentional: tests may register a replacement combatant
// to simulate mid-combat HP changes.
//
// Register lazy-initialises the internal map so that the zero value of
// CombatantRegistry is usable without calling NewCombatantRegistry first.
func (r *CombatantRegistry) Register(id string, c combat.Combatant) {
	if r.entries == nil {
		r.entries = make(map[string]combat.Combatant)
	}
	r.entries[id] = c
}

// Get retrieves a Combatant by id. Returns a CodeNotFound rpgerr if no entry
// exists for the id, matching the convention used by the toolkit's own
// combatant lookup helpers (combat/combatant.go:122-128).
func (r *CombatantRegistry) Get(id string) (combat.Combatant, error) {
	c, ok := r.entries[id]
	if !ok {
		return nil, rpgerr.New(rpgerr.CodeNotFound, fmt.Sprintf("combatant not found: %s", id))
	}
	return c, nil
}
