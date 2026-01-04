package toolkit

import "github.com/KirkDiggler/rpg-api/internal/components/dungeon"

// offsetToCube converts offset grid coordinates (col, row) to cube coordinates.
// In cube coordinates: x + y + z = 0 always
// We use: x = col, z = row, y = -x - z
func offsetToCube(col, row int) dungeon.Position {
	return dungeon.Position{
		X: col,
		Y: -col - row,
		Z: row,
	}
}
