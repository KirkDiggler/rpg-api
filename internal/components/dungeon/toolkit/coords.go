package toolkit

import "github.com/KirkDiggler/rpg-api/internal/components/dungeon"

// offsetToCube converts offset grid coordinates (col, row) to cube coordinates.
// Uses pointy-top hex orientation (odd-q offset coordinates).
// In cube coordinates: x + y + z = 0 always
// Formula matches toolkit's spatial.OffsetCoordinateToCubeWithOrientation for pointy-top.
func offsetToCube(col, row int) dungeon.Position {
	x := col
	z := row - (col-(col&1))/2 // Adjust for odd column stagger
	y := -x - z
	return dungeon.Position{
		X: x,
		Y: y,
		Z: z,
	}
}

// cubeToOffset converts cube coordinates back to offset grid coordinates (col, row).
// Uses pointy-top hex orientation (odd-q offset coordinates).
// Formula matches toolkit's CubeCoordinate.ToOffsetCoordinateWithOrientation for pointy-top.
func cubeToOffset(pos dungeon.Position) (col, row int) {
	col = pos.X
	row = pos.Z + (pos.X-(pos.X&1))/2 // Reverse the stagger adjustment
	return col, row
}
