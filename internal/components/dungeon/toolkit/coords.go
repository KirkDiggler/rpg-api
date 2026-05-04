package toolkit

import "github.com/KirkDiggler/rpg-api/internal/components/dungeon"

// offsetToCube converts offset grid coordinates (col, row) to a room-local cube
// position. In cube coordinates: x + y + z = 0 always. We use: x = col, z = row,
// y = -x - z.
//
// Returns LocalPosition because everything inside the toolkit package is
// authored in room-local space; transforms to absolute happen at the
// orchestrator boundary via dungeon.Module.LocalToAbsolute.
func offsetToCube(col, row int) dungeon.LocalPosition {
	return dungeon.NewLocalPosition(col, row)
}
