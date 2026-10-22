package session_test

import (
	"context"
	_ "embed"

	"google.golang.org/protobuf/proto"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	storypb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
)

// Two floor cells, (1,0) and (2,0), split by the ordinary-discovery wall and
// its closed gate: a seat on either side is walled into its own hex.
//
//go:embed testdata/walled_seats.yaml
var walledSeatsSource string

// seatPlayers saves one armed fighter per member, each controlled by its own
// player, and returns each member's authenticated context.
func (s *StructuralPatchAcceptanceSuite) seatPlayers(h *acceptanceHarness, members ...string) map[string]context.Context {
	out := make(map[string]context.Context, len(members))
	for _, member := range members {
		ctx := auth.WithPlayerID(context.Background(), "player-"+member)
		_, err := h.charRepo.Create(ctx, characterrepo.CreateInput{Character: &entities.Character{Data: armedFighter(member, "player-"+member)}})
		s.Require().NoError(err)
		out[member] = ctx
	}
	return out
}

// TestGetViewCarriesEachSeatsOwnVisibleCells is fog of war R1 on the wire
// (rpg-project#586): GetView answers seen-now for the caller's seat alone,
// so two seats on either side of a closed gate read two different lists.
func (s *StructuralPatchAcceptanceSuite) TestGetViewCarriesEachSeatsOwnVisibleCells() {
	h := newAcceptanceHarness(s.T())
	ctx := s.seatPlayers(h, "alice", "bob")
	dungeon, err := sessionworld.Compile([]byte(ordinaryDiscoverySource))
	s.Require().NoError(err)
	h.launch(s.T(), "seen-now-run", dungeon.Spec, seatAt("alice", 0, 0), seatAt("bob", 3, 0))

	alice, err := h.handler.GetView(ctx["alice"], &sessionpb.GetViewRequest{Session: "seen-now-run", Member: "alice"})
	s.Require().NoError(err)
	bob, err := h.handler.GetView(ctx["bob"], &sessionpb.GetViewRequest{Session: "seen-now-run", Member: "bob"})
	s.Require().NoError(err)

	s.requireCells([]*sessionpb.Position{pbAt(0, 0), pbAt(1, 0)}, alice.VisibleCells, "alice sees her side of the gate")
	s.requireCells([]*sessionpb.Position{pbAt(2, 0), pbAt(3, 0), pbAt(4, 0)}, bob.VisibleCells, "bob sees his side, never a party union")

	_, err = h.handler.GetView(ctx["alice"], &sessionpb.GetViewRequest{Session: "seen-now-run", Member: "bob"})
	s.Require().Error(err, "a caller cannot read another member's seen-now")
}

// TestGetViewAWalledInSeatStillCarriesItsOwnCell pins the contract the web
// relies on (plan Task 4, 2026-10-15): visible_cells is never empty for a
// seated member. A seat that sees nothing else still receives its own cell,
// so an EMPTY list can only mean a server that sends no visible cells.
func (s *StructuralPatchAcceptanceSuite) TestGetViewAWalledInSeatStillCarriesItsOwnCell() {
	h := newAcceptanceHarness(s.T())
	ctx := s.seatPlayers(h, "alice", "bob")
	dungeon, err := sessionworld.Compile([]byte(walledSeatsSource))
	s.Require().NoError(err)
	h.launch(s.T(), "walled-run", dungeon.Spec, seatAt("alice", 1, 0), seatAt("bob", 2, 0))

	alice, err := h.handler.GetView(ctx["alice"], &sessionpb.GetViewRequest{Session: "walled-run", Member: "alice"})
	s.Require().NoError(err)
	s.requireCells([]*sessionpb.Position{pbAt(1, 0)}, alice.VisibleCells, "the walled-in seat carries its own cell")

	knowledge, err := h.handler.GetKnowledge(ctx["bob"], &sessionpb.GetKnowledgeRequest{Session: "walled-run", Member: "bob"})
	s.Require().NoError(err)
	s.requireCells([]*sessionpb.Position{pbAt(2, 0)}, knowledge.View.VisibleCells, "the knowledge snapshot's view carries the same answer")
}

// TestOpeningAGateStreamsEachLearnerItsOwnCellsRevealed: one learning moment,
// one CELLS perception per learner on that learner's own story, each carrying
// only the cells that recipient newly learned. The story is the stream's own
// projection (StoryService's GetStory is the live stream's catch-up), so this
// is what each member's StreamEvents delivered.
func (s *StructuralPatchAcceptanceSuite) TestOpeningAGateStreamsEachLearnerItsOwnCellsRevealed() {
	h := newAcceptanceHarness(s.T())
	ctx := s.seatPlayers(h, "alice", "bob")
	dungeon, err := sessionworld.Compile([]byte(ordinaryDiscoverySource))
	s.Require().NoError(err)
	h.launch(s.T(), "learn-run", dungeon.Spec, seatAt("alice", 1, 0), seatAt("bob", 3, 0))

	before := map[string]int{}
	for _, member := range []string{"alice", "bob"} {
		before[member] = len(storyOf(ctx[member], s.T(), h.story, "learn-run", member))
	}
	_, err = h.handler.OpenDoor(ctx["alice"], &sessionpb.OpenDoorRequest{Session: "learn-run", Member: "alice", Door: "ordinary-discovery/gate"})
	s.Require().NoError(err)

	learned := map[string][]*sessionpb.Position{
		"alice": {pbAt(2, 0), pbAt(3, 0), pbAt(4, 0)},
		"bob":   {pbAt(0, 0), pbAt(1, 0)},
	}
	for member, want := range learned {
		story := storyOf(ctx[member], s.T(), h.story, "learn-run", member)
		var reveals []*storypb.CellsLearned
		for _, act := range story[before[member]:] {
			if cells := act.GetPerception().GetCells(); cells != nil {
				s.Equal(storypb.Family_FAMILY_PERCEPTION, act.GetFamily())
				s.Equal(storypb.Sense_SENSE_CELLS, act.GetPerception().GetSense())
				s.Equal(member, act.GetPerception().GetObserver().GetId(), "%s: the learner's own perception", member)
				reveals = append(reveals, cells)
			}
		}
		s.Require().Len(reveals, 1, "%s: one learning moment, one CELLS perception", member)
		got := make([]*sessionpb.Position, 0, len(want))
		for _, region := range reveals[0].Regions {
			s.NotEmpty(region.Id, "%s: a partial region still names the region it unions into", member)
			got = append(got, v1Cells(region.Cells)...)
		}
		s.Require().Len(got, len(want), "%s receives only the cells it newly learned: %v", member, got)
		for _, cell := range want {
			s.True(containsCell(got, cell), "%s learned %v", member, cell)
		}
	}
}

// requireCells is an exact, ordered match: the wire copies the encounter's
// sorted answer and never reorders it.
func (s *StructuralPatchAcceptanceSuite) requireCells(want, got []*sessionpb.Position, msg string) {
	s.T().Helper()
	s.Require().Len(got, len(want), "%s: got %v", msg, got)
	for i := range want {
		s.Truef(proto.Equal(want[i], got[i]), "%s: cell %d is %v, want %v", msg, i, got[i], want[i])
	}
}
