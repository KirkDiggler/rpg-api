package sessionworld

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	tkdungeonspec "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter/dungeonspec"
)

// heirloomPath is the shipped dungeon that declares intel: the reference tomb
// plus the vault, the heirloom, and a record the captain holds.
var heirloomPath = filepath.Join("..", "..", "content", "reference-tomb-heirloom.yaml")

// TestTheIntelTableIsConstructionTruth is the fact buildWorld's doc leans on
// and nothing else would notice going wrong (rpg-project#372).
//
// A dungeon's authored records are STRUCTURE, like its doors and its exits:
// they ride Compiled.Field, and this package hands the whole field to
// NewEncounter, so the composition ends up owning the table it will consult
// when a record changes hands. There is no line of code here that does it —
// which is exactly why it needs a test. A field assembled by hand, or a
// future refactor that copied Field field-by-field the way the session's
// atlas mirror has to, would drop the table silently, and the failure would
// not look like a missing table: every loot of an intel-holding body would
// simply reveal nothing, in a run that otherwise worked.
func TestTheIntelTableIsConstructionTruth(t *testing.T) {
	raw, err := os.ReadFile(heirloomPath)
	require.NoError(t, err, "the shipped heirloom fixture must exist")

	dungeon, err := Compile(raw)
	require.NoError(t, err, "the shipped heirloom fixture must compile")

	table := dungeon.World.Field.Intel
	require.Len(t, table, 1, "the world carries the record the file declares")

	// COMPILED IDS, both of them. The record's own id and the door it
	// reveals are each minted `<key>/<id>`, because the composition's tables
	// are keyed that way and two dungeons in one process must not collide.
	// A raw authored id here would look right in a diff and reveal nothing
	// at runtime.
	require.Equal(t, tkencounter.IntelID("reference-tomb-heirloom/vault-map"), table[0].ID)
	require.Equal(t, tkencounter.DoorID("reference-tomb-heirloom/vault"), table[0].Door)
}

// TestTheCaptainCarriesTheRecordByItsCompiledID pins the other half of the
// pair: what the monster carries is the RECORD's compiled id, which is what
// the launch forwards into Spawn.
//
// The two ids are deliberately asserted apart from each other. A holder that
// carried the door id instead would still be a plausible-looking string, and
// on this fixture it would even name something real — the composition would
// simply refuse it as a record it does not declare, at spawn, in a message
// about intel rather than about doors.
func TestTheCaptainCarriesTheRecordByItsCompiledID(t *testing.T) {
	raw, err := os.ReadFile(heirloomPath)
	require.NoError(t, err)
	dungeon, err := Compile(raw)
	require.NoError(t, err)

	var captain *Monster
	for i, m := range dungeon.Monsters {
		if m.PlacementID == "captain" {
			captain = &dungeon.Monsters[i]
		}
	}
	require.NotNil(t, captain, "the fixture places a monster the author named captain")
	require.Equal(t, []string{"reference-tomb-heirloom/vault-map"}, captain.Holds,
		"the compiled RECORD id, which is what Spawn takes")

	// And nobody else holds anything: intel is placed, not sprayed.
	for _, m := range dungeon.Monsters {
		if m.PlacementID == "captain" {
			continue
		}
		require.Empty(t, m.Holds, "monster %s was not authored holding anything", m.MemberID)
	}
}

// TestKnowsIsRefusedByName is R1 reaching this package: one spelling of
// knowledge, and the old one is not quietly ignored.
//
// Ignoring it would be the dangerous outcome rather than the harmless one. A
// file that still said `knows:` would compile, spawn a captain holding
// nothing, and play exactly like a dungeon whose author had not authored any
// intel at all — the failure would show up as a scenario that cannot be won
// by its second path, with nothing anywhere saying why.
func TestKnowsIsRefusedByName(t *testing.T) {
	raw, err := os.ReadFile(heirloomPath)
	require.NoError(t, err)

	const holds = "holds: [vault-map]"
	require.Contains(t, string(raw), holds, "the fixture's holds line must be where this test expects it")
	legacy := strings.Replace(string(raw), holds, "knows: [vault-door]", 1)

	_, err = Compile([]byte(legacy))
	require.Error(t, err, "a file still spelling knowledge the old way is refused, never ignored")
	require.Contains(t, err.Error(), "knows", "and the refusal names the word the author used")
}

// TestPropsRideTheFieldRatherThanBeingForwarded is the fact that decides how
// much work each new PLACEMENT field costs this package, and it is worth a
// test because the answer is different for props and for monsters — a
// difference that is easy to get backwards.
//
// A MONSTER needs forwarding. The world this package builds is empty of
// members on purpose (see [Dungeon.World]): a construction-time monster has
// no sheet, so it arrives later through session.Spawn, and every fact about
// it — its cell, its targeting, what it holds — has to be carried across
// that seam by hand. That is why [Monster] exists at all.
//
// A PROP does not. Props are field STRUCTURE, like walls, doors, exits and
// the intel table, so they ride Compiled.Field and this package hands the
// whole field to NewEncounter. Nothing here assembles a PropInput — the
// production code contains no mention of the type — so a field the compiler
// grows is a field the world gets, for free and by construction.
//
// The consequence, stated because it is the thing to check first the next
// time a placement grows a field: whatever dungeonspec adds to a PROP needs
// no forwarding code here, and whatever it adds to a MONSTER needs a line in
// Compile and a line at the launch. What this test guards is that the free
// half stays free — a refactor that copied Field piecemeal, the way the
// session's atlas mirror has to, would silently stop carrying it.
func TestPropsRideTheFieldRatherThanBeingForwarded(t *testing.T) {
	raw, err := os.ReadFile(heirloomPath)
	require.NoError(t, err)
	dungeon, err := Compile(raw)
	require.NoError(t, err)

	byID := map[tkencounter.PropID]tkencounter.PropData{}
	for _, p := range dungeon.World.Field.Props {
		if p.ID != "" {
			byID[p.ID] = p
		}
	}

	// The heirloom is the fixture's one named, holdable prop, and BOTH of
	// those facts reached the world without a line of code in this package
	// putting them there.
	heirloom, placed := byID["heirloom"]
	require.True(t, placed, "the named prop the author placed is in the world")
	require.True(t, heirloom.Holdable, "carrying its holdable flag with it")
	require.Equal(t, "dnd5e:props:reliquary", heirloom.Ref)

	// And the world's prop list is the compiler's, entry for entry — not a
	// subset this package chose to carry.
	require.Len(t, dungeon.World.Field.Props, len(compiledProps(t, raw)),
		"every prop the compiler produced reached the world")
}

// compiledProps is what dungeonspec alone says the file places, so the
// assertion above compares the world against the compiler rather than
// against a number somebody typed.
func compiledProps(t *testing.T, raw []byte) []tkencounter.PropInput {
	t.Helper()

	spec, err := tkdungeonspec.Load(raw)
	require.NoError(t, err)

	return spec.Field.Props
}
