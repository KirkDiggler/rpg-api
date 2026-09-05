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
// ONE RECORD, HELD BY ONE MONSTER. The dungeon declares `vault-map`, which
// reveals the vault door, and the captain `holds:` it — knowledge is a thing
// the author places, not a property typed on a monster (rpg-project#372
// R1/R2). The captain carries NO boss flag: this dungeon ends because a
// scenario says so, not because a monster has a flag on it (slice 2 R8).
// That is what makes path 2 a path: kill them, loot them, and the vault door
// is yours.
//
// Both ids are authored in the AUTHOR'S spelling; dungeonspec mints the
// compiled `<key>/<id>` form on the way through, and it is the minted form a
// spawn must forward.
//
// THREE HOLDABLE THINGS, AND THE PAIR THAT MATTERS. The heirloom is what the
// scenario counts. The chalice and the scroll are both ordinary holdables
// standing in the open hall, and they differ in exactly one thing: the scroll
// holds a record and the chalice holds nothing. That pair is the control for
// R6 — hold one and the vault door is yours, hold the other and you have
// learned nothing — and it lives in ONE run rather than across two, so
// nothing but the `holds:` list can account for the difference.
//
// Holding something everybody can already see is also what makes "the prop
// leaves the map for EVERYONE" observable without a reveal in the way of it.
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

# The knowledge this dungeon declares. A record is a thing in the file, like a
# door: an id and what it reveals. Whoever carries it names it under holds.
intel:
  - id: vault-map
    reveals: { door: vault-door }
  # The same way in, written down twice -- once in the captain's head and
  # once on a scroll in the hall. Two records may reveal one door: knowledge
  # is not scarce. The scroll is what makes the tool testable without a
  # fight (R6).
  - id: hall-notes
    reveals: { door: vault-door }

place:
  - { id: heirloom, ref: "dnd5e:props:reliquary", at: [5,1],
      blocks_movement: false, blocks_los: false, holdable: true }
  - { id: chalice, ref: "dnd5e:props:chalice", at: [1,1],
      blocks_movement: false, blocks_los: false, holdable: true }
  - { id: pillar, ref: "dnd5e:props:pillar", at: [2,0],
      blocks_movement: true, blocks_los: true }
  - { id: hall-scroll, ref: "dnd5e:props:scroll", at: [3,2],
      blocks_movement: false, blocks_los: false, holdable: true, holds: [hall-notes] }
  - { id: captain, ref: "dnd5e:monsters:skeleton-captain", at: [1,0],
      targeting: closest, holds: [vault-map] }

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

	// ScrollPropID is a holdable prop that HOLDS A RECORD: picking it up
	// teaches the holder what the record reveals (R6). Its counterpart is
	// ChalicePropID above, holdable and holding nothing.
	ScrollPropID = "hall-scroll"

	// PillarPropID is the thing nobody declared holdable — what every prop
	// was before this slice, and the one target ErrNotHoldable is reachable
	// through.
	PillarPropID = "pillar"

	// HeirloomVaultDoorID is the concealed door, as the composition mints it:
	// `<key>/<id>`, so two dungeons in one process cannot collide.
	HeirloomVaultDoorID = HeirloomVaultKey + "/vault-door"

	// HeirloomIntelAuthoredID is the intel record as the AUTHOR spells it in
	// the file, and HeirloomIntelRecordID is the same record as the compiler
	// mints it. Both are named because the difference between them is
	// load-bearing: a host that forwards the first gets ErrNoIntel.
	HeirloomIntelAuthoredID = "vault-map"
	HeirloomIntelRecordID   = HeirloomVaultKey + "/" + HeirloomIntelAuthoredID

	// ScrollIntelAuthoredID and ScrollIntelRecordID are the second record —
	// the one on the scroll. It reveals the SAME door the captain's record
	// does, which is the point: knowledge is not scarce, and two ways to
	// learn one thing is ordinary authoring.
	ScrollIntelAuthoredID = "hall-notes"
	ScrollIntelRecordID   = HeirloomVaultKey + "/" + ScrollIntelAuthoredID

	// HeirloomCaptainPlacementID is the author's name for the monster who
	// knows the way into the vault.
	HeirloomCaptainPlacementID = "captain"

	// HeirloomCaptainMemberID is that monster's id inside a run, and it IS
	// the author's name (rpg-project#375, ruled on the hold-out): a named
	// placement joins under its own id, so what the file says about it —
	// a faction's mind, a `{ down }` predicate — means the same member in
	// the run. It was "skeleton-captain-1", the ref plus an ordinal, until
	// the hold-out needed the two to agree; the ordinal form remains for
	// placements the author left unnamed (sessionworld.Monster.MemberID).
	// Named apart from the placement id all the same, because a scene that
	// spawns, loots or downs the captain is naming the MEMBER.
	HeirloomCaptainMemberID = HeirloomCaptainPlacementID

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
