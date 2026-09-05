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

	"github.com/KirkDiggler/rpg-toolkit/core"
	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	tkdungeonspec "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter/dungeonspec"
	tkscenarios "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter/scenarios"
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

	// World is the compiled world, and it is EMPTY OF MEMBERS on purpose.
	//
	// session.StartSessionInput.World is documented as "authored content...
	// not a live encounter", and the composition enforces that reading:
	// encounter.Join refuses a member who is already in the encounter, so a
	// world with the party baked in could never be joined -- and Join is what
	// loads a character's sheet through the host's repository. Monsters have
	// the mirror problem: a construction-time monster has no sheet, and
	// session.Spawn is what builds one from a ref.
	//
	// So the world carries the FIELD -- chambers, walls, doors, props -- and
	// everybody who stands in it arrives through a session verb.
	World *tkencounter.EncounterData

	// PartySeats are where the party comes in, dungeon-absolute, best seat
	// first.
	//
	// A list because a party is more than one person and the author declares
	// one cell: the first is the cell they wrote, the rest are that chamber's
	// other free cells nearest-first, so a caller with four players takes the
	// first four and gets them standing together at the way in.
	PartySeats []spatial.Position

	// Monsters is every authored monster, dungeon-absolute, in the order the
	// compiler reported them.
	Monsters []Monster
}

// Monster is one authored monster, ready to hand to session.Spawn.
type Monster struct {
	// Ref is content's identifier, exactly as authored -- "dnd5e:monsters:skeleton".
	Ref string

	// MemberID is the ID this monster is known by inside the encounter, and it
	// is derived here rather than left to the caller because it is the ONLY
	// thing a client ever sees of it: a Member on the wire is {id, kind,
	// position}, with no ref and no display name anywhere on it. An opaque
	// "monster-2" would therefore be all a UI had to draw a skeleton with.
	//
	// THE AUTHOR'S ID WHEN THEY GAVE ONE (`place[].id`; rpg-project#375,
	// ruled on the hold-out): a placement named `chief` is the member
	// `chief`. That is what lets `factions[].mind: chief` and
	// `{ down: chief }` in the file mean the same member in the run — the
	// composition validates a faction's mind against the id the member
	// JOINS under, so a launch that renamed the chief on the way in would
	// spawn a camp whose mind never arrives and which can never learn.
	//
	// Otherwise the ref's own id plus a per-ref ordinal -- skeleton-1,
	// skeleton-2, skeleton-captain-1 -- which is unique, stable across a
	// recompile of the same content, and legible in a log, a story beat and a
	// client alike. Numbering PER REF rather than per dungeon so that adding a
	// prop or reordering a chamber cannot silently renumber a monster that
	// nothing about it changed; and an ordinal is spent on every placement
	// of the ref, named or not, for the same reason -- naming one skeleton
	// `scout` must not renumber the one after it.
	//
	// ONE ID, ONE MONSTER. An authored id that spells what the ordinal would
	// have minted for some other placement (`id: skeleton-1` on the second
	// skeleton) is refused at compile, naming both placements, rather than
	// letting the second spawn fail as "no such member" halfway through a
	// launch. The launch makes the same refusal one seam out, against the
	// party's own ids ([lobby.StartEncounter]).
	MemberID string

	// At is its cell, dungeon-absolute.
	At spatial.Position

	// Boss is whether this is the monster whose death ends things.
	//
	// ACTED ON since rpg-project#268 — this was "carried and not yet acted
	// on" through the wave that recorded it so the trigger's wave would have
	// the fact already flowing, and that wave is here: Compile turns the
	// flag into a declared [tkencounter.TriggerMemberDown] ending keyed
	// [EndingBossDown], naming this monster's MemberID. When the member the
	// launch spawns under that ID goes down, the encounter closes with that
	// outcome — the fight dissolves first, the run ends on the world clock
	// (rpg-project#269 §6.6).
	Boss bool

	// Targeting is the author's word for how it picks a target, or empty.
	//
	// ALSO CARRIED AND NOT YET ACTED ON, and for a sharper reason: session's
	// SpawnInput has no field for it, so it cannot cross the seam today even
	// though both sides know it. It is kept here rather than dropped at the
	// compile so the gap is visible at the seam that has it, not invisible in
	// the package that threw it away.
	Targeting string

	// PlacementID is the author's own name for this placement
	// (`place[].id`), or empty when they gave it none (rpg-project#368,
	// design P2).
	//
	// SEPARATE FROM MemberID, and deliberately: MemberID is derived here so
	// every monster has one, and this is the author's word, which most
	// placements do not have. Nothing binds a monster by id in this slice —
	// the scenario that will is kill-the-captain, R8's named follow-up — so
	// this is carried for the same reason Targeting beside it is: the fact
	// exists in the file and belongs at the seam that has it.
	PlacementID string

	// Holds is the intel records this monster carries, by COMPILED record id
	// (`<key>/<id>`) — the author's knowledge, which Loot copies off the
	// body (rpg-project#372). Nil when the monster holds nothing.
	//
	// A RECORD, NOT A DOOR. Until this slice a monster carried a door id
	// directly, which made "what does knowing this tell you" a question with
	// exactly one possible answer forever. The record puts one indirection
	// in the middle: what it reveals is read from the field's intel table
	// when it changes hands, so the same fact kind serves a region, a
	// treasure's location or a lock's approach the day a use case asks for
	// one — and none of that touches who holds it. `Knows` is deleted
	// upstream and refused by name in the file.
	//
	// COMPILED IDS, NOT THE AUTHOR'S. dungeonspec mints `<key>/<id>` so two
	// dungeons in one process cannot collide, and it is the minted form that
	// arrives here. Forwarding the raw authored id names a record the
	// composition does not have, and the seam says so loudly rather than
	// spawning a monster that holds nothing (session.ErrNoIntel).
	//
	// NEVER PROJECTED. Who carries intel reaches no wire, no atlas and no
	// beat (slice 2 design P3): a monster holding nothing and one holding
	// the way into the vault are byte-identical to every observer until
	// somebody loots them.
	Holds []string

	// Faction is the faction this monster was placed in, AS AUTHORED
	// (`place[].faction`, rpg-project#375): empty when the author wrote
	// none, which the composition reads as the reserved `monsters` faction
	// -- so a dungeon authored before factions existed spawns exactly as it
	// did. Verbatim, not key-prefixed: a faction is a word the roster shows
	// and a scenario binds by name.
	//
	// THE ONE FACT ABOUT SIDES THAT NEEDS FORWARDING. Factions, dispositions
	// and what a record reveals are field structure and ride Compiled.Field
	// whole into the world ([TestFactionsRideTheFieldRatherThanBeingForwarded]);
	// a monster's membership is the one thing that is about the MEMBER, and
	// a member enters the run through session.Spawn rather than the field,
	// so it is hand-carried across that seam exactly as Holds is.
	//
	// FORWARDED VERBATIM by the launch (internal/orchestrators/lobby's
	// StartEncounter) to session.SpawnInput.Faction -- empty stays empty,
	// never defaulted on this side. It has to be: the composition refuses a
	// faction's MIND that joins any faction but its own (ErrNoFaction), so a
	// launch that dropped this field would fail closed at the chief's spawn,
	// naming him, rather than start a camp that can never learn.
	Faction string
}

// Compile turns one authored dungeon file into a [Dungeon].
//
// A file that does not decode, validate or compile fails with an error that
// wraps [tkdungeonspec.ErrBadSpec] -- a validation failure is a
// *tkdungeonspec.ValidationError carrying every defect and its YAML path;
// anything else is an internal failure. Callers that need to tell the two
// apart (the authoring RPC answers the first as a body and the second as a
// status) use errors.Is / errors.As.
func Compile(raw []byte) (*Dungeon, error) {
	decoded, err := tkdungeonspec.Decode(raw)
	if err != nil {
		return nil, fmt.Errorf("decode spec: %w", err)
	}
	if defects := tkdungeonspec.Validate(decoded); len(defects) > 0 {
		return nil, fmt.Errorf("validate spec: %w", &tkdungeonspec.ValidationError{Errors: defects})
	}
	spec, err := tkdungeonspec.Compile(decoded)
	if err != nil {
		return nil, fmt.Errorf("compile spec: %w", err)
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

	monsters := make([]Monster, len(spec.Monsters))
	ordinals := map[string]int{}
	claimed := map[string]int{}
	for i, m := range spec.Monsters {
		id, idErr := memberIDFor(m.ID, m.Ref, ordinals)
		if idErr != nil {
			return nil, fmt.Errorf("monster %d: %w", i, idErr)
		}
		if prev, taken := claimed[id]; taken {
			return nil, fmt.Errorf(
				"member id %q is claimed twice: by %s and by %s — a monster's member id is its authored id, "+
					"or its ref plus an ordinal when it has none, and two monsters cannot share one",
				id, describePlacement(spec.Monsters[prev]), describePlacement(m),
			)
		}
		claimed[id] = i
		monsters[i] = Monster{
			Ref: m.Ref, MemberID: id, At: cellOf(orientation, m.At),
			Boss: m.Boss, Targeting: m.Targeting,
			PlacementID: m.ID, Holds: m.Holds, Faction: m.Faction,
		}
	}

	// dungeonspec validates at most one boss PER REGION; across regions a
	// file could still author several, and "whose death ends things" cannot
	// be plural while the doom names one member — refused here, loudly, so
	// an authoring mistake fails at compile rather than leaving a run that
	// never ends when the "real" boss falls. Softens when the builder
	// (#169) brings authored multi-ending variety.
	var bossID string
	for _, m := range monsters {
		if !m.Boss {
			continue
		}
		if bossID != "" {
			return nil, fmt.Errorf(
				"dungeon %q authors more than one boss (%q and %q): one death ends things, and it cannot be two",
				decoded.Key,
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

	world, err := buildWorld(spec.Field, bossID, scenarioEndings)
	if err != nil {
		return nil, err
	}

	return &Dungeon{
		Key: decoded.Key, Name: decoded.Name,
		World: world, PartySeats: seats, Monsters: monsters,
	}, nil
}

// cellOf is the one conversion: an authored absolute offset [col,row] to the
// dungeon-absolute axial cell the session speaks, by asking the toolkit.
func cellOf(o tkencounter.Orientation, at spatial.Position) spatial.Position {
	return tkencounter.HexCellAt(o, int(at.X), int(at.Y))
}

// memberIDFor derives a monster's in-encounter ID: the author's own when the
// placement has one, else its ref plus how many of that ref have been placed
// so far. See [Monster.MemberID] for why the ID is built from the ref at all,
// and why the ordinal is spent whether or not it is used.
//
// The ref is PARSED rather than string-split, and parsed for a named
// placement too, so a malformed one is an error here instead of a member
// called "dnd5e:monsters:skeleton-1" that nothing downstream can read.
func memberIDFor(authored, ref string, ordinals map[string]int) (string, error) {
	parsed, err := core.ParseString(ref)
	if err != nil {
		return "", fmt.Errorf("parse ref %q: %w", ref, err)
	}
	ordinals[ref]++
	if authored != "" {
		return authored, nil
	}

	return fmt.Sprintf("%s-%d", parsed.ID, ordinals[ref]), nil
}

// describePlacement names one monster placement the way an author would find
// it in the file: by its id when it has one, and by its ref and authored
// cell either way.
func describePlacement(m tkdungeonspec.MonsterPlacement) string {
	where := fmt.Sprintf("%s at [%d,%d]", m.Ref, int(m.At.X), int(m.At.Y))
	if m.ID == "" {
		return where
	}
	return fmt.Sprintf("%q (%s)", m.ID, where)
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
// scenario packages (see [endingsOfScenarios]).
//
// bossID, when non-empty, is the member whose death ends the dungeon — an
// ending may name a member that has not joined yet (the same contract
// TriggerReachedPosition's filter has), which is what lets an empty world
// declare a doom for a monster the launch spawns minutes later.
func buildWorld(
	field tkencounter.FieldInput, bossID string, scenarioEndings []tkencounter.EndingInput,
) (*tkencounter.EncounterData, error) {
	enc, err := tkencounter.NewEncounter(&tkencounter.SetupInput{
		// Construction-time capabilities only. The session package supplies its
		// own when it loads this world -- including the sight RANGE that decides
		// who is in contact with whom -- so these satisfy NewEncounter's
		// validation for a world that is empty at the moment it is built, and
		// answer no question about the game. They are trivial because there is
		// nobody here yet to see or to be standing, not because a ruling was
		// made quietly.
		Initiative: orderAsGiven{},
		Standing:   nobodyDown{},
		Sight:      nobodySees{},
		// Same trivial stand-in as above: this world is empty at the moment
		// it is built, so no clock ever lands on anyone here either
		// (toolkit#1162, ADR-0043). Striker is the same story one seam over
		// (rpg-project#254). The session package supplies its own
		// TurnDriver -- session.Behavior() today -- and its own Striker
		// when it loads this world to actually play it.
		TurnDriver: tkencounter.PassDriver{},
		Striker:    tkencounter.RefusingStriker{},
		// And REFUSING one seam further on. A world with nobody in it has no
		// clock to advance, so a temporal boundary announced while building
		// one is a bug rather than an event -- and this says so at the point
		// of failure instead of succeeding quietly, which is the whole reason
		// the capability was introduced (rpg-project#294). The session package
		// supplies the real announcer when it loads this world to play it.
		Announcer: tkencounter.RefusingAnnouncer{},
		// Concealment (rpg-toolkit#1371) closes two more capabilities the
		// moment a field carries any concealed door or region, and refuses
		// to build AT ALL without them -- so before this pair existed, any
		// dungeon declaring concealment failed to compile, full stop
		// (rpg-api#887). They are not a matched refusing pair the way
		// PassDriver/RefusingStriker are -- see the comment above
		// orderAsGiven for why CheckResolver refuses and Witness does not.
		CheckResolver: refusingCheckResolver{},
		Witness:       nobodyPerceives{},
		Retention:     tkencounter.RetentionUnbounded,
		Field:         field,
		// What ends a dungeon is not geometry, and the file says it: the
		// party withdrawing (external, always declared), the boss going
		// down when a placement carries the flag (rpg-project#268; see
		// [Monster.Boss]), and — since rpg-project#368 — whatever each
		// scenario the file BINDS declares for itself. A dungeon is
		// geometry; a scenario is what it is for.
		Endings: endingsFor(bossID, scenarioEndings),
	})
	if err != nil {
		return nil, fmt.Errorf("build world: %w", err)
	}

	data := enc.ToData()

	return &data, nil
}

// endingsFor is every ending an authored dungeon declares: withdrawal
// always, the boss's fall when a placement names one, and whatever each
// bound scenario declared (rpg-project#368, design R8).
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
func endingsFor(bossID string, scenarioEndings []tkencounter.EndingInput) []tkencounter.EndingInput {
	endings := []tkencounter.EndingInput{
		{Key: EndingWithdrawn, Trigger: tkencounter.TriggerExternal{}},
	}
	if bossID != "" {
		endings = append(endings, tkencounter.EndingInput{
			Key:     EndingBossDown,
			Trigger: tkencounter.TriggerMemberDown{Member: tkencounter.MemberID(bossID)},
		})
	}

	return append(endings, scenarioEndings...)
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
	// (see nobodyDown.Standing) is that a caller must not have to tell "this
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

// orderAsGiven, nobodyDown and nobodySees are three of the capabilities
// NewEncounter refuses to default (rpg-toolkit#1033). TurnDriver and
// Striker close two more, added by toolkit#1162/ADR-0043 and
// rpg-project#254 respectively (supplied, never assumed) -- both are
// tkencounter.PassDriver{} and tkencounter.RefusingStriker{} directly,
// exported by the toolkit (rpg-toolkit#1167 closed), so there is no
// hand-written stand-in for either any more.
//
// Concealment (rpg-toolkit#1371) closes two further -- CheckResolver and
// Witness -- and the toolkit exports no refusing implementation of either,
// so refusingCheckResolver and nobodyPerceives below are hand-written to
// match (rpg-api#887). They are deliberately NOT a matched refusing pair
// the way PassDriver/RefusingStriker are:
//
//   - CheckResolver is reached only through an explicit Search
//     (rulebooks/dnd5e/encounter's own conceal.go), and nothing in this
//     package ever searches, so refusing it costs nothing and catches a
//     real mistake if one is ever made.
//   - Witness is reached UNCONDITIONALLY, at every construction's first
//     light, for any door authored both concealed and open -- a "hidden
//     passage nobody shut", which is legal authored content, not a bug
//     (confirmed against the toolkit directly: NewEncounter calls
//     Witness.Perceivers for such a door even with zero members, because
//     first light perceives present state before anyone has joined). A
//     refusing Witness would fail that dungeon's compile exactly the way
//     the missing capability failed every dungeon's compile before this
//     fix, so nobodyPerceives answers honestly instead.
//
// All five hand-written types in this file are construction-time only,
// which is what makes a trivial or refusing implementation honest rather
// than a hidden ruling -- see [buildWorld]: the world is empty at the
// moment it is built, and the session package supplies its own
// capabilities when it loads it.
type orderAsGiven struct{}

// RollInitiative returns the members in the order given. Never reached: the
// world this package builds is empty, so no fight can form in the moment it
// exists.
func (orderAsGiven) RollInitiative(members []tkencounter.MemberID) ([]tkencounter.MemberID, error) {
	return members, nil
}

type nobodyDown struct{}

// Standing reports who is DOWN, not who is up -- the interface's own parameter
// is named down, and reading it backwards would report a healthy party as a
// wiped one. Nobody has been hit yet in a world this new, so: nobody, said as
// an empty list rather than a nil one. A nil slice with a nil error is the
// shape this repo never returns: a caller cannot tell "nobody is down" from
// "nothing was answered".
func (nobodyDown) Standing(_ []tkencounter.MemberID) ([]tkencounter.MemberID, error) {
	return []tkencounter.MemberID{}, nil
}

// Assess is the richer half of the same answer, required of a Standing
// capability since encounter/v0.51.0 (toolkit#1453): NewEncounter refuses a
// Standing that is not also a Participation. It says exactly what Standing
// above says, in the fuller vocabulary -- nobody is down, so everybody is up,
// conscious, IN CONTACT, and waiting for their player or driver -- which is
// the session package's own bridge answer for an undowned member, verbatim
// (session.standingSeam.Assess).
//
// CONTACT IS TRUE ON PURPOSE. It is what decides whether a member counts as a
// side of a fight (encounter.fightIsDecided), so answering false would dissolve
// every fight the moment it formed, quietly, and this stand-in would be making
// a ruling instead of standing in for one. Party-defeat and keep-turn-order
// stay false: they are group policy the rulebook owns, and nothing here rules
// on them.
func (nobodyDown) Assess(members []tkencounter.MemberID) (*tkencounter.ParticipationAssessment, error) {
	out := &tkencounter.ParticipationAssessment{
		Members: make([]tkencounter.MemberParticipation, 0, len(members)),
	}
	for _, id := range members {
		out.Members = append(out.Members, tkencounter.MemberParticipation{
			Member:    id,
			Contact:   true,
			Conscious: true,
			Turn:      tkencounter.TurnParticipationWait,
		})
	}
	return out, nil
}

type nobodySees struct{}

// Sight gives every member a range of zero -- there are no members in a world
// this new, so it answers nothing; see [buildWorld].
func (nobodySees) Sight(members []tkencounter.MemberID) (map[tkencounter.MemberID]int, error) {
	out := make(map[tkencounter.MemberID]int, len(members))
	for _, id := range members {
		out[id] = 0
	}

	return out, nil
}

type refusingCheckResolver struct{}

// ResolveCheck always fails. Nothing during construction ever calls this --
// [tkencounter.CheckResolver] is reached only through an explicit Search,
// and this package never searches -- so reaching it at all means a caller
// ran Search against the throwaway world this package builds instead of the
// live one the session package loads from it, which is a bug and should say
// so at the point of failure, the same argument [tkencounter.RefusingStriker]
// makes for a driven attack, one capability over. See the comment above
// [orderAsGiven] for why [nobodyPerceives] beside it does not follow suit.
func (refusingCheckResolver) ResolveCheck(*tkencounter.ResolveCheckInput) (*tkencounter.ResolveCheckOutput, error) {
	return nil, fmt.Errorf("sessionworld: an authored check was rolled against a world still being compiled, not played")
}

type nobodyPerceives struct{}

// Perceivers answers that nobody perceives the door -- there is nobody in a
// world this new to perceive anything, which happens to be the literal
// answer a live Witness would give an encounter with no roster. NOT a
// refusal, unlike [refusingCheckResolver] beside it: NewEncounter's first
// light calls this UNCONDITIONALLY for any door authored both concealed and
// open (a hidden passage nobody shut, legal authored content), so a
// refusing stand-in here would fail that dungeon's compile the same way the
// missing capability failed every concealed dungeon's compile before this
// fix (rpg-api#887). See the comment above [orderAsGiven] for the full
// reasoning.
func (nobodyPerceives) Perceivers(*tkencounter.PerceiversInput) ([]tkencounter.MemberID, error) {
	return []tkencounter.MemberID{}, nil // nobody, as an empty list: never nil with a nil error
}
