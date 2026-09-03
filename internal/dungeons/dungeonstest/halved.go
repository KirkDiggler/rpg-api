package dungeonstest

// HalvedRoomKey is the key HalvedRoomYAML names itself under.
const HalvedRoomKey = "halved-room"

// HalvedRoomYAML is the smallest dungeon that SEALS a cell: one four-by-three
// room with a single wall drawn straight through two of its cells' centres
// (rpg-project#360, wall-geometry design F14 and §5.2 as amended).
//
// It exists because sealing is the fact region membership stopped implying.
// A wall used to run between cells; a line can run THROUGH one, and the cell it
// halves keeps its region, its lighting and its archetype while nobody can
// stand on it any more. A host draws it exactly as it draws the floor beside
// it, which is why it stays in the flat cell list and in its region's cells,
// and why refusing a step onto it has to be said somewhere else.
//
// Deliberately the thick flat-side line rather than the tomb's quarter lines.
// The tomb seals nothing -- that is what makes it the right default -- so
// nothing in the shipped content would ever exercise this.
const HalvedRoomYAML = `# A room with a wall drawn through the middle of it.
version: 2
key: halved-room
name: The Halved Room
orientation: pointy
void: opaque

regions:
  - id: vault
    name: Vault
    archetype: crypt
    lighting: { intensity: 0.5 }
    cells:
      - [[0,0],[1,0],[2,0],[3,0]]
      - [[0,1],[1,1],[2,1],[3,1]]
      - [[0,2],[1,2],[2,2],[3,2]]

start: [0, 1]

# Straight down the flat side between columns 0 and 1, which in a pointy-top
# grid runs through the centre of the even-row cells on it and along the side
# of the odd-row ones. [1,0] and [1,2] are halved; [1,1] is left whole.
walls:
  - start: { cell: [1,0], offset: [0, 0] }
    end:   { cell: [1,2], offset: [0, 0] }
    name: the centre line
`

// HalvedRoomSealedCells are the two cells the centre line runs through, in
// authored [col,row] offset pairs. Floor, owned by the vault, and unstandable.
var HalvedRoomSealedCells = [][2]int{{1, 0}, {1, 2}}

// HalvedRoomRegionCells is every cell the vault owns, sealed ones included, in
// authored [col,row] offset pairs.
var HalvedRoomRegionCells = [][2]int{
	{0, 0}, {1, 0}, {2, 0}, {3, 0},
	{0, 1}, {1, 1}, {2, 1}, {3, 1},
	{0, 2}, {1, 2}, {2, 2}, {3, 2},
}
