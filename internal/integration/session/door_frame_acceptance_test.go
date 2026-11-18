package session_test

import (
	"context"
	"os"
	"strings"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
)

func (s *GroundAcceptanceSuite) TestNativeDoorFrameSurvivesColdHostAfterSourceEdit() {
	const gray = "dnd5e:material:fantasy-kingdom:wall_large_brick_01"
	const stucco = "dnd5e:material:fantasy-kingdom:wall_stucco_01"
	h := newAcceptanceHarness(s.T())
	ctx := auth.WithPlayerID(context.Background(), "player-alice")
	_, err := h.charRepo.Create(ctx, characterrepo.CreateInput{Character: &entities.Character{Data: armedFighter("alice", "player-alice")}})
	s.Require().NoError(err)
	raw, err := os.ReadFile("testdata/door-frame.yaml")
	s.Require().NoError(err)
	compiled, err := sessionworld.Compile(raw)
	s.Require().NoError(err)
	h.launchAtStart(s.T(), "door-frame-run", compiled.Spec, "alice")
	request := &sessionpb.GetKnowledgeRequest{Session: "door-frame-run", Member: "alice"}
	before, err := h.handler.GetKnowledge(ctx, request)
	s.Require().NoError(err)
	s.Require().Len(before.Atlas.StructuralDoors, 1)
	frame := before.Atlas.StructuralDoors[0].FramePresentation
	s.Require().NotNil(frame)
	s.Require().Len(frame.FaceFinishes, 2)
	s.Equal(gray, frame.FaceFinishes[0].MaterialRef)
	s.Require().NotNil(frame.FaceFinishes[0].PhaseU)
	s.Equal(0.8, *frame.FaceFinishes[0].PhaseU)
	s.Require().Len(before.Atlas.StructuralWalls, 1)
	s.Require().Len(before.Atlas.StructuralWalls[0].Openings, 1)
	opening := before.Atlas.StructuralWalls[0].Openings[0]
	s.Equal(frame.Span, opening.FrameSpan)
	s.Greater(frame.Span.Before+frame.Span.After, opening.Width, "visual replacement does not widen the clear opening")
	later, err := sessionworld.Compile([]byte(strings.ReplaceAll(string(raw), gray, stucco)))
	s.Require().NoError(err)
	s.Equal(stucco, later.Spec.Field.StructuralWalls[0].Openings[0].Door.FramePresentation.FaceFinishes[0].MaterialRef)
	compiled.Spec.Field.StructuralWalls[0].Openings[0].Door.FramePresentation.FaceFinishes[0].MaterialRef = stucco
	compiled.Spec.Field.StructuralWalls[0].Openings[0].Door.FramePresentation.FaceFinishes[0].PhaseU = 0.1
	coldSites, err := sessionorch.NewSites(nil)
	s.Require().NoError(err)
	fresh := freshSessionHandler(s.T(), h, coldSites)
	after, err := fresh.GetKnowledge(ctx, request)
	s.Require().NoError(err)
	s.Equal(1, coldSites.Compiles(), "reload the immutable captured site from Redis with no source provider")
	s.Equal(before.Atlas.StructuralDoors, after.Atlas.StructuralDoors)
	s.Equal(before.Atlas.StructuralWalls, after.Atlas.StructuralWalls)
	s.Equal(before.Atlas.Cells, after.Atlas.Cells)
}
