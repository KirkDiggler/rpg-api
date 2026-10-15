package session_test

import (
	"context"
	_ "embed"
	"math"

	"google.golang.org/protobuf/proto"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
)

// The floor and books carry no concealment declarations.
//
//go:embed testdata/ordinary_discovery.yaml
var ordinaryDiscoverySource string

func (s *StructuralPatchAcceptanceSuite) TestOrdinaryClosedDoorWithholdsFloorAndPropsOnTheWire() {
	h := newAcceptanceHarness(s.T())
	ctx := auth.WithPlayerID(context.Background(), "player-alice")
	_, err := h.charRepo.Create(ctx, characterrepo.CreateInput{Character: &entities.Character{Data: armedFighter("alice", "player-alice")}})
	s.Require().NoError(err)
	dungeon, err := sessionworld.Compile([]byte(ordinaryDiscoverySource))
	s.Require().NoError(err)
	h.launch(s.T(), "ordinary-run", dungeon.Spec, seatAt("alice", 1, 0))
	request := &sessionpb.GetKnowledgeRequest{Session: "ordinary-run", Member: "alice"}
	before, err := h.handler.GetKnowledge(ctx, request)
	s.Require().NoError(err)
	for _, cell := range before.Atlas.Cells {
		s.False(proto.Equal(cell, pbAt(4, 0)), "undiscovered far floor is absent from the wire")
	}
	for _, prop := range before.Atlas.Placed {
		s.NotEqual("books", prop.Id, "ordinary far-side props need no concealment")
	}
	s.Require().Len(before.Atlas.PropPresentations, 2, "unblocked decoration is still a renderable world object")
	s.Equal("altar", before.Atlas.PropPresentations[0].Id)
	vase := before.Atlas.PropPresentations[1]
	s.Equal("vase", vase.Id)
	s.Equal("dnd5e:props:vase", vase.Ref)
	s.InDelta(0.2*5/math.Sqrt(3), vase.Origin.X, 1e-12)
	s.InDelta(0.4*5/math.Sqrt(3), vase.Elevation, 1e-12)
	s.InDelta(-0.3*180/math.Pi, vase.FacingDegrees, 1e-12)
	s.Equal(1.2, vase.HeightScale)
	s.Require().Len(before.Atlas.StructuralWalls, 1)
	s.Require().Len(before.Atlas.StructuralDoors, 1)
	beforeStory, err := h.handler.GetStory(ctx, &sessionpb.GetStoryRequest{Session: "ordinary-run", Member: "alice"})
	s.Require().NoError(err)
	_, err = h.handler.OpenDoor(ctx, &sessionpb.OpenDoorRequest{Session: "ordinary-run", Member: "alice", Door: "ordinary-discovery/gate"})
	s.Require().NoError(err)
	after, err := h.handler.GetKnowledge(ctx, request)
	s.Require().NoError(err)
	s.Len(after.Atlas.Cells, 5)
	s.Require().Len(after.Atlas.PropPresentations, 3)
	s.Equal("books", after.Atlas.PropPresentations[1].Id)
	s.Equal("dnd5e:props:books", after.Atlas.PropPresentations[1].Ref)
	found := false
	for _, prop := range after.Atlas.Placed {
		if prop.Id == "books" {
			found = true
		}
	}
	s.True(found)
	story, err := h.handler.GetStory(ctx, &sessionpb.GetStoryRequest{Session: "ordinary-run", Member: "alice"})
	s.Require().NoError(err)
	var reveal *sessionpb.CellsRevealed
	s.Require().GreaterOrEqual(len(story.Entries), len(beforeStory.Entries))
	for _, entry := range story.Entries[len(beforeStory.Entries):] {
		if body := entry.GetCellsRevealed(); body != nil {
			s.Nil(reveal)
			reveal = body
		}
	}
	s.Require().NotNil(reveal, "normal opening delivers the cells it newly permits (rpg-project#586)")
	s.Require().Len(reveal.Regions, 1)
	s.Len(reveal.Regions[0].Cells, 3)
	s.Require().Len(reveal.PropPresentations, 1)
	s.True(proto.Equal(after.Atlas.PropPresentations[1], reveal.PropPresentations[0]), "event-only appearance equals the fresh snapshot")
	found = false
	for _, prop := range reveal.Placed {
		if prop.Id == "books" {
			found = true
		}
	}
	s.True(found, "the event carries the newly permitted prop identity")
	// REGIONS ARE PARTIAL: the cached region united with the revealed cells,
	// by id, is the fresh snapshot's region -- a replace would lose the two
	// cells this member already knew.
	beforeRegion := map[string]*sessionpb.AtlasRegion{}
	for _, region := range before.Atlas.Regions {
		beforeRegion[region.Id] = region
	}
	for _, region := range after.Atlas.Regions {
		if region.Id != reveal.Regions[0].Id {
			continue
		}
		var prior []*sessionpb.Position
		if known := beforeRegion[region.Id]; known != nil {
			prior = known.Cells
		}
		s.Len(region.Cells, len(prior)+len(reveal.Regions[0].Cells))
		for _, cell := range append(append([]*sessionpb.Position{}, prior...), reveal.Regions[0].Cells...) {
			s.True(containsCell(region.Cells, cell), "union by id: %v is in the fresh region", cell)
		}
	}
	_, err = h.handler.CloseDoor(ctx, &sessionpb.CloseDoorRequest{Session: "ordinary-run", Member: "alice", Door: "ordinary-discovery/gate"})
	s.Require().NoError(err)
	// Every manager call reloads from the repositories: this is persisted memory,
	// not an in-process Encounter reused after closing.
	remembered, err := h.handler.GetKnowledge(ctx, request)
	s.Require().NoError(err)
	s.True(proto.Equal(after.Atlas, remembered.Atlas))
	replay, err := h.handler.GetStory(ctx, &sessionpb.GetStoryRequest{Session: "ordinary-run", Member: "alice"})
	s.Require().NoError(err)
	for i, entry := range story.Entries {
		s.True(proto.Equal(entry, replay.Entries[i]), "later knowledge must not rewrite delivered history")
	}
}

func containsCell(cells []*sessionpb.Position, want *sessionpb.Position) bool {
	for _, cell := range cells {
		if proto.Equal(cell, want) {
			return true
		}
	}
	return false
}
