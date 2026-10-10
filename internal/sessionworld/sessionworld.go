// Package sessionworld turns one authored dungeon file into the three things
// the session stack needs to start a game in it: a world, the cells the
// party walks in on, and the monsters standing in it.
//
// It holds NO content. The reference tomb is content/reference-tomb.yaml at
// the repo root, loaded by internal/dungeons' file registry alongside every
// other dungeon under RPG_CONTENT_DIR (rpg-api#806, rpg-project#256). This
// package is "compile bytes -> world" and nothing else.
//
// # Why this package is thin, and where the one conversion lives
//
// rpg-toolkit's rulebooks/dnd5e/encounter/dungeonspec (version 2,
// rpg-project#256) compiles the file, so rpg-api computes NO dungeon
// geometry. What the compiler emits for a placement is the author's own
// ABSOLUTE offset [col,row] pair; what the session's Join and Spawn take is
// the dungeon-absolute AXIAL cell the atlas draws. The conversion between
// the two is encounter.HexCellAt -- exported by the toolkit precisely so a
// content caller asks for it rather than reimplementing it
// (rpg-toolkit#1150: one basis, one place). That call is the only geometry
// this package performs, and it is a lookup, not arithmetic of its own.
//
// Before version 2 the compiler spoke room-local frames and this package
// borrowed the projection by building a throwaway encounter. That seam
// (rpg-toolkit#1139) no longer exists: there is no origin to add, so there
// is nothing to borrow.
package sessionworld

import (
	"fmt"
	"sort"
	"strings"

	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	tkdungeonspec "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter/dungeonspec"
	tkscenarios "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter/scenarios"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster/monsters"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

// Dungeon is authored content compiled into everything needed to seed a
// session, with every cell already in the dungeon-absolute frame the session
// verbs speak.
type Dungeon struct {
	// Key is the dungeon's identifier, exactly as the file's own `key:` line
	// says it. The registry (internal/dungeons) checks it against the name a
	// file was stored under; this package only reports it.
	Key string

	// Name is the display name, exactly as the file's `name:` line says it.
	Name string

	// PartySeats are where the party comes in, dungeon-absolute, best seat
	// first.
	//
	// A list because a party is more than one person and the author declares
	// one cell: the first is the cell they wrote, the rest are that chamber's
	// other free cells nearest-first, so a caller with four players takes the
	// first four and gets them standing together at the way in.
	PartySeats []spatial.Position

	// Spec is the toolkit compiler's own output, every monster's member id
	// already minted by the compile (dungeonspec MonsterPlacement.MemberID).
	// session.Manager.Launch takes it whole: placing, arming, ordering and
	// arriving the garrison is the SDK's, and this package re-projects none
	// of it (rpg-project#542, "rpg-api keeps transport").
	Spec *tkdungeonspec.Compiled

	// Templates is one derived stat block per template the file declares,
	// sorted by template id (rpg-project#555 R7): the numbers the rulebook
	// derived, echoed to the builder so no client computes one. Nil when the
	// file declares none. Read off the monster monster.FromTemplate
	// assembles — the function session's launch calls — never computed here.
	Templates []DerivedStatBlock
}

// A file that does not decode, validate or compile fails with an error that
// wraps [tkdungeonspec.ErrBadSpec] -- a validation failure is a
// *tkdungeonspec.ValidationError carrying every defect and its YAML path;
// anything else is an internal failure. Callers that need to tell the two
// apart (the authoring RPC answers the first as a body and the second as a
// status) use errors.Is / errors.As.
func Compile(raw []byte) (*Dungeon, error) {
	spec, err := tkdungeonspec.Load(raw)
	if err != nil {
		return nil, fmt.Errorf("load spec: %w", err)
	}
	if len(spec.PartyStart) == 0 {
		// Unreachable for a spec that compiled -- dungeonspec documents
		// PartyStart as "never empty for a spec that compiled" -- and checked
		// anyway, because the alternative to this error is an index panic in a
		// caller seating a party.
		return nil, fmt.Errorf("compile spec: dungeon declares no party start")
	}

	orientation := spec.Field.Canvas.Orientation
	seats := make([]spatial.Position, len(spec.PartyStart))
	for i, seat := range spec.PartyStart {
		seats[i] = cellOf(orientation, seat.At)
	}

	// Resolve every content reference before accepting the dungeon. This is
	// deliberately a lookup only: the SDK owns construction and all rules.
	//
	// A monster ref resolves in exactly one place (rpg-project#555 R2): a
	// rulebook constructor, or a template this file declares under the ref's
	// id. Templates are resolved first — a shadowing id or a block the
	// rulebook cannot assemble is refused at `templates.<id>` — and a
	// placement naming a declared template is then known whatever its
	// template's own fate, so one bad block is one defect, not one per
	// placement of it. Every defect in this stage is reported together.
	templates, ferrs := deriveTemplates(spec.Templates)
	for _, m := range spec.Monsters {
		if _, isTemplate := templateIDOf(spec.Templates, m.Ref); isTemplate {
			continue
		}
		if _, known := monsters.ByRef(m.Ref); !known {
			id := m.ID
			if id == "" {
				id = m.Ref
			}
			ferrs = append(ferrs, tkdungeonspec.FieldError{
				Message: fmt.Sprintf("monster %q references unknown monster %q", id, m.Ref),
			})
		}
	}
	if len(ferrs) > 0 {
		return nil, &tkdungeonspec.ValidationError{Errors: ferrs}
	}

	// dungeonspec validates at most one boss PER REGION; across regions a
	// file could still author several, and "whose death ends things" cannot
	// be plural while the doom names one member — refused here, loudly, so
	// an authoring mistake fails at compile rather than leaving a run that
	// never ends when the "real" boss falls. Softens when the builder
	// (#169) brings authored multi-ending variety.
	var bossID string
	for _, m := range spec.Monsters {
		if !m.Boss {
			continue
		}
		if bossID != "" {
			return nil, fmt.Errorf(
				"dungeon %q authors more than one boss (%q and %q): one death ends things, and it cannot be two",
				spec.Key,
				bossID,
				m.MemberID,
			)
		}
		bossID = m.MemberID
	}

	// Wrapped like the three steps above it (Copilot, PR #914), so a refusal
	// that surfaces through the registry says which stage of the compile
	// made it. The wrap is transparent to both matches the registry runs:
	// errors.Is still finds ErrBadSpec and errors.As still finds the
	// *ValidationError whose defects carry the form-filler sentence.
	if err := validateScenarios(spec); err != nil {
		return nil, fmt.Errorf("bind scenarios: %w", err)
	}

	return &Dungeon{
		Key: spec.Key, Name: spec.Name,
		PartySeats: seats, Spec: &spec,
		Templates: templates,
	}, nil
}

// cellOf is the one conversion: an authored absolute offset [col,row] to the
// dungeon-absolute axial cell the session speaks, by asking the toolkit.
func cellOf(o tkencounter.Orientation, at spatial.Position) spatial.Position {
	return tkencounter.HexCellAt(o, int(at.X), int(at.Y))
}

// validateScenarios constructs every scenario the file binds, so an author's
// bad binding answers as field errors on PutDungeon. It returns no endings:
// the session package builds the world a run plays in (Launch), and the
// endings come from it.
//
// THIS PACKAGE LEARNS NO SCENARIO WORD (design §7). It never reads a binding
// key, never knows what an artifact is, and authors no default: it looks the
// id up in the rulebook's registry, hands the author's map and the narrowed
// dungeon facts to that package's own New, and carries back whatever it
// declares. Every refusal is the constructor's own sentence, in the words a
// person filling in the form can act on.
//
// A refusal comes back as a *tkdungeonspec.ValidationError so it travels the
// path a bad file already travels: the registry turns an ErrBadSpec into the
// wire's FieldError list, so PutDungeon answers the builder with the
// sentence on the binding block it is about rather than failing the call.
// The PATH names the block and not the field — the field key is inside the
// constructor's sentence, which is the rulebook's own doing and not
// something this package may parse.
//
// SORTED, because Compiled.Scenarios is a map: two runs of the same file
// must name the same first refusal.
func validateScenarios(spec tkdungeonspec.Compiled) error {
	if len(spec.Scenarios) == 0 {
		return nil
	}

	ids := make([]string, 0, len(spec.Scenarios))
	for id := range spec.Scenarios {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	facts := tkscenarios.FactsFrom(spec.Field)
	for _, id := range ids {
		scenario, known := tkscenarios.Lookup(id)
		if !known {
			// Refused by name, never guessed at: a build that does not have
			// this scenario cannot run the dungeon bound to it, and saying
			// so on the binding is the only answer that helps an author.
			return &tkdungeonspec.ValidationError{Errors: []tkdungeonspec.FieldError{{
				Path:    scenarioPath(id),
				Message: fmt.Sprintf("no scenario named %q — this build offers %s", id, offeredScenarios()),
			}}}
		}
		if _, err := scenario.New(spec.Scenarios[id], facts); err != nil {
			return &tkdungeonspec.ValidationError{Errors: []tkdungeonspec.FieldError{{
				Path: scenarioPath(id), Message: err.Error(),
			}}}
		}
	}

	return nil
}

// scenarioPath is the YAML path of one scenario's binding block, the shape
// dungeonspec's own defects use.
func scenarioPath(id string) string { return "scenarios." + id }

// offeredScenarios lists what this build does have, so an author who
// mistyped an id is told what they could have meant rather than only what
// they could not.
func offeredScenarios() string {
	all := tkscenarios.All()
	if len(all) == 0 {
		return "none"
	}
	ids := make([]string, len(all))
	for i, s := range all {
		ids[i] = s.ID()
	}

	return strings.Join(ids, ", ")
}
