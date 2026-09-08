package character

import (
	"strings"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/spells"
)

// Spells travel the wire as canonical content refs -- "dnd5e:spells:true-strike"
// -- while the toolkit's own choice vocabulary is the bare id, "true-strike"
// (spells.Spell is shared.SelectionID, and refs.Spells.ByID is what resolves
// one into the other). This file is the single place that conversion happens,
// in both directions, so no other converter has to remember which side of the
// seam it is standing on.
//
// The catalog is the authority in both directions: an id this build does not
// carry produces no ref, and a ref this build cannot resolve produces no id.
// Fail closed -- a spell nobody can name is dropped rather than traveling as
// an unresolvable string the far side would have to guess about.

// spellRefString returns the canonical ref string for a bare spell id, or ""
// when this build has no such spell.
func spellRefString(id spells.Spell) string {
	ref := refs.Spells.ByID(id)
	if ref == nil {
		return ""
	}

	return ref.String()
}

// spellRefStrings maps a list of bare spell ids onto canonical refs, dropping
// any this build cannot resolve.
func spellRefStrings(ids []spells.Spell) []string {
	if len(ids) == 0 {
		return nil
	}

	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if ref := spellRefString(id); ref != "" {
			out = append(out, ref)
		}
	}

	return out
}

// spellIDFromRef returns the bare spell id a canonical ref names, or "" when
// the string is not a spell ref this build carries.
//
// The id is everything after the second colon, which is what a core.Ref's own
// String() puts there; the catalog lookup is what decides whether the result
// is real, so a well-formed ref for a spell this build has never heard of is
// refused exactly like a malformed one.
func spellIDFromRef(ref string) spells.Spell {
	parts := strings.SplitN(ref, ":", 3)
	if len(parts) != 3 {
		return ""
	}

	if refs.Spells.ByID(parts[2]) == nil {
		return ""
	}

	return parts[2]
}

// spellIDsFromRefs maps canonical refs onto bare spell ids, dropping any this
// build cannot resolve.
func spellIDsFromRefs(refStrings []string) []spells.Spell {
	if len(refStrings) == 0 {
		return nil
	}

	out := make([]spells.Spell, 0, len(refStrings))
	for _, ref := range refStrings {
		if id := spellIDFromRef(ref); id != "" {
			out = append(out, id)
		}
	}

	return out
}

// spellIDFromProtoEnum reads a deprecated Spell enum value as a bare spell id.
//
// COMPATIBILITY READ ONLY. Producers write refs now (rpg-project#405 R8) and
// this handler never writes the enum onto a choice again, but a client built
// against the previous contract still sends it, and dropping its selection
// silently would lose a player's cantrips at finalize. The enum's own name is
// the mapping -- SPELL_VICIOUS_MOCKERY is "vicious-mockery" -- rather than a
// sixty-case switch that would need a line per spell the catalog already
// knows; refs.Spells.ByID is what confirms the result is a spell.
//
// The enum NAMES NEITHER OF THIS SLICE'S CANTRIPS -- there is no
// SPELL_TRUE_STRIKE and no SPELL_VICIOUS_MOCKERY (enums.proto's Spell ends at
// 59), which is a large part of why refs replaced it -- so no bard cantrip
// can arrive this way and this read carries only the spells the old contract
// could already say. A name the catalog cannot resolve (SPELL_THORNWHIP
// against the catalog's "thorn-whip") is dropped rather than guessed at.
func spellIDFromProtoEnum(value dnd5ev1alpha1.Spell) spells.Spell {
	if value == dnd5ev1alpha1.Spell_SPELL_UNSPECIFIED {
		return ""
	}

	name := strings.TrimPrefix(value.String(), "SPELL_")
	id := strings.ToLower(strings.ReplaceAll(name, "_", "-"))
	if refs.Spells.ByID(id) == nil {
		return ""
	}

	return id
}

// selectedSpells reads a SpellSelection as bare spell ids.
//
// spell_refs is the live field and is read alone when it is populated. The
// deprecated enum list is read ONLY when no refs arrived, so a client built
// against the previous contract still lands its selection instead of losing
// it silently; a producer that filled both would have the refs win, which is
// the succession R8 describes rather than a merge of two vocabularies.
func selectedSpells(selection *dnd5ev1alpha1.SpellSelection) []spells.Spell {
	if selection == nil {
		return nil
	}

	if chosen := spellIDsFromRefs(selection.GetSpellRefs()); len(chosen) > 0 {
		return chosen
	}

	legacy := selection.GetSpells() //nolint:staticcheck // Compatibility read for clients on the pre-ref contract.
	if len(legacy) == 0 {
		return nil
	}

	out := make([]spells.Spell, 0, len(legacy))
	for _, value := range legacy {
		if id := spellIDFromProtoEnum(value); id != "" {
			out = append(out, id)
		}
	}

	return out
}
