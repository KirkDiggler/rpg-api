package dungeonstest

const WorldAssetSceneryKey = "world-asset-scenery"

var WorldAssetSceneryRefs = []string{
	"dnd5e:props:dark-fortress:altar_01",
	"dnd5e:items:dark-fortress:health_potion_01",
	"dnd5e:weapons:dark-fortress:sword_01",
	"dnd5e:env:dark-fortress:gate_01",
}

const WorldAssetSceneryYAML = `version: 2
key: world-asset-scenery
name: World Asset Scenery
orientation: pointy
void: opaque
regions:
  - id: room
    archetype: crypt
    lighting: { intensity: 1 }
    cells:
      - [[0,0],[1,0],[2,0]]
      - [[0,1],[1,1],[2,1]]
start: [0,0]
place:
  - { ref: "dnd5e:props:dark-fortress:altar_01", at: [1,0], blocks_movement: true, blocks_los: false }
  - { ref: "dnd5e:items:dark-fortress:health_potion_01", at: [2,0], blocks_movement: false, blocks_los: false }
  - { ref: "dnd5e:weapons:dark-fortress:sword_01", at: [1,1], blocks_movement: false, blocks_los: false, facing: se }
  - { ref: "dnd5e:env:dark-fortress:gate_01", at: [2,1], blocks_movement: true, blocks_los: true, offset: [0.2,-0.1] }
`
