package dungeonstest

import "testing"

// CreatureFactsWalkKey is the v4 single room that authors BOTH new deed
// readings, so a walk can watch them arrive: its own `key:` line.
//
// A faction's own members are its side (`on: ally`), a creature can read what
// IT did (`as: actor`, which is the pause), and a NEUTRAL pair carries an
// `until` — legal since dispositions turn both ways (rpg-project#493 R1), and
// currently refused by the World Builder (rpg-dnd5e-web#1197).
//
// The watch is one faction of three so `on: ally` has somebody to be about,
// and the wolves are a second faction allied to nobody, so the file carries
// its own control: their wounds are nobody's side.
const CreatureFactsWalkKey = "creature-facts-walk"

// CreatureFactsWalkYAML is that room, read from the shared fixture directory
// so no Go file carries the authored source as a raw string.
func CreatureFactsWalkYAML(t testing.TB) []byte {
	t.Helper()
	return roomFixture(t, CreatureFactsWalkKey)
}
