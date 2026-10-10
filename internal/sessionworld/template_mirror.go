package sessionworld

import (
	"fmt"

	"github.com/KirkDiggler/rpg-toolkit/core"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/armor"
	tkdungeonspec "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter/dungeonspec"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/skills"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/weapons"
)

// template_mirror.go is A MIRROR, and says so.
//
// TODO(rpg-project#555): delete this file when rpg-toolkit's session module
// exports the conversion it uses at launch. templateOf, weaponIDsOf and
// abilityOf below mirror rulebooks/dnd5e/session's unexported functions of
// the same names (entities.go, session d9010a74) line for line: the same
// parse, the same catalog lookups, the same "absence is the base's"
// handling. They exist here only because the session's are unexported, and
// the api must derive the SAME numbers launch derives (design R6). Any change
// to session's templateOf must land here too until the export exists.
//
// The one difference is the return: alongside the error it names the
// template FIELD at fault, so the authoring answer can carry
// `templates.<id>.<field>`. Session's sentinels (ErrBadRef,
// ErrUnknownContent) are spelled as words because an api caller matches on
// the path, not the error.

// templateOf reads an authored template's strings into the rulebook's
// [monster.Template].
func templateOf(spec tkdungeonspec.TemplateSpec) (monster.Template, string, error) {
	base, err := core.ParseString(spec.Base)
	if err != nil {
		return monster.Template{}, templateFieldBase, fmt.Errorf("base %q is not a valid ref: %v", spec.Base, err)
	}

	out := monster.Template{
		Base:        base,
		Name:        spec.Name,
		HitDice:     spec.HitDice,
		Proficiency: spec.Proficiency,
		Experience:  spec.Experience,
	}

	if len(spec.Abilities) > 0 {
		out.Abilities = make(map[abilities.Ability]int, len(spec.Abilities))
		for key, score := range spec.Abilities {
			ability, ok := abilityOf(key)
			if !ok {
				return monster.Template{}, templateFieldAbilities, fmt.Errorf("ability %q is unknown", key)
			}
			out.Abilities[ability] = score
		}
	}

	if spec.Armor != "" {
		parsed, perr := core.ParseString(spec.Armor)
		if perr != nil {
			return monster.Template{}, templateFieldArmor, fmt.Errorf("armor %q is not a valid ref: %v", spec.Armor, perr)
		}
		if parsed.Module != refs.Module || parsed.Type != refs.TypeArmor {
			return monster.Template{}, templateFieldArmor, fmt.Errorf("armor %q is unknown", spec.Armor)
		}
		worn := parsed.ID
		if _, gerr := armor.GetByID(worn); gerr != nil {
			return monster.Template{}, templateFieldArmor, fmt.Errorf("armor %q is unknown", spec.Armor)
		}
		out.Armor = &worn
	}

	if spec.Skills != nil {
		out.Skills = make([]skills.Skill, 0, len(spec.Skills))
		for _, key := range spec.Skills {
			skill, gerr := skills.GetByID(key)
			if gerr != nil {
				return monster.Template{}, templateFieldSkills, fmt.Errorf("skill %q is unknown", key)
			}
			out.Skills = append(out.Skills, skill)
		}
	}

	if spec.Actions != nil {
		out.Actions, err = weaponIDsOf(spec.Actions)
		if err != nil {
			return monster.Template{}, templateFieldActions, err
		}
	}

	return out, "", nil
}

// weaponIDsOf turns authored weapon refs into catalog ids.
func weaponIDsOf(actions []string) ([]weapons.WeaponID, error) {
	ids := make([]weapons.WeaponID, 0, len(actions))
	for _, action := range actions {
		parsed, err := core.ParseString(action)
		if err != nil {
			return nil, fmt.Errorf("action %q is not a valid ref: %v", action, err)
		}
		if parsed.Module != refs.Module || parsed.Type != refs.TypeWeapons {
			return nil, fmt.Errorf("action %q is not a weapon", action)
		}
		id := parsed.ID
		if _, err := weapons.GetByID(id); err != nil {
			return nil, fmt.Errorf("action %q is an unknown weapon", action)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// abilityOf is the ability an authored short name (str dex con int wis cha)
// names, matched exactly.
func abilityOf(key string) (abilities.Ability, bool) {
	for _, ability := range abilities.List() {
		if string(ability) == key {
			return ability, true
		}
	}
	return "", false
}
