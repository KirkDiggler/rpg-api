// Package encounter is the v2 encounter handler.
//
// knowledge.go was originally a SPECIFICATION of what rpg-api needed from
// the toolkit for viewer-scoped fog of war (rpg-toolkit#851's local
// stand-in gap). rpg-toolkit#851 has since landed (encounter/v0.46.0):
// perception.HexObservation, perception.Placement, perception.Edge,
// perception.KnowledgeState, and perception.TerrainKind are now the
// toolkit's OWN canonical types, and *encounter.Encounter implements
// KnownHexes(viewer) map[core.Hex]perception.HexObservation directly. This
// file no longer defines local copies of those types — per the "toolkit
// types are canonical, one conversion point at the proto boundary" house
// rule, project.go and translate.go consume perception.* directly, and the
// proto boundary (observationToProto et al., project.go) is that one
// conversion point.
//
// What remains here is genuinely rpg-api's own: ViewerKnowledge (the
// interface rpg-api code depends on, so it isn't hard-wired to
// *tkenc.Encounter specifically — *tkenc.Encounter satisfies it structurally)
// and DiffKnowledge (pure translation from "what does a viewer know" to
// "what's news for the wire", which is not the toolkit's job — the toolkit
// decides WHAT a viewer knows, not which of those facts have already been
// sent). Both keep their own dedicated tests (knowledge_test.go) written
// against the interface alone, with no toolkit dependency — that coverage
// is exactly why they still earn their place instead of being deleted
// alongside knowledge_adapter.go.
//
// Contract: rpg-project/ideas/fog-of-war/design.md §"The event layer".
// The hex is the unit of truth, a VISIBLE observation is total, and nothing is
// ever deleted.
package encounter

import (
	"sort"

	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/encounter/perception"
)

// ViewerKnowledge is the seam rpg-api code depends on for "what does this
// viewer currently know". *tkenc.Encounter (rpg-toolkit#851) satisfies it
// structurally — no adapter needed — but callers still take this interface
// rather than the concrete toolkit type, so a test can substitute a fake
// (see knowledge_test.go's fakeKnowledge) without standing up a real
// encounter, room, and broker.
type ViewerKnowledge interface {
	// KnownHexes returns every hex this viewer has ever observed, keyed by
	// position, each carrying the last AUTHORIZED observation and whether it is
	// currently visible or remembered.
	//
	// Hexes the viewer has never observed must be absent rather than present
	// with a zero value. The returned map is the caller's to read only.
	KnownHexes(viewer core.PlayerID) map[core.Hex]perception.HexObservation
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
	Hexes []perception.HexObservation
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
func DiffKnowledge(before, after map[core.Hex]perception.HexObservation) KnowledgeDiff {
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
func sortObservations(obs []perception.HexObservation) {
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

func observationsEqual(a, b perception.HexObservation) bool {
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
