package sessionworld

import "testing"

// BenchmarkReferenceTomb measures what a new-stack StartEncounter now pays to
// compile its dungeon, because the borrowed-projection approach costs an EXTRA
// encounter construction (see this package's doc comment) and a cost nobody
// measured is a cost nobody can argue about.
//
// Measured 2026-08-20 on a Ryzen 9 7945HX: ~318us, ~228KB, ~2233 allocs.
//
// The conclusion, so it is not re-derived: this is fine and needs no cache. It
// runs ONCE per encounter start, not per RPC, and for scale a single free-roam
// click is ~187us of load-act-save on the same machine -- so starting a session
// costs about what two clicks in it do. If the extra construction ever does
// need to go, the reason will be rpg-toolkit#1139 making it unnecessary, not
// this number.
func BenchmarkReferenceTomb(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := ReferenceTomb(); err != nil {
			b.Fatal(err)
		}
	}
}
