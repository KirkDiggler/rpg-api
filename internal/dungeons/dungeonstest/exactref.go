package dungeonstest

// ExactRefPropsKey is the key ExactRefPropsYAML names itself under.
const ExactRefPropsKey = "exact-ref-props"

// ExactRefPropsRef is the four-part ref this fixture places: a props ref whose
// ID CARRIES A PART OF ITS OWN. It is spelled out here rather than only inside
// the YAML so a test can compare what arrives on the wire against one literal.
const ExactRefPropsRef = "dnd5e:props:plushie:skeleton-dog"

// ExactRefPropsPlainRef is the ordinary three-part ref standing beside it, so
// a scene can show the two travel the same road and arrive unchanged.
const ExactRefPropsPlainRef = "dnd5e:props:brazier"

// ExactRefPropsYAML is the smallest dungeon that places an EXACT-REF prop
// (rpg-project#367, rpg-toolkit#1536, rpg-api#922): one room, one party start,
// and two props whose refs differ only in how many parts their ids carry.
//
// It exists because the compiler used to refuse the four-part one. A ref is
// module:type:id and the id is everything after the second colon, so
// "dnd5e:props:plushie:skeleton-dog" names a plushie of a skeleton dog and the
// inner structure of that name belongs to content, not to the grammar. The
// walk this fixture stands in for is Kirk placing exactly that prop in the
// builder and pressing Save & Play.
//
// Deliberately not the reference tomb, and deliberately carrying the plain ref
// too. The whole claim is that ref DEPTH stops mattering, and a scene with only
// the deep ref in it could pass while a compiler quietly rewrote every ref it
// touched.
const ExactRefPropsYAML = `# One room, and a plushie of a skeleton dog on the floor of it.
version: 2
key: exact-ref-props
name: The Toy Room
orientation: pointy
void: opaque

regions:
  - id: playroom
    name: Playroom
    archetype: crypt
    lighting: { intensity: 0.5 }
    cells:
      - [[0,0],[1,0],[2,0]]
      - [[0,1],[1,1],[2,1]]
      - [[0,2],[1,2],[2,2]]

start: [0, 1]

place:
  # The four-part ref. Nobody trips over a plushie and nobody hides behind one.
  - { ref: "dnd5e:props:plushie:skeleton-dog", at: [2,0], blocks_movement: false, blocks_los: false }
  # The three-part ref beside it, for contrast.
  - { ref: "dnd5e:props:brazier", at: [2,2], blocks_movement: false, blocks_los: false }
`
