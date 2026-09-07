package sessionworld

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	tkdungeonspec "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter/dungeonspec"
)

// raiderCampPath is the shipped dungeon that declares sides: the hold-out's
// fixture (rpg-project#375), one faction with a mind, hostile to the party
// until the mind learns a fact a placed record reveals.
var raiderCampPath = filepath.Join("..", "..", "content", "reference-raider-camp.yaml")

// compileRaiderCamp compiles the shipped camp through this package and,
// beside it, through dungeonspec alone, so a test can compare what the
// world got against what the compiler said rather than against a number
// somebody typed.
func compileRaiderCamp(t *testing.T) (*Dungeon, tkdungeonspec.Compiled) {
	t.Helper()

	raw, err := os.ReadFile(raiderCampPath)
	require.NoError(t, err, "the shipped raider camp must exist")
	dungeon, err := Compile(raw)
	require.NoError(t, err, "the shipped raider camp must compile")
	spec, err := tkdungeonspec.Load(raw)
	require.NoError(t, err)

	return dungeon, spec
}

// TestFactionsRideTheFieldRatherThanBeingForwarded is the intel table's
// test (TestTheIntelTableIsConstructionTruth) asked again of the three
// things the hold-out adds to a file, because the answer decides how much
// of this wave costs this package anything at all.
//
// FACTIONS, DISPOSITIONS AND WHAT A RECORD REVEALS ARE STRUCTURE. They are
// facts about the dungeon, not about any member -- who the sides are, how
// they stand, what turns them -- so dungeonspec puts them on Compiled.Field
// and this package hands the whole field to NewEncounter. Nothing here
// assembles a FactionInput or a DispositionInput; the production code does
// not mention either type. The composition builds its graph from them at
// New and at Load, folds the stance on every read, and never stores one.
//
// What this guards is the same thing the intel test guards: a refactor that
// copied Field piecemeal would drop the sides silently, and the failure
// would not look like missing factions. Every monster would simply fall
// into the reserved `monsters` faction, the camp would be hostile forever,
// and the hold-out could never end -- in a run that otherwise worked.
func TestFactionsRideTheFieldRatherThanBeingForwarded(t *testing.T) {
	dungeon, spec := compileRaiderCamp(t)
	field := dungeon.World.Field

	// The factions, entry for entry, with their minds.
	require.Len(t, field.Factions, len(spec.Field.Factions), "every faction the compiler produced reached the world")
	require.NotEmpty(t, field.Factions, "and the camp declares one")
	minds := map[tkencounter.FactionID]tkencounter.MemberID{}
	for _, fa := range field.Factions {
		minds[fa.ID] = fa.Mind
	}
	for _, want := range spec.Field.Factions {
		mind, declared := minds[want.ID]
		require.True(t, declared, "faction %q reached the world", want.ID)
		require.Equal(t, want.Mind, mind, "with its mind")
	}
	require.Equal(t, tkencounter.MemberID("chief"), minds["raiders"],
		"the camp knows what its chief knows -- the mind is the placement's authored id")

	// The dispositions, pair for pair, with the predicate that turns them.
	require.Len(t, field.Dispositions, len(spec.Field.Dispositions), "every disposition the compiler produced reached the world")
	require.Len(t, field.Dispositions, 1, "the camp declares one: raiders against the party")
	got := field.Dispositions[0]
	want := spec.Field.Dispositions[0]
	require.ElementsMatch(t, want.Between[:], got.Between)
	require.ElementsMatch(t, []tkencounter.FactionID{"raiders", tkencounter.FactionParty}, got.Between)
	require.Equal(t, string(tkencounter.StanceHostile), got.Stance)
	require.NotNil(t, got.Until, "hostile UNTIL something, and the something arrived")
	until, isFact := want.Until.(tkencounter.TriggerFact)
	require.True(t, isFact, "this slice turns a pair on a fact and nothing else (design §2)")
	require.Equal(t, "fact", got.Until.Kind)
	require.Equal(t, until.Fact, got.Until.Fact, "the fact the compiler named is the fact the world waits for")

	// And the record that can teach it: the disposition's fact IS a fact
	// some placed record reveals, spelled identically at both ends, which is
	// the whole reason the hold-out can be won. A scenario refuses a file
	// where these two disagree; this pins that the world received both
	// halves of the agreement, not one.
	revealed := map[tkencounter.FactID]bool{}
	for _, rec := range field.Intel {
		if rec.Fact != "" {
			revealed[rec.Fact] = true
		}
	}
	require.True(t, revealed[got.Until.Fact],
		"the world carries a record revealing %q, the fact its one disposition turns on", got.Until.Fact)
}

// TestAMonstersFactionIsCarriedToTheSeam is the other half, and the half
// that DOES cost a line: a monster's membership is a fact about the member,
// and a member enters the run through session.Spawn rather than the field,
// so it is hand-carried across that seam like Holds (design §3, "Spawn").
// This pins the compile-side line; the launch-side line lands with the
// session pin that gives SpawnInput a field for it.
func TestAMonstersFactionIsCarriedToTheSeam(t *testing.T) {
	dungeon, spec := compileRaiderCamp(t)

	byPlacement := map[string]Monster{}
	for _, m := range dungeon.Monsters {
		byPlacement[m.PlacementID] = m
	}
	require.Len(t, byPlacement, len(spec.Monsters), "every monster the compiler produced, each under its own name")

	chief, placed := byPlacement["chief"]
	require.True(t, placed, "the author placed a chief")
	require.Equal(t, "raiders", chief.Faction, "in the raiders")
	scout, placed := byPlacement["scout"]
	require.True(t, placed, "and a scout")
	require.Equal(t, "raiders", scout.Faction, "in the same faction")

	for _, want := range spec.Monsters {
		require.Equal(t, want.Faction, byPlacement[want.ID].Faction,
			"placement %q carries the faction the compiler gave it, verbatim", want.ID)
	}
}

// TestADungeonAuthoredBeforeFactionsSpawnsAsItDid is the default that keeps
// every existing dungeon unchanged (design §2): a monster with no authored
// faction carries the EMPTY string to the seam, which the composition reads
// as the reserved `monsters` faction -- and the tomb's garrison, authored
// before the word existed, is exactly that.
func TestADungeonAuthoredBeforeFactionsSpawnsAsItDid(t *testing.T) {
	raw, err := os.ReadFile(referenceTombPath)
	require.NoError(t, err)
	tomb, err := Compile(raw)
	require.NoError(t, err)

	require.Empty(t, tomb.World.Field.Factions, "the tomb declares no faction")
	require.Empty(t, tomb.World.Field.Dispositions, "and no disposition: the defaults are the whole story")
	require.NotEmpty(t, tomb.Monsters)
	for _, m := range tomb.Monsters {
		require.Empty(t, m.Faction, "%s names no faction, so the composition puts it where it always was", m.MemberID)
	}
}
