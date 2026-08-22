// Package sessionworld turns authored dungeon content into the three things
// the new session stack needs to start a game in it: a world, the cells the
// party walks in on, and the monsters standing in it.
//
// # Why this package exists at all, and why it is thin
//
// rpg-toolkit#1133 landed the compiler this is built on
// (rulebooks/dnd5e/encounter/dungeonspec), so rpg-api computes NO dungeon
// geometry -- the same arrangement the old stack has always had, one stack
// over. What is left for a host is the seam the compiler deliberately does not
// cross: it emits placements in the AUTHORED frame (a chamber's own columns and
// rows), and the session package's Join and Spawn take DUNGEON-ABSOLUTE cells.
// Somebody has to carry a placement across that line, and this package is that
// somebody.
//
// # The projection is BORROWED, never recomputed
//
// It would be a few lines to add a room's Origin to a local cell and convert to
// axial. It would also be wrong, and quietly: the entrance's local (1,3) is the
// absolute cell (0,3), because an offset rectangle SHEARS when it becomes
// axial (rpg-toolkit#1131). A copy of that arithmetic in rpg-api is a second
// implementation of the toolkit's geometry that compiles, looks right, and
// drifts the day the projection changes -- exactly the Boundary Rule violation
// the whole session seam exists to prevent. The day came twice: rpg-toolkit#1141
// corrected the hex offset schemes (the cell moved from (1,-4)) and
// rpg-toolkit#1150 corrected the axial basis (from (0,-3)). Nothing here
// changed either time but the number in the test, which is the point.
//
// So this package does not do the conversion. It asks the composition to do it,
// by building a THROWAWAY encounter with the authored placements as
// construction-time members ([encounter.MemberInput] is room-shaped on purpose)
// and reading the absolute cells back out. The projection happens exactly once,
// inside the toolkit, in the same code path the real world will use, because it
// is the same [encounter.FieldInput]. See [compile] for the mechanics.
//
// # Known cost, recorded rather than hidden
//
// That costs one extra construction per encounter start, and it is a shape
// worth removing rather than living with: dungeonspec.Compiled could carry
// absolute placements itself, or the session package could accept a compiled
// dungeon. Filed upstream; until one of those exists this is the honest way to
// get a party into an authored dungeon without a host inventing geometry.
package sessionworld

import (
	_ "embed"
	"fmt"

	"github.com/KirkDiggler/rpg-toolkit/core"
	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	tkdungeonspec "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter/dungeonspec"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

// referenceTombYAML is the reference tomb, authored in the dialect
// rulebooks/dnd5e/encounter/dungeonspec speaks.
//
// It lives here rather than in internal/content/dungeons/ for a reason that
// would otherwise be discovered the hard way: that directory is embedded by
// `//go:embed dungeons/*.yaml` and every file matching it is parsed with the
// OLD stack's dialect at startup, in a mode that PANICS on a file it cannot
// read. A new-dialect file dropped in beside its old-dialect sibling would take
// the server down on boot.
//
// It is the same dungeon as internal/content/dungeons/reference-tomb.yaml --
// verified placement-by-placement, every (ref, cell) pair identical, boss
// included -- re-authored in the newer dialect per Kirk's ruling that the old
// form "has never run in a game... it is legacy and will have no home after".
// The two files are expected to coexist only until the old stack is ripped out
// (rpg-project#227's cutover), and the duplication is the price of the
// coexistence the whole integration is built on: each stack reads content in
// the dialect it can actually compile.
//
//go:embed reference-tomb.yaml
var referenceTombYAML []byte

// Dungeon is authored content compiled into everything needed to seed a
// session, with every cell already in the dungeon-absolute frame the session
// verbs speak.
type Dungeon struct {
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
	// So it is the ref's own id plus a per-ref ordinal -- skeleton-1,
	// skeleton-2, skeleton-captain-1 -- which is unique, stable across a
	// recompile of the same content, and legible in a log, a story beat and a
	// client alike. Numbering PER REF rather than per dungeon so that adding a
	// prop or reordering a chamber cannot silently renumber a monster that
	// nothing about it changed.
	MemberID string

	// At is its cell, dungeon-absolute.
	At spatial.Position

	// Boss is whether this is the monster whose death ends things.
	//
	// CARRIED AND NOT YET ACTED ON. Nothing here turns it into an ending: the
	// compiler emits no endings, and a "the boss died" trigger does not exist
	// in the composition's Trigger set (there is TriggerReachedPosition and
	// TriggerExternal, and that is all). Recorded on the placement so the wave
	// that adds the trigger has the fact already flowing, rather than
	// discovering the compiler knew all along.
	Boss bool

	// Targeting is the author's word for how it picks a target, or empty.
	//
	// ALSO CARRIED AND NOT YET ACTED ON, and for a sharper reason: session's
	// SpawnInput has no field for it, so it cannot cross the seam today even
	// though both sides know it. It is kept here rather than dropped at the
	// compile so the gap is visible at the seam that has it, not invisible in
	// the package that threw it away.
	Targeting string
}

// ReferenceTomb compiles the reference tomb: three chambers west to east, a
// garrison of skeletons in the hall, and a captain behind a DC-12 locked door.
func ReferenceTomb() (*Dungeon, error) {
	d, err := compile(referenceTombYAML)
	if err != nil {
		return nil, fmt.Errorf("reference tomb: %w", err)
	}

	return d, nil
}

// compile does the work described in the package comment: compile once, build a
// throwaway encounter to borrow the composition's projection, then build the
// real world with nobody in it.
func compile(raw []byte) (*Dungeon, error) {
	spec, err := tkdungeonspec.Load(raw)
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

	seats, monsters, err := projectPlacements(spec)
	if err != nil {
		return nil, err
	}

	world, err := buildWorld(spec.Field)
	if err != nil {
		return nil, err
	}

	return &Dungeon{World: world, PartySeats: seats, Monsters: monsters}, nil
}

// seatMemberID and monsterMemberID name the throwaway encounter's members.
//
// They exist only long enough to be placed and read back, so what matters about
// them is that they are unique and that the two families cannot collide -- an
// absolute cell is looked up BY these IDs, and two placements sharing one would
// silently report the same cell twice.
func seatMemberID(i int) tkencounter.MemberID {
	return tkencounter.MemberID(fmt.Sprintf("probe-seat-%d", i))
}

func monsterMemberID(i int) tkencounter.MemberID {
	return tkencounter.MemberID(fmt.Sprintf("probe-monster-%d", i))
}

// projectPlacements turns the compiler's room-local placements into
// dungeon-absolute ones by asking the composition, never by doing the
// arithmetic here. See the package comment for why that distinction is the
// whole point of this file.
func projectPlacements(spec tkdungeonspec.Compiled) ([]spatial.Position, []Monster, error) {
	members := make([]tkencounter.MemberInput, 0, len(spec.PartyStart)+len(spec.Monsters))
	for i, seat := range spec.PartyStart {
		members = append(members, tkencounter.MemberInput{
			ID: seatMemberID(i), Kind: tkencounter.KindPlayer,
			Room: seat.Region, Position: seat.At,
		})
	}
	for i, m := range spec.Monsters {
		members = append(members, tkencounter.MemberInput{
			ID: monsterMemberID(i), Kind: tkencounter.KindMonster,
			Room: m.Region, Position: m.At,
		})
	}

	probe, err := tkencounter.NewEncounter(&tkencounter.SetupInput{
		Initiative: orderAsGiven{},
		Standing:   nobodyDown{},
		// A BLIND probe, and deliberately so: sight is what forms a fight, and
		// this construction places the whole party and the whole garrison at
		// once. Giving them eyes here would roll initiative on a world that is
		// about to be thrown away -- work done for nothing, and a fight formed
		// by a coordinate lookup. Range zero is the honest capability for an
		// encounter whose only purpose is to answer "where is this cell".
		Sight: nobodySees{},
		// No fight ever forms here (blind, thrown away), so no clock ever
		// lands on an unplayed member -- alwaysPasses is a trivial, honest
		// stand-in for the same reason Initiative/Standing/Sight above are
		// (toolkit#1162, ADR-0043).
		TurnDriver: alwaysPasses{},
		Retention:  tkencounter.RetentionUnbounded,
		Field:      spec.Field,
		Members:    members,
		Endings:    []tkencounter.EndingInput{{Key: probeEnding, Trigger: tkencounter.TriggerExternal{}}},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("project placements: %w", err)
	}

	placed, err := probe.Members()
	if err != nil {
		return nil, nil, fmt.Errorf("project placements: read back: %w", err)
	}
	absolute := make(map[tkencounter.MemberID]spatial.Position, len(placed))
	for _, m := range placed {
		absolute[m.ID] = m.Position
	}

	seats := make([]spatial.Position, len(spec.PartyStart))
	for i := range spec.PartyStart {
		at, ok := absolute[seatMemberID(i)]
		if !ok {
			return nil, nil, fmt.Errorf("project placements: seat %d was placed but not reported", i)
		}
		seats[i] = at
	}

	monsters := make([]Monster, len(spec.Monsters))
	ordinals := map[string]int{}
	for i, m := range spec.Monsters {
		at, ok := absolute[monsterMemberID(i)]
		if !ok {
			return nil, nil, fmt.Errorf("project placements: monster %d (%s) was placed but not reported", i, m.Ref)
		}
		id, err := memberIDFor(m.Ref, ordinals)
		if err != nil {
			return nil, nil, fmt.Errorf("project placements: monster %d: %w", i, err)
		}
		monsters[i] = Monster{Ref: m.Ref, MemberID: id, At: at, Boss: m.Boss, Targeting: m.Targeting}
	}

	return seats, monsters, nil
}

// memberIDFor derives a monster's in-encounter ID from its ref and how many of
// that ref have already been named. See [Monster.MemberID] for why the ID is
// built from the ref at all.
//
// The ref is PARSED rather than string-split, so a malformed one is an error
// here instead of a member called "dnd5e:monsters:skeleton-1" that nothing
// downstream can read.
func memberIDFor(ref string, ordinals map[string]int) (string, error) {
	parsed, err := core.ParseString(ref)
	if err != nil {
		return "", fmt.Errorf("parse ref %q: %w", ref, err)
	}
	ordinals[ref]++

	return fmt.Sprintf("%s-%d", parsed.ID, ordinals[ref]), nil
}

// buildWorld constructs the world the session actually plays in: the compiled
// field, and nobody standing in it. See [Dungeon.World] for why it is empty.
func buildWorld(field tkencounter.FieldInput) (*tkencounter.EncounterData, error) {
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
		// (toolkit#1162, ADR-0043). The session package supplies its own
		// TurnDriver -- session.Pass{} today -- when it loads this world to
		// actually play it.
		TurnDriver: alwaysPasses{},
		Retention:  tkencounter.RetentionUnbounded,
		Field:      field,
		// SetupInput requires at least one declared ending and the compiler
		// emits none: what ends a dungeon is not geometry. An externally
		// declared ending is the honest stand-in -- the party withdrawing is a
		// real way this tomb ends, and it is the only one the composition can
		// express today. See [Monster.Boss] for the one that is missing.
		Endings: []tkencounter.EndingInput{{Key: EndingWithdrawn, Trigger: tkencounter.TriggerExternal{}}},
	})
	if err != nil {
		return nil, fmt.Errorf("build world: %w", err)
	}

	data := enc.ToData()

	return &data, nil
}

// EndingWithdrawn is the key of the one ending an authored dungeon declares
// today: the party left. Exported because a caller that wants to close an
// encounter has to name it.
const EndingWithdrawn = "withdrawn"

// probeEnding names the throwaway encounter's ending. Distinct from
// [EndingWithdrawn] so that a probe can never be mistaken for a playable world
// in a stack trace or a persisted blob.
const probeEnding = "probe"

// orderAsGiven, nobodyDown, nobodySees and alwaysPasses are four of the
// capabilities NewEncounter refuses to default (rpg-toolkit#1033 for the
// first three; alwaysPasses closes the fourth, TurnDriver, added by
// toolkit#1162/ADR-0043: supplied, never assumed). alwaysPasses is
// hand-written rather than reused from the toolkit -- see its own doc
// comment and rpg-toolkit#1167.
//
// All four are construction-time only here, which is what makes trivial
// implementations honest rather than a hidden ruling -- see [buildWorld].
type orderAsGiven struct{}

// RollInitiative returns the members in the order given. Never reached for
// either encounter this package builds: the probe is blind and the real world
// is empty, so no fight can form in the moment either one exists.
func (orderAsGiven) RollInitiative(members []tkencounter.MemberID) ([]tkencounter.MemberID, error) {
	return members, nil
}

type nobodyDown struct{}

// Standing reports who is DOWN, not who is up -- the interface's own parameter
// is named down, and reading it backwards would report a healthy party as a
// wiped one. Nobody has been hit yet in a world this new, so: nobody.
func (nobodyDown) Standing(_ []tkencounter.MemberID) ([]tkencounter.MemberID, error) {
	return nil, nil
}

type nobodySees struct{}

// Sight gives every member a range of zero. See the call site in
// [projectPlacements] for why blindness is the correct capability for a
// coordinate lookup, and [buildWorld] for why it does not matter in a world
// with no members at all.
func (nobodySees) Sight(members []tkencounter.MemberID) (map[tkencounter.MemberID]int, error) {
	out := make(map[tkencounter.MemberID]int, len(members))
	for _, id := range members {
		out[id] = 0
	}

	return out, nil
}

// alwaysPasses is hand-written because the toolkit does not export one:
// sdk.Pass{} (what sdk.Config.TurnDriver is set to for the real, played
// session in orchestrator.go, where sdk aliases rpg-toolkit's session
// package) does NOT satisfy tkencounter.TurnDriver -- different signature
// (Act(string) vs Act(tkencounter.MemberID)) and a different TurnOutcome
// type -- the bridge between them (turnDriverSeam) is unexported to
// rpg-toolkit's session package. tkencounter.Pass, confusingly similarly
// named, is the TurnOutcome VALUE a driver returns, not a driver itself.
// Filed rpg-toolkit#1167 to ask the toolkit to export one at the encounter
// level; this type goes away when it does.
type alwaysPasses struct{}

// Act is never reached for either encounter this package builds, for the
// same reason RollInitiative/Standing/Sight above never are: the probe is
// blind and thrown away, and the real world starts with nobody in it, so no
// fight can ever land a clock on a member here. Returning encounter.Pass is
// the only outcome a v1 TurnDriver may return regardless (turndriver.go).
func (alwaysPasses) Act(_ tkencounter.MemberID) (tkencounter.TurnOutcome, error) {
	return tkencounter.Pass{}, nil
}
