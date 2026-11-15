package dungeons

import "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter/dungeonspec/readback"

// SetReadBack stands fn in for readback.Of on a Registry built by
// NewRegistry, so a test can reach the read-back's failure and panic
// branches, which the real read-back never takes on a file that compiled.
func SetReadBack(r Registry, fn func(raw []byte) (*readback.ReadBack, error)) {
	r.(*storeRegistry).readBack = fn
}
