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

	// World is the compiled field with nobody standing in it, and it serves
	// ONE reader: the registry's atlas preview (session.Manager.AtlasOf takes
	// a world, not a compiled spec). A run is never started from it —
	// session.Manager.Launch builds its own world from Spec. It is built
	// through encounter's compile-only constructor, the one such call left in
	// rpg-api, until the SDK projects an atlas from a compiled spec.
	World *tkencounter.EncounterData

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
	for _, m := range spec.Monsters {
		if _, known := monsters.ByRef(m.Ref); !known {
			id := m.ID
			if id == "" {
				id = m.Ref
			}
			return nil, &tkdungeonspec.ValidationError{Errors: []tkdungeonspec.FieldError{{
				Message: fmt.Sprintf("monster %q references unknown monster %q", id, m.Ref),
			}}}
		}
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
	scenarioEndings, err := endingsOfScenarios(spec)
	if err != nil {
		return nil, fmt.Errorf("bind scenarios: %w", err)
	}

	world, err := buildWorld(spec.Field, bossID, scenarioEndings, spec.Endings)
	if err != nil {
		return nil, err
	}

	return &Dungeon{
		Key: spec.Key, Name: spec.Name,
		World: world, PartySeats: seats, Spec: &spec,
	}, nil
}

// cellOf is the one conversion: an authored absolute offset [col,row] to the
// dungeon-absolute axial cell the session speaks, by asking the toolkit.
func cellOf(o tkencounter.Orientation, at spatial.Position) spatial.Position {
	return tkencounter.HexCellAt(o, int(at.X), int(at.Y))
}

// buildWorld constructs the world the session actually plays in: the compiled
// field, and nobody standing in it. See [Dungeon.World] for why it is empty.
//
// THE INTEL TABLE RIDES THE FIELD and needs no wiring of its own: a
// dungeon's authored records are construction truth, exactly like its doors
// and its exits, so [tkdungeonspec.Compiled] carries them on Field.Intel and
// this function hands the whole field over. That is a fact worth stating
// rather than leaving implicit, because it is the reason a record's
// `reveals` can be read at transfer time at all — the composition looks the
// record up in its own table, and a table this package forgot to pass would
// make every loot of an intel-holding body silently reveal nothing.
// [TestTheIntelTableIsConstructionTruth] pins it.
//
// scenarioEndings are the endings every bound scenario declared, already
// constructed and validated against this dungeon by the rulebook's own
// scenario packages (see [endingsOfScenarios]); authoredEndings are the
// file's own `endings[]`, compiled by dungeonspec to the same Trigger types
// (rpg-project#375 step B, R10) and declared beside them (see [endingsFor]).
//
// bossID, when non-empty, is the member whose death ends the dungeon — an
// ending may name a member that has not joined yet (the same contract
// TriggerReachedPosition's filter has), which is what lets an empty world
// declare a doom for a monster the launch spawns minutes later.
func buildWorld(
	field tkencounter.FieldInput, bossID string,
	scenarioEndings, authoredEndings []tkencounter.EndingInput,
) (*tkencounter.EncounterData, error) {
	// Construction-time capabilities only: encounter's own compile-only
	// stand-ins (rpg-toolkit#1956). The session package supplies the real
	// ones when it loads this world to play it.
	//
	// What ends a dungeon is not geometry, and the file says it: the party
	// withdrawing (external, always declared), the boss going down when a
	// placement carries the flag (rpg-project#268; see [Monster.Boss]), and
	// -- since rpg-project#368 -- whatever each scenario the file BINDS
	// declares for itself. A dungeon is geometry; a scenario is what it is
	// for.
	setup := tkencounter.CompileOnlySetup(field, endingsFor(bossID, scenarioEndings, authoredEndings))
	setup.Retention = tkencounter.RetentionUnbounded
	enc, err := tkencounter.NewEncounter(setup)
	if err != nil {
		return nil, fmt.Errorf("build world: %w", err)
	}

	data := enc.ToData()

	return &data, nil
}

// endingsFor is every ending an authored dungeon declares: withdrawal
// always, the boss's fall when a placement names one, whatever each bound
// scenario declared (rpg-project#368, design R8), and whatever the file
// declares in its own `endings[]` (rpg-project#375 step B, R10).
//
// THE AUTHORED ENDINGS ARE THE SCENARIO'S OWN GRAMMAR. `scenarios:
// { hold-out: { convince: raiders } }` is sugar for `endings: [{ id:
// hold-out, when: { stance: { between: [raiders, party], is: neutral } } }]`
// -- dungeonspec compiles the spelled-out form to the very Trigger the
// scenario package constructs, and this function declares both lists to
// the composition the same way, so the two spellings produce the same
// world ([TestAnEndingAuthoredInTheFileIsTheScenariosOwn]). A scenario
// package with nothing left to do is the north star's own test.
//
// THE BOSS ARM IS UNTOUCHED BY THIS SLICE, deliberately. R8 ruled that
// `boss:` keeps working here and is retired by the NAMED FOLLOW-UP — the one
// that turns the reference tomb into the kill-the-captain scenario and
// deletes the flag with its content re-put. A dungeon may legally declare
// both, and one that does simply has two ways to end; that is the author's
// business, visible on the form.
//
// A scenario ending that collided with "withdrawn" or "boss-down" is refused
// by the composition itself (NewEncounter names the duplicate key and returns
// ErrNoEnding), so there is no second copy of that rule here to drift from
// the first.
func endingsFor(bossID string, scenarioEndings, authoredEndings []tkencounter.EndingInput) []tkencounter.EndingInput {
	endings := []tkencounter.EndingInput{
		{Key: EndingWithdrawn, Trigger: tkencounter.TriggerExternal{}},
	}
	if bossID != "" {
		endings = append(endings, tkencounter.EndingInput{
			Key:     EndingBossDown,
			Trigger: tkencounter.TriggerMemberDown{Member: tkencounter.MemberID(bossID)},
		})
	}
	endings = append(endings, scenarioEndings...)

	return append(endings, authoredEndings...)
}

// endingsOfScenarios constructs every scenario the file binds and returns the
// endings they declare, in the order their ids sort.
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
// must declare the same endings in the same order, or the world a dungeon
// compiles to depends on Go's map iteration.
func endingsOfScenarios(spec tkdungeonspec.Compiled) ([]tkencounter.EndingInput, error) {
	// An empty list, never a nil one with a nil error: this package's own law
	// (see encounter's compile-only Standing) is that a caller must not have to tell "this
	// dungeon binds no scenario" from "nothing was answered".
	endings := []tkencounter.EndingInput{}
	if len(spec.Scenarios) == 0 {
		return endings, nil
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
			return nil, &tkdungeonspec.ValidationError{Errors: []tkdungeonspec.FieldError{{
				Path:    scenarioPath(id),
				Message: fmt.Sprintf("no scenario named %q — this build offers %s", id, offeredScenarios()),
			}}}
		}
		declared, err := scenario.New(spec.Scenarios[id], facts)
		if err != nil {
			return nil, &tkdungeonspec.ValidationError{Errors: []tkdungeonspec.FieldError{{
				Path: scenarioPath(id), Message: err.Error(),
			}}}
		}
		endings = append(endings, declared.Endings...)
	}

	return endings, nil
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

// EndingWithdrawn is the key of one of the two endings an authored dungeon
// declares: the party left. Exported because a caller that wants to close an
// encounter has to name it.
const EndingWithdrawn = "withdrawn"

// EndingBossDown is the other: the authored boss went down and the dungeon
// is cleared. Keys are content vocabulary — the client maps key to sentence
// (rpg-project#269 §6.3) — and this one fires from inside the composition
// (TriggerMemberDown), never from a caller naming it.
const EndingBossDown = "boss-down"
