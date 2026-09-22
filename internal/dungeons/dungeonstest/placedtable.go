package dungeonstest

import "testing"

// PlacedTableRoomKey is the v4 single room whose one authored FOOTPRINT can
// be picked up: its own `key:` line.
//
// Two declared props, and a `propBindings` block that makes exactly one of
// them holdable (rpg-toolkit#1854). The bench is the control — same kind of
// thing, same list, no orders — so a scene can say "this one and not that
// one" rather than "the only one there is".
const PlacedTableRoomKey = "placed-table-room"

// PlacedTableRoomYAML is that room, read from the shared fixture directory so
// no Go file carries the authored source as a raw string.
func PlacedTableRoomYAML(t testing.TB) []byte {
	t.Helper()
	return roomFixture(t, PlacedTableRoomKey)
}

// PlacedTableID is the holdable placement the room declares — the id the
// atlas reports and the only handle Hold can name it by.
const PlacedTableID = "table"

// PlacedBenchID is the placement beside it that nobody declared holdable:
// what every placed footprint was before the flag existed.
const PlacedBenchID = "bench"
