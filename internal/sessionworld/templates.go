package sessionworld

import (
	"fmt"
	"sort"
	"strings"

	"github.com/KirkDiggler/rpg-toolkit/core"
	combatactions "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/combat/actions"
	tkdungeonspec "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter/dungeonspec"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster/monsters"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
)

// templates.go is AUTHORING-TIME RESOLUTION of a dungeon's own stat blocks
// (rpg-project#555 R2, R6, R7).
//
// A template is a monster ref: `templates.guard` is placed as
// `dnd5e:monsters:guard`. This file answers the two questions the compiler
// leaves to a layer that may import the rulebook — does the template's id
// collide with a rulebook monster, and what does the rulebook derive from it
// — and carries the answer back as the derived echo the builder shows.
//
// IT COMPUTES NO NUMBER. Hit points, armor class, attack bonus and passive
// Perception are read off the *monster.Monster that monster.FromTemplate
// assembles — the same function session's launch calls — so the echo and the
// live creature cannot disagree. The one thing assembled here is the damage
// notation STRING (see damageNotation), from the toolkit's own facts.

// DerivedStatBlock is one authored template after the rulebook merged it onto
// its base: the numbers a creature placed from it plays with. An rpg-api
// type, converted to the wire at the handler.
type DerivedStatBlock struct {
	// TemplateID is the id the author wrote, e.g. "guard".
	TemplateID string

	// Ref is the monster ref a placement names it by, e.g.
	// "dnd5e:monsters:guard".
	Ref string

	// Name is the display name after the merge.
	Name string

	HitPoints         int
	ArmorClass        int
	PassivePerception int
	ProficiencyBonus  int
	Experience        int

	// Abilities are the merged scores keyed by the rulebook's short names
	// (str dex con int wis cha).
	Abilities map[string]int

	// Attacks are the assembled weapon attacks, in authored order.
	Attacks []DerivedAttack
}

// DerivedAttack is one weapon attack on a derived block.
type DerivedAttack struct {
	// WeaponRef is the weapon's ref, e.g. "dnd5e:weapons:spear".
	WeaponRef string

	// AttackBonus is the total to-hit, as the rulebook assembled it.
	AttackBonus int

	// Damage is dice notation, e.g. "1d6+1".
	Damage string
}

// templatePath is the YAML path of one template's block.
func templatePath(id string) string { return "templates." + id }

// templateFieldPath is the YAML path of one field of a template's block.
func templateFieldPath(id, field string) string { return templatePath(id) + "." + field }

// Template field names, as the dialect spells them in a FieldError path.
const (
	templateFieldBase      = "base"
	templateFieldAbilities = "abilities"
	templateFieldArmor     = "armor"
	templateFieldSkills    = "skills"
	templateFieldActions   = "actions"
)

// monsterRefOf is the ref a template is placed by: `dnd5e:monsters:<id>`.
func monsterRefOf(id string) *core.Ref {
	return &core.Ref{Module: refs.Module, Type: refs.TypeMonsters, ID: id}
}

// templateIDOf is the template a placement's ref names, if any: only a ref on
// the dnd5e monsters route can name one, and the id is everything after the
// second colon — session's templateFor, the same question asked the same way.
func templateIDOf(templates map[string]tkdungeonspec.TemplateSpec, ref string) (string, bool) {
	if len(templates) == 0 {
		return "", false
	}
	parsed, err := core.ParseString(ref)
	if err != nil || parsed.Module != refs.Module || parsed.Type != refs.TypeMonsters {
		return "", false
	}
	if _, ok := templates[parsed.ID]; !ok {
		return "", false
	}
	return parsed.ID, true
}

// deriveTemplates resolves every template the file declares, sorted by id.
//
// A template whose id names a rulebook monster or base is refused at
// `templates.<id>` (R2: one ref, one source). Every other template is derived
// through monsters.BaseByRef + monster.FromTemplate; a refusal lands at
// `templates.<id>.<field>` when the field is known from the conversion, or at
// `templates.<id>` carrying the rulebook's own sentence when the assembly
// refused (its reason is the toolkit's text, never parsed here).
//
// Every defect is collected, not just the first: the builder shows the list.
func deriveTemplates(templates map[string]tkdungeonspec.TemplateSpec) ([]DerivedStatBlock, []tkdungeonspec.FieldError) {
	if len(templates) == 0 {
		return nil, nil
	}

	ids := make([]string, 0, len(templates))
	for id := range templates {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var (
		blocks []DerivedStatBlock
		ferrs  []tkdungeonspec.FieldError
	)
	for _, id := range ids {
		ref := monsterRefOf(id)
		_, constructed := monsters.ByRef(ref.String())
		_, isBase := monsters.BaseByRef(ref.String())
		if constructed || isBase {
			ferrs = append(ferrs, tkdungeonspec.FieldError{
				Path:    templatePath(id),
				Message: fmt.Sprintf("template %q shadows rulebook monster %q; rename the template", id, ref.String()),
			})
			continue
		}

		block, ferr := deriveTemplate(id, ref, templates[id])
		if ferr != nil {
			ferrs = append(ferrs, *ferr)
			continue
		}
		blocks = append(blocks, block)
	}

	return blocks, ferrs
}

// deriveTemplate assembles one template and reads its derived block off the
// assembled monster.
func deriveTemplate(id string, ref *core.Ref, spec tkdungeonspec.TemplateSpec) (DerivedStatBlock, *tkdungeonspec.FieldError) {
	base, ok := monsters.BaseByRef(spec.Base)
	if !ok {
		return DerivedStatBlock{}, &tkdungeonspec.FieldError{
			Path:    templateFieldPath(id, templateFieldBase),
			Message: fmt.Sprintf("template %q: base %q is not a rulebook base", id, spec.Base),
		}
	}

	tmpl, field, err := templateOf(spec)
	if err != nil {
		return DerivedStatBlock{}, &tkdungeonspec.FieldError{
			Path:    templateFieldPath(id, field),
			Message: fmt.Sprintf("template %q: %v", id, err),
		}
	}

	built, err := monster.FromTemplate(&monster.FromTemplateInput{ID: id, Ref: ref, Template: tmpl, Base: base})
	if err != nil {
		return DerivedStatBlock{}, &tkdungeonspec.FieldError{
			Path:    templatePath(id),
			Message: fmt.Sprintf("template %q: %v", id, err),
		}
	}

	attacks, err := attacksOf(built)
	if err != nil {
		return DerivedStatBlock{}, &tkdungeonspec.FieldError{
			Path:    templateFieldPath(id, templateFieldActions),
			Message: fmt.Sprintf("template %q: %v", id, err),
		}
	}

	scores := built.AbilityScores()
	abilityScores := make(map[string]int, len(scores))
	for ability, score := range scores {
		abilityScores[string(ability)] = score
	}

	return DerivedStatBlock{
		TemplateID:        id,
		Ref:               ref.String(),
		Name:              built.Name(),
		HitPoints:         built.MaxHP(),
		ArmorClass:        built.AC(),
		PassivePerception: built.PassivePerception(),
		ProficiencyBonus:  built.ProficiencyBonus(),
		Experience:        built.Experience(),
		Abilities:         abilityScores,
		Attacks:           attacks,
	}, nil
}

// attacksOf reads each assembled weapon attack off the monster, in the order
// the rulebook armed it. Non-attack actions are not attacks and are skipped.
func attacksOf(m *monster.Monster) ([]DerivedAttack, error) {
	var out []DerivedAttack
	for _, definition := range m.Actions() {
		if definition.Attack == nil {
			continue
		}
		described, err := combatactions.Describe(&combatactions.DescribeInput{Definition: definition})
		if err != nil {
			return nil, fmt.Errorf("describe %q: %w", definition.Ref.String(), err)
		}
		out = append(out, DerivedAttack{
			WeaponRef:   definition.Ref.String(),
			AttackBonus: definition.Attack.AttackBonus,
			Damage:      damageNotation(described.Facts.Damage),
		})
	}
	return out, nil
}

// damageNotation writes the rulebook's damage facts as dice notation: each
// pool's dice plus its fixed contributions (the pool's flat bonus, and the
// attack ability's modifier when the rulebook says that pool takes it), pools
// joined by " + ".
//
// The participation decision is the toolkit's (DamageFact.Ability.Participates,
// damage.IncludesAbilityModifier's answer); this only writes the sum the
// proto's notation string asks for. If the toolkit grows a notation of its
// own, this becomes a read of it.
func damageNotation(facts []combatactions.DamageFact) string {
	parts := make([]string, 0, len(facts))
	for _, fact := range facts {
		fixed := fact.FlatBonus
		if fact.Ability != nil && fact.Ability.Participates {
			fixed += fact.Ability.Modifier
		}
		switch {
		case fixed > 0:
			parts = append(parts, fmt.Sprintf("%s+%d", fact.Dice, fixed))
		case fixed < 0:
			parts = append(parts, fmt.Sprintf("%s%d", fact.Dice, fixed))
		default:
			parts = append(parts, fact.Dice)
		}
	}
	return strings.Join(parts, " + ")
}
