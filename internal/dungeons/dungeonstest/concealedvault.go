package dungeonstest

// ConcealedVaultKey is the key ConcealedVaultYAML names itself under.
const ConcealedVaultKey = "concealed-vault"

// ConcealedVaultYAML is the smallest dungeon that hides a room behind a wall:
// a hall anybody can see, a vault nobody can until they find the way in, one
// authored line between them, and a concealed door standing on that line.
//
// It exists for the reveal, which is the half of wall geometry a static atlas
// cannot show (rpg-project#360, design A10). A non-knower is served the whole
// wall and no doorway, so their map looks like an honest dead end rather than
// a suspiciously wall-shaped gap; when they open the door, the room and the
// rest of its geometry arrive as a beat, because an open door is a room seen
// into.
//
// A second line stands INSIDE the vault. Its footprint is vault floor only, so
// no footing ever presents it to a non-knower, and it is exactly what the
// reveal's segment list must hand over (design §5.2a): the seam they already
// had is not news, the wall inside the room is.
//
// The seam is a quarter line, so nothing here is sealed by authoring. What a
// non-knower's sealed list holds is the FOOTING the projection gives that wall
// (design §4.6): floor the wall stands on that belongs to a room they cannot
// see reaches them as ownerless, and ownerless floor is floor nobody stands
// on. Those same cells are ordinary vault floor the moment the vault is
// theirs, which is why the reveal's sealed list REPLACES within the room
// rather than adding to what they had.
const ConcealedVaultYAML = `# A hall, a vault nobody can see, and the line between them.
version: 2
key: concealed-vault
name: The Concealed Vault
orientation: pointy
void: opaque

regions:
  - id: hall
    name: Hall
    archetype: crypt
    lighting: { intensity: 1 }
    cells:
      - [[0,0],[1,0],[2,0],[3,0]]
      - [[0,1],[1,1],[2,1],[3,1]]
      - [[0,2],[1,2],[2,2],[3,2]]
  - id: vault
    name: Vault
    archetype: crypt
    lighting: { intensity: 1 }
    concealed: true
    cells:
      - [[4,0],[5,0],[6,0],[7,0]]
      - [[4,1],[5,1],[6,1],[7,1]]
      - [[4,2],[5,2],[6,2],[7,2]]

start: [0, 1]

# One quarter line down the seam, blocking all five crossings the two columns
# share -- four of them walls, the fifth the door below.
walls:
  - start: { cell: [4,2], offset: [-0.25, 0.375] }
    end:   { cell: [4,0], offset: [-0.25, -0.375] }
    name: the vault seam
  # A quarter line inside the vault, two columns in: no hall cell is in its
  # footprint, so a non-knower is never shown it and the reveal must carry it.
  - start: { cell: [6,2], offset: [-0.25, 0.375] }
    end:   { cell: [6,0], offset: [-0.25, -0.375] }
    name: the vault's inner wall

# Shut and hidden. Finding it is a perception check; crossing it is what makes
# the vault yours.
doors:
  - id: vault-door
    at: { cell: [4,0], offset: [-0.25, 0.375] }
    closed: true
    concealed: [{ ability: perception, dc: 15 }]
`

// ConcealedVaultHallCells and ConcealedVaultVaultCells are each region's cells
// in authored [col,row] offset pairs. The vault's are what a reveal hands over
// and what its sealed list is scoped to.
var (
	ConcealedVaultHallCells = [][2]int{
		{0, 0}, {1, 0}, {2, 0}, {3, 0},
		{0, 1}, {1, 1}, {2, 1}, {3, 1},
		{0, 2}, {1, 2}, {2, 2}, {3, 2},
	}
	ConcealedVaultVaultCells = [][2]int{
		{4, 0}, {5, 0}, {6, 0}, {7, 0},
		{4, 1}, {5, 1}, {6, 1}, {7, 1},
		{4, 2}, {5, 2}, {6, 2}, {7, 2},
	}
)

// ConcealedVaultDoorCrossing is the one way in, in authored [col,row] pairs:
// the hall cell and the vault cell the door's position is the midpoint of.
var ConcealedVaultDoorCrossing = [2][2]int{{3, 1}, {4, 0}}
