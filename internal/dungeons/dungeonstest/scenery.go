package dungeonstest

// SceneryStripKey is the key SceneryStripYAML names itself under.
const SceneryStripKey = "scenery-strip"

// SceneryStripYAML is the smallest dungeon that authors scenery: one region,
// one strip of floor beside it that belongs to no region, and the wall
// between them (rpg-project#360, wall-geometry design §3.1).
//
// Deliberately not the reference tomb. The tomb is the whole content pipeline
// under test and authors no scenery; this file exists to say ONE thing --
// that a `scenery:` list survives the trip through rpg-api unchanged and
// arrives on the wire as floor nobody owns.
//
// The vault is nine cells, the strip is three, and the prop on [3,1] is the
// allowed half of the design's F2: a prop may stand on scenery, while a
// monster and the party start may not.
const SceneryStripYAML = `# A vault with a rubble strip beside it, and the wall between them.
version: 2
key: scenery-strip
name: The Scenery Strip
orientation: pointy
void: opaque

regions:
  - id: vault
    name: Vault
    archetype: crypt
    lighting: { intensity: 0.5 }
    cells:
      - [[0,0],[1,0],[2,0]]
      - [[0,1],[1,1],[2,1]]
      - [[0,2],[1,2],[2,2]]

# The rubble: floor nobody owns and nobody stands on.
scenery:
  - [[3,0]]
  - [[3,1]]
  - [[3,2]]

start: [1, 1]

# The seam between the vault and the rubble: every crossing the two columns
# share.
walls:
  - [[2,0],[3,0]]
  - [[2,1],[3,0]]
  - [[2,1],[3,1]]
  - [[2,1],[3,2]]
  - [[2,2],[3,2]]

# A prop may stand on scenery.
place:
  - { ref: "dnd5e:props:bone-pile", at: [3,1], blocks_movement: false, blocks_los: false }
`

// SceneryStripRegionCells is the vault's nine cells as the file paints them,
// in authored [col,row] offset pairs.
var SceneryStripRegionCells = [][2]int{
	{0, 0}, {1, 0}, {2, 0},
	{0, 1}, {1, 1}, {2, 1},
	{0, 2}, {1, 2}, {2, 2},
}

// SceneryStripSceneryCells is the strip's three cells as the file paints
// them, in authored [col,row] offset pairs.
var SceneryStripSceneryCells = [][2]int{{3, 0}, {3, 1}, {3, 2}}
