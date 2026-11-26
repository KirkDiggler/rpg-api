package session_test

import (
	"context"
	"math"
	"os"
	"strings"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
)

func (s *GroundAcceptanceSuite) TestNativeWallSectionsReachBothClientsAndColdHostWithoutSourceEnrichment() {
	const window = "dnd5e:env:fantasy-kingdom:castle_wall_window_03"
	const laterWindow = "dnd5e:env:fantasy-kingdom:castle_wall_window_fancy_01"
	const run = "wall-sections-run"
	h := newAcceptanceHarness(s.T())
	for _, member := range []string{"alice", "bob"} {
		ctx := auth.WithPlayerID(context.Background(), "player-"+member)
		_, err := h.charRepo.Create(ctx, characterrepo.CreateInput{Character: &entities.Character{Data: armedFighter(member, "player-"+member)}})
		s.Require().NoError(err)
	}
	raw, err := os.ReadFile("testdata/wall-sections.yaml")
	s.Require().NoError(err)
	compiled, err := sessionworld.Compile(raw)
	s.Require().NoError(err)
	h.launchAtStart(s.T(), run, compiled.Spec, "alice", "bob")
	snapshots := make(map[string]*sessionpb.GetKnowledgeResponse)
	for _, member := range []string{"alice", "bob"} {
		ctx := auth.WithPlayerID(context.Background(), "player-"+member)
		snapshot, readErr := h.handler.GetKnowledge(ctx, &sessionpb.GetKnowledgeRequest{Session: run, Member: member})
		s.Require().NoError(readErr)
		s.Require().Len(snapshot.Atlas.StructuralWalls, 1)
		wall := snapshot.Atlas.StructuralWalls[0]
		s.Require().Len(wall.Sections, 1)
		s.Require().Len(wall.Openings, 1)
		s.Equal(window, wall.Sections[0].Ref)
		s.InDelta(9*5/math.Sqrt(3), wall.Sections[0].Position, 1e-10)
		s.InDelta(3.7499998211860657*5/math.Sqrt(3), wall.Sections[0].Width, 1e-10)
		s.Require().Len(snapshot.Atlas.StructuralDoors, 1)
		snapshots[member] = snapshot
	}
	later, err := sessionworld.Compile([]byte(strings.ReplaceAll(string(raw), window, laterWindow)))
	s.Require().NoError(err)
	s.Equal(laterWindow, later.Spec.Field.StructuralWalls[0].Sections[0].Ref)
	compiled.Spec.Field.StructuralWalls[0].Sections[0].Ref = laterWindow
	coldSites, err := sessionorch.NewSites(nil)
	s.Require().NoError(err)
	fresh := freshSessionHandler(s.T(), h, coldSites)
	for _, member := range []string{"alice", "bob"} {
		ctx := auth.WithPlayerID(context.Background(), "player-"+member)
		after, readErr := fresh.GetKnowledge(ctx, &sessionpb.GetKnowledgeRequest{Session: run, Member: member})
		s.Require().NoError(readErr)
		s.Equal(snapshots[member].Atlas.StructuralWalls, after.Atlas.StructuralWalls)
		s.Equal(snapshots[member].Atlas.StructuralDoors, after.Atlas.StructuralDoors)
		s.Equal(snapshots[member].Atlas.Cells, after.Atlas.Cells)
	}
	s.Equal(1, coldSites.Compiles(), "one immutable site loaded from persisted data, not current authoring source")
}
