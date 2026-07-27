// Package encounter is the v2 encounter handler.
//
// knowledge.go is a SPECIFICATION, not an implementation. It states what
// rpg-api needs from the toolkit in order to project viewer-scoped fog of war,
// and nothing else in this file computes visibility, line of sight, or memory.
//
// The order of work here mirrors how the proto contract was settled: the
// playable concept (rpg-dnd5e-web#611) proved the event shape, and
// rpg-api-protos#197 transcribed it. Now the handler names the interface it
// needs, and the toolkit implements THAT — rather than the handler guessing at
// whatever the toolkit happens to expose today.
//
// Contract: rpg-project/ideas/fog-of-war/design.md §"The event layer".
// The hex is the unit of truth, a VISIBLE observation is total, and nothing is
// ever deleted.
package encounter

import (
	"sort"

	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
)

// KnowledgeState is what a viewer knows about one hex right now.
//
// There is no Unseen value, deliberately. A hex the viewer has never observed
// is ABSENT from the knowledge map — omission is the third state. A value would
// be a second way to say the same thing, and the two could disagree.
//
// There is likewise no Gone/Removed value. See HexObservation.
type KnowledgeState int

const (
	// KnowledgeStateUnspecified means the producer failed to set a state. It is
	// a defect to surface, never a synonym for unseen.
	KnowledgeStateUnspecified KnowledgeState = iota
	// KnowledgeStateVisible is current authorized truth: the viewer observes
	// this hex now.
	KnowledgeStateVisible
	// KnowledgeStateRemembered is a frozen last observation. What the viewer
	// saw, not what is there now.
	KnowledgeStateRemembered
)

// TerrainKind mirrors the wire's TerrainType. The toolkit owns which hex has
// which terrain; rpg-api only maps this onto the proto enum.
//
// Nothing in the toolkit sources terrain today — the pre-fog projection never
// set the wire's terrain field at all. It is named here because the contract
// has the field and a remembered hex must carry the terrain AS OBSERVED rather
// than as currently true. Until the toolkit supplies it, this is Unspecified
// and the client falls back to floor.
type TerrainKind int

const (
	TerrainKindUnspecified TerrainKind = iota
	TerrainKindFloor
	TerrainKindRough
	TerrainKindDifficult
	TerrainKindVoid
	TerrainKindWater
)

// Placement is an occupant of a hex, as observed.
//
// Facing rides the placement rather than the entity because an observation
// belongs to one viewer: someone who saw a skeleton facing north and lost sight
// must keep facing north after the skeleton turns and another viewer sees it
// face south. On the entity, one viewer's sighting would rewrite every other
// viewer's memory.
type Placement struct {
	EntityID core.EntityID
	// Facing is a hex-direction index 0-5.
	Facing int
}

// Edge is a wall or door on one hex's boundary, AS OBSERVED by this viewer.
//
// Edges hang off the hex rather than living in one global wall list because a
// remembered hex has to be drawable from its own record. Today rpg-api projects
// walls from world truth unconditionally — project.go's wallsToProto and
// doorWallsToProto both say so in their own comments ("every viewer's snapshot
// carries every door regardless of RevealedHexes"). That is the leak this
// interface exists to close.
//
// The fields are toolkit primitives rather than a wire enum: the toolkit owns
// what a barrier IS, and rpg-api maps that onto WallKind at the proto boundary.
type Edge struct {
	From core.Hex
	To   core.Hex

	BlocksMovement bool
	BlocksLoS      bool

	// DoorID is the door's entity id, empty when this edge is not a door. Set
	// means the client can address it (Wall.id -> InteractRequest).
	DoorID string
	// DoorOpen and DoorLocked are the door's state AS OBSERVED. A viewer who
	// watched a door close and then walked away must keep "closed" even after
	// someone else opens it, so these travel with the observation rather than
	// being read from the live door.
	DoorOpen   bool
	DoorLocked bool
}

// HexObservation is one viewer's complete authorized truth for one hex, at the
// moment it was observed.
//
// A Visible observation is TOTAL: an empty Contents is a positive claim that
// the hex is empty, never "contents not computed". That totality is what lets a
// remembered occupant vanish on re-sight with no forget message — the arriving
// observation simply does not list it. A producer that leaves Contents empty
// because it did not bother to look is stating something false, and the viewer
// will believe it.
//
// A Remembered observation carries its frozen state IN FULL — terrain, edges
// and contents as last seen — rather than expecting the consumer to freeze what
// it already holds. That is what makes reconnect hydration and a live diff the
// same operation instead of two paths that can drift apart.
//
// NOTHING IS EVER DELETED. There is no removal observation and no tombstone. A
// witnessed removal is a Visible observation that no longer lists the thing; a
// hidden removal is no observation at all, leaving memory stale on purpose.
// Deletion is the one operation a later observation cannot correct.
type HexObservation struct {
	Position core.Hex
	State    KnowledgeState
	Terrain  TerrainKind
	ZoneID   string
	Edges    []Edge
	Contents []Placement
}

// ViewerKnowledge is the seam the toolkit must fill.
//
// It is deliberately tiny. Everything fog of war needs — first sight, losing
// sight, re-sight, hidden mutation, reconnect — is expressible as "what does
// this viewer know", asked before and after a mutation. rpg-api diffs the two
// and writes the wire event; it never computes line of sight and never reads
// world truth to fill a gap.
//
// The single method is why the toolkit's current shape is not enough. Its
// per-viewer memory is perception.View.RevealedHexes, a core.HexSet — which is
// map[Hex]struct{}, a set of COORDINATES with no payload. It records that a hex
// was seen but not what was there, so a Remembered observation cannot be
// produced from it at all. The change the toolkit needs is that set becoming a
// map from hex to observation.
type ViewerKnowledge interface {
	// KnownHexes returns every hex this viewer has ever observed, keyed by
	// position, each carrying the last AUTHORIZED observation and whether it is
	// currently visible or remembered.
	//
	// Hexes the viewer has never observed must be absent rather than present
	// with a zero value. The returned map is the caller's to read only.
	KnownHexes(viewer core.PlayerID) map[core.Hex]HexObservation
}

// KnowledgeDiff is the change to one viewer's knowledge, and it is exactly what
// one HexKnowledgeChanged carries.
//
// An empty diff is meaningful and must stay empty: a mutation the viewer cannot
// see produces no observations, and applying nothing must change nothing. The
// wire event for it is empty or absent, never a special "nothing happened"
// message.
type KnowledgeDiff struct {
	// Hexes are observations to replace wholesale by position — never a
	// field-level merge, which would resurrect contents the new observation
	// deliberately omits.
	Hexes []HexObservation
	// Entities are the ids this diff's placements resolve against. The caller
	// discloses each entity's details separately; an entity is never un-sent,
	// because the vocabulary must outlive visibility for a remembered placement
	// to still render as something.
	Entities []core.EntityID
}

// DiffKnowledge computes what changed for one viewer between two snapshots of
// their knowledge, in the form the wire event wants.
//
// This lives in rpg-api rather than the toolkit because it is pure translation:
// the toolkit decides WHAT a viewer knows, and this decides which of those
// facts are news. It is the whole reason ViewerKnowledge needs only one method.
//
// Hexes are emitted when they are new to the viewer or when any part of the
// observation changed — including a state flip from visible to remembered,
// which is how losing sight reaches the client. Unchanged hexes are omitted:
// re-sending them would be correct but would make every mutation a full
// snapshot.
func DiffKnowledge(before, after map[core.Hex]HexObservation) KnowledgeDiff {
	var diff KnowledgeDiff
	seen := make(map[core.EntityID]struct{})

	for pos, now := range after {
		was, existed := before[pos]
		if existed && observationsEqual(was, now) {
			continue
		}
		diff.Hexes = append(diff.Hexes, now)
		for _, p := range now.Contents {
			if _, dup := seen[p.EntityID]; dup {
				continue
			}
			seen[p.EntityID] = struct{}{}
			diff.Entities = append(diff.Entities, p.EntityID)
		}
	}

	// A hex present in before and absent from after is NOT a removal — the
	// toolkit never forgets, so this cannot happen against a conforming
	// implementation. It is deliberately not handled as a delete: there is no
	// wire form for one, and inventing a local forget would be the exact
	// operation the contract removes.

	sortObservations(diff.Hexes)
	sortEntityIDs(diff.Entities)
	return diff
}

// sortObservations and sortEntityIDs keep wire output deterministic. Go
// randomizes map iteration, so without these the same knowledge change would
// serialize differently run to run — flaky golden tests, and a diff that looks
// like a change when nothing moved.
func sortObservations(obs []HexObservation) {
	sort.Slice(obs, func(i, j int) bool {
		a, b := obs[i].Position, obs[j].Position
		if a.Q != b.Q {
			return a.Q < b.Q
		}
		if a.R != b.R {
			return a.R < b.R
		}
		return a.S < b.S
	})
}

func sortEntityIDs(ids []core.EntityID) {
	sort.Slice(ids, func(i, j int) bool { return string(ids[i]) < string(ids[j]) })
}

func observationsEqual(a, b HexObservation) bool {
	if a.State != b.State || a.Terrain != b.Terrain || a.ZoneID != b.ZoneID {
		return false
	}
	if len(a.Edges) != len(b.Edges) || len(a.Contents) != len(b.Contents) {
		return false
	}
	for i := range a.Edges {
		if a.Edges[i] != b.Edges[i] {
			return false
		}
	}
	for i := range a.Contents {
		if a.Contents[i] != b.Contents[i] {
			return false
		}
	}
	return true
}
