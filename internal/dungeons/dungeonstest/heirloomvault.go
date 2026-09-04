package dungeonstest

// HeirloomVaultKey is the key HeirloomVaultYAML names itself under.
const HeirloomVaultKey = "heirloom-vault"

// HeirloomVaultYAML is [ConcealedVaultYAML] with everything
// recover-the-artifact adds and nothing else (rpg-project#368, design §3.1):
// a holdable heirloom standing in the vault, an ordinary pillar in the hall
// that nobody declared holdable, two authored ways out, and the scenario
// binding that turns the two into an ending.
//
// It is the compact sibling of content/reference-tomb-heirloom.yaml, and it
// exists for the same reason ConcealedVaultYAML does: an acceptance scene
// that walks a party from a start cell to a vault should spend its steps on
// the thing under test, not on crossing four rooms. The shipped fixture is
// the one the walk plays; internal/dungeons proves that one compiles and
// declares its ending, and these scenes prove what the wire does with it.
//
// ONE MONSTER, WHO KNOWS THE WAY IN. The captain carries `knows:
// [vault-door]` and NO boss flag — this dungeon ends because a scenario says
// so, not because a monster has a flag on it (design R8) — which is what
// makes path 2 a path: kill them, loot them, and the vault door is yours.
// The link is authored with the AUTHOR'S door id; dungeonspec mints the
// compiled `<key>/<id>` form on the way through, and that is what a spawn
// must forward.
//
// TWO HOLDABLE THINGS, ONE BOUND. The heirloom is what the scenario counts;
// the chalice is an ordinary holdable standing in the open hall. Holding
// something everybody can already see is what makes "the prop leaves the map
// for EVERYONE" observable without a reveal in the way of it.
//
// TWO EXITS, ONE BOUND. `front-gate` is what the scenario counts as escaping
// with the heirloom; `side-door` is an ordinary way out. That pair is what
// makes design R9 testable at all: leaving through the wrong door has to
// drop what you carry, and with only one exit authored "anywhere but the
// exit" cannot be told from "anywhere but a cell".
const HeirloomVaultYAML = `# The concealed vault, with something in it worth carrying out.
version: 2
key: heirloom-vault
name: The Heirloom Vault
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

walls:
  - start: { cell: [4,2], offset: [-0.25, 0.375] }
    end:   { cell: [4,0], offset: [-0.25, -0.375] }
    name: the vault seam

doors:
  - id: vault-door
    at: { cell: [4,0], offset: [-0.25, 0.375] }
    closed: true
    concealed: [{ ability: perception, dc: 15 }]

place:
  - { id: heirloom, ref: "dnd5e:props:reliquary", at: [5,1],
      blocks_movement: false, blocks_los: false, holdable: true }
  - { id: chalice, ref: "dnd5e:props:chalice", at: [1,1],
      blocks_movement: false, blocks_los: false, holdable: true }
  - { id: pillar, ref: "dnd5e:props:pillar", at: [2,0],
      blocks_movement: true, blocks_los: true }
  - { id: captain, ref: "dnd5e:monsters:skeleton-captain", at: [1,0],
      targeting: closest, knows: [vault-door] }

exits:
  - { id: front-gate, at: [0, 1] }
  - { id: side-door, at: [0, 2] }

scenarios:
  recover-the-artifact:
    artifact: heirloom
    exit: front-gate
`

// The ids HeirloomVaultYAML authors, so a scene names them rather than
// spelling them.
const (
	// HeirloomPropID is the holdable thing the scenario binds.
	HeirloomPropID = "heirloom"

	// ChalicePropID is a second holdable thing, standing in the HALL where
	// everybody can see it — so a scene can prove that holding removes a
	// prop for every member without first having to reveal a room.
	ChalicePropID = "chalice"

	// PillarPropID is the thing nobody declared holdable — what every prop
	// was before this slice, and the one target ErrNotHoldable is reachable
	// through.
	PillarPropID = "pillar"

	// HeirloomVaultDoorID is the concealed door, as the composition mints it:
	// `<key>/<id>`, so two dungeons in one process cannot collide.
	HeirloomVaultDoorID = HeirloomVaultKey + "/vault-door"

	// HeirloomCaptainPlacementID is the author's name for the monster who
	// knows the way into the vault. Its MEMBER id inside a run is derived
	// from the ref, not from this (sessionworld.Monster.MemberID), which is
	// why the two are named apart.
	HeirloomCaptainPlacementID = "captain"

	// HeirloomCaptainMemberID is that monster's id inside a run: the ref's
	// own id plus a per-ref ordinal, exactly as the launch derives it.
	HeirloomCaptainMemberID = "skeleton-captain-1"

	// HeirloomBoundExitID is the way out the scenario counts as escaping.
	HeirloomBoundExitID = "front-gate"

	// HeirloomOtherExitID is an authored way out the scenario does NOT bind.
	HeirloomOtherExitID = "side-door"
)

// HeirloomVaultCells and HeirloomHallCells are each region's cells in
// authored [col,row] offset pairs.
var (
	HeirloomHallCells  = ConcealedVaultHallCells
	HeirloomVaultCells = ConcealedVaultVaultCells
)

// HeirloomVaultDoorCrossing is the one way in, in authored [col,row] pairs:
// the hall cell and the vault cell the door's position is the midpoint of.
var HeirloomVaultDoorCrossing = ConcealedVaultDoorCrossing
