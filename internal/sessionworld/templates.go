package sessionworld

import (
	"errors"
	"fmt"
	"sort"

	"github.com/KirkDiggler/rpg-toolkit/core"
	tkdungeonspec "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter/dungeonspec"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster/monsters"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// templates.go is AUTHORING-TIME RESOLUTION of a dungeon's own stat blocks
// (rpg-project#555 R2, R6, R7).
//
// A template is a monster ref: `templates.guard` is placed as
// `dnd5e:monsters:guard`. This file asks the two questions the compiler
// leaves to a layer that may import the rulebook — does the template's id
// collide with a rulebook monster, and what does the rulebook derive from it
// — and carries the answers back.
//
// IT DERIVES NOTHING. Every number comes from session.DeriveTemplate, the
// same assembly session's launch runs, so the block the builder is shown is
// the creature the run gets (R6). The derived block is the SDK's own type,
// carried untouched to the handler, which maps it onto the wire.

// templatePath is the YAML path of one template's block.
func templatePath(id string) string { return "templates." + id }

// templateBasePath is the YAML path of one template's base.
func templateBasePath(id string) string { return templatePath(id) + ".base" }

// monsterRefOf is the ref a template is placed by: `dnd5e:monsters:<id>`.
func monsterRefOf(id string) string {
	return (&core.Ref{Module: refs.Module, Type: refs.TypeMonsters, ID: id}).String()
}

// templateIDOf is the template a placement's ref names, if any: only a ref on
// the dnd5e monsters route can name one, and the id is everything after the
// second colon.
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
// THE RULES ARE session.DeriveTemplate'S. It refuses a template whose ref
// names a rulebook monster or base (ErrShadowedRef: one ref, one source,
// R2), and everything else the rulebook will not assemble. This file only
// decides WHERE on the file each refusal lands:
//
//   - shadowing is the template's block as a whole: `templates.<id>`;
//   - a base the rulebook does not ship is `templates.<id>.base` — a lookup
//     (monsters.BaseByRef), asked here only to put the defect on its field;
//   - anything else is `templates.<id>`, carrying the SDK's sentence, which
//     is never parsed for a field.
//
// Every defect is collected, not just the first: the builder shows the list.
func deriveTemplates(templates map[string]tkdungeonspec.TemplateSpec) ([]sdk.DerivedBlock, []tkdungeonspec.FieldError) {
	if len(templates) == 0 {
		return nil, nil
	}

	ids := make([]string, 0, len(templates))
	for id := range templates {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var (
		blocks []sdk.DerivedBlock
		ferrs  []tkdungeonspec.FieldError
	)
	for _, id := range ids {
		ref := monsterRefOf(id)
		spec := templates[id]

		out, err := sdk.DeriveTemplate(&sdk.DeriveTemplateInput{Ref: ref, Spec: spec})
		switch {
		case err == nil:
			blocks = append(blocks, out.Block)
		case errors.Is(err, sdk.ErrShadowedRef):
			ferrs = append(ferrs, tkdungeonspec.FieldError{
				Path:    templatePath(id),
				Message: fmt.Sprintf("template %q shadows rulebook monster %q; rename the template", id, ref),
			})
		case !isRulebookBase(spec.Base):
			ferrs = append(ferrs, tkdungeonspec.FieldError{
				Path:    templateBasePath(id),
				Message: fmt.Sprintf("template %q: base %q is not a rulebook base", id, spec.Base),
			})
		default:
			ferrs = append(ferrs, tkdungeonspec.FieldError{Path: templatePath(id), Message: err.Error()})
		}
	}

	return blocks, ferrs
}

// isRulebookBase reports whether a ref names a base the rulebook ships.
func isRulebookBase(ref string) bool {
	_, ok := monsters.BaseByRef(ref)
	return ok
}
