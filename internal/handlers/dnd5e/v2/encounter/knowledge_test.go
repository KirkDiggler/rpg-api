package encounter_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/encounter/perception"

	handler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v2/encounter"
)

// fakeKnowledge is a ViewerKnowledge that returns whatever it was handed. It
// exists to prove the interface is sufficient: every case below is driven
// entirely through it, with no toolkit, no world truth, and no line of sight.
// If a case here needed something this fake cannot express, the interface would
// be wrong.
type fakeKnowledge struct {
	perViewer map[core.PlayerID]map[core.Hex]perception.HexObservation
}

func (f *fakeKnowledge) KnownHexes(viewer core.PlayerID) map[core.Hex]perception.HexObservation {
	return f.perViewer[viewer]
}

var _ handler.ViewerKnowledge = (*fakeKnowledge)(nil)

type KnowledgeSuite struct {
	suite.Suite
}

func TestKnowledgeSuite(t *testing.T) {
	suite.Run(t, new(KnowledgeSuite))
}

func hex(q, r int) core.Hex {
	return core.Hex{Q: q, R: r, S: -q - r}
}

// visible builds a VISIBLE observation. Contents defaults to empty, which is a
// positive claim that the hex is empty rather than an omission.
func visible(q, r int, contents ...perception.Placement) perception.HexObservation {
	return perception.HexObservation{
		Position: hex(q, r),
		State:    perception.KnowledgeStateVisible,
		Terrain:  perception.TerrainKindFloor,
		Contents: contents,
	}
}

func remembered(o perception.HexObservation) perception.HexObservation {
	o.State = perception.KnowledgeStateRemembered
	return o
}

func knowledge(obs ...perception.HexObservation) map[core.Hex]perception.HexObservation {
	out := make(map[core.Hex]perception.HexObservation, len(obs))
	for _, o := range obs {
		out[o.Position] = o
	}
	return out
}

var skeletonFacingNorth = perception.Placement{EntityID: "skeleton-1", Facing: 0}

func (s *KnowledgeSuite) TestFirstSightEmitsTheHex() {
	diff := handler.DiffKnowledge(nil, knowledge(visible(0, 0)))

	s.Require().Len(diff.Hexes, 1)
	s.Equal(perception.KnowledgeStateVisible, diff.Hexes[0].State)
}

func (s *KnowledgeSuite) TestLosingSightEmitsTheStateFlip() {
	// The viewer still knows the hex; what changed is that it is now a memory.
	// If this did not emit, losing sight would be invisible to the client and
	// the fog would never close.
	before := knowledge(visible(0, 0, skeletonFacingNorth))
	after := knowledge(remembered(visible(0, 0, skeletonFacingNorth)))

	diff := handler.DiffKnowledge(before, after)

	s.Require().Len(diff.Hexes, 1)
	s.Equal(perception.KnowledgeStateRemembered, diff.Hexes[0].State)
	s.Equal([]perception.Placement{skeletonFacingNorth}, diff.Hexes[0].Contents)
}

func (s *KnowledgeSuite) TestHiddenMutationChangesNothing() {
	// The world changed where this viewer cannot see, so their knowledge is
	// identical. An empty diff is the correct wire event.
	before := knowledge(remembered(visible(1, 0)))
	after := knowledge(remembered(visible(1, 0)))

	diff := handler.DiffKnowledge(before, after)

	s.Empty(diff.Hexes)
	s.Empty(diff.Entities)
}

func (s *KnowledgeSuite) TestReSightDeletesARememberedOccupant() {
	// The load-bearing case. Nothing forgets the skeleton — it is gone because
	// the arriving observation states the hex is empty. There is no forget
	// message anywhere in this contract.
	before := knowledge(remembered(visible(0, 0, skeletonFacingNorth)))
	after := knowledge(visible(0, 0))

	diff := handler.DiffKnowledge(before, after)

	s.Require().Len(diff.Hexes, 1)
	s.Equal(perception.KnowledgeStateVisible, diff.Hexes[0].State)
	s.Empty(diff.Hexes[0].Contents)
}

func (s *KnowledgeSuite) TestUnchangedKnowledgeIsIdempotent() {
	k := knowledge(visible(0, 0, skeletonFacingNorth), visible(1, 0))

	s.Empty(handler.DiffKnowledge(k, k).Hexes)
}

func (s *KnowledgeSuite) TestFacingChangeAloneIsAChange() {
	// Facing rides the placement, so a creature turning in place is news to the
	// viewer who can see it. If this collapsed to "no change", a viewer would
	// keep rendering a stale facing while watching the creature.
	before := knowledge(visible(0, 0, perception.Placement{EntityID: "skeleton-1", Facing: 0}))
	after := knowledge(visible(0, 0, perception.Placement{EntityID: "skeleton-1", Facing: 3}))

	diff := handler.DiffKnowledge(before, after)

	s.Require().Len(diff.Hexes, 1)
	s.Equal(3, diff.Hexes[0].Contents[0].Facing)
}

func (s *KnowledgeSuite) TestEdgeChangeIsAChange() {
	// A door opening is a change to the hex's edges, which is how the client
	// learns about it — DoorOpened itself is now a pure notification.
	closed := visible(0, 0)
	closed.Edges = []perception.Edge{{From: hex(0, 0), To: hex(1, 0), DoorID: "door-1", BlocksLoS: true}}
	opened := visible(0, 0)
	opened.Edges = []perception.Edge{{From: hex(0, 0), To: hex(1, 0), DoorID: "door-1", DoorOpen: true}}

	diff := handler.DiffKnowledge(knowledge(closed), knowledge(opened))

	s.Require().Len(diff.Hexes, 1)
	s.True(diff.Hexes[0].Edges[0].DoorOpen)
}

func (s *KnowledgeSuite) TestEntitiesAreCollectedDedupedAndSorted() {
	// Placements resolve against this list, so every referenced entity must
	// appear exactly once regardless of how many hexes mention it.
	a := visible(0, 0, perception.Placement{EntityID: "skeleton-1"}, perception.Placement{EntityID: "goblin-1"})
	b := visible(1, 0, perception.Placement{EntityID: "skeleton-1"})

	diff := handler.DiffKnowledge(nil, knowledge(a, b))

	s.Equal([]core.EntityID{"goblin-1", "skeleton-1"}, diff.Entities)
}

func (s *KnowledgeSuite) TestOutputIsDeterministic() {
	// Go randomizes map iteration. Without sorting, the same knowledge change
	// would serialize differently run to run.
	k := knowledge(visible(2, 0), visible(0, 0), visible(1, 0), visible(-1, 0))

	first := handler.DiffKnowledge(nil, k)
	for i := 0; i < 20; i++ {
		s.Equal(first.Hexes, handler.DiffKnowledge(nil, k).Hexes)
	}
}

func (s *KnowledgeSuite) TestAHexVanishingFromKnowledgeIsNotADeletion() {
	// Nothing is ever deleted, so a conforming toolkit cannot produce this. If
	// it happens anyway, the diff must not invent a removal — there is no wire
	// form for one, and a local forget is exactly the operation the contract
	// removes.
	before := knowledge(remembered(visible(0, 0, skeletonFacingNorth)))

	diff := handler.DiffKnowledge(before, knowledge())

	s.Empty(diff.Hexes)
}

func (s *KnowledgeSuite) TestKnowledgeIsPerViewer() {
	// Two viewers of the same world hold different memories of the same hex.
	// The interface is keyed by viewer for exactly this reason; a single shared
	// map could not express it.
	k := &fakeKnowledge{perViewer: map[core.PlayerID]map[core.Hex]perception.HexObservation{
		"alice": knowledge(visible(0, 0, skeletonFacingNorth)),
		"bob":   knowledge(remembered(visible(0, 0))),
	}}

	s.Equal(perception.KnowledgeStateVisible, k.KnownHexes("alice")[hex(0, 0)].State)
	s.Equal(perception.KnowledgeStateRemembered, k.KnownHexes("bob")[hex(0, 0)].State)
	s.Empty(k.KnownHexes("bob")[hex(0, 0)].Contents)

	// A viewer who has observed nothing has no entries — omission is UNSEEN.
	s.Empty(k.KnownHexes("carol"))
}
